// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"path"
	"sort"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/library"
)

// A LibraryFile is one file of the standard library that every Workspace
// loads unless it was made WithoutStandardLibrary.
type LibraryFile struct {
	// Path is relative to the library root, using forward slashes, such as
	// "Kernel Libraries/Kernel Semantic Library/Base.kerml". Workspace.FileOf
	// names the file of a library element "library:" followed by this Path.
	Path string
	// Language is the notation of the file, which its extension says.
	Language Language
	// Source is the text of the file, as released by the OMG. It is a copy:
	// the caller may change it.
	Source []byte
}

// LibraryFiles returns every file of the embedded standard library, in path
// order. A workspace made WithoutStandardLibrary can be given the same files
// with AddAs, under their Paths, to compile the library as models of their own.
func LibraryFiles() ([]LibraryFile, error) {
	srcs, err := library.Sources()
	if err != nil {
		return nil, err
	}
	var out []LibraryFile
	for p, src := range srcs {
		lang, err := LanguageOf(p)
		if err != nil {
			continue // the library also carries files that are not models
		}
		out = append(out, LibraryFile{Path: p, Language: lang, Source: []byte(src)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// libraryPath is the Path of a library file, given the name FileOf returns for
// an element in it; ok is false for any other name.
func libraryPath(fileOf string) (p string, ok bool) {
	const prefix = "library:"
	if len(fileOf) > len(prefix) && fileOf[:len(prefix)] == prefix {
		return path.Clean(fileOf[len(prefix):]), true
	}
	return "", false
}
