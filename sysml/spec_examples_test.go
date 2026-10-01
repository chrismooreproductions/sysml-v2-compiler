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
		// AssertConstraintUsage's non-"satisfy" form ("assert constraint
		// massAnalysis : MassAnalysis { ... }") isn't supported -- parseSatisfy
		// only ever accepts "assert"/"not"/"satisfy", not "assert" followed
		// directly by "constraint".
		{"ConstraintTest.sysml", `line 24: unexpected 'constraint', want 'satisfy'`},
		// sequence expressions ("(vehicle.eng.m, vehicle.trans.m)") aren't
		// supported -- deliberately out of scope for this project's
		// expression subsystem (see sysml.Expression's doc comment);
		// parsePrimary's OpenParen case only ever parses one inner
		// expression for grouping, not a comma-separated sequence.
		{"CalculationTest.sysml", `line 23: unexpected ',', want ')'`},
		// "bind"/BindingConnectorAsUsage isn't supported -- a structurally
		// unrelated shorthand form, not a ConnectorPart variant (see
		// Connection's own doc comment).
		{"ConnectionTest.sysml", `line 22: unexpected identifier "bind", want 'package', 'import', or 'part'`},
		// "abstract" as a prefix modifier isn't supported.
		{"InterfaceTest.sysml", `line 24: unexpected identifier "abstract", want 'package', 'import', or 'part'`},
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
		// short names ("<'UR1.1'>") aren't supported -- the same long-tail
		// gap PartTest.sysml's own blocker is, at line 4.
		{"HSUVRequirements.sysml", `line 4: unexpected '<', want ';'`},
		// an inline "satisfy requirement req1 : Req1 by system;"
		// declaration (SatisfyRequirementUsage's other alternative, a brand
		// new nested requirement usage) isn't supported -- only a bare
		// reference to an existing requirement usage is (see sysml.Satisfy's
		// own doc comment).
		{"RequirementDerivationExample.sysml", `line 27: unexpected 'requirement', want identifier`},
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
		// Exercises Phase 7's general usage bodies (increment A),
		// references/"::>" (increment B), and "end" body members
		// (increment C) all at once -- the first Requirements Examples
		// fixture (not from "Simple Tests") to fully parse.
		"VehicleRequirementDerivation.sysml",
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
