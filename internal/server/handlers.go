package server

import (
	"encoding/json"
	"errors"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// slug guards names used to build file paths: no dots (blocks ".."), no
// slashes (blocks traversal). Don't weaken it.
var slug = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// 304s and cache hits are answered before a render slot is taken, so repeat
// traffic is never shed by the concurrency cap. Keep that ordering.
func (s *Server) handleCV(w http.ResponseWriter, r *http.Request) {
	tmpl, status, msg := s.resolveTemplate(r.PathValue("template"))
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	name, ok := strings.CutSuffix(r.PathValue("name"), ".pdf")
	if !ok {
		writeError(w, http.StatusNotFound, "a CV must be requested as /cv/<template>/<name>.pdf")
		return
	}
	data, status, msg := s.resolveCV(name)
	if status != 0 {
		writeError(w, status, msg)
		return
	}

	key := cacheKey(tmpl, data, s.now().Format("2006-01-02"))
	etag := etagFor(key)
	attach := downloadRequested(r)

	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		s.setCacheHeaders(w, etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if pdf, ok := s.cache.get(key); ok {
		s.setCacheHeaders(w, etag)
		s.writePDF(w, name+".pdf", attach, pdf)
		return
	}

	release, err := s.acquireRenderSlot(r.Context())
	if err != nil {
		if errors.Is(err, errRenderBusy) {
			w.Header().Set("Retry-After", "5")
			writeError(w, http.StatusServiceUnavailable, "server busy, try again shortly")
		}
		// Otherwise the client disconnected while queued; nothing to reply to.
		return
	}
	defer release()

	pdf, err := s.renderer.Render(r.Context(), tmpl.path, data.path)
	if err != nil {
		// typst's stderr quotes template source and absolute paths: log only.
		// 500 not 404 — the file exists, so this is a fault worth alerting on.
		log.Printf("render %s with %s: %v", data.path, tmpl.path, err)
		writeError(w, http.StatusInternalServerError, "failed to render CV")
		return
	}
	s.cache.put(key, pdf)
	s.setCacheHeaders(w, etag)
	s.writePDF(w, name+".pdf", attach, pdf)
}

// `private` by default: shared proxies must not retain personal data.
func (s *Server) setCacheHeaders(w http.ResponseWriter, etag string) {
	if !s.cache.enabled() {
		w.Header().Set("Cache-Control", "no-store")
		return
	}
	scope := "private"
	if s.cfg.AllowIndexing {
		scope = "public"
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", scope+", max-age="+strconv.Itoa(cacheMaxAge(s.cfg.CacheTTL, s.now())))
}

// A bare or unparseable value counts as true — asking at all signals intent;
// only an explicit false opts back out.
func downloadRequested(r *http.Request) bool {
	q := r.URL.Query()
	if !q.Has("download") {
		return false
	}
	v := q.Get("download")
	if v == "" {
		return true
	}
	b, err := strconv.ParseBool(v)
	return err != nil || b
}

// size and mod come from the stat that proved existence, and feed the cache key.
type resolved struct {
	path string // root-absolute for data, Root-relative for templates
	size int64
	mod  time.Time
}

// Returns the root-absolute path typst expects, e.g. "/data/cv-example.yaml".
func (s *Server) resolveCV(name string) (r resolved, status int, msg string) {
	if !slug.MatchString(name) {
		return resolved{}, http.StatusBadRequest, "invalid CV name"
	}
	info, err := os.Stat(filepath.Join(s.cfg.Root, s.cfg.DataDir, name+".yaml"))
	if err != nil {
		return resolved{}, http.StatusNotFound, "unknown CV: " + name
	}
	path := "/" + s.cfg.DataDir + "/" + name + ".yaml"
	return resolved{path: path, size: info.Size(), mod: info.ModTime()}, 0, ""
}

func (s *Server) resolveTemplate(name string) (r resolved, status int, msg string) {
	if !slug.MatchString(name) {
		return resolved{}, http.StatusBadRequest, "invalid template name"
	}
	rel := filepath.Join(s.cfg.TemplateDir, name+".typ")
	info, err := os.Stat(filepath.Join(s.cfg.Root, rel))
	if err != nil {
		return resolved{}, http.StatusNotFound, "unknown template: " + name
	}
	return resolved{path: rel, size: info.Size(), mod: info.ModTime()}, 0, ""
}

// Content-Disposition is built with mime.FormatMediaType, never concatenation:
// it escapes the filename, so none can break out of the header.
func (s *Server) writePDF(w http.ResponseWriter, filename string, attach bool, pdf []byte) {
	disp := "inline"
	if attach {
		disp = "attachment"
	}
	value := mime.FormatMediaType(disp, map[string]string{"filename": filename})
	if value == "" { // unrepresentable filename — send the disposition alone
		value = disp
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", value)
	if !s.cfg.AllowIndexing {
		// An indexed CV can't be unpublished retroactively.
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// Liveness probes hit healthPath every few seconds and would bury everything
// else, so they are served but not logged.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == healthPath {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
