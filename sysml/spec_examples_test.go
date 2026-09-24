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
		// public/private/protected visibility modifiers aren't supported.
		{"PartTest.sysml", `line 5: unexpected identifier "public", want 'package', 'import', or 'part'`},
		// "attribute" as a member kind isn't supported.
		{"MultiplicityTest.sysml", `line 4: unexpected identifier "attribute", want 'package', 'import', or 'part'`},
		// visibility on imports (and so "public import") isn't supported.
		{"QualifiedNameImportTest.sysml", `line 7: unexpected identifier "public", want 'package', 'import', or 'part'`},
		// the root of a model is parsed as a single package, not the
		// spec's RootNamespace (multiple top-level packages/imports).
		{"RootPackageTest.sysml", `line 5: unexpected 'package' after package`},
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
