// SPDX-License-Identifier: EPL-2.0

package semantics

import "sort"

// maxCallDepth stops a calculation that calls itself without end.
const maxCallDepth = 64

// EvaluateWith evaluates an expression with values given for features and a
// context usage. A reference to a feature takes its value from, in order: the
// bound values; a feature of the context that redefines it and has a value
// (the usage r in "requirement r : MassReq { :>> massActual = 1800; }" gives
// the massActual that MassReq's constraint reads); the feature itself.
func (w *Workspace) EvaluateWith(e, context *el, bound map[*el]Value) (Value, error) {
	if e == nil {
		return nil, &EvalError{Msg: "no expression"}
	}
	b := map[*el][]Value{}
	for f, v := range bound {
		b[f] = []Value{v}
	}
	vs, err := (&evaluator{w: w, active: map[*el]bool{}, bound: b, context: context}).eval(e)
	if err != nil {
		return nil, err
	}
	return collapse(vs), nil
}

// ResultExpression is the expression that gives the value of a calculation,
// function or constraint: the value of its return parameter, or its result
// expression, looked for in the type and then in the types it specializes (a
// calculation usage takes its definition's). It is nil if there is none: a
// library function such as abs has no body.
func (w *Workspace) ResultExpression(t *el) *el {
	seen := map[*el]bool{}
	queue := []*el{t}
	for i := 0; i < len(queue); i++ {
		x := queue[i]
		if x == nil || seen[x] {
			continue
		}
		seen[x] = true
		for _, m := range w.rels(x, "ReturnParameterMembership") {
			if p := first(w.ownedRelated(m)); p != nil {
				for _, fv := range w.rels(p, "FeatureValue") {
					if r := first(w.ownedRelated(fv)); r != nil && !r.Generated {
						return r
					}
				}
			}
		}
		for _, m := range w.rels(x, "ResultExpressionMembership") {
			if r := first(w.ownedRelated(m)); r != nil && !r.Generated {
				return r
			}
		}
		queue = append(queue, w.generalTypes(x)...)
	}
	return nil
}

// Inputs are the input parameters of a type, in order: the in and inout
// features it owns, then those of the types it specializes that it does not
// redefine under the same name.
func (w *Workspace) Inputs(t *el) []*el {
	var out []*el
	names := map[string]bool{}
	seen := map[*el]bool{}
	queue := []*el{t}
	for i := 0; i < len(queue); i++ {
		x := queue[i]
		if x == nil || seen[x] {
			continue
		}
		seen[x] = true
		for _, m := range w.rels(x, "FeatureMembership") {
			f := first(w.ownedRelated(m))
			if f == nil || !f.IsA("Feature") || f.Generated {
				continue
			}
			if d := f.Text("direction"); d != "in" && d != "inout" {
				continue
			}
			if n := w.EffectiveName(f); n == "" || !names[n] {
				names[n] = true
				out = append(out, f)
			}
		}
		queue = append(queue, w.generalTypes(x)...)
	}
	return out
}

// contextValue is the value the context gives a feature through a feature of
// its own that redefines it. ok is false if there is none.
func (ev *evaluator) contextValue(f *el) (vs []Value, ok bool, err error) {
	w := ev.w
	for _, g := range w.Features(ev.context) {
		if g == f || ev.active[g] || !redefines(w, g, f) {
			continue
		}
		for _, fv := range w.rels(g, "FeatureValue") {
			if x := first(w.ownedRelated(fv)); x != nil {
				ev.active[g] = true
				vs, err = ev.eval(x)
				delete(ev.active, g)
				return vs, true, err
			}
		}
	}
	return nil, false, nil
}

// invoke evaluates a call of a function, calculation or predicate that the
// model defines: the arguments, positional or named, are bound to its inputs
// and its result expression is evaluated.
func (ev *evaluator) invoke(e *el) ([]Value, error) {
	w := ev.w
	var fn *el
	for _, m := range w.rels(e, "Membership") {
		if !m.IsA("ParameterMembership") {
			fn = w.memberElement(m)
			break
		}
	}
	if fn == nil {
		return nil, ev.fail(e, "a call of nothing")
	}
	if !fn.IsA("Function") {
		return nil, ev.fail(e, "cannot evaluate a call of %s", displayName(fn))
	}
	if ev.depth >= maxCallDepth {
		return nil, ev.fail(e, "%s calls itself too deeply", displayName(fn))
	}
	body := w.ResultExpression(fn)
	if body == nil {
		return nil, ev.fail(e, "cannot evaluate a call of %s: it has no body", displayName(fn))
	}
	inputs := w.Inputs(fn)
	bound := map[*el][]Value{}
	pos := 0
	for _, m := range w.rels(e, "ParameterMembership") {
		if m.IsA("ReturnParameterMembership") {
			continue
		}
		arg := first(w.ownedRelated(m))
		if arg == nil {
			continue
		}
		var target *el
		for _, r := range w.rels(arg, "Redefinition") { // a named argument
			target = w.refTarget(r, "redefinedFeature")
		}
		if target == nil {
			if pos >= len(inputs) {
				return nil, ev.fail(e, "%s takes %d inputs, got more", displayName(fn), len(inputs))
			}
			target = inputs[pos]
		}
		pos++
		var expr *el
		for _, fv := range w.rels(arg, "FeatureValue") {
			expr = first(w.ownedRelated(fv))
		}
		if expr == nil {
			return nil, ev.fail(arg, "an argument has no expression")
		}
		vs, err := ev.eval(expr)
		if err != nil {
			return nil, err
		}
		bound[target] = vs
	}
	callee := &evaluator{w: w, active: map[*el]bool{}, bound: bound, depth: ev.depth + 1}
	return callee.eval(body)
}

// EvaluateCall evaluates a calculation, function or predicate: its result
// expression, with the inputs given by name taking the place of the inputs'
// own values. For a usage the context is the usage itself, so the inputs it
// binds (calc c : Range { in :>> tankSize = 40; }) are its inputs. An input not
// given has its default value, or none.
func (w *Workspace) EvaluateCall(calc *el, inputs map[string]Value) (Value, error) {
	body := w.ResultExpression(calc)
	if body == nil {
		return nil, &EvalError{Element: calc, Msg: displayName(calc) + " has no body to evaluate"}
	}
	var context *el
	if calc.IsA("Usage") {
		context = calc
	}
	params := w.Inputs(calc)
	names := make([]string, 0, len(inputs))
	for n := range inputs {
		names = append(names, n)
	}
	sort.Strings(names)
	bound := map[*el]Value{}
	for _, n := range names {
		var p *el
		for _, c := range params {
			if w.EffectiveName(c) == n {
				p = c
				break
			}
		}
		if p == nil {
			return nil, &EvalError{Element: calc, Msg: displayName(calc) + " has no input " + n}
		}
		bound[p] = inputs[n]
		for _, r := range w.allRedefined(p, map[*el]bool{p: true}) {
			bound[r] = inputs[n] // the body reads the feature that p redefines
		}
	}
	return w.EvaluateWith(body, context, bound)
}
