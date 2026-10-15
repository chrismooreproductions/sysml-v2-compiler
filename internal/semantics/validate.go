// SPDX-License-Identifier: EPL-2.0
//
// The constraints follow the checks of the pilot implementation's KerMLValidator
// and SysMLValidator (Copyright (c) 2018-2026 Model Driven Solutions, Inc. and
// others, EPL-2.0), each named by the specification constraint it implements.

package semantics

import (
	"strings"
	"unicode"
)

// Issue is a violation of one of the specification's constraints.
type Issue struct {
	Element    *el
	Constraint string // the specification's name for the constraint
	Message    string
	Severity   string // "error" or "warning"
}

// check is one constraint: it applies to a metaclass, and reports whether an
// element of that metaclass violates it.
type check struct {
	class      string
	constraint string
	violated   func(w *Workspace, e *el) bool  // the element itself is at fault
	at         func(w *Workspace, e *el) []*el // or these elements are
}

// Validate checks the constraints on every element under the roots that the
// source wrote, and returns the violations in document order. The workspace
// must have been resolved.
func (w *Workspace) Validate(roots ...*el) []Issue {
	var out []Issue
	for _, root := range roots {
		root.Walk(func(e *el) {
			if e.Generated {
				return
			}
			for _, c := range structuralChecks {
				if !e.IsA(c.class) {
					continue
				}
				if c.violated != nil && c.violated(w, e) {
					out = append(out, Issue{e, base(c.constraint), messageFor(c.constraint), "error"})
				}
				if c.at != nil {
					for _, bad := range c.at(w, e) {
						out = append(out, Issue{bad, base(c.constraint), messageFor(c.constraint), "error"})
					}
				}
			}
		})
	}
	return out
}

// irregularMessages are the constraints whose message the pilot names
// differently.
var irregularMessages = map[string]string{
	"validateDefinitionVariationOwnedFeatureMembership": "DEFINITION_VARIATION_MEMBERSHIP",
	"validateUsageVariationOwnedFeatureMembership":      "USAGE_VARIATION_MEMBERSHIP",
}

// base is a constraint's name without the message variant.
func base(constraint string) string {
	if i := strings.IndexByte(constraint, '#'); i >= 0 {
		return constraint[:i]
	}
	return constraint
}

// messageFor is the message of a constraint: the pilot names its constant
// after the constraint, in capitals with underscores.
func messageFor(constraint string) string {
	// "constraint#n" asks for the n-th of the constraint's messages
	variant := ""
	if i := strings.IndexByte(constraint, '#'); i >= 0 {
		constraint, variant = constraint[:i], "_"+constraint[i+1:]
	}
	if key, ok := irregularMessages[constraint]; ok {
		return validationMessages[key+variant]
	}
	var sb strings.Builder
	for i, r := range strings.TrimSuffix(strings.TrimPrefix(constraint, "validate"), "_") {
		if i > 0 && unicode.IsUpper(r) {
			sb.WriteByte('_')
		}
		sb.WriteRune(unicode.ToUpper(r))
	}
	if m, ok := validationMessages[sb.String()+variant]; ok {
		return m
	}
	for _, prefix := range []string{"FEATURE_", "TYPE_"} { // validatePortionNotVariable, validateDifferencingTypesNotSelf
		if m, ok := validationMessages[prefix+sb.String()+variant]; ok {
			return m
		}
	}
	return ""
}

// ownerIs reports whether the owner of a membership is one of the classes.
func ownerIs(e *el, classes ...string) bool {
	for _, c := range classes {
		if e.Owner.IsA(c) {
			return true
		}
	}
	return false
}

// notOwnedBy is a check that a membership's owning type is one of the classes.
func notOwnedBy(class, constraint string, classes ...string) check {
	return check{class: class, constraint: constraint, violated: func(_ *Workspace, e *el) bool { return !ownerIs(e, classes...) }}
}

// isVariable is Feature.isVariable, which for a usage is whether it may
// time-vary.
func (w *Workspace) isVariable(f *el) bool {
	if f.IsA("Usage") {
		return w.mayTimeVary(f)
	}
	return f.IsTrue("isVariable")
}

