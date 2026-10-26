// SPDX-License-Identifier: EPL-2.0
//
// Portions derived from the SysML v2 Pilot Implementation
// (org.omg.kerml.xtext scoping, and org.omg.sysml.logic NamespaceUtil, util and
// adapters), Copyright (c) 2018-2025 Model Driven Solutions, Inc. and others,
// EPL-2.0. See NOTICE.

package semantics

import (
	"runtime/debug"
	"sync"

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

	metaIndex     map[*el][]*el  // metadata that names each element with "about"; see metadataOf
	sharedLibrary bool           // the library's roots are the process's, not this workspace's
	files         map[*el]string // the file name of each root

	memo         map[*model.Ref]*el // resolved references, nil results included
	done         map[*model.Ref]bool
	busy         map[*model.Ref]bool // resolutions in progress, to cut cycles
	cut          int                 // incremented whenever a circular resolution is cut
	effNames     map[*el][2]string   // cache: effective short name, name
	libCache     map[string]*el      // library elements by qualified name
	genMemo      map[*el][]*el
	nameMemo     map[nameKey]*el
	specMemo     map[specKey]bool         // whether a library type specializes another
	closureMemo  map[*el]map[*el]bool     // a type and the types reachable from it
	featsMemo    map[*el][]*el            // the features of a type, owned then inherited
	ownedIdx     map[*el]map[string][]*el // a namespace's own memberships by the names they give
	materialized map[*el]bool
	relsMemo     map[relsKey][]*el
	featMemo     map[*el][]*el
	snapshots    map[*el]*el
	featBusy     map[*el]bool
	implMemo     map[*el][]implied
	implBusy     map[*el]bool
	chains       map[[2]*el]*el // generated feature chains, by their two features
	chainOf      map[*el][]*el  // the chaining features of a generated chain
	gaps         map[string]int // things met that this resolver does not handle
}

// NewWorkspace returns an empty workspace.
func NewWorkspace() *Workspace {
	return &Workspace{
		files: map[*el]string{},
		memo:  map[*model.Ref]*el{}, done: map[*model.Ref]bool{}, busy: map[*model.Ref]bool{},
		effNames: map[*el][2]string{}, libCache: map[string]*el{},
		genMemo: map[*el][]*el{}, nameMemo: map[nameKey]*el{}, specMemo: map[specKey]bool{}, featsMemo: map[*el][]*el{}, ownedIdx: map[*el]map[string][]*el{}, materialized: map[*el]bool{}, relsMemo: map[relsKey][]*el{}, featMemo: map[*el][]*el{}, snapshots: map[*el]*el{}, featBusy: map[*el]bool{}, implMemo: map[*el][]implied{}, implBusy: map[*el]bool{},
		chains: map[[2]*el]*el{}, chainOf: map[*el][]*el{}, gaps: map[string]int{},
	}
}

// Add puts a file's root Namespace in the workspace.
func (w *Workspace) Add(root *el) {
	w.roots = append(w.roots, root)
	w.metaIndex = nil
}

// Gaps reports the situations the resolver met and does not yet handle, by
// description.
func (w *Workspace) Gaps() map[string]int { return w.gaps }

// ---- navigating the ownership structure ----

// rels are the relationships an element owns, optionally only of a class.
func (w *Workspace) rels(e *el, class string) []*el {
	k := relsKey{e, class}
	if v, ok := w.relsMemo[k]; ok {
		return v
	}
	out := w.computeRels(e, class)
	w.relsMemo[k] = out
	return out
}

type relsKey struct {
	e     *el
	class string
}

