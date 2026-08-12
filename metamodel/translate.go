package metamodel

import (
	"fmt"

	"github.com/chrismooreproductions/sysml-modeller/sysml"
)

// FromAST translates a parsed SysML AST into a flat metamodel instance.
//
// Type resolution is currently flat and unscoped: a PartUsage's type name
// is looked up against every PartDef in the whole model regardless of
// where it's declared, since the grammar has no qualified type references
// or nested scoping rules to resolve against yet.
func FromAST(pkg *sysml.Package) (*Model, error) {
	t := &translator{
		model:      &Model{Elements: map[ElementID]*Element{}},
		defsByName: map[string]ElementID{},
		usageTypes: map[ElementID]string{},
	}

	t.model.Root = t.addElement(KindPackage, pkg.Name, "")

	if err := t.translateMembers(pkg.Members, t.model.Root); err != nil {
		return nil, err
	}

	if err := t.resolveTypes(); err != nil {
		return nil, err
	}

	return t.model, nil
}

type translator struct {
	model      *Model
	defsByName map[string]ElementID
	usageTypes map[ElementID]string // usage element ID -> unresolved type name
}

func (t *translator) addElement(kind Kind, name string, owner ElementID) ElementID {
	id := ElementID(name)
	if owner != "" {
		id = owner + "::" + ElementID(name)
	}
	t.model.Elements[id] = &Element{ID: id, Kind: kind, Name: name, Owner: owner}
	return id
}

func (t *translator) translateMembers(members []sysml.Member, owner ElementID) error {
	for _, member := range members {
		if err := t.translateMember(member, owner); err != nil {
			return err
		}
	}
	return nil
}

func (t *translator) translateMember(member sysml.Member, owner ElementID) error {
	switch m := member.(type) {
	case *sysml.PartDef:
		id := t.addElement(KindPartDef, m.Name, owner)
		t.defsByName[m.Name] = id
		return t.translateMembers(m.Members, id)

	case *sysml.PartUsage:
		id := t.addElement(KindPartUsage, m.Name, owner)
		t.usageTypes[id] = m.Type
		return nil

	default:
		return fmt.Errorf("metamodel: unhandled member type %T", member)
	}
}

func (t *translator) resolveTypes() error {
	for id, typeName := range t.usageTypes {
		typeID, ok := t.defsByName[typeName]
		if !ok {
			return fmt.Errorf("metamodel: %s: unresolved type %q", t.model.Elements[id].Name, typeName)
		}
		t.model.Elements[id].Type = typeID
	}
	return nil
}
