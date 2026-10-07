// Package metamodel is the platform's generic model representation: a
// flat, KerML-shaped element graph. sysml parses SysML source into a
// syntax-shaped AST; this package translates that AST into Elements
// addressed by ElementID, containment-linked by Owner, with everything
// else (typing, and eventually subsetting/redefinition/connections)
// represented as first-class Relationships rather than bespoke fields.
package metamodel

import "strings"

// ElementID uniquely identifies an Element within a Model. IDs are
// currently derived from qualified names (e.g. "Vehicle::Car::engine"),
// joining each Element's Name to its Owner's ID with "::".
type ElementID string

type Kind int

const (
	KindPackage Kind = iota
	KindDefinition
	KindUsage
	// KindNamespace marks Model.Root itself: the anonymous root every
	// top-level member is declared under (see rootID), mirroring
	// sysml.Namespace. Never appears anywhere else in a Model.
	KindNamespace
	// KindFunction marks a KerML Function element, e.g. a standard-library
	// operator stub like ScalarFunctions::'+' (see stdlib.go). Never
	// produced by translate.go for user source -- sysml has no "function"
	// definition of its own (Function is a KerML-level concrete syntax
	// this project hasn't implemented); it only ever appears as a
	// resolution target an expression's operator points at.
	KindFunction
	// KindSatisfy marks a "satisfy X by Y;" member (see sysml.Satisfy): an
	// anonymous, standalone assertion rather than a named, declared thing,
	// so it gets its own Kind instead of being shoehorned into KindUsage.
	// Its Assert/Negated fields, its References Relationship (X, resolved),
	// and its Connects (Y, resolved -- see those fields' own doc comments)
	// are its only meaningful ones; DefKind, Multiplicity, Value, and Result
	// are always their zero value.
	KindSatisfy
)

func (k Kind) String() string {
	switch k {
	case KindPackage:
		return "package"
	case KindDefinition:
		return "definition"
	case KindUsage:
		return "usage"
	case KindNamespace:
		return "namespace"
	case KindFunction:
		return "function"
	case KindSatisfy:
		return "satisfy"
	default:
		return "unknown"
	}
}

// rootID is the ElementID FromAST/FromASTWithImports assigns to the
// anonymous root namespace (Model.Root), which every top-level member is
// declared under. "::" can never collide with a user-written identifier --
// the lexer tokenizes it as PathSep, never as part of one -- so it's safe
// as a reserved sentinel distinct from any real name.
const rootID ElementID = "::"

// DefKind mirrors sysml.DefKind: the keyword family (e.g. "part") a
// KindDefinition or KindUsage Element was declared with. The two packages
// keep separate types the same way Unbounded does (see the sysml.Unbounded
// doc comment in translate.go's Usage case), so each constant is guaranteed
// to agree with its sysml counterpart only by both listing them in this
// same order, not by a shared definition.
type DefKind int

const (
	DefPart DefKind = iota
	DefAttribute
	DefItem
	DefPort
	DefConstraint
	DefCalculation
	DefConnection
	DefInterface
	DefMetadata
	DefRequirement
	DefConcern
	DefCase
	DefSubject
	DefAssume
	DefRequire
	DefFrame
	DefActor
	DefStakeholder
	// DefEnd mirrors sysml.DefEnd: an "end" body member with no recognized
	// inner keyword.
	DefEnd
	// DefIn, DefOut, DefInOut, DefReturn, and DefRef mirror their sysml
	// counterparts: parameter-direction keywords and the plain
	// reference-usage keyword.
	DefIn
	DefOut
	DefInOut
	DefReturn
	DefRef
	// DefBind mirrors sysml.DefBind: a BindingConnectorAsUsage ("bind a =
	// b;").
	DefBind
)

