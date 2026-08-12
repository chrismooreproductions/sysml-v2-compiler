package modeller

// Package is the root of a parsed model: a named package containing member
// definitions and usages.
type Package struct {
	Name    string
	Members []Member
}

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
