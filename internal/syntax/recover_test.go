// SPDX-License-Identifier: EPL-2.0

package syntax

import (
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
)

func TestParseRecoverReportsEveryError(t *testing.T) {
	src := "package P {\n  part x : ;\n  part def A { attribute a : ; part ok; }\n  part y @@ ;\n  part z;\n}\n"
	root, errs := ParseRecover(grammar.SysML, "m.sysml", src)
	var got []string
	for _, e := range errs {
		got = append(got, e.Pos.String())
	}
	want := []string{"2:12", "3:30", "4:10"}
	if len(got) != len(want) {
		t.Fatalf("errors at %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("errors at %v, want %v", got, want)
		}
	}
	if root == nil {
		t.Fatal("no tree for the statements that parse")
	}
}

func TestParseRecoverOfValidSource(t *testing.T) {
	root, errs := ParseRecover(grammar.SysML, "m.sysml", "package P { part x; }")
	if root == nil || len(errs) != 0 {
		t.Errorf("root %v, errors %v", root, errs)
	}
}

func TestParseRecoverNeverLoopsOnGarbage(t *testing.T) {
	for _, src := range []string{"}", "{{{", "package", "package P {", "part def", ")(", "'unterminated", ""} {
		_, errs := ParseRecover(grammar.SysML, "g.sysml", src)
		if len(errs) > maxErrors {
			t.Errorf("%q: %d errors", src, len(errs))
		}
	}
}
