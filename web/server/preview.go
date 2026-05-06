package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imgproxy/imgproxy/v3/web/client"
)

// handlePreview returns a single processed image for the live UI preview.
//
// Two request shapes are accepted:
//   - multipart/form-data with one "file" + "spec" JSON  → uploads then fetches
//     via local:// scheme.
//   - application/json with {"url": "...", "spec": {...}} → passes the URL
//     straight to imgproxy as a plain source.
//
// Response is the raw imgproxy bytes (Content-Type echoed). The frontend
// renders this directly in an <img> via createObjectURL.
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ct := r.Header.Get("Content-Type")
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	var (
		spec     *client.Spec
		source   string
		usePlain bool
		cleanup  func()
		err      error
	)
	if strings.HasPrefix(ct, "application/json") {
		spec, source, usePlain, cleanup, err = previewFromJSON(r)
	} else {
		spec, source, usePlain, cleanup, err = s.previewFromMultipart(w, r)
	}
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		var sc statusErr
		if errors.As(err, &sc) {
			http.Error(w, sc.msg, sc.code)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	res, err := s.client.Fetch(ctx, spec, source, usePlain)
	if err != nil {
		// res still contains upstream body on non-2xx — surface it.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, err)
		return
	}
	w.Header().Set("Content-Type", res.ContentType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Bytes", fmt.Sprintf("%d", len(res.Bytes)))
	w.Write(res.Bytes)
}

type statusErr struct {
	code int
	msg  string
}

func (e statusErr) Error() string { return e.msg }

// previewFromJSON parses {"url": "...", "spec": {...}}.
func previewFromJSON(r *http.Request) (*client.Spec, string, bool, func(), error) {
	defer r.Body.Close()
	var body struct {
		URL  string       `json:"url"`
		Spec *client.Spec `json:"spec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, "", false, nil, statusErr{http.StatusBadRequest, "invalid json: " + err.Error()}
	}
	if strings.TrimSpace(body.URL) == "" {
		return nil, "", false, nil, statusErr{http.StatusBadRequest, "url is required"}
	}
	if body.Spec == nil {
		body.Spec = &client.Spec{}
	}
	return body.Spec, body.URL, true, nil, nil
}

// previewFromMultipart parses one upload + spec field, writes the file to
// UPLOAD_DIR with a random name, and returns a local:// source URL.
func (s *Server) previewFromMultipart(w http.ResponseWriter, r *http.Request) (*client.Spec, string, bool, func(), error) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadSize+10<<20)
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		return nil, "", false, nil, statusErr{http.StatusBadRequest, "parse multipart: " + err.Error()}
	}
	specRaw := strings.TrimSpace(r.FormValue("spec"))
	spec := &client.Spec{}
	if specRaw != "" {
		if err := json.Unmarshal([]byte(specRaw), spec); err != nil {
			return nil, "", false, nil, statusErr{http.StatusBadRequest, "invalid spec: " + err.Error()}
		}
	}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		return nil, "", false, nil, statusErr{http.StatusBadRequest, "field 'file' required"}
	}
	fh := files[0]
	if fh.Size > s.cfg.MaxUploadSize {
		return nil, "", false, nil, statusErr{http.StatusRequestEntityTooLarge,
			fmt.Sprintf("%s exceeds %d bytes", fh.Filename, s.cfg.MaxUploadSize)}
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(fh.Filename), "."))
	name := newID()
	if ext != "" {
		name += "." + ext
	}
	dst := filepath.Join(s.cfg.UploadDir, name)
	if err := saveUpload(fh, dst); err != nil {
		return nil, "", false, nil, statusErr{http.StatusInternalServerError, "save: " + err.Error()}
	}
	cleanup := func() {
		_ = os.Remove(dst)
		_ = r.MultipartForm.RemoveAll()
	}
	return spec, "local:///" + name, true, cleanup, nil
}

// io.Discard alias kept to silence compilers stripping unused stdlib imports
// when the file is edited later.
var _ = io.Discard
