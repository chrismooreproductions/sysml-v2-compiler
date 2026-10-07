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
		// "constant" (a prefix modifier, alongside "derived"/"readonly"/
		// "nonunique"/"ordered") now parses as a keyword-less member's own
		// bare name instead of being rejected outright -- real KerML lets
		// any Usage omit its keyword, and "constant" isn't one this
		// project recognizes as a prefix, so it's swept up as "the name of
		// an anonymous-keyword attribute" instead, advancing one token
		// further before rejecting "attribute" (another keyword) where a
		// type/value/body/';' was expected.
		{"PartTest.sysml", `line 8: unexpected 'attribute', want ';'`},
		// "as" casts ("(vehicles as VehiclePart).m") aren't supported --
		// a new Expression variant, plus extending postfix "."-chaining to
		// apply after a parenthesized/cast result (today it only ever
		// follows a bare name) -- deliberately out of scope for this
		// project's expression subsystem (see sysml.Expression's doc
		// comment). Sequence expressions are fixed now, so this fixture
		// advances to its next gap.
		{"CalculationTest.sysml", `line 28: unexpected identifier "as", want ')'`},
		// "first"/SuccessionAsUsage isn't supported -- a structurally
		// unrelated shorthand form, not a ConnectorPart variant (see
		// Connection's own doc comment). Like "constant" above, "first"
		// now parses as a keyword-less member's own bare name instead of
		// being rejected outright, advancing one token further before
		// rejecting "a" (an identifier, not a valid specialization/value/
		// body/';') where a terminator was expected.
		{"ConnectionTest.sysml", `line 26: unexpected identifier "a", want ';'`},
		// "enum def" (EnumerationDefinition) isn't supported. Its own body
		// (e.g. "uncl : ClassificationLevel = 0;") is exactly the
		// keyword-less-member shape this phase just added -- but "enum
		// def" the construct itself still isn't a recognized keyword, so
		// (like "constant"/"first" above) it's swept up as a keyword-less
		// member's own bare name first, advancing one token further
		// before rejecting "def" where a terminator was expected.
		{"MetadataTest.sysml", `line 6: unexpected 'def', want ';'`},
		// "doc" annotations aren't supported -- this project's parser
		// drops comments entirely before seeing them (newParser filters
		// every Comment token out up front), so there's nowhere to hang
		// the text that follows "doc". Like "constant"/"first"/"enum"
		// above, "doc" now parses as a keyword-less member's own bare
		// name first, advancing one token further (past the dropped
		// comment) before rejecting "requirement" (another keyword) where
		// a terminator was expected.
		{"RequirementTest.sysml", `line 9: unexpected 'requirement', want ';'`},
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
		// Exercises Phase 8's sequence expressions (increment A),
		// AssertConstraintUsage (increment B), and keyword-less/bare
		// members (increment C) all at once -- the first fixture whose
		// own subject matter (constraint/parameter input) this whole
		// phase was aimed at.
		"ConstraintTest.sysml",
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
