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

	engine, ok := pkg.Members[0].(*sysml.PartDef)
	if !ok {
		t.Fatalf("member 0 is %T, want *sysml.PartDef", pkg.Members[0])
	}
	if engine.Name != "Engine" || len(engine.Members) != 0 {
		t.Errorf("member 0 = %+v, want PartDef{Name: Engine, no members}", engine)
	}

	car, ok := pkg.Members[1].(*sysml.PartDef)
	if !ok {
		t.Fatalf("member 1 is %T, want *sysml.PartDef", pkg.Members[1])
	}
	if car.Name != "Car" {
		t.Errorf("member 1 name = %q, want %q", car.Name, "Car")
	}
	if len(car.Members) != 1 {
		t.Fatalf("Car has %d members, want 1", len(car.Members))
	}

	engineUsage, ok := car.Members[0].(*sysml.PartUsage)
	if !ok {
		t.Fatalf("Car member 0 is %T, want *sysml.PartUsage", car.Members[0])
	}
	if engineUsage.Name != "engine" || engineUsage.Type != "Engine" {
		t.Errorf("Car member 0 = %+v, want PartUsage{Name: engine, Type: Engine}", engineUsage)
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

	bike, ok := pkg.Members[1].(*sysml.PartDef)
	if !ok {
		t.Fatalf("member 1 is %T, want *sysml.PartDef", pkg.Members[1])
	}

	wheelUsage, ok := bike.Members[0].(*sysml.PartUsage)
	if !ok {
		t.Fatalf("Bike member 0 is %T, want *sysml.PartUsage", bike.Members[0])
	}
	if wheelUsage.Type != "Vehicle::Car::Wheel" {
		t.Errorf("wheel usage type = %q, want %q", wheelUsage.Type, "Vehicle::Car::Wheel")
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
