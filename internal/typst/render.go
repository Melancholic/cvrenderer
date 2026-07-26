// Package typst is a thin wrapper around the `typst` CLI. It compiles a Typst
// template against a YAML data file and returns the resulting PDF bytes.
//
// Typst reads files only from within its `--root` directory, so both the
// template and any data file must live under Root. Data paths are passed to the
// template as root-absolute paths (leading "/"), which is how Typst addresses
// files relative to the root.
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

// Note the asymmetry: templatePath is relative to Root, dataPath is
// root-absolute ("/data/cv-example.yaml"). The PDF streams from stdout, so
// nothing touches disk on the output side.
func (r *Renderer) Render(ctx context.Context, templatePath, dataPath string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	args := []string{
		"compile",
		"--root", r.Root,
		templatePath,
		"-", // write PDF to stdout
		"--format", "pdf",
		"--input", "data=" + dataPath,
	}
	if r.FontDir != "" {
		args = append(args, "--font-path", r.FontDir)
	}
	if r.PackageCacheDir != "" {
		args = append(args, "--package-cache-path", r.PackageCacheDir)
	}
	cmd := exec.CommandContext(ctx, r.Bin, args...)
	// The template path is resolved relative to the process working directory,
	// so anchor it at Root (where templates/ lives).
	cmd.Dir = r.Root
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
