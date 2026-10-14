// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"reflect"
	"testing"
)

const factsSource = `package P {
  private import ScalarValues::*;
  alias Q for Engine;
  part def <e1> Engine { attribute power : Real = 5; }
  port def Fuel { in item f; out item g; }
  part def Base { in attribute speed : Integer; part eng [1..2]; }
  part def Car :> Base {
    attribute :>> speed;
    part :>> eng [2];
    port p : ~Fuel;
    ref part driver : Engine [*];
    part sub :> eng;
    part a3 [n];
    attribute n : Integer;
    part a2 [0..*];
    part a1 [3];
    part o ordered nonunique;
    constant attribute k : Integer = 1;
    derived attribute d : Integer;
    abstract part x;
    exhibit state st;
    variation part v { part v1; }
  }
  connection def Link { end part left : Base; end part right : Base; }
  part l1 : Base; part l2 : Base;
  connection k : Link connect left ::> l1 to right ::> l2;
}`

func factsWorkspace(t *testing.T) *Workspace {
	t.Helper()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Add("m.sysml", []byte(factsSource)); err != nil {
		t.Fatal(err)
	}
	for _, d := range w.Resolve() {
		t.Fatalf("unexpected diagnostic: %v", d)
	}
	return w
}

// kinds lists "Kind->Target" for generalizations, with "*" after the implied.
func (w *Workspace) kinds(gs []Generalization) []string {
	var out []string
	for _, g := range gs {
		s := g.Kind + "->" + w.Facts(g.Target).QualifiedName
		if g.Implied {
			s += "*"
		}
		out = append(out, s)
	}
	return out
}

func TestFactsIdentityAndOwnership(t *testing.T) {
	w := factsWorkspace(t)
	engine := w.Facts(w.Lookup("P::Engine"))
	if engine.Class != "PartDefinition" || engine.Name != "Engine" || engine.ShortName != "e1" ||
		engine.QualifiedName != "P::Engine" || engine.IsLibrary {
		t.Errorf("Engine identity: %+v", engine)
	}
	if engine.Owner != w.Lookup("P") || engine.Membership != MemberOwning {
		t.Errorf("Engine owner %v, membership %q", engine.Owner, engine.Membership)
	}
	if len(engine.Members) != 1 || engine.Members[0].Kind != MemberFeature || engine.Members[0].Element != w.Lookup("P::Engine::power") {
		t.Errorf("Engine members: %+v", engine.Members)
	}
	if power := w.Facts(w.Lookup("P::Engine::power")); power.Membership != MemberFeature || power.Owner != w.Lookup("P::Engine") {
		t.Errorf("power membership %q", power.Membership)
	}

	// the package lists, in source order, an import, an alias, then what it owns
	var got []string
	for _, m := range w.Facts(w.Lookup("P")).Members {
		s := string(m.Kind) + " " + QualifiedName(m.Element)
		if m.Name != "" {
			s += " as " + m.Name
		}
		got = append(got, s)
	}
	want := []string{
		"import ScalarValues", "alias P::Engine as Q", "owning P::Engine", "owning P::Fuel", "owning P::Base",
		"owning P::Car", "owning P::Link", "owning P::l1", "owning P::l2", "owning P::k",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("P members:\n got %q\nwant %q", got, want)
	}
	if m := w.Facts(w.Lookup("P")).Members[0]; m.Visibility != "private" || m.Relationship.Class != "NamespaceImport" {
		t.Errorf("import: %+v", m)
	}

	integer := w.Facts(w.Lookup("ScalarValues::Integer"))
	if !integer.IsLibrary || integer.Class != "DataType" {
		t.Errorf("library element: %+v", integer)
	}
	if !reflect.DeepEqual(w.Facts(nil), Facts{}) {
		t.Error("Facts(nil) is not zero")
	}
}

func TestFactsFeatureProperties(t *testing.T) {
	w := factsWorkspace(t)
	type props struct {
		Direction                               string
		Composite, Reference, End, Constant     bool
		Derived, Ordered, Unique, Abstract, Var bool
	}
	for _, c := range []struct {
		name string
		want props
	}{
		{"P::Base::speed", props{Direction: "in", Reference: true, Unique: true}},
		{"P::Base::eng", props{Composite: true, Unique: true}},
		{"P::Car::driver", props{Reference: true, Unique: true}},
		{"P::Car::k", props{Reference: true, Constant: true, Unique: true}},
		{"P::Car::d", props{Reference: true, Derived: true, Unique: true}},
		{"P::Car::o", props{Composite: true, Ordered: true}},
		// the pilot makes an exhibited state non-composite, so it is referential
		{"P::Car::st", props{Reference: true, Unique: true}},
		{"P::Car::x", props{Composite: true, Unique: true, Abstract: true}},
		{"P::Car::v", props{Composite: true, Unique: true, Abstract: true, Var: true}},
		{"P::Fuel::g", props{Direction: "out", Reference: true, Unique: true}},
		{"P::Link::left", props{Reference: true, End: true, Constant: true, Unique: true}},
		// a feature that states no direction takes its redefinition's
		{"P::Car::speed", props{Direction: "in", Reference: true, Unique: true}},
	} {
		f := w.Facts(w.Lookup(c.name))
		got := props{f.Direction, f.IsComposite, f.IsReference, f.IsEnd, f.IsConstant, f.IsDerived, f.IsOrdered, f.IsUnique, f.IsAbstract, f.IsVariation}
		if got != c.want {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, c.want)
		}
	}
	// a feature that redefines takes its name, and is qualified by it
	if f := w.Facts(w.Lookup("P::Car::speed")); f.Name != "speed" || f.QualifiedName != "P::Car::speed" {
		t.Errorf("redefining feature: name %q, qualified %q", f.Name, f.QualifiedName)
	}
}

