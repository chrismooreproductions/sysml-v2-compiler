package sysml

// Package is the root of a parsed model: a named package containing member
// definitions and usages. It's also itself a Member, so packages can
// nest inside one another.
type Package struct {
	Name    string
	Members []Member
}

func (*Package) memberNode() {}

// Member is anything that can appear inside a Package or a PartDef's body.
type Member interface {
	memberNode()
}

// PartDef declares a part definition, e.g. `part def Engine;` or
// `part def Car { ... }`. Members is nil for the semicolon form.
type PartDef struct {
	Name    string
	Members []Member
}

func (*PartDef) memberNode() {}

// PartUsage declares a typed part usage, e.g. `part engine : Engine;`, or
// `part friendlyCombatants : Combatant[*];` for one bounded by a
// Multiplicity. Multiplicity is nil for the plain (unbounded-in-the-other-
// sense -- exactly one) form.
type PartUsage struct {
	Name         string
	Type         string
	Multiplicity *Multiplicity
}

func (*PartUsage) memberNode() {}

// Unbounded marks a Multiplicity's Upper bound as unlimited, e.g. the "*"
// in "[*]" or "[1..*]".
const Unbounded = -1

// Multiplicity bounds how many instances a PartUsage represents, spelled
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
