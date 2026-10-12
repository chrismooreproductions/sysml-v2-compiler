// SPDX-License-Identifier: EPL-2.0

package grammar

import "sort"

// Predecessors computes, for keywords that the grammar only ever uses after a
// required element, the small set of tokens that element can begin with. It
// is the basis for error hints: in `import A;` the keyword `import` is
// unexpected because the mandatory visibility keyword before it is missing.
//
// A keyword is included only if every place it can begin a construct is
// preceded, directly and non-optionally, by an element whose first tokens are
// a set of at most max literals.
func (g *Grammar) Predecessors(max int) map[string][]string {
	type use struct {
		preds []string // nil when the keyword has no required predecessor here
		ok    bool
	}
	uses := map[string][]use{}

	nullable := map[string]bool{}
	var isNullable func(e *Expr, seen map[string]bool) bool
	isNullable = func(e *Expr, seen map[string]bool) bool {
		switch e.Kind {
		case Term, Wild, XRef:
			return false
		case Action:
			return true
		case Rep:
			return e.Op != "+" || isNullable(e.Kids[0], seen)
		case Assign, Pred:
			return isNullable(e.Kids[0], seen)
		case Alt:
			for _, k := range e.Kids {
				if isNullable(k, seen) {
					return true
				}
			}
			return false
		case Seq:
			for _, k := range e.Kids {
				if !isNullable(k, seen) {
					return false
				}
			}
			return true
		case Ref:
			r, ok := g.Rules[e.Text]
			if !ok || r.Kind == Terminal {
				return false
			}
			if r.Kind == Enum {
				return false
			}
			if v, done := nullable[e.Text]; done {
				return v
			}
			if seen[e.Text] {
				return false
			}
			seen[e.Text] = true
			v := isNullable(r.Body, seen)
			delete(seen, e.Text)
			nullable[e.Text] = v
			return v
		}
		return false
	}

	// first returns the literals an expression can begin with, or false when
	// that set is not a small set of literals.
	var first func(e *Expr, seen map[string]bool) (map[string]bool, bool)
	first = func(e *Expr, seen map[string]bool) (map[string]bool, bool) {
		switch e.Kind {
		case Term:
			return map[string]bool{e.Text: true}, true
		case Assign, Pred:
			return first(e.Kids[0], seen)
		case Rep:
			return first(e.Kids[0], seen)
		case Alt:
			out := map[string]bool{}
			for _, k := range e.Kids {
				s, ok := first(k, seen)
				if !ok {
					return nil, false
				}
				for x := range s {
					out[x] = true
				}
			}
			return out, len(out) <= max
		case Seq:
			out := map[string]bool{}
			for _, k := range e.Kids {
				s, ok := first(k, seen)
				if !ok {
					return nil, false
				}
				for x := range s {
					out[x] = true
				}
				if !isNullable(k, map[string]bool{}) {
					return out, len(out) <= max
				}
			}
			return out, len(out) <= max
		case Ref:
			r, ok := g.Rules[e.Text]
			if !ok || r.Kind == Terminal || seen[e.Text] {
				return nil, false
			}
			seen[e.Text] = true
			defer delete(seen, e.Text)
			if r.Kind == Enum {
				out := map[string]bool{}
				Walk(r.Body, func(x *Expr) {
					if x.Kind == Term {
						out[x.Text] = true
					}
				})
				return out, len(out) > 0 && len(out) <= max
			}
			return first(r.Body, seen)
		}
		return nil, false
	}

	// lead returns the keyword an element begins with, if it begins with one.
	var lead func(e *Expr) (string, bool)
	lead = func(e *Expr) (string, bool) {
		switch e.Kind {
		case Term:
			return e.Text, true
		case Assign, Pred:
			return lead(e.Kids[0])
		case Seq:
			if len(e.Kids) > 0 {
				return lead(e.Kids[0])
			}
		}
		return "", false
	}

	for _, name := range g.Order {
		r := g.Rules[name]
		if r.Body == nil {
			continue
		}
		Walk(r.Body, func(e *Expr) {
			switch e.Kind {
			case Seq:
				for i, k := range e.Kids {
					kw, ok := lead(k)
					if !ok || !g.Keywords[kw] {
						continue
					}
					if i == 0 {
						uses[kw] = append(uses[kw], use{})
						continue
					}
					prev := e.Kids[i-1]
					if isNullable(prev, map[string]bool{}) {
						uses[kw] = append(uses[kw], use{})
						continue
					}
					set, ok := first(prev, map[string]bool{})
					if !ok {
						uses[kw] = append(uses[kw], use{})
						continue
					}
					var lits []string
					allWords := true
					for x := range set {
						lits = append(lits, x)
						allWords = allWords && g.Keywords[x]
					}
					if !allWords {
						// punctuation before a keyword is a separator, not a missing prefix
						uses[kw] = append(uses[kw], use{})
						continue
					}
					sort.Strings(lits)
					uses[kw] = append(uses[kw], use{preds: lits, ok: true})
				}
			case Term:
				// a keyword used on its own (an alternative, a rule body of one
				// term) can begin a construct by itself
			}
		})
		// a rule or alternative that is just the keyword
		if r.Body.Kind == Term && g.Keywords[r.Body.Text] {
			uses[r.Body.Text] = append(uses[r.Body.Text], use{})
		}
		if r.Body.Kind == Alt {
			for _, k := range r.Body.Kids {
				if k.Kind == Term && g.Keywords[k.Text] {
					uses[k.Text] = append(uses[k.Text], use{})
				}
			}
		}
	}

	out := map[string][]string{}
	for kw, us := range uses {
		all := true
		merged := map[string]bool{}
		for _, u := range us {
			if !u.ok {
				all = false
				break
			}
			for _, p := range u.preds {
				merged[p] = true
			}
		}
		if !all || len(merged) == 0 || len(merged) > max {
			continue
		}
		var lits []string
		for p := range merged {
			lits = append(lits, p)
		}
		sort.Strings(lits)
		out[kw] = lits
	}
	return out
}
