package modeller

import "fmt"

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
	if _, err := p.expect(Part); err != nil {
		return nil, err
	}

	tok, ok := p.current()
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

func (p *parser) parsePartUsage() (*PartUsage, error) {
	name, err := p.expect(Identifier)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(Colon); err != nil {
		return nil, err
	}
	typ, err := p.expect(Identifier)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(Semicolon); err != nil {
		return nil, err
	}
	return &PartUsage{Name: string(name.Value), Type: string(typ.Value)}, nil
}
