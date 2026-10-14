// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"reflect"
	"strings"
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

const connectorSource = `package P {
  part def A { port out1; port in1; attribute x; attribute y; action a1; action a2; }
  part pa : A; part pb : A;
  connection c1 connect pa to pb;
  connection c2 connect e1 ::> pa to e2 ::> pb;
  binding b1 bind pa.x = pb.y;
  succession s1 first pa.a1 then pb.a2;
  flow f1 from pa.out1 to pb.in1;
  interface i1 connect pa.out1 to pb.in1;
  allocation al1 allocate pa to pb;
  action def Act { action p; action q; first p; then q; }
}`

func TestFactsConnectorEnds(t *testing.T) {
	w, _ := NewWorkspace()
	_, err := w.Add("c.sysml", []byte(connectorSource))
	if err != nil {
		t.Fatal(err)
	}
	if ds := w.Resolve(); len(ds) > 0 {
		t.Fatal(ds)
	}
	// ends lists "name->path" with the path's elements joined by "."
	ends := func(f Facts) []string {
		var out []string
		for _, e := range f.Ends {
			var path []string
			for _, p := range e.Path {
				path = append(path, w.Facts(p).QualifiedName)
			}
			out = append(out, e.Name+"->"+strings.Join(path, "."))
		}
		return out
	}
	for _, c := range []struct {
		name string
		want []string
	}{
		{"P::c1", []string{"->P::pa", "->P::pb"}},
		{"P::c2", []string{"e1->P::pa", "e2->P::pb"}},
		{"P::b1", []string{"->P::pa.P::A::x", "->P::pb.P::A::y"}},
		{"P::s1", []string{"->P::pa.P::A::a1", "->P::pb.P::A::a2"}},
		{"P::f1", []string{"->P::pa.P::A::out1", "->P::pb.P::A::in1"}},
		{"P::i1", []string{"->P::pa.P::A::out1", "->P::pb.P::A::in1"}},
		{"P::al1", []string{"->P::pa", "->P::pb"}},
	} {
		f := w.Facts(w.Lookup(c.name))
		if got := ends(f); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s ends:\n got %q\nwant %q", c.name, got, c.want)
		}
		for i, e := range f.Ends {
			if e.Target != e.Path[len(e.Path)-1] || !e.Element.IsTrue("isEnd") {
				t.Errorf("%s end %d: %+v", c.name, i, e)
			}
		}
	}

	// source and target are the ends of a flow and of a succession only
	for _, c := range []struct{ name, from, to string }{
		{"P::f1", "P::A::out1", "P::A::in1"},
		{"P::s1", "P::A::a1", "P::A::a2"},
	} {
		f := w.Facts(w.Lookup(c.name))
		if f.Source == nil || f.Target == nil || f.Source.Target != w.Lookup(c.from) || f.Target.Target != w.Lookup(c.to) {
			t.Errorf("%s: source %+v, target %+v", c.name, f.Source, f.Target)
		}
	}
	for _, name := range []string{"P::c1", "P::b1", "P::i1", "P::al1", "P::pa"} {
		if f := w.Facts(w.Lookup(name)); f.Source != nil || f.Target != nil {
			t.Errorf("%s has a source or target", name)
		}
	}
	if f := w.Facts(w.Lookup("P::pa")); f.Ends != nil {
		t.Errorf("a part has ends: %+v", f.Ends)
	}

	// "then q" has no first end of its own: the previous step
	var then Facts
	for _, m := range w.Facts(w.Lookup("P::Act")).Members {
		if m.Element.IsA("SuccessionAsUsage") {
			then = w.Facts(m.Element) // the last one is "then q"
		}
	}
	if then.Source == nil || then.Source.Target != nil || len(then.Source.Path) != 0 || then.Target.Target != w.Lookup("P::Act::q") {
		t.Errorf("then q: %+v", then.Ends)
	}
}

