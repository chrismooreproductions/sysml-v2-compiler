// SPDX-License-Identifier: EPL-2.0

package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// cn is a canonical element: a class, scalar attributes and role-labelled
// children, built from either our node tree or the oracle XMI so the two
// can be diffed structurally.
type cn struct {
	chain  []*cn // a feature chain the pilot generates: its chaining features, in order
	id     string
	key    string // position in the file: role[index] path, with the file name
	file   string
	parent *cn
	class  string
	attrs  map[string]string
	kids   []ckid
}

type ckid struct {
	role string
	n    *cn // nil for a cross-reference

	// cross-reference, ours
	text  string // a name written in the source
	owned int    // implied: index of an owned element, or -1
	owner bool   // implied: the owning element
	// cross-reference, oracle
	tgtFile, tgtID string
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var starRe = regexp.MustCompile(`(?m)^\s*\*+`)
var spaceRe = regexp.MustCompile(`\s+`)

// normScalar removes representation differences that carry no meaning:
// quoting of names and strings, and the comment delimiters around doc text.
func normScalar(key, v string) string {
	if key == "body" {
		v = strings.TrimPrefix(v, "/*")
		v = strings.TrimSuffix(v, "*/")
		v = starRe.ReplaceAllString(v, "")
		return strings.TrimSpace(spaceRe.ReplaceAllString(v, " "))
	}
	if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
		return v[1 : len(v)-1]
	}
	return v
}

func fromNode(n *Node) *cn {
	c := &cn{class: n.Class, attrs: map[string]string{}}
	for _, p := range n.Props {
		switch v := p.V.(type) {
		case *Node:
			c.kids = append(c.kids, ckid{role: p.Name, n: fromNode(v), owned: -1})
		case Ref:
			c.kids = append(c.kids, ckid{role: p.Name, text: string(v), owned: -1})
		case RefOwned:
			c.kids = append(c.kids, ckid{role: p.Name, owned: int(v)})
		case RefOwner:
			c.kids = append(c.kids, ckid{role: p.Name, owned: -1, owner: true})
		case Bool:
			c.attrs[p.Name] = fmt.Sprint(bool(v))
		case Str:
			val := normScalar(p.Name, string(v))
			if old, ok := c.attrs[p.Name]; ok {
				val = old + " " + val
			}
			c.attrs[p.Name] = val
		}
	}
	return c
}

// fileBase names a library file the way references to it do: no directory,
// no extension, URL escapes removed.
func fileBase(p string) string {
	p = p[strings.LastIndex(p, "/")+1:]
	p = strings.ReplaceAll(p, "%20", " ")
	if i := strings.LastIndex(p, "."); i >= 0 {
		p = p[:i]
	}
	return p
}

