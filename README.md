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

On top of the generic tree are three views a consumer that draws or tabulates
models can use without knowing how the metamodel encodes things:

- **`Workspace.Facts(e)`** gives the resolved properties of one element:
  - identity, owner, and members with how each is held (owning, feature,
    parameter, import, alias);
  - direction, the composite/reference and other feature flags, the portion
    kind, and multiplicity as numbers (`Upper == -1` for `*`);
  - typing, subsetting, reference subsetting, redefinition, specialization and
    conjugation, written or implied;
  - the ends of a connector or message and the feature each points to, with the
    source and target of a flow, message or succession;
  - the value expression (computed when possible, with its source text);
  - behaviour: entry, do and exit actions, successions and transitions with
    trigger, guard and effect, the condition of an `if` or `while`, and the kind
    of a control node. A succession end the source does not write is the step
    before or the member after it, marked implied;
  - requirement roles (subject, actors, stakeholders, objective, assumed and
    required constraints, and what a `satisfy` or `verify` targets);
  - a view's or package's `filter` conditions, and the documentation, comments
    and textual representations that annotate the element.
- **Evaluation** never guesses. `Workspace.Eval` gives a typed `Result`: a
  value, "not determined" when an input has no value, or "not evaluable" with
  the reason and the element responsible. Built on it:
  - `Matches` evaluates a `filter` condition for an element
    (`@SysML::PartUsage`, `@Safety and Safety::level > 2`);
  - `ViewContent` returns the elements a view's `expose`s bring in that pass its
    filters;
  - `EvalConstraint`, `EvalRequirement` and `EvalCalculation` say whether
    constraints hold and what a calculation gives, including calls of the
    model's own calculations;
  - a number with a unit (`1800 [kg]`) is a `Quantity` that carries its unit.
    Converting between units and derived units are not supported yet.
- **`LibraryFiles`** returns the standard library's files (path, language,
  source) for a consumer that wants to show them; `Workspace.FileOf` names the
  file of a library element `library:` and that path.

## Command line

```
go install github.com/chrismooreproductions/sysml-v2-compiler/cmd/sysmlc@latest

sysmlc check   [-nolib] <file or directory>...   syntax errors and unresolved names
sysmlc resolve [-nolib] <file or directory>...   each reference and its target
sysmlc implied [-nolib] <file or directory>...   the relationships the specification implies
sysmlc facts   [-nolib] <file or directory>...   the resolved facts about each element
sysmlc eval    [-nolib] <file or directory>...   values, constraint results and calculations, or why not
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
  `sysmlc implied`) and match the pilot for 99.80% of the 99,685 in the standard
  library. `Materialize` writes them into the tree as elements marked
  `isImplied`; `sysmlc dump -implied` shows them.
- **Validation:** `Workspace.Validate` and `sysmlc check` apply the structural
  constraints of the specification; the rest are not checked yet.
- **Evaluation:** expressions over literals and values, arithmetic, comparison
  and boolean operators, calls of the model's own calculations, constraints,
  views and filters. Calls of library functions are mostly not evaluated (287
  of the library's 394 functions have no body), and units are not converted.
- **Known gaps:** names in some of the release's example models do not resolve
  (the pilot's own examples report 154 diagnostics for SysML and 48 for KerML
  when checked together; each cause is an open issue), and there are no typed
  accessors per metaclass.

## Status

The work that remains is tracked as [GitHub issues](https://github.com/chrismooreproductions/sysml-v2-compiler/issues):
the open resolver gaps, the validation constraints not yet checked, unit
conversion, and the example models' diagnostics. Releases and what changed in
each are in [CHANGELOG.md](CHANGELOG.md).

## Conformance

The tests compare against the pilot implementation's XMI for the standard
library (pinned in `UPSTREAM`): all 94 library files match structurally
exactly, and 19,114 of the 19,119 name references (99.97%) resolve to the same
element the pilot chooses. The remaining differences are listed and ratcheted in
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
