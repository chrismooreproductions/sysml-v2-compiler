// SPDX-License-Identifier: EPL-2.0

// Package lsp is a Language Server Protocol server for SysML v2 and KerML.
//
// It speaks JSON-RPC over a pair of streams (the editor starts it as a
// subprocess and talks over standard input and output) and answers from a
// sysml.Workspace that holds the standard library, the files of the workspace
// folders and the editor's unsaved buffers. It publishes diagnostics (syntax
// errors, unresolved names and constraint violations) and answers hover,
// go to definition, find references, rename, outline, symbol search and
// completion requests.
package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chrismooreproductions/sysml-v2-compiler/sysml"
)

// Server is a language server. Create one with NewServer and call Run.
type Server struct {
	in  *bufio.Reader
	out io.Writer
	wmu sync.Mutex // guards writes to out

	mu       sync.Mutex // guards everything below
	ws       *sysml.Workspace
	docs     map[string]*document // by file system path
	libDocs  map[string]*document // standard library files by "library:" name
	folders  []string
	dirty    bool // documents changed since the workspace was last resolved
	timer    *time.Timer
	debounce time.Duration
	shutdown bool
	exit     bool
	diags    []sysml.Diagnostic // from the last resolution, with constraint violations
	cacheDir string             // where library files are written for the editor to open
	Logf     func(format string, args ...any)
}

// NewServer returns a server that reads requests from in and writes responses
// and notifications to out.
func NewServer(in io.Reader, out io.Writer) *Server {
	return &Server{
		in: bufio.NewReader(in), out: out,
		docs: map[string]*document{}, libDocs: map[string]*document{},
		debounce: 150 * time.Millisecond,
		Logf:     func(string, ...any) {},
	}
}

// Run serves requests until the editor asks the server to exit or the input
// ends. It returns nil after a clean shutdown and exit.
func (s *Server) Run() error {
	for {
		msg, err := s.read()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		s.handle(msg)
		s.mu.Lock()
		done := s.exit
		s.mu.Unlock()
		if done {
			return nil
		}
	}
}

func (s *Server) read() (*message, error) {
	length := -1
	for {
		line, err := s.in.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(k, "Content-Length") {
			length, err = strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				return nil, fmt.Errorf("bad Content-Length %q", v)
			}
		}
	}
	if length < 0 {
		return nil, fmt.Errorf("a message without Content-Length")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(s.in, body); err != nil {
		return nil, err
	}
	var m message
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("bad message: %w", err)
	}
	return &m, nil
}