func (k DefKind) String() string {
	switch k {
	case DefPart:
		return "part"
	case DefAttribute:
		return "attribute"
	case DefItem:
		return "item"
	case DefPort:
		return "port"
	case DefConstraint:
		return "constraint"
	case DefCalculation:
		return "calc"
	case DefConnection:
		return "connection"
	case DefInterface:
		return "interface"
	case DefMetadata:
		return "metadata"
	case DefRequirement:
		return "requirement"
	case DefConcern:
		return "concern"
	case DefCase:
		return "case"
	case DefSubject:
		return "subject"
	case DefAssume:
		return "assume"
	case DefRequire:
		return "require"
	case DefFrame:
		return "frame"
	case DefActor:
		return "actor"
	case DefStakeholder:
		return "stakeholder"
	case DefEnd:
		return "end"
	case DefIn:
		return "in"
	case DefOut:
		return "out"
	case DefInOut:
		return "inout"
	case DefReturn:
		return "return"
	case DefRef:
		return "ref"
	case DefBind:
		return "bind"
	default:
		return "unknown"
	}
}

// Visibility mirrors sysml.Visibility: an Element's `public`/`private`/
// `protected` prefix, carried over as parsed but not yet enforced anywhere
// (nothing consults it to restrict what Resolve can see). The two packages
// keep separate types the same way DefKind does, so the two enums are
// guaranteed to agree only by both listing Unspecified/Public/Private/
// Protected in this same order.
type Visibility int

const (
	VisibilityUnspecified Visibility = iota
	VisibilityPublic
	VisibilityPrivate
	VisibilityProtected
)

func (v Visibility) String() string {
	switch v {
	case VisibilityPublic:
		return "public"
	case VisibilityPrivate:
		return "private"
	case VisibilityProtected:
		return "protected"
	default:
		return "unspecified"
	}
}

// Element is a single node in the metamodel. Every kind of thing (package,
// definition, usage, ...) is represented the same way and distinguished by
// Kind, rather than as a distinct Go type per kind.
type Element struct {
	ID   ElementID
	Kind Kind
	Name string

	// DefKind is the keyword family (e.g. "part") this Definition or Usage
	// was declared with. Meaningful only when Kind is KindDefinition or
	// KindUsage; always DefPart's zero value otherwise.
	DefKind DefKind

	// Visibility is this Element's public/private/protected prefix, or
	// VisibilityUnspecified if it had none. Recorded but not yet enforced --
	// see Visibility's doc comment.
	Visibility Visibility

	// Abstract is this Element's leading `abstract` prefix, mirroring
	// sysml.Definition/Usage/Connection's own Abstract. Recorded but not
	// enforced, the same as Visibility.
	Abstract bool

	// ShortName is this Element's optional "<Name>" short name, mirroring
	// sysml.Definition/Usage's own ShortName. "" if none; not otherwise
	// used -- nothing can currently reference an Element by it.
	ShortName string

	// Library is this Element's leading `library` prefix, mirroring
	// sysml.Package's own Library. Meaningful only when Kind is
	// KindPackage; always false otherwise.
	Library bool

	// Owner is the ID of the Element that directly contains this one, or
	// "" for the root. Containment is kept as a plain field rather than a
	// Relationship: it's the one edge nearly everything else (scoping,
	// traversal, eventual serialization) needs cheaply and constantly.
	Owner ElementID

	// Multiplicity bounds how many instances a KindUsage represents,
	// e.g. the [*] in `part friendlyCombatants : Combatant[*];`. Nil means
	// exactly one, SysML's implicit default; always nil for kinds other
	// than KindUsage.
	Multiplicity *Multiplicity

	// Value is this Usage's assigned value, e.g. the 5 in
	// `attribute n : ScalarValues::Integer = 5;`, mirroring sysml.Usage.Value
	// (see Expression). nil if none was assigned, and always nil for kinds
	// other than KindUsage.
	Value Expression

	// Result is a constraint/calculation-shaped trailing result expression,
	// e.g. the "totalMass <= massLimit" in `constraint def MassAnalysis {
	// ... totalMass <= massLimit }`, mirroring sysml.Definition.Result /
	// sysml.Usage.Result. nil for every DefKind other than DefConstraint/
	// DefCalculation/DefRequire/DefAssume (see sysml.hasCalculationBody),
	// and for a body without one.
	Result Expression

	// Connects holds the resolved ElementIDs of a connection/interface
	// usage's ends (see sysml.Connection), in the order they were written,
	// e.g. the [p, y] behind `connect p to y;` -- or, for a KindSatisfy
	// Element, its single resolved "by" target (see sysml.Satisfy.By), if
	// it had one. nil for every DefKind other than DefConnection/
	// DefInterface, and for a KindSatisfy Element with no "by" clause.
	Connects []ElementID

	// Assert and Negated mirror either a KindSatisfy Element's
	// sysml.Satisfy.Assert/Negated, or (for a KindUsage Element reached
	// via an AssertConstraintUsage) sysml.Usage.Assert/Negated -- either
	// way, the same independently optional "assert"/"not" prefixes.
	// Meaningless (always false) for every other Element.
	Assert  bool
	Negated bool
}

