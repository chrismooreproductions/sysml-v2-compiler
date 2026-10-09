# Agent notes for sysml-v2-compiler

Repeatable rules for working in this repo. `README.md` describes *what*
is implemented; this file is about *how* to work on it.

## Module

`github.com/chrismooreproductions/sysml-v2-compiler`, Go 1.27. Two packages:
`sysml` (lexer/parser, produces an AST) and `metamodel` (translates that
AST into a flat, KerML-shaped Element/Relationship graph). `main.go` is a
placeholder — this is a library, not a CLI, today.

## Before every commit

Run all four, in this order, and don't commit until all are clean:

```
go build ./...
go vet ./...
gofmt -l .      # must print nothing
go test ./...   # add -count=1 if you suspect stale cache
```

One commit per increment (one coherent construct/fix), not one per file
edit and not a giant batch at the end. Never commit anything the user
didn't ask for — e.g. a scratch example file sitting in the working tree
is the user's to commit, not yours to sweep in.

## Verify, never guess

This project's single most important habit: when you add parsing
support, run the parser against the actual input (a real fixture or a
hand-written snippet) and read the actual output before writing down
what it does. Do not predict error text, predict which fixture advances
how far, or write a test assertion from memory of what "should" happen.
This applies doubly to updating a pinned `wantErr` in
`spec_examples_test.go` — copy the real error string from a real run.

After any change that *widens* what the parser accepts (a new fallback,
a new dispatch branch checked broadly), re-run `TestModelParseErrors` in
full and read every case — confirm nothing that used to correctly fail
now silently parses.

## Fixtures

`sysml/testdata/spec_examples/` vendors real example files from the OMG
SysML-v2-Release repo, unmodified (see that directory's own README for
the license/attribution). Don't edit their contents. `TestModelParseSpec
Examples` pins each one's current parse blocker verbatim; move a fixture
to `TestModelParseSpecExamplesSuccess` once it fully parses. When adding
a new fixture, fetch it from the real repo (`raw.githubusercontent.com/
Systems-Modeling/SysML-v2-Release/master/sysml/src/examples/...`) and
diff/compare rather than retype it from memory or an earlier paraphrase.

## The DefKind mirroring trap

`sysml.DefKind` and `metamodel.DefKind` are two separate Go types kept in
sync *purely by shared iota ordering* — `translate.go` does a direct
`DefKind(m.Kind)` numeric cast, not a lookup table. **A new `DefKind`
constant must always be appended at the end of both packages' const
blocks, in the same relative order, never inserted in the middle of
either one alone.** Inserting in the middle silently shifts every later
constant's value with no compiler error — this has actually happened and
broke several existing tests before being caught by the full suite. The
same shared-ordering trap applies to `sysml.Visibility`/
`metamodel.Visibility`.

## Design pattern: look for structural reuse first

Before writing new AST types or parsing functions for a new grammar
construct, check whether it's structurally identical to something
already implemented, just with different keywords. This has been true
far more often than not in this project:

- `AssertConstraintUsage` and `verify` both turned out to be a plain
  `Usage` reached a different way (the bare-reference-vs-inline-
  declaration duality), not a new Member type.
- `SuccessionAsUsage` (`first a then b;`) is structurally identical to
  `BindingConnectorAsUsage` (`bind a = b;`), just different keywords.
- `use case` is just another `CalculationBody`-shaped `DefKind`, like
  `case`/`analysis`/`verification`.

Re-derive the real grammar (the OMG `.kebnf` files, not memory or a
summary) before assuming a construct needs new machinery.

## Versioning

Semantic versioning, delivered the Go-module way: the version *is* the git
tag (`vMAJOR.MINOR.PATCH`); there is no version constant in the code to
keep in sync.

- While the version is `v0.x.y` the API is unstable: a breaking change to
  `sysml` or `metamodel` exported identifiers bumps MINOR, a new construct
  or backward-compatible addition bumps MINOR, a fix bumps PATCH. From
  `v1.0.0` on, breaking changes bump MAJOR (and need a `/v2` module path).
- Every user-visible change adds a line under `## [Unreleased]` in
  `CHANGELOG.md` in the same commit.
- Releasing: move the Unreleased entries under a new `## [x.y.z] - date`
  heading, commit, then `git tag -a vX.Y.Z -m "vX.Y.Z"`. Tagging and
  pushing a tag are the user's call, never done unprompted.
- Every pushed tag also gets a GitHub Release, so releases are visible
  beyond the Tags list: `git push origin vX.Y.Z`, then
  `gh release create vX.Y.Z --verify-tag --title vX.Y.Z --notes "<that
  version's CHANGELOG section>"`. Use the changelog text for the notes
  (not `--generate-notes`), and add `--prerelease` for any `-rc`/`-beta`
  tag.

## Plan file

Multi-phase work is tracked via Claude Code's plan-mode plan file
(`~/.claude/plans/...`), not in this repo. When starting a genuinely new
chunk of work (not a continuation of the current phase), prefer a fresh
plan file over letting one file accumulate unbounded history — but keep
enough of a "what's already done" summary that the user can tell old
context from new additions at a glance.

## README

`README.md`'s body is a capability description for consumers, grown
incrementally — one paragraph (or edit to an existing one) per phase,
describing the construct and what it deliberately leaves out. Keep it
honest: a "not yet supported" claim must be checked against the current
code, not left stale from an earlier phase (this has gone stale at least
once already).

## Tests

Table-driven (`map[string]struct{...}`) for a family of similar cases,
plain `func Test...(t *testing.T)` for a single scenario — follow
whichever style the surrounding tests in the same file already use.
`parseTopLevelPackage(t, source)` (`sysml/parser_test.go`) is the shared
helper for parser tests. `relationshipTarget`/`relationshipTargets`/
`typeOf` (`metamodel/metamodel_test.go`) are the shared helpers for
walking a translated `*metamodel.Model` in tests.

## Scratch files

Use the session's scratchpad directory for throwaway verification
scripts (e.g. a one-off `go run` to check a fixture), not `/tmp`
directly and not a file inside this repo.
