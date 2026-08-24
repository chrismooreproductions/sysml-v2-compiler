package metamodel

import (
	"fmt"

	"github.com/chrismooreproductions/sysml-modeller/sysml"
)

// FromAST translates a parsed SysML AST into a flat metamodel instance,
// with no other Models importable from it. Equivalent to
// FromASTWithImports(pkg, nil) -- see that function's doc comment for the
// full two-phase translation process.
func FromAST(pkg *sysml.Package) (*Model, error) {
	return FromASTWithImports(pkg, nil)
}

// FromASTWithImports translates a parsed SysML AST into a flat metamodel
// instance, resolving `import` members against imports (keyed by the
// imported package's own root name, e.g. "Vehicle" for `import Vehicle;`)
// rather than re-parsing/re-translating them here -- the caller is
// responsible for supplying each import's already-translated Model (e.g.
// loaded from wherever previously-compiled models are kept).
//
// Translation is two phases. Declaration walks the AST building every
// Element, guarding each namespace (the Package, and each PartDef's body)
// against declaring the same name twice within it, and records each
// `import` member against its supplied Model. Resolution then walks the
// type references collected along the way, resolving each via
// Model.Resolve from the namespace it was declared in — so a PartUsage can
// see names declared in its own namespace or any enclosing one (but not an
// unrelated sibling's) for a bare name. A "::"-qualified name resolves the
// same way for its first segment (see resolveQualifiedFrom), reaching a
// sibling's nested namespace without the full path from Root, before
// falling back to an absolute path from Root or an import. Anything left
// unresolved is an error.
func FromASTWithImports(pkg *sysml.Package, imports map[string]*Model) (*Model, error) {
	t := &translator{
		model:   &Model{Elements: map[ElementID]*Element{}, Imports: map[string]*Model{}},
		imports: imports,
	}

	rootID := ElementID(pkg.Name)
	t.model.Elements[rootID] = &Element{ID: rootID, Kind: KindPackage, Name: pkg.Name}
	t.model.Root = rootID

	if err := t.declareMembers(pkg.Members, rootID, newScope()); err != nil {
		return nil, err
	}

	if err := t.resolveTypes(); err != nil {
		return nil, err
	}

	return t.model, nil
}

// pendingType is a PartUsage's type reference, still unresolved: name must
// be resolved from owner (the namespace containing the usage) once every
// declaration has been seen.
type pendingType struct {
	usage ElementID
	name  string
	owner ElementID
}

type translator struct {
	model   *Model
	pending []pendingType

	// imports is the caller-supplied lookup an `import` member resolves
	// against, keyed the same way as Model.Imports. Left nil by FromAST.
	imports map[string]*Model
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
	case *sysml.Package:
		id, ok := t.declare(KindPackage, m.Name, owner, sc)
		if !ok {
			return fmt.Errorf("metamodel: %q is already declared in this scope", m.Name)
		}
		return t.declareMembers(m.Members, id, newScope())

	case *sysml.PartDef:
		id, ok := t.declare(KindPartDef, m.Name, owner, sc)
		if !ok {
			return fmt.Errorf("metamodel: %q is already declared in this scope", m.Name)
		}
		return t.declareMembers(m.Members, id, newScope())

	case *sysml.PartUsage:
		id, ok := t.declare(KindPartUsage, m.Name, owner, sc)
		if !ok {
			return fmt.Errorf("metamodel: %q is already declared in this scope", m.Name)
		}
		t.pending = append(t.pending, pendingType{usage: id, name: m.Type, owner: owner})
		return nil

	case *sysml.Import:
		// Recorded on t.model.Imports regardless of owner -- imports aren't
		// scoped to the namespace they're written in yet, just to the whole
		// model, unlike every other declaration here. Fine for the common
		// case (imports declared at the top level), wrong for the general
		// one (an import nested inside a PartDef shouldn't leak model-wide);
		// revisit if/when that distinction actually matters.
		imported, ok := t.imports[m.Path]
		if !ok {
			return fmt.Errorf("metamodel: import %q: not supplied", m.Path)
		}
		t.model.Imports[m.Path] = imported
		return nil

	default:
		return fmt.Errorf("metamodel: unhandled member type %T", member)
	}
}

func (t *translator) resolveTypes() error {
	for _, p := range t.pending {
		typeID, ok := t.model.Resolve(p.owner, p.name)
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