// Unbounded marks a Bound's Value as unlimited, e.g. the "*" in "[*]" or
// "[1..*]".
const Unbounded = -1

// Bound mirrors sysml.Bound: a literal integer (Value, where Unbounded
// means "*") or an unresolved name reference to a declared value (Name).
// See sysml.Bound's doc comment -- resolving a name bound isn't done here
// either.
type Bound struct {
	Value int
	Name  string
}

// Expression mirrors sysml.Expression, with every reference to another
// Element genuinely resolved rather than parsed-and-stored: a NameRef's
// Target and a Binary/UnaryExpr's Function are "" until translate.go's
// expression-reference pass (resolveExpressions) resolves them, the same
// deferred, two-phase way a Usage's type is.
type Expression interface {
	expressionNode()
}

// BoolLiteral is a literal `true` or `false`.
type BoolLiteral struct{ Value bool }

func (*BoolLiteral) expressionNode() {}

// IntLiteral is a literal integer, e.g. the 5 in `= 5`.
type IntLiteral struct{ Value int }

func (*IntLiteral) expressionNode() {}

// StringLiteral is a literal double-quoted string, e.g. `"hello"`.
type StringLiteral struct{ Value string }

func (*StringLiteral) expressionNode() {}

// RealLiteral is a literal floating-point number, e.g. `3.14`.
type RealLiteral struct{ Value float64 }

func (*RealLiteral) expressionNode() {}

// NameRef is a resolved reference to another Element by qualified name.
type NameRef struct {
	Path   string
	Target ElementID
}

func (*NameRef) expressionNode() {}

// FeatureChain is a resolved dot-separated feature chain, e.g.
// "vehicle.chassis.mass". Target is "" until translate.go's
// resolveFeatureChains resolves it via Model.ResolveFeatureChain, which --
// unlike a NameRef's plain Model.Resolve -- walks each segment after the
// first through the previous one's TypedBy relationship rather than a
// namespace's direct members.
type FeatureChain struct {
	Path   []string
	Target ElementID
}

func (*FeatureChain) expressionNode() {}

// InvocationExpr is a resolved function/operation invocation, e.g.
// "sum(componentMasses)". Function is the ElementID Callee resolves to,
// via the same pendingExprRef queue an operator symbol already uses --
// resolving an ordinary name instead of a stdlib operator-table lookup.
// "" until translate.go's resolveExpressions resolves it.
type InvocationExpr struct {
	Callee   string
	Args     []Expression
	Function ElementID
}

func (*InvocationExpr) expressionNode() {}

// SequenceExpr mirrors sysml.SequenceExpr: a parenthesized,
// comma-separated sequence of values. Each element is converted and
// resolved independently via the same convertExpression every other
// expression goes through -- no new resolution concept, since an element
// is just an ordinary expression.
type SequenceExpr struct {
	Elements []Expression
}

