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

// TestFromASTMultiplicity checks that a PartUsage's bracketed multiplicity
// (e.g. "[*]") carries over onto its Element, and that a usage with no
// bracket at all still ends up with a nil Multiplicity.
func TestFromASTMultiplicity(t *testing.T) {
	source := `package Battlefield {
		part def Combatant;

		part solo : Combatant;
		part friendlyCombatants : Combatant[*];
		part enemyCombatants : Combatant[1..*];
		part squad : Combatant[5];
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

	tests := []struct {
		id   metamodel.ElementID
		want *metamodel.Multiplicity
	}{
		{"Battlefield::solo", nil},
		{"Battlefield::friendlyCombatants", &metamodel.Multiplicity{Lower: 0, Upper: metamodel.Unbounded}},
		{"Battlefield::enemyCombatants", &metamodel.Multiplicity{Lower: 1, Upper: metamodel.Unbounded}},
		{"Battlefield::squad", &metamodel.Multiplicity{Lower: 5, Upper: 5}},
	}

	for _, tt := range tests {
		el, ok := model.Elements[tt.id]
		if !ok {
			t.Fatalf("missing element %q", tt.id)
		}

		if tt.want == nil {
			if el.Multiplicity != nil {
				t.Errorf("%s: Multiplicity = %+v, want nil", tt.id, el.Multiplicity)
			}
			continue
		}

		if el.Multiplicity == nil || *el.Multiplicity != *tt.want {
			t.Errorf("%s: Multiplicity = %+v, want %+v", tt.id, el.Multiplicity, tt.want)
		}
	}
}

// TestFromASTNestedPackage checks that a package nested inside another
// package (rather than only inside a PartDef) is declared as its own
// namespace, containment-linked to its enclosing package via Owner just
// like any other member.
func TestFromASTNestedPackage(t *testing.T) {
	source := `package Car {
		package Engine {
			part def Cylinder;
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

	want := map[metamodel.ElementID]metamodel.Element{
		"Car":                   {ID: "Car", Kind: metamodel.KindPackage, Name: "Car"},
		"Car::Engine":           {ID: "Car::Engine", Kind: metamodel.KindPackage, Name: "Engine", Owner: "Car"},
		"Car::Engine::Cylinder": {ID: "Car::Engine::Cylinder", Kind: metamodel.KindPartDef, Name: "Cylinder", Owner: "Car::Engine"},
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

// TestFromASTRelativeQualifiedTypeReachesSiblingScope is the case
// resolveQualifiedFrom exists for: Cabin references "Powertrain::Engine",
// not the full "Vehicle::Powertrain::Engine" path from Root. A bare name
// still couldn't see into Powertrain from Cabin (they're siblings), and the
// old absolute-only qualified resolution couldn't either, since the path's
// first segment ("Powertrain") doesn't match Root's own name ("Vehicle").
// This only resolves because resolveQualifiedFrom checks from's own scope
// chain -- here, Cabin's owner Vehicle's children -- before falling back to
// an absolute path.
func TestFromASTRelativeQualifiedTypeReachesSiblingScope(t *testing.T) {
	m := sysml.NewModel(`package Vehicle {
		part def Powertrain {
			part def Engine;
		}
		part def Cabin {
			part engine : Powertrain::Engine;
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

	typeID, ok := typeOf(model, "Vehicle::Cabin::engine")
	if !ok {
		t.Fatal("no TypedBy relationship found for Vehicle::Cabin::engine")
	}
	if typeID != "Vehicle::Powertrain::Engine" {
		t.Errorf("usage type = %q, want %q", typeID, "Vehicle::Powertrain::Engine")
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

// vehicleModel compiles the standalone Vehicle package used by the import
// tests below as the thing being imported.
func vehicleModel(t *testing.T) *metamodel.Model {
	t.Helper()

	m := sysml.NewModel(`package Vehicle {
		part def Engine;
	}`)
	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	model, err := metamodel.FromAST(pkg)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}
	return model
}

// TestFromASTWithImports_ResolvesQualifiedTypeAcrossModels is the actual
// cross-model case: Car references Vehicle::Engine, a type declared in a
// wholly separate, already-translated Model, not anywhere in Car's own AST.
func TestFromASTWithImports_ResolvesQualifiedTypeAcrossModels(t *testing.T) {
	vehicle := vehicleModel(t)

	m := sysml.NewModel(`package Car {
		import Vehicle;
		part def Chassis {
			part engine : Vehicle::Engine;
		}
	}`)
	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromASTWithImports(pkg, map[string]*metamodel.Model{"Vehicle": vehicle})
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	typeID, ok := typeOf(model, "Car::Chassis::engine")
	if !ok {
		t.Fatal("no TypedBy relationship found for Car::Chassis::engine")
	}
	if typeID != "Vehicle::Engine" {
		t.Errorf("usage type = %q, want %q", typeID, "Vehicle::Engine")
	}

	// The imported Element itself lives in vehicle's own Elements map, not
	// copied into model's -- resolving the ID further means knowing to look
	// there.
	if _, ok := model.Elements[typeID]; ok {
		t.Error("imported element should not be copied into the importing model's Elements")
	}
	if _, ok := vehicle.Elements[typeID]; !ok {
		t.Error("imported element should still be found in the imported model's own Elements")
	}
}

// TestFromASTWithImports_MissingImportIsAnError checks that referencing an
// import the caller never supplied a Model for fails translation, the same
// way an unresolved type does -- it's a translation-time error, not a
// resolution-time "not found" the caller has to check for separately.
func TestFromASTWithImports_MissingImportIsAnError(t *testing.T) {
	m := sysml.NewModel(`package Car {
		import Vehicle;
	}`)
	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if _, err := metamodel.FromASTWithImports(pkg, nil); err == nil {
		t.Error("expected an error for an import with no supplied Model, got nil")
	}
}

// TestFromASTWithImports_UnimportedQualifiedTypeStillFails checks that a
// qualified reference to another package doesn't silently succeed just
// because some import happens to be in scope -- only the imports actually
// declared are consulted.
func TestFromASTWithImports_UnimportedQualifiedTypeStillFails(t *testing.T) {
	vehicle := vehicleModel(t)

	m := sysml.NewModel(`package Car {
		part engine : Vehicle::Engine;
	}`)
	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Vehicle is compiled and available, but never imported by Car.
	if _, err := metamodel.FromASTWithImports(pkg, map[string]*metamodel.Model{"Vehicle": vehicle}); err == nil {
		t.Error("expected an error resolving a type from a package that was never imported, got nil")
	}
}

// environmentModel and combatantsModel compile the two standalone packages
// TestFromASTWithImports_Battlefield imports, mirroring vehicleModel above.
func environmentModel(t *testing.T) *metamodel.Model {
	t.Helper()

	m := sysml.NewModel(`package Environment {
		part def Terrain;
	}`)
	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	model, err := metamodel.FromAST(pkg)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}
	return model
}

func combatantsModel(t *testing.T) *metamodel.Model {
	t.Helper()

	m := sysml.NewModel(`package Combatants {
		part def Combatant;
	}`)
	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	model, err := metamodel.FromAST(pkg)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}
	return model
}

// TestFromASTWithImports_Battlefield combines two features at once: a
// model importing from two separate packages at once (not just one), and a
// multiplicity ([*]) on a usage whose type itself comes from an import --
// checking that Multiplicity propagates onto the Element the same way
// regardless of whether its TypedBy relationship resolves locally or
// through an import.
func TestFromASTWithImports_Battlefield(t *testing.T) {
	environment := environmentModel(t)
	combatants := combatantsModel(t)

	m := sysml.NewModel(`package Battlefield {
		import Environment;
		import Combatants;

		part terrain : Environment::Terrain;
		part squad : Combatants::Combatant[*];
	}`)
	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromASTWithImports(pkg, map[string]*metamodel.Model{
		"Environment": environment,
		"Combatants":  combatants,
	})
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	terrainType, ok := typeOf(model, "Battlefield::terrain")
	if !ok {
		t.Fatal("no TypedBy relationship found for Battlefield::terrain")
	}
	if terrainType != "Environment::Terrain" {
		t.Errorf("terrain usage type = %q, want %q", terrainType, "Environment::Terrain")
	}

	squadType, ok := typeOf(model, "Battlefield::squad")
	if !ok {
		t.Fatal("no TypedBy relationship found for Battlefield::squad")
	}
	if squadType != "Combatants::Combatant" {
		t.Errorf("squad usage type = %q, want %q", squadType, "Combatants::Combatant")
	}

	squad, ok := model.Elements["Battlefield::squad"]
	if !ok {
		t.Fatal("missing element Battlefield::squad")
	}
	want := &metamodel.Multiplicity{Lower: 0, Upper: metamodel.Unbounded}
	if squad.Multiplicity == nil || *squad.Multiplicity != *want {
		t.Errorf("squad Multiplicity = %+v, want %+v", squad.Multiplicity, want)
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

// TestFromASTVehicleMultiPackage is a larger, realistic integration case
// beyond the smaller targeted tests above: four sibling subsystem packages
// (Powertrain, Chassis, Cabin, Electrical), each with their own part defs,
// and a Car -- itself a sibling of all four, not nested inside any of them
// -- composed entirely out of relative qualified references into each one
// (e.g. "Powertrain::Engine"). Every one of those only resolves because of
// resolveQualifiedFrom's ancestor-scope walk: the old absolute-only
// resolver would have rejected all of them, since none of their first
// segments match Vehicle (Root)'s own name. Also exercises multiplicity
// ([4], [*]) alongside cross-package resolution in the same model.
func TestFromASTVehicleMultiPackage(t *testing.T) {
	source := `package Vehicle {
		package Powertrain {
			part def Cylinder;
			part def Engine {
				part cylinders : Cylinder[4];
			}
			part def Transmission;
			part def Battery;

			part engine : Engine;
			part transmission : Transmission;
			part battery : Battery;
		}

		package Chassis {
			part def Wheel;
			part def Suspension;
			part def Brake;

			part frontLeftWheel : Wheel;
			part frontRightWheel : Wheel;
			part rearLeftWheel : Wheel;
			part rearRightWheel : Wheel;
			part suspension : Suspension[4];
			part brakes : Brake[4];
		}

		package Cabin {
			part def Seat;
			part def Dashboard;
			part def InfotainmentSystem;

			part driverSeat : Seat;
			part passengerSeats : Seat[4];
			part dashboard : Dashboard;
			part infotainment : InfotainmentSystem;
		}

		package Electrical {
			part def Wire;
			part def Sensor;
			part def ControlUnit;

			part sensors : Sensor[*];
			part ecu : ControlUnit;
		}

		part def Car {
			part engine : Powertrain::Engine;
			part transmission : Powertrain::Transmission;
			part battery : Powertrain::Battery;

			part frontLeftWheel : Chassis::Wheel;
			part frontRightWheel : Chassis::Wheel;
			part rearLeftWheel : Chassis::Wheel;
			part rearRightWheel : Chassis::Wheel;

			part driverSeat : Cabin::Seat;
			part passengerSeats : Cabin::Seat[4];
			part dashboard : Cabin::Dashboard;

			part ecu : Electrical::ControlUnit;
			part sensors : Electrical::Sensor[*];
		}

		part myCar : Car;
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

	if len(model.Elements) != 48 {
		t.Errorf("got %d elements, want 48", len(model.Elements))
	}
	if len(model.Relationships) != 29 {
		t.Errorf("got %d relationships, want 29", len(model.Relationships))
	}

	wantTypes := map[metamodel.ElementID]metamodel.ElementID{
		"Vehicle::Powertrain::Engine::cylinders": "Vehicle::Powertrain::Cylinder",
		"Vehicle::Car::engine":                   "Vehicle::Powertrain::Engine",
		"Vehicle::Car::frontLeftWheel":            "Vehicle::Chassis::Wheel",
		"Vehicle::Car::driverSeat":                "Vehicle::Cabin::Seat",
		"Vehicle::Car::ecu":                       "Vehicle::Electrical::ControlUnit",
		"Vehicle::Car::sensors":                   "Vehicle::Electrical::Sensor",
		"Vehicle::myCar":                          "Vehicle::Car",
	}
	for usage, want := range wantTypes {
		got, ok := typeOf(model, usage)
		if !ok {
			t.Errorf("no TypedBy relationship found for %s", usage)
			continue
		}
		if got != want {
			t.Errorf("%s typed by %q, want %q", usage, got, want)
		}
	}

	wantMultiplicity := map[metamodel.ElementID]metamodel.Multiplicity{
		"Vehicle::Powertrain::Engine::cylinders": {Lower: 4, Upper: 4},
		"Vehicle::Electrical::sensors":            {Lower: 0, Upper: metamodel.Unbounded},
	}
	for id, want := range wantMultiplicity {
		el, ok := model.Elements[id]
		if !ok {
			t.Errorf("missing element %s", id)
			continue
		}
		if el.Multiplicity == nil || *el.Multiplicity != want {
			t.Errorf("%s Multiplicity = %+v, want %+v", id, el.Multiplicity, want)
		}
	}
}
