// SPDX-License-Identifier: EPL-2.0

package semantics

import "fmt"

// A Quantity is a number with a unit: the value of 1800 [kg]. Number is an
// int64 or a float64, and Unit the unit feature (SI::kilogram).
type Quantity struct {
	Number Value
	Unit   *el
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
	return []Value{Quantity{n, unit}}, nil
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
// supported: the same unit throughout, or a plain number scaling a quantity.
func quantityOp(op string, l, r Value) (v Value, reason string) {
	lq, lok := l.(Quantity)
	rq, rok := r.(Quantity)
	same := lok && rok && lq.Unit == rq.Unit
	switch op {
	case "+", "-", "<", ">", "<=", ">=":
		if same {
			v, ok := binary(op, lq.Number, rq.Number)
			if !ok {
				return nil, fmt.Sprintf("cannot apply %s to %s and %s", op, FormatValue(l), FormatValue(r))
			}
			if op == "+" || op == "-" {
				return Quantity{v, lq.Unit}, ""
			}
			return v, ""
		}
		if lok && rok {
			return nil, fmt.Sprintf("cannot apply %s to %s and %s: converting between units is not supported", op, FormatValue(l), FormatValue(r))
		}
		return nil, fmt.Sprintf("cannot apply %s to %s and %s: a quantity and a number", op, FormatValue(l), FormatValue(r))
	case "*":
		switch {
		case lok && !rok:
			if n, ok := binary(op, lq.Number, r); ok {
				return Quantity{n, lq.Unit}, ""
			}
		case rok && !lok:
			if n, ok := binary(op, l, rq.Number); ok {
				return Quantity{n, rq.Unit}, ""
			}
		case lok && rok:
			return nil, fmt.Sprintf("cannot multiply %s and %s: derived units are not supported", FormatValue(l), FormatValue(r))
		}
	case "/":
		switch {
		case lok && !rok:
			if n, ok := binary(op, lq.Number, r); ok {
				return Quantity{n, lq.Unit}, ""
			}
		case same:
			if n, ok := binary(op, lq.Number, rq.Number); ok {
				return n, "" // the units cancel
			}
		case lok && rok:
			return nil, fmt.Sprintf("cannot divide %s by %s: derived units are not supported", FormatValue(l), FormatValue(r))
		default:
			return nil, fmt.Sprintf("cannot divide %s by %s: derived units are not supported", FormatValue(l), FormatValue(r))
		}
	}
	return nil, fmt.Sprintf("cannot apply %s to %s and %s", op, FormatValue(l), FormatValue(r))
}

// quantitiesComparable checks that two values compared for equality are, if
// quantities, in the same unit: equal numbers in different units are not
// known to be equal or different without converting.
func quantitiesComparable(op string, l, r []Value) string {
	for i := 0; i < len(l) && i < len(r); i++ {
		lq, lok := l[i].(Quantity)
		rq, rok := r[i].(Quantity)
		switch {
		case lok && rok && lq.Unit != rq.Unit:
			return fmt.Sprintf("cannot apply %s to %s and %s: converting between units is not supported", op, FormatValue(l[i]), FormatValue(r[i]))
		case lok != rok && (lok || rok):
			return fmt.Sprintf("cannot apply %s to %s and %s: a quantity and a number", op, FormatValue(l[i]), FormatValue(r[i]))
		}
	}
	return ""
}