func (*SequenceExpr) expressionNode() {}

// BinaryExpr is a resolved binary operator expression. Function is the
// ElementID of the standard-library Function stub Op resolves to (see
// stdlib.go).
type BinaryExpr struct {
	Op          string
	Left, Right Expression
	Function    ElementID
}

func (*BinaryExpr) expressionNode() {}

// UnaryExpr is a resolved unary operator expression.
type UnaryExpr struct {
	Op       string
	Operand  Expression
	Function ElementID
}

func (*UnaryExpr) expressionNode() {}

// Multiplicity bounds how many instances a Usage Element represents.
type Multiplicity struct {
	Lower Bound
	Upper Bound
}

// RelationshipKind distinguishes the different ways two Elements can
// relate to one another, beyond containment.
type RelationshipKind int

const (
	// TypedBy relates a usage to the definition that types it, e.g. the
	// Engine in `part engine : Engine;`.
	TypedBy RelationshipKind = iota
	// Subsets relates a usage to the feature it subsets, e.g. the a in
	// `part b subsets a;`.
	Subsets
	// Redefines relates a usage to the feature it redefines, e.g. the
	// B::b in `part B_b redefines B::b;`.
	Redefines
	// AnnotatedBy relates an Element to a metadata definition/usage tagging
	// it, e.g. the Classified in `#Classified z1;` (see sysml.Usage.Metadata).
	// One Relationship per tag, Source always the annotated Element.
	AnnotatedBy
	// References relates a usage to the feature it references, e.g. the d1
	// in `end end2 ::> d1;` (see sysml.Usage.References) -- KerML's
	// ReferenceSubsetting, its own relationship distinct from Subsets/
	// Redefines. Also used for a KindSatisfy Element's reference to the
	// requirement usage it asserts satisfaction of
	// (sysml.Satisfy.Requirement) -- the real grammar's own
	// ReferenceSubsetting there too, not a separate simplification.
	References
)

func (k RelationshipKind) String() string {
	switch k {
	case TypedBy:
		return "typedBy"
	case Subsets:
		return "subsets"
	case Redefines:
		return "redefines"
	case AnnotatedBy:
		return "annotatedBy"
	case References:
		return "references"
	default:
		return "unknown"
	}
}

// Relationship is a directed, typed edge between two Elements.
type Relationship struct {
	ID     ElementID
	Kind   RelationshipKind
	Source ElementID
	Target ElementID
}

// WildcardImport is one `import Foo::*;` written inside some namespace,
// recorded against that namespace's ElementID in Model.WildcardImports.
// Target's members become resolvable from inside the importing namespace
// as if declared there directly; Visibility is the import statement's own
// (not Target's), governing whether Target's members are, in turn,
// re-exported to anyone who wildcard-imports the importing namespace --
// see lookupChild and lookupChildExternal.
type WildcardImport struct {
	Target     ElementID
	Visibility Visibility
}

