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
| 0.1 | Add CI: build, vet, gofmt, test | a workflow runs on every PR |
| 0.2 | Choose the repository license | **Done:** EPL-2.0 (`LICENSE`, `NOTICE`, SPDX headers on every Go file; rule recorded in `AGENTS.md`) |
| 0.3 | Pin the upstream release | `UPSTREAM` records the tag (`2026-08`) for grammar, ecore and library |
| 0.4 | Conformance fixtures policy | **Decided.** Vendored, pinned to the release tag in `UPSTREAM`: the library sources (1.6 MB, under `internal/library`) and the release examples, training and validation models plus the KerML examples (about 1.4 MB, as test data). Fetched by script in CI, never committed: the XMI oracles (25 MB, and 120 MB for `.implied`). Unit tests run offline; only the conformance run needs the download |

### P1. Syntax: parse into a node tree

Port `spike1`/`spike3` into `internal/syntax` as a clean package.

| # | Item | Done when |
|---|---|---|
| 1.1 | Xtext grammar loader (spike2) as a tested package | the merged SysML grammar (549 rules) and the KerML grammar load with no undefined references |
| 1.2 | Lexer with source positions | every token has file, offset, line, column |
| 1.3 | Grammar interpreter producing nodes **with positions** | every node carries the span of the text that produced it (the spikes have none) |
| 1.4 | Error reporting | farthest-failure message names the expected tokens and the position; golden tests for 30+ malformed inputs |
| 1.5 | Ambiguity policy | decide and document what happens when two parses reach the same end (spike keeps the first); a test that counts ambiguous files in the library |
| 1.6 | Corpus test | the 94 library files and the release examples parse; the existing 16 fixtures parse |
| 1.7 | Performance budget | parsing the whole library in under a stated time, with a benchmark |

### P2. Schema from the ecore

| # | Item | Done when |
|---|---|---|
| 2.1 | Generator reads `SysML.ecore` and emits `schema` | classes, supertypes, features, defaults, derived/transient, `redefines` |
| 2.2 | Typed accessors over nodes | `Package.OwnedMembership()` style access for the metaclasses; round-trip tests |
| 2.3 | Staleness check | CI fails if the generated schema differs from the pinned ecore |

### P3. Tier 1: post-processing and structure

Port `postprocess.go` and `semantic.go`.

| # | Item | Done when |
|---|---|---|
| 3.1 | Parser post-processors | the rules in `postprocess.go`, each citing the Java it mirrors |
| 3.2 | Expression structure and chain source-target | operands as parameters, result parameters |
| 3.3 | Structural conformance test | against the oracle: ≥ 93/94 files exact (today's number), tracked in CI; the transition-usage difference is an open item |
| 3.4 | Remaining tier-1 rules not needed by the library | string unescaping, comment bodies, `PortConjugation` |

### P4. Workspace and name resolution

Port `resolve.go`, and give it an API: a `Workspace` of files, with the standard
library loaded on demand.

| # | Item | Done when |
|---|---|---|
| 4.1 | `Workspace` type: add files, resolve, query | multi-file models resolve against each other and the library |
| 4.2 | Embedded standard library, loaded lazily | `go:embed` of `internal/library`; importing `ScalarValues` or `Parts` works with no setup |
| 4.3 | Port the resolution operations | ≥ 99.7% of the oracle's reference targets (today's number) |
| 4.4 | Close the long tail | `$` qualification, filter imports, distinguishability, the pilot's `skip` rule, result-type inference for operator and index chain sources: target 99.95% |
| 4.5 | Diagnostics | an unresolved reference reports its name, position and the scope searched |

### P5. Tier 2: implied relationships

Port `implicit.go`, then the rest of the adapters.

| # | Item | Done when |
|---|---|---|
| 5.1 | Implicit supertype engine | ≥ 96% of the implied-generalizations oracle (today's number) |
| 5.2 | Remaining generalization families | feature result typing, cross-subsetting, per-usage-kind defaults: ≥ 99.5% |
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
| 7.1 | Public API: the `sysml` package | `Parse`, `Workspace`, the model and the generated metaclass types, built over `internal/`; a doc comment on every exported identifier; one example per entry point |
| 7.2 | `sysmlc` | `check`, `dump`, `resolve` over files and directories, exit codes for CI use |
| 7.3 | Cutover: delete `metamodel`, replace the old `sysml` package with the new one | the 16 vendored spec fixtures and any old test still worth keeping pass on the new API, the rest are removed on purpose; `AGENTS.md` and the README rewritten |
| 7.4 | Release `v0.2.0` | CHANGELOG entry, tag, GitHub Release, as `AGENTS.md` requires |

### P8. Later

Emit Go from the grammar IR; error recovery so one syntax error does not hide
the rest; incremental parsing; the expression evaluator.

## Dependencies

```
P0 ─► P1 ─► P3 ─► P4 ─► P5 ─► P7
       └──► P2 ──┘     └──► P6 (can start with 6.1 after P4)
```

P2 and P1 run in parallel. Within a phase most items are independent.

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
