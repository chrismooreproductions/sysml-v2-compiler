// SPDX-License-Identifier: EPL-2.0

package grammar

import (
	"fmt"
	"regexp"
	"strings"
)

// File is one parsed .xtext file.
type File struct {
	Name  string
	Base  string // the grammar it extends ("with"), if any
	Rules []*Rule
}

type tokKind uint8

const (
	tokString tokKind = iota + 1
	tokIdent
	tokSym
)

type token struct {
	kind tokKind
	text string
	line int
}

// An identifier may be qualified (SysML::Usage). A leading ^ escapes a
// keyword used as a name.
var identRe = regexp.MustCompile(`^[\^A-Za-z_$][A-Za-z0-9_$]*(::[\^A-Za-z_$][A-Za-z0-9_$]*)*`)

var multiSyms = []string{"+=", "?=", "=>", "->", "..", "="}

func lex(src, file string) ([]token, error) {
	var out []token
	line := 1
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case strings.HasPrefix(src[i:], "//"):
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case strings.HasPrefix(src[i:], "/*"):
			j := strings.Index(src[i+2:], "*/")
			if j < 0 {
				return nil, fmt.Errorf("%s:%d: unterminated comment", file, line)
			}
			line += strings.Count(src[i:i+j+4], "\n")
			i += j + 4
		case c == '\'' || c == '"':
			j := i + 1
			for j < len(src) && src[j] != c {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(src) {
				return nil, fmt.Errorf("%s:%d: unterminated string", file, line)
			}
			out = append(out, token{tokString, src[i+1 : j], line})
			i = j + 1
		default:
			if m := identRe.FindString(src[i:]); m != "" {
				out = append(out, token{tokIdent, m, line})
				i += len(m)
				continue
			}
			matched := false
			for _, s := range multiSyms {
				if strings.HasPrefix(src[i:], s) {
					out = append(out, token{tokSym, s, line})
					i += len(s)
					matched = true
					break
				}
			}
			if matched {
				continue
			}
			if strings.ContainsRune("|()?*+{}[]:;!&.,<>~@#$%^/-", rune(c)) || (c >= '0' && c <= '9') {
				out = append(out, token{tokSym, string(c), line})
				i++
				continue
			}
			return nil, fmt.Errorf("%s:%d: unexpected character %q", file, line, c)
		}
	}
	return out, nil
}

type parser struct {
	t    []token
	i    int
	file string
}

func (p *parser) peek() *token {
	if p.i < len(p.t) {
		return &p.t[p.i]
	}
	return nil
}

func (p *parser) is(s string) bool {
	t := p.peek()
	return t != nil && t.kind == tokSym && t.text == s
}

func (p *parser) word(s string) bool {
	t := p.peek()
	return t != nil && t.kind == tokIdent && t.text == s
}

func (p *parser) errf(format string, a ...any) error {
	line := 0
	if t := p.peek(); t != nil {
		line = t.line
	} else if len(p.t) > 0 {
		line = p.t[len(p.t)-1].line
	}
	return fmt.Errorf("%s:%d: %s", p.file, line, fmt.Sprintf(format, a...))
}

// ParseFile parses the text of an .xtext file.
func ParseFile(name, src string) (*File, error) {
	toks, err := lex(src, name)
	if err != nil {
		return nil, err
	}
	p := &parser{t: toks, file: name}
	f := &File{Name: name}
	for p.peek() != nil {
		switch {
		case p.word("grammar"):
			p.i++
			p.dotted() // the grammar's own name
			p.headerClauses(f)
		case p.word("import"), p.word("generate"):
			for p.peek() != nil && !p.word("as") {
				p.i++
			}
			p.i += 2
		default:
			r, err := p.rule()
			if err != nil {
				return nil, err
			}
			f.Rules = append(f.Rules, r)
		}
	}
	return f, nil
}

// headerClauses consumes the "with ..." and "hidden(...)" clauses that may
// follow a grammar's name.
func (p *parser) headerClauses(f *File) {
	for {
		switch {
		case p.word("with"):
			p.i++
			f.Base = p.dotted()
			for p.is(",") {
				p.i++
				p.dotted()
			}
		case p.word("hidden"):
			for !p.is(")") && p.peek() != nil {
				p.i++
			}
			p.i++
		default:
			return
		}
	}
}

// dotted consumes a name written with dots, such as org.omg.sysml.xtext.SysML.
func (p *parser) dotted() string {
	var sb strings.Builder
	for {
		t := p.peek()
		if t == nil || t.kind != tokIdent || t.text == "with" || t.text == "hidden" || t.text == "import" {
			break
		}
		sb.WriteString(t.text)
		p.i++
		if !p.is(".") {
			break
		}
		sb.WriteByte('.')
		p.i++
	}
	return sb.String()
}

