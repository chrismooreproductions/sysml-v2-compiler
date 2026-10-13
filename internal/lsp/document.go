// SPDX-License-Identifier: EPL-2.0

package lsp

import (
	"net/url"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/chrismooreproductions/sysml-v2-compiler/sysml"
)

// document is a file the server knows: open in the editor, or found on disk.
type document struct {
	uri     string
	path    string
	text    string
	version int
	open    bool
	errs    []*sysml.SyntaxError // from the last time it was parsed
	starts  []int                // byte offset of each line, computed on demand
}

func (d *document) lineStarts() []int {
	if d.starts == nil {
		d.starts = []int{0}
		for i := 0; i < len(d.text); i++ {
			if d.text[i] == '\n' {
				d.starts = append(d.starts, i+1)
			}
		}
	}
	return d.starts
}

// position converts a byte offset to a line and UTF-16 column.
func (d *document) position(offset int) Position {
	offset = max(0, min(offset, len(d.text)))
	starts := d.lineStarts()
	line := sort.Search(len(starts), func(i int) bool { return starts[i] > offset }) - 1
	col := 0
	for _, r := range d.text[starts[line]:offset] {
		col += utf16Len(r)
	}
	return Position{line, col}
}

// offset converts a line and UTF-16 column to a byte offset, clamped to the
// line.
func (d *document) offset(p Position) int {
	starts := d.lineStarts()
	if p.Line < 0 {
		return 0
	}
	if p.Line >= len(starts) {
		return len(d.text)
	}
	i, end := starts[p.Line], len(d.text)
	if p.Line+1 < len(starts) {
		end = starts[p.Line+1] - 1
	}
	for col := 0; i < end && col < p.Character; {
		r, n := utf8.DecodeRuneInString(d.text[i:])
		col += utf16Len(r)
		i += n
	}
	return min(i, end)
}

func utf16Len(r rune) int {
	if r >= 0x10000 {
		return len(utf16.Encode([]rune{r}))
	}
	return 1
}

// rng converts a source span to a range.
func (d *document) rng(s sysml.Span) Range {
	return Range{d.position(s.Start.Offset), d.position(s.End.Offset)}
}

// firstLine is the range of the first line of a span, which is as much of a
// large element as an editor should underline.
func (d *document) firstLine(s sysml.Span) Range {
	r := d.rng(s)
	if r.End.Line > r.Start.Line {
		starts := d.lineStarts()
		end := len(d.text)
		if r.Start.Line+1 < len(starts) {
			end = starts[r.Start.Line+1] - 1
		}
		r.End = d.position(end)
	}
	return r
}

// wordAt is the name-shaped run of characters around a byte offset: letters,
// digits, underscores and the :: of a qualified name.
func (d *document) wordBefore(offset int) string {
	i := offset
	for i > 0 {
		c := d.text[i-1]
		if c == '_' || c == ':' || c == '\'' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80 {
			i--
			continue
		}
		break
	}
	return d.text[i:offset]
}

// uriToPath turns a file URI into a file system path.
func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return uri
	}
	p := u.Path
	if runtime.GOOS == "windows" && len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

// pathToURI turns a file system path into a file URI.
func pathToURI(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}
