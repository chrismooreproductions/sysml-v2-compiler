# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `Facts.Roles` gives the requirement roles of an element: subject, actors,
  stakeholders, objective, assumed and required constraints, the requirement a
  satisfy usage satisfies and what satisfies it, and the requirements a
  verification case verifies. `sysmlc facts` prints them.
- `Facts.Behavior` gives the structure of an action or state: entry, do and
  exit actions, whether it is parallel, its successions and its transitions
  (source, target, trigger, guard, effect). `Facts.ControlNode` says whether a
  node is a fork, join, decision or merge. A succession end the source does
  not write, such as the first end of `then c`, has no target: it is not
  inferred from the order of the steps. `sysmlc facts` prints them.
- `Facts.Value` gives a feature's value expression: whether it is `default` or
  initial, the computed value when `Workspace.Evaluate` can compute it, and the
  expression's source text. `sysmlc facts` prints it.
- `Facts.Ends` lists the ends of a connector, connection, binding, succession,
  flow, interface or allocation with the feature each points to (the whole
  chain for `pa.out1`), and `Facts.Source` and `Facts.Target` give the ends of
  a flow or succession. `sysmlc facts` prints them.

## [0.10.0] - 2026-10-14

### Added

- `Workspace.Facts` returns the resolved facts about an element: identity,
  owner, members and how each is held, direction, the feature flags,
  multiplicity as numbers, and the typing, subsetting, reference subsetting,
  redefinition, specialization and conjugation relationships, written and
  implied. `sysmlc facts` prints them.

### Fixed

- A conjugated port typing (`port p : ~Fuel`) now resolves to the conjugated
  port definition of `Fuel`; the name was reported as unresolved.
- `QualifiedName` names a feature that has no declared name by the feature it
  redefines (`P::Car::eng`, not `P::Car`), as `Workspace.EffectiveName` does.
  Call it after `Resolve`; before that such a feature is skipped as before.

## [0.9.1] - 2026-10-13

### Changed

- A syntax error in a file that is valid in the other language (KerML text
  named `.sysml`, or SysML named `.kerml`) now says so and how to fix it.

## [0.9.0] - 2026-10-13

### Added

- `sysmlc lsp`: a Language Server Protocol server. It publishes diagnostics as
  files change and answers hover, go to definition, find references, rename,
  document symbols, workspace symbols and completion. `sysmlc lsp -listen addr`
  serves it over TCP, one server per connection.
- `Workspace.Evaluate` computes the value of an expression: literals,
  arithmetic, comparison and boolean operators, conditionals, sequences,
  ranges, indexing and references to features that have values. `sysmlc eval`
  prints the values it can compute. Calls to library functions are not yet
  supported.
- `Workspace.Replace` and `Workspace.Remove`: change one document without
  rebuilding the workspace or discarding what is known about the standard
  library.

### Changed

- Name resolution and implied-relationship computation are about twice as fast
  (the conformance run takes 49 seconds, not 137).

## [0.8.0] - 2026-10-13

### Fixed

- Names after the first in a feature chain such as `fn.samples.domainValue`
  now resolve from what the chain's source is (they were not found at all in a
  chain of three or more), as do chains after an index or select expression.
- A chain a connector's end references, such as `self.endShot` in a
  succession, is looked up from outside the connector.
- A feature that chains (`feature c chains a.b`) resolves its chain from where
  it sits.
- The named arguments of a constructor (`new T(x = 1)`) redefine features of
  `T`.
- A flow usage gets its own default supertypes (a message is `Flows::messages`).

Against the pilot implementation's own output, 99.95% of the standard library's
19,119 name references now resolve to the same element (99.81% before).

## [0.7.0] - 2026-10-12

### Added

- `Workspace.Materialize` writes the implied relationships into the trees of the
  documents, as elements marked Generated with `isImplied` set, with the
  features the specification makes up as their targets. `sysmlc dump -implied`
  shows them.

## [0.6.0] - 2026-10-12

### Added

- Queries over a resolved workspace: `Lookup` by qualified name, `Member`,
  `Members`, `OwnedMembers`, `EffectiveName`, `Supertypes`, `Specializes`,
  `Features` and `TypesOf`.

## [0.5.0] - 2026-10-12

### Added

- Seven more structural constraints are checked, 75 in all: the referential
  and individual rules for usages, references to non-features, conjugated
  specifics, binary associations and connectors, and connection definition ends.
- `Workspace.AddRecovering`: reports every syntax error in a file, not only the
  first, and adds the statements that did parse. `sysmlc check` uses it.

## [0.4.0] - 2026-10-12

### Added

- `Workspace.Validate` and `Diagnostic.Constraint`: 68 of the specification's
  structural constraints, named and worded as in the pilot implementation, are
  checked on the documents you add. `sysmlc check` reports them. The standard
  library is valid under all of them.

## [0.3.0] - 2026-10-12

### Added

- `Workspace.ImpliedGeneralizations`, `ImpliedFeaturingTypes` and
  `ImpliedBindings`: the relationships the specification implies but the source
  does not write (default supertypes per usage kind, cross features, featuring
  types, binding connectors). They match the pilot implementation's output for
  99.79% of the 99,685 implied relationships in the standard library.
- `sysmlc implied` prints them.

### Changed

- Name resolution follows chains after a cast, `(x as T).f`, and the library's
  structure now matches the pilot's in all 94 files (transition link features
  and payload names).

## [0.2.0] - 2026-10-12

### Changed

- **Breaking:** the hand-written parser, the simplified metamodel and the
  `metamodel` package are removed. The `sysml` package is new: `Parse`,
  `Workspace`, `Document`, `Diagnostic`, and a generic model typed by the
  specification's metaclasses with source spans. The grammar is the OMG pilot
  implementation's Xtext grammar, so all of SysML v2 and KerML parses.
- Names resolve across files and the bundled standard library using the pilot's
  scoping rules; unresolved names report file, position and the scope searched.

### Added

- `sysmlc` command with `check`, `resolve` and `dump`.
- Conformance tests against the pilot's XMI for the standard library.
- `LICENSE` (Eclipse Public License 2.0) and `NOTICE`, crediting the OMG SysML
  v2 release and pilot implementation the project draws on. Every Go file now
  carries an SPDX identifier.

## [0.1.0] - 2026-10-09

### Changed

- Require Go 1.27.2.

### Added

- Parser (`sysml`) and KerML-shaped metamodel translation (`metamodel`)
  covering packages, imports, definitions and usages, expressions,
  constraints, calculations, connections, metadata, requirements,
  verification, use cases and a narrow slice of actions. See the README
  for the exact supported constructs and known gaps.

[Unreleased]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.9.1...HEAD
[0.9.1]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/chrismooreproductions/sysml-v2-compiler/releases/tag/v0.1.0
