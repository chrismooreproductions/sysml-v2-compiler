package sysml

// Namespace is the root of a parsed model: an unnamed collection of
// top-level members (packages, definitions, usages, imports), mirroring the
// grammar's RootNamespace. Unlike Package, it has no Name and is never
// itself a Member -- there's no such thing as nesting one root inside
// another.
type Namespace struct {
	Members []Member
}

// Package is a named container of members, e.g. `package Vehicle { ... }`.
// It's also itself a Member, so packages can nest inside one another (or
// appear directly in the root Namespace).
type Package struct {
	Name    string
	Members []Member
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
)

func (k DefKind) String() string {
	switch k {
	case DefPart:
		return "part"
	default:
		return "unknown"
	}
}

// Definition declares a definition, e.g. `part def Engine;` or
// `part def Car { ... }`. Members is nil for the semicolon form.
type Definition struct {
	Kind    DefKind
	Name    string
	Members []Member
}

func (*Definition) memberNode() {}

// Usage declares a typed usage, e.g. `part engine : Engine;`, or
// `part friendlyCombatants : Combatant[*];` for one bounded by a
// Multiplicity. Multiplicity is nil for the plain (unbounded-in-the-other-
// sense -- exactly one) form.
type Usage struct {
	Kind         DefKind
	Name         string
	Type         string
	Multiplicity *Multiplicity
}

func (*Usage) memberNode() {}

// Unbounded marks a Multiplicity's Upper bound as unlimited, e.g. the "*"
// in "[*]" or "[1..*]".
const Unbounded = -1

// Multiplicity bounds how many instances a Usage represents, spelled
// as a bracketed suffix on its type: "[*]" (0..Unbounded), "[3]" (an exact
// count, Lower == Upper), "[1..*]", or "[0..5]".
type Multiplicity struct {
	Lower int
	Upper int
}

// Import declares that another package's members should be resolvable by
// qualified name from this package, e.g. `import Vehicle;`. Path is the
// imported package's ("::"-qualified) name, not yet a member reference or
// wildcard form -- those are wider SysML v2 import forms this doesn't cover
// yet.
type Import struct {
	Path string
}

func (*Import) memberNode() {}
