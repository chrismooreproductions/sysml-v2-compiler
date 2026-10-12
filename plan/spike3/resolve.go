// SPDX-License-Identifier: EPL-2.0
//
// Portions derived from the SysML v2 Pilot Implementation (org.omg.kerml.xtext scoping and org.omg.sysml.logic NamespaceUtil),
// Copyright (c) 2018-2025 Model Driven Solutions, Inc. and others, EPL-2.0.
// See NOTICE.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Name resolution over the canonical element graph, following the KerML
// specification's Namespace operations (resolve, resolveLocal, resolveVisible,
// visibleMemberships, importedMemberships) as they are defined in the
// metamodel, and the pilot implementation's KerMLScopeProvider for where a
// search starts for each kind of reference.

type world struct {
	roots        []*cn             // our file roots, in file order
	mineByKey    map[string]*cn    // our elements by position
	oracleByKey  map[string]*cn    // the oracle's elements by position
	oracleByID   map[string]*cn    // file|id -> oracle element
	memo         map[string]*cn    // resolved references
	done         map[string]bool   // memo has an entry (including a nil result)
	busy         map[string]bool   // resolutions in progress, to cut cycles
	owned        map[*cn][]*cn     // cache: owned relationships by role
	gaps         map[string]int    // things met that this resolver does not handle
	effNames     map[*cn][2]string // cache: effective short name, name
	cut          int               // incremented whenever a circular resolution is cut
	libCache     map[string]*cn    // library elements by qualified name
	implMemo     map[*cn][]ig
	implBusy     map[*cn]bool
	chains       map[[2]*cn]*cn // generated feature chains, by their two features
	impliedByKey map[string]*cn // the implied serialization\'s plain elements by position
}

func assignKeys(c *cn, file, path string, parent *cn, byKey map[string]*cn) {
	c.file, c.key, c.parent = file, file+"|"+path, parent
	byKey[c.key] = c
	counts := map[string]int{}
	for i := range c.kids {
		k := c.kids[i]
		if k.n == nil {
			continue
		}
		idx := counts[k.role]
		counts[k.role]++
		assignKeys(k.n, file, path+"/"+k.role+"["+strconv.Itoa(idx)+"]", c, byKey)
	}
}

func indexIDs(c *cn, file string, byID map[string]*cn) {
	if c.id != "" {
		byID[file+"|"+c.id] = c
	}
	for _, k := range c.kids {
		if k.n != nil {
			indexIDs(k.n, file, byID)
		}
	}
}

func (w *world) is(c *cn, class string) bool { return c != nil && theMeta.isSub(c.class, class) }

func (w *world) nodes(c *cn, role string) []*cn {
	var out []*cn
	for _, k := range c.kids {
		if k.n != nil && k.role == role {
			out = append(out, k.n)
		}
	}
	return out
}

// relationships owned by an element, optionally restricted to a class.
func (w *world) rels(c *cn, class string) []*cn {
	var out []*cn
	for _, r := range w.nodes(c, "ownedRelationship") {
		if class == "" || w.is(r, class) {
			out = append(out, r)
		}
	}
	return out
}

func (w *world) ownedRelated(rel *cn) []*cn { return w.nodes(rel, "ownedRelatedElement") }

// owningNamespace is the namespace of the membership that holds e, if any.
func (w *world) owningNamespace(e *cn) *cn {
	if e.parent != nil && w.is(e.parent, "Membership") {
		return e.parent.parent
	}
	return nil
}

// parentNamespace mirrors NamespaceUtil.getParentNamespaceOf: the nearest
// Namespace containing an element.
func (w *world) parentNamespace(e *cn) *cn {
	for p := e.parent; p != nil; p = p.parent {
		if w.is(p, "Namespace") {
			return p
		}
	}
	return nil
}

func (w *world) owningMembership(e *cn) *cn {
	if e.parent != nil && w.is(e.parent, "Membership") {
		return e.parent
	}
	return nil
}

// expressionNamespaceOf mirrors NamespaceUtil.getExpressionNamespaceOf.
func (w *world) expressionNamespaceOf(e *cn) *cn {
	ns := w.parentNamespace(e)
	if ns == nil {
		return nil
	}
	owning := w.owningNamespace(ns)
	rel := e
	if !w.is(e, "Relationship") {
		rel = w.owningMembership(e)
	}
	if rel != nil && w.is(rel, "FeatureValue") && w.is(owning, "InstantiationExpression") {
		return owning
	}
	return ns
}

