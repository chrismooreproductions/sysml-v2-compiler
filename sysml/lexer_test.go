package sysml_test

import (
	"testing"

	"github.com/chrismooreproductions/sysml-modeller/sysml"
)

func TestModelLex(t *testing.T) {
	m := sysml.NewModel(vehicleModel)

	tokens, err := m.Lex()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	type expected struct {
		kind  sysml.Kind
		value string
		pos   sysml.Pos
	}

	want := []expected{
		{sysml.Pkg, "package", sysml.Pos{Offset: 0, Line: 1, Col: 1}}, {sysml.Space, " ", sysml.Pos{Offset: 7, Line: 1, Col: 8}}, {sysml.Identifier, "Vehicle", sysml.Pos{Offset: 8, Line: 1, Col: 9}}, {sysml.Space, " ", sysml.Pos{Offset: 15, Line: 1, Col: 16}}, {sysml.OpenBrace, "{", sysml.Pos{Offset: 16, Line: 1, Col: 17}},
		{sysml.Space, "    ", sysml.Pos{Offset: 18, Line: 2, Col: 1}}, {sysml.Part, "part", sysml.Pos{Offset: 22, Line: 2, Col: 5}}, {sysml.Space, " ", sysml.Pos{Offset: 26, Line: 2, Col: 9}}, {sysml.Def, "def", sysml.Pos{Offset: 27, Line: 2, Col: 10}}, {sysml.Space, " ", sysml.Pos{Offset: 30, Line: 2, Col: 13}}, {sysml.Identifier, "Engine", sysml.Pos{Offset: 31, Line: 2, Col: 14}}, {sysml.Semicolon, ";", sysml.Pos{Offset: 37, Line: 2, Col: 20}},
		{sysml.Space, "    ", sysml.Pos{Offset: 40, Line: 4, Col: 1}}, {sysml.Part, "part", sysml.Pos{Offset: 44, Line: 4, Col: 5}}, {sysml.Space, " ", sysml.Pos{Offset: 48, Line: 4, Col: 9}}, {sysml.Def, "def", sysml.Pos{Offset: 49, Line: 4, Col: 10}}, {sysml.Space, " ", sysml.Pos{Offset: 52, Line: 4, Col: 13}}, {sysml.Identifier, "Car", sysml.Pos{Offset: 53, Line: 4, Col: 14}}, {sysml.Space, " ", sysml.Pos{Offset: 56, Line: 4, Col: 17}}, {sysml.OpenBrace, "{", sysml.Pos{Offset: 57, Line: 4, Col: 18}},
		{sysml.Space, "        ", sysml.Pos{Offset: 59, Line: 5, Col: 1}}, {sysml.Part, "part", sysml.Pos{Offset: 67, Line: 5, Col: 9}}, {sysml.Space, " ", sysml.Pos{Offset: 71, Line: 5, Col: 13}}, {sysml.Identifier, "engine", sysml.Pos{Offset: 72, Line: 5, Col: 14}}, {sysml.Space, " ", sysml.Pos{Offset: 78, Line: 5, Col: 20}}, {sysml.Colon, ":", sysml.Pos{Offset: 79, Line: 5, Col: 21}}, {sysml.Space, " ", sysml.Pos{Offset: 80, Line: 5, Col: 22}}, {sysml.Identifier, "Engine", sysml.Pos{Offset: 81, Line: 5, Col: 23}}, {sysml.Semicolon, ";", sysml.Pos{Offset: 87, Line: 5, Col: 29}},
		{sysml.Space, "    ", sysml.Pos{Offset: 89, Line: 6, Col: 1}}, {sysml.CloseBrace, "}", sysml.Pos{Offset: 93, Line: 6, Col: 5}},
		{sysml.Space, "\t", sysml.Pos{Offset: 95, Line: 7, Col: 1}}, {sysml.CloseBrace, "}", sysml.Pos{Offset: 96, Line: 7, Col: 2}},
	}

	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(want))
	}

	for i, tok := range tokens {
		if tok.Kind != want[i].kind || string(tok.Value) != want[i].value || tok.Pos != want[i].pos {
			t.Errorf("token %d: got {kind:%v value:%q pos:%+v}, want {kind:%v value:%q pos:%+v}", i, tok.Kind, string(tok.Value), tok.Pos, want[i].kind, want[i].value, want[i].pos)
		}
	}
}

