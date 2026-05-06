package server

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/imgproxy/imgproxy/v3/web/client"
	"github.com/imgproxy/imgproxy/v3/web/schema"
)

// handleHealthz reports sidecar readiness and pings upstream imgproxy.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	upstream := "ok"
	if err := s.client.Ping(ctx); err != nil {
		upstream = err.Error()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"upstream": upstream,
		"upstream_url": s.client.Upstream(),
	})
}

// handleOptions returns the static UI catalog.
func (s *Server) handleOptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, schema.Build())
}

// convertURLRequest is the JSON body for /api/convert-url.
type convertURLRequest struct {
	URLs []string     `json:"urls"`
	Spec *client.Spec `json:"spec"`
}

// handleConvertURL fetches each remote URL via imgproxy and ZIPs results.
func (s *Server) handleConvertURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	var body convertURLRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(body.URLs) == 0 {
		http.Error(w, "urls is required", http.StatusBadRequest)
		return
	}
	if len(body.URLs) > s.cfg.MaxBatch {
		http.Error(w, fmt.Sprintf("batch limit %d exceeded", s.cfg.MaxBatch), http.StatusRequestEntityTooLarge)
		return
	}
	if body.Spec == nil {
		body.Spec = &client.Spec{}
	}
	jobs := make([]job, len(body.URLs))
	for i, u := range body.URLs {
		u = strings.TrimSpace(u)
		jobs[i] = job{
			source:    u,
			usePlain:  true,
			spec:      body.Spec,
			origName:  guessName(u),
		}
	}
	s.streamZip(w, r, "imgproxy-batch.zip", jobs)
}

