package sysml_test

import (
	"os"
	"strings"
	"testing"

	"github.com/chrismooreproductions/sysml-modeller/sysml"
)

// TestModelParseSpecExamples runs the parser against real example models
// from the OMG reference implementation (see testdata/spec_examples/README.md)
// instead of hand-picked snippets. This project implements a deliberate
// subset of SysML v2, so every case here currently fails; wantErr pins the
// exact construct each one first trips on, documenting the current edge of
// that subset. When a case starts parsing successfully -- because the
// construct it was blocked on got implemented -- update its wantErr to the
// next blocker (or move it to a dedicated success test if it fully parses)
// rather than loosening or deleting the assertion.
func TestModelParseSpecExamples(t *testing.T) {
	cases := []struct {
		file    string
		wantErr string
	}{
		// short names ("part <'1'> b: B;") aren't supported. Since Phase 3
		// made a usage's name optional, '<' no longer needs to be an
		// identifier there -- it's just treated as "no name", so parsing
		// now gets one token further before rejecting '<' as neither a
		// specialization part nor a value nor ';'.
		{"PartTest.sysml", `line 6: unexpected '<', want ';'`},
		// invocation expressions ("sum(componentMasses)") aren't
		// supported -- deliberately out of scope for this project's
		// expression subsystem (see sysml.Expression's doc comment).
		{"ConstraintTest.sysml", `line 10: unexpected '(', want '}'`},
		// definition-level specialization ("part def Vehicle :>
		// VehiclePart;", KerML's SubclassificationPart) isn't supported --
		// this project's ":>"/"subsets" only cover usage-level feature
		// subsetting, not classifier specialization on a definition.
		{"CalculationTest.sysml", `line 9: unexpected 'subsets', want ';' or '{'`},
		// a plain (non-constraint/calc) usage body isn't supported -- only
		// constraint/calc usages can carry a body (see hasCalculationBody).
		// General usage bodies for every kind are a bigger, separate piece
		// of work than this phase covers.
		{"ConnectionTest.sysml", `line 3: unexpected '{', want ';'`},
		// "end" as a body member introducing a connector end inside a
		// definition (InterfaceBodyItem's DefaultInterfaceEnd) isn't
		// supported -- this phase only covers the usage-level "connect a
		// to b" / "connect (a, b, ...)" syntax, not this def-body member
		// form.
		{"InterfaceTest.sysml", `line 7: unexpected identifier "end", want 'package', 'import', or 'part'`},
		// single-quoted/restricted names ("'User Defined Extensions'")
		// aren't supported -- the lexer has no handling for '\'' at all,
		// so it gets swept into the identifier it's touching ("'User"),
		// and the space inside the quoted name splits what should be one
		// name into two tokens. This trips well before anything
		// metadata-related: it's the qualified name in this fixture's
		// own private import, on line 2.
		{"MetadataTest.sysml", `line 2: unexpected identifier "Defined", want ';'`},
		// recursive imports ("q::**") aren't supported -- unrelated to
		// requirements themselves, but the first thing this fixture's own
		// "private import q::**;" trips on, at line 4.
		{"RequirementTest.sysml", `line 4: unexpected '**', want identifier`},
		// a plain (non-constraint/calc/metadata/requirement/concern/frame)
		// usage body isn't supported (see hasPlainBody) -- "part vehicle {
		// ... }" at line 4 is an ordinary part usage with a nested body,
		// unrelated to this fixture's requirement-derivation syntax, which
		// (per hand-crafted tests elsewhere) parses fine on its own.
		{"VehicleRequirementDerivation.sysml", `line 4: unexpected '{', want ';'`},
		// short names ("<'UR1.1'>") aren't supported -- the same long-tail
		// gap PartTest.sysml's own blocker is, at line 4.
		{"HSUVRequirements.sysml", `line 4: unexpected '<', want ';'`},
		// "end" as a body member introducing a connector end inside a
		// definition isn't supported (see InterfaceTest.sysml's own
		// blocker) -- the "#derivation connection def Req1_Derivation {
		// end #original r1 : Req1; ... }" at line 10, well past this
		// fixture's own requirement/satisfy syntax parsing successfully.
		{"RequirementDerivationExample.sysml", `line 10: unexpected identifier "end", want 'package', 'import', or 'part'`},
	}

	for _, tt := range cases {
		t.Run(tt.file, func(t *testing.T) {
			source, err := os.ReadFile("testdata/spec_examples/" + tt.file)
			if err != nil {
				t.Fatalf("reading fixture: %v", err)
			}

			m := sysml.NewModel(string(source))
			_, err = m.Parse()
			if err == nil {
				t.Fatalf("Parse() succeeded on %s; update this test's expectations", tt.file)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Parse() error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestModelParseSpecExamplesSuccess holds spec example fixtures (see
// testdata/spec_examples/README.md) that fully parse today -- moved here
// out of TestModelParseSpecExamples once their last blocker was resolved,
// per that test's own doc comment.
func TestModelParseSpecExamplesSuccess(t *testing.T) {
	files := []string{
		// Exercises every increment-6 addition at once: untyped usages,
		// multiplicity before or after a type, name-valued bounds ("[n]"),
		// and an assigned integer value ("= 5").
		"MultiplicityTest.sysml",
		// Exercises increment-7's wildcard import syntax ("P1::*").
		"QualifiedNameImportTest.sysml",
		// Exercises increment-8's subsetting keyword ("subsets"), the last
		// of this plan's three target fixtures to go green.
		"RootPackageTest.sysml",
	}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			source, err := os.ReadFile("testdata/spec_examples/" + file)
			if err != nil {
				t.Fatalf("reading fixture: %v", err)
			}

			if _, err := sysml.NewModel(string(source)).Parse(); err != nil {
				t.Errorf("Parse() failed on %s: %v", file, err)
			}
		})
	}
}
