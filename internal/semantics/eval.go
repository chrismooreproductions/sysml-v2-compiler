// SPDX-License-Identifier: EPL-2.0

package semantics

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
)

// A Value is what an expression evaluates to: a bool, an int64 (Integer), a
// float64 (Rational or Real), a string, or a Seq. An expression yields a
// sequence of values in KerML; Evaluate returns the bare value when there is
// exactly one, and a Seq otherwise (the empty Seq is null).
type Value = any

// Seq is a sequence of values with none of them a sequence.
type Seq []Value

// EvalError says why an expression cannot be evaluated, and where.
type EvalError struct {
	// Element is the expression or feature the failure is about.
	Element *model.Element
	Msg     string
	// Unbound is true when the expression is evaluable but a value it needs is
	// missing: a feature it refers to has no value. Binding the value would let
	// it be computed. It is false for what the evaluator does not support, and
	// for a model that is wrong (an unresolved reference, a type mismatch).
	Unbound bool
}

func (e *EvalError) Error() string { return e.Msg }

// Evaluate computes the value of an expression in a resolved workspace, as far
// as it can be computed from the model alone: literals, operators, sequences
// and references to features that have a value. It returns an *EvalError for
// anything else (calls of library functions, for example, are not yet
// supported).
func (w *Workspace) Evaluate(e *el) (Value, error) {
	if e == nil {
		return nil, &EvalError{Msg: "no expression"}
	}
	vs, err := (&evaluator{w: w, active: map[*el]bool{}}).eval(e)
	if err != nil {
		return nil, err
	}
	return collapse(vs), nil
}

// EvaluateFor evaluates an expression for an element, the way a filter
// condition is evaluated for each member it may keep. The implicit operand of
// "@T" (the "filter @SysML::PartUsage" form) is the element, and a reference to
// a feature of a metadata definition ("Safety::level") is read from the
// element's own metadata. Without an element those are not evaluable.
func (w *Workspace) EvaluateFor(e, self *el) (Value, error) {
	if e == nil {
		return nil, &EvalError{Msg: "no expression"}
	}
	vs, err := (&evaluator{w: w, active: map[*el]bool{}, self: self}).eval(e)
	if err != nil {
		return nil, err
	}
	return collapse(vs), nil
}

func collapse(vs []Value) Value {
	if len(vs) == 1 {
		return vs[0]
	}
	return Seq(vs)
}

type evaluator struct {
	w      *Workspace
	active map[*el]bool // features whose value is being computed, to stop cycles
	self   *el          // the element a filter is evaluated for; nil for no element

	bound   map[*el][]Value // values given for features (the inputs of a call)
	context *el             // the usage whose redefinitions give a feature its value
	depth   int             // how many calls deep
}

func (ev *evaluator) fail(e *el, format string, args ...any) error {
	return &EvalError{Element: e, Msg: fmt.Sprintf(format, args...)}
}

// eval returns the sequence of values of an expression.
func (ev *evaluator) eval(e *el) ([]Value, error) {
	w := ev.w
	switch {
	case e.IsA("LiteralInteger"):
		n, err := strconv.ParseInt(strings.ReplaceAll(e.Text("value"), "_", ""), 10, 64)
		if err != nil {
			f, ferr := strconv.ParseFloat(e.Text("value"), 64)
			if ferr != nil {
				return nil, ev.fail(e, "bad integer %q", e.Text("value"))
			}
			return []Value{f}, nil
		}
		return []Value{n}, nil
	case e.IsA("LiteralRational"):
		f, err := strconv.ParseFloat(strings.ReplaceAll(e.Text("value"), "_", ""), 64)
		if err != nil {
			return nil, ev.fail(e, "bad number %q", e.Text("value"))
		}
		return []Value{f}, nil
	case e.IsA("LiteralBoolean"):
		return []Value{e.IsTrue("value")}, nil
	case e.IsA("LiteralString"):
		return []Value{e.Text("value")}, nil
	case e.IsA("LiteralInfinity"):
		return []Value{math.Inf(1)}, nil
	case e.IsA("NullExpression"):
		return nil, nil
	case e.IsA("FeatureChainExpression"): // a kind of reference expression, so first
		return ev.chain(e)
	case e.IsA("FeatureReferenceExpression"):
		return ev.reference(e)
	case e.IsA("IndexExpression"):
		args, err := ev.operands(e, 2)
		if err != nil {
			return nil, err
		}
		seq, err := ev.eval(args[0])
		if err != nil {
			return nil, err
		}
		idx, err := ev.number(args[1])
		if err != nil {
			return nil, err
		}
		i, ok := idx.(int64)
		if !ok || i < 1 || int(i) > len(seq) {
			return nil, nil // out of range is null
		}
		return []Value{seq[i-1]}, nil
	case e.IsA("InvocationExpression") && !e.IsA("OperatorExpression"):
		if vs, ok, err := ev.sizeOfStructure(e); ok {
			return vs, err
		}
		return ev.invoke(e)
	case e.IsA("OperatorExpression") && !e.IsA("FeatureChainExpression") && !e.IsA("SelectExpression") && !e.IsA("CollectExpression"):
		return ev.operator(e, w.operatorOf(e))
	}
	return nil, ev.fail(e, "cannot evaluate a %s", e.Class)
}

