// SPDX-License-Identifier: EPL-2.0

package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"weak"

	"github.com/chrismooreproductions/sysml-v2-compiler/sysml"
)

// shutdownAndWait ends the conversation the way an editor does, and waits for
// the session to finish.
func (c *client) shutdownAndWait() {
	c.t.Helper()
	c.call("shutdown", nil)
	c.notify("exit", nil)
	select {
	case <-c.done:
	case <-time.After(10 * time.Second):
		c.t.Fatal("the server did not exit")
	}
	c.in.Close()
}

// reply sends a request and returns its response, errors included.
func (c *client) reply(method string, params any) *message {
	c.t.Helper()
	m, ch := c.begin(method, params)
	select {
	case r := <-ch:
		return r
	case <-time.After(20 * time.Second):
		c.t.Fatalf("%s: no reply to request %s", method, m)
	}
	return nil
}

// begin sends a request and returns the channel its response will arrive on.
func (c *client) begin(method string, params any) (string, chan *message) {
	c.nextID++
	id := c.nextID
	ch := make(chan *message, 1)
	c.mu.Lock()
	c.replies[jsonID(id)] = ch
	c.mu.Unlock()
	c.send(map[string]any{"id": id, "method": method, "params": params})
	return jsonID(id), ch
}

func jsonID(id int) string { b, _ := json.Marshal(id); return string(b) }

func TestAHandlerThatPanicsFailsOneRequest(t *testing.T) {
	var panics, calls atomic.Int32
	h := NewHost(Options{Debounce: time.Hour, Hooks: Hooks{
		Panic:    func(SessionInfo, *PanicError) { panics.Add(1) },
		Resolved: func(SessionInfo, ResolveInfo) { calls.Add(1) },
	}})
	h.opts.Hooks.Resolved = func(SessionInfo, ResolveInfo) {
		if calls.Add(1) == 1 {
			panic("boom")
		}
	}
	c := startOn(t, h, nil)
	c.initialize()
	uri := "file:///work/fleet.sysml"
	c.open(uri, fleet)
	r := c.reply("textDocument/hover", at(uri, 4, 16))
	if r.Error == nil || !strings.Contains(r.Error.Message, "boom") {
		t.Fatalf("the request that met the panic: %+v", r)
	}
	var hv Hover
	json.Unmarshal(c.call("textDocument/hover", at(uri, 4, 16)), &hv) // the next one is served
	if !strings.Contains(hv.Contents.Value, "Fleet::Truck") {
		t.Errorf("after the panic: %q", hv.Contents.Value)
	}
	if panics.Load() != 1 {
		t.Errorf("panic hook calls: %d", panics.Load())
	}
}

