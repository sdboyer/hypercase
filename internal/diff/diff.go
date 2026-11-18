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
	// maxUnaligned caps how many lines of each side of a stretch the diff
	// can't align a message prints, so that a long stretch's removed lines
	// don't fill a message alone: each side shows its first maxUnaligned
	// lines and how many more it leaves out. maxLines still caps the whole
	// message, so a stretch that starts late in one is cut there, its notes
	// with it.
	maxUnaligned = maxLines / 4
	// maxCells caps the work of one attempt of Myers' algorithm on a
	// stretch of the files. Past it, the stretch is aligned on its unique
	// lines instead.
	maxCells = 4 << 20
	// maxSteps bounds the edits an attempt within maxCells can find: step
	// d records 2d+1 cells, so the steps before maxSteps record
	// maxSteps^2, which is maxCells.
	maxSteps = 2048
	// maxWork is the work a whole diff shares. Each stretch of the files
	// it aligns draws on it, and once it is spent, on the stretch's own
	// work, perLine for each of the stretch's lines. A pass for unique
	// lines draws on maxWork alone, so a stretch that spends any of its
	// own work is split no further, the stretches that do share no line,
	// and a diff's work is at most maxWork plus perLine for each line of
	// its files.
	maxWork = 4 * maxCells
	// perLine is a stretch's own work for each of its lines. It always
	// covers trimming the lines the stretch's two sides start and end
	// with, so the sides of a stretch left unaligned differ in their first
	// lines and in their last. It also covers an attempt of Myers'
	// algorithm on what is between them, when that is a few lines that
	// all differ.
	perLine = 16
)

