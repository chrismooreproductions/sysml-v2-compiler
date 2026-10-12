// spike2 parses the pilot implementation's Xtext grammars into an IR and
// compares them against the kebnf IR from spike0, to find where the two
// sources of truth disagree.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

type Expr struct {
	K    string  `json:"k"` // alt seq rep term ref xref assign action pred wild
	Text string  `json:"text,omitempty"`
	Op   string  `json:"op,omitempty"`
	Prop string  `json:"prop,omitempty"` // action: property being assigned from current
	Kids []*Expr `json:"kids,omitempty"`
}

type Prod struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
	Kind string `json:"kind"` // rule fragment terminal enum
	Body *Expr  `json:"body,omitempty"`
	File string `json:"file"`
}

type tok struct {
	kind, text string
	line       int
}

var identRe = regexp.MustCompile(`^[\^A-Za-z_$][A-Za-z0-9_$]*(::[\^A-Za-z_$][A-Za-z0-9_$]*)*`)

func lex(src, file string) ([]tok, error) {
	var out []tok
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
			out = append(out, tok{"str", src[i+1 : j], line})
			i = j + 1
		default:
			if m := identRe.FindString(src[i:]); m != "" {
				out = append(out, tok{"id", m, line})
				i += len(m)
				continue
			}
			matched := false
			for _, s := range []string{"+=", "?=", "=>", "->", "..", "="} {
				if strings.HasPrefix(src[i:], s) {
					out = append(out, tok{"sym", s, line})
					i += len(s)
					matched = true
					break
				}
			}
			if matched {
				continue
			}
			if strings.ContainsRune("|()?*+{}[]:;!&.,<>~@#$%^/-", rune(c)) || (c >= '0' && c <= '9') {
				out = append(out, tok{"sym", string(c), line})
				i++
				continue
			}
			return nil, fmt.Errorf("%s:%d: unexpected character %q", file, line, c)
		}
	}
	return out, nil
}

type parser struct {
	t    []tok
	i    int
	file string
}

func (p *parser) peek() *tok {
	if p.i < len(p.t) {
		return &p.t[p.i]
	}
	return nil
}
func (p *parser) is(s string) bool { t := p.peek(); return t != nil && t.kind == "sym" && t.text == s }
func (p *parser) errf(f string, a ...any) error {
	l := 0
	if t := p.peek(); t != nil {
		l = t.line
	}
	return fmt.Errorf("%s:%d: %s", p.file, l, fmt.Sprintf(f, a...))
}

func (p *parser) file0() (prods []*Prod, base string, err error) {
	for p.peek() != nil {
		t := p.peek()
		switch {
		case t.kind == "id" && t.text == "grammar":
			p.i++
			// dotted grammar name: ids separated by '.'
			for n := p.peek(); n != nil && (n.kind == "id" && n.text != "with" && n.text != "hidden" && n.text != "import" || n.kind == "sym" && n.text == "."); n = p.peek() {
				p.i++
			}
			for {
				n := p.peek()
				if n != nil && n.text == "with" {
					p.i++
					base = p.peek().text
					p.i++
					for p.is(".") {
						base += "." + p.t[p.i+1].text
						p.i += 2
					}
					for p.is(",") {
						p.i += 2
					}
				} else if n != nil && n.text == "hidden" {
					for !p.is(")") {
						p.i++
					}
					p.i++
				} else {
					break
				}
			}
		case t.kind == "id" && (t.text == "import" || t.text == "generate"):
			for p.peek() != nil && !(p.peek().kind == "id" && p.peek().text == "as") {
				p.i++
			}
			p.i += 2
		default:
			pr, err := p.rule()
			if err != nil {
				return nil, "", err
			}
			prods = append(prods, pr)
		}
	}
	return
}

