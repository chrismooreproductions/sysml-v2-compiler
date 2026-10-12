// SPDX-License-Identifier: EPL-2.0

package semantics

import (
	"bytes"
	"os"
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/semantics/gen"
)

// The checked-in table must be exactly what the generator makes from the
// vendored Java. When this fails, run `go generate ./internal/semantics`.
func TestImplicitTableIsCurrent(t *testing.T) {
	java, err := os.ReadFile("upstream/ImplicitGeneralizationMap.java.txt")
	if err != nil {
		t.Fatal(err)
	}
	want, err := gen.Generate(java)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("implicit_table_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("implicit_table_gen.go is out of date: run `go generate ./internal/semantics`")
	}
}

func TestImplicitTableContents(t *testing.T) {
	if got := len(implicitTable); got != 232 {
		t.Errorf("%d entries, want 232", got)
	}
	for key, want := range map[string]string{
		"Classifier_base":               "Base::Anything",
		"Feature_base":                  "Base::things",
		"Behavior_base":                 "Performances::Performance",
		"Multiplicity_feature":          "Base::exactlyOne",
		"FeatureChainExpression_target": "ControlFunctions::'.'::source::target",
	} {
		if implicitTable[key] != want {
			t.Errorf("%s = %q, want %q", key, implicitTable[key], want)
		}
	}
}
