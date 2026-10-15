# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `Workspace.Eval` evaluates an expression and returns a `Result`: a typed
  value (`Kind`: boolean, integer, real, string, sequence or null), or a
  status of `NotDetermined` (a feature it needs has no value) or
  `NotEvaluable` (unsupported, or the model is wrong), with the `Reason`, the
  `Source` element and its `Span`. It never guesses a value.
  ([#10](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/10))
- `EvalError.Unbound` says that a failure is only a missing value.
- `Workspace.ViewContent` evaluates a view: the elements its exposes bring in
  (membership, namespace and recursive forms; a view that specializes another
  has its exposes, and the filters of the view and of its definition) that pass
  its filters, in order, each with the expose that brought it in. A filter that
  cannot be decided for an element removes nothing: the result says which
  filters were not applied and why, and which elements they could not be decided
  for. Annotations and the members a model's namespace inherits from the
  standard library are not content. `Workspace.ImportedElements` is the
  lower-level operation.
  ([#12](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/12),
  [#8](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/8))
- `Workspace.Matches` evaluates a `filter` condition for an element: `@T` (the
  element is an instance of the metaclass, as `@SysML::PartUsage`, or has
  metadata whose type is or specializes `T`, as `@Safety`), `not`, `and`, `or`
  and `implies` over those, and comparisons on the element's metadata values
  (`@Safety and Safety::level > 2`). A condition it cannot evaluate is
  `NotEvaluable` with the reason and the part responsible; an element with the
  metadata but no value for the field is `NotDetermined`. `Workspace.EvaluateFor`
  does the same at the lower level.
  ([#11](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/11))

### Changed

- `and`, `or` and `implies` follow three-valued logic: `false and x`,
  `true or x` and `x implies true` are decided even when `x` has no value.
  When a missing value and an unsupported operand meet, the unsupported one is
  reported, so "not determined" always means that binding a value would help.
- `sysmlc eval` reports a feature it cannot compute as `not determined: ...` or
  `not evaluable: ...` with the position, instead of leaving it out.
- The "cannot apply" evaluation error writes strings quoted.

## [0.16.0] - 2026-10-15

### Added

- `Facts.Filters` lists the filter conditions of a view or package
  (`filter @SysML::PartUsage;`) in source order, each with its expression and
  its text as written. `sysmlc facts` prints them.
  ([#7](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/7))

### Changed

- `Facts.Members` no longer lists the condition expression of a `filter` as a
  member; it is in `Facts.Filters`.

## [0.15.0] - 2026-10-15

### Added

- `Facts.PortionKind` says whether a portion of an occurrence is a
  `"timeslice"` or a `"snapshot"`; it is empty for anything else.
  `sysmlc facts` prints it. ([#6](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/6))

## [0.14.0] - 2026-10-15

### Added

- `Facts.Behavior.Condition` and `ConditionText` give the condition of an `if`
  or `while` action (`count > 0`), as written.
  ([#5](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/5))

### Changed

- A succession that is a step of an action or state and writes no reference
  for an end (`first start; then action receive;`, `then if ... { }`,
  `then while ...`) now has the step before it as its first end and the member
  after it as its second, in `Facts.Ends`, `Facts.Source`, `Facts.Target` and
  `Behavior.Successions`, marked `End.Implied`. They were nil. The rule agrees
  with the pilot's output for the standard library; the guarded branches of a
  decision and a transition's source are still not inferred.
  ([#5](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/5))

## [0.13.0] - 2026-10-15

### Added

- `LibraryFiles` lists the files of the embedded standard library, each with
  its `Path`, `Language` and `Source`, in path order, so a consumer can show
  the library or compile it itself (`NewWorkspace(WithoutStandardLibrary())`,
  then `AddAs` each file). `Workspace.FileOf` names the file of a library
  element `library:` followed by that `Path`. All but one of the library's
  references resolve when its files are compiled this way.
  ([#4](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/4))

### Fixed

- A feature with no declared name takes the name of the feature it
  *implicitly* redefines, as the pilot does, not only one the source writes:
  an unnamed `return : T[1]` is `result`, so `F::result::dimension` resolves to
  `F`'s own return parameter instead of the inherited `result` (the reference
  was unresolved). Against the pilot's own output, 19,114 of the standard
  library's 19,119 references now match (19,110 before). `EffectiveName` now
  says `source` or `target` for an unnamed connector end; `Facts.End.Name` is
  still the declared name.

## [0.12.1] - 2026-10-15

### Fixed

- `Facts.Annotations` no longer lists a metadata usage (`@Owner { ... }`) as a
  comment with an empty body. It holds documentation, comments and textual
  representations only. ([#3](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/3))

## [0.12.0] - 2026-10-15

### Added

- `Facts.IsMessage` tells a `message` from a `flow`.
  ([#2](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/2))

### Fixed

- `Facts.Ends` is no longer empty for a `message`: its ends are the event
  occurrences it runs between (`from driver.cmd to vehicle.cmd`, with the whole
  chain in `Path`, or `from driver to vehicle`), and `Facts.Source` and
  `Facts.Target` are set as for a flow.
  ([#2](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/2))

## [0.11.1] - 2026-10-14

### Fixed

- `expose` no longer fails validation with "An expose must import all", and a
  view's `expose` now imports private members as the pilot does: it defaults
  `isImportAll` to true. ([#1](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/1))
- `render` (and any usage given a default multiplicity when the feature it
  subsets is not owned by a type) no longer fails validation with "Only one
  multiplicity is allowed": `Resolve` added the default multiplicity again each
  time it was called, and `check` calls it twice.
  ([#1](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/1))
- A conjugated port typing whose port definition does not resolve (`port p :
  ~Pkg::Fuel` with `Pkg` in another file) crashed the resolver with a nil
  pointer dereference since 0.10.0; it is now reported as unresolved.

## [0.11.0] - 2026-10-14

### Added

- `Facts.Annotations` gives the documentation, comments and textual
  representations that annotate an element (those it owns, and those anywhere
  in the workspace that name it with `about`), each with its text, locale or
  language, and the elements it annotates. `sysmlc facts` prints them.
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
