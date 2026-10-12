// SPDX-License-Identifier: EPL-2.0
//
// Portions derived from the SysML v2 Pilot Implementation
// (org.omg.sysml.logic adapters, UsageAdapter and util), Copyright (c)
// 2018-2025 Model Driven Solutions, Inc. and others, EPL-2.0. See NOTICE.

package semantics

import (
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/schema"
)

// Resolve resolves every cross-reference in the workspace and applies the
// rules that need the resolved names. After it, each *model.Ref that could be
// resolved has its Target set.
func (w *Workspace) Resolve() {
	for _, root := range w.roots {
		root.Walk(func(e *el) {
			for _, p := range e.Props {
				if r, ok := p.Value.(*model.Ref); ok && r.Target == nil && r.Name != "" {
					if t := w.target(e, p.Name, r); t != nil {
						r.Target = t
					}
				}
			}
		})
	}
	for _, root := range w.roots {
		root.Walk(w.applyResolutionRules)
	}
}

// mayTimeVary follows UsageAdapter.mayTimeVary.
func (w *Workspace) mayTimeVary(u *el) bool {
	owningType := w.featureOwningType(u)
	if owningType == nil || !w.specializes(owningType, w.libraryElement("Occurrences::Occurrence")) {
		return false
	}
	return !(u.IsTrue("isPortion") ||
		w.specializes(u, w.libraryElement("Links::SelfLink")) ||
		w.specializes(u, w.libraryElement("Occurrences::HappensLink")) ||
		u.IsTrue("isComposite") && w.specializes(u, w.libraryElement("Actions::Action")))
}

func (w *Workspace) owningType(f *el) *el { return w.featureOwningType(f) }

// wantsDefaultMultiplicity completes UsageAdapter.isAddDefaultMultiplicity, the
// clause Structure could only approximate: an attribute, item or port usage
// owned by a type gets a default multiplicity unless a feature it subsets is
// itself owned by a type.
func (w *Workspace) wantsDefaultMultiplicity(u *el) bool {
	if !(u.IsA("AttributeUsage") || u.IsA("ItemUsage") || u.IsA("PortUsage")) || u.IsA("ConnectionUsage") {
		return false
	}
	if u.IsTrue("isEnd") || w.owningType(u) == nil {
		return false
	}
	for _, m := range w.rels(u, "") {
		if first(w.ownedRelated(m)).IsA("Multiplicity") {
			return false
		}
	}
	subs := w.rels(u, "Subsetting")
	if len(subs) == 0 {
		return false // Structure already added it
	}
	for _, s := range subs {
		for _, r := range w.targetRefsOf(s) {
			if f := w.basicFeature(w.target(s, r.role, r.ref)); f != nil && w.owningType(f) != nil {
				return false
			}
		}
	}
	return true
}

type roleRef struct {
	role string
	ref  *model.Ref
}

func (w *Workspace) targetRefsOf(rel *el) []roleRef {
	var out []roleRef
	for _, p := range rel.Props {
		if r, ok := p.Value.(*model.Ref); ok && schema.IsTargetEnd(rel.Class, p.Name) {
			out = append(out, roleRef{p.Name, r})
		}
	}
	return out
}

// applyResolutionRules applies the adapter rules that need name resolution:
// UsageAdapter.doTransform makes an end usage constant if it may time-vary, and
// the default-multiplicity clause that asks about subsetted features.
func (w *Workspace) applyResolutionRules(e *el) {
	if e.IsA("Usage") && e.IsTrue("isEnd") && w.mayTimeVary(e) {
		e.Set("isConstant", model.Bool(true))
	}
	if w.wantsDefaultMultiplicity(e) {
		mult := &el{Class: "Multiplicity", Generated: true}
		mem := &el{Class: "OwningMembership", Generated: true}
		mem.Add("ownedRelatedElement", "+=", mult)
		e.Add("ownedRelationship", "+=", mem)
	}
}
