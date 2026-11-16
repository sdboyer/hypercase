// Package diff renders line diffs for hypercase's failure messages. A diff
// reads the same in every process: nothing in it depends on the clock, map
// order or a random choice.
package diff

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

const (
	context = 3
	// maxLines caps how many diff lines a message prints.
	maxLines = 200
	// maxCells caps the edit graph Myers' algorithm explores. Past it, the
	// changed region is shown as removed whole, then added whole.
	maxCells = 4 << 20
)

// Lines returns a line diff from a to b. Each line is prefixed "  " (in
// both), "- " (only in a) or "+ " (only in b), with up to three lines of
// context around each change and "  ..." between hunks. A line holding a
// carriage return or another control character is quoted, and a last line
// with no final newline is followed by `\ no final newline`. Lines returns
// "" when a and b are equal.
func Lines(a, b []byte) string {
	lines, _ := Diff(a, b)
	var sb strings.Builder
	printed := 0
	for i, l := range lines {
		if printed >= maxLines {
			rest := 0
			for _, l := range lines[i:] {
				if l.Kind != Skip {
					rest++
				}
			}
			fmt.Fprintf(&sb, "  ... (%d more lines of diff)\n", rest)
			break
		}
		if l.Kind == Skip {
			sb.WriteString("  ...\n")
			continue
		}
		sb.WriteByte(l.Kind)
		sb.WriteByte(' ')
		sb.WriteString(l.Text)
		sb.WriteByte('\n')
		if l.NoNewline {
			sb.WriteByte(l.Kind)
			sb.WriteString(" \\ no final newline\n")
		}
		printed++
	}
	return sb.String()
}

// Skip is the Kind of the [Line] that stands for the lines left out
// between two hunks.
const Skip = '.'

// A Line is one line of a diff, as [Lines] shows it.
type Line struct {
	// Kind is ' ' for a line in both, '-' for one only in a, '+' for one
	// only in b, or Skip.
	Kind byte
	// Text is the line without its newline, quoted when it holds a
	// carriage return or another control character: "" for a Skip.
	Text string
	// NoNewline records that the line is its file's last, and has no
	// final newline.
	NoNewline bool
}

// Diff returns every line [Lines] shows from a to b, with no cap on how
// many, and how many pairs of lines it compared to find them. That count
// grows with the files' lengths and the size of the change, never with the
// product of the lengths. Diff returns no lines when a and b are equal.
func Diff(a, b []byte) (lines []Line, pairs int) {
	if bytes.Equal(a, b) {
		return nil, 0
	}
	x, y := split(a), split(b)
	ops := edits(x, y, &pairs)

	// Mark which ops to show: every change, and context around it.
	show := make([]bool, len(ops))
	for i, op := range ops {
		if op.kind != ' ' {
			for j := max(0, i-context); j <= min(len(ops)-1, i+context); j++ {
				show[j] = true
			}
		}
	}
	skipped := false
	for i, op := range ops {
		if !show[i] {
			skipped = true
			continue
		}
		if skipped && len(lines) > 0 {
			lines = append(lines, Line{Kind: Skip})
		}
		skipped = false
		lines = append(lines, Line{Kind: op.kind, Text: display(op.line), NoNewline: !strings.HasSuffix(op.line, "\n")})
	}
	return lines, pairs
}

func split(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	return strings.SplitAfter(string(b), "\n")[:bytes.Count(b, []byte("\n"))+boolInt(b[len(b)-1] != '\n')]
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func display(line string) string {
	s := strings.TrimSuffix(line, "\n")
	for _, r := range s {
		if r < ' ' && r != '\t' || r == 0x7f {
			return strconv.Quote(s)
		}
	}
	return s
}

type op struct {
	kind byte // ' ', '-' or '+'
	line string
}

// edits returns an edit script from x to y, and adds each pair of lines it
// compares to *pairs.
func edits(x, y []string, pairs *int) []op {
	// Trim the common prefix and suffix, which most diffs are.
	pre := 0
	for pre < len(x) && pre < len(y) && same(x[pre], y[pre], pairs) {
		pre++
	}
	suf := 0
	for suf < len(x)-pre && suf < len(y)-pre && same(x[len(x)-1-suf], y[len(y)-1-suf], pairs) {
		suf++
	}
	var ops []op
	for _, l := range x[:pre] {
		ops = append(ops, op{' ', l})
	}
	ops = append(ops, myers(x[pre:len(x)-suf], y[pre:len(y)-suf], pairs)...)
	for _, l := range x[len(x)-suf:] {
		ops = append(ops, op{' ', l})
	}
	return ops
}

// same compares two lines, and counts the pair in *pairs.
func same(a, b string, pairs *int) bool {
	*pairs++
	return a == b
}

// myers returns a shortest edit script from x to y, by Myers' O(ND)
// algorithm, preferring deletions before insertions. It adds each pair of
// lines it compares to *pairs.
func myers(x, y []string, pairs *int) []op {
	n, m := len(x), len(y)
	whole := func() []op {
		var ops []op
		for _, l := range x {
			ops = append(ops, op{'-', l})
		}
		for _, l := range y {
			ops = append(ops, op{'+', l})
		}
		return ops
	}
	if n == 0 || m == 0 {
		return whole()
	}
	maxD := n + m
	off := maxD
	v := make([]int, 2*maxD+2)
	var trace [][]int
	for d := 0; d <= maxD; d++ {
		if d*len(v) > maxCells {
			return whole()
		}
		trace = append(trace, append([]int(nil), v...))
		for k := -d; k <= d; k += 2 {
			var i int
			if k == -d || k != d && v[off+k-1] < v[off+k+1] {
				i = v[off+k+1] // down: an insertion
			} else {
				i = v[off+k-1] + 1 // right: a deletion
			}
			j := i - k
			for i < n && j < m && same(x[i], y[j], pairs) {
				i, j = i+1, j+1
			}
			v[off+k] = i
			if i >= n && j >= m {
				return backtrack(x, y, trace, d, off)
			}
		}
	}
	return whole()
}

func backtrack(x, y []string, trace [][]int, d, off int) []op {
	var rev []op
	i, j := len(x), len(y)
	for ; d > 0; d-- {
		v := trace[d]
		k := i - j
		var pk int
		if k == -d || k != d && v[off+k-1] < v[off+k+1] {
			pk = k + 1
		} else {
			pk = k - 1
		}
		pi := v[off+pk]
		pj := pi - pk
		for i > pi && j > pj {
			i, j = i-1, j-1
			rev = append(rev, op{' ', x[i]})
		}
		if pk == k+1 {
			rev = append(rev, op{'+', y[pj]})
		} else {
			rev = append(rev, op{'-', x[pi]})
		}
		i, j = pi, pj
	}
	for i > 0 && j > 0 {
		i, j = i-1, j-1
		rev = append(rev, op{' ', x[i]})
	}
	ops := make([]op, len(rev))
	for n, o := range rev {
		ops[len(rev)-1-n] = o
	}
	return ops
}
