package modeller

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
