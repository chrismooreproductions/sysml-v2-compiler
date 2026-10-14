# sysml-v2-compiler

A SysML v2 and KerML front end in Go: it reads the textual notation, builds the
specification's own model of it, and resolves names across files and against the
standard library.

The grammar is not hand-written. The parser interprets the OMG pilot
implementation's Xtext grammars (`KerML.xtext`, `KerMLExpressions.xtext`,
`SysML.xtext`), and the semantic layer is ported from the pilot's Java, checked
against the pilot's own output (see [Conformance](#conformance)).

## Use

```go
w, err := sysml.NewWorkspace()
if err != nil { /* ... */ }
if _, err := w.Add("vehicle.sysml", src); err != nil {
	// a *sysml.SyntaxError with file, line and column
}
for _, d := range w.Resolve() {
	fmt.Println(d) // vehicle.sysml:12:9: cannot resolve "Engin" (searched from Vehicle)
}
```

`sysml.Parse` reads one file when names do not matter. A `Workspace` resolves
files against each other and the bundled standard library (`WithoutStandardLibrary`
turns that off). The model is generic: an `Element` has a metaclass name, a
source span and ordered properties named as in the specification; `IsA`,
`Children`, `Child`, `Text`, `IsTrue` and `Refs` read it. Every exported
identifier is documented; run `go doc github.com/chrismooreproductions/sysml-v2-compiler/sysml`.

`Workspace.Facts(e)` returns the resolved properties of one element so that a
consumer that draws or tabulates models need not know how the metamodel
encodes them: identity, owner and members (with how each is held: owning,
feature, parameter, import, alias), direction, composite/reference and the
other feature flags, multiplicity as numbers (`Upper == -1` for `*`), and the
typing, subsetting, reference subsetting, redefinition, specialization and
conjugation relationships, written or implied, and the ends of a connector
with the feature each points to (and the source and target of a flow or
succession), and the value expression (computed when possible, with its
source text), the control-node kind, and the behaviour an action or state
writes (entry, do and exit actions, successions and transitions with their
trigger, guard and effect; an end the source does not write is left nil, not
inferred). Requirement roles and annotations are not there yet. `sysmlc facts` prints it.

## Command line

```
go install github.com/chrismooreproductions/sysml-v2-compiler/cmd/sysmlc@latest

sysmlc check   [-nolib] <file or directory>...   syntax errors and unresolved names
sysmlc resolve [-nolib] <file or directory>...   each reference and its target
sysmlc implied [-nolib] <file or directory>...   the relationships the specification implies
sysmlc facts   [-nolib] <file or directory>...   the resolved facts about each element
sysmlc eval    [-nolib] <file or directory>...   the value of each feature that has one
sysmlc lsp                                       a language server on stdin/stdout
sysmlc dump    [-spans] <file>...                the model as a tree
```

Exit status: 0 nothing to report, 1 problems in the models, 2 usage or I/O error.

## Language server

`sysmlc lsp` runs a Language Server Protocol server on standard input and
output, so an editor or a platform gets the compiler's knowledge as you type:

- diagnostics: syntax errors (all of them), unresolved names and constraint
  violations, as the file changes;
- hover (kind, qualified name, types, supertypes, computed value,
  documentation), go to definition (including into the standard library, which
  opens from a cache directory), find references and rename;
- outline (document symbols), workspace symbol search, and completion of names
  in scope, qualified names after `::`, and keywords.

The server loads every `.sysml` and `.kerml` file under the workspace folders,
so names resolve across files, and the editor's unsaved buffers take precedence.
Point any LSP client at it with the command `sysmlc lsp` for the file types
`sysml` (`.sysml`) and `kerml` (`.kerml`). For a client that connects over a
socket, such as a web editor through a proxy, run `sysmlc lsp -listen 127.0.0.1:2087`:
each connection gets a server of its own.

## What is and is not covered

- **Syntax:** all of SysML v2 and KerML, including expressions. All 404 models
  in the release (library and examples) parse, and every node the parser makes
  is a metaclass and property the metamodel defines.
- **Structure:** the parser's post-processing and the pilot's adapters that give
  the tree its shape (relationship ends, parameter directions, result
  parameters, default multiplicities).
- **Names:** the KerML/SysML scoping rules, including imports, redefinition,
  feature chains and the standard library.
- **Implied relationships:** the default supertypes, cross features, featuring
  types and binding connectors the specification adds are computed on request
  (`ImpliedGeneralizations`, `ImpliedFeaturingTypes`, `ImpliedBindings`,
  `sysmlc implied`) and match the pilot for 99.79% of the 99,685 in the standard
  library. `Materialize` writes them into the tree as elements marked
  `isImplied`; `sysmlc dump -implied` shows them.
- **Not yet:** the specification's validation constraints are not checked; there
  are no typed accessors per metaclass.

## Conformance

The tests compare against the pilot implementation's XMI for the standard
library (pinned in `UPSTREAM`): all 94 library files match structurally
exactly, and 99.95% of the 19,119 name references resolve to the same element
the pilot chooses. The remaining differences are listed and ratcheted in
`internal/conformance`. The XMI files (25 MB and 120 MB) are fetched by
`tools/fetch-oracle.sh` into `.oracle/`; tests that need them skip when absent
and run in CI.

## Layout

| Path | |
|---|---|
| `sysml/` | the public API |
| `cmd/sysmlc/` | the command |
| `internal/grammar`, `internal/syntax` | Xtext grammar loader and the parser |
| `internal/schema` | the metamodel, generated from `SysML.ecore` |
| `internal/model`, `internal/postprocess` | the element tree and its post-processing |
| `internal/semantics` | structure, implicit types, scoping and resolution |
| `internal/library` | the embedded standard library |
| `internal/conformance` | comparison with the pilot's output |

## Licence

Eclipse Public License 2.0. The grammars, metamodel and library are from the
OMG SysML v2 release and pilot implementation; see `NOTICE`.
