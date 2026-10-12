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
	roots       []*cn             // our file roots, in file order
	mineByKey   map[string]*cn    // our elements by position
	oracleByKey map[string]*cn    // the oracle's elements by position
	oracleByID  map[string]*cn    // file|id -> oracle element
	memo        map[string]*cn    // resolved references
	done        map[string]bool   // memo has an entry (including a nil result)
	busy        map[string]bool   // resolutions in progress, to cut cycles
	owned       map[*cn][]*cn     // cache: owned relationships by role
	gaps        map[string]int    // things met that this resolver does not handle
	effNames    map[*cn][2]string // cache: effective short name, name
	cut         int               // incremented whenever a circular resolution is cut
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

// parentNamespace mirrors NamespaceUtil.getParentNamespaceOf: for an element
// held by a non-membership relationship, the namespace of that relationship's
// owner.
func (w *world) parentNamespace(e *cn) *cn {
	for e != nil {
		if ns := w.owningNamespace(e); ns != nil {
			return ns
		}
		if e.parent == nil || e.parent.parent == nil {
			return nil
		}
		e = e.parent.parent
	}
	return nil
}

func (w *world) nonExpressionNamespace(ns *cn) *cn {
	for ns != nil && w.is(ns, "Expression") {
		ns = w.parentNamespace(ns)
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

// generals are the types a type specializes or conjugates, as resolved.
func (w *world) generals(t *cn) []*cn {
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
		if owner.class == "Membership" && w.is(owner.parent, "FeatureChainExpression") {
			// the target of a chain: look among the members of the source
			return scope{ns: w.chainSource(owner.parent), inside: false}
		}
		return scope{ns: w.nonExpressionNamespace(owner.parent), inside: true}
	case w.is(owner, "Relationship"):
		if owner.parent == nil {
			return scope{}
		}
		ns := w.parentNamespace(owner.parent)
		if w.is(owner, "FeatureTyping") {
			ns = w.nonExpressionNamespace(ns)
		}
		return scope{ns: ns, inside: true, redefinition: w.is(owner, "Redefinition") && role == "redefinedFeature"}
	}
	return scope{ns: w.parentNamespace(owner), inside: true}
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
		memo: map[string]*cn{}, done: map[string]bool{}, busy: map[string]bool{}, gaps: map[string]int{}, effNames: map[*cn][2]string{}}
	oracleRoots := map[string]*cn{}
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
	}
	fmt.Printf("world: %d files loaded (%d skipped)\n", len(w.roots), skipped)
	w.report()
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