func TestFactsValue(t *testing.T) {
	w, _ := NewWorkspace()
	if _, err := w.Add("v.sysml", []byte(`package P {
  part def A {
    attribute a : ScalarValues::Integer = 2 + 3;
    attribute b : ScalarValues::Integer := 7;
    attribute c : ScalarValues::Integer default = 9;
    attribute d : ScalarValues::Real = a * 2.5;
    attribute e = NumericalFunctions::abs(a);
    attribute s = "hi";
    attribute n;
  }
}`)); err != nil {
		t.Fatal(err)
	}
	if ds := w.Resolve(); len(ds) > 0 {
		t.Fatal(ds)
	}
	for _, c := range []struct {
		name, text      string
		computed        bool
		value           any
		isDefault, init bool
	}{
		{"P::A::a", "2 + 3", true, int64(5), false, false},
		{"P::A::b", "7", true, int64(7), false, true},
		{"P::A::c", "9", true, int64(9), true, false},
		{"P::A::d", "a * 2.5", true, 12.5, false, false},
		{"P::A::s", `"hi"`, true, "hi", false, false},
		// a call of a library function cannot be computed yet: the text is what is left
		{"P::A::e", "NumericalFunctions::abs(a)", false, nil, false, false},
	} {
		v := w.Facts(w.Lookup(c.name)).Value
		if v == nil {
			t.Errorf("%s: no value", c.name)
			continue
		}
		if v.Text != c.text || v.Computed != c.computed || v.Value != c.value || v.IsDefault != c.isDefault || v.IsInitial != c.init || v.Expression == nil {
			t.Errorf("%s: %+v, want text %q computed %v value %v", c.name, *v, c.text, c.computed, c.value)
		}
	}
	if v := w.Facts(w.Lookup("P::A::n")).Value; v != nil {
		t.Errorf("a feature with no value has %+v", *v)
	}
	// the standard library's text is not kept
	for _, m := range w.Facts(w.Lookup("ScalarValues")).Members {
		if v := w.Facts(m.Element).Value; v != nil && v.Text != "" {
			t.Errorf("library value text %q", v.Text)
		}
	}
}

const behaviorSource = `package P {
  part def Sig;
  state def Machine {
    entry action init;
    do action run;
    exit action stop;
    state idle;
    state active;
    transition t1 first idle accept Sig if true do action act1 then active;
    transition first active then idle;
    state par parallel { state r1; state r2; }
  }
  action def Flow1 {
    action a; action b; action c;
    first start;
    then fork f;
    then a;
    then join j;
    then decide d;
    if true then b;
    else c;
    merge m;
    first a then b;
  }
}`

