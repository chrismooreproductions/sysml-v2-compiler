// SPDX-License-Identifier: EPL-2.0

package semantics

import (
	"strings"
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
)

// resolved builds a workspace from SysML sources (and the standard library if
// asked), resolves it, and returns the roots.
func resolved(t *testing.T, withLibrary bool, sources ...string) (*Workspace, []*model.Element) {
	t.Helper()
	w := NewWorkspace()
	if withLibrary {
		if err := w.AddLibrary(); err != nil {
			t.Fatal(err)
		}
	}
	var roots []*model.Element
	for i, src := range sources {
		root, err := w.AddSource(grammar.SysML, "t"+string(rune('0'+i)), src)
		if err != nil {
			t.Fatal(err)
		}
		roots = append(roots, root)
	}
	w.Resolve()
	return w, roots
}

// targetOf is what the first reference to a property of the first element of a
// class points at.
func targetOf(t *testing.T, root *model.Element, class, prop string) *model.Element {
	t.Helper()
	var got *model.Element
	root.Walk(func(e *model.Element) {
		if got == nil && e.Class == class {
			if refs := e.Refs(prop); len(refs) > 0 {
				got = refs[0].Target
				if got == nil {
					t.Errorf("%s.%s=%q is unresolved", class, prop, refs[0].Name)
				}
			}
		}
	})
	if got == nil {
		t.Fatalf("no %s with a reference %s", class, prop)
	}
	return got
}

func nameOf(e *model.Element) string { return e.Text("declaredName") }

func TestResolvesLocalAndOuterNames(t *testing.T) {
	_, roots := resolved(t, false, `package P { part def A; package Q { part x : A; } }`)
	if got := targetOf(t, roots[0], "FeatureTyping", "type"); got.Class != "PartDefinition" || nameOf(got) != "A" {
		t.Errorf("type = %s", model.Dump(got))
	}
}

func TestResolvesQualifiedNames(t *testing.T) {
	_, roots := resolved(t, false, `package P { package Q { package R { part def A; } } part x : Q::R::A; }`)
	if got := targetOf(t, roots[0], "FeatureTyping", "type"); nameOf(got) != "A" {
		t.Errorf("type = %s", model.Dump(got))
	}
}

func TestAConjugatedPortTypingResolvesToTheConjugatedPortDefinition(t *testing.T) {
	_, roots := resolved(t, false, `package P { package Q { port def Fuel; } part def A { port p : ~Q::Fuel; } }`)
	got := targetOf(t, roots[0], "ConjugatedPortTyping", "conjugatedPortDefinition")
	if got.Class != "ConjugatedPortDefinition" || !got.Owner.Owner.IsA("PortDefinition") || nameOf(got.Owner.Owner) != "Fuel" {
		t.Errorf("target = %s", model.Dump(got))
	}
	// a port definition that does not resolve leaves the typing unresolved
	w, _ := resolved(t, false, `package P { part def A { port p : ~Elsewhere::Fuel; } }`)
	if len(w.Unresolved()) != 1 {
		t.Errorf("~ of an unknown name: %d unresolved, want 1", len(w.Unresolved()))
	}
	// only a port definition has one
	w, _ = resolved(t, false, `package P { part def A; part def B { port p : ~A; } }`)
	if len(w.Unresolved()) != 1 {
		t.Errorf("~A on a part definition: %d unresolved, want 1", len(w.Unresolved()))
	}
}

