// SPDX-License-Identifier: EPL-2.0

package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// JSON-RPC and LSP error codes.
const (
	codeInternal         = -32603
	codeMethodNotFound   = -32601
	codeRequestCancelled = -32800
)

func (s *Session) write(m *message) {
	m.JSONRPC = "2.0"
	body, err := json.Marshal(m)
	if err != nil {
		s.log.Error("cannot encode a message", "err", err)
		return
	}
	s.dmu.Lock()
	c := s.conn
	s.dmu.Unlock()
	if c == nil {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := c.WriteMessage(s.ctx, body); err != nil && s.ctx.Err() == nil {
		s.log.Error("cannot write a message", "err", err)
		s.fatal(err)
	}
}

func (s *Session) notify(method string, params any) {
	raw, err := json.Marshal(params)
	if err != nil {
		return
	}
	s.write(&message{Method: method, Params: raw})
}

func (s *Session) reply(id json.RawMessage, result any) {
	if result == nil {
		result = json.RawMessage("null")
	}
	s.write(&message{ID: id, Result: result})
}

func (s *Session) fail(id json.RawMessage, code int, format string, args ...any) {
	s.write(&message{ID: id, Error: &responseError{code, fmt.Sprintf(format, args...)}})
}

// dispatch handles one message. Notifications are handled before the next
// message is read, so that edits and requests keep their order; requests that
// compute run in goroutines, so that a cancellation or an edit can be read
// while they work.
func (s *Session) dispatch(ctx context.Context, m *message) {
	isRequest := len(m.ID) > 0
	switch m.Method {
	case "":
		return // a response to a request we never made
	case "initialize":
		s.inline(m, func() (any, error) { return s.initialize(ctx, m.Params) })
	case "shutdown":
		s.inline(m, func() (any, error) { s.shutdown.Store(true); return nil, nil })
	case "exit":
		s.exit.Store(true)
	case "initialized", "workspace/didChangeConfiguration", "textDocument/didSave":
	case "$/cancelRequest":
		var p struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal(m.Params, &p) == nil {
			s.dmu.Lock()
			cancel := s.inflight[string(p.ID)]
			s.dmu.Unlock()
			if cancel != nil {
				cancel()
			}
		}
	case "textDocument/didOpen":
		s.safely(m.Method, func() { s.didOpen(m.Params) })
	case "textDocument/didChange":
		s.safely(m.Method, func() { s.didChange(m.Params) })
	case "textDocument/didClose":
		s.safely(m.Method, func() { s.didClose(ctx, m.Params) })
	case "textDocument/hover":
		s.request(ctx, m, func(c context.Context) (any, error) { return s.withPosition(c, m.Params, s.hover) })
	case "textDocument/definition":
		s.request(ctx, m, func(c context.Context) (any, error) { return s.withPosition(c, m.Params, s.definition) })
	case "textDocument/completion":
		s.request(ctx, m, func(c context.Context) (any, error) { return s.withPosition(c, m.Params, s.completion) })
	case "textDocument/references":
		s.request(ctx, m, func(c context.Context) (any, error) { return s.references(c, m.Params) })
	case "textDocument/rename":
		s.request(ctx, m, func(c context.Context) (any, error) { return s.rename(c, m.Params) })
	case "textDocument/documentSymbol":
		s.request(ctx, m, func(c context.Context) (any, error) { return s.documentSymbol(c, m.Params) })
	case "workspace/symbol":
		s.request(ctx, m, func(c context.Context) (any, error) { return s.workspaceSymbol(c, m.Params) })
	default:
		if isRequest {
			s.fail(m.ID, codeMethodNotFound, "method not found: %s", m.Method)
		}
	}
}

// inline answers a request at once, in the reading goroutine.
func (s *Session) inline(m *message, fn func() (any, error)) {
	var result any
	var err error
	if pe := s.safely(m.Method, func() { result, err = fn() }); pe != nil {
		s.fail(m.ID, codeInternal, "internal error in %s: %v", m.Method, pe.Value)
		return
	}
	s.answer(m.ID, result, err)
}

func (s *Session) answer(id json.RawMessage, result any, err error) {
	switch {
	case err == nil:
		s.reply(id, result)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		s.fail(id, codeRequestCancelled, "request cancelled")
	default:
		s.fail(id, codeInternal, "%v", err)
	}
}

// request runs a request that needs the workspace in a goroutine. A
// $/cancelRequest for it ends its context.
func (s *Session) request(ctx context.Context, m *message, fn func(context.Context) (any, error)) {
	id := string(m.ID)
	rctx, cancel := context.WithCancel(ctx)
	s.dmu.Lock()
	if lim := s.h.opts.MaxInFlight; lim > 0 && len(s.inflight) >= lim {
		s.dmu.Unlock()
		cancel()
		s.fail(m.ID, codeInternal, "too many requests in flight")
		return
	}
	s.inflight[id] = cancel
	s.dmu.Unlock()
	finish := func() {
		cancel()
		s.dmu.Lock()
		delete(s.inflight, id)
		s.dmu.Unlock()
	}
	if !s.spawn(m.Method, func() {
		defer finish()
		var result any
		var err error
		if pe := s.safely(m.Method, func() { result, err = fn(rctx) }); pe != nil {
			s.fail(m.ID, codeInternal, "internal error in %s: %v", m.Method, pe.Value)
			return
		}
		s.answer(m.ID, result, err)
	}) {
		finish()
	}
}

func (s *Session) initialize(ctx context.Context, raw json.RawMessage) (any, error) {
	var p initializeParams
	json.Unmarshal(raw, &p)
	if files := s.h.opts.Files; files != nil {
		var folders []string
		for _, f := range p.WorkspaceFolders {
			folders = append(folders, f.URI)
		}
		if len(folders) == 0 && p.RootURI != "" {
			folders = append(folders, p.RootURI)
		}
		for _, f := range files.List(ctx, folders) {
			key := canonical(f.URI)
			s.dmu.Lock()
			if s.docs[key] == nil && s.fits(key, len(f.Text)) == nil {
				s.store(&source{key: key, uri: f.URI, text: f.Text, lang: languageOf(f.URI, "")})
			}
			s.dmu.Unlock()
		}
	}
	s.dmu.Lock()
	s.scheduleLocked()
	s.dmu.Unlock()
	return map[string]any{
		"capabilities": map[string]any{
			// the whole text is sent with each change: the server parses the
			// whole document anyway, and has no state to keep in step
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

func (s *Session) didOpen(raw json.RawMessage) {
	var p didOpenParams
	if json.Unmarshal(raw, &p) != nil {
		return
	}
	item := p.TextDocument
	key := canonical(item.URI)
	s.dmu.Lock()
	if s.closed || s.docs[key] != nil && s.docs[key].pinned {
		s.dmu.Unlock()
		return // the host's document stands
	}
	if err := s.fits(key, len(item.Text)); err != nil {
		s.dmu.Unlock()
		s.refuse(item.URI, err)
		return
	}
	s.store(&source{key: key, uri: item.URI, text: item.Text, version: item.Version, lang: languageOf(item.URI, item.LanguageID), open: true})
	s.scheduleLocked()
	s.dmu.Unlock()
	s.interrupt()
}

func (s *Session) didChange(raw json.RawMessage) {
	var p didChangeParams
	if json.Unmarshal(raw, &p) != nil || len(p.ContentChanges) == 0 {
		return
	}
	key := canonical(p.TextDocument.URI)
	s.dmu.Lock()
	if s.closed {
		s.dmu.Unlock()
		return
	}
	d := s.docs[key]
	if d != nil && d.pinned {
		s.dmu.Unlock()
		return
	}
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
	if err := s.fits(key, len(text)); err != nil {
		s.dmu.Unlock()
		s.refuse(p.TextDocument.URI, err)
		return
	}
	n := &source{key: key, uri: p.TextDocument.URI, text: text, version: p.TextDocument.Version, open: true}
	if d != nil {
		n.lang = d.lang
	} else {
		n.lang = languageOf(n.uri, "")
	}
	s.store(n)
	s.scheduleLocked()
	s.dmu.Unlock()
	s.interrupt()
}

func (s *Session) didClose(ctx context.Context, raw json.RawMessage) {
	var p didCloseParams
	if json.Unmarshal(raw, &p) != nil {
		return
	}
	key := canonical(p.TextDocument.URI)
	s.dmu.Lock()
	d := s.docs[key]
	if s.closed || d == nil || d.pinned {
		s.dmu.Unlock()
		return
	}
	uri := d.uri
	s.dmu.Unlock()
	s.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{uri, []Diagnostic{}})
	var text string
	var onDisk bool
	if files := s.h.opts.Files; files != nil {
		text, onDisk = files.Read(ctx, uri)
	}
	s.dmu.Lock()
	if cur := s.docs[key]; cur != nil && !cur.pinned && !s.closed {
		if onDisk && s.fits(key, len(text)) == nil {
			s.store(&source{key: key, uri: uri, text: text, lang: cur.lang}) // back to what the file holds
		} else {
			s.dropLocked(key)
		}
		s.scheduleLocked()
	}
	s.dmu.Unlock()
	s.interrupt()
}

// refuse tells the client that a document was not taken.
func (s *Session) refuse(uri string, err error) {
	s.log.Warn("document refused", "uri", uri, "err", err)
	s.notify("window/showMessage", map[string]any{"type": 2, "message": fmt.Sprintf("%s was not taken: %v", uri, err)})
}