func TestFactsBehavior(t *testing.T) {
	w, _ := NewWorkspace()
	if _, err := w.Add("b.sysml", []byte(behaviorSource)); err != nil {
		t.Fatal(err)
	}
	if ds := w.Resolve(); len(ds) > 0 {
		t.Fatal(ds)
	}
	q := func(e *Element) string {
		if e == nil {
			return ""
		}
		return w.Facts(e).QualifiedName
	}

	m := w.Facts(w.Lookup("P::Machine")).Behavior
	if m == nil || q(m.Entry) != "P::Machine::init" || q(m.Do) != "P::Machine::run" || q(m.Exit) != "P::Machine::stop" || m.IsParallel {
		t.Fatalf("Machine: %+v", m)
	}
	if len(m.Transitions) != 2 {
		t.Fatalf("%d transitions, want 2", len(m.Transitions))
	}
	t1 := m.Transitions[0]
	if q(t1.Source) != "P::Machine::idle" || q(t1.Target) != "P::Machine::active" ||
		t1.Trigger == nil || !t1.Trigger.IsA("AcceptActionUsage") ||
		t1.Guard == nil || t1.Guard.Class != "LiteralBoolean" || q(t1.Effect) != "P::Machine::t1::act1" ||
		t1.Element != w.Lookup("P::Machine::t1") {
		t.Errorf("t1: %+v", t1)
	}
	if t2 := m.Transitions[1]; q(t2.Source) != "P::Machine::active" || q(t2.Target) != "P::Machine::idle" ||
		t2.Trigger != nil || t2.Guard != nil || t2.Effect != nil {
		t.Errorf("second transition: %+v", t2)
	}
	if b := w.Facts(w.Lookup("P::Machine::par")).Behavior; b == nil || !b.IsParallel {
		t.Errorf("parallel state: %+v", b)
	}
	// a transition is described by its owner, not as a behaviour of its own
	if b := w.Facts(w.Lookup("P::Machine::t1")).Behavior; b != nil {
		t.Errorf("a transition has behaviour %+v", b)
	}

	// control nodes, in the order they are written
	for name, want := range map[string]string{"P::Flow1::f": "fork", "P::Flow1::j": "join", "P::Flow1::d": "decision", "P::Flow1::m": "merge", "P::Flow1::a": ""} {
		if got := w.Facts(w.Lookup(name)).ControlNode; got != want {
			t.Errorf("%s control node %q, want %q", name, got, want)
		}
	}
	// the successions the action writes; an end it does not write has no target
	flow := w.Facts(w.Lookup("P::Flow1")).Behavior
	if flow == nil || len(flow.Successions) != 5 {
		t.Fatalf("Flow1: %+v", flow)
	}
	var got []string
	for _, s := range flow.Successions {
		got = append(got, q(s.Source.Target)+">"+q(s.Target.Target))
	}
	if want := []string{">", ">P::Flow1::a", ">", ">", "P::Flow1::a>P::Flow1::b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("successions:\n got %q\nwant %q", got, want)
	}
	// the guarded branches of the decision
	if len(flow.Transitions) != 2 || flow.Transitions[0].Source != nil || q(flow.Transitions[0].Target) != "P::Flow1::b" ||
		flow.Transitions[0].Guard == nil || q(flow.Transitions[1].Target) != "P::Flow1::c" || flow.Transitions[1].Guard != nil {
		t.Errorf("decision transitions: %+v", flow.Transitions)
	}

	if b := w.Facts(w.Lookup("P::Sig")).Behavior; b != nil {
		t.Errorf("a part definition has behaviour %+v", b)
	}
}

const rolesSource = `package P {
  part def Car; part def Driver; part def Owner;
  requirement def R1 {
    subject car : Car;
    attribute mass;
    assume constraint ac { true }
    require constraint rc { true }
    actor drv : Driver;
    stakeholder own : Owner;
  }
  requirement r2 : R1;
  use case def U { subject s : Car; objective o { doc /* goal */ } actor a : Driver; }
  part vehicle : Car;
  satisfy r2 by vehicle;
  satisfy requirement r3 : R1 by vehicle;
  verification def V { subject sv : Car; objective vo { verify r2; } }
  concern def Cn { stakeholder st : Owner; }
}`

