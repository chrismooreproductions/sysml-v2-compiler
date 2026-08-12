package modeller

import (
	"unicode/utf8"
)

type lexer struct {
	source string
	pos    Pos
}

func newLexer(source string) *lexer {
	return &lexer{source: source, pos: Pos{Offset: 0, Line: 1, Col: 1}}
}

func (l *lexer) current() (rune, int) {
	if l.pos.Offset >= len(l.source) {
		return 0, 0
	}

	r, size := utf8.DecodeRuneInString(l.source[l.pos.Offset:])
	return r, size
}

func (l *lexer) advance() (rune, int) {
	r, size := l.current()

	if size > 0 {
		l.pos.Offset += size
		l.pos.Col++
	}

	return r, size
}

// scanWhile consumes and returns runes from the current position for as
// long as pred holds, stopping at EOF without consuming past it.
func (l *lexer) scanWhile(pred func(rune) bool) []rune {
	var value []rune
	for {
		r, size := l.current()
		if size == 0 || !pred(r) {
			break
		}
		l.advance()
		value = append(value, r)
	}
	return value
}

const (
	spaceRune = rune(' ')
	tabRune   = rune('\t')
	carriage  = rune('\r')
	newline   = rune('\n')
)

func isSpace(r rune) bool {
	return r == spaceRune || r == tabRune || r == carriage
}

func isPunct(r rune) bool {
	switch r {
	case ';', '{', '}', ':':
		return true
	default:
		return false
	}
}

func isIdentRune(r rune) bool {
	return r != newline && !isSpace(r) && !isPunct(r)
}

// next scans and returns the next token from the source, skipping newlines
// (which only affect line/col bookkeeping). ok is false once the source is
// exhausted.
func (l *lexer) next() (Token, bool) {
	for {
		if l.pos.Offset >= len(l.source) {
			return Token{}, false
		}

		start := l.pos
		r, _ := l.current()

		switch {
		case r == newline:
			l.advance()
			l.pos.Line++
			l.pos.Col = 1

		case isSpace(r):
			value := l.scanWhile(isSpace)
			return Token{Kind: Space, Value: value, Pos: start}, true

		case isPunct(r):
			l.advance()
			return word([]rune{r}, start), true

		default:
			value := l.scanWhile(isIdentRune)
			return word(value, start), true
		}
	}
}
