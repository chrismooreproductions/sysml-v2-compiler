// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"strings"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/schema"
)

// MembershipKind says how a namespace holds one of its members.
type MembershipKind string

const (
	// MemberOwning is a member the namespace owns through a plain
	// OwningMembership: a package's element, a definition in a package.
	MemberOwning MembershipKind = "owning"
	// MemberFeature is a feature the type owns through a FeatureMembership (or
	// EndFeatureMembership): a composite or referential feature of the type.
	MemberFeature MembershipKind = "feature"
	// MemberParameter is a parameter, return parameter, subject, actor or
	// similar role, held through a ParameterMembership.
	MemberParameter MembershipKind = "parameter"
	// MemberImport is an element or namespace the namespace imports, which it
	// does not own.
	MemberImport MembershipKind = "import"
	// MemberAlias is an element another name refers to (alias X for Y), which
	// the namespace does not own.
	MemberAlias MembershipKind = "alias"
)

// A Member is one entry in a namespace's list of members, in source order.
type Member struct {
	// Element is the member: the owned element, or for an import or alias the
	// element it brings in.
	Element *Element
	// Kind says how the namespace holds Element.
	Kind MembershipKind
	// Relationship is the Membership or Import element that holds it, whose
	// Class is the precise metaclass (EndFeatureMembership, ...).
	Relationship *Element
	// Name is the name an alias gives Element; it is empty otherwise.
	Name string
	// Visibility is "public", "private" or "protected".
	Visibility string
}

// A Generalization is one written or implied relationship from an element to a
// more general one.
type Generalization struct {
	// Kind is the metaclass of the relationship: "FeatureTyping",
	// "Subsetting", "Redefinition", "ReferenceSubsetting", "CrossSubsetting",
	// "Subclassification", "ConjugatedPortTyping", "Conjugation" or
	// "PortConjugation".
	Kind string
	// Target is the more general element.
	Target *Element
	// Implied is true when the specification adds the relationship and the
	// source does not write it.
	Implied bool
}

// A Multiplicity is the resolved bounds of a feature's multiplicity.
type Multiplicity struct {
	// Lower and Upper are the bounds; Upper is -1 for * (unbounded). Both are
	// zero when Resolved is false.
	Lower, Upper int
	// Resolved is true when both bounds are computable. A bound such as [n]
	// whose value the evaluator cannot compute leaves it false, and Source
	// holds the expression.
	Resolved bool
	// Source is the MultiplicityRange element the bounds came from.
	Source *Element
}

// Facts are the resolved, ready-to-use properties of one element, derived
// from the generic tree so that a consumer does not need to know how the
// metamodel encodes them. A field that does not apply to the element's
// metaclass, or that the source does not give, is nil, empty or false.
type Facts struct {
	// Element is the element the facts describe.
	Element *Element

	// Class is the metaclass name, such as "PartUsage".
	Class string
	// Name is the effective name: the declared name, or for a feature without
	// one the name of the feature it redefines.
	Name string
	// ShortName is the declared short name (the alias in <angle brackets>).
	ShortName string
	// QualifiedName is as the function QualifiedName gives it; it is empty
	// for an element with no named owner.
	QualifiedName string
	// IsLibrary is true for an element of the standard library.
	IsLibrary bool

	// Owner is the namespace that owns the element, nil for a file's root.
	Owner *Element
	// Membership says how Owner holds the element; empty for a root.
	Membership MembershipKind
	// Members are the entries of the element's namespace in source order:
	// what it owns, its imports and its aliases. Elements the transformation
	// generates, the multiplicity (see Multiplicity) and the feature value
	// expression, are left out.
	Members []Member

	// Direction is "in", "out" or "inout" for a directed feature and empty
	// otherwise. A feature that does not state one takes the direction of the
	// feature it redefines. A feature of a conjugated port is seen the other
	// way round: see Conjugated.
	Direction string
	// IsComposite is the effective value: the written keyword, or false.
	IsComposite bool
	// IsReference is true for a referential usage: one written ref, or one
	// that is not composite. It is false for a feature that is not a usage.
	IsReference bool
	// IsEnd is true for an end feature of a connector or association.
	IsEnd bool
	// IsPortion, IsConstant and IsDerived are the feature's flags.
	IsPortion, IsConstant, IsDerived bool
	// IsOrdered and IsUnique are the feature's ordering and uniqueness; a
	// feature is unique unless it is written nonunique.
	IsOrdered, IsUnique bool
	// IsAbstract and IsVariation are the type's flags. A variation is
	// abstract.
	IsAbstract, IsVariation bool
	// Conjugated is true for a feature typed by a conjugated port
	// (p : ~Fuel), whose directed features all face the other way.
	Conjugated bool

	// Multiplicity is the feature's own multiplicity, nil when the source
	// writes none. No default is substituted.
	Multiplicity *Multiplicity

	// Types are the feature's typings, written then implied.
	Types []Generalization
	// Subsets are the Subsetting relationships (and CrossSubsetting),
	// written then implied; reference subsetting is separate.
	Subsets []Generalization
	// ReferenceSubsets are the ReferenceSubsetting relationships, such as
	// the feature an end of a connector points to.
	ReferenceSubsets []Generalization
	// Redefines are the Redefinition relationships.
	Redefines []Generalization
	// Specializes are the Subclassification relationships of a classifier.
	Specializes []Generalization
	// Conjugates is the type a conjugated type is the conjugate of: from a
	// Conjugation, or for a ConjugatedPortDefinition its port definition.
	Conjugates []Generalization
}

