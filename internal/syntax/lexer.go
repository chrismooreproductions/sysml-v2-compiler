// SPDX-License-Identifier: EPL-2.0

// Package syntax parses SysML v2 and KerML textual notation by interpreting
// the Xtext grammars loaded by the grammar package. It produces a generic
// tree of nodes (a metaclass name and ordered properties) with source
// positions; resolving names and adding the structure the specification
// implies are later stages.
package syntax

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
)

// Pos is a position in a source file. Line and Column are 1-based; Column
// counts bytes, so it matches Offset within a line.
type Pos struct {
	Offset int
	Line   int
	Column int
}

func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Column) }

// Span is a range of source text; End is exclusive.
type Span struct {
	Start, End Pos
}

// TokenKind classifies a token the way the grammar's lexical rules do.
type TokenKind uint8

const (
	// Keyword is a reserved word: any identifier-shaped literal the grammar uses.
	Keyword TokenKind = iota + 1
	// Symbol is punctuation or an operator.
	Symbol
	// Name is a basic name (the grammar's ID rule).
	Name
	// QuotedName is an unrestricted name in single quotes (UNRESTRICTED_NAME).
	QuotedName
	// Decimal is a run of digits (DECIMAL_VALUE).
	Decimal
	// Exponent is a number with an exponent, such as 1e5 (EXP_VALUE).
	Exponent
	// String is a double-quoted string (STRING_VALUE).
	String
	// Comment is a regular comment, /* ... */ (REGULAR_COMMENT). It is the body
	// of comment and documentation elements; elsewhere the parser skips it.
	Comment
)

func (k TokenKind) String() string {
	return [...]string{"", "keyword", "symbol", "name", "quoted name", "number", "number", "string", "comment"}[k]
}

// Token is a lexical unit with its source text.
type Token struct {
	Kind TokenKind
	Text string
	Span Span
}

// Error is a lexical or syntax error at a position. For a syntax error, Got
// and Expected say what was found and what could have continued the input.
type Error struct {
	File string
	Pos  Pos
	Msg  string

	Got      string   // the unexpected token, or "" at the end of the input
	Expected []string // sorted: kinds of token ("a name") first, then literals
}

func (e *Error) Error() string {
	if e.File != "" {
		return fmt.Sprintf("%s:%s: %s", e.File, e.Pos, e.Msg)
	}
	return fmt.Sprintf("%s: %s", e.Pos, e.Msg)
}

// lexer turns source text into tokens using the keywords and symbols of a
// grammar.
type lexer struct {
	keywords map[string]bool
	symbols  []string // longest first, so the first match is the longest
}

func newLexer(g *grammar.Grammar) *lexer {
	l := &lexer{keywords: g.Keywords, symbols: append([]string{}, g.Symbols...)}
	sort.SliceStable(l.symbols, func(i, j int) bool { return len(l.symbols[i]) > len(l.symbols[j]) })
	return l
}

func isIdentStart(c byte) bool {
	return c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// lex tokenizes src. Whitespace and the notes // ... and //* ... */ are
// skipped; regular comments are kept as Comment tokens.
func (l *lexer) lex(file, src string) ([]Token, error) {
	var toks []Token
	line, lineStart := 1, 0
	pos := func(off int) Pos { return Pos{Offset: off, Line: line, Column: off - lineStart + 1} }
	fail := func(off int, format string, a ...any) error {
		return &Error{File: file, Pos: pos(off), Msg: fmt.Sprintf(format, a...)}
	}
	// advance moves over src[i:j], tracking lines.
	advance := func(i, j int) {
		for k := i; k < j; k++ {
			if src[k] == '\n' {
				line++
				lineStart = k + 1
			}
		}
	}
	emit := func(kind TokenKind, start, end int, startPos Pos) {
		toks = append(toks, Token{kind, src[start:end], Span{startPos, pos(end)}})
	}

	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\f':
			i++
		case c == '\n':
			advance(i, i+1)
			i++
		case strings.HasPrefix(src[i:], "//*"):
			j := strings.Index(src[i+3:], "*/")
			if j < 0 {
				return nil, fail(i, "unterminated note")
			}
			end := i + 3 + j + 2
			advance(i, end)
			i = end
		case strings.HasPrefix(src[i:], "//"):
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case strings.HasPrefix(src[i:], "/*"):
			j := strings.Index(src[i+2:], "*/")
			if j < 0 {
				return nil, fail(i, "unterminated comment")
			}
			end := i + 2 + j + 2
			start := pos(i)
			advance(i, end)
			toks = append(toks, Token{Comment, src[i:end], Span{start, pos(end)}})
			i = end
		case c == '"':
			j := i + 1
			for j < len(src) && src[j] != '"' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(src) {
				return nil, fail(i, "unterminated string")
			}
			start := pos(i)
			advance(i, j+1)
			emit(String, i, j+1, start)
			i = j + 1
		case c == '\'':
			j := i + 1
			for j < len(src) && src[j] != '\'' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(src) {
				return nil, fail(i, "unterminated quoted name")
			}
			start := pos(i)
			advance(i, j+1)
			emit(QuotedName, i, j+1, start)
			i = j + 1
		case isDigit(c):
			j := i
			for j < len(src) && isDigit(src[j]) {
				j++
			}
			kind := Decimal
			if j < len(src) && (src[j] == 'e' || src[j] == 'E') {
				k := j + 1
				if k < len(src) && (src[k] == '+' || src[k] == '-') {
					k++
				}
				if k < len(src) && isDigit(src[k]) {
					for k < len(src) && isDigit(src[k]) {
						k++
					}
					j, kind = k, Exponent
				}
			}
			emit(kind, i, j, pos(i))
			i = j
		case isIdentStart(c):
			j := i + 1
			for j < len(src) && (isIdentStart(src[j]) || isDigit(src[j])) {
				j++
			}
			kind := Name
			if l.keywords[src[i:j]] {
				kind = Keyword
			}
			emit(kind, i, j, pos(i))
			i = j
		default:
			matched := ""
			for _, s := range l.symbols {
				if strings.HasPrefix(src[i:], s) {
					matched = s
					break
				}
			}
			if matched == "" {
				return nil, fail(i, "unexpected character %q", rune(c))
			}
			emit(Symbol, i, i+len(matched), pos(i))
			i += len(matched)
		}
	}
	return toks, nil
}
