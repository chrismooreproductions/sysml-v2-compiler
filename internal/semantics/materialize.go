// SPDX-License-Identifier: EPL-2.0

package semantics

import (
	"strings"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
)

// targetRoles name the property that holds the target of each kind of implied
// relationship.
var targetRoles = map[string]string{
	"Subclassification":   "superclassifier",
	"Subsetting":          "subsettedFeature",
	"Redefinition":        "redefinedFeature",
	"FeatureTyping":       "type",
	"CrossSubsetting":     "crossedFeature",
	"ReferenceSubsetting": "referencedFeature",
}

// Materialize writes the relationships the specification implies into the
// trees under the roots: the implied general types and featuring types of
// every type, and the binding connectors the transformation adds. Each is an
// element marked Generated with isImplied set. The features the pilot makes up
// as their targets (feature chains, the snapshots of a type) are written into
// the relationships that use them. The workspace must have been resolved. It
// does nothing for a root it has already materialized.
func (w *Workspace) Materialize(roots ...*el) {
	for _, root := range roots {
		if w.materialized[root] {
			continue
		}
		w.materialized[root] = true
		// decide everything before changing anything: the changes would not
		// alter the answers, but there is no need to find out
		var types []*el
		root.Walk(func(e *el) {
			if e.IsA("Type") {
				types = append(types, e)
			}
		})
		type plan struct {
			e        *el
			implied  []Implied
			featured []*el
			bindings []Binding
		}
		var plans []plan
		for _, e := range types {
			plans = append(plans, plan{e, w.ImpliedGeneralizations(e), w.ImpliedFeaturingTypes(e), w.ImpliedBindings(e)})
		}
		owned := map[*el]bool{} // generated targets already written into a relationship
		for _, p := range plans {
			for _, im := range p.implied {
				w.addImplied(p.e, im.Kind, targetRoles[im.Kind], im.Target, owned)
			}
			for _, t := range p.featured {
				w.addImplied(p.e, "TypeFeaturing", "featuringType", t, owned)
			}
			for _, b := range p.bindings {
				w.addBinding(p.e, b, owned)
			}
		}
	}
}

// impliedRel makes an implied relationship element.
func impliedRel(class string) *el {
	r := &el{Class: class, Generated: true}
	r.Set("isImplied", model.Bool(true))
	return r
}

// addImplied adds to e an implied relationship of a class pointing at target.
func (w *Workspace) addImplied(e *el, class, role string, target *el, owned map[*el]bool) {
	r := impliedRel(class)
	r.Add(role, "=", &model.Ref{Target: target})
	w.own(r, target, owned)
	e.Add("ownedRelationship", "+=", r)
}

// own writes a feature the pilot made up into the relationship that first uses
// it, with the chaining that defines it.
func (w *Workspace) own(r, target *el, owned map[*el]bool) {
	if !target.Generated || target.Owner != nil || owned[target] {
		return
	}
	owned[target] = true
	r.Add("ownedRelatedElement", "+=", target)
	for owner, snap := range w.snapshots {
		if snap == target {
			w.describeSnapshots(target, owner)
		}
	}
	for _, f := range w.chainOf[target] {
		c := impliedRel("FeatureChaining")
		c.Add("chainingFeature", "=", &model.Ref{Target: f})
		w.own(c, f, owned)
		target.Add("ownedRelationship", "+=", c)
	}
}

// addBinding adds to e an implied binding connector between two features.
func (w *Workspace) addBinding(e *el, b Binding, owned map[*el]bool) {
	bc := &el{Class: "BindingConnector", Generated: true}
	bc.Set("isImplied", model.Bool(true))
	for _, f := range []*el{b.Source, b.Target} {
		end := &el{Class: "Feature", Generated: true}
		end.Set("isEnd", model.Bool(true))
		rs := &el{Class: "ReferenceSubsetting", Generated: true}
		rs.Add("referencedFeature", "=", &model.Ref{Target: f})
		w.own(rs, f, owned)
		end.Add("ownedRelationship", "+=", rs)
		mem := &el{Class: "EndFeatureMembership", Generated: true}
		mem.Add("ownedRelatedElement", "+=", end)
		bc.Add("ownedRelationship", "+=", mem)
	}
	mem := &el{Class: "OwningMembership", Generated: true}
	mem.Add("ownedRelatedElement", "+=", bc)
	e.Add("ownedRelationship", "+=", mem)
}

// describeSnapshots gives the feature made for the snapshots of a type its
// name, <qualified name without the first segment>_snapshots, and what it
// redefines and is featured by.
func (w *Workspace) describeSnapshots(snap, owner *el) {
	name := QualifiedName(owner)
	if i := strings.Index(name, "::"); i >= 0 {
		name = name[i+2:]
	}
	snap.Set("declaredName", model.Text(strings.ReplaceAll(name, "::", "_")+"_snapshots"))
	red := impliedRel("Redefinition")
	red.Add("redefinedFeature", "=", &model.Ref{Target: w.libraryElement("Occurrences::Occurrence::snapshots")})
	snap.Add("ownedRelationship", "+=", red)
	tf := impliedRel("TypeFeaturing")
	tf.Add("featuringType", "=", &model.Ref{Target: owner})
	snap.Add("ownedRelationship", "+=", tf)
}
