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

// An End is one end of a connector: the feature the connector's end points to.
type End struct {
	// Element is the end feature the connector owns.
	Element *Element
	// Name is the end feature's declared name; it is empty for an end the
	// source does not name (not "source" or "target", which such an end takes
	// as its effective name from the feature it implicitly redefines).
	Name string
	// Target is the feature the end points to: the one it references, or for
	// a chain (pa.out1) the last feature of the chain. It is nil for an end
	// that writes no reference, except for the ends of a succession that is a
	// step of an action or state: see Facts.Ends.
	Target *Element
	// Path is the whole chain that leads to Target (pa, out1); it has one
	// element when the end references a feature directly, and none when
	// Target is nil.
	Path []*Element
	// Implied is true when the source writes no reference and Target was
	// taken from the position of the succession among the steps of its owner:
	// see Facts.Ends.
	Implied bool
}

// A FeatureValue is the value expression a feature is given (= 5, := 5,
// default = 5).
type FeatureValue struct {
	// Expression is the value expression.
	Expression *Element
	// IsDefault is true for "default = ...", a value the feature has unless
	// something redefines it; IsInitial is true for ":=", a value only at the
	// start.
	IsDefault, IsInitial bool
	// Computed is true when Workspace.Evaluate could compute the expression.
	Computed bool
	// Value is the result when Computed is true, as Evaluate returns it; it is
	// nil otherwise.
	Value any
	// Text is the expression as written in the source, which is also what a
	// reader wants for an expression that could not be computed. It is empty
	// for a file the workspace did not read, such as the standard library.
	Text string
}

// A Succession is a succession a behaviour writes between its steps (first a
// then b, then c).
type Succession struct {
	// Element is the succession usage.
	Element *Element
	// Source and Target are its ends. An end the source does not write (the
	// first end of "then c") is the step before it, or the member after it,
	// marked Implied; see Facts.Ends.
	Source, Target End
}

// A Transition is a transition of a state or an action: transition t first
// idle accept Sig if ready do act then active, or the "if ready then b" of a
// decision.
type Transition struct {
	// Element is the transition usage.
	Element *Element
	// Source is the state or step the transition leaves, nil when the source
	// writes none.
	Source *Element
	// Target is the state or step it enters, nil when none is written.
	Target *Element
	// Trigger is the accept action that triggers it, Guard the guard
	// expression and Effect the action it performs; each is nil when absent.
	Trigger, Guard, Effect *Element
}

// Behavior is the structure of an action or a state: what the source writes
// to order its steps and to move between its states. The steps themselves are
// the Members of the element. A succession that writes no reference for an end
// ("then fork f;") has the step before or the member after it there, marked
// Implied (see Facts.Ends); the guarded branches of a decision do not, and a
// Transition with no source written has a nil Source.
type Behavior struct {
	// Entry, Do and Exit are the state's entry, do and exit actions.
	Entry, Do, Exit *Element
	// IsParallel is true for a state whose substates are all active at once.
	IsParallel bool
	// Successions are the successions the element owns, in source order.
	Successions []Succession
	// Transitions are the transitions the element owns, in source order.
	Transitions []Transition
	// Condition is the condition of an if or while action (the count > 0 of
	// "if count > 0 { ... }"), and ConditionText is it as written. Both are
	// empty for any other element.
	Condition     *Element
	ConditionText string
}

// Roles are the parts a requirement, a concern, a case or a use case gives
// to the elements it relates, and what a satisfy or verify usage targets.
type Roles struct {
	// Subject is the subject usage the element declares.
	Subject *Element
	// Actors and Stakeholders are the actor and stakeholder usages.
	Actors, Stakeholders []*Element
	// Objective is the objective requirement of a case or use case.
	Objective *Element
	// Assumed and Required are the constraint usages of "assume" and
	// "require".
	Assumed, Required []*Element
	// Satisfies is the requirement a satisfy usage satisfies: the one it
	// references (satisfy r2), or the usage itself when it declares one
	// (satisfy requirement r3 : R1). SatisfiedBy is the element it is
	// satisfied by (... by vehicle), nil if the source writes none.
	Satisfies, SatisfiedBy *Element
	// Verifies are the requirements that a verification case, or its
	// objective, verifies (verify r2).
	Verifies []*Element
}

