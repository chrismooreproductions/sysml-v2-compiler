package sysml

type Kind int

const (
	Pkg Kind = iota
	Part
	Attribute
	Item
	Port
	Def
	ImportKw
	PublicKw
	PrivateKw
	ProtectedKw
	Semicolon
	OpenBrace
	CloseBrace
	Colon
	PathSep
	OpenBracket
	CloseBracket
	Star
	DotDot
	Equals
	Space
	Comment
	Identifier
)

func (k Kind) String() string {
	switch k {
	case Pkg:
		return "'package'"
	case Part:
		return "'part'"
	case Attribute:
		return "'attribute'"
	case Item:
		return "'item'"
	case Port:
		return "'port'"
	case Def:
		return "'def'"
	case ImportKw:
		return "'import'"
	case PublicKw:
		return "'public'"
	case PrivateKw:
		return "'private'"
	case ProtectedKw:
		return "'protected'"
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
	case OpenBracket:
		return "'['"
	case CloseBracket:
		return "']'"
	case Star:
		return "'*'"
	case DotDot:
		return "'..'"
	case Equals:
		return "'='"
	case Space:
		return "space"
	case Comment:
		return "comment"
	case Identifier:
		return "identifier"
	default:
		return "unknown"
	}
}

var kinds = map[string]Kind{
	"package":   Pkg,
	"part":      Part,
	"attribute": Attribute,
	"item":      Item,
	"port":      Port,
	"def":       Def,
	"import":    ImportKw,
	"public":    PublicKw,
	"private":   PrivateKw,
	"protected": ProtectedKw,
	";":         Semicolon,
	"{":         OpenBrace,
	"}":         CloseBrace,
	":":         Colon,
	"::":        PathSep,
	"[":         OpenBracket,
	"]":         CloseBracket,
	"*":         Star,
	"..":        DotDot,
	"=":         Equals,
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
