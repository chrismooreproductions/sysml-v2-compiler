// SPDX-License-Identifier: EPL-2.0
//
// Portions derived from the SysML v2 Pilot Implementation
// (org.omg.kerml.xtext scoping: KerMLScope, KerMLScopeProvider and
// KerMLGlobalScope; org.omg.sysml.xtext SysMLScopeProvider), Copyright (c)
// 2018-2025 Model Driven Solutions, Inc. and others, EPL-2.0. See NOTICE.

package semantics

import (
	"strings"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
)

// Name resolution follows the specification's operations on Namespace
// (resolve, resolveLocal, resolveVisible, visibleMemberships,
// importedMemberships) as defined in the metamodel, and the pilot's scope
// providers for where a search starts for each kind of reference.

// ---- inheritance, imports, visible memberships ----

// inherited memberships of a type: the visible and protected memberships of
// each general type, and what they inherit in turn.
func (w *Workspace) inherited(t *el, seen map[*el]bool) []*el {
	if !t.IsA("Type") {
		return nil
	}
	var out []*el
	for _, g := range w.generalTypes(t) {
		if seen[g] {
			continue
		}
		seen[g] = true
		out = append(out, w.membershipsOf(g, false, seen, true)...)
	}
	return out
}

// imported memberships of a namespace.
func (w *Workspace) imported(ns *el, inside bool, seen map[*el]bool) []*el {
	var out []*el
	for _, imp := range w.ownedImports(ns) {
		if !inside && w.visibility(imp) != "public" {
			continue
		}
		out = append(out, w.importedBy(imp, seen)...)
	}
	return out
}

// importedBy are the memberships one import brings in: Import.importedMemberships.
func (w *Workspace) importedBy(imp *el, seen map[*el]bool) []*el {
	var out []*el
	recursive := imp.IsTrue("isRecursive")
	all := imp.IsTrue("isImportAll")
	if imp.IsA("NamespaceImport") {
		target := w.refTarget(imp, "importedNamespace")
		if target == nil || seen[target] {
			return nil
		}
		seen[target] = true
		out = append(out, w.visibleMemberships(target, recursive, all, seen)...)
		delete(seen, target)
		return out
	}
	m := w.refTarget(imp, "importedMembership")
	if m == nil {
		return nil
	}
	out = append(out, m)
	if recursive {
		if e := w.memberElement(m); e.IsA("Namespace") && !seen[e] {
			seen[e] = true
			out = append(out, w.visibleMemberships(e, true, all, seen)...)
			delete(seen, e)
		}
	}
	return out
}

// ImportedElements are the elements one import brings in, in order, with the
// memberships' visibility and the import's isRecursive and isImportAll taken
// into account (Import.importedMemberships). It is nil when the import does not
// resolve.
func (w *Workspace) ImportedElements(imp *el) []*el {
	var out []*el
	for _, m := range w.importedBy(imp, map[*el]bool{}) {
		if e := w.memberElement(m); e != nil {
			out = append(out, e)
		}
	}
	return out
}

// membershipsOf lists a namespace's memberships: owned (all of them when
// inside, otherwise public, plus protected when inherited), then inherited,
// then imported.
func (w *Workspace) membershipsOf(ns *el, inside bool, seen map[*el]bool, inheriting bool) []*el {
	var out []*el
	for _, m := range w.ownedMemberships(ns) {
		v := w.visibility(m)
		if inside || v == "public" || (v == "protected" && inheriting) {
			out = append(out, m)
		}
	}
	out = append(out, w.inherited(ns, seen)...)
	out = append(out, w.imported(ns, inside, seen)...)
	return out
}

func (w *Workspace) visibleMemberships(ns *el, recursive, includeAll bool, seen map[*el]bool) []*el {
	var out []*el
	for _, m := range w.ownedMemberships(ns) {
		if includeAll || w.visibility(m) == "public" {
			out = append(out, m)
		}
	}
	out = append(out, w.inherited(ns, seen)...)
	out = append(out, w.imported(ns, includeAll, seen)...)
	if recursive {
		for _, m := range w.ownedMemberships(ns) {
			if e := w.memberElement(m); e.IsA("Namespace") && (includeAll || w.visibility(m) == "public") && !seen[e] {
				seen[e] = true
				out = append(out, w.visibleMemberships(e, true, includeAll, seen)...)
				delete(seen, e)
			}
		}
	}
	return out
}

