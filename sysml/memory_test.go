// SPDX-License-Identifier: EPL-2.0

//go:build !race

package sysml

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// liveHeap is the memory in use after collecting garbage, in megabytes. (It is
// skipped under the race detector, which changes what memory costs.)
func liveHeap() float64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / (1 << 20)
}

// aModel is a model with n of each of the constructs whose resolution makes
// helper elements that no tree owns: chain expressions, bindings to a value and
// transitions that accept a payload. They once outlived the file.
func aModel(name string, n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "package %s {\n  private import ScalarValues::*;\n  attribute def Sig;\n", name)
	b.WriteString("  part def A { part b : A; attribute x : Integer; }\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "  part p%d : A { attribute y%d = b.x; attribute w%d = x; }\n", i, i, i)
		fmt.Fprintf(&b, "  state def S%d { state a; state b; transition t first a accept s : Sig then b; }\n", i)
	}
	b.WriteString("}\n")
	return b.String()
}

// Replacing a file, resolving and validating must not keep the file's old
// version alive. An editor does it on every change, and each version of a
// model once stayed in memory.
func TestReplacingAFileDoesNotLeak(t *testing.T) {
	const size = 60
	w, _ := NewWorkspace()
	w.Add("m.sysml", []byte(aModel("M0", size)))
	w.Resolve()
	w.Validate()
	replace := func(i int) {
		if _, errs := w.Replace("m.sysml", []byte(aModel(fmt.Sprintf("M%d", i), size))); len(errs) != 0 {
			t.Fatal(errs[0])
		}
		w.Resolve()
		w.Validate()
	}
	replace(1)
	before := liveHeap()
	for i := 2; i <= 5; i++ {
		replace(i)
	}
	grown := liveHeap() - before
	runtime.KeepAlive(w) // or the whole workspace is garbage by the time of the last measurement
	t.Logf("the live heap grew by %.1f MB over 4 replacements", grown)
	// it grew by about 1 MB with each replacement before it was fixed
	if grown > 2 {
		t.Errorf("the live heap grew by %.1f MB over 4 replacements of a file", grown)
	}
}

// The model of the standard library is built once for the process: another
// workspace shares it and costs next to nothing (it was about 25 MB each, and
// several times that in garbage while it was built).
func TestAnotherWorkspaceCostsNextToNothing(t *testing.T) {
	first, _ := NewWorkspace()
	before := liveHeap()
	var more []*Workspace
	for i := 0; i < 4; i++ {
		w, _ := NewWorkspace()
		more = append(more, w)
	}
	grown := liveHeap() - before
	runtime.KeepAlive(first)
	runtime.KeepAlive(more)
	t.Logf("four more workspaces cost %.1f MB", grown)
	if grown > 4 {
		t.Errorf("four more workspaces cost %.1f MB; each should share the library's model", grown)
	}
}

// The model of the library is all a process keeps of it: the parse trees it was
// built from are dropped.
func TestTheLibraryKeepsNoParseTrees(t *testing.T) {
	w, _ := NewWorkspace()
	live := liveHeap()
	runtime.KeepAlive(w)
	t.Logf("a process with the library holds %.1f MB", live)
	if live > 32 {
		t.Errorf("a process with the library holds %.1f MB", live)
	}
}
