# Roadmap: from the spikes to the module

`plan/README.md` records what the spikes found. This file is the plan for
turning them into the real library, written so each item can become a
GitHub issue that a person or a coding agent can finish and a test can
verify.

## Goal

A Go library and CLI that reads SysML v2 and KerML textual notation and
produces the specification's own model: elements and relationships typed by
the real metaclasses, with names resolved and implied relationships made
explicit, matching what the OMG pilot implementation produces.

"Matching" is measured, not asserted. The pilot's serialization of the
standard library (`sysml.library.xmi`, and `.implied` for implied
relationships) is the oracle, and a conformance test reports how close we are.

Not goals for the first release: an editor/LSP server, incremental parsing,
model transformation, graphical notation, the evaluator for model-level
expressions (`org.omg.sysml.logic/.../expressions`).

## Where the spikes leave us

Against the `2026-08` library (94 files, both languages):

| Layer | Measure | Today |
|---|---|---|
| Parse | files accepted | 94/94 library, and every release example (95 SysML, 58 KerML) |
| Structure | files whose tree equals the oracle's | 93/94 |
| Name resolution | references resolved to the oracle's target | 99.7% of 19,118 |
| Implied generalizations | reproduced | 96.4% of 49,323 |
| Implied `TypeFeaturing`, `BindingConnector` | compared | not yet (72k, 12k) |
| Validation constraints | implemented | not started, size unknown |

The spikes are 4,100 lines in `plan/spike3`, built as an *interpreter* of the
grammar. They have no source positions, no useful error messages, no public
API and no tests of their own beyond the oracle comparison.

## Target shape

```
sysml/                  the public API: Parse, Workspace, Model, and the
                        generated metaclass types with typed accessors
                        (replaces today's package at the cutover, item 7.3)
internal/
  grammar/              the Xtext grammars as data, with UPSTREAM
  schema/               generated from SysML.ecore: metaclasses, features,
                        supertypes, redefinitions, defaults
  syntax/               lexer + grammar interpreter -> node tree (with positions)
  postprocess/          tier 1: parser post-processors
  semantics/            tier 2: adapters, implied relationships
  resolve/              name resolution
  library/              the standard library sources, embedded with go:embed
  conformance/          oracle comparison, run against the pinned release
cmd/sysmlc/             CLI: check, dump, resolve
```

Everything is built under `internal/` first, so the existing `sysml` and
`metamodel` packages keep working untouched until the cutover. Only `sysml`
is public, which keeps the surface that has to stay stable small.

Decisions behind it:

- **Model.** A generic node tree (metaclass name plus ordered properties)
  with typed accessors generated from the ecore, as chosen earlier. The
  existing `metamodel.Element`/`Kind`/`DefKind` are hand-simplified and
  cannot represent the real metamodel, so they do not survive.
- **Grammar source.** The pilot's Xtext grammars, not the `.kebnf` (which
  has transcription errors). Checked in as data with an `UPSTREAM` file naming
  the tag, so updating to a new release is a re-vendor plus a conformance run.
- **Engine.** Ship the interpreter first. Emit Go from the grammar IR only if
  profiling says it matters (it is about 5 s for 153 files including
  resolution, so probably later).
- **Versioning.** Decided: the new API *replaces* `sysml` and `metamodel`
  (no compatibility layer, no two parsers kept in step). That is a breaking
  change, so it releases as `v0.2.0` per `AGENTS.md`.

## Phases

Each phase ends with a test that can fail.

### P0. Housekeeping (blocks everything)

| # | Item | Done when |
|---|---|---|
| 0.1 | Add CI: build, vet, gofmt, test | **Done:** `.github/workflows/ci.yml` runs on every push to `main` and every PR, and also vets the `plan/` spikes |
| 0.2 | Choose the repository license | **Done:** EPL-2.0 (`LICENSE`, `NOTICE`, SPDX headers on every Go file; rule recorded in `AGENTS.md`) |
| 0.3 | Pin the upstream release | **Done:** `UPSTREAM` records tag `2026-08` and the commit of both the release and pilot repos |
| 0.4 | Conformance fixtures policy | **Decided.** Vendored, pinned to the release tag in `UPSTREAM`: the library sources (1.6 MB, under `internal/library`) and the release examples, training and validation models plus the KerML examples (about 1.4 MB, as test data). Fetched by script in CI, never committed: the XMI oracles (25 MB, and 120 MB for `.implied`). Unit tests run offline; only the conformance run needs the download. **Done:** `tools/vendor-upstream.sh` and `tools/fetch-oracle.sh`; 94 library files and 310 example models vendored and byte-identical to the tag; all 310 parse with the spike parser, including the 156 training and validation models |

