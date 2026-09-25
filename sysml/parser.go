package sysml

import (
	"fmt"
	"strconv"
)

type parser struct {
	tokens []Token
	pos    int
}

// newParser drops whitespace and comment tokens up front: the parser only
// cares about structure, never layout or documentation.
func newParser(tokens []Token) *parser {
	filtered := make([]Token, 0, len(tokens))
	for _, tok := range tokens {
		if tok.Kind != Space && tok.Kind != Comment {
			filtered = append(filtered, tok)
		}
	}
	return &parser{tokens: filtered}
}

func (p *parser) current() (Token, bool) {
	if p.pos >= len(p.tokens) {
		return Token{}, false
	}
	return p.tokens[p.pos], true
}

func (p *parser) expect(k Kind) (Token, error) {
	tok, ok := p.current()
	if !ok {
		return Token{}, fmt.Errorf("unexpected end of input, want %s", k)
	}
	if tok.Kind != k {
		return Token{}, fmt.Errorf("line %d: unexpected %s, want %s", tok.Pos.Line, describeToken(tok), k)
	}
	p.pos++
	return tok, nil
}

func describeToken(tok Token) string {
	if tok.Kind == Identifier {
		return fmt.Sprintf("identifier %q", string(tok.Value))
	}
	return tok.Kind.String()
}

func (p *parser) parsePackage() (*Package, error) {
	if _, err := p.expect(Pkg); err != nil {
		return nil, err
	}

	name, err := p.expect(Identifier)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(OpenBrace); err != nil {
		return nil, err
	}

	members, err := p.parseMembers()
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(CloseBrace); err != nil {
		return nil, err
	}

	return &Package{Name: string(name.Value), Members: members}, nil
}

