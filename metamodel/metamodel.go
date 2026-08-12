// Package metamodel is the platform's generic model representation: a
// flat, KerML-shaped element graph. sysml parses SysML source into a
// syntax-shaped AST; this package translates that AST into Elements
// addressed by ElementID and linked by Owner/Type, rather than a tree of
// Go types per syntactic construct.
package metamodel

// ElementID uniquely identifies an Element within a Model. IDs are
// currently derived from qualified names (e.g. "Vehicle::Car::engine"),
// joining each Element's Name to its Owner's ID with "::".
type ElementID string

type Kind int

const (
	KindPackage Kind = iota
	KindPartDef
	KindPartUsage
)

func (k Kind) String() string {
	switch k {
	case KindPackage:
		return "package"
	case KindPartDef:
		return "part def"
	case KindPartUsage:
		return "part usage"
	default:
		return "unknown"
	}
}

// Element is a single node in the metamodel. Every kind of thing (package,
// definition, usage, ...) is represented the same way and distinguished by
// Kind, rather than as a distinct Go type per kind.
type Element struct {
	ID   ElementID
	Kind Kind
	Name string

	// Owner is the ID of the Element that directly contains this one, or
	// "" for the root.
	Owner ElementID

	// Type is the ID of the Element this one is typed by. It is only set
	// for KindPartUsage, where it's the PartDef the usage refers to.
	Type ElementID
}

// Model is a flat, ID-addressed collection of Elements plus the ID of the
// root element.
type Model struct {
	Root     ElementID
	Elements map[ElementID]*Element
}
