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

// PartUsage declares a typed part usage, e.g. `part engine : Engine;`.
type PartUsage struct {
	Name string
	Type string
}

func (*PartUsage) memberNode() {}

// Import declares that another package's members should be resolvable by
// qualified name from this package, e.g. `import Vehicle;`. Path is the
// imported package's ("::"-qualified) name, not yet a member reference or
// wildcard form -- those are wider SysML v2 import forms this doesn't cover
// yet.
type Import struct {
	Path string
}

func (*Import) memberNode() {}
