// SPDX-License-Identifier: EPL-2.0

// Command genimplicit regenerates internal/semantics/implicit_table_gen.go from
// the vendored internal/semantics/upstream/ImplicitGeneralizationMap.java.txt.
// Run it through go generate:
//
//	go generate ./internal/semantics
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/semantics/gen"
)

func main() {
	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	java, err := os.ReadFile(filepath.Join(dir, "upstream", "ImplicitGeneralizationMap.java.txt"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	src, err := gen.Generate(java)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(dir, "implicit_table_gen.go"), src, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
