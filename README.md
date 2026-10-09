# sysml-modeller

A from-scratch implementation of a SysML v2-style textual modeling
language: a lexer and recursive-descent parser (`sysml`) producing an
AST, translated into a flat, KerML-shaped element/relationship graph
(`metamodel`). It implements a deliberate, intentionally narrower subset
of the full OMG specification, grown incrementally against real example
models from the [OMG reference
repo](https://github.com/Systems-Modeling/SysML-v2-Release) rather than
against the spec wholesale — see **Known gaps** below for exactly where
the edge of that subset currently is.

For conventions on how to *work on* this codebase (build/test discipline,
fixture handling, a couple of sharp edges that have actually drawn blood),
see [`AGENTS.md`](AGENTS.md). This file is about what it does and how to
consume it.

## Using it

```go
import (
    "github.com/chrismooreproductions/sysml-modeller/sysml"
    "github.com/chrismooreproductions/sysml-modeller/metamodel"
)

ns, err := sysml.NewModel(source).Parse()   // text -> AST
if err != nil {
    // err carries a line/column pointing at the exact construct that
    // first failed to parse, e.g. "line 12: unexpected ')', want ';'"
}

model, err := metamodel.FromAST(ns)         // AST -> resolved graph
if err != nil {
    // e.g. an unresolvable name, type, or feature chain
}

for id, el := range model.Elements {
    // el.Kind, el.DefKind, el.Owner (containment), el.Value, el.Result, ...
}
id, ok := model.Resolve("Vehicle::Car", "Engine") // name lookup from a scope
```

`metamodel.FromASTWithImports(ns, map[string]*metamodel.Model{...})`
resolves `import` statements against externally-supplied models (e.g.
other files already translated), rather than only within one file.

Both `Parse` and `FromAST` genuinely resolve what they can: a qualified
type name, a feature chain, an operator symbol, a `subsets`/`redefines`/
`references`/`satisfy`/`verify` target are all checked against the real
model graph during translation, not just parsed and stored as strings —
an unresolvable reference is a translation error, not a silent gap.
There is currently no reverse direction (graph or AST back to text) and
no CLI — this is a library, consumed by importing the two packages
directly.

## Supported constructs

### Packages and namespaces

- A root namespace of top-level packages/definitions/usages/imports (not
  just one wrapping package).
- `package Name { ... }`, nestable.
- `public`/`private`/`protected` visibility prefixes — parsed and carried
  through; enforced only for wildcard-import re-export (see Imports).
- A leading `library` prefix (`library package 'User Defined Extensions'
  { ... }`) — recorded, not otherwise enforced.
- `//` and `/* */` comments — dropped entirely by the lexer (see **Known
  gaps**: this is why `doc` annotations aren't supported).
- A name anywhere one can appear — including a declared name — can be a
  single-quoted restricted name (`'User Defined Extensions'`), for
  characters an ordinary identifier can't hold. A usage or definition can
  also carry an optional short name ahead of its real one (`part <'1'>
  b: B;`) — recorded, not otherwise referenceable yet.

### Imports

- `import Vehicle;` against an externally supplied model.
- `import P1::*;` (wildcard) against a sibling namespace in the same
  model, making every member of `P1` resolvable as if declared locally.
- `import P1::**;` (recursive wildcard) — parsed, same resolution depth
  as the plain wildcard form today (not yet deepened further).
- A `private import` is not re-exported to whoever, in turn, wildcard-
  imports the importing namespace.

### Definitions and usages

- `part`/`attribute`/`item`/`port`/`constraint`/`calc`/`connection`/
  `interface`/`metadata`/`requirement`/`concern`/`case`/`analysis`/
  `verification`/`use case` as definition/usage keyword families, plus
  `subject`/`assume`/`require`/`frame`/`actor`/`stakeholder`/`in`/`out`/
  `inout`/`return`/`ref` as plain usage-only keyword families — all
  interchangeable in both a `def` and a bare usage (this project doesn't
  restrict which families are definition-only/usage-only the way real
  SysML does).
- Typed or untyped, in either order relative to a multiplicity
  (`[*]`, `[3]`, `[0..5]`, or a name-valued bound like `[n]`, left
  unresolved).
- A usage's name, and its type, are each independently optional
  (`constraint { mass <= massLimit }` is a fully anonymous, untyped
  usage — unambiguous with no lookahead needed).
- `subsets`/`:>`, `redefines`/`:>>`, `references`/`::>` on a usage, in
  any combination with a typing and a multiplicity.
- `specializes`/`:>` on a definition (`part def Vehicle :> VehiclePart;`,
  comma-separated for more than one) — this project doesn't distinguish
  KerML's Classifier-level specialization from Feature-level subsetting
  lexically; both reuse the same token.
- Any usage or definition can carry a nested body (`part p { part x; }`).
  A `constraint`/`calc`-shaped body (see below) can additionally end in a
  trailing, unterminated result expression.
- A member can omit its introducing keyword entirely (`mass :
  MassValue;`, `:>> mass = vehicle3.mass;`, defaulting to a plain
  attribute) — real KerML lets any usage do this for the base `Feature`
  type; this project narrows the default to `DefAttribute`.
- A leading `abstract` prefix on a definition, usage, or connection —
  recorded, not enforced.
- A member can carry one or more leading `#Tag` annotations (see
  Metadata).

### Expressions

- Literals: boolean, integer, real, string.
- A qualified-name reference, resolved by real lookup.
- Binary/unary operators (`mass <= massLimit`, `-x`, `not done`) with
  ordinary arithmetic/logical precedence and parenthesized grouping. Each
  operator resolves to a real `ScalarFunctions::'<='`-shaped
  standard-library stub (see **Standard library stub**), not an
  intrinsic built-in.
- Invocation expressions (`sum(componentMasses)`) — narrowly: the callee
  must be a bare/qualified name (not itself a feature chain or another
  call), and nothing chains after the call.
- Sequence expressions (`(engine.mass, frontAxleAssembly.mass,
  rearAxleAssembly.mass)`, two or more comma-separated elements). A
  single, comma-less parenthesized expression is always plain grouping,
  never a sequence.
- Feature chains (`vehicle.chassis.mass`) — resolved differently from a
  `::`-qualified name: only the first segment is an ordinary namespace
  lookup; each segment after that is looked up against the *type* of the
  previous one (or, if that segment is itself untyped with its own inline
  body, among its own direct members) — matching how a feature chain
  navigates through typed values rather than nested namespaces. Known gap:
  a chain segment always resolves to its type's *original* declaration of
  a name, even when the segment's own usage redefines that name more
  specifically — harmless today since nothing evaluates values, only
  resolves structural references.
- A bare name (in a `subsets`/`redefines`/`references` target or an
  ordinary expression) can resolve to a member a usage inherits from its
  own *type*, not just its containment ancestry — the mechanism real
  requirement derivation depends on (`subject :>> mass = vehicle.mass;`,
  where `mass` is declared only inside the requirement's own type).
- **Not yet supported**: conditional (`if`/`?`/`else`) expressions,
  constructor expressions, `collect`/`select`, metadata-access, casts
  (`as`).

### Constraints and calculations

- `constraint`/`calc` definitions and usages, with a `CalculationBody`:
  zero or more members, then an optional trailing result expression
  (`constraint def MassAnalysis { attribute totalMass : Real; ...
  totalMass <= massLimit }`).
- `AssertConstraintUsage` — `assert constraint massAnalysis : MassAnalysis
  { ... }`, or a bare reference to an existing constraint usage declared
  elsewhere (`assert massAnalysis3 { ... }`), or negated (`assert not
  massLimitation { ... }`) — not a new member type: this is an ordinary
  `constraint` usage reached a different way, with `Assert`/`Negated`
  recorded on it (the same fields `satisfy` uses).

### Connections, interfaces, bindings, successions

- An explicit connector part on a `connection`/`interface` usage:
  `connect p to y;` (bare, binary) or `connection bus : C connect (d1,
  d2, d3, d4);` (named, typed, n-ary). Each end is a plain or dotted
  feature reference, resolved like any other expression.
- `end` as a definition-body member (`end port p1 : P1;`, `end end1;`,
  `end #original ::> vehicleMassRequirement;`) — a bare prefix in front
  of an otherwise ordinary usage.
- `bind a = b;` / `binding ab bind a = b;` / `binding ab1 : AB bind a =
  b;` (`BindingConnectorAsUsage`'s three forms).
- `first a then b;` / `succession s first a then b;` / `succession s1 :
  AB first a then b;` (`SuccessionAsUsage`'s three forms) — structurally
  identical to binding, just spelled with `first`/`then`.

### Metadata

- One or more leading `#Tag` annotations on a member (`#Classified
  #Security z1;`), each resolved to a real `AnnotatedBy` relationship
  pointing at the tag's own declaration — not just parsed and stored.
- `metadata def`/`metadata` (or its `@` shorthand) as one more
  definition/usage family, with a plain member-list body.
- Simplified from the full grammar: a `#Tag` is parsed only once, right
  after an optional visibility keyword and before the definition/usage
  keyword (real SysML allows it interspersed more freely); each tag is a
  plain qualified name, not the fuller typing a real metadata usage
  allows.

### Requirements and concerns

- `requirement`/`concern` definitions and usages, with `subject`/
  `assume`/`require`/`frame`/`actor`/`stakeholder` as specially-kinded
  nested members.
- `require`/`assume` accept an optional inner `constraint` keyword
  (`frame`'s is `concern`) unlocking a `CalculationBody` (`require
  constraint { mass <= massLimit }`); otherwise they're a plain,
  `;`-terminated reference-shaped usage (`require c;`).
- `satisfy X by Y;`, with independently optional `assert`/`not` prefixes
  (`assert satisfy r by q;`). `X` is a bare reference to an existing
  requirement usage, or an inline `requirement req1 : Req1` declaration
  of a brand new one (declared as an ordinary sibling, resolvable by
  name from anywhere that scope reaches). `Y`, when present, resolves
  through the same expression/feature-chain machinery a connection's
  ends use.
- **Not yet supported**: a trailing body on a `satisfy` statement.

### Verification: Case, Analysis, Verification

- `case`/`analysis`/`verification` definitions and usages, all sharing
  `constraint`/`calc`'s own `CalculationBody` shape.
- `objective` as a plain usage-body member, the same shape `subject`/
  `require`/`frame` have.
- `verify x;` / `verify requirement req1 : Req1 { ... }` — usually nested
  inside an `objective` — links a case to the requirement it verifies,
  with the same bare-reference-vs-inline-declaration duality `satisfy`'s
  own `X` has.

### Use cases

- `use case def`/`use case` — the same `CalculationBody` shape as
  `case`/`analysis`/`verification`, spelled with the two-word keyword
  `"use" "case"` (the parser dispatches on it directly; the lexer never
  merges multi-word tokens).
- `include x;` / `include use case uc1 : UC1 { ... }` — the same
  bare-reference-vs-inline-declaration duality `verify` has.
- `actor` (see Requirements and concerns) doubles as a use case's actor.

### Actions (a narrow start)

A deliberately small slice of SysML's behavioral half — enough to
declare actions, nest them, and chain them explicitly; everything else
behavioral (see **Known gaps**) is still open.

- `action`/`action def` — an ordinary definition/usage keyword family,
  but with its own body shape (`ActionBody`): a plain member-list body,
  *except* that a bare `then X;` member implicitly connects from
  whatever the previous chain link was.
- `first X;` records `X` (an already-declared sibling action) as the
  chain's starting point, emitting no member of its own. `then X;`
  connects from that link — or from the most recently declared
  `action`/`perform` member, if no `first` came first — to `X`,
  desugaring into an ordinary `SuccessionAsUsage`-shaped connection
  (see **Connections, interfaces, bindings, successions**). `first X
  then Y;`, both ends spelled out on one member, works too, and
  continues the chain from `Y`.
- `perform u;` / `perform action a : A;` (`PerformActionUsage`) — the
  same bare-reference-vs-inline-declaration duality `verify`/`include`
  have. Unlike those, the resulting usage's kind is always `DefPerform`,
  never `DefAction` — `PerformActionUsage` is its own distinct kind in
  the real grammar, not literally typed as the thing it names.
- `X`/`Y` above are always a plain or dotted feature reference, never an
  inline node declaration the way real SysML's implicit
  `then <declaration>;` sugar allows — see **Known gaps**.

### Standard library stub

`metamodel/stdlib.go` hand-builds a small stand-in for a slice of the
real OMG standard library: `ScalarValues` (`Boolean`, `String`, `Real`,
`Rational`, `Integer`, `Natural`) and `ScalarFunctions` (the operator
symbols the expression subsystem resolves against). Both are implicitly
available without an explicit `import`, matching real SysML. This is a
deliberate bridge, not the genuine article — the real library is written
in KerML's own textual notation, which this project doesn't parse (see
the stub's own doc comment for the full reasoning).

## Known gaps

The biggest one by far: **behavioral modeling is only just started, and
states haven't been touched at all.** Actions (above) cover declaring
and chaining action nodes explicitly; still open within Actions:

- Implicit `start`/`done` pseudo-nodes (referenced without ever being
  declared, the way real examples lean on heavily).
- Control nodes (`decide`/`merge`/`fork`/`join`).
- Structured control (`if`/`else if`/`else`, `while`/`until`,
  `loop`/`until`, `for`/`in`).
- `accept`/`send`/`assign`/`terminate`'s own dedicated sub-grammars
  (trigger values, payloads, `via`/`to`, `:=`).
- `flow`.
- The implicit `then <declaration>;` sugar (declaring a brand-new node
  and connecting to it in one step) — this project only ever supports
  `then` connecting to an *existing*, already-declared reference.

**State machines haven't been started at all**: `state def`/`state`,
`entry`/`do`/`exit`, `TransitionUsage` (trigger/guard/effect clauses),
parallel states, action/state redefinition.

Everything else still open, each its own small, named gap:

- `doc` annotations — the lexer drops comments entirely before the
  parser ever sees them, so there's nowhere to hang the text.
- `enum def` (`EnumerationDefinition`).
- `flow def` (`FlowDefinition`) — structurally separate from connections.
- Conditional/constructor expressions, `collect`/`select`,
  metadata-access, casts (`as`) — see **Expressions** above.
- A trailing body on a `satisfy` statement.
- Semantic well-formedness validation (e.g. `validateUsageIsReferential`)
  — a separate compliance axis from parsing-and-resolving, not attempted
  anywhere in this project.

## References

This project follows the OMG specifications for the language it
implements. The [SysML-v2-Release](https://github.com/Systems-Modeling/SysML-v2-Release)
repo (OMG's own "start here" for SysML v2) is the most useful reference
day to day — in particular its concise textual grammars:

- [SysML-textual-bnf.kebnf](https://github.com/Systems-Modeling/SysML-v2-Release/blob/master/bnf/SysML-textual-bnf.kebnf)
- [KerML-textual-bnf.kebnf](https://github.com/Systems-Modeling/SysML-v2-Release/blob/master/bnf/KerML-textual-bnf.kebnf)

For the full normative specifications:

- [OMG Systems Modeling Language (SysML) v2](https://www.omg.org/spec/SysML/)
- [OMG Kernel Modeling Language (KerML)](https://www.omg.org/spec/KerML/)

SysML v2 is defined on top of KerML, its underlying kernel metamodel —
which is why this project's own `metamodel` package (a flat, containment
and relationship-based element graph) is described as "KerML-shaped": it
mirrors that layering rather than modeling SysML's surface syntax
directly.