// An Annotation is a piece of documentation, a comment or a textual
// representation attached to an element.
type Annotation struct {
	// Element is the Documentation, Comment or TextualRepresentation.
	Element *Element
	// Kind is "documentation" (doc), "comment" or "textualRepresentation"
	// (rep).
	Kind string
	// Name is its declared name, empty for most.
	Name string
	// Body is the text without the comment delimiters, trimmed of
	// surrounding space.
	Body string
	// Locale is the language of a comment's text ("en-US"), empty if none is
	// written; Language is the language of a textual representation ("OCL").
	Locale, Language string
	// Annotates are the elements it annotates: those it names with "about",
	// or the element that owns it when it names none.
	Annotates []*Element
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
	// Ends are the ends of a connector, connection, binding, succession, flow,
	// message, interface or allocation, in order, as it writes them. For a
	// message they are the event occurrences it runs between (from driver.cmd
	// to vehicle.cmd). They are nil for anything else, and for a connector
	// that has only ends it inherits.
	//
	// A succession that is a step of an action or state ("then action b;",
	// "first start;" followed by "then ...") may write no reference for an
	// end. The first end is then the step before it in the owner (the nearest
	// member that is not another succession, a transition or a comment), the
	// second the member after it, and the End is marked Implied. Any other
	// end that writes no reference has a nil Target.
	Ends []End
	// IsMessage is true for a message (message m from a to b) as opposed to a
	// flow: a flow usage that specializes Flows::messages without specializing
	// Flows::flows.
	IsMessage bool
	// Source and Target are the first and last of Ends for a flow or message
	// and a succession (the flow's from and to, the succession's first and then). They
	// point into Ends, and are nil for anything else.
	Source, Target *End

	// Value is the feature's value expression, nil when it has none.
	Value *FeatureValue

	// ControlNode is "fork", "join", "decision" or "merge" for the control node
	// of an action, and empty for anything else.
	ControlNode string
	// Behavior is the structure of an action or state, nil for an element that
	// has no entry, do or exit action, succession or transition and is not a
	// parallel state.
	Behavior *Behavior

	// Roles are the requirement roles of the element, nil when it has none.
	Roles *Roles

	// Annotations are the documentation, comments and textual representations
	// that annotate the element: those it owns that do not name another
	// element, then those anywhere in the workspace's files that name it with
	// "about".
	Annotations []Annotation

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
	w.connectorEnds(&f)
	f.Value = w.valueOf(e)
	f.ControlNode = controlNode(e)
	f.Behavior = w.behaviorOf(&f)
	f.Roles = w.rolesOf(e)
	f.Annotations = w.annotationsOf(e)
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

// connectorEnds fills the ends of a connector from the end features it owns.
func (w *Workspace) connectorEnds(f *Facts) {
	e := f.Element
	if !e.IsA("Connector") {
		return
	}
	for _, r := range ownedRels(e) {
		if !r.IsA("EndFeatureMembership") {
			continue
		}
		for _, end := range r.Children("ownedRelatedElement") {
			if !end.Generated {
				f.Ends = append(f.Ends, w.endOf(end))
			}
		}
	}
	if len(f.Ends) == 0 && e.IsA("Flow") {
		// A message has no end features: its ends are the event occurrences
		// it takes as parameters (message m from a.b to c).
		for _, r := range ownedRels(e) {
			if !r.IsA("ParameterMembership") {
				continue
			}
			for _, p := range r.Children("ownedRelatedElement") {
				if p.IsA("EventOccurrenceUsage") && !p.Generated {
					f.Ends = append(f.Ends, w.endOf(p))
				}
			}
		}
	}
	if e.IsA("Flow") {
		// every flow is also a message in the library (flows :> messages), so
		// a message is one that is not a flow
		msgs, flows := w.Lookup("Flows::messages"), w.Lookup("Flows::flows")
		f.IsMessage = msgs != nil && flows != nil && w.Specializes(e, msgs) && !w.Specializes(e, flows)
	}
	if e.IsA("Succession") && len(f.Ends) == 2 && e.Owner.IsA("FeatureMembership") {
		w.impliedSuccessionEnds(f)
	}
	if len(f.Ends) > 0 && (e.IsA("Flow") || e.IsA("Succession")) {
		f.Source, f.Target = &f.Ends[0], &f.Ends[len(f.Ends)-1]
	}
}

// endOf reads what an end feature points to. A reference subsetting names it
// directly or as a chain, and the end of a flow nests the feature it takes
// from the referenced one (the end referencing pa that redefines out1 is
// pa.out1).
func (w *Workspace) endOf(end *Element) End {
	out := End{Element: end, Name: end.Text("declaredName")}
	for _, r := range ownedRels(end) {
		if !r.IsA("ReferenceSubsetting") {
			continue
		}
		for _, c := range r.Children("ownedRelatedElement") {
			for _, ch := range c.Children("ownedRelationship") {
				if ch.IsA("FeatureChaining") {
					for _, ref := range ch.Refs("chainingFeature") {
						if ref.Target != nil {
							out.Path = append(out.Path, ref.Target)
						}
					}
				}
			}
		}
		if len(out.Path) == 0 { // not a chain: the feature is named directly
			if t := targetOf(r); t != nil {
				out.Path = []*Element{t}
			}
		}
		break
	}
	if len(out.Path) == 0 {
		return out
	}
	// nested redefinitions continue the path
	for cur, seen := end, map[*Element]bool{}; !seen[cur]; {
		seen[cur] = true
		var next *Element
		for _, r := range ownedRels(cur) {
			if !r.IsA("FeatureMembership") || r.IsA("EndFeatureMembership") {
				continue
			}
			for _, inner := range r.Children("ownedRelatedElement") {
				if t := first(relationTargets(inner, "Redefinition")); t != nil && !inner.Generated {
					out.Path = append(out.Path, t)
					next = inner
				}
			}
		}
		if next == nil {
			break
		}
		cur = next
	}
	out.Target = out.Path[len(out.Path)-1]
	return out
}

// valueOf reads the value expression of a feature.
func (w *Workspace) valueOf(e *Element) *FeatureValue {
	for _, r := range ownedRels(e) {
		if !r.IsA("FeatureValue") || r.Generated {
			continue
		}
		for _, x := range r.Children("ownedRelatedElement") {
			if x.Generated {
				continue
			}
			v := &FeatureValue{Expression: x, IsDefault: r.IsTrue("isDefault"), IsInitial: r.IsTrue("isInitial")}
			if val, err := w.Evaluate(x); err == nil {
				v.Computed, v.Value = true, val
			}
			v.Text = w.sourceText(x)
			return v
		}
	}
	return nil
}

// sourceText is the text an element was parsed from, if its file is one the
// workspace was given.
func (w *Workspace) sourceText(e *Element) string {
	name := w.FileOf(e)
	for _, d := range w.docs {
		if d.Name == name {
			a, b := e.Span.Start.Offset, e.Span.End.Offset
			if a >= 0 && a <= b && b <= len(d.src) {
				return string(d.src[a:b])
			}
		}
	}
	return ""
}

func controlNode(e *Element) string {
	switch {
	case e.IsA("ForkNode"):
		return "fork"
	case e.IsA("JoinNode"):
		return "join"
	case e.IsA("DecisionNode"):
		return "decision"
	case e.IsA("MergeNode"):
		return "merge"
	}
	return ""
}

// behaviorOf collects the entry, do and exit actions, the successions and the
// transitions that a type owns.
func (w *Workspace) behaviorOf(f *Facts) *Behavior {
	e := f.Element
	if !e.IsA("Type") || e.IsA("TransitionUsage") { // a transition is described by its owner's Transitions
		return nil
	}
	b := &Behavior{IsParallel: e.IsTrue("isParallel")}
	found := b.IsParallel
	for _, r := range ownedRels(e) {
		if r.IsA("StateSubactionMembership") {
			for _, a := range r.Children("ownedRelatedElement") {
				switch r.Text("kind") {
				case "entry":
					b.Entry = a
				case "do":
					b.Do = a
				case "exit":
					b.Exit = a
				}
				found = true
			}
		}
	}
	if e.IsA("IfActionUsage") || e.IsA("WhileLoopActionUsage") {
		// the condition is the first parameter, ahead of the bodies
		for _, r := range ownedRels(e) {
			if !r.IsA("ParameterMembership") {
				continue
			}
			for _, c := range r.Children("ownedRelatedElement") {
				if c.IsA("Expression") && !c.Generated && b.Condition == nil {
					b.Condition, b.ConditionText = c, w.sourceText(c)
					found = true
				}
			}
			break
		}
	}
	for _, m := range f.Members {
		switch {
		case m.Element.IsA("TransitionUsage"):
			b.Transitions = append(b.Transitions, w.transitionOf(m.Element))
			found = true
		case m.Element.IsA("SuccessionAsUsage"):
			sf := w.Facts(m.Element)
			if len(sf.Ends) == 2 {
				b.Successions = append(b.Successions, Succession{m.Element, sf.Ends[0], sf.Ends[1]})
				found = true
			}
		}
	}
	if !found {
		return nil
	}
	return b
}

// transitionOf reads a transition usage: the source it names, the trigger,
// guard and effect it writes, and the target of the succession it owns.
func (w *Workspace) transitionOf(t *Element) Transition {
	out := Transition{Element: t}
	for _, r := range ownedRels(t) {
		switch {
		case r.IsA("TransitionFeatureMembership"):
			for _, x := range r.Children("ownedRelatedElement") {
				switch r.Text("kind") {
				case "trigger":
					out.Trigger = x
				case "guard":
					out.Guard = x
				case "effect":
					out.Effect = x
				}
			}
		case r.IsA("OwningMembership"):
			for _, x := range r.Children("ownedRelatedElement") {
				if x.IsA("SuccessionAsUsage") {
					if ends := w.Facts(x).Ends; len(ends) == 2 {
						out.Target = ends[1].Target
					}
				}
			}
		case r.IsA("Membership") && !r.IsA("OwningMembership"):
			for _, ref := range r.Refs("memberElement") {
				out.Source = ref.Target
			}
		}
	}
	return out
}

// rolesOf collects the roles an element's memberships give to what they own.
func (w *Workspace) rolesOf(e *Element) *Roles {
	r := &Roles{}
	found := false
	for _, m := range ownedRels(e) {
		owned := m.Children("ownedRelatedElement")
		switch {
		case m.IsA("SubjectMembership"):
			for _, x := range owned {
				if !e.IsA("SatisfyRequirementUsage") {
					r.Subject = x
				} else if t := w.valueReference(x); t != nil {
					r.SatisfiedBy = t
				}
				found = true
			}
		case m.IsA("ActorMembership"):
			r.Actors = append(r.Actors, owned...)
			found = found || len(owned) > 0
		case m.IsA("StakeholderMembership"):
			r.Stakeholders = append(r.Stakeholders, owned...)
			found = found || len(owned) > 0
		case m.IsA("ObjectiveMembership"):
			for _, x := range owned {
				r.Objective = x
				found = true
			}
		case m.IsA("RequirementVerificationMembership"):
			for _, x := range owned {
				if t := first(relationTargets(x, "ReferenceSubsetting")); t != nil {
					r.Verifies = append(r.Verifies, t)
				} else {
					r.Verifies = append(r.Verifies, x)
				}
				found = true
			}
		case m.IsA("RequirementConstraintMembership"):
			switch m.Text("kind") {
			case "assumption":
				r.Assumed = append(r.Assumed, owned...)
			case "requirement":
				r.Required = append(r.Required, owned...)
			}
			found = found || len(owned) > 0
		}
	}
	if r.Objective != nil && (e.IsA("VerificationCaseUsage") || e.IsA("VerificationCaseDefinition")) {
		r.Verifies = append(r.Verifies, w.rolesOf(r.Objective).verifiesOrNil()...)
	}
	if e.IsA("SatisfyRequirementUsage") {
		r.Satisfies = first(relationTargets(e, "ReferenceSubsetting"))
		if r.Satisfies == nil {
			r.Satisfies = e
		}
		found = true
	}
	if !found {
		return nil
	}
	return r
}

func (r *Roles) verifiesOrNil() []*Element {
	if r == nil {
		return nil
	}
	return r.Verifies
}

// valueReference is the element that a subject usage's value expression names
// (the "by vehicle" of a satisfy usage).
func (w *Workspace) valueReference(subject *Element) *Element {
	v := w.valueOf(subject)
	if v == nil {
		return nil
	}
	for _, m := range v.Expression.Children("ownedRelationship") {
		for _, ref := range m.Refs("memberElement") {
			return ref.Target
		}
	}
	return nil
}

// annotationsOf finds what annotates an element: its own unaddressed comments
// and the comments that name it.
func (w *Workspace) annotationsOf(e *Element) []Annotation {
	var out []Annotation
	seen := map[*Element]bool{}
	add := func(a *Element) {
		if !seen[a] {
			seen[a] = true
			out = append(out, w.annotation(a))
		}
	}
	for _, r := range ownedRels(e) {
		if !r.IsA("Membership") {
			continue
		}
		for _, a := range r.Children("ownedRelatedElement") {
			if isAnnotation(a) && !a.Generated && len(annotated(a)) == 0 {
				add(a)
			}
		}
	}
	if w.annotated == nil {
		w.annotated = map[*Element][]*Element{}
		for _, d := range w.docs {
			d.Root.Walk(func(a *Element) {
				if isAnnotation(a) {
					for _, t := range annotated(a) {
						w.annotated[t] = append(w.annotated[t], a)
					}
				}
			})
		}
	}
	for _, a := range w.annotated[e] {
		add(a)
	}
	return out
}

// isAnnotation is true for what Facts.Annotations holds: documentation,
// comments and textual representations. Other annotating elements, such as a
// metadata usage, are not.
func isAnnotation(a *Element) bool {
	return a.IsA("Comment") || a.IsA("TextualRepresentation")
}

// annotated are the elements an annotating element names with "about".
func annotated(a *Element) []*Element {
	var out []*Element
	for _, r := range ownedRels(a) {
		if r.IsA("Annotation") {
			for _, ref := range r.Refs("annotatedElement") {
				if ref.Target != nil {
					out = append(out, ref.Target)
				}
			}
		}
	}
	return out
}

func (w *Workspace) annotation(a *Element) Annotation {
	out := Annotation{
		Element:   a,
		Name:      a.Text("declaredName"),
		Body:      strings.TrimSpace(a.Text("body")),
		Locale:    a.Text("locale"),
		Language:  a.Text("language"),
		Annotates: annotated(a),
	}
	switch {
	case a.IsA("Documentation"):
		out.Kind = "documentation"
	case a.IsA("TextualRepresentation"):
		out.Kind = "textualRepresentation"
	default:
		out.Kind = "comment"
	}
	if len(out.Annotates) == 0 && a.Owner.IsA("Membership") {
		out.Annotates = []*Element{a.Owner.Owner}
	}
	return out
}

// impliedSuccessionEnds fills an end of a succession that is a step of an
// action or state and writes no reference, from where the succession sits
// among the members of its owner.
func (w *Workspace) impliedSuccessionEnds(f *Facts) {
	e := f.Element
	members := w.membersOf(e.Owner.Owner)
	at := -1
	for i, m := range members {
		if m.Element == e {
			at = i
		}
	}
	if at < 0 {
		return
	}
	imply := func(end *End, t *Element) {
		if end.Target == nil && t != nil {
			end.Target, end.Path, end.Implied = t, []*Element{t}, true
		}
	}
	// the step before: not another succession, a transition or an annotation
	for i := at - 1; i >= 0; i-- {
		x := members[i].Element
		if x.IsA("Succession") || x.IsA("TransitionUsage") || x.IsA("AnnotatingElement") || x.IsA("Multiplicity") {
			continue
		}
		imply(&f.Ends[0], x)
		break
	}
	// the member after: what an "then fork f;" or "then action b;" declares
	for i := at + 1; i < len(members); i++ {
		x := members[i].Element
		if x.IsA("AnnotatingElement") {
			continue
		}
		if !x.IsA("Succession") {
			imply(&f.Ends[1], x)
		}
		break
	}
}
