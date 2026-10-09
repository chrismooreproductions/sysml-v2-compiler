package metamodel

// stdlib is a minimal stand-in for a slice of the real OMG Kernel Data
// Type Library and Kernel Function Library (sysml.library/Kernel
// Libraries/...), covering just the scalar types and operators this
// project's expression subsystem needs. It exists so real SysML source
// referencing e.g. "ScalarValues::Integer" or an operator like "<=" can
// resolve against something, without this project having to parse OMG's
// actual library files -- those are written in KerML's own textual
// notation (`function`, `specializes`, `datatype`, ...), a different
// concrete grammar this project hasn't implemented, and each real
// function specializes a chain up through DataFunctions to Base.
//
// This is a deliberate bridge, not a claim of fidelity: it's built
// directly as an Element graph rather than run through sysml.Parse +
// FromAST (Function isn't a definition kind sysml's own grammar has --
// there's no "function def" the parser could ever produce), it skips the
// real library's specializes chains and parameter/return typing (nothing
// here evaluates or type-checks an expression, so nothing needs them),
// and its symbolic names aren't single-quoted the way real KerML concrete
// syntax would write them (e.g. this model's "+" where upstream writes
// '+') since nothing needs the two to round-trip through KerML notation.
// Swapping this for a real KerML-parsed library is a known improvement
// this project hasn't undertaken yet.
var stdlib = newStdlib()

// scalarValueTypes are the ScalarValues datatypes this project models as
// plain KindDefinition/DefAttribute elements (real KerML calls them
// `datatype`, a classifier kind this project doesn't otherwise have).
var scalarValueTypes = []string{"Boolean", "String", "Real", "Rational", "Integer", "Natural"}

// scalarFunctionOperators are the operator symbols this project's
// expression subsystem resolves against ScalarFunctions, matching (most
// of) the real library's own ScalarFunctions.kerml -- notably excluding
// '==' and '!=' there (equality lives elsewhere upstream, likely at the
// more generic Base/Occurrences level) which this stub includes anyway
// under ScalarFunctions for simplicity, a known deviation a real,
// fully KerML-parsed library would correct.
var scalarFunctionOperators = []string{
	"+", "-", "*", "/", "%", "**",
	"<", ">", "<=", ">=", "==", "!=",
	"not", "xor", "|", "&",
}

// newStdlib builds the standard-library Model. Unexported and called once
// (see the stdlib package var); nothing mutates a Model's Elements after
// FromASTWithImports finishes populating one, so sharing this single
// instance across every translation is safe under this project's existing
// no-concurrency assumptions.
func newStdlib() *Model {
	m := &Model{
		Root:     rootID,
		Elements: map[ElementID]*Element{},
	}
	m.Elements[rootID] = &Element{ID: rootID, Kind: KindNamespace}

	scalarValues := m.declareStdlibPackage("ScalarValues")
	for _, name := range scalarValueTypes {
		id := scalarValues + "::" + ElementID(name)
		m.Elements[id] = &Element{ID: id, Kind: KindDefinition, DefKind: DefAttribute, Name: name, Owner: scalarValues}
	}

	scalarFunctions := m.declareStdlibPackage("ScalarFunctions")
	for _, op := range scalarFunctionOperators {
		id := scalarFunctions + "::" + ElementID(op)
		m.Elements[id] = &Element{ID: id, Kind: KindFunction, Name: op, Owner: scalarFunctions}
	}

	return m
}

// declareStdlibPackage adds a top-level package Element directly under m's
// root and returns its ElementID.
func (m *Model) declareStdlibPackage(name string) ElementID {
	id := ElementID(name)
	m.Elements[id] = &Element{ID: id, Kind: KindPackage, Name: name, Owner: rootID}
	return id
}

// stdlibOperators indexes scalarFunctionOperators for O(1) membership
// checks, built once alongside stdlib itself.
var stdlibOperators = func() map[string]bool {
	set := make(map[string]bool, len(scalarFunctionOperators))
	for _, op := range scalarFunctionOperators {
		set[op] = true
	}
	return set
}()

// stdlibOperatorPath returns the qualified path a binary/unary expression
// operator (e.g. "+") resolves to in the standard library (e.g.
// "ScalarFunctions::+"), and whether op is one this project recognizes as
// a resolvable operator at all. Used by the expression subsystem to build
// a pendingReference for an operator the same way a Usage's type
// reference already is, without needing bare-name visibility for
// operator symbols.
func stdlibOperatorPath(op string) (path string, ok bool) {
	if !stdlibOperators[op] {
		return "", false
	}
	return "ScalarFunctions::" + op, true
}
