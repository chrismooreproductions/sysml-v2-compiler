// SPDX-License-Identifier: EPL-2.0

package semantics

import (
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/postprocess"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/syntax"
)

func build(t *testing.T, src string) *model.Element {
	t.Helper()
	n, err := syntax.Parse(grammar.SysML, "t", src)
	if err != nil {
		t.Fatal(err)
	}
	root := model.FromSyntax(n)
	postprocess.Run(root)
	Structure(root)
	return root
}

func find(root *model.Element, class string) []*model.Element {
	var out []*model.Element
	root.Walk(func(e *model.Element) {
		if e.Class == class {
			out = append(out, e)
		}
	})
	return out
}

func TestOperandsAreHeldAsInParameters(t *testing.T) {
	root := build(t, "attribute a = 1 + 2;")
	op := find(root, "OperatorExpression")[0]
	if op.Has("operand") {
		t.Fatalf("operands left in place: %s", model.Dump(op))
	}
	var params []*model.Element
	for _, m := range op.Children("ownedRelationship") {
		if m.IsA("ParameterMembership") && !m.IsA("ReturnParameterMembership") {
			params = append(params, m)
		}
	}
	if len(params) != 2 {
		t.Fatalf("%d parameters: %s", len(params), model.Dump(op))
	}
	for i, m := range params {
		f := m.Child("ownedRelatedElement")
		value := f.Child("ownedRelationship")
		if m.Text("visibility") != "private" || f.Text("direction") != "in" || value.Class != "FeatureValue" ||
			!m.Generated || !f.Generated || !value.Generated {
			t.Errorf("parameter %d is %s", i, model.Dump(m))
		}
		if lit := value.Child("ownedRelatedElement"); lit.Class != "LiteralInteger" || lit.Generated || lit.Owner != value {
			t.Errorf("operand %d is %s (generated=%v)", i, model.Dump(lit), lit.Generated)
		}
	}
	if params[0].Child("ownedRelatedElement").Child("ownedRelationship").Child("ownedRelatedElement").Text("value") != "1" {
		t.Error("operands are out of order")
	}
}

func TestExpressionsGetOneResultParameter(t *testing.T) {
	root := build(t, "attribute a = x + y;")
	for _, class := range []string{"OperatorExpression", "FeatureReferenceExpression"} {
		for _, e := range find(root, class) {
			n := 0
			for _, m := range e.Children("ownedRelationship") {
				if m.IsA("ReturnParameterMembership") {
					n++
					if f := m.Child("ownedRelatedElement"); f.Text("direction") != "out" || !f.Generated {
						t.Errorf("%s result = %s", class, model.Dump(f))
					}
				}
			}
			if n != 1 {
				t.Errorf("%s has %d result parameters", class, n)
			}
		}
	}
	// a literal is not an invocation or a reference: no result parameter
	for _, l := range find(build(t, "attribute a = 1;"), "LiteralInteger") {
		if len(l.Children("ownedRelationship")) != 0 {
			t.Errorf("literal has a result parameter: %s", model.Dump(l))
		}
	}
}

func TestStructureIsAppliedOnce(t *testing.T) {
	root := build(t, "attribute a = x + y;")
	before := model.Dump(root)
	// the expression already has its parameters, so nothing more is added
	for _, e := range find(root, "OperatorExpression") {
		addResultParameter(e)
	}
	if model.Dump(root) != before {
		t.Error("adding a result parameter twice changed the model")
	}
}

func TestFeatureChainSourceTarget(t *testing.T) {
	root := build(t, "attribute a = x.y;")
	chain := find(root, "FeatureChainExpression")[0]
	var src *model.Element
	for _, m := range chain.Children("ownedRelationship") {
		if m.IsA("ParameterMembership") && !m.IsA("ReturnParameterMembership") {
			src = m.Child("ownedRelatedElement")
			break
		}
	}
	fms := src.Children("ownedRelationship")
	var feat *model.Element
	for _, m := range fms {
		if m.Class == "FeatureMembership" {
			feat = m.Child("ownedRelatedElement")
		}
	}
	if feat == nil || !feat.Generated || !feat.Has("declaredName") || feat.Text("declaredName") != "" {
		t.Errorf("source parameter is %s", model.Dump(src))
	}
}

func TestDefaultMultiplicity(t *testing.T) {
	root := build(t, "part def A { attribute plain; attribute withRange [2]; attribute sub :> other; item i; port p; end part e; }")
	got := map[string]bool{}
	for _, class := range []string{"AttributeUsage", "ItemUsage", "PortUsage", "PartUsage"} {
		for _, u := range find(root, class) {
			has := false
			for _, m := range u.Children("ownedRelationship") {
				if m.Child("ownedRelatedElement").IsA("Multiplicity") {
					has = true
				}
			}
			got[u.Text("declaredName")] = has
			// a multiplicity written in the source is not generated; the default is
			for _, m := range u.Children("ownedRelationship") {
				if mem := m.Child("ownedRelatedElement"); mem.IsA("Multiplicity") && mem.Generated != (u.Text("declaredName") != "withRange") {
					t.Errorf("%s: multiplicity generated=%v", u.Text("declaredName"), mem.Generated)
				}
			}
		}
	}
	want := map[string]bool{"plain": true, "withRange": true, "sub": false, "i": true, "p": true, "e": true}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s has multiplicity = %v, want %v", name, got[name], w)
		}
	}
}