// operands are the argument expressions of an expression, in order. An
// argument that only wraps an expression (a deferred operand of if, and, or)
// is unwrapped.
func (ev *evaluator) operands(e *el, want int) ([]*el, error) {
	var args []*el
	for _, m := range ev.w.rels(e, "ParameterMembership") {
		if m.IsA("ReturnParameterMembership") {
			continue
		}
		p := first(ev.w.ownedRelated(m))
		if p == nil {
			continue
		}
		var arg *el
		for _, fv := range ev.w.rels(p, "FeatureValue") {
			arg = first(ev.w.ownedRelated(fv))
		}
		if arg == nil {
			return nil, ev.fail(e, "an operand has no expression")
		}
		args = append(args, arg)
	}
	if want >= 0 && len(args) != want {
		return nil, ev.fail(e, "%d operands, want %d", len(args), want)
	}
	return args, nil
}

// deferred returns the expression a deferred operand wraps, or the operand.
func (ev *evaluator) deferred(arg *el) *el {
	if arg.IsA("FeatureReferenceExpression") {
		for _, m := range ev.w.rels(arg, "FeatureMembership") {
			if x := first(ev.w.ownedRelated(m)); x.IsA("Expression") {
				return x
			}
		}
	}
	return arg
}

func (ev *evaluator) single(arg *el) (Value, error) {
	vs, err := ev.eval(ev.deferred(arg))
	if err != nil {
		return nil, err
	}
	if len(vs) != 1 {
		return nil, ev.fail(arg, "expected one value, got %d", len(vs))
	}
	return vs[0], nil
}

func (ev *evaluator) number(arg *el) (Value, error) {
	v, err := ev.single(arg)
	if err != nil {
		return nil, err
	}
	switch v.(type) {
	case int64, float64:
		return v, nil
	}
	return nil, ev.fail(arg, "expected a number, got %v", v)
}

func (ev *evaluator) boolean(arg *el) (bool, error) {
	v, err := ev.single(arg)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, ev.fail(arg, "expected a boolean, got %v", v)
	}
	return b, nil
}

// reference is the value of a feature reference: the value of the feature it
// names, or of the expression a deferred operand wraps.
func (ev *evaluator) reference(e *el) ([]Value, error) {
	w := ev.w
	for _, m := range w.rels(e, "Membership") {
		if m.IsA("ParameterMembership") {
			continue
		}
		r := w.memberElement(m)
		if r == nil {
			return nil, ev.fail(e, "unresolved reference")
		}
		if m.IsA("FeatureMembership") && r.IsA("Expression") {
			return ev.eval(r) // a deferred operand
		}
		if vs, ok, err := ev.metadataField(r, e); ok {
			return vs, err
		}
		return ev.featureValue(r, e)
	}
	return nil, ev.fail(e, "a reference to nothing")
}

// chain evaluates a feature chain (ac.navigation.positionErrorM): the value of
// its last feature, as the features along the way make it, when each link
// before the last has a single value. A link that redefines a feature of the
// one before it (part ac : Ac { part :>> navigation { :>> x = 7; } }) gives
// its value in place of the declared one.
func (ev *evaluator) chain(e *el) ([]Value, error) {
	cur, err := ev.chainFeature(e)
	if err != nil {
		return nil, err
	}
	return ev.featureValue(cur, e)
}

