package server

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
	"time"
)

type cacheEntry struct {
	pdf     []byte
	expires time.Time
}

// renderCache memoises rendered PDFs. Replaying a hit is sound only because
// typst output is byte-deterministic for identical inputs.
type renderCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
	ttl     time.Duration
	max     int
	now     func() time.Time // overridden in tests
}

func newRenderCache(ttl time.Duration, max int) *renderCache {
	if max < 1 {
		max = 1
	}
	return &renderCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
		max:     max,
		now:     time.Now,
	}
}

func (c *renderCache) enabled() bool { return c != nil && c.ttl > 0 }

func (c *renderCache) get(key string) ([]byte, bool) {
	if !c.enabled() {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if !c.now().Before(e.expires) {
		delete(c.entries, key)
		return nil, false
	}
	return e.pdf, true
}

func (c *renderCache) put(key string, pdf []byte) {
	if !c.enabled() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.max {
		c.evictLocked()
	}
	c.entries[key] = cacheEntry{pdf: pdf, expires: c.now().Add(c.ttl)}
}

func (c *renderCache) evictLocked() {
	now := c.now()
	freed := false
	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
			freed = true
		}
	}
	if freed || len(c.entries) == 0 {
		return
	}
	var oldestKey string
	var oldest time.Time
	for k, e := range c.entries {
		if oldestKey == "" || e.expires.Before(oldest) {
			oldestKey, oldest = k, e.expires
		}
	}
	delete(c.entries, oldestKey)
}

// The date is load-bearing: templates resolve {{age}}, {{experience_years}} and
// `end: Present` from datetime.today(), so the same YAML renders differently
// tomorrow. Size+mtime make an edit invalidate.
func cacheKey(tmpl, data resolved, day string) string {
	var b strings.Builder
	for _, r := range []resolved{tmpl, data} {
		b.WriteString(r.path)
		b.WriteByte(0)
		b.WriteString(strconv.FormatInt(r.size, 10))
		b.WriteByte(0)
		b.WriteString(strconv.FormatInt(r.mod.UnixNano(), 10))
		b.WriteByte(0)
	}
	b.WriteString(day)
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func etagFor(key string) string { return `"` + key[:32] + `"` }

// CACHE_TTL capped at local midnight, when the date-derived fields above go
// stale. Local time: typst's datetime.today() shares this process's timezone.
func cacheMaxAge(ttl time.Duration, now time.Time) int {
	seconds := int(ttl.Seconds())
	midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	if untilMidnight := int(midnight.Sub(now).Seconds()); untilMidnight < seconds {
		seconds = untilMidnight
	}
	if seconds < 0 {
		seconds = 0
	}
	return seconds
}

func etagMatches(header, etag string) bool {
	if header == "" {
		return false
	}
	if strings.TrimSpace(header) == "*" {
		return true
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == etag {
			return true
		}
	}
	return false
}
