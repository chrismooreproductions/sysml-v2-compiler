// SPDX-License-Identifier: EPL-2.0

// Command sysmlc checks and inspects SysML v2 and KerML textual models.
//
//	sysmlc check   [-nolib] <file or directory>...   report syntax errors and names that do not resolve
//	sysmlc lsp                                        run a language server (LSP) on standard input and output
//	sysmlc eval    [-nolib] <file or directory>...   print the value of each feature whose value can be computed
//	sysmlc implied [-nolib] <file or directory>...   print the relationships the specification implies for each type
//	sysmlc dump    [-spans] <file>...                 print the model of each file as a tree
//	sysmlc resolve [-nolib] <file or directory>...   print each reference and the element it resolves to
//	sysmlc version                                    print the version
//
// Files with the extension .sysml are read as SysML v2 and .kerml as KerML;
// directories are searched for both. The exit status is 0 if there is nothing to
// report, 1 if there are problems in the models, and 2 for a usage or I/O error.
package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/lsp"
	"github.com/chrismooreproductions/sysml-v2-compiler/sysml"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// stdin is where the language server reads requests; tests replace it.
var stdin = func() io.Reader { return os.Stdin }

// runLSP serves the Language Server Protocol until the editor exits.
func runLSP(in io.Reader, stdout, stderr io.Writer) int {
	srv := lsp.NewServer(in, stdout)
	srv.Logf = func(format string, args ...any) { fmt.Fprintf(stderr, "sysmlc lsp: "+format+"\n", args...) }
	if err := srv.Run(); err != nil {
		fmt.Fprintln(stderr, "sysmlc lsp:", err)
		return 1
	}
	return 0
}

const usage = `usage: sysmlc <command> [flags] <file or directory>...

commands:
  check     report syntax errors and names that do not resolve
  lsp       run a language server on standard input and output
  eval      print the value of each feature that has a value expression
  implied   print the relationships the specification implies for each type
  dump      print the model of each file as a tree
  resolve   print each reference and the element it resolves to
  version   print the version
`

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "check":
		return withWorkspace(args[1:], stdout, stderr, "check", func(w *sysml.Workspace, docs []*sysml.Document, diags []sysml.Diagnostic, stdout io.Writer) {})
	case "resolve":
		return withWorkspace(args[1:], stdout, stderr, "resolve", printReferences)
	case "lsp":
		return runLSP(stdin(), stdout, stderr)
	case "eval":
		return withWorkspace(args[1:], stdout, stderr, "eval", printValues)
	case "implied":
		return withWorkspace(args[1:], stdout, stderr, "implied", printImplied)
	case "dump":
		return dump(args[1:], stdout, stderr)
	case "version":
		fmt.Fprintln(stdout, version())
		return 0
	case "-h", "-help", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "sysmlc: unknown command %q\n\n%s", args[0], usage)
	return 2
}

func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return "sysmlc " + info.Main.Version
	}
	return "sysmlc (development build)"
}

// collect expands the arguments into model files: files as given, directories
// searched for .sysml and .kerml files, in a stable order.
func collect(args []string) ([]string, error) {
	var files []string
	for _, a := range args {
		info, err := os.Stat(a)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			files = append(files, a)
			continue
		}
		var found []string
		err = filepath.WalkDir(a, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				if _, err := sysml.LanguageOf(p); err == nil {
					found = append(found, p)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		sort.Strings(found)
		files = append(files, found...)
	}
	return files, nil
}

// withWorkspace loads the files into a workspace, resolves it, prints the
// problems, and calls report for the commands that print more.
func withWorkspace(args []string, stdout, stderr io.Writer, name string,
	report func(*sysml.Workspace, []*sysml.Document, []sysml.Diagnostic, io.Writer)) int {
	fl := flag.NewFlagSet("sysmlc "+name, flag.ContinueOnError)
	fl.SetOutput(stderr)
	nolib := fl.Bool("nolib", false, "do not include the standard library")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() == 0 {
		fmt.Fprintf(stderr, "sysmlc %s: no files\n", name)
		return 2
	}
	files, err := collect(fl.Args())
	if err != nil {
		fmt.Fprintln(stderr, "sysmlc:", err)
		return 2
	}
	var opts []sysml.Option
	if *nolib {
		opts = append(opts, sysml.WithoutStandardLibrary())
	}
	w, err := sysml.NewWorkspace(opts...)
	if err != nil {
		fmt.Fprintln(stderr, "sysmlc:", err)
		return 2
	}
	problems := 0
	var docs []*sysml.Document
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintln(stderr, "sysmlc:", err)
			return 2
		}
		doc, errs := w.AddRecovering(f, src)
		for _, e := range errs {
			fmt.Fprintln(stderr, e)
			problems++
		}
		if doc != nil {
			docs = append(docs, doc)
		}
	}
	if problems > 0 {
		// names that the unparsed statements would define are not worth reporting
		return 1
	}
	diags := w.Resolve()
	if name == "check" {
		diags = append(diags, w.Validate()...)
	}
	for _, d := range diags {
		fmt.Fprintln(stderr, d)
		problems++
	}
	report(w, docs, diags, stdout)
	if problems > 0 {
		return 1
	}
	return 0
}

