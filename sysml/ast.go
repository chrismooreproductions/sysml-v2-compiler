package sysml

// Namespace is the root of a parsed model: an unnamed collection of
// top-level members (packages, definitions, usages, imports), mirroring the
// grammar's RootNamespace. Unlike Package, it has no Name and is never
// itself a Member -- there's no such thing as nesting one root inside
// another.
type Namespace struct {
	Members []Member
}

// Visibility is a member's `public`/`private`/`protected` prefix, e.g. the
// `private` in `private import Vehicle;`. The grammar attaches this to the
// membership relationship rather than the element itself (MemberPrefix), but
// this project has no first-class Membership yet, so it's folded onto the
// member directly. VisibilityUnspecified means no keyword was written --
// not yet given any enforced meaning (SysML defaults it to public), just
// recorded as parsed.
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

// Package is a named container of members, e.g. `package Vehicle { ... }`.
// It's also itself a Member, so packages can nest inside one another (or
// appear directly in the root Namespace).
type Package struct {
	Visibility Visibility
	Name       string
	Members    []Member
}

func (*Package) memberNode() {}

// Member is anything that can appear inside a Package or a Definition's
// body.
type Member interface {
	memberNode()
}

// DefKind distinguishes the keyword family a Definition or Usage was
// declared with (e.g. "part" vs "item"), independent of the def-vs-usage
// structural distinction those two types already carry.
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
	default:
		return "unknown"
	}
}

// Definition declares a definition, e.g. `part def Engine;` or
// `part def Car { ... }`. Members is nil for the semicolon form.
//
// A constraint/calculation definition's body (CalculationBody) can end in
// a trailing, unterminated result expression after its members, e.g. the
// "totalMass == sum(componentMasses)" directly before the closing brace in
// `constraint def MassAnalysis { attribute totalMass: MassValue; ...
// totalMass == sum(componentMasses) }` -- captured in Result, nil for
// every other DefKind and for a body without one.
type Definition struct {
	Visibility Visibility
	Kind       DefKind
	Name       string
	Members    []Member
	Result     Expression
}

func (*Definition) memberNode() {}

// Usage declares a usage, e.g. `part engine : Engine;`, `port p;` (untyped),
// or `part friendlyCombatants : Combatant[*];` for one bounded by a
// Multiplicity. Type, Multiplicity, Subsets, Redefines, and Value are each
// independently optional, and any combination of Type/Multiplicity/Subsets/
// Redefines may appear in any order (e.g. `part b[0..2] : P;`,
// `part c : P[2..*];`, or `part b subsets a;`) -- each is "" (or nil, for
// Multiplicity/Value) when absent, regardless of order or which others were
// written. Only a single Subsets and a single Redefines target are
// supported, not SysML's comma-separated lists of either.
//
// Name is also independently optional -- SysML's Identification is broadly
// optional throughout, and an anonymous usage (Name == "") is how e.g. an
// inline constraint (`constraint { mass <= massLimit }`) is written.
// Disambiguating an absent name needs no lookahead: nothing that can start
// a usage's specialization part (':' , '[', 'subsets', ':>', ...) is ever
// spelled as a bare identifier, so seeing anything other than an
// Identifier right after the keyword unambiguously means there's no name.
type Usage struct {
	Visibility   Visibility
	Kind         DefKind
	Name         string
	Type         string
	Multiplicity *Multiplicity

	// Subsets is the feature this usage subsets, e.g. the "a" in
	// `part b subsets a;` or `part b :> a;` (both spellings parse the same
	// way). "" if none.
	Subsets string

	// Redefines is the feature this usage redefines, e.g. the "B::b" in
	// `part B_b redefines B::b;` or `part B_b :>> B::b;`. "" if none.
	Redefines string

	// Value is the usage's assigned value, e.g. the 5 in
	// `attribute n : ScalarValues::Integer = 5;`. Only SysML's plain '='
	// FeatureValue form is supported (not ':=' or 'default'). nil if no
	// value was assigned.
	Value Expression

	// Members and Result are a constraint/calculation usage's body (see
	// Definition's doc comment) -- only ever set for DefConstraint/
	// DefCalculation, since only those keyword families give a usage a
	// CalculationBody instead of a plain "; "-terminated declaration.
	Members []Member
	Result  Expression
}

func (*Usage) memberNode() {}

// Connection is a connection or interface usage's connector part, e.g.
// `connect p to y;` (binary, no name/type at all -- the bare
// "'connect' ConnectorPart" shorthand), or
// `connection bus : C connect (d1, d2, d3, d4);` (n-ary, with a name and
// type). Distinct from Usage because a connector's ends aren't expressed
// through FeatureSpecializationPart syntax: each of Ends is a *NameRef or
// *FeatureChain (a plain or dotted feature reference, e.g. "y" or
// "p1.x"), reusing Phase 2's expression machinery rather than inventing a
// separate reference shape. Kind is always DefConnection or DefInterface.
//
// Deliberately out of scope: the optional multiplicity/`references`
// prefix on an individual end (ConnectorEnd's own fuller grammar), `bind`/
// `succession` (BindingConnectorAsUsage/SuccessionAsUsage -- structurally
// unrelated shorthand forms, not ConnectorPart variants), and `end` as a
// body member introducing a connector end inside a definition (only the
// usage-level connect syntax is covered here).
type Connection struct {
	Visibility Visibility
	Kind       DefKind
	Name       string
	Type       string
	Ends       []Expression
	Members    []Member
}

