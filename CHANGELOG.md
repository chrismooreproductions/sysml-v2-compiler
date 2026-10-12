# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/chrismooreproductions/sysml-v2-compiler/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/chrismooreproductions/sysml-v2-compiler/releases/tag/v0.1.0
