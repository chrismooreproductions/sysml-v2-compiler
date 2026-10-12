// SPDX-License-Identifier: EPL-2.0

package grammar

import (
	"sort"
	"strings"
	"testing"
)

func TestLoadBothLanguages(t *testing.T) {
	for _, l := range []Language{SysML, KerML} {
		g, err := Load(l)
		if err != nil {
			t.Fatalf("Load(%v): %v", l, err)
		}
		if got := g.Undefined(); len(got) != 0 {
			t.Errorf("%v: undefined rules %v", l, got)
		}
		for _, kw := range []string{"package", "part", "import", "specializes"} {
			if l == KerML && kw == "part" {
				continue // a SysML keyword
			}
			if !g.Keywords[kw] {
				t.Errorf("%v: %q is not a keyword", l, kw)
			}
		}
		if _, ok := g.Rules[g.Root]; !ok {
			t.Errorf("%v: no root rule", l)
		}
	}
}

// The numbers pin the vendored release (see UPSTREAM). They change when the
// grammars are re-vendored, which is the point: a changed count prompts a
// look at what changed.
func TestRuleCounts(t *testing.T) {
	cases := []struct {
		l                        Language
		rules, keywords, symbols int
	}{{SysML, 549, 129, 44}, {KerML, 282, 102, 44}}
	for _, c := range cases {
		g, _ := Load(c.l)
		if len(g.Rules) != c.rules || len(g.Keywords) != c.keywords || len(g.Symbols) != c.symbols {
			t.Errorf("%v: %d rules, %d keywords, %d symbols; want %d, %d, %d",
				c.l, len(g.Rules), len(g.Keywords), len(g.Symbols), c.rules, c.keywords, c.symbols)
		}
	}
}

func TestLexicalRulesAreTheOnesTheSyntaxPackageImplements(t *testing.T) {
	g, _ := Load(SysML)
	var terminals []string
	for _, name := range g.Order {
		if g.Rules[name].Kind == Terminal {
			terminals = append(terminals, name)
		}
	}
	want := "DECIMAL_VALUE EXP_VALUE ID ML_NOTE REGULAR_COMMENT SL_NOTE STRING_VALUE UNRESTRICTED_NAME WS"
	sort.Strings(terminals)
	if got := strings.Join(terminals, " "); got != want {
		t.Errorf("terminal rules = %s, want %s", got, want)
	}
}

func TestParseFileConstructs(t *testing.T) {
	src := `
grammar a.b.G with a.b.Base hidden(WS)
import "http://x" as X

@Override
Rule returns X::T : {X::T} a = Other | b += 'k'? c ?= 'flag' ( d = [X::U|Name] )* => e = F;

fragment Frag returns X::T : '(' ( {X::V.op += current} 'o' z = F )+ ')' | .;
enum Kind returns X::K : one = 'a' | two = 'b';
terminal ID : ('a'..'z')+;
`
	f, err := ParseFile("test", src)
	if err != nil {
		t.Fatal(err)
	}
	if f.Base != "a.b.Base" {
		t.Errorf("base = %q", f.Base)
	}
	if len(f.Rules) != 4 {
		t.Fatalf("%d rules, want 4", len(f.Rules))
	}
	kinds := []RuleKind{Normal, Fragment, Enum, Terminal}
	for i, r := range f.Rules {
		if r.Kind != kinds[i] {
			t.Errorf("rule %s kind = %v, want %v", r.Name, r.Kind, kinds[i])
		}
	}
	if f.Rules[0].Returns != "X::T" {
		t.Errorf("returns = %q", f.Rules[0].Returns)
	}
	var seen = map[Kind]bool{}
	for _, r := range f.Rules {
		Walk(r.Body, func(e *Expr) { seen[e.Kind] = true })
	}
	for _, k := range []Kind{Alt, Seq, Rep, Term, Ref, XRef, Assign, Action, Pred, Wild} {
		if !seen[k] {
			t.Errorf("no expression of kind %d parsed", k)
		}
	}
}

func TestParseFileErrors(t *testing.T) {
	cases := map[string]string{
		"missing colon":      "Rule X;",
		"unterminated rule":  "Rule : 'a'",
		"unterminated quote": "Rule : 'a ;",
		"bad character":      "Rule : \x01 ;",
		"unclosed group":     "Rule : ( 'a' ;",
		"bad xref":           "Rule : [ ] ;",
	}
	for name, src := range cases {
		if _, err := ParseFile("bad", src); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// Only `import` is always preceded by a required, small set of keywords (its
// visibility); the computation must find it and no false positives.
func TestPredecessors(t *testing.T) {
	for _, l := range []Language{SysML, KerML} {
		g, _ := Load(l)
		got := g.Predecessors(6)
		if len(got) != 1 || strings.Join(got["import"], " ") != "private protected public" {
			t.Errorf("%v: Predecessors = %v", l, got)
		}
	}
}
