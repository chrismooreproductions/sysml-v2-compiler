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
		// "constant" as a prefix modifier (alongside "derived"/"readonly"/
		// "nonunique"/"ordered") isn't supported -- short names are fixed
		// now, so this fixture advances to its next gap.
		{"PartTest.sysml", `line 8: unexpected identifier "constant", want 'package', 'import', or 'part'`},
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
		// "first"/SuccessionAsUsage isn't supported -- a structurally
		// unrelated shorthand form, not a ConnectorPart variant (see
		// Connection's own doc comment), the same way bind/binding was
		// before it.
		{"ConnectionTest.sysml", `line 26: unexpected identifier "first", want 'package', 'import', or 'part'`},
		// "enum def" (EnumerationDefinition) isn't supported -- its body
		// uses completely keyword-less members ("uncl : ClassificationLevel
		// = 0;", no "attribute" or anything else), which parseMember can't
		// dispatch at all today (every member must start with a recognized
		// keyword). Unrelated to metadata itself, but the next thing this
		// fixture's own library package trips on, now that "library" and
		// restricted names both resolve correctly.
		{"MetadataTest.sysml", `line 6: unexpected identifier "enum", want 'package', 'import', or 'part'`},
		// "doc" annotations aren't supported -- this project's parser
		// drops comments entirely before seeing them (newParser filters
		// every Comment token out up front), so there's nowhere to hang
		// the text that follows "doc". Unrelated to requirements
		// themselves, but the next thing this fixture's own "doc /* */"
		// trips on, now that recursive imports resolve correctly.
		{"RequirementTest.sysml", `line 8: unexpected identifier "doc", want 'package', 'import', or 'part'`},
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
		// Exercises the "abstract" prefix modifier, the only thing it was
		// missing after Phase 7.
		"InterfaceTest.sysml",
		// Exercises Satisfy's inline "requirement req1 : Req1" declaration
		// form, the only thing it was missing.
		"RequirementDerivationExample.sysml",
		// Exercises restricted names and short names together
		// ("<'UR1.1'> Load"), the only things it was missing.
		"HSUVRequirements.sysml",
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