func TestFactsRequirementRoles(t *testing.T) {
	w, _ := NewWorkspace()
	doc, err := w.Add("r.sysml", []byte(rolesSource))
	if err != nil {
		t.Fatal(err)
	}
	if ds := w.Resolve(); len(ds) > 0 {
		t.Fatal(ds)
	}
	q := func(es ...*Element) []string {
		out := []string{}
		for _, e := range es {
			if e != nil {
				out = append(out, w.Facts(e).QualifiedName)
			}
		}
		return out
	}

	r1 := w.Facts(w.Lookup("P::R1")).Roles
	if r1 == nil {
		t.Fatal("R1 has no roles")
	}
	for _, c := range []struct {
		what      string
		got, want []string
	}{
		{"subject", q(r1.Subject), []string{"P::R1::car"}},
		{"actors", q(r1.Actors...), []string{"P::R1::drv"}},
		{"stakeholders", q(r1.Stakeholders...), []string{"P::R1::own"}},
		{"assumed", q(r1.Assumed...), []string{"P::R1::ac"}},
		{"required", q(r1.Required...), []string{"P::R1::rc"}},
		{"objective", q(r1.Objective), []string{}},
	} {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("R1 %s: got %q, want %q", c.what, c.got, c.want)
		}
	}

	u := w.Facts(w.Lookup("P::U")).Roles
	if u == nil || q(u.Subject)[0] != "P::U::s" || q(u.Objective)[0] != "P::U::o" || q(u.Actors...)[0] != "P::U::a" {
		t.Errorf("U: %+v", u)
	}
	if cn := w.Facts(w.Lookup("P::Cn")).Roles; cn == nil || q(cn.Stakeholders...)[0] != "P::Cn::st" {
		t.Errorf("Cn: %+v", cn)
	}

	// a verification case verifies what its objective verifies, and so does the objective
	for _, name := range []string{"P::V", "P::V::vo"} {
		if got := q(w.Facts(w.Lookup(name)).Roles.Verifies...); !reflect.DeepEqual(got, []string{"P::r2"}) {
			t.Errorf("%s verifies %q", name, got)
		}
	}
	if v := w.Facts(w.Lookup("P::V")).Roles; len(v.Required) != 0 {
		t.Errorf("verify is also counted as require: %q", q(v.Required...))
	}

	// a satisfy usage references the requirement or declares one, and names what satisfies it
	var satisfies [][]string
	doc.Root.Walk(func(e *Element) {
		if e.IsA("SatisfyRequirementUsage") {
			r := w.Facts(e).Roles
			satisfies = append(satisfies, append(q(r.Satisfies), q(r.SatisfiedBy)...))
			if r.Subject != nil {
				t.Errorf("a satisfy usage's by is its subject: %+v", r)
			}
		}
	})
	if want := [][]string{{"P::r2", "P::vehicle"}, {"P::r3", "P::vehicle"}}; !reflect.DeepEqual(satisfies, want) {
		t.Errorf("satisfy: got %q, want %q", satisfies, want)
	}

	for _, name := range []string{"P::Car", "P::vehicle", "P::r2"} {
		if r := w.Facts(w.Lookup(name)).Roles; r != nil {
			t.Errorf("%s has roles %+v", name, r)
		}
	}
}

const annotationSource = `package P {
  doc /* about P */
  part def Car {
    doc /* a car */
    comment c1 /* free note */
    comment about Car /* about car */
    comment c3 about Car, Truck locale "en-US" /* both */
    rep r1 language "OCL" /* x > 0 */
    part wheel;
  }
  part def Truck;
  /* bracket comment */
  part def Bike;
  comment cx about Bike /* about bike */
}`

func TestFactsAnnotations(t *testing.T) {
	w, _ := NewWorkspace()
	if _, err := w.Add("a.sysml", []byte(annotationSource)); err != nil {
		t.Fatal(err)
	}
	w.Resolve()
	// summary lists "kind body -> annotated" for each annotation
	summary := func(name string) []string {
		out := []string{}
		for _, a := range w.Facts(w.Lookup(name)).Annotations {
			var to []string
			for _, e := range a.Annotates {
				to = append(to, w.Facts(e).QualifiedName)
			}
			s := a.Kind + " " + a.Body + " -> " + strings.Join(to, ",")
			if a.Locale != "" || a.Language != "" {
				s += " [" + a.Locale + a.Language + "]"
			}
			if a.Name != "" {
				s += " #" + a.Name
			}
			out = append(out, s)
		}
		return out
	}
	for _, c := range []struct {
		name string
		want []string
	}{
		{"P", []string{"documentation about P -> P", "comment bracket comment -> P"}},
		// what Car owns without naming another element, then what names it
		{"P::Car", []string{
			"documentation a car -> P::Car",
			"comment free note -> P::Car #c1",
			"textualRepresentation x > 0 -> P::Car [OCL] #r1",
			"comment about car -> P::Car",
			"comment both -> P::Car,P::Truck [en-US] #c3",
		}},
		// a comment owned by Car that is about Truck annotates Truck too
		{"P::Truck", []string{"comment both -> P::Car,P::Truck [en-US] #c3"}},
		// a comment elsewhere that names the element
		{"P::Bike", []string{"comment about bike -> P::Bike #cx"}},
		{"P::Car::wheel", []string{}},
	} {
		if got := summary(c.name); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}

	// the index follows the files: replacing one forgets what it said
	w.Replace("a.sysml", []byte("package P { part def Bike; part def Truck; }"))
	w.Resolve()
	if got := summary("P::Bike"); len(got) != 0 {
		t.Errorf("after Replace, Bike has %q", got)
	}

	// the documentation of a library element is found through the element that owns it
	if a := w.Facts(w.Lookup("ScalarValues")).Annotations; len(a) == 0 || a[0].Kind != "documentation" ||
		!strings.HasPrefix(a[0].Body, "This package contains a basic set of primitive scalar") {
		t.Errorf("library documentation: %+v", a)
	}
}

