// SPDX-License-Identifier: EPL-2.0
//
// Portions derived from the SysML v2 Pilot Implementation
// (org.omg.kerml.xtext scoping, and org.omg.sysml.logic NamespaceUtil, util and
// adapters), Copyright (c) 2018-2025 Model Driven Solutions, Inc. and others,
// EPL-2.0. See NOTICE.

package semantics

import (
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/library"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/postprocess"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/schema"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/syntax"
)

type el = model.Element

// Workspace is a set of parsed files, each a root Namespace, that resolve
// names against each other. The roots are searched, in order, as the global
// scope.
type Workspace struct {
	roots []*el
	files map[*el]string // the file name of each root

	memo     map[*model.Ref]*el // resolved references, nil results included
	done     map[*model.Ref]bool
	busy     map[*model.Ref]bool // resolutions in progress, to cut cycles
	cut      int                 // incremented whenever a circular resolution is cut
	effNames map[*el][2]string   // cache: effective short name, name
	libCache map[string]*el      // library elements by qualified name
	implMemo map[*el][]implied
	implBusy map[*el]bool
	chains   map[[2]*el]*el // generated feature chains, by their two features
	chainOf  map[*el][]*el  // the chaining features of a generated chain
	gaps     map[string]int // things met that this resolver does not handle
}

// NewWorkspace returns an empty workspace.
func NewWorkspace() *Workspace {
	return &Workspace{
		files: map[*el]string{},
		memo:  map[*model.Ref]*el{}, done: map[*model.Ref]bool{}, busy: map[*model.Ref]bool{},
		effNames: map[*el][2]string{}, libCache: map[string]*el{},
		implMemo: map[*el][]implied{}, implBusy: map[*el]bool{},
		chains: map[[2]*el]*el{}, chainOf: map[*el][]*el{}, gaps: map[string]int{},
	}
}

// Add puts a file's root Namespace in the workspace.
func (w *Workspace) Add(root *el) { w.roots = append(w.roots, root) }

// Gaps reports the situations the resolver met and does not yet handle, by
// description.
func (w *Workspace) Gaps() map[string]int { return w.gaps }

// ---- navigating the ownership structure ----

// rels are the relationships an element owns, optionally only of a class.
func (w *Workspace) rels(e *el, class string) []*el {
	var out []*el
	for _, r := range e.Children("ownedRelationship") {
		if class == "" || r.IsA(class) {
			out = append(out, r)
		}
	}
	return out
}

func (w *Workspace) ownedRelated(rel *el) []*el { return rel.Children("ownedRelatedElement") }

func first(l []*el) *el {
	if len(l) == 0 {
		return nil
	}
	return l[0]
}

// owningNamespace is the namespace of the membership that holds e, if any.
func (w *Workspace) owningNamespace(e *el) *el {
	if e.Owner.IsA("Membership") {
		return e.Owner.Owner
	}
	return nil
}

// parentNamespace follows NamespaceUtil.getParentNamespaceOf: the nearest
// Namespace containing an element.
func (w *Workspace) parentNamespace(e *el) *el {
	for p := e.Owner; p != nil; p = p.Owner {
		if p.IsA("Namespace") {
			return p
		}
	}
	return nil
}

func (w *Workspace) owningMembership(e *el) *el {
	if e.Owner.IsA("Membership") {
		return e.Owner
	}
	return nil
}

// expressionNamespaceOf follows NamespaceUtil.getExpressionNamespaceOf.
func (w *Workspace) expressionNamespaceOf(e *el) *el {
	ns := w.parentNamespace(e)
	if ns == nil {
		return nil
	}
	owning := w.owningNamespace(ns)
	rel := e
	if !e.IsA("Relationship") {
		rel = w.owningMembership(e)
	}
	if rel != nil && rel.IsA("FeatureValue") && owning.IsA("InstantiationExpression") {
		return owning
	}
	return ns
}

// nonExpressionNamespaceFor follows NamespaceUtil.getNonExpressionNamespaceFor:
// the namespace outside any enclosing instantiation or feature-reference
// expression, and, for a feature's value expression, outside the feature
// itself.
func (w *Workspace) nonExpressionNamespaceFor(e *el) *el {
	if e == nil {
		return nil
	}
	ns := w.expressionNamespaceOf(e)
	for ns != nil && (e.IsA("FeatureValue") || ns.IsA("InstantiationExpression") || ns.IsA("FeatureReferenceExpression")) {
		e = w.owningMembership(ns)
		if e == nil {
			return nil
		}
		ns = w.expressionNamespaceOf(e)
	}
	return ns
}

// ---- names and visibility ----

// defaultOf is the default of an enumeration-valued property in the schema.
func defaultOf(class, prop string) string {
	if f, _, ok := schema.FeatureOf(class, prop); ok {
		return f.Default
	}
	return ""
}

