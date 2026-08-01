package server

import (
	"slices"
	"strings"
	"testing"

	"github.com/Melancholic/cvrenderer/internal/config"
)

func TestBaseURL(t *testing.T) {
	cases := map[string]string{
		"":                       "http://localhost:8080",
		"localhost:8080":         "http://localhost:8080",
		"cv.example.com":         "http://cv.example.com",
		"cv.example.com/":        "http://cv.example.com",
		"https://cv.example.com": "https://cv.example.com",
	}
	for in, want := range cases {
		if got := baseURL(in); got != want {
			t.Errorf("baseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// The listing is the service's only index: every pair, nothing that would 404.
func TestURLs(t *testing.T) {
	cfg := config.Load() // listing only reads the dirs; no typst needed
	cfg.Root = repoRoot(t)
	cfg.VirtualHost = "cv.example.com"
	cfg.DefaultCV = "cv-example"
	srv := New(cfg)

	urls := srv.URLs()
	for _, want := range []string{
		"http://cv.example.com/cv.pdf",
		"http://cv.example.com/cv.pdf?template=primeats",
		"http://cv.example.com/cv/cv-example.pdf",
		"http://cv.example.com/cv/cv-example.pdf?template=primeats",
	} {
		if !slices.Contains(urls, want) {
			t.Errorf("missing %q in %v", want, urls)
		}
	}
	// The default template is addressed by the bare URL, never spelled out.
	for _, u := range urls {
		if strings.Contains(u, "template="+cfg.DefaultTemplate) {
			t.Errorf("%q spells out the default template", u)
		}
		if strings.Contains(u, "_common") || strings.Contains(u, healthPath) {
			t.Errorf("%q must not be listed", u)
		}
	}
	// /cv.pdf comes first: it is the URL to hand out.
	if len(urls) == 0 || urls[0] != "http://cv.example.com/cv.pdf" {
		t.Errorf("first URL = %q, want the default CV", urls)
	}
}

// A DEFAULT_CV nobody created must not be advertised as if it worked.
func TestURLsOmitsMissingDefaultCV(t *testing.T) {
	cfg := config.Load()
	cfg.Root = repoRoot(t)
	srv := New(cfg) // DEFAULT_CV=cv-default, which this repo does not ship
	for _, u := range srv.URLs() {
		if strings.HasSuffix(u, defaultCVPath) || strings.Contains(u, defaultCVPath+"?") {
			t.Errorf("listed %q for a DEFAULT_CV that has no file", u)
		}
	}
}
