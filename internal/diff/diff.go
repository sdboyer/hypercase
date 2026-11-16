// Package diff renders line diffs for hypercase's failure messages and its
// report. A diff reads the same in every process: nothing in it depends on
// the clock, map order or a random choice.
package diff

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	context = 3
	// maxLines caps how many diff lines a message prints.
	maxLines = 200
	// maxCells caps the edit graph Myers' algorithm explores in one stretch
	// of the files. Past it, the stretch is aligned on its unique lines
	// instead, and a stretch with none is left unaligned.
	maxCells = 4 << 20
)

// Lines returns a line diff from a to b. Each line is prefixed "  " (in
// both), "- " (only in a) or "+ " (only in b), with up to three lines of
// context around each change and "  ..." between hunks. A stretch the diff
// can't align is shown as removed whole, then added whole. A line holding
// a carriage return or another control character is quoted, and a last
// line with no final newline is followed by `\ no final newline`. Lines
// returns "" when a and b are equal.
func Lines(a, b []byte) string {
	lines, _ := diff(a, b, false)
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

// The Kinds of a [Line] that stands for other lines.
const (
	// Skip stands for the lines left out between two hunks.
	Skip = '.'
	// Unaligned stands for a stretch of a and one of b that the diff
	// can't align, which it leaves out: Old lines of a and New lines of b.
	Unaligned = '?'
)

// A Line is one line of a diff, as [Lines] shows it, or one that stands
// for others.
type Line struct {
	// Kind is ' ' for a line in both, '-' for one only in a, '+' for one
	// only in b, or Skip or Unaligned.
	Kind byte
	// Text is the line without its newline, quoted when it holds a
	// carriage return or another control character: "" for a Skip or
	// Unaligned.
	Text string
	// NoNewline records that the line is its file's last, and has no
	// final newline.
	NoNewline bool
	// Old and New count the lines of a and of b an Unaligned line stands
	// for.
	Old, New int
}

// Diff returns the line diff from a to b, with no cap on how many lines,
// and how many pairs of lines it compared to find them. Each stretch it
// can't align is one Unaligned line, so the diff holds only lines it
// aligned, and its '+' and '-' lines count them exactly. The pairs grow
// with the files' lengths times the changes between the lines each holds
// once, never with the product of the lengths. Diff returns no lines when
// a and b are equal.
func Diff(a, b []byte) (lines []Line, pairs int) {
	return diff(a, b, true)
}

// diff returns the lines of a diff from a to b. With collapse, each
// unaligned stretch is one Unaligned line; without, it is its lines of a
// removed, then its lines of b added.
func diff(a, b []byte, collapse bool) (lines []Line, pairs int) {
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
		if op.unaligned && collapse {
			if n := len(lines); n == 0 || lines[n-1].Kind != Unaligned || !ops[i-1].unaligned {
				lines = append(lines, Line{Kind: Unaligned})
			}
			l := &lines[len(lines)-1]
			if op.kind == '-' {
				l.Old++
			} else {
				l.New++
			}
			continue
		}
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
	// unaligned marks a '-' or '+' of a stretch the diff couldn't align.
	unaligned bool
}

// edits returns an edit script from x to y, and adds each pair of lines it
// compares to *pairs. It trims the common prefix and suffix, which most
// diffs are, and aligns what is between them.
func edits(x, y []string, pairs *int) []op {
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
		ops = append(ops, op{kind: ' ', line: l})
	}
	ops = append(ops, align(x[pre:len(x)-suf], y[pre:len(y)-suf], pairs)...)
	for _, l := range x[len(x)-suf:] {
		ops = append(ops, op{kind: ' ', line: l})
	}
	return ops
}

