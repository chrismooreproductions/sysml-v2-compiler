// SPDX-License-Identifier: EPL-2.0

package lsp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// A MessageConn carries whole JSON-RPC messages, one per call: a WebSocket text
// frame, or a postMessage. Both methods may be called from different goroutines
// at the same time, but each is called by one goroutine at a time. ReadMessage
// returns io.EOF when the peer has gone away cleanly.
type MessageConn interface {
	ReadMessage(ctx context.Context) ([]byte, error)
	WriteMessage(ctx context.Context, msg []byte) error
}

// framed is the stream encoding of the protocol: a Content-Length header, a
// blank line and the message.
type framed struct {
	rd  *bufio.Reader
	w   io.Writer
	max int

	once sync.Once
	msgs chan frame
	quit chan struct{}
	stop sync.Once
}

type frame struct {
	body []byte
	err  error
}

func newFramed(rw io.ReadWriter, max int) *framed {
	return &framed{rd: bufio.NewReader(rw), w: rw, max: max, msgs: make(chan frame), quit: make(chan struct{})}
}

// ReadMessage reads in a goroutine of its own, so that a context that ends can
// stop the wait. The goroutine goes on until the reader returns, which is why
// the owner of the stream closes it to stop it.
func (f *framed) ReadMessage(ctx context.Context) ([]byte, error) {
	f.once.Do(func() {
		go func() {
			for {
				body, err := f.read()
				select {
				case f.msgs <- frame{body, err}:
				case <-f.quit:
					return
				}
				if err != nil {
					return
				}
			}
		}()
	})
	select {
	case m := <-f.msgs:
		return m.body, m.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *framed) close() { f.stop.Do(func() { close(f.quit) }) }

func (f *framed) read() ([]byte, error) {
	length := -1
	for {
		line, err := f.rd.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(k, "Content-Length") {
			length, err = strconv.Atoi(strings.TrimSpace(v))
			if err != nil || length < 0 {
				return nil, fmt.Errorf("bad Content-Length %q", v)
			}
		}
	}
	if length < 0 {
		return nil, errors.New("a message without Content-Length")
	}
	if f.max > 0 && length > f.max {
		return nil, fmt.Errorf("a message of %d bytes is over the limit of %d", length, f.max)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(f.rd, body); err != nil {
		return nil, err
	}
	return body, nil
}

func (f *framed) WriteMessage(ctx context.Context, msg []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	buf := make([]byte, 0, len(msg)+32)
	buf = fmt.Appendf(buf, "Content-Length: %d\r\n\r\n", len(msg))
	buf = append(buf, msg...)
	_, err := f.w.Write(buf) // one write, so a writer that makes a frame of each write keeps the message whole
	return err
}