### P1. Syntax: parse into a node tree

Port `spike1`/`spike3` into `internal/syntax` as a clean package.

| # | Item | Done when |
|---|---|---|
| 1.1 | Xtext grammar loader (spike2) as a tested package | **Done:** `internal/grammar`; the merged SysML grammar (549 rules) and the KerML grammar (282) load with no undefined references, and the counts are pinned |
| 1.2 | Lexer with source positions | **Done:** every token has an offset, line and column, and a span |
| 1.3 | Grammar interpreter producing nodes **with positions** | **Done:** every node carries a span, including the nodes built by actions (an operator expression covers its operands) |
| 1.4 | Error reporting | **Done:** errors carry the file, position, the unexpected token and the sorted expected tokens; 37 malformed inputs are pinned (35 SysML, 2 KerML). A keyword the grammar only uses after a required element gets a hint computed from the grammar (`import` reports `"import" must follow 'private', 'protected', 'public'` and lists those first). **Known limit:** at a position where a whole construct can start, the list is long ("or 76 more"); it is sorted and truncated, not ranked by context |
| 1.5 | Ambiguity policy | **Done:** documented on `syntax.Parse`: one match per rule, start and end position, the first in grammar order. Instrumenting the discards found 137,000 differing ones across 95 of the 404 models, but that counts abandoned speculative branches, not ambiguity in the parse returned, so it is not a measure. The check that matters is that the chosen parse equals the pilot's, which the conformance suite (P3.3) verifies against the oracle |
| 1.6 | Corpus test | **Done:** all 404 models in the library and the vendored example sets, and the 16 older fixtures, parse (`TestVendoredCorpusParses`) |
| 1.7 | Performance budget | **Done:** the 94-file library (1.35 MB) parses in about 0.77 s (`BenchmarkParseLibrary`), down from 3.6 s. The engine compiles the grammar once (references resolved, no lookups by name), joins effects as a rope, and records failed attempts and calls in progress as bitsets instead of map entries. Budget: stay under 1 s for the library on a developer machine; check the benchmark when the engine changes |

### P2. Schema from the ecore

| # | Item | Done when |
|---|---|---|
| 2.1 | Generator reads `SysML.ecore` and emits `internal/schema` | **Done:** `internal/schema/gen` and `tools/genschema` (`go generate ./internal/schema`); 175 classes, 7 enums, 415 features with supertypes, types, defaults, derived/transient/volatile flags, `redefines`, `subsets`, opposites; lookups for kind-of, feature-by-name, effective role, source/target ends |
| 2.2 | Typed accessors over nodes | **Moved to P7.1.** Nothing in P3 to P5 needs them (those stages work on metaclass and property names, which the schema now validates), and their shape should follow the public API, so building them here would be building them twice |
| 2.3 | Staleness check | **Done:** `TestGeneratedTablesAreCurrent` fails if `schema_gen.go` differs from what the generator makes from the vendored ecore |
| 2.4 | Parsed trees conform to the schema | **Done:** `TestParsedTreesConformToTheSchema` checks every node of the 404 models (128,039 nodes): the metaclass exists, each property exists on it, node/reference/boolean/text values suit the property, enum values are real literals. It found a real parser bug (rules returning an enumeration built nodes), now fixed by consulting the schema |

### P3. Tier 1: post-processing and structure

Ported from `plan/spike3` onto the new `internal/model` (elements with owners,
resolved references and a generated mark).

