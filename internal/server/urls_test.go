package server

import (
	"slices"
	"strings"
	"testing"

	"github.com/Melancholic/cvrenderer/internal/config"
)

func TestBaseURL(t *testing.T) {
	cases := []struct {
		virtualHost, letsencryptHost, want string
	}{
		{"", "", "http://localhost:8080"},
		{"localhost:8080", "", "http://localhost:8080"},
		{"cv.example.com", "", "http://cv.example.com"},
		{"cv.example.com/", "", "http://cv.example.com"},
		{"https://cv.example.com", "", "https://cv.example.com"},
		// A certificate for the host means the proxy serves it over TLS.
		{"cv.example.com", "cv.example.com", "https://cv.example.com"},
		{"cv.example.com", "cv.example.com,www.example.com", "https://cv.example.com"},
		{"cv.example.com, www.example.com", "cv.example.com", "https://cv.example.com"},
		// An explicit scheme still wins.
		{"http://cv.example.com", "cv.example.com", "http://cv.example.com"},
	}
	for _, c := range cases {
		if got := baseURL(c.virtualHost, c.letsencryptHost); got != c.want {
			t.Errorf("baseURL(%q, %q) = %q, want %q", c.virtualHost, c.letsencryptHost, got, c.want)
		}
	}
}

// VIRTUAL_HOST unset but LETSENCRYPT_HOST set: the certificate names the real
// host, so the listing must not fall back to localhost.
func TestConfigHostFallsBackToLetsencryptHost(t *testing.T) {
	t.Setenv("LETSENCRYPT_HOST", "cv.example.com")
	cfg := config.Load()
	if cfg.VirtualHost != "cv.example.com" {
		t.Errorf("VirtualHost = %q, want cv.example.com", cfg.VirtualHost)
	}
	if got := baseURL(cfg.VirtualHost, cfg.LetsencryptHost); got != "https://cv.example.com" {
		t.Errorf("baseURL = %q, want https://cv.example.com", got)
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
