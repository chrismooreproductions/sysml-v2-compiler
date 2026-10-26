// SPDX-License-Identifier: EPL-2.0

package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chrismooreproductions/sysml-v2-compiler/sysml"
)

// Errors SetDocument returns, and a document the client sends meets, when a
// limit of the host is reached.
var (
	ErrTooManyDocuments = errors.New("lsp: the session holds as many documents as the host allows")
	ErrTooMuchText      = errors.New("lsp: the session holds as much text as the host allows")
	ErrClosed           = errors.New("lsp: the session is closed")

	errInterrupted = errors.New("lsp: a newer edit made the work obsolete")
	errTimedOut    = errors.New("lsp: analysis timed out")
)

func toString(v any) string { return fmt.Sprint(v) }

// source is a document in the table the program and the client edit.
type source struct {
	key, uri string
	text     string
	version  int
	lang     sysml.Language
	pinned   bool // set by the host: the client cannot change or close it
	open     bool // the client has it open: its diagnostics are published
	gen      uint64
}

// A Session serves one client. It is safe to call its methods from any
// goroutine. The documents are only parsed and resolved when something needs
// them, so setting a document is cheap.
type Session struct {
	h     *Host
	id    int64
	start time.Time
	log   *slog.Logger

	ctx    context.Context // ends when the session is closed
	cancel context.CancelFunc
	wg     sync.WaitGroup // the goroutines of requests and publishing

	dmu      sync.Mutex // guards the document table
	docs     map[string]*source
	removed  map[string]bool // documents dropped since the workspace was updated
	bytes    int64
	gens     uint64
	closed   bool
	timer    *time.Timer
	inflight map[string]context.CancelFunc

	wmu      sync.Mutex // guards the workspace and what is derived from it
	ws       *sysml.Workspace
	snaps    map[string]*document // the documents as last parsed
	diags    []sysml.Diagnostic
	resolved bool        // the workspace is resolved for the snapshots
	timedOut uint64      // the generation of the documents at which resolving ran out of time
	tainted  atomic.Bool // work on the workspace was abandoned or panicked: rebuild it

	cmu        sync.Mutex // guards cancelSync
	cancelSync context.CancelFunc

	conn     MessageConn
	sctx     context.Context
	scancel  context.CancelFunc
	writeMu  sync.Mutex
	shutdown atomic.Bool
	exit     atomic.Bool
	fatalMu  sync.Mutex
	fatalErr error

	closeOnce sync.Once
	reason    string
	reasonErr error
}

func newSession(h *Host) (*Session, error) {
	ws, err := sysml.NewWorkspace()
	if err != nil {
		return nil, err
	}
	id := h.next.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	return &Session{
		h: h, id: id, start: time.Now(), log: h.log.With("session", id),
		ctx: ctx, cancel: cancel,
		docs: map[string]*source{}, removed: map[string]bool{}, inflight: map[string]context.CancelFunc{},
		ws: ws, snaps: map[string]*document{},
	}, nil
}

func (s *Session) info() SessionInfo { return SessionInfo{ID: s.id} }

// SetDocument gives the session a document: a file the program supplies, which
// the client can use in its models and navigate to but not change, whatever
// the client opens or closes under that URI. Setting it again replaces its
// text. It returns an error, and changes nothing, if a limit of the host would
// be exceeded.
func (s *Session) SetDocument(uri, text string, lang sysml.Language) error {
	key := canonical(uri)
	s.dmu.Lock()
	if s.closed {
		s.dmu.Unlock()
		return ErrClosed
	}
	if err := s.fits(key, len(text)); err != nil {
		s.dmu.Unlock()
		return err
	}
	wasOpen := false
	if d := s.docs[key]; d != nil {
		wasOpen = d.open && !d.pinned
	}
	s.store(&source{key: key, uri: uri, text: text, lang: lang, pinned: true})
	s.scheduleLocked()
	s.dmu.Unlock()
	s.interrupt()
	if wasOpen {
		s.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{uri, []Diagnostic{}})
	}
	return nil
}

// RemoveDocument drops a document, whoever set it.
func (s *Session) RemoveDocument(uri string) {
	key := canonical(uri)
	s.dmu.Lock()
	if s.closed || s.docs[key] == nil {
		s.dmu.Unlock()
		return
	}
	s.dropLocked(key)
	s.scheduleLocked()
	s.dmu.Unlock()
	s.interrupt()
}