func TestFailOnPanicEndsTheSession(t *testing.T) {
	var closed CloseInfo
	h := NewHost(Options{Debounce: time.Hour, FailOnPanic: true, Hooks: Hooks{
		Resolved:      func(SessionInfo, ResolveInfo) { panic("boom") },
		SessionClosed: func(_ SessionInfo, ci CloseInfo) { closed = ci },
	}})
	c := startOn(t, h, nil)
	c.initialize()
	uri := "file:///work/fleet.sysml"
	c.open(uri, fleet)
	c.begin("textDocument/hover", at(uri, 4, 16)) // its reply may be lost: the session ends
	var pe *PanicError
	select {
	case err := <-c.done:
		if !errors.As(err, &pe) {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the session did not end")
	}
	c.in.Close()
	if closed.Reason != "panic" || h.Stats().Sessions != 0 {
		t.Errorf("close: %+v, sessions %d", closed, h.Stats().Sessions)
	}
}

// One session cannot compute while another holds the host's only slot, and
// a request that waits for the slot can be cancelled.
func TestTheHostBoundsComputationAndRequestsCanBeCancelled(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	h := NewHost(Options{Concurrency: 1, Debounce: time.Millisecond})
	h.beforeResolve = func(*Session) {
		once.Do(func() { close(entered); <-release })
	}
	a := startOn(t, h, nil)
	a.initialize()
	a.open("file:///work/a.sysml", fleet)
	select {
	case <-entered: // a holds the slot
	case <-time.After(10 * time.Second):
		t.Fatal("the first session never started to compute")
	}
	b := startOn(t, h, nil)
	b.initialize()
	uri := "file:///work/b.sysml"
	b.open(uri, fleet)
	id, ch := b.begin("textDocument/hover", at(uri, 4, 16))
	select {
	case r := <-ch:
		t.Fatalf("answered while the host was busy: %+v", r)
	case <-time.After(300 * time.Millisecond):
	}
	b.notify("$/cancelRequest", map[string]any{"id": 2})
	_ = id
	select {
	case r := <-ch:
		if r.Error == nil || r.Error.Code != -32800 {
			t.Errorf("a cancelled request: %+v", r)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the cancelled request was not answered")
	}
	close(release)
	var hv Hover
	json.Unmarshal(b.call("textDocument/hover", at(uri, 4, 16)), &hv)
	if !strings.Contains(hv.Contents.Value, "Fleet::Truck") {
		t.Errorf("after the slot was free: %q", hv.Contents.Value)
	}
}

func TestAnEditAbandonsTheResolutionItMakesObsolete(t *testing.T) {
	var got []ResolveInfo
	var mu sync.Mutex
	entered, proceed := make(chan struct{}), make(chan struct{})
	var once sync.Once
	h := NewHost(Options{Debounce: time.Millisecond, Hooks: Hooks{
		Resolved: func(_ SessionInfo, ri ResolveInfo) { mu.Lock(); got = append(got, ri); mu.Unlock() },
	}})
	h.beforeResolve = func(*Session) { once.Do(func() { close(entered); <-proceed }) }
	c := startOn(t, h, nil)
	c.initialize()
	uri := "file:///work/fleet.sysml"
	c.open(uri, strings.Replace(fleet, "part rig : Truck;", "part rig : Trck;", 1))
	<-entered // resolving the first text
	c.change(uri, 2, fleet)
	time.Sleep(100 * time.Millisecond) // the edit has been read, and has stopped the resolution
	close(proceed)
	c.diagnostics(uri, none) // the diagnostics are those of the new text
	mu.Lock()
	defer mu.Unlock()
	if len(got) < 2 || !got[0].Cancelled || got[len(got)-1].Cancelled || got[0].Documents != 1 {
		t.Errorf("resolutions: %+v", got)
	}
}

func TestClosingASessionReleasesItsWorkspace(t *testing.T) {
	h := NewHost(Options{Debounce: time.Millisecond})
	before := runtime.NumGoroutine()
	c := startOn(t, h, nil)
	c.initialize()
	uri := "file:///work/fleet.sysml"
	c.open(uri, fleet)
	c.diagnostics(uri, none)
	c.srv.wmu.Lock()
	w := weak.Make(c.srv.ws)
	c.srv.wmu.Unlock()
	c.shutdownAndWait()
	if st := h.Stats(); st.Sessions != 0 || st.Documents != 0 || st.Bytes != 0 {
		t.Errorf("after the session ended: %+v", st)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && (w.Value() != nil || runtime.NumGoroutine() > before) {
		runtime.GC()
		time.Sleep(20 * time.Millisecond)
	}
	if w.Value() != nil {
		t.Error("the workspace is still alive after the session closed")
	}
	if n := runtime.NumGoroutine(); n > before {
		buf := make([]byte, 1<<16)
		t.Errorf("goroutines: %d before, %d after\n%s", before, n, buf[:runtime.Stack(buf, true)])
	}
}

type chanConn struct{ in, out chan []byte }

func (c *chanConn) ReadMessage(ctx context.Context) ([]byte, error) {
	select {
	case m, ok := <-c.in:
		if !ok {
			return nil, io.EOF
		}
		return m, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *chanConn) WriteMessage(ctx context.Context, m []byte) error {
	select {
	case c.out <- m:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestServeMessagesCarriesOneMessageEach(t *testing.T) {
	var closed CloseInfo
	h := NewHost(Options{Debounce: time.Millisecond, Hooks: Hooks{
		SessionClosed: func(_ SessionInfo, ci CloseInfo) { closed = ci },
	}})
	conn := &chanConn{make(chan []byte, 10), make(chan []byte, 100)}
	done := make(chan error, 1)
	go func() { done <- h.ServeMessages(context.Background(), conn) }()
	conn.in <- []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}`)
	if m := string(<-conn.out); !strings.HasPrefix(m, `{"id":1`) && !strings.Contains(m, `"hoverProvider":true`) || strings.Contains(m, "Content-Length") {
		t.Errorf("the reply: %s", m)
	}
	conn.in <- []byte(`{"jsonrpc":"2.0","method":"exit"}`)
	if err := <-done; err != nil {
		t.Errorf("ServeMessages: %v", err)
	}
	if closed.Reason != "exit" {
		t.Errorf("close: %+v", closed)
	}
}

func TestServeStopsWhenTheContextEnds(t *testing.T) {
	var closed CloseInfo
	h := NewHost(Options{Hooks: Hooks{SessionClosed: func(_ SessionInfo, ci CloseInfo) { closed = ci }}})
	conn := &chanConn{make(chan []byte), make(chan []byte, 10)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.ServeMessages(ctx, conn) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("ServeMessages: %v", err)
	}
	if closed.Reason != "canceled" || h.Stats().Sessions != 0 {
		t.Errorf("close: %+v", closed)
	}
}

func TestClosingASessionThatIsServingEndsIt(t *testing.T) {
	h := NewHost(Options{})
	s, _ := h.NewSession()
	conn := &chanConn{make(chan []byte), make(chan []byte, 10)}
	done := make(chan error, 1)
	go func() { done <- s.ServeMessages(context.Background(), conn) }()
	time.Sleep(50 * time.Millisecond)
	s.Close()
	s.Close() // twice is fine
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("ServeMessages: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not end")
	}
	if err := s.SetDocument("file:///a.sysml", "package A;", sysml.SysML); err != ErrClosed {
		t.Errorf("SetDocument after Close: %v", err)
	}
}

func TestLibraryAddresses(t *testing.T) {
	uri := LibraryURI("Systems Library/Parts.sysml")
	if uri != "sysml-library:///Systems%20Library/Parts.sysml" {
		t.Errorf("LibraryURI = %s", uri)
	}
	if p, ok := LibraryPath(uri); !ok || p != "Systems Library/Parts.sysml" {
		t.Errorf("LibraryPath = %q, %v", p, ok)
	}
	if text, ok := LibraryText("Systems Library/Parts.sysml"); !ok || !strings.Contains(text, "part def Part") && !strings.Contains(text, "Part") {
		t.Errorf("LibraryText: %v", ok)
	}
	if _, ok := LibraryPath("file:///a.sysml"); ok {
		t.Error("a file URI is not a library address")
	}
}

func TestAnalysisThatRunsOutOfTimeIsAbandonedAndNotRetried(t *testing.T) {
	var calls atomic.Int32
	var mu sync.Mutex
	var infos []ResolveInfo
	h := NewHost(Options{ResolveTimeout: 50 * time.Millisecond, Debounce: time.Millisecond, Hooks: Hooks{
		Resolved: func(_ SessionInfo, ri ResolveInfo) { mu.Lock(); infos = append(infos, ri); mu.Unlock() },
	}})
	h.beforeResolve = func(*Session) {
		if calls.Add(1) == 1 {
			time.Sleep(150 * time.Millisecond) // a resolution that takes too long
		}
	}
	c := startOn(t, h, nil)
	c.initialize()
	uri := "file:///work/fleet.sysml"
	c.open(uri, fleet)
	d := c.diagnostics(uri, some)
	if len(d) != 1 || !strings.Contains(d[0].Message, "timed out") {
		t.Fatalf("the diagnostic: %+v", d)
	}
	r := c.reply("textDocument/hover", at(uri, 4, 16))
	if r.Error == nil || r.Error.Code != -32801 {
		t.Errorf("a request for the same text: %+v", r)
	}
	if calls.Load() != 1 {
		t.Errorf("the same text was tried again: %d resolutions", calls.Load())
	}
	c.change(uri, 2, fleet+"\n") // a change allows another try
	c.diagnostics(uri, none)
	var hv Hover
	json.Unmarshal(c.call("textDocument/hover", at(uri, 4, 16)), &hv)
	if !strings.Contains(hv.Contents.Value, "Fleet::Truck") {
		t.Errorf("after the change: %q", hv.Contents.Value)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(infos) < 2 || !infos[0].TimedOut || infos[len(infos)-1].TimedOut {
		t.Errorf("resolutions: %+v", infos)
	}
}
