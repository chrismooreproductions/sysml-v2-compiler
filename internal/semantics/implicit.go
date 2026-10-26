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
	"strings"
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
	if v, ok := w.genMemo[t]; ok {
		return v
	}
	cutBefore := w.cut
	out := w.computeGeneralTypes(t)
	if w.cut == cutBefore {
		w.genMemo[t] = out
	}
	return out
}

func (w *Workspace) computeGeneralTypes(t *el) []*el {
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
		g := w.refTarget(r, "originalType")
		if g == nil { // a PortConjugation holds it as the original port definition
			g = w.refTarget(r, "originalPortDefinition")
		}
		if g != nil {
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
	case t.Owner != nil && t.Owner.IsA("Specialization") && t.Owner.Owner.IsA("Membership"):
		// a chain a standalone specialization owns (subset a.b subsets c.d) is not transformed
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
	// FeatureAdapter.addDefaultGeneralType adds the cross feature's typing first,
	// because the default supertype depends on it
	if t.IsA("Feature") {
		w.addOwnedCrossFeatureSpecialization(t, add)
	}
	// TypeAdapter.addDefaultGeneralType: the metaclass's default supertype
	emit := func(kind string) {
		if qn := implicitKind(t.Class, kind); qn != "" {
			add(baseKind(t), w.libraryElement(qn))
		}
	}
	if t.IsA("FlowUsage") {
		w.flowUsageKinds(t, emit)
	} else if t.IsA("OccurrenceUsage") {
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
		w.addParticipantSubsetting(t, add)
		w.addCrossingSpecialization(t, add)
		w.addRedefinitions(t, add) // FeatureAdapter.addRedefinitions
		// the payload of a transition subsets its trigger's payload, as the
		// chain [accept action, payload] (TransitionUsageAdapter)
		if accepter, accepted := w.transitionPayload(t); accepted != nil {
			add("Subsetting", w.chainFeatures(accepter, accepted))
		}
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
func (w *Workspace) specializesExcluding(sub, super, exclude *el) (result bool) {
	// What a library type specializes does not depend on the type being
	// computed, which no library type reaches, so the answer is remembered (for
	// library types only, so that what is remembered never holds a user's file).
	lib := sub != nil && super != nil && strings.HasPrefix(w.FileOf(sub), "library:") && strings.HasPrefix(w.FileOf(super), "library:")
	key := specKey{sub, super}
	if lib {
		if v, ok := w.specMemo[key]; ok {
			return v
		}
		cutBefore := w.cut
		defer func() {
			if w.cut == cutBefore {
				w.specMemo[key] = result
			}
		}()
	}
	// the types visited are few (a type's ancestors), so a slice is searched
	// rather than a map hashed
	var seen []*el
	if exclude != nil {
		seen = append(seen, exclude)
	}
	var walk func(x *el) bool
	walk = func(x *el) bool {
		if x == super {
			return true
		}
		if x == nil {
			return false
		}
		for _, y := range seen {
			if y == x {
				return false
			}
		}
		seen = append(seen, x)
		for _, g := range w.generalTypes(x) {
			if walk(g) {
				return true
			}
		}
		return false
	}
	return walk(sub)
}

type specKey struct{ sub, super *el }

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
	if v, ok := w.featsMemo[t]; ok {
		return v
	}
	cutBefore := w.cut
	out := w.ownedFeatures(t)
	for _, m := range w.inherited(t, map[*el]bool{t: true}) {
		if m.IsA("FeatureMembership") {
			if f := w.memberElement(m); f.IsA("Feature") {
				out = append(out, f)
			}
		}
	}
	if w.cut == cutBefore {
		w.featsMemo[t] = out
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
		for _, g := range w.typesOf(t) {
			if g.IsA("Function") {
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
	case e.IsA("ConstructorExpression"):
		// ConstructorExpressionAdapter.addResultTyping
		if inst := w.instantiatedType(e); inst != nil {
			if inst.IsA("Feature") {
				add("Subsetting", inst)
			} else {
				add("FeatureTyping", inst)
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
		if g := w.typingTarget(r); g.IsA(class) {
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
	case t.IsA("ConnectionUsage") && !t.IsA("FlowUsage"):
		// ConnectionUsageAdapter
		if w.ownedEndCount(t) == 2 {
			return "binary"
		}
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
	case t.IsA("RenderingUsage"), t.IsA("ViewUsage"), t.IsA("ConnectionUsage"):
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

// ---- end features and their cross features ----

// ownedCrossFeature is FeatureUtil.getOwnedCrossFeatureOf: the first feature an
// end feature owns that is not a multiplicity, metadata, binding or a member
// by feature membership or value.
func (w *Workspace) ownedCrossFeature(ns *el) *el {
	if !ns.IsA("Feature") || !ns.IsTrue("isEnd") || w.featureOwningType(ns) == nil {
		return nil
	}
	for _, m := range w.rels(ns, "Membership") {
		if !m.IsA("OwningMembership") || m.IsA("FeatureMembership") || m.IsA("FeatureValue") {
			continue
		}
		e := first(w.ownedRelated(m))
		if e.IsA("Feature") && !e.IsA("Multiplicity") && !e.IsA("MetadataFeature") && !e.IsA("BindingConnector") {
			return e
		}
	}
	return nil
}

// typesOf is Feature.type: the feature's own and implied types and those of
// the features it subsets, redefines or chains to, less any type another of
// them specializes (FeatureAdapter.getAllTypes).
func (w *Workspace) typesOf(f *el) []*el {
	types := w.collectTypes(f, map[*el]bool{})
	for i := len(types) - 1; i >= 0; i-- {
		for _, o := range types {
			if o != types[i] && w.specializes(o, types[i]) {
				types = append(types[:i], types[i+1:]...)
				break
			}
		}
	}
	return types
}

func (w *Workspace) collectTypes(f *el, visited map[*el]bool) []*el {
	visited[f] = true
	var out []*el
	var via []*el // typing features
	if c := w.chainOf[f]; len(c) > 0 {
		via = append(via, c[len(c)-1])
	} else {
		for _, r := range w.rels(f, "FeatureTyping") {
			if g := w.typingTarget(r); g != nil {
				out = append(out, g)
			}
		}
		for _, r := range w.rels(f, "Subsetting") {
			if r.IsA("CrossSubsetting") {
				continue
			}
			role := "subsettedFeature"
			switch {
			case r.IsA("Redefinition"):
				role = "redefinedFeature"
			case r.IsA("ReferenceSubsetting"):
				role = "referencedFeature"
			}
			if g := w.refTarget(r, role); g != nil {
				via = append(via, g)
			}
		}
		for _, x := range w.implicit(f) {
			switch x.kind {
			case "FeatureTyping":
				out = append(out, x.target)
			case "Subsetting", "ReferenceSubsetting", "Redefinition":
				via = append(via, x.target)
			}
		}
		if ch := w.rels(f, "FeatureChaining"); len(ch) > 0 {
			if g := w.refTarget(ch[len(ch)-1], "chainingFeature"); g != nil {
				via = append(via, g)
			}
		}
	}
	for _, g := range via {
		if g.IsA("Feature") && !visited[g] {
			out = append(out, w.collectTypes(g, visited)...)
		}
	}
	return out
}

// typingTarget is the type a feature typing gives its feature: its type, or for
// a conjugated port typing (p : ~Fuel) the conjugated port definition.
func (w *Workspace) typingTarget(r *el) *el {
	if t := w.refTarget(r, "type"); t != nil {
		return t
	}
	return w.refTarget(r, "conjugatedPortDefinition")
}

// redefinedWithComputed is FeatureUtil.getRedefinedFeaturesWithComputedOf.
func (w *Workspace) redefinedWithComputed(f *el) []*el {
	var out []*el
	for _, r := range w.rels(f, "Redefinition") {
		if g := w.refTarget(r, "redefinedFeature"); g != nil {
			out = append(out, g)
		}
	}
	for _, x := range w.implicit(f) {
		if x.kind == "Redefinition" {
			out = append(out, x.target)
		}
	}
	return out
}

// addOwnedCrossFeatureSpecialization is FeatureAdapter's: the cross feature of
// an end is typed like the end and subsets the cross features of the ends it
// redefines.
func (w *Workspace) addOwnedCrossFeatureSpecialization(t *el, add func(string, *el)) {
	if t.Owner == nil || !t.Owner.IsA("OwningMembership") || t.Owner.Owner == nil {
		return
	}
	owner := t.Owner.Owner
	if w.ownedCrossFeature(owner) != t {
		return
	}
	for _, g := range w.typesOf(owner) {
		add("FeatureTyping", g)
	}
	for _, r := range w.redefinedWithComputed(owner) {
		if r.IsTrue("isEnd") {
			if c := w.crossFeatureOf(r); c != nil {
				add("Subsetting", c)
			}
		}
	}
}

// addParticipantSubsetting is FeatureAdapter.addParticipantSubsetting.
func (w *Workspace) addParticipantSubsetting(t *el, add func(string, *el)) {
	if !t.IsTrue("isEnd") || !w.owningTypeIs(t, "Association", "Connector") || len(w.rels(t, "Redefinition")) > 0 {
		return
	}
	if qn := implicitKind(t.Class, "participant"); qn != "" {
		add("Subsetting", w.libraryElement(qn))
	}
}

// addCrossingSpecialization is FeatureAdapter.addCrossingSpecialization: an end
// with a cross feature cross-subsets a chain the pilot generates.
func (w *Workspace) addCrossingSpecialization(t *el, add func(string, *el)) {
	cross := w.ownedCrossFeature(t)
	if cross == nil || len(w.rels(t, "CrossSubsetting")) > 0 {
		return
	}
	owner := w.featureOwningType(t)
	var ends []*el
	for _, f := range w.ownedFeatures(owner) {
		if f.IsTrue("isEnd") {
			ends = append(ends, f)
		}
	}
	if len(ends) == 2 {
		other := ends[0]
		if other == t {
			other = ends[1]
		}
		add("CrossSubsetting", w.chainFeatures(other, cross))
		return
	}
	first := &el{Class: "Feature", Generated: true}
	add("CrossSubsetting", w.chainFeatures(first, cross))
}

// chainingFeatures are the features a (written or generated) chain chains.
func (w *Workspace) chainingFeatures(f *el) []*el {
	if c := w.chainOf[f]; len(c) > 0 {
		return c
	}
	var out []*el
	for _, r := range w.rels(f, "FeatureChaining") {
		out = append(out, w.refTarget(r, "chainingFeature"))
	}
	return out
}

// crossFeatureOf is FeatureUtil.getCrossFeatureOf: the second feature of the
// chain an end feature crosses, written (crosses a.b) or implied.
func (w *Workspace) crossFeatureOf(f *el) *el {
	var crossed *el
	for _, r := range w.rels(f, "CrossSubsetting") {
		crossed = w.refTarget(r, "crossedFeature")
		break
	}
	if crossed == nil {
		for _, x := range w.implicit(f) {
			if x.kind == "CrossSubsetting" {
				crossed = x.target
				break
			}
		}
	}
	if crossed == nil {
		return nil
	}
	if c := w.chainingFeatures(crossed); len(c) >= 2 {
		return c[1]
	}
	return nil
}

// ---- featuring types ----

// ImpliedFeaturingTypes returns the types the pilot's transformation features
// a feature by without writing a TypeFeaturing: the type that owns it and the
// others FeatureAdapter.doTransform and its subclasses add. The workspace must
// have been resolved.
func (w *Workspace) ImpliedFeaturingTypes(f *el) []*el {
	if !f.IsA("Feature") {
		return nil
	}
	if v, ok := w.featMemo[f]; ok {
		return v
	}
	if w.featBusy[f] {
		return nil
	}
	w.featBusy[f] = true
	defer delete(w.featBusy, f)
	var out []*el
	add := func(ts ...*el) {
		for _, t := range ts {
			dup := t == nil
			for _, x := range out {
				dup = dup || x == t
			}
			if !dup {
				out = append(out, t)
			}
		}
	}
	// FeatureAdapter.computeFeaturingType: a variable feature (a usage that may
	// time-vary, or one marked var) is featured by the snapshots of its type
	if f.Owner.IsA("FeatureMembership") && f.Owner.Owner != nil {
		if variable := f.IsA("Usage") && w.mayTimeVary(f) || !f.IsA("Usage") && f.IsTrue("isVariable"); !variable {
			add(f.Owner.Owner)
		} else if occurrence := w.libraryElement("Occurrences::Occurrence"); f.Owner.Owner == occurrence {
			add(w.libraryElement("Occurrences::Occurrence::snapshots"))
		} else {
			add(w.snapshotsOf(f.Owner.Owner))
		}
	}
	owner := w.owningNamespace(f)
	// addImplicitFeaturingTypesIfNecessary: a feature in a feature is featured as it is
	ifNecessary := func() {
		if owner.IsA("Feature") && len(out) == 0 {
			add(w.featuringTypesOf(owner)...)
		}
	}
	switch {
	case f.IsA("Multiplicity"):
		ifNecessary() // MultiplicityAdapter.doTransform
	case f.IsA("Expression"):
		// ExpressionAdapter.addImplicitFeaturingTypesIfNecessary
		if owner.IsA("MultiplicityRange") && f.Owner.IsA("OwningMembership") {
			if outer := w.owningNamespace(owner); outer.IsA("Feature") {
				if w.isOwnedCrossFeature(outer) && len(out) == 0 {
					add(w.featuringTypesOf(outer.Owner.Owner)...)
				} else {
					ifNecessary()
				}
			}
		} else if f.Owner.IsA("FeatureValue") {
			ifNecessary()
		}
	}
	// FeatureAdapter.addOwnedCrossFeatureTypeFeaturing
	if w.isOwnedCrossFeature(f) && len(w.rels(f, "TypeFeaturing")) == 0 && len(out) == 0 {
		end := f.Owner.Owner
		if t := w.featureOwningType(end); t != nil {
			ends := w.endFeatures(t)
			if len(ends) == 2 {
				other := ends[0]
				if other == end {
					other = ends[1]
				}
				add(w.typesOf(other)...)
			} else if len(ends) > 2 {
				add(&el{Class: "Feature", Generated: true})
			}
		}
	}
	w.featMemo[f] = out
	return out
}

// snapshotsOf is the feature the pilot makes for the snapshots of a type.
func (w *Workspace) snapshotsOf(t *el) *el {
	if s, ok := w.snapshots[t]; ok {
		return s
	}
	s := &el{Class: "Feature", Generated: true}
	w.snapshots[t] = s
	return s
}

// endFeatures are Type.endFeature: the end features, owned or inherited, that
// no other of them redefines.
func (w *Workspace) endFeatures(t *el) []*el {
	var ends []*el
	for _, x := range w.features(t) {
		if x.IsTrue("isEnd") {
			ends = append(ends, x)
		}
	}
	var out []*el
	for _, x := range ends {
		redefined := false
		for _, y := range ends {
			if y == x {
				continue
			}
			for _, r := range w.redefinedWithComputed(y) {
				redefined = redefined || r == x
			}
		}
		if !redefined {
			out = append(out, x)
		}
	}
	return out
}

func (w *Workspace) isOwnedCrossFeature(f *el) bool {
	return f.Owner.IsA("OwningMembership") && f.Owner.Owner != nil && w.ownedCrossFeature(f.Owner.Owner) == f
}

// featuringTypesOf is Feature.featuringType: the written and implied featuring
// types and those of the first chaining feature.
func (w *Workspace) featuringTypesOf(f *el) []*el {
	var out []*el
	for _, r := range w.rels(f, "TypeFeaturing") {
		if g := w.refTarget(r, "featuringType"); g != nil {
			out = append(out, g)
		}
	}
	out = append(out, w.ImpliedFeaturingTypes(f)...)
	if c := w.chainingFeatures(f); len(c) > 0 && c[0] != nil {
		out = append(out, w.featuringTypesOf(c[0])...)
	}
	return out
}

// ---- binding connectors ----

// Binding is a BindingConnector the pilot's transformation adds to a type: it
// binds Source to Target.
type Binding struct{ Source, Target *model.Element }

// ImpliedBindings returns the binding connectors the transformation of t adds:
// for a feature value, a reference, an invocation of a behavior or a result
// expression. The workspace must have been resolved.
func (w *Workspace) ImpliedBindings(t *el) []Binding {
	var out []Binding
	add := func(src, tgt *el) {
		if src != nil && tgt != nil {
			out = append(out, Binding{src, tgt})
		}
	}
	// FeatureAdapter.computeValueConnector (checkFeatureValueBindingConnector)
	if t.IsA("Feature") {
		for _, fv := range w.rels(t, "FeatureValue") {
			if fv.IsTrue("isDefault") {
				break
			}
			if value := first(w.ownedRelated(fv)); value != nil {
				if res := w.resultOf(value); res != nil {
					add(w.chainFeatures(value, res), t)
				}
			}
			break
		}
	}
	// FeatureReferenceExpressionAdapter.addReferenceConnector
	if t.IsA("FeatureReferenceExpression") {
		// ExpressionUtil.getReferentFor: the first membership that is not a parameter's
		for _, m := range w.rels(t, "Membership") {
			if m.IsA("ParameterMembership") {
				continue
			}
			if r := w.memberElement(m); r.IsA("Feature") {
				add(r, w.resultOf(t))
			}
			break
		}
	}
	// InvocationExpressionAdapter.createSelfResultConnector
	if t.IsA("InvocationExpression") && !t.IsA("OperatorExpression") {
		if inst := w.instantiatedType(t); inst != nil && !w.isFunctionType(inst) {
			add(t, w.resultOf(t))
		}
	}
	// TypeAdapter.createResultConnector
	if t.IsA("Function") || t.IsA("Expression") || t.IsA("CalculationUsage") || t.IsA("ConstraintUsage") {
		for _, m := range w.rels(t, "ResultExpressionMembership") {
			if e := first(w.ownedRelated(m)); e != nil && e.IsA("Expression") {
				add(w.resultOf(e), w.resultParameterOf(t, map[*el]bool{}))
			}
			break
		}
	}
	return out
}

// flowUsageKinds follows FlowUsageAdapter: a flow usage that has no ends of its
// own is a message; the rest is what an occurrence, action and step add.
func (w *Workspace) flowUsageKinds(t *el, emit func(string)) {
	isMessage := true
	for _, f := range w.ownedFeatures(t) {
		isMessage = isMessage && !f.IsTrue("isEnd")
	}
	if isMessage {
		emit("message")
	} else {
		emit("base")
	}
	if w.isSuboccurrence(t) {
		emit("suboccurrence")
	}
	switch t.Text("portionKind") {
	case "snapshot":
		emit("snapshot")
	case "timeslice":
		emit("timeslice")
	}
	switch {
	case w.isActionOwnedComposite(t):
		emit("subaction")
	case w.isPartOwnedComposite(t):
		emit("ownedAction")
	}
	w.performanceKinds(t, emit, true)
}