// nonExpressionNamespaceFor mirrors NamespaceUtil.getNonExpressionNamespaceFor:
// the namespace outside any enclosing instantiation or feature-reference
// expression, and, for a feature's value expression, outside the feature
// itself.
func (w *world) nonExpressionNamespaceFor(e *cn) *cn {
	if e == nil {
		return nil
	}
	ns := w.expressionNamespaceOf(e)
	for ns != nil && (w.is(e, "FeatureValue") || w.is(ns, "InstantiationExpression") || w.is(ns, "FeatureReferenceExpression")) {
		e = w.owningMembership(ns)
		if e == nil {
			return nil
		}
		ns = w.expressionNamespaceOf(e)
	}
	return ns
}

func (w *world) attr(c *cn, name string) string { return c.attrs[name] }

func (w *world) visibility(rel *cn) string {
	if v := rel.attrs["visibility"]; v != "" {
		return v
	}
	if w.is(rel, "Import") {
		return "private"
	}
	return "public"
}

func (w *world) ownedMemberships(ns *cn) []*cn { return w.rels(ns, "Membership") }
func (w *world) ownedImports(ns *cn) []*cn     { return w.rels(ns, "Import") }

// memberElement of a membership: the element it owns, or the element an
// alias refers to.
func (w *world) memberElement(mem *cn) *cn {
	if o := w.ownedRelated(mem); len(o) > 0 {
		return o[0]
	}
	for i, k := range mem.kids {
		if k.n == nil && k.role == "memberElement" {
			return w.resolveKid(mem, i)
		}
	}
	return nil
}

// effectiveNames mirrors Element.effectiveShortName/effectiveName, which
// for a Feature without a declared name fall back to the name of its
// namingFeature: the first feature its first ownedRedefinition redefines.
func (w *world) effectiveNames(e *cn) (short, name string) {
	short, name = e.attrs["declaredShortName"], e.attrs["declaredName"]
	if name != "" || short != "" || !w.is(e, "Feature") {
		return
	}
	if v, ok := w.effNames[e]; ok {
		return v[0], v[1]
	}
	cutBefore := w.cut
	if reds := w.rels(e, "Redefinition"); len(reds) > 0 {
		for i, k := range reds[0].kids {
			if k.n == nil && k.role == "redefinedFeature" {
				if f := w.resolveKid(reds[0], i); f != nil {
					short, name = w.effectiveNames(f)
				}
				break
			}
		}
	}
	if w.cut == cutBefore { // only cache a result no circular cut influenced
		w.effNames[e] = [2]string{short, name}
	}
	return
}

