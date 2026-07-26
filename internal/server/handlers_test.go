package server

import (
	"bytes"
	"context"
	"errors"
	"log"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Melancholic/cvrenderer/internal/config"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	bin := os.Getenv("TYPST_BIN")
	if bin == "" {
		bin = "typst"
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Skipf("typst not installed (%v); skipping HTTP render tests", err)
	}
	cfg := config.Load()
	cfg.TypstBin = bin
	cfg.Root = repoRoot(t)
	cfg.RenderTimeout = 30 * time.Second
	// DataDir/TemplateDir keep their Load() defaults, which exist under repoRoot.
	return New(cfg)
}

func TestHealth(t *testing.T) {
	cfg := config.Load()
	srv := New(cfg)
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

func TestHealthIsNotLogged(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	srv := New(config.Load()) // neither request below needs typst
	srv.Routes().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if buf.Len() != 0 {
		t.Errorf("liveness probe was logged: %s", buf.String())
	}

	srv.Routes().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/cv/nope/none.pdf", nil))
	if !strings.Contains(buf.String(), "/cv/nope/none.pdf") {
		t.Errorf("ordinary requests must still be logged, got: %q", buf.String())
	}
}

func TestNamedCVReturnsPDF(t *testing.T) {
	srv := newTestServer(t)
	for _, tmpl := range []string{"helsinki", "primeats"} {
		t.Run(tmpl, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/"+tmpl+"/cv-example.pdf", nil))
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
			}
			if ct := rr.Header().Get("Content-Type"); ct != "application/pdf" {
				t.Errorf("Content-Type = %q, want application/pdf", ct)
			}
			if !strings.HasPrefix(rr.Body.String(), "%PDF-") {
				t.Errorf("body is not a PDF")
			}
		})
	}
}

func TestCVDisposition(t *testing.T) {
	srv := newTestServer(t)
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"default is inline", "", "inline"},
		{"bare download", "?download", "attachment"},
		{"download=1", "?download=1", "attachment"},
		{"download=true", "?download=true", "attachment"},
		{"non-boolean value still downloads", "?download=yes", "attachment"},
		{"download=0 opts back out", "?download=0", "inline"},
		{"download=false opts back out", "?download=false", "inline"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/cv/helsinki/cv-example.pdf"+c.query, nil)
			srv.Routes().ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
			}
			got := rr.Header().Get("Content-Disposition")
			disp, params, err := mime.ParseMediaType(got)
			if err != nil {
				t.Fatalf("unparseable Content-Disposition %q: %v", got, err)
			}
			if disp != c.want {
				t.Errorf("disposition = %q, want %q (header: %q)", disp, c.want, got)
			}
			if params["filename"] != "cv-example.pdf" {
				t.Errorf("filename = %q, want cv-example.pdf", params["filename"])
			}
		})
	}
}

func TestCVRouteErrors(t *testing.T) {
	srv := newTestServer(t)
	cases := []struct {
		name string
		path string
		want int
	}{
		{"missing .pdf suffix", "/cv/helsinki/cv-example", http.StatusNotFound},
		{"path traversal in name", "/cv/helsinki/..%2f..%2fetc%2fpasswd.pdf", http.StatusBadRequest},
		{"unknown cv", "/cv/helsinki/does-not-exist.pdf", http.StatusNotFound},
		{"unknown template", "/cv/nope/cv-example.pdf", http.StatusNotFound},
		{"invalid template", "/cv/..%2fsecret/cv-example.pdf", http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, c.path, nil))
			if rr.Code != c.want {
				t.Errorf("status = %d, want %d (body: %s)", rr.Code, c.want, rr.Body.String())
			}
		})
	}
}

func TestCVCaching(t *testing.T) {
	srv := newTestServer(t)
	const path = "/cv/helsinki/cv-example.pdf"
	// Pin the clock to midday so the midnight-capped max-age is deterministic.
	noon := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	srv.now = func() time.Time { return noon }

	get := func(t *testing.T, ifNoneMatch string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, req)
		return rr
	}

	first := get(t, "")
	if first.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", first.Code)
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on a rendered PDF")
	}
	// 24h TTL, but only 12h until the date-derived fields change.
	wantCC := "private, max-age=" + strconv.Itoa(12*3600)
	if got := first.Header().Get("Cache-Control"); got != wantCC {
		t.Errorf("Cache-Control = %q, want %q", got, wantCC)
	}

	second := get(t, "")
	if second.Code != http.StatusOK {
		t.Fatalf("cached status = %d, want 200", second.Code)
	}
	if got := second.Header().Get("ETag"); got != etag {
		t.Errorf("ETag changed between requests: %q then %q", etag, got)
	}
	if first.Body.String() != second.Body.String() {
		t.Error("cached response body differs from the freshly rendered one")
	}

	cond := get(t, etag)
	if cond.Code != http.StatusNotModified {
		t.Fatalf("conditional status = %d, want 304", cond.Code)
	}
	if cond.Body.Len() != 0 {
		t.Errorf("304 carried a %d-byte body", cond.Body.Len())
	}
	if cond.Header().Get("ETag") != etag {
		t.Error("304 must repeat the ETag")
	}

	if got := get(t, `"stale"`).Code; got != http.StatusOK {
		t.Errorf("stale If-None-Match status = %d, want 200", got)
	}

	// Editing the CV invalidates: the mtime is part of the key.
	dataFile := filepath.Join(srv.cfg.Root, srv.cfg.DataDir, "cv-example.yaml")
	info, err := os.Stat(dataFile)
	if err != nil {
		t.Fatal(err)
	}
	touched := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(dataFile, touched, touched); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chtimes(dataFile, info.ModTime(), info.ModTime()) })

	if got := get(t, "").Header().Get("ETag"); got == etag {
		t.Error("ETag unchanged after the CV file was modified")
	}
}

