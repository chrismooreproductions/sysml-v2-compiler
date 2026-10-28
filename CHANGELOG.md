# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- The evaluator evaluates a feature chain (`ac.navigation.positionErrorM`): the
  value of its last feature, taken from the feature a part usage redefines it
  with when it does (`part ac : Ac { part :>> navigation { :>> x = 7.0; } }`),
  so a requirement constraint over values held on nested parts is no longer
  "not evaluable". A chain through a feature whose multiplicity allows more than
  one value says so (`engines has a multiplicity that allows 2 values, so
  ac.engines is a sequence of values, not one`). A requirement's subject in a
  chain (`subject s : Ac; ... s.x`) is the usage a `satisfy` names, so the
  satisfy's constraints see `ac.x`.

## [0.20.5] - 2026-10-26

### Fixed

- A variant is typed by the variation definition it is a variant of (an
  enumerated value by its enumeration), or subsets the variation usage, as
  `UsageAdapter.addVariationTyping` does. All 3,905 implied `FeatureTyping`
  relationships of the standard library now match the pilot's (33 were missing),
  and 99,519 of the 99,685 implied relationships do.

## [0.20.4] - 2026-10-26

### Fixed

- The release's example models now report no diagnostics, SysML and KerML
  (they reported 154 and 48 at v0.18.0, 2 and 28 at v0.20.3):
  - the target of a type featuring (`feature x featured by A { feature A ... }`)
    and of any other relationship that is not a specialization or conjugation is
    looked up from the type that owns it, as the pilot's `KerMLScopeProvider`
    does, so a feature can be featured by one of its own members;
  - `$::A` is `A` among the root namespaces (`'$'::A` is still a namespace
    named `$`);
  - a semantic metadata `baseType` can be a conditional that chooses by the
    metaclass of the annotated element (`if annotatedElement istype
    KerML::Structure ? SS meta ... else CC meta ...`);
  - the entry, do and exit actions of a state and the trigger, guard and effect
    actions of a transition redefine the library feature they stand for
    (`States::StateAction::entryAction`, `Actions::TransitionAction::effect`,
    ...), so `delivering.effect.sentMessage` finds the transition's own send
    action. One more implied relationship now matches the oracle (99,486).

## [0.20.3] - 2026-10-26

### Added

- Semantic metadata: a definition or usage annotated by metadata whose type
  specializes `Metaobjects::SemanticMetadata` is now specialized by the base
  type the metadata gives (`#service port def X` with
  `:>> baseType = SD meta SysML::PortDefinition` specializes `SD`, a usage is
  subset the same way), as TypeAdapter.getBaseTypes does in the pilot. Names
  from the base type resolve in it. The release's SysML example models now
  report 2 diagnostics where they reported 16 (154 at v0.18.0).

## [0.20.2] - 2026-10-26

### Changed

- Resolving a workspace is about three times faster after an edit: an edit of
  the 1,500-line Annex A model (replace, resolve and validate) takes 0.37 s
  where it took 1.33 s, and resolving alone 0.28 s where it took 1.2 s. A name
  is looked up in the namespaces that can give it without listing every
  membership each sees, a type's features are remembered, whether a library type
  specializes another is remembered, and the schema's redefinition queries no
  longer allocate. The conformance numbers are unchanged.

## [0.20.1] - 2026-10-26
### Added

- `lsp.Options.ResolveTimeout` bounds how long a session may hold a compute slot
  bringing its workspace up to date. When it runs out the work is abandoned,
  requests that waited for it fail with ContentModified (-32801), the client's
  open documents get a diagnostic saying the analysis timed out, the workspace
  is rebuilt, and the same text is not tried again until a document changes.
  `ResolveInfo.TimedOut` reports it to the hook. Off by default; parsing one
  document is not interrupted.

## [0.20.0] - 2026-10-26

### Changed

- The first workspace of a process builds the standard library's model with a
  lower memory peak: the library files are parsed four at a time instead of all
  94 at once, the parse trees are dropped once the model is built, and the
  memory the build left is returned to the operating system. The Go heap the
  process takes for it falls from about 267 MB to about 119 MB (live memory
  from 42 MB to 28 MB), for about a third more time (260 ms to 350 ms on a
  laptop).

### Added

