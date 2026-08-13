// Package metamodel is the platform's generic model representation: a
// flat, KerML-shaped element graph. sysml parses SysML source into a
// syntax-shaped AST; this package translates that AST into Elements
// addressed by ElementID, containment-linked by Owner, with everything
// else (typing, and eventually subsetting/redefinition/connections)
// represented as first-class Relationships rather than bespoke fields.
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
	// "" for the root. Containment is kept as a plain field rather than a
	// Relationship: it's the one edge nearly everything else (scoping,
	// traversal, eventual serialization) needs cheaply and constantly.
	Owner ElementID
}

// RelationshipKind distinguishes the different ways two Elements can
// relate to one another, beyond containment.
type RelationshipKind int

const (
	// TypedBy relates a usage to the definition that types it, e.g. the
	// Engine in `part engine : Engine;`.
	TypedBy RelationshipKind = iota
)

func (k RelationshipKind) String() string {
	switch k {
	case TypedBy:
		return "typedBy"
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

// Model is a flat, ID-addressed collection of Elements and the
// Relationships between them, plus the ID of the root element.
type Model struct {
	Root          ElementID
	Elements      map[ElementID]*Element
	Relationships []*Relationship

	// bySource indexes Relationships by Source, built lazily by
	// RelationshipsFrom. It's a cache derived from Relationships, not a
	// second source of truth, so it's left unexported and rebuilt (via
	// bySourceLen) if Relationships has grown since it was last built.
	bySource    map[ElementID][]*Relationship
	bySourceLen int
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