// visibility of a membership or import; the schema's defaults apply when the
// source does not say (public for a membership, private for an import).
func (w *Workspace) visibility(rel *el) string {
	if v := rel.Text("visibility"); v != "" {
		return v
	}
	return defaultOf(rel.Class, "visibility")
}

func (w *Workspace) ownedMemberships(ns *el) []*el { return w.rels(ns, "Membership") }
func (w *Workspace) ownedImports(ns *el) []*el     { return w.rels(ns, "Import") }

// memberElement of a membership: the element it owns, or the element an alias
// refers to.
func (w *Workspace) memberElement(mem *el) *el {
	if o := w.ownedRelated(mem); len(o) > 0 {
		return o[0]
	}
	for _, p := range mem.Props {
		if r, ok := p.Value.(*model.Ref); ok && p.Name == "memberElement" {
			return w.target(mem, p.Name, r)
		}
	}
	return nil
}

// refTarget is the target of the first reference a property holds.
func (w *Workspace) refTarget(e *el, role string) *el {
	for _, p := range e.Props {
		if r, ok := p.Value.(*model.Ref); ok && p.Name == role {
			return w.target(e, role, r)
		}
	}
	return nil
}

// effectiveNames follows Element.effectiveShortName and effectiveName, which
// for a Feature without a declared name fall back to the name of its
// namingFeature: the first feature its first ownedRedefinition redefines.
func (w *Workspace) effectiveNames(e *el) (short, name string) {
	short, name = e.Text("declaredShortName"), e.Text("declaredName")
	if name != "" || short != "" || !e.IsA("Feature") {
		return
	}
	if v, ok := w.effNames[e]; ok {
		return v[0], v[1]
	}
	cutBefore := w.cut
	if reds := w.rels(e, "Redefinition"); len(reds) > 0 {
		if f := w.refTarget(reds[0], "redefinedFeature"); f != nil {
			short, name = w.effectiveNames(f)
		}
	}
	if w.cut == cutBefore { // only cache a result no circular cut influenced
		w.effNames[e] = [2]string{short, name}
	}
	return
}

// names are the names a membership gives its member.
func (w *Workspace) names(mem *el) []string {
	var out []string
	if o := w.ownedRelated(mem); len(o) > 0 {
		short, name := w.effectiveNames(o[0])
		for _, n := range []string{short, name} {
			if n != "" {
				out = append(out, n)
			}
		}
	}
	for _, n := range []string{mem.Text("memberShortName"), mem.Text("memberName")} {
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

func (w *Workspace) named(mems []*el, name string) []*el {
	var out []*el
	for _, m := range mems {
		for _, n := range w.names(m) {
			if n == name {
				out = append(out, m)
				break
			}
		}
	}
	return w.removeRedefined(out)
}

// allRedefined lists the features f redefines, directly or indirectly.
func (w *Workspace) allRedefined(f *el, seen map[*el]bool) []*el {
	var out []*el
	for _, red := range w.rels(f, "Redefinition") {
		if g := w.refTarget(red, "redefinedFeature"); g != nil && !seen[g] {
			seen[g] = true
			out = append(out, g)
			out = append(out, w.allRedefined(g, seen)...)
		}
	}
	return out
}

// removeRedefined follows Type.removeRedefinedFeatures: a membership whose
// feature is redefined by another candidate is dropped.
func (w *Workspace) removeRedefined(ms []*el) []*el {
	if len(ms) < 2 {
		return ms
	}
	redefined := map[*el]bool{}
	for _, m := range ms {
		if e := w.memberElement(m); e.IsA("Feature") {
			for _, g := range w.allRedefined(e, map[*el]bool{e: true}) {
				redefined[g] = true
			}
		}
	}
	var out []*el
	for _, m := range ms {
		if e := w.memberElement(m); e == nil || !redefined[e] {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return ms
	}
	return out
}

// AddSource parses a file, applies the post-processing and structural rules, and
// adds it to the workspace. It returns the file's root Namespace.
func (w *Workspace) AddSource(lang grammar.Language, file, src string) (*el, error) {
	n, err := syntax.Parse(lang, file, src)
	if err != nil {
		return nil, err
	}
	root := w.addTree(n)
	w.files[root] = file
	return root, nil
}

func (w *Workspace) addTree(n *syntax.Node) *el {
	root := model.FromSyntax(n)
	postprocess.Run(root)
	Structure(root)
	w.Add(root)
	return root
}

// FileOf is the name of the file an element came from, or "" for a generated
// element or one that was added without a name.
func (w *Workspace) FileOf(e *el) string {
	for e.Owner != nil {
		e = e.Owner
	}
	return w.files[e]
}

// AddLibrary adds the standard library to the workspace. The library's parse
// trees are shared between workspaces; each workspace builds its own model.
func (w *Workspace) AddLibrary() error {
	files, err := library.Files()
	if err != nil {
		return err
	}
	for _, f := range files {
		w.files[w.addTree(f.Tree)] = "library:" + f.Path
	}
	return nil
}