| # | Item | Done when |
|---|---|---|
| 3.1 | Parser post-processors | **Done:** `internal/postprocess`: relationship ends (including the owner back-references the spike skipped), `isEnd`, parameter `direction`, `isComposite` defaults, abstract variations and enumerations, filter-package imports, `PortConjugation`; each rule names the Java it follows and has a unit test |
| 3.2 | Expression structure and chain source-target | **Done:** `internal/semantics`: operands as private `in` parameters, result parameters, the chain's anonymous target feature, the default multiplicity (without the clause that needs resolution); generated elements are marked |
| 3.3 | Structural conformance test | **Done, with a ratchet:** `internal/conformance` loads the pilot's XMI and our model into one canonical form and diffs them. 84 of 94 library files match exactly (79 differences, all in files that need P4 or the transition adapter); the test fails if either number gets worse. A CI job downloads and caches the 25 MB oracle. The earlier target of 93/94 is reached by P4: the 74 `isConstant` and 3 multiplicity differences need name resolution |
| 3.4 | Remaining tier-1 rules not needed by the library | **Done:** names and strings are unescaped and comment bodies cleaned exactly as `ElementUtil.unescapeString` and `processCommentBody` do (including their quirks), and `PortConjugation` is ported |

### P4. Workspace and name resolution

Ported from `plan/spike3` into `internal/semantics` (the resolver and the
implicit-supertype engine call each other, so they share a package with the
structural rules) and `internal/library`.

| # | Item | Done when |
|---|---|---|
| 4.1 | `Workspace` type: add files, resolve, query | **Done:** `semantics.Workspace` with `AddSource`, `AddLibrary`, `Resolve`; files resolve against each other and the library. (It is internal until the public API in P7.1.) |
| 4.2 | Embedded standard library, loaded lazily | **Done:** `internal/library` embeds the vendored sources with `go:embed` and parses them once per process, in parallel (94 files in about 0.3 s); each workspace builds its own model from the shared, immutable trees |
| 4.3 | Port the resolution operations | **Done, with a ratchet:** 19,063 of 19,119 references (99.71%) resolve to the oracle's target, by position; 93 of 94 files match structurally after the resolution rules (`isConstant`, the subsetting clause of the default multiplicity) |
| 4.4 | Close the long tail | **In progress:** 19,082 of 19,119 (99.81%) match; 37 left (20 unresolved, 17 wrong). Closed so far: chains after a cast (`(x as T).f`), the `binary` default supertype for two-ended types. Left: chains whose source is an index expression (`edges#(i).vertices`), the pilot's `skip` rule, nested chain features, `$` qualification, distinguishability. Each is a separate pilot quirk; target 99.95% (at most 9 left) continues alongside P5 |
| 4.5 | Diagnostics | **Done** (`sysml.Diagnostic`). An unresolved reference reports its name, position and the scope searched |

### P5. Tier 2: implied relationships

Port `implicit.go`, then the rest of the adapters.

