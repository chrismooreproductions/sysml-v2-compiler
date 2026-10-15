// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"errors"
	"fmt"
)

// A Status says whether an expression could be evaluated.
type Status int

const (
	// Determined means the expression has a value: Result.Value and Result.Kind.
	Determined Status = iota
	// NotDetermined means the expression can be evaluated but a value it needs
	// is missing: a feature it refers to has no value. Binding that value
	// would determine it. Result.Reason names the feature, and Result.Source
	// the reference to it.
	NotDetermined
	// NotEvaluable means the evaluator cannot compute the expression: it uses
	// something that is not supported (a call of a library function, for
	// example), or the model is wrong (a reference that does not resolve, an
	// operator applied to the wrong kind of value). Result.Reason says which.
	NotEvaluable
)

func (s Status) String() string {
	switch s {
	case Determined:
		return "determined"
	case NotDetermined:
		return "not determined"
	}
	return "not evaluable"
}

// A ValueKind says what a Determined value is.
type ValueKind int

const (
	// KindNull is the empty sequence, which KerML calls null.
	KindNull ValueKind = iota
	// KindBoolean is a bool.
	KindBoolean
	// KindInteger is an int64.
	KindInteger
	// KindReal is a float64 (a Rational or Real; an unbounded * is +Inf).
	KindReal
	// KindString is a string.
	KindString
	// KindSequence is a Seq of two or more values, or of one value that is
	// itself a sequence.
	KindSequence
)

func (k ValueKind) String() string {
	switch k {
	case KindNull:
		return "null"
	case KindBoolean:
		return "boolean"
	case KindInteger:
		return "integer"
	case KindReal:
		return "real"
	case KindString:
		return "string"
	}
	return "sequence"
}

// A Result is the outcome of evaluating an expression: a value that says what
// it is, or the reason there is none, and where it came from. It never holds a
// guessed value: a missing input makes the result NotDetermined, and anything
// unsupported makes it NotEvaluable.
type Result struct {
	// Status says whether there is a value.
	Status Status
	// Kind and Value are set when Status is Determined. Value is a bool,
	// int64, float64 or string for a single value, a Seq for a sequence, and
	// nil for null.
	Kind  ValueKind
	Value any
	// Reason says why there is no value; it is empty when Status is
	// Determined.
	Reason string
	// Source is the element the result is about: the expression that was
	// evaluated when there is a value, and the expression or feature that
	// could not be evaluated when there is not. Span is its source range.
	Source *Element
	Span   Span
}

// String writes the value as KerML would, or the status and reason.
func (r Result) String() string {
	if r.Status == Determined {
		return FormatValue(r.Value)
	}
	return fmt.Sprintf("%s: %s", r.Status, r.Reason)
}

// Eval evaluates an expression in a resolved workspace, as Evaluate does, and
// reports the outcome as a Result: the typed value, or whether it is not
// determined (an input has no value) or not evaluable (unsupported, or wrong),
// with the element and span responsible. Call it after Resolve.
//
// Boolean operators follow three-valued logic where it is sound: false and x
// is false, true or x is true, and x implies true is true, even when x is not
// determined.
func (w *Workspace) Eval(expression *Element) Result {
	v, err := w.Evaluate(expression)
	if err == nil {
		return determined(expression, v)
	}
	r := Result{Status: NotEvaluable, Reason: err.Error(), Source: expression}
	var ee *EvalError
	if errors.As(err, &ee) {
		r.Reason = ee.Msg
		if ee.Element != nil {
			r.Source = ee.Element
		}
		if ee.Unbound {
			r.Status = NotDetermined
		}
	}
	if r.Source != nil {
		r.Span = r.Source.Span
	}
	return r
}

func determined(e *Element, v any) Result {
	r := Result{Status: Determined, Value: v, Source: e}
	if e != nil {
		r.Span = e.Span
	}
	switch x := v.(type) {
	case bool:
		r.Kind = KindBoolean
	case int64:
		r.Kind = KindInteger
	case float64:
		r.Kind = KindReal
	case string:
		r.Kind = KindString
	case Seq:
		r.Kind = KindSequence
		if len(x) == 0 {
			r.Kind, r.Value = KindNull, nil
		}
	default:
		r.Kind = KindNull
	}
	return r
}
