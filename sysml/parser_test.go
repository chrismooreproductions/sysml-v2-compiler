package sysml_test

import (
	"reflect"
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

// TestModelParseDefKeywords checks that "attribute", "item", and "port" are
// each accepted as a definition/usage keyword family alongside "part",
// dispatched through the same defKeywords table -- so a definition and a
// usage of each kind parse into a Definition/Usage carrying the matching
// DefKind, with no bespoke per-keyword parsing.
func TestModelParseDefKeywords(t *testing.T) {
	cases := map[string]struct {
		keyword string
		want    sysml.DefKind
	}{
		"part":       {"part", sysml.DefPart},
		"attribute":  {"attribute", sysml.DefAttribute},
		"item":       {"item", sysml.DefItem},
		"port":       {"port", sysml.DefPort},
		"constraint": {"constraint", sysml.DefConstraint},
		"calc":       {"calc", sysml.DefCalculation},
	}

	for name, tt := range cases {
		t.Run(name+" definition", func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { `+tt.keyword+` def X; }`)

			def, ok := pkg.Members[0].(*sysml.Definition)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Definition", pkg.Members[0])
			}
			if def.Kind != tt.want || def.Name != "X" {
				t.Errorf("member 0 = %+v, want Definition{Kind: %v, Name: X}", def, tt.want)
			}
		})

		t.Run(name+" usage", func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { `+tt.keyword+` def X; `+tt.keyword+` x : X; }`)

			usage, ok := pkg.Members[1].(*sysml.Usage)
			if !ok {
				t.Fatalf("member 1 is %T, want *sysml.Usage", pkg.Members[1])
			}
			if usage.Kind != tt.want || usage.Name != "x" || usage.Type != "X" {
				t.Errorf("member 1 = %+v, want Usage{Kind: %v, Name: x, Type: X}", usage, tt.want)
			}
		})
	}
}

// TestModelParseCalculationBodyResult checks that a constraint/calc
// definition's body can end in a trailing, unterminated result expression
// after its ordinary members (CalculationBody), and that a body with only
// members (no result) leaves Result nil.
func TestModelParseCalculationBodyResult(t *testing.T) {
	t.Run("constraint def with result", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			constraint def MassAnalysis {
				attribute totalMass : Real;
				attribute massLimit : Real;
				totalMass <= massLimit
			}
		}`)

		def, ok := pkg.Members[0].(*sysml.Definition)
		if !ok {
			t.Fatalf("member 0 is %T, want *sysml.Definition", pkg.Members[0])
		}
		if len(def.Members) != 2 {
			t.Fatalf("got %d members, want 2", len(def.Members))
		}
		result, ok := def.Result.(*sysml.BinaryExpr)
		if !ok {
			t.Fatalf("Result is %T, want *sysml.BinaryExpr", def.Result)
		}
		if result.Op != "<=" {
			t.Errorf("Result.Op = %q, want %q", result.Op, "<=")
		}
	})

	t.Run("calc def with result", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			calc def Sum {
				attribute a : Real;
				attribute b : Real;
				a + b
			}
		}`)

		def, ok := pkg.Members[0].(*sysml.Definition)
		if !ok {
			t.Fatalf("member 0 is %T, want *sysml.Definition", pkg.Members[0])
		}
		result, ok := def.Result.(*sysml.BinaryExpr)
		if !ok {
			t.Fatalf("Result is %T, want *sysml.BinaryExpr", def.Result)
		}
		if result.Op != "+" {
			t.Errorf("Result.Op = %q, want %q", result.Op, "+")
		}
	})

	t.Run("members only, no result", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			constraint def C {
				attribute a : Real;
			}
		}`)

		def, ok := pkg.Members[0].(*sysml.Definition)
		if !ok {
			t.Fatalf("member 0 is %T, want *sysml.Definition", pkg.Members[0])
		}
		if len(def.Members) != 1 {
			t.Errorf("got %d members, want 1", len(def.Members))
		}
		if def.Result != nil {
			t.Errorf("Result = %+v, want nil", def.Result)
		}
	})

	t.Run("empty body", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle { constraint def C { } }`)

		def, ok := pkg.Members[0].(*sysml.Definition)
		if !ok {
			t.Fatalf("member 0 is %T, want *sysml.Definition", pkg.Members[0])
		}
		if len(def.Members) != 0 || def.Result != nil {
			t.Errorf("Definition = %+v, want no members and nil Result", def)
		}
	})
}

// TestModelParseAnonymousUsage checks that a usage's name is optional --
// an inline constraint like `constraint { mass <= massLimit }` has no
// identifier at all between the keyword and its body.
func TestModelParseAnonymousUsage(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		attribute mass : Real;
		attribute massLimit : Real;
		constraint { mass <= massLimit }
	}`)

	usage, ok := pkg.Members[2].(*sysml.Usage)
	if !ok {
		t.Fatalf("member 2 is %T, want *sysml.Usage", pkg.Members[2])
	}
	if usage.Name != "" {
		t.Errorf("Name = %q, want %q (anonymous)", usage.Name, "")
	}
	if usage.Type != "" {
		t.Errorf("Type = %q, want %q (untyped)", usage.Type, "")
	}
	result, ok := usage.Result.(*sysml.BinaryExpr)
	if !ok {
		t.Fatalf("Result is %T, want *sysml.BinaryExpr", usage.Result)
	}
	if result.Op != "<=" {
		t.Errorf("Result.Op = %q, want %q", result.Op, "<=")
	}
}

// TestModelParseUsageWithBody checks that a named constraint/calc usage
// can carry its own CalculationBody (members and/or a trailing result),
// the same shape a definition's body has -- unlike every other DefKind's
// usage, which is always a plain ";"-terminated declaration.
func TestModelParseUsageWithBody(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		constraint def C;
		constraint check : C {
			attribute a : Real;
			attribute b : Real;
			a == b
		}
	}`)

	usage, ok := pkg.Members[1].(*sysml.Usage)
	if !ok {
		t.Fatalf("member 1 is %T, want *sysml.Usage", pkg.Members[1])
	}
	if usage.Name != "check" || usage.Type != "C" {
		t.Errorf("Name/Type = %q/%q, want %q/%q", usage.Name, usage.Type, "check", "C")
	}
	if len(usage.Members) != 2 {
		t.Fatalf("got %d members, want 2", len(usage.Members))
	}
	result, ok := usage.Result.(*sysml.BinaryExpr)
	if !ok {
		t.Fatalf("Result is %T, want *sysml.BinaryExpr", usage.Result)
	}
	if result.Op != "==" {
		t.Errorf("Result.Op = %q, want %q", result.Op, "==")
	}
}

// TestModelParseUsageGeneralBody checks that a usage of any DefKind --
// not just constraint/calc-shaped ones -- can carry a plain "{ members }"
// body with no trailing result expression, e.g. "part p { part x; }". This
// used to be a hasCalculationBody/hasPlainBody-gated capability; parseUsage
// now accepts a body for every kind uniformly, deciding only its *shape*
// (calc vs. plain) once an OpenBrace is actually seen.
func TestModelParseUsageGeneralBody(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		part p {
			part x {
				part x1;
			}
		}
	}`)

	p, ok := pkg.Members[0].(*sysml.Usage)
	if !ok {
		t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
	}
	if p.Name != "p" || len(p.Members) != 1 {
		t.Fatalf("p = %+v, want Usage{Name: p, 1 member}", p)
	}
	if p.Result != nil {
		t.Errorf("p.Result = %+v, want nil (plain body, not a CalculationBody)", p.Result)
	}

	x, ok := p.Members[0].(*sysml.Usage)
	if !ok {
		t.Fatalf("p's member 0 is %T, want *sysml.Usage", p.Members[0])
	}
	if x.Name != "x" || len(x.Members) != 1 {
		t.Errorf("x = %+v, want Usage{Name: x, 1 member}", x)
	}
}

// TestModelParseUsageEmptyGeneralBody checks that a plain (non-calc) body
// can be empty ("part p { }"), distinct from the ";"-terminated form.
func TestModelParseUsageEmptyGeneralBody(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle { part p { } }`)

	p, ok := pkg.Members[0].(*sysml.Usage)
	if !ok {
		t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
	}
	if p.Name != "p" || p.Members != nil {
		t.Errorf("p = %+v, want Usage{Name: p, no members}", p)
	}
}

// TestModelParseConnectionBareShorthand checks the bare
// "'connect' ConnectorPart" form -- no "connection" keyword, no name, no
// type at all -- for both the binary ("a to b") and n-ary ("(a, b, ...)")
// connector shapes.
func TestModelParseConnectionBareShorthand(t *testing.T) {
	t.Run("binary", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			part p;
			part y;
			connect p to y;
		}`)

		conn, ok := pkg.Members[2].(*sysml.Connection)
		if !ok {
			t.Fatalf("member 2 is %T, want *sysml.Connection", pkg.Members[2])
		}
		if conn.Name != "" || conn.Type != "" {
			t.Errorf("Name/Type = %q/%q, want both empty", conn.Name, conn.Type)
		}
		if len(conn.Ends) != 2 {
			t.Fatalf("got %d ends, want 2", len(conn.Ends))
		}
		p, ok := conn.Ends[0].(*sysml.NameRef)
		if !ok || p.Path != "p" {
			t.Errorf("Ends[0] = %+v, want NameRef{Path: p}", conn.Ends[0])
		}
		y, ok := conn.Ends[1].(*sysml.NameRef)
		if !ok || y.Path != "y" {
			t.Errorf("Ends[1] = %+v, want NameRef{Path: y}", conn.Ends[1])
		}
	})

	t.Run("n-ary with feature chain ends", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			part p1;
			part d1;
			part d2;
			connect (p1.x, d1, d2);
		}`)

		conn, ok := pkg.Members[3].(*sysml.Connection)
		if !ok {
			t.Fatalf("member 3 is %T, want *sysml.Connection", pkg.Members[3])
		}
		if len(conn.Ends) != 3 {
			t.Fatalf("got %d ends, want 3", len(conn.Ends))
		}
		chain, ok := conn.Ends[0].(*sysml.FeatureChain)
		if !ok || len(chain.Path) != 2 || chain.Path[0] != "p1" || chain.Path[1] != "x" {
			t.Errorf("Ends[0] = %+v, want FeatureChain{Path: [p1 x]}", conn.Ends[0])
		}
	})
}

