// spike0 parses the OMG .kebnf grammar files into an IR and reports what a
// grammar-driven parser would have to cope with.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// ---- IR ----

type Expr struct {
	K    string  `json:"k"`              // alt seq rep term ref assign action
	Text string  `json:"text,omitempty"` // term text, ref name, assign property
	Op   string  `json:"op,omitempty"`   // rep: ? * + ; assign: = += ?=
	Kids []*Expr `json:"kids,omitempty"`
}

type Prod struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
	Body *Expr  `json:"body"`
	Line int    `json:"line"`
	File string `json:"file"`
}

// ---- tokenizer ----

type tok struct {
	kind string // str id sym
	text string
	line int
	bol  bool
}

func lex(src, file string) ([]tok, error) {
	var out []tok
	line, col := 1, 0
	atLineStart := true
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '\n':
			line++
			col = 0
			atLineStart = true
			i++
		case c == ' ' || c == '\t' || c == '\r':
			col++
			i++
			atLineStart = atLineStart && false
			_ = col
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '\'':
			j := i + 1
			for j < len(src) && src[j] != '\'' {
				if src[j] == '\\' {
					j++
				}
				if src[j] == '\n' {
					return nil, fmt.Errorf("%s:%d: unterminated string", file, line)
				}
				j++
			}
			out = append(out, tok{"str", src[i+1 : j], line, col == 0})
			col += j - i + 1
			i = j + 1
		case c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z'):
			j := i
			for j < len(src) && (src[j] == '_' || (src[j] >= 'A' && src[j] <= 'Z') || (src[j] >= 'a' && src[j] <= 'z') || (src[j] >= '0' && src[j] <= '9')) {
				j++
			}
			out = append(out, tok{"id", src[i:j], line, col == 0})
			col += j - i
			i = j
		default:
			two := ""
			if i+1 < len(src) {
				two = src[i : i+2]
			}
			if two == "+=" || two == "?=" {
				out = append(out, tok{"sym", two, line, col == 0})
				i += 2
				col += 2
				continue
			}
			if strings.ContainsRune("=:|()?*+{}[].,~", rune(c)) {
				out = append(out, tok{"sym", string(c), line, col == 0})
				i++
				col++
				continue
			}
			return nil, fmt.Errorf("%s:%d: unexpected character %q", file, line, c)
		}
	}
	return out, nil
}

// ---- parser ----

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

// bodyEnds reports whether the next token begins a new production.
func (p *parser) bodyEnds() bool {
	t := p.peek()
	return t == nil || (t.kind == "id" && t.bol)
}

func (p *parser) errf(f string, a ...any) error {
	l := 0
	if t := p.peek(); t != nil {
		l = t.line
	} else if len(p.t) > 0 {
		l = p.t[len(p.t)-1].line
	}
	return fmt.Errorf("%s:%d: %s", p.file, l, fmt.Sprintf(f, a...))
}

func (p *parser) parseFile() ([]*Prod, error) {
	var prods []*Prod
	for p.peek() != nil {
		t := p.peek()
		if t.kind != "id" || !t.bol {
			return nil, p.errf("expected production name at line start, got %q", t.text)
		}
		pr := &Prod{Name: t.text, Line: t.line, File: p.file}
		p.i++
		if s := p.peek(); s != nil && s.kind == "sym" && s.text == ":" {
			p.i++
			ty := p.peek()
			if ty == nil || ty.kind != "id" {
				return nil, p.errf("expected type name after ':'")
			}
			pr.Type = ty.text
			p.i++
		}
		if s := p.peek(); s == nil || s.text != "=" || s.kind != "sym" {
			return nil, p.errf("expected '=' after production %s", pr.Name)
		}
		p.i++
		body, err := p.alt()
		if err != nil {
			return nil, err
		}
		if !p.bodyEnds() {
			return nil, p.errf("unexpected %q in body of %s", p.peek().text, pr.Name)
		}
		pr.Body = body
		prods = append(prods, pr)
	}
	return prods, nil
}

func (p *parser) isSym(s string) bool {
	t := p.peek()
	return t != nil && t.kind == "sym" && t.text == s
}

