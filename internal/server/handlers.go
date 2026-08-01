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

// slug guards names used to build file paths: no dots (".."), no slashes.
// Don't weaken it.
var slug = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDefaultCV(w http.ResponseWriter, r *http.Request) {
	s.serveCV(w, r, s.cfg.DefaultCV)
}

func (s *Server) handleCV(w http.ResponseWriter, r *http.Request) {
	name, ok := strings.CutSuffix(r.PathValue("name"), ".pdf")
	if !ok {
		writeError(w, http.StatusNotFound, "a CV must be requested as /cv/<name>.pdf")
		return
	}
	s.serveCV(w, r, name)
}

// 304s and cache hits are answered before a render slot is taken, so repeat
// traffic is never shed by the concurrency cap. Keep that ordering.
func (s *Server) serveCV(w http.ResponseWriter, r *http.Request, name string) {
	tmpl, status, msg := s.resolveTemplate(templateRequested(r, s.cfg.DefaultTemplate))
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	data, status, msg := s.resolveCV(name)
	if status != 0 {
		writeError(w, status, msg)
		return
	}

	photo := s.resolvePhoto(name)

	key := cacheKey(tmpl, data, photo, s.now().Format("2006-01-02"))
	etag := etagFor(key)
	attach := downloadRequested(r)

	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		s.setCacheHeaders(w, etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if pdf, ok := s.cache.get(key); ok {
		s.setCacheHeaders(w, etag)
		s.writePDF(w, s.outFilename(), attach, pdf)
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

	pdf, err := s.renderer.Render(r.Context(), tmpl.path, data.path, photo.path)
	if err != nil {
		// typst's stderr quotes template source and absolute paths: log only.
		log.Printf("render %s with %s: %v", data.path, tmpl.path, err)
		writeError(w, http.StatusInternalServerError, "failed to render CV")
		return
	}
	s.cache.put(key, pdf)
	s.setCacheHeaders(w, etag)
	s.writePDF(w, s.outFilename(), attach, pdf)
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

// "resume-202608011841.pdf" — a response header only, not part of the cache
// key, so cached bytes replay under a fresh name.
func (s *Server) outFilename() string {
	return s.cfg.OutFilePrefix + s.now().Format("200601021504") + ".pdf"
}

func templateRequested(r *http.Request, def string) string {
	if v := strings.TrimSpace(r.URL.Query().Get("template")); v != "" {
		return v
	}
	return def
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

// Tried in order, so a name present as both resolves to .yaml.
var dataExts = []string{".yaml", ".yml"}

// data/<cv>-photo.<ext>, tried in order.
const photoSuffix = "-photo"

var photoExts = []string{".png", ".jpg", ".jpeg", ".svg"}

// Returns the root-absolute path typst expects, e.g. "/data/cv-example.yaml".
// The chosen extension lands in resolved.path, hence in the cache key, so
// renaming cv.yml -> cv.yaml invalidates rather than replaying stale bytes.
func (s *Server) resolveCV(name string) (r resolved, status int, msg string) {
	if !slug.MatchString(name) {
		return resolved{}, http.StatusBadRequest, "invalid CV name"
	}
	for _, ext := range dataExts {
		info, err := os.Stat(filepath.Join(s.cfg.Root, s.cfg.DataDir, name+ext))
		if err != nil {
			continue
		}
		path := "/" + s.cfg.DataDir + "/" + name + ext
		return resolved{path: path, size: info.Size(), mod: info.ModTime()}, 0, ""
	}
	return resolved{}, http.StatusNotFound, "unknown CV: " + name
}

// Resolved here rather than in the template because typst treats a missing
// image() as a hard error: no file means no photo, not a failed render.
func (s *Server) resolvePhoto(name string) resolved {
	if !slug.MatchString(name) {
		return resolved{}
	}
	for _, ext := range photoExts {
		info, err := os.Stat(filepath.Join(s.cfg.Root, s.cfg.DataDir, name+photoSuffix+ext))
		if err != nil {
			continue
		}
		path := "/" + s.cfg.DataDir + "/" + name + photoSuffix + ext
		return resolved{path: path, size: info.Size(), mod: info.ModTime()}
	}
	return resolved{}
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
// it escapes the env-supplied filename so it can't break out of the header.
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
		w.Header().Set("X-Robots-Tag", "noindex, nofollow") // an indexed CV can't be unpublished
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

// Liveness probes hit healthPath every few seconds, so they go unlogged.
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
