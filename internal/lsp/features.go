// SPDX-License-Identifier: EPL-2.0

package lsp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/chrismooreproductions/sysml-v2-compiler/sysml"
)

// withPosition parses a position request, brings the workspace up to date and
// calls fn with the document and the byte offset the position is at.
func (s *Server) withPosition(raw json.RawMessage, fn func(d *document, offset int) (any, error)) (any, error) {
	var p positionParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ws == nil {
		return nil, nil
	}
	s.sync()
	d := s.docs[uriToPath(p.TextDocument.URI)]
	if d == nil {
		return nil, nil
	}
	return fn(d, d.offset(p.Position))
}

// root is the tree of a document in the workspace.
func (s *Server) root(d *document) *sysml.Element {
	for _, doc := range s.ws.Documents() {
		if doc.Name == d.path {
			return doc.Root
		}
	}
	return nil
}

func contains(sp sysml.Span, offset int) bool {
	return sp.Start.Offset <= offset && offset <= sp.End.Offset
}

// refAt finds the reference written at an offset: the narrowest if several
// overlap.
func refAt(root *sysml.Element, offset int) *sysml.Ref {
	var best *sysml.Ref
	root.Walk(func(e *sysml.Element) {
		if !e.Generated && !contains(e.Span, offset) {
			return
		}
		for _, p := range e.Props {
			if r, ok := p.Value.(*sysml.Ref); ok && r.Name != "" && contains(r.Span, offset) {
				if best == nil || r.Span.End.Offset-r.Span.Start.Offset < best.Span.End.Offset-best.Span.Start.Offset {
					best = r
				}
			}
		}
	})
	return best
}

// declAt finds the named element whose declaration is at an offset: the
// innermost element that is on the line the offset is on and starts there.
func (s *Server) declAt(d *document, root *sysml.Element, offset int) *sysml.Element {
	var best *sysml.Element
	line := d.position(offset).Line
	root.Walk(func(e *sysml.Element) {
		if e.Generated || s.ws.EffectiveName(e) == "" || !contains(e.Span, offset) {
			return
		}
		if d.position(e.Span.Start.Offset).Line != line {
			return
		}
		if best == nil || e.Span.End.Offset-e.Span.Start.Offset <= best.Span.End.Offset-best.Span.Start.Offset {
			best = e
		}
	})
	return best
}

// target is the element a position names: the target of a reference, or the
// element declared there.
func (s *Server) target(d *document, offset int) (*sysml.Element, *sysml.Ref) {
	root := s.root(d)
	if root == nil {
		return nil, nil
	}
	if r := refAt(root, offset); r != nil {
		return r.Target, r
	}
	return s.declAt(d, root, offset), nil
}

// locate is where an element is declared.
func (s *Server) locate(e *sysml.Element) *Location {
	if e == nil || e.Generated {
		return nil
	}
	file := s.ws.FileOf(e)
	var d *document
	if strings.HasPrefix(file, "library:") {
		d = s.libraryDoc(file)
	} else {
		d = s.docs[file]
	}
	if d == nil {
		return nil
	}
	return &Location{d.uri, d.firstLine(e.Span)}
}

func (s *Server) definition(d *document, offset int) (any, error) {
	e, _ := s.target(d, offset)
	if loc := s.locate(e); loc != nil {
		return loc, nil
	}
	return nil, nil
}

// kindLabel writes a metaclass name in words: PartDefinition as part definition.
func kindLabel(class string) string {
	var sb strings.Builder
	for i, r := range class {
		if i > 0 && unicode.IsUpper(r) {
			sb.WriteByte(' ')
		}
		sb.WriteRune(unicode.ToLower(r))
	}
	return sb.String()
}

