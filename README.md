# sysml-modeller

A from-scratch implementation of a SysML v2-style modeling language: a
lexer and recursive-descent parser (`sysml`) over a flat, KerML-shaped
metamodel (`metamodel`). Currently covers a root namespace of top-level
packages/definitions/usages/imports (not just one wrapping package),
`part`/`attribute`/`item`/`port` definitions and usages (typed or untyped,
in either order relative to a multiplicity), qualified (`::`) references,
`public`/`private`/`protected` visibility prefixes (parsed; enforced only
for wildcard-import re-export, see below), `//` and `/* */` comments,
multiplicity (`[*]`, `[3]`, `[0..5]`, or a name-valued bound like `[n]`,
left unresolved), assigned integer values (`= 5`), and `import` statements
against both an externally supplied `Model` (`import Vehicle;`) and a
sibling namespace within the same model (`import P1::*;`, making every
member of `P1` resolvable as if declared in the importing namespace) —
where a `private import` isn't re-exported to whoever, in turn,
wildcard-imports the importing namespace. Usages can also `subsets`/`:>`
or `redefines`/`:>>` another feature, in any combination with a typing and
a multiplicity. A deliberate subset of the full language, grown
incrementally rather than implemented against the spec wholesale.

`metamodel` also seeds every translation with a small hand-built stand-in
for a slice of the real OMG standard library (`metamodel/stdlib.go`):
`ScalarValues` (`Boolean`, `String`, `Real`, `Rational`, `Integer`,
`Natural`) and `ScalarFunctions` (the operator symbols the in-progress
expression subsystem resolves against, e.g. `+`, `<=`), both implicitly
available without an explicit `import`, matching real SysML. This is a
deliberate bridge rather than the genuine article — the real library is
written in KerML's own textual notation, which this project doesn't parse
— see the stub's own doc comment for the full reasoning.

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
