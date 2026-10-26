// SPDX-License-Identifier: EPL-2.0

package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chrismooreproductions/sysml-v2-compiler/sysml"
)

// client is an editor talking to a server over pipes.
type client struct {
	t       *testing.T
	srv     *Session
	host    *Host
	in      io.WriteCloser
	nextID  int
	mu      sync.Mutex
	replies map[string]chan *message
	notes   chan *message
	done    chan error
}

func start(t *testing.T) *client {
	t.Helper()
	return startWith(t, Options{Debounce: 10 * time.Millisecond}, nil)
}

// startWith serves a new session of a new host. If setup is not nil it is
// called with the session before the client speaks.
func startWith(t *testing.T, opts Options, setup func(*Session)) *client {
	t.Helper()
	return startOn(t, NewHost(opts), setup)
}

// startOn is startWith on a host that already exists.
func startOn(t *testing.T, h *Host, setup func(*Session)) *client {
	t.Helper()
	cr, sw := io.Pipe() // server writes, client reads
	sr, cw := io.Pipe() // client writes, server reads
	srv, err := h.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if setup != nil {
		setup(srv)
	}
	c := &client{t: t, in: cw, srv: srv, host: h, replies: map[string]chan *message{}, notes: make(chan *message, 1000), done: make(chan error, 1)}
	go func() {
		c.done <- srv.Serve(context.Background(), struct {
			io.Reader
			io.Writer
		}{sr, sw})
		sw.Close()
	}()
	go func() {
		rd := bufio.NewReader(cr)
		for {
			length := 0
			for {
				line, err := rd.ReadString('\n')
				if err != nil {
					return
				}
				line = strings.TrimRight(line, "\r\n")
				if line == "" {
					break
				}
				fmt.Sscanf(line, "Content-Length: %d", &length)
			}
			body := make([]byte, length)
			if _, err := io.ReadFull(rd, body); err != nil {
				return
			}
			var m message
			json.Unmarshal(body, &m)
			if len(m.ID) > 0 && m.Method == "" {
				c.mu.Lock()
				ch := c.replies[string(m.ID)]
				c.mu.Unlock()
				if ch != nil {
					ch <- &m
				}
			} else {
				c.notes <- &m
			}
		}
	}()
	t.Cleanup(func() { cw.Close() })
	return c
}

func (c *client) send(m map[string]any) {
	m["jsonrpc"] = "2.0"
	body, _ := json.Marshal(m)
	fmt.Fprintf(c.in, "Content-Length: %d\r\n\r\n%s", len(body), body)
}

func (c *client) notify(method string, params any) {
	c.send(map[string]any{"method": method, "params": params})
}

// call sends a request and returns its result, or fails the test on an error.
func (c *client) call(method string, params any) json.RawMessage {
	c.t.Helper()
	c.nextID++
	id := fmt.Sprint(c.nextID)
	ch := make(chan *message, 1)
	c.mu.Lock()
	c.replies[id] = ch
	c.mu.Unlock()
	c.send(map[string]any{"id": c.nextID, "method": method, "params": params})
	select {
	case m := <-ch:
		if m.Error != nil {
			c.t.Fatalf("%s: error %d %s", method, m.Error.Code, m.Error.Message)
		}
		raw, _ := json.Marshal(m.Result)
		return raw
	case <-time.After(20 * time.Second):
		c.t.Fatalf("%s: no reply", method)
	}
	return nil
}

// diagnostics waits for the next diagnostics published for a URI that satisfy ok.
func (c *client) diagnostics(uri string, ok func([]Diagnostic) bool) []Diagnostic {
	c.t.Helper()
	deadline := time.After(20 * time.Second)
	for {
		select {
		case m := <-c.notes:
			if m.Method != "textDocument/publishDiagnostics" {
				continue
			}
			var p publishDiagnosticsParams
			json.Unmarshal(m.Params, &p)
			if p.URI == uri && ok(p.Diagnostics) {
				return p.Diagnostics
			}
		case <-deadline:
			c.t.Fatalf("no matching diagnostics for %s", uri)
		}
	}
}

func (c *client) initialize() {
	c.t.Helper()
	params := map[string]any{"rootUri": "file:///work", "capabilities": map[string]any{}}
	c.call("initialize", params)
	c.notify("initialized", map[string]any{})
}

func (c *client) open(uri, text string) {
	c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": uri, "version": 1, "languageId": "sysml", "text": text}})
}

func (c *client) change(uri string, version int, text string) {
	c.notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": version},
		"contentChanges": []map[string]any{{"text": text}},
	})
}

