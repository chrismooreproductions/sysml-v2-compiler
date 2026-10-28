// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"reflect"
	"testing"
)

const constraintSource = `package P {
    requirement def MassReq {
        attribute massActual : ScalarValues::Integer;
        attribute massLimit : ScalarValues::Integer = 2000;
        assume constraint { massLimit > 0 }
        require constraint { massActual <= massLimit }
    }
    requirement r : MassReq { attribute :>> massActual = 1800; }
    requirement r2 : MassReq { attribute :>> massActual = 2500; }
    requirement r3 : MassReq;
    requirement r4 : MassReq { attribute :>> massActual = 1800; attribute :>> massLimit = 1000; }
    constraint def Ok { in attribute x : ScalarValues::Integer; x > 0 }
    constraint plain : Ok { in attribute :>> x = 5; }
    constraint notBool { 1 + 1 }
    calc def Range {
        in attribute tankSize : ScalarValues::Integer;
        in attribute consumption : ScalarValues::Integer;
        return attribute range : ScalarValues::Integer = tankSize / consumption;
    }
    calc rc : Range { in attribute :>> tankSize = 40; in attribute :>> consumption = 8; }
    calc def Loop { in attribute n : ScalarValues::Integer; return attribute m : ScalarValues::Integer = Loop(n); }
    calc def Doubled { in attribute x : ScalarValues::Integer; x * 2 }
    part def Pt;
    attribute inv = Range(40, 8);
    attribute inv2 = Range(tankSize = 80, consumption = 8);
    attribute inv3 = Range(40);
    attribute inv4 = Range(40, 8, 2);
    attribute looped = Loop(1);
    attribute twice = Doubled(21);
    attribute nested = Range(Doubled(20), 4) + 1;
}`

func TestEvalRequirement(t *testing.T) {
	w, _, diags := checked(t, "c.sysml", []byte(constraintSource))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	// "kind result" for each constraint, in order
	outcomes := func(name string) []string {
		var out []string
		for _, o := range w.EvalRequirement(w.Lookup(name)) {
			out = append(out, o.Kind+" "+o.Result.String())
		}
		return out
	}
	for name, want := range map[string][]string{
		// the usage gives the definition's attribute a value
		"P::r":  {"assume true", "require true"},
		"P::r2": {"assume true", "require false"},
		// no value for massActual: not determined, and it says which
		"P::r3": {"assume true", "require not determined: P::MassReq::massActual has no value"},
		// the usage overrides the default the definition gives massLimit
		"P::r4": {"assume true", "require false"},
		// the definition alone
		"P::MassReq": {"assume true", "require not determined: P::MassReq::massActual has no value"},
	} {
		if got := outcomes(name); !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n got %q\nwant %q", name, got, want)
		}
	}
	for _, o := range w.EvalRequirement(w.Lookup("P::r3")) {
		if o.Constraint == nil || o.Result.Source == nil || o.Result.Span.Start.Line == 0 {
			t.Errorf("no provenance: %+v", o)
		}
	}
	if w.EvalRequirement(w.Lookup("P::Range")) != nil || w.EvalRequirement(nil) != nil {
		t.Error("only a requirement has outcomes")
	}
}

func TestEvalConstraint(t *testing.T) {
	w, _, _ := checked(t, "c.sysml", []byte(constraintSource))
	for _, c := range []struct {
		what   string
		got    Result
		status Status
		value  any
		reason string
	}{
		{"a usage is its own context", w.EvalConstraint(w.Lookup("P::plain"), nil), Determined, true, ""},
		{"a definition has no values", w.EvalConstraint(w.Lookup("P::Ok"), nil), NotDetermined, nil, "P::Ok::x has no value"},
		{"a constraint is a boolean", w.EvalConstraint(w.Lookup("P::notBool"), nil), NotEvaluable, nil, "a constraint must be a boolean, not integer"},
		{"not a constraint", w.EvalConstraint(w.Lookup("P::Range"), nil), NotEvaluable, nil, "not a constraint"},
		{"nothing", w.EvalConstraint(nil, nil), NotEvaluable, nil, "not a constraint"},
	} {
		if c.got.Status != c.status || c.got.Value != c.value || c.got.Reason != c.reason {
			t.Errorf("%s: %+v", c.what, c.got)
		}
	}
	// the same constraint for two requirements
	c := w.EvalRequirement(w.Lookup("P::r"))[1].Constraint
	if a, b := w.EvalConstraint(c, w.Lookup("P::r")), w.EvalConstraint(c, w.Lookup("P::r2")); a.Value != true || b.Value != false {
		t.Errorf("r: %v, r2: %v", a, b)
	}
}

func TestEvalCalculation(t *testing.T) {
	w, _, _ := checked(t, "c.sysml", []byte(constraintSource))
	type in = map[string]any
	for _, c := range []struct {
		what   string
		calc   string
		inputs in
		want   string
	}{
		{"a usage binds its inputs", "P::rc", nil, "5"},
		{"and the caller can override one", "P::rc", in{"consumption": 4}, "10"},
		{"a definition needs its inputs", "P::Range", nil, "not determined: P::Range::tankSize has no value"},
		{"given", "P::Range", in{"tankSize": 40, "consumption": 10}, "4"},
		{"a real result", "P::Range", in{"tankSize": 7, "consumption": 2}, "3.5"},
		{"a float input", "P::Range", in{"tankSize": 9.0, "consumption": 2}, "4.5"},
		{"one missing", "P::Range", in{"tankSize": 40}, "not determined: P::Range::consumption has no value"},
		{"no such input", "P::Range", in{"nope": 1}, "not evaluable: P::Range has no input nope"},
		{"an input of the wrong kind", "P::Range", in{"tankSize": []int{1}}, "not evaluable: input tankSize is a []int, not a boolean, number or string"},
		{"a result expression", "P::Doubled", in{"x": 4}, "8"},
		{"division by zero is not a value", "P::Range", in{"tankSize": 1, "consumption": 0}, "not evaluable: cannot apply / to 1 and 0"},
		{"not a calculation", "P::Pt", nil, "not evaluable: not a calculation"},
	} {
		if got := w.EvalCalculation(w.Lookup(c.calc), c.inputs).String(); got != c.want {
			t.Errorf("%s: %q, want %q", c.what, got, c.want)
		}
	}
}