func (p *parser) rule() (*Prod, error) {
	pr := &Prod{Kind: "rule", File: p.file}
	for p.is("@") { // annotation such as @Override
		p.i += 2
	}
	for {
		t := p.peek()
		if t != nil && t.kind == "id" && (t.text == "fragment" || t.text == "terminal" || t.text == "enum") {
			pr.Kind = t.text
			p.i++
			continue
		}
		break
	}
	n := p.peek()
	if n == nil || n.kind != "id" {
		return nil, p.errf("expected rule name")
	}
	pr.Name = n.text
	p.i++
	if t := p.peek(); t != nil && t.kind == "id" && t.text == "returns" {
		p.i++
		pr.Type = p.peek().text
		p.i++
	}
	if t := p.peek(); t != nil && t.kind == "id" && t.text == "hidden" {
		for !p.is(")") {
			p.i++
		}
		p.i++
	}
	if !p.is(":") {
		return nil, p.errf("expected ':' after rule %s", pr.Name)
	}
	p.i++
	if pr.Kind == "terminal" {
		for p.peek() != nil && !p.is(";") {
			p.i++
		}
		p.i++
		return pr, nil
	}
	body, err := p.alt()
	if err != nil {
		return nil, err
	}
	if !p.is(";") {
		return nil, p.errf("expected ';' ending rule %s, got %q", pr.Name, p.peek().text)
	}
	p.i++
	pr.Body = body
	return pr, nil
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
	return &Expr{K: "alt", Kids: alts}, nil
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
		return &Expr{K: "seq"}, nil // empty alternative
	case 1:
		return items[0], nil
	}
	return &Expr{K: "seq", Kids: items}, nil
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
		e = &Expr{K: "rep", Op: p.peek().text, Kids: []*Expr{e}}
		p.i++
	}
	if pre != "" {
		e = &Expr{K: "pred", Text: pre, Kids: []*Expr{e}}
	}
	return e, nil
}

func (p *parser) primary() (*Expr, error) {
	t := p.peek()
	if t == nil {
		return nil, p.errf("unexpected end")
	}
	switch {
	case t.kind == "str":
		p.i++
		if p.is("..") { // character range, terminal-style
			p.i += 2
		}
		return &Expr{K: "term", Text: t.text}, nil
	case t.kind == "id":
		p.i++
		if n := p.peek(); n != nil && n.kind == "sym" && (n.text == "=" || n.text == "+=" || n.text == "?=") {
			p.i++
			v, err := p.primary()
			if err != nil {
				return nil, err
			}
			return &Expr{K: "assign", Text: t.text, Op: n.text, Kids: []*Expr{v}}, nil
		}
		return &Expr{K: "ref", Text: t.text}, nil
	case t.kind == "sym" && t.text == "(":
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
	case t.kind == "sym" && t.text == "[":
		p.i++
		ty := p.peek().text
		p.i++
		name := ""
		if p.is("|") {
			p.i++
			name = p.peek().text
			p.i++
		}
		if !p.is("]") {
			return nil, p.errf("expected ']'")
		}
		p.i++
		return &Expr{K: "xref", Text: ty, Op: name}, nil
	case t.kind == "sym" && t.text == "{":
		p.i++
		ty := p.peek().text
		p.i++
		a := &Expr{K: "action", Text: ty}
		if p.is(".") {
			p.i++
			a.Prop = p.peek().text
			p.i++
			a.Op = p.peek().text
			p.i++
			p.i++ // 'current'
		}
		if !p.is("}") {
			return nil, p.errf("expected '}'")
		}
		p.i++
		return a, nil
	case t.kind == "sym" && t.text == ".":
		p.i++
		return &Expr{K: "wild"}, nil
	}
	return nil, p.errf("unexpected %q", t.text)
}

func walk(e *Expr, f func(*Expr)) {
	if e == nil {
		return
	}
	f(e)
	for _, k := range e.Kids {
		walk(k, f)
	}
}

func load(path string) ([]*Prod, string) {
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	toks, err := lex(string(b), path)
	if err != nil {
		fmt.Println("LEX ERROR:", err)
		os.Exit(1)
	}
	p := &parser{t: toks, file: path[strings.LastIndex(path, "/")+1:]}
	prods, base, err := p.file0()
	if err != nil {
		fmt.Println("PARSE ERROR:", err)
		os.Exit(1)
	}
	return prods, base
}

// ---- comparison against the kebnf IR ----

type kExpr struct {
	K    string   `json:"k"`
	Text string   `json:"text"`
	Op   string   `json:"op"`
	Kids []*kExpr `json:"kids"`
}
type kProd struct {
	Name string `json:"name"`
	Body *kExpr `json:"body"`
}

// minLen is the minimum number of tokens a production must consume.
func minLen(prods map[string]*Expr, e *Expr, memo map[string]int, busy map[string]bool) int {
	if e == nil {
		return 0
	}
	switch e.K {
	case "term", "xref", "wild":
		return 1
	case "ref":
		if _, ok := prods[e.Text]; !ok {
			return 1 // lexical / inherited terminal
		}
		if v, ok := memo[e.Text]; ok {
			return v
		}
		if busy[e.Text] {
			return 0
		}
		busy[e.Text] = true
		v := minLen(prods, prods[e.Text], memo, busy)
		delete(busy, e.Text)
		memo[e.Text] = v
		return v
	case "action":
		return 0
	case "assign", "pred":
		return minLen(prods, e.Kids[0], memo, busy)
	case "rep":
		if e.Op == "+" {
			return minLen(prods, e.Kids[0], memo, busy)
		}
		return 0
	case "alt":
		m := -1
		for _, k := range e.Kids {
			if v := minLen(prods, k, memo, busy); m < 0 || v < m {
				m = v
			}
		}
		return m
	case "seq":
		s := 0
		for _, k := range e.Kids {
			s += minLen(prods, k, memo, busy)
		}
		return s
	}
	return 0
}

