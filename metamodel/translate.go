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
// references collected along the way -- each Usage's type, subsetted
// feature, and/or redefined feature (see pendingReference) -- resolving
// each via Model.Resolve from the namespace it was declared in — so a Usage
// can see names declared in its own namespace or any enclosing one (but not
// an unrelated sibling's) for a bare name. A "::"-qualified name resolves
// the same way for its first segment (see resolveQualifiedFrom), reaching a
// sibling's nested namespace without the full path from Root, before
// falling back to an absolute path from Root or an import. Anything left
// unresolved is an error.
func FromASTWithImports(ns *sysml.Namespace, imports map[string]*Model) (*Model, error) {
	t := &translator{
		model: &Model{
			Root:            rootID,
			Elements:        map[ElementID]*Element{},
			Imports:         map[string]*Model{},
			WildcardImports: map[ElementID][]WildcardImport{},
		},
		imports: imports,
	}

	t.model.Elements[rootID] = &Element{ID: rootID, Kind: KindNamespace}

	// The standard library is implicitly available in every model, the
	// same as in real SysML -- seeded before any user `import` is
	// processed, so declareMember's own Import case (below) can still
	// shadow these entries if a caller-supplied import happens to reuse
	// one of these path names.
	t.model.Imports["ScalarValues"] = stdlib
	t.model.Imports["ScalarFunctions"] = stdlib

	if err := t.declareMembers(ns.Members, rootID, newScope()); err != nil {
		return nil, err
	}

	// Wildcard imports resolve before types: a usage's type may only be
	// reachable through one (see resolveWildcardImports).
	if err := t.resolveWildcardImports(); err != nil {
		return nil, err
	}

	if err := t.resolveReferences(); err != nil {
		return nil, err
	}

	return t.model, nil
}

// pendingReference is a Usage's not-yet-resolved reference to another
// Element -- its type ("part engine : Engine;", kind TypedBy), a subsetted
// feature ("part b subsets a;", kind Subsets), or a redefined one
// ("part x redefines y;", kind Redefines). name must be resolved from owner
// (the namespace containing the usage) once every declaration has been
// seen, producing a Relationship of kind once it does.
type pendingReference struct {
	usage ElementID
	name  string
	owner ElementID
	kind  RelationshipKind
}

// pendingWildcardImport is an `import Foo::*;` member, still unresolved:
// name (the "Foo") must be resolved from namespace (where the import
// statement itself was written) the same deferred way a pendingType is, so
// an import doesn't have to textually precede whatever it targets.
type pendingWildcardImport struct {
	namespace  ElementID
	name       string
	visibility Visibility
}

type translator struct {
	model   *Model
	pending []pendingReference

	// pendingWildcardImports queues each `import Foo::*;` member for
	// resolveWildcardImports, the same way pending queues type references
	// for resolveTypes.
	pendingWildcardImports []pendingWildcardImport

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
		// An untyped usage (e.g. "port p;") has no type to resolve; a usage
		// can independently have a type, a subsets, and/or a redefines,
		// each queued as its own pendingReference.
		if m.Type != "" {
			t.pending = append(t.pending, pendingReference{usage: id, name: m.Type, owner: owner, kind: TypedBy})
		}
		if m.Subsets != "" {
			t.pending = append(t.pending, pendingReference{usage: id, name: m.Subsets, owner: owner, kind: Subsets})
		}
		if m.Redefines != "" {
			t.pending = append(t.pending, pendingReference{usage: id, name: m.Redefines, owner: owner, kind: Redefines})
		}
		return nil

	case *sysml.Import:
		if m.Wildcard {
			// Deferred to resolveWildcardImports, the same two-phase way a
			// Usage's type reference is -- m.Path names a sibling elsewhere
			// in this same model (see WildcardImports), correctly scoped to
			// owner, not the whole model.
			t.pendingWildcardImports = append(t.pendingWildcardImports, pendingWildcardImport{
				namespace:  owner,
				name:       m.Path,
				visibility: Visibility(m.Visibility),
			})
			return nil
		}

		// Recorded on t.model.Imports regardless of owner -- imports aren't
		// scoped to the namespace they're written in yet, just to the whole
		// model, unlike every other declaration here (including, now,
		// wildcard imports just above). Fine for the common case (imports
		// declared at the top level), wrong for the general one (an import
		// nested inside a Definition shouldn't leak model-wide); revisit
		// if/when that distinction actually matters for this, the
		// externally-supplied-Model case.
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

// resolveWildcardImports resolves each queued `import Foo::*;` member's
// target namespace, via Model.Resolve from the namespace the import was
// written in -- so "import P1::*;" written inside a sibling of P1 finds it
// the same way any other reference to a sibling scope would. Only a single
// hop of transitivity is handled: resolving one import doesn't yet see
// through another (an import naming something only reachable via a further
// wildcard import elsewhere isn't resolved here).
func (t *translator) resolveWildcardImports() error {
	for _, p := range t.pendingWildcardImports {
		targetID, ok := t.model.Resolve(p.namespace, p.name)
		if !ok {
			return fmt.Errorf("metamodel: import %q: unresolved", p.name)
		}
		t.model.WildcardImports[p.namespace] = append(t.model.WildcardImports[p.namespace], WildcardImport{
			Target:     targetID,
			Visibility: p.visibility,
		})
	}
	return nil
}

// resolveReferences resolves every queued pendingReference -- a usage's
// type, subsetted feature, or redefined feature -- into a Relationship of
// the matching kind. A usage with more than one (e.g. both a type and a
// subsets) gets one Relationship per reference, distinguished by kind, so
// their IDs (each usage's ID joined with that kind's own name) never
// collide.
func (t *translator) resolveReferences() error {
	for _, p := range t.pending {
		targetID, ok := t.model.Resolve(p.owner, p.name)
		if !ok {
			return fmt.Errorf("metamodel: %s: unresolved %s %q", t.model.Elements[p.usage].Name, p.kind, p.name)
		}
		t.model.Relationships = append(t.model.Relationships, &Relationship{
			ID:     p.usage + "::" + ElementID(p.kind.String()),
			Kind:   p.kind,
			Source: p.usage,
			Target: targetID,
		})
	}
	return nil
}
