// Package server wires the HTTP layer to the typst renderer.
package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Melancholic/cvrenderer/internal/config"
	"github.com/Melancholic/cvrenderer/internal/typst"
)

type Server struct {
	cfg         config.Config
	renderer    *typst.Renderer
	renderSlots chan struct{}
	cache       *renderCache
	now         func() time.Time // overridden in tests to cross a day boundary
}

func New(cfg config.Config) *Server {
	slots := cfg.MaxConcurrentRenders
	if slots < 1 {
		slots = 1 // a zero-capacity channel would deadlock every render
	}
	return &Server{
		cfg: cfg,
		renderer: &typst.Renderer{
			Bin:             cfg.TypstBin,
			Root:            cfg.Root,
			FontDir:         cfg.FontDir,
			PackageCacheDir: cfg.PackageCacheDir,
			Timeout:         cfg.RenderTimeout,
		},
		renderSlots: make(chan struct{}, slots),
		cache:       newRenderCache(cfg.CacheTTL, cfg.CacheMaxEntries),
		now:         time.Now,
	}
}

var errRenderBusy = errors.New("render capacity exhausted")

// acquireRenderSlot bounds concurrent typst subprocesses: each is CPU-bound on
// an unauthenticated endpoint, so a burst would otherwise saturate the host.
func (s *Server) acquireRenderSlot(ctx context.Context) (release func(), err error) {
	release = func() { <-s.renderSlots }
	select {
	case s.renderSlots <- struct{}{}:
		return release, nil
	default:
	}
	timer := time.NewTimer(s.cfg.RenderQueueTimeout)
	defer timer.Stop()
	select {
	case s.renderSlots <- struct{}{}:
		return release, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, errRenderBusy
	}
}

const healthPath = "/healthz"

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+healthPath, s.handleHealth)
	// GET /cv/<template>/<name>.pdf
	mux.HandleFunc("GET /cv/{template}/{name}", s.handleCV)
	return logRequests(mux)
}

func (s *Server) HTTPServer() *http.Server {
	return &http.Server{
		Addr:              s.cfg.Addr,
		Handler:           s.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
}
