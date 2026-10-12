# Grammar-driven parser: feasibility spikes

Throwaway Go programs (each its own module, so they are invisible to
`go build ./...` in the main module) that test whether the SysML v2 / KerML
parser can be driven by the OMG grammar instead of hand-written. They read
two checkouts that are **not** vendored here:

- `~/repos/sysml-release` — https://github.com/Systems-Modeling/SysML-v2-Release
  (the `bnf/*.kebnf` grammars, `sysml.library`, `sysml/src/examples`)
- `~/repos/sysml-pilot-implementation` — https://github.com/Systems-Modeling/SysML-v2-Pilot-Implementation
  (Xtext grammars, `SysML.ecore` metamodel)

Both are EPL-2.0. `spike1/ir.json` is derived from the `.kebnf` files and
`spike2/xtext-ir.json` from the Xtext files; regenerate rather than edit.

## spike0 — parse the `.kebnf` into an IR (`report.txt`)

```
cd plan/spike0 && go run . ~/repos/sysml-release/bnf/KerML-textual-bnf.kebnf ~/repos/sysml-release/bnf/SysML-textual-bnf.kebnf
```

Writes `ir.json`. Findings: 641 productions load (290 KerML + 351 SysML), no
undefined references. 83 names are defined in both files and 34 of those
differ — the SysML file overrides KerML. 23 productions are left-recursive
(the import `FilterPackage` cycle and the whole expression subsystem).
91 of 176 alternations share a first token, so the grammar needs
backtracking, not LL(1). The grammar is annotated with abstract-syntax
assignments (`ownedRelationship += X`), so a generator can build the element
graph directly. 96 of 99 metaclass names and all but 4 assigned property
names exist in `SysML.ecore`.

## spike1 — interpret the IR, build a generic node tree

```
cd plan/spike1 && go run . ~/repos/sysml-pilot-implementation/org.omg.sysml/model/SysML.ecore <file.sysml>... [-tree]
```

A memoizing backtracking interpreter. Whether a production creates a node or
splices into the enclosing one (an Xtext `fragment`) is decided by whether
the node under construction is already a subtype of the production's
metaclass, using the ecore supertype links. Left recursion is cut by a
runtime guard, so operator expressions fail instead of looping.

| Corpus | Parsed |
|---|---|
| `sysml.library` `.sysml`, unmodified | 17 / 58 (`run_library.txt`) |
| `sysml/src/examples` | 32 / 95 (`run_examples.txt`) |

Remaining failures are mostly operator expressions (need the Xtext
precedence ladder) and grammar errata (below). Keeps one parse per end
position, so genuine ambiguity is not surfaced.

## spike2 — parse the Xtext grammars, diff against the `.kebnf`

```
cd plan/spike2 && go run . ~/repos/sysml-pilot-implementation ../spike1/ir.json
```

Merged SysML grammar: 549 rules, no undefined references. Compares the
minimum token length of the 368 rules present in both grammars; 41 differ.
Two are confirmed `.kebnf` errata where the optional marker was dropped:

- `LibraryPackage`: `( isStandard ?= 'standard' ) 'library'` (Xtext: `)?`)
- `SatisfyRequirementUsage`: `'assert' ( isNegated ?= 'not' ) 'satisfy'`
  (Xtext: `'assert'? ( isNegated ?= 'not' )?`)

Others on the list are benign factoring differences (e.g. `Identification`
is nullable in the `.kebnf` and its callers omit `?`; in Xtext it is
non-empty and callers add `?`). The rest are unreviewed.

## spike3 — spike1's interpreter on the Xtext IR

```
cd plan/spike3 && go run . ~/repos/sysml-pilot-implementation/org.omg.sysml/model/SysML.ecore <file.sysml>... [-tree]
GRAMMAR=kerml go run . <same ecore> <file.kerml>...        # KerML grammar
```

Same interpreter, now with Xtext semantics: `fragment` rules splice into the
enclosing node, `{Type.prop += current}` wraps the node built so far (this is
what gives operators their precedence and associativity with no
left-recursion handling), enums become strings, and the keyword set is every
identifier-shaped literal in the grammar. No per-construct code.

| Corpus | spike1 (`.kebnf`) | spike3 (Xtext) |
|---|---|---|
| Your 16 vendored fixtures | 5 | **16** (`run_fixtures.txt`) |
| `sysml.library` `.sysml`, unmodified | 17 / 58 | **58 / 58** (`run_library.txt`; also 58 / 58 against the `2026-08` library) |
| `sysml/src/examples` | 32 / 95 | **95 / 95** (`run_examples.txt`) |
| `sysml.library` `.kerml` (Kernel Libraries) | not run | **36 / 36** (`run_kerml_library.txt`) |
| `kerml/src/examples` | not run | **58 / 58** (`run_kerml_examples.txt`) |

All 263 trees conform to `SysML.ecore` (every class and property name
exists). `1 + 2 * 3` builds `1 + (2 * 3)`. Malformed input is rejected
(`attribute x = ;`, an unclosed brace, an unknown keyword), with an
expected-token list at the farthest failure. `InterfaceTest.sysml`'s
`end port p1: P1;` parses here and not under the `.kebnf`, so that fixture
was never stale; the `.kebnf` is.

