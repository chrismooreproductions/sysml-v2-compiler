package sysml_test

import (
	"os"
	"strings"
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/sysml"
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
		// "nonunique"/"ordered") isn't a keyword this project recognizes
		// as a prefix, so it parses as a keyword-less member's own bare
		// name instead of being rejected outright -- real KerML lets any
		// Usage omit its keyword, and this project's parser takes "the
		// name of an anonymous-keyword attribute" as the fallback,
		// advancing one token further before rejecting "attribute"
		// (another keyword) where a type/value/body/';' was expected.
		{"PartTest.sysml", `line 8: unexpected 'attribute', want ';'`},
		// "as" casts ("(vehicles as VehiclePart).m") aren't supported --
		// a new Expression variant, plus extending postfix "."-chaining to
		// apply after a parenthesized/cast result (today it only ever
		// follows a bare name) -- deliberately out of scope for this
		// project's expression subsystem (see sysml.Expression's doc
		// comment).
		{"CalculationTest.sysml", `line 28: unexpected identifier "as", want ')'`},
		// "flow def" (FlowDefinition) isn't supported -- a structurally
		// separate feature from connections (Clause 8.2.2.16).
		{"ConnectionTest.sysml", `line 57: unexpected 'def', want ';'`},
		// "enum def" (EnumerationDefinition) isn't a recognized keyword,
		// so it parses as a keyword-less member's own bare name (see
		// PartTest.sysml's own comment above for why) before rejecting
		// "def" where a terminator was expected -- even though its body
		// shape itself (e.g. "uncl : ClassificationLevel = 0;") is
		// exactly what keyword-less members already support.
		{"MetadataTest.sysml", `line 6: unexpected 'def', want ';'`},
		// "doc" annotations aren't supported -- this project's parser
		// drops comments entirely before seeing them (newParser filters
		// every Comment token out up front), so there's nowhere to hang
		// the text that follows "doc". Like "constant"/"enum" above,
		// "doc" parses as a keyword-less member's own bare name first,
		// advancing one token further (past the dropped comment) before
		// rejecting "requirement" (another keyword) where a terminator
		// was expected.
		{"RequirementTest.sysml", `line 9: unexpected 'requirement', want ';'`},
		// Both of these hit the exact same "doc" gap RequirementTest.sysml
		// above also hits (a "doc /* ... */" comment, dropped entirely
		// before the parser ever sees it, swept up as a keyword-less
		// member's own bare name before rejecting the next keyword) at
		// the same early line, before ever reaching the "analysis"/
		// "verification"/"objective"/"verify" constructs each is actually
		// vendored to exercise -- covered directly instead by dedicated
		// hand-crafted parser/metamodel tests (see
		// TestModelParseCaseAnalysisVerificationDefKeywords,
		// TestModelParseObjectiveDefKeyword, TestModelParseVerify).
		{"AnalysisTest.sysml", `line 11: unexpected '}', want ';'`},
		{"VerificationTest.sysml", `line 11: unexpected '}', want ';'`},
		// This fixture parses cleanly through subject/actor/objective/
		// include/nested-use-case-usage/perform before tripping on
		// "include system.uc1;" -- include's own bare-reference form
		// only accepts a plain or qualified name (parseQualifiedName),
		// not a dotted feature chain, so it rejects the "." where a
		// terminator was expected. "include u;" just above it, a plain
		// bare name with no dots, parses fine.
		{"UseCaseTest.sysml", `line 40: unexpected '.', want ';'`},
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
		// Exercises untyped usages, multiplicity before or after a type,
		// name-valued bounds ("[n]"), and an assigned integer value
		// ("= 5") together.
		"MultiplicityTest.sysml",
		// Exercises the wildcard import syntax ("P1::*").
		"QualifiedNameImportTest.sysml",
		// Exercises the subsetting keyword ("subsets").
		"RootPackageTest.sysml",
		// Exercises general usage bodies, references/"::>", and "end"
		// body members together -- a Requirements Examples fixture (not
		// one of the "Simple Tests"), denser and more realistic than
		// anything else in this table.
		"VehicleRequirementDerivation.sysml",
		// Exercises the "abstract" prefix modifier.
		"InterfaceTest.sysml",
		// Exercises Satisfy's inline "requirement req1 : Req1" declaration
		// form.
		"RequirementDerivationExample.sysml",
		// Exercises restricted names and short names together
		// ("<'UR1.1'> Load").
		"HSUVRequirements.sysml",
		// Exercises sequence expressions, AssertConstraintUsage, and
		// keyword-less/bare members together -- a fixture whose own
		// subject matter is specifically constraint/parameter input.
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
