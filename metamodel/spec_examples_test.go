package metamodel_test

import (
	"os"
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/metamodel"
	"github.com/chrismooreproductions/sysml-v2-compiler/sysml"
)

// TestFromASTSpecExamples runs FromAST (not just Parse) against real OMG
// example models -- see sysml/testdata/spec_examples/README.md for
// provenance, the same files sysml's own TestModelParseSpecExamples(Success)
// tracks at the syntax level. This is the first point this project's
// fixture suite validates semantic translation against real examples
// rather than just parsing, now that the standard library stub
// (metamodel/stdlib.go) lets them resolve ScalarValues/ScalarFunctions
// references without this project needing a real one yet.
//
// Unlike TestModelParseSpecExamples, there's no "pin the current blocker"
// convention here: every file listed is expected to fully translate right
// now. Add a file once it reaches that point (PartTest.sysml doesn't yet
// -- it still fails to parse at all).
func TestFromASTSpecExamples(t *testing.T) {
	files := []string{
		"MultiplicityTest.sysml",
		"QualifiedNameImportTest.sysml",
		"RootPackageTest.sysml",
	}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			source, err := os.ReadFile("../sysml/testdata/spec_examples/" + file)
			if err != nil {
				t.Fatalf("reading fixture: %v", err)
			}

			ns, err := sysml.NewModel(string(source)).Parse()
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			if _, err := metamodel.FromAST(ns); err != nil {
				t.Errorf("FromAST failed on %s: %v", file, err)
			}
		})
	}
}