func loadXMI(path string) (*cn, error) {
	base := fileBase(path)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := xml.NewDecoder(f)
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	type frame struct {
		role string
		n    *cn // nil: a reference element
	}
	var stack []frame
	var root *cn
	for {
		t, err := dec.Token()
		if err != nil {
			break
		}
		switch e := t.(type) {
		case xml.StartElement:
			var typ, href, id string
			var localRefs [][2]string // role, target id
			parentID := ""
			if len(stack) > 0 && stack[len(stack)-1].n != nil {
				parentID = stack[len(stack)-1].n.id
			}
			ownType := ""
			for _, a := range e.Attr {
				if a.Name.Space == "http://www.omg.org/XMI" && a.Name.Local == "id" {
					id = a.Value
				}
				if a.Name.Local == "type" && a.Name.Space == "http://www.w3.org/2001/XMLSchema-instance" {
					ownType = a.Value[strings.Index(a.Value, ":")+1:]
				}
			}
			attrs := map[string]string{}
			for _, a := range e.Attr {
				switch {
				case a.Name.Space == "xmlns" || a.Name.Local == "xmlns" || a.Name.Space == "http://www.omg.org/XMI":
				case a.Name.Local == "type" && a.Name.Space == "http://www.w3.org/2001/XMLSchema-instance":
					typ = a.Value
				case a.Name.Local == "href":
					href = a.Value
				case a.Name.Local == "elementId":
				case uuidRe.MatchString(strings.Fields(a.Value + " x")[0]) && xrefProps[a.Name.Local]:
					for _, ref := range strings.Fields(a.Value) {
						// a reference to the owning element is the inverse end of
						// containment, not a cross-reference written in the source
						if uuidRe.MatchString(ref) && (ref != parentID || !theMeta.isSourceEnd(ownType, a.Name.Local)) {
							localRefs = append(localRefs, [2]string{a.Name.Local, ref})
						}
					}
				case uuidRe.MatchString(a.Value):
				default:
					attrs[a.Name.Local] = normScalar(a.Name.Local, a.Value)
				}
			}
			if len(stack) == 0 {
				root = &cn{id: id, class: e.Name.Local, attrs: attrs}
				for _, r := range localRefs {
					root.kids = append(root.kids, ckid{role: r[0], tgtFile: base, tgtID: r[1], owned: -1})
				}
				stack = append(stack, frame{"", root})
				continue
			}
			parent := stack[len(stack)-1].n
			if href != "" { // an external element, whether or not it carries an xsi:type hint
				if parent != nil {
					file, id := base, ""
					if i := strings.Index(href, "#"); i >= 0 {
						id = href[i+1:]
						if i > 0 {
							file = fileBase(href[:i])
						}
					}
					parent.kids = append(parent.kids, ckid{role: e.Name.Local, tgtFile: file, tgtID: id, owned: -1})
				}
				stack = append(stack, frame{e.Name.Local, nil})
				continue
			}
			if i := strings.Index(typ, ":"); i >= 0 {
				typ = typ[i+1:]
			}
			c := &cn{id: id, class: typ, attrs: attrs}
			for _, r := range localRefs {
				c.kids = append(c.kids, ckid{role: r[0], tgtFile: base, tgtID: r[1], owned: -1})
			}
			if parent != nil {
				parent.kids = append(parent.kids, ckid{role: e.Name.Local, n: c, owned: -1})
			}
			stack = append(stack, frame{e.Name.Local, c})
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	if root == nil {
		return nil, fmt.Errorf("empty XMI")
	}
	return root, nil
}

var xrefProps map[string]bool
var theMeta *meta

// prune drops what the oracle never serializes or serializes as a default:
// derived/transient features and attributes equal to their default.
func prune(c *cn) {
	for k, v := range c.attrs {
		fi, ok := theMeta.feature(c.class, k)
		if ok && (fi.derived || theMeta.effectiveRole(c.class, k) != k) {
			delete(c.attrs, k)
			continue
		}
		def := fi.def
		if def == "" && ok {
			switch {
			case strings.HasSuffix(fi.typ, "EBoolean"):
				def = "false"
			case strings.HasSuffix(fi.typ, "EInt"), strings.HasSuffix(fi.typ, "ELong"), strings.HasSuffix(fi.typ, "EDouble"):
				def = "0"
			}
		}
		if ok && def != "" && sameScalar(v, def) {
			delete(c.attrs, k)
		}
	}
	var kept []ckid
	for _, k := range c.kids {
		if fi, ok := theMeta.feature(c.class, k.role); ok && (fi.derived || theMeta.effectiveRole(c.class, k.role) != k.role) {
			continue
		}
		if k.n != nil {
			prune(k.n)
		}
		kept = append(kept, k)
	}
	c.kids = kept
}

func sameScalar(a, b string) bool {
	if a == b {
		return true
	}
	x, e1 := strconv.ParseFloat(a, 64)
	y, e2 := strconv.ParseFloat(b, 64)
	return e1 == nil && e2 == nil && x == y
}

type diff struct{ path, kind, detail string }

// sig names a child by its class and, for a relationship, the class of the
// element it owns, so a count mismatch can say what is missing.
func sig(n *cn) string {
	if n == nil {
		return "ref"
	}
	for _, k := range n.kids {
		if k.role == "ownedRelatedElement" && k.n != nil {
			return n.class + ">" + k.n.class
		}
	}
	return n.class
}

// sigDiff summarizes which children only one side has, as " +oracle[...] -oracle[...]".
func sigDiff(mine, oracle []*cn) string {
	cnt := map[string]int{}
	for _, m := range mine {
		cnt[sig(m)]--
	}
	for _, o := range oracle {
		cnt[sig(o)]++
	}
	var keys []string
	for k, v := range cnt {
		if v != 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		if cnt[k] > 0 {
			fmt.Fprintf(&sb, " oracle-only:%s", k)
		} else {
			fmt.Fprintf(&sb, " ours-only:%s", k)
		}
	}
	return sb.String()
}

func groupKids(c *cn) (map[string][]*cn, []string) {
	g := map[string][]*cn{}
	var order []string
	for _, k := range c.kids {
		if _, ok := g[k.role]; !ok {
			order = append(order, k.role)
		}
		g[k.role] = append(g[k.role], k.n)
	}
	return g, order
}

// diffCN compares ours (a) against the oracle (b).
func diffCN(path string, a, b *cn, out *[]diff) {
	if a.class != b.class {
		*out = append(*out, diff{path, "class", a.class + " vs oracle " + b.class})
		return
	}
	here := path + "/" + a.class
	keys := map[string]bool{}
	for k := range a.attrs {
		keys[k] = true
	}
	for k := range b.attrs {
		keys[k] = true
	}
	var ks []string
	for k := range keys {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		av, aok := a.attrs[k]
		bv, bok := b.attrs[k]
		switch {
		case aok && !bok:
			*out = append(*out, diff{here, "attr-extra:" + k, av})
		case !aok && bok:
			*out = append(*out, diff{here, "attr-missing:" + k, bv})
		case !sameScalar(av, bv):
			*out = append(*out, diff{here, "attr-value:" + k, fmt.Sprintf("%q vs oracle %q", av, bv)})
		}
	}
	ag, ao := groupKids(a)
	bg, bo := groupKids(b)
	seen := map[string]bool{}
	var roles []string
	for _, r := range append(append([]string{}, bo...), ao...) {
		if !seen[r] {
			seen[r] = true
			roles = append(roles, r)
		}
	}
	for _, r := range roles {
		x, y := ag[r], bg[r]
		if len(x) != len(y) {
			*out = append(*out, diff{here, "kids-count:" + r + sigDiff(x, y), fmt.Sprintf("%d vs oracle %d", len(x), len(y))})
		}
		for i := 0; i < len(x) && i < len(y); i++ {
			if x[i] == nil || y[i] == nil {
				if (x[i] == nil) != (y[i] == nil) {
					*out = append(*out, diff{here, "ref-vs-node:" + r, ""})
				}
				continue
			}
			diffCN(fmt.Sprintf("%s.%s[%d]", here, r, i), x[i], y[i], out)
		}
	}
}

type comparator struct {
	libRoot, xmiRoot string
	showFiles        int
	exact, total     int
	missing          int
	kinds            map[string]int
	samples          map[string][]string
	firstDiffs       []string
}

func newComparator() *comparator {
	lr, xr := os.Getenv("LIB_ROOT"), os.Getenv("XMI_ROOT")
	if lr == "" || xr == "" {
		return nil
	}
	return &comparator{libRoot: lr, xmiRoot: xr, kinds: map[string]int{}, samples: map[string][]string{}, showFiles: 0}
}

func (c *comparator) compare(file string, n *Node) {
	rel := strings.TrimPrefix(file, c.libRoot+"/")
	oracle := c.xmiRoot + "/" + rel + "x"
	want, err := loadXMI(oracle)
	if err != nil {
		c.missing++
		fmt.Printf("        oracle: %v\n", err)
		return
	}
	var ds []diff
	if os.Getenv("NO_SEMANTIC") == "" {
		n = transform(postProcess(n), ctx{})
	}
	mine := fromNode(n)
	prune(mine)
	prune(want)
	diffCN("", mine, want, &ds)
	c.total++
	if len(ds) == 0 {
		c.exact++
		fmt.Println("        oracle: EXACT MATCH")
		return
	}
	fmt.Printf("        oracle: %d differences\n", len(ds))
	for _, d := range ds {
		kind := d.kind
		cls := d.path[strings.LastIndex(d.path, "/")+1:]
		if i := strings.Index(cls, "."); i >= 0 {
			cls = cls[:i]
		}
		cat := kind + " @ " + cls
		c.kinds[cat]++
		if len(c.samples[cat]) < 3 {
			c.samples[cat] = append(c.samples[cat], fmt.Sprintf("%s | %s | %s", rel[strings.LastIndex(rel, "/")+1:], d.detail, d.path[max(0, len(d.path)-110):]))
		}
	}
	if c.showFiles > 0 {
		c.showFiles--
		for i, d := range ds {
			if i >= 12 {
				break
			}
			fmt.Printf("          %s  %s  %s\n", d.kind, d.detail, d.path[max(0, len(d.path)-90):])
		}
	}
}

func (c *comparator) report() {
	fmt.Printf("\noracle comparison: %d files compared, %d exact matches, %d without an oracle file\n", c.total, c.exact, c.missing)
	type kv struct {
		k string
		v int
	}
	var all []kv
	sum := 0
	for k, v := range c.kinds {
		all = append(all, kv{k, v})
		sum += v
	}
	sort.Slice(all, func(i, j int) bool { return all[i].v > all[j].v || (all[i].v == all[j].v && all[i].k < all[j].k) })
	fmt.Printf("%d differences in %d categories; top 400:\n", sum, len(all))
	for i, e := range all {
		if i >= 400 {
			break
		}
		fmt.Printf("  %6d  %s\n", e.v, e.k)
		if os.Getenv("SAMPLES") != "" {
			for _, sm := range c.samples[e.k] {
				fmt.Printf("            %s\n", sm)
			}
		}
	}
}
