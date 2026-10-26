// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"context"
	"errors"
	"testing"
)

func TestResolveAndValidateGiveUpWhenCancelled(t *testing.T) {
	w, _ := NewWorkspace()
	w.Replace("m.sysml", []byte(aModelSource))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if d, err := w.ResolveContext(ctx); !errors.Is(err, context.Canceled) || d != nil {
		t.Errorf("ResolveContext on a cancelled context = %v, %v", d, err)
	}
	if d, err := w.ValidateContext(ctx); !errors.Is(err, context.Canceled) || d != nil {
		t.Errorf("ValidateContext on a cancelled context = %v, %v", d, err)
	}
}

func TestResolveContextMatchesResolve(t *testing.T) {
	w, _ := NewWorkspace()
	w.Replace("m.sysml", []byte(aModelSource))
	want := w.Resolve()
	w.Replace("m.sysml", []byte(aModelSource))
	got, err := w.ResolveContext(context.Background())
	if err != nil || len(got) != len(want) || len(want) == 0 {
		t.Fatalf("ResolveContext = %v, %v; Resolve = %v", got, err, want)
	}
	if v, err := w.ValidateContext(context.Background()); err != nil || len(v) != len(w.Validate()) {
		t.Errorf("ValidateContext = %v, %v", v, err)
	}
}

func TestReplaceAsTakesTheLanguageFromTheCaller(t *testing.T) {
	w, _ := NewWorkspace()
	d, errs := w.ReplaceAs(SysML, "file:///a b/doc", []byte("package P { part def A; }"))
	if len(errs) != 0 || d == nil || d.Language != SysML {
		t.Fatalf("ReplaceAs = %v, %v", d, errs)
	}
	if d2, _ := w.ReplaceAs(SysML, "file:///a b/doc", []byte("package Q;")); len(w.Documents()) != 1 || d2 == nil {
		t.Errorf("a replaced document should leave one: %d", len(w.Documents()))
	}
}

const aModelSource = "package P { part def A; part a : A; part b : Missing; }"
