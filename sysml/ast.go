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
	default:
		return "unknown"
	}
}

// Definition declares a definition, e.g. `part def Engine;` or
// `part def Car { ... }`. Members is nil for the semicolon form.
type Definition struct {
	Visibility Visibility
	Kind       DefKind
	Name       string
	Members    []Member
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
	// FeatureValue form is supported (not ':=' or 'default'), and only
	// integer literals, not full expressions -- nil if no value was
	// assigned.
	Value *int
}

func (*Usage) memberNode() {}

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
