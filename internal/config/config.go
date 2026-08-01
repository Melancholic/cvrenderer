// Package config loads runtime configuration from environment variables.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all runtime settings, each from the env var named below.
type Config struct {
	Addr                 string        // PORT (8080), stored as ":8080"
	TypstBin             string        // TYPST_BIN (typst), path to the typst executable; a bare name is looked up on $PATH
	Root                 string        // ROOT (/app), the only dir typst may read; the four below are relative to it
	TemplateDir          string        // TEMPLATE_DIR (templates), <template> -> <dir>/<template>.typ
	DataDir              string        // DATA_DIR (data), <name> -> <dir>/<name>.yaml
	DefaultCV            string        // DEFAULT_CV (cv-default), the CV served at /cv.pdf
	DefaultTemplate      string        // DEFAULT_TEMPLATE (helsinki), used when ?template= is absent
	VirtualHost          string        // VIRTUAL_HOST (localhost:8080), only used to print the URL list at startup
	LetsencryptHost      string        // LETSENCRYPT_HOST (unset), presence means the proxy terminates TLS, so the listing says https
	OutFilePrefix        string        // OUT_FILE_PREFIX (resume-), download filename is <prefix><timestamp>.pdf
	FontDir              string        // FONT_DIR (fonts), "" leaves only typst's embedded serif faces
	PackageCacheDir      string        // PACKAGE_CACHE_DIR (typst-packages), vendored so @preview/... resolves offline
	RenderTimeout        time.Duration // RENDER_TIMEOUT (30s), bounds one typst invocation
	MaxConcurrentRenders int           // MAX_CONCURRENT_RENDERS (1), the cache absorbs repeats so only cold keys render
	RenderQueueTimeout   time.Duration // RENDER_QUEUE_TIMEOUT (10s), wait for a slot, then 503
	CacheTTL             time.Duration // CACHE_TTL (24h), 0 disables caching and sends no-store
	CacheMaxEntries      int           // CACHE_MAX_ENTRIES (64), one key per CV per template per day
	AllowIndexing        bool          // ALLOW_INDEXING (false), true drops X-Robots-Tag: noindex
}

// Load applies defaults so the service runs with zero configuration.
func Load() Config {
	return Config{
		Addr:            ":" + getEnv("PORT", "8080"),
		TypstBin:        getEnv("TYPST_BIN", "typst"),
		Root:            getEnv("ROOT", "/app"),
		TemplateDir:     getEnv("TEMPLATE_DIR", "templates"),
		DataDir:         getEnv("DATA_DIR", "data"),
		DefaultCV:       getEnv("DEFAULT_CV", "cv-default"),
		DefaultTemplate: getEnv("DEFAULT_TEMPLATE", "helsinki"),
		// Both are nginx-proxy/acme-companion conventions and normally set together.
		VirtualHost:     getEnv("VIRTUAL_HOST", getEnv("LETSENCRYPT_HOST", "localhost:8080")),
		LetsencryptHost: os.Getenv("LETSENCRYPT_HOST"),
		OutFilePrefix:   getEnv("OUT_FILE_PREFIX", "resume-"),
		FontDir:         getEnv("FONT_DIR", "fonts"),
		PackageCacheDir: getEnv("PACKAGE_CACHE_DIR", "typst-packages"),
		RenderTimeout:   getDuration("RENDER_TIMEOUT", 30*time.Second),

		MaxConcurrentRenders: max(1, getInt("MAX_CONCURRENT_RENDERS", 1)),
		RenderQueueTimeout:   getDuration("RENDER_QUEUE_TIMEOUT", 10*time.Second),
		CacheTTL:             getDuration("CACHE_TTL", 24*time.Hour),
		CacheMaxEntries:      max(1, getInt("CACHE_MAX_ENTRIES", 64)),
		AllowIndexing:        getBool("ALLOW_INDEXING", false),
	}
}

// The getters below fall back to the default on a malformed value, so a typo'd
// env var is silent.
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func getDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