func TestFactsMultiplicity(t *testing.T) {
	w := factsWorkspace(t)
	for _, c := range []struct {
		name         string
		lower, upper int
		resolved     bool
	}{
		{"P::Base::eng", 1, 2, true},
		{"P::Car::eng", 2, 2, true}, // [2] is 2..2
		{"P::Car::driver", 0, -1, true},
		{"P::Car::a2", 0, -1, true},
		{"P::Car::a1", 3, 3, true},
		{"P::Car::a3", 0, 0, false}, // [n]: n has no value to compute
	} {
		m := w.Facts(w.Lookup(c.name)).Multiplicity
		if m == nil {
			t.Errorf("%s: no multiplicity", c.name)
			continue
		}
		if m.Lower != c.lower || m.Upper != c.upper || m.Resolved != c.resolved || m.Source == nil {
			t.Errorf("%s: %+v, want %d..%d resolved=%v", c.name, *m, c.lower, c.upper, c.resolved)
		}
	}
	// nothing written means nothing reported, not a default
	for _, name := range []string{"P::Car::x", "P::Base::speed", "P::Engine"} {
		if m := w.Facts(w.Lookup(name)).Multiplicity; m != nil {
			t.Errorf("%s: multiplicity %+v where none is written", name, *m)
		}
	}
}

func TestFactsRelationshipsAreSplitByKind(t *testing.T) {
	w := factsWorkspace(t)
	for _, c := range []struct {
		name                                                   string
		types, subsets, refSubsets, redefines, specs, conjugat []string
	}{
		{name: "P::Car", specs: []string{"Subclassification->P::Base"}},
		{name: "P::Engine", specs: []string{"Subclassification->Parts::Part*"}},
		{name: "P::Engine::power", types: []string{"FeatureTyping->ScalarValues::Real"}, subsets: []string{"Subsetting->Base::dataValues*"}},
		{name: "P::Car::sub", subsets: []string{"Subsetting->P::Car::eng"}},
		{name: "P::Car::eng", redefines: []string{"Redefinition->P::Base::eng"}},
		{name: "P::Link::left", types: []string{"FeatureTyping->P::Base"}, subsets: []string{"Subsetting->Parts::parts*"},
			redefines: []string{"Redefinition->Connections::BinaryConnection::source*"}},
		// the end of a connector points at a feature by reference subsetting
		{name: "P::k::left", refSubsets: []string{"ReferenceSubsetting->P::l1"},
			redefines: []string{"Redefinition->P::Link::left*", "Redefinition->Links::BinaryLink::source*"}},
	} {
		f := w.Facts(w.Lookup(c.name))
		for _, g := range []struct {
			what      string
			got, want []string
		}{
			{"types", w.kinds(f.Types), c.types}, {"subsets", w.kinds(f.Subsets), c.subsets},
			{"reference subsets", w.kinds(f.ReferenceSubsets), c.refSubsets}, {"redefines", w.kinds(f.Redefines), c.redefines},
			{"specializes", w.kinds(f.Specializes), c.specs}, {"conjugates", w.kinds(f.Conjugates), c.conjugat},
		} {
			if !reflect.DeepEqual(g.got, g.want) {
				t.Errorf("%s %s:\n got %q\nwant %q", c.name, g.what, g.got, g.want)
			}
		}
	}
}

func TestFactsConjugation(t *testing.T) {
	w := factsWorkspace(t)
	// the port typed ~Fuel is conjugated, and its typing is Fuel's conjugated port definition
	p := w.Facts(w.Lookup("P::Car::p"))
	if !p.Conjugated || len(p.Types) < 1 || p.Types[0].Kind != "ConjugatedPortTyping" || !p.Types[0].Target.IsA("ConjugatedPortDefinition") {
		t.Errorf("P::Car::p: conjugated %v, types %q", p.Conjugated, w.kinds(p.Types))
	}
	if w.Facts(w.Lookup("P::Car::driver")).Conjugated {
		t.Error("an ordinary port is conjugated")
	}
	// the conjugated port definition belongs to Fuel and conjugates it
	fuel := w.Lookup("P::Fuel")
	var conj *Element
	for _, m := range w.Facts(fuel).Members {
		if m.Element.IsA("ConjugatedPortDefinition") {
			conj = m.Element
		}
	}
	if conj == nil {
		t.Fatal("no ConjugatedPortDefinition among Fuel's members")
	}
	if got := w.kinds(w.Facts(conj).Conjugates); !reflect.DeepEqual(got, []string{"PortConjugation->P::Fuel"}) {
		t.Errorf("conjugates: %q", got)
	}
}

func TestFactsLeaveOutWhatOtherFieldsDescribe(t *testing.T) {
	w := factsWorkspace(t)
	// the multiplicity, the value and the generated structure are not members
	for _, name := range []string{"P::Car::a1", "P::Engine::power", "P::Car::eng"} {
		for _, m := range w.Facts(w.Lookup(name)).Members {
			t.Errorf("%s has member %s %s", name, m.Kind, m.Element.Class)
		}
	}
	got := w.Facts(w.Lookup("P::Link")).Members
	if len(got) != 2 || got[0].Kind != MemberFeature || got[1].Relationship.Class != "FeatureMembership" {
		t.Errorf("Link members: %+v", got)
	}
	// the ends of a connector are held as end features
	if k := w.Facts(w.Lookup("P::k")).Members; len(k) != 2 || k[0].Relationship.Class != "EndFeatureMembership" || k[0].Kind != MemberFeature {
		t.Errorf("k members: %+v", k)
	}
}