// isVariation: an enumeration definition is always a variation.
func isVariation(e *el) bool { return e.IsTrue("isVariation") || e.IsA("EnumerationDefinition") }

// isConstructorResult is ExpressionUtil.isConstructorResult: the result of a
// constructor expression.
func isConstructorResult(t *el) bool {
	return t.Owner.IsA("ReturnParameterMembership") && t.Owner.Owner.IsA("ConstructorExpression")
}

// (validateEnumerationDefinitionIsVariation cannot fail: an enumeration
// definition is a variation by definition.)
var structuralChecks = []check{
	{class: "Type", constraint: "validateTypeAtMostOneConjugator", violated: func(w *Workspace, e *el) bool { return len(w.rels(e, "Conjugation")) > 1 }},
	{class: "Feature", constraint: "validateFeatureEndNoDirection", violated: func(_ *Workspace, e *el) bool { return e.IsTrue("isEnd") && e.Has("direction") }},
	{class: "Feature", constraint: "validateFeatureConstantIsVariable", violated: func(w *Workspace, e *el) bool { return e.IsTrue("isConstant") && !w.isVariable(e) }},
	{class: "Feature", constraint: "validatePortionNotVariable", violated: func(w *Workspace, e *el) bool { return e.IsTrue("isPortion") && w.isVariable(e) }},
	{class: "Feature", constraint: "validateFeatureEndNotDerivedAbstractCompositeOrPortion", violated: func(_ *Workspace, e *el) bool {
		return e.IsTrue("isEnd") && (e.IsTrue("isDerived") || e.IsTrue("isAbstract") || e.IsTrue("isComposite") || e.IsTrue("isPortion"))
	}},
	{class: "EndFeatureMembership", constraint: "validateEndFeatureMembershipIsEnd", violated: func(w *Workspace, e *el) bool {
		f := first(w.ownedRelated(e))
		return f != nil && !f.IsTrue("isEnd")
	}},
	{class: "FlowEnd", constraint: "validateFlowEndIsEnd", violated: func(_ *Workspace, e *el) bool { return !e.IsTrue("isEnd") }},
	{class: "VariantMembership", constraint: "validateVariantMembershipOwningNamespace", violated: func(_ *Workspace, e *el) bool { return !isVariation(e.Owner) }},
	{class: "Usage", constraint: "validateUsageVariationIsAbstract", violated: func(_ *Workspace, e *el) bool { return isVariation(e) && !e.IsTrue("isAbstract") }},
	{class: "ParameterMembership", constraint: "validateParameterMembershipOwningType", violated: func(_ *Workspace, e *el) bool {
		return !e.Owner.IsA("Behavior") && !e.Owner.IsA("Step") && !isConstructorResult(e.Owner) && !e.IsA("ReturnParameterMembership") && !e.IsA("ResultExpressionMembership")
	}},
	{class: "ReturnParameterMembership", constraint: "validateReturnParameterMembershipOwningType", violated: func(_ *Workspace, e *el) bool {
		return !e.Owner.IsA("Function") && !e.Owner.IsA("Expression")
	}},
	{class: "ResultExpressionMembership", constraint: "validateResultExpressionMembershipOwningType", violated: func(_ *Workspace, e *el) bool {
		return !e.Owner.IsA("Function") && !e.Owner.IsA("Expression")
	}},
	notOwnedBy("ActorMembership", "validateActorMembershipOwningType", "RequirementDefinition", "RequirementUsage", "CaseDefinition", "CaseUsage"),
	notOwnedBy("SubjectMembership", "validateSubjectMembershipOwningType", "RequirementDefinition", "RequirementUsage", "CaseDefinition", "CaseUsage"),
	notOwnedBy("ObjectiveMembership", "validateObjectiveMembershipOwningType", "CaseDefinition", "CaseUsage"),
	notOwnedBy("StakeholderMembership", "validateStakeholderMembershipOwningType", "RequirementDefinition", "RequirementUsage"),
	notOwnedBy("TransitionFeatureMembership", "validateTransitionFeatureMembershipOwningType", "TransitionUsage"),
	notOwnedBy("StateSubactionMembership", "validateStateSubactionMembershipOwningType", "StateUsage", "StateDefinition"),
	{class: "ControlNode", constraint: "validateControlNodeOwningType", violated: func(w *Workspace, e *el) bool {
		return !w.owningTypeIs(e, "ActionUsage", "ActionDefinition")
	}},

	// -- counts and distinctness --
	{class: "Type", constraint: "validateTypeOwnedDifferencingNotOne", violated: func(w *Workspace, e *el) bool { return len(w.rels(e, "Differencing")) == 1 }},
	{class: "Type", constraint: "validateTypeOwnedIntersectingNotOne", violated: func(w *Workspace, e *el) bool { return len(w.rels(e, "Intersecting")) == 1 }},
	{class: "Type", constraint: "validateTypeOwnedUnioningNotOne", violated: func(w *Workspace, e *el) bool { return len(w.rels(e, "Unioning")) == 1 }},
	{class: "Feature", constraint: "validateFeatureChainingFeatureNotOne", violated: func(w *Workspace, e *el) bool { return len(w.rels(e, "FeatureChaining")) == 1 }},
	{class: "Type", constraint: "validateDifferencingTypesNotSelf", at: relatedToSelf("Differencing", "differencingType")},
	{class: "Type", constraint: "validateTypeIntersectingTypesNotSelf", at: relatedToSelf("Intersecting", "intersectingType")},
	{class: "Type", constraint: "validateTypeUnioningTypesNotSelf", at: relatedToSelf("Unioning", "unioningType")},
	{class: "Feature", constraint: "validateFeatureChainingFeaturesNotSelf", at: relatedToSelf("FeatureChaining", "chainingFeature")},
	{class: "Type", constraint: "validateTypeOwnedMultiplicity", violated: func(w *Workspace, e *el) bool {
		n := 0
		for _, m := range w.rels(e, "Membership") {
			if first(w.ownedRelated(m)).IsA("Multiplicity") {
				n++
			}
		}
		return n > 1
	}},
	{class: "Feature", constraint: "validateFeatureOwnedCrossSubsetting", at: func(w *Workspace, e *el) []*el {
		if cs := w.rels(e, "CrossSubsetting"); len(cs) > 1 {
			return cs[1:]
		}
		return nil
	}},
	{class: "Expression", constraint: "validateExpressionResultParameterMembership", violated: func(w *Workspace, e *el) bool { return len(w.rels(e, "ReturnParameterMembership")) > 1 }},
	{class: "Function", constraint: "validateFunctionResultParameterMembership", violated: func(w *Workspace, e *el) bool { return len(w.rels(e, "ReturnParameterMembership")) > 1 }},
	// -- flags that depend on one another --
	{class: "FeatureValue", constraint: "validateFeatureValueIsInitial", violated: func(w *Workspace, e *el) bool {
		return e.IsTrue("isInitial") && e.Owner.IsA("Feature") && !w.isVariable(e.Owner)
	}},
	{class: "OccurrenceUsage", constraint: "validateOccurrenceUsageIsPortion", violated: func(_ *Workspace, e *el) bool { return e.Has("portionKind") && !e.IsTrue("isPortion") }},
	{class: "OccurrenceUsage", constraint: "validateOccurrenceUsagePortionKind", violated: func(w *Workspace, e *el) bool {
		return e.Has("portionKind") && !w.owningTypeIs(e, "OccurrenceDefinition", "OccurrenceUsage")
	}},
	{class: "Import", constraint: "validateImportTopLevelVisibility", violated: func(_ *Workspace, e *el) bool {
		return e.Owner != nil && e.Owner.Owner == nil && e.Text("visibility") != "private"
	}},
	{class: "Expose", constraint: "validateExposeIsImportAll", violated: func(_ *Workspace, e *el) bool { return !e.IsTrue("isImportAll") }},
	{class: "Expose", constraint: "validateExposeOwningNamespace", violated: func(_ *Workspace, e *el) bool { return !e.Owner.IsA("ViewUsage") }},
	{class: "RequirementConstraintMembership", constraint: "validateRequirementConstraintMembershipIsComposite", violated: func(w *Workspace, e *el) bool {
		c := first(w.ownedRelated(e))
		return c != nil && !c.IsTrue("isComposite")
	}},
	{class: "ObjectiveMembership", constraint: "validateObjectiveMembershipIsComposite", violated: func(w *Workspace, e *el) bool {
		c := first(w.ownedRelated(e))
		return c != nil && !c.IsTrue("isComposite")
	}},
	{class: "RequirementVerificationMembership", constraint: "validateRequirementVerificationMembershipKind", violated: func(_ *Workspace, e *el) bool { return e.Text("kind") != "requirement" }},
	{class: "FramedConcernMembership", constraint: "validateFramedConcernMembershipConstraintKind", violated: func(_ *Workspace, e *el) bool { return e.Text("kind") != "requirement" }},
	// -- the parameters of the control actions --
	{class: "ForLoopActionUsage", constraint: "validateForLoopActionUsageParameters", violated: func(w *Workspace, e *el) bool { return w.inputParameterCount(e) != 2 }},
	{class: "IfActionUsage", constraint: "validateIfActionUsageParameters", violated: func(w *Workspace, e *el) bool { return w.inputParameterCount(e) < 2 }},
	{class: "WhileLoopActionUsage", constraint: "validateWhileLoopActionUsageParameters", violated: func(w *Workspace, e *el) bool { return w.inputParameterCount(e) < 2 }},
	{class: "AcceptActionUsage", constraint: "validateAcceptActionUsageParameters", violated: func(w *Workspace, e *el) bool { return w.inputParameterCount(e) < 1 }},
	// -- specialization --
	{class: "Feature", constraint: "validateFeatureOwnedReferenceSubsetting", at: func(w *Workspace, e *el) []*el {
		if rs := w.rels(e, "ReferenceSubsetting"); len(rs) > 1 {
			return rs[1:]
		}
		return nil
	}},
	{class: "DataType", constraint: "validateDataTypeSpecialization", at: badGenerals("Class", "Association")},
	{class: "Structure", constraint: "validateStructureSpecialization", at: badGenerals("Behavior")},
	{class: "Behavior", constraint: "validateBehaviorSpecialization", at: badGenerals("Structure")},
	{class: "Subsetting", constraint: "validateSubsettingUniquenessConformance", violated: func(w *Workspace, e *el) bool {
		sub, sup := e.Owner, w.generalOf(e)
		return sup.IsA("Feature") && sub.IsA("Feature") && !isNonunique(sup) && isNonunique(sub)
	}},
	{class: "Subsetting", constraint: "validateSubsettingConstantConformance", violated: func(w *Workspace, e *el) bool {
		sub, sup := e.Owner, w.generalOf(e)
		return sup.IsA("Feature") && sub.IsA("Feature") && sup.IsTrue("isConstant") && w.isVariable(sub) && !sub.IsTrue("isConstant")
	}},
	{class: "Feature", constraint: "validateFeatureIsVariable", violated: func(w *Workspace, e *el) bool {
		owner := w.featureOwningType(e)
		return w.isVariable(e) && (owner == nil || !w.specializes(owner, w.libraryElement("Occurrences::Occurrence")))
	}},
	{class: "Redefinition", constraint: "validateRedefinitionEndConformance", violated: func(w *Workspace, e *el) bool {
		redefining, redefined := e.Owner, w.generalOf(e)
		if !redefined.IsA("Feature") || !redefining.IsA("Feature") || !redefined.IsTrue("isEnd") || redefining.IsTrue("isEnd") {
			return false
		}
		return w.owningTypeIs(redefining, "Association", "Connector")
	}},
	// -- variations --
	{class: "Definition", constraint: "validateDefinitionVariationOwnedFeatureMembership", at: func(w *Workspace, e *el) []*el {
		if !e.IsTrue("isVariation") {
			return nil
		}
		return w.rels(e, "FeatureMembership")
	}},
	{class: "Usage", constraint: "validateUsageVariationOwnedFeatureMembership", at: func(w *Workspace, e *el) []*el {
		var out []*el
		if e.IsTrue("isVariation") {
			for _, m := range w.rels(e, "FeatureMembership") {
				if !m.IsA("ParameterMembership") && !m.IsA("ObjectiveMembership") {
					out = append(out, m)
				}
			}
		}
		return out
	}},
	{class: "Definition", constraint: "validateDefinitionVariationSpecialization", at: variationGenerals},
	{class: "Usage", constraint: "validateUsageVariationSpecialization", at: variationGenerals},
	// -- expressions and flows --
	{class: "ConstructorExpression", constraint: "validateConstructorExpressionOwnedFeatures", at: func(w *Workspace, e *el) []*el {
		result := w.resultOf(e)
		var out []*el
		for _, f := range w.ownedFeatures(e) {
			if f != result {
				out = append(out, f)
			}
		}
		return out
	}},
	{class: "FlowEnd", constraint: "validateFlowEndNestedFeature", violated: func(w *Workspace, e *el) bool { return len(w.ownedFeatures(e)) != 1 }},
	{class: "FlowEnd", constraint: "validateFlowEndOwningType", violated: func(w *Workspace, e *el) bool { return !w.owningTypeIs(e, "Flow") }},
	{class: "MetadataFeature", constraint: "validateMetadataFeatureMetaclass", violated: func(w *Workspace, e *el) bool {
		n := 0
		for _, t := range w.typesOf(e) {
			if t.IsA("Metaclass") {
				n++
			}
		}
		return n != 1
	}},
	{class: "ForLoopActionUsage", constraint: "validateForLoopActionUsageLoopVariable", violated: func(w *Workspace, e *el) bool {
		fs := w.ownedFeatures(e)
		return len(fs) == 0 || !fs[0].IsA("ReferenceUsage")
	}},
	{class: "TransitionFeatureMembership", constraint: "validateTransitionFeatureMembershipTriggerAction", violated: func(w *Workspace, e *el) bool {
		return e.Text("kind") == "trigger" && !first(w.ownedRelated(e)).IsA("AcceptActionUsage")
	}},
	{class: "TransitionFeatureMembership", constraint: "validateTransitionFeatureMembershipEffectAction", violated: func(w *Workspace, e *el) bool {
		return e.Text("kind") == "effect" && !first(w.ownedRelated(e)).IsA("ActionUsage")
	}},
	{class: "OccurrenceUsage", constraint: "validateOccurrenceUsageIndividualDefinition", violated: func(w *Workspace, e *el) bool {
		return w.individualDefinitions(e) > 1
	}},
	// -- ports --
	{class: "PortDefinition", constraint: "validatePortDefinitionOwnedUsagesNotComposite", at: compositeUsages},
	{class: "PortUsage", constraint: "validatePortUsageNestedUsagesNotComposite", at: compositeUsages},
	// -- usages --
	{class: "Usage", constraint: "validateUsageIsReferential#1", violated: func(_ *Workspace, e *el) bool { return e.IsTrue("isComposite") && e.Has("direction") }},
	{class: "Usage", constraint: "validateUsageIsReferential#2", violated: func(_ *Workspace, e *el) bool {
		return e.IsTrue("isComposite") && !e.Has("direction") && e.IsTrue("isEnd")
	}},
	{class: "OccurrenceUsage", constraint: "validateOccurrenceUsageIndividualUsage", violated: func(w *Workspace, e *el) bool {
		if !e.IsTrue("isIndividual") {
			return false
		}
		return w.individualDefinitions(e) != 1
	}},
	// -- references and conjugation --
	{class: "FeatureReferenceExpression", constraint: "validateFeatureReferenceExpressionReferentIsFeature", violated: func(w *Workspace, e *el) bool {
		for _, m := range w.rels(e, "Membership") {
			if m.IsA("ParameterMembership") {
				continue
			}
			t := w.memberElement(m)
			return t != nil && !t.IsA("Feature")
		}
		return false
	}},
	{class: "Specialization", constraint: "validateSpecializationSpecificNotConjugated", violated: func(w *Workspace, e *el) bool {
		return e.Owner.IsA("Type") && len(w.rels(e.Owner, "Conjugation")) > 0
	}},
	// -- binary associations and connectors --
	{class: "Association", constraint: "validateAssociationBinarySpecialization", at: func(w *Workspace, e *el) []*el {
		var ends []*el
		for _, f := range w.ownedFeatures(e) {
			if f.IsTrue("isEnd") {
				ends = append(ends, f)
			}
		}
		if len(ends) > 2 && w.specializes(e, w.libraryElement("Links::BinaryLink")) {
			return ends[2:]
		}
		return nil
	}},
	{class: "Connector", constraint: "validateConnectorBinarySpecialization", violated: func(w *Workspace, e *el) bool {
		owned := 0
		for _, f := range w.ownedFeatures(e) {
			if f.IsTrue("isEnd") {
				owned++
			}
		}
		return len(w.endFeatures(e)) > 2 && owned <= 2 && w.specializes(e, w.libraryElement("Links::BinaryLink"))
	}},
	{class: "ConnectionDefinition", constraint: "validateConnectionDefinitionEndsAreUsages_", at: nonUsageEnds},
}