// TestLexerUnicode guards against offsets being tracked in runes instead of
// bytes: DecodeRuneInString's size is a byte count, so a multi-byte rune
// (café's "é" is 2 bytes, the car emoji is 4) must advance the offset by
// more than one per rune, or every subsequent decode reads from the middle
// of a UTF-8 sequence and the rest of the token stream comes out garbled.
func TestLexerUnicode(t *testing.T) {
	source := `part 🚗 Motoré : Café;`

	m := sysml.NewModel(source)

	tokens, err := m.Lex()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tokens) == 0 {
		t.Fatal("got no tokens")
	}

	rebuilt := ""
	offset := 0
	for _, tok := range tokens {
		value := string(tok.Value)

		if tok.Pos.Offset != offset {
			t.Errorf("token %q: pos.Offset = %d, want %d", value, tok.Pos.Offset, offset)
		}

		rebuilt += value
		offset += len(value)
	}

	if rebuilt != source {
		t.Errorf("rebuilt source = %q, want %q", rebuilt, source)
	}

	if offset != len(source) {
		t.Errorf("final offset = %d, want %d (len(source) in bytes)", offset, len(source))
	}
}

// TestLexerLineComment checks a "//" comment scans as one Comment token
// running to (but not including) the newline, and that the newline still
// advances Line/Col for whatever follows -- matching how a bare newline is
// otherwise invisible to the token stream.
func TestLexerLineComment(t *testing.T) {
	m := sysml.NewModel("part // a comment\nx")

	tokens, err := m.Lex()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []struct {
		kind  sysml.Kind
		value string
	}{
		{sysml.Part, "part"},
		{sysml.Space, " "},
		{sysml.Comment, "// a comment"},
		{sysml.Identifier, "x"},
	}

	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(want))
	}
	for i, tok := range tokens {
		if tok.Kind != want[i].kind || string(tok.Value) != want[i].value {
			t.Errorf("token %d: got {%v %q}, want {%v %q}", i, tok.Kind, string(tok.Value), want[i].kind, want[i].value)
		}
	}

	x := tokens[3]
	if x.Pos != (sysml.Pos{Offset: 18, Line: 2, Col: 1}) {
		t.Errorf("x pos = %+v, want {Offset:18 Line:2 Col:1}", x.Pos)
	}
}

// TestLexerBlockComment checks a "/* ... */" comment scans as one Comment
// token, including one that spans multiple lines -- where, unlike every
// other token, Line/Col bookkeeping has to be tracked by hand inside the
// scan rather than left to the next() loop's own newline case.
func TestLexerBlockComment(t *testing.T) {
	cases := map[string]struct {
		source      string
		wantComment string
		wantAfter   sysml.Pos
	}{
		"single line": {
			source:      "part /* a */ x",
			wantComment: "/* a */",
			wantAfter:   sysml.Pos{Offset: 13, Line: 1, Col: 14},
		},
		"multi line": {
			source:      "part /* a\nb */ x",
			wantComment: "/* a\nb */",
			wantAfter:   sysml.Pos{Offset: 15, Line: 2, Col: 6},
		},
		"unterminated runs to EOF": {
			source:      "part /* a",
			wantComment: "/* a",
			wantAfter:   sysml.Pos{}, // no token follows
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			m := sysml.NewModel(tt.source)

			tokens, err := m.Lex()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var comment *sysml.Token
			for i := range tokens {
				if tokens[i].Kind == sysml.Comment {
					comment = &tokens[i]
					break
				}
			}
			if comment == nil {
				t.Fatalf("no Comment token in %v", tokens)
			}
			if string(comment.Value) != tt.wantComment {
				t.Errorf("comment = %q, want %q", string(comment.Value), tt.wantComment)
			}

			if name == "unterminated runs to EOF" {
				if len(tokens) != 3 {
					t.Fatalf("got %d tokens, want 3 (Part, Space, Comment)", len(tokens))
				}
				return
			}

			last := tokens[len(tokens)-1]
			if last.Pos != tt.wantAfter {
				t.Errorf("token after comment: pos = %+v, want %+v", last.Pos, tt.wantAfter)
			}
		})
	}
}

