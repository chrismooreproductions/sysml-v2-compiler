package sysml

type Kind int

const (
	Pkg Kind = iota
	Part
	Attribute
	Item
	Port
	ConstraintKw
	CalcKw
	ConnectionKw
	InterfaceKw
	ConnectKw
	ToKw
	MetadataKw
	RequirementKw
	ConcernKw
	CaseKw
	SubjectKw
	AssumeKw
	RequireKw
	FrameKw
	ActorKw
	StakeholderKw
	SatisfyKw
	AssertKw
	ByKw
	EndKw
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
	Dot
	DotDot
	Equals
	Subsets
	Redefines
	References
	Plus
	Minus
	Slash
	Percent
	Power
	Ampersand
	Pipe
	Lt
	Gt
	Le
	Ge
	Eq
	NotEq
	OpenParen
	CloseParen
	Comma
	Hash
	TrueKw
	FalseKw
	NotKw
	XorKw
	Space
	Comment
	StringLit
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
	case ConstraintKw:
		return "'constraint'"
	case CalcKw:
		return "'calc'"
	case ConnectionKw:
		return "'connection'"
	case InterfaceKw:
		return "'interface'"
	case ConnectKw:
		return "'connect'"
	case ToKw:
		return "'to'"
	case MetadataKw:
		return "'metadata'"
	case RequirementKw:
		return "'requirement'"
	case ConcernKw:
		return "'concern'"
	case CaseKw:
		return "'case'"
	case SubjectKw:
		return "'subject'"
	case AssumeKw:
		return "'assume'"
	case RequireKw:
		return "'require'"
	case FrameKw:
		return "'frame'"
	case ActorKw:
		return "'actor'"
	case StakeholderKw:
		return "'stakeholder'"
	case SatisfyKw:
		return "'satisfy'"
	case AssertKw:
		return "'assert'"
	case ByKw:
		return "'by'"
	case EndKw:
		return "'end'"
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
	case Dot:
		return "'.'"
	case DotDot:
		return "'..'"
	case Equals:
		return "'='"
	case Subsets:
		return "'subsets'"
	case Redefines:
		return "'redefines'"
	case References:
		return "'references'"
	case Plus:
		return "'+'"
	case Minus:
		return "'-'"
	case Slash:
		return "'/'"
	case Percent:
		return "'%'"
	case Power:
		return "'**'"
	case Ampersand:
		return "'&'"
	case Pipe:
		return "'|'"
	case Lt:
		return "'<'"
	case Gt:
		return "'>'"
	case Le:
		return "'<='"
	case Ge:
		return "'>='"
	case Eq:
		return "'=='"
	case NotEq:
		return "'!='"
	case OpenParen:
		return "'('"
	case CloseParen:
		return "')'"
	case Comma:
		return "','"
	case Hash:
		return "'#'"
	case TrueKw:
		return "'true'"
	case FalseKw:
		return "'false'"
	case NotKw:
		return "'not'"
	case XorKw:
		return "'xor'"
	case Space:
		return "space"
	case Comment:
		return "comment"
	case StringLit:
		return "string literal"
	case Identifier:
		return "identifier"
	default:
		return "unknown"
	}
}

var kinds = map[string]Kind{
	"package":     Pkg,
	"part":        Part,
	"attribute":   Attribute,
	"item":        Item,
	"port":        Port,
	"constraint":  ConstraintKw,
	"calc":        CalcKw,
	"connection":  ConnectionKw,
	"interface":   InterfaceKw,
	"connect":     ConnectKw,
	"to":          ToKw,
	"metadata":    MetadataKw,
	"@":           MetadataKw,
	"requirement": RequirementKw,
	"concern":     ConcernKw,
	"case":        CaseKw,
	"subject":     SubjectKw,
	"assume":      AssumeKw,
	"require":     RequireKw,
	"frame":       FrameKw,
	"actor":       ActorKw,
	"stakeholder": StakeholderKw,
	"satisfy":     SatisfyKw,
	"assert":      AssertKw,
	"by":          ByKw,
	"end":         EndKw,
	"def":         Def,
	"import":      ImportKw,
	"public":      PublicKw,
	"private":     PrivateKw,
	"protected":   ProtectedKw,
	";":           Semicolon,
	"{":           OpenBrace,
	"}":           CloseBrace,
	":":           Colon,
	"::":          PathSep,
	"[":           OpenBracket,
	"]":           CloseBracket,
	"*":           Star,
	".":           Dot,
	"..":          DotDot,
	"=":           Equals,
	"subsets":     Subsets,
	"redefines":   Redefines,
	"references":  References,
	":>":          Subsets,
	":>>":         Redefines,
	"::>":         References,
	"+":           Plus,
	"-":           Minus,
	"/":           Slash,
	"%":           Percent,
	"**":          Power,
	"&":           Ampersand,
	"|":           Pipe,
	"<":           Lt,
	">":           Gt,
	"<=":          Le,
	">=":          Ge,
	"==":          Eq,
	"!=":          NotEq,
	"(":           OpenParen,
	",":           Comma,
	"#":           Hash,
	")":           CloseParen,
	"true":        TrueKw,
	"false":       FalseKw,
	"not":         NotKw,
	"xor":         XorKw,
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
