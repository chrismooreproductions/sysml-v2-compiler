// SPDX-License-Identifier: EPL-2.0

// Command genschema regenerates internal/schema/schema_gen.go from the
// vendored internal/schema/SysML.ecore. Run it through go generate:
//
//	go generate ./internal/schema
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/schema/gen"
)

func main() {
	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	ecore, err := os.ReadFile(filepath.Join(dir, "SysML.ecore"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	src, err := gen.Generate(ecore)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(dir, "schema_gen.go"), src, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
