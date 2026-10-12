// SPDX-License-Identifier: EPL-2.0

package conformance

import (
	"fmt"
	"sort"
)

// RefStats counts how our references compare with the oracle's.
type RefStats struct {
	Total, Match, Wrong, Unresolved, NoOracle int
	ByKind                                    map[string]*RefKind
	WrongSamples, UnresolvedSamples           []string
}

// RefKind is the tally for one kind of reference (metaclass and role).
type RefKind struct{ Total, Match, Wrong, Unresolved int }

// Index records every oracle element by file and identifier, and by position.
type Index struct {
	ByID  map[string]*Elem
	ByKey map[string]*Elem
}

// NewIndex returns an empty index.
func NewIndex() *Index { return &Index{ByID: map[string]*Elem{}, ByKey: map[string]*Elem{}} }

// Add records an oracle file's elements, giving them position keys.
func (x *Index) Add(root *Elem, file string) {
	AssignKeys(root, file, "", nil, x.ByKey)
	var walk func(c *Elem)
	walk = func(c *Elem) {
		if c.ID != "" {
			x.ByID[file+"|"+c.ID] = c
		}
		for _, k := range c.Kids {
			if k.Elem != nil {
				walk(k.Elem)
			}
		}
	}
	walk(root)
}

// describe names an element for a message: the names along its ownership
// chain and its class.
func describe(c *Elem) string {
	if c == nil {
		return "(none)"
	}
	var parts []string
	for e := c; e != nil; e = e.Parent {
		switch {
		case e.Attrs["declaredName"] != "":
			parts = append(parts, e.Attrs["declaredName"])
		case e.Attrs["declaredShortName"] != "":
			parts = append(parts, "<"+e.Attrs["declaredShortName"]+">")
		case e.Attrs["memberName"] != "":
			parts = append(parts, e.Attrs["memberName"])
		}
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	if len(parts) == 0 {
		return fmt.Sprintf("(%s %s)", c.Class, c.Key)
	}
	s := ""
	for i, p := range parts {
		if i > 0 {
			s += "::"
		}
		s += p
	}
	return s + " (" + c.Class + ")"
}

// CompareReferences compares the target of every reference in ours (position
// keys assigned) with the target of the oracle's reference at the same position.
func CompareReferences(ours []*Elem, oracle *Index) *RefStats {
	st := &RefStats{ByKind: map[string]*RefKind{}}
	var visit func(c *Elem)
	visit = func(c *Elem) {
		o := oracle.ByKey[c.Key]
		seen := map[string]int{}
		for _, k := range c.Kids {
			if k.Elem != nil {
				visit(k.Elem)
				continue
			}
			kind := c.Class + "." + k.Role
			if k.Name == "" {
				kind += " (implied)"
			}
			rk := st.ByKind[kind]
			if rk == nil {
				rk = &RefKind{}
				st.ByKind[kind] = rk
			}
			st.Total++
			rk.Total++
			ordinal := seen[k.Role]
			seen[k.Role]++
			var theirs *Kid
			if o != nil {
				n := 0
				for i := range o.Kids {
					if o.Kids[i].Elem == nil && o.Kids[i].Role == k.Role {
						if n == ordinal {
							theirs = &o.Kids[i]
							break
						}
						n++
					}
				}
			}
			if theirs == nil {
				st.NoOracle++
				continue
			}
			want := oracle.ByID[theirs.TgtFile+"|"+theirs.TgtID]
			switch {
			case k.Target == nil:
				st.Unresolved++
				rk.Unresolved++
				if len(st.UnresolvedSamples) < 400 {
					st.UnresolvedSamples = append(st.UnresolvedSamples, fmt.Sprintf("%-34s %q in %s (oracle: %s)", kind, k.Name, describe(c.Parent), describe(want)))
				}
			case want != nil && k.Target.Key == want.Key:
				st.Match++
				rk.Match++
			default:
				st.Wrong++
				rk.Wrong++
				if len(st.WrongSamples) < 400 {
					st.WrongSamples = append(st.WrongSamples, fmt.Sprintf("%-34s %q in %s\n      ours:   %s\n      oracle: %s", kind, k.Name, describe(c.Parent), describe(k.Target), describe(want)))
				}
			}
		}
	}
	for _, r := range ours {
		visit(r)
	}
	return st
}

// Kinds lists the kinds of reference, most frequent first.
func (s *RefStats) Kinds() []string {
	var ks []string
	for k := range s.ByKind {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		return s.ByKind[ks[i]].Total > s.ByKind[ks[j]].Total || (s.ByKind[ks[i]].Total == s.ByKind[ks[j]].Total && ks[i] < ks[j])
	})
	return ks
}
