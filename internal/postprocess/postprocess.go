// SPDX-License-Identifier: EPL-2.0
//
// Portions derived from the SysML v2 Pilot Implementation
// (org.omg.kerml.xtext and org.omg.sysml.xtext, package postprocessing),
// Copyright (c) 2018-2025 Model Driven Solutions, Inc. and others, EPL-2.0.
// See NOTICE.

// Package postprocess applies the rules the pilot implementation runs on each
// element right after parsing, which fill in what the grammar leaves implicit:
// relationship ends that the syntax gives as a feature chain or as the owner,
// the isEnd flag of an end feature, the direction of a parameter, the default
// of isComposite, and so on.
//
// The rules run on every element, parents before children, so a rule that sets
// a property of a child (a parameter's direction) is seen by the child's own
// rules. Each rule names the Java post-processor it follows.
package postprocess

import (
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/schema"
)

// Run applies the post-processing rules to a model, in place.
func Run(root *model.Element) {
	root.Walk(process)
}

func process(e *model.Element) {
	// A cross-reference the grammar writes under a redefined feature's name
	// (specific, general, ...) is held under the feature that redefines it.
	for i, p := range e.Props {
		if _, ok := p.Value.(*model.Ref); ok {
			e.Props[i].Name = schema.EffectiveRole(e.Class, p.Name)
		}
	}
	text(e)
	if e.IsA("Relationship") {
		relationship(e)
	}
	if e.IsA("Membership") {
		membership(e)
	}
	if e.IsA("Expose") && !e.Has("isImportAll") {
		// An expose has no syntax for `all`; MembershipExposeImpl and
		// NamespaceExposeImpl default isImportAll to true.
		e.Set("isImportAll", model.Bool(true))
	}
	if e.IsA("Feature") && !e.IsA("Usage") && e.IsTrue("isConstant") {
		e.Set("isVariable", model.Bool(true)) // FeatureParserPostProcessor
	}
	if e.IsA("Definition") {
		definition(e)
	}
	if e.IsA("Usage") {
		usage(e)
	}
}

// ref records a cross-reference to an element, unless the grammar already gave
// the property a reference.
func ref(e *model.Element, role string, target *model.Element) {
	role = schema.EffectiveRole(e.Class, role)
	if target == nil || e.Has(role) {
		return
	}
	e.Add(role, "=", &model.Ref{Target: target})
}

func first(l []*model.Element) *model.Element {
	if len(l) == 0 {
		return nil
	}
	return l[0]
}

func last(l []*model.Element) *model.Element {
	if len(l) == 0 {
		return nil
	}
	return l[len(l)-1]
}

// owningRelatedElement is the element that owns a relationship: the element
// whose ownedRelationship it is, or the relationship whose ownedRelatedElement
// it is.
func owningRelatedElement(r *model.Element) *model.Element { return r.Owner }

// sourceOrFirst sets the source end of a relationship: the owner if it is a
// kind of ownerClass, otherwise the first owned element (a feature chain).
func sourceOrFirst(r *model.Element, role, ownerClass string, owned []*model.Element) {
	if r.Has(schema.EffectiveRole(r.Class, role)) {
		return
	}
	if o := owningRelatedElement(r); o.IsA(ownerClass) {
		ref(r, role, o)
	} else {
		ref(r, role, first(owned))
	}
}

