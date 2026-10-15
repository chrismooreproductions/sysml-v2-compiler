// SPDX-License-Identifier: EPL-2.0

package semantics

import (
	"fmt"
	"math"
	"strings"
)

// A Quantity is a number with a unit: the value of 1800 [kg]. Number is an
// int64 or a float64, and Unit the unit feature (SI::kilogram).
type Quantity struct {
	Number Value
	Unit   *el
	// Inexact is true when the number depends on a conversion factor the
	// library gives as approximate (isExact = false), such as the dalton.
	Inexact bool
}

// Symbol is the unit's short name (kg), or its name if it has none.
func (q Quantity) Symbol() string {
	if q.Unit == nil {
		return ""
	}
	if s := q.Unit.Text("declaredShortName"); s != "" {
		return s
	}
	return q.Unit.Text("declaredName")
}

func (q Quantity) String() string { return FormatValue(q.Number) + " [" + q.Symbol() + "]" }

// quantity evaluates "n [u]": a number in a unit. The unit is the feature the
// second operand names; it is not evaluated, since a unit has no value.
func (ev *evaluator) quantity(e *el) ([]Value, error) {
	args, err := ev.operands(e, 2)
	if err != nil {
		return nil, err
	}
	n, err := ev.number(args[0])
	if err != nil {
		return nil, err
	}
	ref := ev.deferred(args[1])
	if !ref.IsA("FeatureReferenceExpression") {
		return nil, ev.fail(args[1], "only a unit can follow a number in [ ]: derived units are not supported")
	}
	var unit *el
	for _, m := range ev.w.rels(ref, "Membership") {
		if !m.IsA("ParameterMembership") {
			unit = ev.w.memberElement(m)
			break
		}
	}
	if unit == nil {
		return nil, ev.fail(args[1], "the unit does not resolve")
	}
	if !ev.w.isUnit(unit) {
		return nil, ev.fail(args[1], "%s is not a unit", displayName(unit))
	}
	return []Value{Quantity{Number: n, Unit: unit}}, nil
}

// isUnit is whether a feature is a measurement unit: one of its types
// specializes MeasurementReferences::MeasurementUnit.
func (w *Workspace) isUnit(f *el) bool {
	if !f.IsA("Feature") {
		return false
	}
	mu := w.libraryElement("MeasurementReferences::MeasurementUnit")
	if mu == nil {
		return false
	}
	for _, t := range w.typesOf(f) {
		if t == mu || w.specializes(t, mu) {
			return true
		}
	}
	return false
}

func isQuantity(v Value) bool { _, ok := v.(Quantity); return ok }

