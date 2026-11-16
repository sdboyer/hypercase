package diff

import (
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
			ops := edits(x, y)
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