func (w *world) names(mem *cn) []string {
	var out []string
	if o := w.ownedRelated(mem); len(o) > 0 {
		short, name := w.effectiveNames(o[0])
		for _, n := range []string{short, name} {
			if n != "" {
				out = append(out, n)
			}
		}
	}
	for _, n := range []string{mem.attrs["memberShortName"], mem.attrs["memberName"]} {
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

func (w *world) named(mems []*cn, name string) []*cn {
	var out []*cn
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

// redefinedBy lists the features f redefines, directly or indirectly.
func (w *world) allRedefined(f *cn, seen map[*cn]bool) []*cn {
	var out []*cn
	for _, red := range w.rels(f, "Redefinition") {
		for i, k := range red.kids {
			if k.n == nil && k.role == "redefinedFeature" {
				if g := w.resolveKid(red, i); g != nil && !seen[g] {
					seen[g] = true
					out = append(out, g)
					out = append(out, w.allRedefined(g, seen)...)
				}
			}
		}
	}
	return out
}

// removeRedefined mirrors Type.removeRedefinedFeatures: a membership whose
// feature is redefined by another candidate is dropped.
func (w *world) removeRedefined(ms []*cn) []*cn {
	if len(ms) < 2 {
		return ms
	}
	redefined := map[*cn]bool{}
	for _, m := range ms {
		if e := w.memberElement(m); e != nil && w.is(e, "Feature") {
			for _, g := range w.allRedefined(e, map[*cn]bool{e: true}) {
				redefined[g] = true
			}
		}
	}
	var out []*cn
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

// ---- inheritance, imports, visible memberships ----

// implicitKind names the library type a metaclass implicitly specializes for
// a kind (ImplicitGeneralizationMap), searching up the metaclass hierarchy as
// the Java does up the implementation class hierarchy.
func implicitKind(class, kind string) string {
	queue, seen := []string{class}, map[string]bool{}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if seen[c] {
			continue
		}
		seen[c] = true
		if v, ok := implicitTable[c+"_"+kind]; ok {
			return v
		}
		queue = append(queue, theMeta.supers[c]...)
	}
	return ""
}

func implicitBase(class string) string { return implicitKind(class, "base") }

// libraryElement resolves a qualified library name from the global scope.
func (w *world) libraryElement(qn string) *cn {
	if e, ok := w.libCache[qn]; ok {
		return e
	}
	var e *cn
	if m := w.resolveName(scope{inside: true}, qn); m != nil {
		e = w.memberElement(m)
	}
	w.libCache[qn] = e
	return e
}

// generals are the types a type specializes or conjugates, as resolved,
// followed by its implicit general types (TypeUtil.getGeneralTypesOf).
func (w *world) generals(t *cn) []*cn { return w.generalTypes(t) }

func (w *world) generalTypes(t *cn) []*cn {
	var out []*cn
	for _, r := range w.rels(t, "Specialization") {
		for i, k := range r.kids {
			if k.n == nil && theMeta.isTargetEnd(r.class, k.role) {
				if g := w.resolveKid(r, i); g != nil {
					out = append(out, g)
				}
			}
		}
	}
	for _, r := range w.rels(t, "Conjugation") {
		for i, k := range r.kids {
			if k.n == nil && k.role == "originalType" {
				if g := w.resolveKid(r, i); g != nil {
					out = append(out, g)
				}
			}
		}
	}
	if os.Getenv("NO_IMPLICIT") == "" {
		for _, x := range w.implicit(t) {
			out = append(out, x.target)
		}
	}
	if w.is(t, "Feature") {
		if ch := w.rels(t, "FeatureChaining"); len(ch) > 0 || len(t.chain) > 0 {
			if f := w.basicFeature(t); f != nil {
				out = append(out, f)
			}
		}
	}
	return out
}

// inherited memberships of a type: the visible and protected memberships of
// each general type, and what they inherit in turn.
func (w *world) inherited(t *cn, seen map[*cn]bool) []*cn {
	if !w.is(t, "Type") {
		return nil
	}
	var out []*cn
	for _, g := range w.generals(t) {
		if seen[g] {
			continue
		}
		seen[g] = true
		out = append(out, w.membershipsOf(g, false, seen, true)...)
	}
	return out
}

// imported memberships of a namespace.
func (w *world) imported(ns *cn, inside bool, seen map[*cn]bool) []*cn {
	var out []*cn
	for _, imp := range w.ownedImports(ns) {
		if !inside && w.visibility(imp) != "public" {
			continue
		}
		recursive := imp.attrs["isRecursive"] == "true"
		all := imp.attrs["isImportAll"] == "true"
		if w.is(imp, "NamespaceImport") {
			target := w.importTarget(imp, "importedNamespace")
			if target == nil || seen[target] {
				continue
			}
			seen[target] = true
			out = append(out, w.visibleMemberships(target, recursive, all, seen)...)
			delete(seen, target)
		} else {
			m := w.importTarget(imp, "importedMembership")
			if m == nil {
				continue
			}
			out = append(out, m)
			if recursive {
				if e := w.memberElement(m); e != nil && w.is(e, "Namespace") && !seen[e] {
					seen[e] = true
					out = append(out, w.visibleMemberships(e, true, all, seen)...)
					delete(seen, e)
				}
			}
		}
	}
	return out
}

func (w *world) importTarget(imp *cn, role string) *cn {
	for i, k := range imp.kids {
		if k.n == nil && k.role == role {
			return w.resolveKid(imp, i)
		}
	}
	return nil
}

// membershipsOf lists a namespace's memberships: owned (all of them when
// inside, otherwise public, plus protected when inherited), then inherited,
// then imported.
func (w *world) membershipsOf(ns *cn, inside bool, seen map[*cn]bool, inheriting bool) []*cn {
	var out []*cn
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

func (w *world) visibleMemberships(ns *cn, recursive, includeAll bool, seen map[*cn]bool) []*cn {
	var out []*cn
	for _, m := range w.ownedMemberships(ns) {
		if includeAll || w.visibility(m) == "public" {
			out = append(out, m)
		}
	}
	out = append(out, w.inherited(ns, seen)...)
	out = append(out, w.imported(ns, includeAll, seen)...)
	if recursive {
		for _, m := range w.ownedMemberships(ns) {
			if e := w.memberElement(m); e != nil && w.is(e, "Namespace") && (includeAll || w.visibility(m) == "public") && !seen[e] {
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

func (w *world) global(name string) []*cn {
	var out []*cn
	for _, r := range w.roots {
		for _, m := range w.ownedMemberships(r) {
			if w.visibility(m) == "public" {
				out = append(out, m)
			}
		}
	}
	return w.named(out, name)
}

// resolveLocal: search the namespace, then outward through the containing
// namespaces, then the global scope.
func (w *world) resolveLocal(ns *cn, name string, redefinition bool) *cn {
	first := true
	for cur := ns; cur != nil; cur = w.parentNamespace(cur) {
		var ms []*cn
		if redefinition && first && w.is(cur, "Type") {
			ms = w.named(w.inherited(cur, map[*cn]bool{cur: true}), name)
		} else {
			ms = w.named(w.membershipsOf(cur, true, map[*cn]bool{cur: true}, false), name)
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

func (w *world) resolveVisible(ns *cn, name string) *cn {
	ms := w.named(w.membershipsOf(ns, false, map[*cn]bool{ns: true}, true), name)
	if len(ms) > 0 {
		return ms[0]
	}
	return nil
}

type scope struct {
	ns           *cn
	inside       bool // search outward through containing namespaces
	redefinition bool
}

// resolveName resolves a written name to a Membership from a scope.
func (w *world) resolveName(s scope, text string) *cn {
	segs := splitName(text)
	if segs[0] == "$" {
		w.gaps["global-qualified"]++
		return nil
	}
	var m *cn
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
		if e == nil || !w.is(e, "Namespace") {
			return nil
		}
		m = w.resolveVisible(e, seg)
	}
	return m
}

// scopeFor chooses where a reference's search starts, after the pilot's
// KerMLScopeProvider.getScope.
// chainSource returns the feature an expression's result is, for the
// expressions that name one: a feature reference, or a feature chain's
// target. Other kinds of expression need type inference and are a gap.
func (w *world) chainResult(e *cn) *cn {
	switch {
	case w.is(e, "FeatureChainExpression"):
		return w.chainTarget(e)
	case w.is(e, "FeatureReferenceExpression"):
		return w.chainTarget(e)
	}
	w.gaps["chain-source:"+e.class]++
	return nil
}

// chainTarget is the feature named by an expression's own plain Membership.
func (w *world) chainTarget(e *cn) *cn {
	for _, r := range w.rels(e, "Membership") {
		if r.class == "Membership" {
			return w.memberElement(r)
		}
	}
	return nil
}

// chainSource is the feature a FeatureChainExpression navigates from: the
// result of its first operand.
func (w *world) chainSource(fce *cn) *cn {
	for _, r := range w.rels(fce, "ParameterMembership") {
		if w.is(r, "ReturnParameterMembership") {
			continue
		}
		param := first1(w.ownedRelated(r))
		if param == nil {
			return nil
		}
		for _, fv := range w.rels(param, "FeatureValue") {
			if expr := first1(w.ownedRelated(fv)); expr != nil {
				return w.chainResult(expr)
			}
		}
		return nil
	}
	return nil
}

func first1(l []*cn) *cn {
	if len(l) == 0 {
		return nil
	}
	return l[0]
}

func (w *world) scopeFor(owner *cn, ki int) scope {
	role := owner.kids[ki].role
	switch {
	case w.is(owner, "Import"):
		return scope{ns: owner.parent, inside: true}
	case w.is(owner, "FeatureChaining"):
		chained := owner.parent
		var chain []*cn
		for _, r := range w.rels(chained, "FeatureChaining") {
			chain = append(chain, r)
		}
		idx := 0
		for i, r := range chain {
			if r == owner {
				idx = i
			}
		}
		if idx > 0 {
			prev := chain[idx-1]
			for i, k := range prev.kids {
				if k.n == nil && k.role == "chainingFeature" {
					return scope{ns: w.resolveKid(prev, i), inside: false}
				}
			}
			return scope{}
		}
		rel := chained.parent // the relationship whose target this chain is
		if rel == nil || rel.parent == nil {
			return scope{}
		}
		return scope{ns: w.parentNamespace(rel.parent), inside: true,
			redefinition: w.is(rel, "Redefinition")}
	case w.is(owner, "Membership"):
		// SysMLScopeProvider: the source of a transition is looked up outside it.
		if tu := owner.parent; w.is(tu, "TransitionUsage") && !w.is(owner, "FeatureMembership") {
			var firstPlain *cn
			for _, m := range w.ownedMemberships(tu) {
				if !w.is(m, "FeatureMembership") {
					firstPlain = m
					break
				}
			}
			if firstPlain == owner {
				return scope{ns: w.parentNamespace(tu), inside: true}
			}
		}
		if owner.class == "Membership" && w.is(owner.parent, "FeatureChainExpression") {
			// the target of a chain: look among the members of the source
			return scope{ns: w.chainSource(owner.parent), inside: false}
		}
		return scope{ns: w.nonExpressionNamespaceFor(owner), inside: true}
	case w.is(owner, "Relationship"):
		if owner.parent == nil {
			return scope{}
		}
		if s, ok := w.endScope(owner); ok {
			return s
		}
		ns := w.parentNamespace(owner.parent)
		if w.is(owner, "FeatureTyping") {
			ns = w.nonExpressionNamespaceFor(owner.parent)
		}
		return scope{ns: ns, inside: true, redefinition: w.is(owner, "Redefinition") && role == "redefinedFeature"}
	}
	return scope{ns: w.parentNamespace(owner), inside: true}
}

// endScope applies the pilot's special scoping for the target of a
// ReferenceSubsetting on an end feature: KerMLScopeProvider looks it up
// outside a connector, and SysMLScopeProvider outside a transition's
// succession.
func (w *world) endScope(rs *cn) (scope, bool) {
	if !w.is(rs, "ReferenceSubsetting") {
		return scope{}, false
	}
	f := rs.parent
	held := f.parent
	if held == nil || !w.is(held, "FeatureMembership") || held.parent == nil {
		return scope{}, false
	}
	owningType := held.parent
	if f.attrs["isEnd"] == "true" && w.is(owningType, "Connector") {
		return scope{ns: w.parentNamespace(owningType), inside: true}, true
	}
	if w.is(owningType, "SuccessionAsUsage") && w.is(held, "EndFeatureMembership") {
		ends := w.rels(owningType, "EndFeatureMembership")
		if len(ends) == 2 && ends[1] == held {
			if outer := w.owningNamespace(owningType); w.is(outer, "TransitionUsage") {
				return scope{ns: w.parentNamespace(outer), inside: true}, true
			}
		}
	}
	return scope{}, false
}

// resolveKid resolves the ki-th child of owner, a cross-reference.
func (w *world) resolveKid(owner *cn, ki int) *cn {
	k := owner.kids[ki]
	switch {
	case k.owner:
		return owner.parent
	case k.owned >= 0:
		if o := w.ownedRelated(owner); k.owned < len(o) {
			return o[k.owned]
		}
		return nil
	case k.text == "":
		return nil
	}
	key := owner.key + "#" + strconv.Itoa(ki)
	if w.done[key] {
		return w.memo[key]
	}
	if w.busy[key] {
		w.cut++
		return nil
	}
	w.busy[key] = true
	m := w.resolveName(w.scopeFor(owner, ki), k.text)
	delete(w.busy, key)
	var out *cn
	if m != nil {
		if k.role == "importedMembership" {
			out = m
		} else if w.is(owner, "Import") || w.is(owner, "Relationship") && k.role != "memberElement" || k.role == "memberElement" {
			out = w.memberElement(m)
		}
	}
	w.done[key] = true
	w.memo[key] = out
	return out
}

// ---- rules that need resolution ----

// specializes mirrors TypeUtil.specializes: a type specializes itself and
// every type reachable through its (explicit and implicit) generals.
func (w *world) specializes(t, general *cn) bool {
	if t == nil || general == nil {
		return false
	}
	seen := map[*cn]bool{}
	var walk func(x *cn) bool
	walk = func(x *cn) bool {
		if x == general {
			return true
		}
		if seen[x] {
			return false
		}
		seen[x] = true
		for _, g := range w.generals(x) {
			if walk(g) {
				return true
			}
		}
		return false
	}
	return walk(t)
}

// mayTimeVary mirrors UsageAdapter.mayTimeVary.
func (w *world) mayTimeVary(u *cn) bool {
	if !w.is(u.parent, "FeatureMembership") || u.parent.parent == nil {
		return false
	}
	owningType := u.parent.parent
	if !w.specializes(owningType, w.libraryElement("Occurrences::Occurrence")) {
		return false
	}
	return !(u.attrs["isPortion"] == "true" ||
		w.specializes(u, w.libraryElement("Links::SelfLink")) ||
		w.specializes(u, w.libraryElement("Occurrences::HappensLink")) ||
		u.attrs["isComposite"] == "true" && w.specializes(u, w.libraryElement("Actions::Action")))
}

// basicFeature mirrors FeatureUtil.getBasicFeatureOf: a chain stands for its
// last chaining feature.
func (w *world) basicFeature(f *cn) *cn {
	if f == nil {
		return nil
	}
	if len(f.chain) > 0 {
		return f.chain[len(f.chain)-1]
	}
	if ch := w.rels(f, "FeatureChaining"); len(ch) > 0 {
		last := ch[len(ch)-1]
		for i, k := range last.kids {
			if k.n == nil && k.role == "chainingFeature" {
				return w.resolveKid(last, i)
			}
		}
		return nil
	}
	return f
}

func (w *world) owningType(f *cn) *cn {
	if w.is(f.parent, "FeatureMembership") {
		return f.parent.parent
	}
	return nil
}

// wantsDefaultMultiplicity completes UsageAdapter.isAddDefaultMultiplicity,
// the clause semantic.go could only approximate: an attribute, item or port
// usage owned by a type gets a default multiplicity unless a feature it
// subsets is itself owned by a type.
func (w *world) wantsDefaultMultiplicity(u *cn) bool {
	if !(w.is(u, "AttributeUsage") || w.is(u, "ItemUsage") || w.is(u, "PortUsage")) || w.is(u, "ConnectionUsage") {
		return false
	}
	if u.attrs["isEnd"] == "true" || w.owningType(u) == nil {
		return false
	}
	for _, m := range w.rels(u, "") {
		if member := first1(w.ownedRelated(m)); member != nil && w.is(member, "Multiplicity") {
			return false
		}
	}
	subs := w.rels(u, "Subsetting")
	if len(subs) == 0 {
		return false // already added at the node level
	}
	for _, s := range subs {
		for i, k := range s.kids {
			if k.n == nil && theMeta.isTargetEnd(s.class, k.role) {
				if f := w.basicFeature(w.resolveKid(s, i)); f != nil && w.owningType(f) != nil {
					return false
				}
			}
		}
	}
	return true
}

// applyResolutionRules applies the adapter rules that need name resolution:
// UsageAdapter.doTransform makes an end usage constant if it may time-vary,
// and the default-multiplicity clause that asks about subsetted features.
func (w *world) applyResolutionRules() {
	var visit func(c *cn)
	visit = func(c *cn) {
		if w.is(c, "Usage") && c.attrs["isEnd"] == "true" && w.mayTimeVary(c) {
			c.attrs["isConstant"] = "true"
		}
		if w.wantsDefaultMultiplicity(c) {
			mult := &cn{class: "Multiplicity", attrs: map[string]string{}}
			mem := &cn{class: "OwningMembership", attrs: map[string]string{}, parent: c}
			mult.parent = mem
			mem.kids = []ckid{{role: "ownedRelatedElement", n: mult, owned: -1}}
			c.kids = append(c.kids, ckid{role: "ownedRelationship", n: mem, owned: -1})
		}
		for _, k := range c.kids {
			if k.n != nil {
				visit(k.n)
			}
		}
	}
	for _, r := range w.roots {
		visit(r)
	}
}

// ---- driver and report ----

func describe(c *cn) string {
	var parts []string
	for e := c; e != nil; e = e.parent {
		if n := e.attrs["declaredName"]; n != "" {
			parts = append(parts, n)
		} else if n := e.attrs["declaredShortName"]; n != "" {
			parts = append(parts, "<"+n+">")
		} else if n := e.attrs["memberName"]; n != "" {
			parts = append(parts, n)
		}
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	if len(parts) == 0 {
		return "(" + c.class + " " + c.key + ")"
	}
	return strings.Join(parts, "::") + " (" + c.class + ")"
}

func worldMain() {
	libRoot, xmiRoot := os.Getenv("LIB_ROOT"), os.Getenv("XMI_ROOT")
	grammars := map[string]*Grammar{}
	xrefProps = map[string]bool{}
	for ext, which := range map[string]string{".sysml": "sysml", ".kerml": "kerml"} {
		g, err := loadGrammar("xtext-ir.json", which, theMeta)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		grammars[ext] = g
		for _, p := range g.prods {
			walk(p.Body, func(e *Expr) {
				if e.K == "assign" && e.Kids[0].K == "xref" {
					xrefProps[e.Text] = true
				}
			})
		}
	}
	var files []string
	filepath.Walk(libRoot, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && (strings.HasSuffix(p, ".sysml") || strings.HasSuffix(p, ".kerml")) {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)

	w := &world{mineByKey: map[string]*cn{}, oracleByKey: map[string]*cn{}, oracleByID: map[string]*cn{},
		memo: map[string]*cn{}, done: map[string]bool{}, busy: map[string]bool{}, gaps: map[string]int{}, effNames: map[*cn][2]string{}, libCache: map[string]*cn{}, implMemo: map[*cn][]ig{}, implBusy: map[*cn]bool{}, chains: map[[2]*cn]*cn{}, impliedByKey: map[string]*cn{}}
	oracleRoots := map[string]*cn{}
	impliedRoots := map[string]*cn{}
	impliedByID := map[string]*cn{}
	skipped := 0
	for _, f := range files {
		src, _ := os.ReadFile(f)
		g := grammars[filepath.Ext(f)]
		toks, err := g.lex(string(src))
		if err != nil {
			skipped++
			continue
		}
		ps := &Parser{g: g, toks: toks, memo: map[memoKey][]res{}, vmemo: map[memoKey][]vres{}, busy: map[memoKey]bool{}, expected: map[string]bool{}, anomalies: map[string]int{}}
		n, ok := ps.parse()
		if !ok {
			skipped++
			continue
		}
		base := fileBase(f)
		mine := fromNode(transform(postProcess(n), ctx{}))
		prune(mine)
		assignKeys(mine, base, "", nil, w.mineByKey)
		w.roots = append(w.roots, mine)
		want, err := loadXMI(xmiRoot + "/" + strings.TrimPrefix(f, libRoot+"/") + "x")
		if err != nil {
			continue
		}
		prune(want)
		assignKeys(want, base, "", nil, w.oracleByKey)
		indexIDs(want, base, w.oracleByID)
		oracleRoots[base] = want
		if impliedRoot := os.Getenv("IMPLIED_ROOT"); impliedRoot != "" {
			if imp, err := loadXMI(impliedRoot + "/" + strings.TrimPrefix(f, libRoot+"/") + "x"); err == nil {
				prune(imp)
				impliedRoots[base] = imp
			}
		}
	}
	fmt.Printf("world: %d files loaded (%d skipped)\n", len(w.roots), skipped)
	w.applyResolutionRules()
	w.structure(oracleRoots)
	w.report()
	if len(impliedRoots) > 0 {
		for file, root := range impliedRoots {
			w.assignPlainKeys(root, file, "", nil, w.impliedByKey)
			indexIDs(root, file, impliedByID)
		}
		w.impliedReport(impliedRoots, impliedByID)
	}
}

// structure re-runs the structural comparison after the resolution rules.
func (w *world) structure(oracleRoots map[string]*cn) {
	kinds := map[string]int{}
	exact, files, total := 0, 0, 0
	for _, root := range w.roots {
		want := oracleRoots[root.file]
		if want == nil {
			continue
		}
		files++
		var ds []diff
		diffCN("", root, want, &ds)
		if len(ds) == 0 {
			exact++
		}
		for _, d := range ds {
			cls := d.path[strings.LastIndex(d.path, "/")+1:]
			if i := strings.Index(cls, "."); i >= 0 {
				cls = cls[:i]
			}
			kinds[d.kind+" @ "+cls]++
			total++
		}
	}
	fmt.Printf("\nstructure after resolution rules: %d of %d files match the oracle exactly, %d differences\n", exact, files, total)
	var ks []string
	for k := range kinds {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		return kinds[ks[i]] > kinds[ks[j]] || (kinds[ks[i]] == kinds[ks[j]] && ks[i] < ks[j])
	})
	for i, k := range ks {
		if i >= 12 {
			break
		}
		fmt.Printf("  %5d  %s\n", kinds[k], k)
	}
}

type tally struct{ total, match, wrong, unresolved, noOracle int }

func (w *world) report() {
	byKind := map[string]*tally{}
	var wrong, unres []string
	for _, root := range w.roots {
		var visit func(c *cn)
		visit = func(c *cn) {
			var oracleKids []ckid
			if o := w.oracleByKey[c.key]; o != nil {
				oracleKids = o.kids
			}
			seenByRole := map[string]int{}
			for ki, k := range c.kids {
				if k.n != nil {
					visit(k.n)
					continue
				}
				if k.text == "" && k.owned < 0 && !k.owner {
					continue
				}
				kind := c.class + "." + k.role
				if k.text == "" {
					kind += " (implied)"
				}
				t := byKind[kind]
				if t == nil {
					t = &tally{}
					byKind[kind] = t
				}
				t.total++
				// the oracle's matching reference: same role, same ordinal
				ord := seenByRole[k.role]
				seenByRole[k.role]++
				var theirs *ckid
				n := 0
				for oi := range oracleKids {
					if oracleKids[oi].n == nil && oracleKids[oi].role == k.role {
						if n == ord {
							theirs = &oracleKids[oi]
							break
						}
						n++
					}
				}
				if theirs == nil {
					t.noOracle++
					continue
				}
				want := w.oracleByID[theirs.tgtFile+"|"+theirs.tgtID]
				got := w.resolveKid(c, ki)
				switch {
				case got == nil:
					t.unresolved++
					if len(unres) < 400 {
						unres = append(unres, fmt.Sprintf("%-26s %q in %s  (oracle: %s)", kind, k.text, describe(c.parent), orNone(want)))
					}
				case want != nil && got.key == want.key:
					t.match++
				default:
					t.wrong++
					if len(wrong) < 400 {
						wrong = append(wrong, fmt.Sprintf("%-26s %q in %s\n      ours:   %s\n      oracle: %s", kind, k.text, describe(c.parent), describe(got), orNone(want)))
					}
				}
			}
		}
		visit(root)
	}
	var kinds []string
	var all tally
	for k, t := range byKind {
		kinds = append(kinds, k)
		all.total += t.total
		all.match += t.match
		all.wrong += t.wrong
		all.unresolved += t.unresolved
		all.noOracle += t.noOracle
	}
	sort.Slice(kinds, func(i, j int) bool { return byKind[kinds[i]].total > byKind[kinds[j]].total })
	fmt.Printf("\nreferences: %d total, %d match the oracle, %d resolve elsewhere, %d unresolved, %d without an oracle counterpart\n", all.total, all.match, all.wrong, all.unresolved, all.noOracle)
	fmt.Printf("%-44s %6s %6s %6s %6s\n", "kind", "total", "match", "wrong", "unres")
	for i, k := range kinds {
		if i >= 40 {
			break
		}
		t := byKind[k]
		fmt.Printf("%-44s %6d %6d %6d %6d\n", k, t.total, t.match, t.wrong, t.unresolved)
	}
	if len(w.gaps) > 0 {
		fmt.Println("gaps:", w.gaps)
	}
	n := 12
	if v := os.Getenv("SHOW"); v != "" {
		fmt.Sscan(v, &n)
	}
	fmt.Println("\n-- resolved to a different element than the oracle --")
	for i, s := range wrong {
		if i >= n {
			break
		}
		fmt.Println("  " + s)
	}
	fmt.Println("\n-- unresolved --")
	for i, s := range unres {
		if i >= n {
			break
		}
		fmt.Println("  " + s)
	}
}

func orNone(c *cn) string {
	if c == nil {
		return "(none)"
	}
	return describe(c)
}
