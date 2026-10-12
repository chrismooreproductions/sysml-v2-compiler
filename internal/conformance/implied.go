// SPDX-License-Identifier: EPL-2.0

package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	"TypeFeaturing":       "featuringType",
}

// detachImplied removes the implied relationships and elements from an oracle
// tree, with the membership that holds an implied element, so that the positions
// of the other elements are those of the file without them, and returns them by
// owner.
func detachImplied(c *Elem, into map[*Elem][]*Elem, generated map[string]bool) {
	var kept []Kid
	for _, k := range c.Kids {
		if k.Elem != nil {
			if k.Elem.Attrs["isImplied"] == "true" {
				into[c] = append(into[c], k.Elem)
				markGenerated(k.Elem, generated)
				continue
			}
			// a membership holding an implied element is itself implied
			var held []*Elem
			for _, kk := range k.Elem.Kids {
				if kk.Elem != nil && kk.Elem.Attrs["isImplied"] == "true" && kk.Elem.Class == "BindingConnector" {
					held = append(held, kk.Elem)
				}
			}
			if len(held) > 0 {
				into[c] = append(into[c], held...)
				for _, h := range held {
					markGenerated(h, generated)
				}
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
			for _, b := range w.ImpliedBindings(e) {
				key := func(x *model.Element) string {
					if t := z.Canon(x); t != nil && t.Key != "" {
						return t.Key
					}
					return "(generated)"
				}
				if have[c.Key] == nil {
					have[c.Key] = map[string]int{}
				}
				have[c.Key]["BindingConnector "+key(b.Source)+" "+key(b.Target)]++
			}
			for _, ft := range w.ImpliedFeaturingTypes(e) {
				target := "(generated)"
				if t := z.Canon(ft); t != nil && t.Key != "" {
					target = t.Key
				} else if !ft.Generated {
					continue
				}
				if have[c.Key] == nil {
					have[c.Key] = map[string]int{}
				}
				have[c.Key]["TypeFeaturing "+target]++
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
			if r.Class == "BindingConnector" {
				if want[owner.Key] == nil {
					want[owner.Key] = map[string]int{}
				}
				want[owner.Key]["BindingConnector "+bindingEnds(r, index, generated)]++
				continue
			}
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
			if n > m && kind == "BindingConnector" && os.Getenv("ORACLE_DEBUG") != "" {
				fmt.Printf("DEBUG owner %s\n  want %s\n  have %v\n", describe(index.ByKey[owner]), rel, have[owner])
			}
			if n > m && len(st.Samples) < 200000 {
				st.Samples = append(st.Samples, fmt.Sprintf("missing %-18s %s -> %s", kind, describe(index.ByKey[owner])+" #"+index.ByKey[owner].ID, describeKey(index, rel[len(kind)+1:])))
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
			if len(st.Samples) < 200000 {
				st.Samples = append(st.Samples, fmt.Sprintf("extra   %-18s %s -> %s", kind, describe(index.ByKey[owner])+" #"+index.ByKey[owner].ID, describeKey(index, rel[len(kind)+1:])))
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

// bindingEnds names the two features an oracle binding connector binds: the
// features its ends reference, by position, or (generated).
func bindingEnds(bc *Elem, index *Index, generated map[string]bool) string {
	var ends []string
	for _, k := range bc.Kids {
		if k.Elem == nil || !strings.HasSuffix(k.Elem.Class, "Membership") {
			continue
		}
		for _, kk := range k.Elem.Kids {
			if kk.Elem == nil || kk.Elem.Attrs["isEnd"] != "true" {
				continue
			}
			ends = append(ends, endTarget(kk.Elem, index, generated))
		}
	}
	for len(ends) < 2 {
		ends = append(ends, "?")
	}
	return ends[0] + " " + ends[1]
}

func endTarget(end *Elem, index *Index, generated map[string]bool) string {
	for _, k := range end.Kids {
		if k.Elem != nil && k.Elem.Class == "ReferenceSubsetting" {
			for _, kk := range k.Elem.Kids {
				switch {
				case kk.Elem == nil && kk.Role == "referencedFeature":
					if t := index.ByID[kk.TgtFile+"|"+kk.TgtID]; t != nil {
						return t.Key
					}
					return "(generated)"
				case kk.Elem != nil:
					return "(generated)"
				}
			}
		}
	}
	return "?"
}