- The language server is now a public package, `lsp`, to host many sessions in
  one Go program: `lsp.NewHost(Options)` for the limits, a bound on concurrent
  work, hooks and `Stats`; `Host.NewSession`, `Serve` (framed stream) and
  `ServeMessages` (one JSON-RPC message per call, for WebSocket frames or
  postMessage). `Session.SetDocument` and `RemoveDocument` give the session
  documents the client can use but not change, with the URIs the host chose in
  go to definition results; the package reads no files, ignores the client's
  root and workspace folders and writes no cache directory. Go to definition
  into the standard library answers `sysml-library:///<path>` (`LibraryURI`,
  `LibraryPath`, `LibraryText`). A newer edit abandons the resolution it made
  obsolete and `$/cancelRequest` ends a request; a panic in a session is
  recovered, and closing a session releases its workspace. `sysmlc lsp` is
  unchanged for the user and uses the package, with the disk and cache
  directory supplied by the command.
- `Workspace.ResolveContext` and `ValidateContext` give up when their context is
  done and return its error, so a host can abandon a resolve that a newer edit
  has made obsolete. `ReplaceAs` and `AddRecoveringAs` take the language
  explicitly, for documents whose names do not end in an extension.

## [0.19.2] - 2026-10-26

### Changed

- The model of the standard library is built once for the process and shared by
  every workspace, instead of being rebuilt for each one. A second workspace
  used to cost about 25 MB of live heap (and several times that in garbage
  while it was built, which is what a server saw as roughly 300 MB per
  connection); it now costs next to nothing. Workspaces can be used
  concurrently, one goroutine each; a single `Workspace` is still not safe for
  concurrent use. Nothing writes to the shared model: `ResolveAll`, which
  applies resolution rules to library elements, works on a private copy.

### Fixed

- A workspace no longer keeps every earlier version of a file in memory.
  `Replace` (and `Remove`) forgot what had been worked out about the old file
  by walking its tree, but resolution also makes helper elements that no tree
  owns (the features for `a.b` chains, bindings and the payload of a
  transition), and what was remembered about those still pointed into the old
  tree. A language server, which replaces the file on every edit, grew by about
  2 MB per edit for a 1,500-line model, which is why a connection's memory kept
  climbing. It is now flat.

## [0.19.1] - 2026-10-19

### Fixed

- `perform a.b;` is called `b`, so `driver.b` resolves. The Annex A
  SimpleVehicleModel example no longer reports four unresolved names
  (`driver.turnVehicleOn`, `vehicle.trigger1`, `vehicle.sendStatus`,
  `driver.trigger2`).

## [0.19.0] - 2026-10-16

### Added