func (s *Server) write(m *message) {
	m.JSONRPC = "2.0"
	body, err := json.Marshal(m)
	if err != nil {
		s.Logf("cannot encode a message: %v", err)
		return
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()
	fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n", len(body))
	s.out.Write(body)
}

func (s *Server) notify(method string, params any) {
	raw, err := json.Marshal(params)
	if err != nil {
		return
	}
	s.write(&message{Method: method, Params: raw})
}

func (s *Server) reply(id json.RawMessage, result any) {
	if result == nil {
		result = json.RawMessage("null")
	}
	s.write(&message{ID: id, Result: result})
}

func (s *Server) fail(id json.RawMessage, code int, format string, args ...any) {
	s.write(&message{ID: id, Error: &responseError{code, fmt.Sprintf(format, args...)}})
}

// handle dispatches one message. A failure inside a handler answers that one
// request with an error and leaves the server running.
func (s *Server) handle(m *message) {
	isRequest := len(m.ID) > 0
	defer func() {
		if r := recover(); r != nil {
			s.Logf("%s: panic: %v\n%s", m.Method, r, debug.Stack())
			if isRequest {
				s.fail(m.ID, -32603, "internal error in %s: %v", m.Method, r)
			}
		}
	}()
	var result any
	var err error
	switch m.Method {
	case "initialize":
		result, err = s.initialize(m.Params)
	case "initialized", "$/cancelRequest", "workspace/didChangeConfiguration":
	case "shutdown":
		s.mu.Lock()
		s.shutdown = true
		s.mu.Unlock()
	case "exit":
		s.mu.Lock()
		s.exit = true
		s.mu.Unlock()
	case "textDocument/didOpen":
		s.didOpen(m.Params)
	case "textDocument/didChange":
		s.didChange(m.Params)
	case "textDocument/didClose":
		s.didClose(m.Params)
	case "textDocument/didSave":
	case "textDocument/hover":
		result, err = s.withPosition(m.Params, s.hover)
	case "textDocument/definition":
		result, err = s.withPosition(m.Params, s.definition)
	case "textDocument/references":
		result, err = s.references(m.Params)
	case "textDocument/rename":
		result, err = s.rename(m.Params)
	case "textDocument/documentSymbol":
		result, err = s.documentSymbol(m.Params)
	case "textDocument/completion":
		result, err = s.withPosition(m.Params, s.completion)
	case "workspace/symbol":
		result, err = s.workspaceSymbol(m.Params)
	default:
		if isRequest {
			s.fail(m.ID, -32601, "method not found: %s", m.Method)
		}
		return
	}
	if !isRequest {
		return
	}
	if err != nil {
		s.fail(m.ID, -32603, "%v", err)
		return
	}
	s.reply(m.ID, result)
}

func (s *Server) initialize(raw json.RawMessage) (any, error) {
	var p initializeParams
	json.Unmarshal(raw, &p)
	ws, err := sysml.NewWorkspace()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.ws = ws
	for _, f := range p.WorkspaceFolders {
		s.folders = append(s.folders, uriToPath(f.URI))
	}
	if len(s.folders) == 0 && p.RootURI != "" {
		s.folders = append(s.folders, uriToPath(p.RootURI))
	}
	for _, folder := range s.folders {
		s.scan(folder)
	}
	s.dirty = true
	s.mu.Unlock()
	return map[string]any{
		"capabilities": map[string]any{
			"textDocumentSync":        map[string]any{"openClose": true, "change": 1},
			"hoverProvider":           true,
			"definitionProvider":      true,
			"referencesProvider":      true,
			"renameProvider":          true,
			"documentSymbolProvider":  true,
			"workspaceSymbolProvider": true,
			"completionProvider":      map[string]any{"triggerCharacters": []string{":", "."}},
		},
		"serverInfo": map[string]any{"name": "sysmlc lsp"},
	}, nil
}

// scan adds the model files under a folder to the workspace. The caller holds mu.
func (s *Server) scan(folder string) {
	n := 0
	filepath.WalkDir(folder, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != folder && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if _, err := sysml.LanguageOf(p); err != nil || n >= 5000 {
			return nil
		}
		if _, known := s.docs[p]; known {
			return nil
		}
		if b, err := os.ReadFile(p); err == nil {
			s.setText(p, string(b), 0, false)
			n++
		}
		return nil
	})
}

// setText records a document's text and parses it into the workspace. The
// caller holds mu.
func (s *Server) setText(path, text string, version int, open bool) {
	d := s.docs[path]
	if d == nil {
		d = &document{path: path, uri: pathToURI(path)}
		s.docs[path] = d
	}
	d.text, d.version, d.starts = text, version, nil
	d.open = d.open || open
	if _, err := sysml.LanguageOf(path); err != nil {
		return
	}
	_, d.errs = s.ws.Replace(path, []byte(text))
	s.dirty = true
}

func (s *Server) didOpen(raw json.RawMessage) {
	var p didOpenParams
	if json.Unmarshal(raw, &p) != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ws == nil {
		return
	}
	path := uriToPath(p.TextDocument.URI)
	s.setText(path, p.TextDocument.Text, p.TextDocument.Version, true)
	s.docs[path].uri = p.TextDocument.URI
	s.schedule()
}

