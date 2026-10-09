package sysml

// Namespace is the root of a parsed model: an unnamed collection of
// top-level members (packages, definitions, usages, imports), mirroring the
// grammar's RootNamespace. Unlike Package, it has no Name and is never
// itself a Member -- there's no such thing as nesting one root inside
// another.
type Namespace struct {
	Members []Member
}

// Visibility is a member's `public`/`private`/`protected` prefix, e.g. the
// `private` in `private import Vehicle;`. The grammar attaches this to the
// membership relationship rather than the element itself (MemberPrefix), but
// this project has no first-class Membership yet, so it's folded onto the
// member directly. VisibilityUnspecified means no keyword was written --
// not yet given any enforced meaning (SysML defaults it to public), just
// recorded as parsed.
type Visibility int

const (
	VisibilityUnspecified Visibility = iota
	VisibilityPublic
	VisibilityPrivate
	VisibilityProtected
)

func (v Visibility) String() string {
	switch v {
	case VisibilityPublic:
		return "public"
	case VisibilityPrivate:
		return "private"
	case VisibilityProtected:
		return "protected"
	default:
		return "unspecified"
	}
}

// Package is a named container of members, e.g. `package Vehicle { ... }`.
// It's also itself a Member, so packages can nest inside one another (or
// appear directly in the root Namespace).
type Package struct {
	Visibility Visibility
	Name       string
	Members    []Member

	// Metadata is this member's `#Tag` prefix annotations, e.g. the
	// "Security" in `#Security enum def ...`. See DefMetadata's sibling
	// doc comment on Usage for what this simplifies away.
	Metadata []string

	// Library is this package's leading `library` prefix (LibraryPackage),
	// e.g. `library package 'User Defined Extensions' { ... }`. Recorded
	// but not otherwise used -- nothing currently treats a library package
	// any differently from an ordinary one. Real KerML also allows an
	// optional `standard` before `library`; that spelling isn't supported,
	// since no fixture here needs it yet.
	Library bool
}

func (*Package) memberNode() {}

// Member is anything that can appear inside a Package or a Definition's
// body.
type Member interface {
	memberNode()
}

// DefKind distinguishes the keyword family a Definition or Usage was
// declared with (e.g. "part" vs "item"), independent of the def-vs-usage
// structural distinction those two types already carry.
type DefKind int

