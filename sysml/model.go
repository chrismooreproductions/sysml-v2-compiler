package sysml

import "fmt"

type Model struct {
	source string
}

func NewModel(source string) *Model {
	return &Model{source}
}

func (m Model) Lex() ([]Token, error) {
	l := newLexer(m.source)

	var tokens []Token
	for {
		tok, ok := l.next()
		if !ok {
			break
		}
		tokens = append(tokens, tok)
	}

	return tokens, nil
}

func (m Model) Parse() (*Package, error) {
	tokens, err := m.Lex()
	if err != nil {
		return nil, err
	}

	p := newParser(tokens)

	pkg, err := p.parsePackage()
	if err != nil {
		return nil, err
	}

	if tok, ok := p.current(); ok {
		return nil, fmt.Errorf("line %d: unexpected %s after package", tok.Pos.Line, describeToken(tok))
	}

	return pkg, nil
}
