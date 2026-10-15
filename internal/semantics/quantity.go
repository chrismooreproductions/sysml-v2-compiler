// SPDX-License-Identifier: EPL-2.0

package semantics

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// A UnitFactor is a unit raised to a whole power: the km and the h^-1 of km/h.
type UnitFactor struct {
	Unit     *el
	Exponent int
}

// A Quantity is a number with a unit: the value of 1800 [kg] or 100 [km/h].
// Number is an int64 or a float64. The unit is Unit, a unit feature
// (SI::kilogram), or for a derived unit Factors, the units it is a product of
// (km and h^-1); exactly one of the two is set.
type Quantity struct {
	Number  Value
	Unit    *el
	Factors []UnitFactor
	// Inexact is true when the number depends on a conversion factor the
	// library gives as approximate (isExact = false), such as the dalton.
	Inexact bool
}

// UnitFactors are the units the quantity's unit is a product of.
func (q Quantity) UnitFactors() []UnitFactor {
	if q.Unit != nil {
		return []UnitFactor{{q.Unit, 1}}
	}
	return q.Factors
}

func symbolOf(u *el) string {
	if s := u.Text("declaredShortName"); s != "" {
		return s
	}
	return u.Text("declaredName")
}

// Symbol writes the unit as its symbols: kg, km/h, kg*m/s^2, 1/s.
func (q Quantity) Symbol() string {
	var num, den []string
	for _, f := range q.UnitFactors() {
		e := f.Exponent
		if e < 0 {
			e = -e
		}
		s := symbolOf(f.Unit)
		if e != 1 {
			s += "^" + strconv.Itoa(e)
		}
		if f.Exponent > 0 {
			num = append(num, s)
		} else {
			den = append(den, s)
		}
	}
	n := strings.Join(num, "*")
	if n == "" && len(den) > 0 {
		n = "1"
	}
	if len(den) == 0 {
		return n
	}
	return n + "/" + strings.Join(den, "*")
}

func (q Quantity) String() string { return FormatValue(q.Number) + " [" + q.Symbol() + "]" }

// combine multiplies out a list of unit factors: the same unit twice is one
// factor with the exponents added, and a unit that cancels is dropped.
func combine(fs []UnitFactor) []UnitFactor {
	var out []UnitFactor
	for _, f := range fs {
		merged := false
		for i := range out {
			if out[i].Unit == f.Unit {
				out[i].Exponent += f.Exponent
				merged = true
				break
			}
		}
		if !merged {
			out = append(out, f)
		}
	}
	kept := out[:0]
	for _, f := range out {
		if f.Exponent != 0 {
			kept = append(kept, f)
		}
	}
	return kept
}

func invert(fs []UnitFactor) []UnitFactor {
	out := make([]UnitFactor, len(fs))
	for i, f := range fs {
		out[i] = UnitFactor{f.Unit, -f.Exponent}
	}
	return out
}

// newQuantity makes the value of a number in a product of units: a plain
// number if the units cancel, a quantity with a unit if it is one unit to the
// first power, otherwise a quantity with a derived unit.
func newQuantity(n Value, fs []UnitFactor, inexact bool) Value {
	fs = combine(fs)
	switch {
	case len(fs) == 0:
		return n
	case len(fs) == 1 && fs[0].Exponent == 1:
		return Quantity{Number: n, Unit: fs[0].Unit, Inexact: inexact}
	}
	return Quantity{Number: n, Factors: fs, Inexact: inexact}
}

// quantity evaluates "n [u]": a number in a unit, where u is a unit or a
// product, quotient or power of units (km/h, kg*m/s^2). The unit is not
// evaluated, since a unit has no value.
func (ev *evaluator) quantity(e *el) ([]Value, error) {
	args, err := ev.operands(e, 2)
	if err != nil {
		return nil, err
	}
	n, err := ev.number(args[0])
	if err != nil {
		return nil, err
	}
	fs, err := ev.unitFactors(args[1])
	if err != nil {
		return nil, err
	}
	return []Value{newQuantity(n, fs, false)}, nil
}