const (
	DefPart DefKind = iota
	DefAttribute
	DefItem
	DefPort
	DefConstraint
	DefCalculation
	DefConnection
	DefInterface
	DefMetadata
	DefRequirement
	DefConcern
	DefCase
	DefSubject
	DefAssume
	DefRequire
	DefFrame
	DefActor
	DefStakeholder
	// DefEnd tags an "end" body member with no recognized inner keyword
	// (e.g. "end p1: P1;", "end end1;") -- see parseEndMember. An "end"
	// with a recognized inner keyword (e.g. "end port p1: P1;") is tagged
	// with that keyword's own DefKind instead (DefPort here), not DefEnd.
	DefEnd
	// DefIn, DefOut, and DefInOut tag a parameter's direction (e.g. the
	// "in" in `in partMasses : MassValue[0..*];`), and DefReturn tags a
	// calc's result parameter (`return totalMass : MassValue = ...;`).
	// DefRef tags a plain reference-usage keyword (`ref :>> system;`).
	// Each is an ordinary defKeywords entry like any other -- no dedicated
	// parsing beyond the keyword itself.
	DefIn
	DefOut
	DefInOut
	DefReturn
	DefRef
	// DefBind tags a BindingConnectorAsUsage ("bind a = b;", "binding ab
	// bind a = b;", "binding ab1 : AB bind a = b;") -- see Connection's
	// own doc comment and parseBindPart. Not a defKeywords entry (unlike
	// every other DefKind): "bind"/"binding" are dispatched from
	// parseMember directly, the same way "connect"'s own bare shorthand
	// is.
	DefBind
	// DefAnalysis and DefVerification tag "analysis"/"verification" --
	// AnalysisCaseDefinition/Usage and VerificationCaseDefinition/Usage,
	// two more CalculationBody-shaped keyword families alongside DefCase
	// (see hasCalculationBody). DefObjective tags "objective" --
	// ObjectiveRequirementUsage, a plain usage-body keyword like
	// DefSubject. None of the three need any parsing beyond the keyword
	// itself. Appended here, after DefBind, rather than alongside
	// DefCase/DefConcern/DefRequirement above where they conceptually
	// belong -- metamodel.DefKind mirrors this type's iota values purely
	// by shared ordering (see translate.go's direct DefKind(m.Kind) casts
	// and metamodel.DefKind's own doc comment), so a new constant must
	// always be appended at the end of both const blocks together, never
	// inserted in the middle of either one alone.
	DefAnalysis
	DefVerification
	DefObjective
	// DefSuccession tags a SuccessionAsUsage ("first a then b;",
	// "succession s first a then b;", "succession s : S first a then
	// b;") -- see Connection's own doc comment and parseSuccessionPart.
	// Structurally identical to DefBind (two ConnectorEnds, an optional
	// name/type, an optional body), just with "first"/"then" in place of
	// "bind"/"="; not a defKeywords entry, for the same reason DefBind
	// isn't -- "first"/"succession" are dispatched from parseMember
	// directly.
	DefSuccession
	// DefUseCase tags "use case"/"use case def" -- UseCaseDefinition/
	// Usage, one more CalculationBody-shaped keyword family alongside
	// DefCase/DefAnalysis/DefVerification (see hasCalculationBody), just
	// spelled with the two-word keyword "use case" rather than one
	// token. Not a defKeywords entry (unlike every other
	// CalculationBody-shaped kind): "use" is dispatched from parseMember
	// directly, since the lexer never merges "use"+"case" into a single
	// token.
	DefUseCase
	// DefAction tags "action"/"action def" -- an ordinary defKeywords
	// entry, but with its own distinct body shape (ActionBody, see
	// parseActionBody) -- neither CalculationBody-shaped (no trailing
	// result) nor a plain member list, since a bare "then X;" member
	// threads an implicit predecessor through the body.
	DefAction
	// DefPerform tags "perform" (PerformActionUsage) -- a bare reference
	// to an existing action ("perform u;") or an inline declaration
	// ("perform action a : A;"), the same duality
	// AssertConstraintUsage/verify/include already have. Its own body is
	// an ordinary ActionBody too. Not a defKeywords entry -- "perform"
	// is dispatched from parseMember directly.
	DefPerform
)

func (k DefKind) String() string {
	switch k {
	case DefPart:
		return "part"
	case DefAttribute:
		return "attribute"
	case DefItem:
		return "item"
	case DefPort:
		return "port"
	case DefConstraint:
		return "constraint"
	case DefCalculation:
		return "calc"
	case DefConnection:
		return "connection"
	case DefInterface:
		return "interface"
	case DefMetadata:
		return "metadata"
	case DefRequirement:
		return "requirement"
	case DefConcern:
		return "concern"
	case DefCase:
		return "case"
	case DefSubject:
		return "subject"
	case DefAssume:
		return "assume"
	case DefRequire:
		return "require"
	case DefFrame:
		return "frame"
	case DefActor:
		return "actor"
	case DefStakeholder:
		return "stakeholder"
	case DefAnalysis:
		return "analysis"
	case DefVerification:
		return "verification"
	case DefObjective:
		return "objective"
	case DefEnd:
		return "end"
	case DefIn:
		return "in"
	case DefOut:
		return "out"
	case DefInOut:
		return "inout"
	case DefReturn:
		return "return"
	case DefRef:
		return "ref"
	case DefBind:
		return "bind"
	case DefSuccession:
		return "succession"
	case DefUseCase:
		return "use case"
	case DefAction:
		return "action"
	case DefPerform:
		return "perform"
	default:
		return "unknown"
	}
}

