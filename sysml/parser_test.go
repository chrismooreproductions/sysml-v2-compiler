package sysml_test

import (
	"testing"

	"github.com/chrismooreproductions/sysml-modeller/sysml"
)

// parseTopLevelPackage parses source, expecting it to produce a root
// Namespace containing exactly one member, a *sysml.Package -- the common
// shape most of this file's fixtures use. Fails the test otherwise.
func parseTopLevelPackage(t *testing.T, source string) *sysml.Package {
	t.Helper()

	ns, err := sysml.NewModel(source).Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ns.Members) != 1 {
		t.Fatalf("got %d top-level members, want 1", len(ns.Members))
	}

	pkg, ok := ns.Members[0].(*sysml.Package)
	if !ok {
		t.Fatalf("top-level member is %T, want *sysml.Package", ns.Members[0])
	}
	return pkg
}

func TestModelParse(t *testing.T) {
	pkg := parseTopLevelPackage(t, vehicleModel)

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
	pkg := parseTopLevelPackage(t, `package Vehicle {
		part def Car {
			part def Wheel;
		}
		part def Bike {
			part wheel : Vehicle::Car::Wheel;
		}
	}`)

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
			pkg := parseTopLevelPackage(t, `package Vehicle { `+tt.source+` }`)

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
	pkg := parseTopLevelPackage(t, `package Car {
		import Vehicle;
		part def Chassis;
	}`)

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
	pkg := parseTopLevelPackage(t, `package Car { import Vehicle::Electrical; }`)

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
	pkg := parseTopLevelPackage(t, `// leading comment
	package Vehicle { // trailing comment
		/* a block
		   comment */
		part def Engine; // another one
		part engine /* inline */ : Engine;
	}`)

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

// TestModelParseRootNamespace checks that a model's root can directly
// contain multiple top-level members -- packages and a bare definition --
// rather than requiring exactly one wrapping package. The grammar's root is
// RootNamespace (PackageBodyElement*), not a single Package.
func TestModelParseRootNamespace(t *testing.T) {
	ns, err := sysml.NewModel(`
		package P1 {
			part def A;
		}
		package P2 {
			part a : P1::A;
		}
		part def TopLevel;
	`).Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ns.Members) != 3 {
		t.Fatalf("got %d top-level members, want 3", len(ns.Members))
	}

	p1, ok := ns.Members[0].(*sysml.Package)
	if !ok || p1.Name != "P1" {
		t.Fatalf("member 0 = %+v, want Package{Name: P1}", ns.Members[0])
	}

	p2, ok := ns.Members[1].(*sysml.Package)
	if !ok || p2.Name != "P2" {
		t.Fatalf("member 1 = %+v, want Package{Name: P2}", ns.Members[1])
	}

	topLevel, ok := ns.Members[2].(*sysml.Definition)
	if !ok || topLevel.Name != "TopLevel" {
		t.Fatalf("member 2 = %+v, want Definition{Name: TopLevel}", ns.Members[2])
	}
}

// TestModelParseEmptySource checks that an empty (or all-whitespace) source
// parses successfully to a Namespace with no members, rather than erroring
// -- the grammar's RootNamespace is PackageBodyElement*, zero repetitions
// included, so there's nothing to require here.
func TestModelParseEmptySource(t *testing.T) {
	for name, source := range map[string]string{"empty": "", "whitespace only": "  \n\t "} {
		t.Run(name, func(t *testing.T) {
			ns, err := sysml.NewModel(source).Parse()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ns.Members) != 0 {
				t.Errorf("got %d members, want 0", len(ns.Members))
			}
		})
	}
}

func TestModelParseErrors(t *testing.T) {
	cases := map[string]string{
		"unterminated package body":        "package Vehicle {",
		"missing package braces":           "package Vehicle",
		"part usage missing colon":         "package Vehicle { part Engine; }",
		"part usage missing type":          "package Vehicle { part engine : ; }",
		"unmatched closing brace":          "package Vehicle { } }",
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
