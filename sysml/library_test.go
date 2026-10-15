// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"sort"
	"strings"
	"testing"
)

func TestLibraryFilesListsTheEmbeddedLibrary(t *testing.T) {
	files, err := LibraryFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 94 {
		t.Errorf("%d library files, want 94", len(files))
	}
	seen := map[string]bool{}
	var paths []string
	for _, f := range files {
		switch {
		case len(f.Source) == 0:
			t.Errorf("%s has no source", f.Path)
		case seen[f.Path]:
			t.Errorf("%s is listed twice", f.Path)
		case strings.Contains(f.Path, `\`) || strings.HasPrefix(f.Path, "/"):
			t.Errorf("%q is not a relative path with forward slashes", f.Path)
		}
		seen[f.Path] = true
		paths = append(paths, f.Path)
		if lang, err := LanguageOf(f.Path); err != nil || lang != f.Language {
			t.Errorf("%s: language %v, extension says %v (%v)", f.Path, f.Language, lang, err)
		}
	}
	if !sort.StringsAreSorted(paths) {
		t.Error("files are not in path order")
	}
	if !seen["Kernel Libraries/Kernel Semantic Library/Base.kerml"] {
		t.Error("Base.kerml is missing")
	}

	// the source is a copy
	files[0].Source[0] ^= 0xff
	again, _ := LibraryFiles()
	if again[0].Source[0] == files[0].Source[0] {
		t.Error("changing a returned Source changed the library")
	}
}

// The library compiles on its own: given to a workspace of its own, its files
// resolve as models a consumer can show. One reference still does not resolve,
// the payload of a transition's accept action in Actions.sysml (a gap shared
// with the conformance run); a new diagnostic is a regression.
func TestTheLibraryFilesCompileOnTheirOwn(t *testing.T) {
	files, _ := LibraryFiles()
	w, err := NewWorkspace(WithoutStandardLibrary())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if _, err := w.AddAs(f.Language, f.Path, f.Source); err != nil {
			t.Fatalf("%s: %v", f.Path, err)
		}
	}
	const known = `Systems Library/Actions.sysml:238:37: cannot resolve "apayload" (searched from Actions::AcceptAction::aState::aTransition)`
	for _, d := range w.Resolve() {
		if d.String() != known {
			t.Errorf("unexpected diagnostic: %v", d)
		}
	}
	if n := len(w.Documents()); n != len(files) {
		t.Errorf("%d documents, want %d", n, len(files))
	}
}

// FileOf names the file of a library element by its LibraryFile Path.
func TestFileOfNamesTheLibraryFileOfAnElement(t *testing.T) {
	w, _ := NewWorkspace()
	files, _ := LibraryFiles()
	paths := map[string]bool{}
	for _, f := range files {
		paths[f.Path] = true
	}
	for name, want := range map[string]string{
		"ScalarValues::Integer": "Kernel Libraries/Kernel Data Type Library/ScalarValues.kerml",
		"Base::Anything":        "Kernel Libraries/Kernel Semantic Library/Base.kerml",
	} {
		e := w.Lookup(name)
		if e == nil {
			t.Fatalf("%s not found", name)
		}
		p, ok := libraryPath(w.FileOf(e))
		if !ok || p != want || !paths[p] {
			t.Errorf("FileOf(%s) = %q, want library:%s", name, w.FileOf(e), want)
		}
	}
}