// Definition declares a definition, e.g. `part def Engine;` or
// `part def Car { ... }`. Members is nil for the semicolon form.
//
// A constraint/calculation definition's body (CalculationBody) can end in
// a trailing, unterminated result expression after its members, e.g. the
// "totalMass == sum(componentMasses)" directly before the closing brace in
// `constraint def MassAnalysis { attribute totalMass: MassValue; ...
// totalMass == sum(componentMasses) }` -- captured in Result, nil for
// every other DefKind and for a body without one.
type Definition struct {
	Visibility Visibility
	Kind       DefKind
	Name       string
	Members    []Member
	Result     Expression

	// Metadata is this member's `#Tag` prefix annotations -- see Usage's
	// doc comment on its own Metadata field for the full explanation.
	Metadata []string

	// Specializes is the definition's classifier-level specialization list
	// (KerML's SubclassificationPart), e.g. the "VehiclePart" in
	// `part def Vehicle :> VehiclePart;` -- comma-separated, so more than
	// one target can appear (`part def C :> A, B;`), unlike Usage.Subsets's
	// single target. nil if none. Spelled "specializes" or ":>" (both
	// parse the same way); this project reuses Usage's own Subsets token
	// for both spellings, a deliberate simplification -- real KerML keeps
	// Classifier-level specialization and Feature-level subsetting as
	// distinct relationships that merely share the ":>" symbol (not the
	// word: only "subsets" is Feature-level, only "specializes" is
	// Classifier-level), a distinction this project doesn't model.
	Specializes []string

	// Abstract is this definition's leading `abstract` prefix, e.g.
	// `abstract part def B { ... }`. Recorded but not enforced -- nothing
	// checks that an abstract definition is never directly instantiated,
	// the one thing being abstract is actually supposed to mean.
	Abstract bool

	// ShortName is this definition's optional "<Name>" (Identification's
	// own declaredShortName), e.g. the "xx" in `part def <xx> B { ... }`.
	// "" if none. Recorded but not otherwise used -- nothing can currently
	// reference a definition by its short name instead of its real one.
	ShortName string
}

func (*Definition) memberNode() {}