// TestModelParseBind checks BindingConnectorAsUsage's three forms: the
// bare "'bind' ... '=' ..." shorthand (no name/type at all), "binding"
// with just a name, and "binding" with both a name and a type -- each
// producing a DefBind *sysml.Connection with two Ends, the same shape
// "connect"'s own binary form has.
func TestModelParseBind(t *testing.T) {
	cases := map[string]struct {
		source   string
		wantName string
		wantType string
	}{
		"bare":           {"bind a = b;", "", ""},
		"binding, name":  {"binding ab bind a = b;", "ab", ""},
		"binding, typed": {"binding ab1 : AB bind a = b;", "ab1", "AB"},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle {
				part a;
				part b;
				`+tt.source+`
			}`)

			conn, ok := pkg.Members[2].(*sysml.Connection)
			if !ok {
				t.Fatalf("member 2 is %T, want *sysml.Connection", pkg.Members[2])
			}
			if conn.Kind != sysml.DefBind || conn.Name != tt.wantName || conn.Type != tt.wantType {
				t.Errorf("Connection = %+v, want Kind: DefBind, Name: %q, Type: %q", conn, tt.wantName, tt.wantType)
			}
			if len(conn.Ends) != 2 {
				t.Fatalf("got %d ends, want 2", len(conn.Ends))
			}
			left, ok := conn.Ends[0].(*sysml.NameRef)
			if !ok || left.Path != "a" {
				t.Errorf("Ends[0] = %+v, want NameRef{Path: a}", conn.Ends[0])
			}
			right, ok := conn.Ends[1].(*sysml.NameRef)
			if !ok || right.Path != "b" {
				t.Errorf("Ends[1] = %+v, want NameRef{Path: b}", conn.Ends[1])
			}
		})
	}
}

// TestModelParseBindFeatureChainEndsAndBody checks that a bind's ends can
// be dotted feature chains, and that it can carry an ordinary body
// instead of a bare ";" -- the same general-body shape every usage
// already has.
func TestModelParseBindFeatureChainEndsAndBody(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		part massAnalysis {
			part totalMass;
		}
		part mass;
		bind massAnalysis.totalMass = mass {
			part nested;
		}
	}`)

	conn, ok := pkg.Members[2].(*sysml.Connection)
	if !ok {
		t.Fatalf("member 2 is %T, want *sysml.Connection", pkg.Members[2])
	}
	chain, ok := conn.Ends[0].(*sysml.FeatureChain)
	if !ok || !reflect.DeepEqual(chain.Path, []string{"massAnalysis", "totalMass"}) {
		t.Errorf("Ends[0] = %+v, want FeatureChain{Path: [massAnalysis totalMass]}", conn.Ends[0])
	}
	if len(conn.Members) != 1 {
		t.Errorf("got %d members, want 1", len(conn.Members))
	}
}

// TestModelParseConnectionUsage checks the "connection"/"interface"
// keyword form, with a name, an optional type, and an explicit "connect"
// clause -- as opposed to a plain "connection bus : C;" with no connect
// clause at all, which stays an ordinary *sysml.Usage (checked separately
// below) since it uses none of Connection's machinery.
func TestModelParseConnectionUsage(t *testing.T) {
	cases := map[string]struct {
		keyword string
		want    sysml.DefKind
	}{
		"connection": {"connection", sysml.DefConnection},
		"interface":  {"interface", sysml.DefInterface},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle {
				`+tt.keyword+` def C;
				part d1;
				part d2;
				part d3;
				part d4;
				`+tt.keyword+` bus : C connect (d1, d2, d3, d4);
			}`)

			conn, ok := pkg.Members[5].(*sysml.Connection)
			if !ok {
				t.Fatalf("member 5 is %T, want *sysml.Connection", pkg.Members[5])
			}
			if conn.Kind != tt.want || conn.Name != "bus" || conn.Type != "C" {
				t.Errorf("Connection = %+v, want Kind: %v, Name: bus, Type: C", conn, tt.want)
			}
			if len(conn.Ends) != 4 {
				t.Errorf("got %d ends, want 4", len(conn.Ends))
			}
		})
	}
}

// TestModelParseConnectionUsageWithoutConnect checks that a
// "connection"/"interface" usage with no explicit "connect" clause parses
// as an ordinary *sysml.Usage, not a *sysml.Connection -- there are no
// ends to carry.
func TestModelParseConnectionUsageWithoutConnect(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		connection def C;
		connection bus : C;
	}`)

	usage, ok := pkg.Members[1].(*sysml.Usage)
	if !ok {
		t.Fatalf("member 1 is %T, want *sysml.Usage", pkg.Members[1])
	}
	if usage.Kind != sysml.DefConnection || usage.Name != "bus" || usage.Type != "C" {
		t.Errorf("Usage = %+v, want Kind: DefConnection, Name: bus, Type: C", usage)
	}
}

// TestModelParseEndMember checks "end" as a definition-body member in each
// of its shapes: with a recognized inner keyword (using that keyword's own
// DefKind), with none at all (DefEnd), typed or untyped, and with a
// metadata prefix that comes *after* "end" rather than before it.
func TestModelParseEndMember(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		port def P1;
		part def D1;

		interface def I1 {
			end port p1: P1;
			end p1: P1;
		}

		connection def C {
			end end1;
			end #original ::> end1;
		}
	}`)

	iface, ok := pkg.Members[2].(*sysml.Definition)
	if !ok {
		t.Fatalf("member 2 is %T, want *sysml.Definition", pkg.Members[2])
	}
	if len(iface.Members) != 2 {
		t.Fatalf("got %d interface members, want 2", len(iface.Members))
	}

	withPort, ok := iface.Members[0].(*sysml.Usage)
	if !ok {
		t.Fatalf("interface member 0 is %T, want *sysml.Usage", iface.Members[0])
	}
	if withPort.Kind != sysml.DefPort || withPort.Name != "p1" || withPort.Type != "P1" {
		t.Errorf("member 0 = %+v, want a DefPort usage p1 : P1", withPort)
	}

	bare, ok := iface.Members[1].(*sysml.Usage)
	if !ok {
		t.Fatalf("interface member 1 is %T, want *sysml.Usage", iface.Members[1])
	}
	if bare.Kind != sysml.DefEnd || bare.Name != "p1" || bare.Type != "P1" {
		t.Errorf("member 1 = %+v, want a DefEnd usage p1 : P1", bare)
	}

	conn, ok := pkg.Members[3].(*sysml.Definition)
	if !ok {
		t.Fatalf("member 3 is %T, want *sysml.Definition", pkg.Members[3])
	}
	if len(conn.Members) != 2 {
		t.Fatalf("got %d connection members, want 2", len(conn.Members))
	}

	end1, ok := conn.Members[0].(*sysml.Usage)
	if !ok {
		t.Fatalf("connection member 0 is %T, want *sysml.Usage", conn.Members[0])
	}
	if end1.Kind != sysml.DefEnd || end1.Name != "end1" {
		t.Errorf("member 0 = %+v, want a DefEnd usage named end1", end1)
	}

	tagged, ok := conn.Members[1].(*sysml.Usage)
	if !ok {
		t.Fatalf("connection member 1 is %T, want *sysml.Usage", conn.Members[1])
	}
	if tagged.Kind != sysml.DefEnd || tagged.Name != "" || tagged.References != "end1" {
		t.Errorf("member 1 = %+v, want an anonymous DefEnd usage referencing end1", tagged)
	}
	if !reflect.DeepEqual(tagged.Metadata, []string{"original"}) {
		t.Errorf("member 1 Metadata = %#v, want [original]", tagged.Metadata)
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
		"unbounded star":     {`part combatants : Combatant[*];`, &sysml.Multiplicity{Lower: sysml.Bound{Value: 0}, Upper: sysml.Bound{Value: sysml.Unbounded}}},
		"exact count":        {`part combatants : Combatant[3];`, &sysml.Multiplicity{Lower: sysml.Bound{Value: 3}, Upper: sysml.Bound{Value: 3}}},
		"bounded range":      {`part combatants : Combatant[0..5];`, &sysml.Multiplicity{Lower: sysml.Bound{Value: 0}, Upper: sysml.Bound{Value: 5}}},
		"lower to unbounded": {`part combatants : Combatant[1..*];`, &sysml.Multiplicity{Lower: sysml.Bound{Value: 1}, Upper: sysml.Bound{Value: sysml.Unbounded}}},
		"name bound":         {`part combatants : Combatant[n];`, &sysml.Multiplicity{Lower: sysml.Bound{Name: "n"}, Upper: sysml.Bound{Name: "n"}}},
		"name to unbounded":  {`part combatants : Combatant[n..*];`, &sysml.Multiplicity{Lower: sysml.Bound{Name: "n"}, Upper: sysml.Bound{Value: sysml.Unbounded}}},
		"literal to name":    {`part combatants : Combatant[1..n];`, &sysml.Multiplicity{Lower: sysml.Bound{Value: 1}, Upper: sysml.Bound{Name: "n"}}},
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

// TestModelParseUsageUntyped checks that a usage's type is optional --
// "port p;" is a complete usage with Type == "" and no Multiplicity -- and
// that an untyped usage can still carry a multiplicity ("part a[1];").
func TestModelParseUsageUntyped(t *testing.T) {
	cases := map[string]struct {
		source   string
		wantType string
		wantMult *sysml.Multiplicity
	}{
		"bare":              {`port p;`, "", nil},
		"with multiplicity": {`part a[1];`, "", &sysml.Multiplicity{Lower: sysml.Bound{Value: 1}, Upper: sysml.Bound{Value: 1}}},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { `+tt.source+` }`)

			usage, ok := pkg.Members[0].(*sysml.Usage)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
			}
			if usage.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", usage.Type, tt.wantType)
			}
			if tt.wantMult == nil {
				if usage.Multiplicity != nil {
					t.Errorf("Multiplicity = %+v, want nil", usage.Multiplicity)
				}
				return
			}
			if usage.Multiplicity == nil || *usage.Multiplicity != *tt.wantMult {
				t.Errorf("Multiplicity = %+v, want %+v", usage.Multiplicity, tt.wantMult)
			}
		})
	}
}

