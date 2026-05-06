// Package client talks to an upstream imgproxy server. It builds option
// strings via Spec.Build, signs the URL when configured, and streams the
// processed image bytes back to the caller.
package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a thin wrapper around http.Client targeting an imgproxy upstream.
type Client struct {
	upstream string // e.g. http://localhost:8080
	bearer   string
	signer   *Signer
	hc       *http.Client
}

// Options for the client constructor.
type Options struct {
	UpstreamURL  string
	Bearer       string
	KeysHex      string
	SaltsHex     string
	SignatureLen int
	Timeout      time.Duration
}

// New builds a Client. Returns an error if the URL is missing or signer parse
// fails.
func New(opts Options) (*Client, error) {
	up := strings.TrimRight(strings.TrimSpace(opts.UpstreamURL), "/")
	if up == "" {
		return nil, errors.New("imgproxy upstream URL is required")
	}
	signer, err := NewSigner(opts.KeysHex, opts.SaltsHex, opts.SignatureLen)
	if err != nil {
		return nil, err
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	return &Client{
		upstream: up,
		bearer:   strings.TrimSpace(opts.Bearer),
		signer:   signer,
		hc:       &http.Client{Timeout: opts.Timeout},
	}, nil
}

// Upstream returns the configured upstream root.
func (c *Client) Upstream() string { return c.upstream }

// BuildPath assembles "/{sig}/{opts}/{plain|base64-source}".
//
// usePlain controls source encoding: true → "plain/<src>", false → base64url.
func (c *Client) BuildPath(spec *Spec, source string, usePlain bool) (string, error) {
	optsStr, err := spec.Build()
	if err != nil {
		return "", err
	}
	var src string
	if usePlain {
		src = "plain/" + source
	} else {
		src = EncodeSourceURL(source)
	}
	body := "/"
	if optsStr != "" {
		body = "/" + optsStr + "/" + src
	} else {
		body = "/" + src
	}
	sig := "_"
	if c.signer != nil {
		sig = c.signer.Sign(body)
	}
	return "/" + sig + body, nil
}

// FullURL is the absolute upstream URL for a given source + spec.
func (c *Client) FullURL(spec *Spec, source string, usePlain bool) (string, error) {
	p, err := c.BuildPath(spec, source, usePlain)
	if err != nil {
		return "", err
	}
	return c.upstream + p, nil
}

// Result is one fetched image.
type Result struct {
	ContentType string
	Bytes       []byte
	StatusCode  int
}

// Fetch GETs the processed image. Caller may pass ctx to cancel.
func (c *Client) Fetch(ctx context.Context, spec *Spec, source string, usePlain bool) (*Result, error) {
	url, err := c.FullURL(spec, source, usePlain)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("imgproxy request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read imgproxy body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 256 {
			msg = msg[:256] + "…"
		}
		return &Result{StatusCode: resp.StatusCode, Bytes: body, ContentType: resp.Header.Get("Content-Type")},
			fmt.Errorf("imgproxy %d: %s", resp.StatusCode, msg)
	}
	return &Result{
		ContentType: resp.Header.Get("Content-Type"),
		Bytes:       body,
		StatusCode:  resp.StatusCode,
	}, nil
}

// Ping issues a GET /health against the upstream. Returns nil on 2xx.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.upstream+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("imgproxy health %d", resp.StatusCode)
	}
	return nil
}