// Usage declares a usage, e.g. `part engine : Engine;`, `port p;` (untyped),
// or `part friendlyCombatants : Combatant[*];` for one bounded by a
// Multiplicity. Type, Multiplicity, Subsets, Redefines, and Value are each
// independently optional, and any combination of Type/Multiplicity/Subsets/
// Redefines may appear in any order (e.g. `part b[0..2] : P;`,
// `part c : P[2..*];`, or `part b subsets a;`) -- each is "" (or nil, for
// Multiplicity/Value) when absent, regardless of order or which others were
// written. Only a single Subsets and a single Redefines target are
// supported, not SysML's comma-separated lists of either.
//
// Name is also independently optional -- SysML's Identification is broadly
// optional throughout, and an anonymous usage (Name == "") is how e.g. an
// inline constraint (`constraint { mass <= massLimit }`) is written.
// Disambiguating an absent name needs no lookahead: nothing that can start
// a usage's specialization part (':' , '[', 'subsets', ':>', ...) is ever
// spelled as a bare identifier, so seeing anything other than an
// Identifier right after the keyword unambiguously means there's no name.
type Usage struct {
	Visibility   Visibility
	Kind         DefKind
	Name         string
	Type         string
	Multiplicity *Multiplicity

	// Subsets is the feature this usage subsets, e.g. the "a" in
	// `part b subsets a;` or `part b :> a;` (both spellings parse the same
	// way). "" if none.
	Subsets string

	// Redefines is the feature this usage redefines, e.g. the "B::b" in
	// `part B_b redefines B::b;` or `part B_b :>> B::b;`. "" if none.
	Redefines string

	// References is the feature this usage references, e.g. the "d1" in
	// `end end2 ::> d1;` or `attribute ::> m = ms.totalMass;` (both spellings
	// -- "references"/"::>" -- parse the same way). "" if none. Distinct
	// from Subsets/Redefines the same way real KerML's ReferenceSubsetting
	// is its own relationship, not a synonym for either.
	References string

	// Value is the usage's assigned value, e.g. the 5 in
	// `attribute n : ScalarValues::Integer = 5;`. Only SysML's plain '='
	// FeatureValue form is supported (not ':=' or 'default'). nil if no
	// value was assigned.
	Value Expression

	// Members and Result are this usage's optional body: any usage may have
	// one ("; " or "{ members }"), populating Members alone. Result is also
	// populated when the body is a CalculationBody (see hasCalculationBody
	// in parser.go) instead of a plain member-list one -- DefConstraint/
	// DefCalculation always, DefRequire/DefAssume only when written with
	// their optional inner "constraint" keyword (e.g. the
	// "mass <= massLimit" in `require constraint { mass <= massLimit }`,
	// see requirementConstraintInnerKeyword in parser.go).
	Members []Member
	Result  Expression

	// Metadata is this usage's `#Tag` prefix annotations, e.g. the
	// "Classified" and "Security" in `#Classified #Security z1;` -- each a
	// shorthand for "this member has an anonymous metadata usage typed by
	// Tag" (PrefixMetadataAnnotation). Simplified from the full grammar in
	// two ways: only parsed once, immediately after an optional leading
	// visibility keyword and before the definition/usage keyword itself
	// (real SysML allows `#Tag` interspersed with other prefix keywords in
	// more positions, including on a bare usage with no kind keyword at
	// all -- e.g. `ref #Classified z1;` -- which this project doesn't
	// support declaring in the first place); and each Tag is a plain
	// qualified name, not the fuller OwnedFeatureTyping a real
	// PrefixMetadataUsage allows.
	Metadata []string

	// Abstract is this usage's leading `abstract` prefix, e.g.
	// `abstract part a: A[1..2];`. Recorded but not enforced -- see
	// Definition.Abstract's own doc comment.
	Abstract bool

	// ShortName is this usage's optional "<Name>" (FeatureIdentification's
	// own declaredShortName), e.g. the "'1'" in `part <'1'> b: B;`. "" if
	// none -- see Definition.ShortName's own doc comment for what it's
	// (not) used for. Independent of Name: a short name can appear with a
	// real name, or (not yet exercised by any fixture here) alone.
	ShortName string

	// Assert and Negated are an AssertConstraintUsage's own "assert"/
	// "not" prefixes, e.g. `assert constraint massAnalysis : MassAnalysis
	// { ... }` or `assert not massLimitation { ... }` -- mirroring
	// Satisfy's own Assert/Negated fields (see its doc comment), since
	// AssertConstraintUsage turns out not to need a new Member type at
	// all: ConstraintUsageDeclaration is itself a ConstraintUsage, so this
	// is just an ordinary DefConstraint usage reached via parseAssertConstraint
	// instead of the usual defKeywords dispatch. Always false for every
	// other DefKind.
	Assert  bool
	Negated bool
}

func (*Usage) memberNode() {}

// Connection is a connection/interface usage's connector part or a
// BindingConnectorAsUsage, e.g. `connect p to y;` (binary, no name/type at
// all -- the bare "'connect' ConnectorPart" shorthand),
// `connection bus : C connect (d1, d2, d3, d4);` (n-ary, with a name and
// type), or `bind a = b;` / `binding ab bind a = b;` /
// `binding ab1 : AB bind a = b;` (always binary, its own bare "'bind' ...
// '=' ..." shorthand or an optional "binding" name/type ahead of it --
// see parseBindPart). Distinct from Usage because a connector's ends
// aren't expressed through FeatureSpecializationPart syntax: each of Ends
// is a *NameRef or *FeatureChain (a plain or dotted feature reference,
// e.g. "y" or "p1.x"), reusing the ordinary expression machinery rather
// than inventing a separate reference shape. Kind is DefConnection,
// DefInterface, or DefBind.
//
// Deliberately out of scope: the optional multiplicity/`references`
// prefix on an individual end (ConnectorEnd's own fuller grammar),
// `succession` (SuccessionAsUsage -- a structurally unrelated shorthand
// form, not a ConnectorPart variant, though the same general shape `bind`
// is), and `end` as a body member introducing a connector end inside a
// definition (only the usage-level connect syntax is covered here).
type Connection struct {
	Visibility Visibility
	Kind       DefKind
	Name       string
	Type       string
	Ends       []Expression
	Members    []Member

	// Metadata is this member's `#Tag` prefix annotations -- see Usage's
	// doc comment on its own Metadata field for the full explanation.
	Metadata []string

	// Abstract is this connection's leading `abstract` prefix. Recorded
	// but not enforced -- see Definition.Abstract's own doc comment.
	Abstract bool
}