// TestModelParseUsageMultiplicityBeforeType checks that a multiplicity may
// precede its usage's type ("part b[0..2] : P;"), not just follow it, since
// FeatureSpecializationPart allows either order.
func TestModelParseUsageMultiplicityBeforeType(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle { part def P; part b[0..2] : P; }`)

	usage, ok := pkg.Members[1].(*sysml.Usage)
	if !ok {
		t.Fatalf("member 1 is %T, want *sysml.Usage", pkg.Members[1])
	}
	if usage.Type != "P" {
		t.Errorf("Type = %q, want %q", usage.Type, "P")
	}
	want := sysml.Multiplicity{Lower: sysml.Bound{Value: 0}, Upper: sysml.Bound{Value: 2}}
	if usage.Multiplicity == nil || *usage.Multiplicity != want {
		t.Errorf("Multiplicity = %+v, want %+v", usage.Multiplicity, want)
	}
}

// TestModelParseUsageSubsets checks that both spellings of a subsetting
// ("subsets a" and ":> a") parse into the same Usage.Subsets, and that an
// untyped usage can subset without ever mentioning a type.
func TestModelParseUsageSubsets(t *testing.T) {
	cases := map[string]string{
		"keyword": `part b subsets a;`,
		"symbol":  `part b :> a;`,
	}

	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { part a; `+source+` }`)

			usage, ok := pkg.Members[1].(*sysml.Usage)
			if !ok {
				t.Fatalf("member 1 is %T, want *sysml.Usage", pkg.Members[1])
			}
			if usage.Subsets != "a" {
				t.Errorf("Subsets = %q, want %q", usage.Subsets, "a")
			}
			if usage.Type != "" {
				t.Errorf("Type = %q, want %q", usage.Type, "")
			}
		})
	}
}

// TestModelParseUsageRedefines checks that both spellings of a redefinition
// ("redefines B::b" and ":>> B::b") parse into the same Usage.Redefines,
// including a qualified target.
func TestModelParseUsageRedefines(t *testing.T) {
	cases := map[string]string{
		"keyword": `part B_b redefines B::b;`,
		"symbol":  `part B_b :>> B::b;`,
	}

	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { `+source+` }`)

			usage, ok := pkg.Members[0].(*sysml.Usage)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
			}
			if usage.Redefines != "B::b" {
				t.Errorf("Redefines = %q, want %q", usage.Redefines, "B::b")
			}
		})
	}
}

// TestModelParseUsageReferences checks that both spellings of a reference
// ("references B::b" and "::> B::b") parse into the same Usage.References,
// including a qualified target -- the same pair TestModelParseUsageRedefines
// checks for redefines.
func TestModelParseUsageReferences(t *testing.T) {
	cases := map[string]string{
		"keyword": `part B_b references B::b;`,
		"symbol":  `part B_b ::> B::b;`,
	}

	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { `+source+` }`)

			usage, ok := pkg.Members[0].(*sysml.Usage)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
			}
			if usage.References != "B::b" {
				t.Errorf("References = %q, want %q", usage.References, "B::b")
			}
		})
	}
}

// TestModelParseAnonymousReferencesWithValue checks the shape a real
// example uses ("attribute ::> m = ms.totalMass;"): an anonymous usage
// (no name at all) whose only specialization is a references, with an
// assigned value.
func TestModelParseAnonymousReferencesWithValue(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		attribute m : Real;
		attribute x : Real;
		attribute ::> m = x;
	}`)

	usage, ok := pkg.Members[2].(*sysml.Usage)
	if !ok {
		t.Fatalf("member 2 is %T, want *sysml.Usage", pkg.Members[2])
	}
	if usage.Name != "" || usage.References != "m" {
		t.Errorf("member 2 = %+v, want an anonymous usage referencing m", usage)
	}
	if ref, ok := usage.Value.(*sysml.NameRef); !ok || ref.Path != "x" {
		t.Errorf("Value = %+v, want NameRef{Path: x}", usage.Value)
	}
}

// TestModelParseUsageRedefinesAndReferences checks that a redefines and a
// references can appear together on the same anonymous usage, e.g. the
// "end :>> end1 ::> d1;" shape a real connection definition body uses.
func TestModelParseUsageRedefinesAndReferences(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		part end1;
		part d1;
		part x :>> end1 ::> d1;
	}`)

	usage, ok := pkg.Members[2].(*sysml.Usage)
	if !ok {
		t.Fatalf("member 2 is %T, want *sysml.Usage", pkg.Members[2])
	}
	if usage.Redefines != "end1" || usage.References != "d1" {
		t.Errorf("member 2 = %+v, want Redefines: end1, References: d1", usage)
	}
}

// TestModelParseUsageTypeAndSubsets checks that a typing and a subsetting
// can appear together on the same usage, in either order.
func TestModelParseUsageTypeAndSubsets(t *testing.T) {
	cases := map[string]string{
		"type then subsets": `part b : Wheel subsets wheels;`,
		"subsets then type": `part b subsets wheels : Wheel;`,
	}

	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { `+source+` }`)

			usage, ok := pkg.Members[0].(*sysml.Usage)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
			}
			if usage.Type != "Wheel" {
				t.Errorf("Type = %q, want %q", usage.Type, "Wheel")
			}
			if usage.Subsets != "wheels" {
				t.Errorf("Subsets = %q, want %q", usage.Subsets, "wheels")
			}
		})
	}
}

// TestModelParseDefinitionSpecializes checks a definition's own
// classifier-level specialization list (SubclassificationPart): both
// spellings ("specializes" and ":>"), a single target, and a
// comma-separated multi-target list.
func TestModelParseDefinitionSpecializes(t *testing.T) {
	cases := map[string]struct {
		source string
		want   []string
	}{
		"symbol, single target": {
			`part def Vehicle :> VehiclePart;`, []string{"VehiclePart"},
		},
		"keyword, single target": {
			`part def Vehicle specializes VehiclePart;`, []string{"VehiclePart"},
		},
		"multiple targets": {
			`part def Vehicle :> A, B;`, []string{"A", "B"},
		},
		"with body": {
			`part def Vehicle :> VehiclePart { attribute m : Real; }`, []string{"VehiclePart"},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { `+tt.source+` }`)

			def, ok := pkg.Members[0].(*sysml.Definition)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Definition", pkg.Members[0])
			}
			if !reflect.DeepEqual(def.Specializes, tt.want) {
				t.Errorf("Specializes = %#v, want %#v", def.Specializes, tt.want)
			}
		})
	}
}

