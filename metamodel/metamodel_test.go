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
		"Vehicle::Car::engine": {ID: "Vehicle::Car::engine", Kind: metamodel.KindPartUsage, Name: "engine", Owner: "Vehicle::Car", Type: "Vehicle::Engine"},
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
