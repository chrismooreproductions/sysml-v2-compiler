// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/postprocess"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/semantics"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/syntax"
)

// An Element is an instance of a metaclass of the KerML or SysML metamodel. Its
// Class is the metaclass name ("PartUsage"), Span says where in the source it
// came from, and Generated marks an element that a rule created rather than the
// text. Props holds the properties in the order the grammar assigned them; an
// owned element is a property value that is itself an *Element, and Owner points
// back to the owning element.
type Element = model.Element

// A Prop is one property assignment of an Element.
type Prop = model.Prop

// A Value is the value of a property: an *Element (an owned element), Text,
// Bool, or *Ref.
type Value = model.Value

// Text is a string value: a keyword, an enumeration literal's name, or a name,
// number or string as written (strings and names are unescaped).
type Text = model.Text

// Bool is a boolean value.
type Bool = model.Bool

// A Ref is a cross-reference: a name written in the source, and after
// Workspace.Resolve the element it refers to (Target), or nil if it did not
// resolve.
type Ref = model.Ref

// Pos is a position in a source file; Line and Column are 1-based and Column
// counts bytes.
type Pos = syntax.Pos

// A Span is a range of source text; End is exclusive.
type Span = syntax.Span

// A SyntaxError is a lexical or syntax error. Pos is where the parser could go
// no further, Got is the unexpected token ("" at the end of the input), and
// Expected lists what could have continued the input.
type SyntaxError = syntax.Error

// Language is the notation of a file.
type Language int

const (
	// SysML is the SysML v2 textual notation (.sysml files).
	SysML Language = iota
	// KerML is the Kernel Modeling Language textual notation (.kerml files).
	KerML
)

func (l Language) String() string {
	if l == KerML {
		return "KerML"
	}
	return "SysML"
}

func (l Language) grammar() grammar.Language {
	if l == KerML {
		return grammar.KerML
	}
	return grammar.SysML
}

// LanguageOf chooses the language from a file name's extension.
func LanguageOf(name string) (Language, error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".sysml":
		return SysML, nil
	case ".kerml":
		return KerML, nil
	}
	return 0, fmt.Errorf("%s: unknown file type (want .sysml or .kerml)", name)
}

// A Document is one parsed file.
type Document struct {
	Name     string
	Language Language
	// Root is the file's Namespace, which owns its top-level packages and
	// elements through memberships.
	Root *Element
}

// Parse parses one file and builds its model, without resolving any names (every
// Ref has a nil Target). The language is taken from the file name's extension.
// A syntax error is returned as a *SyntaxError.
func Parse(name string, src []byte) (*Document, error) {
	lang, err := LanguageOf(name)
	if err != nil {
		return nil, err
	}
	return ParseAs(lang, name, src)
}

// ParseAs is Parse with the language given.
func ParseAs(lang Language, name string, src []byte) (*Document, error) {
	tree, err := syntax.Parse(lang.grammar(), name, string(src))
	if err != nil {
		return nil, err
	}
	root := model.FromSyntax(tree)
	postprocess.Run(root)
	semantics.Structure(root)
	return &Document{Name: name, Language: lang, Root: root}, nil
}

// A Workspace is a set of files whose names resolve against each other and,
// unless it was made without it, the standard library.
type Workspace struct {
	w    *semantics.Workspace
	docs []*Document
}

type config struct{ library bool }

// An Option configures NewWorkspace.
type Option func(*config)

// WithoutStandardLibrary makes a workspace that holds only the files added to
// it. Names from the standard library (ScalarValues, Parts, ...) then do not
// resolve.
func WithoutStandardLibrary() Option { return func(c *config) { c.library = false } }

// NewWorkspace returns a workspace. By default it includes the standard library
// of KerML and SysML v2, embedded in this package.
func NewWorkspace(opts ...Option) (*Workspace, error) {
	cfg := config{library: true}
	for _, o := range opts {
		o(&cfg)
	}
	w := &Workspace{w: semantics.NewWorkspace()}
	if cfg.library {
		if err := w.w.AddLibrary(); err != nil {
			return nil, err
		}
	}
	return w, nil
}

// Add parses a file and adds it to the workspace. The language is taken from
// the file name's extension. A syntax error is returned as a *SyntaxError and
// the file is not added.
func (w *Workspace) Add(name string, src []byte) (*Document, error) {
	lang, err := LanguageOf(name)
	if err != nil {
		return nil, err
	}
	return w.AddAs(lang, name, src)
}

// AddAs is Add with the language given.
func (w *Workspace) AddAs(lang Language, name string, src []byte) (*Document, error) {
	root, err := w.w.AddSource(lang.grammar(), name, string(src))
	if err != nil {
		return nil, err
	}
	d := &Document{Name: name, Language: lang, Root: root}
	w.docs = append(w.docs, d)
	return d, nil
}

// Documents lists the files added to the workspace, in order. It does not
// include the standard library.
func (w *Workspace) Documents() []*Document { return w.docs }

// A Diagnostic is a problem found in a file.
type Diagnostic struct {
	File    string
	Span    Span
	Message string
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s:%s: %s", d.File, d.Span.Start, d.Message)
}

// Resolve resolves every cross-reference in the workspace, setting each Ref's
// Target. It returns a diagnostic for each name in the added files that does
// not resolve, in source order; names in the standard library are not reported.
func (w *Workspace) Resolve() []Diagnostic {
	w.w.Resolve()
	var out []Diagnostic
	for _, u := range w.w.Unresolved() {
		scope := "the global scope"
		if u.Scope != nil {
			if q := semantics.QualifiedName(u.Scope); q != "" {
				scope = q
			}
		}
		out = append(out, Diagnostic{
			File:    u.File,
			Span:    u.Ref.Span,
			Message: fmt.Sprintf("cannot resolve %q (searched from %s)", u.Ref.Name, scope),
		})
	}
	return out
}

// QualifiedName names an element by the declared names along its ownership
// chain, such as "Vehicles::Car::engine". It is "" for an element with no
// named owner. A name that is not a plain identifier is quoted.
func QualifiedName(e *Element) string { return semantics.QualifiedName(e) }

// FileOf is the name of the file an element came from. It is "library:" followed
// by the path for an element of the standard library, and "" for an element a
// rule generated that cannot be traced to a file.
func (w *Workspace) FileOf(e *Element) string { return w.w.FileOf(e) }
