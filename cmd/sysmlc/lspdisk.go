// SPDX-License-Identifier: EPL-2.0

package main

import (
	"bytes"
	"context"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/chrismooreproductions/sysml-v2-compiler/lsp"
	"github.com/chrismooreproductions/sysml-v2-compiler/sysml"
)

// lspOptions are the options of the language server the command runs for an
// editor: no limits, the model files under the editor's folders read from the
// disk, and the standard library written to a cache directory for the editor to
// open.
func lspOptions() lsp.Options {
	return lsp.Options{
		MaxDocuments: -1, MaxBytes: -1, MaxInFlight: -1, Concurrency: -1,
		FailOnPanic: false,
		Files:       diskFiles{},
		LibraryURI:  (&libraryCache{}).uri,
	}
}

// diskFiles reads the model files of the folders the editor names.
type diskFiles struct{}

func (diskFiles) List(_ context.Context, folders []string) []lsp.File {
	var out []lsp.File
	for _, f := range folders {
		folder := uriToPath(f)
		n := 0
		filepath.WalkDir(folder, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if p != folder && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
					return filepath.SkipDir
				}
				return nil
			}
			if _, err := sysml.LanguageOf(p); err != nil || n >= 5000 {
				return nil
			}
			if b, err := os.ReadFile(p); err == nil {
				out = append(out, lsp.File{URI: pathToURI(p), Text: string(b)})
				n++
			}
			return nil
		})
	}
	return out
}

func (diskFiles) Read(_ context.Context, uri string) (string, bool) {
	b, err := os.ReadFile(uriToPath(uri))
	return string(b), err == nil
}

// libraryCache writes the standard library files an editor is sent to into a
// directory and gives their file URIs.
type libraryCache struct{ dir string }

func (c *libraryCache) uri(path string) string {
	text, ok := lsp.LibraryText(path)
	if !ok {
		return lsp.LibraryURI(path)
	}
	if c.dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			base = os.TempDir()
		}
		c.dir = filepath.Join(base, "sysml-v2-compiler", "library")
	}
	full := filepath.Join(c.dir, filepath.FromSlash(path))
	if existing, err := os.ReadFile(full); err != nil || !bytes.Equal(existing, []byte(text)) {
		if os.MkdirAll(filepath.Dir(full), 0o755) == nil {
			os.WriteFile(full, []byte(text), 0o444)
		}
	}
	return pathToURI(full)
}

// uriToPath turns a file URI into a file system path.
func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return uri
	}
	p := u.Path
	if runtime.GOOS == "windows" && len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

// pathToURI turns a file system path into a file URI.
func pathToURI(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}
