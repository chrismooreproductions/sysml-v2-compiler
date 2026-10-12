// SPDX-License-Identifier: EPL-2.0

// spike3 interprets the Xtext grammar IR (from spike2) with a memoizing
// backtracking matcher and builds a generic node tree. It is spike1 with the
// Xtext semantics: fragment rules, {Type.prop += current} actions, enum rules
// and keywords taken from the grammar itself.
package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// ---------- IR ----------

type Expr struct {
	K    string  `json:"k"`
	Text string  `json:"text"`
	Op   string  `json:"op"`
	Prop string  `json:"prop"`
	Kids []*Expr `json:"kids"`
}
type Prod struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Kind string `json:"kind"` // rule fragment terminal enum
	Body *Expr  `json:"body"`
}

// bare strips a metamodel namespace: "SysML::Usage" -> "Usage".
func bare(t string) string {
	if i := strings.LastIndex(t, "::"); i >= 0 {
		return t[i+2:]
	}
	return t
}

// ---------- ecore ----------

type ecoreFile struct {
	Classes []struct {
		Name     string `xml:"name,attr"`
		XsiType  string `xml:"http://www.w3.org/2001/XMLSchema-instance type,attr"`
		Supers   string `xml:"eSuperTypes,attr"`
		Features []struct {
			Name    string `xml:"name,attr"`
			Type    string `xml:"eType,attr"`
			Default string `xml:"defaultValueLiteral,attr"`
			Derived string `xml:"derived,attr"`
			Transie string `xml:"transient,attr"`
			Annots  []struct {
				Source string `xml:"source,attr"`
				Refs   string `xml:"references,attr"`
			} `xml:"eAnnotations"`
		} `xml:"eStructuralFeatures"`
	} `xml:"eClassifiers"`
}

type featInfo struct {
	typ, def string
	derived  bool
}

type meta struct {
	supers   map[string][]string
	features map[string]map[string]bool
	enums    map[string]bool
	redef    map[string]map[string][]string // class -> feature -> names of the features it redefines
	info     map[string]map[string]featInfo
}

// feature looks a feature up on a class or any of its supertypes.
func (m *meta) feature(class, name string) (featInfo, bool) {
	seen := map[string]bool{}
	var f func(string) (featInfo, bool)
	f = func(c string) (featInfo, bool) {
		if seen[c] {
			return featInfo{}, false
		}
		seen[c] = true
		if fi, ok := m.info[c][name]; ok {
			return fi, true
		}
		for _, s := range m.supers[c] {
			if fi, ok := f(s); ok {
				return fi, true
			}
		}
		return featInfo{}, false
	}
	return f(class)
}

func loadEcore(path string) (*meta, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f ecoreFile
	if err := xml.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	m := &meta{redef: map[string]map[string][]string{}, enums: map[string]bool{}, supers: map[string][]string{}, features: map[string]map[string]bool{}, info: map[string]map[string]featInfo{}}
	for _, c := range f.Classes {
		if c.XsiType == "ecore:EEnum" {
			m.enums[c.Name] = true
		}
		m.features[c.Name] = map[string]bool{}
		m.info[c.Name] = map[string]featInfo{}
		for _, ft := range c.Features {
			m.features[c.Name][ft.Name] = true
			for _, a := range ft.Annots {
				if a.Source != "redefines" {
					continue
				}
				if m.redef[c.Name] == nil {
					m.redef[c.Name] = map[string][]string{}
				}
				for _, r := range strings.Fields(a.Refs) {
					m.redef[c.Name][ft.Name] = append(m.redef[c.Name][ft.Name], r[strings.LastIndex(r, "/")+1:])
				}
			}
			m.info[c.Name][ft.Name] = featInfo{typ: ft.Type, def: ft.Default, derived: ft.Derived == "true" || ft.Transie == "true"}
		}
		for _, s := range strings.Fields(c.Supers) {
			if i := strings.LastIndex(s, "//"); i >= 0 {
				s = s[i+2:]
			}
			m.supers[c.Name] = append(m.supers[c.Name], s)
		}
	}
	return m, nil
}