// TestLexerBareSlash checks a "/" not followed by another "/" or "*" lexes
// as its own Slash token (division) rather than being swallowed as a
// (malformed) comment opener, or glued onto the identifiers around it.
func TestLexerBareSlash(t *testing.T) {
	m := sysml.NewModel("a/b")

	tokens, err := m.Lex()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantKinds := []sysml.Kind{sysml.Identifier, sysml.Slash, sysml.Identifier}
	wantValues := []string{"a", "/", "b"}

	if len(tokens) != len(wantKinds) {
		t.Fatalf("got %d tokens (%v), want %d", len(tokens), tokens, len(wantKinds))
	}
	for i, tok := range tokens {
		if tok.Kind != wantKinds[i] || string(tok.Value) != wantValues[i] {
			t.Errorf("token %d: got {%v %q}, want {%v %q}", i, tok.Kind, string(tok.Value), wantKinds[i], wantValues[i])
		}
	}
}

// TestLexerPathSep checks "::" lexes as one PathSep token rather than two
// separate Colon tokens, while a lone ":" (including one merely adjacent
// to whitespace rather than another ":") still lexes as Colon.
// TestLexerColonForms checks that a leading ':' resolves to the right
// token depending on how many characters follow it -- ':' alone (Colon),
// '::' (PathSep), ':>' (Subsets), or ':>>' (Redefines) -- and that each
// consumes only its own characters, leaving whatever comes after untouched.
func TestLexerColonForms(t *testing.T) {
	cases := map[string]struct {
		source    string
		wantKind  sysml.Kind
		wantValue string
	}{
		"colon":     {"a : b", sysml.Colon, ":"},
		"path sep":  {"a::b", sysml.PathSep, "::"},
		"subsets":   {"a :> b", sysml.Subsets, ":>"},
		"redefines": {"a :>> b", sysml.Redefines, ":>>"},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			tokens, err := sysml.NewModel(tt.source).Lex()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var found *sysml.Token
			for i := range tokens {
				if tokens[i].Kind != sysml.Identifier && tokens[i].Kind != sysml.Space {
					found = &tokens[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("no punctuation token in %v", tokens)
			}
			if found.Kind != tt.wantKind || string(found.Value) != tt.wantValue {
				t.Errorf("got {%v %q}, want {%v %q}", found.Kind, string(found.Value), tt.wantKind, tt.wantValue)
			}
		})
	}
}

func TestLexerPathSep(t *testing.T) {
	m := sysml.NewModel(`Vehicle::Car : x`)

	tokens, err := m.Lex()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var kinds []sysml.Kind
	var values []string
	for _, tok := range tokens {
		kinds = append(kinds, tok.Kind)
		values = append(values, string(tok.Value))
	}

	wantKinds := []sysml.Kind{
		sysml.Identifier, sysml.PathSep, sysml.Identifier, sysml.Space,
		sysml.Colon, sysml.Space, sysml.Identifier,
	}
	wantValues := []string{"Vehicle", "::", "Car", " ", ":", " ", "x"}

	if len(tokens) != len(wantKinds) {
		t.Fatalf("got %d tokens (%v), want %d (%v)", len(tokens), values, len(wantKinds), wantValues)
	}
	for i := range tokens {
		if kinds[i] != wantKinds[i] || values[i] != wantValues[i] {
			t.Errorf("token %d: got {%v %q}, want {%v %q}", i, kinds[i], values[i], wantKinds[i], wantValues[i])
		}
	}
}

// firstNonTrivia returns the first token in tokens whose Kind is neither
// Space nor Comment -- a small helper for tests that only care about one
// meaningful token amid the whitespace around it.
func firstNonTrivia(tokens []sysml.Token) (sysml.Token, bool) {
	for _, tok := range tokens {
		if tok.Kind != sysml.Space && tok.Kind != sysml.Comment {
			return tok, true
		}
	}
	return sysml.Token{}, false
}

// TestLexerOperators checks that each new punctuation-based operator token
// (added for Phase 1's expression subsystem) lexes correctly, including
// each two-character form's one-character fallback ("<" without a
// following "=", etc.).
func TestLexerOperators(t *testing.T) {
	cases := map[string]struct {
		source string
		want   sysml.Kind
	}{
		"plus":        {"+", sysml.Plus},
		"minus":       {"-", sysml.Minus},
		"slash":       {"/", sysml.Slash},
		"percent":     {"%", sysml.Percent},
		"power":       {"**", sysml.Power},
		"star alone":  {"*", sysml.Star},
		"ampersand":   {"&", sysml.Ampersand},
		"pipe":        {"|", sysml.Pipe},
		"lt":          {"<", sysml.Lt},
		"gt":          {">", sysml.Gt},
		"le":          {"<=", sysml.Le},
		"ge":          {">=", sysml.Ge},
		"eq":          {"==", sysml.Eq},
		"not eq":      {"!=", sysml.NotEq},
		"bang alone":  {"!x", sysml.Identifier}, // no standalone '!' operator
		"open paren":  {"(", sysml.OpenParen},
		"close paren": {")", sysml.CloseParen},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			tokens, err := sysml.NewModel(tt.source).Lex()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tok, ok := firstNonTrivia(tokens)
			if !ok {
				t.Fatalf("no token in %v", tokens)
			}
			if tok.Kind != tt.want {
				t.Errorf("Lex(%q) first token kind = %v, want %v", tt.source, tok.Kind, tt.want)
			}
		})
	}
}

