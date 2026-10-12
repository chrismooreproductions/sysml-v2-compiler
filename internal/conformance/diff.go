// SPDX-License-Identifier: EPL-2.0

package conformance

import (
	"fmt"
	"sort"
	"strings"
)

// Diff is one difference between our canonical element and the oracle's.
type Diff struct {
	Path   string // the chain of metaclasses and roles from the root
	Kind   string // class, attr-missing:x, attr-extra:x, attr-value:x, kids-count:role...
	Detail string
}

// Compare lists the differences between ours and the oracle's element. Where
// the children for a role differ in number it says which sorts of child one
// side lacks.
func Compare(ours, oracle *Elem) []Diff {
	var out []Diff
	compare("", ours, oracle, &out)
	return out
}

func compare(path string, a, b *Elem, out *[]Diff) {
	if a.Class != b.Class {
		*out = append(*out, Diff{path, "class", a.Class + " vs oracle " + b.Class})
		return
	}
	here := path + "/" + a.Class
	keys := map[string]bool{}
	for k := range a.Attrs {
		keys[k] = true
	}
	for k := range b.Attrs {
		keys[k] = true
	}
	var ks []string
	for k := range keys {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		av, aok := a.Attrs[k]
		bv, bok := b.Attrs[k]
		switch {
		case aok && !bok:
			*out = append(*out, Diff{here, "attr-extra:" + k, av})
		case !aok && bok:
			*out = append(*out, Diff{here, "attr-missing:" + k, bv})
		case !sameScalar(av, bv):
			*out = append(*out, Diff{here, "attr-value:" + k, fmt.Sprintf("%q vs oracle %q", av, bv)})
		}
	}
	ag, ao := group(a)
	bg, bo := group(b)
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
			*out = append(*out, Diff{here, "kids-count:" + r + sigDiff(x, y), fmt.Sprintf("%d vs oracle %d", len(x), len(y))})
		}
		for i := 0; i < len(x) && i < len(y); i++ {
			switch {
			case x[i] == nil && y[i] == nil:
			case x[i] == nil || y[i] == nil:
				*out = append(*out, Diff{here, "ref-vs-element:" + r, ""})
			default:
				compare(fmt.Sprintf("%s.%s[%d]", here, r, i), x[i], y[i], out)
			}
		}
	}
}

// group collects the children of an element by role; a reference is nil.
func group(c *Elem) (map[string][]*Elem, []string) {
	g := map[string][]*Elem{}
	var order []string
	for _, k := range c.Kids {
		if _, ok := g[k.Role]; !ok {
			order = append(order, k.Role)
		}
		g[k.Role] = append(g[k.Role], k.Elem)
	}
	return g, order
}

// sig names a child by its class and, for a relationship, the class of the
// element it owns, so a count mismatch can say what is missing.
func sig(n *Elem) string {
	if n == nil {
		return "ref"
	}
	for _, k := range n.Kids {
		if k.Role == "ownedRelatedElement" && k.Elem != nil {
			return n.Class + ">" + k.Elem.Class
		}
	}
	return n.Class
}

func sigDiff(ours, oracle []*Elem) string {
	count := map[string]int{}
	for _, m := range ours {
		count[sig(m)]--
	}
	for _, o := range oracle {
		count[sig(o)]++
	}
	var keys []string
	for k, v := range count {
		if v != 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		if count[k] > 0 {
			fmt.Fprintf(&sb, " oracle-only:%s", k)
		} else {
			fmt.Fprintf(&sb, " ours-only:%s", k)
		}
	}
	return sb.String()
}

// Summary counts differences by kind and the class they occur in, most
// frequent first.
func Summary(diffs []Diff) []string {
	counts := map[string]int{}
	for _, d := range diffs {
		cls := d.Path[strings.LastIndex(d.Path, "/")+1:]
		if i := strings.Index(cls, "."); i >= 0 {
			cls = cls[:i]
		}
		counts[d.Kind+" @ "+cls]++
	}
	var keys []string
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return counts[keys[i]] > counts[keys[j]] || (counts[keys[i]] == counts[keys[j]] && keys[i] < keys[j])
	})
	var out []string
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%6d  %s", counts[k], k))
	}
	return out
}
