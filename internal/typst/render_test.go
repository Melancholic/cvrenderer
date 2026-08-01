package typst

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// findRoot locates templates/ and data/ regardless of the working directory.
func findRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root (go.mod)")
		}
		dir = parent
	}
}

// newRenderer skips the test when typst is not installed, so the unit suite
// still passes in minimal environments.
func newRenderer(t *testing.T) *Renderer {
	t.Helper()
	bin := os.Getenv("TYPST_BIN")
	if bin == "" {
		bin = "typst"
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Skipf("typst binary not found (%v); skipping render test", err)
	}
	root := findRoot(t)
	return &Renderer{
		Bin:             bin,
		Root:            root,
		FontDir:         filepath.Join(root, "fonts"),
		PackageCacheDir: filepath.Join(root, "typst-packages"),
		Timeout:         30 * time.Second,
	}
}

const helsinki = "templates/helsinki.typ"

func TestRenderDefaultData(t *testing.T) {
	r := newRenderer(t)
	pdf, err := r.Render(context.Background(), helsinki, "/data/cv-example.yaml", "/data/cv-example-photo.svg")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("output is not a PDF")
	}
	if len(pdf) < 1000 {
		t.Errorf("PDF suspiciously small: %d bytes", len(pdf))
	}
}

// Every shipped template must compile against the sample data, so a broken one
// fails the suite rather than a request.
func TestRenderTemplateSelection(t *testing.T) {
	r := newRenderer(t)
	for _, tmpl := range []string{"templates/helsinki.typ", "templates/primeats.typ"} {
		t.Run(filepath.Base(tmpl), func(t *testing.T) {
			pdf, err := r.Render(context.Background(), tmpl, "/data/cv-example.yaml", "/data/cv-example-photo.svg")
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
				t.Fatalf("output is not a PDF")
			}
		})
	}
}

// Every template must compile with an empty photo input rather than erroring.
func TestRenderWithoutPhoto(t *testing.T) {
	r := newRenderer(t)
	for _, tmpl := range []string{"templates/helsinki.typ", "templates/primeats.typ"} {
		t.Run(filepath.Base(tmpl), func(t *testing.T) {
			pdf, err := r.Render(context.Background(), tmpl, "/data/cv-example.yaml", "")
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
				t.Fatalf("output is not a PDF")
			}
		})
	}
}

// typst's stderr must surface as a Go error rather than an empty PDF.
func TestRenderMissingData(t *testing.T) {
	r := newRenderer(t)
	_, err := r.Render(context.Background(), helsinki, "/data/does-not-exist.yaml", "")
	if err == nil {
		t.Fatal("expected an error for a missing data file")
	}
}