func (p *parser) rule() (*Rule, error) {
	r := &Rule{File: p.file}
	for p.is("@") { // an annotation such as @Override
		p.i += 2
	}
	for {
		t := p.peek()
		if t == nil || t.kind != tokIdent {
			break
		}
		switch t.text {
		case "fragment":
			r.Kind = Fragment
		case "terminal":
			r.Kind = Terminal
		case "enum":
			r.Kind = Enum
		default:
			goto named
		}
		p.i++
	}
named:
	n := p.peek()
	if n == nil || n.kind != tokIdent {
		return nil, p.errf("expected a rule name")
	}
	r.Name, r.Line = n.text, n.line
	p.i++
	if p.word("returns") {
		p.i++
		if t := p.peek(); t != nil && t.kind == tokIdent {
			r.Returns = t.text
			p.i++
		}
	}
	if p.word("hidden") {
		for !p.is(")") && p.peek() != nil {
			p.i++
		}
		p.i++
	}
	if !p.is(":") {
		return nil, p.errf("expected ':' after rule %s", r.Name)
	}
	p.i++
	if r.Kind == Terminal {
		for p.peek() != nil && !p.is(";") {
			p.i++
		}
		p.i++
		return r, nil
	}
	body, err := p.alt()
	if err != nil {
		return nil, err
	}
	if !p.is(";") {
		got := "end of file"
		if t := p.peek(); t != nil {
			got = fmt.Sprintf("%q", t.text)
		}
		return nil, p.errf("expected ';' ending rule %s, got %s", r.Name, got)
	}
	p.i++
	r.Body = body
	return r, nil
}

func (p *parser) alt() (*Expr, error) {
	var alts []*Expr
	for {
		s, err := p.seq()
		if err != nil {
			return nil, err
		}
		alts = append(alts, s)
		if p.is("|") {
			p.i++
			continue
		}
		break
	}
	if len(alts) == 1 {
		return alts[0], nil
	}
	return &Expr{Kind: Alt, Kids: alts}, nil
}

func (p *parser) seq() (*Expr, error) {
	var items []*Expr
	for p.peek() != nil && !p.is("|") && !p.is(")") && !p.is(";") {
		it, err := p.item()
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	switch len(items) {
	case 0:
		return &Expr{Kind: Seq}, nil // an empty alternative
	case 1:
		return items[0], nil
	}
	return &Expr{Kind: Seq, Kids: items}, nil
}

func (p *parser) item() (*Expr, error) {
	pre := ""
	if p.is("=>") || p.is("->") || p.is("!") {
		pre = p.peek().text
		p.i++
	}
	e, err := p.primary()
	if err != nil {
		return nil, err
	}
	for p.is("?") || p.is("*") || p.is("+") {
		e = &Expr{Kind: Rep, Op: p.peek().text, Kids: []*Expr{e}}
		p.i++
	}
	if pre != "" {
		e = &Expr{Kind: Pred, Text: pre, Kids: []*Expr{e}}
	}
	return e, nil
}

func (p *parser) primary() (*Expr, error) {
	t := p.peek()
	if t == nil {
		return nil, p.errf("unexpected end of file")
	}
	switch {
	case t.kind == tokString:
		p.i++
		if p.is("..") { // a character range, as in terminal rules
			p.i += 2
		}
		return &Expr{Kind: Term, Text: t.text}, nil
	case t.kind == tokIdent:
		p.i++
		if n := p.peek(); n != nil && n.kind == tokSym && (n.text == "=" || n.text == "+=" || n.text == "?=") {
			p.i++
			v, err := p.primary()
			if err != nil {
				return nil, err
			}
			return &Expr{Kind: Assign, Text: t.text, Op: n.text, Kids: []*Expr{v}}, nil
		}
		return &Expr{Kind: Ref, Text: t.text}, nil
	case t.kind == tokSym && t.text == "(":
		p.i++
		e, err := p.alt()
		if err != nil {
			return nil, err
		}
		if !p.is(")") {
			return nil, p.errf("expected ')'")
		}
		p.i++
		return e, nil
	case t.kind == tokSym && t.text == "[":
		p.i++
		ty := p.peek()
		if ty == nil || ty.kind != tokIdent {
			return nil, p.errf("expected a type in a cross-reference")
		}
		p.i++
		rule := ""
		if p.is("|") {
			p.i++
			if n := p.peek(); n != nil && n.kind == tokIdent {
				rule = n.text
				p.i++
			}
		}
		if !p.is("]") {
			return nil, p.errf("expected ']'")
		}
		p.i++
		if rule == "" {
			rule = "ID"
		}
		return &Expr{Kind: XRef, Text: ty.text, Op: rule}, nil
	case t.kind == tokSym && t.text == "{":
		p.i++
		ty := p.peek()
		if ty == nil || ty.kind != tokIdent {
			return nil, p.errf("expected a type in an action")
		}
		p.i++
		a := &Expr{Kind: Action, Text: ty.text}
		if p.is(".") {
			p.i++
			prop := p.peek()
			if prop == nil || prop.kind != tokIdent {
				return nil, p.errf("expected a property in an action")
			}
			a.Prop = prop.text
			p.i++
			op := p.peek()
			if op == nil || op.kind != tokSym || (op.text != "=" && op.text != "+=") {
				return nil, p.errf("expected '=' or '+=' in an action")
			}
			a.Op = op.text
			p.i++
			if !p.word("current") {
				return nil, p.errf("expected 'current' in an action")
			}
			p.i++
		}
		if !p.is("}") {
			return nil, p.errf("expected '}'")
		}
		p.i++
		return a, nil
	case t.kind == tokSym && t.text == ".":
		p.i++
		return &Expr{Kind: Wild}, nil
	}
	return nil, p.errf("unexpected %q", t.text)
}
