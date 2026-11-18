package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/gittest"
	"github.com/sdboyer/hypercase/internal/report"
	"github.com/sdboyer/hypercase/internal/revmap"
)

// TestInputBounds: the page's input lines stay within their caps, over a
// generated corpus whose changed outputs each map long sections that
// changed. Each archive's one cell has one or two outputs, each mapping
// the first l lines of each of k sections, where l is about
// report.MaxRangeLines or three times it, and k such ranges hold fewer
// lines than report.MaxFileLines, as many, or more.
//
// Each range shows its first report.MaxRangeLines lines, then a line
// saying how many more it leaves out. Each file shows its first
// report.MaxFileLines lines, then a note of how many more its ranges hold,
// of how many sections, and how many of those sections' diffs it leaves
// out. A section's diff is shown once per change, under its first file,
// and the other files say where. Two runs give the same bytes.
func TestInputBounds(t *testing.T) {
	t.Parallel()
	const m, f = report.MaxRangeLines, report.MaxFileLines
	n := 3 * m // each section's lines
	type cell struct{ k, l, outs int }
	var cells []cell
	for _, k := range []int{1, f / m, f/m + 1, 2 * f / m} {
		for _, l := range []int{m - 1, m, m + 1, n} {
			for _, outs := range []int{1, 2} {
				cells = append(cells, cell{k, l, outs})
			}
		}
	}
	name := func(c cell) string { return fmt.Sprintf("k%d-l%d-o%d", c.k, c.l, c.outs) }
	outputs := []string{"one.txt", "two.txt"}
	var revs [2]map[string]string
	for i := range 2 {
		revs[i] = map[string]string{"go.mod": "module example.com/m\n\ngo 1.27\n", "testdata/hypercase.json": string(moduleRegistry().Marshal())}
		for _, c := range cells {
			s := "A case.\n"
			var ranges []revmap.Range
			for sec := 1; sec <= c.k; sec++ {
				s += fmt.Sprintf("-- in/sheet/S%d --\n", sec)
				for line := 1; line <= n; line++ {
					s += fmt.Sprintf("S%d line %d", sec, line)
					if i == 1 && line == n {
						s += " at b"
					}
					s += "\n"
				}
				ranges = append(ranges, lines(fmt.Sprintf("in/sheet/S%d", sec), 1, c.l))
			}
			var files [][2]string
			var maps []revmap.File
			for _, o := range outputs[:c.outs] {
				files = append(files, [2]string{o, fmt.Sprintf("%d\n", i)})
				maps = append(maps, revmap.File{Path: o, Ranges: ranges})
			}
			revs[i]["testdata/sheet/"+name(c)+".txtar"] = s + cellOf("", "calc", "accepted", files, maps...)
		}
	}
	g := gittest.New(t)
	c := revisions(t, g, revs[0], revs[1])
	page := ok(t, g, "", "report", c[0], c[1])
	if again := ok(t, g, "", "report", c[0], c[1]); again != page {
		t.Errorf("two runs over the same revisions wrote different pages")
	}

	numbered := regexp.MustCompile(`<span class="ctx">\s*(\d+)  `)
	for _, cl := range cells {
		entry := change(t, page, "sheet/"+name(cl), "calc")
		what := name(cl) + " calc's change"
		shown := min(cl.l, m) // each range's lines
		total := cl.k * shown
		q, j := f/shown, f%shown // past the cap: the excerpt it falls in, and its lines shown
		excerpts := cl.k
		if total > f {
			excerpts = q
			if j > 0 {
				excerpts++
			}
		}
		files := strings.Split(entry, `<li class="changed">`)[1:]
		if len(files) != cl.outs {
			t.Fatalf("%s lists %d changed files, want %d:\n%s", what, len(files), cl.outs, entry)
		}
		for fi, file := range files {
			fwhat := what + ", " + outputs[fi] + ","
			var got int
			for _, pre := range regexp.MustCompile(`<pre class="lines">(.*?)</pre>`).FindAllStringSubmatch(file, -1) {
				got += len(numbered.FindAllString(pre[1], -1))
			}
			if want := min(total, f); got != want {
				t.Errorf("%s shows %d input lines, want %d:\n%s", fwhat, got, want, file)
			}
			if got := strings.Count(file, `<div class="excerpt">`); got != excerpts {
				t.Errorf("%s shows %d excerpts, want %d:\n%s", fwhat, got, excerpts, file)
			}
			// Each excerpt shown whole has the line that says what its range
			// leaves out, where it leaves out any.
			whole := excerpts
			if total > f && j > 0 {
				whole--
			}
			past := fmt.Sprintf("%d lines", cl.l-shown)
			if cl.l-shown == 1 {
				past = "1 line"
			}
			marker := fmt.Sprintf(`<span class="skip">  ... left out: %s past the first %d of a range</span>`, past, m)
			if want := map[bool]int{true: whole, false: 0}[cl.l > m]; strings.Count(file, marker) != want {
				t.Errorf("%s holds %q %d times, want %d:\n%s", fwhat, marker, strings.Count(file, marker), want, file)
			}
			note := "The page shows the first "
			if total <= f {
				lacks(t, fwhat, file, note)
			} else {
				left := (cl.k-q)*cl.l - j
				diffs := cl.k - excerpts
				want := fmt.Sprintf("%s%d lines the file&#39;s ranges hold, and leaves out %d more, of %d section", note, f, left, cl.k-q)
				if cl.k-q != 1 {
					want += "s"
				}
				if diffs > 0 {
					want += fmt.Sprintf(", and what changed in %d of them", diffs)
				}
				holds(t, fwhat, file, `<p class="note">`+want+".</p>")
			}
			// The first file shows each section's diff, and the second says
			// where it is.
			diffs := strings.Count(file, `</code> changed <span class="stat">`)
			pointers := strings.Count(file, `is shown under <code>one.txt</code>.</p>`)
			if fi == 0 && (diffs != excerpts+1 || pointers != 0) || fi == 1 && (diffs != 1 || pointers != excerpts) {
				t.Errorf("%s shows %d diffs and %d pointers to one.txt's, want %d and %d:\n%s", fwhat, diffs, pointers,
					map[bool]int{true: excerpts + 1, false: 1}[fi == 0], map[bool]int{true: 0, false: excerpts}[fi == 0], file)
			}
		}
	}
}
