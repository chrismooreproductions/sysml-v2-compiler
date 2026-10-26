// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The release's example models are the pilot's own, so checked together they
// report nothing, and now they do. These stay as ratchets: a
// change that makes the compiler report a name it does not resolve, or a
// constraint it applies wrongly, in one of them fails (the count was 154 and 48
// at v0.18.0; see the issue "Examples: the release's example models report 154
// diagnostics").
const (
	maxSysMLExampleDiagnostics = 0
	maxKerMLExampleDiagnostics = 0
)

// exampleDiagnostics checks every model with an extension under a directory as
// one workspace and returns its diagnostics, resolution and validation.
func exampleDiagnostics(t *testing.T, dir, ext string) (files int, diags []Diagnostic) {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ext) {
			paths = append(paths, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Add(p, src); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	diags = append(w.Resolve(), w.Validate()...)
	return len(paths), diags
}

func TestExampleModelsDiagnosticsRatchet(t *testing.T) {
	for _, c := range []struct {
		dir, ext string
		files    int
		max      int
	}{
		{"../testdata/upstream/sysml/examples", ".sysml", 96, maxSysMLExampleDiagnostics},
		{"../testdata/upstream/kerml/examples", ".kerml", 58, maxKerMLExampleDiagnostics},
	} {
		files, diags := exampleDiagnostics(t, c.dir, c.ext)
		if files != c.files {
			t.Errorf("%s: %d files, want %d", c.dir, files, c.files)
		}
		t.Logf("%s: %d files, %d diagnostics", c.dir, files, len(diags))
		if len(diags) > c.max {
			t.Errorf("%s: %d diagnostics, more than the %d already reported; the first new ones may be among:\n%v", c.dir, len(diags), c.max, diags[:min(5, len(diags))])
		}
		if len(diags) < c.max {
			t.Errorf("%s: %d diagnostics, fewer than the ratchet's %d: lower it", c.dir, len(diags), c.max)
		}
	}
}
