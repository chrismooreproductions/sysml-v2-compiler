// SPDX-License-Identifier: EPL-2.0

package sysml

import "fmt"

// EvalConstraint evaluates a constraint usage or definition: whether its
// condition holds, given the values in scope. The result is a boolean when
// Determined; NotDetermined when a value it reads is missing, and NotEvaluable
// for what is not supported.
//
// The context is the usage whose values the condition reads. A constraint in a
// requirement definition reads the definition's attributes (massActual), and a
// requirement usage gives them values by redefining them:
//
//	requirement r : MassReq { attribute :>> massActual = 1800; }
//
// so EvalConstraint(c, r) evaluates c's condition for r. With a nil context a
// constraint usage is its own context (constraint c : Ok { in :>> x = 5; }),
// and a definition reads the values it declares. Call it after Resolve.
func (w *Workspace) EvalConstraint(constraint, context *Element) Result {
	if constraint == nil || !(constraint.IsA("ConstraintUsage") || constraint.IsA("ConstraintDefinition")) {
		return notEvaluable(constraint, "not a constraint")
	}
	expr := w.w.ResultExpression(constraint)
	if expr == nil {
		return notEvaluable(constraint, "the constraint has no condition")
	}
	if context == nil && constraint.IsA("Usage") {
		context = constraint
	}
	v, err := w.w.EvaluateWith(expr, context, nil)
	r := resultOf(expr, v, err)
	if r.Status == Determined && r.Kind != KindBoolean {
		return notEvaluable(expr, fmt.Sprintf("a constraint must be a boolean, not %s", r.Kind))
	}
	return r
}

// EvalCalculation evaluates a calculation usage or definition (or a function):
// the value of its return parameter or result expression. The inputs give the
// calculation's in parameters by name, taking the place of any value they have;
// an input that is not given uses its own value, and is NotDetermined if it has
// none. A usage reads the inputs it binds itself:
//
//	calc rc : Range { in attribute :>> tankSize = 40; in attribute :>> consumption = 8; }
//
// A value is a bool, an int or int64, a float64 or a string. Call it after
// Resolve.
func (w *Workspace) EvalCalculation(calc *Element, inputs map[string]any) Result {
	if calc == nil || !(calc.IsA("CalculationUsage") || calc.IsA("CalculationDefinition") || calc.IsA("Function")) {
		return notEvaluable(calc, "not a calculation")
	}
	in := map[string]any{}
	for name, v := range inputs {
		switch x := v.(type) {
		case bool, int64, float64, string:
			in[name] = x
		case int:
			in[name] = int64(x)
		case int32:
			in[name] = int64(x)
		case float32:
			in[name] = float64(x)
		default:
			return notEvaluable(calc, fmt.Sprintf("input %s is a %T, not a boolean, number or string", name, v))
		}
	}
	v, err := w.w.EvaluateCall(calc, in)
	return resultOf(calc, v, err)
}

// A ConstraintOutcome is the result of one constraint of a requirement.
type ConstraintOutcome struct {
	// Constraint is the constraint usage.
	Constraint *Element
	// Kind is "assume" or "require".
	Kind string
	// Result is whether its condition holds for the requirement.
	Result Result
}

// EvalRequirement evaluates the assumed and required constraints of a
// requirement usage or definition, including those it inherits from its
// definition and the requirements it specializes, each for the requirement
// itself. The requirement's own constraints come first, in source order. It
// does not combine them: a requirement holds when its required constraints do
// given that its assumed constraints do, and that is for the caller to say.
func (w *Workspace) EvalRequirement(requirement *Element) []ConstraintOutcome {
	if requirement == nil || !(requirement.IsA("RequirementUsage") || requirement.IsA("RequirementDefinition")) {
		return nil
	}
	var out []ConstraintOutcome
	seen := map[*Element]bool{}
	queue := []*Element{requirement}
	for i := 0; i < len(queue); i++ {
		x := queue[i]
		if seen[x] {
			continue
		}
		seen[x] = true
		for _, m := range ownedRels(x) {
			if !m.IsA("RequirementConstraintMembership") || m.IsA("RequirementVerificationMembership") {
				continue
			}
			kind := map[string]string{"assumption": "assume", "requirement": "require"}[m.Text("kind")]
			for _, c := range m.Children("ownedRelatedElement") {
				if !c.Generated {
					out = append(out, ConstraintOutcome{c, kind, w.EvalConstraint(c, requirement)})
				}
			}
		}
		queue = append(queue, w.Supertypes(x)...)
	}
	return out
}

// notEvaluable is a result for a request that cannot be evaluated at all.
func notEvaluable(e *Element, reason string) Result {
	r := Result{Status: NotEvaluable, Reason: reason, Source: e}
	if e != nil {
		r.Span = e.Span
	}
	return r
}
