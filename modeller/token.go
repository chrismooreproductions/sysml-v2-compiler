package modeller

type Kind int

const (
	Package Kind = iota
	Part
	Def
	Semicolon
	OpenBrace
	CloseBrace
	Colon
	Space
	Identifier
)

var kinds = map[string]Kind{
	"package": Package,
	"part":    Part,
	"def":     Def,
	";":       Semicolon,
	"{":       OpenBrace,
	"}":       CloseBrace,
	":":       Colon,
}

type Pos struct {
	Offset int
	Line   int
	Col    int
}

type Token struct {
	Kind  Kind
	Value []rune
	Pos   Pos
}

// word builds a token whose kind is resolved through the keyword/punctuation
// table, falling back to identifier for anything not in it.
func word(value []rune, start Pos) Token {
	k := Identifier
	if kk, ok := kinds[string(value)]; ok {
		k = kk
	}
	return Token{Kind: k, Value: value, Pos: start}
}