func conv(k *kExpr) *Expr {
	if k == nil {
		return nil
	}
	e := &Expr{K: k.K, Text: k.Text, Op: k.Op}
	for _, c := range k.Kids {
		e.Kids = append(e.Kids, conv(c))
	}
	return e
}

func main() {
	root := os.Args[1] // pilot implementation checkout
	kebnf := os.Args[2]
	x := root + "/"
	exprs, _ := load(x + "org.omg.kerml.expressions.xtext/src/org/omg/kerml/expressions/xtext/KerMLExpressions.xtext")
	sysml, base := load(x + "org.omg.sysml.xtext/src/org/omg/sysml/xtext/SysML.xtext")
	kerml, kbase := load(x + "org.omg.kerml.xtext/src/org/omg/kerml/xtext/KerML.xtext")
	fmt.Printf("KerMLExpressions: %d rules; SysML: %d rules (with %s); KerML: %d rules (with %s)\n", len(exprs), len(sysml), base[strings.LastIndex(base, ".")+1:], len(kerml), kbase[strings.LastIndex(kbase, ".")+1:])

	merged := func(over ...[]*Prod) []*Prod {
		idx := map[string]int{}
		var out []*Prod
		for _, set := range over {
			for _, p := range set {
				if i, ok := idx[p.Name]; ok {
					out[i] = p
				} else {
					idx[p.Name] = len(out)
					out = append(out, p)
				}
			}
		}
		return out
	}
	sys := merged(exprs, sysml)
	ker := merged(exprs, kerml)
	js, _ := json.MarshalIndent(map[string]any{"sysml": sys, "kerml": ker}, "", " ")
	os.WriteFile("xtext-ir.json", js, 0o644)

	kinds := map[string]int{}
	cons := map[string]int{}
	for _, p := range sys {
		kinds[p.Kind]++
		walk(p.Body, func(e *Expr) {
			switch e.K {
			case "pred", "action", "xref", "wild":
				k := e.K
				if e.K == "pred" {
					k += e.Text
				}
				if e.K == "action" && e.Prop != "" {
					k += "(" + e.Op + " current)"
				}
				cons[k]++
			}
		})
	}
	fmt.Printf("SysML grammar (merged with expressions): %d rules %v\n  constructs: %v\n", len(sys), kinds, cons)

	// undefined references
	defined := map[string]bool{"ID": true, "STRING": true, "INT": true, "WS": true, "ML_COMMENT": true, "SL_COMMENT": true, "ANY_OTHER": true, "EOF": true}
	for _, p := range sys {
		defined[p.Name] = true
	}
	und := map[string]bool{}
	for _, p := range sys {
		walk(p.Body, func(e *Expr) {
			if e.K == "ref" && !defined[e.Text] {
				und[e.Text] = true
			}
		})
	}
	var us []string
	for k := range und {
		us = append(us, k)
	}
	sort.Strings(us)
	fmt.Println("undefined references in merged SysML grammar:", us)

	// compare with kebnf
	b, err := os.ReadFile(kebnf)
	if err != nil {
		fmt.Println(err)
		return
	}
	var kp []*kProd
	json.Unmarshal(b, &kp)
	kmap := map[string]*Expr{}
	for _, p := range kp { // SysML overrides KerML, as in spike1
		kmap[p.Name] = conv(p.Body)
	}
	xmap := map[string]*Expr{}
	xkind := map[string]string{}
	for _, p := range sys {
		if p.Kind == "rule" || p.Kind == "fragment" {
			xmap[p.Name] = p.Body
			xkind[p.Name] = p.Kind
		}
	}
	km, xm := map[string]int{}, map[string]int{}
	var shared []string
	for n := range kmap {
		if _, ok := xmap[n]; ok {
			shared = append(shared, n)
		}
	}
	sort.Strings(shared)
	var diffs []string
	for _, n := range shared {
		a := minLen(kmap, kmap[n], km, map[string]bool{})
		bb := minLen(xmap, xmap[n], xm, map[string]bool{})
		if a != bb {
			diffs = append(diffs, fmt.Sprintf("%s (kebnf %d, xtext %d)", n, a, bb))
		}
	}
	fmt.Printf("\nrules in both grammars: %d; differing minimum token length: %d\n", len(shared), len(diffs))
	for _, d := range diffs {
		fmt.Println("  ", d)
	}
}
