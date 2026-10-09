package metamodel_test

import (
	"testing"

	"github.com/chrismooreproductions/sysml-modeller/metamodel"
	"github.com/chrismooreproductions/sysml-modeller/sysml"
)

// TestFromASTResolvesStandardLibraryScalarValues checks that a usage typed
// by a standard-library scalar type (e.g. "ScalarValues::Integer")
// resolves and translates successfully, with no explicit `import
// ScalarValues;` in the source -- matching real SysML, where the standard
// library is implicitly available everywhere.
func TestFromASTResolvesStandardLibraryScalarValues(t *testing.T) {
	for _, name := range []string{"Boolean", "String", "Real", "Rational", "Integer", "Natural"} {
		t.Run(name, func(t *testing.T) {
			ns, err := sysml.NewModel(`package Vehicle { attribute n : ScalarValues::` + name + `; }`).Parse()
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			model, err := metamodel.FromAST(ns)
			if err != nil {
				t.Fatalf("unexpected translate error: %v", err)
			}

			typeID, ok := model.Resolve(model.Root, "Vehicle::n")
			if !ok {
				t.Fatal("missing element Vehicle::n")
			}
			for _, rel := range model.RelationshipsFrom(typeID) {
				if rel.Kind == metamodel.TypedBy && rel.Target == metamodel.ElementID("ScalarValues::"+name) {
					return
				}
			}
			t.Errorf("Vehicle::n not typed by ScalarValues::%s", name)
		})
	}
}

// TestFromASTResolvesStandardLibraryScalarFunctions checks that each
// operator symbol the expression subsystem needs resolves, via the
// ordinary qualified-name mechanism, to a stub Function element under
// ScalarFunctions -- again with no explicit import.
func TestFromASTResolvesStandardLibraryScalarFunctions(t *testing.T) {
	ns, err := sysml.NewModel(`package Empty { }`).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	for _, op := range []string{"+", "-", "*", "/", "%", "**", "<", ">", "<=", ">=", "==", "!=", "not", "xor", "|", "&"} {
		t.Run(op, func(t *testing.T) {
			id, ok := model.Resolve(model.Root, "ScalarFunctions::"+op)
			if !ok {
				t.Fatalf("ScalarFunctions::%s did not resolve", op)
			}
			if id != metamodel.ElementID("ScalarFunctions::"+op) {
				t.Errorf("resolved to %q, want %q", id, "ScalarFunctions::"+op)
			}
		})
	}
}

// TestFromASTStandardLibraryUnknownMember checks that a name that isn't
// actually in the stubbed standard library still fails to resolve, rather
// than the stdlib import silently absorbing anything.
func TestFromASTStandardLibraryUnknownMember(t *testing.T) {
	ns, err := sysml.NewModel(`package Vehicle { attribute n : ScalarValues::NoSuchType; }`).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if _, err := metamodel.FromAST(ns); err == nil {
		t.Error("expected an error for an unresolved standard-library type, got nil")
	}
}
