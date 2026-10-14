// SPDX-License-Identifier: EPL-2.0

package semantics

import (
	"strings"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
)

// Unresolved describes a name that did not resolve to an element.
type Unresolved struct {
	Ref   *model.Ref
	Role  string // the property that holds the reference
	Owner *el    // the element that holds it
	Scope *el    // the namespace the search started in; nil if only the global scope was searched
	File  string
}

// Unresolved lists the references that Resolve could not resolve, in source
// order within each file. Call it after Resolve.
func (w *Workspace) Unresolved() []Unresolved {
	var out []Unresolved
	for _, root := range w.roots {
		if w.isLibrary(root) {
			continue // the standard library is not the user's to fix
		}
		root.Walk(func(e *el) {
			for _, p := range e.Props {
				if r, ok := p.Value.(*model.Ref); ok && r.Target == nil && r.Name != "" {
					out = append(out, Unresolved{Ref: r, Role: p.Name, Owner: e, Scope: w.scopeFor(e, p.Name).ns, File: w.files[root]})
				}
			}
		})
	}
	return out
}

// QualifiedName names an element by the names along its ownership chain, such
// as "Vehicles::Car::engine". A feature without a declared name is named, as
// Element.effectiveName has it, by the feature it redefines (once the
// workspace is resolved). An element with no name on its chain gets "". A name
// that is not a plain identifier is quoted.
func QualifiedName(e *el) string {
	var parts []string
	for ; e != nil; e = e.Owner {
		if n := nameForQualification(e, map[*el]bool{}); n != "" {
			parts = append(parts, quoteName(n))
		}
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "::")
}

// nameForQualification is the name an element has in a qualified name: its
// declared name, else its declared short name, else for a feature the name of
// the first feature it redefines.
func nameForQualification(e *el, seen map[*el]bool) string {
	if n := e.Text("declaredName"); n != "" {
		return n
	}
	if n := e.Text("declaredShortName"); n != "" {
		return n
	}
	if !e.IsA("Feature") || seen[e] {
		return ""
	}
	seen[e] = true
	for _, r := range e.Children("ownedRelationship") {
		if r.IsTrue("isImplied") || !r.IsA("Redefinition") {
			continue
		}
		for _, ref := range r.Refs("redefinedFeature") {
			if ref.Target != nil {
				return nameForQualification(ref.Target, seen)
			}
		}
		return "" // only the first redefinition names the feature
	}
	return ""
}

func quoteName(n string) string {
	for i := 0; i < len(n); i++ {
		c := n[i]
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9') {
			return "'" + strings.ReplaceAll(n, "'", `\'`) + "'"
		}
	}
	return n
}