func (s *Server) hover(d *document, offset int) (any, error) {
	e, ref := s.target(d, offset)
	if e == nil {
		return nil, nil
	}
	var md strings.Builder
	name := sysml.QualifiedName(e)
	if name == "" {
		name = s.ws.EffectiveName(e)
	}
	fmt.Fprintf(&md, "**%s** `%s`", kindLabel(e.Class), name)
	if e.IsA("Feature") {
		var types []string
		for _, t := range s.ws.TypesOf(e) {
			types = append(types, "`"+sysml.QualifiedName(t)+"`")
		}
		if len(types) > 0 {
			fmt.Fprintf(&md, "\n\ntype: %s", strings.Join(types, ", "))
		}
	}
	if e.IsA("Type") {
		var supers []string
		for _, t := range s.ws.Supertypes(e) {
			supers = append(supers, "`"+sysml.QualifiedName(t)+"`")
		}
		if len(supers) > 0 {
			fmt.Fprintf(&md, "\n\nspecializes: %s", strings.Join(supers, ", "))
		}
	}
	for _, r := range e.Children("ownedRelationship") {
		if !r.IsA("FeatureValue") {
			continue
		}
		for _, x := range r.Children("ownedRelatedElement") {
			if v, err := s.ws.Evaluate(x); err == nil {
				fmt.Fprintf(&md, "\n\nvalue: `%s`", sysml.FormatValue(v))
			}
		}
	}
	if doc := documentationOf(e); doc != "" {
		fmt.Fprintf(&md, "\n\n---\n\n%s", doc)
	}
	h := Hover{Contents: MarkupContent{"markdown", md.String()}}
	if ref != nil {
		r := d.rng(ref.Span)
		h.Range = &r
	}
	return h, nil
}

// documentationOf is the text of the documentation an element owns.
func documentationOf(e *sysml.Element) string {
	for _, r := range e.Children("ownedRelationship") {
		for _, x := range r.Children("ownedRelatedElement") {
			if x.IsA("Documentation") {
				return cleanComment(x.Text("body"))
			}
		}
	}
	return ""
}

var commentStars = regexp.MustCompile(`(?m)^\s*\*+ ?`)

func cleanComment(body string) string {
	body = strings.TrimSpace(body)
	body = strings.TrimPrefix(body, "/*")
	body = strings.TrimSuffix(body, "*/")
	return strings.TrimSpace(commentStars.ReplaceAllString(body, ""))
}

// refsTo lists every place in the documents that refers to an element.
func (s *Server) refsTo(target *sysml.Element) []Location {
	var out []Location
	for _, doc := range s.ws.Documents() {
		d := s.docs[doc.Name]
		if d == nil {
			continue
		}
		doc.Root.Walk(func(e *sysml.Element) {
			for _, p := range e.Props {
				if r, ok := p.Value.(*sysml.Ref); ok && r.Target == target && r.Name != "" {
					out = append(out, Location{d.uri, d.rng(r.Span)})
				}
			}
		})
	}
	return out
}

func (s *Server) references(raw json.RawMessage) (any, error) {
	var p referenceParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ws == nil {
		return nil, nil
	}
	s.sync()
	d := s.docs[uriToPath(p.TextDocument.URI)]
	if d == nil {
		return nil, nil
	}
	e, _ := s.target(d, d.offset(p.Position))
	if e == nil {
		return []Location{}, nil
	}
	out := s.refsTo(e)
	if p.Context.IncludeDeclaration {
		if loc := s.locate(e); loc != nil {
			out = append([]Location{*loc}, out...)
		}
	}
	if out == nil {
		out = []Location{}
	}
	return out, nil
}

// nameRange finds the declared name of an element in its source: the first
// whole-word occurrence in the element's text, quoted or not.
func nameRange(d *document, e *sysml.Element, name string) (Range, bool) {
	text := d.text[e.Span.Start.Offset:min(e.Span.End.Offset, len(d.text))]
	re := regexp.MustCompile(`(?:^|[^\w'])('` + regexp.QuoteMeta(name) + `'|\b` + regexp.QuoteMeta(name) + `\b)`)
	loc := re.FindStringSubmatchIndex(text)
	if loc == nil {
		return Range{}, false
	}
	start := e.Span.Start.Offset + loc[2]
	return Range{d.position(start), d.position(start + loc[3] - loc[2])}, true
}

