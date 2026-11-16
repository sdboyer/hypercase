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
	if bytes.Equal(a, b) {
		return ""
	}
	x, y := split(a), split(b)
	ops := edits(x, y)

	// Mark which ops to print: every change, and context around it.
	show := make([]bool, len(ops))
	for i, op := range ops {
		if op.kind != ' ' {
			for j := max(0, i-context); j <= min(len(ops)-1, i+context); j++ {
				show[j] = true
			}
		}
	}
	var sb strings.Builder
	printed, skipped := 0, false
	for i, op := range ops {
		if !show[i] {
			skipped = true
			continue
		}
		if printed >= maxLines {
			rest := 0
			for j := i; j < len(ops); j++ {
				if show[j] {
					rest++
				}
			}
			fmt.Fprintf(&sb, "  ... (%d more lines of diff)\n", rest)
			break
		}
		if skipped && printed > 0 {
			sb.WriteString("  ...\n")
		}
		skipped = false
		sb.WriteByte(op.kind)
		sb.WriteByte(' ')
		sb.WriteString(display(op.line))
		sb.WriteByte('\n')
		if !strings.HasSuffix(op.line, "\n") {
			sb.WriteByte(op.kind)
			sb.WriteString(" \\ no final newline\n")
		}
		printed++
	}
	return sb.String()
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

// edits returns an edit script from x to y.
func edits(x, y []string) []op {
	// Trim the common prefix and suffix, which most diffs are.
	pre := 0
	for pre < len(x) && pre < len(y) && x[pre] == y[pre] {
		pre++
	}
	suf := 0
	for suf < len(x)-pre && suf < len(y)-pre && x[len(x)-1-suf] == y[len(y)-1-suf] {
		suf++
	}
	var ops []op
	for _, l := range x[:pre] {
		ops = append(ops, op{' ', l})
	}
	ops = append(ops, myers(x[pre:len(x)-suf], y[pre:len(y)-suf])...)
	for _, l := range x[len(x)-suf:] {
		ops = append(ops, op{' ', l})
	}
	return ops
}

// myers returns a shortest edit script from x to y, by Myers' O(ND)
// algorithm, preferring deletions before insertions.
func myers(x, y []string) []op {
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
			for i < n && j < m && x[i] == y[j] {
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