- Quantities in different units of the same quantity convert through the
  conversions the standard library declares: by prefix (`km` to `m`) and by
  convention (`ft` to `m`, `h` to `min` to `s`). `3 [m] + 2 [km]` is
  `2003 [m]` (in the left operand's unit) and `3 [ft] == 0.9144 [m]` is true.
  Units of different quantities are reported as such (`kg and m measure
  different quantities`). A conversion the library gives as approximate
  (`isExact = false`, the dalton, the degree) marks the result
  `Quantity.Inexact`, and a comparison that is equal only to within an inexact
  factor is not decided.
- Derived units: a unit in `[ ]` can be a product, quotient or whole power of
  units (`100 [km/h]`, `9.81 [m/s^2]`), quantities can be multiplied and
  divided (`100 [km] / 2 [h]` is `50 [km/h]`; units of one quantity cancel, so
  `1 [km] / 1 [m]` is `1000`), and a derived unit of the library is expanded
  through its definition, so `5 [kg] * 2 [m/s^2]` equals `10 [N]` and
  `10 [N] * 2 [m]` equals `20 [J]`. `Quantity.Factors`, `UnitFactors()` and
  `UnitFactor` describe a derived unit. Conversions are the library's own, as
  released: its `tonne` is defined with a factor of `1.0e-3` against `kg` (every
  other entry reads "reference units per one of this unit", which would make a
  tonne 1000 kg) and its astronomical unit as `149597870691e11` metres (about
  1.5e22; the real value is 1.5e11), so quantities in those two convert as
  defined, not as physics. The vendored library is not edited.
  ([#19](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/19))

## [0.18.1] - 2026-10-15

Name resolution and validation fixes, found by checking the release's example
models together: the SysML examples go from 154 diagnostics to 20, and the KerML
ones from 48 to 28. The standard library now compiles on its own with no
diagnostics, and 19,115 of its 19,119 references match the pilot's (19,114
before). The counts in the entries below are the ones after each fix.

### Fixed

- An individual usage typed through what it subsets (`individual timeslice t3
  :> ind;`) is no longer reported as "must be typed by one individual
  definition": the constraints count the individual definitions among the
  usage's distinct types, not only the ones it writes.
  ([#28](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/28))
- The feature an assignment assigns is found among the features of the
  expression it is assigned on (`assign counter.count := 0`,
  `assign counting.counter.count := ...`), and a feature chain can follow the
  result of a call (`Increment(c).count`) or of a constructor
  (`new A().g.g`). The release's SysML example models report 21 diagnostics
  instead of 31, and the KerML ones 28 instead of 29.
  ([#24](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/24),
  [#25](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/25))
- A variant that names an existing usage (`variant q { ... }`) and a `perform`
  that names an action (`perform ActionTree::providePower;`) are called what
  they reference, so `v::q` and `perform action :>> providePower { ... }`
  resolve. The release's SysML example models report 31 diagnostics instead of
  48. ([#26](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/26))
- The payload an `accept` declares is visible to the rest of its transition
  (`accept pub : Sig if pub.topic > 0 do ...`), and has the type the accept
  gives it. The transition's payload parameter takes the accept's name before
  references are resolved, and subsets the chain from the accept to its payload
  as the pilot does. This was the standard library's last unresolved reference
  (`apayload` in `Actions.sysml`): the library now compiles on its own with no
  diagnostics, and 19,115 of its 19,119 references match the pilot (19,114
  before). The release's SysML example models report 48 diagnostics instead of
  71. ([#27](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/27))
- `Resolve` added a link feature to every transition that has a succession each
  time it was called (so `sysmlc check`, which resolves twice, made two); it is
  added once.
- A port typed by a conjugated port definition (`port p : ~FuelPort`) has the
  features of `FuelPort`: `p.fuel` and `in item :>> fuel` resolve. The port had
  no type from `~FuelPort`, and the conjugated definition inherited nothing
  from the definition it conjugates. The release's SysML example models report
  71 diagnostics instead of 119.
  ([#22](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/22))
- An import whose name comes through another import of the same namespace
  (`private import RiskMetadata::*; private import RiskLevelEnum::*;`) now
  resolves; it was reported as unresolved. The resolver remembered a lookup
  that failed only because a sibling import was still being resolved. The
  release's example models report 119 diagnostics instead of 154 (SysML) and 29
  instead of 48 (KerML) as a result.
  ([#21](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/21))

## [0.18.0] - 2026-10-15

### Added

- `Workspace.EvalConstraint` says whether a constraint holds, `EvalRequirement`
  does so for each assumed and required constraint of a requirement (its own
  and those it inherits), and `EvalCalculation` computes a calculation, for
  given inputs or the inputs a usage binds. A requirement usage gives its
  definition's attributes values by redefining them
  (`requirement r : MassReq { :>> massActual = 1800; }`), and a value that is
  missing makes the result not determined, naming it.
  ([#13](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/13))
- A call of a calculation, function or constraint the model defines is
  evaluated (`Range(40, 8)`, `Range(tankSize = 40, consumption = 8)`); a call
  of a library function says it has no body, instead of "cannot evaluate a
  InvocationExpression". A calculation that calls itself without end is not
  evaluable.
- `sysmlc eval` prints whether each requirement's constraints hold, and the
  value of each calculation usage and named constraint.
- A number with a unit (`1800 [kg]`) evaluates to a `Quantity` (`KindQuantity`)
  that carries its unit (`SI::kilogram`, symbol `kg`), instead of "SI::kilogram
  has no value". Within one unit quantities can be added, subtracted, compared
  and scaled by a number, and `mass / extra` in the same unit is a plain number.
  Converting between units (`3 [m] + 2 [km]`) and derived units (`mass * extra`,
  `[m/s]`) are not supported: they are not evaluable, with the reason, and
  never approximated. ([#14](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/14);
  the rest is [#19](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/19))

## [0.17.0] - 2026-10-15

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
  standard library are not content.
  ([#12](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/12),
  [#8](https://github.com/chrismooreproductions/sysml-v2-compiler/issues/8))
- `Workspace.Matches` evaluates a `filter` condition for an element: `@T` (the
  element is an instance of the metaclass, as `@SysML::PartUsage`, or has
  metadata whose type is or specializes `T`, as `@Safety`), `not`, `and`, `or`
  and `implies` over those, and comparisons on the element's metadata values
  (`@Safety and Safety::level > 2`). A condition it cannot evaluate is
  `NotEvaluable` with the reason and the part responsible; an element with the
  metadata but no value for the field is `NotDetermined`.
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
