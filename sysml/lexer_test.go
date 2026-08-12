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