func pos(line, char int) map[string]any { return map[string]any{"line": line, "character": char} }

func at(uri string, line, char int) map[string]any {
	return map[string]any{"textDocument": map[string]any{"uri": uri}, "position": pos(line, char)}
}

func none(d []Diagnostic) bool { return len(d) == 0 }
func some(d []Diagnostic) bool { return len(d) > 0 }

const fleet = `package Fleet {
    part def Truck {
        attribute payload : ScalarValues::Real;
    }
    part rig : Truck;
    part spare : Truck;
}
`

func TestDiagnosticsFollowTheText(t *testing.T) {
	c := start(t)
	c.initialize()
	uri := "file:///work/" + "fleet.sysml"

	c.open(uri, strings.Replace(fleet, "part rig : Truck;", "part rig : Trck;", 1))
	d := c.diagnostics(uri, some)
	if len(d) != 1 || !strings.Contains(d[0].Message, `cannot resolve "Trck"`) || d[0].Range.Start.Line != 4 || d[0].Range.Start.Character != 15 {
		t.Fatalf("unresolved name: %+v", d)
	}

	c.change(uri, 2, strings.Replace(fleet, "part rig : Truck;", "part rig : ;", 1))
	d = c.diagnostics(uri, func(d []Diagnostic) bool { return len(d) == 1 && strings.Contains(d[0].Message, "unexpected") })
	if d[0].Range.Start.Line != 4 {
		t.Errorf("syntax error: %+v", d)
	}

	c.change(uri, 3, fleet)
	c.diagnostics(uri, none)

	c.change(uri, 4, "package P { part def V { variant part x; } }")
	d = c.diagnostics(uri, some)
	if d[0].Code != "validateVariantMembershipOwningNamespace" {
		t.Errorf("constraint: %+v", d)
	}
}

func TestDefinitionHoverAndReferences(t *testing.T) {
	c := start(t)
	c.initialize()
	uri := "file:///work/" + "fleet.sysml"
	c.open(uri, fleet)
	c.diagnostics(uri, none)

	// "part rig : Truck;" is line 4; Truck starts at column 15
	var loc Location
	json.Unmarshal(c.call("textDocument/definition", at(uri, 4, 16)), &loc)
	if loc.URI != uri || loc.Range.Start.Line != 1 {
		t.Errorf("definition of Truck: %+v", loc)
	}

	var lib Location
	json.Unmarshal(c.call("textDocument/definition", at(uri, 2, 33)), &lib)
	path, ok := LibraryPath(lib.URI)
	if !ok || !strings.HasSuffix(path, "/ScalarValues.kerml") || !strings.HasPrefix(lib.URI, "sysml-library:///") {
		t.Fatalf("definition of ScalarValues::Real: %+v", lib)
	}
	if text, ok := LibraryText(path); !ok || !strings.Contains(text, "datatype Real") {
		t.Errorf("the library file the editor is sent to: %q", path)
	}

	var h Hover
	json.Unmarshal(c.call("textDocument/hover", at(uri, 4, 16)), &h)
	if !strings.Contains(h.Contents.Value, "**part definition** `Fleet::Truck`") {
		t.Errorf("hover: %q", h.Contents.Value)
	}
	json.Unmarshal(c.call("textDocument/hover", at(uri, 4, 9)), &h) // on rig itself
	if !strings.Contains(h.Contents.Value, "`Fleet::rig`") || !strings.Contains(h.Contents.Value, "type: `Fleet::Truck`") {
		t.Errorf("hover on a declaration: %q", h.Contents.Value)
	}

	var refs []Location
	args := at(uri, 1, 18) // on the Truck definition
	args["context"] = map[string]any{"includeDeclaration": false}
	json.Unmarshal(c.call("textDocument/references", args), &refs)
	if len(refs) != 2 || refs[0].Range.Start.Line != 4 || refs[1].Range.Start.Line != 5 {
		t.Errorf("references: %+v", refs)
	}
}

func TestOutlineAndSymbols(t *testing.T) {
	c := start(t)
	c.initialize()
	uri := "file:///work/" + "fleet.sysml"
	c.open(uri, fleet)
	c.diagnostics(uri, none)

	var syms []DocumentSymbol
	json.Unmarshal(c.call("textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": uri}}), &syms)
	if len(syms) != 1 || syms[0].Name != "Fleet" || len(syms[0].Children) != 3 {
		t.Fatalf("outline: %+v", syms)
	}
	truck := syms[0].Children[0]
	if truck.Name != "Truck" || truck.Detail != "part definition" || len(truck.Children) != 1 || truck.Children[0].Name != "payload" {
		t.Errorf("Truck in the outline: %+v", truck)
	}

	var found []SymbolInformation
	json.Unmarshal(c.call("workspace/symbol", map[string]any{"query": "ru"}), &found)
	if len(found) != 1 || found[0].Name != "Truck" || found[0].ContainerName != "Fleet" {
		t.Errorf("workspace symbols: %+v", found)
	}
}

