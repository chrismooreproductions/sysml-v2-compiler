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
					out = append(out, Issue{e, c.constraint, messageFor(c.constraint), "error"})
				}
				if c.at != nil {
					for _, bad := range c.at(w, e) {
						out = append(out, Issue{bad, c.constraint, messageFor(c.constraint), "error"})
					}
				}
			}
		})
	}
	return out
}

// messageFor is the message of a constraint: the pilot names its constant
// after the constraint, in capitals with underscores.
func messageFor(constraint string) string {
	var sb strings.Builder
	for i, r := range strings.TrimPrefix(constraint, "validate") {
		if i > 0 && unicode.IsUpper(r) {
			sb.WriteByte('_')
		}
		sb.WriteRune(unicode.ToUpper(r))
	}
	if m, ok := validationMessages[sb.String()]; ok {
		return m
	}
	for _, prefix := range []string{"FEATURE_", "TYPE_"} { // validatePortionNotVariable, validateDifferencingTypesNotSelf
		if m, ok := validationMessages[prefix+sb.String()]; ok {
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