func (m *meta) isSub(c, target string) bool {
	if c == "" {
		return false
	}
	seen := map[string]bool{}
	var f func(string) bool
	f = func(x string) bool {
		if x == target {
			return true
		}
		if seen[x] {
			return false
		}
		seen[x] = true
		for _, s := range m.supers[x] {
			if f(s) {
				return true
			}
		}
		return false
	}
	return f(c)
}

// effectiveRole follows redefinitions down the class hierarchy: a redefined
// feature shares storage with the feature that redefines it, so a value for
// Specialization.general on a CrossSubsetting is held as crossedFeature.
func (m *meta) effectiveRole(class, role string) string {
	for changed := true; changed; {
		changed = false
		queue, seen := []string{class}, map[string]bool{}
		for len(queue) > 0 && !changed {
			c := queue[0]
			queue = queue[1:]
			if seen[c] {
				continue
			}
			seen[c] = true
			for f, redefined := range m.redef[c] {
				for _, r := range redefined {
					if r == role {
						role, changed = f, true
					}
				}
			}
			queue = append(queue, m.supers[c]...)
		}
	}
	return role
}

// isSourceEnd reports whether a relationship feature is the source end,
// directly or through redefinitions (specific, subsettingFeature,
// featureInverted, ...). A source-end reference to the owner is the inverse
// of containment; a target-end reference to the owner is a real reference.
func (m *meta) isSourceEnd(class, role string) bool {
	seen := map[string]bool{}
	var walk func(r string) bool
	walk = func(r string) bool {
		if r == "source" {
			return true
		}
		if seen[r] {
			return false
		}
		seen[r] = true
		queue, visited := []string{class}, map[string]bool{}
		for len(queue) > 0 {
			c := queue[0]
			queue = queue[1:]
			if visited[c] {
				continue
			}
			visited[c] = true
			for _, red := range m.redef[c][r] {
				if walk(red) {
					return true
				}
			}
			queue = append(queue, m.supers[c]...)
		}
		return false
	}
	return walk(role)
}

// isTargetEnd reports whether a relationship feature is the target end,
// directly or through redefinitions (general, subsettedFeature, type, ...).
func (m *meta) isTargetEnd(class, role string) bool {
	seen := map[string]bool{}
	var walk func(r string) bool
	walk = func(r string) bool {
		if r == "target" || r == "general" {
			return true
		}
		if seen[r] {
			return false
		}
		seen[r] = true
		queue, visited := []string{class}, map[string]bool{}
		for len(queue) > 0 {
			c := queue[0]
			queue = queue[1:]
			if visited[c] {
				continue
			}
			visited[c] = true
			for _, red := range m.redef[c][r] {
				if walk(red) {
					return true
				}
			}
			queue = append(queue, m.supers[c]...)
		}
		return false
	}
	return walk(role)
}

func (m *meta) hasFeature(c, f string) bool {
	seen := map[string]bool{}
	var g func(string) bool
	g = func(x string) bool {
		if seen[x] {
			return false
		}
		seen[x] = true
		if m.features[x][f] {
			return true
		}
		for _, s := range m.supers[x] {
			if g(s) {
				return true
			}
		}
		return false
	}
	return g(c)
}

// ---------- tokens ----------

