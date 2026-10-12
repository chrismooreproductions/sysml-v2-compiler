# Agent notes for sysml-v2-compiler

Repeatable rules for working in this repo. `README.md` describes *what*
is implemented; this file is about *how* to work on it.

## Module

`github.com/chrismooreproductions/sysml-v2-compiler`, Go 1.27. The public API
is the single package `sysml`; everything else lives under `internal/`
(grammar, syntax, schema, model, postprocess, semantics, library,
conformance). `cmd/sysmlc` is the command. The roadmap is kept locally in
`plan/` (git-ignored, not published).

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

Run the code on real input and read the real output before writing down what
it does. Do not predict error text or a count and pin it from memory; copy it
from a run. Conformance numbers are ratchets in `internal/conformance`: run
with the oracle (`tools/fetch-oracle.sh`, or `ORACLE_DIR`), and only raise a
ratchet to the measured value.

## Fixtures

`testdata/upstream`, `internal/library` and the xtext/ecore files are copied
unmodified from the pinned release; never edit them.

### Upstream sources

`UPSTREAM` pins the OMG release (tag and commit) that the standard library,
the example models and the XMI oracles come from. `internal/library` and
`testdata/upstream` are copied from it unmodified by
`tools/vendor-upstream.sh <SysML-v2-Release checkout>`; never edit them by
hand, and never add a file there that is not in the pinned release. The XMI
oracles (25 MB and 120 MB) are not committed: `tools/fetch-oracle.sh`
downloads them into `.oracle/` (ignored by git). To move to a new release,
change `UPSTREAM`, re-run the vendoring script and the conformance run, and
record behaviour changes in `CHANGELOG.md`.

## Porting from the pilot

Semantic behaviour is ported from the pilot's Java (adapters, scope
providers), guided by the oracle diff rather than memory. The generic tree
means a fix is usually a rule about metaclass and property names; check them
against `internal/schema`. Prefer the smallest rule that closes a diff, and
record what remains in the roadmap.

## Versioning

Semantic versioning, delivered the Go-module way: the version *is* the git
tag (`vMAJOR.MINOR.PATCH`); there is no version constant in the code to
keep in sync.

- While the version is `v0.x.y` the API is unstable: a breaking change to
  `sysml` exported identifiers bumps MINOR, a new construct
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

## Licensing

The project is EPL-2.0 (`LICENSE`, `NOTICE`). Every Go file starts with
`// SPDX-License-Identifier: EPL-2.0`. A file that ports or derives from the
pilot implementation's Java or Xtext also carries the upstream copyright
notice (see the existing ports under `internal/semantics`) and names the Java it
follows. Do not vendor anything whose license is not EPL-2.0 or compatible
without asking, and add vendored sources to `NOTICE`.

## Plan and README

`plan/ROADMAP.md` (local only, git-ignored) tracks phases and is updated with the work. The README's
"covered / not covered" claims must be checked against the current code, not
left stale.

## Tests

Plain `func Test...` for a scenario, table-driven for a family. Tests that
need the oracle skip when it is absent; CI runs them. The CLI is tested
in-process through `run(args, stdout, stderr)`.

## Scratch files

Use the session's scratchpad directory for throwaway verification
scripts (e.g. a one-off `go run` to check a fixture), not `/tmp`
directly and not a file inside this repo.
