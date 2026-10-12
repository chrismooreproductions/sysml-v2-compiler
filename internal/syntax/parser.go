// SPDX-License-Identifier: EPL-2.0

package syntax

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
)

// Parse parses a SysML v2 or KerML document. It returns the root Namespace.
//
// The grammars are ambiguous in places, so the parser keeps every way a rule
// can match and chooses a complete parse by trying alternatives in grammar
// order. When there is none, the error reports the furthest position reached
// and what could have continued the input there.
func Parse(lang grammar.Language, file, src string) (*Node, error) {
	g, err := grammar.Load(lang)
	if err != nil {
		return nil, err
	}
	return parserFor(g).parse(file, src)
}

// A compiled grammar: the lexer and the facts about rules that do not change
// between documents.
type compiled struct {
	g        *grammar.Grammar
	lx       *lexer
	datatype map[string]bool // rules that yield a plain value, not a node
	enums    map[string]bool // metaclass names that are enumerations
}

var (
	compiledMu       sync.Mutex
	compiledGrammars = map[*grammar.Grammar]*compiled{}
)

func parserFor(g *grammar.Grammar) *compiled {
	compiledMu.Lock()
	defer compiledMu.Unlock()
	if c, ok := compiledGrammars[g]; ok {
		return c
	}
	c := &compiled{g: g, lx: newLexer(g), datatype: map[string]bool{}, enums: map[string]bool{}}
	for _, r := range g.Rules {
		if r.Kind == grammar.Enum && r.Returns != "" {
			c.enums[bare(r.Returns)] = true
		}
	}
	// Xtext infers a datatype rule: one with no declared type whose body does
	// not assign properties and calls only terminals, enums and other
	// datatype rules.
	cand := map[string]bool{}
	for name, r := range g.Rules {
		if r.Kind == grammar.Normal && r.Returns == "" && !assigns(r.Body) {
			cand[name] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for name := range cand {
			grammar.Walk(g.Rules[name].Body, func(e *grammar.Expr) {
				if e.Kind != grammar.Ref || !cand[name] {
					return
				}
				callee, ok := g.Rules[e.Text]
				if !ok || callee.Kind == grammar.Terminal || callee.Kind == grammar.Enum {
					return
				}
				if !cand[e.Text] {
					delete(cand, name)
					changed = true
				}
			})
		}
	}
	c.datatype = cand
	compiledGrammars[g] = c
	return c
}

func assigns(e *grammar.Expr) bool {
	found := false
	grammar.Walk(e, func(x *grammar.Expr) {
		if x.Kind == grammar.Assign || x.Kind == grammar.Action {
			found = true
		}
	})
	return found
}

func bare(t string) string {
	if i := strings.LastIndex(t, "::"); i >= 0 {
		return t[i+2:]
	}
	return t
}

// classOf is the metaclass a rule instantiates, or "" if it yields a value.
func (c *compiled) classOf(r *grammar.Rule) string {
	switch {
	case r.Kind == grammar.Enum:
		return ""
	case r.Returns != "":
		if strings.HasPrefix(r.Returns, "Ecore::") || c.enums[bare(r.Returns)] {
			return ""
		}
		return bare(r.Returns)
	case c.datatype[r.Name]:
		return ""
	}
	return r.Name
}

// ---- results of matching ----

// eff is one effect of matching part of a rule: an assignment, or an action
// that changes the node under construction.
type eff struct {
	prop, op string
	v        Value
	rebind   bool   // v is a *Node that replaces the node under construction
	wrapType string // {Type.prop op current}: a new node holding the old one
	setType  string // {Type}: the node under construction has this type
	pos      int    // for wrapType: the token index where the action ran
}

// result is one way a piece of the grammar matched: where it ended and what
// it did.
type result struct {
	end  int
	effs []eff
}

type valueResult struct {
	end int
	v   Value
}

type memoKey struct {
	rule  string
	pos   int
	value bool
}

type parser struct {
	*compiled
	file string
	src  string
	toks []Token

	memo  map[memoKey][]result
	vmemo map[memoKey][]valueResult
	busy  map[memoKey]bool

	farthest int
	expected map[string]bool
}

func (c *compiled) parse(file, src string) (*Node, error) {
	toks, err := c.lx.lex(file, src)
	if err != nil {
		return nil, err
	}
	ps := &parser{
		compiled: c, file: file, src: src, toks: toks,
		memo: map[memoKey][]result{}, vmemo: map[memoKey][]valueResult{}, busy: map[memoKey]bool{},
		expected: map[string]bool{},
	}
	root := c.g.Rules[c.g.Root]
	for _, v := range ps.invoke(root, 0) {
		if ps.skipComments(v.end) == len(toks) {
			if n, ok := v.v.(*Node); ok {
				return n, nil
			}
		}
	}
	return nil, ps.syntaxError()
}

func (ps *parser) skipComments(i int) int {
	for i < len(ps.toks) && ps.toks[i].Kind == Comment {
		i++
	}
	return i
}

// ---- matching tokens ----

func (ps *parser) fail(i int, what string) {
	if i > ps.farthest {
		ps.farthest = i
		ps.expected = map[string]bool{}
	}
	if i == ps.farthest {
		ps.expected[what] = true
	}
}

func (ps *parser) tokenAt(p int) (int, *Token) {
	p = ps.skipComments(p)
	if p >= len(ps.toks) {
		return p, nil
	}
	return p, &ps.toks[p]
}

func (ps *parser) matchTerm(text string, p int) (int, bool) {
	i, t := ps.tokenAt(p)
	if t != nil && t.Text == text && (t.Kind == Keyword || t.Kind == Symbol) {
		return i + 1, true
	}
	ps.fail(i, "'"+text+"'")
	return 0, false
}

// lexical maps a terminal rule to the token kind that implements it.
var lexical = map[string]struct {
	kind TokenKind
	desc string
}{
	"ID":                {Name, "a name"},
	"UNRESTRICTED_NAME": {QuotedName, "a quoted name"},
	"DECIMAL_VALUE":     {Decimal, "a number"},
	"EXP_VALUE":         {Exponent, "a number"},
	"STRING_VALUE":      {String, "a string"},
	"REGULAR_COMMENT":   {Comment, "a comment"},
}

func (ps *parser) matchLexical(name string, p int) (int, string, bool) {
	spec := lexical[name]
	if spec.kind == Comment {
		if p < len(ps.toks) && ps.toks[p].Kind == Comment {
			return p + 1, ps.toks[p].Text, true
		}
		ps.fail(p, spec.desc)
		return 0, "", false
	}
	i, t := ps.tokenAt(p)
	if t != nil && t.Kind == spec.kind {
		return i + 1, t.Text, true
	}
	ps.fail(i, spec.desc)
	return 0, "", false
}

// ---- spans and text ----

// spanOf is the source range of tokens [p, end).
func (ps *parser) spanOf(p, end int) Span {
	start := ps.skipComments(p)
	if start >= end || start >= len(ps.toks) {
		// an empty match: a zero-width span at the next token, or at the end
		var at Pos
		if start < len(ps.toks) {
			at = ps.toks[start].Span.Start
		} else if n := len(ps.toks); n > 0 {
			at = ps.toks[n-1].Span.End
		} else {
			at = Pos{Line: 1, Column: 1}
		}
		return Span{at, at}
	}
	return Span{ps.toks[start].Span.Start, ps.toks[end-1].Span.End}
}

func (ps *parser) tokenText(p, end int) string {
	var sb strings.Builder
	for i := p; i < end && i < len(ps.toks); i++ {
		if ps.toks[i].Kind != Comment {
			sb.WriteString(ps.toks[i].Text)
		}
	}
	return sb.String()
}

// ---- interpreting the grammar ----

func dedupe(rs []result) []result {
	seen := map[int]bool{}
	out := rs[:0:0]
	for _, r := range rs {
		if !seen[r.end] {
			seen[r.end] = true
			out = append(out, r)
		}
	}
	return out
}

func cat(a, b []eff) []eff {
	if len(b) == 0 {
		return a
	}
	out := make([]eff, 0, len(a)+len(b))
	out = append(out, a...)
	return append(out, b...)
}

func (ps *parser) ev(e *grammar.Expr, p int) []result {
	switch e.Kind {
	case grammar.Term:
		if end, ok := ps.matchTerm(e.Text, p); ok {
			return []result{{end, nil}}
		}
		return nil
	case grammar.XRef:
		var out []result
		for _, v := range ps.xrefValues(e, p) {
			out = append(out, result{v.end, []eff{{prop: "", v: v.v}}})
		}
		return out
	case grammar.Action:
		if e.Prop == "" {
			return []result{{p, []eff{{setType: bare(e.Text)}}}}
		}
		return []result{{p, []eff{{wrapType: bare(e.Text), prop: e.Prop, op: e.Op, pos: p}}}}
	case grammar.Pred:
		return ps.ev(e.Kids[0], p)
	case grammar.Wild:
		if i, t := ps.tokenAt(p); t != nil {
			return []result{{i + 1, nil}}
		}
		return nil
	case grammar.Ref:
		return ps.evRef(e.Text, p)
	case grammar.Assign:
		var out []result
		for _, v := range ps.evValue(e.Kids[0], p) {
			val := v.v
			if e.Op == "?=" {
				val = Bool(true)
			}
			out = append(out, result{v.end, []eff{{prop: e.Text, op: e.Op, v: val}}})
		}
		return dedupe(out)
	case grammar.Alt:
		var out []result
		for _, k := range e.Kids {
			out = append(out, ps.ev(k, p)...)
		}
		return dedupe(out)
	case grammar.Seq:
		cur := []result{{p, nil}}
		for _, k := range e.Kids {
			var next []result
			for _, r := range cur {
				for _, m := range ps.ev(k, r.end) {
					next = append(next, result{m.end, cat(r.effs, m.effs)})
				}
			}
			cur = dedupe(next)
			if len(cur) == 0 {
				return nil
			}
		}
		return cur
	case grammar.Rep:
		kid := e.Kids[0]
		var all []result
		cur := []result{{p, nil}}
		for len(cur) > 0 {
			var next []result
			for _, r := range cur {
				for _, m := range ps.ev(kid, r.end) {
					if m.end > r.end {
						next = append(next, result{m.end, cat(r.effs, m.effs)})
					}
				}
			}
			next = dedupe(next)
			if e.Op == "?" {
				all = next
				break
			}
			all = append(next, all...) // longer matches first
			cur = next
		}
		if e.Op == "+" {
			return dedupe(all)
		}
		return dedupe(append(all, result{p, nil}))
	}
	return nil
}

// xrefValues matches the name of a cross-reference with the rule it names.
func (ps *parser) xrefValues(e *grammar.Expr, p int) []valueResult {
	var out []valueResult
	for _, r := range ps.evRef(e.Op, p) {
		out = append(out, valueResult{r.end, Ref{Name: ps.tokenText(p, r.end), Span: ps.spanOf(p, r.end)}})
	}
	return out
}

func (ps *parser) evRef(name string, p int) []result {
	if _, ok := lexical[name]; ok {
		if end, _, ok := ps.matchLexical(name, p); ok {
			return []result{{end, nil}}
		}
		return nil
	}
	r := ps.g.Rules[name]
	if r == nil {
		return nil
	}
	if r.Kind == grammar.Fragment || r.Kind == grammar.Enum {
		return ps.inline(r, p)
	}
	// A rule called without an assignment produces a node that becomes the
	// node under construction.
	var out []result
	for _, v := range ps.invoke(r, p) {
		if n, ok := v.v.(*Node); ok {
			out = append(out, result{v.end, []eff{{v: n, rebind: true}}})
		} else {
			out = append(out, result{v.end, nil}) // a value rule: the value is discarded
		}
	}
	return dedupe(out)
}

// inline matches a rule's body in the context of the caller.
func (ps *parser) inline(r *grammar.Rule, p int) []result {
	k := memoKey{r.Name, p, false}
	if res, ok := ps.memo[k]; ok {
		return res
	}
	if ps.busy[k] {
		return nil // a left-recursive call: it can only match through its other alternatives
	}
	ps.busy[k] = true
	res := ps.ev(r.Body, p)
	delete(ps.busy, k)
	ps.memo[k] = res
	return res
}

// invoke matches a rule in value context: the result is a node, or for
// datatype and enum rules a plain value.
func (ps *parser) invoke(r *grammar.Rule, p int) []valueResult {
	k := memoKey{r.Name, p, true}
	if res, ok := ps.vmemo[k]; ok {
		return res
	}
	if ps.busy[k] {
		return nil
	}
	ps.busy[k] = true
	class := ps.classOf(r)
	var out []valueResult
	seen := map[int]bool{}
	for _, res := range ps.inline(r, p) {
		if seen[res.end] {
			continue
		}
		seen[res.end] = true
		out = append(out, valueResult{res.end, ps.finalize(r, class, p, res)})
	}
	delete(ps.busy, k)
	ps.vmemo[k] = out
	return out
}

func (ps *parser) evValue(e *grammar.Expr, p int) []valueResult {
	switch e.Kind {
	case grammar.Term:
		if end, ok := ps.matchTerm(e.Text, p); ok {
			return []valueResult{{end, Text(e.Text)}}
		}
		return nil
	case grammar.XRef:
		return ps.xrefValues(e, p)
	case grammar.Ref:
		if _, ok := lexical[e.Text]; ok {
			if end, txt, ok := ps.matchLexical(e.Text, p); ok {
				return []valueResult{{end, Text(txt)}}
			}
			return nil
		}
		if r := ps.g.Rules[e.Text]; r != nil {
			return ps.invoke(r, p)
		}
		return nil
	case grammar.Alt:
		var out []valueResult
		seen := map[int]bool{}
		for _, k := range e.Kids {
			for _, v := range ps.evValue(k, p) {
				if !seen[v.end] {
					seen[v.end] = true
					out = append(out, v)
				}
			}
		}
		return out
	}
	var out []valueResult
	for _, r := range ps.ev(e, p) {
		out = append(out, valueResult{r.end, Text(ps.tokenText(p, r.end))})
	}
	return out
}

// finalize builds a rule's value from the effects its body had.
func (ps *parser) finalize(r *grammar.Rule, class string, p int, res result) Value {
	if r.Kind == grammar.Enum {
		// an enum yields the literal's name (requirement = 'require'), not the keyword
		for _, e := range res.effs {
			if e.prop != "" {
				return Text(e.prop)
			}
		}
		return Text(ps.tokenText(p, res.end))
	}

	var cur *Node
	ensure := func() *Node {
		if cur == nil {
			cur = &Node{Class: class}
		}
		return cur
	}
	// the nodes that wrap what came before, and where each action ran; they
	// cover the input up to the next action, and the last up to the end
	type wrap struct {
		n   *Node
		pos int
	}
	var wraps []wrap
	var base *Node

	for _, e := range res.effs {
		switch {
		case e.rebind:
			n := e.v.(*Node)
			c := &Node{Class: n.Class, Props: append([]Prop{}, n.Props...)}
			if cur != nil { // effects set before the rebind belong to the same element
				c.Props = append(c.Props, cur.Props...)
			}
			cur = c
		case e.wrapType != "":
			b := ensure()
			if len(wraps) == 0 {
				base = b
			}
			cur = &Node{Class: e.wrapType, Props: []Prop{{e.prop, e.op, b}}}
			wraps = append(wraps, wrap{cur, e.pos})
		case e.setType != "":
			ensure().Class = e.setType
		case e.prop == "":
			// the value of a cross-reference that is not assigned: nothing to record
		default:
			c := ensure()
			c.Props = append(c.Props, Prop{e.prop, e.op, e.v})
		}
	}

	if cur == nil {
		if class != "" {
			return &Node{Class: class, Span: ps.spanOf(p, res.end)}
		}
		txt := ps.tokenText(p, res.end)
		if strings.HasSuffix(r.Returns, "EBoolean") {
			return Bool(txt != "nonunique" && txt != "false") // the pilot's value converter
		}
		return Text(txt)
	}
	if cur.Class == "" {
		cur.Class = r.Name // a rule with no declared type and assignments is typed by its name
	}
	if base != nil {
		base.Span = ps.spanOf(p, wraps[0].pos)
	}
	for i, w := range wraps {
		end := res.end
		if i+1 < len(wraps) {
			end = wraps[i+1].pos
		}
		w.n.Span = ps.spanOf(p, end)
	}
	if len(wraps) == 0 || cur != wraps[len(wraps)-1].n {
		cur.Span = ps.spanOf(p, res.end)
	}
	return cur
}

// ---- errors ----

func (ps *parser) syntaxError() error {
	got, pos := "", Pos{Line: 1, Column: 1}
	if ps.farthest < len(ps.toks) {
		got, pos = ps.toks[ps.farthest].Text, ps.toks[ps.farthest].Span.Start
	} else if n := len(ps.toks); n > 0 {
		pos = ps.toks[n-1].Span.End
	}
	var exp []string
	for e := range ps.expected {
		exp = append(exp, e)
	}
	sort.Slice(exp, func(i, j int) bool {
		// descriptions of kinds of token ("a name") before literals
		ai, aj := strings.HasPrefix(exp[i], "a "), strings.HasPrefix(exp[j], "a ")
		if ai != aj {
			return ai
		}
		return exp[i] < exp[j]
	})
	msg := "unexpected end of input"
	if got != "" {
		msg = fmt.Sprintf("unexpected %q", got)
	}
	if len(exp) > 0 {
		shown := exp
		more := ""
		if len(shown) > 8 {
			more = fmt.Sprintf(", or %d more", len(shown)-8)
			shown = shown[:8]
		}
		msg += ", expected " + strings.Join(shown, ", ") + more
	}
	return &Error{File: ps.file, Pos: pos, Msg: msg, Got: got, Expected: exp}
}