func TestEvalInvocationOfAModelsOwnCalculation(t *testing.T) {
	w, doc, _ := checked(t, "c.sysml", []byte(constraintSource))
	got := map[string]string{}
	doc.Root.Walk(func(e *Element) {
		if !e.IsA("AttributeUsage") || e.Text("declaredName") == "" {
			return
		}
		for _, r := range e.Children("ownedRelationship") {
			if r.IsA("FeatureValue") {
				for _, x := range r.Children("ownedRelatedElement") {
					got[e.Text("declaredName")] = w.Eval(x).String()
				}
			}
		}
	})
	for name, want := range map[string]string{
		"inv":    "5",                                                  // Range(40, 8)
		"inv2":   "10",                                                 // named arguments
		"inv3":   "not determined: P::Range::consumption has no value", // an argument left out
		"inv4":   "not evaluable: P::Range takes 2 inputs, got more",   // too many
		"looped": "not evaluable: P::Loop calls itself too deeply",     // no value is guessed
		"twice":  "42",                                                 // a result expression
		"nested": "11",                                                 // Range(Doubled(20), 4) + 1
	} {
		if got[name] != want {
			t.Errorf("%s: %q, want %q", name, got[name], want)
		}
	}
}

// A requirement whose attribute is bound to a value held on nested parts
// (ac.navigation.positionErrorM) is evaluated through the chain.
func TestEvalRequirementThroughAFeatureChain(t *testing.T) {
	w, _, diags := checked(t, "chain.sysml", []byte(`package T {
    private import ScalarValues::*;
    part def Nav { attribute positionErrorM : Real = 5.0; }
    part def Eng { attribute thrust : Real = 3.0; }
    part def Ac  { part navigation : Nav; part engines : Eng [2]; part spares : Eng [0..*]; }
    part ac : Ac;
    part ac7 : Ac { part :>> navigation { attribute :>> positionErrorM = 7.0; } }

    requirement def R;
    requirement positionAccuracy : R {
        attribute positionErrorM : Real = ac.navigation.positionErrorM;
        require constraint { positionErrorM <= 10.0 }
    }
    requirement redefined : R {
        attribute e : Real = ac7.navigation.positionErrorM;
        require constraint { e == 7.0 }
    }
    requirement sequence : R {
        attribute t : Real = ac.engines.thrust;
        require constraint { t <= 6.0 }
    }
    requirement unbounded : R {
        attribute t : Real = ac.spares.thrust;
        require constraint { t <= 6.0 }
    }
    satisfy positionAccuracy by ac;
}`))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	for name, want := range map[string]string{
		"T::positionAccuracy": "require true",
		"T::redefined":        "require true",
		"T::sequence":         "require not evaluable: engines has a multiplicity that allows 2 values, so ac.engines is a sequence of values, not one",
		"T::unbounded":        "require not evaluable: spares has an unbounded multiplicity, so ac.spares is a sequence of values, not one",
	} {
		outs := w.EvalRequirement(w.Lookup(name))
		if len(outs) != 1 {
			t.Fatalf("%s: %d outcomes", name, len(outs))
		}
		if got := outs[0].Kind + " " + outs[0].Result.String(); got != want {
			t.Errorf("%s:\n got %q\nwant %q", name, got, want)
		}
	}
}

// A satisfy gives a requirement its subject, and the attributes it redefines,
// for the constraints of the satisfy itself: s.x is ac.x, and a value redefined
// in the satisfy is seen. The requirement alone has neither.
func TestEvalASatisfyBindsTheSubject(t *testing.T) {
	w, doc, diags := checked(t, "s.sysml", []byte(`package T {
    private import ScalarValues::*;
    part def Ac  { attribute x : Real; }
    part ac : Ac { attribute :>> x = 4.0; }
    requirement def R { attribute x : Real; require constraint { x <= 10.0 } }
    requirement viaSatisfy : R;
    satisfy viaSatisfy by ac { attribute :>> x = ac.x; }
    requirement bySubject : R { subject s : Ac; attribute :>> x = s.x; }
    satisfy bySubject by ac;
}`))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	var satisfies []*Element
	doc.Root.Walk(func(e *Element) {
		if e.IsA("SatisfyRequirementUsage") {
			satisfies = append(satisfies, e)
		}
	})
	if len(satisfies) != 2 {
		t.Fatalf("%d satisfy usages", len(satisfies))
	}
	for i, s := range satisfies {
		outs := w.EvalRequirement(s)
		if len(outs) != 1 || outs[0].Result.String() != "true" {
			t.Errorf("satisfy %d: %+v", i, outs)
		}
	}
	// the requirement on its own has no subject to take the value from
	if outs := w.EvalRequirement(w.Lookup("T::bySubject")); len(outs) != 1 || outs[0].Result.Status == Determined {
		t.Errorf("the requirement alone: %+v", outs)
	}
}
