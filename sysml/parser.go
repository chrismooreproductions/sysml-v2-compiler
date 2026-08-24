package sysml

import (
	"fmt"
	"strconv"
)

type parser struct {
	tokens []Token
	pos    int
}

// newParser drops whitespace tokens up front: the parser only cares about
// structure, never layout.
func newParser(tokens []Token) *parser {
	filtered := make([]Token, 0, len(tokens))
	for _, tok := range tokens {
		if tok.Kind != Space {
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

func (p *parser) parseMember() (Member, error) {
	tok, ok := p.current()
	if !ok {
		return nil, fmt.Errorf("unexpected end of input, want %s, %s, or %s", Pkg, ImportKw, Part)
	}

	if tok.Kind == Pkg {
		return p.parsePackage()
	}

	if tok.Kind == ImportKw {
		return p.parseImport()
	}

	if _, err := p.expect(Part); err != nil {
		return nil, err
	}

	tok, ok = p.current()
	if !ok {
		return nil, fmt.Errorf("unexpected end of input after %s", Part)
	}

	if tok.Kind == Def {
		p.pos++
		return p.parsePartDef()
	}

	return p.parsePartUsage()
}

func (p *parser) parsePartDef() (*PartDef, error) {
	name, err := p.expect(Identifier)
	if err != nil {
		return nil, err
	}

	tok, ok := p.current()
	if !ok {
		return nil, fmt.Errorf("unexpected end of input after part def %q, want %s or %s", string(name.Value), Semicolon, OpenBrace)
	}

	switch tok.Kind {
	case Semicolon:
		p.pos++
		return &PartDef{Name: string(name.Value)}, nil

	case OpenBrace:
		p.pos++
		members, err := p.parseMembers()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(CloseBrace); err != nil {
			return nil, err
		}
		return &PartDef{Name: string(name.Value), Members: members}, nil

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

func (p *parser) parsePartUsage() (*PartUsage, error) {
	name, err := p.expect(Identifier)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(Colon); err != nil {
		return nil, err
	}
	typ, err := p.parseQualifiedName()
	if err != nil {
		return nil, err
	}

	var mult *Multiplicity
	if tok, ok := p.current(); ok && tok.Kind == OpenBracket {
		mult, err = p.parseMultiplicity()
		if err != nil {
			return nil, err
		}
	}

	if _, err := p.expect(Semicolon); err != nil {
		return nil, err
	}
	return &PartUsage{Name: string(name.Value), Type: typ, Multiplicity: mult}, nil
}

// parseMultiplicity parses a bracketed multiplicity clause following a part
// usage's type: "[*]" (0..Unbounded), "[3]" (Lower == Upper == 3), or a
// "lower..upper" range where either side may itself be "*".
func (p *parser) parseMultiplicity() (*Multiplicity, error) {
	if _, err := p.expect(OpenBracket); err != nil {
		return nil, err
	}

	if tok, ok := p.current(); ok && tok.Kind == Star {
		p.pos++
		if _, err := p.expect(CloseBracket); err != nil {
			return nil, err
		}
		return &Multiplicity{Lower: 0, Upper: Unbounded}, nil
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

// parseBound parses one side of a multiplicity range: either "*"
// (Unbounded) or a non-negative integer literal.
func (p *parser) parseBound() (int, error) {
	tok, ok := p.current()
	if !ok {
		return 0, fmt.Errorf("unexpected end of input in multiplicity, want a bound or %s", Star)
	}

	if tok.Kind == Star {
		p.pos++
		return Unbounded, nil
	}

	if tok.Kind != Identifier {
		return 0, fmt.Errorf("line %d: unexpected %s, want a multiplicity bound", tok.Pos.Line, describeToken(tok))
	}

	n, err := strconv.Atoi(string(tok.Value))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("line %d: invalid multiplicity bound %q", tok.Pos.Line, string(tok.Value))
	}
	p.pos++
	return n, nil
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