// parseMembers parses members until it sees a closing brace or runs out of
// tokens; the caller is responsible for consuming the closing brace itself.
func (p *parser) parseMembers() ([]Member, error) {
	var members []Member
	for {
		tok, ok := p.current()
		if !ok || tok.Kind == CloseBrace {
			return members, nil
		}

		member, err := p.parseMember()
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
}

// defKeywords maps each definition/usage keyword token to the DefKind it
// introduces -- the dispatch table parseMember consults after ruling out
// package and import members.
var defKeywords = map[Kind]DefKind{
	Part:      DefPart,
	Attribute: DefAttribute,
	Item:      DefItem,
	Port:      DefPort,
}

// visibilityKeywords maps each visibility keyword token to the Visibility
// it introduces, for parseMember's optional leading prefix.
var visibilityKeywords = map[Kind]Visibility{
	PublicKw:    VisibilityPublic,
	PrivateKw:   VisibilityPrivate,
	ProtectedKw: VisibilityProtected,
}

// parseVisibility consumes a leading 'public'/'private'/'protected'
// keyword if the next token is one, returning the Visibility it names.
// Consumes nothing and returns VisibilityUnspecified otherwise.
func (p *parser) parseVisibility() Visibility {
	tok, ok := p.current()
	if !ok {
		return VisibilityUnspecified
	}
	vis, ok := visibilityKeywords[tok.Kind]
	if !ok {
		return VisibilityUnspecified
	}
	p.pos++
	return vis
}

func (p *parser) parseMember() (Member, error) {
	vis := p.parseVisibility()

	tok, ok := p.current()
	if !ok {
		return nil, fmt.Errorf("unexpected end of input, want %s, %s, or %s", Pkg, ImportKw, Part)
	}

	if tok.Kind == Pkg {
		pkg, err := p.parsePackage()
		if err != nil {
			return nil, err
		}
		pkg.Visibility = vis
		return pkg, nil
	}

	if tok.Kind == ImportKw {
		imp, err := p.parseImport()
		if err != nil {
			return nil, err
		}
		imp.Visibility = vis
		return imp, nil
	}

	defKind, ok := defKeywords[tok.Kind]
	if !ok {
		return nil, fmt.Errorf("line %d: unexpected %s, want %s, %s, or %s", tok.Pos.Line, describeToken(tok), Pkg, ImportKw, Part)
	}
	keyword := tok.Kind
	p.pos++

	tok, ok = p.current()
	if !ok {
		return nil, fmt.Errorf("unexpected end of input after %s", keyword)
	}

	if tok.Kind == Def {
		p.pos++
		def, err := p.parseDefinition(defKind)
		if err != nil {
			return nil, err
		}
		def.Visibility = vis
		return def, nil
	}

	usage, err := p.parseUsage(defKind)
	if err != nil {
		return nil, err
	}
	usage.Visibility = vis
	return usage, nil
}

func (p *parser) parseDefinition(kind DefKind) (*Definition, error) {
	name, err := p.expect(Identifier)
	if err != nil {
		return nil, err
	}

	tok, ok := p.current()
	if !ok {
		return nil, fmt.Errorf("unexpected end of input after %s def %q, want %s or %s", kind, string(name.Value), Semicolon, OpenBrace)
	}

	switch tok.Kind {
	case Semicolon:
		p.pos++
		return &Definition{Kind: kind, Name: string(name.Value)}, nil

	case OpenBrace:
		p.pos++
		members, err := p.parseMembers()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(CloseBrace); err != nil {
			return nil, err
		}
		return &Definition{Kind: kind, Name: string(name.Value), Members: members}, nil

	default:
		return nil, fmt.Errorf("line %d: unexpected %s, want %s or %s", tok.Pos.Line, describeToken(tok), Semicolon, OpenBrace)
	}
}

func (p *parser) parseImport() (*Import, error) {
	if _, err := p.expect(ImportKw); err != nil {
		return nil, err
	}

	path, err := p.parseQualifiedName()
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(Semicolon); err != nil {
		return nil, err
	}

	return &Import{Path: path}, nil
}

func (p *parser) parseUsage(kind DefKind) (*Usage, error) {
	name, err := p.expect(Identifier)
	if err != nil {
		return nil, err
	}

	usage := &Usage{Kind: kind, Name: string(name.Value)}

	if err := p.parseFeatureSpecializationPart(usage); err != nil {
		return nil, err
	}

	if err := p.parseValuePart(usage); err != nil {
		return nil, err
	}

	if _, err := p.expect(Semicolon); err != nil {
		return nil, err
	}
	return usage, nil
}

// parseFeatureSpecializationPart consumes a typing (": Type") and a
// multiplicity ("[...]") in whichever order they appear, each at most once,
// and either or both absent -- so "part b[0..2] : P;", "part c : P[2..*];",
// "port p;", and "part a[1];" all parse through the same loop.
func (p *parser) parseFeatureSpecializationPart(usage *Usage) error {
	for {
		tok, ok := p.current()
		if !ok {
			return nil
		}

		switch tok.Kind {
		case Colon:
			if usage.Type != "" {
				return fmt.Errorf("line %d: unexpected %s, a type was already given", tok.Pos.Line, Colon)
			}
			p.pos++
			typ, err := p.parseQualifiedName()
			if err != nil {
				return err
			}
			usage.Type = typ

		case OpenBracket:
			if usage.Multiplicity != nil {
				return fmt.Errorf("line %d: unexpected %s, a multiplicity was already given", tok.Pos.Line, OpenBracket)
			}
			mult, err := p.parseMultiplicity()
			if err != nil {
				return err
			}
			usage.Multiplicity = mult

		default:
			return nil
		}
	}
}

// parseValuePart consumes an optional assigned value, e.g. the "= 5" in
// `attribute n : ScalarValues::Integer = 5;`. Only SysML's plain '='
// FeatureValue form is supported (not ':=' or 'default'), and only integer
// literals, not full expressions. Consumes nothing if the next token isn't
// '='.
func (p *parser) parseValuePart(usage *Usage) error {
	tok, ok := p.current()
	if !ok || tok.Kind != Equals {
		return nil
	}
	p.pos++

	valTok, err := p.expect(Identifier)
	if err != nil {
		return err
	}
	n, err := strconv.Atoi(string(valTok.Value))
	if err != nil {
		return fmt.Errorf("line %d: unsupported value %q -- only integer literals are supported", valTok.Pos.Line, string(valTok.Value))
	}
	usage.Value = &n
	return nil
}

// parseMultiplicity parses a bracketed multiplicity clause: "[*]"
// (0..Unbounded), "[3]" (Lower == Upper == 3), or a "lower..upper" range
// where either side may be "*" or a name (see Bound's doc comment).
func (p *parser) parseMultiplicity() (*Multiplicity, error) {
	if _, err := p.expect(OpenBracket); err != nil {
		return nil, err
	}

	if tok, ok := p.current(); ok && tok.Kind == Star {
		p.pos++
		if _, err := p.expect(CloseBracket); err != nil {
			return nil, err
		}
		return &Multiplicity{Lower: Bound{Value: 0}, Upper: Bound{Value: Unbounded}}, nil
	}

	lower, err := p.parseBound()
	if err != nil {
		return nil, err
	}

	upper := lower
	if tok, ok := p.current(); ok && tok.Kind == DotDot {
		p.pos++
		if upper, err = p.parseBound(); err != nil {
			return nil, err
		}
	}

	if _, err := p.expect(CloseBracket); err != nil {
		return nil, err
	}
	return &Multiplicity{Lower: lower, Upper: upper}, nil
}

// parseBound parses one side of a multiplicity range: "*" (Unbounded), a
// non-negative integer literal, or -- anything else identifier-shaped -- a
// name reference, left unresolved (see Bound's doc comment).
func (p *parser) parseBound() (Bound, error) {
	tok, ok := p.current()
	if !ok {
		return Bound{}, fmt.Errorf("unexpected end of input in multiplicity, want a bound or %s", Star)
	}

	if tok.Kind == Star {
		p.pos++
		return Bound{Value: Unbounded}, nil
	}

	if tok.Kind != Identifier {
		return Bound{}, fmt.Errorf("line %d: unexpected %s, want a multiplicity bound", tok.Pos.Line, describeToken(tok))
	}
	p.pos++

	if n, err := strconv.Atoi(string(tok.Value)); err == nil {
		if n < 0 {
			return Bound{}, fmt.Errorf("line %d: invalid multiplicity bound %q", tok.Pos.Line, string(tok.Value))
		}
		return Bound{Value: n}, nil
	}

	return Bound{Name: string(tok.Value)}, nil
}

// parseQualifiedName parses an identifier, optionally followed by more
// '::'-separated identifiers (e.g. "Engine" or "Vehicle::Car::Wheel"),
// returning it as a single "::"-joined string.
func (p *parser) parseQualifiedName() (string, error) {
	first, err := p.expect(Identifier)
	if err != nil {
		return "", err
	}

	name := string(first.Value)
	for {
		tok, ok := p.current()
		if !ok || tok.Kind != PathSep {
			return name, nil
		}
		p.pos++

		next, err := p.expect(Identifier)
		if err != nil {
			return "", err
		}
		name += "::" + string(next.Value)
	}
}
