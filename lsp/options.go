// SPDX-License-Identifier: EPL-2.0

package lsp

import (
	"context"
	"log/slog"
	"time"
)

// Options configure a Host. The zero value is usable and applies the defaults
// noted on each field; a negative limit means no limit.
type Options struct {
	// MaxDocuments is how many documents a session may hold, those the host
	// sets and those the client opens together. Default 200.
	MaxDocuments int
	// MaxBytes is how many bytes of text a session may hold in all. Default 8 MiB.
	MaxBytes int
	// MaxMessageBytes is the largest message the server accepts from a client
	// over a framed stream. Default 16 MiB.
	MaxMessageBytes int
	// MaxInFlight is how many requests of one session may be unanswered at
	// once; further ones are refused. Default 64.
	MaxInFlight int
	// Concurrency bounds how many sessions of the host may be parsing,
	// resolving or answering at once, so that many sessions cannot use more of
	// the CPU than that. Default runtime.GOMAXPROCS(0); negative for no bound.
	Concurrency int
	// Debounce is how long a document is quiet before its diagnostics are
	// published. Default 150 ms.
	Debounce time.Duration
	// FailOnPanic ends a session whose request handler panics, with a
	// *PanicError, instead of answering the request with an error and carrying on.
	FailOnPanic bool
	// Logger receives the host's logs. Default: none.
	Logger *slog.Logger
	// Hooks are called as sessions open and close and work happens.
	Hooks Hooks
	// Files lets the server see files outside the client's documents, as the
	// command line tool does with the disk. Default none: the server then knows
	// only the documents the host sets and the client opens, and ignores the
	// client's root and workspace folders.
	Files Files
	// LibraryURI says where an editor can open a standard library file, given
	// its path relative to the library root. Default: LibraryURI, an address
	// that no file system holds; the client has to fetch the text itself.
	LibraryURI func(path string) string
}

// Hooks observe a host. Each may be nil, and is called from the goroutine that
// did the work, so it must be quick and must not call back into the session.
type Hooks struct {
	SessionOpened func(SessionInfo)
	SessionClosed func(SessionInfo, CloseInfo)
	Resolved      func(SessionInfo, ResolveInfo)
	Panic         func(SessionInfo, *PanicError)
}

// SessionInfo identifies a session in the hooks.
type SessionInfo struct {
	ID int64
}

// CloseInfo says why a session ended and how long it lasted.
type CloseInfo struct {
	// Reason is "exit" (the client's exit notification), "eof" (the client
	// went away), "canceled" (the context ended), "closed" (Close was
	// called), "error" (the transport failed) or "panic".
	Reason   string
	Duration time.Duration
	Err      error
}

// ResolveInfo describes one resolution of a session's documents.
type ResolveInfo struct {
	Duration  time.Duration
	Cancelled bool // an edit or the end of the session made the result obsolete
	Documents int
}

// A PanicError is a panic recovered inside a session.
type PanicError struct {
	Where string // the request method or the activity
	Value any
	Stack []byte
}

func (e *PanicError) Error() string { return "panic in " + e.Where + ": " + toString(e.Value) }

// Files gives the server files outside those the client opens. Both methods
// are called with the session's lock not held but must be safe to call
// concurrently.
type Files interface {
	// List returns the model files under the client's root and workspace
	// folders (given as URIs).
	List(ctx context.Context, folders []string) []File
	// Read returns the text of a file, when a client's document is closed and
	// the file's content is again what is stored.
	Read(ctx context.Context, uri string) (string, bool)
}

// A File is a document found by Files.
type File struct {
	URI  string
	Text string
}

// Stats is what a host or a session holds now.
type Stats struct {
	Sessions  int
	Documents int
	// Bytes is the size of the documents' text; the model built from it takes
	// a few times that.
	Bytes int64
}