// Stats is what the session holds now.
func (s *Session) Stats() Stats {
	s.dmu.Lock()
	defer s.dmu.Unlock()
	return Stats{Sessions: 1, Documents: len(s.docs), Bytes: s.bytes}
}

// fits says whether a document of a size can be held. The caller holds dmu.
func (s *Session) fits(key string, n int) error {
	old, exists := 0, false
	if d := s.docs[key]; d != nil {
		old, exists = len(d.text), true
	}
	if lim := s.h.opts.MaxDocuments; lim > 0 && !exists && len(s.docs) >= lim {
		return ErrTooManyDocuments
	}
	if lim := s.h.opts.MaxBytes; lim > 0 && s.bytes-int64(old)+int64(n) > int64(lim) {
		return ErrTooMuchText
	}
	return nil
}

// store puts a document in the table. The caller holds dmu.
func (s *Session) store(n *source) {
	if old := s.docs[n.key]; old != nil {
		s.bytes -= int64(len(old.text))
	}
	s.gens++
	n.gen = s.gens
	s.bytes += int64(len(n.text))
	s.docs[n.key] = n
}

func (s *Session) dropLocked(key string) {
	if d := s.docs[key]; d != nil {
		s.gens++
		s.bytes -= int64(len(d.text))
		delete(s.docs, key)
		s.removed[key] = true
	}
}

// scheduleLocked publishes diagnostics once the documents have been quiet for a
// moment. The caller holds dmu.
func (s *Session) scheduleLocked() {
	if s.closed {
		return
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.h.opts.Debounce, func() { s.spawn("publish", s.publish) })
}

// interrupt abandons the resolution in progress, which an edit has made
// obsolete.
func (s *Session) interrupt() {
	s.cmu.Lock()
	if s.cancelSync != nil {
		s.cancelSync()
	}
	s.cmu.Unlock()
}

// spawn runs fn in a goroutine that Close waits for, and recovers its panic.
func (s *Session) spawn(where string, fn func()) bool {
	s.dmu.Lock()
	if s.closed {
		s.dmu.Unlock()
		return false
	}
	s.wg.Add(1)
	s.dmu.Unlock()
	go func() {
		defer s.wg.Done()
		s.safely(where, fn)
	}()
	return true
}

// safely calls fn and turns a panic into a *PanicError, reported to the log and
// the hook. The workspace may be half changed by it, so it is rebuilt.
func (s *Session) safely(where string, fn func()) (pe *PanicError) {
	defer func() {
		if r := recover(); r != nil {
			pe = &PanicError{Where: where, Value: r, Stack: debug.Stack()}
			s.tainted.Store(true)
			s.log.Error("panic", "where", where, "value", toString(r), "stack", string(pe.Stack))
			if f := s.h.opts.Hooks.Panic; f != nil {
				f(s.info(), pe)
			}
			if s.h.opts.FailOnPanic {
				s.fatal(pe)
			}
		}
	}()
	fn()
	return nil
}

func (s *Session) fatal(err error) {
	s.fatalMu.Lock()
	if s.fatalErr == nil {
		s.fatalErr = err
	}
	s.fatalMu.Unlock()
	if s.scancel != nil {
		s.scancel()
	}
}

func (s *Session) fatalError() error {
	s.fatalMu.Lock()
	defer s.fatalMu.Unlock()
	return s.fatalErr
}

// Close ends the session and releases its documents and workspace. It is safe
// to call more than once, and while the session is serving.
func (s *Session) Close() { s.close("closed", nil) }

func (s *Session) close(reason string, err error) {
	s.closeOnce.Do(func() {
		s.reason, s.reasonErr = reason, err
		s.dmu.Lock()
		s.closed = true
		if s.timer != nil {
			s.timer.Stop()
		}
		s.dmu.Unlock()
		s.cancel()
		s.interrupt()
		s.wg.Wait()
		s.wmu.Lock()
		s.ws, s.snaps, s.diags = nil, nil, nil
		s.wmu.Unlock()
		s.dmu.Lock()
		s.docs, s.removed, s.inflight, s.bytes = map[string]*source{}, map[string]bool{}, map[string]context.CancelFunc{}, 0
		s.dmu.Unlock()
		s.h.forget(s)
		d := time.Since(s.start)
		s.log.Info("session closed", "reason", reason, "duration", d)
		if f := s.h.opts.Hooks.SessionClosed; f != nil {
			f(s.info(), CloseInfo{Reason: reason, Duration: d, Err: err})
		}
	})
}