// quantityOp applies an operator when an operand is a quantity. A non-empty
// reason says why it cannot be applied. Only what has a definite answer is
// supported: units that convert into one another (km and m, h and s), or a plain
// number scaling a quantity. Quantities in different units are first expressed
// in the left operand's unit.
func (ev *evaluator) quantityOp(op string, l, r Value) (v Value, reason string) {
	lq, lok := l.(Quantity)
	rq, rok := r.(Quantity)
	converted := false
	switch op {
	case "+", "-", "<", ">", "<=", ">=", "==", "!=":
		if lok && rok && lq.Unit != rq.Unit {
			n, inexact, why := ev.convert(rq, lq.Unit)
			if why != "" {
				return nil, fmt.Sprintf("cannot apply %s to %s and %s: %s", op, FormatValue(l), FormatValue(r), why)
			}
			rq = Quantity{n, lq.Unit, rq.Inexact || inexact}
			converted = true
		}
	}
	same := lok && rok && lq.Unit == rq.Unit
	inexact := lq.Inexact || rq.Inexact
	switch op {
	case "+", "-":
		if same {
			if n, ok := binary(op, lq.Number, rq.Number); ok {
				return Quantity{n, lq.Unit, inexact}, ""
			}
		}
	case "<", ">", "<=", ">=", "==", "!=":
		if same {
			lf, _ := toFloat(lq.Number)
			rf, _ := toFloat(rq.Number)
			near := converted && nearlyEqual(lf, rf)
			if near && inexact {
				return nil, fmt.Sprintf("cannot apply %s to %s and %s: they are equal to within the precision of an inexact conversion factor", op, FormatValue(l), FormatValue(r))
			}
			switch op {
			case "==":
				return near || lf == rf, ""
			case "!=":
				return !(near || lf == rf), ""
			case "<=":
				return near || lf <= rf, ""
			case ">=":
				return near || lf >= rf, ""
			case "<":
				return !near && lf < rf, ""
			case ">":
				return !near && lf > rf, ""
			}
		}
	case "*":
		switch {
		case lok && !rok:
			if n, ok := binary(op, lq.Number, r); ok {
				return Quantity{n, lq.Unit, lq.Inexact}, ""
			}
		case rok && !lok:
			if n, ok := binary(op, l, rq.Number); ok {
				return Quantity{n, rq.Unit, rq.Inexact}, ""
			}
		case lok && rok:
			return nil, fmt.Sprintf("cannot multiply %s and %s: derived units are not supported", FormatValue(l), FormatValue(r))
		}
	case "/":
		switch {
		case lok && !rok:
			if n, ok := binary(op, lq.Number, r); ok {
				return Quantity{n, lq.Unit, lq.Inexact}, ""
			}
		case same:
			if n, ok := binary(op, lq.Number, rq.Number); ok {
				return n, "" // the units cancel
			}
		default:
			return nil, fmt.Sprintf("cannot divide %s by %s: derived units are not supported", FormatValue(l), FormatValue(r))
		}
	}
	if lok != rok && strings.ContainsAny(op, "+-<>=!") {
		return nil, fmt.Sprintf("cannot apply %s to %s and %s: a quantity and a number", op, FormatValue(l), FormatValue(r))
	}
	return nil, fmt.Sprintf("cannot apply %s to %s and %s", op, FormatValue(l), FormatValue(r))
}

// nearlyEqual is equality of two numbers that went through a conversion: a
// factor such as 0.3048 is not exact in binary, so 3 ft is not exactly 0.9144 m.
func nearlyEqual(a, b float64) bool {
	d := math.Abs(a - b)
	return d <= 1e-12*math.Max(math.Abs(a), math.Abs(b))
}

// A unitScale says how many of a root unit one of a unit is.
type unitScale struct {
	root    *el
	factor  float64
	inexact bool
}

// scaleOf follows a unit's conversions to a unit with none (kilometre to metre,
// hour to minute to second). why is not empty if a conversion cannot be read.
func (w *Workspace) scaleOf(u *el) (sc unitScale, why string) {
	sc = unitScale{root: u, factor: 1}
	seen := map[*el]bool{}
	for {
		if seen[sc.root] {
			return sc, displayName(sc.root) + " is defined in terms of itself"
		}
		seen[sc.root] = true
		ref, factor, exact, found, why := w.conversionOf(sc.root)
		if why != "" {
			return sc, why
		}
		if !found {
			return sc, ""
		}
		sc.root, sc.factor, sc.inexact = ref, sc.factor*factor, sc.inexact || !exact
	}
}

