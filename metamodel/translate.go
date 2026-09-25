package metamodel

import (
	"fmt"

	"github.com/chrismooreproductions/sysml-modeller/sysml"
)

// FromAST translates a parsed SysML AST into a flat metamodel instance,
// with no other Models importable from it. Equivalent to
// FromASTWithImports(ns, nil) -- see that function's doc comment for the
// full two-phase translation process.
func FromAST(ns *sysml.Namespace) (*Model, error) {
	return FromASTWithImports(ns, nil)
}

// FromASTWithImports translates a parsed SysML AST into a flat metamodel
// instance, resolving `import` members against imports (keyed by the
// imported package's own root name, e.g. "Vehicle" for `import Vehicle;`)
// rather than re-parsing/re-translating them here -- the caller is
// responsible for supplying each import's already-translated Model (e.g.
// loaded from wherever previously-compiled models are kept).
//
// Translation is two phases. Declaration walks the AST building every
// Element under a synthesized anonymous root (see rootID), guarding each
// namespace (that root itself, each Package, and each Definition's body)
// against declaring the same name twice within it, and records each
// `import` member against its supplied Model. Resolution then walks the
// type references collected along the way, resolving each via
// Model.Resolve from the namespace it was declared in — so a Usage can
// see names declared in its own namespace or any enclosing one (but not an
// unrelated sibling's) for a bare name. A "::"-qualified name resolves the
// same way for its first segment (see resolveQualifiedFrom), reaching a
// sibling's nested namespace without the full path from Root, before
// falling back to an absolute path from Root or an import. Anything left
// unresolved is an error.
func FromASTWithImports(ns *sysml.Namespace, imports map[string]*Model) (*Model, error) {
	t := &translator{
		model:   &Model{Root: rootID, Elements: map[ElementID]*Element{}, Imports: map[string]*Model{}},
		imports: imports,
	}

	t.model.Elements[rootID] = &Element{ID: rootID, Kind: KindNamespace}

	if err := t.declareMembers(ns.Members, rootID, newScope()); err != nil {
		return nil, err
	}

	if err := t.resolveTypes(); err != nil {
		return nil, err
	}

	return t.model, nil
}

// pendingType is a Usage's type reference, still unresolved: name must
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
// sc. A top-level member (owner == rootID, the anonymous root) gets name
// itself as its ID, with no "::" prefix -- the root has no name of its own
// to join against.
func (t *translator) declare(kind Kind, name string, owner ElementID, sc *scope) (id ElementID, ok bool) {
	id = ElementID(name)
	if owner != rootID {
		id = owner + "::" + ElementID(name)
	}

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
		t.model.Elements[id].Visibility = Visibility(m.Visibility)
		return t.declareMembers(m.Members, id, newScope())

	case *sysml.Definition:
		id, ok := t.declare(KindDefinition, m.Name, owner, sc)
		if !ok {
			return fmt.Errorf("metamodel: %q is already declared in this scope", m.Name)
		}
		// sysml.DefKind and metamodel.DefKind are both 0 for "part" by
		// convention (see DefKind's doc comment), so this conversion is
		// safe without a lookup table.
		t.model.Elements[id].DefKind = DefKind(m.Kind)
		t.model.Elements[id].Visibility = Visibility(m.Visibility)
		return t.declareMembers(m.Members, id, newScope())

	case *sysml.Usage:
		id, ok := t.declare(KindUsage, m.Name, owner, sc)
		if !ok {
			return fmt.Errorf("metamodel: %q is already declared in this scope", m.Name)
		}
		t.model.Elements[id].DefKind = DefKind(m.Kind)
		t.model.Elements[id].Visibility = Visibility(m.Visibility)
		if m.Multiplicity != nil {
			// sysml.Unbounded and metamodel.Unbounded are both -1 by
			// convention, so each Bound's Value carries over unchanged.
			t.model.Elements[id].Multiplicity = &Multiplicity{
				Lower: Bound{Value: m.Multiplicity.Lower.Value, Name: m.Multiplicity.Lower.Name},
				Upper: Bound{Value: m.Multiplicity.Upper.Value, Name: m.Multiplicity.Upper.Name},
			}
		}
		t.model.Elements[id].Value = m.Value
		// An untyped usage (e.g. "port p;") has nothing to resolve.
		if m.Type != "" {
			t.pending = append(t.pending, pendingType{usage: id, name: m.Type, owner: owner})
		}
		return nil

	case *sysml.Import:
		// Recorded on t.model.Imports regardless of owner -- imports aren't
		// scoped to the namespace they're written in yet, just to the whole
		// model, unlike every other declaration here. Fine for the common
		// case (imports declared at the top level), wrong for the general
		// one (an import nested inside a Definition shouldn't leak model-wide);
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
