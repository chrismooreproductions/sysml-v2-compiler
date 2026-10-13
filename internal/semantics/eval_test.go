// SPDX-License-Identifier: EPL-2.0

package semantics

import (
	"strings"
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
)

// values evaluates every attribute with a value in a model and returns
// "name = value" for each, or "name: error".
func values(t *testing.T, src string) map[string]string {
	t.Helper()
	w, roots := resolved(t, true, src)
	out := map[string]string{}
	roots[0].Walk(func(e *model.Element) {
		if !e.IsA("AttributeUsage") || e.Text("declaredName") == "" {
			return
		}
		for _, fv := range w.rels(e, "FeatureValue") {
			v, err := w.Evaluate(first(w.ownedRelated(fv)))
			if err != nil {
				out[e.Text("declaredName")] = "error: " + err.Error()
			} else {
				out[e.Text("declaredName")] = FormatValue(v)
			}
		}
	})
	return out
}

func TestEvaluate(t *testing.T) {
	got := values(t, `package P {
		attribute a = 4500;
		attribute b = 1.5 * 2 + a;
		attribute c = if a > 3 ? "big" else "small";
		attribute d = (1, 2, 3);
		attribute e = not true and (1 == 1);
		attribute f = d#(2);
		attribute g = 1..4;
		attribute h = 7 / 2;
		attribute i = 8 / 2;
		attribute j = 7 % 3 - 2 ** 3;
		attribute k = "ab" + "cd";
		attribute l = true or (1 / 0 > 1);
		attribute m = a == 4500.0;
		attribute n = (1, 2) == (1, 2);
		attribute o = null ?? 5;
		attribute p = -a;
		attribute q = (1, 2, 3)#(7);
	}`)
	want := map[string]string{
		"a": "4500", "b": "4503", "c": `"big"`, "d": "(1, 2, 3)", "e": "false", "f": "2",
		"g": "(1, 2, 3, 4)", "h": "3.5", "i": "4", "j": "-7", "k": `"abcd"`, "l": "true",
		"m": "true", "n": "true", "o": "5", "p": "-4500", "q": "null",
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s = %s, want %s", name, got[name], w)
		}
	}
}

func TestEvaluateReportsWhatItCannotDo(t *testing.T) {
	got := values(t, `package P {
		attribute noValue;
		attribute a = noValue;
		attribute b = a + 1;
		attribute c = 1 + "x";
		attribute again = again + 1;
	}`)
	for name, frag := range map[string]string{
		"a": "has no value", "b": "has no value", "c": "cannot apply +", "again": "defined in terms of itself",
	} {
		if !strings.Contains(got[name], frag) {
			t.Errorf("%s: %q, want it to mention %q", name, got[name], frag)
		}
	}
}