func (p *parser) alt() (*Expr, error) {
	if p.isSym("|") { // tolerate leading bar
		p.i++
	}
	var alts []*Expr
	for {
		s, err := p.seq()
		if err != nil {
			return nil, err
		}
		alts = append(alts, s)
		if p.isSym("|") {
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
	for !p.bodyEnds() && !p.isSym("|") && !p.isSym(")") && !p.isSym("}") {
		it, err := p.item()
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	if len(items) == 0 {
		return nil, p.errf("empty sequence")
	}
	if len(items) == 1 {
		return items[0], nil
	}
	return &Expr{K: "seq", Kids: items}, nil
}

func (p *parser) item() (*Expr, error) {
	e, err := p.primary()
	if err != nil {
		return nil, err
	}
	for p.isSym("?") || p.isSym("*") || p.isSym("+") {
		e = &Expr{K: "rep", Op: p.peek().text, Kids: []*Expr{e}}
		p.i++
	}
	return e, nil
}

func (p *parser) primary() (*Expr, error) {
	t := p.peek()
	if t == nil {
		return nil, p.errf("unexpected end of input")
	}
	switch {
	case t.kind == "str":
		p.i++
		return &Expr{K: "term", Text: t.text}, nil
	case t.kind == "id":
		p.i++
		// dotted property path (only seen in action blocks)
		for p.isSym(".") {
			p.i++
			n := p.peek()
			if n == nil || n.kind != "id" {
				return nil, p.errf("expected name after '.'")
			}
			t = &tok{kind: "id", text: t.text + "." + n.text, line: t.line}
			p.i++
		}
		if n := p.peek(); n != nil && n.kind == "sym" && (n.text == "=" || n.text == "+=" || n.text == "?=") && !n.bol {
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
		if !p.isSym(")") {
			return nil, p.errf("expected ')'")
		}
		p.i++
		return e, nil
	case t.kind == "sym" && (t.text == "[" || t.text == "~"):
		op := ""
		if t.text == "~" {
			op = "~"
			p.i++
			if !p.isSym("[") {
				return nil, p.errf("expected '[' after '~'")
			}
		}
		p.i++
		n := p.peek()
		if n == nil || n.kind != "id" {
			return nil, p.errf("expected name in cross-reference")
		}
		p.i++
		if !p.isSym("]") {
			return nil, p.errf("expected ']'")
		}
		p.i++
		return &Expr{K: "xref", Text: n.text, Op: op}, nil
	case t.kind == "sym" && t.text == "{":
		p.i++
		act := &Expr{K: "action"}
		for !p.isSym("}") {
			if p.peek() == nil {
				return nil, p.errf("unterminated action")
			}
			it, err := p.item()
			if err != nil {
				return nil, err
			}
			// inside an action, bare identifiers on an assignment's right
			// side name properties/literals, not productions
			if it.K == "assign" && it.Kids[0].K == "ref" {
				it.Kids[0].K = "propref"
			}
			act.Kids = append(act.Kids, it)
		}
		p.i++
		return act, nil
	}
	return nil, p.errf("unexpected %q", t.text)
}

// ---- analysis ----

type grammar struct {
	prods  map[string]*Prod
	order  []string
	classy map[string]bool // lexical productions treated as opaque tokens
}

var upper = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func walk(e *Expr, f func(*Expr)) {
	if e == nil {
		return
	}
	f(e)
	for _, k := range e.Kids {
		walk(k, f)
	}
}

func newGrammar(all []*Prod) *grammar {
	g := &grammar{prods: map[string]*Prod{}, classy: map[string]bool{}}
	for _, p := range all {
		if _, dup := g.prods[p.Name]; !dup {
			g.order = append(g.order, p.Name)
		}
		g.prods[p.Name] = p
	}
	regexish := regexp.MustCompile(`[\[\]\\*()^]|#x|\.\*`)
	for ch := true; ch; {
		ch = false
		for _, n := range g.order {
			if !upper.MatchString(n) || g.classy[n] {
				continue
			}
			c := false
			walk(g.prods[n].Body, func(e *Expr) {
				if e.K == "term" && regexish.MatchString(e.Text) {
					c = true
				}
				if e.K == "ref" && g.classy[e.Text] {
					c = true
				}
			})
			if c {
				g.classy[n] = true
				ch = true
			}
		}
	}
	return g
}

type set map[string]bool

func (s set) add(o set) bool {
	ch := false
	for k := range o {
		if !s[k] {
			s[k] = true
			ch = true
		}
	}
	return ch
}

// firstNull computes FIRST sets (terminals; classy lexicals are opaque
// "<NAME>" tokens) and nullability per production, to fixpoint.
func (g *grammar) firstNull() (map[string]set, map[string]bool) {
	first := map[string]set{}
	null := map[string]bool{}
	for _, n := range g.order {
		first[n] = set{}
	}
	var fe func(e *Expr) (set, bool)
	fe = func(e *Expr) (set, bool) {
		switch e.K {
		case "term":
			return set{"'" + e.Text + "'": true}, false
		case "ref":
			if g.classy[e.Text] {
				return set{"<" + e.Text + ">": true}, false
			}
			if _, ok := g.prods[e.Text]; !ok {
				return set{"?" + e.Text: true}, false
			}
			return first[e.Text], null[e.Text]
		case "xref":
			return set{"[" + e.Text + "]": true}, false
		case "action":
			return set{}, true
		case "assign":
			return fe(e.Kids[0])
		case "rep":
			s, n := fe(e.Kids[0])
			return s, n || e.Op != "+"
		case "alt":
			s, n := set{}, false
			for _, k := range e.Kids {
				ks, kn := fe(k)
				s.add(ks)
				n = n || kn
			}
			return s, n
		case "seq":
			s, n := set{}, true
			for _, k := range e.Kids {
				ks, kn := fe(k)
				s.add(ks)
				if !kn {
					n = false
					break
				}
			}
			return s, n
		}
		return set{}, false
	}
	for ch := true; ch; {
		ch = false
		for _, n := range g.order {
			if g.classy[n] {
				continue
			}
			s, nl := fe(g.prods[n].Body)
			if first[n].add(s) {
				ch = true
			}
			if nl && !null[n] {
				null[n] = true
				ch = true
			}
		}
	}
	return first, null
}

// leftRefs: nonterminals reachable in leftmost position (through nullable prefixes).
func (g *grammar) leftRefs(null map[string]bool) map[string]set {
	lr := map[string]set{}
	var le func(e *Expr, out set) bool // returns nullable
	le = func(e *Expr, out set) bool {
		switch e.K {
		case "term":
			return false
		case "ref":
			if !g.classy[e.Text] {
				if _, ok := g.prods[e.Text]; ok {
					out[e.Text] = true
					return null[e.Text]
				}
			}
			return false
		case "xref":
			return false
		case "action":
			return true
		case "assign":
			return le(e.Kids[0], out)
		case "rep":
			n := le(e.Kids[0], out)
			return n || e.Op != "+"
		case "alt":
			n := false
			for _, k := range e.Kids {
				if le(k, out) {
					n = true
				}
			}
			return n
		case "seq":
			for _, k := range e.Kids {
				if !le(k, out) {
					return false
				}
			}
			return true
		}
		return false
	}
	for _, n := range g.order {
		s := set{}
		le(g.prods[n].Body, s)
		lr[n] = s
	}
	return lr
}

func sorted(s set) []string {
	var o []string
	for k := range s {
		o = append(o, k)
	}
	sort.Strings(o)
	return o
}

func main() {
	files := os.Args[1:]
	var all []*Prod
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		toks, err := lex(string(b), f)
		if err != nil {
			fmt.Println("LEX ERROR:", err)
			os.Exit(1)
		}
		p := &parser{t: toks, file: f}
		prods, err := p.parseFile()
		if err != nil {
			fmt.Println("PARSE ERROR:", err)
			os.Exit(1)
		}
		fmt.Printf("%s: %d productions, %d tokens\n", f[strings.LastIndex(f, "/")+1:], len(prods), len(toks))
		all = append(all, prods...)
	}
	js, _ := json.MarshalIndent(all, "", " ")
	os.WriteFile("ir.json", js, 0o644)

	g := newGrammar(all)
	fmt.Printf("total %d productions, %d unique names, %d opaque lexical tokens\n", len(all), len(g.order), len(g.classy))

	// duplicates across files
	seen := map[string]string{}
	var dups []string
	for _, p := range all {
		if f, ok := seen[p.Name]; ok && f != p.File {
			dups = append(dups, p.Name)
		}
		seen[p.Name] = p.File
	}
	fmt.Printf("names defined in both files: %d\n", len(dups))

	// undefined refs
	undef := set{}
	used := set{}
	for _, p := range all {
		walk(p.Body, func(e *Expr) {
			if e.K == "ref" {
				used[e.Text] = true
				if _, ok := g.prods[e.Text]; !ok {
					undef[e.Text] = true
				}
			}
		})
	}
	fmt.Println("undefined references:", sorted(undef))
	var unused []string
	for _, n := range g.order {
		if !used[n] {
			unused = append(unused, n)
		}
	}
	fmt.Printf("never referenced (%d, likely entry points): %v\n", len(unused), unused)

	first, null := g.firstNull()
	var nl []string
	for n := range null {
		nl = append(nl, n)
	}
	sort.Strings(nl)
	fmt.Printf("nullable productions (%d): %v\n", len(nl), nl)

	lr := g.leftRefs(null)
	var lrec []string
	for _, n := range g.order {
		// find a cycle n -> ... -> n
		type st struct {
			n    string
			path []string
		}
		q := []st{{n, []string{n}}}
		vis := set{}
		found := ""
		for len(q) > 0 && found == "" {
			c := q[0]
			q = q[1:]
			for _, nx := range sorted(lr[c.n]) {
				if nx == n {
					found = strings.Join(append(c.path, n), " -> ")
					break
				}
				if !vis[nx] {
					vis[nx] = true
					q = append(q, st{nx, append(append([]string{}, c.path...), nx)})
				}
			}
		}
		if found != "" {
			lrec = append(lrec, found)
		}
	}
	fmt.Printf("left-recursive productions: %d\n", len(lrec))
	for _, l := range lrec {
		fmt.Println("  ", l)
	}

	// 1-token-lookahead conflicts between alternatives
	type conf struct {
		prod, a, b string
		toks       []string
	}
	var confs []conf
	conflictAlts, totalAlts := 0, 0
	var fe func(e *Expr) (set, bool)
	fe = func(e *Expr) (set, bool) {
		switch e.K {
		case "term":
			return set{"'" + e.Text + "'": true}, false
		case "ref":
			if g.classy[e.Text] {
				return set{"<" + e.Text + ">": true}, false
			}
			if _, ok := g.prods[e.Text]; !ok {
				return set{}, false
			}
			return first[e.Text], null[e.Text]
		case "xref":
			return set{"[" + e.Text + "]": true}, false
		case "action":
			return set{}, true
		case "assign":
			return fe(e.Kids[0])
		case "rep":
			s, n := fe(e.Kids[0])
			return s, n || e.Op != "+"
		case "alt":
			s, n := set{}, false
			for _, k := range e.Kids {
				ks, kn := fe(k)
				s.add(ks)
				n = n || kn
			}
			return s, n
		case "seq":
			s, n := set{}, true
			for _, k := range e.Kids {
				ks, kn := fe(k)
				s.add(ks)
				if !kn {
					n = false
					break
				}
			}
			return s, n
		}
		return set{}, false
	}
	for _, name := range g.order {
		if g.classy[name] || upper.MatchString(name) {
			continue
		}
		walk(g.prods[name].Body, func(e *Expr) {
			if e.K != "alt" {
				return
			}
			totalAlts++
			bad := false
			for i := 0; i < len(e.Kids); i++ {
				for j := i + 1; j < len(e.Kids); j++ {
					a, _ := fe(e.Kids[i])
					b, _ := fe(e.Kids[j])
					var both []string
					for k := range a {
						if b[k] {
							both = append(both, k)
						}
					}
					if len(both) > 0 {
						sort.Strings(both)
						confs = append(confs, conf{name, fmt.Sprint(i), fmt.Sprint(j), both})
						bad = true
					}
				}
			}
			if bad {
				conflictAlts++
			}
		})
	}
	fmt.Printf("syntax alternations: %d, with 1-token FIRST conflicts: %d\n", totalAlts, conflictAlts)
	cp := map[string]int{}
	for _, c := range confs {
		cp[c.prod]++
	}
	var cn []string
	for k := range cp {
		cn = append(cn, k)
	}
	sort.Slice(cn, func(i, j int) bool { return cp[cn[i]] > cp[cn[j]] || (cp[cn[i]] == cp[cn[j]] && cn[i] < cn[j]) })
	fmt.Printf("productions containing conflicts: %d; worst 15:\n", len(cn))
	for i, n := range cn {
		if i >= 15 {
			break
		}
		fmt.Printf("   %-45s %d conflicting pairs\n", n, cp[n])
	}

	// metamodel cross-check
	crossCheck(all)
}

func crossCheck(all []*Prod) {
	home := os.Getenv("HOME")
	pil := home + "/repos/sysml-pilot-implementation/"
	classRe := regexp.MustCompile(`<eClassifiers[^>]*name="([^"]+)"`)
	featRe := regexp.MustCompile(`<eStructuralFeatures[^>]*name="([^"]+)"`)
	classes, feats := set{}, set{}
	for _, f := range []string{"org.omg.sysml/model/kerml.ecore", "org.omg.sysml/model/SysML.ecore"} {
		b, err := os.ReadFile(pil + f)
		if err != nil {
			fmt.Println("ecore read:", err)
			return
		}
		for _, m := range classRe.FindAllStringSubmatch(string(b), -1) {
			classes[m[1]] = true
		}
		for _, m := range featRe.FindAllStringSubmatch(string(b), -1) {
			feats[m[1]] = true
		}
	}
	fmt.Printf("\necore: %d classifiers, %d distinct feature names\n", len(classes), len(feats))

	types, props := set{}, set{}
	for _, p := range all {
		if p.Type != "" {
			types[p.Type] = true
		}
		walk(p.Body, func(e *Expr) {
			if e.K == "assign" {
				props[e.Text] = true
			}
		})
	}
	var missT, missP []string
	for t := range types {
		if !classes[t] {
			missT = append(missT, t)
		}
	}
	for t := range props {
		if !feats[t] {
			missP = append(missP, t)
		}
	}
	sort.Strings(missT)
	sort.Strings(missP)
	fmt.Printf("kebnf metaclass names after ':' : %d distinct; not in ecore: %v\n", len(types), missT)
	fmt.Printf("kebnf assigned property names: %d distinct; not in ecore: %v\n", len(props), missP)
	// production names that are themselves metaclasses (implicit type)
	implicit := 0
	for _, p := range all {
		if p.Type == "" && classes[p.Name] {
			implicit++
		}
	}
	fmt.Printf("untyped productions whose own name is an ecore class (implicit metaclass): %d\n", implicit)

	// xtext cross-check
	rule := regexp.MustCompile(`^(?:fragment\s+)?(?:terminal\s+)?([A-Za-z_]\w*)\s*(?:returns\s+[\w:.]+)?\s*(?:hidden\([^)]*\))?\s*:`)
	xr := set{}
	for _, f := range []string{
		"org.omg.kerml.xtext/src/org/omg/kerml/xtext/KerML.xtext",
		"org.omg.sysml.xtext/src/org/omg/sysml/xtext/SysML.xtext",
		"org.omg.kerml.expressions.xtext/src/org/omg/kerml/expressions/xtext/KerMLExpressions.xtext"} {
		b, _ := os.ReadFile(pil + f)
		for _, l := range strings.Split(string(b), "\n") {
			if m := rule.FindStringSubmatch(l); m != nil {
				xr[m[1]] = true
			}
		}
	}
	kn := set{}
	for _, p := range all {
		kn[p.Name] = true
	}
	var onlyK, onlyX []string
	for n := range kn {
		if !xr[n] && !upper.MatchString(n) {
			onlyK = append(onlyK, n)
		}
	}
	for n := range xr {
		if !kn[n] {
			onlyX = append(onlyX, n)
		}
	}
	sort.Strings(onlyK)
	sort.Strings(onlyX)
	fmt.Printf("\nxtext rules: %d; kebnf productions: %d\n", len(xr), len(kn))
	fmt.Printf("syntax productions only in kebnf (%d): %v\n", len(onlyK), onlyK)
	fmt.Printf("rules only in xtext (%d): %v\n", len(onlyX), onlyX)
}