const messageSource = `package SeqProbe {
    item def Start;
    part def Driver  { out item cmd : Start; }
    part def Vehicle { in  item cmd : Start; }

    occurrence def Ignition {
        part driver : Driver;
        part vehicle : Vehicle;
        message startCmd of Start from driver.cmd to vehicle.cmd;
        message short from driver to vehicle;
        message bare;
        flow f1 of Start from driver.cmd to vehicle.cmd;
    }
}`

func TestFactsMessageEnds(t *testing.T) {
	w, _ := NewWorkspace()
	if _, err := w.Add("m.sysml", []byte(messageSource)); err != nil {
		t.Fatal(err)
	}
	if ds := w.Resolve(); len(ds) > 0 {
		t.Fatal(ds)
	}
	// paths lists the names along each end's chain: driver.cmd is "driver cmd"
	paths := func(f Facts) []string {
		var out []string
		for _, e := range f.Ends {
			var names []string
			for _, p := range e.Path {
				names = append(names, w.EffectiveName(p))
			}
			out = append(out, strings.Join(names, " "))
		}
		return out
	}
	for _, c := range []struct {
		name    string
		message bool
		paths   []string
		from    string // the source's target, "" for none
		to      string
	}{
		{"SeqProbe::Ignition::startCmd", true, []string{"driver cmd", "vehicle cmd"}, "SeqProbe::Driver::cmd", "SeqProbe::Vehicle::cmd"},
		{"SeqProbe::Ignition::short", true, []string{"driver", "vehicle"}, "SeqProbe::Ignition::driver", "SeqProbe::Ignition::vehicle"},
		{"SeqProbe::Ignition::bare", true, nil, "", ""},
		// a flow in the same occurrence keeps its own ends, and is not a message
		{"SeqProbe::Ignition::f1", false, []string{"driver cmd", "vehicle cmd"}, "SeqProbe::Driver::cmd", "SeqProbe::Vehicle::cmd"},
	} {
		f := w.Facts(w.Lookup(c.name))
		if got := paths(f); !reflect.DeepEqual(got, c.paths) {
			t.Errorf("%s ends: got %q, want %q", c.name, got, c.paths)
		}
		if f.IsMessage != c.message {
			t.Errorf("%s IsMessage = %v", c.name, f.IsMessage)
		}
		if c.from == "" {
			if f.Source != nil || f.Target != nil {
				t.Errorf("%s has a source or target", c.name)
			}
			continue
		}
		if f.Source == nil || f.Target == nil || w.Facts(f.Source.Target).QualifiedName != c.from || w.Facts(f.Target.Target).QualifiedName != c.to {
			t.Errorf("%s source %+v target %+v", c.name, f.Source, f.Target)
		}
	}
	// nothing else is a message
	for _, name := range []string{"SeqProbe::Ignition", "SeqProbe::Ignition::driver", "SeqProbe::Start"} {
		if w.Facts(w.Lookup(name)).IsMessage {
			t.Errorf("%s is a message", name)
		}
	}
}
