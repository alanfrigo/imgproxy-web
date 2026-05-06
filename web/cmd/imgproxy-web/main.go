// imgproxy-web is a sidecar HTTP service that fronts an upstream imgproxy
// server with a UI and a bulk-conversion API. It accepts multipart uploads,
// stores them in a directory shared with imgproxy via the local:// scheme, and
// streams a ZIP of processed images back to the caller.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/imgproxy/imgproxy/v3/web/config"
	"github.com/imgproxy/imgproxy/v3/web/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("server: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := srv.Run(ctx); err != nil {
		log.Fatalf("run: %v", err)
		os.Exit(1)
	}
}