func (s *Server) rename(raw json.RawMessage) (any, error) {
	var p renameParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ws == nil {
		return nil, nil
	}
	s.sync()
	d := s.docs[uriToPath(p.TextDocument.URI)]
	if d == nil {
		return nil, nil
	}
	e, _ := s.target(d, d.offset(p.Position))
	if e == nil {
		return nil, fmt.Errorf("nothing to rename here")
	}
	file := s.ws.FileOf(e)
	if strings.HasPrefix(file, "library:") {
		return nil, fmt.Errorf("%s is part of the standard library", sysml.QualifiedName(e))
	}
	if !validName(p.NewName) {
		return nil, fmt.Errorf("%q is not a name", p.NewName)
	}
	name := s.ws.EffectiveName(e)
	edits := map[string][]TextEdit{}
	if decl := s.docs[file]; decl != nil {
		if r, ok := nameRange(decl, e, name); ok {
			edits[decl.uri] = append(edits[decl.uri], TextEdit{r, quoteIfNeeded(p.NewName)})
		}
	}
	for _, loc := range s.refsTo(e) {
		doc := s.docs[uriToPath(loc.URI)]
		// a qualified name changes only its last segment
		start, end := doc.offset(loc.Range.Start), doc.offset(loc.Range.End)
		written := doc.text[start:end]
		if i := strings.LastIndex(written, "::"); i >= 0 {
			start += i + 2
		}
		edits[loc.URI] = append(edits[loc.URI], TextEdit{Range{doc.position(start), doc.position(end)}, quoteIfNeeded(p.NewName)})
	}
	return WorkspaceEdit{edits}, nil
}

var simpleName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validName(n string) bool { return strings.TrimSpace(n) != "" && !strings.Contains(n, "\n") }

// quoteIfNeeded writes a name the way the notation needs it: with quotes unless
// it is a plain identifier.
func quoteIfNeeded(n string) string {
	if simpleName.MatchString(n) || strings.HasPrefix(n, "'") {
		return n
	}
	return "'" + n + "'"
}

func symbolKind(e *sysml.Element) int {
	switch {
	case e.IsA("Package"):
		return symPackage
	case e.IsA("EnumerationDefinition"):
		return symEnum
	case e.IsA("EnumerationUsage"):
		return symEnumMem
	case e.IsA("InterfaceDefinition"), e.IsA("PortDefinition"), e.IsA("ConnectionDefinition"):
		return symInterface
	case e.IsA("Behavior"), e.IsA("Function"), e.IsA("ActionDefinition"), e.IsA("CalculationDefinition"):
		return symFunction
	case e.IsA("Definition"), e.IsA("Classifier"):
		return symClass
	case e.IsA("AttributeUsage"):
		return symProperty
	case e.IsA("Usage"):
		return symField
	}
	return symVariable
}

func (s *Server) documentSymbol(raw json.RawMessage) (any, error) {
	var p documentSymbolParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ws == nil {
		return nil, nil
	}
	d := s.docs[uriToPath(p.TextDocument.URI)]
	if d == nil || s.root(d) == nil {
		return []DocumentSymbol{}, nil
	}
	return s.symbolsUnder(d, s.root(d)), nil
}

// symbolsUnder is the outline of the members of a namespace.
func (s *Server) symbolsUnder(d *document, ns *sysml.Element) []DocumentSymbol {
	out := []DocumentSymbol{}
	for _, m := range s.ws.OwnedMembers(ns) {
		if m.Generated {
			continue
		}
		name := s.ws.EffectiveName(m)
		if name == "" {
			// unnamed elements still hold named ones
			out = append(out, s.symbolsUnder(d, m)...)
			continue
		}
		sym := DocumentSymbol{
			Name:           name,
			Detail:         kindLabel(m.Class),
			Kind:           symbolKind(m),
			Range:          d.rng(m.Span),
			SelectionRange: d.firstLine(m.Span),
			Children:       s.symbolsUnder(d, m),
		}
		out = append(out, sym)
	}
	return out
}