// Model is a flat, ID-addressed collection of Elements and the
// Relationships between them, plus the ID of the root element.
type Model struct {
	Root          ElementID
	Elements      map[ElementID]*Element
	Relationships []*Relationship

	// Imports holds other Models this one can reach by qualified name,
	// keyed by their Root Element's Name (e.g. "Vehicle"). It's populated
	// by FromASTWithImports from an `import Vehicle;` member and consulted
	// only by resolveQualified -- a bare (unqualified) reference still only
	// sees its own model, matching this package's minimal import form (see
	// sysml.Import's doc comment). Elements/Relationships that live in an
	// imported Model stay addressed by their own Model's Elements map, not
	// copied into this one -- a Relationship.Target can point into an
	// import, so resolving an ID to an *Element in general means checking
	// the right Model, not just this one.
	Imports map[string]*Model

	// WildcardImports maps a namespace's ElementID to the wildcard imports
	// (`import Foo::*;`) declared directly inside it, each targeting a
	// sibling elsewhere in this same Model -- not a separately compiled
	// one; see Imports for that case. Populated by FromASTWithImports from
	// an `import Foo::*;` member with Wildcard set, resolved the same
	// deferred, two-phase way a Usage's type reference is (see
	// resolveWildcardImports).
	WildcardImports map[ElementID][]WildcardImport

	// bySource indexes Relationships by Source, built lazily by
	// RelationshipsFrom. It's a cache derived from Relationships, not a
	// second source of truth, so it's left unexported and rebuilt (via
	// bySourceLen) if Relationships has grown since it was last built.
	bySource    map[ElementID][]*Relationship
	bySourceLen int

	// childrenByOwner indexes Elements by Owner and then Name, built
	// lazily by Resolve/resolveQualified. Same caching approach as
	// bySource: a derived cache, rebuilt if Elements has grown since it
	// was last built.
	childrenByOwner    map[ElementID]map[string]ElementID
	childrenByOwnerLen int
}

// RelationshipsFrom returns the Relationships whose Source is id, in the
// order they appear in Relationships. Prefer this over scanning
// Relationships directly: repeated lookups (e.g. one per Element) are
// O(1) each instead of O(len(Relationships)) each.
func (m *Model) RelationshipsFrom(id ElementID) []*Relationship {
	if m.bySourceLen != len(m.Relationships) {
		m.bySource = make(map[ElementID][]*Relationship, len(m.Relationships))
		for _, rel := range m.Relationships {
			m.bySource[rel.Source] = append(m.bySource[rel.Source], rel)
		}
		m.bySourceLen = len(m.Relationships)
	}
	return m.bySource[id]
}

// QualifiedName returns id's dotted-path name (e.g. "Vehicle::Car::engine"),
// computed by walking Owner links from id up to the root and joining each
// Element's Name with "::". It deliberately recomputes this from the
// current Owner chain rather than trusting id's own string shape: id is an
// identity, QualifiedName is a position, and the two are only guaranteed
// to look the same today because translate.go happens to derive IDs from
// qualified names. The anonymous root itself (Owner == "") and its direct
// children (Owner == m.Root) both bottom out at their own bare Name, so a
// top-level element's qualified name has no leading "::" for the unnamed
// root. Returns "" if id isn't in the model.
func (m *Model) QualifiedName(id ElementID) string {
	el, ok := m.Elements[id]
	if !ok {
		return ""
	}
	if el.Owner == "" || el.Owner == m.Root {
		return el.Name
	}
	return m.QualifiedName(el.Owner) + "::" + el.Name
}

// children indexes Elements by Owner and then Name, rebuilding the cache
// if Elements has grown since it was last built.
func (m *Model) children() map[ElementID]map[string]ElementID {
	if m.childrenByOwnerLen != len(m.Elements) {
		m.childrenByOwner = make(map[ElementID]map[string]ElementID, len(m.Elements))
		for _, el := range m.Elements {
			if el.Owner == "" {
				continue
			}
			byName, ok := m.childrenByOwner[el.Owner]
			if !ok {
				byName = make(map[string]ElementID)
				m.childrenByOwner[el.Owner] = byName
			}
			byName[el.Name] = el.ID
		}
		m.childrenByOwnerLen = len(m.Elements)
	}
	return m.childrenByOwner
}

