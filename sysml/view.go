// SPDX-License-Identifier: EPL-2.0

package sysml

import (
	"fmt"
	"strings"
)

// A ViewElement is one element a view shows.
type ViewElement struct {
	// Element is the exposed element.
	Element *Element
	// Via is the expose relationship that brought it in; when two exposes
	// bring in the same element it is the first.
	Via *Element
	// Library is true for an element of the standard library, which a view
	// shows only when it exposes a library namespace (expose ScalarValues::*).
	Library bool
	// Undecided are the indexes, into ViewResult.Filters, of the filters that
	// could not be decided for this element (a value they read is missing, or
	// they use something not supported). The element is shown all the same:
	// no filter that could be decided removed it.
	Undecided []int
}

// An AppliedFilter is a filter condition of a view and how far it could be
// applied.
type AppliedFilter struct {
	Filter Filter
	// Owner is the view, view definition or view it specializes that writes
	// the filter. It is not the view itself for an inherited filter.
	Owner *Element
	// Applied is true when the filter was decided for every exposed element.
	// When it is false, Reason says why the first element it failed on could
	// not be decided (the Status is in Failure), and Undecided counts the
	// elements it could not be decided for. An undecided filter removes
	// nothing.
	Applied   bool
	Undecided int
	Reason    string
	Failure   Status
}

// A ViewResult is what a view shows.
type ViewResult struct {
	// Exposes are the expose relationships in effect: the view's own, then
	// those of its definition and of the views it specializes.
	Exposes []*Element
	// Unresolved are the exposes among them that name nothing.
	Unresolved []*Element
	// Filters are the filter conditions in effect, own first, then inherited.
	Filters []AppliedFilter
	// Elements are the exposed elements no decided filter removed, in order:
	// exposes in the order above, and within each the order the namespace
	// lists its members. An element exposed twice appears once.
	Elements []ViewElement
}

// ViewContent evaluates a view: the elements its exposes bring in (membership,
// namespace and recursive forms, with the exposes and filters of its definition
// and of the views it specializes) that pass its filters. Annotating elements
// (comments, documentation, metadata) and the expressions that members own are
// not elements of the content.
//
// A filter that cannot be decided for an element does not remove it; the result
// says which filters could not be applied and why, instead of guessing. Call it
// after Resolve. It returns an error only if the element is not a view.
func (w *Workspace) ViewContent(view *Element) (ViewResult, error) {
	if view == nil || !(view.IsA("ViewUsage") || view.IsA("ViewDefinition")) {
		return ViewResult{}, fmt.Errorf("not a view")
	}
	var res ViewResult
	for _, src := range w.viewSources(view) {
		for _, r := range ownedRels(src) {
			if r.IsA("Expose") {
				res.Exposes = append(res.Exposes, r)
			}
		}
		for _, f := range w.Facts(src).Filters {
			res.Filters = append(res.Filters, AppliedFilter{Filter: f, Owner: src, Applied: true})
		}
	}

	seen := map[*Element]bool{}
	for _, x := range res.Exposes {
		found := w.w.ImportedElements(x)
		if len(found) == 0 && !exposeResolves(x) {
			res.Unresolved = append(res.Unresolved, x)
		}
		fromLibrary := isLibraryElement(w, exposedTarget(x))
		for _, e := range found {
			if seen[e] || !showable(e) {
				continue
			}
			// what a model's namespace inherits from the standard library
			// (start, done, the features of Occurrence ...) is not its content
			if !fromLibrary && isLibraryElement(w, e) {
				continue
			}
			seen[e] = true
			res.Elements = append(res.Elements, ViewElement{Element: e, Via: x, Library: isLibraryElement(w, e)})
		}
	}

	kept := res.Elements[:0]
	for _, ve := range res.Elements {
		removed := false
		for i := range res.Filters {
			af := &res.Filters[i]
			r := w.Matches(af.Filter, ve.Element)
			switch {
			case r.Status == Determined && r.Value == false:
				removed = true
			case r.Status != Determined:
				ve.Undecided = append(ve.Undecided, i)
				if af.Applied {
					af.Applied, af.Reason, af.Failure = false, r.Reason, r.Status
				}
				af.Undecided++
			}
		}
		if !removed {
			kept = append(kept, ve)
		}
	}
	res.Elements = kept
	return res, nil
}

// viewSources are the view and, breadth first, the types it specializes: the
// elements whose exposes and filters apply to it.
func (w *Workspace) viewSources(view *Element) []*Element {
	out := []*Element{view}
	seen := map[*Element]bool{view: true}
	for i := 0; i < len(out); i++ {
		for _, s := range w.Supertypes(out[i]) {
			if !seen[s] && (s.IsA("ViewUsage") || s.IsA("ViewDefinition")) {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// exposedTarget is the element an expose names: the namespace, or the member.
func exposedTarget(x *Element) *Element {
	for _, r := range x.Refs("importedNamespace") {
		return r.Target
	}
	for _, r := range x.Refs("importedMembership") {
		if r.Target == nil {
			return nil
		}
		for _, o := range r.Target.Children("ownedRelatedElement") {
			return o
		}
		for _, m := range r.Target.Refs("memberElement") {
			return m.Target
		}
	}
	return nil
}

func isLibraryElement(w *Workspace, e *Element) bool {
	return e != nil && strings.HasPrefix(w.FileOf(e), "library:")
}

// exposeResolves is whether an expose names an element.
func exposeResolves(x *Element) bool {
	for _, role := range []string{"importedNamespace", "importedMembership"} {
		for _, r := range x.Refs(role) {
			if r.Target != nil {
				return true
			}
		}
	}
	return false
}

// showable is whether an imported element is part of a view's content: not an
// annotation, not a generated element, and not an expression a member owns
// (a filter condition, a value, a result).
func showable(e *Element) bool {
	if e.Generated || e.IsA("AnnotatingElement") || e.IsA("Multiplicity") {
		return false
	}
	if m := e.Owner; m.IsA("ElementFilterMembership") || m.IsA("FeatureValue") || m.IsA("ParameterMembership") || m.IsA("ResultExpressionMembership") {
		return false
	}
	return true
}
