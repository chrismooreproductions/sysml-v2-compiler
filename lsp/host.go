// SPDX-License-Identifier: EPL-2.0

// Package lsp is a Language Server Protocol server for SysML v2 and KerML, to
// run inside another program: any number of sessions in one process, sharing
// the model of the standard library, with their work bounded by the host.
//
// A Host owns what sessions share: the limits, a limit on how many of them
// compute at once, the logger and the hooks. A Session serves one client. The
// documents of a session are the ones its program sets with SetDocument, which
// the client can read but not change, and the ones the client opens; the
// package reads no files, and ignores the root and workspace folders a client
// announces (the command line tool passes Options.Files to read the disk).
//
//	host := lsp.NewHost(lsp.Options{})
//	s, _ := host.NewSession()
//	s.SetDocument("file:///imports/1.sysml", text, sysml.SysML)
//	err := s.ServeMessages(ctx, conn) // or s.Serve(ctx, readWriter)
//
// Positions in a go to definition result use the URI the document was given.
// A standard library file is sent as the URI Options.LibraryURI makes of its
// path (by default "sysml-library:///Systems Library/Parts.sysml", see
// LibraryURI), which the client displays from LibraryText or its own copy.
//
// The package keeps no state of its own between hosts, imports nothing that
// touches the operating system and builds for GOOS=js GOARCH=wasm.
package lsp

import (
	"context"
	"io"
	"log/slog"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/library"
	"github.com/chrismooreproductions/sysml-v2-compiler/sysml"
)

// A Host runs sessions. Its methods are safe to call from any goroutine.
type Host struct {
	opts Options
	log  *slog.Logger
	sem  chan struct{} // the compute slots, nil if unbounded
	next atomic.Int64

	beforeResolve func(*Session) // for tests: called before each resolution

	mu       sync.Mutex
	sessions map[*Session]struct{}
	libDocs  map[string]*document
}

// NewHost returns a host. See Options for the defaults.
func NewHost(opts Options) *Host {
	if opts.MaxDocuments == 0 {
		opts.MaxDocuments = 200
	}
	if opts.MaxBytes == 0 {
		opts.MaxBytes = 8 << 20
	}
	if opts.MaxMessageBytes == 0 {
		opts.MaxMessageBytes = 16 << 20
	}
	if opts.MaxInFlight == 0 {
		opts.MaxInFlight = 64
	}
	if opts.Concurrency == 0 {
		opts.Concurrency = runtime.GOMAXPROCS(0)
	}
	if opts.Debounce == 0 {
		opts.Debounce = 150 * time.Millisecond
	}
	if opts.LibraryURI == nil {
		opts.LibraryURI = LibraryURI
	}
	h := &Host{opts: opts, log: opts.Logger, sessions: map[*Session]struct{}{}, libDocs: map[string]*document{}}
	if h.log == nil {
		h.log = slog.New(slog.DiscardHandler)
	}
	if opts.Concurrency > 0 {
		h.sem = make(chan struct{}, opts.Concurrency)
	}
	return h
}

// acquire takes a compute slot, waiting for one; release gives it back.
func (h *Host) acquire(ctx context.Context) error {
	if h.sem == nil {
		return ctx.Err()
	}
	select {
	case h.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *Host) release() {
	if h.sem != nil {
		<-h.sem
	}
}

// NewSession starts a session. Close it, or serve a connection with it, which
// closes it when the connection ends.
func (h *Host) NewSession() (*Session, error) {
	s, err := newSession(h)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	h.sessions[s] = struct{}{}
	h.mu.Unlock()
	if f := h.opts.Hooks.SessionOpened; f != nil {
		f(s.info())
	}
	h.log.Info("session opened", "session", s.id)
	return s, nil
}

// Serve serves one client on a stream with a new session, until the client
// exits, the stream ends or ctx is done. See Session.Serve.
func (h *Host) Serve(ctx context.Context, rw io.ReadWriter) error {
	s, err := h.NewSession()
	if err != nil {
		return err
	}
	return s.Serve(ctx, rw)
}

// ServeMessages is Serve over a MessageConn.
func (h *Host) ServeMessages(ctx context.Context, c MessageConn) error {
	s, err := h.NewSession()
	if err != nil {
		return err
	}
	return s.ServeMessages(ctx, c)
}

// Stats is what the host's sessions hold now.
func (h *Host) Stats() Stats {
	h.mu.Lock()
	sessions := make([]*Session, 0, len(h.sessions))
	for s := range h.sessions {
		sessions = append(sessions, s)
	}
	h.mu.Unlock()
	st := Stats{Sessions: len(sessions)}
	for _, s := range sessions {
		x := s.Stats()
		st.Documents += x.Documents
		st.Bytes += x.Bytes
	}
	return st
}

func (h *Host) forget(s *Session) {
	h.mu.Lock()
	delete(h.sessions, s)
	h.mu.Unlock()
}

// libraryDoc returns a standard library file as a document, shared by the
// host's sessions. Its line table is built here and not changed after, so
// sessions can read it at the same time.
func (h *Host) libraryDoc(path string) *document {
	h.mu.Lock()
	defer h.mu.Unlock()
	if d, ok := h.libDocs[path]; ok {
		return d
	}
	text, ok := library.Source(path)
	if !ok {
		return nil
	}
	d := &document{key: "library:" + path, uri: h.opts.LibraryURI(path), text: text, readOnly: true}
	d.lineStarts()
	h.libDocs[path] = d
	return d
}

// LibraryURI is the address of a standard library file, given its path
// relative to the library root ("Systems Library/Parts.sysml"):
// sysml-library:///Systems%20Library/Parts.sysml.
func LibraryURI(path string) string {
	return (&url.URL{Scheme: "sysml-library", Path: "/" + path}).String()
}

// LibraryPath is the path in the standard library that a URI made by LibraryURI
// stands for, and whether it is one.
func LibraryPath(uri string) (string, bool) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "sysml-library" || len(u.Path) < 2 {
		return "", false
	}
	return strings.TrimPrefix(u.Path, "/"), true
}

// LibraryText is the text of a standard library file, by the path LibraryPath
// gives, so that a program can serve the files the client is sent to.
func LibraryText(path string) (string, bool) { return library.Source(path) }

// canonical is the form of a URI that documents are keyed by, so that two
// spellings of one address are one document.
func canonical(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	return u.String()
}

// languageOf chooses a document's language: by its address's extension, then
// by the language the client names, and SysML if neither says.
func languageOf(uri, languageID string) sysml.Language {
	path := uri
	if u, err := url.Parse(uri); err == nil {
		path = u.Path
	}
	if l, err := sysml.LanguageOf(path); err == nil {
		return l
	}
	if strings.EqualFold(languageID, "kerml") {
		return sysml.KerML
	}
	return sysml.SysML
}