// lookupChild returns the ElementID of ns's child named name, as seen by a
// request originating from inside ns's own subtree: ns's direct children
// first, then -- regardless of their own Visibility, since privacy only
// ever restricts access from outside a namespace, never from within it --
// each namespace ns wildcard-imports, then (last, so a locally-declared or
// imported name always wins over a same-named inherited one) a member ns
// inherits from its own type, if it has one -- e.g. the "mass" in
// `requirement r : R { subject :>> mass = ...; }`, declared only inside R's
// own body, not r's, needed so a usage's nested body can see (and
// redefine/subset/reference) whatever its type declares, the same way real
// KerML feature inheritance works. Only one TypedBy hop deep, the same as
// ResolveFeatureChain takes per segment -- a type's own further
// specialization chain isn't walked. Used by Resolve's bare-name climb and
// resolveQualifiedFrom's climb, where every ns checked is, by construction,
// an ancestor of the original request.
func (m *Model) lookupChild(ns ElementID, name string) (ElementID, bool) {
	if id, ok := m.children()[ns][name]; ok {
		return id, true
	}
	for _, imp := range m.WildcardImports[ns] {
		if id, ok := m.children()[imp.Target][name]; ok {
			return id, true
		}
	}
	if typeID, ok := m.typeOf(ns); ok {
		if id, ok := m.lookupChildExternal(typeID, name); ok {
			return id, true
		}
	}
	return "", false
}

// lookupChildExternal returns the ElementID of ns's child named name, as
// seen by a request qualifying into ns from outside it (e.g. resolving
// "ns::name"): ns's direct children, then only its public (or
// unspecified -- SysML's default) wildcard imports. A private wildcard
// import's targets stay visible only to requests already inside ns (via
// lookupChild), never re-exported to an external qualifier -- this is the
// one piece of visibility enforcement this project has today; the general
// case (any private/protected member, not just an import) is future work.
// protected is treated the same as private here for lack of a
// specialization/subclassing concept to give it its own, narrower meaning.
// Used by descend, the only place a qualified path steps into a namespace
// it didn't climb up to.
func (m *Model) lookupChildExternal(ns ElementID, name string) (ElementID, bool) {
	if id, ok := m.children()[ns][name]; ok {
		return id, true
	}
	for _, imp := range m.WildcardImports[ns] {
		if imp.Visibility == VisibilityPrivate || imp.Visibility == VisibilityProtected {
			continue
		}
		if id, ok := m.children()[imp.Target][name]; ok {
			return id, true
		}
	}
	return "", false
}

// Resolve looks up name from the perspective of from, the ElementID of the
// namespace (Package or Definition) containing the reference.
//
// A bare name is looked up against from's own direct children first, then
// each enclosing owner's in turn — so a reference can see names declared in
// its own namespace or any ancestor, but not an unrelated sibling's. A
// "::"-qualified name (e.g. "Powertrain::Engine") resolves the same way for
// its first segment -- from's own children, then each ancestor's, and each
// ancestor's own name too -- before descending through the rest of the
// path; only if that fails does it fall back to an absolute path from Root
// or an import (see resolveQualified). This is what lets a reference reach
// a sibling's nested namespace (e.g. "Powertrain::Engine" from within a
// Definition declared alongside Powertrain, not inside it) without needing the
// full path from Root.
func (m *Model) Resolve(from ElementID, name string) (ElementID, bool) {
	if strings.Contains(name, "::") {
		return m.resolveQualifiedFrom(from, name)
	}

	for current := from; current != ""; {
		if id, ok := m.lookupChild(current, name); ok {
			return id, true
		}
		el, ok := m.Elements[current]
		if !ok {
			return "", false
		}
		current = el.Owner
	}
	return "", false
}

// resolveQualifiedFrom resolves a "::"-joined path's first segment the same
// way Resolve resolves a bare name -- checking from's own children, then
// each enclosing owner's, and (unlike a bare name) each owner's own name as
// well, since a qualified path's first segment can name an enclosing scope
// itself (e.g. "Vehicle::Car::Wheel" written inside Vehicle::Bike needs to
// find Vehicle, not just a child called Vehicle). Once the first segment
// resolves, the rest of the path descends through direct children exactly
// as resolveQualified's absolute walk does. Falls back to resolveQualified
// (absolute from Root, or an import) if nothing in from's own scope chain
// matches -- a relative resolution only ever widens what's reachable, it
// never shadows the absolute/import fallback.
func (m *Model) resolveQualifiedFrom(from ElementID, path string) (ElementID, bool) {
	segments := strings.Split(path, "::")
	first := segments[0]

	for current := from; ; {
		if el, ok := m.Elements[current]; ok && el.Name == first {
			return m.descend(current, segments[1:])
		}
		if id, ok := m.lookupChild(current, first); ok {
			return m.descend(id, segments[1:])
		}
		el, ok := m.Elements[current]
		if !ok || el.Owner == "" {
			break
		}
		current = el.Owner
	}

	return m.resolveQualified(path)
}