// ---- the memberships that give a name ----
//
// The functions above list every membership a namespace sees, and a lookup
// wants the few that give one name. The functions below walk the same
// namespaces in the same order, changing the set of those seen the same way,
// and keep only the memberships that give the name, so that the answer is the
// one the lists would give without making the lists. A namespace's own
// memberships are found in an index by name.

// ownedNamed are the namespace's own memberships that give a name.
func (w *Workspace) ownedNamed(ns *el, name string) []*el {
	idx, ok := w.ownedIdx[ns]
	if !ok {
		cutBefore := w.cut
		idx = map[string][]*el{}
		for _, m := range w.ownedMemberships(ns) {
			given := w.names(m)
		names:
			for i, n := range given {
				for _, p := range given[:i] {
					if p == n {
						continue names // a membership is listed once under a name
					}
				}
				idx[n] = append(idx[n], m)
			}
		}
		if w.cut == cutBefore {
			w.ownedIdx[ns] = idx
		}
	}
	return idx[name]
}

func (w *Workspace) membershipsNamed(ns *el, inside bool, seen map[*el]bool, inheriting bool, name string) []*el {
	var out []*el
	for _, m := range w.ownedNamed(ns, name) {
		v := w.visibility(m)
		if inside || v == "public" || (v == "protected" && inheriting) {
			out = append(out, m)
		}
	}
	out = append(out, w.inheritedNamed(ns, seen, name)...)
	out = append(out, w.importedNamed(ns, inside, seen, name)...)
	return out
}

func (w *Workspace) inheritedNamed(t *el, seen map[*el]bool, name string) []*el {
	if !t.IsA("Type") {
		return nil
	}
	var out []*el
	for _, g := range w.generalTypes(t) {
		if seen[g] {
			continue
		}
		seen[g] = true
		out = append(out, w.membershipsNamed(g, false, seen, true, name)...)
	}
	return out
}

func (w *Workspace) importedNamed(ns *el, inside bool, seen map[*el]bool, name string) []*el {
	var out []*el
	for _, imp := range w.ownedImports(ns) {
		if !inside && w.visibility(imp) != "public" {
			continue
		}
		out = append(out, w.importedByNamed(imp, seen, name)...)
	}
	return out
}

func (w *Workspace) importedByNamed(imp *el, seen map[*el]bool, name string) []*el {
	var out []*el
	recursive := imp.IsTrue("isRecursive")
	all := imp.IsTrue("isImportAll")
	if imp.IsA("NamespaceImport") {
		target := w.refTarget(imp, "importedNamespace")
		if target == nil || seen[target] {
			return nil
		}
		seen[target] = true
		out = append(out, w.visibleNamed(target, recursive, all, seen, name)...)
		delete(seen, target)
		return out
	}
	m := w.refTarget(imp, "importedMembership")
	if m == nil {
		return nil
	}
	for _, n := range w.names(m) {
		if n == name {
			out = append(out, m)
			break
		}
	}
	if recursive {
		if e := w.memberElement(m); e.IsA("Namespace") && !seen[e] {
			seen[e] = true
			out = append(out, w.visibleNamed(e, true, all, seen, name)...)
			delete(seen, e)
		}
	}
	return out
}

func (w *Workspace) visibleNamed(ns *el, recursive, includeAll bool, seen map[*el]bool, name string) []*el {
	var out []*el
	for _, m := range w.ownedNamed(ns, name) {
		if includeAll || w.visibility(m) == "public" {
			out = append(out, m)
		}
	}
	out = append(out, w.inheritedNamed(ns, seen, name)...)
	out = append(out, w.importedNamed(ns, includeAll, seen, name)...)
	if recursive {
		for _, m := range w.ownedMemberships(ns) {
			if e := w.memberElement(m); e.IsA("Namespace") && (includeAll || w.visibility(m) == "public") && !seen[e] {
				seen[e] = true
				out = append(out, w.visibleNamed(e, true, includeAll, seen, name)...)
				delete(seen, e)
			}
		}
	}
	return out
}

// ---- resolution ----

// splitName splits a written qualified name into unquoted segments.
func splitName(s string) []string {
	var segs []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\'':
			inQuote = !inQuote
		case !inQuote && s[i] == ':' && i+1 < len(s) && s[i+1] == ':':
			segs = append(segs, cur.String())
			cur.Reset()
			i++
		default:
			cur.WriteByte(s[i])
		}
	}
	return append(segs, cur.String())
}

