// SPDX-License-Identifier: EPL-2.0

package sysml_test

import (
	"fmt"

	"github.com/chrismooreproductions/sysml-v2-compiler/sysml"
)

// Parse reads one file and builds its model. Names are not resolved, so a
// reference holds the name as written.
func ExampleParse() {
	doc, err := sysml.Parse("vehicle.sysml", []byte(`
package Vehicles {
	part def Car;
	part car : Car;
}`))
	if err != nil {
		fmt.Println(err)
		return
	}
	doc.Root.Walk(func(e *sysml.Element) {
		if e.IsA("Definition") || e.IsA("Usage") {
			fmt.Printf("%s %s at line %d\n", e.Class, e.Text("declaredName"), e.Span.Start.Line)
		}
	})
	// Output:
	// PartDefinition Car at line 3
	// PartUsage car at line 4
}

// A syntax error says where the parser stopped and what could have continued.
func ExampleParse_syntaxError() {
	_, err := sysml.Parse("bad.sysml", []byte("package P {\n  part x : ;\n}"))
	var se *sysml.SyntaxError
	se, _ = err.(*sysml.SyntaxError)
	fmt.Println(se.Pos, se.Got, se.Expected[:2])
	// Output:
	// 2:12 ; [a name a quoted name]
}

// A workspace resolves names against the added files and the standard library.
func ExampleWorkspace_Resolve() {
	w, err := sysml.NewWorkspace()
	if err != nil {
		fmt.Println(err)
		return
	}
	if _, err := w.Add("model.sysml", []byte(`
package Fleet {
	part def Truck {
		attribute weight : ScalarValues::Real;
		attribute count : Integr;
	}
}`)); err != nil {
		fmt.Println(err)
		return
	}
	for _, d := range w.Resolve() {
		fmt.Println(d)
	}
	// Output:
	// model.sysml:5:21: cannot resolve "Integr" (searched from Fleet::Truck)
}

// After Resolve, each reference's Target is the element it names.
func ExampleRef() {
	w, _ := sysml.NewWorkspace()
	doc, _ := w.Add("model.sysml", []byte(`package P { part def Engine; part e : Engine; }`))
	w.Resolve()
	doc.Root.Walk(func(e *sysml.Element) {
		for _, r := range e.Refs("type") {
			fmt.Printf("%s -> %s (%s)\n", r.Name, sysml.QualifiedName(r.Target), r.Target.Class)
		}
	})
	// Output:
	// Engine -> P::Engine (PartDefinition)
}