func (s *Server) workspaceSymbol(raw json.RawMessage) (any, error) {
	var p workspaceSymbolParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []SymbolInformation{}
	if s.ws == nil {
		return out, nil
	}
	query := strings.ToLower(p.Query)
	for _, doc := range s.ws.Documents() {
		d := s.docs[doc.Name]
		if d == nil {
			continue
		}
		doc.Root.Walk(func(e *sysml.Element) {
			if e.Generated || len(out) >= 200 {
				return
			}
			name := s.ws.EffectiveName(e)
			if name == "" || !strings.Contains(strings.ToLower(name), query) {
				return
			}
			container := ""
			if e.Owner != nil && e.Owner.Owner != nil {
				container = sysml.QualifiedName(e.Owner.Owner)
			}
			out = append(out, SymbolInformation{name, symbolKind(e), Location{d.uri, d.firstLine(e.Span)}, container})
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func completionKind(e *sysml.Element) int {
	switch symbolKind(e) {
	case symPackage:
		return compModule
	case symEnum:
		return compEnum
	case symEnumMem:
		return compEnumMember
	case symInterface:
		return compInterface
	case symFunction:
		return compFunction
	case symClass:
		return compClass
	case symProperty:
		return compProperty
	case symField:
		return compField
	}
	return compVariable
}

func (s *Server) completion(d *document, offset int) (any, error) {
	prefix := d.wordBefore(offset)
	items := []CompletionItem{}
	seen := map[string]bool{}
	add := func(e *sysml.Element) {
		name := s.ws.EffectiveName(e)
		if name == "" || seen[name] || !strings.HasPrefix(strings.ToLower(name), strings.ToLower(lastSegment(prefix))) {
			return
		}
		seen[name] = true
		label := quoteIfNeeded(name)
		items = append(items, CompletionItem{Label: label, Kind: completionKind(e), Detail: kindLabel(e.Class)})
	}
	if i := strings.LastIndex(prefix, "::"); i >= 0 {
		// a qualified name: the members of what comes before the last ::
		if ns := s.resolveScoped(d, offset, prefix[:i]); ns != nil {
			for _, m := range s.ws.Members(ns) {
				add(m)
			}
		}
		return items, nil
	}
	// a plain name: what is visible from each enclosing namespace, innermost
	// first, then the packages at the top
	for _, ns := range s.enclosing(d, offset) {
		for _, m := range s.ws.Members(ns) {
			add(m)
		}
	}
	for _, doc := range s.ws.Documents() {
		for _, m := range s.ws.OwnedMembers(doc.Root) {
			add(m)
		}
	}
	for _, name := range []string{"ScalarValues", "Base", "ISQ", "SI", "Quantities", "Items", "Parts", "Actions", "States", "Requirements", "Occurrences"} {
		if e := s.ws.Lookup(name); e != nil {
			add(e)
		}
	}
	for _, kw := range keywords() {
		if !seen[kw] && strings.HasPrefix(kw, strings.ToLower(prefix)) {
			items = append(items, CompletionItem{Label: kw, Kind: compKeyword})
		}
	}
	return items, nil
}

func lastSegment(q string) string {
	if i := strings.LastIndex(q, "::"); i >= 0 {
		return q[i+2:]
	}
	return q
}

// enclosing lists the namespaces that contain an offset, innermost first.
func (s *Server) enclosing(d *document, offset int) []*sysml.Element {
	root := s.root(d)
	if root == nil {
		return nil
	}
	var out []*sysml.Element
	root.Walk(func(e *sysml.Element) {
		if !e.Generated && e.IsA("Namespace") && e != root && contains(e.Span, offset) {
			out = append(out, e)
		}
	})
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Span.End.Offset-out[i].Span.Start.Offset < out[j].Span.End.Offset-out[j].Span.Start.Offset
	})
	return out
}

// resolveScoped resolves the written name of a namespace from where the cursor
// is: outward through the enclosing namespaces, then globally.
func (s *Server) resolveScoped(d *document, offset int, written string) *sysml.Element {
	first, rest, _ := strings.Cut(written, "::")
	var start *sysml.Element
	for _, ns := range s.enclosing(d, offset) {
		if m := s.ws.Member(ns, first); m != nil {
			start = m
			break
		}
	}
	if start == nil {
		start = s.ws.Lookup(first)
	}
	for rest != "" && start != nil {
		var seg string
		seg, rest, _ = strings.Cut(rest, "::")
		start = s.ws.Member(start, seg)
	}
	return start
}