// global looks a simple name up among the public members of the root
// namespaces.
func (w *Workspace) global(name string) []*el {
	var out []*el
	for _, r := range w.roots {
		for _, m := range w.ownedMemberships(r) {
			if w.visibility(m) == "public" {
				out = append(out, m)
			}
		}
	}
	return w.named(out, name)
}

// resolveLocal searches the namespace, then outward through the containing
// namespaces, then the global scope.
func (w *Workspace) resolveLocal(ns *el, name string, redefinition bool) *el {
	first := true
	for cur := ns; cur != nil; cur = w.parentNamespace(cur) {
		var ms []*el
		if redefinition && first && cur.IsA("Type") {
			ms = w.removeRedefined(w.inheritedNamed(cur, map[*el]bool{cur: true}, name))
		} else {
			ms = w.removeRedefined(w.membershipsNamed(cur, true, map[*el]bool{cur: true}, false, name))
		}
		first = false
		if len(ms) > 0 {
			return ms[0]
		}
	}
	if gs := w.global(name); len(gs) > 0 {
		return gs[0]
	}
	return nil
}

func (w *Workspace) resolveVisible(ns *el, name string) *el {
	ms := w.removeRedefined(w.membershipsNamed(ns, false, map[*el]bool{ns: true}, true, name))
	if len(ms) > 0 {
		return ms[0]
	}
	return nil
}

type scope struct {
	ns           *el
	inside       bool // search outward through containing namespaces
	redefinition bool
}

type nameKey struct {
	s    scope
	text string
}

// resolveName resolves a written name to a Membership from a scope. The answer
// is remembered unless a circular resolution was cut to get it.
func (w *Workspace) resolveName(s scope, text string) *el {
	key := nameKey{s, text}
	if m, ok := w.nameMemo[key]; ok {
		return m
	}
	cutBefore := w.cut
	m := w.resolveNameUncached(s, text)
	if w.cut == cutBefore {
		w.nameMemo[key] = m
	}
	return m
}

func (w *Workspace) resolveNameUncached(s scope, text string) *el {
	if rest, ok := strings.CutPrefix(text, "~"); ok {
		return w.conjugatedPortOf(w.resolveName(s, rest))
	}
	segs := splitName(text)
	if segs[0] == "$" {
		w.gaps["global-qualified"]++
		return nil
	}
	var m *el
	if s.inside {
		m = w.resolveLocal(s.ns, segs[0], s.redefinition)
	} else if s.ns != nil {
		m = w.resolveVisible(s.ns, segs[0])
	}
	for _, seg := range segs[1:] {
		if m == nil {
			return nil
		}
		e := w.memberElement(m)
		if !e.IsA("Namespace") {
			return nil
		}
		m = w.resolveVisible(e, seg)
	}
	return m
}

// conjugatedPortOf is the membership of the ConjugatedPortDefinition that a
// port definition owns, given the membership that names the port definition.
// The written name of a conjugated port typing is "~" and the port
// definition's name (ConjugatedPortTyping in the Xtext grammar).
func (w *Workspace) conjugatedPortOf(mem *el) *el {
	if mem == nil {
		return nil // the port definition does not resolve (it may be in another file)
	}
	port := w.memberElement(mem)
	if !port.IsA("PortDefinition") {
		return nil
	}
	for _, m := range w.ownedMemberships(port) {
		if w.memberElement(m).IsA("ConjugatedPortDefinition") {
			return m
		}
	}
	return nil
}

// libraryElement resolves a qualified library name from the global scope.
func (w *Workspace) libraryElement(qn string) *el {
	if e, ok := w.libCache[qn]; ok {
		return e
	}
	var e *el
	if m := w.resolveName(scope{inside: true}, qn); m != nil {
		e = w.memberElement(m)
	}
	w.libCache[qn] = e
	return e
}

// assignmentTarget is the feature the expression an assignment is made on
// names: the counter of "assign counter.count := 0". It is the result of the
// expression the assignment's first parameter is bound to.
func (w *Workspace) assignmentTarget(a *el) *el {
	for _, m := range w.rels(a, "ParameterMembership") {
		p := first(w.ownedRelated(m))
		if p == nil {
			continue
		}
		for _, fv := range w.rels(p, "FeatureValue") {
			if x := first(w.ownedRelated(fv)); x != nil {
				return w.chainResult(x)
			}
		}
		return nil
	}
	return nil
}