// relatedToSelf reports the relationships of a class that point at the element
// itself (checkTargetNotObject).
func relatedToSelf(class, role string) func(w *Workspace, e *el) []*el {
	return func(w *Workspace, e *el) []*el {
		var out []*el
		for _, r := range w.rels(e, class) {
			if w.refTarget(r, role) == e {
				out = append(out, r)
			}
		}
		return out
	}
}

// inputParameterCount is the number of owned features that take input
// (UsageUtil.getOwnedInputParametersOf).
func (w *Workspace) inputParameterCount(e *el) int {
	n := 0
	for _, f := range w.ownedFeatures(e) {
		if d := f.Text("direction"); d == "in" || d == "inout" {
			n++
		}
	}
	return n
}

// generalOf is the general type or feature a specialization points at.
func (w *Workspace) generalOf(r *el) *el {
	for _, role := range []string{"superclassifier", "redefinedFeature", "referencedFeature", "crossedFeature", "subsettedFeature", "type", "general"} {
		if g := w.refTarget(r, role); g != nil {
			return g
		}
	}
	return nil
}

// isNonunique: a feature is unique unless it says nonunique.
func isNonunique(f *el) bool { return f.Has("isUnique") && !f.IsTrue("isUnique") }

// badGenerals reports the owned specializations whose general type is of one
// of the classes.
func badGenerals(classes ...string) func(w *Workspace, e *el) []*el {
	return func(w *Workspace, e *el) []*el {
		var out []*el
		for _, r := range w.rels(e, "Specialization") {
			g := w.generalOf(r)
			for _, c := range classes {
				if g.IsA(c) {
					out = append(out, r)
					break
				}
			}
		}
		return out
	}
}

