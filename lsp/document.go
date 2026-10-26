// SPDX-License-Identifier: EPL-2.0

package lsp

import (
	"sort"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/chrismooreproductions/sysml-v2-compiler/sysml"
)

// document is the text of a file as it was last parsed, with what is known
// about it.
type document struct {
	key      string // what the workspace calls it
	uri      string // as the host or the client wrote it
	text     string
	gen      uint64 // the generation of the text that was parsed
	readOnly bool   // the host set it: the client may not change it
	errs     []*sysml.SyntaxError
	starts   []int // byte offset of each line, computed on demand
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
