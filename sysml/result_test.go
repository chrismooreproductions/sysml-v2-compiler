// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"reflect"
	"testing"
)

// results evaluates the value expression of each named attribute of P.
func results(t *testing.T, src string) (*Workspace, map[string]Result) {
	t.Helper()
	w, doc, diags := checked(t, "r.sysml", []byte(src))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	out := map[string]Result{}
	doc.Root.Walk(func(e *Element) {
		if !e.IsA("AttributeUsage") || e.Text("declaredName") == "" {
			return
		}
		for _, r := range e.Children("ownedRelationship") {
			if r.IsA("FeatureValue") {
				for _, x := range r.Children("ownedRelatedElement") {
					out[e.Text("declaredName")] = w.Eval(x)
				}
			}
		}
	})
	return w, out
}

func TestEvalReportsTypedValues(t *testing.T) {
	_, got := results(t, `package P {
		attribute i = 4500;
		attribute r = 7 / 2;
		attribute b = 1 < 2;
		attribute s = "ab" + "cd";
		attribute q = (1, 2, 3);
		attribute n = (1, 2, 3)#(7);
	}`)
	for name, want := range map[string]struct {
		kind  ValueKind
		value any
	}{
		"i": {KindInteger, int64(4500)},
		"r": {KindReal, 3.5},
		"b": {KindBoolean, true},
		"s": {KindString, "abcd"},
		"q": {KindSequence, Seq{int64(1), int64(2), int64(3)}},
		"n": {KindNull, nil},
	} {
		r := got[name]
		if r.Status != Determined || r.Kind != want.kind || !reflect.DeepEqual(r.Value, want.value) || r.Reason != "" {
			t.Errorf("%s: %+v, want %v %v", name, r, want.kind, want.value)
		}
		if r.Source == nil || r.Span != r.Source.Span || r.Span.Start.Line == 0 {
			t.Errorf("%s: no source or span: %+v", name, r)
		}
	}
}

func TestEvalSaysNotDeterminedWhenAValueIsMissing(t *testing.T) {
	_, got := results(t, `package P {
		attribute unbound;
		attribute known = 1800;
		attribute plus = unbound + 1;
		attribute cmp = known <= unbound;
		attribute chain = (unbound + 1) * 2 > 0;
	}`)
	for _, name := range []string{"plus", "cmp", "chain"} {
		r := got[name]
		if r.Status != NotDetermined || r.Reason != "P::unbound has no value" || r.Value != nil {
			t.Errorf("%s: %+v", name, r)
		}
		// the span is the reference to the missing value
		if r.Source == nil || r.Source.Class != "FeatureReferenceExpression" || r.Span != r.Source.Span {
			t.Errorf("%s source: %+v", name, r.Source)
		}
	}
}

func TestEvalSaysNotEvaluableForWhatIsUnsupported(t *testing.T) {
	_, got := results(t, `package P {
		attribute unbound;
		attribute call = RealFunctions::sqrt(4.0);
		attribute mixed = unbound + RealFunctions::sqrt(4.0);
		attribute text = "a" - 1;
	}`)
	for name, reason := range map[string]string{
		"call":  "cannot evaluate a InvocationExpression",
		"mixed": "cannot evaluate a InvocationExpression", // unsupported wins over a missing value
		"text":  `cannot apply - to "a" and 1`,
	} {
		r := got[name]
		if r.Status != NotEvaluable || r.Reason != reason || r.Value != nil || r.Source == nil {
			t.Errorf("%s: %+v, want not evaluable: %s", name, r, reason)
		}
	}
}

// A missing value does not stop a boolean operator that the other operand
// decides; it never makes a value up when it does not.
func TestEvalFollowsThreeValuedLogic(t *testing.T) {
	_, got := results(t, `package P {
		attribute u;
		attribute a1 = false and (u > 0);
		attribute a2 = (u > 0) and false;
		attribute a3 = true and (u > 0);
		attribute o1 = true or (u > 0);
		attribute o2 = (u > 0) or true;
		attribute o3 = false or (u > 0);
		attribute i1 = (u > 0) implies true;
		attribute i2 = false implies (u > 0);
		attribute i3 = (u > 0) implies false;
		attribute i4 = true implies (u > 0);
	}`)
	for name, want := range map[string]any{
		"a1": false, "a2": false, "o1": true, "o2": true, "i1": true, "i2": true,
	} {
		if r := got[name]; r.Status != Determined || r.Value != want {
			t.Errorf("%s: %+v, want %v", name, r, want)
		}
	}
	for _, name := range []string{"a3", "o3", "i3", "i4"} {
		if r := got[name]; r.Status != NotDetermined {
			t.Errorf("%s: %+v, want not determined", name, r)
		}
	}
}

func TestResultString(t *testing.T) {
	_, got := results(t, `package P { attribute u; attribute a = 2 + 3; attribute b = u + 1; attribute c = "x" - 1; }`)
	for name, want := range map[string]string{
		"a": "5", "b": "not determined: P::u has no value", "c": `not evaluable: cannot apply - to "x" and 1`,
	} {
		if s := got[name].String(); s != want {
			t.Errorf("%s: %q, want %q", name, s, want)
		}
	}
	if Determined.String() != "determined" || KindReal.String() != "real" || KindSequence.String() != "sequence" {
		t.Error("String methods are wrong")
	}
}