// Facts returns the facts about an element. Call it after Resolve; before
// that the relationships have no targets and are left out.
func (w *Workspace) Facts(e *Element) Facts {
	if e == nil {
		return Facts{}
	}
	f := Facts{
		Element:       e,
		Class:         e.Class,
		Name:          w.EffectiveName(e),
		QualifiedName: QualifiedName(e),
		IsLibrary:     strings.HasPrefix(w.FileOf(e), "library:"),
	}
	f.ShortName = e.Text("declaredShortName")
	if f.ShortName == "" && e.Text("declaredName") == "" {
		f.ShortName = w.effectiveShortName(e)
	}
	if m := e.Owner; m.IsA("Membership") {
		f.Owner = m.Owner
		f.Membership = membershipKind(m)
	}
	f.Members = w.membersOf(e)

	f.IsComposite = flag(e, "isComposite")
	f.IsEnd = flag(e, "isEnd")
	f.IsPortion = flag(e, "isPortion")
	f.IsConstant = flag(e, "isConstant")
	f.IsDerived = flag(e, "isDerived")
	f.IsOrdered = flag(e, "isOrdered")
	f.IsUnique = flag(e, "isUnique")
	f.IsVariation = flag(e, "isVariation")
	f.IsAbstract = flag(e, "isAbstract") || f.IsVariation
	if e.IsA("Usage") {
		f.IsReference = flag(e, "isReference") || !f.IsComposite
	}
	f.Direction = w.directionOf(e, map[*Element]bool{})
	f.Multiplicity = w.multiplicityOf(e)
	w.generalizations(&f)
	return f
}

// flag is a boolean property: as written, or else the metamodel's default.
func flag(e *Element, name string) bool {
	if e.Has(name) {
		return e.IsTrue(name)
	}
	if p, _, ok := schema.FeatureOf(e.Class, name); ok {
		return p.Default == "true"
	}
	return false
}

// effectiveShortName is the short name of a feature that has no name of its own
// and so takes the one of the feature it redefines.
func (w *Workspace) effectiveShortName(e *Element) string {
	seen := map[*Element]bool{}
	for f := e; f != nil && !seen[f]; {
		seen[f] = true
		if s := f.Text("declaredShortName"); s != "" {
			return s
		}
		if f.Text("declaredName") != "" {
			return ""
		}
		f = first(relationTargets(f, "Redefinition"))
	}
	return ""
}

func first(l []*Element) *Element {
	if len(l) == 0 {
		return nil
	}
	return l[0]
}

// ownedRels are the relationships an element owns in the source (not those
// Materialize writes in).
func ownedRels(e *Element) []*Element {
	var out []*Element
	for _, r := range e.Children("ownedRelationship") {
		if !r.IsTrue("isImplied") {
			out = append(out, r)
		}
	}
	return out
}

// targetOf is the target of a relationship's target end, if it resolved.
func targetOf(rel *Element) *Element {
	for _, p := range rel.Props {
		if r, ok := p.Value.(*model.Ref); ok && schema.IsTargetEnd(rel.Class, p.Name) {
			return r.Target
		}
	}
	return nil
}

// relationTargets are the targets of the owned relationships of a class.
func relationTargets(e *Element, class string) []*Element {
	var out []*Element
	for _, r := range ownedRels(e) {
		if r.IsA(class) {
			if t := targetOf(r); t != nil {
				out = append(out, t)
			}
		}
	}
	return out
}

func membershipKind(m *Element) MembershipKind {
	switch {
	case m.IsA("ParameterMembership"):
		return MemberParameter
	case m.IsA("FeatureMembership"):
		return MemberFeature
	}
	return MemberOwning
}

func (w *Workspace) membersOf(ns *Element) []Member {
	var out []Member
	for _, r := range ownedRels(ns) {
		switch {
		case r.IsA("Import"):
			if t := importedElement(r); t != nil {
				out = append(out, Member{t, MemberImport, r, "", visibility(r)})
			}
		case r.IsA("Membership"):
			owned := r.Children("ownedRelatedElement")
			if len(owned) == 0 {
				for _, p := range r.Props {
					if ref, ok := p.Value.(*model.Ref); ok && p.Name == "memberElement" && ref.Target != nil {
						out = append(out, Member{ref.Target, MemberAlias, r, r.Text("memberName"), visibility(r)})
					}
				}
				continue
			}
			if r.IsA("FeatureValue") {
				continue
			}
			for _, m := range owned {
				if m.Generated || m.IsA("Multiplicity") {
					continue
				}
				out = append(out, Member{m, membershipKind(r), r, "", visibility(r)})
			}
		}
	}
	return out
}