// printReferences prints each cross-reference in the documents and its target.
func printReferences(w *sysml.Workspace, docs []*sysml.Document, _ []sysml.Diagnostic, stdout io.Writer) {
	for _, doc := range docs {
		doc.Root.Walk(func(e *sysml.Element) {
			for _, p := range e.Props {
				r, ok := p.Value.(*sysml.Ref)
				if !ok || r.Name == "" {
					continue
				}
				target := "?"
				if r.Target != nil {
					target = fmt.Sprintf("%s (%s, %s)", describe(r.Target), r.Target.Class, w.FileOf(r.Target))
				}
				fmt.Fprintf(stdout, "%s:%s: %s -> %s\n", doc.Name, r.Span.Start, r.Name, target)
			}
		})
	}
}

// printValues prints each feature with a value expression and the value, for
// those the evaluator can compute.
func printValues(w *sysml.Workspace, docs []*sysml.Document, _ []sysml.Diagnostic, stdout io.Writer) {
	for _, doc := range docs {
		doc.Root.Walk(func(e *sysml.Element) {
			if e.Generated || !e.IsA("Feature") || e.Text("declaredName") == "" {
				return
			}
			for _, r := range e.Children("ownedRelationship") {
				if !r.IsA("FeatureValue") {
					continue
				}
				for _, x := range r.Children("ownedRelatedElement") {
					if v, err := w.Evaluate(x); err == nil {
						fmt.Fprintf(stdout, "%s:%s: %s = %s\n", doc.Name, e.Span.Start, describe(e), sysml.FormatValue(v))
					}
				}
			}
		})
	}
}

// printImplied prints, for each type, the general types, featuring types and
// binding connectors the specification implies for it.
func printImplied(w *sysml.Workspace, docs []*sysml.Document, _ []sysml.Diagnostic, stdout io.Writer) {
	for _, doc := range docs {
		doc.Root.Walk(func(e *sysml.Element) {
			if e.Generated || !e.IsA("Type") {
				return
			}
			name := describe(e)
			for _, im := range w.ImpliedGeneralizations(e) {
				fmt.Fprintf(stdout, "%s:%s: %s %s -> %s\n", doc.Name, e.Span.Start, name, im.Kind, describe(im.Target))
			}
			for _, t := range w.ImpliedFeaturingTypes(e) {
				fmt.Fprintf(stdout, "%s:%s: %s TypeFeaturing -> %s\n", doc.Name, e.Span.Start, name, describe(t))
			}
			for _, b := range w.ImpliedBindings(e) {
				fmt.Fprintf(stdout, "%s:%s: %s BindingConnector %s = %s\n", doc.Name, e.Span.Start, name, describe(b.Source), describe(b.Target))
			}
		})
	}
}

func describe(e *sysml.Element) string {
	if e.Generated {
		return "(generated " + e.Class + ")"
	}
	if q := sysml.QualifiedName(e); q != "" {
		return q
	}
	return "(" + e.Class + ")"
}

func dump(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("sysmlc dump", flag.ContinueOnError)
	fl.SetOutput(stderr)
	spans := fl.Bool("spans", false, "show the source span of each element")
	withImplied := fl.Bool("implied", false, "include the relationships the specification implies")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() == 0 {
		fmt.Fprintln(stderr, "sysmlc dump: no files")
		return 2
	}
	files, err := collect(fl.Args())
	if err != nil {
		fmt.Fprintln(stderr, "sysmlc:", err)
		return 2
	}
	status := 0
	var w *sysml.Workspace
	if *withImplied {
		if w, err = sysml.NewWorkspace(); err != nil {
			fmt.Fprintln(stderr, "sysmlc:", err)
			return 2
		}
	}
	var docs []*sysml.Document
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintln(stderr, "sysmlc:", err)
			return 2
		}
		var doc *sysml.Document
		if w != nil {
			doc, err = w.Add(f, src)
		} else {
			doc, err = sysml.Parse(f, src)
		}
		if err != nil {
			fmt.Fprintln(stderr, err)
			status = 1
			docs = append(docs, nil)
			continue
		}
		docs = append(docs, doc)
	}
	if w != nil {
		w.Materialize()
	}
	for i, f := range files {
		doc := docs[i]
		if doc == nil {
			continue
		}
		if len(files) > 1 {
			if i > 0 {
				fmt.Fprintln(stdout)
			}
			fmt.Fprintf(stdout, "# %s\n", f)
		}
		dumpElement(stdout, doc.Root, "", *spans)
	}
	return status
}

// dumpElement prints an element: its class, its written properties on the same
// line, then its owned elements and references indented below.
func dumpElement(out io.Writer, e *sysml.Element, indent string, spans bool) {
	var line strings.Builder
	line.WriteString(e.Class)
	if e.Generated {
		line.WriteString("*")
	}
	for _, p := range e.Props {
		switch v := p.Value.(type) {
		case sysml.Text:
			fmt.Fprintf(&line, " %s=%q", p.Name, string(v))
		case sysml.Bool:
			fmt.Fprintf(&line, " %s=%t", p.Name, bool(v))
		}
	}
	if spans && !e.Generated {
		fmt.Fprintf(&line, "  [%s-%s]", e.Span.Start, e.Span.End)
	}
	fmt.Fprintln(out, line.String())
	for _, p := range e.Props {
		switch v := p.Value.(type) {
		case *sysml.Element:
			fmt.Fprintf(out, "%s  %s ", indent, p.Name)
			dumpElement(out, v, indent+"  ", spans)
		case *sysml.Ref:
			switch {
			case v.Name != "":
				fmt.Fprintf(out, "%s  %s -> @%s\n", indent, p.Name, v.Name)
			case v.Target != nil && e.Generated:
				fmt.Fprintf(out, "%s  %s -> @%s\n", indent, p.Name, describe(v.Target))
			}
		}
	}
}
