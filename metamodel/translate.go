package metamodel

import (
	"fmt"
	"strings"

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

	// Expression references resolve next: an expression can reference a
	// sibling usage (e.g. a constraint naming an attribute by its own
	// name), so its type/subsets/redefines needs to already be resolved
	// first, the same way pendingWildcardImports resolves before pending.
	if err := t.resolveExpressions(); err != nil {
		return nil, err
	}

	// Feature chains resolve next: each segment after the first walks a
	// TypedBy relationship (see ResolveFeatureChain), so every ordinary
	// reference above needs to have already produced the TypedBy
	// relationships a chain might step through.
	if err := t.resolveFeatureChains(); err != nil {
		return nil, err
	}

	// Connects resolve last: each end is an expression (NameRef or
	// FeatureChain) already resolved by one of the two passes above --
	// this just harvests the result.
	if err := t.resolveConnects(); err != nil {
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

// pendingExprRef is an unresolved reference discovered while converting an
// expression tree: a NameRef's path, or an operator symbol (resolved via
// stdlibOperatorPath to its standard-library qualified path -- see
// convertExpression). owner is the namespace the containing usage was
// declared in. set writes the resolved ElementID directly onto whichever
// tree node this reference came from (NameRef.Target, or a Binary/
// UnaryExpr's Function) once resolveExpressions finds it -- a closure
// lets one queue shape handle all three node kinds without a parallel
// kind tag the way pendingReference needs one.
type pendingExprRef struct {
	name  string
	owner ElementID
	set   func(ElementID)
}

// pendingFeatureChain is a FeatureChain's not-yet-resolved path, resolved
// via Model.ResolveFeatureChain rather than Model.Resolve -- see that
// method's doc comment for why the two differ. Deferred to its own pass
// (resolveFeatureChains) after resolveExpressions: it depends on the
// TypedBy relationships resolveReferences produces for every element each
// segment steps through, the same dependency resolveExpressions itself
// already has, and is kept as its own pass for clarity between the two
// resolution algorithms rather than a strict ordering need beyond that.
type pendingFeatureChain struct {
	path  []string
	owner ElementID
	set   func(ElementID)
}

// pendingConnect records one end of a connection/interface usage (see
// sysml.Connection): element is the connection's own ElementID, and end is
// that end's already-converted expression (a *NameRef or *FeatureChain,
// queued for resolution the same as any other expression) -- resolveConnects
// reads end's Target once resolveExpressions/resolveFeatureChains have set
// it, appending it to element's Connects in order.
type pendingConnect struct {
	element ElementID
	end     Expression
}

type translator struct {
	model   *Model
	pending []pendingReference

	// anonCount counts anonymous usages declared so far, giving each a
	// distinct synthetic ID (see declare).
	anonCount int

	// pendingWildcardImports queues each `import Foo::*;` member for
	// resolveWildcardImports, the same way pending queues type references
	// for resolveTypes.
	pendingWildcardImports []pendingWildcardImport

	// pendingExprRefs queues every reference found inside an expression
	// tree (see convertExpression) for resolveExpressions.
	pendingExprRefs []pendingExprRef

	// pendingFeatureChains queues every FeatureChain found inside an
	// expression tree for resolveFeatureChains.
	pendingFeatureChains []pendingFeatureChain

	// pendingConnects queues each connection/interface usage's ends for
	// resolveConnects, run once expression/feature-chain resolution (which
	// each end's already-converted expression goes through like any
	// other) has resolved every end's target.
	pendingConnects []pendingConnect

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
	if name == "" {
		// An anonymous usage (e.g. "constraint { ... }", Phase 3) has
		// nothing to deduplicate against or address by name -- skip the
		// scope-uniqueness check (there's no name to compare) and give it
		// a synthetic ID instead of the empty-name one every anonymous
		// usage in the same scope would otherwise collide on. t.anonCount
		// only needs to be unique per translator run, which it is by
		// construction; "::" can't appear inside a real identifier (see
		// rootID's doc comment), so this can never collide with a real
		// qualified name either.
		t.anonCount++
		id = owner + ElementID(fmt.Sprintf("::<anonymous %d>", t.anonCount))
		t.model.Elements[id] = &Element{ID: id, Kind: kind, Owner: owner}
		return id, true
	}

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

// queueMetadata queues one pendingReference per #Tag annotation (see
// sysml.Usage.Metadata) on an Element just declared, each resolving (from
// owner, the namespace containing the annotated member) to an AnnotatedBy
// Relationship. Shared by every Member case that can carry Metadata
// (Package, Definition, Usage, Connection -- everything but Import, which
// isn't declared as an Element at all).
func (t *translator) queueMetadata(id ElementID, tags []string, owner ElementID) {
	for _, tag := range tags {
		t.pending = append(t.pending, pendingReference{usage: id, name: tag, owner: owner, kind: AnnotatedBy})
	}
}

func (t *translator) declareMembers(members []sysml.Member, owner ElementID, sc *scope) error {
	for _, member := range members {
		if err := t.declareMember(member, owner, sc); err != nil {
			return err
		}
	}
	return nil
}

// declareUsage declares a *sysml.Usage under owner/sc exactly the way
// declareMember's own *sysml.Usage case does, returning the new Element's
// ID so a caller that needs it synchronously -- Satisfy's inline
// "requirement req1 : Req1" declaration needs its own ID immediately, to
// record as the satisfy's requirement target, rather than through a
// deferred pendingReference -- can use it without re-resolving by name.
func (t *translator) declareUsage(m *sysml.Usage, owner ElementID, sc *scope) (ElementID, error) {
	id, ok := t.declare(KindUsage, m.Name, owner, sc)
	if !ok {
		return "", fmt.Errorf("metamodel: %q is already declared in this scope", m.Name)
	}
	t.model.Elements[id].DefKind = DefKind(m.Kind)
	t.model.Elements[id].Visibility = Visibility(m.Visibility)
	t.model.Elements[id].Abstract = m.Abstract
	t.queueMetadata(id, m.Metadata, owner)
	if m.Multiplicity != nil {
		// sysml.Unbounded and metamodel.Unbounded are both -1 by
		// convention, so each Bound's Value carries over unchanged.
		t.model.Elements[id].Multiplicity = &Multiplicity{
			Lower: Bound{Value: m.Multiplicity.Lower.Value, Name: m.Multiplicity.Lower.Name},
			Upper: Bound{Value: m.Multiplicity.Upper.Value, Name: m.Multiplicity.Upper.Name},
		}
	}
	if m.Value != nil {
		val, err := t.convertExpression(m.Value, owner)
		if err != nil {
			return "", err
		}
		t.model.Elements[id].Value = val
	}
	// Only a constraint/calc usage ever has Members/Result (see
	// hasCalculationBody in sysml/parser.go) -- declareMembers on a nil
	// slice is a no-op for every other kind. Both resolve against id,
	// the same as a Definition's own Members/Result do, for the same
	// reason: they're the usage's own body, not its containing scope.
	if err := t.declareMembers(m.Members, id, newScope()); err != nil {
		return "", err
	}
	if m.Result != nil {
		result, err := t.convertExpression(m.Result, id)
		if err != nil {
			return "", err
		}
		t.model.Elements[id].Result = result
	}
	// An untyped usage (e.g. "port p;") has no type to resolve; a usage
	// can independently have a type, a subsets, a redefines, and/or a
	// references, each queued as its own pendingReference.
	if m.Type != "" {
		t.pending = append(t.pending, pendingReference{usage: id, name: m.Type, owner: owner, kind: TypedBy})
	}
	if m.Subsets != "" {
		t.pending = append(t.pending, pendingReference{usage: id, name: m.Subsets, owner: owner, kind: Subsets})
	}
	if m.Redefines != "" {
		t.pending = append(t.pending, pendingReference{usage: id, name: m.Redefines, owner: owner, kind: Redefines})
	}
	if m.References != "" {
		t.pending = append(t.pending, pendingReference{usage: id, name: m.References, owner: owner, kind: References})
	}
	return id, nil
}

func (t *translator) declareMember(member sysml.Member, owner ElementID, sc *scope) error {
	switch m := member.(type) {
	case *sysml.Package:
		id, ok := t.declare(KindPackage, m.Name, owner, sc)
		if !ok {
			return fmt.Errorf("metamodel: %q is already declared in this scope", m.Name)
		}
		t.model.Elements[id].Visibility = Visibility(m.Visibility)
		t.queueMetadata(id, m.Metadata, owner)
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
		t.model.Elements[id].Abstract = m.Abstract
		t.queueMetadata(id, m.Metadata, owner)
		// Each comma-separated specialization target is its own
		// pendingReference -- a Definition can have more than one, unlike a
		// Usage's single Subsets, which is why resolveReferences folds the
		// target name into a Definition-sourced Relationship's ID.
		for _, target := range m.Specializes {
			t.pending = append(t.pending, pendingReference{usage: id, name: target, owner: owner, kind: Subsets})
		}
		if err := t.declareMembers(m.Members, id, newScope()); err != nil {
			return err
		}
		// A constraint/calculation's trailing result expression is part of
		// its own body, so it resolves names against id (its own scope,
		// where its Members were just declared), not owner (the scope
		// containing the definition itself) -- "totalMass <= massLimit"
		// needs to see totalMass/massLimit as its own siblings.
		if m.Result != nil {
			result, err := t.convertExpression(m.Result, id)
			if err != nil {
				return err
			}
			t.model.Elements[id].Result = result
		}
		return nil

	case *sysml.Usage:
		_, err := t.declareUsage(m, owner, sc)
		return err

	case *sysml.Connection:
		id, ok := t.declare(KindUsage, m.Name, owner, sc)
		if !ok {
			return fmt.Errorf("metamodel: %q is already declared in this scope", m.Name)
		}
		t.model.Elements[id].DefKind = DefKind(m.Kind)
		t.model.Elements[id].Visibility = Visibility(m.Visibility)
		t.model.Elements[id].Abstract = m.Abstract
		t.queueMetadata(id, m.Metadata, owner)
		if m.Type != "" {
			t.pending = append(t.pending, pendingReference{usage: id, name: m.Type, owner: owner, kind: TypedBy})
		}
		if err := t.declareMembers(m.Members, id, newScope()); err != nil {
			return err
		}
		// Each end is a plain expression (NameRef or FeatureChain) already
		// -- converting it queues its own resolution exactly like any
		// other expression reference (resolveExpressions/
		// resolveFeatureChains). resolveConnects (after both) harvests
		// each end's now-resolved Target into Connects, in order.
		for _, end := range m.Ends {
			converted, err := t.convertExpression(end, owner)
			if err != nil {
				return err
			}
			t.pendingConnects = append(t.pendingConnects, pendingConnect{element: id, end: converted})
		}
		return nil

	case *sysml.Satisfy:
		// Always anonymous (Satisfy has no Name of its own) -- declare never
		// fails for an anonymous name (see declare), so ok is always true
		// here; checked anyway for the same defensive consistency every
		// other case in this switch already has.
		id, ok := t.declare(KindSatisfy, "", owner, sc)
		if !ok {
			return fmt.Errorf("metamodel: satisfy: unexpected duplicate declaration")
		}
		t.model.Elements[id].Assert = m.Assert
		t.model.Elements[id].Negated = m.Negated
		if m.Declaration != nil {
			// The inline-declaration alternative: declare the brand new
			// requirement usage as an ordinary sibling in owner/sc (so it's
			// resolvable by name from anywhere owner's own scope already
			// reaches, e.g. a later "end r1 ::> req1;" in a derivation
			// connection) and link it in directly -- its ID is already
			// known synchronously, so this needs no pendingReference at
			// all, unlike the bare-reference alternative below.
			declID, err := t.declareUsage(m.Declaration, owner, sc)
			if err != nil {
				return err
			}
			t.model.Relationships = append(t.model.Relationships, &Relationship{
				ID:     id + "::" + ElementID(References.String()),
				Kind:   References,
				Source: id,
				Target: declID,
			})
		} else {
			// m.Requirement (X) is a reference-subsetting-shaped
			// resolution -- the real grammar's own ReferenceSubsetting --
			// resolved the same way a usage's own References is (see the
			// References RelationshipKind's own doc comment).
			t.pending = append(t.pending, pendingReference{usage: id, name: m.Requirement, owner: owner, kind: References})
		}
		// m.By (Y), if present, is a plain expression (NameRef or
		// FeatureChain) already -- converting and queuing it is identical to
		// how a Connection's own ends are handled, harvested into Connects
		// by the same resolveConnects pass.
		if m.By != nil {
			converted, err := t.convertExpression(m.By, owner)
			if err != nil {
				return err
			}
			t.pendingConnects = append(t.pendingConnects, pendingConnect{element: id, end: converted})
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
	// TypedBy resolves in a pass of its own, before every other kind: a
	// nested member's own pendingReference is queued before its enclosing
	// usage's Type is (declareMembers runs, queuing everything nested,
	// before the containing Usage/Definition case queues its own Type --
	// see those cases in declareMember), so e.g. the "mass" in
	// "requirement r : R { subject :>> mass = ...; }" would otherwise try
	// to resolve before r's own TypedBy relationship to R exists --
	// exactly what lookupChild's inherited-member fallback (see its own
	// doc comment) needs to already be there.
	for _, p := range t.pending {
		if p.kind != TypedBy {
			continue
		}
		if err := t.resolvePendingReference(p); err != nil {
			return err
		}
	}
	for _, p := range t.pending {
		if p.kind == TypedBy {
			continue
		}
		if err := t.resolvePendingReference(p); err != nil {
			return err
		}
	}
	return nil
}

// resolvePendingReference resolves a single pendingReference into a
// Relationship, appended to t.model.Relationships.
func (t *translator) resolvePendingReference(p pendingReference) error {
	targetID, ok := t.model.Resolve(p.owner, p.name)
	if !ok {
		return fmt.Errorf("metamodel: %s: unresolved %s %q", t.model.Elements[p.usage].Name, p.kind, p.name)
	}
	// AnnotatedBy (e.g. "#Classified #Security z1;") and a Definition's
	// own Subsets (its comma-separated Specializes list, e.g.
	// "part def C :> A, B;") are the two cases a single source Element
	// can carry more than one Relationship of the same kind, so their
	// ID also folds in the target's own name to stay unique -- every
	// other case keeps the plain usage+kind ID, since a usage only
	// ever has at most one type/subsets/redefines/references.
	id := p.usage + "::" + ElementID(p.kind.String())
	if p.kind == AnnotatedBy || t.model.Elements[p.usage].Kind == KindDefinition {
		id += "::" + ElementID(p.name)
	}
	t.model.Relationships = append(t.model.Relationships, &Relationship{
		ID:     id,
		Kind:   p.kind,
		Source: p.usage,
		Target: targetID,
	})
	return nil
}

// convertExpression translates a sysml.Expression into a metamodel one,
// queuing every NameRef, operator (Binary/UnaryExpr), and InvocationExpr
// callee it contains onto pendingExprRefs for resolveExpressions -- the
// tree shape carries over unchanged, only reference resolution is
// deferred, the same as a Usage's own type/subsets/redefines already is
// via pendingReference.
func (t *translator) convertExpression(expr sysml.Expression, owner ElementID) (Expression, error) {
	switch e := expr.(type) {
	case *sysml.BoolLiteral:
		return &BoolLiteral{Value: e.Value}, nil

	case *sysml.IntLiteral:
		return &IntLiteral{Value: e.Value}, nil

	case *sysml.StringLiteral:
		return &StringLiteral{Value: e.Value}, nil

	case *sysml.RealLiteral:
		return &RealLiteral{Value: e.Value}, nil

	case *sysml.NameRef:
		ref := &NameRef{Path: e.Path}
		t.pendingExprRefs = append(t.pendingExprRefs, pendingExprRef{
			name:  e.Path,
			owner: owner,
			set:   func(id ElementID) { ref.Target = id },
		})
		return ref, nil

	case *sysml.FeatureChain:
		fc := &FeatureChain{Path: e.Path}
		t.pendingFeatureChains = append(t.pendingFeatureChains, pendingFeatureChain{
			path:  e.Path,
			owner: owner,
			set:   func(id ElementID) { fc.Target = id },
		})
		return fc, nil

	case *sysml.InvocationExpr:
		args := make([]Expression, len(e.Args))
		for i, a := range e.Args {
			converted, err := t.convertExpression(a, owner)
			if err != nil {
				return nil, err
			}
			args[i] = converted
		}
		inv := &InvocationExpr{Callee: e.Callee, Args: args}
		t.pendingExprRefs = append(t.pendingExprRefs, pendingExprRef{
			name:  e.Callee,
			owner: owner,
			set:   func(id ElementID) { inv.Function = id },
		})
		return inv, nil

	case *sysml.BinaryExpr:
		left, err := t.convertExpression(e.Left, owner)
		if err != nil {
			return nil, err
		}
		right, err := t.convertExpression(e.Right, owner)
		if err != nil {
			return nil, err
		}
		bin := &BinaryExpr{Op: e.Op, Left: left, Right: right}
		if path, ok := stdlibOperatorPath(e.Op); ok {
			t.pendingExprRefs = append(t.pendingExprRefs, pendingExprRef{
				name:  path,
				owner: owner,
				set:   func(id ElementID) { bin.Function = id },
			})
		}
		return bin, nil

	case *sysml.UnaryExpr:
		operand, err := t.convertExpression(e.Operand, owner)
		if err != nil {
			return nil, err
		}
		un := &UnaryExpr{Op: e.Op, Operand: operand}
		if path, ok := stdlibOperatorPath(e.Op); ok {
			t.pendingExprRefs = append(t.pendingExprRefs, pendingExprRef{
				name:  path,
				owner: owner,
				set:   func(id ElementID) { un.Function = id },
			})
		}
		return un, nil

	default:
		return nil, fmt.Errorf("metamodel: unhandled expression type %T", expr)
	}
}

// resolveExpressions resolves every queued pendingExprRef -- a NameRef's
// path, or an operator's standard-library path -- into an ElementID,
// written directly onto whichever expression-tree node it came from.
func (t *translator) resolveExpressions() error {
	for _, p := range t.pendingExprRefs {
		id, ok := t.model.Resolve(p.owner, p.name)
		if !ok {
			return fmt.Errorf("metamodel: unresolved reference %q", p.name)
		}
		p.set(id)
	}
	return nil
}

// resolveFeatureChains resolves every queued pendingFeatureChain via
// Model.ResolveFeatureChain, writing the result directly onto whichever
// FeatureChain node it came from.
func (t *translator) resolveFeatureChains() error {
	for _, p := range t.pendingFeatureChains {
		id, ok := t.model.ResolveFeatureChain(p.owner, p.path)
		if !ok {
			return fmt.Errorf("metamodel: unresolved feature chain %q", strings.Join(p.path, "."))
		}
		p.set(id)
	}
	return nil
}

// resolveConnects harvests each queued pendingConnect's already-resolved
// target (its end's NameRef.Target or FeatureChain.Target -- resolved by
// resolveExpressions or resolveFeatureChains before this runs) into its
// connection's Connects, in order.
func (t *translator) resolveConnects() error {
	for _, p := range t.pendingConnects {
		var target ElementID
		switch end := p.end.(type) {
		case *NameRef:
			target = end.Target
		case *FeatureChain:
			target = end.Target
		default:
			return fmt.Errorf("metamodel: connector end has unexpected expression type %T", p.end)
		}
		if target == "" {
			return fmt.Errorf("metamodel: %s: unresolved connector end", t.model.Elements[p.element].Name)
		}
		t.model.Elements[p.element].Connects = append(t.model.Elements[p.element].Connects, target)
	}
	return nil
}
