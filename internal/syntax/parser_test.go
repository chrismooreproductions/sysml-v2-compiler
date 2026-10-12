// SPDX-License-Identifier: EPL-2.0

package syntax

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
)

func mustParse(t *testing.T, lang grammar.Language, src string) *Node {
	t.Helper()
	n, err := Parse(lang, "t", src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return n
}

// find returns the nodes of a class in document order.
func find(n *Node, class string) []*Node {
	var out []*Node
	var walk func(*Node)
	walk = func(x *Node) {
		if x.Class == class {
			out = append(out, x)
		}
		for _, p := range x.Props {
			if c, ok := p.Value.(*Node); ok {
				walk(c)
			}
		}
	}
	walk(n)
	return out
}

func text(src string, s Span) string { return src[s.Start.Offset:s.End.Offset] }

func TestTrees(t *testing.T) {
	cases := []struct {
		lang grammar.Language
		src  string
		want string
	}{
		{grammar.SysML, "package P { part x; }",
			`Namespace(ownedRelationship+=OwningMembership(ownedRelatedElement+=Package(declaredName="P" ownedRelationship+=OwningMembership(ownedRelatedElement+=PartUsage(declaredName="x")))))`},
		{grammar.SysML, "package P { part def A; part x : A[1..*]; }",
			`Namespace(ownedRelationship+=OwningMembership(ownedRelatedElement+=Package(declaredName="P" ownedRelationship+=OwningMembership(ownedRelatedElement+=PartDefinition(declaredName="A")) ownedRelationship+=OwningMembership(ownedRelatedElement+=PartUsage(declaredName="x" ownedRelationship+=FeatureTyping(type=@A) ownedRelationship+=OwningMembership(ownedRelatedElement+=MultiplicityRange(ownedRelationship+=OwningMembership(ownedRelatedElement+=LiteralInteger(value="1")) ownedRelationship+=OwningMembership(ownedRelatedElement+=LiteralInfinity()))))))))`},
		// a flag set by a keyword, and an enumeration literal
		{grammar.SysML, "abstract part def A { in attribute x; }",
			`Namespace(ownedRelationship+=OwningMembership(ownedRelatedElement+=PartDefinition(isAbstract?=true declaredName="A" ownedRelationship+=FeatureMembership(ownedRelatedElement+=AttributeUsage(direction="in" declaredName="x")))))`},
		// `nonunique` is converted to false by the pilot's value converter
		{grammar.SysML, "part x nonunique;",
			`Namespace(ownedRelationship+=OwningMembership(ownedRelatedElement+=PartUsage(declaredName="x" isUnique=false)))`},
		{grammar.KerML, "classifier A specializes B { feature f : C; }",
			`Namespace(ownedRelationship+=OwningMembership(ownedRelatedElement+=Classifier(declaredName="A" ownedRelationship+=Subclassification(superclassifier=@B) ownedRelationship+=FeatureMembership(ownedRelatedElement+=Feature(declaredName="f" ownedRelationship+=FeatureTyping(type=@C))))))`},
	}
	for _, c := range cases {
		got := Dump(mustParse(t, c.lang, c.src))
		if got != c.want {
			t.Errorf("%q:\n got  %s\n want %s", c.src, got, c.want)
		}
	}
}

func TestEmptyAndCommentOnlyDocuments(t *testing.T) {
	for _, src := range []string{"", "  \n", "// just a note\n", "//* another\n kind of note */"} {
		n := mustParse(t, grammar.SysML, src)
		if n.Class != "Namespace" || len(n.Props) != 0 {
			t.Errorf("%q: %s", src, Dump(n))
		}
	}
}

// A regular comment is not skipped like a note: standing alone, it is the body
// of an anonymous Comment element.
func TestRegularCommentIsAnElement(t *testing.T) {
	n := mustParse(t, grammar.SysML, "/* a comment */")
	if got := len(find(n, "Comment")); got != 1 {
		t.Errorf("%d Comment elements in %s", got, Dump(n))
	}
}

func TestSpans(t *testing.T) {
	src := "package P {\n  part x : A;\n}"
	n := mustParse(t, grammar.SysML, src)
	cases := []struct {
		class string
		want  string
	}{
		{"Namespace", src},
		{"Package", src},
		{"PartUsage", "part x : A;"},
		{"FeatureTyping", "A"}, // the ":" belongs to the rule that owns the typing
	}
	for _, c := range cases {
		nodes := find(n, c.class)
		if len(nodes) != 1 {
			t.Fatalf("%s: %d nodes", c.class, len(nodes))
		}
		if got := text(src, nodes[0].Span); got != c.want {
			t.Errorf("%s spans %q, want %q", c.class, got, c.want)
		}
	}
	// the node's position as a line and column
	if s := find(n, "PartUsage")[0].Span.Start; s.Line != 2 || s.Column != 3 {
		t.Errorf("PartUsage starts at %v, want 2:3", s)
	}
}

func TestSpansOfOperatorExpressions(t *testing.T) {
	src := "attribute a = 1 - 2 - 3 * 4;"
	n := mustParse(t, grammar.SysML, src)
	var got []string
	for _, e := range find(n, "OperatorExpression") {
		got = append(got, text(src, e.Span))
	}
	// left associative, and * binds tighter: ((1 - 2) - (3 * 4))
	want := []string{"1 - 2 - 3 * 4", "1 - 2", "3 * 4"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("operator expression spans %q, want %q", got, want)
	}
	// the literals
	var lits []string
	for _, l := range find(n, "LiteralInteger") {
		lits = append(lits, text(src, l.Span))
	}
	if strings.Join(lits, ",") != "1,2,3,4" {
		t.Errorf("literal spans %q", lits)
	}
}

func TestReferencesCarryTheirSpan(t *testing.T) {
	src := "part x : 'Q R'::A;"
	n := mustParse(t, grammar.SysML, src)
	typing := find(n, "FeatureTyping")[0]
	v, ok := typing.Get("type")
	r, isRef := v.(Ref)
	if !ok || !isRef {
		t.Fatalf("type = %#v", v)
	}
	if r.Name != "'Q R'::A" || text(src, r.Span) != "'Q R'::A" {
		t.Errorf("reference %q spans %q", r.Name, text(src, r.Span))
	}
}

func TestEmptyNodeHasZeroWidthSpan(t *testing.T) {
	src := "part def A;"
	n := mustParse(t, grammar.SysML, src)
	// the grammar creates a Multiplicity for individual definitions without any text of its own
	src2 := "individual part def B;"
	n2 := mustParse(t, grammar.SysML, src2)
	_ = n
	for _, m := range find(n2, "Multiplicity") {
		if m.Span.Start != m.Span.End {
			t.Errorf("empty node spans %v-%v", m.Span.Start, m.Span.End)
		}
	}
}

func TestSyntaxErrors(t *testing.T) {
	type want struct {
		line, col int
		got       string
		expect    []string // each must be among the expected tokens
	}
	cases := map[string]want{
		"package P {":                               {1, 12, "", []string{"'}'"}},
		"package P { part x : ; }":                  {1, 22, ";", []string{"a name", "a quoted name"}},
		"package P { import A; }":                   {1, 13, "import", []string{"'private'", "'public'", "'protected'"}},
		"package P { private import ; }":            {1, 28, ";", []string{"a name", "'all'"}},
		"package P { attribute a = ; }":             {1, 29, "}", []string{"';'"}},
		"package P { part x [1.. ; }":               {1, 25, ";", []string{"a number", "'*'"}},
		"pakage P { }":                              {1, 8, "P", []string{"';'", "':'"}},
		"package P { part x } ":                     {1, 20, "}", []string{"';'", "':'"}},
		"package P { alias A for ; }":               {1, 25, ";", []string{"a name"}},
		"package P { part x :> ; }":                 {1, 23, ";", []string{"a name"}},
		"package P { part x redefines ; }":          {1, 30, ";", []string{"a name"}},
		"package P { connection c connect a to ; }": {1, 39, ";", []string{"a name"}},
		"package P {\n  part x;\n  part y\n}":       {4, 1, "}", []string{"';'"}},
		"part def A;;":                              {1, 12, ";", nil},
		"package P { doc /* ok */ part x; } extra":  {1, 41, "", []string{"';'"}},
	}
	for src, w := range cases {
		_, err := Parse(grammar.SysML, "f.sysml", src)
		e, ok := err.(*Error)
		if !ok {
			t.Errorf("%q: error %v is not a *Error", src, err)
			continue
		}
		if e.Pos.Line != w.line || e.Pos.Column != w.col || e.Got != w.got {
			t.Errorf("%q: error at %v got %q (%s), want %d:%d got %q", src, e.Pos, e.Got, e.Msg, w.line, w.col, w.got)
		}
		if !strings.HasPrefix(e.Error(), "f.sysml:") {
			t.Errorf("%q: message %q does not start with the file", src, e.Error())
		}
		for _, x := range w.expect {
			found := false
			for _, got := range e.Expected {
				if got == x {
					found = true
				}
			}
			if !found {
				t.Errorf("%q: %s is not among the %d expected tokens", src, x, len(e.Expected))
			}
		}
	}
}

func TestLexicalErrorsComeFromParse(t *testing.T) {
	_, err := Parse(grammar.SysML, "f", "package P { /* open")
	e, ok := err.(*Error)
	if !ok || e.Pos.Line != 1 || e.Pos.Column != 13 || !strings.Contains(e.Msg, "unterminated comment") {
		t.Errorf("got %v", err)
	}
}

// The corpus test: every model in the vendored standard library and example
// sets parses.
func TestVendoredCorpusParses(t *testing.T) {
	roots := []string{"../library/sysml.library", "../../testdata/upstream", "../../sysml/testdata/spec_examples"}
	count := 0
	for _, root := range roots {
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			var lang grammar.Language
			switch filepath.Ext(path) {
			case ".sysml":
				lang = grammar.SysML
			case ".kerml":
				lang = grammar.KerML
			default:
				return nil
			}
			src, _ := os.ReadFile(path)
			if _, err := Parse(lang, path, string(src)); err != nil {
				t.Errorf("%v", err)
			}
			count++
			return nil
		})
	}
	if count < 410 {
		t.Errorf("only %d models parsed; is the corpus vendored?", count)
	}
}

func BenchmarkParseLibrary(b *testing.B) {
	var files []string
	filepath.Walk("../library/sysml.library", func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && (filepath.Ext(path) == ".sysml" || filepath.Ext(path) == ".kerml") {
			files = append(files, path)
		}
		return nil
	})
	srcs := make([]string, len(files))
	for i, f := range files {
		data, _ := os.ReadFile(f)
		srcs[i] = string(data)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j, f := range files {
			lang := grammar.SysML
			if filepath.Ext(f) == ".kerml" {
				lang = grammar.KerML
			}
			if _, err := Parse(lang, f, srcs[j]); err != nil {
				b.Fatal(err)
			}
		}
	}
}