// TestModelParseExpression checks the expression parser's tree shapes:
// each literal kind, a bare/qualified name reference, binary and unary
// operators, precedence climbing (including "**"'s right-associativity),
// left-associativity for same-precedence operators, parentheses
// overriding precedence, invocation expressions (zero, one, and multiple
// arguments; a qualified callee; nested inside a binary expression), and
// sequence expressions (two or more comma-separated elements -- and that
// a single-element parenthesized expression stays plain grouping, not a
// SequenceExpr).
func TestModelParseExpression(t *testing.T) {
	cases := map[string]struct {
		source string
		want   sysml.Expression
	}{
		"bool true":  {"true", &sysml.BoolLiteral{Value: true}},
		"bool false": {"false", &sysml.BoolLiteral{Value: false}},
		"int":        {"5", &sysml.IntLiteral{Value: 5}},
		"real":       {"3.14", &sysml.RealLiteral{Value: 3.14}},
		"string":     {`"hello"`, &sysml.StringLiteral{Value: "hello"}},
		"name ref":   {"massLimit", &sysml.NameRef{Path: "massLimit"}},
		"qualified name ref": {
			"Vehicle::mass", &sysml.NameRef{Path: "Vehicle::mass"},
		},
		"feature chain": {
			"vehicle.chassis.mass",
			&sysml.FeatureChain{Path: []string{"vehicle", "chassis", "mass"}},
		},
		"feature chain with qualified first segment": {
			"Vehicle::vehicle.mass",
			&sysml.FeatureChain{Path: []string{"Vehicle::vehicle", "mass"}},
		},
		"feature chain in binary expression": {
			"vehicle.mass <= massLimit",
			&sysml.BinaryExpr{
				Op:    "<=",
				Left:  &sysml.FeatureChain{Path: []string{"vehicle", "mass"}},
				Right: &sysml.NameRef{Path: "massLimit"},
			},
		},
		"binary add": {
			"1 + 2",
			&sysml.BinaryExpr{Op: "+", Left: &sysml.IntLiteral{Value: 1}, Right: &sysml.IntLiteral{Value: 2}},
		},
		"comparison": {
			"mass <= massLimit",
			&sysml.BinaryExpr{Op: "<=", Left: &sysml.NameRef{Path: "mass"}, Right: &sysml.NameRef{Path: "massLimit"}},
		},
		"multiplication binds tighter than addition": {
			"1 + 2 * 3",
			&sysml.BinaryExpr{
				Op:    "+",
				Left:  &sysml.IntLiteral{Value: 1},
				Right: &sysml.BinaryExpr{Op: "*", Left: &sysml.IntLiteral{Value: 2}, Right: &sysml.IntLiteral{Value: 3}},
			},
		},
		"same precedence is left-associative": {
			"1 + 2 - 3",
			&sysml.BinaryExpr{
				Op:    "-",
				Left:  &sysml.BinaryExpr{Op: "+", Left: &sysml.IntLiteral{Value: 1}, Right: &sysml.IntLiteral{Value: 2}},
				Right: &sysml.IntLiteral{Value: 3},
			},
		},
		"parens override precedence": {
			"(1 + 2) * 3",
			&sysml.BinaryExpr{
				Op:    "*",
				Left:  &sysml.BinaryExpr{Op: "+", Left: &sysml.IntLiteral{Value: 1}, Right: &sysml.IntLiteral{Value: 2}},
				Right: &sysml.IntLiteral{Value: 3},
			},
		},
		"power is right-associative": {
			"2 ** 3 ** 2",
			&sysml.BinaryExpr{
				Op:    "**",
				Left:  &sysml.IntLiteral{Value: 2},
				Right: &sysml.BinaryExpr{Op: "**", Left: &sysml.IntLiteral{Value: 3}, Right: &sysml.IntLiteral{Value: 2}},
			},
		},
		"unary minus": {
			"-x", &sysml.UnaryExpr{Op: "-", Operand: &sysml.NameRef{Path: "x"}},
		},
		"unary not": {
			"not done", &sysml.UnaryExpr{Op: "not", Operand: &sysml.NameRef{Path: "done"}},
		},
		"stacked unary": {
			"- -x",
			&sysml.UnaryExpr{Op: "-", Operand: &sysml.UnaryExpr{Op: "-", Operand: &sysml.NameRef{Path: "x"}}},
		},
		"invocation, one arg": {
			"sum(componentMasses)",
			&sysml.InvocationExpr{Callee: "sum", Args: []sysml.Expression{&sysml.NameRef{Path: "componentMasses"}}},
		},
		"invocation, no args": {
			"now()", &sysml.InvocationExpr{Callee: "now"},
		},
		"invocation, multiple args": {
			"sum(a, b)",
			&sysml.InvocationExpr{Callee: "sum", Args: []sysml.Expression{&sysml.NameRef{Path: "a"}, &sysml.NameRef{Path: "b"}}},
		},
		"invocation, qualified callee": {
			"NumericalFunctions::sum(a)",
			&sysml.InvocationExpr{Callee: "NumericalFunctions::sum", Args: []sysml.Expression{&sysml.NameRef{Path: "a"}}},
		},
		"invocation in binary expression": {
			"totalMass == sum(componentMasses)",
			&sysml.BinaryExpr{
				Op:   "==",
				Left: &sysml.NameRef{Path: "totalMass"},
				Right: &sysml.InvocationExpr{
					Callee: "sum",
					Args:   []sysml.Expression{&sysml.NameRef{Path: "componentMasses"}},
				},
			},
		},
		"sequence, two elements": {
			"(vehicle, vehicle)",
			&sysml.SequenceExpr{Elements: []sysml.Expression{
				&sysml.NameRef{Path: "vehicle"}, &sysml.NameRef{Path: "vehicle"},
			}},
		},
		"sequence, three feature-chain elements": {
			"(engine.mass, frontAxleAssembly.mass, rearAxleAssembly.mass)",
			&sysml.SequenceExpr{Elements: []sysml.Expression{
				&sysml.FeatureChain{Path: []string{"engine", "mass"}},
				&sysml.FeatureChain{Path: []string{"frontAxleAssembly", "mass"}},
				&sysml.FeatureChain{Path: []string{"rearAxleAssembly", "mass"}},
			}},
		},
		"single-element parens stay plain grouping": {
			"(1 + 2)",
			&sysml.BinaryExpr{Op: "+", Left: &sysml.IntLiteral{Value: 1}, Right: &sysml.IntLiteral{Value: 2}},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { attribute n = `+tt.source+`; }`)

			usage, ok := pkg.Members[0].(*sysml.Usage)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
			}

			if !reflect.DeepEqual(usage.Value, tt.want) {
				t.Errorf("Value = %#v, want %#v", usage.Value, tt.want)
			}
		})
	}
}

// TestModelParseUsageValue checks that a usage's assigned value ("= 5") is
// parsed as an IntLiteral and attached to Usage.Value, and that a usage
// with no assigned value leaves it nil.
func TestModelParseUsageValue(t *testing.T) {
	cases := map[string]struct {
		source string
		want   *int
	}{
		"assigned":   {`attribute n : Integer = 5;`, ptr(5)},
		"no value":   {`attribute n : Integer;`, nil},
		"zero value": {`attribute n : Integer = 0;`, ptr(0)},
		"combined":   {`attribute n : Integer[1] = 5;`, ptr(5)},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { `+tt.source+` }`)

			usage, ok := pkg.Members[0].(*sysml.Usage)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
			}

			if tt.want == nil {
				if usage.Value != nil {
					t.Errorf("Value = %+v, want nil", usage.Value)
				}
				return
			}
			lit, ok := usage.Value.(*sysml.IntLiteral)
			if !ok {
				t.Fatalf("Value is %T, want *sysml.IntLiteral", usage.Value)
			}
			if lit.Value != *tt.want {
				t.Errorf("Value = %d, want %d", lit.Value, *tt.want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }

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

// TestModelParseImportWildcard checks that "import P1::*;" parses as a
// wildcard import of P1 (Wildcard set, Path holding just "P1" -- not
// "P1::*"), and that a plain "import P1;" leaves Wildcard false.
func TestModelParseImportWildcard(t *testing.T) {
	cases := map[string]struct {
		source       string
		wantPath     string
		wantWildcard bool
	}{
		"plain":              {`import P1;`, "P1", false},
		"wildcard":           {`import P1::*;`, "P1", true},
		"qualified wildcard": {`import Vehicle::Electrical::*;`, "Vehicle::Electrical", true},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Car { `+tt.source+` }`)

			imp, ok := pkg.Members[0].(*sysml.Import)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Import", pkg.Members[0])
			}
			if imp.Path != tt.wantPath || imp.Wildcard != tt.wantWildcard {
				t.Errorf("import = %+v, want Path: %q, Wildcard: %v", imp, tt.wantPath, tt.wantWildcard)
			}
		})
	}
}

// TestModelParseImportRecursive checks that "::**" (NamespaceImport's
// recursive form, "**" lexing as the same Power token "2 ** 3" uses) sets
// both Wildcard and Recursive, alongside a qualified path, and that a
// plain "::*" import leaves Recursive false.
func TestModelParseImportRecursive(t *testing.T) {
	cases := map[string]struct {
		source        string
		wantPath      string
		wantRecursive bool
	}{
		"recursive":           {`import P1::**;`, "P1", true},
		"qualified recursive": {`import Vehicle::Electrical::**;`, "Vehicle::Electrical", true},
		"plain wildcard":      {`import P1::*;`, "P1", false},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Car { `+tt.source+` }`)

			imp, ok := pkg.Members[0].(*sysml.Import)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Import", pkg.Members[0])
			}
			if imp.Path != tt.wantPath || !imp.Wildcard || imp.Recursive != tt.wantRecursive {
				t.Errorf("import = %+v, want Path: %q, Wildcard: true, Recursive: %v", imp, tt.wantPath, tt.wantRecursive)
			}
		})
	}
}