// syncLocked brings the workspace up to date with the documents: it parses the
// ones that changed and resolves and validates the whole. The caller holds wmu
// and a compute slot. An edit that arrives meanwhile abandons the work and
// returns errInterrupted; ctx ending returns its error.
func (s *Session) syncLocked(ctx context.Context) error {
	s.dmu.Lock()
	gen := s.gens
	s.dmu.Unlock()
	if s.timedOut != 0 && s.timedOut == gen {
		return errTimedOut // the same text again: it would take as long
	}
	if s.tainted.Swap(false) {
		ws, err := sysml.NewWorkspace()
		if err != nil {
			return err
		}
		s.ws, s.snaps, s.resolved = ws, map[string]*document{}, false
	}
	if s.ws == nil {
		return ErrClosed
	}
	var jobs []source
	s.dmu.Lock()
	if s.closed {
		s.dmu.Unlock()
		return ErrClosed
	}
	for _, d := range s.docs {
		if sn := s.snaps[d.key]; sn == nil || sn.gen != d.gen || sn.readOnly != d.pinned {
			jobs = append(jobs, *d)
		}
	}
	removed := s.removed
	s.removed = map[string]bool{}
	s.dmu.Unlock()
	if len(jobs) == 0 && len(removed) == 0 && s.resolved {
		return nil
	}
	s.resolved = false

	bctx, bcancel := ctx, context.CancelFunc(func() {})
	if d := s.h.opts.ResolveTimeout; d > 0 {
		bctx, bcancel = context.WithTimeout(ctx, d)
	}
	defer bcancel()
	sctx, cancel := context.WithCancel(bctx)
	defer cancel()
	s.cmu.Lock()
	s.cancelSync = cancel
	s.cmu.Unlock()
	defer func() {
		s.cmu.Lock()
		s.cancelSync = nil
		s.cmu.Unlock()
	}()
	stopped := func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if sctx.Err() != nil {
			if errors.Is(sctx.Err(), context.DeadlineExceeded) {
				return errTimedOut
			}
			return errInterrupted
		}
		return nil
	}

	for k := range removed {
		s.ws.Remove(k)
		delete(s.snaps, k)
	}
	for _, j := range jobs {
		if err := stopped(); err != nil {
			if err == errTimedOut {
				s.timedOut = gen
				s.tainted.Store(true)
			}
			return err
		}
		_, errs := s.ws.ReplaceAs(j.lang, j.key, []byte(j.text))
		s.snaps[j.key] = &document{key: j.key, uri: j.uri, text: j.text, gen: j.gen, readOnly: j.pinned, errs: errs}
	}
	if f := s.h.beforeResolve; f != nil {
		f(s)
	}
	start := time.Now()
	diags, err := s.ws.ResolveContext(sctx)
	if err == nil {
		var v []sysml.Diagnostic
		if v, err = s.ws.ValidateContext(sctx); err == nil {
			diags = append(diags, v...)
		}
	}
	info := ResolveInfo{Duration: time.Since(start), Cancelled: err != nil, Documents: len(s.snaps)}
	info.TimedOut = err != nil && ctx.Err() == nil && errors.Is(sctx.Err(), context.DeadlineExceeded)
	if f := s.h.opts.Hooks.Resolved; f != nil {
		f(s.info(), info)
	}
	s.log.Debug("resolved", "duration", info.Duration, "cancelled", info.Cancelled, "documents", info.Documents)
	if err != nil {
		s.tainted.Store(true) // what the memos hold may reflect references not yet resolved
		if e := stopped(); e != nil {
			if e == errTimedOut {
				s.timedOut = gen
				s.log.Warn("analysis timed out", "after", s.h.opts.ResolveTimeout)
			}
			return e
		}
		return err
	}
	s.diags, s.resolved = diags, true
	return nil
}

