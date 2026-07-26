// Command server runs the Typst-based CV renderer HTTP service.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Melancholic/cvrenderer/internal/config"
	"github.com/Melancholic/cvrenderer/internal/server"
)

func main() {
	cfg := config.Load()
	srv := server.New(cfg)
	httpSrv := srv.HTTPServer()

	go func() {
		log.Printf("listening on %s (root=%s templates=%s data=%s)", cfg.Addr, cfg.Root, cfg.TemplateDir, cfg.DataDir)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	log.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
		os.Exit(1)
	}
}