func TestRename(t *testing.T) {
	c := start(t)
	c.initialize()
	uri := "file:///work/" + "fleet.sysml"
	c.open(uri, fleet)
	c.diagnostics(uri, none)

	var edit WorkspaceEdit
	args := at(uri, 4, 16)
	args["newName"] = "Lorry"
	json.Unmarshal(c.call("textDocument/rename", args), &edit)
	edits := edit.Changes[uri]
	if len(edits) != 3 {
		t.Fatalf("edits: %+v", edit)
	}
	// apply them to the text and check it is still valid
	d := &document{text: fleet}
	text := fleet
	// apply from the end so earlier offsets stay right
	sorted := append([]TextEdit(nil), edits...)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if d.offset(sorted[j].Range.Start) > d.offset(sorted[i].Range.Start) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	for _, e := range sorted {
		text = text[:d.offset(e.Range.Start)] + e.NewText + text[d.offset(e.Range.End):]
	}
	want := strings.ReplaceAll(fleet, "Truck", "Lorry")
	if text != want {
		t.Errorf("after the rename:\n%s\nwant:\n%s", text, want)
	}
}

func TestCompletion(t *testing.T) {
	c := start(t)
	c.initialize()
	uri := "file:///work/" + "fleet.sysml"
	src := "package Fleet {\n    part def Truck;\n    part rig : Tr\n    attribute w : ScalarValues::Re\n}\n"
	c.open(uri, src)
	c.diagnostics(uri, some)

	labels := func(raw json.RawMessage) string {
		var items []CompletionItem
		json.Unmarshal(raw, &items)
		var out []string
		for _, i := range items {
			out = append(out, i.Label)
		}
		return strings.Join(out, ",")
	}
	if got := labels(c.call("textDocument/completion", at(uri, 2, 17))); !strings.Contains(got, "Truck") {
		t.Errorf("completions for Tr: %s", got)
	}
	if got := labels(c.call("textDocument/completion", at(uri, 3, 34))); !strings.Contains(got, "Real") || strings.Contains(got, "Integer") {
		t.Errorf("completions for ScalarValues::Re: %s", got)
	}
}

func TestDocumentsTheHostSetAreInScope(t *testing.T) {
	c := startWith(t, Options{Debounce: 10 * time.Millisecond}, func(s *Session) {
		if err := s.SetDocument("file:///imports/7.sysml", "package Lib { part def Engine; }", sysml.SysML); err != nil {
			t.Fatal(err)
		}
	})
	c.initialize()
	uri := "file:///work/use.sysml"
	c.open(uri, "package Use { part e : Lib::Engine; part f : Lib::Moter; }")
	d := c.diagnostics(uri, some)
	if len(d) != 1 || !strings.Contains(d[0].Message, `"Lib::Moter"`) {
		t.Fatalf("diagnostics: %+v", d)
	}
	var loc Location
	json.Unmarshal(c.call("textDocument/definition", at(uri, 0, 30)), &loc)
	if loc.URI != "file:///imports/7.sysml" {
		t.Errorf("definition into the host's document: %+v", loc)
	}
	// the host replaces the document, and what resolves changes
	c.srv.SetDocument("file:///imports/7.sysml", "package Lib { part def Engine; part def Moter; }", sysml.SysML)
	c.diagnostics(uri, none)
}

