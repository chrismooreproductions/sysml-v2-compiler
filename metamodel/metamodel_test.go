package metamodel_test

import (
	"testing"

	"github.com/chrismooreproductions/sysml-modeller/metamodel"
	"github.com/chrismooreproductions/sysml-modeller/sysml"
)

// TestModelResolve exercises Resolve directly (rather than only indirectly
// through FromAST's own use of it for PartUsage typing), since it's public
// API meant to be called post-translation by things other than the
// translator itself.
func TestModelResolve(t *testing.T) {
	m := sysml.NewModel(`package Vehicle {
		part def Engine;
		part def Car {
			part def Wheel;
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

	tests := []struct {
		desc string
		from metamodel.ElementID
		name string
		want metamodel.ElementID
	}{
		{"finds a name declared locally", "Vehicle::Car", "Wheel", "Vehicle::Car::Wheel"},
		{"finds a name declared in an ancestor", "Vehicle::Car", "Engine", "Vehicle::Engine"},
		{"a qualified path works regardless of from", "Vehicle::Car", "Vehicle::Engine", "Vehicle::Engine"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got, ok := model.Resolve(tt.from, tt.name)
			if !ok {
				t.Fatalf("Resolve(%q, %q): not found", tt.from, tt.name)
			}
			if got != tt.want {
				t.Errorf("Resolve(%q, %q) = %q, want %q", tt.from, tt.name, got, tt.want)
			}
		})
	}

	if _, ok := model.Resolve("Vehicle::Car", "NoSuchThing"); ok {
		t.Error("Resolve found a name that doesn't exist, want not found")
	}
}

// TestQualifiedName uses IDs that deliberately don't look like qualified
// names, to prove QualifiedName is derived by walking Owner/Name rather
// than just handed back the ID string — which happens to look identical
// today, since FromAST currently derives IDs the same way it computes
// qualified names.
func TestQualifiedName(t *testing.T) {
	model := &metamodel.Model{
		Elements: map[metamodel.ElementID]*metamodel.Element{
			"1": {ID: "1", Kind: metamodel.KindPackage, Name: "Vehicle"},
			"2": {ID: "2", Kind: metamodel.KindPartDef, Name: "Car", Owner: "1"},
			"3": {ID: "3", Kind: metamodel.KindPartUsage, Name: "engine", Owner: "2"},
		},
	}

	want := map[metamodel.ElementID]string{
		"1": "Vehicle",
		"2": "Vehicle::Car",
		"3": "Vehicle::Car::engine",
	}

	for id, wantName := range want {
		if got := model.QualifiedName(id); got != wantName {
			t.Errorf("QualifiedName(%q) = %q, want %q", id, got, wantName)
		}
	}

	if got := model.QualifiedName("missing"); got != "" {
		t.Errorf("QualifiedName(missing) = %q, want %q", got, "")
	}
}

// TestQualifiedNameMatchesID pins down today's coincidence: FromAST
// derives ElementIDs from qualified names, so for now the two agree for
// every element. If ID generation ever changes (e.g. to opaque IDs that
// survive renames), this test is the one that should start failing.
func TestQualifiedNameMatchesID(t *testing.T) {
	m := sysml.NewModel(`package Vehicle {
		part def Engine;
		part def Car {
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

	for id := range model.Elements {
		if got := model.QualifiedName(id); got != string(id) {
			t.Errorf("QualifiedName(%q) = %q, want %q", id, got, string(id))
		}
	}
}

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

// TestFromASTQualifiedTypeReachesSiblingScope is the same shape as
// TestFromASTScopingRestrictsVisibility, except Bike spells the reference
// out as a fully-qualified path. Bare-name resolution still couldn't see
// into Car from Bike, but an absolute path bypasses scope entirely, so
// this must succeed where the bare-name version correctly fails.
func TestFromASTQualifiedTypeReachesSiblingScope(t *testing.T) {
	m := sysml.NewModel(`package Vehicle {
		part def Car {
			part def Wheel;
		}
		part def Bike {
			part wheel : Vehicle::Car::Wheel;
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

	typeID, ok := typeOf(model, "Vehicle::Bike::wheel")
	if !ok {
		t.Fatal("no TypedBy relationship found for Vehicle::Bike::wheel")
	}
	if typeID != "Vehicle::Car::Wheel" {
		t.Errorf("usage type = %q, want %q", typeID, "Vehicle::Car::Wheel")
	}
}

func TestFromASTUnresolvedQualifiedType(t *testing.T) {
	cases := map[string]string{
		"wrong root":      `package Vehicle { part engine : Nope::Engine; }`,
		"missing segment": `package Vehicle { part def Car {} part engine : Vehicle::Car::Nope; }`,
	}

	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			m := sysml.NewModel(source)

			pkg, err := m.Parse()
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			if _, err := metamodel.FromAST(pkg); err == nil {
				t.Error("expected an error for an unresolved qualified type, got nil")
			}
		})
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