func (*Connection) memberNode() {}

// Expression is a value-producing expression, e.g. the "5" in
// `attribute n : Integer = 5;` or the "mass <= massLimit" a constraint
// body will eventually carry. This project implements a deliberate subset
// of KerML's full expression grammar: literals, qualified-name references,
// binary/unary operators, (narrowly, see InvocationExpr) invocation
// expressions, and (see SequenceExpr) parenthesized sequences. Not
// supported, each being its own separate chunk of grammar: conditional
// (`if`/`?`/`else`) expressions, constructor expressions, index
// expressions, `collect`/`select`, metadata-access, and casts (`as`/
// `istype`/`hastype`/`@`).
type Expression interface {
	expressionNode()
}

// BoolLiteral is a literal `true` or `false`.
type BoolLiteral struct{ Value bool }

func (*BoolLiteral) expressionNode() {}

// IntLiteral is a literal integer, e.g. the 5 in `= 5`.
type IntLiteral struct{ Value int }

func (*IntLiteral) expressionNode() {}

// StringLiteral is a literal double-quoted string, e.g. `"hello"`. No
// escape sequences are supported.
type StringLiteral struct{ Value string }

func (*StringLiteral) expressionNode() {}

// RealLiteral is a literal floating-point number, e.g. `3.14`.
type RealLiteral struct{ Value float64 }

func (*RealLiteral) expressionNode() {}

// NameRef is a reference to another element by qualified name, e.g. the
// "massLimit" in `mass <= massLimit`. Resolved the same way a Usage's Type
// is -- see metamodel.NameRef.
type NameRef struct{ Path string }

func (*NameRef) expressionNode() {}

// FeatureChain is a dot-separated chain of feature references, e.g.
// "vehicle.chassis.mass" in `subject :>> mass = vehicle.chassis.mass;`.
// Path[0] is the (possibly "::"-qualified) first segment; each segment
// after that is a single plain name. Resolved differently from a
// "::"-qualified NameRef: each later segment is looked up among the
// *type* of the previous one, not among a namespace's direct members --
// see metamodel.Model.ResolveFeatureChain.
type FeatureChain struct{ Path []string }

func (*FeatureChain) expressionNode() {}

// InvocationExpr is a function/operation invocation, e.g.
// "sum(componentMasses)" in `totalMass == sum(componentMasses)`. Callee is
// a bare (possibly "::"-qualified) name -- resolved the same way a NameRef
// is -- not itself a feature chain or another invocation, and nothing
// chains after the call (no "foo().bar" or "foo()()"): each is its own
// separate chunk of grammar, deliberately out of scope here the same way
// Expression's own doc comment excludes conditional/cast expressions.
type InvocationExpr struct {
	Callee string
	Args   []Expression
}

func (*InvocationExpr) expressionNode() {}

// SequenceExpr is a parenthesized, comma-separated sequence of values,
// e.g. "(vehicle, vehicle)" or "(engine.mass, frontAxleAssembly.mass,
// rearAxleAssembly.mass)" -- KerML's SequenceExpression, used for
// multi-valued parameter bindings and sequence-typed attributes. A
// single-element, comma-less parenthesized expression is never a
// SequenceExpr -- see parsePrimary's OpenParen case -- so "(1 + 2)"
// parses exactly as it always has, as a plain grouped expression with no
// extra node at all. Always at least two Elements.
type SequenceExpr struct {
	Elements []Expression
}

func (*SequenceExpr) expressionNode() {}

// BinaryExpr is a binary operator expression, e.g. `mass <= massLimit`.
// The supported operator set matches this project's standard-library stub
// (see metamodel/stdlib.go) exactly: "+ - * / % ** < > <= >= == != xor |
// &" -- notably not "and"/"or"/"implies"/"??" (KerML's separate
// ConditionalBinaryOperatorExpression, not BinaryOperatorExpression, and
// unstubbed). Op resolves to a standard-library Function the same way a
// Usage's Type resolves to a Definition -- see metamodel.BinaryExpr.
type BinaryExpr struct {
	Op          string
	Left, Right Expression
}

