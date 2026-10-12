// SPDX-License-Identifier: EPL-2.0
//
// Portions derived from the SysML v2 Pilot Implementation
// (org.omg.sysml.logic adapters, TypeAdapter, FeatureAdapter and the expression
// adapters, and util ImplicitGeneralizationMap), Copyright (c) 2018-2025 Model
// Driven Solutions, Inc. and others, EPL-2.0. See NOTICE.

package semantics

import (
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/schema"
)

// Implicit general types are the specializations the pilot's adapters add to a
// type that the source does not write (TypeAdapter.addDefaultGeneralType and
// its overrides, then removeUnnecessaryImplicitGeneralTypes). Each is one of
// the specification's check<Class>Specialization constraints.

// implied is an implicit specialization: the kind of relationship, and its
// general type.
type implied struct {
	kind   string // Subclassification, Subsetting, Redefinition, FeatureTyping, ...
	target *el
}

var operatorPackages = []string{"BaseFunctions", "DataFunctions", "ControlFunctions"}

// implicitKind names the library type a metaclass implicitly specializes for a
// kind (ImplicitGeneralizationMap), searching up the metaclass hierarchy as the
// Java does up the implementation class hierarchy.
func implicitKind(class, kind string) string {
	for _, c := range schema.Supertypes(class) {
		if v, ok := implicitTable[c+"_"+kind]; ok {
			return v
		}
	}
	return ""
}

func baseKind(t *el) string {
	switch {
	case t.IsA("Feature"):
		return "Subsetting"
	case t.IsA("Classifier"):
		return "Subclassification"
	}
	return "Specialization"
}

// generalTypes follows TypeUtil.getGeneralTypesOf: the types a type specializes
// or conjugates, as resolved, then its implicit general types, then the last
// feature of a chain.
func (w *Workspace) generalTypes(t *el) []*el {
	var out []*el
	for _, r := range w.rels(t, "Specialization") {
		for _, p := range r.Props {
			if ref, ok := p.Value.(*model.Ref); ok && schema.IsTargetEnd(r.Class, p.Name) {
				if g := w.target(r, p.Name, ref); g != nil {
					out = append(out, g)
				}
			}
		}
	}
	for _, r := range w.rels(t, "Conjugation") {
		if g := w.refTarget(r, "originalType"); g != nil {
			out = append(out, g)
		}
	}
	for _, x := range w.implicit(t) {
		out = append(out, x.target)
	}
	if t.IsA("Feature") {
		if len(w.rels(t, "FeatureChaining")) > 0 || len(w.chainOf[t]) > 0 {
			if f := w.basicFeature(t); f != nil {
				out = append(out, f)
			}
		}
	}
	return out
}

// implicit returns a type's implicit general types, memoized.
func (w *Workspace) implicit(t *el) []implied {
	if v, ok := w.implMemo[t]; ok {
		return v
	}
	if w.implBusy[t] {
		w.cut++
		return nil
	}
	w.implBusy[t] = true
	cutBefore := w.cut
	var out []implied
	add := func(kind string, g *el) {
		if g == nil || g == t {
			return
		}
		for _, x := range out {
			if x.kind == kind && x.target == g {
				return
			}
		}
		out = append(out, implied{kind, g})
	}
	switch {
	case w.chainSourceTarget(t) != nil:
		// FeatureChainExpressionAdapter.addTargetRedefinition: the source target
		// redefines the library target and the chain's own target, and gets no
		// other implicit general type.
		src := w.chainSourceTarget(t)
		add("Redefinition", w.libraryElement(implicitKind("FeatureChainExpression", "target")))
		add("Redefinition", w.chainTarget(src))
	case t.IsA("Type") && len(w.rels(t, "Conjugation")) == 0:
		w.computeImplicit(t, add, func() []implied { return out })
		out = w.removeUnnecessary(t, out)
	}
	delete(w.implBusy, t)
	if w.cut == cutBefore {
		w.implMemo[t] = out
	}
	return out
}

