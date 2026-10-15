// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"os"
	"strings"
	"testing"
)

func TestLanguageOf(t *testing.T) {
	for name, want := range map[string]Language{"a.sysml": SysML, "dir/b.SYSML": SysML, "c.kerml": KerML, "d.KerML": KerML} {
		if got, err := LanguageOf(name); err != nil || got != want {
			t.Errorf("LanguageOf(%q) = %v, %v; want %v", name, got, err, want)
		}
	}
	for _, name := range []string{"a.txt", "sysml", "x.sysmlx"} {
		if _, err := LanguageOf(name); err == nil || !strings.Contains(err.Error(), "unknown file type") {
			t.Errorf("LanguageOf(%q) error = %v", name, err)
		}
	}
	if SysML.String() != "SysML" || KerML.String() != "KerML" {
		t.Error("Language.String is wrong")
	}
}

func TestParseChoosesTheLanguageByExtension(t *testing.T) {
	if doc, err := Parse("a.kerml", []byte("classifier A { feature f; }")); err != nil || doc.Language != KerML || doc.Name != "a.kerml" {
		t.Fatalf("KerML: %+v, %v", doc, err)
	}
	// the same text is a syntax error in the other language
	if _, err := Parse("a.sysml", []byte("classifier A { feature f; }")); err == nil {
		t.Error("KerML text parsed as SysML")
	}
	doc, err := ParseAs(KerML, "noextension", []byte("classifier A;"))
	if err != nil || doc.Root.Class != "Namespace" {
		t.Errorf("ParseAs: %+v, %v", doc, err)
	}
	if _, err := Parse("a.txt", nil); err == nil {
		t.Error("an unknown extension was accepted")
	}
}

func TestParseReturnsTheStructureTheSpecificationImplies(t *testing.T) {
	doc, err := Parse("a.sysml", []byte("package P { attribute a = 1 + 2; }"))
	if err != nil {
		t.Fatal(err)
	}
	var op *Element
	doc.Root.Walk(func(e *Element) {
		if e.Class == "OperatorExpression" {
			op = e
		}
	})
	// operands are held as parameters and the expression has a result parameter
	params, results := 0, 0
	for _, m := range op.Children("ownedRelationship") {
		switch {
		case m.IsA("ReturnParameterMembership"):
			results++
		case m.IsA("ParameterMembership"):
			params++
		}
	}
	if params != 2 || results != 1 {
		t.Errorf("%d parameters and %d results", params, results)
	}
	// references are not resolved by Parse
	doc, _ = Parse("b.sysml", []byte("part x : A;"))
	doc.Root.Walk(func(e *Element) {
		for _, r := range e.Refs("type") {
			if r.Target != nil || r.Name != "A" {
				t.Errorf("ref = %+v", r)
			}
		}
	})
}

func TestSyntaxErrorIsTyped(t *testing.T) {
	_, err := Parse("bad.sysml", []byte("package P {\n  part 'open\n}"))
	se, ok := err.(*SyntaxError)
	if !ok || se.Pos.Line != 2 || se.File != "bad.sysml" {
		t.Fatalf("got %T %v", err, err)
	}
	if !strings.HasPrefix(err.Error(), "bad.sysml:2:") {
		t.Errorf("message = %s", err)
	}
}

func TestWorkspaceWithAndWithoutTheStandardLibrary(t *testing.T) {
	src := []byte("package P { attribute a : ScalarValues::Real; }")
	with, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	with.Add("m.sysml", src)
	if got := with.Resolve(); len(got) != 0 {
		t.Errorf("with the library: %v", got)
	}
	without, _ := NewWorkspace(WithoutStandardLibrary())
	without.Add("m.sysml", src)
	got := without.Resolve()
	if len(got) != 1 || !strings.Contains(got[0].Message, `"ScalarValues::Real"`) {
		t.Errorf("without the library: %v", got)
	}
}

