package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// The implied-relationships oracle (sysml.library.xmi.implied) is the same
// serialization with the relationships the pilot's adapters add made explicit
// (isImplied="true"), plus the elements those relationships need. This
// harness compares, element by element, the implied generalizations our
// rules produce with the oracle's.

// skipGenerated reports whether a node exists only in the implied
// serialization: an implied relationship, or a membership holding only
// generated content.
func (w *world) skipGenerated(c *cn) bool {
	if c.attrs["isImplied"] == "true" {
		return true
	}
	if w.is(c, "Membership") {
		o := w.ownedRelated(c)
		return len(o) > 0 && w.skipGenerated(o[0])
	}
	return false
}

// assignPlainKeys keys the implied tree by position among the nodes that also
// exist in the plain serialization, so its elements line up with ours.
func (w *world) assignPlainKeys(c *cn, file, path string, parent *cn, byKey map[string]*cn) {
	c.file, c.key, c.parent = file, file+"|"+path, parent
	byKey[c.key] = c
	counts := map[string]int{}
	for i := range c.kids {
		k := c.kids[i]
		if k.n == nil {
			continue
		}
		if w.skipGenerated(k.n) {
			k.n.parent, k.n.key, k.n.file = c, "", file
			continue
		}
		idx := counts[k.role]
		counts[k.role]++
		w.assignPlainKeys(k.n, file, path+"/"+k.role+"["+fmt.Sprint(idx)+"]", c, byKey)
	}
}

// impliedGeneralizations are the classes of implied relationship this
// harness compares.
var impliedClasses = []string{"Subclassification", "FeatureTyping", "Subsetting", "Redefinition", "CrossSubsetting"}

type impl struct{ class, target string }

func (w *world) targetKey(c *cn) string {
	if c == nil {
		return "(none)"
	}
	if c.key == "" {
		return "(generated " + c.class + ")"
	}
	return c.key
}

// oracleImplied lists an element's implied generalizations in the implied
// serialization.
func (w *world) oracleImplied(e *cn, byID map[string]*cn) []impl {
	var out []impl
	for _, r := range w.rels(e, "") {
		if r.attrs["isImplied"] != "true" {
			continue
		}
		cls := ""
		for _, c := range impliedClasses {
			if r.class == c {
				cls = c
			}
		}
		if cls == "" {
			continue
		}
		var target *cn
		for _, k := range r.kids {
			if k.n == nil && theMeta.isTargetEnd(r.class, k.role) {
				target = byID[k.tgtFile+"|"+k.tgtID]
				break
			}
		}
		out = append(out, impl{cls, w.targetKey(target)})
	}
	return out
}

// ourImplied lists the implied generalizations our rules give an element.
func (w *world) ourImplied(e *cn) []impl {
	var out []impl
	if !w.is(e, "Type") {
		return out
	}
	for _, x := range w.implicit(e) {
		out = append(out, impl{x.kind, w.targetKey(x.target)})
	}
	return out
}

func (w *world) impliedReport(impliedRoots map[string]*cn, impliedByID map[string]*cn) {
	type row struct{ expected, matched, extra int }
	byKind := map[string]*row{}
	get := func(k string) *row {
		if byKind[k] == nil {
			byKind[k] = &row{}
		}
		return byKind[k]
	}
	var misses, extras []string
	for _, root := range w.roots {
		oroot := impliedRoots[root.file]
		if oroot == nil {
			continue
		}
		var visit func(e *cn)
		visit = func(e *cn) {
			if oe := w.impliedByKey[e.key]; oe != nil {
				want := map[impl]int{}
				for _, i := range w.oracleImplied(oe, impliedByID) {
					want[i]++
				}
				have := map[impl]int{}
				for _, i := range w.ourImplied(e) {
					have[i]++
				}
				for i, n := range want {
					r := get(oe.class + " " + i.class)
					r.expected += n
					m := n
					if have[i] < m {
						m = have[i]
					}
					r.matched += m
					if m < n && len(misses) < 400 {
						misses = append(misses, fmt.Sprintf("%-34s %-18s -> %s   in %s", oe.class, i.class, shorten(w, i.target), describe(oe)))
					}
				}
				for i, n := range have {
					if want[i] < n {
						get(e.class + " " + i.class).extra += n - want[i]
						if len(extras) < 400 {
							extras = append(extras, fmt.Sprintf("%-34s %-18s -> %s   in %s", e.class, i.class, shorten(w, i.target), describe(e)))
						}
					}
				}
			}
			for _, k := range e.kids {
				if k.n != nil {
					visit(k.n)
				}
			}
		}
		visit(root)
	}
	var kinds []string
	var te, tm, tx int
	for k, r := range byKind {
		kinds = append(kinds, k)
		te += r.expected
		tm += r.matched
		tx += r.extra
	}
	sort.Slice(kinds, func(i, j int) bool { return byKind[kinds[i]].expected > byKind[kinds[j]].expected })
	fmt.Printf("\nimplied generalizations: %d in the oracle, %d reproduced, %d extra\n", te, tm, tx)
	fmt.Printf("%-52s %8s %8s %6s\n", "owner class / relationship", "oracle", "ours", "extra")
	for i, k := range kinds {
		if i >= 25 {
			break
		}
		r := byKind[k]
		fmt.Printf("%-52s %8d %8d %6d\n", k, r.expected, r.matched, r.extra)
	}
	n := 8
	if v := os.Getenv("SHOW"); v != "" {
		fmt.Sscan(v, &n)
	}
	fmt.Println("\n-- in the oracle, not produced --")
	for i, s := range misses {
		if i >= n {
			break
		}
		fmt.Println("  " + s)
	}
	fmt.Println("\n-- produced, not in the oracle --")
	for i, s := range extras {
		if i >= n {
			break
		}
		fmt.Println("  " + s)
	}
}

func shorten(w *world, key string) string {
	if c := w.impliedByKey[key]; c != nil {
		return describe(c)
	}
	if c := w.mineByKey[key]; c != nil {
		return describe(c)
	}
	return strings.TrimPrefix(key, "")
}
