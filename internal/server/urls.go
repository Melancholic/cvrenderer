package server

import (
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// A bare host gets http://; a value with a scheme is used verbatim.
func baseURL(virtualHost string) string {
	host := strings.TrimRight(strings.TrimSpace(virtualHost), "/")
	if host == "" {
		return "http://localhost:8080"
	}
	if strings.Contains(host, "://") {
		return host
	}
	return "http://" + host
}

// URLs lists every URL that renders a PDF, one per CV per template. The
// default template is addressed by the bare path rather than ?template=.
func (s *Server) URLs() []string {
	base := baseURL(s.cfg.VirtualHost)
	templates := s.listTemplates()
	if len(templates) == 0 {
		return nil
	}

	// Listing the bare path would advertise a 404 if DEFAULT_TEMPLATE is missing.
	hasDefault := slices.Contains(templates, s.cfg.DefaultTemplate)
	var urls []string
	add := func(path string) {
		if hasDefault {
			urls = append(urls, base+path)
		}
		for _, tmpl := range templates {
			if tmpl != s.cfg.DefaultTemplate {
				urls = append(urls, base+path+"?template="+tmpl)
			}
		}
	}

	names := s.listCVs()
	for _, name := range names {
		if name == s.cfg.DefaultCV {
			add(defaultCVPath)
		}
	}
	for _, name := range names {
		add("/cv/" + name + ".pdf")
	}
	return urls
}

// LogURLs prints the listing at startup; a default naming a file nobody added
// would otherwise surface only as a 404 in the browser.
func (s *Server) LogURLs() {
	urls := s.URLs()
	if len(urls) == 0 {
		log.Printf("no CVs to serve: found no templates in %s or no CV files in %s",
			filepath.Join(s.cfg.Root, s.cfg.TemplateDir), filepath.Join(s.cfg.Root, s.cfg.DataDir))
		return
	}
	log.Printf("available URLs (VIRTUAL_HOST=%s):", s.cfg.VirtualHost)
	for _, u := range urls {
		log.Printf("  %s", u)
	}
	if _, status, _ := s.resolveCV(s.cfg.DefaultCV); status != 0 {
		log.Printf("warning: DEFAULT_CV %q has no file in %s — %s will 404",
			s.cfg.DefaultCV, filepath.Join(s.cfg.Root, s.cfg.DataDir), defaultCVPath)
	}
	if _, status, _ := s.resolveTemplate(s.cfg.DefaultTemplate); status != 0 {
		log.Printf("warning: DEFAULT_TEMPLATE %q has no file in %s — requests without ?template= will 404",
			s.cfg.DefaultTemplate, filepath.Join(s.cfg.Root, s.cfg.TemplateDir))
	}
}

// A name present as both .yaml and .yml is listed once, as resolveCV serves it.
func (s *Server) listCVs() []string {
	return listNames(filepath.Join(s.cfg.Root, s.cfg.DataDir), func(name string) (string, bool) {
		for _, ext := range dataExts {
			if base, ok := strings.CutSuffix(name, ext); ok {
				return base, true
			}
		}
		return "", false
	})
}

func (s *Server) listTemplates() []string {
	return listNames(filepath.Join(s.cfg.Root, s.cfg.TemplateDir), func(name string) (string, bool) {
		return strings.CutSuffix(name, ".typ")
	})
}

func listNames(dir string, base func(string) (string, bool)) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name, ok := base(e.Name())
		if !ok || seen[name] || !slug.MatchString(name) {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
