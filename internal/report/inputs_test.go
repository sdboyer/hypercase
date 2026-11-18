package report

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/diff"
	"github.com/sdboyer/hypercase/internal/revmap"
)

// TestExcerptLinesGenerated: over every pair of sets of at most two ranges,
// drawn from the whole section and every run of lines of a section of up
// to four lines and one line past it, an excerpt holds each line that a
// range of either set holds, in order, numbered from 1 and quoted as a
// diff quotes it, with one line standing for those left out between two
// lines apart and none elsewhere. Where marks says to, each line is marked
// by whose ranges hold it. A range past the section's last line holds
// none. Each section is tried with and without its final newline.
func TestExcerptLinesGenerated(t *testing.T) {
	ranges := []revmap.Range{{Section: "s"}}
	for start := 1; start <= 5; start++ {
		for end := start; end <= 5; end++ {
			ranges = append(ranges, revmap.Range{Section: "s", Start: start, End: end})
		}
	}
	sets := [][]revmap.Range{nil}
	for i := range ranges {
		sets = append(sets, ranges[i:i+1])
		for j := i + 1; j < len(ranges); j++ {
			sets = append(sets, []revmap.Range{ranges[i], ranges[j]})
		}
	}
	// holds reports whether a set holds line i of a section of n lines.
	holds := func(set []revmap.Range, i, n int) bool {
		for _, r := range set {
			if r.Start == 0 && r.End == 0 || r.End <= n && r.Start <= i && i <= r.End {
				return true
			}
		}
		return false
	}
	for n := 0; n <= 4; n++ {
		var text []string
		for i := 1; i <= n; i++ {
			text = append(text, fmt.Sprintf("line %d", i))
		}
		if n >= 3 {
			text[2] = "a\x01b" // quoted
		}
		for _, final := range []bool{true, false} {
			data := strings.Join(text, "\n")
			if final && n > 0 {
				data += "\n"
			}
			for _, a := range sets {
				for _, b := range sets {
					for _, marks := range []bool{true, false} {
						if !marks && b != nil {
							continue
						}
						var want []ExcerptLine
						last := 0
						for i := 1; i <= n; i++ {
							ha, hb := holds(a, i, n), holds(b, i, n)
							if !ha && !hb {
								continue
							}
							if last > 0 && i > last+1 {
								want = append(want, ExcerptLine{})
							}
							last = i
							l := ExcerptLine{N: i, Text: diff.Display(text[i-1]), NoNewline: i == n && !final}
							switch {
							case !marks:
							case ha && hb:
								l.Mark = " "
							case ha:
								l.Mark = "-"
							default:
								l.Mark = "+"
							}
							want = append(want, l)
						}
						got := excerptLines([]byte(data), [2][]revmap.Range{a, b}, marks)
						if !reflect.DeepEqual(got, want) {
							t.Fatalf("over %q, with A's ranges %v and B's %v, marks %v, the excerpt is\n\t%+v\nwant\n\t%+v", data, a, b, marks, got, want)
						}
					}
				}
			}
		}
	}
}
