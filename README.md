# sysml-modeller

A from-scratch implementation of a SysML v2-style modeling language: a
lexer and recursive-descent parser (`sysml`) over a flat, KerML-shaped
metamodel (`metamodel`). Currently covers a root namespace of top-level
packages/definitions/usages/imports (not just one wrapping package),
`part`/`attribute`/`item`/`port`/`constraint`/`calc`/`connection`/`interface`
definitions and usages (typed or untyped, in either order relative to a
multiplicity), qualified (`::`) references,
`public`/`private`/`protected` visibility prefixes (parsed; enforced only
for wildcard-import re-export, see below), `//` and `/* */` comments,
multiplicity (`[*]`, `[3]`, `[0..5]`, or a name-valued bound like `[n]`,
left unresolved), assigned integer values (`= 5`), and `import` statements
against both an externally supplied `Model` (`import Vehicle;`) and a
sibling namespace within the same model (`import P1::*;`, making every
member of `P1` resolvable as if declared in the importing namespace) —
where a `private import` isn't re-exported to whoever, in turn,
wildcard-imports the importing namespace. Usages can also `subsets`/`:>`,
`redefines`/`:>>`, or `references`/`::>` another feature, in any
combination with a typing and a multiplicity. A definition can likewise
`specializes`/`:>` one or more other definitions (`part def Vehicle :>
VehiclePart;`, or a comma-separated list) — this project doesn't
distinguish real KerML's Classifier-level specialization from Feature-level
subsetting lexically, reusing the same token for both. A deliberate subset
of the full language, grown incrementally rather than implemented against
the spec wholesale.

A usage's assigned value (`= ...`) can now be a full expression, not just
an integer literal: boolean/integer/real/string literals, a qualified-name
reference, binary/unary operators over them (`mass <= massLimit`, `-x`,
`not done`), and invocation expressions (`sum(componentMasses)`, zero or
more comma-separated arguments) — narrowly: the callee must be a bare/
qualified name, not itself a feature chain or another call, and nothing
chains after the call. Ordinary arithmetic/logical precedence and
parenthesized grouping apply throughout. Not yet supported: conditional
(`if`/`?`/`else`) expressions, constructor expressions, sequences,
`collect`/`select`, metadata-access, and casts. Every name reference,
operator, and invocation callee is genuinely resolved during translation
(an operator like `<=` resolves to a real `ScalarFunctions::'<='`-shaped
standard-library stub), not just parsed and stored.

An expression can also be a dot-separated feature chain
(`vehicle.chassis.mass`), resolved differently from a `::`-qualified name:
only the first segment is an ordinary namespace lookup, and each segment
after that is looked up among the *type* of the previous one (following
its typing relationship to a definition and finding a member there) — or,
if that segment has no type at all (an untyped usage with its own inline
body instead), among its own direct members — matching how a feature chain
actually navigates through typed values rather than through nested
namespaces. One related, narrower known gap: a chain segment always
resolves to its *type's* original declaration of a name, even when the
segment's own usage redefines that name more specifically — harmless today
since this project only ever resolves structural references, never
evaluates values.

A bare name — in a `subsets`/`redefines`/`references` target, or an
ordinary expression reference — can also resolve to a member a usage
inherits from its own type (not just its containment ancestry), e.g. the
"mass" in `requirement r : R { subject :>> mass = vehicle.mass; }`,
declared only inside `R`'s own body. This is the mechanism real requirement
derivation depends on throughout.

Any usage, not just a definition, can carry its own nested body (`part p {
part x; }`), and a `constraint`/`calc` definition or usage's body can
additionally end in a trailing, unterminated result expression
(`constraint def MassAnalysis { attribute totalMass : Real; attribute
massLimit : Real; totalMass <= massLimit }`) — the one shape a plain
member-list body doesn't have. A usage's name is also independently
optional throughout, since nothing that can start a usage's specialization
part is ever spelled as a bare identifier: an anonymous usage like
`constraint { mass <= massLimit }` is unambiguous with no lookahead needed.

