package diff

import (
	"fmt"
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
			var pairs int
			ops := edits(x, y, &pairs)
			changes := 0
			var gotA, gotB []string
			for _, o := range ops {
				switch o.kind {
				case ' ':
					gotA, gotB = append(gotA, o.line), append(gotB, o.line)
				case '-':
					gotA = append(gotA, o.line)
					changes++
				case '+':
					gotB = append(gotB, o.line)
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
	for range 1000 {
		a.WriteString("a\n")
		b.WriteString("b\n")
	}
	if got, _ := Diff([]byte(a.String()), []byte(b.String())); len(got) != 2000 {
		t.Errorf("a diff of 2,000 changed lines has %d lines, want every one", len(got))
	}
	if got, pairs := Diff([]byte("a\n"), []byte("a\n")); got != nil || pairs != 0 {
		t.Errorf("equal files: got %v, %d pairs", got, pairs)
	}
}

// TestDiffPairs: a diff of two 100,000-line files compares a number of
// line pairs that grows with their length and the size of the change, far
// under the 10^10 of a table of every pair: with one line changed in the
// middle, and with the first and last lines changed, which leave no common
// prefix or suffix to trim.
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
		got, pairs := Diff(lines(), lines(change...))
		if pairs >= 1_000_000 {
			t.Errorf("changing lines %v compared %d pairs of lines, want fewer than 1,000,000", change, pairs)
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

// TestDiffBudget: a change past the edit graph's budget shows as the
// changed region removed whole, then added whole, where a minimal diff
// would interleave them.
func TestDiffBudget(t *testing.T) {
	const n = 3000
	var a, b strings.Builder
	for i := range n {
		fmt.Fprintf(&a, "line %d\n", i)
		if i%2 == 1 {
			fmt.Fprintf(&b, "line %d, changed\n", i)
			continue
		}
		fmt.Fprintf(&b, "line %d\n", i)
	}
	got, _ := Diff([]byte(a.String()), []byte(b.String()))
	var kinds strings.Builder
	for _, l := range got {
		kinds.WriteByte(l.Kind)
	}
	want := " " + strings.Repeat("-", n-1) + strings.Repeat("+", n-1)
	if kinds.String() != want {
		t.Errorf("the diff's kinds begin %.40q, want one line of context, %d removed, then %d added", kinds.String(), n-1, n-1)
	}
}