func (*BinaryExpr) expressionNode() {}

// UnaryExpr is a unary operator expression, e.g. `-x` or `not done`. Op is
// one of "+", "-", or "not" -- reusing BinaryExpr's "+"/"-" Function stubs
// regardless of arity, since nothing here models a Function's parameters
// to distinguish a one-argument overload from a two-argument one anyway.
type UnaryExpr struct {
	Op      string
	Operand Expression
}

func (*UnaryExpr) expressionNode() {}

// Unbounded marks a Bound's Value as unlimited, e.g. the "*" in "[*]" or
// "[1..*]".
const Unbounded = -1

// Bound is one side of a Multiplicity range: either a literal integer
// (Value, where Unbounded means "*"), or an unresolved name reference to a
// declared value, e.g. the "n" in "[n]" (Name). Resolving a name bound
// needs feature-reference expressions, which are out of scope here -- it's
// parsed and stored, never resolved. Name is "" for a literal or Unbounded
// bound, in which case Value holds the actual bound; Value is meaningless
// when Name is set.
type Bound struct {
	Value int
	Name  string
}

// Multiplicity bounds how many instances a Usage represents, spelled
// as a bracketed suffix on its type: "[*]" (0..Unbounded), "[3]" (an exact
// count, Lower == Upper), "[1..*]", "[0..5]", or a name-valued bound like
// "[n]" or "[1..n]".
type Multiplicity struct {
	Lower Bound
	Upper Bound
}

// Import declares that another namespace's members should be resolvable
// from this one, e.g. `import Vehicle;` (a MembershipImport, importing
// Vehicle itself by name), `import Vehicle::*;` (a NamespaceImport,
// Wildcard set, importing every one of Vehicle's members as if declared
// directly in the importing namespace), or `import Vehicle::**;` (also
// Recursive, reaching every member of every namespace nested inside
// Vehicle too, not just Vehicle's own direct members). Path is the
// imported namespace's ("::"-qualified) name. Recursive is only ever true
// alongside Wildcard; parsed but not yet given its fuller resolution
// semantics -- metamodel's wildcard-import resolution reaches only Path's
// own direct children today, the same as a plain Wildcard import, so a
// name declared in a nested namespace still won't resolve through one of
// these. The FilterPackage form of NamespaceImport isn't supported.
type Import struct {
	Visibility Visibility
	Path       string
	Wildcard   bool
	Recursive  bool
}

func (*Import) memberNode() {}

// Satisfy is a "satisfy X by Y;" member (SatisfyRequirementUsage), asserting
// that Y satisfies the requirement X, e.g. `satisfy r by p;` or
// `assert not satisfy r1 by q;`. Assert and Negated mirror the "assert" and
// "not" prefixes, each independently optional (all four combinations --
// neither, either, or both -- appear in real examples). X is one of two
// alternatives (SatisfyRequirementUsage's own grammar): a bare (possibly
// "::"-qualified) reference to an existing requirement usage -- the real
// grammar's own OwnedReferenceSubsetting, resolved the same way a usage's
// own References is (see metamodel's Satisfy handling) -- stored in
// Requirement, with Declaration nil; or an inline
// `requirement req1 : Req1` declaration of a brand new requirement usage
// (just its UsageDeclaration -- see parseUsageDeclaration -- no value or
// body of its own), stored in Declaration, with Requirement "". Exactly
// one of the two is ever set. By is nil if no "by" clause was written;
// when present, it's a plain or dotted feature reference (*NameRef or
// *FeatureChain), reusing the same expression machinery a Connection's
// ends do. Deliberately out of scope: a trailing RequirementBody (real
// SysML lets a satisfy statement carry its own nested members after the
// "by" clause; this project's satisfy is always ";"-terminated).
type Satisfy struct {
	Visibility  Visibility
	Assert      bool
	Negated     bool
	Requirement string
	Declaration *Usage
	By          Expression
}

func (*Satisfy) memberNode() {}
