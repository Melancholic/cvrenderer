package server

import (
	"testing"
	"time"
)

func TestRenderCache(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	c := newRenderCache(time.Hour, 4)
	c.now = func() time.Time { return now }

	if _, ok := c.get("a"); ok {
		t.Error("empty cache reported a hit")
	}
	c.put("a", []byte("%PDF-a"))
	if pdf, ok := c.get("a"); !ok || string(pdf) != "%PDF-a" {
		t.Errorf("get after put = %q, %v; want the stored PDF", pdf, ok)
	}

	now = now.Add(time.Hour - time.Second)
	if _, ok := c.get("a"); !ok {
		t.Error("entry expired early")
	}
	now = now.Add(time.Second)
	if _, ok := c.get("a"); ok {
		t.Error("entry served past its TTL")
	}
}

func TestRenderCacheDisabled(t *testing.T) {
	c := newRenderCache(0, 4)
	if c.enabled() {
		t.Error("zero TTL should disable the cache")
	}
	c.put("a", []byte("%PDF-a"))
	if _, ok := c.get("a"); ok {
		t.Error("disabled cache stored an entry")
	}
}

func TestRenderCacheBounded(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	c := newRenderCache(time.Hour, 3)
	c.now = func() time.Time { return now }
	for i := 0; i < 20; i++ {
		c.put(string(rune('a'+i)), []byte("%PDF-"))
		now = now.Add(time.Second) // stagger expiry so eviction has an order
	}
	c.mu.Lock()
	n := len(c.entries)
	c.mu.Unlock()
	if n > 3 {
		t.Errorf("cache holds %d entries, want <= 3", n)
	}
}

func TestCacheKeyIdentity(t *testing.T) {
	mod := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	tmpl := resolved{path: "templates/helsinki.typ", size: 100, mod: mod}
	data := resolved{path: "/data/cv.yaml", size: 200, mod: mod}
	photo := resolved{path: "/data/cv-photo.png", size: 300, mod: mod}
	base := cacheKey(tmpl, data, photo, "2026-07-28")

	if got := cacheKey(tmpl, data, photo, "2026-07-28"); got != base {
		t.Error("identical inputs produced different keys")
	}
	cases := map[string]string{
		"different day":       cacheKey(tmpl, data, photo, "2026-07-29"),
		"edited data file":    cacheKey(tmpl, resolved{path: data.path, size: 201, mod: mod}, photo, "2026-07-28"),
		"touched data file":   cacheKey(tmpl, resolved{path: data.path, size: 200, mod: mod.Add(time.Second)}, photo, "2026-07-28"),
		"edited template":     cacheKey(resolved{path: tmpl.path, size: 101, mod: mod}, data, photo, "2026-07-28"),
		"different template":  cacheKey(resolved{path: "templates/primeats.typ", size: 100, mod: mod}, data, photo, "2026-07-28"),
		"different data file": cacheKey(tmpl, resolved{path: "/data/other.yaml", size: 200, mod: mod}, photo, "2026-07-28"),
		"replaced photo":      cacheKey(tmpl, data, resolved{path: photo.path, size: 301, mod: mod}, "2026-07-28"),
		"deleted photo":       cacheKey(tmpl, data, resolved{}, "2026-07-28"),
	}
	for name, got := range cases {
		if got == base {
			t.Errorf("%s produced the same cache key", name)
		}
	}
}

func TestCacheMaxAge(t *testing.T) {
	day := func(h, m, s int) time.Time {
		return time.Date(2026, 7, 28, h, m, s, 0, time.UTC)
	}
	cases := []struct {
		name string
		ttl  time.Duration
		now  time.Time
		want int
	}{
		{"midday, 24h TTL: capped at midnight", 24 * time.Hour, day(12, 0, 0), 12 * 3600},
		{"just after midnight: nearly a full day", 24 * time.Hour, day(0, 0, 0), 24 * 3600},
		{"one minute to midnight", 24 * time.Hour, day(23, 59, 0), 60},
		{"short TTL wins over the cap", time.Hour, day(12, 0, 0), 3600},
		{"short TTL still capped near midnight", time.Hour, day(23, 30, 0), 1800},
		{"zero TTL", 0, day(12, 0, 0), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cacheMaxAge(c.ttl, c.now); got != c.want {
				t.Errorf("cacheMaxAge(%s, %s) = %d, want %d", c.ttl, c.now.Format("15:04:05"), got, c.want)
			}
		})
	}
}

func TestETagMatches(t *testing.T) {
	const etag = `"abc123"`
	cases := []struct {
		header string
		want   bool
	}{
		{"", false},
		{`"abc123"`, true},
		{`W/"abc123"`, true},
		{`"other", "abc123"`, true},
		{`  "abc123"  `, true},
		{"*", true},
		{`"other"`, false},
		{`"abc1234"`, false},
	}
	for _, c := range cases {
		if got := etagMatches(c.header, etag); got != c.want {
			t.Errorf("etagMatches(%q) = %v, want %v", c.header, got, c.want)
		}
	}
}