// TestLexerKeywordOperators checks the word-spelled operators/literals
// Phase 1 adds: true/false (boolean literals), not, and xor.
func TestLexerKeywordOperators(t *testing.T) {
	cases := map[string]sysml.Kind{
		"true":  sysml.TrueKw,
		"false": sysml.FalseKw,
		"not":   sysml.NotKw,
		"xor":   sysml.XorKw,
	}

	for word, want := range cases {
		t.Run(word, func(t *testing.T) {
			tokens, err := sysml.NewModel(word).Lex()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(tokens) != 1 || tokens[0].Kind != want {
				t.Errorf("Lex(%q) = %v, want a single %v token", word, tokens, want)
			}
		})
	}
}

// TestLexerStringLiteral checks that a double-quoted string lexes as one
// StringLit token including both quotes, and that an unterminated string
// runs to EOF rather than erroring (the lexer has no error path, matching
// how an unterminated block comment is already handled).
func TestLexerStringLiteral(t *testing.T) {
	cases := map[string]struct {
		source string
		want   string
	}{
		"simple":              {`"hello"`, `"hello"`},
		"empty":               {`""`, `""`},
		"with spaces":         {`"hello world"`, `"hello world"`},
		"unterminated at EOF": {`"hello`, `"hello`},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			tokens, err := sysml.NewModel(tt.source).Lex()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(tokens) != 1 || tokens[0].Kind != sysml.StringLit || string(tokens[0].Value) != tt.want {
				t.Errorf("Lex(%q) = %v, want a single StringLit %q", tt.source, tokens, tt.want)
			}
		})
	}
}

// TestLexerNumericLiteral checks that a run of digits lexes as one
// Identifier-kind token (matching how "5" already worked for multiplicity
// bounds), that a decimal point followed by more digits extends it into a
// real literal ("3.14"), and that a '.' NOT followed by a digit is left as
// its own separate token -- needed so a feature chain like "a.b" (Phase 2)
// never has its '.' mistaken for part of a number.
func TestLexerNumericLiteral(t *testing.T) {
	cases := map[string]struct {
		source    string
		wantFirst string
		wantTotal int
	}{
		"integer":                   {"5", "5", 1},
		"real":                      {"3.14", "3.14", 1},
		"dot not followed by digit": {"3.x", "3", 3}, // "3", ".", "x"
		"trailing dot at EOF":       {"3.", "3", 2},  // "3", "."
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			tokens, err := sysml.NewModel(tt.source).Lex()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(tokens) != tt.wantTotal {
				t.Fatalf("Lex(%q) = %v, want %d tokens", tt.source, tokens, tt.wantTotal)
			}
			if string(tokens[0].Value) != tt.wantFirst {
				t.Errorf("Lex(%q) first token = %q, want %q", tt.source, string(tokens[0].Value), tt.wantFirst)
			}
		})
	}
}

// TestLexerDot checks that a bare '.' (not doubled into "..") lexes as its
// own Dot token -- needed for Phase 2's feature chains (e.g. "a.b") -- and
// that "::" and ".." are unaffected.
func TestLexerDot(t *testing.T) {
	tokens, err := sysml.NewModel("a.b").Lex()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantKinds := []sysml.Kind{sysml.Identifier, sysml.Dot, sysml.Identifier}
	wantValues := []string{"a", ".", "b"}

	if len(tokens) != len(wantKinds) {
		t.Fatalf("got %d tokens (%v), want %d", len(tokens), tokens, len(wantKinds))
	}
	for i, tok := range tokens {
		if tok.Kind != wantKinds[i] || string(tok.Value) != wantValues[i] {
			t.Errorf("token %d: got {%v %q}, want {%v %q}", i, tok.Kind, string(tok.Value), wantKinds[i], wantValues[i])
		}
	}
}
