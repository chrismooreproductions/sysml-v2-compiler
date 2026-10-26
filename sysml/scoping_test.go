// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"strings"
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/semantics"
)

func resolved(t *testing.T, name, src string) (*Workspace, []Diagnostic) {
	t.Helper()
	w, _ := NewWorkspace()
	if _, errs := w.AddRecovering(name, []byte(src)); len(errs) > 0 {
		t.Fatal(errs[0])
	}
	return w, w.Resolve()
}

// The target of a featuring is looked up from the type that owns it, so a
// feature can be featured by one of its own members.
func TestAFeaturedByTargetIsLookedUpFromTheFeature(t *testing.T) {
	_, d := resolved(t, "f.kerml", `class C {
    member feature x featured by C_snapshots {
        member feature C_snapshots :>> Occurrences::Occurrence::snapshots featured by C;
    }
}`)
	if len(d) != 0 {
		t.Errorf("diagnostics: %v", d)
	}
}

// $::A is A among the root namespaces; '$'::A is a namespace named $.
func TestAGlobalQualifiedName(t *testing.T) {
	_, d := resolved(t, "g.kerml", `package P {
    package Objects { class Object { feature one; } }
    package '$' { class Objects { class Object { feature two; } } }
    package Q {
        class C :> Objects::Object { feature :>> one; }
        class D :> '$'::Objects::Object { feature :>> two; }
        class E :> $::Objects::Object { feature :>> subobjects; }
    }
}`)
	if len(d) != 0 {
		t.Errorf("diagnostics: %v", d)
	}
}

// A base type chosen by the metaclass of the annotated element.
func TestAConditionalBaseType(t *testing.T) {
	w, d := resolved(t, "m.kerml", `package P {
    class CC;
    struct SS { feature cc : CC; }
    metaclass M :> Metaobjects::SemanticMetadata {
        :>> annotatedElement : KerML::Class;
        :>> baseType = if annotatedElement istype KerML::Structure ? SS meta KerML::Type else CC meta KerML::Class;
    }
    #M struct T { feature :>> cc; }
    #M class U;
}`)
	if len(d) != 0 {
		t.Errorf("diagnostics: %v", d)
	}
	implied := func(qn string) string {
		var out []string
		for _, im := range w.ImpliedGeneralizations(w.Lookup(qn)) {
			out = append(out, im.Kind+" "+semantics.QualifiedName(im.Target))
		}
		return strings.Join(out, ", ")
	}
	if got := implied("P::T"); !strings.Contains(got, "P::SS") || strings.Contains(got, "P::CC") {
		t.Errorf("T, a structure: %s", got)
	}
	if got := implied("P::U"); !strings.Contains(got, "P::CC") || strings.Contains(got, "P::SS") {
		t.Errorf("U, a class: %s", got)
	}
}

// The effect action of a transition and the entry action of a state stand for
// the library features they redefine, so their features are found through the
// name effect.
func TestTransitionAndStateActionsRedefineTheirLibraryFeatures(t *testing.T) {
	w, d := resolved(t, "t.sysml", `package P {
    part def S { attribute n : ScalarValues::Integer; }
    action def Act { out x : ScalarValues::Integer; }
    state def Machine {
        entry action start : Act;
        state a;
        state b;
        transition go first a accept Act do action work : Act then b;
    }
    part m : S { bind m.n = n; }
}`)
	if len(d) != 0 {
		t.Errorf("diagnostics: %v", d)
	}
	implied := func(qn string) string {
		var out []string
		for _, im := range w.ImpliedGeneralizations(w.Lookup(qn)) {
			out = append(out, im.Kind+" "+semantics.QualifiedName(im.Target))
		}
		return strings.Join(out, ", ")
	}
	if got := implied("P::Machine::start"); !strings.Contains(got, "States::StateAction::entryAction") {
		t.Errorf("entry action: %s", got)
	}
	if got := implied("P::Machine::go::work"); !strings.Contains(got, "Actions::TransitionAction::effect") {
		t.Errorf("effect action: %s", got)
	}
}

// A variant is typed by the definition, or subsets the usage, it is a variant
// of: an enumerated value is typed by its enumeration.
func TestAVariantTakesItsVariation(t *testing.T) {
	w, d := resolved(t, "v.sysml", `package P {
    enum def E { enum a; enum b; }
    part def V { variation part vp { variant part x; variant part y; } }
    part def W { variation part def VD { variant part z; } }
}`)
	if len(d) != 0 {
		t.Errorf("diagnostics: %v", d)
	}
	implied := func(qn string) string {
		var out []string
		for _, im := range w.ImpliedGeneralizations(w.Lookup(qn)) {
			out = append(out, im.Kind+" "+semantics.QualifiedName(im.Target))
		}
		return strings.Join(out, ", ")
	}
	if got := implied("P::E::a"); !strings.Contains(got, "FeatureTyping P::E") {
		t.Errorf("an enumerated value: %s", got)
	}
	if got := implied("P::V::vp::x"); !strings.Contains(got, "Subsetting P::V::vp") {
		t.Errorf("a variant of a usage: %s", got)
	}
}
