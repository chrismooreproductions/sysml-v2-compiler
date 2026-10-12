// SPDX-License-Identifier: EPL-2.0
//
// Portions derived from the SysML v2 Pilot Implementation
// (org.omg.sysml.logic adapters and util), Copyright (c) 2018-2025 Model
// Driven Solutions, Inc. and others, EPL-2.0. See NOTICE.

// Package semantics adds the structure the specification implies but the
// syntax does not write. This file holds the rules that need no name
// resolution: operands held as parameters, result parameters, the anonymous
// target feature of a feature chain, and the default multiplicity. Each names
// the Java it follows (the adapters in org.omg.sysml.logic).
package semantics

import (
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
)

// Structure applies the structural rules to a post-processed model, in place.
// Children are handled before their parents, so an expression's operands are
// already in their final form when the expression is.
func Structure(root *model.Element) {
	var post func(e *model.Element)
	post = func(e *model.Element) {
		for _, p := range append([]model.Prop{}, e.Props...) {
			if c, ok := p.Value.(*model.Element); ok {
				post(c)
			}
		}
		apply(e)
	}
	post(root)
}

func apply(e *model.Element) {
	if e.IsA("InvocationExpression") {
		wrapOperands(e)
	}
	if e.IsA("InvocationExpression") || e.IsA("FeatureReferenceExpression") {
		addResultParameter(e)
	}
	if e.IsA("FeatureChainExpression") {
		addChainSourceTarget(e)
	}
	if e.IsA("Usage") && wantsMultiplicity(e) {
		addMultiplicity(e)
	}
}

func generated(class string, props ...model.Prop) *model.Element {
	e := &model.Element{Class: class, Generated: true}
	for _, p := range props {
		e.Add(p.Name, p.Op, p.Value)
	}
	return e
}

func prop(name, op string, v model.Value) model.Prop { return model.Prop{Name: name, Op: op, Value: v} }

// wrapOperands follows OperandEList.addUnique and TypeUtil.addOwnedParameterTo:
// the grammar assigns the derived `operand` feature, but an operand is really
// held as the value of an `in` parameter of the expression.
func wrapOperands(e *model.Element) {
	var props []model.Prop
	for _, p := range e.Props {
		operand, ok := p.Value.(*model.Element)
		if p.Name != "operand" || !ok {
			props = append(props, p)
			continue
		}
		value := generated("FeatureValue", prop("ownedRelatedElement", "+=", operand))
		param := generated("Feature", prop("direction", "=", model.Text("in")), prop("ownedRelationship", "+=", value))
		membership := generated("ParameterMembership", prop("visibility", "=", model.Text("private")), prop("ownedRelatedElement", "+=", param))
		membership.Owner = e
		props = append(props, prop("ownedRelationship", "+=", membership))
	}
	e.Props = props
}

// addResultParameter follows TypeUtil.addResultParameterTo, called from
// InvocationExpressionAdapter and FeatureReferenceExpressionAdapter: an
// expression always has a result parameter.
func addResultParameter(e *model.Element) {
	for _, m := range e.Children("ownedRelationship") {
		if m.IsA("ReturnParameterMembership") {
			return
		}
	}
	result := generated("Feature", prop("direction", "=", model.Text("out")))
	e.Add("ownedRelationship", "+=", generated("ReturnParameterMembership", prop("ownedRelatedElement", "+=", result)))
}

// addChainSourceTarget follows FeatureChainExpressionAdapter.addAdditionalMembers:
// the source parameter gets an anonymous feature for the chain's target to
// hang off.
func addChainSourceTarget(e *model.Element) {
	for _, m := range e.Children("ownedRelationship") {
		if !m.IsA("ParameterMembership") || m.IsA("ReturnParameterMembership") {
			continue
		}
		source := m.Child("ownedRelatedElement")
		if source == nil || !source.IsA("Feature") {
			return
		}
		for _, q := range source.Children("ownedRelationship") {
			if q.IsA("FeatureMembership") {
				return // it already owns a feature
			}
		}
		target := generated("Feature", prop("declaredName", "=", model.Text("")))
		source.Add("ownedRelationship", "+=", generated("FeatureMembership", prop("ownedRelatedElement", "+=", target)))
		return
	}
}

// wantsMultiplicity follows UsageAdapter.isAddMultiplicity and its overrides.
// The default-multiplicity clause also asks whether a subsetted feature has an
// owning type, which needs name resolution; any owned subsetting is treated
// here as "yes, it does", so this under-adds for a usage that subsets a feature
// with no owner. The resolution pass completes that case.
func wantsMultiplicity(u *model.Element) bool {
	for _, m := range u.Children("ownedRelationship") {
		if member := m.Child("ownedRelatedElement"); member.IsA("Multiplicity") {
			return false // TypeUtil.addMultiplicityTo: it already has one
		}
	}
	isEnd := u.IsTrue("isEnd")
	switch {
	case u.IsA("ConnectionUsage"):
		return isEnd
	case u.IsA("AttributeUsage"), u.IsA("ItemUsage"), u.IsA("PortUsage"):
		if isEnd {
			return true
		}
		if !u.Owner.IsA("FeatureMembership") || !u.Owner.Owner.IsA("Type") {
			return false // no owning type
		}
		for _, m := range u.Children("ownedRelationship") {
			if m.IsA("Subsetting") {
				return false
			}
		}
		return true
	}
	return isEnd
}

// addMultiplicity follows TypeUtil.addMultiplicityTo.
func addMultiplicity(u *model.Element) {
	u.Add("ownedRelationship", "+=", generated("OwningMembership",
		prop("ownedRelatedElement", "+=", generated("Multiplicity"))))
}