// chainFeature is the feature a chain ends at, as the features along the way
// make it, when each link before the last has a single value.
func (ev *evaluator) chainFeature(e *el) (*el, error) {
	links, err := ev.chainLinks(e)
	if err != nil {
		return nil, err
	}
	links = ev.fromContext(links)
	cur := links[0]
	for i, l := range links[1:] {
		if many, why := ev.manyValued(cur); many {
			return nil, ev.fail(e, "%s has %s, so %s is a sequence of values, not one", ev.w.EffectiveName(cur), why, ev.chainText(links[:i+1]))
		}
		cur = ev.effective(cur, l)
	}
	return cur, nil
}

// sizeOfStructure answers size(...): the number of values of its argument, or
// for a feature that has no value (size(ac.engines)), from
// the multiplicity the model writes for it: engines : Engine[2] says the
// aircraft has two engines. A range has no single answer, so it stays
// undetermined, and says so.
func (ev *evaluator) sizeOfStructure(e *el) (vs []Value, ok bool, err error) {
	w := ev.w
	var fn *el
	for _, m := range w.rels(e, "Membership") {
		if !m.IsA("ParameterMembership") {
			fn = w.memberElement(m)
			break
		}
	}
	if fn == nil || QualifiedName(fn) != "SequenceFunctions::size" {
		return nil, false, nil
	}
	arg := w.firstArgument(e)
	if arg == nil {
		return nil, false, nil
	}
	// the number of values, which the model's own function reaches only through
	// collect, a function with no body here
	vs, aerr := ev.eval(arg)
	if aerr == nil {
		return []Value{int64(len(vs))}, true, nil
	}
	if !unbound(aerr) || !(arg.IsA("FeatureReferenceExpression") || arg.IsA("FeatureChainExpression")) {
		return nil, false, nil // it fails for another reason: the call reports it
	}
	f, cerr := ev.chainFeature(arg)
	if cerr != nil {
		return nil, false, nil
	}
	lo, hi, written := ev.bounds(f)
	switch {
	case !written:
		return nil, false, nil
	case lo == hi:
		return []Value{int64(lo)}, true, nil
	}
	upper := strconv.Itoa(hi)
	if hi < 0 {
		upper = "*"
	}
	return nil, true, &EvalError{Element: e, Msg: fmt.Sprintf("%s has the multiplicity %d..%s, so its number of values is not one number", ev.w.EffectiveName(f), lo, upper), Unbound: true}
}

// fromContext replaces the first feature of a chain by the feature the context
// binds it to: a requirement's subject (subject s : Ac; s.x) is the usage a
// satisfy names (satisfy r by ac), so s.x is ac.x.
func (ev *evaluator) fromContext(links []*el) []*el {
	if ev.context == nil {
		return links
	}
	w := ev.w
	for _, g := range w.Features(ev.context) {
		if g == links[0] || !w.allRedefinedWithComputed(g)[links[0]] {
			continue
		}
		for _, fv := range w.rels(g, "FeatureValue") {
			if x := first(w.ownedRelated(fv)); x != nil {
				if bound, err := ev.chainLinks(x); err == nil {
					return append(bound, links[1:]...)
				}
			}
		}
	}
	return links
}

// chainLinks are the features a chain names, from the first.
func (ev *evaluator) chainLinks(e *el) ([]*el, error) {
	w := ev.w
	switch {
	case e.IsA("FeatureChainExpression"):
		src := w.firstArgument(e)
		if src == nil {
			return nil, ev.fail(e, "a feature chain without a source")
		}
		links, err := ev.chainLinks(src)
		if err != nil {
			return nil, err
		}
		// the target is a plain member (a.b), or a feature that chains the
		// rest (a.b.c chains b and c)
		if t := w.chainTarget(e); t != nil && t.IsA("Feature") {
			return append(links, t), nil
		}
		for _, m := range w.rels(e, "OwningMembership") {
			if m.Class != "OwningMembership" { // not the parameters
				continue
			}
			if chained := first(w.ownedRelated(m)); chained.IsA("Feature") {
				for _, c := range w.rels(chained, "FeatureChaining") {
					f := w.refTarget(c, "chainingFeature")
					if f == nil {
						return nil, ev.fail(e, "a feature chain that does not resolve")
					}
					links = append(links, f)
				}
				return links, nil
			}
		}
		return nil, ev.fail(e, "a feature chain that does not resolve")
	case e.IsA("FeatureReferenceExpression"):
		if t := w.chainTarget(e); t != nil && t.IsA("Feature") {
			return []*el{t}, nil
		}
	default:
		return nil, ev.fail(e, "cannot evaluate a feature chain from a %s", e.Class)
	}
	return nil, ev.fail(e, "a reference to nothing")
}