type Tok struct {
	Kind string // kw sym name str dec exp rc
	Text string
	Line int
	Col  int
}

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*`)

func (g *Grammar) lex(src string) ([]Tok, error) {
	var out []Tok
	i, line, col := 0, 1, 1
	adv := func(n int) {
		for k := 0; k < n; k++ {
			if src[i] == '\n' {
				line++
				col = 1
			} else {
				col++
			}
			i++
		}
	}
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f':
			adv(1)
		case strings.HasPrefix(src[i:], "//*"):
			j := strings.Index(src[i:], "*/")
			if j < 0 {
				return nil, fmt.Errorf("%d:%d unterminated note", line, col)
			}
			adv(j + 2)
		case strings.HasPrefix(src[i:], "//"):
			for i < len(src) && src[i] != '\n' {
				adv(1)
			}
		case strings.HasPrefix(src[i:], "/*"):
			j := strings.Index(src[i+2:], "*/")
			if j < 0 {
				return nil, fmt.Errorf("%d:%d unterminated comment", line, col)
			}
			out = append(out, Tok{"rc", src[i : i+j+4], line, col})
			adv(j + 4)
		case c == '"':
			j := i + 1
			for j < len(src) && src[j] != '"' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			out = append(out, Tok{"str", src[i : j+1], line, col})
			adv(j + 1 - i)
		case c == '\'':
			j := i + 1
			for j < len(src) && src[j] != '\'' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			out = append(out, Tok{"name", src[i : j+1], line, col})
			adv(j + 1 - i)
		case c >= '0' && c <= '9':
			j := i
			for j < len(src) && src[j] >= '0' && src[j] <= '9' {
				j++
			}
			kind := "dec"
			if j < len(src) && (src[j] == 'e' || src[j] == 'E') {
				k := j + 1
				if k < len(src) && (src[k] == '+' || src[k] == '-') {
					k++
				}
				if k < len(src) && src[k] >= '0' && src[k] <= '9' {
					for k < len(src) && src[k] >= '0' && src[k] <= '9' {
						k++
					}
					j = k
					kind = "exp"
				}
			}
			out = append(out, Tok{kind, src[i:j], line, col})
			adv(j - i)
		default:
			if m := identRe.FindString(src[i:]); m != "" {
				kind := "name"
				if g.keywords[m] {
					kind = "kw"
				}
				out = append(out, Tok{kind, m, line, col})
				adv(len(m))
				continue
			}
			best := ""
			for _, s := range g.symbols {
				if strings.HasPrefix(src[i:], s) && len(s) > len(best) {
					best = s
				}
			}
			if best == "" {
				return nil, fmt.Errorf("%d:%d unexpected character %q", line, col, c)
			}
			out = append(out, Tok{"sym", best, line, col})
			adv(len(best))
		}
	}
	return out, nil
}

// ---------- generic node tree ----------

type Val interface{}
type Str string
type Bool bool
type Ref string // a name written in the source, not yet resolved

// RefOwned is a reference to the i-th element the relationship owns (a
// feature chain); RefOwner is a reference to the element owning it.
type RefOwned int
type RefOwner struct{}

type Prop struct {
	Name, Op string
	V        Val
}
type Node struct {
	Class string
	Props []Prop
}

func (n *Node) with(effs []eff) *Node {
	c := &Node{Class: n.Class, Props: append([]Prop{}, n.Props...)}
	for _, e := range effs {
		c.Props = append(c.Props, Prop{e.prop, e.op, e.v})
	}
	return c
}

type eff struct {
	prop, op string
	v        Val
	rebind   bool   // v is a *Node that replaces the node under construction
	wrapType string // {Type.prop op current}: new node of this type holding the old one
	setType  string // {Type}: the node under construction has this type
}

type res struct {
	end  int
	effs []eff
}

type vres struct {
	end int
	v   Val
}

// ---------- grammar ----------

type Grammar struct {
	prods    map[string]*Prod
	keywords map[string]bool
	symbols  []string
	meta     *meta
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

var lexicalToks = map[string]bool{"ID": true, "UNRESTRICTED_NAME": true, "DECIMAL_VALUE": true,
	"EXP_VALUE": true, "STRING_VALUE": true, "REGULAR_COMMENT": true}

func loadGrammar(irPath, which string, m *meta) (*Grammar, error) {
	b, err := os.ReadFile(irPath)
	if err != nil {
		return nil, err
	}
	var doc map[string][]*Prod
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	g := &Grammar{prods: map[string]*Prod{}, keywords: map[string]bool{}, meta: m}
	for _, p := range doc[which] {
		g.prods[p.Name] = p
	}
	// Every identifier-shaped string literal in a rule is a reserved
	// keyword, as in Xtext; every other literal is a symbol.
	syms := map[string]bool{}
	for _, p := range g.prods {
		walk(p.Body, func(e *Expr) {
			if e.K != "term" || e.Text == "" {
				return
			}
			if identRe.MatchString(e.Text) && identRe.FindString(e.Text) == e.Text {
				g.keywords[e.Text] = true
			} else if !(e.Text[0] >= '0' && e.Text[0] <= '9') {
				syms[e.Text] = true
			}
		})
	}
	for s := range syms {
		g.symbols = append(g.symbols, s)
	}
	sort.Strings(g.symbols)
	return g, nil
}

// ---------- parser ----------

type memoKey struct {
	name string
	pos  int
	cls  string
	val  bool
}

type Parser struct {
	g     *Grammar
	toks  []Tok
	memo  map[memoKey][]res
	vmemo map[memoKey][]vres
	busy  map[memoKey]bool

	farthest int
	expected map[string]bool

	anomalies map[string]int
	guards    int
}

// classOf is the metaclass a rule instantiates, or "" for a datatype rule.
func (ps *Parser) classOf(p *Prod) string {
	if strings.HasPrefix(p.Type, "Ecore::") {
		return ""
	}
	if p.Type != "" {
		if ps.g.meta.enums[bare(p.Type)] {
			return "" // an enum-valued rule yields the literal, not a node
		}
		return bare(p.Type)
	}
	if _, ok := ps.g.meta.features[p.Name]; ok {
		return p.Name
	}
	return ""
}

func (ps *Parser) skipRC(i int) int {
	for i < len(ps.toks) && ps.toks[i].Kind == "rc" {
		i++
	}
	return i
}

func (ps *Parser) fail(i int, what string) {
	if i > ps.farthest {
		ps.farthest = i
		ps.expected = map[string]bool{}
	}
	if i == ps.farthest {
		ps.expected[what] = true
	}
}

func (ps *Parser) tokenAt(p int) (int, *Tok) {
	p = ps.skipRC(p)
	if p >= len(ps.toks) {
		return p, nil
	}
	return p, &ps.toks[p]
}

func (ps *Parser) matchTerm(text string, p int) (int, bool) {
	i, t := ps.tokenAt(p)
	if t != nil && t.Text == text && (t.Kind == "kw" || t.Kind == "sym" || t.Kind == "name") {
		return i + 1, true
	}
	ps.fail(i, "'"+text+"'")
	return 0, false
}

func (ps *Parser) matchLexical(name string, p int) (int, string, bool) {
	if name == "REGULAR_COMMENT" {
		if p < len(ps.toks) && ps.toks[p].Kind == "rc" {
			return p + 1, ps.toks[p].Text, true
		}
		ps.fail(p, name)
		return 0, "", false
	}
	i, t := ps.tokenAt(p)
	if t != nil {
		ok := false
		switch name {
		case "ID":
			ok = t.Kind == "name" && t.Text[0] != '\''
		case "UNRESTRICTED_NAME":
			ok = t.Kind == "name" && t.Text[0] == '\''
		case "DECIMAL_VALUE":
			ok = t.Kind == "dec"
		case "EXP_VALUE":
			ok = t.Kind == "exp"
		case "STRING_VALUE":
			ok = t.Kind == "str"
		}
		if ok {
			return i + 1, t.Text, true
		}
	}
	ps.fail(i, name)
	return 0, "", false
}

func dedupe(rs []res) []res {
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

func (ps *Parser) ev(e *Expr, p int, cls string) []res {
	switch e.K {
	case "term":
		if end, ok := ps.matchTerm(e.Text, p); ok {
			return []res{{end, nil}}
		}
		return nil
	case "xref":
		return ps.evXref(e, p, cls)
	case "action":
		if e.Prop == "" {
			return []res{{p, []eff{{setType: bare(e.Text)}}}}
		}
		return []res{{p, []eff{{wrapType: bare(e.Text), prop: e.Prop, op: e.Op}}}}
	case "pred":
		return ps.ev(e.Kids[0], p, cls)
	case "wild":
		if i, t := ps.tokenAt(p); t != nil {
			return []res{{i + 1, nil}}
		}
		return nil
	case "ref":
		return ps.evRef(e.Text, p, cls)
	case "assign":
		var out []res
		for _, v := range ps.evVal(e.Kids[0], p, cls) {
			val := v.v
			if e.Op == "?=" {
				val = Bool(true)
			}
			out = append(out, res{v.end, []eff{{prop: e.Text, op: e.Op, v: val}}})
		}
		return dedupe(out)
	case "alt":
		var out []res
		for _, k := range e.Kids {
			out = append(out, ps.ev(k, p, cls)...)
		}
		return dedupe(out)
	case "seq":
		cur := []res{{p, nil}}
		for _, k := range e.Kids {
			var next []res
			for _, r := range cur {
				for _, m := range ps.ev(k, r.end, cls) {
					next = append(next, res{m.end, cat(r.effs, m.effs)})
				}
			}
			cur = dedupe(next)
			if len(cur) == 0 {
				return nil
			}
		}
		return cur
	case "rep":
		kid := e.Kids[0]
		var all []res
		cur := []res{{p, nil}}
		for len(cur) > 0 {
			var next []res
			for _, r := range cur {
				for _, m := range ps.ev(kid, r.end, cls) {
					if m.end > r.end {
						next = append(next, res{m.end, cat(r.effs, m.effs)})
					}
				}
			}
			next = dedupe(next)
			if e.Op == "?" {
				all = next
				break
			}
			all = append(next, all...) // longer first
			cur = next
		}
		if e.Op == "+" {
			return dedupe(all)
		}
		return dedupe(append(all, res{p, nil}))
	}
	return nil
}

func (ps *Parser) evXref(e *Expr, p int, cls string) []res {
	var out []res
	for _, v := range ps.xrefVals(e, p) {
		out = append(out, res{v.end, []eff{{prop: "(xref)", op: "", v: v.v}}})
	}
	return out
}

func (ps *Parser) xrefVals(e *Expr, p int) []vres {
	name := e.Op
	if name == "" {
		name = "ID"
	}
	var out []vres
	for _, r := range ps.evRef(name, p, "") {
		var sb strings.Builder
		for i := p; i < r.end; i++ {
			if ps.toks[i].Kind != "rc" {
				sb.WriteString(ps.toks[i].Text)
			}
		}
		out = append(out, vres{r.end, Ref(sb.String())})
	}
	return out
}

func (ps *Parser) evRef(name string, p int, cls string) []res {
	if lexicalToks[name] {
		if end, _, ok := ps.matchLexical(name, p); ok {
			return []res{{end, nil}}
		}
		return nil
	}
	pr := ps.g.prods[name]
	if pr == nil {
		ps.anomalies["undefined:"+name]++
		return nil
	}
	if pr.Kind == "fragment" || pr.Kind == "enum" {
		return ps.inline(pr, p, "")
	}
	// A non-fragment rule called without an assignment produces a node that
	// becomes the node under construction.
	var out []res
	for _, v := range ps.invoke(pr, p) {
		if n, ok := v.v.(*Node); ok {
			out = append(out, res{v.end, []eff{{v: n, rebind: true}}})
		} else {
			out = append(out, res{v.end, nil}) // datatype rule: value discarded
		}
	}
	return dedupe(out)
}

func (ps *Parser) inline(pr *Prod, p int, cls string) []res {
	k := memoKey{pr.Name, p, cls, false}
	if r, ok := ps.memo[k]; ok {
		return r
	}
	if ps.busy[k] {
		ps.guards++
		return nil
	}
	ps.busy[k] = true
	r := ps.ev(pr.Body, p, cls)
	delete(ps.busy, k)
	ps.memo[k] = r
	return r
}

// invoke evaluates pr in value context: a node (or datatype string) is produced.
func (ps *Parser) invoke(pr *Prod, p int) []vres {
	k := memoKey{pr.Name, p, "", true}
	if r, ok := ps.vmemo[k]; ok {
		return r
	}
	if ps.busy[k] {
		ps.guards++
		return nil
	}
	ps.busy[k] = true
	cls := ps.classOf(pr)
	var out []vres
	seen := map[int]bool{}
	for _, r := range ps.inline(pr, p, "") {
		if seen[r.end] {
			continue
		}
		seen[r.end] = true
		out = append(out, vres{r.end, ps.finalize(pr, cls, p, r)})
	}
	delete(ps.busy, k)
	ps.vmemo[k] = out
	return out
}

func (ps *Parser) tokenText(p, end int) string {
	var sb strings.Builder
	for i := p; i < end; i++ {
		if ps.toks[i].Kind != "rc" {
			sb.WriteString(ps.toks[i].Text)
		}
	}
	return sb.String()
}

// finalize turns the effects a rule accumulated into its value: a node, or a
// plain string for datatype and enum rules.
func (ps *Parser) finalize(pr *Prod, cls string, p int, r res) Val {
	if pr.Kind == "enum" {
		// an enum yields the literal's name (requirement = 'require'), not the keyword
		for _, e := range r.effs {
			if e.prop != "" {
				return Str(e.prop)
			}
		}
		return Str(ps.tokenText(p, r.end))
	}
	var cur *Node
	ensure := func() *Node {
		if cur == nil {
			cur = &Node{Class: cls}
		}
		return cur
	}
	for _, e := range r.effs {
		switch {
		case e.rebind:
			n := e.v.(*Node)
			c := &Node{Class: n.Class, Props: append([]Prop{}, n.Props...)}
			if cur != nil { // effects set before the rebind belong to the same element
				c.Props = append(c.Props, cur.Props...)
			}
			cur = c
		case e.wrapType != "":
			base := ensure()
			cur = &Node{Class: e.wrapType, Props: []Prop{{e.prop, e.op, base}}}
		case e.setType != "":
			ensure().Class = e.setType
		default:
			c := ensure()
			c.Props = append(c.Props, Prop{e.prop, e.op, e.v})
		}
	}
	if cur == nil {
		if cls != "" {
			return &Node{Class: cls}
		}
		txt := ps.tokenText(p, r.end)
		if pr.Type == "Ecore::EBoolean" && txt == "nonunique" {
			txt = "false" // the pilot implementation's value converter
		}
		return Str(txt)
	}
	if cur.Class == "" {
		ps.anomalies["classless-with-effects:"+pr.Name]++
		cur.Class = "?" + pr.Name
	}
	return cur
}

func (ps *Parser) evVal(e *Expr, p int, cls string) []vres {
	switch e.K {
	case "term":
		if end, ok := ps.matchTerm(e.Text, p); ok {
			return []vres{{end, Str(e.Text)}}
		}
		return nil
	case "xref":
		return ps.xrefVals(e, p)
	case "ref":
		if lexicalToks[e.Text] {
			if end, txt, ok := ps.matchLexical(e.Text, p); ok {
				return []vres{{end, Str(txt)}}
			}
			return nil
		}
		pr := ps.g.prods[e.Text]
		if pr == nil {
			ps.anomalies["undefined:"+e.Text]++
			return nil
		}
		return ps.invoke(pr, p)
	case "alt":
		var out []vres
		seen := map[int]bool{}
		for _, k := range e.Kids {
			for _, v := range ps.evVal(k, p, cls) {
				if !seen[v.end] {
					seen[v.end] = true
					out = append(out, v)
				}
			}
		}
		return out
	default:
		var out []vres
		for _, r := range ps.ev(e, p, cls) {
			out = append(out, vres{r.end, Str(ps.tokenText(p, r.end))})
		}
		return out
	}
}

// ---------- driver ----------

func (ps *Parser) parse() (*Node, bool) {
	pr := ps.g.prods["RootNamespace"]
	for _, v := range ps.invoke(pr, 0) {
		if ps.skipRC(v.end) == len(ps.toks) {
			if n, ok := v.v.(*Node); ok {
				return n, true
			}
		}
	}
	return nil, false
}

func count(n *Node, m *meta, bad map[string]int) int {
	c := 1
	if _, ok := m.features[n.Class]; !ok {
		bad["class:"+n.Class]++
	}
	for _, p := range n.Props {
		if p.Name != "(xref)" && !m.hasFeature(n.Class, p.Name) {
			bad["prop:"+n.Class+"."+p.Name]++
		}
		if ch, ok := p.V.(*Node); ok {
			c += count(ch, m, bad)
		}
	}
	return c
}

func depthFromEnv() int {
	d := 6
	if v := os.Getenv("DEPTH"); v != "" {
		fmt.Sscan(v, &d)
	}
	return d
}

func dump(n *Node, depth, max int, sb *strings.Builder) {
	ind := strings.Repeat("  ", depth)
	fmt.Fprintf(sb, "%s%s\n", ind, n.Class)
	for _, p := range n.Props {
		switch v := p.V.(type) {
		case *Node:
			if depth+1 > max {
				fmt.Fprintf(sb, "%s  %s %s <%s ...>\n", ind, p.Name, p.Op, v.Class)
			} else {
				fmt.Fprintf(sb, "%s  %s %s\n", ind, p.Name, p.Op)
				dump(v, depth+2, max, sb)
			}
		default:
			fmt.Fprintf(sb, "%s  %s %s %v\n", ind, p.Name, p.Op, v)
		}
	}
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: spike3 <SysML.ecore> <file>... [-tree]")
		os.Exit(2)
	}
	m, err := loadEcore(os.Args[1])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	which := os.Getenv("GRAMMAR")
	if which == "" {
		which = "sysml"
	}
	g, err := loadGrammar("xtext-ir.json", which, m)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	xrefProps = map[string]bool{}
	for _, p := range g.prods {
		walk(p.Body, func(e *Expr) {
			if e.K == "assign" && e.Kids[0].K == "xref" {
				xrefProps[e.Text] = true
			}
		})
	}
	theMeta = m
	theMeta = m
	if os.Getenv("WORLD") != "" {
		worldMain()
		return
	}
	tree := false
	var files []string
	for _, a := range os.Args[2:] {
		if a == "-tree" {
			tree = true
		} else {
			files = append(files, a)
		}
	}
	fmt.Printf("grammar: %d productions, %d keywords, %d symbols; ecore: %d classes\n", len(g.prods), len(g.keywords), len(g.symbols), len(m.features))
	okc := 0
	cmp := newComparator()
	for _, f := range files {
		src, _ := os.ReadFile(f)
		toks, err := g.lex(string(src))
		short := f[strings.LastIndex(f, "/")+1:]
		if err != nil {
			fmt.Printf("LEXFAIL %-45s %v\n", short, err)
			continue
		}
		ps := &Parser{g: g, toks: toks, memo: map[memoKey][]res{}, vmemo: map[memoKey][]vres{}, busy: map[memoKey]bool{}, expected: map[string]bool{}, anomalies: map[string]int{}}
		n, ok := ps.parse()
		if ok {
			okc++
			bad := map[string]int{}
			nn := count(n, m, bad)
			fmt.Printf("OK      %-45s %d tokens, %d nodes, %d ecore mismatches, %d guards\n", short, len(toks), nn, len(bad), ps.guards)
			if len(bad) > 0 {
				var ks []string
				for k := range bad {
					ks = append(ks, k)
				}
				sort.Strings(ks)
				if len(ks) > 8 {
					ks = ks[:8]
				}
				fmt.Println("        mismatches:", ks)
			}
			if cmp != nil {
				cmp.compare(f, n)
			}
			if tree {
				var sb strings.Builder
				dump(n, 0, depthFromEnv(), &sb)
				fmt.Println(sb.String())
			}
		} else {
			at := ps.farthest
			line, col := 0, 0
			if at < len(toks) {
				line, col = toks[at].Line, toks[at].Col
			}
			var ex []string
			for k := range ps.expected {
				ex = append(ex, k)
			}
			sort.Strings(ex)
			if len(ex) > 400 {
				ex = append(ex[:400], "...")
			}
			got := "EOF"
			if at < len(toks) {
				got = toks[at].Text
			}
			fmt.Printf("FAIL    %-45s line %d:%d got %q expected %v\n", short, line, col, got, ex)
		}
		if len(ps.anomalies) > 0 && len(files) == 1 {
			fmt.Println("anomalies:", ps.anomalies)
		}
	}
	fmt.Printf("%d/%d parsed\n", okc, len(files))
	if cmp != nil {
		cmp.report()
	}
}
