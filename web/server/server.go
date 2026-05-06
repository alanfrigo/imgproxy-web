// Package server hosts the imgproxy-web HTTP API and static UI.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/imgproxy/imgproxy/v3/web/client"
	"github.com/imgproxy/imgproxy/v3/web/config"
	"github.com/imgproxy/imgproxy/v3/web/static"
)

// Server wires deps and exposes routes.
type Server struct {
	cfg    *config.Config
	client *client.Client
	mux    *http.ServeMux
}

// New constructs a Server from config.
func New(cfg *config.Config) (*Server, error) {
	c, err := client.New(client.Options{
		UpstreamURL:  cfg.UpstreamURL,
		Bearer:       cfg.Bearer,
		KeysHex:      cfg.KeysHex,
		SaltsHex:     cfg.SaltsHex,
		SignatureLen: cfg.SignatureLen,
		Timeout:      cfg.Timeout,
	})
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, client: c, mux: http.NewServeMux()}
	s.registerRoutes()
	return s, nil
}

func (s *Server) registerRoutes() {
	staticFS, err := fs.Sub(static.FS, ".")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(staticFS))

	s.mux.HandleFunc("/api/healthz", s.handleHealthz)
	s.mux.HandleFunc("/api/options", s.handleOptions)
	s.mux.HandleFunc("/api/convert", s.handleConvert)
	s.mux.HandleFunc("/api/convert-url", s.handleConvertURL)
	s.mux.Handle("/static/", http.StripPrefix("/static/", fileServer))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			data, err := static.FS.ReadFile("index.html")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Write(data)
			return
		}
		http.NotFound(w, r)
	})
}

// ServeHTTP implements http.Handler with CORS + access log.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AllowedOrigin != "" {
		w.Header().Set("Access-Control-Allow-Origin", s.cfg.AllowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	start := time.Now()
	rw := &statusRecorder{ResponseWriter: w, status: 200}
	s.mux.ServeHTTP(rw, r)
	log.Printf("%s %s → %d %s", r.Method, r.URL.Path, rw.status, time.Since(start).Truncate(time.Millisecond))
}

// Run starts the HTTP server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.Bind,
		Handler:           s,
		ReadHeaderTimeout: 15 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("imgproxy-web listening on %s (upstream %s, upload-dir %s)", s.cfg.Bind, s.cfg.UpstreamURL, s.cfg.UploadDir)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// statusRecorder captures the response status for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// newID returns a 16-hex-char random id, used for upload filenames.
func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000"), ".", "")
	}
	return hex.EncodeToString(b[:])
}