// unitFactors reads a unit expression: a unit, or units multiplied, divided and
// raised to a whole power; 1 is the empty product.
func (ev *evaluator) unitFactors(arg *el) ([]UnitFactor, error) {
	w := ev.w
	x := ev.deferred(arg)
	switch {
	case x.IsA("FeatureReferenceExpression"):
		unit := w.referencedElement(x)
		if unit == nil {
			return nil, ev.fail(arg, "the unit does not resolve")
		}
		if !w.isUnit(unit) {
			return nil, ev.fail(arg, "%s is not a unit", displayName(unit))
		}
		return []UnitFactor{{unit, 1}}, nil
	case x.IsA("LiteralInteger") && x.Text("value") == "1":
		return nil, nil
	case x.IsA("OperatorExpression") && !x.IsA("FeatureChainExpression"):
		switch op := w.operatorOf(x); op {
		case "*", "/":
			args, err := ev.operands(x, 2)
			if err != nil {
				return nil, err
			}
			l, err := ev.unitFactors(args[0])
			if err != nil {
				return nil, err
			}
			r, err := ev.unitFactors(args[1])
			if err != nil {
				return nil, err
			}
			if op == "/" {
				r = invert(r)
			}
			return combine(append(l, r...)), nil
		case "^", "**":
			args, err := ev.operands(x, 2)
			if err != nil {
				return nil, err
			}
			base, err := ev.unitFactors(args[0])
			if err != nil {
				return nil, err
			}
			p, err := ev.number(args[1])
			if err != nil {
				return nil, err
			}
			n, ok := p.(int64)
			if !ok {
				return nil, ev.fail(args[1], "a unit can be raised to a whole power, not %s", FormatValue(p))
			}
			out := make([]UnitFactor, len(base))
			for i, f := range base {
				out[i] = UnitFactor{f.Unit, f.Exponent * int(n)}
			}
			return combine(out), nil
		}
	}
	return nil, ev.fail(arg, "only a unit, or units multiplied, divided or raised to a power, can follow a number in [ ]")
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

// A dimScale says what a unit is made of: the units that have no conversion
// and no definition in terms of others (the root units), to whole powers, and
// the number of that product that one of the unit is. Kilometre is {m: 1} and
// 1000; newton is {g: 1, m: 1, s: -2} and 1000 (kilogram is 1000 g).
type dimScale struct {
	dim     map[*el]int
	scale   float64
	inexact bool
}

// unitDim expresses a unit in root units: through its unitConversion (kilometre
// is 1000 metres, hour is 60 minutes), or through its definition as a product of
// other units (newton = kg*m/s^2); a unit with neither is a root.
func (ev *evaluator) unitDim(u *el, seen map[*el]bool) (dimScale, string) {
	w := ev.w
	if seen[u] {
		return dimScale{}, displayName(u) + " is defined in terms of itself"
	}
	seen[u] = true
	defer delete(seen, u)
	if ref, factor, exact, found, why := w.conversionOf(u); why != "" {
		return dimScale{}, why
	} else if found {
		d, why := ev.unitDim(ref, seen)
		if why != "" {
			return d, why
		}
		return dimScale{d.dim, d.scale * factor, d.inexact || !exact}, ""
	}
	if expr := w.valueExpression(u); expr != nil {
		if fs, err := ev.unitFactors(expr); err == nil && len(fs) > 0 {
			return ev.factorsDim(fs, seen)
		}
	}
	return dimScale{map[*el]int{u: 1}, 1, false}, ""
}

// factorsDim expresses a product of units in root units.
func (ev *evaluator) factorsDim(fs []UnitFactor, seen map[*el]bool) (dimScale, string) {
	out := dimScale{dim: map[*el]int{}, scale: 1}
	for _, f := range fs {
		d, why := ev.unitDim(f.Unit, seen)
		if why != "" {
			return out, why
		}
		for root, e := range d.dim {
			out.dim[root] += e * f.Exponent
		}
		out.scale *= math.Pow(d.scale, float64(f.Exponent))
		out.inexact = out.inexact || d.inexact
	}
	for root, e := range out.dim {
		if e == 0 {
			delete(out.dim, root)
		}
	}
	return out, ""
}

func sameDim(a, b map[*el]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func (ev *evaluator) quantityDim(q Quantity) (dimScale, string) {
	return ev.factorsDim(q.UnitFactors(), map[*el]bool{})
}

// convert expresses a quantity in the unit of another. why says why that cannot
// be done: the units measure different quantities, or the library gives no way
// from one to the other.
func (ev *evaluator) convert(q Quantity, to Quantity) (n Value, inexact bool, why string) {
	from, why := ev.quantityDim(q)
	if why != "" {
		return nil, false, why
	}
	target, why := ev.quantityDim(to)
	if why != "" {
		return nil, false, why
	}
	if !sameDim(from.dim, target.dim) {
		if q.Unit != nil && to.Unit != nil && !ev.w.unrelatedUnits(q.Unit, to.Unit) {
			return nil, false, fmt.Sprintf("the library gives no conversion from %s to %s", q.Symbol(), to.Symbol())
		}
		return nil, false, fmt.Sprintf("%s and %s measure different quantities", to.Symbol(), q.Symbol())
	}
	f, _ := toFloat(q.Number)
	x := f * from.scale / target.scale
	inexact = from.inexact || target.inexact
	if _, isInt := q.Number.(int64); isInt && x == math.Trunc(x) && math.Abs(x) < 1<<53 {
		return int64(x), inexact, ""
	}
	return x, inexact, ""
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

// quantityOp applies an operator when an operand is a quantity. A non-empty
// reason says why it cannot be applied. Quantities in different units of the
// same quantity are first expressed in the left operand's unit; a product or
// quotient of quantities is a quantity in a derived unit, or a plain number if
// the units cancel; a plain number scales a quantity.
func (ev *evaluator) quantityOp(op string, l, r Value) (v Value, reason string) {
	lq, lok := l.(Quantity)
	rq, rok := r.(Quantity)
	converted := false
	switch op {
	case "+", "-", "<", ">", "<=", ">=", "==", "!=":
		if lok && rok && !sameUnits(lq, rq) {
			n, inexact, why := ev.convert(rq, lq)
			if why != "" {
				return nil, fmt.Sprintf("cannot apply %s to %s and %s: %s", op, FormatValue(l), FormatValue(r), why)
			}
			rq = Quantity{Number: n, Unit: lq.Unit, Factors: lq.Factors, Inexact: rq.Inexact || inexact}
			converted = true
		}
	}
	same := lok && rok && sameUnits(lq, rq)
	inexact := lq.Inexact || rq.Inexact
	switch op {
	case "+", "-":
		if same {
			if n, ok := binary(op, lq.Number, rq.Number); ok {
				return Quantity{Number: n, Unit: lq.Unit, Factors: lq.Factors, Inexact: inexact}, ""
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
		case lok && rok:
			if n, ok := binary(op, lq.Number, rq.Number); ok {
				return newQuantity(n, append(append([]UnitFactor{}, lq.UnitFactors()...), rq.UnitFactors()...), inexact), ""
			}
		case lok:
			if n, ok := binary(op, lq.Number, r); ok {
				return Quantity{Number: n, Unit: lq.Unit, Factors: lq.Factors, Inexact: lq.Inexact}, ""
			}
		case rok:
			if n, ok := binary(op, l, rq.Number); ok {
				return Quantity{Number: n, Unit: rq.Unit, Factors: rq.Factors, Inexact: rq.Inexact}, ""
			}
		}
	case "/":
		switch {
		case lok && rok:
			// units of the same quantity cancel, whatever they are
			ld, lwhy := ev.quantityDim(lq)
			rd, rwhy := ev.quantityDim(rq)
			if lwhy == "" && rwhy == "" && sameDim(ld.dim, rd.dim) {
				if sameUnits(lq, rq) {
					if n, ok := binary(op, lq.Number, rq.Number); ok {
						return n, ""
					}
				}
				lf, _ := toFloat(lq.Number)
				rf, _ := toFloat(rq.Number)
				if rf != 0 {
					return lf * ld.scale / (rf * rd.scale), ""
				}
				break
			}
			if n, ok := binary(op, lq.Number, rq.Number); ok {
				return newQuantity(n, append(append([]UnitFactor{}, lq.UnitFactors()...), invert(rq.UnitFactors())...), inexact), ""
			}
		case lok:
			if n, ok := binary(op, lq.Number, r); ok {
				return Quantity{Number: n, Unit: lq.Unit, Factors: lq.Factors, Inexact: lq.Inexact}, ""
			}
		case rok:
			if n, ok := binary(op, l, rq.Number); ok {
				return newQuantity(n, invert(rq.UnitFactors()), rq.Inexact), ""
			}
		}
	}
	if lok != rok && strings.ContainsAny(op, "+-<>=!") {
		return nil, fmt.Sprintf("cannot apply %s to %s and %s: a quantity and a number", op, FormatValue(l), FormatValue(r))
	}
	return nil, fmt.Sprintf("cannot apply %s to %s and %s", op, FormatValue(l), FormatValue(r))
}

// sameUnits is whether two quantities are written in the same unit.
func sameUnits(a, b Quantity) bool {
	af, bf := a.UnitFactors(), b.UnitFactors()
	if len(af) != len(bf) {
		return false
	}
	for i := range af {
		if af[i] != bf[i] {
			return false
		}
	}
	return true
}

// nearlyEqual is equality of two numbers that went through a conversion: a
// factor such as 0.3048 is not exact in binary, so 3 ft is not exactly 0.9144 m.
func nearlyEqual(a, b float64) bool {
	d := math.Abs(a - b)
	return d <= 1e-12*math.Max(math.Abs(a), math.Abs(b))
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

// quantitiesComparable checks that two sequences compared for equality hold, if
// quantities, quantities in the same unit. (A single value in another unit is
// converted by quantityOp.)
func quantitiesComparable(op string, l, r []Value) string {
	for i := 0; i < len(l) && i < len(r); i++ {
		lq, lok := l[i].(Quantity)
		rq, rok := r[i].(Quantity)
		switch {
		case lok && rok && !sameUnits(lq, rq):
			return fmt.Sprintf("cannot apply %s to %s and %s: converting between units in a sequence is not supported", op, FormatValue(l[i]), FormatValue(r[i]))
		case lok != rok && (lok || rok):
			return fmt.Sprintf("cannot apply %s to %s and %s: a quantity and a number", op, FormatValue(l[i]), FormatValue(r[i]))
		}
	}
	return ""
}