// chainResult returns the feature an expression's result is, for the
// expressions that name one: a feature reference, or a feature chain's target.
// Other kinds of expression need type inference and are a gap.
func (w *Workspace) chainResult(e *el) *el {
	switch {
	case e.IsA("FeatureChainExpression"), e.IsA("FeatureReferenceExpression"):
		return w.chainTarget(e)
	case e.IsA("ConstructorExpression"), e.IsA("InvocationExpression") && !e.IsA("OperatorExpression"):
		// new A().g, f(x).g: the result of the expression, which is typed by
		// the type it instantiates or by what the function returns
		if r := w.resultOf(e); r != nil {
			return r
		}
	case e.IsA("IndexExpression"), e.IsA("SelectExpression"):
		// IndexExpressionAdapter and SelectExpressionAdapter: the result
		// subsets the result of the first argument, unless that is a collection
		if arg := w.firstArgument(e); arg != nil {
			if src := w.chainResult(arg); src != nil && !w.specializes(src, w.libraryElement("Collections::Collection")) {
				return src
			}
		}
	case e.IsA("OperatorExpression"):
		// a cast (x as T) writes its result parameter and types it
		for _, r := range w.rels(e, "ReturnParameterMembership") {
			if f := first(w.ownedRelated(r)); f != nil && len(w.rels(f, "FeatureTyping")) > 0 {
				return f
			}
		}
	}
	w.gaps["chain-source:"+e.Class]++
	return nil
}

// chainTarget is the feature named by an expression's own plain Membership.
func (w *Workspace) chainTarget(e *el) *el {
	for _, r := range w.rels(e, "Membership") {
		if r.Class == "Membership" {
			return w.memberElement(r)
		}
	}
	return nil
}

// firstArgument is the expression that is an expression's first operand.
func (w *Workspace) firstArgument(e *el) *el {
	for _, r := range w.rels(e, "ParameterMembership") {
		if r.IsA("ReturnParameterMembership") {
			continue
		}
		param := first(w.ownedRelated(r))
		if param == nil {
			return nil
		}
		for _, fv := range w.rels(param, "FeatureValue") {
			return first(w.ownedRelated(fv))
		}
		return nil
	}
	return nil
}

// chainSource is the feature a FeatureChainExpression navigates from: the
// result of its first operand.
func (w *Workspace) chainSource(fce *el) *el {
	for _, r := range w.rels(fce, "ParameterMembership") {
		if r.IsA("ReturnParameterMembership") {
			continue
		}
		param := first(w.ownedRelated(r))
		if param == nil {
			return nil
		}
		for _, fv := range w.rels(param, "FeatureValue") {
			if expr := first(w.ownedRelated(fv)); expr != nil {
				return w.chainResult(expr)
			}
		}
		return nil
	}
	return nil
}