func (ev *evaluator) chainText(links []*el) string {
	var names []string
	for _, l := range links {
		names = append(names, ev.w.EffectiveName(l))
	}
	return strings.Join(names, ".")
}

// effective is the feature of cur that stands for l: l itself, or the most
// specific feature of cur that redefines it.
func (ev *evaluator) effective(cur, l *el) *el {
	w := ev.w
	var matches []*el
	for _, x := range w.features(cur) {
		if x == l || w.allRedefinedWithComputed(x)[l] {
			matches = append(matches, x)
		}
	}
	for _, x := range matches {
		redefined := false
		for _, y := range matches {
			if y != x && w.allRedefinedWithComputed(y)[x] {
				redefined = true
			}
		}
		if !redefined {
			return x
		}
	}
	return l
}

// manyValued says whether a feature, or one it redefines, writes a multiplicity
// that allows more than one value, and what it is.
func (ev *evaluator) manyValued(f *el) (bool, string) {
	_, hi, ok := ev.bounds(f)
	switch {
	case !ok:
		return false, ""
	case hi < 0:
		return true, "an unbounded multiplicity"
	case hi > 1:
		return true, "a multiplicity that allows " + strconv.Itoa(hi) + " values"
	}
	return false, ""
}

// bounds are the lower and upper bounds of the multiplicity a feature, or the
// first feature it redefines, writes; an upper bound of -1 is *. ok is false if
// none is written or a bound is not a number.
func (ev *evaluator) bounds(f *el) (lo, hi int, ok bool) {
	w := ev.w
	for _, g := range append([]*el{f}, keys(w.allRedefinedWithComputed(f))...) {
		for _, m := range w.rels(g, "OwningMembership") {
			for _, r := range w.ownedRelated(m) {
				if !r.IsA("MultiplicityRange") || r.Generated {
					continue
				}
				var bs []*el
				for _, bm := range w.rels(r, "Membership") {
					bs = append(bs, w.ownedRelated(bm)...)
				}
				num := func(x *el) (int, bool) {
					if x.IsA("LiteralInfinity") {
						return -1, true
					}
					vs, err := ev.eval(x)
					if err != nil || len(vs) != 1 {
						return 0, false
					}
					n, isInt := vs[0].(int64)
					return int(n), isInt
				}
				switch len(bs) {
				case 1: // [n] is n..n, and [*] is 0..*
					n, good := num(bs[0])
					if !good {
						continue
					}
					if n < 0 {
						return 0, -1, true
					}
					return n, n, true
				case 2:
					l, good1 := num(bs[0])
					h, good2 := num(bs[1])
					if good1 && good2 {
						return l, h, true
					}
				}
			}
		}
	}
	return 0, 0, false
}

func keys(m map[*el]bool) []*el {
	var out []*el
	for k := range m {
		out = append(out, k)
	}
	return out
}

// featureValue is the value a feature is bound to, taken from the feature or,
// failing that, from a feature it redefines.
func (ev *evaluator) featureValue(f, at *el) ([]Value, error) {
	if !f.IsA("Feature") {
		return nil, ev.fail(at, "%s is not a feature", QualifiedName(f))
	}
	if vs, ok := ev.bound[f]; ok {
		return vs, nil
	}
	if ev.context != nil {
		if vs, ok, err := ev.contextValue(f); ok {
			return vs, err
		}
	}
	if ev.active[f] {
		return nil, ev.fail(at, "%s is defined in terms of itself", QualifiedName(f))
	}
	ev.active[f] = true
	defer delete(ev.active, f)
	candidates := append([]*el{f}, ev.w.redefinedWithComputed(f)...)
	for _, c := range candidates {
		for _, fv := range ev.w.rels(c, "FeatureValue") {
			if x := first(ev.w.ownedRelated(fv)); x != nil {
				return ev.eval(x)
			}
		}
	}
	return nil, &EvalError{Element: at, Msg: fmt.Sprintf("%s has no value", displayName(f)), Unbound: true}
}

// both evaluates two operands, and reports the failure that matters most: one
// that no value would fix (unsupported, or a wrong model) before one that only
// says a value is missing, so that "not determined" always means that binding
// the missing value could determine the result.
func (ev *evaluator) both(args []*el, f func(*el) (Value, error)) (l, r Value, err error) {
	l, lerr := f(args[0])
	r, rerr := f(args[1])
	switch {
	case lerr != nil && !unbound(lerr):
		return nil, nil, lerr
	case rerr != nil && !unbound(rerr):
		return nil, nil, rerr
	case lerr != nil:
		return nil, nil, lerr
	}
	return l, r, rerr
}