// conversionOf reads the unitConversion a unit declares: the unit it refers to,
// the factor (a number, or a unit prefix's), and whether the factor is exact.
func (w *Workspace) conversionOf(u *el) (ref *el, factor float64, exact, found bool, why string) {
	var conv *el
	for _, m := range w.rels(u, "FeatureMembership") {
		if f := first(w.ownedRelated(m)); f.IsA("Feature") && w.EffectiveName(f) == "unitConversion" {
			conv = f
		}
	}
	if conv == nil {
		return nil, 1, true, false, ""
	}
	exact = true
	number := func(x *el) (float64, bool) {
		v, err := w.Evaluate(x)
		if err != nil {
			return 0, false
		}
		f, ok := toFloat(v)
		return f, ok
	}
	for _, m := range w.rels(conv, "FeatureMembership") {
		g := first(w.ownedRelated(m))
		if !g.IsA("Feature") {
			continue
		}
		expr := w.valueExpression(g)
		if expr == nil {
			continue
		}
		switch w.EffectiveName(g) {
		case "referenceUnit":
			ref = w.referencedElement(expr)
		case "conversionFactor":
			if f, ok := number(expr); ok {
				factor = f
			}
		case "prefix":
			if p := w.referencedElement(expr); p != nil {
				for _, pm := range w.rels(p, "FeatureMembership") {
					if pf := first(w.ownedRelated(pm)); pf.IsA("Feature") && w.EffectiveName(pf) == "conversionFactor" {
						if x := w.valueExpression(pf); x != nil {
							if f, ok := number(x); ok {
								factor = f
							}
						}
					}
				}
			}
		case "isExact":
			if v, err := w.Evaluate(expr); err == nil && v == false {
				exact = false
			}
		}
	}
	if ref == nil || factor == 0 {
		return nil, 0, true, true, "the conversion of " + displayName(u) + " is not understood"
	}
	return ref, factor, exact, true, ""
}

// valueExpression is the expression a feature is given as its value.
func (w *Workspace) valueExpression(f *el) *el {
	for _, fv := range w.rels(f, "FeatureValue") {
		if x := first(w.ownedRelated(fv)); x != nil {
			return x
		}
	}
	return nil
}

// referencedElement is the element a feature reference expression names.
func (w *Workspace) referencedElement(x *el) *el {
	if !x.IsA("FeatureReferenceExpression") {
		return nil
	}
	for _, m := range w.rels(x, "Membership") {
		if !m.IsA("ParameterMembership") {
			return w.memberElement(m)
		}
	}
	return nil
}

// convert expresses a quantity in another unit of the same quantity. why says
// why that cannot be done: the units measure different quantities, or the
// library gives no way from one to the other.
func (ev *evaluator) convert(q Quantity, to *el) (n Value, inexact bool, why string) {
	w := ev.w
	from, why := w.scaleOf(q.Unit)
	if why != "" {
		return nil, false, why
	}
	target, why := w.scaleOf(to)
	if why != "" {
		return nil, false, why
	}
	if from.root != target.root {
		if w.unrelatedUnits(q.Unit, to) {
			return nil, false, fmt.Sprintf("%s and %s measure different quantities", Quantity{Unit: to}.Symbol(), q.Symbol())
		}
		return nil, false, fmt.Sprintf("the library gives no conversion from %s to %s", q.Symbol(), Quantity{Unit: to}.Symbol())
	}
	f, _ := toFloat(q.Number)
	x := f * from.factor / target.factor
	if _, isInt := q.Number.(int64); isInt && x == math.Trunc(x) && math.Abs(x) < 1<<53 {
		return int64(x), from.inexact || target.inexact, ""
	}
	return x, from.inexact || target.inexact, ""
}

// unrelatedUnits is whether two units are of unrelated kinds of unit (a MassUnit
// and a LengthUnit): none of the types of one specializes one of the other's.
func (w *Workspace) unrelatedUnits(a, b *el) bool {
	for _, ta := range w.typesOf(a) {
		for _, tb := range w.typesOf(b) {
			if ta == tb || w.specializes(ta, tb) || w.specializes(tb, ta) {
				return false
			}
		}
	}
	return true
}

// quantitiesComparable checks that two sequences compared for equality hold, if
// quantities, quantities in the same unit. (A single value in another unit is
// converted by quantityOp.)
func quantitiesComparable(op string, l, r []Value) string {
	for i := 0; i < len(l) && i < len(r); i++ {
		lq, lok := l[i].(Quantity)
		rq, rok := r[i].(Quantity)
		switch {
		case lok && rok && lq.Unit != rq.Unit:
			return fmt.Sprintf("cannot apply %s to %s and %s: converting between units in a sequence is not supported", op, FormatValue(l[i]), FormatValue(r[i]))
		case lok != rok && (lok || rok):
			return fmt.Sprintf("cannot apply %s to %s and %s: a quantity and a number", op, FormatValue(l[i]), FormatValue(r[i]))
		}
	}
	return ""
}
