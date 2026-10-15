// SPDX-License-Identifier: EPL-2.0

package semantics

// The files of the library that reflect the metamodel: SysML::PartUsage is a
// metadata definition in the first, KerML::Feature a metaclass in the second.
// An element is an instance of the metaclass named like it.
var reflectiveFiles = map[string]bool{
	"library:Systems Library/SysML.sysml":                          true,
	"library:Kernel Libraries/Kernel Semantic Library/KerML.kerml": true,
}

// metaclassTest evaluates "@T" for the element the evaluation is for: true if
// the element is an instance of the metaclass T (@SysML::PartUsage), or carries
// metadata whose type is T or specializes it (@Safety). The first operand must
// be the implicit one a filter writes; "x @ T" for another x is not supported.
func (ev *evaluator) metaclassTest(e *el) ([]Value, error) {
	w := ev.w
	var params []*el
	for _, m := range w.rels(e, "ParameterMembership") {
		if !m.IsA("ReturnParameterMembership") {
			params = append(params, first(w.ownedRelated(m)))
		}
	}
	if len(params) != 2 || params[0] == nil || params[1] == nil {
		return nil, ev.fail(e, "a type test needs an operand and a type")
	}
	var operand *el
	for _, fv := range w.rels(params[0], "FeatureValue") {
		operand = first(w.ownedRelated(fv))
	}
	if operand == nil || !ev.isSelf(operand) {
		return nil, ev.fail(e, "@ is only supported on the element a filter is evaluated for")
	}
	var typ *el
	for _, ft := range w.rels(params[1], "FeatureTyping") {
		typ = w.typingTarget(ft)
	}
	if typ == nil {
		return nil, ev.fail(e, "the type of @ does not resolve")
	}
	if ev.self == nil {
		return nil, ev.fail(e, "@%s needs an element to test", displayName(typ))
	}
	return []Value{w.hasMetadataType(ev.self, typ)}, nil
}

// isSelf is true for the operand a filter's "@T" leaves implicit: a feature
// reference that names nothing. (Written out, "self" does not resolve in a
// package's filter, so only the implicit form is supported.)
func (ev *evaluator) isSelf(x *el) bool {
	if !x.IsA("FeatureReferenceExpression") {
		return false
	}
	for _, m := range ev.w.rels(x, "Membership") {
		if !m.IsA("ParameterMembership") {
			return false
		}
	}
	return true
}

// hasMetadataType is whether an element is an instance of the metaclass t, or
// has metadata annotation whose type is, or specializes, t.
func (w *Workspace) hasMetadataType(self, t *el) bool {
	if reflectiveFiles[w.FileOf(t)] {
		return self.IsA(t.Text("declaredName"))
	}
	for _, m := range w.metadataOf(self) {
		for _, ty := range w.typesOf(m) {
			if ty == t || w.specializes(ty, t) {
				return true
			}
		}
	}
	return false
}

// metadataOf are the metadata features that annotate an element: those it owns
// that name no other element, and those in the files added to the workspace
// that name it with "about".
func (w *Workspace) metadataOf(e *el) []*el {
	var out []*el
	for _, m := range w.rels(e, "Membership") {
		for _, x := range w.ownedRelated(m) {
			if x.IsA("MetadataFeature") && !x.Generated && len(w.annotatedBy(x)) == 0 {
				out = append(out, x)
			}
		}
	}
	if w.metaIndex == nil {
		w.metaIndex = map[*el][]*el{}
		for _, root := range w.roots {
			if w.isLibrary(root) {
				continue
			}
			root.Walk(func(x *el) {
				if x.IsA("MetadataFeature") {
					for _, t := range w.annotatedBy(x) {
						w.metaIndex[t] = append(w.metaIndex[t], x)
					}
				}
			})
		}
	}
	return append(out, w.metaIndex[e]...)
}

// annotatedBy are the elements a metadata feature names with "about".
func (w *Workspace) annotatedBy(m *el) []*el {
	var out []*el
	for _, r := range w.rels(m, "Annotation") {
		for _, ref := range r.Refs("annotatedElement") {
			if ref.Target != nil {
				out = append(out, ref.Target)
			}
		}
	}
	return out
}

// metadataField is the value of a reference to a feature of a metadata
// definition (Safety::level) for the element being evaluated for: what the
// element's metadata gives that feature. ok is false when the reference is to
// anything else, or no element is being evaluated for.
func (ev *evaluator) metadataField(f, at *el) (vs []Value, ok bool, err error) {
	w := ev.w
	if ev.self == nil || !f.Owner.IsA("FeatureMembership") || !f.Owner.Owner.IsA("MetadataDefinition") {
		return nil, false, nil
	}
	def := f.Owner.Owner
	var carriers []*el
	for _, m := range w.metadataOf(ev.self) {
		for _, ty := range w.typesOf(m) {
			if ty == def || w.specializes(ty, def) {
				carriers = append(carriers, m)
				break
			}
		}
	}
	if len(carriers) == 0 {
		return nil, true, &EvalError{Element: at, Msg: displayName(ev.self) + " has no " + displayName(def) + " metadata", Unbound: true}
	}
	for _, m := range carriers {
		for _, mem := range w.rels(m, "FeatureMembership") {
			for _, g := range w.ownedRelated(mem) {
				if g == nil || !g.IsA("Feature") {
					continue
				}
				if !redefines(w, g, f) {
					continue
				}
				for _, fv := range w.rels(g, "FeatureValue") {
					if x := first(w.ownedRelated(fv)); x != nil {
						vs, err := ev.eval(x)
						return vs, true, err
					}
				}
			}
		}
	}
	return nil, true, &EvalError{Element: at, Msg: displayName(ev.self) + " gives " + displayName(f) + " no value", Unbound: true}
}

// redefines is whether g redefines f, directly or through others.
func redefines(w *Workspace, g, f *el) bool {
	for _, r := range w.allRedefined(g, map[*el]bool{g: true}) {
		if r == f {
			return true
		}
	}
	return false
}
