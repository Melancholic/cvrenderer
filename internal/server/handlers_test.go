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

	srv.Routes().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/cv/none.pdf", nil))
	if !strings.Contains(buf.String(), "/cv/none.pdf") {
		t.Errorf("ordinary requests must still be logged, got: %q", buf.String())
	}
}

func TestNamedCVReturnsPDF(t *testing.T) {
	srv := newTestServer(t)
	for _, tmpl := range []string{"helsinki", "primeats"} {
		t.Run(tmpl, func(t *testing.T) {
			rr := httptest.NewRecorder()
			srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/cv-example.pdf?template="+tmpl, nil))
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

func TestDefaultCVRoute(t *testing.T) {
	srv := newTestServer(t)
	srv.cfg.DefaultCV = "cv-example" // the repo ships no data/cv-default.yml

	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv.pdf", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	if !strings.HasPrefix(rr.Body.String(), "%PDF-") {
		t.Error("body is not a PDF")
	}
	if disp := rr.Header().Get("Content-Disposition"); !strings.Contains(disp, srv.cfg.OutFilePrefix) {
		t.Errorf("Content-Disposition = %q, want a %s… filename", disp, srv.cfg.OutFilePrefix)
	}

	// ?template= works on the default route too.
	rr2 := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/cv.pdf?template=primeats", nil))
	if rr2.Code != http.StatusOK {
		t.Fatalf("primeats status = %d, want 200 (body: %s)", rr2.Code, rr2.Body.String())
	}
	if rr2.Body.String() == rr.Body.String() {
		t.Error("?template=primeats returned the default template's bytes")
	}
}

// An absent or empty ?template= means DEFAULT_TEMPLATE, not an error.
func TestTemplateDefaults(t *testing.T) {
	srv := newTestServer(t)
	body := func(query string) string {
		t.Helper()
		rr := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/cv-example.pdf"+query, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d for %q, want 200 (body: %s)", rr.Code, query, rr.Body.String())
		}
		return rr.Body.String()
	}
	want := body("?template=" + srv.cfg.DefaultTemplate)
	for _, query := range []string{"", "?template=", "?download=0"} {
		if body(query) != want {
			t.Errorf("%q did not render DEFAULT_TEMPLATE (%s)", query, srv.cfg.DefaultTemplate)
		}
	}
}

// Both spellings are served, under the same extensionless URL.
func TestNamedCVAcceptsYmlExtension(t *testing.T) {
	srv := newTestServer(t)
	name := "zz-yml-" + strconv.Itoa(os.Getpid())
	src := filepath.Join(srv.cfg.Root, srv.cfg.DataDir, "cv-example.yaml")
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(srv.cfg.Root, srv.cfg.DataDir, name+".yml")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(path) })

	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/"+name+".pdf", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	if !strings.HasPrefix(rr.Body.String(), "%PDF-") {
		t.Error("body is not a PDF")
	}
	if disp := rr.Header().Get("Content-Disposition"); strings.Contains(disp, name) {
		t.Errorf("Content-Disposition = %q leaks the CV filename", disp)
	}
}

// A name spelled both ways is ambiguous; .yaml wins so the choice is stable.
func TestResolveCVPrefersYamlOverYml(t *testing.T) {
	cfg := config.Load()
	cfg.Root = repoRoot(t) // no typst needed: resolveCV only stats
	srv := New(cfg)
	name := "zz-both-" + strconv.Itoa(os.Getpid())
	for _, ext := range []string{".yaml", ".yml"} {
		p := filepath.Join(cfg.Root, cfg.DataDir, name+ext)
		if err := os.WriteFile(p, []byte("name: Ambiguous\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Remove(p) })
	}

	got, status, msg := srv.resolveCV(name)
	if status != 0 {
		t.Fatalf("resolveCV = %d %q, want success", status, msg)
	}
	if want := "/" + cfg.DataDir + "/" + name + ".yaml"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
}

func TestCVDisposition(t *testing.T) {
	srv := newTestServer(t)
	srv.now = func() time.Time { return time.Date(2026, 8, 1, 18, 41, 9, 0, time.UTC) }
	const wantFilename = "resume-202608011841.pdf"
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
			req := httptest.NewRequest(http.MethodGet, "/cv/cv-example.pdf"+c.query, nil)
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
			if params["filename"] != wantFilename {
				t.Errorf("filename = %q, want %q", params["filename"], wantFilename)
			}
		})
	}
}

// A prefix is operator config, but still must not reach the header raw.
func TestOutFilePrefix(t *testing.T) {
	srv := newTestServer(t)
	srv.now = func() time.Time { return time.Date(2026, 8, 1, 18, 41, 0, 0, time.UTC) }

	srv.cfg.OutFilePrefix = "John_Smith-"
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/cv-example.pdf?download", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	_, params, err := mime.ParseMediaType(rr.Header().Get("Content-Disposition"))
	if err != nil {
		t.Fatalf("unparseable Content-Disposition: %v", err)
	}
	if want := "John_Smith-202608011841.pdf"; params["filename"] != want {
		t.Errorf("filename = %q, want %q", params["filename"], want)
	}

	// A prefix carrying CRLF must be escaped, never split into a second header.
	srv.cfg.OutFilePrefix = "evil\r\nX-Injected: 1-"
	rr = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/cv-example.pdf", nil))
	if got := rr.Header().Get("X-Injected"); got != "" {
		t.Errorf("prefix injected a header: X-Injected = %q", got)
	}
	if disp := rr.Header().Get("Content-Disposition"); strings.ContainsAny(disp, "\r\n") {
		t.Errorf("Content-Disposition carries a raw newline: %q", disp)
	}
}

func TestCVRouteErrors(t *testing.T) {
	srv := newTestServer(t)
	cases := []struct {
		name string
		path string
		want int
	}{
		{"missing .pdf suffix", "/cv/cv-example", http.StatusNotFound},
		{"old template-in-path scheme", "/cv/helsinki/cv-example.pdf", http.StatusNotFound},
		{"path traversal in name", "/cv/..%2f..%2fetc%2fpasswd.pdf", http.StatusBadRequest},
		{"unknown cv", "/cv/does-not-exist.pdf", http.StatusNotFound},
		{"unknown template", "/cv/cv-example.pdf?template=nope", http.StatusNotFound},
		{"invalid template", "/cv/cv-example.pdf?template=..%2fsecret", http.StatusBadRequest},
		{"shared partial is not a template", "/cv/cv-example.pdf?template=_common", http.StatusBadRequest},
		// DEFAULT_CV is cv-default, and this repo ships no data/cv-default.yml.
		{"default CV without a file", "/cv.pdf", http.StatusNotFound},
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
	const path = "/cv/cv-example.pdf"
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
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/cv-example.pdf", nil))
	before := rr.Header().Get("ETag")

	day = day.Add(2 * time.Minute) // 00:01 the next day
	rr = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/cv-example.pdf", nil))
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
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/cv-example.pdf", nil))
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
	const path = "/cv/cv-example.pdf"

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
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/"+name+".pdf", nil))
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
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/cv-example.pdf", nil))
	if got := rr.Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
		t.Errorf("X-Robots-Tag = %q, want %q", got, "noindex, nofollow")
	}

	cfg := srv.cfg
	cfg.AllowIndexing = true
	rr = httptest.NewRecorder()
	New(cfg).Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/cv-example.pdf", nil))
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
	srv.Routes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/cv/cv-example.pdf", nil))
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
