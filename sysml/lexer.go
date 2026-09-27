package sysml

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

// peek returns the rune immediately after the current one, without
// consuming either. size is 0 if there is no such rune (current is the
// last rune in the source, or the source is exhausted).
func (l *lexer) peek() (rune, int) {
	_, size := l.current()
	if size == 0 || l.pos.Offset+size >= len(l.source) {
		return 0, 0
	}
	r, sz := utf8.DecodeRuneInString(l.source[l.pos.Offset+size:])
	return r, sz
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
	case ';', '{', '}', ':', '[', ']', '*', '.', '=':
		return true
	default:
		return false
	}
}

// doubled is the set of punctuation runes that pair with themselves to form
// a two-character token ("..") rather than standing alone. ':' is handled
// separately by scanColon, since its own multi-character forms ("::", ":>",
// ":>>") don't just double the same rune.
var doubled = map[rune]bool{'.': true}

func isIdentRune(r rune) bool {
	return r != newline && !isSpace(r) && !isPunct(r)
}

// scanIdentifier scans an identifier (or keyword/punctuation resolved
// through word) starting at the current position.
func (l *lexer) scanIdentifier(start Pos) Token {
	return word(l.scanWhile(isIdentRune), start)
}

// scanColon scans a leading ':' through however many of its multi-character
// forms follow: "::" (PathSep), ":>" (Subsets), or ":>>" (Redefines) --
// falling back to a bare ':' (Colon) if none do. '>' is deliberately not
// punctuation in its own right (isPunct), so this is the only place it's
// ever meaningful; elsewhere it's just an ordinary identifier rune.
func (l *lexer) scanColon(start Pos) Token {
	l.advance()

	if next, size := l.current(); size > 0 && next == ':' {
		l.advance()
		return word([]rune{':', ':'}, start)
	}

	if next, size := l.current(); size > 0 && next == '>' {
		l.advance()
		if next2, size2 := l.current(); size2 > 0 && next2 == '>' {
			l.advance()
			return word([]rune{':', '>', '>'}, start)
		}
		return word([]rune{':', '>'}, start)
	}

	return word([]rune{':'}, start)
}

// scanLineComment scans a "//"-style comment through end of line, leaving
// the newline itself unconsumed (matching how a Space token never crosses
// one either).
func (l *lexer) scanLineComment(start Pos) Token {
	value := []rune{'/', '/'}
	l.advance()
	l.advance()
	value = append(value, l.scanWhile(func(r rune) bool { return r != newline })...)
	return Token{Kind: Comment, Value: value, Pos: start}
}

// scanBlockComment scans a "/* ... */"-style comment, which (unlike every
// other token) can itself span multiple lines -- so, unlike scanWhile, it
// tracks Line/Col by hand as it crosses each newline. An unterminated
// comment runs to EOF rather than erroring: the lexer has no error path
// today (see Model.Lex), so this stays consistent with how it treats every
// other malformed input.
func (l *lexer) scanBlockComment(start Pos) Token {
	value := []rune{'/', '*'}
	l.advance()
	l.advance()

	for {
		r, size := l.current()
		if size == 0 {
			break
		}

		if r == newline {
			l.advance()
			l.pos.Line++
			l.pos.Col = 1
			value = append(value, r)
			continue
		}

		l.advance()
		value = append(value, r)

		if r == '*' {
			if next, size := l.current(); size > 0 && next == '/' {
				l.advance()
				value = append(value, next)
				break
			}
		}
	}

	return Token{Kind: Comment, Value: value, Pos: start}
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
		next, nextSize := l.peek()

		switch {
		case r == newline:
			l.advance()
			l.pos.Line++
			l.pos.Col = 1

		case isSpace(r):
			value := l.scanWhile(isSpace)
			return Token{Kind: Space, Value: value, Pos: start}, true

		case r == '/' && nextSize > 0 && next == '/':
			return l.scanLineComment(start), true

		case r == '/' && nextSize > 0 && next == '*':
			return l.scanBlockComment(start), true

		case r == ':':
			return l.scanColon(start), true

		case isPunct(r):
			l.advance()
			if doubled[r] {
				if next, size := l.current(); size > 0 && next == r {
					l.advance()
					return word([]rune{r, r}, start), true
				}
			}
			return word([]rune{r}, start), true

		default:
			return l.scanIdentifier(start), true
		}
	}
}
