// SPDX-License-Identifier: EPL-2.0

// Package conformance compares what this project produces for the standard
// library with the pilot implementation's own serialization of it (the XMI
// files of the OMG release). Both sides are reduced to the same canonical
// form first, so what is left is a difference in substance.
package conformance

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/schema"
)

// Elem is an element in canonical form.
type Elem struct {
	ID     string // the oracle's identifier, if it came from the oracle
	Class  string
	Attrs  map[string]string
	Kids   []Kid
	Parent *Elem
	File   string
	Key    string // the position in the file: role[index] path, with the file name
}

// Kid is a child of an element: an owned element, or a cross-reference.
type Kid struct {
	Role string
	Elem *Elem // an owned element; nil for a reference

	// A reference on our side.
	Name   string // as written, or "" for one a rule set
	Target *Elem  // where it points, if known
	// A reference on the oracle's side.
	TgtFile, TgtID string
}

var (
	starRe  = regexp.MustCompile(`(?m)^\s*\*+`)
	spaceRe = regexp.MustCompile(`\s+`)
)

// normScalar removes representation differences that carry no meaning: the
// quoting of names and strings, and the comment delimiters around doc text.
func normScalar(key, v string) string {
	if key == "body" {
		v = strings.TrimPrefix(v, "/*")
		v = strings.TrimSuffix(v, "*/")
		v = starRe.ReplaceAllString(v, "")
		return strings.TrimSpace(spaceRe.ReplaceAllString(v, " "))
	}
	if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
		return v[1 : len(v)-1]
	}
	return v
}

// sameScalar compares attribute values, numbers numerically.
func sameScalar(a, b string) bool {
	if a == b {
		return true
	}
	x, e1 := strconv.ParseFloat(a, 64)
	y, e2 := strconv.ParseFloat(b, 64)
	return e1 == nil && e2 == nil && x == y
}

// Canonicalizer reduces element graphs to canonical form. A reference in one
// file can point into another, so all the files of a workspace share one.
type Canonicalizer struct {
	canon map[*model.Element]*Elem
	roots []*model.Element
	outs  []*Elem
}

// NewCanonicalizer returns an empty canonicalizer.
func NewCanonicalizer() *Canonicalizer {
	return &Canonicalizer{canon: map[*model.Element]*Elem{}}
}

// Add reduces a root to canonical form. Reference targets are filled in by Link.
func (z *Canonicalizer) Add(root *model.Element, file string) *Elem {
	var build func(e *model.Element, parent *Elem) *Elem
	build = func(e *model.Element, parent *Elem) *Elem {
		c := &Elem{Class: e.Class, Attrs: map[string]string{}, Parent: parent, File: file}
		z.canon[e] = c
		for _, p := range e.Props {
			switch v := p.Value.(type) {
			case *model.Element:
				c.Kids = append(c.Kids, Kid{Role: p.Name, Elem: build(v, c)})
			case *model.Ref:
				c.Kids = append(c.Kids, Kid{Role: p.Name, Name: v.Name})
			case model.Bool:
				c.Attrs[p.Name] = strconv.FormatBool(bool(v))
			case model.Text:
				val := normScalar(p.Name, string(v))
				if old, ok := c.Attrs[p.Name]; ok {
					val = old + " " + val
				}
				c.Attrs[p.Name] = val
			}
		}
		return c
	}
	out := build(root, nil)
	z.roots = append(z.roots, root)
	z.outs = append(z.outs, out)
	return out
}

// Link sets the targets of the references of every root added so far.
func (z *Canonicalizer) Link() {
	var link func(e *model.Element)
	link = func(e *model.Element) {
		c := z.canon[e]
		i := 0
		for _, p := range e.Props {
			switch v := p.Value.(type) {
			case *model.Element:
				link(v)
				i++
			case *model.Ref:
				for c.Kids[i].Elem != nil {
					i++
				}
				c.Kids[i].Target = z.canon[v.Target]
				i++
			}
		}
	}
	for _, r := range z.roots {
		link(r)
	}
}

// FromModel reduces one element graph to canonical form.
func FromModel(root *model.Element, file string) *Elem {
	z := NewCanonicalizer()
	out := z.Add(root, file)
	z.Link()
	return out
}

// Prune drops what the oracle never serializes or serializes as a default:
// derived and redefined features, attributes equal to their default, and the
// references that are only the inverse of containment (a relationship's source
// end pointing at its owner).
func Prune(c *Elem) {
	for k, v := range c.Attrs {
		f, _, ok := schema.FeatureOf(c.Class, k)
		if !ok {
			continue
		}
		if schema.IsDerived(c.Class, k) {
			delete(c.Attrs, k)
			continue
		}
		def := f.Default
		if def == "" {
			switch f.Type {
			case "EBoolean":
				def = "false"
			case "EInt", "ELong", "EDouble":
				def = "0"
			}
		}
		if def != "" && sameScalar(v, def) {
			delete(c.Attrs, k)
		}
	}
	var kept []Kid
	for _, k := range c.Kids {
		if _, _, ok := schema.FeatureOf(c.Class, k.Role); ok && schema.IsDerived(c.Class, k.Role) {
			continue
		}
		if k.Elem == nil && schema.IsSourceEnd(c.Class, k.Role) && k.Target != nil && k.Target == c.Parent {
			continue // the inverse of containment, not written in the source
		}
		if k.Elem != nil {
			Prune(k.Elem)
		}
		kept = append(kept, k)
	}
	c.Kids = kept
}

// AssignKeys gives every element a key made of its file and its position:
// the role and index among the owned elements with that role, from the root.
func AssignKeys(c *Elem, file, path string, parent *Elem, byKey map[string]*Elem) {
	c.File, c.Key, c.Parent = file, file+"|"+path, parent
	byKey[c.Key] = c
	counts := map[string]int{}
	for _, k := range c.Kids {
		if k.Elem == nil {
			continue
		}
		idx := counts[k.Role]
		counts[k.Role]++
		AssignKeys(k.Elem, file, fmt.Sprintf("%s/%s[%d]", path, k.Role, idx), c, byKey)
	}
}
