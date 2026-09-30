package sysml

import (
	"fmt"
	"strconv"
)

type parser struct {
	tokens []Token
	pos    int
}

// newParser drops whitespace and comment tokens up front: the parser only
// cares about structure, never layout or documentation.
func newParser(tokens []Token) *parser {
	filtered := make([]Token, 0, len(tokens))
	for _, tok := range tokens {
		if tok.Kind != Space && tok.Kind != Comment {
			filtered = append(filtered, tok)
		}
	}
	return &parser{tokens: filtered}
}

func (p *parser) current() (Token, bool) {
	if p.pos >= len(p.tokens) {
		return Token{}, false
	}
	return p.tokens[p.pos], true
}

func (p *parser) expect(k Kind) (Token, error) {
	tok, ok := p.current()
	if !ok {
		return Token{}, fmt.Errorf("unexpected end of input, want %s", k)
	}
	if tok.Kind != k {
		return Token{}, fmt.Errorf("line %d: unexpected %s, want %s", tok.Pos.Line, describeToken(tok), k)
	}
	p.pos++
	return tok, nil
}

func describeToken(tok Token) string {
	if tok.Kind == Identifier {
		return fmt.Sprintf("identifier %q", string(tok.Value))
	}
	return tok.Kind.String()
}

func (p *parser) parsePackage() (*Package, error) {
	if _, err := p.expect(Pkg); err != nil {
		return nil, err
	}

	name, err := p.expect(Identifier)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(OpenBrace); err != nil {
		return nil, err
	}

	members, err := p.parseMembers()
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(CloseBrace); err != nil {
		return nil, err
	}

	return &Package{Name: string(name.Value), Members: members}, nil
}

