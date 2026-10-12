// SPDX-License-Identifier: EPL-2.0

// Package grammar loads the Xtext grammars the OMG pilot implementation uses
// for SysML v2 and KerML into a small expression tree that the syntax package
// interprets.
//
// The grammars are vendored unmodified under xtext/ (see UPSTREAM and
// tools/vendor-upstream.sh). The SysML and KerML languages each have their own
// grammar file and both build on KerMLExpressions.xtext; a rule defined in
// the language file replaces one of the same name in the expressions file.
package grammar

import (
	"embed"
	"fmt"
	"regexp"
	"sort"
	"sync"
)

//go:embed xtext/*.xtext
var files embed.FS

// Language selects a grammar.
type Language int

const (
	SysML Language = iota
	KerML
)

func (l Language) String() string {
	if l == KerML {
		return "KerML"
	}
	return "SysML"
}

// Kind is the kind of a grammar expression.
type Kind uint8

const (
	Alt    Kind = iota + 1 // Kids are the alternatives
	Seq                    // Kids are matched in order
	Rep                    // Op is "?", "*" or "+"; Kids[0] is repeated
	Term                   // Text is a keyword or symbol
	Ref                    // Text is the name of a rule
	XRef                   // a cross-reference [Text|Op]; Op is the rule that parses the name
	Assign                 // Text is the property, Op is "=", "+=" or "?="; Kids[0] is the value
	Action                 // {Text} or {Text.Prop Op current}
	Pred                   // a syntactic predicate (=> or ->); Text is the operator, Kids[0] the element
	Wild                   // any token
)

// Expr is a node of a rule body.
type Expr struct {
	Kind Kind
	Text string
	Op   string
	Prop string
	Kids []*Expr
}

// RuleKind distinguishes the kinds of Xtext rule.
type RuleKind uint8

const (
	// Normal rules create a node of their metaclass.
	Normal RuleKind = iota
	// Fragment rules contribute to the node of the rule that calls them.
	Fragment
	// Terminal rules are lexical. Their bodies are not interpreted; the
	// syntax package's lexer implements them.
	Terminal
	// Enum rules map keywords to enumeration literals.
	Enum
)

// Rule is one rule of a grammar.
type Rule struct {
	Name    string
	Returns string // the declared metaclass or datatype, with its namespace, or ""
	Kind    RuleKind
	Body    *Expr // nil for terminal rules
	File    string
	Line    int
}

// Grammar is a language's rules with the lexical information derived from
// them.
type Grammar struct {
	Language Language
	Rules    map[string]*Rule
	Order    []string // rule names in definition order, base grammar first

	// Keywords are the identifier-shaped literals used in the rules. As in
	// Xtext, they are reserved: an identifier with one of these spellings is a
	// keyword, never a name.
	Keywords map[string]bool
	// Symbols are the other literals, sorted.
	Symbols []string

	// Root is the rule a document is parsed with.
	Root string
}

// Walk calls f for e and every expression below it.
func Walk(e *Expr, f func(*Expr)) {
	if e == nil {
		return
	}
	f(e)
	for _, k := range e.Kids {
		Walk(k, f)
	}
}

var (
	once    [2]sync.Once
	loaded  [2]*Grammar
	loadErr [2]error
)

// Load returns the grammar for a language. The result is shared and must not
// be modified.
func Load(l Language) (*Grammar, error) {
	once[l].Do(func() { loaded[l], loadErr[l] = build(l) })
	return loaded[l], loadErr[l]
}

func build(l Language) (*Grammar, error) {
	names := []string{"KerMLExpressions.xtext", "SysML.xtext"}
	if l == KerML {
		names = []string{"KerMLExpressions.xtext", "KerML.xtext"}
	}
	g := &Grammar{Language: l, Rules: map[string]*Rule{}, Keywords: map[string]bool{}, Root: "RootNamespace"}
	for _, n := range names {
		src, err := files.ReadFile("xtext/" + n)
		if err != nil {
			return nil, err
		}
		f, err := ParseFile(n, string(src))
		if err != nil {
			return nil, err
		}
		for _, r := range f.Rules {
			if _, dup := g.Rules[r.Name]; !dup {
				g.Order = append(g.Order, r.Name)
			}
			g.Rules[r.Name] = r
		}
	}
	if _, ok := g.Rules[g.Root]; !ok {
		return nil, fmt.Errorf("grammar %v has no %s rule", l, g.Root)
	}

	ident := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	symbols := map[string]bool{}
	for _, name := range g.Order {
		Walk(g.Rules[name].Body, func(e *Expr) {
			if e.Kind != Term || e.Text == "" {
				return
			}
			switch {
			case ident.MatchString(e.Text):
				g.Keywords[e.Text] = true
			case e.Text[0] < '0' || e.Text[0] > '9':
				symbols[e.Text] = true
			}
		})
	}
	for s := range symbols {
		g.Symbols = append(g.Symbols, s)
	}
	sort.Strings(g.Symbols)
	return g, nil
}

// builtinTerminals are the rules Xtext's common Terminals grammar provides.
var builtinTerminals = map[string]bool{
	"ID": true, "STRING": true, "INT": true, "WS": true,
	"ML_COMMENT": true, "SL_COMMENT": true, "ANY_OTHER": true, "EOF": true,
}

// Undefined lists the rule names the grammar refers to but does not define,
// excluding Xtext's built-in terminals.
func (g *Grammar) Undefined() []string {
	missing := map[string]bool{}
	for _, name := range g.Order {
		Walk(g.Rules[name].Body, func(e *Expr) {
			ref := ""
			switch e.Kind {
			case Ref:
				ref = e.Text
			case XRef:
				ref = e.Op
			}
			if ref != "" {
				if _, ok := g.Rules[ref]; !ok && !builtinTerminals[ref] {
					missing[ref] = true
				}
			}
		})
	}
	var out []string
	for m := range missing {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}