// align returns an edit script from x to y: a shortest one, when Myers'
// algorithm finds it within its budget. Otherwise it anchors the script on
// the longest run, in order in both, of lines that x and y each hold once,
// and aligns each stretch between two anchors as edits does. A stretch with
// no such line is left unaligned: its lines of x removed, then its lines
// of y added, each marked.
func align(x, y []string, pairs *int) []op {
	if ops, ok := myers(x, y, pairs); ok {
		return ops
	}
	anchors := uniqueAnchors(x, y, pairs)
	if len(anchors) == 0 {
		var ops []op
		for _, l := range x {
			ops = append(ops, op{kind: '-', line: l, unaligned: true})
		}
		for _, l := range y {
			ops = append(ops, op{kind: '+', line: l, unaligned: true})
		}
		return ops
	}
	var ops []op
	i, j := 0, 0
	for _, a := range anchors {
		ops = append(ops, edits(x[i:a[0]], y[j:a[1]], pairs)...)
		ops = append(ops, op{kind: ' ', line: x[a[0]]})
		i, j = a[0]+1, a[1]+1
	}
	return append(ops, edits(x[i:], y[j:], pairs)...)
}

// uniqueAnchors returns the longest run of pairs (i, j), increasing in
// both, where x[i] and y[j] are one line that x and y each hold once. It
// adds each line it looks up by its text to *pairs.
func uniqueAnchors(x, y []string, pairs *int) [][2]int {
	type seen struct{ nx, ny, i, j int }
	lines := map[string]*seen{}
	for i, l := range x {
		*pairs++
		s := lines[l]
		if s == nil {
			s = &seen{}
			lines[l] = s
		}
		s.nx++
		s.i = i
	}
	for j, l := range y {
		*pairs++
		if s := lines[l]; s != nil {
			s.ny++
			s.j = j
		}
	}
	var cand [][2]int // in x's order
	for _, l := range x {
		if s := lines[l]; s.nx == 1 && s.ny == 1 {
			cand = append(cand, [2]int{s.i, s.j})
		}
	}
	// The longest run increasing in j, by patience sorting: tails[k] is
	// the candidate ending the best run of length k+1 found so far.
	var tails []int
	prev := make([]int, len(cand))
	for c, p := range cand {
		k, _ := slices.BinarySearchFunc(tails, p[1], func(t, j int) int { return cand[t][1] - j })
		if k > 0 {
			prev[c] = tails[k-1]
		} else {
			prev[c] = -1
		}
		if k == len(tails) {
			tails = append(tails, c)
		} else {
			tails[k] = c
		}
	}
	if len(tails) == 0 {
		return nil
	}
	run := make([][2]int, len(tails))
	for c, k := tails[len(tails)-1], len(tails)-1; k >= 0; c, k = prev[c], k-1 {
		run[k] = cand[c]
	}
	return run
}

// same compares two lines, and counts the pair in *pairs.
func same(a, b string, pairs *int) bool {
	*pairs++
	return a == b
}

// myers returns a shortest edit script from x to y, by Myers' O(ND)
// algorithm, preferring deletions before insertions, and whether it found
// one within maxCells. It adds each pair of lines it compares to *pairs.
func myers(x, y []string, pairs *int) ([]op, bool) {
	n, m := len(x), len(y)
	if n == 0 || m == 0 {
		var ops []op
		for _, l := range x {
			ops = append(ops, op{kind: '-', line: l})
		}
		for _, l := range y {
			ops = append(ops, op{kind: '+', line: l})
		}
		return ops, true
	}
	maxD := n + m
	off := maxD
	v := make([]int, 2*maxD+2)
	var trace [][]int
	for d := 0; d <= maxD; d++ {
		if d*len(v) > maxCells {
			return nil, false
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
				return backtrack(x, y, trace, d, off), true
			}
		}
	}
	return nil, false
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
			rev = append(rev, op{kind: ' ', line: x[i]})
		}
		if pk == k+1 {
			rev = append(rev, op{kind: '+', line: y[pj]})
		} else {
			rev = append(rev, op{kind: '-', line: x[pi]})
		}
		i, j = pi, pj
	}
	for i > 0 && j > 0 {
		i, j = i-1, j-1
		rev = append(rev, op{kind: ' ', line: x[i]})
	}
	ops := make([]op, len(rev))
	for n, o := range rev {
		ops[len(rev)-1-n] = o
	}
	return ops
}
