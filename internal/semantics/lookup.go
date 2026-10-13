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
		recursive := imp.IsTrue("isRecursive")
		all := imp.IsTrue("isImportAll")
		if imp.IsA("NamespaceImport") {
			target := w.refTarget(imp, "importedNamespace")
			if target == nil || seen[target] {
				continue
			}
			seen[target] = true
			out = append(out, w.visibleMemberships(target, recursive, all, seen)...)
			delete(seen, target)
		} else {
			m := w.refTarget(imp, "importedMembership")
			if m == nil {
				continue
			}
			out = append(out, m)
			if recursive {
				if e := w.memberElement(m); e.IsA("Namespace") && !seen[e] {
					seen[e] = true
					out = append(out, w.visibleMemberships(e, true, all, seen)...)
					delete(seen, e)
				}
			}
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
			ms = w.named(w.inherited(cur, map[*el]bool{cur: true}), name)
		} else {
			ms = w.named(w.membershipsOf(cur, true, map[*el]bool{cur: true}, false), name)
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
	ms := w.named(w.membershipsOf(ns, false, map[*el]bool{ns: true}, true), name)
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

// resolveName resolves a written name to a Membership from a scope.
func (w *Workspace) resolveName(s scope, text string) *el {
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

// chainResult returns the feature an expression's result is, for the
// expressions that name one: a feature reference, or a feature chain's target.
// Other kinds of expression need type inference and are a gap.
func (w *Workspace) chainResult(e *el) *el {
	switch {
	case e.IsA("FeatureChainExpression"), e.IsA("FeatureReferenceExpression"):
		return w.chainTarget(e)
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
	w.done[ref] = true
	w.memo[ref] = out
	return out
}