// unbound is true if err says only that a value is missing.
func unbound(err error) bool {
	e, ok := err.(*EvalError)
	return ok && e.Unbound
}

func displayName(f *el) string {
	if n := QualifiedName(f); n != "" {
		return n
	}
	return "a " + f.Class
}

// operator evaluates an operator expression.
func (ev *evaluator) operator(e *el, op string) ([]Value, error) {
	switch op {
	case "[":
		return ev.quantity(e)
	case "@":
		return ev.metaclassTest(e)
	case "if":
		args, err := ev.operands(e, 3)
		if err != nil {
			return nil, err
		}
		c, err := ev.boolean(args[0])
		if err != nil {
			return nil, err
		}
		if c {
			return ev.eval(ev.deferred(args[1]))
		}
		return ev.eval(ev.deferred(args[2]))
	case "and", "or", "implies", "??":
		args, err := ev.operands(e, 2)
		if err != nil {
			return nil, err
		}
		if op == "??" {
			vs, err := ev.eval(args[0])
			if err != nil {
				return nil, err
			}
			if len(vs) > 0 {
				return vs, nil
			}
			return ev.eval(ev.deferred(args[1]))
		}
		l, lerr := ev.boolean(args[0])
		if lerr != nil {
			if !unbound(lerr) {
				return nil, lerr
			}
			// The left operand is missing a value. The right one can still
			// decide the result (false and x, true or x, x implies true);
			// otherwise it is not determined.
			r, rerr := ev.boolean(args[1])
			switch {
			case rerr != nil && !unbound(rerr):
				return nil, rerr
			case rerr != nil:
				return nil, lerr
			case op == "and" && !r, op == "or" && r, op == "implies" && r:
				return []Value{op != "and"}, nil
			}
			return nil, lerr
		}
		switch {
		case op == "and" && !l:
			return []Value{false}, nil
		case op == "or" && l:
			return []Value{true}, nil
		case op == "implies" && !l:
			return []Value{true}, nil
		}
		r, err := ev.boolean(args[1])
		return []Value{r}, err
	case ",":
		args, err := ev.operands(e, 2)
		if err != nil {
			return nil, err
		}
		var out []Value
		for _, a := range args {
			vs, err := ev.eval(a)
			if err != nil {
				return nil, err
			}
			out = append(out, vs...)
		}
		return out, nil
	case "..":
		args, err := ev.operands(e, 2)
		if err != nil {
			return nil, err
		}
		lo, err := ev.number(args[0])
		if err != nil {
			return nil, err
		}
		hi, err := ev.number(args[1])
		if err != nil {
			return nil, err
		}
		l, lok := lo.(int64)
		h, hok := hi.(int64)
		if !lok || !hok {
			return nil, ev.fail(e, "a range needs integer bounds")
		}
		var out []Value
		for i := l; i <= h; i++ {
			out = append(out, i)
		}
		return out, nil
	case "not", "~":
		args, err := ev.operands(e, 1)
		if err != nil {
			return nil, err
		}
		b, err := ev.boolean(args[0])
		return []Value{!b}, err
	case "-", "+":
		args, err := ev.operands(e, -1)
		if err != nil {
			return nil, err
		}
		if len(args) == 1 {
			v, err := ev.number(args[0])
			if err != nil {
				return nil, err
			}
			if op == "+" {
				return []Value{v}, nil
			}
			if n, ok := v.(int64); ok {
				return []Value{-n}, nil
			}
			return []Value{-v.(float64)}, nil
		}
	}
	args, err := ev.operands(e, 2)
	if err != nil {
		return nil, err
	}
	switch op {
	case "==", "!=", "===", "!==":
		lv, rv, err := ev.both(args, func(a *el) (Value, error) {
			vs, err := ev.eval(a)
			return vs, err
		})
		if err != nil {
			return nil, err
		}
		l, _ := lv.([]Value)
		r, _ := rv.([]Value)
		if len(l) == 1 && len(r) == 1 && (isQuantity(l[0]) || isQuantity(r[0])) {
			v, why := ev.quantityOp(op, l[0], r[0])
			if why != "" {
				return nil, ev.fail(e, "%s", why)
			}
			return []Value{v}, nil
		}
		if why := quantitiesComparable(op, l, r); why != "" {
			return nil, ev.fail(e, "%s", why)
		}
		eq := equal(l, r)
		if op == "!=" || op == "!==" {
			eq = !eq
		}
		return []Value{eq}, nil
	case "xor", "&", "|":
		lv, rv, err := ev.both(args, func(a *el) (Value, error) {
			b, err := ev.boolean(a)
			return b, err
		})
		if err != nil {
			return nil, err
		}
		l, r := lv.(bool), rv.(bool)
		switch op {
		case "xor":
			return []Value{l != r}, nil
		case "&":
			return []Value{l && r}, nil
		}
		return []Value{l || r}, nil
	}
	l, r, err := ev.both(args, ev.single)
	if err != nil {
		return nil, err
	}
	if isQuantity(l) || isQuantity(r) {
		v, why := ev.quantityOp(op, l, r)
		if why != "" {
			return nil, ev.fail(e, "%s", why)
		}
		return []Value{v}, nil
	}
	v, ok := binary(op, l, r)
	if !ok {
		return nil, ev.fail(e, "cannot apply %s to %s and %s", op, FormatValue(l), FormatValue(r))
	}
	return []Value{v}, nil
}