// computeImplicit adds the implicit general types of a type, in the order the
// adapters do.
func (w *Workspace) computeImplicit(t *el, add func(string, *el), soFar func() []implied) {
	// OperatorExpressionAdapter.computeImplicitGeneralTypes
	if t.IsA("OperatorExpression") {
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
	if t.IsA("InvocationExpression") && !t.IsA("OperatorExpression") {
		if inst := w.instantiatedType(t); inst != nil {
			if inst.IsA("Feature") {
				add("Subsetting", inst)
			} else {
				add("FeatureTyping", inst)
			}
		}
	}
	// TypeAdapter.addDefaultGeneralType: the metaclass's default supertype
	emit := func(kind string) {
		if qn := implicitKind(t.Class, kind); qn != "" {
			add(baseKind(t), w.libraryElement(qn))
		}
	}
	if t.IsA("OccurrenceUsage") {
		w.occurrenceUsageKinds(t, emit, soFar)
	} else {
		emit(w.defaultKind(t, soFar()))
	}
	// ExpressionAdapter.addDefaultGeneralType: where the expression sits
	if t.IsA("Expression") && !t.IsA("Usage") {
		if w.isStructureOwnedComposite(t) {
			add("Subsetting", w.libraryElement(implicitKind(t.Class, "ownedPerformance")))
		}
		if w.isBehaviorOwnedComposite(t) {
			add("Subsetting", w.libraryElement(implicitKind(t.Class, "subperformance")))
		}
		if w.isBehaviorOwned(t) {
			add("Subsetting", w.libraryElement(implicitKind(t.Class, "enclosedPerformance")))
		}
	}
	if t.IsA("Feature") {
		w.addRedefinitions(t, add) // FeatureAdapter.addRedefinitions
		w.addBoundValue(t, add)
		w.addPushedByOwner(t, add)
	}
}

// removeUnnecessary follows TypeAdapter.removeUnnecessaryImplicitGeneralTypes:
// an implicit general type is dropped if an explicit or other implicit one
// already specializes it.
func (w *Workspace) removeUnnecessary(t *el, in []implied) []implied {
	var generals, redefined []*el
	for _, r := range w.rels(t, "Specialization") {
		for _, p := range r.Props {
			if ref, ok := p.Value.(*model.Ref); ok && schema.IsTargetEnd(r.Class, p.Name) {
				if g := w.target(r, p.Name, ref); g != nil && g != t {
					generals = append(generals, g)
					if r.IsA("Redefinition") {
						redefined = append(redefined, g)
					}
				}
			}
		}
	}
	var out []implied
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

// specializes follows TypeUtil.specializes: a type specializes itself and every
// type reachable through its explicit and implicit general types.
func (w *Workspace) specializes(t, general *el) bool {
	if t == nil || general == nil {
		return false
	}
	return w.specializesExcluding(t, general, nil)
}

// specializesExcluding is specializes with one type already visited, so a
// search never passes back through the type being computed.
func (w *Workspace) specializesExcluding(sub, super, exclude *el) bool {
	seen := map[*el]bool{}
	if exclude != nil {
		seen[exclude] = true
	}
	var walk func(x *el) bool
	walk = func(x *el) bool {
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

func (w *Workspace) featureOwningType(f *el) *el {
	if f.Owner.IsA("FeatureMembership") {
		return f.Owner.Owner
	}
	return nil
}

func (w *Workspace) ownedFeatures(t *el) []*el {
	var out []*el
	for _, m := range w.rels(t, "FeatureMembership") {
		if f := first(w.ownedRelated(m)); f.IsA("Feature") {
			out = append(out, f)
		}
	}
	return out
}

// ownedEndCount is TypeUtil.getOwnedEndFeaturesOf(t).size().
func (w *Workspace) ownedEndCount(t *el) int {
	n := 0
	for _, f := range w.ownedFeatures(t) {
		if f.IsTrue("isEnd") {
			n++
		}
	}
	return n
}

// features of a type: owned, then inherited.
func (w *Workspace) features(t *el) []*el {
	out := w.ownedFeatures(t)
	for _, m := range w.inherited(t, map[*el]bool{t: true}) {
		if m.IsA("FeatureMembership") {
			if f := w.memberElement(m); f.IsA("Feature") {
				out = append(out, f)
			}
		}
	}
	return out
}

func (w *Workspace) isParameter(f *el) bool {
	t := w.featureOwningType(f)
	return t != nil && (t.IsA("Behavior") || t.IsA("Step")) && f.Has("direction")
}

func (w *Workspace) isResultParameter(f *el) bool { return f.Owner.IsA("ReturnParameterMembership") }

func (w *Workspace) ownedParameters(t *el) []*el {
	if t.IsA("Feature") {
		t = w.basicFeature(t)
	}
	if t == nil {
		return nil
	}
	var out []*el
	for _, f := range w.ownedFeatures(t) {
		if w.isParameter(f) {
			out = append(out, f)
		}
	}
	return out
}

func (w *Workspace) resultParameterOf(t *el, seen map[*el]bool) *el {
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

// relevantFeatures follows FeatureAdapter.getRelevantFeatures: the features of
// a type that a feature's position among its owner's is matched against.
func (w *Workspace) relevantFeatures(f, t *el) []*el {
	if t == nil {
		return nil
	}
	owning := w.featureOwningType(f)
	switch {
	case f.IsTrue("isEnd"):
		var out []*el
		src := w.features(t)
		if owning == t {
			src = w.ownedFeatures(t)
		}
		for _, x := range src {
			if x.IsTrue("isEnd") {
				out = append(out, x)
			}
		}
		return out
	case w.isParameter(f):
		if w.isResultParameter(f) {
			if r := w.resultParameterOf(t, map[*el]bool{}); r != nil {
				return []*el{r}
			}
			return nil
		}
		var params []*el
		if owning == t {
			params = w.ownedParameters(t)
		} else {
			for _, x := range w.features(t) {
				if x.Has("direction") {
					params = append(params, x)
				}
			}
		}
		var out []*el
		for _, p := range params {
			if !w.isResultParameter(p) {
				out = append(out, p)
			}
		}
		return out
	}
	return nil
}

// addRedefinitions follows FeatureAdapter.addRedefinitions: a parameter or end
// feature redefines the feature at the same position in each general type of
// its owner.
func (w *Workspace) addRedefinitions(f *el, add func(string, *el)) {
	t := w.featureOwningType(f)
	if t == nil {
		return
	}
	i := -1
	for j, x := range w.relevantFeatures(f, t) {
		if x == f {
			i = j
		}
	}
	if i < 0 {
		return
	}
	seen := map[*el]bool{}
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

// basicFeature follows FeatureUtil.getBasicFeatureOf: a chain stands for its
// last chaining feature.
func (w *Workspace) basicFeature(f *el) *el {
	if f == nil {
		return nil
	}
	if c := w.chainOf[f]; len(c) > 0 {
		return c[len(c)-1]
	}
	if ch := w.rels(f, "FeatureChaining"); len(ch) > 0 {
		return w.refTarget(ch[len(ch)-1], "chainingFeature")
	}
	return f
}

// chainFeatures follows FeatureUtil.chainFeatures: a feature the pilot
// generates, chaining two features. It has no source position.
func (w *Workspace) chainFeatures(a, b *el) *el {
	k := [2]*el{a, b}
	if c, ok := w.chains[k]; ok {
		return c
	}
	c := &el{Class: "Feature", Generated: true}
	w.chains[k] = c
	w.chainOf[c] = []*el{a, b}
	return c
}

// resultOf is an expression's result parameter (Expression.result).
func (w *Workspace) resultOf(e *el) *el {
	if !e.IsA("Expression") {
		return nil
	}
	return w.resultParameterOf(e, map[*el]bool{})
}

// addBoundValue follows FeatureAdapter.addBoundValueSubsetting: a feature with a
// value subsets the chain of the value expression and its result, unless it
// already specializes something or has a direction.
func (w *Workspace) addBoundValue(f *el, add func(string, *el)) {
	if f.Has("direction") || len(w.rels(f, "Specialization")) > 0 {
		return
	}
	for _, fv := range w.rels(f, "FeatureValue") {
		if fv.IsTrue("isDefault") {
			return
		}
		if value := first(w.ownedRelated(fv)); value != nil {
			if res := w.resultOf(value); res != nil {
				add("Subsetting", w.chainFeatures(value, res))
			}
		}
		return
	}
}

func (w *Workspace) isFunctionType(t *el) bool {
	if t.IsA("Function") {
		return true
	}
	if t.IsA("Feature") {
		for _, r := range w.rels(t, "FeatureTyping") {
			if g := w.refTarget(r, "type"); g.IsA("Function") {
				return true
			}
		}
	}
	return false
}

// addPushedByOwner covers the rules an expression's adapter applies to its own
// result parameter: reference subsetting, invocation result typing and the
// feature-chain result.
func (w *Workspace) addPushedByOwner(f *el, add func(string, *el)) {
	if !w.isResultParameter(f) {
		return
	}
	e := w.featureOwningType(f)
	if e == nil || w.resultOf(e) != f {
		return
	}
	switch {
	case e.IsA("FeatureReferenceExpression"):
		for _, m := range w.rels(e, "Membership") {
			if m.IsA("ParameterMembership") {
				continue
			}
			if r := w.memberElement(m); r.IsA("Feature") {
				add("Subsetting", r)
			}
			break
		}
	case e.IsA("FeatureChainExpression"):
		var src *el
		for _, m := range w.rels(e, "ParameterMembership") {
			if !m.IsA("ReturnParameterMembership") {
				if p := first(w.ownedRelated(m)); p != nil && p.Text("direction") == "in" {
					src = p
					break
				}
			}
		}
		if src != nil {
			if fs := w.ownedFeatures(src); len(fs) > 0 {
				add("Subsetting", w.chainFeatures(src, fs[0]))
			}
		}
	case e.IsA("InvocationExpression"):
		if inst := w.instantiatedType(e); inst != nil && !w.isFunctionType(inst) {
			if inst.IsA("Feature") {
				add("Subsetting", inst)
			} else {
				add("FeatureTyping", inst)
			}
		}
	}
}

// chainSourceTarget returns the FeatureChainExpression for which f is the
// anonymous source-target feature (the first owned feature of the source
// parameter), or nil.
func (w *Workspace) chainSourceTarget(f *el) *el {
	if !f.IsA("Feature") || f.Text("declaredName") != "" || !f.Generated {
		return nil
	}
	param := w.featureOwningType(f)
	if param == nil || !param.IsA("Feature") || param.Text("direction") != "in" {
		return nil
	}
	e := w.featureOwningType(param)
	if !e.IsA("FeatureChainExpression") {
		return nil
	}
	if fs := w.ownedFeatures(param); len(fs) == 0 || fs[0] != f {
		return nil
	}
	return e
}

// operatorOf is an operator expression's operator: written, or the default its
// metaclass declares.
func (w *Workspace) operatorOf(t *el) string {
	if op := t.Text("operator"); op != "" {
		return op
	}
	switch {
	case t.IsA("FeatureChainExpression"):
		return "."
	case t.IsA("IndexExpression"):
		return "#"
	case t.IsA("SelectExpression"):
		return "select"
	case t.IsA("CollectExpression"):
		return "collect"
	}
	return ""
}

// instantiatedType follows InstantiationExpression.instantiatedType: the member
// element of the first owned membership that is not a FeatureMembership.
func (w *Workspace) instantiatedType(e *el) *el {
	for _, m := range w.rels(e, "Membership") {
		if m.IsA("FeatureMembership") {
			continue
		}
		if t := w.memberElement(m); t.IsA("Type") {
			return t
		}
		return nil
	}
	return nil
}

func (w *Workspace) hasType(f *el, class string, implicit []implied) bool {
	for _, r := range w.rels(f, "FeatureTyping") {
		if g := w.refTarget(r, "type"); g.IsA(class) {
			return true
		}
	}
	for _, x := range implicit {
		if x.kind == "FeatureTyping" && x.target.IsA(class) {
			return true
		}
	}
	return false
}

func (w *Workspace) isBehaviorOwned(f *el) bool {
	t := w.featureOwningType(f)
	return t.IsA("Behavior") || t.IsA("Step")
}

func (w *Workspace) isBehaviorOwnedComposite(f *el) bool {
	return w.isBehaviorOwned(f) && f.IsTrue("isComposite")
}

func (w *Workspace) isStructureOwnedComposite(f *el) bool {
	t := w.featureOwningType(f)
	return f.IsTrue("isComposite") && t != nil &&
		(t.IsA("Structure") || t.IsA("Feature") && w.hasType(t, "Structure", w.implicit(t)))
}

// defaultKind chooses which entry of ImplicitGeneralizationMap is a type's
// default supertype: the getDefaultSupertype of the adapter for its metaclass.
func (w *Workspace) defaultKind(t *el, soFar []implied) string {
	switch {
	case t.IsA("Association"), t.IsA("ConnectionDefinition"), t.IsA("FlowDefinition"):
		// AssociationAdapter, ConnectionDefinitionAdapter, FlowDefinitionAdapter
		if w.ownedEndCount(t) == 2 {
			return "binary"
		}
		return "base"
	case t.IsA("ConnectionUsage") && !t.IsA("FlowUsage"):
		if w.ownedEndCount(t) == 2 {
			return "binary"
		}
		return "base"
	case t.IsA("Connector") && !t.IsA("Flow") && !t.IsA("FlowUsage"):
		// ConnectorAdapter
		binary := w.ownedEndCount(t) == 2
		switch {
		case w.hasType(t, "Structure", soFar) && binary:
			return "binaryObject"
		case w.hasType(t, "Structure", soFar):
			return "object"
		case binary:
			return "binary"
		}
		return "base"
	case t.IsA("Multiplicity"):
		owner := t.Owner
		if owner != nil {
			owner = owner.Owner
		}
		switch {
		case owner.IsA("Classifier"):
			return "classifier"
		case owner.IsA("Feature"):
			return "feature"
		}
		return "base"
	case t.IsA("Expression"):
		return "base" // ExpressionAdapter: the context kinds are added separately
	case t.IsA("Step"):
		switch {
		case w.isStructureOwnedComposite(t):
			return "ownedPerformance"
		case w.isBehaviorOwnedComposite(t):
			return "subperformance"
		case w.isBehaviorOwned(t):
			return "enclosedPerformance"
		}
		return "base"
	case t.IsA("Feature"):
		owner := w.featureOwningType(t)
		classOwned := owner.IsA("Class") || owner.IsA("Feature") && w.hasType(owner, "Class", w.implicit(owner))
		switch {
		case w.hasType(t, "Structure", soFar):
			if t.IsTrue("isComposite") && (owner.IsA("Structure") || owner.IsA("Feature") && w.hasType(owner, "Structure", w.implicit(owner))) {
				return "subobject"
			}
			return "object"
		case w.hasType(t, "Class", soFar):
			switch {
			case t.IsTrue("isComposite") && classOwned:
				return "suboccurrence"
			case t.IsTrue("isPortion") && classOwned:
				return "portion"
			}
			return "occurrence"
		case w.hasType(t, "DataType", soFar):
			return "dataValue"
		}
	}
	return "base"
}

// Implied is a relationship the specification adds to a type that its source
// does not write: a Subclassification, Subsetting, Redefinition, FeatureTyping
// and so on, from the type to Target.
type Implied struct {
	Kind   string
	Target *model.Element
}

// ImpliedGeneralizations returns the implied general types of t, in the order
// the pilot adds them. The workspace must have been resolved.
func (w *Workspace) ImpliedGeneralizations(t *model.Element) []Implied {
	var out []Implied
	for _, x := range w.implicit(t) {
		out = append(out, Implied{x.kind, x.target})
	}
	return out
}

// ---- usages ----

func (w *Workspace) owningTypeIs(t *el, classes ...string) bool {
	owner := w.featureOwningType(t)
	if owner == nil {
		return false
	}
	for _, c := range classes {
		if owner.IsA(c) {
			return true
		}
	}
	return false
}

// hasClassType and hasStructureType follow FeatureAdapter: a Feature (or its
// owner) is typed by a Class or Structure, written or implied.
func (w *Workspace) hasClassOwner(t *el) bool {
	owner := w.featureOwningType(t)
	return owner.IsA("Class") || owner.IsA("Feature") && w.hasType(owner, "Class", w.implicit(owner))
}

func (w *Workspace) hasStructureOwner(t *el) bool {
	owner := w.featureOwningType(t)
	return owner.IsA("Structure") || owner.IsA("Feature") && w.hasType(owner, "Structure", w.implicit(owner))
}

// isSubitem is ItemUsageAdapter.isSubitem.
func (w *Workspace) isSubitem(t *el) bool {
	return t.IsA("ItemUsage") && t.IsTrue("isComposite") && w.owningTypeIs(t, "ItemDefinition", "ItemUsage")
}

func (w *Workspace) isEntryExitAction(t *el) bool {
	if t.Owner == nil || !t.Owner.IsA("StateSubactionMembership") {
		return false
	}
	k := t.Owner.Text("kind")
	return k == "entry" || k == "exit"
}

func (w *Workspace) isActionOwnedComposite(t *el) bool {
	return t.IsTrue("isComposite") && !w.isEntryExitAction(t) && w.owningTypeIs(t, "ActionDefinition", "ActionUsage")
}

func (w *Workspace) isPartOwnedComposite(t *el) bool {
	return t.IsTrue("isComposite") && w.owningTypeIs(t, "PartDefinition", "PartUsage")
}

// isSuboccurrence and isSubobject are the overrides down FeatureAdapter,
// OccurrenceUsageAdapter, ItemUsageAdapter and ActionUsageAdapter.
func (w *Workspace) isSuboccurrence(t *el) bool {
	r := t.IsTrue("isComposite") && w.hasClassOwner(t) || t.IsTrue("isComposite") && w.owningTypeIs(t, "OccurrenceUsage")
	switch {
	case t.IsA("ItemUsage"):
		return r && !w.isSubitem(t)
	case t.IsA("ActionUsage"):
		return r && !w.isActionOwnedComposite(t)
	}
	return r
}

func (w *Workspace) isSubobject(t *el) bool {
	r := t.IsTrue("isComposite") && w.hasStructureOwner(t)
	if t.IsA("ItemUsage") {
		return r && !w.isSubitem(t)
	}
	return r
}

// usageDefaultKind is getDefaultSupertype() of the adapter for a usage's
// metaclass.
func (w *Workspace) usageDefaultKind(t *el) string {
	isSubrequirement := t.IsTrue("isComposite") && w.owningTypeIs(t, "RequirementDefinition", "RequirementUsage") &&
		!t.Owner.IsA("RequirementConstraintMembership")
	switch {
	case t.IsA("AssertConstraintUsage"), t.IsA("SatisfyRequirementUsage"):
		if t.IsTrue("isNegated") {
			return "negated"
		}
	case t.IsA("ViewpointUsage"):
		if w.owningTypeIs(t, "ViewDefinition", "ViewUsage") {
			return "satisfied"
		}
	case t.IsA("ConcernUsage"):
	case t.IsA("RequirementUsage"):
		if isSubrequirement {
			return "subrequirement"
		}
	case t.IsA("ConstraintUsage"):
	case t.IsA("PortUsage"):
		switch {
		case w.owningTypeIs(t, "PartDefinition", "PartUsage"):
			return "ownedPort"
		case t.IsTrue("isComposite") && w.owningTypeIs(t, "PortDefinition", "PortUsage"):
			return "subport"
		}
	case t.IsA("RenderingUsage"):
		if w.owningTypeIs(t, "RenderingDefinition", "RenderingUsage") {
			return "subrendering"
		}
	case t.IsA("ViewUsage"):
		if w.owningTypeIs(t, "ViewDefinition", "ViewUsage") {
			return "subview"
		}
	case t.IsA("PartUsage"):
		actor := t.Owner.IsA("ActorMembership")
		switch {
		case actor && w.owningTypeIs(t, "RequirementDefinition", "RequirementUsage"):
			return "requirementActor"
		case t.Owner.IsA("StakeholderMembership") && w.owningTypeIs(t, "RequirementDefinition", "RequirementUsage"):
			return "requirementStakeholder"
		case actor && w.owningTypeIs(t, "CaseDefinition", "CaseUsage"):
			return "caseActor"
		}
		if w.isSubitem(t) {
			return "subitem"
		}
	case t.IsA("MetadataUsage"):
	case t.IsA("ItemUsage"):
		if w.isSubitem(t) {
			return "subitem"
		}
	case t.IsA("EventOccurrenceUsage"):
		if w.owningTypeIs(t, "OccurrenceDefinition", "OccurrenceUsage") {
			return "suboccurrence"
		}
	}
	return "base"
}

// occurrenceUsageKinds emits, in order, the kinds of implicit general type the
// adapters from OccurrenceUsageAdapter down add to a usage.
func (w *Workspace) occurrenceUsageKinds(t *el, emit func(string), soFar func() []implied) {
	// ConstraintUsageAdapter.computeImplicitGeneralTypes: the requirement
	// constraint kind comes first
	if t.IsA("ConstraintUsage") && t.Owner.IsA("RequirementConstraintMembership") {
		emit(t.Owner.Text("kind"))
	}
	if t.IsA("ConcernUsage") && t.Owner.IsA("FramedConcernMembership") {
		emit("concern")
	}
	if t.IsA("RequirementUsage") && t.Owner.IsA("RequirementVerificationMembership") {
		emit("verification")
	}
	emit(w.usageDefaultKind(t))
	// OccurrenceUsageAdapter.addDefaultGeneralType
	if w.hasType(t, "DataType", soFar()) {
		emit("dataValue")
	}
	switch {
	case w.hasType(t, "Structure", soFar()):
		if w.isSubobject(t) {
			emit("subobject")
		} else {
			emit("object")
		}
	case w.isSuboccurrence(t):
		emit("suboccurrence")
	}
	switch t.Text("portionKind") {
	case "snapshot":
		emit("snapshot")
	case "timeslice":
		emit("timeslice")
	}
	// the subclasses' own additions
	switch {
	case t.IsA("ActionUsage"):
		switch {
		case w.isActionOwnedComposite(t):
			emit("subaction")
		case w.isPartOwnedComposite(t):
			emit("ownedAction")
		}
		w.performanceKinds(t, emit, true)
		if t.IsA("PerformActionUsage") || t.IsA("ExhibitStateUsage") || t.IsA("IncludeUseCaseUsage") {
			if w.owningTypeIs(t, "PartDefinition", "PartUsage") {
				emit("performedAction")
			}
		}
		if t.IsA("IfActionUsage") && len(w.rels(t, "ParameterMembership")) > 2 {
			emit("ifThenElse")
		}
	case t.IsA("ConstraintUsage"):
		checked := t.IsTrue("isComposite") && w.owningTypeIs(t, "ItemDefinition", "ItemUsage")
		if checked {
			emit("checkedConstraint")
		}
		w.performanceKinds(t, emit, false)
	case t.IsA("PortUsage"):
		if w.isStructureOwnedComposite(t) {
			emit("subobject")
		}
	case t.IsA("RenderingUsage"), t.IsA("ViewUsage"):
		if w.isSubitem(t) {
			emit("subpart")
		}
	}
	if t.IsA("ConcernUsage") && t.IsTrue("isComposite") && w.owningTypeIs(t, "RequirementDefinition", "RequirementUsage") &&
		!t.Owner.IsA("RequirementConstraintMembership") {
		emit("subrequirement")
	}
}

// performanceKinds are StepAdapter's: where a step or constraint sits among
// the performances.
func (w *Workspace) performanceKinds(t *el, emit func(string), exclusive bool) {
	behavior := w.isBehaviorOwned(t)
	if t.IsA("AssertConstraintUsage") {
		behavior = w.owningTypeIs(t, "ActionDefinition", "ActionUsage")
	}
	owned := w.isStructureOwnedComposite(t)
	sub := behavior && t.IsTrue("isComposite")
	if owned {
		emit("ownedPerformance")
	}
	if sub && !(exclusive && owned) {
		emit("subperformance")
	}
	if behavior && !(exclusive && (owned || sub)) {
		emit("enclosedPerformance")
	}
}