// descend walks down from start through a chain of names, each step
// qualifying into the next namespace from outside it (see
// lookupChildExternal) -- e.g. descend(carID, []string{"Wheel"}) to reach
// Car's Wheel.
func (m *Model) descend(start ElementID, segments []string) (ElementID, bool) {
	current := start
	for _, segment := range segments {
		next, ok := m.lookupChildExternal(current, segment)
		if !ok {
			return "", false
		}
		current = next
	}
	return current, true
}

// resolveQualified resolves a "::"-joined absolute path (e.g.
// "Vehicle::Car::Wheel") by matching its first segment against a direct
// child of the anonymous root -- the root itself has no name to match
// against, so any of Root's top-level packages/definitions can start an
// absolute path -- then walking down through each element's direct children
// to match the rest. If the first segment instead names an imported Model,
// resolution delegates to that Model's own resolveQualified for the full
// path -- an import only ever widens what a qualified path can reach, so
// this always tries the local Root first. Used both as
// resolveQualifiedFrom's fallback and directly by an import's own
// resolution (which has no local "from" scope to be relative to).
func (m *Model) resolveQualified(path string) (ElementID, bool) {
	segments := strings.Split(path, "::")

	first, ok := m.lookupChild(m.Root, segments[0])
	if !ok {
		if imported, ok := m.Imports[segments[0]]; ok {
			return imported.resolveQualified(path)
		}
		return "", false
	}

	return m.descend(first, segments[1:])
}

// typeOf returns the ElementID id is TypedBy, if it has one.
func (m *Model) typeOf(id ElementID) (ElementID, bool) {
	for _, rel := range m.RelationshipsFrom(id) {
		if rel.Kind == TypedBy {
			return rel.Target, true
		}
	}
	return "", false
}

// ResolveFeatureChain resolves a dot-separated feature chain (e.g.
// "vehicle.chassis.mass") from the perspective of from, the ElementID of
// the namespace containing the reference -- unlike a "::"-qualified name,
// where every segment is looked up as a namespace member. Only path[0]
// resolves that way, via the ordinary Resolve; each segment after that is
// looked up among the *type* of the previous one -- following its TypedBy
// relationship to a Definition and finding a child of that Definition
// named by the next segment (via lookupChildExternal, the same "stepping
// in from outside" visibility a qualified path's descend uses) -- since
// "vehicle.chassis" means "chassis, a feature of whatever vehicle is typed
// by," not "chassis, a member of vehicle's own namespace." If the previous
// segment's element has no TypedBy relationship at all (an untyped usage
// with its own inline body instead, e.g. "part vehicle { part chassis;
// };" -- see general usage bodies), the next segment is looked up among
// its own direct children instead, since that's the only "feature of
// vehicle" there is to mean in that case. Returns false if any segment
// fails to resolve either way.
func (m *Model) ResolveFeatureChain(from ElementID, path []string) (ElementID, bool) {
	current, ok := m.Resolve(from, path[0])
	if !ok {
		return "", false
	}

	for _, segment := range path[1:] {
		scope := current
		if typeID, ok := m.typeOf(current); ok {
			scope = typeID
		}
		next, ok := m.lookupChildExternal(scope, segment)
		if !ok {
			return "", false
		}
		current = next
	}
	return current, true
}
