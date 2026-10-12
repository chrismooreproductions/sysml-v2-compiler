// SPDX-License-Identifier: EPL-2.0

// Package library holds the standard library of KerML and SysML v2: the
// sources vendored from the OMG release (see UPSTREAM), embedded in the
// binary, and parsed on first use.
package library

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"sync"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/syntax"
)

//go:embed all:sysml.library
var sources embed.FS

// File is one library source file and its parse tree. The tree is shared by
// every caller and must not be modified; build a model from it.
type File struct {
	Path string // relative to the library root, e.g. "Kernel Libraries/Kernel Semantic Library/Base.kerml"
	Lang grammar.Language
	Tree *syntax.Node
}

var (
	once  sync.Once
	files []File
	err   error
)

// Files returns every library file, parsed, in path order. The first call parses
// them (in parallel); later calls return the same result.
func Files() ([]File, error) {
	once.Do(func() { files, err = load() })
	return files, err
}

// Sources returns the library's source text by path, for tools and tests.
func Sources() (map[string]string, error) {
	out := map[string]string{}
	err := fs.WalkDir(sources, "sysml.library", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := sources.ReadFile(p)
		if err != nil {
			return err
		}
		out[p[len("sysml.library/"):]] = string(data)
		return nil
	})
	return out, err
}

func load() ([]File, error) {
	srcs, err := Sources()
	if err != nil {
		return nil, err
	}
	var paths []string
	for p := range srcs {
		if ext := path.Ext(p); ext == ".sysml" || ext == ".kerml" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	out := make([]File, len(paths))
	errs := make([]error, len(paths))
	var wg sync.WaitGroup
	for i, p := range paths {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lang := grammar.SysML
			if path.Ext(p) == ".kerml" {
				lang = grammar.KerML
			}
			tree, err := syntax.Parse(lang, p, srcs[p])
			out[i], errs[i] = File{Path: p, Lang: lang, Tree: tree}, err
		}()
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			return nil, fmt.Errorf("standard library: %w", e)
		}
	}
	return out, nil
}
