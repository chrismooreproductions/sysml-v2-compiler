// SPDX-License-Identifier: EPL-2.0

// Package sysml reads SysML v2 and KerML textual notation and builds the
// specification's own model of it: elements typed by the real metaclasses, with
// source positions, the structure the specification implies but the syntax does
// not write, and names resolved against each other and against the standard
// library.
//
// Use Parse for a single file when names do not matter. Use a Workspace when
// they do: add the files, call Resolve, and read each reference's Target.
//
//	w, err := sysml.NewWorkspace()
//	if err != nil { ... }
//	doc, err := w.Add("vehicle.sysml", src)
//	if err != nil { ... } // a *SyntaxError with the position
//	for _, d := range w.Resolve() {
//		fmt.Println(d) // vehicle.sysml:12:9: cannot resolve "Engin" ...
//	}
//
// The model is generic: an Element has a metaclass name (Class), a source Span,
// and ordered properties named as in the specification. Element.IsA asks
// whether its metaclass is a kind of another, and the methods Children, Child,
// Text, IsTrue and Refs read properties. The metamodel's classes and
// properties are those of the OMG SysML v2 release recorded in the UPSTREAM
// file of the repository.
//
// The relationships the specification implies but the source does not write are
// computed on request: Workspace.ImpliedGeneralizations,
// Workspace.ImpliedFeaturingTypes and Workspace.ImpliedBindings, after Resolve.
//
// What it does not do yet: put those implied relationships into the tree
// itself, or check the specification's validation constraints.
package sysml