// variationGenerals reports the owned specializations of a variation that
// specialize another variation.
func variationGenerals(w *Workspace, e *el) []*el {
	var out []*el
	if e.IsTrue("isVariation") {
		for _, r := range w.rels(e, "Specialization") {
			if g := w.generalOf(r); g != nil && isVariation(g) {
				out = append(out, r)
			}
		}
	}
	return out
}

// compositeUsages reports the owned usages other than ports that are composite.
func compositeUsages(w *Workspace, e *el) []*el {
	var out []*el
	for _, f := range w.ownedFeatures(e) {
		if f.IsA("Usage") && !f.IsA("PortUsage") && f.IsTrue("isComposite") {
			out = append(out, f)
		}
	}
	return out
}

// nonUsageEnds is checkEndsAreUsages: the end features of a type that are not
// usages, or the type itself if it inherits them.
func nonUsageEnds(w *Workspace, e *el) []*el {
	var bad, owned []*el
	for _, f := range w.endFeatures(e) {
		if !f.IsA("Usage") {
			bad = append(bad, f)
			if w.featureOwningType(f) == e {
				owned = append(owned, f)
			}
		}
	}
	if len(bad) == 0 {
		return nil
	}
	if len(owned) == 0 {
		return []*el{e}
	}
	return owned
}

// individualDefinitions counts the individual definitions among a usage's types:
// the occurrence definitions in OccurrenceUsage.occurrenceDefinition, which are
// the types the usage has, written, implied or inherited by subsetting.
func (w *Workspace) individualDefinitions(e *el) int {
	n := 0
	for _, t := range w.typesOf(e) {
		if t.IsA("OccurrenceDefinition") && t.IsTrue("isIndividual") {
			n++
		}
	}
	return n
}
