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
		// short names ("part <'1'> b: B;") aren't supported -- the whole
		// "<'1'>" gets swallowed as one identifier by the current lexer,
		// so parseUsage's expected name/colon shape trips up on "b" next.
		{"PartTest.sysml", `line 6: unexpected identifier "b", want ':'`},
		// "attribute" as a member kind isn't supported.
		{"MultiplicityTest.sysml", `line 4: unexpected identifier "attribute", want 'package', 'import', or 'part'`},
		// wildcard imports ("P1::*") aren't supported.
		{"QualifiedNameImportTest.sysml", `line 7: unexpected '*', want identifier`},
		// wildcard imports ("P2::*") aren't supported.
		{"RootPackageTest.sysml", `line 6: unexpected '*', want identifier`},
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
