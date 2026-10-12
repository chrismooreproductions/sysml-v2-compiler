// SPDX-License-Identifier: EPL-2.0

package sysml

import (
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
