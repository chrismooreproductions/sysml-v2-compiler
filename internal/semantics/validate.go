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
	violated   func(w *Workspace, e *el) bool
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
				if e.IsA(c.class) && c.violated(w, e) {
					out = append(out, Issue{e, c.constraint, messageFor(c.constraint), "error"})
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
	return validationMessages["FEATURE_"+sb.String()] // validatePortionNotVariable
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
	return check{class, constraint, func(_ *Workspace, e *el) bool { return !ownerIs(e, classes...) }}
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
	{"Type", "validateTypeAtMostOneConjugator", func(w *Workspace, e *el) bool { return len(w.rels(e, "Conjugation")) > 1 }},
	{"Feature", "validateFeatureEndNoDirection", func(_ *Workspace, e *el) bool { return e.IsTrue("isEnd") && e.Has("direction") }},
	{"Feature", "validateFeatureConstantIsVariable", func(w *Workspace, e *el) bool { return e.IsTrue("isConstant") && !w.isVariable(e) }},
	{"Feature", "validatePortionNotVariable", func(w *Workspace, e *el) bool { return e.IsTrue("isPortion") && w.isVariable(e) }},
	{"Feature", "validateFeatureEndNotDerivedAbstractCompositeOrPortion", func(_ *Workspace, e *el) bool {
		return e.IsTrue("isEnd") && (e.IsTrue("isDerived") || e.IsTrue("isAbstract") || e.IsTrue("isComposite") || e.IsTrue("isPortion"))
	}},
	{"EndFeatureMembership", "validateEndFeatureMembershipIsEnd", func(w *Workspace, e *el) bool {
		f := first(w.ownedRelated(e))
		return f != nil && !f.IsTrue("isEnd")
	}},
	{"FlowEnd", "validateFlowEndIsEnd", func(_ *Workspace, e *el) bool { return !e.IsTrue("isEnd") }},
	{"VariantMembership", "validateVariantMembershipOwningNamespace", func(_ *Workspace, e *el) bool { return !isVariation(e.Owner) }},
	{"Usage", "validateUsageVariationIsAbstract", func(_ *Workspace, e *el) bool { return isVariation(e) && !e.IsTrue("isAbstract") }},
	{"ParameterMembership", "validateParameterMembershipOwningType", func(_ *Workspace, e *el) bool {
		return !e.Owner.IsA("Behavior") && !e.Owner.IsA("Step") && !isConstructorResult(e.Owner) && !e.IsA("ReturnParameterMembership") && !e.IsA("ResultExpressionMembership")
	}},
	{"ReturnParameterMembership", "validateReturnParameterMembershipOwningType", func(_ *Workspace, e *el) bool {
		return !e.Owner.IsA("Function") && !e.Owner.IsA("Expression")
	}},
	{"ResultExpressionMembership", "validateResultExpressionMembershipOwningType", func(_ *Workspace, e *el) bool {
		return !e.Owner.IsA("Function") && !e.Owner.IsA("Expression")
	}},
	notOwnedBy("ActorMembership", "validateActorMembershipOwningType", "RequirementDefinition", "RequirementUsage", "CaseDefinition", "CaseUsage"),
	notOwnedBy("SubjectMembership", "validateSubjectMembershipOwningType", "RequirementDefinition", "RequirementUsage", "CaseDefinition", "CaseUsage"),
	notOwnedBy("ObjectiveMembership", "validateObjectiveMembershipOwningType", "CaseDefinition", "CaseUsage"),
	notOwnedBy("StakeholderMembership", "validateStakeholderMembershipOwningType", "RequirementDefinition", "RequirementUsage"),
	notOwnedBy("TransitionFeatureMembership", "validateTransitionFeatureMembershipOwningType", "TransitionUsage"),
	notOwnedBy("StateSubactionMembership", "validateStateSubactionMembershipOwningType", "StateUsage", "StateDefinition"),
	{"ControlNode", "validateControlNodeOwningType", func(w *Workspace, e *el) bool {
		return !w.owningTypeIs(e, "ActionUsage", "ActionDefinition")
	}},
}
