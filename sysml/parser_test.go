package sysml_test

import (
	"testing"

	"github.com/chrismooreproductions/sysml-modeller/sysml"
)

func TestModelParse(t *testing.T) {
	m := sysml.NewModel(vehicleModel)

	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pkg.Name != "Vehicle" {
		t.Errorf("package name = %q, want %q", pkg.Name, "Vehicle")
	}

	if len(pkg.Members) != 2 {
		t.Fatalf("got %d members, want 2", len(pkg.Members))
	}

	engine, ok := pkg.Members[0].(*sysml.Definition)
	if !ok {
		t.Fatalf("member 0 is %T, want *sysml.Definition", pkg.Members[0])
	}
	if engine.Name != "Engine" || len(engine.Members) != 0 {
		t.Errorf("member 0 = %+v, want Definition{Name: Engine, no members}", engine)
	}

	car, ok := pkg.Members[1].(*sysml.Definition)
	if !ok {
		t.Fatalf("member 1 is %T, want *sysml.Definition", pkg.Members[1])
	}
	if car.Name != "Car" {
		t.Errorf("member 1 name = %q, want %q", car.Name, "Car")
	}
	if len(car.Members) != 1 {
		t.Fatalf("Car has %d members, want 1", len(car.Members))
	}

	engineUsage, ok := car.Members[0].(*sysml.Usage)
	if !ok {
		t.Fatalf("Car member 0 is %T, want *sysml.Usage", car.Members[0])
	}
	if engineUsage.Name != "engine" || engineUsage.Type != "Engine" {
		t.Errorf("Car member 0 = %+v, want Usage{Name: engine, Type: Engine}", engineUsage)
	}
}

func TestModelParseQualifiedType(t *testing.T) {
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
		t.Fatalf("unexpected error: %v", err)
	}

	bike, ok := pkg.Members[1].(*sysml.Definition)
	if !ok {
		t.Fatalf("member 1 is %T, want *sysml.Definition", pkg.Members[1])
	}

	wheelUsage, ok := bike.Members[0].(*sysml.Usage)
	if !ok {
		t.Fatalf("Bike member 0 is %T, want *sysml.Usage", bike.Members[0])
	}
	if wheelUsage.Type != "Vehicle::Car::Wheel" {
		t.Errorf("wheel usage type = %q, want %q", wheelUsage.Type, "Vehicle::Car::Wheel")
	}
}

func TestModelParseMultiplicity(t *testing.T) {
	cases := map[string]struct {
		source string
		want   *sysml.Multiplicity
	}{
		"unbounded star":     {`part combatants : Combatant[*];`, &sysml.Multiplicity{Lower: 0, Upper: sysml.Unbounded}},
		"exact count":        {`part combatants : Combatant[3];`, &sysml.Multiplicity{Lower: 3, Upper: 3}},
		"bounded range":      {`part combatants : Combatant[0..5];`, &sysml.Multiplicity{Lower: 0, Upper: 5}},
		"lower to unbounded": {`part combatants : Combatant[1..*];`, &sysml.Multiplicity{Lower: 1, Upper: sysml.Unbounded}},
		"no multiplicity":    {`part combatants : Combatant;`, nil},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			m := sysml.NewModel(`package Vehicle { ` + tt.source + ` }`)

			pkg, err := m.Parse()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			usage, ok := pkg.Members[0].(*sysml.Usage)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
			}

			if tt.want == nil {
				if usage.Multiplicity != nil {
					t.Errorf("Multiplicity = %+v, want nil", usage.Multiplicity)
				}
				return
			}

			if usage.Multiplicity == nil || *usage.Multiplicity != *tt.want {
				t.Errorf("Multiplicity = %+v, want %+v", usage.Multiplicity, tt.want)
			}
		})
	}
}

func TestModelParseImport(t *testing.T) {
	m := sysml.NewModel(`package Car {
		import Vehicle;
		part def Chassis;
	}`)

	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pkg.Members) != 2 {
		t.Fatalf("got %d members, want 2", len(pkg.Members))
	}

	imp, ok := pkg.Members[0].(*sysml.Import)
	if !ok {
		t.Fatalf("member 0 is %T, want *sysml.Import", pkg.Members[0])
	}
	if imp.Path != "Vehicle" {
		t.Errorf("import path = %q, want %q", imp.Path, "Vehicle")
	}

	if _, ok := pkg.Members[1].(*sysml.Definition); !ok {
		t.Fatalf("member 1 is %T, want *sysml.Definition", pkg.Members[1])
	}
}

func TestModelParseImportQualifiedPath(t *testing.T) {
	m := sysml.NewModel(`package Car { import Vehicle::Electrical; }`)

	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	imp, ok := pkg.Members[0].(*sysml.Import)
	if !ok {
		t.Fatalf("member 0 is %T, want *sysml.Import", pkg.Members[0])
	}
	if imp.Path != "Vehicle::Electrical" {
		t.Errorf("import path = %q, want %q", imp.Path, "Vehicle::Electrical")
	}
}

// TestModelParseIgnoresComments checks that line and block comments can
// appear anywhere insignificant whitespace can -- between members, inside a
// member's body, even splitting a declaration across lines -- without
// affecting the parsed result.
func TestModelParseIgnoresComments(t *testing.T) {
	m := sysml.NewModel(`// leading comment
	package Vehicle { // trailing comment
		/* a block
		   comment */
		part def Engine; // another one
		part engine /* inline */ : Engine;
	}`)

	pkg, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pkg.Name != "Vehicle" {
		t.Errorf("package name = %q, want %q", pkg.Name, "Vehicle")
	}
	if len(pkg.Members) != 2 {
		t.Fatalf("got %d members, want 2", len(pkg.Members))
	}

	if _, ok := pkg.Members[0].(*sysml.Definition); !ok {
		t.Fatalf("member 0 is %T, want *sysml.Definition", pkg.Members[0])
	}

	usage, ok := pkg.Members[1].(*sysml.Usage)
	if !ok {
		t.Fatalf("member 1 is %T, want *sysml.Usage", pkg.Members[1])
	}
	if usage.Name != "engine" || usage.Type != "Engine" {
		t.Errorf("member 1 = %+v, want Usage{Name: engine, Type: Engine}", usage)
	}
}

func TestModelParseErrors(t *testing.T) {
	cases := map[string]string{
		"empty source":                     "",
		"missing package keyword":          "part def Engine;",
		"unterminated package body":        "package Vehicle {",
		"missing package braces":           "package Vehicle",
		"part usage missing colon":         "package Vehicle { part Engine; }",
		"part usage missing type":          "package Vehicle { part engine : ; }",
		"trailing garbage":                 "package Vehicle { } part def Engine;",
		"qualified type missing segment":   "package Vehicle { part engine : Vehicle::; }",
		"qualified type trailing path sep": "package Vehicle { part engine : Vehicle::Engine::; }",
		"import missing path":              "package Vehicle { import; }",
		"import missing semicolon":         "package Vehicle { import Engine }",
		"multiplicity missing bound":       "package Vehicle { part combatants : Combatant[]; }",
		"multiplicity missing close":       "package Vehicle { part combatants : Combatant[*; }",
		"multiplicity non-numeric bound":   "package Vehicle { part combatants : Combatant[abc]; }",
		"multiplicity negative bound":      "package Vehicle { part combatants : Combatant[-1]; }",
	}

	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			m := sysml.NewModel(source)
			if _, err := m.Parse(); err == nil {
				t.Errorf("Parse(%q): expected error, got nil", source)
			}
		})
	}
}
