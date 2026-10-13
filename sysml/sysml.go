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

// AddRecovering is Add for a file that may have syntax errors. It returns every
// syntax error in the file, not only the first, each with its position in the
// source, and adds the statements around them to the workspace. The document is
// nil if nothing in the file could be parsed. With no errors it behaves as Add.
func (w *Workspace) AddRecovering(name string, src []byte) (*Document, []*SyntaxError) {
	lang, err := LanguageOf(name)
	if err != nil {
		return nil, []*SyntaxError{{File: name, Msg: err.Error()}}
	}
	root, errs := w.w.AddSourceRecovering(lang.grammar(), name, string(src))
	var doc *Document
	if root != nil {
		doc = &Document{Name: name, Language: lang, Root: root}
		w.docs = append(w.docs, doc)
	}
	return doc, errs
}

// Replace adds a document, or if one with the same name was added before,
// replaces it, and reports its syntax errors as AddRecovering does. Everything
// that was resolved or computed about the other documents is forgotten, so call
// Resolve again before using it. The standard library is unaffected, which
// makes this much cheaper than building a new workspace: an editor calls it on
// every change.
func (w *Workspace) Replace(name string, src []byte) (*Document, []*SyntaxError) {
	w.Remove(name)
	return w.AddRecovering(name, src)
}

// Remove drops the document with a name from the workspace, if there is one.
func (w *Workspace) Remove(name string) {
	var kept []*Document
	for _, d := range w.docs {
		if d.Name == name {
			w.w.Remove(d.Root)
		} else {
			kept = append(kept, d)
		}
	}
	w.docs = kept
}

// Documents lists the files added to the workspace, in order. It does not
// include the standard library.
func (w *Workspace) Documents() []*Document { return w.docs }

