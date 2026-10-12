// SPDX-License-Identifier: EPL-2.0

package conformance

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/schema"
)

const (
	xmiNS = "http://www.omg.org/XMI"
	xsiNS = "http://www.w3.org/2001/XMLSchema-instance"
)

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// FileBase names a library file the way references to it do: no directory, no
// extension, URL escapes removed.
func FileBase(p string) string {
	p = p[strings.LastIndex(p, "/")+1:]
	p = strings.ReplaceAll(p, "%20", " ")
	if i := strings.LastIndex(p, "."); i >= 0 {
		p = p[:i]
	}
	return p
}

// LoadXMI reads one of the pilot implementation's serialized models into
// canonical form. A reference to an element in the same file is an attribute
// holding its identifier; to another file, a child element with an href.
func LoadXMI(path string) (*Elem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := xml.NewDecoder(f)
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	base := FileBase(path)

	var stack []*Elem // nil entries stand for reference elements
	var root *Elem
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		switch e := tok.(type) {
		case xml.StartElement:
			var typ, href, id string
			attrs := map[string]string{}
			var refs [][2]string // role, target id
			for _, a := range e.Attr {
				switch {
				case a.Name.Space == "xmlns" || a.Name.Local == "xmlns":
				case a.Name.Space == xmiNS && a.Name.Local == "id":
					id = a.Value
				case a.Name.Space == xmiNS:
				case a.Name.Space == xsiNS && a.Name.Local == "type":
					typ = a.Value
				case a.Name.Local == "href":
					href = a.Value
				case a.Name.Local == "elementId":
				}
			}
			if i := strings.Index(typ, ":"); i >= 0 {
				typ = typ[i+1:]
			}
			class := typ
			if len(stack) == 0 {
				class = e.Name.Local
			}
			for _, a := range e.Attr {
				if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" || a.Name.Space == xmiNS ||
					(a.Name.Space == xsiNS && a.Name.Local == "type") || a.Name.Local == "href" || a.Name.Local == "elementId" {
					continue
				}
				first := strings.Fields(a.Value + " x")[0]
				if uuidRe.MatchString(first) {
					// an identifier: a reference if the metamodel says the feature is one
					if f, _, ok := schema.FeatureOf(class, a.Name.Local); ok && !f.Attribute {
						for _, ref := range strings.Fields(a.Value) {
							refs = append(refs, [2]string{a.Name.Local, ref})
						}
					}
					continue
				}
				attrs[a.Name.Local] = normScalar(a.Name.Local, a.Value)
			}

			var parent *Elem
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			if href != "" { // an element in another file
				file, target := base, ""
				if i := strings.Index(href, "#"); i >= 0 {
					target = href[i+1:]
					if i > 0 {
						file = FileBase(href[:i])
					}
				}
				if parent != nil {
					parent.Kids = append(parent.Kids, Kid{Role: e.Name.Local, TgtFile: file, TgtID: target})
				}
				stack = append(stack, nil)
				continue
			}
			c := &Elem{ID: id, Class: class, Attrs: attrs, Parent: parent, File: base}
			for _, r := range refs {
				// A reference to the owner from a source end is the inverse of
				// containment: not a reference written in the source.
				if parent != nil && schema.IsSourceEnd(class, r[0]) && r[1] == parentID(stack) {
					continue
				}
				c.Kids = append(c.Kids, Kid{Role: r[0], TgtFile: base, TgtID: r[1]})
			}
			if parent != nil {
				parent.Kids = append(parent.Kids, Kid{Role: e.Name.Local, Elem: c})
			} else {
				root = c
			}
			stack = append(stack, c)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	if root == nil {
		return nil, fmt.Errorf("%s: empty document", path)
	}
	return root, nil
}

// parentID is the identifier of the element on top of the stack.
func parentID(stack []*Elem) string {
	if len(stack) == 0 || stack[len(stack)-1] == nil {
		return ""
	}
	return stack[len(stack)-1].ID
}
