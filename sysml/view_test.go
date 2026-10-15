// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"os"
	"reflect"
	"testing"
)

const viewSource = `package P {
    private import Views::*;
    private import SysML::*;
    metadata def Safety { attribute level : ScalarValues::Integer; }
    part def Vehicle {
        part wheels[4];
        part engine { part piston; }
        port fuelIn;
        @Safety { level = 3; }
        doc /* the vehicle */
        private part secret;
    }
    view def TreeView2;
    view def PartsView :> TreeView2 {
        filter @SysML::PartUsage;
    }
    view vehicleParts : PartsView {
        expose Vehicle::*;
    }
    view recursive : PartsView {
        expose Vehicle::**;
    }
    view one {
        expose Vehicle::wheels;
        expose Vehicle::engine;
    }
    view twice {
        expose Vehicle::*;
        expose Vehicle::wheels;
    }
    view specialized :> vehicleParts;
    view lib {
        expose ScalarValues::*;
    }
    part def Parts {
        part a { @Safety { level = 3; } }
        part b { @Safety { level = 1; } }
        part c;
        part d { @Safety; }
    }
    view lvl {
        expose Parts::*;
        filter Safety::level > 2;
    }
    view call {
        expose Parts::*;
        filter RealFunctions::sqrt(4.0) > 1;
    }
    view dangling {
        expose Nowhere::*;
    }
}`