// A Diagnostic is a problem found in a file.
type Diagnostic struct {
	File    string
	Span    Span
	Message string
	// Constraint names the specification constraint a Validate diagnostic
	// reports (for example "validateFeatureEndNoDirection"); it is empty for
	// the diagnostics Resolve returns.
	Constraint string
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

// Implied is a relationship the specification adds to a type without the source
// writing it: a Subclassification, Subsetting, Redefinition, FeatureTyping or
// CrossSubsetting from the type to Target. A Target that is Generated is a
// feature the specification makes up (a feature chain, or the snapshots of a
// type) and has no source position.
type Implied struct {
	Kind   string
	Target *Element
}

// ImpliedGeneralizations returns the general types the specification implies
// for a type: for example that a part usage typed by a part definition
// subsets Parts::parts. Call it after Resolve; a type with nothing implied
// returns nil.
func (w *Workspace) ImpliedGeneralizations(e *Element) []Implied {
	var out []Implied
	for _, x := range w.w.ImpliedGeneralizations(e) {
		out = append(out, Implied{x.Kind, x.Target})
	}
	return out
}

// ImpliedFeaturingTypes returns the types a feature is featured by without
// writing a TypeFeaturing, such as the type that owns it. Call it after
// Resolve.
func (w *Workspace) ImpliedFeaturingTypes(e *Element) []*Element {
	return w.w.ImpliedFeaturingTypes(e)
}

// Binding is a binding connector the specification implies between two
// features: for example between a feature and the result of its value
// expression.
type Binding struct{ Source, Target *Element }

// ImpliedBindings returns the binding connectors implied for a type. Call it
// after Resolve.
func (w *Workspace) ImpliedBindings(e *Element) []Binding {
	var out []Binding
	for _, b := range w.w.ImpliedBindings(e) {
		out = append(out, Binding{b.Source, b.Target})
	}
	return out
}

// Validate checks the specification's constraints on the documents added to
// the workspace (not on the standard library) and returns the violations. It
// resolves the workspace first if that has not been done. Only the structural
// constraints are checked so far; see the README.
func (w *Workspace) Validate() []Diagnostic {
	w.w.Resolve()
	var out []Diagnostic
	for _, doc := range w.docs {
		for _, is := range w.w.Validate(doc.Root) {
			out = append(out, Diagnostic{
				File:       doc.Name,
				Span:       is.Element.Span,
				Message:    is.Message,
				Constraint: is.Constraint,
			})
		}
	}
	return out
}

// Seq is a sequence of values, as an expression with several results (or none,
// which is null) evaluates to.
type Seq = semantics.Seq

// EvalError says why an expression could not be evaluated, and where.
type EvalError = semantics.EvalError

// Evaluate computes the value of an expression element in a resolved
// workspace: literals, operators, sequences and references to features that
// have values. It returns an *EvalError for what it cannot yet compute, such
// as calls to library functions. The value is a bool, an int64, a float64 or a
// string when there is a single result, and a Seq for any other number.
func (w *Workspace) Evaluate(expression *Element) (any, error) { return w.w.Evaluate(expression) }

// FormatValue writes a value as KerML would: strings quoted, sequences in
// parentheses, null as null.
func FormatValue(v any) string { return semantics.FormatValue(v) }

// Materialize writes the relationships the specification implies into the
// trees of the documents added to the workspace, as elements marked Generated
// with isImplied set: the implied general types and featuring types of each
// type and the binding connectors the transformation adds. Without it they are
// available from ImpliedGeneralizations, ImpliedFeaturingTypes and
// ImpliedBindings but not in the tree. It resolves the workspace first, and
// does nothing for a document already materialized.
func (w *Workspace) Materialize() {
	w.w.Resolve()
	for _, d := range w.docs {
		w.w.Materialize(d.Root)
	}
}

// Lookup finds an element by its qualified name from the global scope, such as
// "ScalarValues::Real" or "Fleet::'Heavy Truck'", searching the documents and
// the standard library. It returns nil if there is no such element. Call it
// after Resolve.
func (w *Workspace) Lookup(qualifiedName string) *Element { return w.w.Lookup(qualifiedName) }

// Member finds the member of a namespace with the given name among those
// visible from outside it: owned, inherited and public imported members. It
// returns nil if there is none.
func (w *Workspace) Member(namespace *Element, name string) *Element {
	return w.w.Member(namespace, name)
}

// Members lists the elements visible from outside a namespace: owned,
// inherited and public imported, less any that another redefines.
func (w *Workspace) Members(namespace *Element) []*Element { return w.w.Members(namespace) }

// OwnedMembers lists the elements a namespace owns, in source order.
func (w *Workspace) OwnedMembers(namespace *Element) []*Element { return w.w.OwnedMembers(namespace) }

// EffectiveName is the name of an element: its declared name, or for a feature
// without one the name of the feature it redefines.
func (w *Workspace) EffectiveName(e *Element) string { return w.w.EffectiveName(e) }

// Supertypes lists the types a type specializes directly, whether the source
// says so or the specification implies it.
func (w *Workspace) Supertypes(t *Element) []*Element { return w.w.Supertypes(t) }

// Specializes reports whether t is, or directly or indirectly specializes,
// general.
func (w *Workspace) Specializes(t, general *Element) bool { return w.w.Specializes(t, general) }

// Features lists the features of a type, owned then inherited, less any that
// another of them redefines.
func (w *Workspace) Features(t *Element) []*Element { return w.w.Features(t) }

// TypesOf lists the types of a feature: its own, implied and inherited types.
func (w *Workspace) TypesOf(f *Element) []*Element { return w.w.TypesOf(f) }

// QualifiedName names an element by the declared names along its ownership
// chain, such as "Vehicles::Car::engine". It is "" for an element with no
// named owner. A name that is not a plain identifier is quoted.
func QualifiedName(e *Element) string { return semantics.QualifiedName(e) }

// FileOf is the name of the file an element came from. It is "library:" followed
// by the path for an element of the standard library, and "" for an element a
// rule generated that cannot be traced to a file.
func (w *Workspace) FileOf(e *Element) string { return w.w.FileOf(e) }