// withWorkspace calls fn with the workspace up to date, holding the lock on it
// and one of the host's compute slots. An edit that arrives while the
// workspace is being brought up to date makes it start again with the new text.
func (s *Session) withWorkspace(ctx context.Context, fn func() (any, error)) (any, error) {
	for {
		res, err, retry := s.attempt(ctx, fn)
		if !retry {
			return res, err
		}
	}
}

func (s *Session) attempt(ctx context.Context, fn func() (any, error)) (res any, err error, retry bool) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if err := s.h.acquire(ctx); err != nil {
		return nil, err, false
	}
	defer s.h.release()
	if err := s.syncLocked(ctx); err != nil {
		return nil, err, errors.Is(err, errInterrupted)
	}
	res, err = fn()
	return res, err, false
}

// publish sends the diagnostics of every document the client has open.
func (s *Session) publish() {
	if s.shutdown.Load() {
		return
	}
	var out []publishDiagnosticsParams
	_, err := s.withWorkspace(s.ctx, func() (any, error) {
		s.dmu.Lock()
		var open []*source
		for _, d := range s.docs {
			if d.open && !d.pinned {
				open = append(open, d)
			}
		}
		s.dmu.Unlock()
		for _, d := range open {
			if sn := s.snaps[d.key]; sn != nil {
				out = append(out, publishDiagnosticsParams{d.uri, s.diagnosticsFor(sn)})
			}
		}
		return nil, nil
	})
	if errors.Is(err, errTimedOut) {
		s.dmu.Lock()
		for _, d := range s.docs {
			if d.open && !d.pinned {
				out = append(out, publishDiagnosticsParams{d.uri, []Diagnostic{{
					Severity: 1, Source: "sysml",
					Message: fmt.Sprintf("analysis timed out after %v; change the model to try again", s.h.opts.ResolveTimeout),
				}}})
			}
		}
		s.dmu.Unlock()
	} else if err != nil {
		return
	}
	for _, p := range out {
		s.notify("textDocument/publishDiagnostics", p)
	}
}

// diagnosticsFor lists the problems in a document: its syntax errors, or if it
// has none, its unresolved names and constraint violations.
func (s *Session) diagnosticsFor(d *document) []Diagnostic {
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
		if g.File != d.key {
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

// Serve serves a client on a stream: messages with a Content-Length header, as
// an editor speaks over standard input and output. It returns nil when the
// client exits or the stream ends, ctx's error if ctx ends first, and any
// other error that stopped it, among them a *PanicError. The session is closed
// when it returns. A read that ctx has stopped waiting for goes on until the
// stream returns, so close the stream to end it.
func (s *Session) Serve(ctx context.Context, rw io.ReadWriter) error {
	f := newFramed(rw, s.h.opts.MaxMessageBytes)
	defer f.close()
	return s.ServeMessages(ctx, f)
}

// ServeMessages is Serve over a connection that carries whole messages.
func (s *Session) ServeMessages(ctx context.Context, c MessageConn) error {
	s.dmu.Lock()
	if s.conn != nil || s.closed {
		s.dmu.Unlock()
		return errors.New("lsp: the session is serving, or closed")
	}
	sctx, scancel := context.WithCancel(ctx)
	s.conn, s.sctx, s.scancel = c, sctx, scancel
	s.dmu.Unlock()
	stop := context.AfterFunc(s.ctx, scancel)
	defer stop()

	reason, err := "eof", error(nil)
loop:
	for {
		raw, rerr := c.ReadMessage(sctx)
		if ferr := s.fatalError(); ferr != nil {
			reason, err = "panic", ferr
			if _, ok := ferr.(*PanicError); !ok {
				reason = "error"
			}
			break
		}
		switch {
		case rerr == nil:
		case errors.Is(rerr, io.EOF):
			break loop
		case s.ctx.Err() != nil:
			reason = "closed"
			break loop
		case ctx.Err() != nil:
			reason, err = "canceled", ctx.Err()
			break loop
		default:
			reason, err = "error", rerr
			break loop
		}
		var m message
		if jerr := json.Unmarshal(raw, &m); jerr != nil {
			s.write(&message{Error: &responseError{-32700, "parse error: " + jerr.Error()}})
			continue
		}
		s.dispatch(sctx, &m)
		if s.exit.Load() {
			reason = "exit"
			break
		}
	}
	scancel()
	s.close(reason, err)
	return err
}
