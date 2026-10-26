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
	"runtime"
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
	mu    sync.Mutex
	files []File
)

// Files returns every library file, parsed, in path order. The first call parses
// them (a few at a time); later calls return the same result, until Release.
func Files() ([]File, error) {
	mu.Lock()
	defer mu.Unlock()
	if files == nil {
		f, err := load()
		if err != nil {
			return nil, err
		}
		files = f
	}
	return files, nil
}

// Release drops the parse trees that Files holds, for a process that has built
// what it needs from them: they are about 15 MB. A later call to Files parses
// again.
func Release() {
	mu.Lock()
	files = nil
	mu.Unlock()
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

// Source returns the text of one library file by path, and whether there is
// such a file.
func Source(p string) (string, bool) {
	data, err := sources.ReadFile("sysml.library/" + p)
	if err != nil {
		return "", false
	}
	return string(data), true
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
	// A few at a time: parsing makes many times the memory it keeps as garbage,
	// and all the files at once make a peak the process keeps.
	var wg sync.WaitGroup
	slots := make(chan struct{}, parallelism())
	for i, p := range paths {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
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

// parallelism is how many files are parsed at once: with 4 the first workspace
// of a process takes a peak of about 120 MB where all at once took 280, for a
// third more time.
func parallelism() int { return max(1, min(4, runtime.GOMAXPROCS(0))) }
