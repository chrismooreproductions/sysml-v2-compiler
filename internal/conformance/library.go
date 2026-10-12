// SPDX-License-Identifier: EPL-2.0

package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/library"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/postprocess"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/semantics"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/syntax"
)

// Build runs the pipeline on one source file as far as it goes today.
func Build(lang grammar.Language, file, src string) (*model.Element, error) {
	n, err := syntax.Parse(lang, file, src)
	if err != nil {
		return nil, err
	}
	root := model.FromSyntax(n)
	postprocess.Run(root)
	semantics.Structure(root)
	return root, nil
}

// LibraryFile is one source file of the standard library and the oracle's
// serialization of it.
type LibraryFile struct {
	Path   string // the source, relative to the library root
	Lang   grammar.Language
	Source string
	Oracle string // the XMI file
}

// LibraryFiles lists the standard library sources under libRoot (the
// directory that holds "Kernel Libraries", "Systems Library", ...) and where
// the oracle has the serialization of each, under oracleRoot.
func LibraryFiles(libRoot, oracleRoot string) ([]LibraryFile, error) {
	var files []LibraryFile
	err := filepath.Walk(libRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		var lang grammar.Language
		switch filepath.Ext(path) {
		case ".sysml":
			lang = grammar.SysML
		case ".kerml":
			lang = grammar.KerML
		default:
			return nil
		}
		rel, _ := filepath.Rel(libRoot, path)
		files = append(files, LibraryFile{Path: rel, Lang: lang, Oracle: filepath.Join(oracleRoot, rel+"x")})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for i := range files {
		src, err := os.ReadFile(filepath.Join(libRoot, files[i].Path))
		if err != nil {
			return nil, err
		}
		files[i].Source = string(src)
	}
	return files, err
}

// FileResult is the comparison for one library file.
type FileResult struct {
	File  LibraryFile
	Diffs []Diff
	Err   error
}

// CompareLibrary builds each file and compares it with the oracle.
func CompareLibrary(files []LibraryFile) []FileResult {
	var out []FileResult
	for _, f := range files {
		r := FileResult{File: f}
		root, err := Build(f.Lang, f.Path, f.Source)
		if err != nil {
			r.Err = err
			out = append(out, r)
			continue
		}
		ours := FromModel(root, FileBase(f.Path))
		Prune(ours)
		oracle, err := LoadXMI(f.Oracle)
		if err != nil {
			r.Err = err
			out = append(out, r)
			continue
		}
		Prune(oracle)
		r.Diffs = Compare(ours, oracle)
		out = append(out, r)
	}
	return out
}

// Totals summarizes results: files compared, files that match exactly,
// errors, and total differences.
func Totals(results []FileResult) (files, exact, errs, diffs int) {
	for _, r := range results {
		files++
		switch {
		case r.Err != nil:
			errs++
		case len(r.Diffs) == 0:
			exact++
		default:
			diffs += len(r.Diffs)
		}
	}
	return
}

// Report renders the per-category summary for results, for test logs.
func Report(results []FileResult) string {
	var all []Diff
	var sb strings.Builder
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(&sb, "ERROR %s: %v\n", r.File.Path, r.Err)
		}
		all = append(all, r.Diffs...)
	}
	for _, line := range Summary(all) {
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

// ResolvedResult is the comparison of the whole library after name resolution.
type ResolvedResult struct {
	Files []FileResult
	Refs  *RefStats
}

// CompareResolved loads the whole standard library into one workspace, resolves
// it, and compares both the structure of each file and every reference target
// with the oracle.
func CompareResolved(oracleRoot string) (*ResolvedResult, error) {
	files, err := library.Files()
	if err != nil {
		return nil, err
	}
	w := semantics.NewWorkspace()
	z := NewCanonicalizer()
	var roots []*model.Element
	for _, f := range files {
		root := model.FromSyntax(f.Tree)
		postprocess.Run(root)
		semantics.Structure(root)
		w.Add(root)
		roots = append(roots, root)
	}
	w.Resolve()
	var ours []*Elem
	for i, f := range files {
		ours = append(ours, z.Add(roots[i], FileBase(f.Path)))
	}
	z.Link()

	res := &ResolvedResult{}
	index := NewIndex()
	for i, f := range files {
		r := FileResult{File: LibraryFile{Path: f.Path, Lang: f.Lang}}
		Prune(ours[i])
		oracle, err := LoadXMI(filepath.Join(oracleRoot, f.Path+"x"))
		if err != nil {
			r.Err = err
			res.Files = append(res.Files, r)
			continue
		}
		Prune(oracle)
		r.Diffs = Compare(ours[i], oracle)
		res.Files = append(res.Files, r)
		index.Add(oracle, FileBase(f.Path))
	}
	byKey := map[string]*Elem{}
	for i, f := range files {
		AssignKeys(ours[i], FileBase(f.Path), "", nil, byKey)
	}
	res.Refs = CompareReferences(ours, index)
	return res, nil
}
