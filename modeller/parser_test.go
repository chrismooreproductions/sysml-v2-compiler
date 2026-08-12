package modeller_test

import (
	"testing"

	"github.com/chrismooreproductions/sysml-modeller/modeller"
)

func TestModelParse(t *testing.T) {
	m := modeller.NewModel(vehicleModel)

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

	engine, ok := pkg.Members[0].(*modeller.PartDef)
	if !ok {
		t.Fatalf("member 0 is %T, want *modeller.PartDef", pkg.Members[0])
	}
	if engine.Name != "Engine" || len(engine.Members) != 0 {
		t.Errorf("member 0 = %+v, want PartDef{Name: Engine, no members}", engine)
	}

	car, ok := pkg.Members[1].(*modeller.PartDef)
	if !ok {
		t.Fatalf("member 1 is %T, want *modeller.PartDef", pkg.Members[1])
	}
	if car.Name != "Car" {
		t.Errorf("member 1 name = %q, want %q", car.Name, "Car")
	}
	if len(car.Members) != 1 {
		t.Fatalf("Car has %d members, want 1", len(car.Members))
	}

	engineUsage, ok := car.Members[0].(*modeller.PartUsage)
	if !ok {
		t.Fatalf("Car member 0 is %T, want *modeller.PartUsage", car.Members[0])
	}
	if engineUsage.Name != "engine" || engineUsage.Type != "Engine" {
		t.Errorf("Car member 0 = %+v, want PartUsage{Name: engine, Type: Engine}", engineUsage)
	}
}

func TestModelParseErrors(t *testing.T) {
	cases := map[string]string{
		"empty source":              "",
		"missing package keyword":   "part def Engine;",
		"unterminated package body": "package Vehicle {",
		"missing package braces":    "package Vehicle",
		"part usage missing colon":  "package Vehicle { part Engine; }",
		"part usage missing type":   "package Vehicle { part engine : ; }",
		"trailing garbage":          "package Vehicle { } part def Engine;",
	}

	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			m := modeller.NewModel(source)
			if _, err := m.Parse(); err == nil {
				t.Errorf("Parse(%q): expected error, got nil", source)
			}
		})
	}
}
