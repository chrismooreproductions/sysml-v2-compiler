// SPDX-License-Identifier: EPL-2.0

// Package model is the element graph that the stages after parsing work on:
// the syntax tree with owners, resolved cross-references, and a mark on the
// elements that rules (not the source text) created.
package model

import (
	"fmt"
	"strings"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/schema"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/syntax"
)

// Element is an instance of a metaclass.
type Element struct {
	Class string
	// Span is the source text the element came from; the zero Span for an
	// element a rule generated.
	Span syntax.Span
	// Generated marks an element created by a rule rather than written in the
	// source.
	Generated bool
	// Owner is the element that contains this one, nil for the root.
	Owner *Element
	Props []Prop
}

// Prop is one property assignment.
type Prop struct {
	Name  string
	Op    string // "=", "+=" or "?="
	Value Value
}

// Value is the value of a property: an *Element (containment), Text, Bool, or a
// *Ref.
type Value interface{ isValue() }

// Text is a string value: a keyword, an enumeration literal's name, or the
// text of a name, number or string as written.
type Text string

// Bool is a boolean value.
type Bool bool

// Ref is a cross-reference: a name that is resolved to an element, or a
// reference a rule set directly.
type Ref struct {
	Name   string      // as written, or "" for a reference a rule created
	Span   syntax.Span // where the name was written
	Target *Element    // the element it refers to, nil until resolved
}

func (*Element) isValue() {}
func (Text) isValue()     {}
func (Bool) isValue()     {}
func (*Ref) isValue()     {}

// FromSyntax builds the element graph for a parse tree.
func FromSyntax(n *syntax.Node) *Element { return fromSyntax(n, nil) }

func fromSyntax(n *syntax.Node, owner *Element) *Element {
	e := &Element{Class: n.Class, Span: n.Span, Owner: owner}
	for _, p := range n.Props {
		switch v := p.Value.(type) {
		case *syntax.Node:
			e.Props = append(e.Props, Prop{p.Name, p.Op, fromSyntax(v, e)})
		case syntax.Text:
			e.Props = append(e.Props, Prop{p.Name, p.Op, Text(v)})
		case syntax.Bool:
			e.Props = append(e.Props, Prop{p.Name, p.Op, Bool(v)})
		case syntax.Ref:
			e.Props = append(e.Props, Prop{p.Name, p.Op, &Ref{Name: v.Name, Span: v.Span}})
		}
	}
	return e
}

// IsA reports whether the element's metaclass is class or a subtype of it.
func (e *Element) IsA(class string) bool { return e != nil && schema.IsKindOf(e.Class, class) }

// Add appends a property. An element value becomes owned by e.
func (e *Element) Add(name, op string, v Value) {
	if c, ok := v.(*Element); ok {
		c.Owner = e
	}
	e.Props = append(e.Props, Prop{name, op, v})
}

// Has reports whether the property has been assigned.
func (e *Element) Has(name string) bool {
	for _, p := range e.Props {
		if p.Name == name {
			return true
		}
	}
	return false
}

// Get returns the first value assigned to a property.
func (e *Element) Get(name string) (Value, bool) {
	for _, p := range e.Props {
		if p.Name == name {
			return p.Value, true
		}
	}
	return nil, false
}

// Text returns a Text or Bool property as a string ("true" for a true Bool), or
// "" if it is not set.
func (e *Element) Text(name string) string {
	for _, p := range e.Props {
		if p.Name != name {
			continue
		}
		switch v := p.Value.(type) {
		case Text:
			return string(v)
		case Bool:
			if v {
				return "true"
			}
			return "false"
		}
	}
	return ""
}

// IsTrue reports whether a boolean property is set to true.
func (e *Element) IsTrue(name string) bool { return e.Text(name) == "true" }

// Set replaces the first value of a property, or appends it.
func (e *Element) Set(name string, v Value) {
	for i, p := range e.Props {
		if p.Name == name {
			e.Props[i].Value = v
			return
		}
	}
	e.Add(name, "=", v)
}

// Children returns the owned elements assigned to a property, in order.
func (e *Element) Children(name string) []*Element {
	var out []*Element
	for _, p := range e.Props {
		if c, ok := p.Value.(*Element); ok && p.Name == name {
			out = append(out, c)
		}
	}
	return out
}

// Child returns the first owned element assigned to a property, or nil.
func (e *Element) Child(name string) *Element {
	for _, p := range e.Props {
		if c, ok := p.Value.(*Element); ok && p.Name == name {
			return c
		}
	}
	return nil
}

// Refs returns the cross-references assigned to a property.
func (e *Element) Refs(name string) []*Ref {
	var out []*Ref
	for _, p := range e.Props {
		if r, ok := p.Value.(*Ref); ok && p.Name == name {
			out = append(out, r)
		}
	}
	return out
}

// Walk calls f for e and every element it owns, parents first.
func (e *Element) Walk(f func(*Element)) {
	f(e)
	for _, p := range e.Props {
		if c, ok := p.Value.(*Element); ok {
			c.Walk(f)
		}
	}
}

// Dump renders an element compactly for tests and debugging.
func Dump(e *Element) string {
	var sb strings.Builder
	dump(&sb, e)
	return sb.String()
}

func dump(sb *strings.Builder, e *Element) {
	sb.WriteString(e.Class)
	if e.Generated {
		sb.WriteByte('*')
	}
	sb.WriteByte('(')
	for i, p := range e.Props {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(p.Name + p.Op)
		switch v := p.Value.(type) {
		case *Element:
			dump(sb, v)
		case Text:
			fmt.Fprintf(sb, "%q", string(v))
		case Bool:
			fmt.Fprintf(sb, "%t", bool(v))
		case *Ref:
			switch {
			case v.Name != "":
				sb.WriteString("@" + v.Name)
			case v.Target != nil:
				sb.WriteString("->" + v.Target.Class)
			default:
				sb.WriteString("@?")
			}
		}
	}
	sb.WriteByte(')')
}
