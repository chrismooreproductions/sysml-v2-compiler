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
	case ';', '{', '}', ':', '[', ']', '*', '.', '=',
		'+', '-', '/', '%', '&', '|', '(', ')', ',', '#', '@':
		return true
	default:
		return false
	}
}

// doubled is the set of punctuation runes that pair with themselves to form
// a two-character token ("..", "**", "==") rather than standing alone.
// ':' is handled separately by scanColon, since its own multi-character
// forms ("::", ":>", ":>>") don't just double the same rune; '<', '>', and
// '!' are handled separately too, since their two-character forms ("<=",
// ">=", "!=") pair with a *different* rune ('=').
var doubled = map[rune]bool{'.': true, '*': true, '=': true}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isIdentRune(r rune) bool {
	return r != newline && !isSpace(r) && !isPunct(r)
}

// scanIdentifier scans an identifier (or keyword/punctuation resolved
// through word) starting at the current position.
func (l *lexer) scanIdentifier(start Pos) Token {
	return word(l.scanWhile(isIdentRune), start)
}

// scanColon scans a leading ':' through however many of its multi-character
// forms follow: "::" (PathSep), "::>" (References), ":>" (Subsets), or
// ":>>" (Redefines) -- falling back to a bare ':' (Colon) if none do. '>' is
// deliberately not punctuation in its own right (isPunct), so this is the
// only place it's ever meaningful; elsewhere it's just an ordinary
// identifier rune.
func (l *lexer) scanColon(start Pos) Token {
	l.advance()

	if next, size := l.current(); size > 0 && next == ':' {
		l.advance()
		if next2, size2 := l.current(); size2 > 0 && next2 == '>' {
			l.advance()
			return word([]rune{':', ':', '>'}, start)
		}
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

// scanBang scans a leading '!' as the start of "!=" (NotEq) if followed by
// '=', falling back to an ordinary identifier otherwise -- there's no
// standalone '!' operator in this project's expression subset, so a lone
// '!' is just whatever identifier-shaped text follows it, the same as any
// other unrecognized rune would be.
func (l *lexer) scanBang(start Pos) Token {
	if next, size := l.peek(); size > 0 && next == '=' {
		l.advance()
		l.advance()
		return word([]rune{'!', '='}, start)
	}
	return l.scanIdentifier(start)
}

// scanLessThan scans a leading '<' through its one two-character form,
// "<=" (Le), falling back to a bare '<' (Lt).
func (l *lexer) scanLessThan(start Pos) Token {
	l.advance()
	if next, size := l.current(); size > 0 && next == '=' {
		l.advance()
		return word([]rune{'<', '='}, start)
	}
	return word([]rune{'<'}, start)
}

// scanGreaterThan is scanLessThan's mirror for '>': "=>" isn't a thing
// here, but ">=" (Ge) is, falling back to a bare '>' (Gt). A '>' reaching
// this dispatch was never part of ":>"/":>>" -- scanColon already
// consumed those greedily when the preceding ':' was seen.
func (l *lexer) scanGreaterThan(start Pos) Token {
	l.advance()
	if next, size := l.current(); size > 0 && next == '=' {
		l.advance()
		return word([]rune{'>', '='}, start)
	}
	return word([]rune{'>'}, start)
}

// scanString scans a double-quoted string literal, e.g. `"hello"`. No
// escape sequences are supported -- nothing in scope needs them yet. The
// token's Value includes both quotes (parsePrimary strips them), matching
// how a Comment token's Value includes its own "//"/"/*"/"*/" markers. An
// unterminated string runs to EOF, the same as an unterminated block
// comment does (see scanBlockComment) -- the lexer has no error path.
func (l *lexer) scanString(start Pos) Token {
	value := []rune{'"'}
	l.advance()

	for {
		r, size := l.current()
		if size == 0 || r == '"' {
			break
		}
		l.advance()
		value = append(value, r)
	}

	if r, size := l.current(); size > 0 && r == '"' {
		l.advance()
		value = append(value, r)
	}

	return Token{Kind: StringLit, Value: value, Pos: start}
}

// scanRestrictedName scans a single-quoted restricted name (e.g. 'User
// Defined Extensions'), which can contain characters an ordinary
// identifier can't (spaces, dots, a leading digit, ...). Unlike
// scanString, the result is a plain Identifier token -- not a new kind --
// whose Value is the *unquoted* inner text, so every consumer of an
// Identifier (qualified names, declaration names, short names) handles it
// exactly like a bare name, with no further unquoting step; StringLit
// keeps its quotes instead, for parsePrimary's own unquoteString, since a
// string literal is a value, not a name. An unterminated restricted name
// runs to EOF, the same as an unterminated string does.
func (l *lexer) scanRestrictedName(start Pos) Token {
	l.advance()

	var value []rune
	for {
		r, size := l.current()
		if size == 0 || r == '\'' {
			break
		}
		l.advance()
		value = append(value, r)
	}

	if r, size := l.current(); size > 0 && r == '\'' {
		l.advance()
	}

	return Token{Kind: Identifier, Value: value, Pos: start}
}

// scanNumber scans a numeric literal: a run of digits, optionally followed
// by '.' and more digits (a real literal), e.g. "5" or "3.14". Kept as an
// ordinary Identifier-kind token, like any other bare word -- parsePrimary
// and parseBound already disambiguate an Identifier token's shape via
// strconv, so this doesn't need its own Kind. It exists only so a real
// literal's '.' isn't mistaken for a feature-chain separator: '.' needs to
// stay its own token everywhere else, so scanning "3.14" as one token
// needs this lookahead rather than leaving '.' to the generic isPunct
// path, which would split it into "3", '.', "14".
func (l *lexer) scanNumber(start Pos) Token {
	value := l.scanWhile(isDigit)

	if next, size := l.current(); size > 0 && next == '.' {
		if after, asize := l.peek(); asize > 0 && isDigit(after) {
			r, _ := l.advance()
			value = append(value, r)
			value = append(value, l.scanWhile(isDigit)...)
		}
	}

	return word(value, start)
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

		case r == '!':
			return l.scanBang(start), true

		case r == '<':
			return l.scanLessThan(start), true

		case r == '>':
			return l.scanGreaterThan(start), true

		case r == '"':
			return l.scanString(start), true

		case r == '\'':
			return l.scanRestrictedName(start), true

		case isDigit(r):
			return l.scanNumber(start), true

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
