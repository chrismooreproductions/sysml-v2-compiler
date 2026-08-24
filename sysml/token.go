package sysml

type Kind int

const (
	Pkg Kind = iota
	Part
	Def
	ImportKw
	Semicolon
	OpenBrace
	CloseBrace
	Colon
	PathSep
	Space
	Identifier
)

func (k Kind) String() string {
	switch k {
	case Pkg:
		return "'package'"
	case Part:
		return "'part'"
	case Def:
		return "'def'"
	case ImportKw:
		return "'import'"
	case Semicolon:
		return "';'"
	case OpenBrace:
		return "'{'"
	case CloseBrace:
		return "'}'"
	case Colon:
		return "':'"
	case PathSep:
		return "'::'"
	case Space:
		return "space"
	case Identifier:
		return "identifier"
	default:
		return "unknown"
	}
}

var kinds = map[string]Kind{
	"package": Pkg,
	"part":    Part,
	"def":     Def,
	"import":  ImportKw,
	";":       Semicolon,
	"{":       OpenBrace,
	"}":       CloseBrace,
	":":       Colon,
	"::":      PathSep,
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