// handleConvert handles multipart upload + spec → ZIP.
func (s *Server) handleConvert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadSize*int64(s.cfg.MaxBatch)+10<<20)
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "parse multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	// Spec is JSON in field "spec".
	specRaw := strings.TrimSpace(r.FormValue("spec"))
	spec := &client.Spec{}
	if specRaw != "" {
		if err := json.Unmarshal([]byte(specRaw), spec); err != nil {
			http.Error(w, "invalid spec json: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		// Allow alternate field name "files".
		files = r.MultipartForm.File["files"]
	}
	if len(files) == 0 {
		http.Error(w, "no files uploaded (field 'file')", http.StatusBadRequest)
		return
	}
	if len(files) > s.cfg.MaxBatch {
		http.Error(w, fmt.Sprintf("batch limit %d exceeded", s.cfg.MaxBatch), http.StatusRequestEntityTooLarge)
		return
	}

	jobs := make([]job, 0, len(files))
	cleanup := make([]string, 0, len(files))
	defer func() {
		for _, p := range cleanup {
			_ = os.Remove(p)
		}
	}()

	for _, fh := range files {
		if fh.Size > s.cfg.MaxUploadSize {
			http.Error(w, fmt.Sprintf("%s exceeds %d bytes", fh.Filename, s.cfg.MaxUploadSize), http.StatusRequestEntityTooLarge)
			return
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(fh.Filename), "."))
		id := newID()
		if ext != "" {
			id += "." + ext
		}
		dst := filepath.Join(s.cfg.UploadDir, id)
		if err := saveUpload(fh, dst); err != nil {
			http.Error(w, "save upload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		cleanup = append(cleanup, dst)
		jobs = append(jobs, job{
			source:   "local:///" + id,
			usePlain: true,
			spec:     spec,
			origName: fh.Filename,
		})
	}

	zipName := "imgproxy-batch.zip"
	s.streamZip(w, r, zipName, jobs)
}

// job is one image to fetch + add to the zip.
type job struct {
	source   string
	usePlain bool
	spec     *client.Spec
	origName string
}

// streamZip processes jobs concurrently and writes a streaming ZIP to w.
//
// Errors per file are written to a manifest entry inside the zip ("errors.txt")
// rather than aborting the whole batch.
func (s *Server) streamZip(w http.ResponseWriter, r *http.Request, zipName string, jobs []job) {
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+zipName+`"`)
	zw := zip.NewWriter(w)
	defer zw.Close()

	type result struct {
		idx     int
		name    string
		ct      string
		bytes   []byte
		err     error
	}
	results := make([]result, len(jobs))

	sem := make(chan struct{}, s.cfg.Concurrency)
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	for i, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, j job) {
			defer wg.Done()
			defer func() { <-sem }()
			res, err := s.client.Fetch(ctx, j.spec, j.source, j.usePlain)
			results[i] = result{idx: i, name: outputName(j, i), err: err}
			if err == nil {
				results[i].bytes = res.Bytes
				results[i].ct = res.ContentType
			} else if res != nil {
				results[i].bytes = res.Bytes
				results[i].ct = res.ContentType
			}
		}(i, j)
	}
	wg.Wait()

	var errBuf strings.Builder
	for _, r := range results {
		if r.err != nil {
			fmt.Fprintf(&errBuf, "[%d] %s: %v\n", r.idx, r.name, r.err)
			continue
		}
		header := &zip.FileHeader{
			Name:     r.name,
			Method:   zip.Store, // images already compressed; skip deflate
			Modified: time.Now(),
		}
		fw, err := zw.CreateHeader(header)
		if err != nil {
			fmt.Fprintf(&errBuf, "[%d] zip header %s: %v\n", r.idx, r.name, err)
			continue
		}
		if _, err := fw.Write(r.bytes); err != nil {
			fmt.Fprintf(&errBuf, "[%d] zip write %s: %v\n", r.idx, r.name, err)
		}
	}
	if errBuf.Len() > 0 {
		fw, err := zw.Create("errors.txt")
		if err == nil {
			io.WriteString(fw, errBuf.String())
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// guessName extracts a sensible filename for a remote URL.
func guessName(url string) string {
	url = strings.TrimSpace(url)
	if i := strings.IndexAny(url, "?#"); i >= 0 {
		url = url[:i]
	}
	base := path.Base(url)
	if base == "." || base == "/" || base == "" {
		return "image"
	}
	return base
}

// outputName picks the ZIP entry filename for a job, applying any format
// override + filename template from the spec.
func outputName(j job, idx int) string {
	base := j.origName
	if base == "" {
		base = fmt.Sprintf("image-%d", idx+1)
	}
	stem := base
	if i := strings.LastIndex(base, "."); i > 0 {
		stem = base[:i]
	}
	ext := j.spec.OutputExtension()
	if ext == "" {
		if i := strings.LastIndex(base, "."); i > 0 {
			ext = strings.ToLower(strings.TrimPrefix(base[i:], "."))
		}
	}
	if ext == "jpeg" {
		ext = "jpg"
	}
	tpl := strings.TrimSpace(j.spec.FilenameTemplate)
	if tpl == "" {
		if ext == "" {
			return stem
		}
		return stem + "." + ext
	}
	return expandTemplate(tpl, stem, ext, idx+1)
}

// expandTemplate fills {name}, {ext}, {i}, {i:0Nd} placeholders.
func expandTemplate(tpl, name, ext string, i int) string {
	tpl = strings.ReplaceAll(tpl, "{name}", name)
	tpl = strings.ReplaceAll(tpl, "{ext}", ext)
	// Fixed-width index, e.g. {i:02d}.
	for {
		start := strings.Index(tpl, "{i:")
		if start < 0 {
			break
		}
		end := strings.Index(tpl[start:], "}")
		if end < 0 {
			break
		}
		end += start
		spec := tpl[start+1 : end] // i:02d
		var width int
		// Accept both "i:02d" and "i:2".
		fmt.Sscanf(spec, "i:%dd", &width)
		if width == 0 {
			fmt.Sscanf(spec, "i:%d", &width)
		}
		if width <= 0 {
			width = 1
		}
		replacement := fmt.Sprintf("%0*d", width, i)
		tpl = tpl[:start] + replacement + tpl[end+1:]
	}
	tpl = strings.ReplaceAll(tpl, "{i}", strconv.Itoa(i))
	return tpl
}

// saveUpload writes a multipart file to disk.
func saveUpload(fh *multipart.FileHeader, dst string) error {
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, src)
	return err
}