// The client can neither change nor close a document the host set, and a
// rename leaves it alone.
func TestDocumentsTheHostSetAreReadOnly(t *testing.T) {
	c := startWith(t, Options{Debounce: 10 * time.Millisecond}, func(s *Session) {
		s.SetDocument("file:///imports/1.sysml", "package Lib { part def Engine; }", sysml.SysML)
	})
	c.initialize()
	lib := "file:///imports/1.sysml"
	c.open(lib, "package Other { }")
	c.change(lib, 2, "package Broken {")
	c.notify("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": lib}})
	uri := "file:///work/use.sysml"
	c.open(uri, "package Use { part e : Lib::Engine; }")
	c.diagnostics(uri, none)
	var loc Location
	json.Unmarshal(c.call("textDocument/definition", at(uri, 0, 30)), &loc)
	if loc.URI != lib || loc.Range.Start.Line != 0 {
		t.Errorf("the host's document should stand: %+v", loc)
	}
	args := at(uri, 0, 30)
	args["newName"] = "Motor"
	c.nextID++
	id := c.nextID
	ch := make(chan *message, 1)
	c.mu.Lock()
	c.replies[fmt.Sprint(id)] = ch
	c.mu.Unlock()
	c.send(map[string]any{"id": id, "method": "textDocument/rename", "params": args})
	if m := <-ch; m.Error == nil || !strings.Contains(m.Error.Message, "cannot be changed") {
		t.Errorf("renaming what the host owns: %+v", m)
	}
}

func TestLimits(t *testing.T) {
	h := NewHost(Options{MaxDocuments: 2, MaxBytes: 100})
	s, err := h.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.SetDocument("file:///a.sysml", "package A;", sysml.SysML); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDocument("file:///b.sysml", "package B;", sysml.SysML); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDocument("file:///c.sysml", "package C;", sysml.SysML); err != ErrTooManyDocuments {
		t.Errorf("a third document: %v", err)
	}
	if err := s.SetDocument("file:///a.sysml", strings.Repeat("x", 200), sysml.SysML); err != ErrTooMuchText {
		t.Errorf("too much text: %v", err)
	}
	if err := s.SetDocument("file:///a.sysml", "package A2;", sysml.SysML); err != nil {
		t.Errorf("replacing a document: %v", err)
	}
	if st := s.Stats(); st.Documents != 2 || st.Bytes != int64(len("package A2;")+len("package B;")) {
		t.Errorf("stats: %+v", st)
	}
	s.RemoveDocument("file:///b.sysml")
	if st := h.Stats(); st.Sessions != 1 || st.Documents != 1 {
		t.Errorf("host stats: %+v", st)
	}
}

// Files lets a program that wants it, as the command line tool does, put the
// files of the client's folders in scope.
type memFiles map[string]string

func (m memFiles) List(_ context.Context, folders []string) []File {
	var out []File
	for k, v := range m {
		out = append(out, File{k, v})
	}
	return out
}

func (m memFiles) Read(_ context.Context, uri string) (string, bool) { v, ok := m[uri]; return v, ok }

func TestFilesOfTheFoldersAreInScopeWhenTheHostAsks(t *testing.T) {
	files := memFiles{"file:///work/lib.sysml": "package Lib { part def Engine; }"}
	c := startWith(t, Options{Debounce: 10 * time.Millisecond, Files: files}, nil)
	c.initialize()
	uri := "file:///work/use.sysml"
	c.open(uri, "package Use { part e : Lib::Engine; part f : Lib::Moter; }")
	if d := c.diagnostics(uri, some); len(d) != 1 {
		t.Fatalf("diagnostics: %+v", d)
	}
	// editing the file in the editor changes what resolves; closing it goes back to the file
	libURI := "file:///work/lib.sysml"
	c.open(libURI, "package Lib { part def Engine; part def Moter; }")
	c.diagnostics(uri, none)
	c.notify("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": libURI}})
	c.diagnostics(uri, some)
}

func TestTheServerKnowsNoFilesUnlessTheHostAsks(t *testing.T) {
	c := startWith(t, Options{Debounce: 10 * time.Millisecond}, nil)
	c.call("initialize", map[string]any{"rootUri": "file:///", "workspaceFolders": []map[string]any{{"uri": "file:///"}}, "capabilities": map[string]any{}})
	if st := c.srv.Stats(); st.Documents != 0 {
		t.Errorf("documents after initialize: %+v", st)
	}
}

func TestPositionsCountUTF16Units(t *testing.T) {
	d := &document{text: "a😀b\nxyz"}
	// 😀 is one rune, four bytes, two UTF-16 units
	if p := d.position(5); p != (Position{0, 3}) {
		t.Errorf("position(5) = %+v", p)
	}
	if o := d.offset(Position{0, 3}); o != 5 {
		t.Errorf("offset(0,3) = %d", o)
	}
	if o := d.offset(Position{1, 99}); o != len(d.text) {
		t.Errorf("a column past the end is clamped: %d", o)
	}
}

func TestShutdown(t *testing.T) {
	c := start(t)
	c.initialize()
	c.call("shutdown", nil)
	c.notify("exit", nil)
	select {
	case err := <-c.done:
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the server did not exit")
	}
}
