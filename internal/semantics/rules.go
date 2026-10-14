// SPDX-License-Identifier: EPL-2.0
//
// Portions derived from the SysML v2 Pilot Implementation
// (org.omg.sysml.logic adapters, UsageAdapter and util), Copyright (c)
// 2018-2025 Model Driven Solutions, Inc. and others, EPL-2.0. See NOTICE.

package semantics

import (
	"strings"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/schema"
)

// isLibrary reports whether a root is a standard library file.
func (w *Workspace) isLibrary(root *el) bool {
	return strings.HasPrefix(w.files[root], "library:")
}

// Resolve resolves every cross-reference in the files added to the workspace
// (not the standard library, whose references resolve on demand, as names in
// the user's files reach them) and applies the rules that need the resolved
// names. After it, each *model.Ref that could be resolved has its Target set.
func (w *Workspace) Resolve() { w.resolve(false) }

// ResolveAll is Resolve for every file, the standard library included.
func (w *Workspace) ResolveAll() { w.resolve(true) }

func (w *Workspace) resolve(includeLibrary bool) {
	var roots []*el
	for _, root := range w.roots {
		if includeLibrary || !w.isLibrary(root) {
			roots = append(roots, root)
		}
	}
	for _, root := range roots {
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
	for _, root := range roots {
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
	// the tree itself, not w.rels: the memoized list does not see the
	// multiplicity added by an earlier Resolve, which runs this rule again
	for _, m := range u.Children("ownedRelationship") {
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
	if e.IsA("TransitionUsage") {
		w.transitionRules(e)
	}
	if w.wantsDefaultMultiplicity(e) {
		mult := &el{Class: "Multiplicity", Generated: true}
		mem := &el{Class: "OwningMembership", Generated: true}
		mem.Add("ownedRelatedElement", "+=", mult)
		e.Add("ownedRelationship", "+=", mem)
	}
}

// transitionRules follows TransitionUsageAdapter and ReferenceUsageAdapter: a
// transition with a succession gets a link feature, and its payload parameter
// takes the name of the parameter of the action that accepts the trigger.
func (w *Workspace) transitionRules(t *el) {
	var succession *el
	hasLink := false
	var params []*el
	var accepter *el
	for _, r := range w.rels(t, "") {
		switch {
		case r.IsA("ParameterMembership"):
			params = append(params, first(w.ownedRelated(r)))
		case r.IsA("TransitionFeatureMembership"):
			if r.Text("kind") == "trigger" && accepter == nil {
				accepter = first(w.ownedRelated(r))
			}
		case r.IsA("FeatureMembership"):
			hasLink = true
		case r.IsA("OwningMembership") && succession == nil:
			if x := first(w.ownedRelated(r)); x.IsA("SuccessionAsUsage") {
				succession = x
			}
		}
	}
	if succession != nil && !hasLink {
		link := &el{Class: "ReferenceUsage", Generated: true}
		mem := &el{Class: "FeatureMembership", Generated: true}
		mem.Add("ownedRelatedElement", "+=", link)
		t.Add("ownedRelationship", "+=", mem)
	}
	if len(params) >= 2 && accepter != nil && params[1].Text("declaredName") == "" {
		for _, r := range w.rels(accepter, "ParameterMembership") {
			if p := first(w.ownedRelated(r)); p != nil {
				if name := p.Text("declaredName"); name != "" {
					params[1].Set("declaredName", model.Text(name))
				}
				break
			}
		}
	}
}