func (*Connection) memberNode() {}

// Expression is a value-producing expression, e.g. the "5" in
// `attribute n : Integer = 5;` or the "mass <= massLimit" a constraint
// body will eventually carry. This project implements a deliberate subset
// of KerML's full expression grammar: literals, qualified-name references,
// and binary/unary operators over them. Not supported, each being its own
// separate chunk of grammar: conditional (`if`/`?`/`else`) expressions,
// invocation/constructor expressions, sequence/index expressions,
// `collect`/`select`, metadata-access, and casts (`as`/`istype`/`hastype`/
// `@`).
type Expression interface {
	expressionNode()
}

// BoolLiteral is a literal `true` or `false`.
type BoolLiteral struct{ Value bool }

func (*BoolLiteral) expressionNode() {}

// IntLiteral is a literal integer, e.g. the 5 in `= 5`.
type IntLiteral struct{ Value int }

func (*IntLiteral) expressionNode() {}

// StringLiteral is a literal double-quoted string, e.g. `"hello"`. No
// escape sequences are supported.
type StringLiteral struct{ Value string }

func (*StringLiteral) expressionNode() {}

// RealLiteral is a literal floating-point number, e.g. `3.14`.
type RealLiteral struct{ Value float64 }

func (*RealLiteral) expressionNode() {}

// NameRef is a reference to another element by qualified name, e.g. the
// "massLimit" in `mass <= massLimit`. Resolved the same way a Usage's Type
// is -- see metamodel.NameRef.
type NameRef struct{ Path string }

func (*NameRef) expressionNode() {}

// FeatureChain is a dot-separated chain of feature references, e.g.
// "vehicle.chassis.mass" in `subject :>> mass = vehicle.chassis.mass;`.
// Path[0] is the (possibly "::"-qualified) first segment; each segment
// after that is a single plain name. Resolved differently from a
// "::"-qualified NameRef: each later segment is looked up among the
// *type* of the previous one, not among a namespace's direct members --
// see metamodel.Model.ResolveFeatureChain.
type FeatureChain struct{ Path []string }

func (*FeatureChain) expressionNode() {}

// BinaryExpr is a binary operator expression, e.g. `mass <= massLimit`.
// The supported operator set matches this project's standard-library stub
// (see metamodel/stdlib.go) exactly: "+ - * / % ** < > <= >= == != xor |
// &" -- notably not "and"/"or"/"implies"/"??" (KerML's separate
// ConditionalBinaryOperatorExpression, not BinaryOperatorExpression, and
// unstubbed). Op resolves to a standard-library Function the same way a
// Usage's Type resolves to a Definition -- see metamodel.BinaryExpr.
type BinaryExpr struct {
	Op          string
	Left, Right Expression
}

func (*BinaryExpr) expressionNode() {}

// UnaryExpr is a unary operator expression, e.g. `-x` or `not done`. Op is
// one of "+", "-", or "not" -- reusing BinaryExpr's "+"/"-" Function stubs
// regardless of arity, since nothing here models a Function's parameters
// to distinguish a one-argument overload from a two-argument one anyway.
type UnaryExpr struct {
	Op      string
	Operand Expression
}

func (*UnaryExpr) expressionNode() {}

// Unbounded marks a Bound's Value as unlimited, e.g. the "*" in "[*]" or
// "[1..*]".
const Unbounded = -1

// Bound is one side of a Multiplicity range: either a literal integer
// (Value, where Unbounded means "*"), or an unresolved name reference to a
// declared value, e.g. the "n" in "[n]" (Name). Resolving a name bound
// needs feature-reference expressions, which are out of scope here -- it's
// parsed and stored, never resolved. Name is "" for a literal or Unbounded
// bound, in which case Value holds the actual bound; Value is meaningless
// when Name is set.
type Bound struct {
	Value int
	Name  string
}

// Multiplicity bounds how many instances a Usage represents, spelled
// as a bracketed suffix on its type: "[*]" (0..Unbounded), "[3]" (an exact
// count, Lower == Upper), "[1..*]", "[0..5]", or a name-valued bound like
// "[n]" or "[1..n]".
type Multiplicity struct {
	Lower Bound
	Upper Bound
}

// Import declares that another namespace's members should be resolvable
// from this one, e.g. `import Vehicle;` (a MembershipImport, importing
// Vehicle itself by name) or `import Vehicle::*;` (a NamespaceImport,
// Wildcard set, importing every one of Vehicle's members as if declared
// directly in the importing namespace). Path is the imported namespace's
// ("::"-qualified) name. Recursive imports ("::**") aren't supported, and
// neither is the FilterPackage form of NamespaceImport.
type Import struct {
	Visibility Visibility
	Path       string
	Wildcard   bool
}

func (*Import) memberNode() {}