A `connection`/`interface` usage can carry an explicit connector part:
`connect p to y;` (binary, no name or type at all — the bare shorthand
form) or `connection bus : C connect (d1, d2, d3, d4);` (n-ary, named and
typed). Each end is a plain or dotted feature reference (`y`, `p1.x`),
resolved through the same feature-chain machinery as any other expression.
`end` can also introduce a body member inside a *definition* (`end port
p1: P1;`, `end end1;`, `end #original ::> vehicleMassRequirement;`) — a
bare prefix in front of an otherwise ordinary usage, optionally itself
starting with a recognized keyword (used as that usage's own kind) or with
none at all. Not yet supported: `bind`/`succession` (structurally
unrelated shorthand forms) and `abstract` as a prefix modifier.

`metamodel` also seeds every translation with a small hand-built stand-in
for a slice of the real OMG standard library (`metamodel/stdlib.go`):
`ScalarValues` (`Boolean`, `String`, `Real`, `Rational`, `Integer`,
`Natural`) and `ScalarFunctions` (the operator symbols the expression
subsystem resolves against, e.g. `+`, `<=`), both implicitly available
without an explicit `import`, matching real SysML. This is a deliberate
bridge rather than the genuine article — the real library is written in
KerML's own textual notation, which this project doesn't parse — see the
stub's own doc comment for the full reasoning.

A member can also carry one or more leading `#Tag` annotations (`#Classified
#Security z1;`), each resolved to a real `AnnotatedBy` relationship pointing
at the tag's own declaration, the same as a type or subsets reference is —
not just parsed and stored. `metadata def`/usage (or its `@` shorthand) is
one more definition/usage keyword family, whose usage form takes a plain
member-list body rather than the constraint/calc shape. Simplified from the
full grammar: a `#Tag` is only parsed once, right after an optional leading
visibility keyword and before the definition/usage keyword itself (real
SysML allows it interspersed more freely, including on a bare usage with no
kind keyword at all); and each tag is a plain qualified name, not the fuller
typing a real metadata usage allows. Not yet supported: single-quoted/
restricted names (`'User Defined Extensions'`) — unrelated to metadata
itself, but the first thing a real metadata-heavy example file trips on.

`requirement`/`concern`/`case` are three more definition/usage keyword
families, alongside `subject`/`assume`/`require`/`frame`/`actor`/
`stakeholder`, each introducing a specially-kinded nested member inside a
requirement's body (e.g. the `subject mass :> ISQ::mass;` inside
`requirement def MassRequirement { ... }`). `require`/`assume` accept an
optional inner `constraint` keyword (`frame`'s is `concern`) that unlocks a
trailing-result body of their own — `require constraint { mass <= massLimit
}` — otherwise (`require c;`, `assume c1 [0..*];`) they're a plain,
";"-terminated reference-shaped usage. A `requirement`/`concern` usage (not
just a `def`) can itself carry a plain nested body, e.g. `requirement
vehicleMassRequirement : MassRequirement { subject :>> mass = vehicle.mass;
}`. `satisfy X by Y;` asserts that Y satisfies the requirement usage named
X, with independently optional `assert`/`not` prefixes (`assert satisfy r by
q;`, `not satisfy r1 by p;`); X resolves the same reference-subsetting way a
usage's own `references` does, Y through the same expression/feature-chain
machinery a Connection's ends use. Not yet supported: an inline `satisfy
requirement req1 : Req1 by system;` declaration (only a bare reference to an
existing requirement usage is), a trailing body on a `satisfy` statement,
`doc` annotations (this project's parser drops comments entirely before
seeing them, so there's nowhere to hang the text), and `case`'s own richer
`CaseBody` (a bare `case`/`case def` parses, but never with a body).

A handful of recurring gaps that blocked real examples well before their
own subject matter have since closed: every usage kind (not just
constraint/calc-shaped ones) can carry a general body; `end` introduces a
body member inside a connection/interface *definition* (see above); and
`in`/`out`/`inout`/`return` (parameter-direction keywords) and `ref`
(the plain reference-usage keyword) are five more keyword families, each
the exact same plain usage shape `subject`/`actor`/`stakeholder` already
have. Still open, each its own fixture's current blocker: `bind`/
`succession`, `abstract`, `AssertConstraintUsage`'s non-`satisfy` form,
sequence expressions, the inline `satisfy` declaration above, short names,
single-quoted/restricted names, and recursive imports.

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
