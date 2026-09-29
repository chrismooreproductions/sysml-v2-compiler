package metamodel_test

import (
	"testing"

	"github.com/chrismooreproductions/sysml-modeller/metamodel"
	"github.com/chrismooreproductions/sysml-modeller/sysml"
)

// TestModelResolve exercises Resolve directly (rather than only indirectly
// through FromAST's own use of it for Usage typing), since it's public
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

	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
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
			"2": {ID: "2", Kind: metamodel.KindDefinition, Name: "Car", Owner: "1"},
			"3": {ID: "3", Kind: metamodel.KindUsage, Name: "engine", Owner: "2"},
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
// every element except the synthetic root itself (which has no name of its
// own to derive an ID from -- see rootID's doc comment). If ID generation
// ever changes for the rest (e.g. to opaque IDs that survive renames), this
// test is the one that should start failing.
func TestQualifiedNameMatchesID(t *testing.T) {
	m := sysml.NewModel(`package Vehicle {
		part def Engine;
		part def Car {
			part engine : Engine;
		}
	}`)

	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	for id := range model.Elements {
		if id == model.Root {
			continue
		}
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

	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	want := map[metamodel.ElementID]metamodel.Element{
		model.Root:             {ID: model.Root, Kind: metamodel.KindNamespace},
		"Vehicle":              {ID: "Vehicle", Kind: metamodel.KindPackage, Name: "Vehicle", Owner: model.Root},
		"Vehicle::Engine":      {ID: "Vehicle::Engine", Kind: metamodel.KindDefinition, Name: "Engine", Owner: "Vehicle"},
		"Vehicle::Car":         {ID: "Vehicle::Car", Kind: metamodel.KindDefinition, Name: "Car", Owner: "Vehicle"},
		"Vehicle::Car::engine": {ID: "Vehicle::Car::engine", Kind: metamodel.KindUsage, Name: "engine", Owner: "Vehicle::Car"},
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

// TestFromASTMultiplicity checks that a Usage's bracketed multiplicity
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

	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	tests := []struct {
		id   metamodel.ElementID
		want *metamodel.Multiplicity
	}{
		{"Battlefield::solo", nil},
		{"Battlefield::friendlyCombatants", &metamodel.Multiplicity{Lower: metamodel.Bound{Value: 0}, Upper: metamodel.Bound{Value: metamodel.Unbounded}}},
		{"Battlefield::enemyCombatants", &metamodel.Multiplicity{Lower: metamodel.Bound{Value: 1}, Upper: metamodel.Bound{Value: metamodel.Unbounded}}},
		{"Battlefield::squad", &metamodel.Multiplicity{Lower: metamodel.Bound{Value: 5}, Upper: metamodel.Bound{Value: 5}}},
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

// TestFromASTUntypedUsage checks that an untyped usage (e.g. "port p;")
// translates without error and without a TypedBy relationship -- there's no
// type reference to queue as pendingType or resolve, unlike every other
// usage this package translates.
func TestFromASTUntypedUsage(t *testing.T) {
	ns, err := sysml.NewModel(`package Vehicle { port p; }`).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	if _, ok := model.Elements["Vehicle::p"]; !ok {
		t.Fatal("missing element Vehicle::p")
	}
	if _, ok := typeOf(model, "Vehicle::p"); ok {
		t.Error("untyped usage has a TypedBy relationship, want none")
	}
}

// TestFromASTUsageValue checks that a usage's assigned value carries over
// onto Element.Value, and that a usage with none leaves it nil.
func TestFromASTUsageValue(t *testing.T) {
	source := `package Vehicle {
		attribute def Integer;
		attribute n : Integer = 5;
		attribute m : Integer;
	}`

	ns, err := sysml.NewModel(source).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	n, ok := model.Elements["Vehicle::n"]
	if !ok {
		t.Fatal("missing element Vehicle::n")
	}
	lit, ok := n.Value.(*metamodel.IntLiteral)
	if !ok {
		t.Fatalf("n.Value is %T, want *metamodel.IntLiteral", n.Value)
	}
	if lit.Value != 5 {
		t.Errorf("n.Value = %d, want 5", lit.Value)
	}

	m, ok := model.Elements["Vehicle::m"]
	if !ok {
		t.Fatal("missing element Vehicle::m")
	}
	if m.Value != nil {
		t.Errorf("m.Value = %+v, want nil", m.Value)
	}
}

// TestFromASTExpressionNameRef checks that a NameRef inside a usage's
// value resolves to the sibling it names, the same way a type reference
// does.
func TestFromASTExpressionNameRef(t *testing.T) {
	ns, err := sysml.NewModel(`package Vehicle {
		attribute def Real;
		attribute massLimit : Real;
		attribute n : Real = massLimit;
	}`).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	n, ok := model.Elements["Vehicle::n"]
	if !ok {
		t.Fatal("missing element Vehicle::n")
	}
	ref, ok := n.Value.(*metamodel.NameRef)
	if !ok {
		t.Fatalf("n.Value is %T, want *metamodel.NameRef", n.Value)
	}
	if ref.Target != "Vehicle::massLimit" {
		t.Errorf("NameRef.Target = %q, want %q", ref.Target, "Vehicle::massLimit")
	}
}

// TestFromASTExpressionUnresolvedNameRef checks that a NameRef with no
// matching declaration fails translation, the same as an unresolved type
// does.
func TestFromASTExpressionUnresolvedNameRef(t *testing.T) {
	ns, err := sysml.NewModel(`package Vehicle { attribute def Real; attribute n : Real = nope; }`).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if _, err := metamodel.FromAST(ns); err == nil {
		t.Error("expected an error for an unresolved name reference, got nil")
	}
}

// TestFromASTExpressionOperatorResolvesToStdlib checks that a binary and a
// unary operator each resolve their Function field to the matching
// standard-library stub (see metamodel/stdlib.go) -- the "real
// library-function resolution" this project's expression subsystem is
// built around, not an intrinsic/built-in operator.
func TestFromASTExpressionOperatorResolvesToStdlib(t *testing.T) {
	ns, err := sysml.NewModel(`package Vehicle {
		attribute def Real;
		attribute mass : Real;
		attribute massLimit : Real;
		attribute ok : Real = mass <= massLimit;
		attribute negated : Real = -mass;
	}`).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	ok, found := model.Elements["Vehicle::ok"]
	if !found {
		t.Fatal("missing element Vehicle::ok")
	}
	bin, isBin := ok.Value.(*metamodel.BinaryExpr)
	if !isBin {
		t.Fatalf("ok.Value is %T, want *metamodel.BinaryExpr", ok.Value)
	}
	if bin.Function != "ScalarFunctions::<=" {
		t.Errorf("BinaryExpr.Function = %q, want %q", bin.Function, "ScalarFunctions::<=")
	}
	left, isRef := bin.Left.(*metamodel.NameRef)
	if !isRef || left.Target != "Vehicle::mass" {
		t.Errorf("BinaryExpr.Left = %+v, want a NameRef targeting Vehicle::mass", bin.Left)
	}
	right, isRef := bin.Right.(*metamodel.NameRef)
	if !isRef || right.Target != "Vehicle::massLimit" {
		t.Errorf("BinaryExpr.Right = %+v, want a NameRef targeting Vehicle::massLimit", bin.Right)
	}

	negated, found := model.Elements["Vehicle::negated"]
	if !found {
		t.Fatal("missing element Vehicle::negated")
	}
	un, isUn := negated.Value.(*metamodel.UnaryExpr)
	if !isUn {
		t.Fatalf("negated.Value is %T, want *metamodel.UnaryExpr", negated.Value)
	}
	if un.Function != "ScalarFunctions::-" {
		t.Errorf("UnaryExpr.Function = %q, want %q", un.Function, "ScalarFunctions::-")
	}
}

// TestFromASTFeatureChain checks the VehicleRequirementDerivation.sysml
// shape -- a multi-segment feature chain ("vehicle.chassis.mass") --
// resolving each segment after the first by walking the previous
// segment's TypedBy relationship rather than by namespace containment.
func TestFromASTFeatureChain(t *testing.T) {
	ns, err := sysml.NewModel(`package Vehicle {
		attribute def Real;

		part def Chassis {
			attribute mass : Real;
		}
		part def Car {
			part chassis : Chassis;
		}

		part vehicle : Car;
		attribute n : Real = vehicle.chassis.mass;
	}`).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	n, ok := model.Elements["Vehicle::n"]
	if !ok {
		t.Fatal("missing element Vehicle::n")
	}
	fc, ok := n.Value.(*metamodel.FeatureChain)
	if !ok {
		t.Fatalf("n.Value is %T, want *metamodel.FeatureChain", n.Value)
	}
	if fc.Target != "Vehicle::Chassis::mass" {
		t.Errorf("FeatureChain.Target = %q, want %q", fc.Target, "Vehicle::Chassis::mass")
	}
}

// TestFromASTFeatureChainErrors checks that a feature chain fails
// translation both when its first segment doesn't resolve at all, and
// when a later segment names something that isn't a member of the
// previous segment's type.
func TestFromASTFeatureChainErrors(t *testing.T) {
	cases := map[string]string{
		"unresolved first segment": `package Vehicle {
			attribute def Real;
			attribute n : Real = nope.mass;
		}`,
		"unresolved later segment": `package Vehicle {
			attribute def Real;
			part def Chassis;
			part vehicle : Chassis;
			attribute n : Real = vehicle.nope;
		}`,
	}

	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			ns, err := sysml.NewModel(source).Parse()
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			if _, err := metamodel.FromAST(ns); err == nil {
				t.Error("expected an error for an unresolved feature chain, got nil")
			}
		})
	}
}

// TestFromASTConstraintDefinitionResult checks that a constraint
// definition's trailing result expression resolves its names against the
// definition's own body -- "totalMass <= massLimit" needs to see
// totalMass/massLimit as its own members, not as siblings of the
// definition itself.
func TestFromASTConstraintDefinitionResult(t *testing.T) {
	ns, err := sysml.NewModel(`package Vehicle {
		attribute def Real;
		constraint def MassAnalysis {
			attribute totalMass : Real;
			attribute massLimit : Real;
			totalMass <= massLimit
		}
	}`).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	def, ok := model.Elements["Vehicle::MassAnalysis"]
	if !ok {
		t.Fatal("missing element Vehicle::MassAnalysis")
	}
	result, ok := def.Result.(*metamodel.BinaryExpr)
	if !ok {
		t.Fatalf("Result is %T, want *metamodel.BinaryExpr", def.Result)
	}
	left, ok := result.Left.(*metamodel.NameRef)
	if !ok || left.Target != "Vehicle::MassAnalysis::totalMass" {
		t.Errorf("Result.Left = %+v, want a NameRef targeting Vehicle::MassAnalysis::totalMass", result.Left)
	}
	right, ok := result.Right.(*metamodel.NameRef)
	if !ok || right.Target != "Vehicle::MassAnalysis::massLimit" {
		t.Errorf("Result.Right = %+v, want a NameRef targeting Vehicle::MassAnalysis::massLimit", result.Right)
	}
}

// TestFromASTUsageCalculationBody checks that a named constraint/calc
// usage's own body (Members and Result) is declared and resolved the same
// way a definition's is -- as its own scope, not its containing one.
func TestFromASTUsageCalculationBody(t *testing.T) {
	ns, err := sysml.NewModel(`package Vehicle {
		attribute def Real;
		constraint def C;
		constraint check : C {
			attribute a : Real;
			attribute b : Real;
			a == b
		}
	}`).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	if _, ok := model.Elements["Vehicle::check::a"]; !ok {
		t.Error("missing element Vehicle::check::a")
	}

	check, ok := model.Elements["Vehicle::check"]
	if !ok {
		t.Fatal("missing element Vehicle::check")
	}
	result, ok := check.Result.(*metamodel.BinaryExpr)
	if !ok {
		t.Fatalf("Result is %T, want *metamodel.BinaryExpr", check.Result)
	}
	left, ok := result.Left.(*metamodel.NameRef)
	if !ok || left.Target != "Vehicle::check::a" {
		t.Errorf("Result.Left = %+v, want a NameRef targeting Vehicle::check::a", result.Left)
	}
}

// TestFromASTAnonymousUsage checks that an anonymous usage (no name --
// Phase 3's "constraint { ... }" form) still translates to its own
// distinct Element, and that two anonymous usages in the same scope don't
// collide with each other despite neither having a name to deduplicate
// against.
func TestFromASTAnonymousUsage(t *testing.T) {
	ns, err := sysml.NewModel(`package Vehicle {
		attribute def Real;
		attribute mass : Real;
		attribute massLimit : Real;
		constraint { mass <= massLimit }
		constraint { mass == mass }
	}`).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	var anonymous []*metamodel.Element
	for _, el := range model.Elements {
		if el.Owner == "Vehicle" && el.Kind == metamodel.KindUsage && el.DefKind == metamodel.DefConstraint {
			anonymous = append(anonymous, el)
		}
	}
	if len(anonymous) != 2 {
		t.Fatalf("got %d anonymous constraint usages, want 2", len(anonymous))
	}
	if anonymous[0].ID == anonymous[1].ID {
		t.Errorf("both anonymous usages share ID %q, want distinct IDs", anonymous[0].ID)
	}
	for _, el := range anonymous {
		if el.Name != "" {
			t.Errorf("anonymous usage Name = %q, want %q", el.Name, "")
		}
		if el.Result == nil {
			t.Errorf("anonymous usage %q has no Result", el.ID)
		}
	}
}

// TestFromASTVisibility checks that a package, definition, and usage each
// carry their parsed sysml.Visibility over onto their Element unchanged
// (including the unspecified case), across the sysml.Visibility ->
// metamodel.Visibility conversion translate.go does by enum-order
// convention rather than a lookup table.
func TestFromASTVisibility(t *testing.T) {
	source := `package Vehicle {
		private package Sub { }
		public part def Engine;
		part engine : Engine;
	}`

	ns, err := sysml.NewModel(source).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	tests := []struct {
		id   metamodel.ElementID
		want metamodel.Visibility
	}{
		{"Vehicle::Sub", metamodel.VisibilityPrivate},
		{"Vehicle::Engine", metamodel.VisibilityPublic},
		{"Vehicle::engine", metamodel.VisibilityUnspecified},
	}

	for _, tt := range tests {
		el, ok := model.Elements[tt.id]
		if !ok {
			t.Fatalf("missing element %q", tt.id)
		}
		if el.Visibility != tt.want {
			t.Errorf("%s: Visibility = %v, want %v", tt.id, el.Visibility, tt.want)
		}
	}
}

// TestFromASTDefKeywords checks that "attribute", "item", and "port"
// definitions/usages each carry their sysml.DefKind over onto Element.DefKind
// correctly -- across the sysml.DefKind -> metamodel.DefKind conversion
// translate.go does by enum-order convention, not a lookup table, so a
// mismatch in either package's const order would silently mislabel these
// rather than fail to compile.
func TestFromASTDefKeywords(t *testing.T) {
	source := `package Vehicle {
		attribute def A;
		item def I;
		port def P;

		attribute a : A;
		item i : I;
		port p : P;
	}`

	ns, err := sysml.NewModel(source).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	tests := []struct {
		id   metamodel.ElementID
		want metamodel.DefKind
	}{
		{"Vehicle::A", metamodel.DefAttribute},
		{"Vehicle::I", metamodel.DefItem},
		{"Vehicle::P", metamodel.DefPort},
		{"Vehicle::a", metamodel.DefAttribute},
		{"Vehicle::i", metamodel.DefItem},
		{"Vehicle::p", metamodel.DefPort},
	}

	for _, tt := range tests {
		el, ok := model.Elements[tt.id]
		if !ok {
			t.Fatalf("missing element %q", tt.id)
		}
		if el.DefKind != tt.want {
			t.Errorf("%s: DefKind = %v, want %v", tt.id, el.DefKind, tt.want)
		}
	}
}

// TestFromASTNestedPackage checks that a package nested inside another
// package (rather than only inside a Definition) is declared as its own
// namespace, containment-linked to its enclosing package via Owner just
// like any other member.
func TestFromASTNestedPackage(t *testing.T) {
	source := `package Car {
		package Engine {
			part def Cylinder;
		}
	}`

	m := sysml.NewModel(source)

	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	want := map[metamodel.ElementID]metamodel.Element{
		model.Root:              {ID: model.Root, Kind: metamodel.KindNamespace},
		"Car":                   {ID: "Car", Kind: metamodel.KindPackage, Name: "Car", Owner: model.Root},
		"Car::Engine":           {ID: "Car::Engine", Kind: metamodel.KindPackage, Name: "Engine", Owner: "Car"},
		"Car::Engine::Cylinder": {ID: "Car::Engine::Cylinder", Kind: metamodel.KindDefinition, Name: "Cylinder", Owner: "Car::Engine"},
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

// relationshipTarget finds the ElementID a usage relates to via a
// Relationship of the given kind, e.g. relationshipTarget(model, id,
// metamodel.Subsets) for the feature it subsets.
func relationshipTarget(model *metamodel.Model, usage metamodel.ElementID, kind metamodel.RelationshipKind) (metamodel.ElementID, bool) {
	for _, rel := range model.RelationshipsFrom(usage) {
		if rel.Kind == kind {
			return rel.Target, true
		}
	}
	return "", false
}

// typeOf finds the ElementID a usage is typed by, via the TypedBy
// relationship model.FromAST produced for it.
func typeOf(model *metamodel.Model, usage metamodel.ElementID) (metamodel.ElementID, bool) {
	return relationshipTarget(model, usage, metamodel.TypedBy)
}

func TestFromASTEmptyPackage(t *testing.T) {
	m := sysml.NewModel(`package Empty { }`)

	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	empty, ok := model.Elements["Empty"]
	if !ok || empty.Owner != model.Root {
		t.Errorf("Empty = %+v, want a top-level element (Owner == model.Root)", empty)
	}
	if len(model.Elements) != 2 {
		t.Errorf("got %d elements, want 2 (the anonymous root and Empty)", len(model.Elements))
	}
}

func TestFromASTUnresolvedType(t *testing.T) {
	m := sysml.NewModel(`package Vehicle { part engine : Engine; }`)

	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if _, err := metamodel.FromAST(ns); err == nil {
		t.Error("expected an error for an unresolved type, got nil")
	}
}

func TestFromASTDuplicateDeclaration(t *testing.T) {
	m := sysml.NewModel(`package Vehicle {
		part def Engine;
		part def Engine;
	}`)

	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if _, err := metamodel.FromAST(ns); err == nil {
		t.Error("expected an error for a duplicate declaration, got nil")
	}
}

// TestFromASTDuplicateTopLevelDeclaration is TestFromASTDuplicateDeclaration's
// counterpart one level up: the anonymous root is itself a namespace guarded
// against redeclaration, exactly like a Package or Definition body, so two
// top-level packages with the same name must collide too.
func TestFromASTDuplicateTopLevelDeclaration(t *testing.T) {
	m := sysml.NewModel(`
		package Vehicle { }
		package Vehicle { }
	`)

	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if _, err := metamodel.FromAST(ns); err == nil {
		t.Error("expected an error for a duplicate top-level declaration, got nil")
	}
}

// TestFromASTMultipleTopLevelPackages checks the new capability the
// anonymous root namespace unlocks: two sibling top-level packages, neither
// nested in the other, where one resolves an absolute qualified reference
// into the other via resolveQualified's root-children lookup rather than a
// single named Root.
func TestFromASTMultipleTopLevelPackages(t *testing.T) {
	m := sysml.NewModel(`
		package P1 {
			part def A;
		}
		package P2 {
			part a : P1::A;
		}
	`)

	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	typeID, ok := typeOf(model, "P2::a")
	if !ok {
		t.Fatal("no TypedBy relationship found for P2::a")
	}
	if typeID != "P1::A" {
		t.Errorf("usage type = %q, want %q", typeID, "P1::A")
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

	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if _, err := metamodel.FromAST(ns); err == nil {
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

	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
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

	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
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
		"unknown top-level package": `package Vehicle { part engine : Nope::Engine; }`,
		"missing segment":           `package Vehicle { part def Car {} part engine : Vehicle::Car::Nope; }`,
	}

	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			m := sysml.NewModel(source)

			ns, err := m.Parse()
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			if _, err := metamodel.FromAST(ns); err == nil {
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
	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	model, err := metamodel.FromAST(ns)
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
	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromASTWithImports(ns, map[string]*metamodel.Model{"Vehicle": vehicle})
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
	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if _, err := metamodel.FromASTWithImports(ns, nil); err == nil {
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
	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Vehicle is compiled and available, but never imported by Car.
	if _, err := metamodel.FromASTWithImports(ns, map[string]*metamodel.Model{"Vehicle": vehicle}); err == nil {
		t.Error("expected an error resolving a type from a package that was never imported, got nil")
	}
}

// TestFromASTWildcardImport checks the basic case: "import P1::*;" inside a
// sibling of P1 makes P1's members resolvable unqualified, from directly
// inside the importing namespace -- as if they'd been declared there.
func TestFromASTWildcardImport(t *testing.T) {
	source := `package Root {
		package P1 {
			part def A;
		}
		package P2 {
			import P1::*;
			part a : A;
		}
	}`

	ns, err := sysml.NewModel(source).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	typeID, ok := typeOf(model, "Root::P2::a")
	if !ok {
		t.Fatal("no TypedBy relationship found for Root::P2::a")
	}
	if typeID != "Root::P1::A" {
		t.Errorf("usage type = %q, want %q", typeID, "Root::P1::A")
	}
}

// TestFromASTWildcardImportAtRoot is the same shape as
// TestFromASTWildcardImport, but with the import written at the anonymous
// root namespace itself -- the RootPackageTest.sysml pattern -- checking
// that a namespace of KindNamespace works as an importing scope exactly
// like a Package does.
func TestFromASTWildcardImportAtRoot(t *testing.T) {
	source := `
		package P1 {
			part def A;
		}
		import P1::*;
		package P2 {
			part a : A;
		}
	`

	ns, err := sysml.NewModel(source).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	typeID, ok := typeOf(model, "P2::a")
	if !ok {
		t.Fatal("no TypedBy relationship found for P2::a")
	}
	if typeID != "P1::A" {
		t.Errorf("usage type = %q, want %q", typeID, "P1::A")
	}
}

// TestFromASTWildcardImportReachesFromOutside checks the
// QualifiedNameImportTest.sysml scenario: P2a wildcard-imports P1, and code
// outside P2a (in its parent P2) reaches P1::A through P2a's own name --
// "P2a::A" -- because the import makes A resolve as if it were declared
// directly inside P2a, not just visible from within it.
func TestFromASTWildcardImportReachesFromOutside(t *testing.T) {
	source := `package Root {
		package P1 {
			part def A;
		}
		package P2 {
			package P2a {
				public import P1::*;
			}
			part x : P2a::A;
		}
	}`

	ns, err := sysml.NewModel(source).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	typeID, ok := typeOf(model, "Root::P2::x")
	if !ok {
		t.Fatal("no TypedBy relationship found for Root::P2::x")
	}
	if typeID != "Root::P1::A" {
		t.Errorf("usage type = %q, want %q", typeID, "Root::P1::A")
	}
}

// TestFromASTPrivateWildcardImportNotReexported checks the negative case of
// TestFromASTWildcardImportReachesFromOutside: with "private" instead of
// "public", P1's members stay visible from inside P2a itself, but aren't
// re-exported to an external qualifier like "P2a::A" -- private means not
// re-exported to importers of this namespace, per SysML.
func TestFromASTPrivateWildcardImportNotReexported(t *testing.T) {
	source := `package Root {
		package P1 {
			part def A;
		}
		package P2 {
			package P2a {
				private import P1::*;
			}
			part x : P2a::A;
		}
	}`

	ns, err := sysml.NewModel(source).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if _, err := metamodel.FromAST(ns); err == nil {
		t.Error("expected an error resolving through a private wildcard import from outside, got nil")
	}
}

// TestFromASTPrivateWildcardImportVisibleInside is the other half of
// TestFromASTPrivateWildcardImportNotReexported: a private import's target
// is still fully visible to code inside the importing namespace itself --
// privacy only restricts access from outside it.
func TestFromASTPrivateWildcardImportVisibleInside(t *testing.T) {
	source := `package Root {
		package P1 {
			part def A;
		}
		package P2a {
			private import P1::*;
			part x : A;
		}
	}`

	ns, err := sysml.NewModel(source).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	typeID, ok := typeOf(model, "Root::P2a::x")
	if !ok {
		t.Fatal("no TypedBy relationship found for Root::P2a::x")
	}
	if typeID != "Root::P1::A" {
		t.Errorf("usage type = %q, want %q", typeID, "Root::P1::A")
	}
}

// TestFromASTWildcardImportUnresolvedTarget checks that "import Nope::*;",
// naming a namespace that doesn't exist anywhere reachable from where the
// import was written, fails translation -- the same as an unresolved type
// reference does -- rather than silently importing nothing.
func TestFromASTWildcardImportUnresolvedTarget(t *testing.T) {
	ns, err := sysml.NewModel(`package Root { import Nope::*; }`).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if _, err := metamodel.FromAST(ns); err == nil {
		t.Error("expected an error for a wildcard import of an unresolved namespace, got nil")
	}
}

// environmentModel and combatantsModel compile the two standalone packages
// TestFromASTWithImports_Battlefield imports, mirroring vehicleModel above.
func environmentModel(t *testing.T) *metamodel.Model {
	t.Helper()

	m := sysml.NewModel(`package Environment {
		part def Terrain;
	}`)
	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	model, err := metamodel.FromAST(ns)
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
	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	model, err := metamodel.FromAST(ns)
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
	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromASTWithImports(ns, map[string]*metamodel.Model{
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
	want := &metamodel.Multiplicity{Lower: metamodel.Bound{Value: 0}, Upper: metamodel.Bound{Value: metamodel.Unbounded}}
	if squad.Multiplicity == nil || *squad.Multiplicity != *want {
		t.Errorf("squad Multiplicity = %+v, want %+v", squad.Multiplicity, want)
	}
}

// TestFromASTSubsets checks that "part b subsets a;" produces a Subsets
// Relationship from b to a, resolved the same way a type reference is.
func TestFromASTSubsets(t *testing.T) {
	ns, err := sysml.NewModel(`package Vehicle {
		part a;
		part b subsets a;
	}`).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	target, ok := relationshipTarget(model, "Vehicle::b", metamodel.Subsets)
	if !ok {
		t.Fatal("no Subsets relationship found for Vehicle::b")
	}
	if target != "Vehicle::a" {
		t.Errorf("b subsets %q, want %q", target, "Vehicle::a")
	}
}

// TestFromASTRedefines is TestFromASTSubsets's counterpart for
// "redefines", including a qualified target ("B::b").
func TestFromASTRedefines(t *testing.T) {
	ns, err := sysml.NewModel(`package Vehicle {
		part def B {
			part b;
		}
		part B_b redefines B::b;
	}`).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	target, ok := relationshipTarget(model, "Vehicle::B_b", metamodel.Redefines)
	if !ok {
		t.Fatal("no Redefines relationship found for Vehicle::B_b")
	}
	if target != "Vehicle::B::b" {
		t.Errorf("B_b redefines %q, want %q", target, "Vehicle::B::b")
	}
}

// TestFromASTTypeAndSubsetsTogether checks that a usage with both a type
// and a subsets gets one Relationship of each kind, since they're resolved
// as independent pendingReferences.
func TestFromASTTypeAndSubsetsTogether(t *testing.T) {
	ns, err := sysml.NewModel(`package Vehicle {
		part def Wheel;
		part wheels : Wheel;
		part b : Wheel subsets wheels;
	}`).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	typeTarget, ok := relationshipTarget(model, "Vehicle::b", metamodel.TypedBy)
	if !ok {
		t.Fatal("no TypedBy relationship found for Vehicle::b")
	}
	if typeTarget != "Vehicle::Wheel" {
		t.Errorf("b typed by %q, want %q", typeTarget, "Vehicle::Wheel")
	}

	subsetsTarget, ok := relationshipTarget(model, "Vehicle::b", metamodel.Subsets)
	if !ok {
		t.Fatal("no Subsets relationship found for Vehicle::b")
	}
	if subsetsTarget != "Vehicle::wheels" {
		t.Errorf("b subsets %q, want %q", subsetsTarget, "Vehicle::wheels")
	}
}

// TestFromASTUnresolvedSubsets checks that "subsets" is resolved the same
// strictly-required way a type is: an unresolvable target fails translation.
func TestFromASTUnresolvedSubsets(t *testing.T) {
	ns, err := sysml.NewModel(`package Vehicle { part b subsets nope; }`).Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if _, err := metamodel.FromAST(ns); err == nil {
		t.Error("expected an error for an unresolved subsets target, got nil")
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

	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
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
	ns, err := m.Parse()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	model, err := metamodel.FromAST(ns)
	if err != nil {
		t.Fatalf("unexpected translate error: %v", err)
	}

	if len(model.Elements) != 49 {
		t.Errorf("got %d elements, want 49 (48 declared, plus the anonymous root)", len(model.Elements))
	}
	if len(model.Relationships) != 29 {
		t.Errorf("got %d relationships, want 29", len(model.Relationships))
	}

	wantTypes := map[metamodel.ElementID]metamodel.ElementID{
		"Vehicle::Powertrain::Engine::cylinders": "Vehicle::Powertrain::Cylinder",
		"Vehicle::Car::engine":                   "Vehicle::Powertrain::Engine",
		"Vehicle::Car::frontLeftWheel":           "Vehicle::Chassis::Wheel",
		"Vehicle::Car::driverSeat":               "Vehicle::Cabin::Seat",
		"Vehicle::Car::ecu":                      "Vehicle::Electrical::ControlUnit",
		"Vehicle::Car::sensors":                  "Vehicle::Electrical::Sensor",
		"Vehicle::myCar":                         "Vehicle::Car",
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
		"Vehicle::Powertrain::Engine::cylinders": {Lower: metamodel.Bound{Value: 4}, Upper: metamodel.Bound{Value: 4}},
		"Vehicle::Electrical::sensors":           {Lower: metamodel.Bound{Value: 0}, Upper: metamodel.Bound{Value: metamodel.Unbounded}},
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