func TestWorkspaceDocumentsAndCrossFileResolution(t *testing.T) {
	w, _ := NewWorkspace(WithoutStandardLibrary())
	a, err := w.Add("a.sysml", []byte("package A { part def Engine; }"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := w.Add("b.sysml", []byte("package B { private import A::*; part e : Engine; }"))
	if err != nil {
		t.Fatal(err)
	}
	if docs := w.Documents(); len(docs) != 2 || docs[0] != a || docs[1] != b {
		t.Errorf("Documents = %v", docs)
	}
	if got := w.Resolve(); len(got) != 0 {
		t.Errorf("diagnostics: %v", got)
	}
	var def *Element
	a.Root.Walk(func(e *Element) {
		if e.Class == "PartDefinition" {
			def = e
		}
	})
	if got := w.FileOf(def); got != "a.sysml" {
		t.Errorf("FileOf = %q", got)
	}
	b.Root.Walk(func(e *Element) {
		for _, r := range e.Refs("type") {
			if r.Target != def {
				t.Errorf("Engine resolved to %v", r.Target)
			}
		}
	})
}

func TestAFileWithASyntaxErrorIsNotAdded(t *testing.T) {
	w, _ := NewWorkspace(WithoutStandardLibrary())
	if _, err := w.Add("a.sysml", []byte("package {")); err == nil {
		t.Fatal("no error")
	}
	if len(w.Documents()) != 0 {
		t.Error("the broken file was added")
	}
	if _, err := w.Add("a.txt", nil); err == nil {
		t.Error("an unknown extension was accepted")
	}
}

func TestDiagnosticsAreInSourceOrderAndResolveIsRepeatable(t *testing.T) {
	w, _ := NewWorkspace(WithoutStandardLibrary())
	w.Add("m.sysml", []byte("package P {\n  part a : One;\n  part b : Two;\n}"))
	first := w.Resolve()
	second := w.Resolve()
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("%d then %d diagnostics", len(first), len(second))
	}
	if first[0].Span.Start.Line != 2 || first[1].Span.Start.Line != 3 {
		t.Errorf("order: %v", first)
	}
	if got := first[0].String(); got != `m.sysml:2:12: cannot resolve "One" (searched from P)` {
		t.Errorf("String = %q", got)
	}
}

func TestElementsKnowTheirFileAndQualifiedName(t *testing.T) {
	w, _ := NewWorkspace()
	doc, _ := w.Add("m.sysml", []byte("package P { part def A; part a : A; }"))
	w.Resolve()
	var target *Element
	doc.Root.Walk(func(e *Element) {
		for _, r := range e.Refs("type") {
			target = r.Target
		}
	})
	if w.FileOf(target) != "m.sysml" {
		t.Errorf("FileOf(user element) = %q", w.FileOf(target))
	}
	if got := QualifiedName(target); got != "P::A" {
		t.Errorf("QualifiedName = %q", got)
	}
}

func TestValidateReportsConstraintViolations(t *testing.T) {
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Add("v.sysml", []byte("package P { part def V { variant part x; } }")); err != nil {
		t.Fatal(err)
	}
	got := w.Validate()
	if len(got) != 1 || got[0].Constraint != "validateVariantMembershipOwningNamespace" || got[0].File != "v.sysml" {
		t.Errorf("Validate() = %+v", got)
	}
}

func TestAddRecoveringKeepsTheStatementsThatParse(t *testing.T) {
	w, err := NewWorkspace(WithoutStandardLibrary())
	if err != nil {
		t.Fatal(err)
	}
	doc, errs := w.AddRecovering("m.sysml", []byte("package P {\n  part def A;\n  part x : ;\n  part def B;\n}\n"))
	if len(errs) != 1 || errs[0].Pos.Line != 3 {
		t.Fatalf("errors %v", errs)
	}
	if doc == nil || len(w.Documents()) != 1 {
		t.Fatal("the statements that parse were not added")
	}
	var names []string
	doc.Root.Walk(func(e *Element) {
		if n := e.Text("declaredName"); n != "" {
			names = append(names, n)
		}
	})
	if got := strings.Join(names, ","); got != "P,A,B" {
		t.Errorf("names %s, want P,A,B", got)
	}
}

func TestQueries(t *testing.T) {
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Add("fleet.sysml", []byte(`
package Fleet {
	part def Vehicle { attribute mass : ScalarValues::Real; }
	part def Truck :> Vehicle { attribute payload : ScalarValues::Real; }
	part rig : Truck;
}`)); err != nil {
		t.Fatal(err)
	}
	if d := w.Resolve(); len(d) != 0 {
		t.Fatal(d)
	}
	truck := w.Lookup("Fleet::Truck")
	vehicle := w.Lookup("Fleet::Vehicle")
	if truck == nil || vehicle == nil || w.Lookup("Fleet::Nope") != nil || w.Lookup("Nope::Truck") != nil {
		t.Fatalf("Lookup: truck %v, vehicle %v", truck, vehicle)
	}
	if got := w.Lookup("ScalarValues::Real"); got == nil || got.Class != "DataType" {
		t.Errorf("Lookup of a library element: %v", got)
	}
	if !w.Specializes(truck, vehicle) || w.Specializes(vehicle, truck) {
		t.Error("Specializes")
	}
	var supers []string
	for _, s := range w.Supertypes(truck) {
		supers = append(supers, QualifiedName(s))
	}
	if strings.Join(supers, ",") != "Fleet::Vehicle" {
		t.Errorf("Supertypes = %v", supers)
	}
	var feats []string
	for _, f := range w.Features(truck) {
		feats = append(feats, w.EffectiveName(f))
	}
	if !strings.Contains(strings.Join(feats, ","), "payload") || !strings.Contains(strings.Join(feats, ","), "mass") {
		t.Errorf("Features = %v", feats)
	}
	var owned []string
	for _, m := range w.OwnedMembers(w.Lookup("Fleet")) {
		owned = append(owned, w.EffectiveName(m))
	}
	if strings.Join(owned, ",") != "Vehicle,Truck,rig" {
		t.Errorf("OwnedMembers = %v", owned)
	}
	if m := w.Member(truck, "mass"); m == nil || m.Class != "AttributeUsage" {
		t.Errorf("Member(Truck, mass) = %v", m)
	}
	rig := w.Member(w.Lookup("Fleet"), "rig")
	if ts := w.TypesOf(rig); len(ts) != 1 || ts[0] != truck {
		t.Errorf("TypesOf(rig) = %v", ts)
	}
}

func TestMaterializeAddsTheImpliedRelationshipsToTheTree(t *testing.T) {
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	doc, err := w.Add("m.sysml", []byte("package P { part def A; part a : A; }"))
	if err != nil {
		t.Fatal(err)
	}
	count := func() (n int) {
		doc.Root.Walk(func(e *Element) {
			if e.IsTrue("isImplied") {
				n++
			}
		})
		return
	}
	if count() != 0 {
		t.Fatal("implied relationships in the tree before Materialize")
	}
	w.Materialize()
	n := count()
	if n != 2 {
		t.Errorf("%d implied relationships, want 2 (A's supertype, a's subsetting)", n)
	}
	w.Materialize()
	if count() != n {
		t.Error("Materialize is not idempotent")
	}
}

func TestEvaluate(t *testing.T) {
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	doc, err := w.Add("m.sysml", []byte("package P { attribute a = 2; attribute b = (a, a + 1); attribute c = a / 0; }"))
	if err != nil {
		t.Fatal(err)
	}
	w.Resolve()
	value := func(name string) (any, error) {
		var out any
		var oerr error
		doc.Root.Walk(func(e *Element) {
			if e.Text("declaredName") == name {
				for _, r := range e.Children("ownedRelationship") {
					for _, x := range r.Children("ownedRelatedElement") {
						out, oerr = w.Evaluate(x)
					}
				}
			}
		})
		return out, oerr
	}
	if v, err := value("a"); err != nil || v != int64(2) {
		t.Errorf("a = %v, %v", v, err)
	}
	if v, err := value("b"); err != nil || FormatValue(v) != "(2, 3)" {
		t.Errorf("b = %v, %v", v, err)
	}
	if _, err := value("c"); err == nil {
		t.Error("c = a / 0 evaluated")
	} else if _, ok := err.(*EvalError); !ok {
		t.Errorf("error %T, want *EvalError", err)
	}
}

func TestReplaceUpdatesWhatIsResolved(t *testing.T) {
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if _, errs := w.Replace("lib.sysml", []byte("package L { part def A; }")); len(errs) != 0 {
		t.Fatal(errs)
	}
	if _, errs := w.Replace("use.sysml", []byte("package U { part x : L::A; }")); len(errs) != 0 {
		t.Fatal(errs)
	}
	if d := w.Resolve(); len(d) != 0 {
		t.Fatalf("diagnostics: %v", d)
	}
	// rename the definition: the use now fails to resolve
	w.Replace("lib.sysml", []byte("package L { part def B; }"))
	d := w.Resolve()
	if len(d) != 1 || !strings.Contains(d[0].Message, `"L::A"`) {
		t.Fatalf("after the rename: %v", d)
	}
	// and fixing the use makes it resolve again
	w.Replace("use.sysml", []byte("package U { part x : L::B; }"))
	if d := w.Resolve(); len(d) != 0 {
		t.Fatalf("after the fix: %v", d)
	}
	if len(w.Documents()) != 2 {
		t.Errorf("%d documents", len(w.Documents()))
	}
	w.Remove("use.sysml")
	if len(w.Documents()) != 1 || w.Lookup("U") != nil {
		t.Error("Remove left the document behind")
	}
}

func TestAWrongLanguageIsSaid(t *testing.T) {
	kerml := []byte("function f { in x : ScalarValues::Integer; return : ScalarValues::Integer = x; }")
	_, err := Parse("model.sysml", kerml)
	if err == nil || !strings.Contains(err.Error(), "the text is valid KerML: name the file .kerml") {
		t.Errorf("a KerML file named .sysml: %v", err)
	}
	if _, err := Parse("model.kerml", kerml); err != nil {
		t.Errorf("as KerML: %v", err)
	}
	_, err = Parse("model.kerml", []byte("package P { part def A; }"))
	if err == nil || !strings.Contains(err.Error(), "the text is valid SysML") {
		t.Errorf("a SysML file named .kerml: %v", err)
	}
	// text that is valid in neither gets no hint
	_, err = Parse("model.sysml", []byte("package P { part x : ; }"))
	if err == nil || strings.Contains(err.Error(), "valid") {
		t.Errorf("a plain syntax error: %v", err)
	}
	w, _ := NewWorkspace(WithoutStandardLibrary())
	_, errs := w.AddRecovering("model.sysml", kerml)
	if len(errs) == 0 || !strings.Contains(errs[0].Msg, "the text is valid KerML") {
		t.Errorf("AddRecovering: %v", errs)
	}
}

// checked adds a file to a fresh workspace and returns what Resolve and
// Validate report.
func checked(t *testing.T, name string, src []byte) (*Workspace, *Document, []Diagnostic) {
	t.Helper()
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	doc, err := w.Add(name, src)
	if err != nil {
		t.Fatal(err)
	}
	diags := w.Resolve()
	return w, doc, append(diags, w.Validate()...)
}

func TestExposeAndRenderAreValid(t *testing.T) {
	for name, src := range map[string]string{
		"expose.sysml": `package P {
    part def Vehicle;
    view v {
        expose Vehicle::*;
    }
}`,
		"render.sysml": `package P {
    public import Views::*;
    view def TreeView {
        render asTreeDiagram;
    }
}`,
	} {
		if _, _, diags := checked(t, name, []byte(src)); len(diags) != 0 {
			t.Errorf("%s: %v", name, diags)
		}
	}
}

func TestTheSpecificationsViewExampleIsValid(t *testing.T) {
	src, err := os.ReadFile("../testdata/upstream/sysml/examples/Simple Tests/ViewTest.sysml")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, diags := checked(t, "ViewTest.sysml", src); len(diags) != 0 {
		t.Errorf("ViewTest.sysml: %v", diags)
	}
}

// An expose imports everything it names, private members included ("expose
// imports all" in the example), because the pilot defaults isImportAll to true.
func TestAnExposeImportsAll(t *testing.T) {
	_, doc, diags := checked(t, "e.sysml", []byte(`package P {
    part def Vehicle;
    private part hidden;
    view v {
        expose P::*;
        alias h for hidden;
    }
}`))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	n := 0
	doc.Root.Walk(func(e *Element) {
		if e.IsA("Expose") {
			n++
			if !e.IsTrue("isImportAll") {
				t.Errorf("%s does not import all", e.Class)
			}
		}
	})
	if n != 1 {
		t.Errorf("%d exposes, want 1", n)
	}
}

// Resolve can be called again and again; a default the resolution rules add
// (here the multiplicity of a rendering usage) must be added once.
func TestResolveAddsADefaultMultiplicityOnce(t *testing.T) {
	w, doc, _ := checked(t, "r.sysml", []byte(`package P {
    public import Views::*;
    view def TreeView { render asTreeDiagram; }
}`))
	w.Resolve()
	w.Resolve()
	count := 0
	doc.Root.Walk(func(e *Element) {
		if e.Class != "RenderingUsage" {
			return
		}
		for _, m := range e.Children("ownedRelationship") {
			if m.Child("ownedRelatedElement").IsA("Multiplicity") {
				count++
			}
		}
	})
	if count != 1 {
		t.Errorf("%d multiplicities on the rendering usage, want 1", count)
	}
}

// F::result is F's own unnamed return parameter, which takes the name of the
// result it implicitly redefines, not the result it inherits; so its members
// are found.
func TestAnUnnamedReturnParameterIsNamedForTheResultItImplicitlyRedefines(t *testing.T) {
	w, err := NewWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Add("q.kerml", []byte(`package Q {
	datatype V { feature dimension : ScalarValues::Natural; }
	function F { in x : ScalarValues::Real; return : V[1] { :>> dimension = 3; } }
	function G specializes F {
		return : V[1] { feature :>> F::result::dimension; }
	}
}`)); err != nil {
		t.Fatal(err)
	}
	if diags := w.Resolve(); len(diags) != 0 {
		t.Fatalf("unresolved: %v", diags)
	}
	f := w.Lookup("Q::F")
	res := w.Member(f, "result")
	if res == nil || res.Owner == nil || res.Owner.Owner != f {
		t.Errorf("F::result is %v, want the return parameter F owns", res)
	}
}
