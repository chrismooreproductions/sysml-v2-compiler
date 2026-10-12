// SPDX-License-Identifier: EPL-2.0

package library

import (
	"strings"
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
)

func TestFilesParseAndAreCached(t *testing.T) {
	files, err := Files()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 94 {
		t.Fatalf("%d files, want 94", len(files))
	}
	var sysml, kerml int
	for _, f := range files {
		if f.Tree == nil || f.Tree.Class != "Namespace" {
			t.Errorf("%s: no tree", f.Path)
		}
		if f.Lang == grammar.KerML {
			kerml++
			if !strings.HasSuffix(f.Path, ".kerml") {
				t.Errorf("%s parsed as KerML", f.Path)
			}
		} else {
			sysml++
		}
	}
	if sysml != 58 || kerml != 36 {
		t.Errorf("%d SysML and %d KerML files, want 58 and 36", sysml, kerml)
	}
	again, _ := Files()
	if &again[0] != &files[0] {
		t.Error("Files parsed the library again")
	}
}

func TestSources(t *testing.T) {
	srcs, err := Sources()
	if err != nil {
		t.Fatal(err)
	}
	base, ok := srcs["Kernel Libraries/Kernel Semantic Library/Base.kerml"]
	if !ok || !strings.Contains(base, "abstract classifier Anything") {
		t.Errorf("Base.kerml missing or wrong (%d bytes)", len(base))
	}
}
