# sysml-modeller

A from-scratch implementation of a SysML v2-style modeling language: a
lexer and recursive-descent parser (`sysml`) over a flat, KerML-shaped
metamodel (`metamodel`). Currently covers packages, part definitions and
usages, qualified (`::`) references, multiplicity (`[*]`, `[n]`, `[n..m]`),
and a minimal `import` statement for cross-package type resolution — a
deliberate subset of the full language, grown incrementally rather than
implemented against the spec wholesale.

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