// TestModelParseVisibility checks that an optional leading
// public/private/protected keyword is parsed and attached to whichever kind
// of member follows it -- a package, a definition, a usage, or an import --
// and that omitting it leaves Visibility at its unspecified zero value.
func TestModelParseVisibility(t *testing.T) {
	cases := map[string]struct {
		member string
		want   sysml.Visibility
	}{
		"unspecified": {`part def Engine;`, sysml.VisibilityUnspecified},
		"public":      {`public part def Engine;`, sysml.VisibilityPublic},
		"private":     {`private part def Engine;`, sysml.VisibilityPrivate},
		"protected":   {`protected part def Engine;`, sysml.VisibilityProtected},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { `+tt.member+` }`)

			def, ok := pkg.Members[0].(*sysml.Definition)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Definition", pkg.Members[0])
			}
			if def.Visibility != tt.want {
				t.Errorf("Visibility = %v, want %v", def.Visibility, tt.want)
			}
		})
	}
}

// TestModelParseVisibilityOnEveryMemberKind checks that a package, a usage,
// and an import each accept the same leading visibility prefix as a
// definition does -- one shared parseVisibility step ahead of whichever
// member follows, not something special-cased per keyword.
func TestModelParseVisibilityOnEveryMemberKind(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		private package Sub { }
		public part def Engine;
		protected part engine : Engine;
		private import Other;
	}`)

	if len(pkg.Members) != 4 {
		t.Fatalf("got %d members, want 4", len(pkg.Members))
	}

	sub, ok := pkg.Members[0].(*sysml.Package)
	if !ok || sub.Visibility != sysml.VisibilityPrivate {
		t.Errorf("member 0 = %+v, want a private Package", pkg.Members[0])
	}

	def, ok := pkg.Members[1].(*sysml.Definition)
	if !ok || def.Visibility != sysml.VisibilityPublic {
		t.Errorf("member 1 = %+v, want a public Definition", pkg.Members[1])
	}

	usage, ok := pkg.Members[2].(*sysml.Usage)
	if !ok || usage.Visibility != sysml.VisibilityProtected {
		t.Errorf("member 2 = %+v, want a protected Usage", pkg.Members[2])
	}

	imp, ok := pkg.Members[3].(*sysml.Import)
	if !ok || imp.Visibility != sysml.VisibilityPrivate {
		t.Errorf("member 3 = %+v, want a private Import", pkg.Members[3])
	}
}

// TestModelParseAbstractPrefix checks that a leading "abstract" keyword is
// accepted ahead of a definition, a usage, and a connection alike (every
// Member kind that has an Abstract field), the same shared-prefix pattern
// TestModelParseVisibilityOnEveryMemberKind already checks for visibility
// -- and that it's independently optional, alongside visibility and
// metadata, in whichever combination is present.
func TestModelParseAbstractPrefix(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		abstract part def Engine;
		part engine : Engine;
		abstract part eng2 : Engine;
		private #Classified abstract part eng3 : Engine;
		abstract connect engine to eng2;
	}`)

	if len(pkg.Members) != 5 {
		t.Fatalf("got %d members, want 5", len(pkg.Members))
	}

	def, ok := pkg.Members[0].(*sysml.Definition)
	if !ok || !def.Abstract {
		t.Errorf("member 0 = %+v, want an abstract Definition", pkg.Members[0])
	}

	plain, ok := pkg.Members[1].(*sysml.Usage)
	if !ok || plain.Abstract {
		t.Errorf("member 1 = %+v, want a non-abstract Usage", pkg.Members[1])
	}

	usage, ok := pkg.Members[2].(*sysml.Usage)
	if !ok || !usage.Abstract {
		t.Errorf("member 2 = %+v, want an abstract Usage", pkg.Members[2])
	}

	combined, ok := pkg.Members[3].(*sysml.Usage)
	if !ok || !combined.Abstract || combined.Visibility != sysml.VisibilityPrivate || !reflect.DeepEqual(combined.Metadata, []string{"Classified"}) {
		t.Errorf("member 3 = %+v, want an abstract, private, #Classified Usage", pkg.Members[3])
	}

	conn, ok := pkg.Members[4].(*sysml.Connection)
	if !ok || !conn.Abstract {
		t.Errorf("member 4 = %+v, want an abstract Connection", pkg.Members[4])
	}
}

// TestModelParseLibraryPrefix checks that a leading "library" keyword
// ahead of a package is recorded on Package.Library, and that an ordinary
// package without it leaves Library false.
func TestModelParseLibraryPrefix(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		library package Lib { }
		package Plain { }
	}`)

	lib, ok := pkg.Members[0].(*sysml.Package)
	if !ok || !lib.Library || lib.Name != "Lib" {
		t.Errorf("member 0 = %+v, want a library Package named Lib", pkg.Members[0])
	}

	plain, ok := pkg.Members[1].(*sysml.Package)
	if !ok || plain.Library {
		t.Errorf("member 1 = %+v, want a non-library Package", pkg.Members[1])
	}
}

// TestModelParseShortName checks a usage's and a definition's optional
// "<Name>" short name (Identification's declaredShortName), both as a bare
// identifier and as a single-quoted restricted name, alongside its own
// real name.
func TestModelParseShortName(t *testing.T) {
	t.Run("usage, restricted name", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			part def B;
			part <'1'> b : B;
		}`)

		usage, ok := pkg.Members[1].(*sysml.Usage)
		if !ok {
			t.Fatalf("member 1 is %T, want *sysml.Usage", pkg.Members[1])
		}
		if usage.ShortName != "1" || usage.Name != "b" || usage.Type != "B" {
			t.Errorf("member 1 = %+v, want ShortName: 1, Name: b, Type: B", usage)
		}
	})

	t.Run("definition, bare identifier", func(t *testing.T) {
		// Spaced out around "xx": '<'/'>' are only ever recognized as
		// their own token when they *start* a fresh scan (see scanColon's
		// own doc comment on '>' not being real punctuation) -- tight,
		// unspaced brackets around a bare (non-quoted) name, as PartTest
		// .sysml's own "<xx>" is actually written, get swallowed into one
		// greedy identifier ("xx>") instead. Not fixed here: every target
		// fixture for this increment only ever uses a quoted short name
		// (see the "restricted name" case above), which naturally
		// delimits itself regardless of spacing.
		pkg := parseTopLevelPackage(t, `package Vehicle { part def < xx > B; }`)

		def, ok := pkg.Members[0].(*sysml.Definition)
		if !ok {
			t.Fatalf("member 0 is %T, want *sysml.Definition", pkg.Members[0])
		}
		if def.ShortName != "xx" || def.Name != "B" {
			t.Errorf("member 0 = %+v, want ShortName: xx, Name: B", def)
		}
	})

	t.Run("no short name", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle { part b; }`)

		usage, ok := pkg.Members[0].(*sysml.Usage)
		if !ok {
			t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
		}
		if usage.ShortName != "" {
			t.Errorf("ShortName = %q, want %q", usage.ShortName, "")
		}
	})
}

// TestModelParseMetadataPrefixes checks that a member can carry one or more
// leading "#Tag" annotations (PrefixMetadataAnnotation), stacked in the
// order written, alongside its own leading visibility prefix.
func TestModelParseMetadataPrefixes(t *testing.T) {
	cases := map[string]struct {
		member string
		want   []string
	}{
		"none":             {`part engine : Engine;`, nil},
		"single":           {`#Classified part engine : Engine;`, []string{"Classified"}},
		"stacked":          {`#Classified #Security part engine : Engine;`, []string{"Classified", "Security"}},
		"qualified":        {`#Meta::Classified part engine : Engine;`, []string{"Meta::Classified"}},
		"after visibility": {`private #Classified part engine : Engine;`, []string{"Classified"}},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { `+tt.member+` }`)

			usage, ok := pkg.Members[0].(*sysml.Usage)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
			}
			if !reflect.DeepEqual(usage.Metadata, tt.want) {
				t.Errorf("Metadata = %#v, want %#v", usage.Metadata, tt.want)
			}
		})
	}
}

// TestModelParseMetadataOnEveryMemberKind checks that "#Tag" is accepted
// ahead of a package, a definition, a usage, and a connection alike (every
// Member kind that has a Metadata field), the same shared-prefix pattern
// TestModelParseVisibilityOnEveryMemberKind already checks for visibility.
func TestModelParseMetadataOnEveryMemberKind(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		#Classified package Sub { }
		#Classified part def Engine;
		#Classified part engine : Engine;
		#Classified connect engine to engine;
	}`)

	if len(pkg.Members) != 4 {
		t.Fatalf("got %d members, want 4", len(pkg.Members))
	}

	sub, ok := pkg.Members[0].(*sysml.Package)
	if !ok || !reflect.DeepEqual(sub.Metadata, []string{"Classified"}) {
		t.Errorf("member 0 = %+v, want a Package with Metadata [Classified]", pkg.Members[0])
	}

	def, ok := pkg.Members[1].(*sysml.Definition)
	if !ok || !reflect.DeepEqual(def.Metadata, []string{"Classified"}) {
		t.Errorf("member 1 = %+v, want a Definition with Metadata [Classified]", pkg.Members[1])
	}

	usage, ok := pkg.Members[2].(*sysml.Usage)
	if !ok || !reflect.DeepEqual(usage.Metadata, []string{"Classified"}) {
		t.Errorf("member 2 = %+v, want a Usage with Metadata [Classified]", pkg.Members[2])
	}

	conn, ok := pkg.Members[3].(*sysml.Connection)
	if !ok || !reflect.DeepEqual(conn.Metadata, []string{"Classified"}) {
		t.Errorf("member 3 = %+v, want a Connection with Metadata [Classified]", pkg.Members[3])
	}
}