// scopeFor chooses where a reference's search starts, after the pilot's
// KerMLScopeProvider.getScope and SysMLScopeProvider.getScope.
func (w *Workspace) scopeFor(owner *el, role string) scope {
	switch {
	case owner.IsA("Import"):
		return scope{ns: owner.Owner, inside: true}
	case owner.IsA("FeatureChaining"):
		chained := owner.Owner
		chain := w.rels(chained, "FeatureChaining")
		idx := 0
		for i, r := range chain {
			if r == owner {
				idx = i
			}
		}
		if idx > 0 {
			return scope{ns: w.refTarget(chain[idx-1], "chainingFeature"), inside: false}
		}
		// FeatureChainExpression: a.b.c chains b and c from what a is
		if fce := w.owningNamespace(chained); fce.IsA("FeatureChainExpression") {
			if src := w.chainSource(fce); src != nil {
				return scope{ns: src, inside: false}
			}
		}
		rel := chained.Owner // the relationship whose target this chain is
		if rel == nil || rel.Owner == nil {
			return scope{}
		}
		// the feature path an assignment assigns (assign a.b.c := 1 chains b and
		// c from what a is)
		if rel.IsA("Membership") && rel.Owner.IsA("AssignmentActionUsage") {
			if target := w.assignmentTarget(rel.Owner); target != nil {
				return scope{ns: target, inside: false}
			}
		}
		// a feature that chains (feature c chains a.b) is a member of its
		// namespace: look from where it sits
		if rel.IsA("Membership") {
			return scope{ns: w.nonExpressionNamespaceFor(chained), inside: true}
		}
		// KerMLScopeProvider.scope_featureChaining: the chain an end of a
		// connector references is looked up from outside the connector
		if rel.IsA("ReferenceSubsetting") {
			if conn := w.owningNamespace(rel.Owner); conn.IsA("Connector") {
				return scope{ns: w.parentNamespace(conn), inside: true}
			}
		}
		return scope{ns: w.parentNamespace(rel.Owner), inside: true, redefinition: rel.IsA("Redefinition")}
	case owner.IsA("Membership"):
		// SysMLScopeProvider: the source of a transition is looked up outside it.
		if tu := owner.Owner; tu.IsA("TransitionUsage") && !owner.IsA("FeatureMembership") {
			var firstPlain *el
			for _, m := range w.ownedMemberships(tu) {
				if !m.IsA("FeatureMembership") {
					firstPlain = m
					break
				}
			}
			if firstPlain == owner {
				return scope{ns: w.parentNamespace(tu), inside: true}
			}
		}
		if owner.Class == "Membership" && owner.Owner.IsA("FeatureChainExpression") {
			// the target of a chain: look among the members of the source
			return scope{ns: w.chainSource(owner.Owner), inside: false}
		}
		if owner.Class == "Membership" && owner.Owner.IsA("AssignmentActionUsage") {
			// the feature an assignment assigns (counter.count := 0): look among
			// the features of the expression it is assigned on (counter)
			if target := w.assignmentTarget(owner.Owner); target != nil {
				return scope{ns: target, inside: false}
			}
		}
		return scope{ns: w.nonExpressionNamespaceFor(owner), inside: true}
	case owner.IsA("Relationship"):
		if owner.Owner == nil {
			return scope{}
		}
		if s, ok := w.endScope(owner); ok {
			return s
		}
		ns := w.parentNamespace(owner.Owner)
		if owner.IsA("FeatureTyping") {
			ns = w.nonExpressionNamespaceFor(owner.Owner)
		} else if !owner.IsA("Specialization") && !owner.IsA("Conjugation") {
			// KerMLScopeProvider.getScope: any other element (a TypeFeaturing, a
			// Disjoining) looks from the namespace around it, which for a
			// relationship a type owns is the type itself
			ns = w.parentNamespace(owner)
		}
		return scope{ns: ns, inside: true, redefinition: owner.IsA("Redefinition") && role == "redefinedFeature"}
	}
	return scope{ns: w.parentNamespace(owner), inside: true}
}

// endScope applies the pilot's special scoping for the target of a
// ReferenceSubsetting on an end feature: KerMLScopeProvider looks it up
// outside a connector, and SysMLScopeProvider outside a transition's
// succession.
func (w *Workspace) endScope(rs *el) (scope, bool) {
	if !rs.IsA("ReferenceSubsetting") {
		return scope{}, false
	}
	f := rs.Owner
	held := f.Owner
	if !held.IsA("FeatureMembership") || held.Owner == nil {
		return scope{}, false
	}
	owningType := held.Owner
	if f.IsTrue("isEnd") && owningType.IsA("Connector") {
		return scope{ns: w.parentNamespace(owningType), inside: true}, true
	}
	if owningType.IsA("SuccessionAsUsage") && held.IsA("EndFeatureMembership") {
		ends := w.rels(owningType, "EndFeatureMembership")
		if len(ends) == 2 && ends[1] == held {
			if outer := w.owningNamespace(owningType); outer.IsA("TransitionUsage") {
				return scope{ns: w.parentNamespace(outer), inside: true}, true
			}
		}
	}
	return scope{}, false
}

// target resolves a cross-reference held by owner under role. A reference that
// already has a target (one a rule set) is returned as it is.
func (w *Workspace) target(owner *el, role string, ref *model.Ref) *el {
	if ref.Target != nil {
		return ref.Target
	}
	if ref.Name == "" {
		return nil
	}
	if w.done[ref] {
		return w.memo[ref]
	}
	if w.busy[ref] {
		w.cut++
		return nil
	}
	w.busy[ref] = true
	cutBefore := w.cut
	m := w.resolveName(w.scopeFor(owner, role), ref.Name)
	delete(w.busy, ref)
	var out *el
	if m != nil {
		if role == "importedMembership" {
			out = m
		} else {
			out = w.memberElement(m)
		}
	}
	if out == nil && w.cut != cutBefore {
		// A reference still being resolved (a sibling import, say) was cut out
		// of the search, so what was missing may be there once it is resolved:
		// do not remember the failure.
		return nil
	}
	w.done[ref] = true
	w.memo[ref] = out
	return out
}