// relationship applies the Relationship post-processors:
// Specialization, Subsetting, Redefinition, ReferenceSubsetting, FeatureTyping,
// Conjugation, Disjoining, Differencing, Intersecting, Unioning,
// FeatureInverting, TypeFeaturing, Subclassification, Annotation, Dependency,
// NamespaceImport and Import ParserPostProcessor.
func relationship(r *model.Element) {
	owned := r.Children("ownedRelatedElement")
	is := r.IsA

	if is("Specialization") {
		ref(r, "general", last(owned))
		sourceOrFirst(r, "specific", "Type", owned)
	}
	if is("Subclassification") && !r.Has("subclassifier") && owningRelatedElement(r).IsA("Classifier") {
		ref(r, "subclassifier", owningRelatedElement(r))
	}
	if is("Subsetting") {
		ref(r, "subsettedFeature", last(owned))
		sourceOrFirst(r, "subsettingFeature", "Feature", owned)
	}
	if is("Redefinition") {
		sourceOrFirst(r, "redefiningFeature", "Feature", owned)
		ref(r, "redefinedFeature", last(owned))
	}
	if is("ReferenceSubsetting") {
		ref(r, "referencedFeature", last(owned))
	}
	if is("FeatureTyping") {
		ref(r, "type", first(owned))
		if o := owningRelatedElement(r); o.IsA("Feature") {
			ref(r, "typedFeature", o)
		}
	}
	if is("Conjugation") {
		sourceOrFirst(r, "conjugatedType", "Type", owned)
		ref(r, "originalType", last(owned))
	}
	if is("Disjoining") {
		ref(r, "disjoiningType", last(owned))
		sourceOrFirst(r, "typeDisjoined", "Type", owned)
	}
	if is("Differencing") {
		ref(r, "differencingType", last(owned))
	}
	if is("Intersecting") {
		ref(r, "intersectingType", last(owned))
	}
	if is("Unioning") {
		ref(r, "unioningType", last(owned))
	}
	if is("FeatureInverting") {
		sourceOrFirst(r, "featureInverted", "Feature", owned)
		ref(r, "invertingFeature", last(owned))
	}
	if is("TypeFeaturing") {
		if o := owningRelatedElement(r); o.IsA("Feature") {
			ref(r, "featureOfType", o)
		}
	}
	// importedNamespace and annotatedElement are target ends, so a reference to
	// the owning element is a real reference.
	if is("NamespaceImport") && !r.Has("importedNamespace") {
		if f := first(owned); f.IsA("Namespace") {
			ref(r, "importedNamespace", f)
		} else {
			ref(r, "importedNamespace", owningRelatedElement(r))
		}
	}
	if is("Annotation") && !r.Has("annotatedElement") && !owningRelatedElement(r).IsA("AnnotatingElement") {
		ref(r, "annotatedElement", owningRelatedElement(r))
	}
	if is("Dependency") {
		for _, o := range owned {
			r.Add("supplier", "+=", &model.Ref{Target: o})
		}
	}
	// PortConjugationParserPostProcessor: a port definition has an implicit
	// conjugated definition; the conjugation's original is the port definition
	// that owns it.
	if is("PortConjugation") && !r.Has("originalPortDefinition") {
		if conjugated := owningRelatedElement(r); conjugated.IsA("ConjugatedPortDefinition") && conjugated.Owner != nil {
			if def := conjugated.Owner.Owner; def.IsA("PortDefinition") {
				ref(r, "originalPortDefinition", def)
			}
		}
	}
	// An import inside a filter package is public (ImportParserPostProcessor).
	if is("Import") {
		if ns := owningRelatedElement(r); ns != nil && ns.Owner.IsA("NamespaceImport") {
			r.Set("visibility", model.Text("public"))
		}
	}
}

// membership applies the post-processors that set properties of the element a
// membership holds: EndFeatureMembership (isEnd) and ParameterMembership
// (direction, in for a parameter and out for a return parameter).
func membership(m *model.Element) {
	member := first(m.Children("ownedRelatedElement"))
	if member == nil || !member.IsA("Feature") {
		return
	}
	if m.IsA("EndFeatureMembership") {
		member.Set("isEnd", model.Bool(true))
	}
	if m.IsA("ParameterMembership") {
		if m.IsA("ReturnParameterMembership") {
			member.Set("direction", model.Text("out"))
		} else {
			member.Set("direction", model.Text("in"))
		}
	}
}

// definition applies DefinitionParserPostProcessor: a variation is abstract. An
// enumeration definition is always a variation (a Java override).
func definition(d *model.Element) {
	if d.IsTrue("isVariation") || d.IsA("EnumerationDefinition") {
		d.Set("isAbstract", model.Bool(true))
	}
}

// nonCompositeUsages are the usage kinds that are not composite by default.
var nonCompositeUsages = []string{"AttributeUsage", "BindingConnectorAsUsage", "EventOccurrenceUsage",
	"ExhibitStateUsage", "IncludeUseCaseUsage", "PerformActionUsage", "ReferenceUsage", "SuccessionAsUsage"}

// expectedFeaturingType mirrors UsageUtil.getExpectedFeaturingTypeOf: the type
// owning the feature membership that holds the usage.
func expectedFeaturingType(u *model.Element) *model.Element {
	if u.Owner.IsA("FeatureMembership") {
		return u.Owner.Owner
	}
	return nil
}

// usage applies UsageParserPostProcessor, OccurrenceUsageParserPostProcessor
// and PortUsageParserPostProcessor.
func usage(u *model.Element) {
	// isComposite defaults from the usage kind unless the source said either
	// `ref` (isReference) or isComposite explicitly.
	if !u.Has("isComposite") && !u.Has("isReference") {
		composite := true
		for _, k := range nonCompositeUsages {
			if u.IsA(k) {
				composite = false
			}
		}
		if composite {
			u.Set("isComposite", model.Bool(true))
		}
	}
	if u.IsTrue("isVariation") {
		u.Set("isAbstract", model.Bool(true))
	}
	ft := expectedFeaturingType(u)
	if u.Has("direction") || u.IsTrue("isEnd") || ft == nil || ft.IsA("AttributeDefinition") || ft.IsA("AttributeUsage") {
		u.Set("isComposite", model.Bool(false))
	}
	if u.IsA("OccurrenceUsage") && u.Has("portionKind") {
		u.Set("isPortion", model.Bool(true))
	}
	if u.IsA("PortUsage") && !(ft.IsA("PortDefinition") || ft.IsA("PortUsage")) {
		u.Set("isComposite", model.Bool(false))
	}
}
