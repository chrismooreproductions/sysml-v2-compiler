package main

// Implicit general types: the specializations the pilot's adapters add to
// a type that the source does not write (TypeAdapter.addDefaultGeneralType and
// its overrides, then removeUnnecessaryImplicitGeneralTypes). Each is one of
// the specification's check<Class>Specialization constraints.

type ig struct {
	kind   string // the relationship class: Subclassification, Subsetting, ...
	target *cn
}

var operatorPackages = []string{"BaseFunctions", "DataFunctions", "ControlFunctions"}

func (w *world) baseKind(t *cn) string {
	switch {
	case w.is(t, "Feature"):
		return "Subsetting"
	case w.is(t, "Classifier"):
		return "Subclassification"
	}
	return "Specialization"
}

// implicit returns a type's implicit general types, memoized.
func (w *world) implicit(t *cn) []ig {
	if v, ok := w.implMemo[t]; ok {
		return v
	}
	if w.implBusy[t] {
		w.cut++
		return nil
	}
	w.implBusy[t] = true
	cutBefore := w.cut
	var out []ig
	add := func(kind string, g *cn) {
		if g == nil || g == t {
			return
		}
		for _, x := range out {
			if x.kind == kind && x.target == g {
				return
			}
		}
		out = append(out, ig{kind, g})
	}
	if w.is(t, "Type") && len(w.rels(t, "Conjugation")) == 0 {
		// OperatorExpressionAdapter.computeImplicitGeneralTypes
		if w.is(t, "OperatorExpression") {
			if op := w.operatorOf(t); op != "" {
				for _, pkg := range operatorPackages {
					if g := w.libraryElement(pkg + "::'" + op + "'"); g != nil {
						add("FeatureTyping", g)
						break
					}
				}
			}
		}
		// InvocationExpressionAdapter.addDefaultGeneralType: the instantiated type
		if w.is(t, "InvocationExpression") && !w.is(t, "OperatorExpression") {
			if inst := w.instantiatedType(t); inst != nil {
				if w.is(inst, "Feature") {
					add("Subsetting", inst)
				} else {
					add("FeatureTyping", inst)
				}
			}
		}
		// TypeAdapter.addDefaultGeneralType: the metaclass's default supertype
		if qn := implicitKind(t.class, w.defaultKind(t, out)); qn != "" {
			add(w.baseKind(t), w.libraryElement(qn))
		}
		// ExpressionAdapter.addDefaultGeneralType: where the expression sits
		if w.is(t, "Expression") {
			if w.isStructureOwnedComposite(t) {
				add("Subsetting", w.libraryElement(implicitKind(t.class, "ownedPerformance")))
			}
			if w.isBehaviorOwnedComposite(t) {
				add("Subsetting", w.libraryElement(implicitKind(t.class, "subperformance")))
			}
			if w.isBehaviorOwned(t) {
				add("Subsetting", w.libraryElement(implicitKind(t.class, "enclosedPerformance")))
			}
		}
		// FeatureAdapter.addRedefinitions
		if w.is(t, "Feature") {
			w.addRedefinitions(t, add)
		}
		out = w.removeUnnecessary(t, out)
	}
	delete(w.implBusy, t)
	if w.cut == cutBefore {
		w.implMemo[t] = out
	}
	return out
}

// removeUnnecessary mirrors TypeAdapter.removeUnnecessaryImplicitGeneralTypes.
func (w *world) removeUnnecessary(t *cn, in []ig) []ig {
	var generals, redefined []*cn
	for _, r := range w.rels(t, "Specialization") {
		for i, k := range r.kids {
			if k.n == nil && theMeta.isTargetEnd(r.class, k.role) {
				if g := w.resolveKid(r, i); g != nil && g != t {
					generals = append(generals, g)
					if w.is(r, "Redefinition") {
						redefined = append(redefined, g)
					}
				}
			}
		}
	}
	var out []ig
	for _, x := range in {
		drop := false
		if x.kind == "Redefinition" {
			for _, r := range redefined {
				if r == x.target {
					drop = true
				}
			}
		} else {
			for _, g := range generals {
				if w.specializesExcluding(g, x.target, t) {
					drop = true
				}
			}
			for _, y := range in {
				if y.target != x.target && w.specializesExcluding(y.target, x.target, t) {
					drop = true
				}
			}
		}
		if !drop {
			out = append(out, x)
		}
	}
	return out
}

