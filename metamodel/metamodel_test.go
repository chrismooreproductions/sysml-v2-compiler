package metamodel_test

import (
	"testing"

	"github.com/chrismooreproductions/sysml-modeller/metamodel"
	"github.com/chrismooreproductions/sysml-modeller/sysml"
)

func TestFromAST(t *testing.T) {
	source := `package Vehicle {
		part def Engine;

		part def Car {
			part engine : Engine;
		}
	}`

	m := sysml.NewModel(source)

	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(pkg)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	if model.Root != "Vehicle" {
		t.Errorf("root = %q, want %q", model.Root, "Vehicle")
	}

	want := map[metamodel.ElementID]metamodel.Element{
		"Vehicle":              {ID: "Vehicle", Kind: metamodel.KindPackage, Name: "Vehicle"},
		"Vehicle::Engine":      {ID: "Vehicle::Engine", Kind: metamodel.KindPartDef, Name: "Engine", Owner: "Vehicle"},
		"Vehicle::Car":         {ID: "Vehicle::Car", Kind: metamodel.KindPartDef, Name: "Car", Owner: "Vehicle"},
		"Vehicle::Car::engine": {ID: "Vehicle::Car::engine", Kind: metamodel.KindPartUsage, Name: "engine", Owner: "Vehicle::Car"},
	}

	if len(model.Elements) != len(want) {
		t.Fatalf("got %d elements, want %d", len(model.Elements), len(want))
	}

	for id, wantElem := range want {
		got, ok := model.Elements[id]
		if !ok {
			t.Errorf("missing element %q", id)
			continue
		}
		if *got != wantElem {
			t.Errorf("element %q = %+v, want %+v", id, *got, wantElem)
		}
	}

	wantRels := []metamodel.Relationship{
		{ID: "Vehicle::Car::engine::typedBy", Kind: metamodel.TypedBy, Source: "Vehicle::Car::engine", Target: "Vehicle::Engine"},
	}

	if len(model.Relationships) != len(wantRels) {
		t.Fatalf("got %d relationships, want %d", len(model.Relationships), len(wantRels))
	}
	for i, rel := range model.Relationships {
		if *rel != wantRels[i] {
			t.Errorf("relationship %d = %+v, want %+v", i, *rel, wantRels[i])
		}
	}
}

// typeOf finds the ElementID a usage is typed by, via the TypedBy
// relationship model.FromAST produced for it.
func typeOf(model *metamodel.Model, usage metamodel.ElementID) (metamodel.ElementID, bool) {
	for _, rel := range model.RelationshipsFrom(usage) {
		if rel.Kind == metamodel.TypedBy {
			return rel.Target, true
		}
	}
	return "", false
}

func TestFromASTEmptyPackage(t *testing.T) {
	m := sysml.NewModel(`package Empty { }`)

	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(pkg)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	if model.Root != "Empty" {
		t.Errorf("root = %q, want %q", model.Root, "Empty")
	}
	if len(model.Elements) != 1 {
		t.Errorf("got %d elements, want 1", len(model.Elements))
	}
}

func TestFromASTUnresolvedType(t *testing.T) {
	m := sysml.NewModel(`package Vehicle { part engine : Engine; }`)

	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if _, err := metamodel.FromAST(pkg); err == nil {
		t.Error("expected an error for an unresolved type, got nil")
	}
}

func TestFromASTDuplicateDeclaration(t *testing.T) {
	m := sysml.NewModel(`package Vehicle {
		part def Engine;
		part def Engine;
	}`)

	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if _, err := metamodel.FromAST(pkg); err == nil {
		t.Error("expected an error for a duplicate declaration, got nil")
	}
}

// TestFromASTScopingRestrictsVisibility is the case flat, unscoped
// resolution would get wrong: Wheel is only declared inside Car, so a
// usage in the unrelated sibling Bike must not be able to see it even
// though both live in the same model.
func TestFromASTScopingRestrictsVisibility(t *testing.T) {
	m := sysml.NewModel(`package Vehicle {
		part def Car {
			part def Wheel;
		}
		part def Bike {
			part wheel : Wheel;
		}
	}`)

	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if _, err := metamodel.FromAST(pkg); err == nil {
		t.Error("expected an error resolving a type only visible in a sibling scope, got nil")
	}
}

// TestFromASTShadowing checks that a name redeclared in a nested scope
// doesn't collide with the outer declaration (that's legal shadowing, not
// a duplicate declaration), and that a usage in the inner scope resolves
// to the nearest declaration rather than the outer one.
func TestFromASTShadowing(t *testing.T) {
	m := sysml.NewModel(`package Vehicle {
		part def Engine;
		part def Car {
			part def Engine;
			part engine : Engine;
		}
	}`)

	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(pkg)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	typeID, ok := typeOf(model, "Vehicle::Car::engine")
	if !ok {
		t.Fatal("no TypedBy relationship found for Vehicle::Car::engine")
	}
	if typeID != "Vehicle::Car::Engine" {
		t.Errorf("usage type = %q, want %q (the nearer, shadowing declaration)", typeID, "Vehicle::Car::Engine")
	}
}
