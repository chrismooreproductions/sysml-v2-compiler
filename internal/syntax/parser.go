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

// A compiled grammar: the lexer, and the rules with their references already
// resolved, so parsing does no lookups by name.
type compiled struct {
	g     *grammar.Grammar
	lx    *lexer
	rules []*crule
	root  *crule
}

// lexSpec says which token kind implements a terminal rule.
type lexSpec struct {
	name string
	kind TokenKind
	desc string
}

// crule is a rule ready to interpret.
type crule struct {
	id    int
	r     *grammar.Rule
	class string // the metaclass it instantiates, or "" for a value rule
	body  *cexpr
	enum  bool
	frag  bool
	eBool bool // a value rule returning Ecore::EBoolean
}

// cexpr is a grammar expression with its references resolved.
type cexpr struct {
	kind     grammar.Kind
	text     string
	op       string
	prop     string
	kids     []*cexpr
	rule     *crule   // the rule a Ref (or an XRef's name rule) calls
	lex      *lexSpec // set instead of rule when the call is to a terminal rule
	typeName string   // for an Action: the metaclass, without namespace
	desc     string   // for a Term: how it is named in an error message
}

var (
	compiledMu       sync.Mutex
	compiledGrammars = map[*grammar.Grammar]*compiled{}
)

var lexicalSpecs = map[string]*lexSpec{
	"ID":                {"ID", Name, "a name"},
	"UNRESTRICTED_NAME": {"UNRESTRICTED_NAME", QuotedName, "a quoted name"},
	"DECIMAL_VALUE":     {"DECIMAL_VALUE", Decimal, "a number"},
	"EXP_VALUE":         {"EXP_VALUE", Exponent, "a number"},
	"STRING_VALUE":      {"STRING_VALUE", String, "a string"},
	"REGULAR_COMMENT":   {"REGULAR_COMMENT", Comment, "a comment"},
}

func parserFor(g *grammar.Grammar) *compiled {
	compiledMu.Lock()
	defer compiledMu.Unlock()
	if c, ok := compiledGrammars[g]; ok {
		return c
	}
	c := compile(g)
	compiledGrammars[g] = c
	return c
}

