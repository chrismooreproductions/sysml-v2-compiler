// SPDX-License-Identifier: EPL-2.0

package syntax

import (
	"errors"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
)

// maxErrors bounds how many syntax errors ParseRecover reports for a file.
const maxErrors = 100

// ParseRecover parses a file and reports every syntax error in it, not only the
// first. After an error it blanks out the statement that contains it, with
// spaces so that positions elsewhere do not move, and parses what is left. The
// tree is the model of the statements that were not blanked, or nil if none
// could be parsed. The errors are in order of position and every position is in
// the original source.
func ParseRecover(lang grammar.Language, file, src string) (*Node, []*Error) {
	g, err := grammar.Load(lang)
	if err != nil {
		return nil, []*Error{{File: file, Msg: err.Error()}}
	}
	c := parserFor(g)
	var errs []*Error
	cur := []byte(src)
	for len(errs) < maxErrors {
		node, err := c.parse(file, string(cur))
		if err == nil {
			return node, errs
		}
		var e *Error
		if !errors.As(err, &e) {
			return nil, append(errs, &Error{File: file, Msg: err.Error()})
		}
		// an error where the last one was is a consequence of blanking, not news
		if len(errs) == 0 || errs[len(errs)-1].Pos.Offset != e.Pos.Offset {
			errs = append(errs, e)
		}
		if !blank(c, file, cur, e) {
			return nil, errs
		}
	}
	return nil, errs
}

// blank replaces the statement containing the error with spaces, or the single
// offending character for a lexical error, and reports whether it changed
// anything.
func blank(c *compiled, file string, src []byte, e *Error) bool {
	at := e.Pos.Offset
	toks, err := c.lx.lex(file, string(src))
	if err != nil {
		// a lexical error: blank the character it stands on
		if at < len(src) && src[at] != '\n' {
			src[at] = ' '
			return true
		}
		return false
	}
	// the token the error is at
	ti := len(toks)
	for i, t := range toks {
		if t.Span.Start.Offset >= at {
			ti = i
			break
		}
	}
	isSym := func(t Token, s string) bool { return t.Kind == Symbol && t.Text == s }
	// the statement starts after the previous ; { or }
	start := 0
	for i := min(ti, len(toks)) - 1; i >= 0; i-- {
		if isSym(toks[i], ";") || isSym(toks[i], "{") || isSym(toks[i], "}") {
			start = i + 1
			break
		}
	}
	// and ends at its ; or at its closing }, or before the } that closes the body
	end, depth := len(toks), 0
scan:
	for i := start; i < len(toks); i++ {
		switch {
		case isSym(toks[i], "{"):
			depth++
		case isSym(toks[i], "}"):
			if depth == 0 {
				end = i
				break scan
			}
			depth--
			if depth == 0 {
				end = i + 1
				break scan
			}
		case isSym(toks[i], ";") && depth == 0:
			end = i + 1
			break scan
		}
	}
	if end <= start {
		// nothing but the closing brace: blank the offending token itself
		if ti >= len(toks) {
			return false
		}
		start, end = ti, ti+1
	}
	from, to := toks[start].Span.Start.Offset, toks[end-1].Span.End.Offset
	changed := false
	for i := from; i < to && i < len(src); i++ {
		if src[i] != '\n' && src[i] != ' ' {
			src[i] = ' '
			changed = true
		}
	}
	return changed
}
