// Package config loads imgproxy-web settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBind         = ":8081"
	defaultUpstream     = "http://localhost:8080"
	defaultUploadDir    = "/tmp/imgproxy-web"
	defaultMaxUpload    = 100 << 20  // 100 MiB
	defaultMaxBatch     = 200
	defaultSignatureLen = 32
	defaultTimeout      = 60 * time.Second
)

// Config is the loaded sidecar configuration.
type Config struct {
	Bind          string
	UpstreamURL   string
	UploadDir     string
	MaxUploadSize int64
	MaxBatch      int
	Concurrency   int
	KeysHex       string
	SaltsHex      string
	SignatureLen  int
	Bearer        string
	Timeout       time.Duration
	AllowedOrigin string
}

// Load reads IMGPROXY_WEB_* env vars and returns a validated config.
func Load() (*Config, error) {
	c := &Config{
		Bind:          envStr("IMGPROXY_WEB_BIND", defaultBind),
		UpstreamURL:   envStr("IMGPROXY_WEB_IMGPROXY_URL", defaultUpstream),
		UploadDir:     envStr("IMGPROXY_WEB_UPLOAD_DIR", defaultUploadDir),
		MaxUploadSize: envInt64("IMGPROXY_WEB_MAX_UPLOAD_SIZE", defaultMaxUpload),
		MaxBatch:      envInt("IMGPROXY_WEB_MAX_BATCH", defaultMaxBatch),
		Concurrency:   envInt("IMGPROXY_WEB_CONCURRENCY", runtime.NumCPU()),
		KeysHex:       envStr("IMGPROXY_WEB_KEY", os.Getenv("IMGPROXY_KEY")),
		SaltsHex:      envStr("IMGPROXY_WEB_SALT", os.Getenv("IMGPROXY_SALT")),
		SignatureLen:  envInt("IMGPROXY_WEB_SIGNATURE_SIZE", defaultSignatureLen),
		Bearer:        envStr("IMGPROXY_WEB_BEARER", os.Getenv("IMGPROXY_SECRET")),
		Timeout:       time.Duration(envInt("IMGPROXY_WEB_TIMEOUT_SEC", int(defaultTimeout/time.Second))) * time.Second,
		AllowedOrigin: envStr("IMGPROXY_WEB_ALLOW_ORIGIN", ""),
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) validate() error {
	if c.Bind == "" {
		return errors.New("IMGPROXY_WEB_BIND empty")
	}
	if c.UpstreamURL == "" {
		return errors.New("IMGPROXY_WEB_IMGPROXY_URL empty")
	}
	if c.UploadDir == "" {
		return errors.New("IMGPROXY_WEB_UPLOAD_DIR empty")
	}
	if c.Concurrency < 1 {
		c.Concurrency = 1
	}
	if c.MaxBatch < 1 {
		c.MaxBatch = 1
	}
	if err := os.MkdirAll(c.UploadDir, 0o755); err != nil {
		return fmt.Errorf("upload dir %s: %w", c.UploadDir, err)
	}
	return nil
}

func envStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(v)
	}
	return def
}

func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return n
		}
	}
	return def
}

func envInt64(key string, def int64) int64 {
	if v, ok := os.LookupEnv(key); ok {
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err == nil {
			return n
		}
	}
	return def
}