func TestCVCacheRollsOverAtMidnight(t *testing.T) {
	srv := newTestServer(t)
	day := time.Date(2026, 7, 28, 23, 59, 0, 0, time.UTC)
	srv.now = func() time.Time { return day }

	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/helsinki/cv-example.pdf", nil))
	before := rr.Header().Get("ETag")

	day = day.Add(2 * time.Minute) // 00:01 the next day
	rr = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/helsinki/cv-example.pdf", nil))
	if after := rr.Header().Get("ETag"); after == before {
		t.Error("ETag survived a day boundary; date-derived fields would go stale")
	}
}

func TestCVCacheDisabled(t *testing.T) {
	base := newTestServer(t)
	cfg := base.cfg
	cfg.CacheTTL = 0
	srv := New(cfg)

	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/helsinki/cv-example.pdf", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := rr.Header().Get("ETag"); got != "" {
		t.Errorf("ETag = %q, want none when caching is off", got)
	}
}

func TestCachedResponseSkipsRenderSlot(t *testing.T) {
	srv := newTestServer(t)
	const path = "/cv/helsinki/cv-example.pdf"

	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("warm-up status = %d, want 200", rr.Code)
	}
	etag := rr.Header().Get("ETag")

	srv.cfg.RenderQueueTimeout = 20 * time.Millisecond
	srv.renderSlots = make(chan struct{}, 1)
	srv.renderSlots <- struct{}{}

	rr = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	if rr.Code != http.StatusOK {
		t.Errorf("cache hit status = %d, want 200 (cache should bypass the slot)", rr.Code)
	}

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("If-None-Match", etag)
	rr = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotModified {
		t.Errorf("revalidation status = %d, want 304 (should bypass the slot)", rr.Code)
	}
}

// typst's error quotes template source and absolute paths; none may escape.
func TestRenderFailureIsOpaque(t *testing.T) {
	srv := newTestServer(t)
	name := "zz-broken-" + strconv.Itoa(os.Getpid())
	path := filepath.Join(srv.cfg.Root, srv.cfg.DataDir, name+".yaml")
	if err := os.WriteFile(path, []byte("name: Broken\nexperience: \"not a list\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(path) })

	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/helsinki/"+name+".pdf", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body: %s)", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, leak := range []string{"typst", ".typ", srv.cfg.Root, "experience"} {
		if strings.Contains(body, leak) {
			t.Errorf("error body leaks %q: %s", leak, body)
		}
	}
}

func TestNoIndexHeader(t *testing.T) {
	srv := newTestServer(t)
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/helsinki/cv-example.pdf", nil))
	if got := rr.Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
		t.Errorf("X-Robots-Tag = %q, want %q", got, "noindex, nofollow")
	}

	cfg := srv.cfg
	cfg.AllowIndexing = true
	rr = httptest.NewRecorder()
	New(cfg).Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/helsinki/cv-example.pdf", nil))
	if got := rr.Header().Get("X-Robots-Tag"); got != "" {
		t.Errorf("X-Robots-Tag = %q with ALLOW_INDEXING, want it absent", got)
	}
}

func TestRenderSlots(t *testing.T) {
	cfg := config.Load()
	cfg.MaxConcurrentRenders = 1
	cfg.RenderQueueTimeout = 20 * time.Millisecond
	srv := New(cfg)

	release, err := srv.acquireRenderSlot(context.Background())
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if _, err := srv.acquireRenderSlot(context.Background()); !errors.Is(err, errRenderBusy) {
		t.Errorf("second acquire err = %v, want errRenderBusy", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := srv.acquireRenderSlot(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled acquire err = %v, want context.Canceled", err)
	}
	release()
	if _, err := srv.acquireRenderSlot(context.Background()); err != nil {
		t.Errorf("acquire after release: %v", err)
	}
}

func TestBusyReturns503(t *testing.T) {
	srv := newTestServer(t)
	srv.cfg.RenderQueueTimeout = 20 * time.Millisecond
	srv.renderSlots = make(chan struct{}, 1)
	srv.renderSlots <- struct{}{} // occupy the only slot

	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/helsinki/cv-example.pdf", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
	if got := rr.Header().Get("Retry-After"); got == "" {
		t.Error("503 should carry a Retry-After header")
	}
}

func TestRemovedRenderEndpoint(t *testing.T) {
	srv := New(config.Load())
	rr := httptest.NewRecorder()
	body := strings.NewReader("name: Uploaded Person\n")
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/render", body))
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (POST /render is gone)", rr.Code)
	}
}
