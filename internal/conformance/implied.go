// SPDX-License-Identifier: EPL-2.0

package conformance

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/library"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/postprocess"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/semantics"
)

// ImpliedStats counts how the implied general types we compute compare with
// the relationships the pilot marks isImplied.
type ImpliedStats struct {
	Expected, Match, Missing, Extra int
	ByKind                          map[string]*RefKind // Total = expected, Match, Wrong = extra, Unresolved = missing
	Samples                         []string
}

// impliedRoles name the property holding each implied relationship's target.
var impliedRoles = map[string]string{
	"Subclassification":   "superclassifier",
	"Subsetting":          "subsettedFeature",
	"Redefinition":        "redefinedFeature",
	"FeatureTyping":       "type",
	"CrossSubsetting":     "crossedFeature",
	"ReferenceSubsetting": "referencedFeature",
}

// detachImplied removes the implied relationships from an oracle tree, so that
// the positions of the other elements are those of the file without them, and
// returns them by owner.
func detachImplied(c *Elem, into map[*Elem][]*Elem, generated map[string]bool) {
	var kept []Kid
	for _, k := range c.Kids {
		if k.Elem != nil {
			if k.Elem.Attrs["isImplied"] == "true" {
				into[c] = append(into[c], k.Elem)
				markGenerated(k.Elem, generated)
				continue
			}
			detachImplied(k.Elem, into, generated)
		}
		kept = append(kept, k)
	}
	c.Kids = kept
}

// markGenerated records the elements owned by an implied relationship: the
// features the pilot makes to be its targets.
func markGenerated(c *Elem, into map[string]bool) {
	for _, k := range c.Kids {
		if k.Elem != nil {
			if k.Elem.ID != "" {
				into[k.Elem.File+"|"+k.Elem.ID] = true
			}
			markGenerated(k.Elem, into)
		}
	}
}

// CompareImplied resolves the standard library, computes the implied general
// types of every type in it, and compares them with the oracle's isImplied
// relationships, which oracleRoot holds in sysml.library.xmi.implied.
func CompareImplied(oracleRoot string) (*ImpliedStats, error) {
	files, err := library.Files()
	if err != nil {
		return nil, err
	}
	w := semantics.NewWorkspace()
	z := NewCanonicalizer()
	var roots []*model.Element
	for _, f := range files {
		root := model.FromSyntax(f.Tree)
		postprocess.Run(root)
		semantics.Structure(root)
		w.Add(root)
		roots = append(roots, root)
	}
	w.ResolveAll()
	var ours []*Elem
	for i, f := range files {
		ours = append(ours, z.Add(roots[i], FileBase(f.Path)))
	}
	z.Link()

	index := NewIndex()
	impliedOf := map[*Elem][]*Elem{}
	generated := map[string]bool{}
	for i, f := range files {
		Prune(ours[i])
		oracle, err := LoadXMI(filepath.Join(oracleRoot, "sysml.library.xmi.implied", f.Path+"x"))
		if err != nil {
			return nil, err
		}
		Prune(oracle)
		detachImplied(oracle, impliedOf, generated)
		index.Add(oracle, FileBase(f.Path))
	}
	byKey := map[string]*Elem{}
	for i, f := range files {
		AssignKeys(ours[i], FileBase(f.Path), "", nil, byKey)
	}

	st := &ImpliedStats{ByKind: map[string]*RefKind{}}
	tally := func(kind string) *RefKind {
		rk := st.ByKind[kind]
		if rk == nil {
			rk = &RefKind{}
			st.ByKind[kind] = rk
		}
		return rk
	}
	// what we computed and what the oracle has, by owner position
	have := map[string]map[string]int{} // owner key -> "Kind target-key" -> count
	for _, root := range roots {
		root.Walk(func(e *model.Element) {
			c := z.Canon(e)
			if c == nil || !e.IsA("Type") {
				return
			}
			for _, im := range w.ImpliedGeneralizations(e) {
				target := "(generated)" // a feature the pilot makes: compared by kind only
				if t := z.Canon(im.Target); t != nil && t.Key != "" {
					target = t.Key
				} else if !im.Target.Generated {
					continue // a target outside the library: nothing to compare with
				}
				if have[c.Key] == nil {
					have[c.Key] = map[string]int{}
				}
				have[c.Key][im.Kind+" "+target]++
			}
		})
	}
	want := map[string]map[string]int{}
	for owner, rels := range impliedOf {
		for _, r := range rels {
			role, ok := impliedRoles[r.Class]
			if !ok {
				continue
			}
			target := ""
			for _, k := range r.Kids {
				if k.Elem == nil && k.Role == role {
					if t := index.ByID[k.TgtFile+"|"+k.TgtID]; t != nil {
						target = t.Key
					} else if generated[k.TgtFile+"|"+k.TgtID] {
						target = "(generated)"
					}
				}
			}
			if want[owner.Key] == nil {
				want[owner.Key] = map[string]int{}
			}
			want[owner.Key][r.Class+" "+target]++
		}
	}
	keys := map[string]bool{}
	for k := range have {
		keys[k] = true
	}
	for k := range want {
		keys[k] = true
	}
	var sorted []string
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, owner := range sorted {
		seen := map[string]bool{}
		for rel, n := range want[owner] {
			seen[rel] = true
			kind := rel[:indexByte(rel, ' ')]
			m := n
			if h := have[owner][rel]; h < m {
				m = h
			}
			rk := tally(kind)
			rk.Total += n
			rk.Match += m
			rk.Unresolved += n - m
			st.Expected += n
			st.Match += m
			st.Missing += n - m
			if n > m && len(st.Samples) < 5000 {
				st.Samples = append(st.Samples, fmt.Sprintf("missing %-18s %s -> %s", kind, describe(index.ByKey[owner]), describeKey(index, rel[len(kind)+1:])))
			}
		}
		for rel, n := range have[owner] {
			extra := n
			if seen[rel] {
				extra = n - want[owner][rel]
			}
			if extra <= 0 {
				continue
			}
			kind := rel[:indexByte(rel, ' ')]
			tally(kind).Wrong += extra
			st.Extra += extra
			if len(st.Samples) < 5000 {
				st.Samples = append(st.Samples, fmt.Sprintf("extra   %-18s %s -> %s", kind, describe(index.ByKey[owner]), describeKey(index, rel[len(kind)+1:])))
			}
		}
	}
	return st, nil
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return len(s)
}

func describeKey(x *Index, key string) string {
	if e := x.ByKey[key]; e != nil {
		return describe(e)
	}
	return key
}