func (w *Workspace) computeRels(e *el, class string) []*el {
	var out []*el
	for _, r := range e.Children("ownedRelationship") {
		if r.IsTrue("isImplied") {
			continue // written into the tree by Materialize: not the source's
		}
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
	} else if (e.Owner.IsA("VariantMembership") || e.IsA("PerformActionUsage")) && len(w.rels(e, "ReferenceSubsetting")) > 0 {
		// a variant that names an existing usage (variant q { ... }) is that
		// usage under the variation point, and is called what it is
		if f := w.refTarget(w.rels(e, "ReferenceSubsetting")[0], "referencedFeature"); f != nil {
			// perform a.b references the anonymous chain a.b, which is
			// called what its last feature is
			if g := w.basicFeature(f); g != nil {
				short, name = w.effectiveNames(g)
			}
		}
	} else {
		// the namingFeature is the first ownedRedefinition, which in the
		// pilot includes the ones the transformation adds: an unnamed return
		// parameter is named for the result it implicitly redefines
		for _, im := range w.implicit(e) {
			if im.kind == "Redefinition" {
				short, name = w.effectiveNames(im.target)
				break
			}
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
// AddSourceRecovering is AddSource for a file with syntax errors: it reports
// all of them and adds the statements that did parse, if any (the root is nil
// otherwise).
func (w *Workspace) AddSourceRecovering(lang grammar.Language, file, src string) (*el, []*syntax.Error) {
	n, errs := syntax.ParseRecover(lang, file, src)
	if n == nil {
		return nil, errs
	}
	root := w.addTree(n)
	w.files[root] = file
	return root, errs
}

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
	root := buildTree(n)
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

// The standard library's model is built once for the process and shared by every
// workspace that adds it: building it is most of what a workspace costs (about
// 25 MB, and several times that in garbage on the way), and nothing changes it
// afterwards. The names in it resolve on demand into each workspace's own memo
// maps, and the rules that edit elements run on a workspace's own files only.
// ResolveAll, which does edit the library's elements, works on a private copy.
var shared struct {
	once  sync.Once
	roots []*el
	paths []string
	err   error
}

func sharedLibrary() ([]*el, []string, error) {
	shared.once.Do(func() {
		files, err := library.Files()
		if err != nil {
			shared.err = err
			return
		}
		for _, f := range files {
			shared.roots = append(shared.roots, buildTree(f.Tree))
			shared.paths = append(shared.paths, f.Path)
		}
		library.Release()    // the model is built, and the parse trees are not needed again
		debug.FreeOSMemory() // building it left much of its garbage, which the process need not keep
	})
	return shared.roots, shared.paths, shared.err
}

// buildTree makes the model of a syntax tree: its elements with owners, the
// parser's post-processing, and the structure the specification implies.
func buildTree(n *syntax.Node) *el {
	root := model.FromSyntax(n)
	postprocess.Run(root)
	Structure(root)
	return root
}

// AddLibrary adds the standard library to the workspace. Its model is shared
// with every other workspace that adds it, and must not be modified.
func (w *Workspace) AddLibrary() error {
	roots, paths, err := sharedLibrary()
	if err != nil {
		return err
	}
	for i, root := range roots {
		w.roots = append(w.roots, root)
		w.files[root] = "library:" + paths[i]
	}
	w.metaIndex = nil
	w.sharedLibrary = true
	return nil
}

// privateLibrary replaces the shared model of the standard library by a copy of
// the workspace's own, and forgets everything computed so far, which refers to
// the shared elements.
func (w *Workspace) privateLibrary() error {
	if !w.sharedLibrary {
		return nil
	}
	files, err := library.Files()
	if err != nil {
		return err
	}
	var roots []*el
	for _, r := range w.roots {
		if !w.isLibrary(r) {
			roots = append(roots, r)
		}
	}
	for r := range w.files {
		if w.isLibrary(r) {
			delete(w.files, r)
		}
	}
	for _, f := range files {
		root := buildTree(f.Tree)
		roots = append(roots, root)
		w.files[root] = "library:" + f.Path
	}
	w.roots = roots
	fresh := NewWorkspace()
	w.memo, w.done, w.busy, w.effNames, w.libCache = fresh.memo, fresh.done, fresh.busy, fresh.effNames, fresh.libCache
	w.genMemo, w.nameMemo, w.specMemo, w.featsMemo, w.ownedIdx, w.materialized, w.relsMemo, w.featMemo = fresh.genMemo, fresh.nameMemo, fresh.specMemo, fresh.featsMemo, fresh.ownedIdx, fresh.materialized, fresh.relsMemo, fresh.featMemo
	w.snapshots, w.featBusy, w.implMemo, w.implBusy, w.chains, w.chainOf = fresh.snapshots, fresh.featBusy, fresh.implMemo, fresh.implBusy, fresh.chains, fresh.chainOf
	w.metaIndex = nil
	w.sharedLibrary = false
	return nil
}

// Remove drops a file from the workspace. Everything computed about the files
// that are not part of the standard library is forgotten with it, since those
// results may refer to the removed file; the references in those files are
// unresolved again until the next Resolve. What was computed about the standard
// library is kept.
func (w *Workspace) Remove(root *el) {
	var kept []*el
	for _, r := range w.roots {
		if r != root {
			kept = append(kept, r)
		}
	}
	w.roots = kept
	w.metaIndex = nil
	// the elements and references whose results are to be forgotten
	user := map[*el]bool{}
	var refs []*model.Ref
	forget := func(r *el) {
		r.Walk(func(e *el) {
			user[e] = true
			for _, p := range e.Props {
				if ref, ok := p.Value.(*model.Ref); ok {
					refs = append(refs, ref)
				}
			}
		})
	}
	forget(root)
	for _, r := range w.roots {
		if !w.isLibrary(r) {
			forget(r)
		}
	}
	delete(w.files, root)
	for _, ref := range refs {
		delete(w.memo, ref)
		delete(w.done, ref)
		delete(w.busy, ref)
		if ref.Name != "" {
			ref.Target = nil
		}
	}
	for k := range w.relsMemo {
		if user[k.e] {
			delete(w.relsMemo, k)
		}
	}
	for k, m := range w.nameMemo {
		if user[k.s.ns] || user[m] {
			delete(w.nameMemo, k)
		}
	}
	// Resolution makes helper elements that no tree owns (the chain features of
	// a.b, the features for the snapshots of a type). The walk above cannot find
	// them, so what is remembered about them is found by what it points at: an
	// entry whose value is in a forgotten tree is dropped, and with it the last
	// thing that kept the tree alive.
	touches := func(es []*el) bool {
		for _, x := range es {
			if user[x] {
				return true
			}
		}
		return false
	}
	for k, idx := range w.ownedIdx {
		drop := user[k]
		for _, ms := range idx {
			if drop || touches(ms) {
				drop = true
				break
			}
		}
		if drop {
			delete(w.ownedIdx, k)
		}
	}
	for k, v := range w.featsMemo {
		if user[k] || touches(v) {
			delete(w.featsMemo, k)
		}
	}
	for k, v := range w.genMemo {
		if touches(v) {
			delete(w.genMemo, k)
		}
	}
	for k, v := range w.featMemo {
		if touches(v) {
			delete(w.featMemo, k)
		}
	}
	for k, v := range w.relsMemo {
		if touches(v) {
			delete(w.relsMemo, k)
		}
	}
	for k, v := range w.implMemo {
		for _, x := range v {
			if user[x.target] {
				delete(w.implMemo, k)
				break
			}
		}
	}
	for c, parts := range w.chainOf {
		if touches(parts) {
			user[c] = true // the chain feature goes too, and what is known of it
			delete(w.chainOf, c)
		}
	}
	for k, c := range w.snapshots {
		if user[k] {
			user[c] = true
		}
	}
	for k, c := range w.chains {
		if user[k[0]] || user[k[1]] {
			user[c] = true
		}
	}
	for k := range w.relsMemo { // again: the helper elements were added to user above
		if user[k.e] {
			delete(w.relsMemo, k)
		}
	}
	for e := range user {
		delete(w.effNames, e)
		delete(w.featMemo, e)
		delete(w.featBusy, e)
		delete(w.implMemo, e)
		delete(w.genMemo, e)
		delete(w.implBusy, e)
		delete(w.snapshots, e)
		delete(w.chainOf, e)
		delete(w.materialized, e)
	}
	for k, c := range w.chains {
		if user[k[0]] || user[k[1]] {
			delete(w.chains, k)
			delete(w.chainOf, c)
		}
	}
	w.cut++ // anything cached under a cut resolution is suspect
	w.libCache = map[string]*el{}
}
