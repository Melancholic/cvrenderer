// Package typst is a thin wrapper around the `typst` CLI: it compiles a
// template against a YAML data file and returns the PDF bytes.
//
// Typst reads files only from within its `--root` directory, so both the
// template and any data file must live under Root.
package typst

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Renderer holds no template or data file: both are chosen per request.
type Renderer struct {
	Bin             string
	Root            string // --root
	FontDir         string // --font-path; "" to skip
	PackageCacheDir string // --package-cache-path; "" to skip
	Timeout         time.Duration
}

// Note the asymmetry: templatePath is relative to Root, dataPath and photoPath
// are root-absolute ("/data/cv-example.yaml"). An empty photoPath means the CV
// has no photo; the caller checks, since typst hard-errors on a missing image().
func (r *Renderer) Render(ctx context.Context, templatePath, dataPath, photoPath string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	args := []string{
		"compile",
		"--root", r.Root,
		templatePath,
		"-", // write PDF to stdout
		"--format", "pdf",
		"--input", "data=" + dataPath,
		"--input", "photo=" + photoPath,
	}
	if r.FontDir != "" {
		args = append(args, "--font-path", r.FontDir)
	}
	if r.PackageCacheDir != "" {
		args = append(args, "--package-cache-path", r.PackageCacheDir)
	}
	cmd := exec.CommandContext(ctx, r.Bin, args...)
	cmd.Dir = r.Root // templatePath is resolved against the working directory
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("typst render timed out after %s", r.Timeout)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("typst render failed: %s", msg)
	}
	return stdout.Bytes(), nil
}