| # | Item | Done when |
|---|---|---|
| 5.1 | Implicit supertype engine | ≥ 96% of the implied-generalizations oracle (today's number) — **Done:** 99.59% of 49,334 implied general types match (`TestImpliedGeneralizationsMatchTheOracle`, compared by owner position and target).
| 5.2 | Remaining generalization families | feature result typing, cross-subsetting, per-usage-kind defaults: ≥ 99.5% — **Done** for the generalization families: usage-kind defaults (`subitem`, `subaction`, binary connections, ports, requirements), cross features (typing, subsetting of redefined ends' cross features, cross subsetting) and the generated chains, compared by kind. 203 missing and 60 extra remain: enumeration literal typing, `ForLoopAction` reference subsettings, a few measurement and geometry features.
| 5.3 | `TypeFeaturing` | 72k implied relationships compared and ≥ 99% reproduced |
| 5.4 | `BindingConnector`s and the generated features they need | 12k compared and ≥ 99% reproduced |
| 5.5 | Transition usage adapter | the last structural difference closes |

### P6. Validation (scope unknown)

The ecore carries the specification's constraints (`check<X>Specialization`
and others) as OCL. Nobody has measured how many there are or how many a
parser-level tool needs. Item 6.1 is a spike to size it before committing to
the rest.

| # | Item | Done when |
|---|---|---|
| 6.1 | Inventory the constraints | a table of every constraint, its metaclass, whether it needs resolution, whether it needs the evaluator |
| 6.2 | Implement the structural ones | each constraint has a passing and a failing test |

### P7. API, CLI, release

| # | Item | Done when |
|---|---|---|
| 7.1 | Public API: the `sysml` package | `Parse`, `Workspace`, the model and the generated metaclass types with typed accessors (moved here from 2.2), built over `internal/`; a doc comment on every exported identifier; one example per entry point — **Done** (generic model; typed accessors deferred past v0.2.0).
| 7.2 | `sysmlc` | `check`, `dump`, `resolve` over files and directories, exit codes for CI use — **Done.**
| 7.3 | Cutover: delete `metamodel`, replace the old `sysml` package with the new one | the 16 vendored spec fixtures and any old test still worth keeping pass on the new API, the rest are removed on purpose; `AGENTS.md` and the README rewritten — **Done** (old packages removed in a3cc65e; docs rewritten).
| 7.4 | Release `v0.2.0` | CHANGELOG entry, tag, GitHub Release, as `AGENTS.md` requires |

### P8. Later

Emit Go from the grammar IR; error recovery so one syntax error does not hide
the rest; incremental parsing; the expression evaluator.

## Dependencies

```
P0 ─► P1 ─► P3 ─► P4 ─► P7 (v0.2.0)
       └──► P2 ──┘   └──► P5 (v0.3.0) ─► P6 (v0.4.0)
```

P2 and P1 ran in parallel. **Scope call, made while executing:** the original
order put P5 (materializing implied relationships) and P6 (validation) before the
public API. A first release needs only what P1 to P4 provide: parse, add the
structure the specification implies without resolution, resolve names against
the standard library. So `v0.2.0` ships after P7, and the implied relationships
(P5) and validation (P6) follow as `v0.3.0` and `v0.4.0`. The model already
computes implicit general types internally (they drive resolution and are checked
to 96.4% against the `.implied` oracle in the spike); what `v0.2.0` does not do
is add them to the tree as relationships.

## Running it with agents

What makes an issue safe to give to an agent here:

- **A mechanical acceptance test.** Every item above names one: a count from
  the conformance run, or a golden test. An item without one is not ready.
- **One rule or one family per issue.** The spikes show this granularity
  works: a rule, the Java it mirrors, and the oracle category it should move.
- **A pointer to the Java.** Issues cite the pilot class and method to port
  (`org.omg.sysml.logic/.../UsageAdapter.addAdditionalMembers`), not a prose
  description.
- **Serialise changes to shared structure.** `AGENTS.md` already warns that
  the `DefKind` mirroring trap bites when two edits race; the equivalents
  here are the grammar interpreter and the generated schema. Parser-core and
  schema issues go one at a time; rule ports (P3, P5) are independent.
- **CI is the gate.** Build, vet, gofmt, the unit tests and the conformance
  numbers, with a ratchet: a PR may not lower a conformance number.
- **A human reads every PR**, against the oracle category it claims to move.

## Risks and open questions

- **Licensing.** No license file exists; the vendored material is EPL-2.0.
  This needs a decision before any vendoring (item 0.2).
- **The oracle only covers the library.** It is the best evidence available
  but user models exercise paths the library does not. Some rules are only
  tested by reading the Java. Mitigation: port from the Java, keep a note of
  which rules have oracle coverage, and add hand-written golden models for
  the rest.
- **The pilot's behavior moves.** The `AttributeDefinition` rule changed
  between releases `2026-07` and `2026-08`. Pinning (0.3) and a documented
  re-vendor procedure are the answer; expect work at each release.
- **Source positions.** The spikes carry none, and retrofitting them is the
  largest unknown in P1.
- **Validation could be large.** Item 6.1 exists to find out before it is
  scheduled.
- **Generic tree ergonomics.** Typed accessors (2.2) are what make the tree
  pleasant to use; if they prove awkward, generating typed structs is the
  fallback and the schema package is the seam.
- **Performance of resolution.** About 9 s today for 94 files because
  nothing is indexed; fine for a library, not for an editor. Caching name
  tables per namespace is the first fix if it matters.

## Decisions

All settled.

1. **License:** EPL-2.0 (`LICENSE`, `NOTICE`, SPDX headers).
2. **API break:** replace `sysml` and `metamodel` and release `v0.2.0`.
3. **Layout:** one public `sysml` package over `internal/` subpackages.
4. **Vendoring:** the library source and the release examples are vendored at
   the pinned tag; only the XMI oracles are fetched in CI.

What is left is to start: P0.1 (CI), 0.3 (`UPSTREAM`, the pin) and 0.4 (the
vendoring), then P1.
