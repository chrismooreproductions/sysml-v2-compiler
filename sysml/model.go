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

// Parse parses the model's source as a root Namespace: zero or more
// top-level members (packages, definitions, usages, imports), matching the
// grammar's RootNamespace rather than requiring a single wrapping package.
func (m Model) Parse() (*Namespace, error) {
	tokens, err := m.Lex()
	if err != nil {
		return nil, err
	}

	p := newParser(tokens)

	members, err := p.parseMembers()
	if err != nil {
		return nil, err
	}

	if tok, ok := p.current(); ok {
		return nil, fmt.Errorf("line %d: unexpected %s", tok.Pos.Line, describeToken(tok))
	}

	return &Namespace{Members: members}, nil
}
