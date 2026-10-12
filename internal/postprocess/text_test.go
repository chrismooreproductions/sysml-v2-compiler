// SPDX-License-Identifier: EPL-2.0

package postprocess

import (
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
)

func TestUnescapeString(t *testing.T) {
	cases := map[string]string{
		``:                   ``,
		`plain`:              `plain`, // not quoted: unchanged
		`'a b'`:              `a b`,
		`"text"`:             `text`,
		`''`:                 ``,
		`"a\"b"`:             `a"b`,
		`'it\'s'`:            `it's`,
		`"tab\there"`:        "tab\there",
		`"nl\nx"`:            "nl\nx",
		`"back\\slash"`:      `back\slash`,
		`"\b\f\r"`:           "\b\f\r",
		`"odd\qescape"`:      `odd escape`,
		`'trailing\\'`:       `trailing\`,
		`"two \" and \" ok"`: `two " and " ok`,
	}
	for in, want := range cases {
		if got := unescapeString(in); got != want {
			t.Errorf("unescapeString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProcessCommentBody(t *testing.T) {
	cases := map[string]string{
		"/* one line */":            "one line ",
		"/**/":                      "",
		"/* a\n * b\n * c\n */":     "a\nb\nc\n",
		"/*\n * A.\n * B.\n */":     "A.\nB.\n",
		"/* x\n   *y\n */":          "x\ny\n",
		"/* first\n second\n */":    "first\nsecond\n",
		"/*\r\n * A\r\n */":         "A\n",
		"/* no star\n  \n  more */": "no star\n\nmore ",
		"/* ends\n*/":               "ends",
	}
	for in, want := range cases {
		if got := processCommentBody(in); got != want {
			t.Errorf("processCommentBody(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNamesAndStringsAreUnescapedInTheModel(t *testing.T) {
	root := run(t, grammar.SysML, `package 'My \'P\'' { doc /* Hello.
	 * World. */ attribute s = "a\tb"; alias <'x y'> Z for A; }`)
	pkg := one(t, root, "Package")
	if got := pkg.Text("declaredName"); got != "My 'P'" {
		t.Errorf("declaredName = %q", got)
	}
	if got := one(t, root, "Documentation").Text("body"); got != "Hello.\nWorld. " {
		t.Errorf("doc body = %q", got)
	}
	if got := one(t, root, "LiteralString").Text("value"); got != "a\tb" {
		t.Errorf("string value = %q", got)
	}
	alias := one(t, root, "Membership")
	if alias.Text("memberShortName") != "x y" || alias.Text("memberName") != "Z" {
		t.Errorf("alias names = %q, %q", alias.Text("memberShortName"), alias.Text("memberName"))
	}
}