func compile(g *grammar.Grammar) *compiled {
	c := &compiled{g: g, lx: newLexer(g)}
	enums := map[string]bool{}
	for _, r := range g.Rules {
		if r.Kind == grammar.Enum && r.Returns != "" {
			enums[bare(r.Returns)] = true
		}
	}
	// Xtext infers a datatype rule: one with no declared type whose body does
	// not assign properties and calls only terminals, enums and other
	// datatype rules.
	datatype := map[string]bool{}
	for name, r := range g.Rules {
		if r.Kind == grammar.Normal && r.Returns == "" && !assigns(r.Body) {
			datatype[name] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for name := range datatype {
			grammar.Walk(g.Rules[name].Body, func(e *grammar.Expr) {
				if e.Kind != grammar.Ref || !datatype[name] {
					return
				}
				callee, ok := g.Rules[e.Text]
				if !ok || callee.Kind == grammar.Terminal || callee.Kind == grammar.Enum {
					return
				}
				if !datatype[e.Text] {
					delete(datatype, name)
					changed = true
				}
			})
		}
	}
	byName := map[string]*crule{}
	for _, name := range g.Order {
		r := g.Rules[name]
		cr := &crule{id: len(c.rules), r: r, enum: r.Kind == grammar.Enum, frag: r.Kind == grammar.Fragment}
		switch {
		case r.Kind == grammar.Enum:
		case r.Returns != "":
			if !strings.HasPrefix(r.Returns, "Ecore::") && !enums[bare(r.Returns)] {
				cr.class = bare(r.Returns)
			}
		case !datatype[name]:
			cr.class = name
		}
		cr.eBool = cr.class == "" && strings.HasSuffix(r.Returns, "EBoolean")
		c.rules = append(c.rules, cr)
		byName[name] = cr
	}
	var build func(e *grammar.Expr) *cexpr
	build = func(e *grammar.Expr) *cexpr {
		if e == nil {
			return nil
		}
		x := &cexpr{kind: e.Kind, text: e.Text, op: e.Op, prop: e.Prop}
		for _, k := range e.Kids {
			x.kids = append(x.kids, build(k))
		}
		switch e.Kind {
		case grammar.Ref:
			x.lex = lexicalSpecs[e.Text]
			if x.lex == nil {
				x.rule = byName[e.Text]
			}
		case grammar.XRef:
			x.lex = lexicalSpecs[e.Op]
			if x.lex == nil {
				x.rule = byName[e.Op]
			}
		case grammar.Action:
			x.typeName = bare(e.Text)
		case grammar.Term:
			x.desc = "'" + e.Text + "'"
		}
		return x
	}
	for _, cr := range c.rules {
		cr.body = build(cr.r.Body)
	}
	c.root = byName[g.Root]
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

// effs is a sequence of effects as a rope, so that joining the effects of
// consecutive matches costs one allocation and no copying.
type effs struct {
	l, r *effs
	e    *eff
}

func leaf(e eff) *effs { return &effs{e: &e} }

func cat(a, b *effs) *effs {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return &effs{l: a, r: b}
}

// each calls f for the effects in order. It is iterative: ropes built by
// long repetitions are deep on the left.
func (x *effs) each(f func(*eff)) {
	var stack []*effs
	for cur := x; cur != nil || len(stack) > 0; {
		if cur == nil {
			cur, stack = stack[len(stack)-1], stack[:len(stack)-1]
		}
		if cur.e != nil {
			f(cur.e)
			cur = nil
			continue
		}
		stack = append(stack, cur.r)
		cur = cur.l
	}
}

// result is one way a piece of the grammar matched: where it ended and what
// it did.
type result struct {
	end  int
	effs *effs
}

type valueResult struct {
	end int
	v   Value
}

// key identifies a rule at a token position.
func key(r *crule, pos int) uint64 { return uint64(r.id)<<32 | uint64(pos) }

// bits is a set of token positions, allocated on first use. Most rule
// attempts fail, and recording that in a bit costs far less than a map entry.
type bits []uint64

func (b bits) has(i int) bool { w := i >> 6; return w < len(b) && b[w]&(1<<(uint(i)&63)) != 0 }

// table is one set of positions per rule.
type table []bits

func (t table) has(r *crule, pos int) bool { return t[r.id].has(pos) }

func (t table) set(r *crule, pos, n int) {
	if t[r.id] == nil {
		t[r.id] = make(bits, n>>6+1)
	}
	t[r.id][pos>>6] |= 1 << (uint(pos) & 63)
}

func (t table) clear(r *crule, pos int) { t[r.id][pos>>6] &^= 1 << (uint(pos) & 63) }

type parser struct {
	*compiled
	file string
	src  string
	toks []Token

	// Successful matches are memoized in the maps; failures, and calls in
	// progress (to cut left recursion), are bits.
	memo                       map[uint64][]result
	vmemo                      map[uint64][]valueResult
	failI, failV, busyI, busyV table

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
		memo: make(map[uint64][]result, len(toks)*2), vmemo: make(map[uint64][]valueResult, len(toks)),
		failI: make(table, len(c.rules)), failV: make(table, len(c.rules)),
		busyI: make(table, len(c.rules)), busyV: make(table, len(c.rules)),
		expected: map[string]bool{},
	}
	for _, v := range ps.invoke(c.root, 0) {
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
		clear(ps.expected)
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

func (ps *parser) matchTerm(e *cexpr, p int) (int, bool) {
	i, t := ps.tokenAt(p)
	if t != nil && t.Text == e.text && (t.Kind == Keyword || t.Kind == Symbol) {
		return i + 1, true
	}
	if i >= ps.farthest {
		ps.fail(i, e.desc)
	}
	return 0, false
}

func (ps *parser) matchLexical(spec *lexSpec, p int) (int, string, bool) {
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
	if i >= ps.farthest {
		ps.fail(i, spec.desc)
	}
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
	if end-p == 1 && p < len(ps.toks) && ps.toks[p].Kind != Comment {
		return ps.toks[p].Text
	}
	var sb strings.Builder
	for i := p; i < end && i < len(ps.toks); i++ {
		if ps.toks[i].Kind != Comment {
			sb.WriteString(ps.toks[i].Text)
		}
	}
	return sb.String()
}

// ---- interpreting the grammar ----

// dedupe keeps the first result for each end position.
func dedupe(rs []result) []result {
	if len(rs) < 2 {
		return rs
	}
	if len(rs) <= 12 {
		out := rs[:0:0]
	next:
		for _, r := range rs {
			for _, o := range out {
				if o.end == r.end {
					continue next
				}
			}
			out = append(out, r)
		}
		return out
	}
	seen := make(map[int]bool, len(rs))
	out := rs[:0:0]
	for _, r := range rs {
		if !seen[r.end] {
			seen[r.end] = true
			out = append(out, r)
		}
	}
	return out
}

func (ps *parser) ev(e *cexpr, p int) []result {
	switch e.kind {
	case grammar.Term:
		if end, ok := ps.matchTerm(e, p); ok {
			return []result{{end, nil}}
		}
		return nil
	case grammar.XRef:
		var out []result
		for _, v := range ps.xrefValues(e, p) {
			out = append(out, result{v.end, leaf(eff{v: v.v})})
		}
		return out
	case grammar.Action:
		if e.prop == "" {
			return []result{{p, leaf(eff{setType: e.typeName})}}
		}
		return []result{{p, leaf(eff{wrapType: e.typeName, prop: e.prop, op: e.op, pos: p})}}
	case grammar.Pred:
		return ps.ev(e.kids[0], p)
	case grammar.Wild:
		if i, t := ps.tokenAt(p); t != nil {
			return []result{{i + 1, nil}}
		}
		return nil
	case grammar.Ref:
		return ps.evRef(e, p)
	case grammar.Assign:
		var out []result
		for _, v := range ps.evValue(e.kids[0], p) {
			val := v.v
			if e.op == "?=" {
				val = Bool(true)
			}
			out = append(out, result{v.end, leaf(eff{prop: e.text, op: e.op, v: val})})
		}
		return dedupe(out)
	case grammar.Alt:
		var out []result
		for _, k := range e.kids {
			out = append(out, ps.ev(k, p)...)
		}
		return dedupe(out)
	case grammar.Seq:
		cur := []result{{p, nil}}
		for _, k := range e.kids {
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
		kid := e.kids[0]
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
			if e.op == "?" {
				all = next
				break
			}
			all = append(next, all...) // longer matches first
			cur = next
		}
		if e.op == "+" {
			return dedupe(all)
		}
		return dedupe(append(all, result{p, nil}))
	}
	return nil
}

// xrefValues matches the name of a cross-reference with the rule it names.
func (ps *parser) xrefValues(e *cexpr, p int) []valueResult {
	var out []valueResult
	for _, r := range ps.callResults(e, p) {
		out = append(out, valueResult{r.end, Ref{Name: ps.tokenText(p, r.end), Span: ps.spanOf(p, r.end)}})
	}
	return out
}

// callResults matches the rule or terminal an expression names, ignoring
// what it builds.
func (ps *parser) callResults(e *cexpr, p int) []result {
	if e.lex != nil {
		if end, _, ok := ps.matchLexical(e.lex, p); ok {
			return []result{{end, nil}}
		}
		return nil
	}
	if e.rule == nil {
		return nil
	}
	return ps.evRule(e.rule, p)
}

func (ps *parser) evRef(e *cexpr, p int) []result { return ps.callResults(e, p) }

func (ps *parser) evRule(r *crule, p int) []result {
	if r.frag || r.enum {
		return ps.inline(r, p)
	}
	// A rule called without an assignment produces a node that becomes the
	// node under construction.
	var out []result
	for _, v := range ps.invoke(r, p) {
		if n, ok := v.v.(*Node); ok {
			out = append(out, result{v.end, leaf(eff{v: n, rebind: true})})
		} else {
			out = append(out, result{v.end, nil}) // a value rule: the value is discarded
		}
	}
	return dedupe(out)
}

// inline matches a rule's body in the context of the caller.
func (ps *parser) inline(r *crule, p int) []result {
	if ps.failI.has(r, p) {
		return nil
	}
	k := key(r, p)
	if res, ok := ps.memo[k]; ok {
		return res
	}
	if ps.busyI.has(r, p) {
		return nil // a left-recursive call: it can only match through its other alternatives
	}
	n := len(ps.toks) + 1
	ps.busyI.set(r, p, n)
	res := ps.ev(r.body, p)
	ps.busyI.clear(r, p)
	if len(res) == 0 {
		ps.failI.set(r, p, n)
	} else {
		ps.memo[k] = res
	}
	return res
}

// invoke matches a rule in value context: the result is a node, or for
// datatype and enum rules a plain value.
func (ps *parser) invoke(r *crule, p int) []valueResult {
	if ps.failV.has(r, p) {
		return nil
	}
	k := key(r, p)
	if res, ok := ps.vmemo[k]; ok {
		return res
	}
	if ps.busyV.has(r, p) {
		return nil
	}
	n := len(ps.toks) + 1
	ps.busyV.set(r, p, n)
	var out []valueResult
	for _, res := range ps.inline(r, p) {
		dup := false
		for _, o := range out {
			if o.end == res.end {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, valueResult{res.end, ps.finalize(r, p, res)})
		}
	}
	ps.busyV.clear(r, p)
	if len(out) == 0 {
		ps.failV.set(r, p, n)
	} else {
		ps.vmemo[k] = out
	}
	return out
}

func (ps *parser) evValue(e *cexpr, p int) []valueResult {
	switch e.kind {
	case grammar.Term:
		if end, ok := ps.matchTerm(e, p); ok {
			return []valueResult{{end, Text(e.text)}}
		}
		return nil
	case grammar.XRef:
		return ps.xrefValues(e, p)
	case grammar.Ref:
		if e.lex != nil {
			if end, txt, ok := ps.matchLexical(e.lex, p); ok {
				return []valueResult{{end, Text(txt)}}
			}
			return nil
		}
		if e.rule != nil {
			return ps.invoke(e.rule, p)
		}
		return nil
	case grammar.Alt:
		var out []valueResult
		for _, k := range e.kids {
			for _, v := range ps.evValue(k, p) {
				dup := false
				for _, o := range out {
					if o.end == v.end {
						dup = true
						break
					}
				}
				if !dup {
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
func (ps *parser) finalize(r *crule, p int, res result) Value {
	if r.enum {
		// an enum yields the literal's name (requirement = 'require'), not the keyword
		name := ""
		res.effs.each(func(e *eff) {
			if name == "" && e.prop != "" {
				name = e.prop
			}
		})
		if name != "" {
			return Text(name)
		}
		return Text(ps.tokenText(p, res.end))
	}

	class := r.class
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

	res.effs.each(func(e *eff) {
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
	})

	if cur == nil {
		if class != "" {
			return &Node{Class: class, Span: ps.spanOf(p, res.end)}
		}
		txt := ps.tokenText(p, res.end)
		if r.eBool {
			return Bool(txt != "nonunique" && txt != "false") // the pilot's value converter
		}
		return Text(txt)
	}
	if cur.Class == "" {
		cur.Class = r.r.Name // a rule with no declared type and assignments is typed by its name
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