// specializesExcluding is specializes with one type already visited, so a
// search never passes back through the type being computed.
func (w *world) specializesExcluding(sub, super, exclude *cn) bool {
	seen := map[*cn]bool{exclude: true}
	var walk func(x *cn) bool
	walk = func(x *cn) bool {
		if x == super {
			return true
		}
		if x == nil || seen[x] {
			return false
		}
		seen[x] = true
		for _, g := range w.generalTypes(x) {
			if walk(g) {
				return true
			}
		}
		return false
	}
	return walk(sub)
}

// ---- features of a type ----

func (w *world) featureOwningType(f *cn) *cn {
	if w.is(f.parent, "FeatureMembership") {
		return f.parent.parent
	}
	return nil
}

func (w *world) ownedFeatures(t *cn) []*cn {
	var out []*cn
	for _, m := range w.rels(t, "FeatureMembership") {
		if f := first1(w.ownedRelated(m)); f != nil && w.is(f, "Feature") {
			out = append(out, f)
		}
	}
	return out
}

// features of a type: owned, then inherited.
func (w *world) features(t *cn) []*cn {
	out := w.ownedFeatures(t)
	for _, m := range w.inherited(t, map[*cn]bool{t: true}) {
		if w.is(m, "FeatureMembership") {
			if f := w.memberElement(m); f != nil && w.is(f, "Feature") {
				out = append(out, f)
			}
		}
	}
	return out
}

func (w *world) isParameter(f *cn) bool {
	t := w.featureOwningType(f)
	return t != nil && (w.is(t, "Behavior") || w.is(t, "Step")) && f.attrs["direction"] != ""
}

func (w *world) isResultParameter(f *cn) bool { return w.is(f.parent, "ReturnParameterMembership") }

func (w *world) ownedParameters(t *cn) []*cn {
	if w.is(t, "Feature") {
		t = w.basicFeature(t)
	}
	if t == nil {
		return nil
	}
	var out []*cn
	for _, f := range w.ownedFeatures(t) {
		if w.isParameter(f) {
			out = append(out, f)
		}
	}
	return out
}

func (w *world) resultParameterOf(t *cn, seen map[*cn]bool) *cn {
	seen[t] = true
	for _, p := range w.ownedParameters(t) {
		if w.isResultParameter(p) {
			return p
		}
	}
	for _, g := range w.generalTypes(t) {
		if g != nil && !seen[g] {
			if r := w.resultParameterOf(g, seen); r != nil {
				return r
			}
		}
	}
	return nil
}

// relevantFeatures mirrors FeatureAdapter.getRelevantFeatures: the features of
// a type that a feature's position among its owner's is matched against.
func (w *world) relevantFeatures(f, t *cn) []*cn {
	if t == nil {
		return nil
	}
	owning := w.featureOwningType(f)
	switch {
	case f.attrs["isEnd"] == "true":
		var out []*cn
		src := w.features(t)
		if owning == t {
			src = w.ownedFeatures(t)
		}
		for _, x := range src {
			if x.attrs["isEnd"] == "true" {
				out = append(out, x)
			}
		}
		return out
	case w.isParameter(f):
		if w.isResultParameter(f) {
			if r := w.resultParameterOf(t, map[*cn]bool{}); r != nil {
				return []*cn{r}
			}
			return nil
		}
		var params []*cn
		if owning == t {
			params = w.ownedParameters(t)
		} else {
			for _, x := range w.features(t) {
				if x.attrs["direction"] != "" {
					params = append(params, x)
				}
			}
		}
		var out []*cn
		for _, p := range params {
			if !w.isResultParameter(p) {
				out = append(out, p)
			}
		}
		return out
	}
	return nil
}