### Oracle comparison

`compare.go` diffs our node tree against `sysml.library.xmi`, the pilot
implementation's own serialization of the same library files (plain XMI =
what the parser produces plus the parser post-processors; the `.implied`
variant adds implicit relationships and is not used yet).

**Pair the versions.** The oracle must come from the same release as the
Java being ported. The pilot checkout here is past tag `2026-08`; the
`sysml-release` checkout was an older master. The `AttributeDefinition`
clause in `UsageParserPostProcessor` was added between tags `2026-07` and
`2026-08`, which showed up as 13 spurious `isComposite` differences until
the oracle was taken from the `2026-08` tag:

```
cd ~/repos/sysml-release && git fetch --depth 1 origin tag 2026-08
git archive 2026-08 sysml.library sysml.library.xmi | tar -x -C /some/dir/rel2608
R=/some/dir/rel2608
find "$R/sysml.library" -name "*.sysml" -print0 | LIB_ROOT="$R/sysml.library" XMI_ROOT="$R/sysml.library.xmi" SAMPLES=1 \
  xargs -0 go run . ~/repos/sysml-pilot-implementation/org.omg.sysml/model/SysML.ecore
# KerML: the same with  GRAMMAR=kerml  and  "$R/sysml.library/Kernel Libraries" -name "*.kerml"
```

Both trees are canonicalized first: derived/transient features and features
redefined by a subclass are dropped, attributes equal to their ecore default
are dropped, in-file references (UUID attributes) count as references,
inverse-of-containment references are ignored (decided from the ecore: only
*source-end* roles), numbers compare numerically, quotes and comment
delimiters are stripped. `SAMPLES=1` prints three examples per difference
category, and each count mismatch names the sub-elements one side lacks.

Result against the `2026-08` library:

| | files | exact | differences |
|---|---|---|---|
| `.kerml` Kernel Libraries | 36 | **36** | **0** (`oracle_kerml.txt`) |
| `.sysml` Systems + Domain Libraries | 58 | **48** | 79 (`oracle_sysml.txt`) |

History, SysML differences: 10,418 (raw) → 4,114 (comparator normalizations)
→ 2,030 (expression structure) → 1,344 (tier-1 post-processors) → 79
(redefinition-aware roles, chain target feature, default multiplicity,
enumeration variations, owner-context fixes).

What is left in SysML, all of it needing name resolution or an unported
rule:

- `isConstant`, 74 differences (`UsageAdapter.doTransform`: an end usage is
  constant when `mayTimeVary()`, which asks whether the owning type
  *specializes* `Occurrences::Occurrence`, and whether the usage specializes
  `SelfLink`, `HappensLink` or `Action`).
- implied empty `Multiplicity` when a subsetted feature has no owning type, 3
  (the subsetting clause of `isAddDefaultMultiplicity`).
- an implied transition payload name and parameter, 2
  (`TransitionUsageAdapter`).

### Where the extra structure comes from

The grammar produces the skeleton; the rest is code in the pilot
implementation, in three tiers:

| Tier | Where | Size | Status here |
|---|---|---|---|
| 1. Parser post-processors | `org.omg.kerml.xtext/.../postprocessing`, `org.omg.sysml.xtext/.../postprocessing` | 46 files, ~2.2k lines | **Ported** (`postprocess.go`): relationship targets, `isEnd`, parameter `direction`, `isComposite` defaults, `isAbstract` for variations, filtered-import visibility. Not ported: string unescaping, comment-body processing (the comparator normalizes both), the `PortConjugation` original definition. |
| 2. Adapters | `org.omg.sysml.logic/.../adapter` (+ `util`) | 124 files, ~9.3k lines | **Started** (`semantic.go`): operand parameters, result parameters, the feature-chain source target, default multiplicity. The rest of the tier (implicit generalizations, `mayTimeVary`, connector ends, ...) needs name resolution. |
| 3. Delegates | `org.omg.sysml.logic/.../delegate` | 409 files, ~17.8k lines | Not started. Implementations of derived properties; mostly for queries and validation, not tree shape. |

(`org.omg.sysml.logic/.../expressions`, 2.5k lines, is the expression
evaluator and is not needed for structure.)

Rules are written as Go over the generic node tree, each naming the Java it
mirrors, and tested by re-running the comparison.

Still open: name resolution (the prerequisite for the rest of tier 2 and for
the `.implied` oracle), ambiguity (one parse is kept per end position),
emitting Go instead of interpreting (about 5 s for 153 files), and error
messages.

## Conclusion so far

A grammar-driven parser is not just workable, it is nearly done at the
syntax level: the Xtext grammar plus a ~900-line interpreter accepts the
entire standard library (both languages) and every example in the release.
Use the Xtext grammar as the input; the `.kebnf` has transcription errors
and is only useful as a cross-check. The syntax tree is not the whole job:
the oracle shows the pilot adds structure the grammar never mentions
(derived-feature wrapping, context-implied attributes and relationships),
and that layer has to be written by hand, test by test, against the oracle.