// importedElement is what an import brings in: the namespace or element it
// names, whichever the import is.
func importedElement(imp *Element) *Element {
	for _, p := range imp.Props {
		r, ok := p.Value.(*model.Ref)
		if !ok || r.Target == nil || (p.Name != "importedNamespace" && p.Name != "importedMembership") {
			continue
		}
		t := r.Target
		if t.IsA("Membership") { // a membership import names the membership
			if o := t.Children("ownedRelatedElement"); len(o) > 0 {
				return o[0]
			}
			for _, q := range t.Props {
				if mr, ok := q.Value.(*model.Ref); ok && q.Name == "memberElement" {
					return mr.Target
				}
			}
		}
		return t
	}
	return nil
}

func visibility(rel *Element) string {
	if v := rel.Text("visibility"); v != "" {
		return v
	}
	if p, _, ok := schema.FeatureOf(rel.Class, "visibility"); ok {
		return p.Default
	}
	return ""
}

// directionOf is the direction a feature states, or else that of the feature
// it redefines.
func (w *Workspace) directionOf(e *Element, seen map[*Element]bool) string {
	if d := e.Text("direction"); d != "" {
		return d
	}
	if seen[e] {
		return ""
	}
	seen[e] = true
	for _, r := range relationTargets(e, "Redefinition") {
		if d := w.directionOf(r, seen); d != "" {
			return d
		}
	}
	return ""
}

// multiplicityOf reads the bounds of the multiplicity range a feature writes.
func (w *Workspace) multiplicityOf(e *Element) *Multiplicity {
	for _, r := range ownedRels(e) {
		if !r.IsA("OwningMembership") {
			continue
		}
		for _, m := range r.Children("ownedRelatedElement") {
			if m.Generated || !m.IsA("MultiplicityRange") {
				continue
			}
			var bounds []*Element
			for _, br := range m.Children("ownedRelationship") {
				bounds = append(bounds, br.Children("ownedRelatedElement")...)
			}
			out := &Multiplicity{Source: m}
			switch len(bounds) {
			case 1: // [n] is n..n, and [*] is 0..*
				lo, ok1 := w.bound(bounds[0])
				if lo == -1 {
					lo = 0
				}
				hi, ok2 := w.bound(bounds[0])
				out.Lower, out.Upper, out.Resolved = lo, hi, ok1 && ok2
			case 2:
				lo, ok1 := w.bound(bounds[0])
				hi, ok2 := w.bound(bounds[1])
				out.Lower, out.Upper, out.Resolved = lo, hi, ok1 && ok2
			default:
				return out
			}
			if !out.Resolved {
				out.Lower, out.Upper = 0, 0
			}
			return out
		}
	}
	return nil
}

// bound evaluates one bound of a range; -1 is *.
func (w *Workspace) bound(e *Element) (int, bool) {
	if e.IsA("LiteralInfinity") {
		return -1, true
	}
	v, err := w.Evaluate(e)
	if err != nil {
		return 0, false
	}
	if n, ok := v.(int64); ok {
		return int(n), true
	}
	return 0, false
}

// generalizations fills the relationship lists: those the source writes, in
// order, then those the specification implies.
func (w *Workspace) generalizations(f *Facts) {
	e := f.Element
	add := func(kind string, target *Element, implied bool) {
		g := Generalization{kind, target, implied}
		switch kind {
		case "FeatureTyping", "ConjugatedPortTyping":
			f.Types = append(f.Types, g)
		case "Redefinition":
			f.Redefines = append(f.Redefines, g)
		case "ReferenceSubsetting":
			f.ReferenceSubsets = append(f.ReferenceSubsets, g)
		case "Subsetting", "CrossSubsetting":
			f.Subsets = append(f.Subsets, g)
		case "Subclassification", "Specialization":
			f.Specializes = append(f.Specializes, g)
		case "Conjugation", "PortConjugation":
			f.Conjugates = append(f.Conjugates, g)
		}
	}
	for _, r := range ownedRels(e) {
		if !r.IsA("Specialization") && !r.IsA("Conjugation") {
			continue
		}
		if r.IsA("ConjugatedPortTyping") {
			f.Conjugated = true // even when the conjugated port does not resolve
		}
		t := targetOf(r)
		if r.IsA("PortConjugation") && t == nil && e.Owner.IsA("Membership") {
			t = e.Owner.Owner // the port definition a ConjugatedPortDefinition belongs to
		}
		if t != nil {
			add(r.Class, t, false)
		}
	}
	for _, im := range w.ImpliedGeneralizations(e) {
		if im.Target != nil {
			add(im.Kind, im.Target, true)
		}
	}
}
