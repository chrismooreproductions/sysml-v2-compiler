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
		if name := w.files[root]; strings.HasPrefix(name, "library:") {
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

// QualifiedName names an element by the declared names along its ownership
// chain, such as "Vehicles::Car::engine". An element with no name on its chain
// gets "". A name that is not a plain identifier is quoted.
func QualifiedName(e *el) string {
	var parts []string
	for ; e != nil; e = e.Owner {
		if n := e.Text("declaredName"); n != "" {
			parts = append(parts, quoteName(n))
		} else if n := e.Text("declaredShortName"); n != "" {
			parts = append(parts, quoteName(n))
		}
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "::")
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