func (s *Server) didChange(raw json.RawMessage) {
	var p didChangeParams
	if json.Unmarshal(raw, &p) != nil || len(p.ContentChanges) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ws == nil {
		return
	}
	path := uriToPath(p.TextDocument.URI)
	d := s.docs[path]
	text := p.ContentChanges[len(p.ContentChanges)-1].Text
	if d != nil && p.ContentChanges[0].Range != nil {
		// incremental changes: apply them in order
		text = d.text
		for _, c := range p.ContentChanges {
			if c.Range == nil {
				text = c.Text
				continue
			}
			tmp := &document{text: text}
			text = text[:tmp.offset(c.Range.Start)] + c.Text + text[tmp.offset(c.Range.End):]
		}
	}
	s.setText(path, text, p.TextDocument.Version, true)
	s.schedule()
}

func (s *Server) didClose(raw json.RawMessage) {
	var p didCloseParams
	if json.Unmarshal(raw, &p) != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := uriToPath(p.TextDocument.URI)
	d := s.docs[path]
	if d == nil {
		return
	}
	d.open = false
	s.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{d.uri, []Diagnostic{}})
	if b, err := os.ReadFile(path); err == nil {
		s.setText(path, string(b), 0, false) // back to what is on disk
	} else {
		delete(s.docs, path)
		s.ws.Remove(path)
		s.dirty = true
	}
	s.schedule()
}

// schedule publishes diagnostics once the document has been quiet for a moment.
// The caller holds mu.
func (s *Server) schedule() {
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.debounce, s.publish)
}

// sync resolves the workspace if documents changed. The caller holds mu.
func (s *Server) sync() {
	if !s.dirty || s.ws == nil {
		return
	}
	s.dirty = false
	s.diags = append(s.ws.Resolve(), s.ws.Validate()...)
}

// publish sends the diagnostics of every open document.
func (s *Server) publish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ws == nil || s.shutdown {
		return
	}
	s.sync()
	for _, d := range s.docs {
		if !d.open {
			continue
		}
		s.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{d.uri, s.diagnosticsFor(d)})
	}
}

// diagnosticsFor lists the problems in a document: its syntax errors, or if it
// has none, its unresolved names and constraint violations.
func (s *Server) diagnosticsFor(d *document) []Diagnostic {
	out := []Diagnostic{}
	if len(d.errs) > 0 {
		for _, e := range d.errs {
			start := d.position(e.Pos.Offset)
			end := d.position(e.Pos.Offset + max(1, len(e.Got)))
			out = append(out, Diagnostic{Range: Range{start, end}, Severity: 1, Source: "sysml", Message: e.Msg})
		}
		return out
	}
	for _, g := range s.diags {
		if g.File != d.path {
			continue
		}
		code := g.Constraint
		r := d.rng(g.Span)
		if code != "" {
			r = d.firstLine(g.Span)
		}
		out = append(out, Diagnostic{Range: r, Severity: 1, Code: code, Source: "sysml", Message: g.Message})
	}
	return out
}

// libraryDoc returns a standard library file as a document, extracted into the
// cache directory so the editor can open it.
func (s *Server) libraryDoc(name string) *document {
	if d, ok := s.libDocs[name]; ok {
		return d
	}
	path := strings.TrimPrefix(name, "library:")
	srcs, err := librarySources()
	if err != nil {
		return nil
	}
	text, ok := srcs[path]
	if !ok {
		return nil
	}
	if s.cacheDir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			base = os.TempDir()
		}
		s.cacheDir = filepath.Join(base, "sysml-v2-compiler", "library")
	}
	full := filepath.Join(s.cacheDir, filepath.FromSlash(path))
	if existing, err := os.ReadFile(full); err != nil || !bytes.Equal(existing, []byte(text)) {
		if os.MkdirAll(filepath.Dir(full), 0o755) == nil {
			os.WriteFile(full, []byte(text), 0o444)
		}
	}
	d := &document{path: full, uri: pathToURI(full), text: text}
	s.libDocs[name] = d
	return d
}