// TestModelParseMetadataDefKind checks that "metadata"/"@" is accepted as a
// definition/usage keyword family alongside part/attribute/etc, and that
// its usage form accepts a plain member-list body (MetadataBody) rather
// than the calc/constraint shape.
func TestModelParseMetadataDefKind(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		metadata def Classified {
			attribute classificationLevel : Integer;
		}
		metadata x : Classified;
		@x2 : Classified {
			attribute other : Integer;
		}
	}`)

	def, ok := pkg.Members[0].(*sysml.Definition)
	if !ok {
		t.Fatalf("member 0 is %T, want *sysml.Definition", pkg.Members[0])
	}
	if def.Kind != sysml.DefMetadata || def.Name != "Classified" || len(def.Members) != 1 {
		t.Errorf("member 0 = %+v, want DefMetadata Classified with 1 member", def)
	}

	usage, ok := pkg.Members[1].(*sysml.Usage)
	if !ok {
		t.Fatalf("member 1 is %T, want *sysml.Usage", pkg.Members[1])
	}
	if usage.Kind != sysml.DefMetadata || usage.Name != "x" || usage.Type != "Classified" {
		t.Errorf("member 1 = %+v, want DefMetadata usage x : Classified", usage)
	}

	usage2, ok := pkg.Members[2].(*sysml.Usage)
	if !ok {
		t.Fatalf("member 2 is %T, want *sysml.Usage", pkg.Members[2])
	}
	if usage2.Kind != sysml.DefMetadata || usage2.Name != "x2" || len(usage2.Members) != 1 {
		t.Errorf("member 2 = %+v, want DefMetadata usage x2 with 1 member", usage2)
	}
}

// TestModelParseRequirementConcernCaseDefKeywords checks that "requirement",
// "concern", and "case" are each accepted as a definition/usage keyword
// family alongside every other one, dispatched through the same defKeywords
// table -- the same shape TestModelParseDefKeywords already checks for
// part/attribute/item/port/constraint/calc.
func TestModelParseRequirementConcernCaseDefKeywords(t *testing.T) {
	cases := map[string]struct {
		keyword string
		want    sysml.DefKind
	}{
		"requirement": {"requirement", sysml.DefRequirement},
		"concern":     {"concern", sysml.DefConcern},
		"case":        {"case", sysml.DefCase},
	}

	for name, tt := range cases {
		t.Run(name+" definition", func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { `+tt.keyword+` def X; }`)

			def, ok := pkg.Members[0].(*sysml.Definition)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Definition", pkg.Members[0])
			}
			if def.Kind != tt.want || def.Name != "X" {
				t.Errorf("member 0 = %+v, want Definition{Kind: %v, Name: X}", def, tt.want)
			}
		})

		t.Run(name+" usage", func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { `+tt.keyword+` def X; `+tt.keyword+` x : X; }`)

			usage, ok := pkg.Members[1].(*sysml.Usage)
			if !ok {
				t.Fatalf("member 1 is %T, want *sysml.Usage", pkg.Members[1])
			}
			if usage.Kind != tt.want || usage.Name != "x" || usage.Type != "X" {
				t.Errorf("member 1 = %+v, want Usage{Kind: %v, Name: x, Type: X}", usage, tt.want)
			}
		})
	}
}

// TestModelParseRequirementUsagePlainBody checks that a requirement (or
// concern) usage can carry a plain member-list body (RequirementBody) with
// no trailing result -- e.g. the nested "subject :>> mass = vehicle.mass;"
// a requirement usage typed by a requirement def carries in real examples.
func TestModelParseRequirementUsagePlainBody(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		requirement def MassRequirement;
		requirement vehicleMassRequirement : MassRequirement {
			subject :>> mass = p;
		}
	}`)

	usage, ok := pkg.Members[1].(*sysml.Usage)
	if !ok {
		t.Fatalf("member 1 is %T, want *sysml.Usage", pkg.Members[1])
	}
	if usage.Kind != sysml.DefRequirement || usage.Name != "vehicleMassRequirement" || usage.Type != "MassRequirement" {
		t.Errorf("member 1 = %+v, want a DefRequirement usage vehicleMassRequirement : MassRequirement", usage)
	}
	if len(usage.Members) != 1 {
		t.Fatalf("got %d members, want 1", len(usage.Members))
	}
	subject, ok := usage.Members[0].(*sysml.Usage)
	if !ok {
		t.Fatalf("nested member is %T, want *sysml.Usage", usage.Members[0])
	}
	if subject.Kind != sysml.DefSubject || subject.Name != "" || subject.Redefines != "mass" {
		t.Errorf("nested member = %+v, want an anonymous DefSubject usage redefining mass", subject)
	}
	if ref, ok := subject.Value.(*sysml.NameRef); !ok || ref.Path != "p" {
		t.Errorf("nested member Value = %+v, want NameRef{Path: p}", subject.Value)
	}
}

// TestModelParseSubjectActorStakeholderDefKeywords checks that "subject",
// "actor", and "stakeholder" are each accepted as a usage keyword family
// (the same plain, always ";"-terminated shape "part"/"attribute" have).
func TestModelParseSubjectActorStakeholderDefKeywords(t *testing.T) {
	cases := map[string]struct {
		keyword string
		want    sysml.DefKind
	}{
		"subject":     {"subject", sysml.DefSubject},
		"actor":       {"actor", sysml.DefActor},
		"stakeholder": {"stakeholder", sysml.DefStakeholder},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { part def P; `+tt.keyword+` s : P; }`)

			usage, ok := pkg.Members[1].(*sysml.Usage)
			if !ok {
				t.Fatalf("member 1 is %T, want *sysml.Usage", pkg.Members[1])
			}
			if usage.Kind != tt.want || usage.Name != "s" || usage.Type != "P" {
				t.Errorf("member 1 = %+v, want Usage{Kind: %v, Name: s, Type: P}", usage, tt.want)
			}
		})
	}
}

// TestModelParseParameterAndRefDefKeywords checks that "in"/"out"/"inout"/
// "return" (parameter-direction keywords) and "ref" (ReferenceUsage's own
// keyword) are each accepted as a usage keyword family, the same plain
// shape "subject"/"actor"/"stakeholder" have -- no dedicated parsing beyond
// the keyword itself.
func TestModelParseParameterAndRefDefKeywords(t *testing.T) {
	cases := map[string]struct {
		keyword string
		want    sysml.DefKind
	}{
		"in":     {"in", sysml.DefIn},
		"out":    {"out", sysml.DefOut},
		"inout":  {"inout", sysml.DefInOut},
		"return": {"return", sysml.DefReturn},
		"ref":    {"ref", sysml.DefRef},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { part def P; `+tt.keyword+` s : P; }`)

			usage, ok := pkg.Members[1].(*sysml.Usage)
			if !ok {
				t.Fatalf("member 1 is %T, want *sysml.Usage", pkg.Members[1])
			}
			if usage.Kind != tt.want || usage.Name != "s" || usage.Type != "P" {
				t.Errorf("member 1 = %+v, want Usage{Kind: %v, Name: s, Type: P}", usage, tt.want)
			}
		})
	}
}

// TestModelParseRefWithRedefinesNoType checks the real-example shape
// "ref :>> system;" -- an anonymous DefRef usage redefining an existing
// feature, with no type at all.
func TestModelParseRefWithRedefinesNoType(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		part system;
		part satisfactionContext {
			ref :>> system;
		}
	}`)

	ctx, ok := pkg.Members[1].(*sysml.Usage)
	if !ok {
		t.Fatalf("member 1 is %T, want *sysml.Usage", pkg.Members[1])
	}
	if len(ctx.Members) != 1 {
		t.Fatalf("got %d members, want 1", len(ctx.Members))
	}
	ref, ok := ctx.Members[0].(*sysml.Usage)
	if !ok {
		t.Fatalf("nested member is %T, want *sysml.Usage", ctx.Members[0])
	}
	if ref.Kind != sysml.DefRef || ref.Name != "" || ref.Redefines != "system" || ref.Type != "" {
		t.Errorf("nested member = %+v, want an anonymous, untyped DefRef usage redefining system", ref)
	}
}

// TestModelParseRequirementConstraintBareForm checks "assume"/"require"/
// "frame"'s plain reference shorthand (no inner "constraint"/"concern"
// keyword) -- a bare name (optionally with a multiplicity), parsed as an
// ordinary Usage of the matching DefKind.
func TestModelParseRequirementConstraintBareForm(t *testing.T) {
	cases := map[string]struct {
		member string
		want   sysml.DefKind
		name   string
	}{
		"assume":        {"assume c1;", sysml.DefAssume, "c1"},
		"require":       {"require c2 [0..*];", sysml.DefRequire, "c2"},
		"frame":         {"frame c3[0..*];", sysml.DefFrame, "c3"},
		"require plain": {"require c;", sysml.DefRequire, "c"},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { `+tt.member+` }`)

			usage, ok := pkg.Members[0].(*sysml.Usage)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
			}
			if usage.Kind != tt.want || usage.Name != tt.name {
				t.Errorf("member 0 = %+v, want Usage{Kind: %v, Name: %s}", usage, tt.want, tt.name)
			}
		})
	}
}