// addRedefinitions mirrors FeatureAdapter.addRedefinitions.
func (w *world) addRedefinitions(f *cn, add func(string, *cn)) {
	t := w.featureOwningType(f)
	if t == nil {
		return
	}
	rel := w.relevantFeatures(f, t)
	i := -1
	for j, x := range rel {
		if x == f {
			i = j
		}
	}
	if i < 0 {
		return
	}
	seen := map[*cn]bool{}
	for _, g := range w.generalTypes(t) {
		if seen[g] {
			continue
		}
		seen[g] = true
		feats := w.relevantFeatures(f, g)
		if i < len(feats) && feats[i] != nil && feats[i] != f {
			add("Redefinition", feats[i])
		}
	}
}

// operatorOf is an operator expression's operator: written, or the default
// its metaclass declares.
func (w *world) operatorOf(t *cn) string {
	if op := t.attrs["operator"]; op != "" {
		return op
	}
	switch {
	case w.is(t, "FeatureChainExpression"):
		return "."
	case w.is(t, "IndexExpression"):
		return "#"
	case w.is(t, "SelectExpression"):
		return "select"
	case w.is(t, "CollectExpression"):
		return "collect"
	}
	return ""
}

// instantiatedType mirrors InstantiationExpression.instantiatedType: the
// member element of the first owned membership that is not a FeatureMembership.
func (w *world) instantiatedType(e *cn) *cn {
	for _, m := range w.rels(e, "Membership") {
		if w.is(m, "FeatureMembership") {
			continue
		}
		if t := w.memberElement(m); t != nil && w.is(t, "Type") {
			return t
		}
		return nil
	}
	return nil
}

func (w *world) hasType(f *cn, class string, implicit []ig) bool {
	for _, r := range w.rels(f, "FeatureTyping") {
		for i, k := range r.kids {
			if k.n == nil && k.role == "type" {
				if g := w.resolveKid(r, i); g != nil && w.is(g, class) {
					return true
				}
			}
		}
	}
	for _, x := range implicit {
		if x.kind == "FeatureTyping" && w.is(x.target, class) {
			return true
		}
	}
	return false
}

func (w *world) isBehaviorOwned(f *cn) bool {
	t := w.featureOwningType(f)
	return t != nil && (w.is(t, "Behavior") || w.is(t, "Step"))
}

func (w *world) isBehaviorOwnedComposite(f *cn) bool {
	return w.isBehaviorOwned(f) && f.attrs["isComposite"] == "true"
}

func (w *world) isStructureOwnedComposite(f *cn) bool {
	t := w.featureOwningType(f)
	return f.attrs["isComposite"] == "true" && t != nil &&
		(w.is(t, "Structure") || w.is(t, "Feature") && w.hasType(t, "Structure", w.implicit(t)))
}

// defaultKind chooses which entry of ImplicitGeneralizationMap is a type's
// default supertype: the getDefaultSupertype of the adapter for its metaclass.
func (w *world) defaultKind(t *cn, soFar []ig) string {
	switch {
	case w.is(t, "Multiplicity"):
		owner := t.parent
		if owner != nil {
			owner = owner.parent
		}
		switch {
		case w.is(owner, "Classifier"):
			return "classifier"
		case w.is(owner, "Feature"):
			return "feature"
		}
		return "base"
	case w.is(t, "Expression"):
		return "base" // ExpressionAdapter: the context kinds are added separately
	case w.is(t, "Step"):
		switch {
		case w.isStructureOwnedComposite(t):
			return "ownedPerformance"
		case w.isBehaviorOwnedComposite(t):
			return "subperformance"
		case w.isBehaviorOwned(t):
			return "enclosedPerformance"
		}
		return "base"
	case w.is(t, "Feature"):
		owner := w.featureOwningType(t)
		classOwned := w.is(owner, "Class") || w.is(owner, "Feature") && w.hasType(owner, "Class", w.implicit(owner))
		switch {
		case w.hasType(t, "Structure", soFar):
			if t.attrs["isComposite"] == "true" && (w.is(owner, "Structure") || w.is(owner, "Feature") && w.hasType(owner, "Structure", w.implicit(owner))) {
				return "subobject"
			}
			return "object"
		case w.hasType(t, "Class", soFar):
			switch {
			case t.attrs["isComposite"] == "true" && classOwned:
				return "suboccurrence"
			case t.attrs["isPortion"] == "true" && classOwned:
				return "portion"
			}
			return "occurrence"
		case w.hasType(t, "DataType", soFar):
			return "dataValue"
		}
	}
	return "base"
}