// parseMembers parses members until it sees a closing brace or runs out of
// tokens; the caller is responsible for consuming the closing brace itself.
func (p *parser) parseMembers() ([]Member, error) {
	var members []Member
	for {
		tok, ok := p.current()
		if !ok || tok.Kind == CloseBrace {
			return members, nil
		}

		member, err := p.parseMember()
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
}

// defKeywords maps each definition/usage keyword token to the DefKind it
// introduces -- the dispatch table parseMember consults after ruling out
// package and import members. Every entry is accepted equally in both a
// "X def Name { ... }" definition and a bare "X name : Type;" usage; this
// project doesn't restrict which keyword families are definition-only or
// usage-only the way real SysML does (e.g. "subject def Foo;" parses fine
// here, even though real SysML only ever lets "subject" introduce a usage).
var defKeywords = map[Kind]DefKind{
	Part:          DefPart,
	Attribute:     DefAttribute,
	Item:          DefItem,
	Port:          DefPort,
	ConstraintKw:  DefConstraint,
	CalcKw:        DefCalculation,
	ConnectionKw:  DefConnection,
	InterfaceKw:   DefInterface,
	MetadataKw:    DefMetadata,
	RequirementKw: DefRequirement,
	ConcernKw:     DefConcern,
	CaseKw:        DefCase,
	SubjectKw:     DefSubject,
	AssumeKw:      DefAssume,
	RequireKw:     DefRequire,
	FrameKw:       DefFrame,
	ActorKw:       DefActor,
	StakeholderKw: DefStakeholder,
}

// isConnectorKind reports whether kind's usage form can carry an explicit
// "connect" connector part (see Connection), instead of only ever being a
// plain ";"-terminated declaration.
func isConnectorKind(kind DefKind) bool {
	return kind == DefConnection || kind == DefInterface
}

// hasCalculationBody reports whether kind's usage/definition body, once an
// OpenBrace is seen, is a CalculationBody (zero or more members, then an
// optional trailing result expression) rather than an ordinary member-list
// body (every usage/definition can have a body at all now -- see
// parseUsage/parseDefinition's shared OpenBrace handling; this predicate
// only decides its shape once one is present). DefRequire/DefAssume only
// take this shape when written with their optional inner "constraint"
// keyword (see requirementConstraintInnerKeyword) -- e.g. the
// "{ mass <= massLimit }" in `require constraint { mass <= massLimit }` --
// the plain bare-reference form ("require c;") never has a body at all.
func hasCalculationBody(kind DefKind) bool {
	return kind == DefConstraint || kind == DefCalculation || kind == DefRequire || kind == DefAssume
}

// requirementConstraintInnerKeyword maps a RequirementBodyItem prefix
// keyword (require/assume/frame) to the optional inner keyword its own
// nested usage may (but need not) spell out explicitly -- e.g. the
// "constraint" in `require constraint c1 :>> c;`, present in
// `require constraint { mass <= massLimit }` (the only way its
// CalculationBody -- see hasCalculationBody -- becomes reachable at all),
// but omittable in the plain reference shorthand ("require c;"). Consumed
// as a no-op by parseUsage before its usual name/specialization parsing --
// it never changes the resulting Usage.Kind, which stays DefRequire/
// DefAssume/DefFrame regardless, the same way every other DefKind is tagged
// by the keyword that introduced the member, not by any keyword nested
// inside it.
var requirementConstraintInnerKeyword = map[DefKind]Kind{
	DefRequire: ConstraintKw,
	DefAssume:  ConstraintKw,
	DefFrame:   ConcernKw,
}

// visibilityKeywords maps each visibility keyword token to the Visibility
// it introduces, for parseMember's optional leading prefix.
var visibilityKeywords = map[Kind]Visibility{
	PublicKw:    VisibilityPublic,
	PrivateKw:   VisibilityPrivate,
	ProtectedKw: VisibilityProtected,
}

// parseVisibility consumes a leading 'public'/'private'/'protected'
// keyword if the next token is one, returning the Visibility it names.
// Consumes nothing and returns VisibilityUnspecified otherwise.
func (p *parser) parseVisibility() Visibility {
	tok, ok := p.current()
	if !ok {
		return VisibilityUnspecified
	}
	vis, ok := visibilityKeywords[tok.Kind]
	if !ok {
		return VisibilityUnspecified
	}
	p.pos++
	return vis
}

func (p *parser) parseMember() (Member, error) {
	vis := p.parseVisibility()
	metadata, err := p.parseMetadataPrefixes()
	if err != nil {
		return nil, err
	}

	tok, ok := p.current()
	if !ok {
		return nil, fmt.Errorf("unexpected end of input, want %s, %s, or %s", Pkg, ImportKw, Part)
	}

	if tok.Kind == Pkg {
		pkg, err := p.parsePackage()
		if err != nil {
			return nil, err
		}
		pkg.Visibility = vis
		pkg.Metadata = metadata
		return pkg, nil
	}

	if tok.Kind == ImportKw {
		imp, err := p.parseImport()
		if err != nil {
			return nil, err
		}
		imp.Visibility = vis
		return imp, nil
	}

	if tok.Kind == ConnectKw {
		// The bare "'connect' ConnectorPart" shorthand: no "connection"/
		// "interface" keyword, no name, no type at all.
		p.pos++
		conn, err := p.parseConnectorPart(DefConnection, "", "")
		if err != nil {
			return nil, err
		}
		conn.Visibility = vis
		conn.Metadata = metadata
		return conn, nil
	}

	if tok.Kind == SatisfyKw || tok.Kind == AssertKw || p.startsSatisfyNot() {
		sat, err := p.parseSatisfy()
		if err != nil {
			return nil, err
		}
		sat.Visibility = vis
		return sat, nil
	}

	if tok.Kind == EndKw {
		member, err := p.parseEndMember()
		if err != nil {
			return nil, err
		}
		if usage, ok := member.(*Usage); ok {
			usage.Visibility = vis
		}
		return member, nil
	}

	defKind, ok := defKeywords[tok.Kind]
	if !ok {
		return nil, fmt.Errorf("line %d: unexpected %s, want %s, %s, or %s", tok.Pos.Line, describeToken(tok), Pkg, ImportKw, Part)
	}
	keyword := tok.Kind
	p.pos++

	tok, ok = p.current()
	if !ok {
		return nil, fmt.Errorf("unexpected end of input after %s", keyword)
	}

	if tok.Kind == Def {
		p.pos++
		def, err := p.parseDefinition(defKind)
		if err != nil {
			return nil, err
		}
		def.Visibility = vis
		def.Metadata = metadata
		return def, nil
	}

	member, err := p.parseUsage(defKind)
	if err != nil {
		return nil, err
	}
	switch m := member.(type) {
	case *Usage:
		m.Visibility = vis
		m.Metadata = metadata
	case *Connection:
		m.Visibility = vis
		m.Metadata = metadata
	}
	return member, nil
}

// parseMetadataPrefixes consumes zero or more `#Tag` prefix annotations
// (PrefixMetadataAnnotation), e.g. the "Classified" and "Security" in
// `#Classified #Security z1;`, returning their (plain, not feature-chain)
// qualified type names in the order written. See Usage.Metadata's doc
// comment for what this simplifies away from the full grammar.
func (p *parser) parseMetadataPrefixes() ([]string, error) {
	var tags []string
	for {
		tok, ok := p.current()
		if !ok || tok.Kind != Hash {
			return tags, nil
		}
		p.pos++

		tag, err := p.parseQualifiedName()
		if err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
}

func (p *parser) parseDefinition(kind DefKind) (*Definition, error) {
	name, err := p.expect(Identifier)
	if err != nil {
		return nil, err
	}

	tok, ok := p.current()
	if !ok {
		return nil, fmt.Errorf("unexpected end of input after %s def %q, want %s or %s", kind, string(name.Value), Semicolon, OpenBrace)
	}

	switch tok.Kind {
	case Semicolon:
		p.pos++
		return &Definition{Kind: kind, Name: string(name.Value)}, nil

	case OpenBrace:
		p.pos++
		if hasCalculationBody(kind) {
			members, result, err := p.parseCalculationBody()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(CloseBrace); err != nil {
				return nil, err
			}
			return &Definition{Kind: kind, Name: string(name.Value), Members: members, Result: result}, nil
		}
		members, err := p.parseMembers()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(CloseBrace); err != nil {
			return nil, err
		}
		return &Definition{Kind: kind, Name: string(name.Value), Members: members}, nil

	default:
		return nil, fmt.Errorf("line %d: unexpected %s, want %s or %s", tok.Pos.Line, describeToken(tok), Semicolon, OpenBrace)
	}
}

func (p *parser) parseImport() (*Import, error) {
	if _, err := p.expect(ImportKw); err != nil {
		return nil, err
	}

	path, wildcard, err := p.parseImportTarget()
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(Semicolon); err != nil {
		return nil, err
	}

	return &Import{Path: path, Wildcard: wildcard}, nil
}

// parseImportTarget parses an import's target: a qualified name (e.g.
// "Vehicle::Electrical"), optionally followed by "::*" naming every member
// of that namespace rather than the namespace itself, e.g. "P1::*" (a
// NamespaceImport). Shaped like parseQualifiedName's loop, but checks for a
// trailing "::*" at each "::" rather than always requiring another
// identifier.
func (p *parser) parseImportTarget() (path string, wildcard bool, err error) {
	first, err := p.expect(Identifier)
	if err != nil {
		return "", false, err
	}

	name := string(first.Value)
	for {
		tok, ok := p.current()
		if !ok || tok.Kind != PathSep {
			return name, false, nil
		}
		p.pos++

		if tok, ok := p.current(); ok && tok.Kind == Star {
			p.pos++
			return name, true, nil
		}

		next, err := p.expect(Identifier)
		if err != nil {
			return "", false, err
		}
		name += "::" + string(next.Value)
	}
}

// parseUsage parses a usage. Its name is optional (see Usage's doc
// comment): a leading Identifier is consumed as the name if present,
// otherwise the usage is anonymous and nothing is consumed for it.
// parseUsage parses a usage declaration and, for most DefKinds, the plain
// Usage that follows from it. For a connector kind (see isConnectorKind)
// followed by an explicit "connect" clause, it instead returns a
// Connection carrying the name/type already parsed -- the one case this
// function's result isn't a *Usage.
func (p *parser) parseUsage(kind DefKind) (Member, error) {
	usage := &Usage{Kind: kind}

	if inner, ok := requirementConstraintInnerKeyword[kind]; ok {
		if tok, ok := p.current(); ok && tok.Kind == inner {
			p.pos++
		}
	}

	if tok, ok := p.current(); ok && tok.Kind == Identifier {
		p.pos++
		usage.Name = string(tok.Value)
	}

	if err := p.parseFeatureSpecializationPart(usage); err != nil {
		return nil, err
	}

	if err := p.parseValuePart(usage); err != nil {
		return nil, err
	}

	if isConnectorKind(kind) {
		if tok, ok := p.current(); ok && tok.Kind == ConnectKw {
			p.pos++
			return p.parseConnectorPart(kind, usage.Name, usage.Type)
		}
	}

	if tok, ok := p.current(); ok && tok.Kind == OpenBrace {
		p.pos++
		if hasCalculationBody(kind) {
			members, result, err := p.parseCalculationBody()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(CloseBrace); err != nil {
				return nil, err
			}
			usage.Members = members
			usage.Result = result
			return usage, nil
		}
		members, err := p.parseMembers()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(CloseBrace); err != nil {
			return nil, err
		}
		usage.Members = members
		return usage, nil
	}

	if _, err := p.expect(Semicolon); err != nil {
		return nil, err
	}
	return usage, nil
}

// parseConnectorPart parses a ConnectorPart -- "a to b" (binary) or
// "(a, b, c, ...)" (n-ary) -- producing a Connection with kind/name/typ
// already known (both "" for the bare "connect a to b;" shorthand, which
// has no preceding usage declaration at all). The caller has already
// consumed "connect".
func (p *parser) parseConnectorPart(kind DefKind, name, typ string) (*Connection, error) {
	conn := &Connection{Kind: kind, Name: name, Type: typ}

	if tok, ok := p.current(); ok && tok.Kind == OpenParen {
		p.pos++
		for {
			end, err := p.parseConnectorEnd()
			if err != nil {
				return nil, err
			}
			conn.Ends = append(conn.Ends, end)

			tok, ok := p.current()
			if !ok {
				return nil, fmt.Errorf("unexpected end of input, want %s or %s", Comma, CloseParen)
			}
			if tok.Kind == CloseParen {
				p.pos++
				break
			}
			if _, err := p.expect(Comma); err != nil {
				return nil, err
			}
		}
	} else {
		first, err := p.parseConnectorEnd()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(ToKw); err != nil {
			return nil, err
		}
		second, err := p.parseConnectorEnd()
		if err != nil {
			return nil, err
		}
		conn.Ends = []Expression{first, second}
	}

	if _, err := p.expect(Semicolon); err != nil {
		return nil, err
	}
	return conn, nil
}

// parseConnectorEnd parses one connector end: a plain or dotted feature
// reference (e.g. "y" or "p1.x"), reusing the same NameRef/FeatureChain
// parsing a primary expression already uses (see Connection's doc comment
// for what a fuller ConnectorEnd -- an optional leading multiplicity or
// "name references" form -- leaves out).
func (p *parser) parseConnectorEnd() (Expression, error) {
	path, err := p.parseQualifiedName()
	if err != nil {
		return nil, err
	}
	return p.parseFeatureChainTail(path)
}

// peekKind returns the Kind of the token offset positions ahead of the
// current one (0 is current() itself), without consuming anything. ok is
// false past the end of input.
func (p *parser) peekKind(offset int) (kind Kind, ok bool) {
	i := p.pos + offset
	if i >= len(p.tokens) {
		return 0, false
	}
	return p.tokens[i].Kind, true
}

// startsSatisfyNot reports whether the upcoming tokens are "not satisfy" --
// a Satisfy statement's bare-"not" form, e.g. "not satisfy r1 by p;". A
// leading "not" alone is ambiguous with a unary-"not" expression (e.g. a
// calc body's trailing "not done" result), so this needs a one-token
// lookahead: only a Satisfy statement is ever followed by "satisfy" here.
func (p *parser) startsSatisfyNot() bool {
	tok, ok := p.current()
	if !ok || tok.Kind != NotKw {
		return false
	}
	next, ok := p.peekKind(1)
	return ok && next == SatisfyKw
}

// parseSatisfy parses a "satisfy X by Y;" member (see Satisfy's doc
// comment): an optional leading "assert", an optional leading "not"
// (independently of each other), the "satisfy" keyword, a bare (possibly
// "::"-qualified) reference to an existing requirement usage, and an
// optional "by <expression>" clause. The caller has already established
// (via startsSatisfyNot, or seeing "satisfy"/"assert" directly) that this
// is the right production to try.
func (p *parser) parseSatisfy() (*Satisfy, error) {
	sat := &Satisfy{}

	if tok, ok := p.current(); ok && tok.Kind == AssertKw {
		p.pos++
		sat.Assert = true
	}

	if tok, ok := p.current(); ok && tok.Kind == NotKw {
		p.pos++
		sat.Negated = true
	}

	if _, err := p.expect(SatisfyKw); err != nil {
		return nil, err
	}

	name, err := p.parseQualifiedName()
	if err != nil {
		return nil, err
	}
	sat.Requirement = name

	if tok, ok := p.current(); ok && tok.Kind == ByKw {
		p.pos++
		by, err := p.parseConnectorEnd()
		if err != nil {
			return nil, err
		}
		sat.By = by
	}

	if _, err := p.expect(Semicolon); err != nil {
		return nil, err
	}
	return sat, nil
}

// parseEndMember parses an "end" body member (DefaultInterfaceEnd/
// ConnectorEnd's def-body form), e.g. "end port p1: P1;", "end p1: P1;",
// "end end1;", or "end #original ::> vehicleMassRequirement;" -- "end" is a
// bare prefix in front of an otherwise ordinary Usage, optionally itself
// starting with a recognized keyword (consumed and used as that keyword's
// own DefKind, e.g. "port" -> DefPort) or with none at all (DefEnd).
// Reuses parseUsage for everything else (name/type/multiplicity/subsets/
// redefines/references/value/body), so an "end" gets the exact same
// specialization machinery any other usage does.
func (p *parser) parseEndMember() (Member, error) {
	if _, err := p.expect(EndKw); err != nil {
		return nil, err
	}

	// Metadata comes after "end" here (e.g. "end #original ::> x;"), unlike
	// everywhere else it's parsed ahead of the introducing keyword -- see
	// parseMember's own leading parseMetadataPrefixes call, which is a
	// harmless no-op for "end" members since none of this project's real
	// fixtures put a "#tag" before "end" itself.
	metadata, err := p.parseMetadataPrefixes()
	if err != nil {
		return nil, err
	}

	kind := DefEnd
	if tok, ok := p.current(); ok {
		if inner, ok := defKeywords[tok.Kind]; ok {
			kind = inner
			p.pos++
		}
	}

	member, err := p.parseUsage(kind)
	if err != nil {
		return nil, err
	}
	if usage, ok := member.(*Usage); ok {
		usage.Metadata = metadata
	}
	return member, nil
}

// parseCalculationBody parses a constraint/calculation body
// (CalculationBody): zero or more ordinary members, followed optionally
// by one trailing, unterminated result expression -- e.g. the
// "totalMass == sum(componentMasses)" directly before the closing brace,
// with no semicolon of its own. A token is treated as starting another
// member (see startsMember) for as long as one keeps matching; the first
// token that doesn't is either the result expression or (if it's the
// closing brace) simply the end of an empty/member-only body. The caller
// consumes the surrounding braces.
func (p *parser) parseCalculationBody() (members []Member, result Expression, err error) {
	for {
		tok, ok := p.current()
		if !ok {
			return nil, nil, fmt.Errorf("unexpected end of input, want %s", CloseBrace)
		}
		if tok.Kind == CloseBrace {
			return members, result, nil
		}
		if startsMember(tok.Kind) || p.startsSatisfyNot() {
			member, err := p.parseMember()
			if err != nil {
				return nil, nil, err
			}
			members = append(members, member)
			continue
		}

		result, err = p.parseExpression()
		if err != nil {
			return nil, nil, err
		}
		return members, result, nil
	}
}

// startsMember reports whether kind can begin a member: a visibility
// prefix, 'package', 'import', any definition/usage keyword, or a Satisfy
// statement's 'assert'/'satisfy' prefixes. Deliberately doesn't cover
// Satisfy's third, bare-'not' prefix -- see startsSatisfyNot, which needs a
// lookahead this single-token predicate can't do, to tell it apart from a
// unary-'not' expression (e.g. a calc body's trailing "not done" result).
func startsMember(kind Kind) bool {
	if kind == Pkg || kind == ImportKw || kind == AssertKw || kind == SatisfyKw {
		return true
	}
	if _, ok := visibilityKeywords[kind]; ok {
		return true
	}
	_, ok := defKeywords[kind]
	return ok
}

// parseFeatureSpecializationPart consumes a typing (": Type"), a
// multiplicity ("[...]"), a subsetting ("subsets X" or ":> X"), a
// redefinition ("redefines X" or ":>> X"), and a reference ("references X"
// or "::> X") in whichever order they appear, each at most once, and all
// absent -- so "part b[0..2] : P;", "part c : P[2..*];", "port p;",
// "part b subsets a;", "part B_b :>> B::b;", and "attribute ::> m = x;"
// all parse through the same loop.
func (p *parser) parseFeatureSpecializationPart(usage *Usage) error {
	for {
		tok, ok := p.current()
		if !ok {
			return nil
		}

		switch tok.Kind {
		case Colon:
			if usage.Type != "" {
				return fmt.Errorf("line %d: unexpected %s, a type was already given", tok.Pos.Line, Colon)
			}
			p.pos++
			typ, err := p.parseQualifiedName()
			if err != nil {
				return err
			}
			usage.Type = typ

		case OpenBracket:
			if usage.Multiplicity != nil {
				return fmt.Errorf("line %d: unexpected %s, a multiplicity was already given", tok.Pos.Line, OpenBracket)
			}
			mult, err := p.parseMultiplicity()
			if err != nil {
				return err
			}
			usage.Multiplicity = mult

		case Subsets:
			if usage.Subsets != "" {
				return fmt.Errorf("line %d: unexpected %s, a subsetted feature was already given", tok.Pos.Line, Subsets)
			}
			p.pos++
			target, err := p.parseQualifiedName()
			if err != nil {
				return err
			}
			usage.Subsets = target

		case Redefines:
			if usage.Redefines != "" {
				return fmt.Errorf("line %d: unexpected %s, a redefined feature was already given", tok.Pos.Line, Redefines)
			}
			p.pos++
			target, err := p.parseQualifiedName()
			if err != nil {
				return err
			}
			usage.Redefines = target

		case References:
			if usage.References != "" {
				return fmt.Errorf("line %d: unexpected %s, a referenced feature was already given", tok.Pos.Line, References)
			}
			p.pos++
			target, err := p.parseQualifiedName()
			if err != nil {
				return err
			}
			usage.References = target

		default:
			return nil
		}
	}
}

// parseValuePart consumes an optional assigned value, e.g. the "= 5" in
// `attribute n : ScalarValues::Integer = 5;`. Only SysML's plain '='
// FeatureValue form is supported (not ':=' or 'default'). Consumes nothing
// if the next token isn't '='.
func (p *parser) parseValuePart(usage *Usage) error {
	tok, ok := p.current()
	if !ok || tok.Kind != Equals {
		return nil
	}
	p.pos++

	val, err := p.parseExpression()
	if err != nil {
		return err
	}
	usage.Value = val
	return nil
}

// binaryOperators maps each binary-operator token to the operator symbol a
// BinaryExpr.Op carries, and, via its presence as a map key, which tokens
// parseBinaryExpression treats as binary operators at all.
var binaryOperators = map[Kind]string{
	Pipe:      "|",
	XorKw:     "xor",
	Ampersand: "&",
	Eq:        "==",
	NotEq:     "!=",
	Lt:        "<",
	Gt:        ">",
	Le:        "<=",
	Ge:        ">=",
	Plus:      "+",
	Minus:     "-",
	Star:      "*",
	Slash:     "/",
	Percent:   "%",
	Power:     "**",
}

// unaryOperators is binaryOperators' counterpart for parseUnaryExpression's
// leading prefix -- "+"/"-" reuse the same operator symbols as their
// binary forms (see UnaryExpr's doc comment on why that's fine here).
var unaryOperators = map[Kind]string{
	Plus:  "+",
	Minus: "-",
	NotKw: "not",
}

// operatorPrecedence gives each binary operator its precedence, higher
// binding tighter; ties resolve left-to-right except "**", which is
// right-associative (2 ** 3 ** 2 is 2 ** (3 ** 2)). The OMG grammar's own
// BNF doesn't encode precedence (BinaryOperator lists every operator in
// one flat alternation -- precedence is specified separately, outside the
// grammar itself), so this table is this project's own documented,
// best-effort choice, following ordinary arithmetic/logical convention.
var operatorPrecedence = map[string]int{
	"|":   1,
	"xor": 1,
	"&":   2,
	"==":  3,
	"!=":  3,
	"<":   4,
	">":   4,
	"<=":  4,
	">=":  4,
	"+":   5,
	"-":   5,
	"*":   6,
	"/":   6,
	"%":   6,
	"**":  7,
}

// parseExpression parses an expression: the entry point for a usage's
// assigned value (and, later, a constraint/calculation body's result
// expression). See the Expression doc comment for exactly which shapes of
// expression this project supports.
func (p *parser) parseExpression() (Expression, error) {
	return p.parseBinaryExpression(0)
}

// parseBinaryExpression implements precedence climbing: parses a unary
// expression for its left-hand side, then repeatedly consumes any binary
// operator that binds at least as tightly as minPrec and a right-hand side
// parsed at one precedence tighter (or, for "**", at the same precedence,
// giving it right-associativity) -- so "1 + 2 * 3" parses as "1 + (2 * 3)"
// and "2 ** 3 ** 2" as "2 ** (3 ** 2)".
func (p *parser) parseBinaryExpression(minPrec int) (Expression, error) {
	left, err := p.parseUnaryExpression()
	if err != nil {
		return nil, err
	}

	for {
		tok, ok := p.current()
		if !ok {
			return left, nil
		}
		op, ok := binaryOperators[tok.Kind]
		if !ok {
			return left, nil
		}
		prec := operatorPrecedence[op]
		if prec < minPrec {
			return left, nil
		}
		p.pos++

		nextMin := prec + 1
		if op == "**" {
			nextMin = prec
		}
		right, err := p.parseBinaryExpression(nextMin)
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Op: op, Left: left, Right: right}
	}
}

// parseUnaryExpression parses an optional leading "+"/"-"/"not", which may
// itself stack (e.g. "- -x" or "not not done"), then a primary expression.
func (p *parser) parseUnaryExpression() (Expression, error) {
	if tok, ok := p.current(); ok {
		if op, ok := unaryOperators[tok.Kind]; ok {
			p.pos++
			operand, err := p.parseUnaryExpression()
			if err != nil {
				return nil, err
			}
			return &UnaryExpr{Op: op, Operand: operand}, nil
		}
	}
	return p.parsePrimary()
}

// parsePrimary parses a boolean/string/numeric literal, a qualified-name
// reference, or a parenthesized expression. An Identifier token is
// disambiguated the same way parseBound already disambiguates a
// multiplicity bound: try an integer, then a real number, then fall back
// to a qualified name.
func (p *parser) parsePrimary() (Expression, error) {
	tok, ok := p.current()
	if !ok {
		return nil, fmt.Errorf("unexpected end of input, want an expression")
	}

	switch tok.Kind {
	case TrueKw:
		p.pos++
		return &BoolLiteral{Value: true}, nil

	case FalseKw:
		p.pos++
		return &BoolLiteral{Value: false}, nil

	case StringLit:
		p.pos++
		return &StringLiteral{Value: unquoteString(tok.Value)}, nil

	case OpenParen:
		p.pos++
		inner, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(CloseParen); err != nil {
			return nil, err
		}
		return inner, nil

	case Identifier:
		if n, err := strconv.Atoi(string(tok.Value)); err == nil {
			p.pos++
			return &IntLiteral{Value: n}, nil
		}
		if f, err := strconv.ParseFloat(string(tok.Value), 64); err == nil {
			p.pos++
			return &RealLiteral{Value: f}, nil
		}
		path, err := p.parseQualifiedName()
		if err != nil {
			return nil, err
		}
		if tok, ok := p.current(); ok && tok.Kind == OpenParen {
			return p.parseInvocationArgs(path)
		}
		return p.parseFeatureChainTail(path)

	default:
		return nil, fmt.Errorf("line %d: unexpected %s, want an expression", tok.Pos.Line, describeToken(tok))
	}
}

// parseFeatureChainTail continues parsing a primary expression begun as a
// qualified name (first) into a FeatureChain if a '.'-separated segment
// follows, e.g. the ".chassis.mass" in "vehicle.chassis.mass" -- returning
// a plain NameRef unchanged if no '.' follows at all.
func (p *parser) parseFeatureChainTail(first string) (Expression, error) {
	if tok, ok := p.current(); !ok || tok.Kind != Dot {
		return &NameRef{Path: first}, nil
	}

	path := []string{first}
	for {
		tok, ok := p.current()
		if !ok || tok.Kind != Dot {
			break
		}
		p.pos++
		seg, err := p.expect(Identifier)
		if err != nil {
			return nil, err
		}
		path = append(path, string(seg.Value))
	}
	return &FeatureChain{Path: path}, nil
}

// parseInvocationArgs parses an InvocationExpr's parenthesized,
// comma-separated argument list, e.g. the "(componentMasses)" in
// "sum(componentMasses)" -- callee is the qualified name already parsed
// ahead of the OpenParen the caller has confirmed is next. An empty
// argument list ("()") is fine, matching zero-argument invocations.
func (p *parser) parseInvocationArgs(callee string) (Expression, error) {
	if _, err := p.expect(OpenParen); err != nil {
		return nil, err
	}

	inv := &InvocationExpr{Callee: callee}
	if tok, ok := p.current(); ok && tok.Kind == CloseParen {
		p.pos++
		return inv, nil
	}

	for {
		arg, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		inv.Args = append(inv.Args, arg)

		tok, ok := p.current()
		if !ok {
			return nil, fmt.Errorf("unexpected end of input, want %s or %s", Comma, CloseParen)
		}
		if tok.Kind == CloseParen {
			p.pos++
			return inv, nil
		}
		if _, err := p.expect(Comma); err != nil {
			return nil, err
		}
	}
}

// unquoteString strips a StringLit token's surrounding quotes. value
// always starts with '"' (see scanString); it may be missing the closing
// one if the source was unterminated, in which case there's nothing to
// strip off the end.
func unquoteString(value []rune) string {
	s := string(value[1:])
	if len(s) > 0 && s[len(s)-1] == '"' {
		s = s[:len(s)-1]
	}
	return s
}

// parseMultiplicity parses a bracketed multiplicity clause: "[*]"
// (0..Unbounded), "[3]" (Lower == Upper == 3), or a "lower..upper" range
// where either side may be "*" or a name (see Bound's doc comment).
func (p *parser) parseMultiplicity() (*Multiplicity, error) {
	if _, err := p.expect(OpenBracket); err != nil {
		return nil, err
	}

	if tok, ok := p.current(); ok && tok.Kind == Star {
		p.pos++
		if _, err := p.expect(CloseBracket); err != nil {
			return nil, err
		}
		return &Multiplicity{Lower: Bound{Value: 0}, Upper: Bound{Value: Unbounded}}, nil
	}

	lower, err := p.parseBound()
	if err != nil {
		return nil, err
	}

	upper := lower
	if tok, ok := p.current(); ok && tok.Kind == DotDot {
		p.pos++
		if upper, err = p.parseBound(); err != nil {
			return nil, err
		}
	}

	if _, err := p.expect(CloseBracket); err != nil {
		return nil, err
	}
	return &Multiplicity{Lower: lower, Upper: upper}, nil
}

// parseBound parses one side of a multiplicity range: "*" (Unbounded), a
// non-negative integer literal, or -- anything else identifier-shaped -- a
// name reference, left unresolved (see Bound's doc comment).
func (p *parser) parseBound() (Bound, error) {
	tok, ok := p.current()
	if !ok {
		return Bound{}, fmt.Errorf("unexpected end of input in multiplicity, want a bound or %s", Star)
	}

	if tok.Kind == Star {
		p.pos++
		return Bound{Value: Unbounded}, nil
	}

	if tok.Kind != Identifier {
		return Bound{}, fmt.Errorf("line %d: unexpected %s, want a multiplicity bound", tok.Pos.Line, describeToken(tok))
	}
	p.pos++

	if n, err := strconv.Atoi(string(tok.Value)); err == nil {
		if n < 0 {
			return Bound{}, fmt.Errorf("line %d: invalid multiplicity bound %q", tok.Pos.Line, string(tok.Value))
		}
		return Bound{Value: n}, nil
	}

	return Bound{Name: string(tok.Value)}, nil
}

// parseQualifiedName parses an identifier, optionally followed by more
// '::'-separated identifiers (e.g. "Engine" or "Vehicle::Car::Wheel"),
// returning it as a single "::"-joined string.
func (p *parser) parseQualifiedName() (string, error) {
	first, err := p.expect(Identifier)
	if err != nil {
		return "", err
	}

	name := string(first.Value)
	for {
		tok, ok := p.current()
		if !ok || tok.Kind != PathSep {
			return name, nil
		}
		p.pos++

		next, err := p.expect(Identifier)
		if err != nil {
			return "", err
		}
		name += "::" + string(next.Value)
	}
}