// TestModelParseRequirementConstraintInnerKeyword checks "assume"/"require"'s
// optional explicit "constraint" keyword: a named usage with a redefines
// (no body), and the bare, unnamed CalculationBody form ("require
// constraint { mass <= massLimit }") that's only reachable this way -- the
// bare reference shorthand above never has a body at all.
func TestModelParseRequirementConstraintInnerKeyword(t *testing.T) {
	t.Run("named with redefines", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			constraint c;
			require constraint c1 :>> c;
		}`)

		usage, ok := pkg.Members[1].(*sysml.Usage)
		if !ok {
			t.Fatalf("member 1 is %T, want *sysml.Usage", pkg.Members[1])
		}
		if usage.Kind != sysml.DefRequire || usage.Name != "c1" || usage.Redefines != "c" {
			t.Errorf("member 1 = %+v, want a DefRequire usage c1 redefining c", usage)
		}
	})

	t.Run("anonymous with calculation body", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			attribute mass : Real;
			attribute massLimit : Real;
			require constraint { mass <= massLimit }
		}`)

		usage, ok := pkg.Members[2].(*sysml.Usage)
		if !ok {
			t.Fatalf("member 2 is %T, want *sysml.Usage", pkg.Members[2])
		}
		if usage.Kind != sysml.DefRequire || usage.Name != "" {
			t.Errorf("member 2 = %+v, want an anonymous DefRequire usage", usage)
		}
		result, ok := usage.Result.(*sysml.BinaryExpr)
		if !ok || result.Op != "<=" {
			t.Errorf("Result = %+v, want a BinaryExpr for <=", usage.Result)
		}
	})

	t.Run("assume with explicit constraint keyword and type", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			constraint def C;
			assume constraint c1 : C;
		}`)

		usage, ok := pkg.Members[1].(*sysml.Usage)
		if !ok {
			t.Fatalf("member 1 is %T, want *sysml.Usage", pkg.Members[1])
		}
		if usage.Kind != sysml.DefAssume || usage.Name != "c1" || usage.Type != "C" {
			t.Errorf("member 1 = %+v, want a DefAssume usage c1 : C", usage)
		}
	})
}

// TestModelParseSatisfy checks every combination of Satisfy's independently
// optional "assert"/"not" prefixes, plus its optional "by" clause (a plain
// name or a dotted feature chain).
func TestModelParseSatisfy(t *testing.T) {
	cases := map[string]struct {
		member      string
		wantAssert  bool
		wantNegated bool
		wantBy      bool
	}{
		"bare":             {"satisfy r by p;", false, false, true},
		"assert":           {"assert satisfy r by p;", true, false, true},
		"not":              {"not satisfy r by p;", false, true, true},
		"assert not":       {"assert not satisfy r by p;", true, true, true},
		"no by":            {"satisfy r;", false, false, false},
		"by feature chain": {"satisfy r by p.chassis.mass;", false, false, true},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { `+tt.member+` }`)

			sat, ok := pkg.Members[0].(*sysml.Satisfy)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Satisfy", pkg.Members[0])
			}
			if sat.Assert != tt.wantAssert || sat.Negated != tt.wantNegated {
				t.Errorf("Satisfy{Assert: %v, Negated: %v}, want {%v, %v}", sat.Assert, sat.Negated, tt.wantAssert, tt.wantNegated)
			}
			if sat.Requirement != "r" {
				t.Errorf("Requirement = %q, want %q", sat.Requirement, "r")
			}
			if tt.wantBy && sat.By == nil {
				t.Error("By = nil, want a non-nil expression")
			}
			if !tt.wantBy && sat.By != nil {
				t.Errorf("By = %+v, want nil", sat.By)
			}
		})
	}
}

// TestModelParseSatisfyInlineDeclaration checks SatisfyRequirementUsage's
// other alternative -- an inline "requirement req1 : Req1" declaration of
// a brand new requirement usage, instead of a bare reference to an
// existing one -- in both its typed and untyped forms, and alongside a
// "by" clause.
func TestModelParseSatisfyInlineDeclaration(t *testing.T) {
	t.Run("typed, with by", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			satisfy requirement req1 : Req1 by system;
		}`)

		sat, ok := pkg.Members[0].(*sysml.Satisfy)
		if !ok {
			t.Fatalf("member 0 is %T, want *sysml.Satisfy", pkg.Members[0])
		}
		if sat.Requirement != "" {
			t.Errorf("Requirement = %q, want %q (empty)", sat.Requirement, "")
		}
		if sat.Declaration == nil {
			t.Fatal("Declaration = nil, want a *sysml.Usage")
		}
		if sat.Declaration.Kind != sysml.DefRequirement || sat.Declaration.Name != "req1" || sat.Declaration.Type != "Req1" {
			t.Errorf("Declaration = %+v, want a DefRequirement usage req1 : Req1", sat.Declaration)
		}
		ref, ok := sat.By.(*sysml.NameRef)
		if !ok || ref.Path != "system" {
			t.Errorf("By = %+v, want NameRef{Path: system}", sat.By)
		}
	})

	t.Run("untyped, no by", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			satisfy requirement req1;
		}`)

		sat, ok := pkg.Members[0].(*sysml.Satisfy)
		if !ok {
			t.Fatalf("member 0 is %T, want *sysml.Satisfy", pkg.Members[0])
		}
		if sat.Declaration == nil || sat.Declaration.Name != "req1" || sat.Declaration.Type != "" {
			t.Errorf("Declaration = %+v, want an untyped DefRequirement usage named req1", sat.Declaration)
		}
		if sat.By != nil {
			t.Errorf("By = %+v, want nil", sat.By)
		}
	})

	t.Run("by feature chain", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			satisfy requirement req1_1 : Req1_1 by system.sub1;
		}`)

		sat, ok := pkg.Members[0].(*sysml.Satisfy)
		if !ok {
			t.Fatalf("member 0 is %T, want *sysml.Satisfy", pkg.Members[0])
		}
		fc, ok := sat.By.(*sysml.FeatureChain)
		if !ok || !reflect.DeepEqual(fc.Path, []string{"system", "sub1"}) {
			t.Errorf("By = %+v, want FeatureChain{Path: [system sub1]}", sat.By)
		}
	})
}

// TestModelParseAssertConstraint checks AssertConstraintUsage's forms:
// "constraint" + a named/typed declaration with a CalculationBody,
// "constraint" + a named/typed declaration with no body at all (a bare
// ";"), a bare reference to an existing constraint with a body, and the
// fully anonymous "assert constraint { ... }" form -- each independently
// combined with "not".
func TestModelParseAssertConstraint(t *testing.T) {
	t.Run("constraint keyword, named and typed, with body", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			assert constraint massAnalysis : MassAnalysis {
				attribute redefines totalMass;
			}
		}`)

		usage, ok := pkg.Members[0].(*sysml.Usage)
		if !ok {
			t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
		}
		if usage.Kind != sysml.DefConstraint || usage.Name != "massAnalysis" || usage.Type != "MassAnalysis" {
			t.Errorf("member 0 = %+v, want a DefConstraint usage massAnalysis : MassAnalysis", usage)
		}
		if !usage.Assert || usage.Negated {
			t.Errorf("Assert = %v, Negated = %v, want true, false", usage.Assert, usage.Negated)
		}
		if len(usage.Members) != 1 {
			t.Fatalf("got %d members, want 1", len(usage.Members))
		}
	})

	t.Run("constraint keyword, named and typed, no body", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			assert constraint massConstraint : MassAnalysis2;
		}`)

		usage, ok := pkg.Members[0].(*sysml.Usage)
		if !ok {
			t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
		}
		if usage.Name != "massConstraint" || usage.Type != "MassAnalysis2" || usage.Members != nil {
			t.Errorf("member 0 = %+v, want massConstraint : MassAnalysis2, no members", usage)
		}
	})

	t.Run("bare reference, with body", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			assert massAnalysis3 {
				in totalMass = mass;
			}
		}`)

		usage, ok := pkg.Members[0].(*sysml.Usage)
		if !ok {
			t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
		}
		if usage.Name != "" || usage.References != "massAnalysis3" {
			t.Errorf("member 0 = %+v, want an anonymous usage referencing massAnalysis3", usage)
		}
		if len(usage.Members) != 1 {
			t.Fatalf("got %d members, want 1", len(usage.Members))
		}
	})

	t.Run("anonymous, with trailing result", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			assert constraint {
				mass == engine.mass + frontAxleAssembly.mass
			}
		}`)

		usage, ok := pkg.Members[0].(*sysml.Usage)
		if !ok {
			t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
		}
		if usage.Name != "" || usage.Type != "" {
			t.Errorf("member 0 = %+v, want a fully anonymous, untyped usage", usage)
		}
		result, ok := usage.Result.(*sysml.BinaryExpr)
		if !ok || result.Op != "==" {
			t.Errorf("Result = %+v, want a '==' BinaryExpr", usage.Result)
		}
	})

	t.Run("not, bare reference", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle {
			assert not massLimitation {
				attribute redefines mass = vehicle3.mass;
			}
		}`)

		usage, ok := pkg.Members[0].(*sysml.Usage)
		if !ok {
			t.Fatalf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
		}
		if !usage.Assert || !usage.Negated || usage.References != "massLimitation" {
			t.Errorf("member 0 = %+v, want Assert: true, Negated: true, References: massLimitation", usage)
		}
	})
}

// TestModelParseAssertConstraintVsSatisfyDispatch checks that "assert"
// correctly dispatches to Satisfy when followed by (optionally "not"
// then) "satisfy", and to AssertConstraintUsage otherwise -- in every
// combination of the two prefixes.
func TestModelParseAssertConstraintVsSatisfyDispatch(t *testing.T) {
	cases := map[string]struct {
		source      string
		wantSatisfy bool
	}{
		"assert satisfy":        {"assert satisfy r;", true},
		"assert not satisfy":    {"assert not satisfy r;", true},
		"assert constraint":     {"assert constraint { true }", false},
		"assert not constraint": {"assert not c { true }", false},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { `+tt.source+` }`)

			_, isSatisfy := pkg.Members[0].(*sysml.Satisfy)
			_, isUsage := pkg.Members[0].(*sysml.Usage)
			if tt.wantSatisfy && !isSatisfy {
				t.Errorf("member 0 is %T, want *sysml.Satisfy", pkg.Members[0])
			}
			if !tt.wantSatisfy && !isUsage {
				t.Errorf("member 0 is %T, want *sysml.Usage", pkg.Members[0])
			}
		})
	}
}

// TestModelParseSatisfyNotDisambiguatesFromUnaryExpression checks that a
// calc body's trailing "not X" result expression still parses as a unary
// expression, not a Satisfy member -- the ambiguity startsSatisfyNot exists
// to resolve (see its doc comment): only "not satisfy" is a member.
func TestModelParseSatisfyNotDisambiguatesFromUnaryExpression(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		attribute done : Boolean;
		constraint def C {
			not done
		}
	}`)

	def, ok := pkg.Members[1].(*sysml.Definition)
	if !ok {
		t.Fatalf("member 1 is %T, want *sysml.Definition", pkg.Members[1])
	}
	result, ok := def.Result.(*sysml.UnaryExpr)
	if !ok || result.Op != "not" {
		t.Errorf("Result = %+v, want UnaryExpr{Op: not}", def.Result)
	}
}

