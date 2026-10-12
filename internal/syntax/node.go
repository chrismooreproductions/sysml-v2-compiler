// SPDX-License-Identifier: EPL-2.0

package syntax

import (
	"fmt"
	"strings"
)

// Value is the value of a property: a *Node, Text, Bool or Ref.
type Value interface{ isValue() }

// Node is an element of the parse tree: an instance of a metaclass with its
// properties in the order the grammar assigned them, and the source text it
// came from.
type Node struct {
	Class string // the metaclass name, without namespace
	Span  Span
	Props []Prop
}

// Prop is one assignment to a property.
type Prop struct {
	Name  string
	Op    string // "=", "+=" or "?="
	Value Value
}

// Text is a string value: a keyword or symbol, an enumeration literal's name,
// or the text of a name, number or string as written.
type Text string

// Bool is a boolean value.
type Bool bool

// Ref is a cross-reference: a name written in the source that a later stage
// resolves to an element.
type Ref struct {
	Name string // as written, with quotes and '::' separators
	Span Span
}

func (*Node) isValue() {}
func (Text) isValue()  {}
func (Bool) isValue()  {}
func (Ref) isValue()   {}

// Get returns the first value assigned to a property.
func (n *Node) Get(name string) (Value, bool) {
	for _, p := range n.Props {
		if p.Name == name {
			return p.Value, true
		}
	}
	return nil, false
}

// All returns every value assigned to a property, in order.
func (n *Node) All(name string) []Value {
	var out []Value
	for _, p := range n.Props {
		if p.Name == name {
			out = append(out, p.Value)
		}
	}
	return out
}

// Dump renders a tree compactly, for tests and debugging:
//
//	Package(declaredName="P" ownedRelationship+=OwningMembership(...))
func Dump(n *Node) string {
	var sb strings.Builder
	dump(&sb, n)
	return sb.String()
}

func dump(sb *strings.Builder, n *Node) {
	sb.WriteString(n.Class)
	sb.WriteByte('(')
	for i, p := range n.Props {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(p.Name)
		sb.WriteString(p.Op)
		switch v := p.Value.(type) {
		case *Node:
			dump(sb, v)
		case Text:
			fmt.Fprintf(sb, "%q", string(v))
		case Bool:
			fmt.Fprintf(sb, "%t", bool(v))
		case Ref:
			sb.WriteString("@" + v.Name)
		}
	}
	sb.WriteByte(')')
}
