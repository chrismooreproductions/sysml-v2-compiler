// SPDX-License-Identifier: EPL-2.0
//
// Portions derived from the SysML v2 Pilot Implementation
// (org.omg.sysml.util.ElementUtil), Copyright (c) 2018-2025 Model Driven
// Solutions, Inc. and others, EPL-2.0. See NOTICE.

package postprocess

import (
	"strings"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
)

// unescapeString follows ElementUtil.unescapeString: a literal that starts
// with a double or single quote loses its quotes and its backslash escapes
// (\b \t \n \f \r \" \' \; any other escaped character becomes a space). Any
// other text is returned unchanged.
func unescapeString(literal string) string {
	if literal == "" || literal[0] != '"' && literal[0] != '\'' {
		return literal
	}
	var s strings.Builder
	i := 1
	for {
		j := strings.IndexByte(literal[i:], '\\')
		if j < 0 {
			break
		}
		j += i
		s.WriteString(literal[i:j])
		var c byte = ' '
		if j+1 < len(literal) {
			switch literal[j+1] {
			case 'b':
				c = '\b'
			case 't':
				c = '\t'
			case 'n':
				c = '\n'
			case 'f':
				c = '\f'
			case 'r':
				c = '\r'
			case '"':
				c = '"'
			case '\'':
				c = '\''
			case '\\':
				c = '\\'
			}
		}
		s.WriteByte(c)
		i = j + 2
	}
	if n := len(literal); n > 1 && i <= n-1 {
		s.WriteString(literal[i : n-1])
	}
	return s.String()
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func trimLeftSpace(s string) string {
	i := 0
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	return s[i:]
}

// processCommentBody follows ElementUtil.processCommentBody: remove the
// comment delimiters, and from a multi-line body the leading white space and
// the conventional '*' (with one following space) of each line.
func processCommentBody(body string) string {
	body = strings.Replace(body, "/*", "", 1)
	body = trimLeftSpace(body)
	body = strings.TrimSuffix(body, "*/")

	// Java's String.split drops trailing empty strings.
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 1 {
		return lines[0]
	}
	var b strings.Builder
	for i, line := range lines {
		s := trimLeftSpace(line)
		if strings.HasPrefix(s, "*") {
			if len(s) > 1 && isSpace(s[1]) {
				s = s[2:]
			} else {
				s = s[1:]
			}
		}
		if i != 0 {
			b.WriteByte('\n')
		}
		b.WriteString(s)
	}
	return b.String()
}

// mapText applies f to the Text values of the named properties.
func mapText(e *model.Element, f func(string) string, names ...string) {
	for i, p := range e.Props {
		if t, ok := p.Value.(model.Text); ok {
			for _, n := range names {
				if p.Name == n {
					e.Props[i].Value = model.Text(f(string(t)))
				}
			}
		}
	}
}

// text applies the post-processors that unescape names and strings and clean
// comment bodies: ElementParserPostProcessor, MembershipParserPostProcessor,
// LiteralStringParserPostProcessor, CommentParserPostProcessor and
// TextualRepresentationParserPostProcessor.
func text(e *model.Element) {
	mapText(e, unescapeString, "declaredName", "declaredShortName")
	if e.IsA("Membership") && !e.IsA("OwningMembership") {
		mapText(e, unescapeString, "memberName", "memberShortName")
	}
	if e.IsA("LiteralString") {
		mapText(e, unescapeString, "value")
	}
	if e.IsA("Comment") {
		mapText(e, unescapeString, "locale")
		mapText(e, processCommentBody, "body")
	}
	if e.IsA("TextualRepresentation") {
		mapText(e, unescapeString, "language")
		mapText(e, processCommentBody, "body")
	}
}
