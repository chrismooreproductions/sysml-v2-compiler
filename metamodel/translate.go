package metamodel

import (
	"fmt"

	"github.com/chrismooreproductions/sysml-modeller/sysml"
)

// FromAST translates a parsed SysML AST into a flat metamodel instance.
//
// Translation is two phases. Declaration walks the AST building every
// Element plus a lexically-scoped symbol table of their names, rejecting
// two sibling elements that declare the same name in the same scope.
// Resolution then walks the type references collected along the way,
// resolving each against the scope it was declared in (so a PartUsage can
// see names declared in its own container and any enclosing one, but not
// unrelated sibling containers), and fails on anything left unresolved.
func FromAST(pkg *sysml.Package) (*Model, error) {
	t := &translator{model: &Model{Elements: map[ElementID]*Element{}}}

	rootID := ElementID(pkg.Name)
	t.model.Elements[rootID] = &Element{ID: rootID, Kind: KindPackage, Name: pkg.Name}
	t.model.Root = rootID

	if err := t.declareMembers(pkg.Members, rootID, newScope(nil)); err != nil {
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
		return t.declareMembers(m.Members, id, newScope(sc))

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
		typeID, ok := p.scope.resolve(p.name)
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
