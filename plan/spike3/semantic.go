// SPDX-License-Identifier: EPL-2.0
//
// Portions derived from the SysML v2 Pilot Implementation (org.omg.sysml.logic adapters and util),
// Copyright (c) 2018-2025 Model Driven Solutions, Inc. and others, EPL-2.0.
// See NOTICE.

package main

// The semantic layer: structure the pilot implementation adds after parsing
// (org.omg.sysml.logic) that the grammar never states. Each rule names the
// Java it mirrors. Rules here are purely structural; rules that need name
// resolution (implied multiplicity, implied attributes) come later.

// transform returns a copy of the tree with the adapter rules applied
// bottom-up. The input is not modified.
func transform(n *Node, c ctx) *Node {
	out := &Node{Class: n.Class}
	for _, p := range n.Props {
		if child, ok := p.V.(*Node); ok {
			switch p.Name {
			case "ownedRelationship":
				p.V = transform(child, ctx{owner: n, ownerRel: c.rel})
			case "ownedRelatedElement":
				p.V = transform(child, ctx{rel: n, owner: c.owner, ownerRel: c.ownerRel})
			default:
				p.V = transform(child, ctx{owner: n, ownerRel: c.rel})
			}
		}
		out.Props = append(out.Props, p)
	}
	if theMeta.isSub(out.Class, "InvocationExpression") {
		wrapOperands(out)
	}
	if theMeta.isSub(out.Class, "InvocationExpression") || theMeta.isSub(out.Class, "FeatureReferenceExpression") {
		addResultParameter(out)
	}
	if theMeta.isSub(out.Class, "FeatureChainExpression") {
		addChainSourceTarget(out)
	}
	if theMeta.isSub(out.Class, "Usage") && wantsMultiplicity(out, c) {
		addMultiplicity(out)
	}
	return out
}

// addChainSourceTarget mirrors FeatureChainExpressionAdapter.addAdditionalMembers:
// the source parameter gets an anonymous feature for the chain's target to
// hang off.
func addChainSourceTarget(e *Node) {
	for _, p := range e.Props {
		m, ok := p.V.(*Node)
		if p.Name != "ownedRelationship" || !ok || !theMeta.isSub(m.Class, "ParameterMembership") || theMeta.isSub(m.Class, "ReturnParameterMembership") {
			continue
		}
		src := first(m.owned())
		if src == nil || !theMeta.isSub(src.Class, "Feature") {
			return
		}
		for _, q := range src.Props {
			if qm, ok := q.V.(*Node); ok && q.Name == "ownedRelationship" && theMeta.isSub(qm.Class, "FeatureMembership") {
				return // already has an owned feature
			}
		}
		src.Props = append(src.Props, Prop{"ownedRelationship", "+=", &Node{Class: "FeatureMembership", Props: []Prop{
			{"ownedRelatedElement", "+=", &Node{Class: "Feature", Props: []Prop{{"declaredName", "=", Str("")}}}},
		}}})
		return
	}
}

// wantsMultiplicity mirrors UsageAdapter.isAddMultiplicity and its overrides.
// The default-multiplicity clause also asks whether a subsetted feature has
// an owning type, which needs name resolution; any owned subsetting is
// treated here as "yes, it does", so this under-adds for usages that subset
// a feature with no owner.
func wantsMultiplicity(u *Node, c ctx) bool {
	for _, p := range u.Props {
		if m, ok := p.V.(*Node); ok && p.Name == "ownedRelationship" {
			if member := first(m.owned()); member != nil && theMeta.isSub(member.Class, "Multiplicity") {
				return false // TypeUtil.addMultiplicityTo: already has one
			}
		}
	}
	isEnd := u.str("isEnd") == "true"
	if theMeta.isSub(u.Class, "ConnectionUsage") {
		return isEnd
	}
	if theMeta.isSub(u.Class, "AttributeUsage") || theMeta.isSub(u.Class, "ItemUsage") || theMeta.isSub(u.Class, "PortUsage") {
		if isEnd {
			return true
		}
		if !isA(c.rel, "FeatureMembership") || !isA(c.owner, "Type") {
			return false // no owning type
		}
		for _, p := range u.Props {
			if m, ok := p.V.(*Node); ok && p.Name == "ownedRelationship" && theMeta.isSub(m.Class, "Subsetting") {
				return false
			}
		}
		return true
	}
	return isEnd
}

// addMultiplicity mirrors TypeUtil.addMultiplicityTo.
func addMultiplicity(u *Node) {
	u.Props = append(u.Props, Prop{"ownedRelationship", "+=", &Node{Class: "OwningMembership", Props: []Prop{
		{"ownedRelatedElement", "+=", &Node{Class: "Multiplicity"}},
	}}})
}

// wrapOperands mirrors OperandEList.addUnique -> TypeUtil.addOwnedParameterTo:
// the grammar assigns the derived `operand` feature, but an operand is really
// held as the value of an `in` parameter of the expression.
func wrapOperands(e *Node) {
	var props []Prop
	for _, p := range e.Props {
		op, ok := p.V.(*Node)
		if p.Name != "operand" || !ok {
			props = append(props, p)
			continue
		}
		param := &Node{Class: "Feature", Props: []Prop{
			{"direction", "=", Str("in")},
			{"ownedRelationship", "+=", &Node{Class: "FeatureValue", Props: []Prop{
				{"ownedRelatedElement", "+=", op},
			}}},
		}}
		props = append(props, Prop{"ownedRelationship", "+=", &Node{Class: "ParameterMembership", Props: []Prop{
			{"visibility", "=", Str("private")},
			{"ownedRelatedElement", "+=", param},
		}}})
	}
	e.Props = props
}

// addResultParameter mirrors TypeUtil.addResultParameterTo, called from
// InvocationExpressionAdapter and FeatureReferenceExpressionAdapter: an
// expression always has a result parameter.
func addResultParameter(e *Node) {
	for _, p := range e.Props {
		if m, ok := p.V.(*Node); ok && p.Name == "ownedRelationship" && theMeta.isSub(m.Class, "ReturnParameterMembership") {
			return
		}
	}
	e.Props = append(e.Props, Prop{"ownedRelationship", "+=", &Node{Class: "ReturnParameterMembership", Props: []Prop{
		{"ownedRelatedElement", "+=", &Node{Class: "Feature", Props: []Prop{{"direction", "=", Str("out")}}}},
	}}})
}
