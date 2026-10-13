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

// EvalError says why an expression cannot be evaluated.
type EvalError struct {
	Element *model.Element
	Msg     string
}

func (e *EvalError) Error() string { return e.Msg }

// Evaluate computes the value of an expression in a resolved workspace, as far
// as it can be computed from the model alone: literals, operators, sequences
// and references to features that have a value. It returns an *EvalError for
// anything else (calls of library functions, for example, are not yet
// supported).
func (w *Workspace) Evaluate(e *el) (Value, error) {
	if e == nil {
		return nil, &EvalError{nil, "no expression"}
	}
	vs, err := (&evaluator{w: w, active: map[*el]bool{}}).eval(e)
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
}

func (ev *evaluator) fail(e *el, format string, args ...any) error {
	return &EvalError{e, fmt.Sprintf(format, args...)}
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
		return ev.featureValue(r, e)
	}
	return nil, ev.fail(e, "a reference to nothing")
}

// featureValue is the value a feature is bound to, taken from the feature or,
// failing that, from a feature it redefines.
func (ev *evaluator) featureValue(f, at *el) ([]Value, error) {
	if !f.IsA("Feature") {
		return nil, ev.fail(at, "%s is not a feature", QualifiedName(f))
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
	return nil, ev.fail(at, "%s has no value", displayName(f))
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
		l, err := ev.boolean(args[0])
		if err != nil {
			return nil, err
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
		l, err := ev.eval(args[0])
		if err != nil {
			return nil, err
		}
		r, err := ev.eval(args[1])
		if err != nil {
			return nil, err
		}
		eq := equal(l, r)
		if op == "!=" || op == "!==" {
			eq = !eq
		}
		return []Value{eq}, nil
	case "xor", "&", "|":
		l, err := ev.boolean(args[0])
		if err != nil {
			return nil, err
		}
		r, err := ev.boolean(args[1])
		if err != nil {
			return nil, err
		}
		switch op {
		case "xor":
			return []Value{l != r}, nil
		case "&":
			return []Value{l && r}, nil
		}
		return []Value{l || r}, nil
	}
	l, err := ev.single(args[0])
	if err != nil {
		return nil, err
	}
	r, err := ev.single(args[1])
	if err != nil {
		return nil, err
	}
	v, ok := binary(op, l, r)
	if !ok {
		return nil, ev.fail(e, "cannot apply %s to %v and %v", op, l, r)
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
