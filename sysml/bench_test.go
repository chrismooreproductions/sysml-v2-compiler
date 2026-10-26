// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"os"
	"testing"
)

// BenchmarkEdit is what the language server does on an edit of a 1,500-line
// model: replace the file, resolve and validate.
func BenchmarkEdit(b *testing.B) {
	src, err := os.ReadFile("../testdata/upstream/sysml/examples/Vehicle Example/SysML v2 Spec Annex A SimpleVehicleModel.sysml")
	if err != nil {
		b.Skip(err)
	}
	w, _ := NewWorkspace()
	for b.Loop() {
		w.Replace("m.sysml", src)
		w.Resolve()
		w.Validate()
	}
}
