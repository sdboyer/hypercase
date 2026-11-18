package report

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/diff"
	"github.com/sdboyer/hypercase/internal/revmap"
)

// TestExcerptLinesGenerated: over pairs of sets of at most two ranges, an
// excerpt holds each line that a range of either set holds among the
// range's first MaxRangeLines, in order, numbered from 1 and quoted as a
// diff quotes it. One line stands for those left out between two lines
// apart, and after the last where a range holds lines past its first
// MaxRangeLines, and counts those; no other line does. Where marks says
// to, each line is marked by whose ranges hold it. A range past the
// section's last line holds none. Each section is tried with and without
// its final newline.
//
// The ranges are drawn from the whole section and every run of lines of
// sections of up to four lines and one line past them, every pair of
// sets; and from the whole section and runs about MaxRangeLines long, from
// starts about it, of a section of three times as many lines, each set
// paired with no set and with each single range.
func TestExcerptLinesGenerated(t *testing.T) {
	type family struct {
		sizes  []int
		ranges []revmap.Range
		// pairOne pairs each set with no set and the single ranges alone.
		pairOne bool
	}
	small := family{sizes: []int{0, 1, 2, 3, 4}, ranges: []revmap.Range{{Section: "s"}}}
	for start := 1; start <= 5; start++ {
		for end := start; end <= 5; end++ {
			small.ranges = append(small.ranges, revmap.Range{Section: "s", Start: start, End: end})
		}
	}
	const m = MaxRangeLines
	long := family{sizes: []int{3 * m}, ranges: []revmap.Range{{Section: "s"}}, pairOne: true}
	for _, start := range []int{1, 2, m, 2 * m} {
		for _, n := range []int{1, m - 1, m, m + 1, 2 * m} {
			if end := start + n - 1; end <= 3*m+1 {
				long.ranges = append(long.ranges, revmap.Range{Section: "s", Start: start, End: end})
			}
		}
	}
	// holds reports whether a range holds line i of a section of n lines,
	// and shows whether the excerpt shows it.
	holds := func(r revmap.Range, i, n int) (holds, shows bool) {
		first, last := r.Start, r.End
		if first == 0 && last == 0 {
			first, last = 1, n
		}
		holds = last <= n && first <= i && i <= last
		return holds, holds && i < first+MaxRangeLines
	}
	for _, f := range []family{small, long} {
		sets := [][]revmap.Range{nil}
		for i := range f.ranges {
			sets = append(sets, f.ranges[i:i+1])
			for j := i + 1; j < len(f.ranges); j++ {
				sets = append(sets, []revmap.Range{f.ranges[i], f.ranges[j]})
			}
		}
		others := sets
		if f.pairOne {
			others = sets[:1]
			for _, r := range f.ranges {
				others = append(others, []revmap.Range{r})
			}
		}
		for _, n := range f.sizes {
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
					for _, b := range others {
						for _, marks := range []bool{true, false} {
							if !marks && b != nil {
								continue
							}
							var want []ExcerptLine
							last, cut := 0, 0
							for i := 1; i <= n; i++ {
								var held [2]bool
								shown := false
								for s, set := range [2][]revmap.Range{a, b} {
									for _, r := range set {
										h, sh := holds(r, i, n)
										held[s] = held[s] || h
										shown = shown || sh
									}
								}
								if !shown {
									if held[0] || held[1] {
										cut++
									}
									continue
								}
								if last > 0 && i > last+1 {
									want = append(want, ExcerptLine{Cut: cut})
								}
								last, cut = i, 0
								l := ExcerptLine{N: i, Text: diff.Display(text[i-1]), NoNewline: i == n && !final}
								switch {
								case !marks:
								case held[0] && held[1]:
									l.Mark = " "
								case held[0]:
									l.Mark = "-"
								default:
									l.Mark = "+"
								}
								want = append(want, l)
							}
							if cut > 0 {
								want = append(want, ExcerptLine{Cut: cut})
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
}