func TestViewContent(t *testing.T) {
	w, _, diags := checked(t, "v.sysml", []byte(viewSource))
	// "dangling" is the one name that does not resolve
	if len(diags) != 1 || diags[0].Message != `cannot resolve "Nowhere" (searched from P::dangling)` {
		t.Fatal(diags)
	}
	content := func(name string) (ViewResult, []string) {
		t.Helper()
		res, err := w.ViewContent(w.Lookup(name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var names []string
		for _, ve := range res.Elements {
			names = append(names, w.Facts(ve.Element).QualifiedName)
		}
		return res, names
	}

	// a namespace expose, narrowed by the filter the view inherits from its definition
	res, got := content("P::vehicleParts")
	if want := []string{"P::Vehicle::wheels", "P::Vehicle::engine", "P::Vehicle::secret"}; !reflect.DeepEqual(got, want) {
		t.Errorf("vehicleParts:\n got %q\nwant %q", got, want)
	}
	if len(res.Exposes) != 1 || res.Exposes[0].Class != "NamespaceExpose" || len(res.Filters) != 1 ||
		res.Filters[0].Owner != w.Lookup("P::PartsView") || !res.Filters[0].Applied || res.Filters[0].Filter.Text != "@SysML::PartUsage" {
		t.Errorf("vehicleParts exposes %d, filters %+v", len(res.Exposes), res.Filters)
	}
	for _, ve := range res.Elements {
		if ve.Via != res.Exposes[0] || ve.Library || len(ve.Undecided) != 0 {
			t.Errorf("%s: %+v", w.Facts(ve.Element).QualifiedName, ve)
		}
	}

	// a recursive expose reaches the nested part; the definition it starts from
	// is not a part usage, so the inherited filter removes it
	_, got = content("P::recursive")
	if want := []string{"P::Vehicle::wheels", "P::Vehicle::engine", "P::Vehicle::secret", "P::Vehicle::engine::piston"}; !reflect.DeepEqual(got, want) {
		t.Errorf("recursive:\n got %q\nwant %q", got, want)
	}

	// membership exposes, in the order written, and an element exposed twice is shown once
	res, got = content("P::one")
	if want := []string{"P::Vehicle::wheels", "P::Vehicle::engine"}; !reflect.DeepEqual(got, want) || len(res.Filters) != 0 || res.Exposes[0].Class != "MembershipExpose" {
		t.Errorf("one: %q, %d filters", got, len(res.Filters))
	}
	res, got = content("P::twice")
	if want := []string{"P::Vehicle::wheels", "P::Vehicle::engine", "P::Vehicle::fuelIn", "P::Vehicle::secret"}; !reflect.DeepEqual(got, want) {
		t.Errorf("twice:\n got %q\nwant %q", got, want)
	}
	if len(res.Exposes) != 2 || res.Elements[0].Via != res.Exposes[0] {
		t.Errorf("twice: wheels is brought in by the first expose, got %v", res.Elements[0].Via)
	}

	// a view that specializes another has its exposes, and the filters of the
	// view and of the definition above it (only a view usage can write an expose)
	res, got = content("P::specialized")
	if want := []string{"P::Vehicle::wheels", "P::Vehicle::engine", "P::Vehicle::secret"}; !reflect.DeepEqual(got, want) ||
		len(res.Exposes) != 1 || res.Exposes[0].Owner != w.Lookup("P::vehicleParts") ||
		len(res.Filters) != 1 || res.Filters[0].Owner != w.Lookup("P::PartsView") {
		t.Errorf("specialized: %q, %d exposes, %d filters", got, len(res.Exposes), len(res.Filters))
	}

	// a filter that is decided for some elements removes them; the others it
	// cannot decide are kept and flagged
	res, got = content("P::lvl")
	if want := []string{"P::Parts::a", "P::Parts::c", "P::Parts::d"}; !reflect.DeepEqual(got, want) {
		t.Errorf("lvl:\n got %q\nwant %q", got, want)
	}
	f := res.Filters[0]
	if f.Applied || f.Undecided != 2 || f.Failure != NotDetermined || f.Reason != "P::Parts::c has no P::Safety metadata" {
		t.Errorf("lvl filter: %+v", f)
	}
	if !reflect.DeepEqual(res.Elements[0].Undecided, []int(nil)) || !reflect.DeepEqual(res.Elements[1].Undecided, []int{0}) {
		t.Errorf("lvl undecided: %v %v", res.Elements[0].Undecided, res.Elements[1].Undecided)
	}

	// a filter that cannot be evaluated is reported, and removes nothing
	res, got = content("P::call")
	if want := []string{"P::Parts::a", "P::Parts::b", "P::Parts::c", "P::Parts::d"}; !reflect.DeepEqual(got, want) {
		t.Errorf("call: %q", got)
	}
	if f := res.Filters[0]; f.Applied || f.Undecided != 4 || f.Failure != NotEvaluable || f.Reason != "cannot evaluate a call of RealFunctions::sqrt: it has no body" {
		t.Errorf("call filter: %+v", f)
	}

	// the standard library is content only when a library namespace is exposed
	res, got = content("P::lib")
	if len(got) == 0 {
		t.Fatal("expose ScalarValues::* shows nothing")
	}
	foundInteger := false
	for _, ve := range res.Elements {
		if !ve.Library {
			t.Errorf("%s is not marked as library", w.Facts(ve.Element).QualifiedName)
		}
		foundInteger = foundInteger || w.Facts(ve.Element).QualifiedName == "ScalarValues::Integer"
	}
	if !foundInteger {
		t.Errorf("ScalarValues::Integer is missing from %q", got)
	}

	// an expose that names nothing is reported
	res, got = content("P::dangling")
	if len(got) != 0 || len(res.Unresolved) != 1 {
		t.Errorf("dangling: %q, %d unresolved", got, len(res.Unresolved))
	}

	// not a view
	if _, err := w.ViewContent(w.Lookup("P::Vehicle")); err == nil {
		t.Error("a part definition is not a view")
	}
	if _, err := w.ViewContent(nil); err == nil {
		t.Error("nil is not a view")
	}
}

// The specification's own view example: "expose imports all", so the private
// part is shown too.
func TestViewContentOfTheSpecificationsExample(t *testing.T) {
	src, err := os.ReadFile("../testdata/upstream/sysml/examples/Simple Tests/ViewTest.sysml")
	if err != nil {
		t.Fatal(err)
	}
	w, _, diags := checked(t, "ViewTest.sysml", src)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	res, err := w.ViewContent(w.Lookup("ViewTest::V::v"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, ve := range res.Elements {
		got = append(got, w.Facts(ve.Element).QualifiedName)
	}
	if want := []string{"ViewTest::P::p1", "ViewTest::P::p2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