// binary applies an arithmetic, comparison or string operator.
func binary(op string, l, r Value) (Value, bool) {
	if ls, ok := l.(string); ok {
		rs, ok := r.(string)
		if !ok {
			return nil, false
		}
		switch op {
		case "+":
			return ls + rs, true
		case "<":
			return ls < rs, true
		case ">":
			return ls > rs, true
		case "<=":
			return ls <= rs, true
		case ">=":
			return ls >= rs, true
		}
		return nil, false
	}
	li, lint := l.(int64)
	ri, rint := r.(int64)
	if lint && rint {
		switch op {
		case "+":
			return li + ri, true
		case "-":
			return li - ri, true
		case "*":
			return li * ri, true
		case "/":
			if ri == 0 {
				return nil, false
			}
			if li%ri == 0 {
				return li / ri, true
			}
			return float64(li) / float64(ri), true
		case "%":
			if ri == 0 {
				return nil, false
			}
			return li % ri, true
		case "**", "^":
			if ri >= 0 {
				out := int64(1)
				for i := int64(0); i < ri; i++ {
					out *= li
				}
				return out, true
			}
		case "<":
			return li < ri, true
		case ">":
			return li > ri, true
		case "<=":
			return li <= ri, true
		case ">=":
			return li >= ri, true
		}
	}
	lf, lok := toFloat(l)
	rf, rok := toFloat(r)
	if !lok || !rok {
		return nil, false
	}
	switch op {
	case "+":
		return lf + rf, true
	case "-":
		return lf - rf, true
	case "*":
		return lf * rf, true
	case "/":
		return lf / rf, true
	case "%":
		return math.Mod(lf, rf), true
	case "**", "^":
		return math.Pow(lf, rf), true
	case "<":
		return lf < rf, true
	case ">":
		return lf > rf, true
	case "<=":
		return lf <= rf, true
	case ">=":
		return lf >= rf, true
	}
	return nil, false
}

func toFloat(v Value) (float64, bool) {
	switch n := v.(type) {
	case int64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

// equal compares two sequences of values; numbers compare by value.
func equal(l, r []Value) bool {
	if len(l) != len(r) {
		return false
	}
	for i := range l {
		if lq, ok := l[i].(Quantity); ok { // same unit: quantitiesComparable has checked
			rq, ok := r[i].(Quantity)
			lf, _ := toFloat(lq.Number)
			rf, _ := toFloat(rq.Number)
			if !ok || lf != rf {
				return false
			}
			continue
		}
		if lf, ok := toFloat(l[i]); ok {
			if rf, ok := toFloat(r[i]); ok && lf == rf {
				continue
			}
			return false
		}
		if l[i] != r[i] {
			return false
		}
	}
	return true
}

// FormatValue writes a value as KerML would: strings quoted, sequences in
// parentheses, null as null.
func FormatValue(v Value) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		if math.IsInf(x, 1) {
			return "*"
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	case string:
		return strconv.Quote(x)
	case Quantity:
		return x.String()
	case Seq:
		if len(x) == 0 {
			return "null"
		}
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = FormatValue(e)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	}
	return fmt.Sprint(v)
}