// Lines returns a line diff from a to b. Each line is prefixed "  " (in
// both), "- " (only in a) or "+ " (only in b), with up to three lines of
// context around each change and "  ..." between hunks. A stretch the diff
// can't align is shown as removed, then added, each side's first lines
// and how many more it leaves out. A line holding a carriage return or
// another control character is quoted, and a last line with no final
// newline is followed by `\ no final newline`, as is the note for a side
// that leaves such a line out. Lines returns "" when a and b are equal.
func Lines(a, b []byte) string {
	lines, _ := diff(a, b, false)
	var sb strings.Builder
	printed := 0
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if printed >= maxLines {
			rest := 0
			for _, l := range lines[i:] {
				if l.Kind != Skip {
					rest++
				}
			}
			fmt.Fprintf(&sb, "  ... (%s of diff)\n", more(rest))
			break
		}
		if l.Kind == Skip {
			sb.WriteString("  ...\n")
			continue
		}
		if l.unaligned {
			// The side's lines so far, l among them.
			run := 1
			for i-run >= 0 && lines[i-run].unaligned && lines[i-run].Kind == l.Kind {
				run++
			}
			if run > maxUnaligned {
				end := i
				for end < len(lines) && lines[end].unaligned && lines[end].Kind == l.Kind {
					end++
				}
				how := "removed"
				if l.Kind == '+' {
					how = "added"
				}
				fmt.Fprintf(&sb, "  ... (%s %s, which the diff can't align)\n", more(end-i), how)
				if lines[end-1].NoNewline {
					sb.WriteByte(l.Kind)
					sb.WriteString(" \\ no final newline\n")
				}
				printed++
				i = end - 1
				continue
			}
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

// more says how many more lines there are.
func more(n int) string {
	if n == 1 {
		return "1 more line"
	}
	return fmt.Sprintf("%d more lines", n)
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
	// for, and OldNoNewline and NewNoNewline record that the last of them
	// is a's, or b's, last line, and has no final newline.
	Old, New                   int
	OldNoNewline, NewNoNewline bool
	// unaligned marks a '-' or '+' line of a stretch the diff can't align,
	// in the lines Lines shows, which hold no Unaligned line.
	unaligned bool
}

// Diff returns the line diff from a to b, with no cap on how many lines,
// and the work it did to find them: each pair of lines it compared, each
// line a pass for unique lines looked up, and each entry of the arrays
// Myers' algorithm keeps, its furthest point on each diagonal and each
// step's record of them, the cells. A pair Myers' algorithm finds unequal
// ends that step on that diagonal, so it is counted in the step's cell for
// the diagonal, not again. Each stretch Diff can't align is one Unaligned
// line, so the diff holds only lines it aligned, and its '+' and '-' lines
// count them exactly. The work is at most a fixed budget plus a fixed
// amount for each line of a and b. Diff numbers each distinct line before
// it compares any, so comparing two lines costs the same whatever their
// length, and its cost grows with the files' sizes plus that budget, never
// with the product of their lengths. Diff returns no lines when a and b
// are equal.
func Diff(a, b []byte) (lines []Line, work int) {
	return diff(a, b, true)
}

// diff returns the lines of a diff from a to b. With collapse, each
// unaligned stretch is one Unaligned line; without, it is its lines of a
// removed, then its lines of b added.
func diff(a, b []byte, collapse bool) (lines []Line, work int) {
	if bytes.Equal(a, b) {
		return nil, 0
	}
	x, y, text := number(split(a), split(b))
	w := &budget{shared: maxWork}
	var ops []op
	edits(x, y, w, &ops)

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
		line := text[op.line]
		noNewline := !strings.HasSuffix(line, "\n")
		if op.unaligned && collapse {
			if n := len(lines); n == 0 || lines[n-1].Kind != Unaligned || !ops[i-1].unaligned {
				lines = append(lines, Line{Kind: Unaligned})
			}
			l := &lines[len(lines)-1]
			if op.kind == '-' {
				l.Old++
				l.OldNoNewline = noNewline
			} else {
				l.New++
				l.NewNoNewline = noNewline
			}
			continue
		}
		lines = append(lines, Line{Kind: op.kind, Text: Display(line), NoNewline: noNewline, unaligned: op.unaligned})
	}
	return lines, w.spent
}

func split(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	return strings.SplitAfter(string(b), "\n")[:bytes.Count(b, []byte("\n"))+boolInt(b[len(b)-1] != '\n')]
}

// number returns the lines of a and of b as numbers, one for each distinct
// line, and the lines by their numbers. It reads each line once, so that
// comparing two lines after it costs the same whatever their length.
func number(a, b []string) (x, y []int, text []string) {
	ids := map[string]int{}
	num := func(lines []string) []int {
		out := make([]int, len(lines))
		for i, l := range lines {
			id, ok := ids[l]
			if !ok {
				id = len(text)
				ids[l] = id
				text = append(text, l)
			}
			out[i] = id
		}
		return out
	}
	x = num(a)
	y = num(b)
	return x, y, text
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Display returns a line as a diff shows it: without its newline, and
// quoted when it holds a carriage return or another control character, so
// that the line reads the same wherever it is shown.
func Display(line string) string {
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
	line int  // the line's number
	// unaligned marks a '-' or '+' of a stretch the diff couldn't align.
	unaligned bool
}

// A budget counts the work of a diff, and holds what is left of maxWork.
type budget struct {
	spent, shared int
}

// spend records n units of work, as charge does, if what is left of
// maxWork and *own hold them, or what is left of maxWork alone when own is
// nil, and reports whether they do. When they don't, it records none.
func (b *budget) spend(n int, own *int) bool {
	left := 0
	if own != nil {
		left = *own
	}
	if n > b.shared+left {
		return false
	}
	b.charge(n, own)
	return true
}

// charge records n units of work done, drawn from what is left of maxWork,
// then, once that is spent, from *own, what is left of a stretch's own
// work.
func (b *budget) charge(n int, own *int) {
	from := min(n, b.shared)
	b.shared -= from
	if own != nil {
		*own -= n - from
	}
	b.spent += n
}

// An attempt counts the work of one attempt of Myers' algorithm, and
// refuses work past its limit.
type attempt struct {
	spent, limit int
}

// spend records n units of work, and reports whether the attempt holds
// them. When it doesn't, it records none.
func (a *attempt) spend(n int) bool {
	if a.spent+n > a.limit {
		return false
	}
	a.spent += n
	return true
}

// edits appends an edit script from x to y to *out, spending from w and
// from the stretch's own work, perLine for each of its lines. It trims the
// lines x and y start and end with, which most diffs are, and aligns what
// is between them. Its own work always pays for the trim: every comparison
// but two consumes a line of each side.
func edits(x, y []int, w *budget, out *[]op) {
	own := perLine * (len(x) + len(y))
	pre := 0
	for pre < len(x) && pre < len(y) && w.spend(1, &own) && x[pre] == y[pre] {
		pre++
	}
	suf := 0
	for suf < len(x)-pre && suf < len(y)-pre && w.spend(1, &own) && x[len(x)-1-suf] == y[len(y)-1-suf] {
		suf++
	}
	for _, l := range x[:pre] {
		*out = append(*out, op{kind: ' ', line: l})
	}
	align(x[pre:len(x)-suf], y[pre:len(y)-suf], w, &own, out)
	for _, l := range x[len(x)-suf:] {
		*out = append(*out, op{kind: ' ', line: l})
	}
}

// align appends an edit script from x to y to *out, spending from w and
// *own: a shortest one, when Myers' algorithm finds it within maxCells.
// Otherwise it anchors the script on the longest run, in order in both, of
// lines that x and y each hold once, and aligns each stretch between two
// anchors as edits does. A stretch with no such line, or whose pass to
// look for one what is left of maxWork can't pay, is left unaligned: its
// lines of x removed, then its lines of y added, each marked.
func align(x, y []int, w *budget, own *int, out *[]op) {
	if len(x) == 0 || len(y) == 0 {
		for _, l := range x {
			*out = append(*out, op{kind: '-', line: l})
		}
		for _, l := range y {
			*out = append(*out, op{kind: '+', line: l})
		}
		return
	}
	if myers(x, y, w, own, out) {
		return
	}
	var anchors [][2]int
	if w.spend(len(x)+len(y), nil) {
		anchors = uniqueAnchors(x, y)
	}
	if len(anchors) == 0 {
		for _, l := range x {
			*out = append(*out, op{kind: '-', line: l, unaligned: true})
		}
		for _, l := range y {
			*out = append(*out, op{kind: '+', line: l, unaligned: true})
		}
		return
	}
	i, j := 0, 0
	for _, a := range anchors {
		edits(x[i:a[0]], y[j:a[1]], w, out)
		*out = append(*out, op{kind: ' ', line: x[a[0]]})
		i, j = a[0]+1, a[1]+1
	}
	edits(x[i:], y[j:], w, out)
}

// uniqueAnchors returns the longest run of pairs (i, j), increasing in
// both, where x[i] and y[j] are one line that x and y each hold once. It
// looks up each line of x twice, and each line of y once.
func uniqueAnchors(x, y []int) [][2]int {
	type seen struct{ nx, ny, i, j int }
	lines := map[int]*seen{}
	for i, l := range x {
		s := lines[l]
		if s == nil {
			s = &seen{}
			lines[l] = s
		}
		s.nx++
		s.i = i
	}
	for j, l := range y {
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

// myers appends a shortest edit script from x to y to *out, by Myers'
// O(ND) algorithm, preferring deletions before insertions, and reports
// whether it found one within maxCells of work, which it spends from w and
// *own whether or not it does. Step d records the furthest point reached
// on each diagonal from -d to d, the 2d+1 cells that backtracking reads.
func myers(x, y []int, w *budget, own *int, out *[]op) bool {
	at := attempt{limit: min(maxCells, w.shared+*own)}
	defer func() { w.charge(at.spent, own) }()
	n, m := len(x), len(y)
	maxD := min(n+m, maxSteps)
	off := maxD
	if !at.spend(2*maxD + 2) {
		return false
	}
	v := make([]int, 2*maxD+2)
	var trace [][]int
	for d := 0; d <= maxD; d++ {
		if !at.spend(2*d + 1) {
			return false
		}
		trace = append(trace, slices.Clone(v[off-d:off+d+1]))
		for k := -d; k <= d; k += 2 {
			var i int
			if k == -d || k != d && v[off+k-1] < v[off+k+1] {
				i = v[off+k+1] // down: an insertion
			} else {
				i = v[off+k-1] + 1 // right: a deletion
			}
			j := i - k
			for i < n && j < m && x[i] == y[j] {
				if !at.spend(1) {
					return false
				}
				i, j = i+1, j+1
			}
			v[off+k] = i
			if i >= n && j >= m {
				*out = append(*out, backtrack(x, y, trace, d)...)
				return true
			}
		}
	}
	return false
}

// backtrack returns the edit script that ends at step d, from trace, where
// trace[d] holds the furthest point on diagonal k at trace[d][k+d], as the
// steps before d left it.
func backtrack(x, y []int, trace [][]int, d int) []op {
	var rev []op
	i, j := len(x), len(y)
	for ; d > 0; d-- {
		v := trace[d]
		k := i - j
		var pk int
		if k == -d || k != d && v[k-1+d] < v[k+1+d] {
			pk = k + 1
		} else {
			pk = k - 1
		}
		pi := v[pk+d]
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
