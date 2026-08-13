package metamodel

import (
	"fmt"
	"strings"

	"github.com/chrismooreproductions/sysml-modeller/sysml"
)

// FromAST translates a parsed SysML AST into a flat metamodel instance.
//
// Translation is two phases. Declaration walks the AST building every
// Element plus a lexically-scoped symbol table of their names, rejecting
// two sibling elements that declare the same name in the same scope.
// Resolution then walks the type references collected along the way. A
// bare name (no "::") resolves against the scope it was declared in, so a
// PartUsage can see names declared in its own container and any enclosing
// one, but not unrelated sibling containers. A "::"-qualified name is
// instead resolved as an absolute path from the model root, which can
// reach into any container regardless of where the reference sits.
// Anything left unresolved is an error.
func FromAST(pkg *sysml.Package) (*Model, error) {
	t := &translator{
		model:           &Model{Elements: map[ElementID]*Element{}},
		containerScopes: map[ElementID]*scope{},
	}

	rootID := ElementID(pkg.Name)
	t.model.Elements[rootID] = &Element{ID: rootID, Kind: KindPackage, Name: pkg.Name}
	t.model.Root = rootID

	rootScope := newScope(nil)
	t.containerScopes[rootID] = rootScope

	if err := t.declareMembers(pkg.Members, rootID, rootScope); err != nil {
		return nil, err
	}

	if err := t.resolveTypes(); err != nil {
		return nil, err
	}

	return t.model, nil
}

// pendingType is a PartUsage's type reference, still unresolved: name must
// be looked up in scope once every declaration has been seen.
type pendingType struct {
	usage ElementID
	name  string
	scope *scope
}

type translator struct {
	model   *Model
	pending []pendingType

	// containerScopes maps each container Element's ID to the scope
	// holding its direct children's names, keyed independently of the
	// lexical scope chain so absolute (qualified-name) resolution can
	// walk down from the root by name rather than search outward from a
	// reference's own position.
	containerScopes map[ElementID]*scope
}

// declare creates a new Element under owner and adds it to sc under name.
// ok is false, and no Element is created, if name is already declared in
// sc.
func (t *translator) declare(kind Kind, name string, owner ElementID, sc *scope) (id ElementID, ok bool) {
	id = owner + "::" + ElementID(name)

	if !sc.define(name, id) {
		return "", false
	}
	t.model.Elements[id] = &Element{ID: id, Kind: kind, Name: name, Owner: owner}
	return id, true
}

func (t *translator) declareMembers(members []sysml.Member, owner ElementID, sc *scope) error {
	for _, member := range members {
		if err := t.declareMember(member, owner, sc); err != nil {
			return err
		}
	}
	return nil
}

func (t *translator) declareMember(member sysml.Member, owner ElementID, sc *scope) error {
	switch m := member.(type) {
	case *sysml.PartDef:
		id, ok := t.declare(KindPartDef, m.Name, owner, sc)
		if !ok {
			return fmt.Errorf("metamodel: %q is already declared in this scope", m.Name)
		}
		inner := newScope(sc)
		t.containerScopes[id] = inner
		return t.declareMembers(m.Members, id, inner)

	case *sysml.PartUsage:
		id, ok := t.declare(KindPartUsage, m.Name, owner, sc)
		if !ok {
			return fmt.Errorf("metamodel: %q is already declared in this scope", m.Name)
		}
		t.pending = append(t.pending, pendingType{usage: id, name: m.Type, scope: sc})
		return nil

	default:
		return fmt.Errorf("metamodel: unhandled member type %T", member)
	}
}

func (t *translator) resolveTypes() error {
	for _, p := range t.pending {
		typeID, ok := t.resolve(p.name, p.scope)
		if !ok {
			return fmt.Errorf("metamodel: %s: unresolved type %q", t.model.Elements[p.usage].Name, p.name)
		}
		t.model.Relationships = append(t.model.Relationships, &Relationship{
			ID:     p.usage + "::" + ElementID(TypedBy.String()),
			Kind:   TypedBy,
			Source: p.usage,
			Target: typeID,
		})
	}
	return nil
}

// resolve resolves name against sc: a "::"-qualified name is treated as an
// absolute path from the model root, regardless of where the reference
// sits; a bare name is looked up through sc's lexical scope chain.
func (t *translator) resolve(name string, sc *scope) (ElementID, bool) {
	if strings.Contains(name, "::") {
		return t.resolveQualified(name)
	}
	return sc.resolve(name)
}

// resolveQualified resolves a "::"-joined absolute path (e.g.
// "Vehicle::Car::Wheel") by matching its first segment against the model
// root, then walking down through each container's own scope (not its
// scope chain) to match the remaining segments.
func (t *translator) resolveQualified(path string) (ElementID, bool) {
	segments := strings.Split(path, "::")

	root, ok := t.model.Elements[t.model.Root]
	if !ok || root.Name != segments[0] {
		return "", false
	}

	current := t.model.Root
	for _, segment := range segments[1:] {
		sc, ok := t.containerScopes[current]
		if !ok {
			return "", false
		}
		next, ok := sc.local(segment)
		if !ok {
			return "", false
		}
		current = next
	}

	return current, true
}
