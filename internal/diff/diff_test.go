package diff

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestLines(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want string
	}{
		{"equal", "a\nb\n", "a\nb\n", ""},
		{"changed line", "serves: 4\nflour: 200 g\nmilk: 300 ml\negg: 2\n", "serves: 4\nflour: 200.0 g\nmilk: 300 ml\negg: 2\n",
			"  serves: 4\n- flour: 200 g\n+ flour: 200.0 g\n  milk: 300 ml\n  egg: 2\n"},
		{"added", "a\n", "a\nb\n", "  a\n+ b\n"},
		{"removed", "a\nb\n", "b\n", "- a\n  b\n"},
		{"from empty", "", "a\n", "+ a\n"},
		{"to empty", "a\n", "", "- a\n"},
		{"final newline", "a\n", "a", "- a\n+ a\n+ \\ no final newline\n"},
		{"line ending", "a\n", "a\r\n", "- a\n+ \"a\\r\"\n"},
		{"hunks", "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n", "0\n2\n3\n4\n5\n6\n7\n8\n9\n11\n",
			"- 1\n+ 0\n  2\n  3\n  4\n  ...\n  7\n  8\n  9\n- 10\n+ 11\n"},
		{"interleaved", "a\nb\nc\n", "a\nx\nc\ny\n", "  a\n- b\n+ x\n  c\n+ y\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Lines([]byte(tt.a), []byte(tt.b)); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// TestLinesMinimal checks the edit script is a shortest one, against an
// LCS computed directly, on inputs small enough to enumerate.
func TestLinesMinimal(t *testing.T) {
	inputs := []string{"", "a", "b", "ab", "ba", "abc", "acb", "bca", "aab", "abab", "cabc", "bbca"}
	for _, a := range inputs {
		for _, b := range inputs {
			x, y := chars(a), chars(b)
			nx, ny, text := number(x, y)
			var ops []op
			edits(nx, ny, &budget{shared: maxWork}, &ops)
			changes := 0
			var gotA, gotB []string
			for _, o := range ops {
				switch o.kind {
				case ' ':
					gotA, gotB = append(gotA, text[o.line]), append(gotB, text[o.line])
				case '-':
					gotA = append(gotA, text[o.line])
					changes++
				case '+':
					gotB = append(gotB, text[o.line])
					changes++
				}
			}
			if strings.Join(gotA, "") != strings.Join(x, "") || strings.Join(gotB, "") != strings.Join(y, "") {
				t.Errorf("%q -> %q: the script doesn't rebuild both sides: %v", a, b, ops)
			}
			if want := len(x) + len(y) - 2*lcs(x, y); changes != want {
				t.Errorf("%q -> %q: %d changes, want %d", a, b, changes, want)
			}
		}
	}
}

func chars(s string) []string {
	var out []string
	for _, r := range s {
		out = append(out, string(r)+"\n")
	}
	return out
}

func lcs(x, y []string) int {
	t := make([][]int, len(x)+1)
	for i := range t {
		t[i] = make([]int, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				t[i][j] = t[i+1][j+1] + 1
			} else {
				t[i][j] = max(t[i+1][j], t[i][j+1])
			}
		}
	}
	return t[0][0]
}

func TestLinesTruncated(t *testing.T) {
	var a, b strings.Builder
	for i := range 1000 {
		a.WriteString("a\n")
		_ = i
		b.WriteString("b\n")
	}
	got := Lines([]byte(a.String()), []byte(b.String()))
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != maxLines+1 || lines[len(lines)-1] != "  ... (1800 more lines of diff)" {
		t.Errorf("got %d lines, ending %q", len(lines), lines[len(lines)-1])
	}
}

// TestDiff: Diff gives the lines Lines shows, uncapped, with each skip
// between hunks and each missing final newline marked.
func TestDiff(t *testing.T) {
	got, _ := Diff([]byte("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"), []byte("0\n2\n3\n4\n5\n6\n7\n8\n9\n10"))
	want := []Line{
		{Kind: '-', Text: "1"}, {Kind: '+', Text: "0"}, {Kind: ' ', Text: "2"}, {Kind: ' ', Text: "3"}, {Kind: ' ', Text: "4"},
		{Kind: Skip},
		{Kind: ' ', Text: "7"}, {Kind: ' ', Text: "8"}, {Kind: ' ', Text: "9"}, {Kind: '-', Text: "10"}, {Kind: '+', Text: "10", NoNewline: true},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	var a, b strings.Builder
	for range 300 {
		a.WriteString("a\n")
		b.WriteString("b\n")
	}
	if got, _ := Diff([]byte(a.String()), []byte(b.String())); len(got) != 600 {
		t.Errorf("a diff of 600 changed lines has %d lines, want every one", len(got))
	}
	if got, work := Diff([]byte("a\n"), []byte("a\n")); got != nil || work != 0 {
		t.Errorf("equal files: got %v, %d work", got, work)
	}
}

// TestDiffPairs: a diff of two 100,000-line files does work, its cells of
// Myers' trace included, that grows with their length and the size of the
// change, far under the 10^10 of a table of every pair: with one line
// changed in the middle, and with the first and last lines changed, which
// leave no common prefix or suffix to trim.
func TestDiffPairs(t *testing.T) {
	const n = 100_000
	lines := func(change ...int) []byte {
		var b strings.Builder
		for i := range n {
			if slices.Contains(change, i) {
				fmt.Fprintf(&b, "line %d, changed\n", i)
				continue
			}
			fmt.Fprintf(&b, "line %d\n", i)
		}
		return []byte(b.String())
	}
	for _, change := range [][]int{{n / 2}, {0, n - 1}} {
		got, work := Diff(lines(), lines(change...))
		if work >= 1_000_000 {
			t.Errorf("changing lines %v did %d work, want less than 1,000,000", change, work)
		}
		var changed, want []string
		for _, l := range got {
			if l.Kind == '-' || l.Kind == '+' {
				changed = append(changed, string(l.Kind)+" "+l.Text)
			}
		}
		for _, i := range change {
			want = append(want, fmt.Sprintf("- line %d", i), fmt.Sprintf("+ line %d, changed", i))
		}
		if !slices.Equal(changed, want) {
			t.Errorf("changing lines %v: the diff's changed lines are %q, want %q", change, changed, want)
		}
	}
}

// scattered returns n lines, each unique, with the lines at change changed.
func scattered(n int, change ...int) []byte {
	var b strings.Builder
	for i := range n {
		if slices.Contains(change, i) {
			fmt.Fprintf(&b, "line %d, changed\n", i)
			continue
		}
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return []byte(b.String())
}

// counts returns how many lines of each kind a diff holds.
func counts(lines []Line) map[byte]int {
	n := map[byte]int{}
	for _, l := range lines {
		n[l.Kind]++
	}
	return n
}

// TestDiffAnchors: edits scattered over a long file come out exactly, so
// the diff holds the changed lines and their context. Six edits in 100,000
// lines are within one attempt of Myers' algorithm, whose work grows with
// the files' lengths. More edits than an attempt covers are aligned on the
// lines each file holds once, for one failed attempt's work more.
func TestDiffAnchors(t *testing.T) {
	every := func(n, step int) (c []int) {
		for i := 1; i < n; i += step {
			c = append(c, i)
		}
		return c
	}
	for _, tt := range []struct {
		n      int
		change []int
		work   int
	}{
		{100_000, []int{3, 17_000, 33_333, 50_001, 81_234, 99_990}, 2 * 100_000},
		{3000, every(3000, 2), maxCells + 100*3000},
		{100_000, every(100_000, 40), maxCells + 100*100_000},
	} {
		got, work := Diff(scattered(tt.n), scattered(tt.n, tt.change...))
		n := counts(got)
		if n['-'] != len(tt.change) || n['+'] != len(tt.change) || n[Unaligned] != 0 {
			t.Errorf("%d lines, %d changed: the diff holds %d removed, %d added and %d unaligned, want %d, %d and 0", tt.n, len(tt.change), n['-'], n['+'], n[Unaligned], len(tt.change), len(tt.change))
		}
		if work > tt.work {
			t.Errorf("%d lines, %d changed: the diff did %d work, want at most %d", tt.n, len(tt.change), work, tt.work)
		}
	}
	// Each changed line is removed, then added, in place.
	got, _ := Diff(scattered(10), scattered(10, 4))
	want := []Line{{Kind: ' ', Text: "line 1"}, {Kind: ' ', Text: "line 2"}, {Kind: ' ', Text: "line 3"}, {Kind: '-', Text: "line 4"}, {Kind: '+', Text: "line 4, changed"}, {Kind: ' ', Text: "line 5"}, {Kind: ' ', Text: "line 6"}, {Kind: ' ', Text: "line 7"}}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// TestDiffUnaligned: a stretch past Myers' budget with no line that both
// files hold once is one Unaligned line, which counts its lines of each
// file and holds none of them. Lines shows it removed, then added, each
// side's first lines and how many more it leaves out.
func TestDiffUnaligned(t *testing.T) {
	const n = 10_000
	file := func(first string, mul, mod, below int) []byte {
		var b strings.Builder
		b.WriteString(first + "\n")
		for i := range n - 2 {
			if i*mul%mod < below {
				b.WriteString("a\n")
			} else {
				b.WriteString("b\n")
			}
		}
		b.WriteString(first + " again\n")
		return []byte(b.String())
	}
	a, b := file("first of a", 7919, 13, 6), file("first of b", 104_729, 17, 8)
	got, _ := Diff(a, b)
	if want := []Line{{Kind: Unaligned, Old: n, New: n}}; !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	lines := strings.Split(strings.TrimSuffix(Lines(a, b), "\n"), "\n")
	if len(lines) != 2*maxUnaligned+2 || lines[0] != "- first of a" || lines[maxUnaligned+1] != "+ first of b" ||
		lines[maxUnaligned] != fmt.Sprintf("  ... (%d more lines removed, which the diff can't align)", n-maxUnaligned) ||
		lines[2*maxUnaligned+1] != fmt.Sprintf("  ... (%d more lines added, which the diff can't align)", n-maxUnaligned) {
		t.Errorf("Lines gives:\n%s", strings.Join(lines, "\n"))
	}
}

// TestDiffUnalignedEnds: a stretch the diff can't align that ends both
// files, with no final newline, says so of each side: Diff's Unaligned
// line, and in Lines the note for each side that leaves the last line out.
// A note that leaves out one line says "line".
func TestDiffUnalignedEnds(t *testing.T) {
	// 51 lines of a and 5,000 of b, which share no line and which no
	// attempt of Myers' algorithm aligns.
	a := []byte(strings.Repeat("a\n", maxUnaligned) + "a")
	b := []byte(strings.Repeat("b\n", 4999) + "b")
	got, _ := Diff(a, b)
	if want := []Line{{Kind: Unaligned, Old: maxUnaligned + 1, New: 5000, OldNoNewline: true, NewNoNewline: true}}; !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	lines := strings.Split(strings.TrimSuffix(Lines(a, b), "\n"), "\n")
	if len(lines) != 2*maxUnaligned+4 ||
		lines[maxUnaligned] != "  ... (1 more line removed, which the diff can't align)" ||
		lines[maxUnaligned+1] != `- \ no final newline` || lines[maxUnaligned+2] != "+ b" ||
		lines[2*maxUnaligned+2] != fmt.Sprintf("  ... (%d more lines added, which the diff can't align)", 5000-maxUnaligned) ||
		lines[2*maxUnaligned+3] != `+ \ no final newline` {
		t.Errorf("Lines gives:\n%s", strings.Join(lines, "\n"))
	}
	// One line past the cap is "1 more line".
	if got := Lines([]byte(strings.Repeat("a\n", maxLines/2)), []byte(strings.Repeat("b\n", maxLines/2+1))); !strings.HasSuffix(got, "\n+ b\n  ... (1 more line of diff)\n") {
		t.Errorf("Lines of %d changed lines ends:\n%s", maxLines+1, got[max(0, len(got)-100):])
	}
}

// chain returns a file of k marker lines, each after a filler line, where
// each marker but the last appears again after the next: so the only line
// two such files each hold once is the last marker, and only with it taken
// as an anchor does the marker before it stand alone in the stretch before
// it, and so on down the file, one marker per pass.
func chain(k int, filler, first, last string) []byte {
	var b strings.Builder
	b.WriteString(first + "\n")
	for i := 1; i <= k; i++ {
		fmt.Fprintf(&b, "%s\nm%d\n", filler, i)
		if i >= 2 {
			fmt.Fprintf(&b, "m%d\n", i-1)
		}
	}
	b.WriteString(filler + "\n" + last + "\n")
	return []byte(b.String())
}

// TestDiffChain: two files that each pass for unique lines aligns one line
// of, at its stretch's end, and that no attempt of Myers' algorithm covers,
// so each pass leaves the next one nearly all of what it had. The whole
// diff's work, each cell of Myers' trace included, stays within maxWork
// plus perLine for each line of the files, and so does what it allocates,
// at 16 bytes a unit of work: a cell of the trace is 8. What is still to
// align once the work is spent is left unaligned, and Lines shows the
// start of each side of it under its cap.
func TestDiffChain(t *testing.T) {
	const k = 1500
	a, b := chain(k, "a", "first of a", "end of a"), chain(k, "b", "first of b", "end of b")
	na, nb := strings.Count(string(a), "\n"), strings.Count(string(b), "\n")
	most := maxWork + perLine*(na+nb)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got, work := Diff(a, b)
	runtime.ReadMemStats(&after)
	if work > most {
		t.Errorf("the diff did %d work, want at most %d", work, most)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > uint64(16*most) {
		t.Errorf("the diff allocated %d bytes, want at most %d", alloc, 16*most)
	}
	n := counts(got)
	old, new := 0, 0
	for _, l := range got {
		if l.Kind == Unaligned {
			old, new = old+l.Old, new+l.New
		}
	}
	if n[Unaligned] != 1 || n[' ']+n['-']+old != na || n[' ']+n['+']+new != nb {
		t.Errorf("the diff holds %v, and leaves %d lines of a and %d of b unaligned, want one unaligned stretch and every line of a's %d and b's %d", n, old, new, na, nb)
	}
	msg := Lines(a, b)
	if lines := strings.Count(msg, "\n"); !strings.HasPrefix(msg, "- first of a\n") || lines > maxLines+1 ||
		!strings.Contains(msg, fmt.Sprintf("\n  ... (%d more lines removed, which the diff can't align)\n+ first of b\n", old-maxUnaligned)) ||
		!strings.Contains(msg, fmt.Sprintf("\n  ... (%d more lines added, which the diff can't align)\n", new-maxUnaligned)) {
		t.Errorf("Lines gives %d lines:\n%s", lines, msg)
	}
}

// rewritten returns regions stretches of size lines, which each side
// writes its own way and each follows a line both sides hold once, then
// keys pairs of a key line and a blank line, with the key at change
// changed.
func rewritten(side string, regions, size, keys, change int) []byte {
	var b strings.Builder
	for r := range regions {
		fmt.Fprintf(&b, "region %d\n", r)
		for i := range size {
			fmt.Fprintf(&b, "%s %d %d\n", side, r, i)
		}
	}
	for i := range keys {
		if i == change {
			fmt.Fprintf(&b, "key %d, changed\n\n", i)
			continue
		}
		fmt.Fprintf(&b, "key %d\n\n", i)
	}
	return []byte(b.String())
}

// TestDiffSpent: rewritten stretches early in two files, which no attempt
// of Myers' algorithm covers, spend maxWork long before the diff reaches
// the rest. The rest is still aligned exactly, on each stretch's own work:
// the lines both files hold in the same place, and a line changed among
// them. So the diff holds each rewritten stretch as one Unaligned line,
// and the change as it is, whether the files share 10,000 lines after it
// or 20,000, and its work stays within maxWork plus perLine for each line
// of the files.
func TestDiffSpent(t *testing.T) {
	const regions, size = 8, 1100
	var want []Line
	for r := range regions {
		want = append(want, Line{Kind: ' ', Text: fmt.Sprintf("region %d", r)}, Line{Kind: Unaligned, Old: size, New: size})
	}
	want = append(want, Line{Kind: ' ', Text: "key 0"}, Line{Kind: ' '}, Line{Kind: ' ', Text: "key 1"}, Line{Kind: Skip},
		Line{Kind: ' '}, Line{Kind: ' ', Text: "key 3999"}, Line{Kind: ' '},
		Line{Kind: '-', Text: "key 4000"}, Line{Kind: '+', Text: "key 4000, changed"},
		Line{Kind: ' '}, Line{Kind: ' ', Text: "key 4001"}, Line{Kind: ' '})
	for _, keys := range []int{5000, 10_000} {
		a, b := rewritten("a", regions, size, keys, -1), rewritten("b", regions, size, keys, 4000)
		got, work := Diff(a, b)
		if !slices.Equal(got, want) {
			n := counts(got)
			t.Errorf("%d keys: the diff has %d lines, %d removed, %d added and %d unaligned, want:\n%+v", keys, len(got), n['-'], n['+'], n[Unaligned], want)
		}
		if most := maxWork + perLine*2*(regions*(size+1)+2*keys); work > most {
			t.Errorf("%d keys: the diff did %d work, want at most %d", keys, work, most)
		}
	}
}

// TestEditsOwnWork: with maxWork spent, a stretch aligns on its own work.
// It trims the lines its sides start and end with, a comparison for each
// and one more where each trim stops, and runs Myers' algorithm on the
// changed line between them: 6 entries for the furthest point on each
// diagonal, and 1, 3 and 5 cells for steps 0 to 2. The comparison that
// fails at step 0 is counted in its cell.
func TestEditsOwnWork(t *testing.T) {
	x, y, text := number(chars("abcxdef"), chars("abcydef"))
	w := &budget{}
	var ops []op
	edits(x, y, w, &ops)
	var got []string
	for _, o := range ops {
		got = append(got, string(o.kind)+strings.TrimSuffix(text[o.line], "\n"))
	}
	if want := []string{" a", " b", " c", "-x", "+y", " d", " e", " f"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if want := 4 + 4 + 6 + 1 + 3 + 5; w.spent != want {
		t.Errorf("the stretch did %d work, want %d", w.spent, want)
	}
	if _, work := Diff([]byte("a\n"), []byte("b\n")); work != 2+6+1+3+5 {
		t.Errorf(`Diff("a\n", "b\n") did %d work, want %d`, work, 2+6+1+3+5)
	}
}

// TestEditsSplitNoFurther: a stretch whose attempt of Myers' algorithm
// spends the rest of maxWork and some of the stretch's own work is split no
// further, though what is left of its own work would pay a pass for unique
// lines, and its sides hold such a line every 100 lines. So every line is
// left unaligned, and the work is at most what was left of maxWork plus
// perLine for each line. The test gives the budget itself, so it doesn't
// depend on maxWork.
func TestEditsSplitNoFurther(t *testing.T) {
	var a, b []string
	for i := range 3000 {
		if i%100 == 50 {
			both := fmt.Sprintf("both %d\n", i)
			a, b = append(a, both), append(b, both)
		}
		a, b = append(a, fmt.Sprintf("a %d\n", i)), append(b, fmt.Sprintf("b %d\n", i))
	}
	x, y, _ := number(a, b)
	lines := len(x) + len(y)
	const shared = maxCells - 10_000
	w := &budget{shared: shared}
	var ops []op
	edits(x, y, w, &ops)
	unaligned := 0
	for _, o := range ops {
		if o.unaligned {
			unaligned++
		}
	}
	if len(ops) != lines || unaligned != lines {
		t.Errorf("the script has %d ops, %d of them unaligned, want %d, all unaligned", len(ops), unaligned, lines)
	}
	if most := shared + perLine*lines; w.spent > most {
		t.Errorf("the stretch did %d work, want at most %d", w.spent, most)
	}
	if own := w.spent - shared; w.shared != 0 || own <= 0 || perLine*lines-own < lines {
		t.Errorf("the stretch did %d work, %d of it its own, and left %d of maxWork, want all of maxWork spent and some of its own work, with at least %d of it left", w.spent, own, w.shared, lines)
	}
}