func TestQualifiedNameOfAFeatureThatTakesItsNameFromWhatItRedefines(t *testing.T) {
	_, roots := resolved(t, false, `package P { part def Base { part eng; } part def Car :> Base { part :>> eng; } }`)
	var got []string
	roots[0].Walk(func(e *model.Element) {
		if e.IsA("PartUsage") {
			got = append(got, QualifiedName(e))
		}
	})
	if want := []string{"P::Base::eng", "P::Car::eng"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAPerformedChainIsNamedForItsLastFeature(t *testing.T) {
	// perform a.b has no name of its own and is called what a.b is, so that
	// the performer's feature can be reached as driver.turnOn
	w, _ := resolved(t, false, `package P {
		part def Driver;
		part s {
			perform action start { action turnOn; }
			part driver : Driver { perform start.turnOn; }
			first driver.turnOn then start.turnOn;
		}
	}`)
	if u := w.Unresolved(); len(u) != 0 {
		t.Errorf("%d unresolved, want 0: %v", len(u), u)
	}
}

func TestOnlyVisibleMembersAreReachableByQualification(t *testing.T) {
	// a private member is not visible from outside its package
	_, roots := resolved(t, false, `package P { package Q { private part def Hidden; part def Shown; } part x : Q::Hidden; part y : Q::Shown; }`)
	var hidden, shown []*model.Ref
	roots[0].Walk(func(e *model.Element) {
		if e.Class == "FeatureTyping" {
			for _, r := range e.Refs("type") {
				switch r.Name {
				case "Q::Hidden":
					hidden = append(hidden, r)
				case "Q::Shown":
					shown = append(shown, r)
				}
			}
		}
	})
	if len(hidden) != 1 || hidden[0].Target != nil {
		t.Errorf("Q::Hidden resolved to %v", hidden[0].Target)
	}
	if len(shown) != 1 || shown[0].Target == nil {
		t.Error("Q::Shown did not resolve")
	}
}

func TestImportsMakeMembersVisible(t *testing.T) {
	_, roots := resolved(t, false, `package Q { part def A; } package P { private import Q::*; part x : A; }`)
	if got := targetOf(t, roots[0], "FeatureTyping", "type"); nameOf(got) != "A" {
		t.Errorf("type = %s", model.Dump(got))
	}
	_, roots = resolved(t, false, `package Q { part def A; part def B; } package P { private import Q::A; part x : A; part y : B; }`)
	var unresolved []string
	roots[0].Walk(func(e *model.Element) {
		if e.Class == "FeatureTyping" {
			for _, r := range e.Refs("type") {
				if r.Target == nil {
					unresolved = append(unresolved, r.Name)
				}
			}
		}
	})
	if len(unresolved) != 1 || unresolved[0] != "B" {
		t.Errorf("unresolved = %v, want only B", unresolved)
	}
}

func TestInheritedMembersAreVisibleToRedefinitions(t *testing.T) {
	_, roots := resolved(t, false, `part def A { part p; } part def B :> A { part q :>> p; }`)
	got := targetOf(t, roots[0], "Redefinition", "redefinedFeature")
	if got.Class != "PartUsage" || nameOf(got) != "p" {
		t.Fatalf("redefinedFeature = %s", model.Dump(got))
	}
	if got.Owner.Owner.Text("declaredName") != "A" {
		t.Error("p resolved to something other than A's p")
	}
}

func TestAFeatureWithoutANameTakesTheNameItRedefines(t *testing.T) {
	// "part :>> p" is unnamed but is effectively named p, so a later reference to p finds it
	_, roots := resolved(t, false, `part def A { part p; } part def B :> A { part :>> p; ref r = p; }`)
	var ref *model.Element
	roots[0].Walk(func(e *model.Element) {
		if e.Class == "FeatureReferenceExpression" {
			ref = e
		}
	})
	var member *model.Element
	for _, m := range ref.Children("ownedRelationship") {
		if m.Class == "Membership" {
			member = m
		}
	}
	got := member.Refs("memberElement")[0].Target
	if got == nil || got.Owner.Owner.Text("declaredName") != "B" {
		t.Errorf("p resolved to %v, want B's redefinition", got)
	}
}

func TestResolvesAgainstTheStandardLibrary(t *testing.T) {
	_, roots := resolved(t, true, `package P { attribute a : ScalarValues::Real; attribute b : Real; part c; }`)
	r := targetOf(t, roots[0], "FeatureTyping", "type")
	if nameOf(r) != "Real" || r.Class != "DataType" {
		t.Errorf("type = %s", model.Dump(r))
	}
}

func TestUnresolvedNamesStayUnresolved(t *testing.T) {
	_, roots := resolved(t, false, `package P { part x : Missing; }`)
	var refs []*model.Ref
	roots[0].Walk(func(e *model.Element) {
		if e.Class == "FeatureTyping" {
			refs = append(refs, e.Refs("type")...)
		}
	})
	if len(refs) != 1 || refs[0].Target != nil || refs[0].Name != "Missing" {
		t.Errorf("refs = %+v", refs)
	}
}

func TestEverySourceFileResolvesAgainstEachOther(t *testing.T) {
	_, roots := resolved(t, false, `package A { part def Engine; }`, `package B { private import A::*; part e : Engine; part f : A::Engine; }`)
	if got := targetOf(t, roots[1], "FeatureTyping", "type"); nameOf(got) != "Engine" {
		t.Errorf("type = %s", model.Dump(got))
	}
}

func TestImplicitSupertypesAreGeneralTypes(t *testing.T) {
	w, roots := resolved(t, true, `package P { part def A; part x : A; }`)
	var def *model.Element
	roots[0].Walk(func(e *model.Element) {
		if e.Class == "PartDefinition" {
			def = e
		}
	})
	found := false
	for _, g := range w.generalTypes(def) {
		if nameOf(g) == "Part" {
			found = true // Parts::Part
		}
	}
	if !found {
		t.Error("a part definition does not specialize Parts::Part")
	}
}

func TestUnresolvedNamesAreReportedWithWhereTheyWereWritten(t *testing.T) {
	w := NewWorkspace()
	if _, err := w.AddSource(grammar.SysML, "model.sysml", "package P {\n  part x : Missing;\n  package Q { part y : 'Also Missing'::T; }\n}"); err != nil {
		t.Fatal(err)
	}
	w.Resolve()
	got := w.Unresolved()
	if len(got) != 2 {
		t.Fatalf("%d unresolved: %+v", len(got), got)
	}
	u := got[0]
	if u.Ref.Name != "Missing" || u.File != "model.sysml" || u.Role != "type" || u.Ref.Span.Start.Line != 2 || u.Ref.Span.Start.Column != 12 {
		t.Errorf("first = %q in %s at %v role %s", u.Ref.Name, u.File, u.Ref.Span.Start, u.Role)
	}
	if QualifiedName(u.Scope) != "P" {
		t.Errorf("searched from %q, want P", QualifiedName(u.Scope))
	}
	if got[1].Ref.Name != "'Also Missing'::T" || QualifiedName(got[1].Scope) != "P::Q" {
		t.Errorf("second = %q from %q", got[1].Ref.Name, QualifiedName(got[1].Scope))
	}
}

func TestTheLibraryIsNotReportedAsUnresolved(t *testing.T) {
	w := NewWorkspace()
	if err := w.AddLibrary(); err != nil {
		t.Fatal(err)
	}
	w.Resolve()
	if got := w.Unresolved(); len(got) != 0 {
		t.Errorf("%d unresolved in the library were reported", len(got))
	}
	if got := w.FileOf(w.roots[0]); !strings.HasPrefix(got, "library:") {
		t.Errorf("FileOf = %q", got)
	}
}

func TestQualifiedNameQuotesWhenNeeded(t *testing.T) {
	_, roots := resolved(t, false, `package 'My Pkg' { part def 'A-1' { part x; } }`)
	var x *model.Element
	roots[0].Walk(func(e *model.Element) {
		if e.Class == "PartUsage" {
			x = e
		}
	})
	if got := QualifiedName(x); got != `'My Pkg'::'A-1'::x` {
		t.Errorf("QualifiedName = %s", got)
	}
}

func TestResolvesEveryStepOfALongChain(t *testing.T) {
	w, roots := resolved(t, true, `package P {
		attribute def SamplePair { attribute domainValue : ScalarValues::Real; }
		attribute def SF { attribute samples : SamplePair[0..*]; }
		calc def D { in fn : SF; return : ScalarValues::Real[0..*] = fn.samples.domainValue; }
	}`)
	if un := w.Unresolved(); len(un) != 0 {
		t.Fatalf("unresolved: %v", un[0].Ref.Name)
	}
	var got []string
	roots[0].Walk(func(e *model.Element) {
		if e.Class == "FeatureChaining" {
			got = append(got, QualifiedName(e.Refs("chainingFeature")[0].Target))
		}
	})
	if strings.Join(got, ",") != "P::SF::samples,P::SamplePair::domainValue" {
		t.Errorf("chained %v", got)
	}
}

// The chain a connector's end references is looked up from outside the
// connector, so self there is the owner's, not one the connector inherits.
func TestConnectorEndChainsStartOutsideTheConnector(t *testing.T) {
	w, roots := resolved(t, true, `package P {
		action def A {
			action step;
			succession first step then self.endShot;
		}
	}`)
	if un := w.Unresolved(); len(un) != 0 {
		t.Fatalf("unresolved: %v", un[0].Ref.Name)
	}
	var chained []string
	roots[0].Walk(func(e *model.Element) {
		if e.Class == "FeatureChaining" {
			chained = append(chained, QualifiedName(e.Refs("chainingFeature")[0].Target))
		}
	})
	if strings.Join(chained, ",") != "Actions::Action::self,Occurrences::Occurrence::endShot" {
		t.Errorf("chained %v", chained)
	}
}

// A constructor's named arguments redefine features of the type it constructs.
func TestResolvesTheNamedArgumentsOfAConstructor(t *testing.T) {
	w, _ := resolved(t, true, `package P {
		attribute def SF { attribute samples : ScalarValues::Integer[*]; }
		calc def S { in attribute dv [0..*]; return sampling = new SF ( samples = dv ); }
	}`)
	if un := w.Unresolved(); len(un) != 0 {
		t.Errorf("unresolved: %q", un[0].Ref.Name)
	}
}

// Workspaces share the standard library's model, so the operation that edits
// library elements (ResolveAll applies the resolution rules to them) works on a
// private copy and leaves what the others see alone.
func TestResolveAllWorksOnAPrivateLibrary(t *testing.T) {
	a, b := NewWorkspace(), NewWorkspace()
	if err := a.AddLibrary(); err != nil {
		t.Fatal(err)
	}
	if err := b.AddLibrary(); err != nil {
		t.Fatal(err)
	}
	libraryRoots := func(w *Workspace) []*model.Element {
		var out []*model.Element
		for _, r := range w.roots {
			if w.isLibrary(r) {
				out = append(out, r)
			}
		}
		return out
	}
	shared, before := libraryRoots(a), libraryRoots(b)
	if len(shared) == 0 || len(shared) != len(before) || shared[0] != before[0] {
		t.Fatal("two workspaces do not share the library's model")
	}
	count := func(e *model.Element) int {
		n := 0
		e.Walk(func(*model.Element) { n++ })
		return n
	}
	size := count(before[0])

	a.ResolveAll()
	own := libraryRoots(a)
	if len(own) != len(shared) || own[0] == shared[0] {
		t.Error("ResolveAll did not switch to a private copy of the library")
	}
	if got := count(libraryRoots(b)[0]); got != size || libraryRoots(b)[0] != shared[0] {
		t.Errorf("the shared library changed: %d elements, was %d", got, size)
	}
	// and the other workspace still resolves against it
	root, err := b.AddSource(grammar.SysML, "m.sysml", "package P { part def A; part a : A; attribute x : ScalarValues::Integer; }")
	if err != nil {
		t.Fatal(err)
	}
	b.Resolve()
	if len(b.Unresolved()) != 0 {
		t.Errorf("unresolved: %v", b.Unresolved())
	}
	_ = root
}
