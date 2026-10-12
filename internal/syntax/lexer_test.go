// SPDX-License-Identifier: EPL-2.0

package syntax

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
)

func testLexer(t *testing.T, l grammar.Language) *lexer {
	t.Helper()
	g, err := grammar.Load(l)
	if err != nil {
		t.Fatal(err)
	}
	return newLexer(g)
}

// summary renders tokens as kind:text, for comparison.
func summary(toks []Token) string {
	var parts []string
	for _, tk := range toks {
		p := tk.Text
		switch tk.Kind {
		case Keyword:
			p = "kw:" + p
		case Name:
			p = "id:" + p
		case QuotedName:
			p = "q:" + p
		case String:
			p = "s:" + p
		case Comment:
			p = "c"
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, " ")
}

func TestLexKinds(t *testing.T) {
	l := testLexer(t, grammar.SysML)
	cases := map[string]string{
		"part def Vehicle;":               "kw:part kw:def id:Vehicle ;",
		"'my name' :>> x ::> y":           "q:'my name' :>> id:x ::> id:y",
		`doc /* hi */ x = "a\"b"`:         `kw:doc c id:x = s:"a\"b"`,
		"a.b..c => d":                     "id:a . id:b .. id:c => id:d",
		"1 22 3e5 4E-2 7e":                "1 22 3e5 4E-2 7 id:e",
		"x // gone\ny //* a\nb */ z":      "id:x id:y id:z",
		"a.?b ?? c":                       "id:a .? id:b ?? id:c",
		"'it\\'s' _x9 Y_":                 "q:'it\\'s' id:_x9 id:Y_",
		"private import A::B::*;":         "kw:private kw:import id:A :: id:B :: * ;",
		"attribute x : Real [0..1] = 5.0": "kw:attribute id:x : id:Real [ 0 .. 1 ] = 5 . 0",
	}
	for src, want := range cases {
		toks, err := l.lex("t", src)
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		if got := summary(toks); got != want {
			t.Errorf("%q:\n got  %s\n want %s", src, got, want)
		}
	}
}

func TestLexLongestSymbolWins(t *testing.T) {
	l := testLexer(t, grammar.SysML)
	toks, err := l.lex("t", ":>> ::> :> :: : =>=== ==")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := summary(toks), ":>> ::> :> :: : => === =="; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestLexPositions(t *testing.T) {
	l := testLexer(t, grammar.SysML)
	src := "package P {\n\tpart x;\n  /* a\n b */ y\n}"
	toks, err := l.lex("t", src)
	if err != nil {
		t.Fatal(err)
	}
	type at struct {
		text string
		line int
		col  int
	}
	want := []at{{"package", 1, 1}, {"P", 1, 9}, {"{", 1, 11}, {"part", 2, 2}, {"x", 2, 7}, {";", 2, 8}, {"/* a\n b */", 3, 3}, {"y", 4, 7}, {"}", 5, 1}}
	if len(toks) != len(want) {
		t.Fatalf("%d tokens, want %d: %s", len(toks), len(want), summary(toks))
	}
	for i, w := range want {
		g := toks[i]
		if g.Text != w.text || g.Span.Start.Line != w.line || g.Span.Start.Column != w.col {
			t.Errorf("token %d = %q at %v, want %q at %d:%d", i, g.Text, g.Span.Start, w.text, w.line, w.col)
		}
		if src[g.Span.Start.Offset:g.Span.End.Offset] != g.Text {
			t.Errorf("token %d: span does not cover its text", i)
		}
	}
	// the comment spans two lines
	if c := toks[6]; c.Span.End.Line != 4 {
		t.Errorf("comment ends on line %d, want 4", c.Span.End.Line)
	}
}

func TestLexErrors(t *testing.T) {
	l := testLexer(t, grammar.SysML)
	cases := map[string]string{
		"x\n  /* open": "2:3: unterminated comment",
		"a 'open":      "1:3: unterminated quoted name",
		"a \"open":     "1:3: unterminated string",
		"//* open":     "1:1: unterminated note",
		"a \u00e9":     "1:3: unexpected character",
		"x\ny `":       "2:3: unexpected character",
	}
	for src, want := range cases {
		_, err := l.lex("f.sysml", src)
		if err == nil || !strings.Contains(err.Error(), "f.sysml:"+want) {
			t.Errorf("%q: error %v, want it to contain %q", src, err, "f.sysml:"+want)
		}
	}
}

// Every vendored model must lex: this is the lexical half of the corpus test.
func TestLexVendoredCorpus(t *testing.T) {
	roots := map[grammar.Language][]string{
		grammar.SysML: {"../library/sysml.library", "../../testdata/upstream/sysml"},
		grammar.KerML: {"../library/sysml.library", "../../testdata/upstream/kerml"},
	}
	count := 0
	for lang, dirs := range roots {
		l := testLexer(t, lang)
		ext := ".sysml"
		if lang == grammar.KerML {
			ext = ".kerml"
		}
		for _, dir := range dirs {
			filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || filepath.Ext(path) != ext {
					return nil
				}
				src, _ := os.ReadFile(path)
				if _, err := l.lex(path, string(src)); err != nil {
					t.Errorf("%v", err)
				}
				count++
				return nil
			})
		}
	}
	if count < 400 {
		t.Errorf("only %d files lexed; is the corpus vendored?", count)
	}
}
