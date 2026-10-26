// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"strings"
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/semantics"
)

// A type annotated by semantic metadata is specialized by the metadata's base
// type, so the features of the base type are its features.
func TestSemanticMetadataGivesABaseType(t *testing.T) {
	const src = `package SM {
    private import Metaobjects::SemanticMetadata;
    port def SD { attribute serviceDefinition : ScalarValues::String; }
    port gsd : SD;
    metadata def <service> SDMetadata :> SemanticMetadata {
        :>> baseType = SD meta SysML::PortDefinition;
    }
    metadata def <instance> InstanceMetadata :> SemanticMetadata {
        :>> baseType default gsd meta SysML::PortUsage;
    }
    #service port def MyService {
        attribute :>> serviceDefinition = "x";
    }
    #instance port p {
        attribute :>> serviceDefinition = "y";
    }
    #instance port def Other;
}
`
	w, _ := NewWorkspace()
	if _, errs := w.AddRecovering("sm.sysml", []byte(src)); len(errs) > 0 {
		t.Fatal(errs[0])
	}
	if d := w.Resolve(); len(d) != 0 {
		t.Errorf("diagnostics: %v", d)
	}
	implied := func(qn string) string {
		e := w.Lookup(qn)
		if e == nil {
			t.Fatalf("no %s", qn)
		}
		var out []string
		for _, im := range w.ImpliedGeneralizations(e) {
			out = append(out, im.Kind+" "+semantics.QualifiedName(im.Target))
		}
		return strings.Join(out, ", ")
	}
	// a definition takes a definition, and the types of a usage
	if got := implied("SM::MyService"); !strings.Contains(got, "Subclassification SM::SD") {
		t.Errorf("MyService: %s", got)
	}
	if got := implied("SM::Other"); !strings.Contains(got, "Subclassification SM::SD") {
		t.Errorf("Other, whose base type is a usage typed by SD: %s", got)
	}
	// a usage takes a usage
	if got := implied("SM::p"); !strings.Contains(got, "Subsetting SM::gsd") {
		t.Errorf("p: %s", got)
	}
}