// TestModelParseBareMember checks that a keyword-less (bare) member -- no
// "attribute" or other defKeywords prefix at all -- parses as an ordinary
// DefAttribute Usage, across each of the four ways a
// FeatureSpecializationPart/ValuePart/bare terminator can follow the name
// directly (see startsBareMember's own doc comment for why each of these,
// and only these, disambiguates a bare member's start from a calc body's
// trailing result expression).
func TestModelParseBareMember(t *testing.T) {
	cases := map[string]struct {
		source   string
		wantName string
		wantType string
	}{
		"typed":        {"mass : MassValue;", "mass", "MassValue"},
		"multiplicity": {"items [3];", "items", ""},
		"valued":       {"count = 5;", "count", ""},
		"terminator":   {"flag;", "flag", ""},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { constraint def C { `+tt.source+` } }`)

			def, ok := pkg.Members[0].(*sysml.Definition)
			if !ok {
				t.Fatalf("member 0 is %T, want *sysml.Definition", pkg.Members[0])
			}
			if len(def.Members) != 1 {
				t.Fatalf("got %d members, want 1", len(def.Members))
			}
			usage, ok := def.Members[0].(*sysml.Usage)
			if !ok {
				t.Fatalf("body member is %T, want *sysml.Usage", def.Members[0])
			}
			if usage.Kind != sysml.DefAttribute || usage.Name != tt.wantName || usage.Type != tt.wantType {
				t.Errorf("member = %+v, want a DefAttribute %s : %q", usage, tt.wantName, tt.wantType)
			}
		})
	}

	t.Run("valued, value content", func(t *testing.T) {
		pkg := parseTopLevelPackage(t, `package Vehicle { constraint def C { count = 5; } }`)

		def := pkg.Members[0].(*sysml.Definition)
		usage := def.Members[0].(*sysml.Usage)
		lit, ok := usage.Value.(*sysml.IntLiteral)
		if !ok || lit.Value != 5 {
			t.Errorf("Value = %+v, want IntLiteral{5}", usage.Value)
		}
	})
}

// TestModelParseBareMemberSubsetsRedefinesReferences checks that a
// keyword-less member can also start directly with "subsets"/"redefines"/
// "references" (or their symbol forms) and no name at all -- e.g. the
// ":>> mass = vehicle3.mass;" inside "assert not massLimitation { ... }" --
// unambiguous without any lookahead, since none of those three tokens can
// ever start an expression either (see startsMember's own doc comment).
func TestModelParseBareMemberSubsetsRedefinesReferences(t *testing.T) {
	cases := map[string]struct {
		source string
		check  func(t *testing.T, usage *sysml.Usage)
	}{
		"subsets": {
			"part a; :> a;",
			func(t *testing.T, usage *sysml.Usage) {
				if usage.Name != "" || usage.Subsets != "a" {
					t.Errorf("usage = %+v, want an anonymous usage subsetting a", usage)
				}
			},
		},
		"redefines": {
			"part b; :>> b = x;",
			func(t *testing.T, usage *sysml.Usage) {
				if usage.Name != "" || usage.Redefines != "b" {
					t.Errorf("usage = %+v, want an anonymous usage redefining b", usage)
				}
			},
		},
		"references": {
			"part c; ::> c;",
			func(t *testing.T, usage *sysml.Usage) {
				if usage.Name != "" || usage.References != "c" {
					t.Errorf("usage = %+v, want an anonymous usage referencing c", usage)
				}
			},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			pkg := parseTopLevelPackage(t, `package Vehicle { attribute x : Real; `+tt.source+` }`)

			usage, ok := pkg.Members[len(pkg.Members)-1].(*sysml.Usage)
			if !ok {
				t.Fatalf("last member is %T, want *sysml.Usage", pkg.Members[len(pkg.Members)-1])
			}
			if usage.Kind != sysml.DefAttribute {
				t.Errorf("Kind = %v, want DefAttribute", usage.Kind)
			}
			tt.check(t, usage)
		})
	}
}

// TestModelParseBareMemberVsTrailingResult checks the trickiest
// disambiguation this phase introduces: a calc body mixing bare, keyword-
// less members with a trailing result expression that also starts with a
// bare identifier -- e.g. ConstraintTest.sysml's own
// "mass : MassValue; massLimit : MassValue; mass < massLimit" shape. Both
// "mass : MassValue;" and "mass < massLimit" start with the identifier
// "mass", so startsBareMember's one-token-past-the-identifier lookahead is
// what tells them apart.
func TestModelParseBareMemberVsTrailingResult(t *testing.T) {
	pkg := parseTopLevelPackage(t, `package Vehicle {
		constraint def C {
			mass : MassValue;
			massLimit : MassValue;
			mass < massLimit
		}
	}`)

	def, ok := pkg.Members[0].(*sysml.Definition)
	if !ok {
		t.Fatalf("member 0 is %T, want *sysml.Definition", pkg.Members[0])
	}
	if len(def.Members) != 2 {
		t.Fatalf("got %d members, want 2", len(def.Members))
	}
	for i, wantName := range []string{"mass", "massLimit"} {
		usage, ok := def.Members[i].(*sysml.Usage)
		if !ok || usage.Kind != sysml.DefAttribute || usage.Name != wantName || usage.Type != "MassValue" {
			t.Errorf("member %d = %+v, want a bare DefAttribute %s : MassValue", i, def.Members[i], wantName)
		}
	}
	result, ok := def.Result.(*sysml.BinaryExpr)
	if !ok || result.Op != "<" {
		t.Errorf("Result = %+v, want a '<' BinaryExpr", def.Result)
	}
	left, ok := result.Left.(*sysml.NameRef)
	if !ok || left.Path != "mass" {
		t.Errorf("Result.Left = %+v, want NameRef{Path: mass}", result.Left)
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
		"unterminated package body":                   "package Vehicle {",
		"missing package braces":                      "package Vehicle",
		"part usage missing type":                     "package Vehicle { part engine : ; }",
		"part usage duplicate type":                   "package Vehicle { part engine : Engine : Engine; }",
		"part usage duplicate multiplicity":           "package Vehicle { part engine : Engine[1][1]; }",
		"part usage duplicate subsets":                "package Vehicle { part b subsets a subsets a; }",
		"part usage duplicate redefines":              "package Vehicle { part b redefines a redefines a; }",
		"part usage duplicate references":             "package Vehicle { part b references a references a; }",
		"definition specializes missing target":       "package Vehicle { part def V :> ; }",
		"definition specializes trailing comma":       "package Vehicle { part def V :> A,; }",
		"unmatched closing brace":                     "package Vehicle { } }",
		"qualified type missing segment":              "package Vehicle { part engine : Vehicle::; }",
		"qualified type trailing path sep":            "package Vehicle { part engine : Vehicle::Engine::; }",
		"import missing path":                         "package Vehicle { import; }",
		"import missing semicolon":                    "package Vehicle { import Engine }",
		"connection missing to":                       "package Vehicle { connect a b; }",
		"connection missing close paren":              "package Vehicle { connect (a, b; }",
		"connection missing end":                      "package Vehicle { connect a to ; }",
		"bind missing equals":                         "package Vehicle { bind a b; }",
		"bind missing right end":                      "package Vehicle { bind a = ; }",
		"binding missing bind keyword":                "package Vehicle { binding ab a = b; }",
		"multiplicity missing bound":                  "package Vehicle { part combatants : Combatant[]; }",
		"multiplicity missing close":                  "package Vehicle { part combatants : Combatant[*; }",
		"multiplicity punctuation bound":              "package Vehicle { part combatants : Combatant[;]; }",
		"multiplicity negative bound":                 "package Vehicle { part combatants : Combatant[-1]; }",
		"assigned value missing operand":              "package Vehicle { attribute n : Integer = ; }",
		"assigned value trailing operator":            "package Vehicle { attribute n : Integer = 1 + ; }",
		"assigned value unclosed paren":               "package Vehicle { attribute n : Integer = (1 + 2; }",
		"invocation missing close paren":              "package Vehicle { attribute n = sum(a, b; }",
		"invocation trailing comma":                   "package Vehicle { attribute n = sum(a,); }",
		"invocation chained call not supported":       "package Vehicle { attribute n = sum(a)(b); }",
		"sequence trailing comma":                     "package Vehicle { attribute n = (a, b,); }",
		"sequence missing close paren":                "package Vehicle { attribute n = (a, b; }",
		"metadata prefix missing tag":                 "package Vehicle { # part engine : Engine; }",
		"metadata prefix qualified trailing path sep": "package Vehicle { #Meta:: part engine : Engine; }",
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
