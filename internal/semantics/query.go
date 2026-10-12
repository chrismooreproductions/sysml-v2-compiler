// SPDX-License-Identifier: EPL-2.0

package semantics

// The queries below answer questions about a resolved model: what a name is,
// what a namespace contains, what a type specializes. They follow the derived
// properties of the specification (Element.effectiveName, Namespace.member,
// Type.feature, Feature.type) as the pilot implementation computes them.

// Lookup finds an element by its qualified name from the global scope, for
// example "ScalarValues::Real" or "Fleet::'Heavy Truck'"; nil if there is none.
func (w *Workspace) Lookup(qualifiedName string) *el {
	segs := splitName(qualifiedName)
	if len(segs) == 0 || segs[0] == "" {
		return nil
	}
	gs := w.global(segs[0])
	if len(gs) == 0 {
		return nil
	}
	m := gs[0]
	for _, seg := range segs[1:] {
		ns := w.memberElement(m)
		if !ns.IsA("Namespace") {
			return nil
		}
		if m = w.resolveVisible(ns, seg); m == nil {
			return nil
		}
	}
	return w.memberElement(m)
}

// Member finds the member of a namespace with a name, among those visible from
// outside it (owned and public, inherited and imported); nil if there is none.
func (w *Workspace) Member(ns *el, name string) *el {
	if !ns.IsA("Namespace") {
		return nil
	}
	if m := w.resolveVisible(ns, name); m != nil {
		return w.memberElement(m)
	}
	return nil
}

// EffectiveName is Element.effectiveName: the declared name, or for a feature
// without one the name of the feature it redefines.
func (w *Workspace) EffectiveName(e *el) string {
	_, name := w.effectiveNames(e)
	return name
}

// OwnedMembers are the elements a namespace owns, in order.
func (w *Workspace) OwnedMembers(ns *el) []*el {
	var out []*el
	for _, m := range w.ownedMemberships(ns) {
		if m.IsA("OwningMembership") {
			out = append(out, w.ownedRelated(m)...)
		}
	}
	return out
}

// Members are the elements visible from outside a namespace: those it owns
// and those it inherits or imports, without any another member redefines.
func (w *Workspace) Members(ns *el) []*el {
	if !ns.IsA("Namespace") {
		return nil
	}
	var out []*el
	seen := map[*el]bool{}
	for _, m := range w.removeRedefined(w.membershipsOf(ns, false, map[*el]bool{ns: true}, false)) {
		if e := w.memberElement(m); e != nil && !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return out
}

// Supertypes are the types a type specializes directly, written or implied.
func (w *Workspace) Supertypes(t *el) []*el {
	var out []*el
	seen := map[*el]bool{}
	for _, g := range w.generalTypes(t) {
		if g != nil && !seen[g] {
			seen[g] = true
			out = append(out, g)
		}
	}
	return out
}

// Specializes reports whether a type is, or specializes, another.
func (w *Workspace) Specializes(t, general *el) bool { return w.specializes(t, general) }

// Features are the features of a type: the ones it owns, then the ones it
// inherits, less any that another of them redefines.
func (w *Workspace) Features(t *el) []*el {
	if !t.IsA("Type") {
		return nil
	}
	all := w.features(t)
	redefined := map[*el]bool{}
	for _, f := range all {
		for r := range w.allRedefinedWithComputed(f) {
			redefined[r] = true
		}
	}
	var out []*el
	seen := map[*el]bool{}
	for _, f := range all {
		if !redefined[f] && !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

// TypesOf are the types of a feature (Feature.type): its own and implied
// types and those of the features it subsets or redefines.
func (w *Workspace) TypesOf(f *el) []*el {
	if !f.IsA("Feature") {
		return nil
	}
	return w.typesOf(f)
}

// allRedefinedWithComputed are the features f redefines, written or implied,
// directly or through others.
func (w *Workspace) allRedefinedWithComputed(f *el) map[*el]bool {
	out := map[*el]bool{}
	var walk func(x *el)
	walk = func(x *el) {
		for _, r := range w.redefinedWithComputed(x) {
			if !out[r] {
				out[r] = true
				walk(r)
			}
		}
	}
	walk(f)
	return out
}
