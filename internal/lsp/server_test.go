// SPDX-License-Identifier: EPL-2.0

package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// client is an editor talking to a server over pipes.
type client struct {
	t       *testing.T
	in      io.WriteCloser
	nextID  int
	mu      sync.Mutex
	replies map[string]chan *message
	notes   chan *message
	done    chan error
}

func start(t *testing.T) *client {
	t.Helper()
	cr, sw := io.Pipe() // server writes, client reads
	sr, cw := io.Pipe() // client writes, server reads
	srv := NewServer(sr, sw)
	srv.debounce = 10 * time.Millisecond
	c := &client{t: t, in: cw, replies: map[string]chan *message{}, notes: make(chan *message, 1000), done: make(chan error, 1)}
	go func() { c.done <- srv.Run(); sw.Close() }()
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

func (c *client) initialize(folder string) {
	c.t.Helper()
	params := map[string]any{"rootUri": pathToURI(folder), "capabilities": map[string]any{}}
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

func workspace(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDiagnosticsFollowTheText(t *testing.T) {
	dir := workspace(t, nil)
	c := start(t)
	c.initialize(dir)
	uri := pathToURI(filepath.Join(dir, "fleet.sysml"))

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
	dir := workspace(t, nil)
	c := start(t)
	c.initialize(dir)
	uri := pathToURI(filepath.Join(dir, "fleet.sysml"))
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
	if !strings.HasSuffix(lib.URI, "ScalarValues.kerml") {
		t.Errorf("definition of ScalarValues::Real: %+v", lib)
	}
	if data, err := os.ReadFile(uriToPath(lib.URI)); err != nil || !strings.Contains(string(data), "datatype Real") {
		t.Errorf("the library file the editor is sent to: %v", err)
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
	dir := workspace(t, nil)
	c := start(t)
	c.initialize(dir)
	uri := pathToURI(filepath.Join(dir, "fleet.sysml"))
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
	dir := workspace(t, nil)
	c := start(t)
	c.initialize(dir)
	uri := pathToURI(filepath.Join(dir, "fleet.sysml"))
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
	dir := workspace(t, nil)
	c := start(t)
	c.initialize(dir)
	uri := pathToURI(filepath.Join(dir, "fleet.sysml"))
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

func TestOtherFilesInTheFolderAreInScope(t *testing.T) {
	dir := workspace(t, map[string]string{
		"lib.sysml": "package Lib { part def Engine; }",
	})
	c := start(t)
	c.initialize(dir)
	uri := pathToURI(filepath.Join(dir, "use.sysml"))
	c.open(uri, "package Use { part e : Lib::Engine; part f : Lib::Moter; }")
	d := c.diagnostics(uri, some)
	if len(d) != 1 || !strings.Contains(d[0].Message, `"Lib::Moter"`) {
		t.Fatalf("diagnostics: %+v", d)
	}
	var loc Location
	json.Unmarshal(c.call("textDocument/definition", at(uri, 0, 30)), &loc)
	if !strings.HasSuffix(loc.URI, "lib.sysml") {
		t.Errorf("definition into another file: %+v", loc)
	}
	// editing the other file in the editor changes what resolves
	libURI := pathToURI(filepath.Join(dir, "lib.sysml"))
	c.open(libURI, "package Lib { part def Engine; part def Moter; }")
	c.diagnostics(uri, none)
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
	dir := workspace(t, nil)
	c := start(t)
	c.initialize(dir)
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
