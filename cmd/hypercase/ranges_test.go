package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/gittest"
	"github.com/sdboyer/hypercase/internal/registry"
)

// registryAt returns a registry's bytes at an older format than today's,
// one from before registry.InputsFormat: what Marshal writes, less the
// keys registry.InputsFormat and the formats after it added.
func registryAt(r registry.Registry, format int) string {
	if format >= registry.InputsFormat {
		panic(fmt.Sprintf("registryAt writes formats before %d, not %d", registry.InputsFormat, format))
	}
	var v map[string]any
	if err := json.Unmarshal(r.Marshal(), &v); err != nil {
		panic(err)
	}
	v["format"] = format
	delete(v, "positions")
	delete(v, "checks")
	for _, list := range []string{"suites", "lifts"} {
		cols, _ := v[list].([]any)
		for _, c := range cols {
			for _, key := range []string{"outputs", "productFiles"} {
				outs, _ := c.(map[string]any)[key].([]any)
				for _, o := range outs {
					delete(o.(map[string]any), "inputs")
				}
			}
		}
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(data) + "\n"
}

// rangesModule returns a module as module does, with its registry at the
// format given, and the archives as they are given: a cell holds the
// inputs file it is given, or none.
func rangesModule(format int, archives map[string]string) map[string]string {
	reg := string(moduleRegistry().Marshal())
	if format < registry.InputsFormat {
		reg = registryAt(moduleRegistry(), format)
	}
	files := map[string]string{"go.mod": "module example.com/m\n\ngo 1.27\n", "testdata/hypercase.json": reg}
	for name, data := range archives {
		files["testdata/sheet/"+name+".txtar"] = data
	}
	return files
}

// TestRangesRank: a cell whose files are the same and whose inputs file
// differs, byte for byte, ranks ranges changed, after bytes changed. Under
// a file both revisions map, the lines of its map that only one holds are
// listed, A's marked - and B's +, whether its bytes changed or not, and in
// a cell of any rank; an added or removed file lists none. The inputs file
// is never listed as a file.
//
// Across the format that added inputs files, no cell ranks on its inputs
// file, and the page says once which revision records no ranges. Before
// it, an inputs file is a problem, left out. From it on, what the report
// can't read is a problem at its revision: an inputs file that doesn't
// parse, whose cell ranks on its bytes and lists no ranges; a cell that ran
// with none; one beside a cell that didn't run, whose maps it shows none
// of; a file it maps that the cell lacks; and a file of the cell it
// doesn't map.
func TestRangesRank(t *testing.T) {
	t.Parallel()
	const head = "A case.\n-- in/sheet/A1 --\n1\n"
	inputs := func(data string) string { return "-- out/calc/inputs --\n" + data }
	accepted := func(stdout string) string {
		return "-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n" + stdout + "\n"
	}
	const x = "testdata/sheet/x.txtar, out/calc/"
	noInputs := func(state string) string {
		return x + ": the cell is " + state + ", and has no inputs file, which every cell that ran has from registry format 8 on. So the report shows no ranges for its files."
	}
	for _, tt := range []struct {
		name    string
		formats [2]int
		a, b    string
		want    string
		// text is the problem -text writes at a and at b, if any.
		text [2]string
		// unranged is the revision the page says records no ranges, if
		// any: 0 for a, 1 for b.
		unranged int
	}{
		{
			name: "ranges alone",
			a:    head + inputs("stdout.txt\n  in/sheet/A1\n") + accepted("1"),
			b:    head + inputs("stdout.txt\n  in/sheet/A1:1-1\n") + accepted("1"),
			want: "ranges changed\n  sheet/x calc: accepted\n    stdout.txt ranges changed\n      - in/sheet/A1\n      + in/sheet/A1:1-1\n",
		},
		{
			name: "ranges and another file, ranked bytes changed",
			a:    head + inputs("stderr.txt\nstdout.txt\n  in/sheet/A1\n") + "-- out/calc/stderr.txt --\nw\n" + accepted("1"),
			b:    head + inputs("stderr.txt\nstdout.txt\n  in/sheet/A1:1-1\n") + "-- out/calc/stderr.txt --\nw2\n" + accepted("1"),
			want: "bytes changed\n  sheet/x calc: accepted\n    stderr.txt changed\n    stdout.txt ranges changed\n      - in/sheet/A1\n      + in/sheet/A1:1-1\n",
		},
		{
			name: "a changed file's ranges, less the lines both maps hold",
			a:    head + "-- in/sheet/B1 --\n2\n-- in/sheet/C1 --\n3\n" + inputs("stdout.txt\n  in/sheet/A1:1-1\n  in/sheet/B1\n") + accepted("1"),
			b:    head + "-- in/sheet/B1 --\n2\n-- in/sheet/C1 --\n3\n" + inputs("stdout.txt\n  in/sheet/A1:1-1\n  in/sheet/C1\n") + accepted("2"),
			want: "bytes changed\n  sheet/x calc: accepted\n    stdout.txt changed\n      - in/sheet/B1\n      + in/sheet/C1\n",
		},
		{
			name: "all inputs",
			a:    head + inputs("stdout.txt\n  all inputs\n") + accepted("1"),
			b:    head + inputs("stdout.txt\n  in/sheet/A1\n") + accepted("1"),
			want: "ranges changed\n  sheet/x calc: accepted\n    stdout.txt ranges changed\n      - all inputs\n      + in/sheet/A1\n",
		},
		{
			name: "ranges under a changed outcome",
			a:    head + inputs("stdout.txt\n  in/sheet/A1\n") + accepted("1"),
			b:    head + inputs("stdout.txt\n  in/sheet/A1:1-1\n") + "-- out/calc/outcome --\nrejected\n-- out/calc/stdout.txt --\n1\n",
			want: "outcome changed\n  sheet/x calc: accepted -> rejected\n    stdout.txt ranges changed\n      - in/sheet/A1\n      + in/sheet/A1:1-1\n",
		},
		{
			name: "an added and a removed file list no ranges",
			a:    head + inputs("stderr.txt\n  in/sheet/A1\n") + "-- out/calc/outcome --\naccepted\n-- out/calc/stderr.txt --\nw\n",
			b:    head + inputs("stdout.txt\n  in/sheet/A1\n") + accepted("1"),
			want: "bytes changed\n  sheet/x calc: accepted\n    stderr.txt removed\n    stdout.txt added\n",
		},
		{
			name:     "an inputs file at b only, with a before inputs files",
			formats:  [2]int{7, 8},
			a:        head + accepted("1"),
			b:        head + inputs("stdout.txt\n  in/sheet/A1\n") + accepted("1"),
			want:     "",
			unranged: 0,
		},
		{
			name:     "an inputs file at a only, with b before inputs files",
			formats:  [2]int{8, 7},
			a:        head + inputs("stdout.txt\n  in/sheet/A1\n") + accepted("1"),
			b:        head + accepted("1"),
			want:     "",
			unranged: 1,
		},
		{
			name: "an inputs file at b only, with a at inputs files",
			a:    head + accepted("1"),
			b:    head + inputs("stdout.txt\n  in/sheet/A1\n") + accepted("1"),
			want: "ranges changed\n  sheet/x calc: accepted\n",
			text: [2]string{noInputs("accepted"), ""},
		},
		{
			name: "an empty inputs file added",
			a:    head + "-- out/calc/outcome --\naccepted\n",
			b:    head + inputs("") + "-- out/calc/outcome --\naccepted\n",
			want: "ranges changed\n  sheet/x calc: accepted\n",
			text: [2]string{noInputs("accepted"), ""},
		},
		{
			name:    "an inputs file before inputs files",
			formats: [2]int{7, 7},
			a:       head + accepted("1"),
			b:       head + inputs("stdout.txt\n  in/sheet/A1\n") + accepted("1"),
			want:    "",
			text:    [2]string{"", x + "inputs: the section is an inputs file, and registry format 7 predates inputs files, so the report leaves it out."},
		},
		{
			name: "an inputs file that doesn't parse",
			a:    head + inputs("stdout.txt\n  in/sheet/A1\n") + accepted("1"),
			b:    head + inputs("stdout.txt\n  in/sheet/B1\n  in/sheet/A1\n") + accepted("1"),
			want: "ranges changed\n  sheet/x calc: accepted\n",
			text: [2]string{"", x + "inputs: the inputs file can't be read: it isn't as Format writes it: its files, or a file's ranges, are out of order, or two of a file's ranges are one. So the report shows no ranges for the cell's files."},
		},
		{
			name: "a file the map names and the cell lacks",
			a:    head + inputs("stdout.txt\n  in/sheet/A1\n") + accepted("1"),
			b:    head + inputs("stdout.txt\n  in/sheet/A1\nvalues.txt\n") + accepted("1"),
			want: "ranges changed\n  sheet/x calc: accepted\n",
			text: [2]string{"", x + "inputs: the inputs file maps values.txt, and the cell has no such file."},
		},
		{
			name: "a file of the cell the map doesn't name",
			a:    head + inputs("stdout.txt\n  in/sheet/A1\n") + accepted("1"),
			b:    head + inputs("") + accepted("2"),
			want: "bytes changed\n  sheet/x calc: accepted\n    stdout.txt changed\n",
			text: [2]string{"", x + "inputs: the inputs file doesn't map stdout.txt, a file of the cell, so the report shows no ranges for it."},
		},
		{
			name: "an inputs file beside a cell that didn't run",
			a:    head + inputs("stdout.txt\n  in/sheet/A1\n") + "-- out/calc/outcome --\nskipped no formulas\n-- out/calc/stdout.txt --\n1\n",
			b:    head + inputs("stdout.txt\n  in/sheet/A1:1-1\n") + "-- out/calc/outcome --\nskipped no formulas\n-- out/calc/stdout.txt --\n1\n",
			want: "ranges changed\n  sheet/x calc: skipped no formulas\n",
			text: [2]string{
				x + "inputs: the cell is skipped, and a cell that didn't run has no inputs file, so the report shows no ranges from it.",
				x + "inputs: the cell is skipped, and a cell that didn't run has no inputs file, so the report shows no ranges from it.",
			},
		},
		{
			name: "an inputs file with no outcome file is no cell's",
			a:    head + inputs(""),
			b:    head + inputs("stdout.txt\n"),
			want: "",
			text: [2]string{
				x + ": the cell has files and no outcome file. A cell exists where its outcome file does, so the report shows no cell here.",
				x + ": the cell has files and no outcome file. A cell exists where its outcome file does, so the report shows no cell here.",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			formats := tt.formats
			if formats == [2]int{} {
				formats = [2]int{registry.Format, registry.Format}
			}
			g := gittest.New(t)
			c := revisions(t, g, rangesModule(formats[0], map[string]string{"x": tt.a}), rangesModule(formats[1], map[string]string{"x": tt.b}))
			want := problemsText(c, tt.text) + tt.want
			if got := ok(t, g, "", "report", "-text", c[0], c[1]); got != want {
				t.Errorf("got:\n%s\nwant:\n%s", got, want)
			}
			page := ok(t, g, "", "report", c[0], c[1])
			changes := section(t, page, `<section id="changes">`, "</section>")
			const note = `<p class="note">`
			if formats[0] == formats[1] {
				lacks(t, "the changes", changes, note)
				return
			}
			at := c[tt.unranged]
			holds(t, "the changes", changes, note+`The newest registry at <span class="rev">`+at+`</span> is of format 7, from before inputs files, so the report shows no ranges at <span class="rev">`+at+`</span>, and ranks no cell on its inputs file.</p>`)
			if n := strings.Count(page, note); n != 1 {
				t.Errorf("the page says %d times that a revision records no ranges, want once", n)
			}
		})
	}
}

// TestRangesPage: the page ranks a change to ranges alone under its own
// heading, marks the cell so in the matrix, and lists under each file what
// changed in its map, after the file's diff where its bytes changed too.
func TestRangesPage(t *testing.T) {
	t.Parallel()
	const head = "A case.\n-- in/sheet/A1 --\n1\n"
	cell := func(lines, stdout string) string {
		return head + "-- out/calc/inputs --\nstdout.txt\n" + lines + "-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n" + stdout + "\n"
	}
	g := gittest.New(t)
	c := revisions(t, g,
		rangesModule(registry.Format, map[string]string{"r": cell("  in/sheet/A1\n", "1"), "s": cell("  in/sheet/A1\n", "1")}),
		rangesModule(registry.Format, map[string]string{"r": cell("  in/sheet/A1:1-1\n", "1"), "s": cell("  all inputs\n", "2")}))
	page := ok(t, g, "", "report", c[0], c[1])
	changes := section(t, page, `<section id="changes">`, "</section>")
	holds(t, "the changes", changes, `<h3 class="rank ranges-changed">ranges changed <span class="count">1</span></h3>`)
	holds(t, "r calc's change", change(t, page, "sheet/r", "calc"),
		`<li class="ranges-changed"><code>stdout.txt</code> ranges changed`+"\n"+
			`<pre class="ranges"><span class="del">- in/sheet/A1</span><span class="add">&#43; in/sheet/A1:1-1</span></pre>`+"\n"+`<details class="inputs">`)
	holds(t, "s calc's change", change(t, page, "sheet/s", "calc"),
		`<pre class="diff"><span class="del">- 1</span><span class="add">&#43; 2</span></pre></details>`+"\n"+
			`<pre class="ranges"><span class="del">- in/sheet/A1</span><span class="add">&#43; all inputs</span></pre>`+"\n"+`<details class="inputs">`)
	_, cells := matrixRow(t, page, "sheet", "sheet/r")
	holds(t, "r's calc", cells[0], `class="cell s-accepted ranges-changed"`, `<span class="badge">ranges changed</span>`)
	lacks(t, "the page", page, "<code>inputs</code>")
}

// TestRangesGenerated: over every pair of generated states of a cell, each
// an outcome, its one file's bytes and that file's reverse map, the cell
// ranks outcome changed where its outcome differs, bytes changed where
// only its file does, ranges changed where only its inputs file does, and
// is no change otherwise; and its file lists the lines of its map that
// only one revision's holds, A's marked - and B's +, whenever both differ,
// which every rank shows. With A's registry from before inputs files, whose
// cells commit none, no cell ranks on its inputs file, and no file lists
// ranges. Each pair is reported both ways.
func TestRangesGenerated(t *testing.T) {
	t.Parallel()
	type state struct {
		outcome, stdout string
		ranges          []string // the map's lines, as the inputs file writes them
	}
	var states []state
	for _, o := range []string{"accepted", "rejected"} {
		for _, out := range []string{"1", "2"} {
			for _, m := range [][]string{nil, {"in/sheet/A1"}, {"in/sheet/A1:1-1"}, {"in/sheet/A1:1-1", "in/sheet/B1"}, {"all inputs"}} {
				states = append(states, state{o, out, m})
			}
		}
	}
	archive := func(s state, inputs bool) string {
		a := "A case.\n-- in/sheet/A1 --\n1\n-- in/sheet/B1 --\n2\n"
		if inputs {
			a += "-- out/calc/inputs --\nstdout.txt\n"
			for _, r := range s.ranges {
				a += "  " + r + "\n"
			}
		}
		return a + "-- out/calc/outcome --\n" + s.outcome + "\n-- out/calc/stdout.txt --\n" + s.stdout + "\n"
	}
	// want is the text of the changes from x to y, each a revision's
	// states by archive, in name order, and whether both record ranges.
	want := func(names []string, x, y map[string]state, ranged bool) string {
		var ranks [5][]string
		for _, n := range names {
			sx, sy := x[n], y[n]
			differ := ranged && !slices.Equal(sx.ranges, sy.ranges)
			var lines []string
			if differ {
				for _, r := range sx.ranges {
					if !slices.Contains(sy.ranges, r) {
						lines = append(lines, "      - "+r)
					}
				}
				for _, r := range sy.ranges {
					if !slices.Contains(sx.ranges, r) {
						lines = append(lines, "      + "+r)
					}
				}
			}
			file := "    stdout.txt ranges changed"
			if sx.stdout != sy.stdout {
				file = "    stdout.txt changed"
			}
			if sx.stdout != sy.stdout || differ {
				lines = append([]string{file}, lines...)
			}
			head := "  sheet/" + n + " calc: "
			switch {
			case sx.outcome != sy.outcome:
				ranks[0] = append(ranks[0], head+sx.outcome+" -> "+sy.outcome)
				ranks[0] = append(ranks[0], lines...)
			case sx.stdout != sy.stdout:
				ranks[3] = append(ranks[3], head+sy.outcome)
				ranks[3] = append(ranks[3], lines...)
			case differ:
				ranks[4] = append(ranks[4], head+sy.outcome)
				ranks[4] = append(ranks[4], lines...)
			}
		}
		var b strings.Builder
		for i, heading := range []string{"outcome changed", "appeared", "disappeared", "bytes changed", "ranges changed"} {
			if len(ranks[i]) > 0 {
				b.WriteString(heading + "\n" + strings.Join(ranks[i], "\n") + "\n")
			}
		}
		return b.String()
	}
	for _, formatA := range []int{7, registry.Format} {
		t.Run(fmt.Sprint("a at format ", formatA), func(t *testing.T) {
			t.Parallel()
			at := [2]map[string]state{{}, {}}
			arch := [2]map[string]string{{}, {}}
			var names []string
			for i, sa := range states {
				for j, sb := range states {
					n := fmt.Sprintf("p%02d-%02d", i, j)
					names = append(names, n)
					at[0][n], at[1][n] = sa, sb
					arch[0][n], arch[1][n] = archive(sa, formatA >= registry.InputsFormat), archive(sb, true)
				}
			}
			g := gittest.New(t)
			c := revisions(t, g, rangesModule(formatA, arch[0]), rangesModule(registry.Format, arch[1]))
			ranged := formatA >= registry.InputsFormat
			if got, w := ok(t, g, "", "report", "-text", c[0], c[1]), want(names, at[0], at[1], ranged); got != w {
				t.Errorf("from a to b, the report is:\n%s\nwant:\n%s", got, w)
			}
			if got, w := ok(t, g, "", "report", "-text", c[1], c[0]), want(names, at[1], at[0], ranged); got != w {
				t.Errorf("from b to a, the report is:\n%s\nwant:\n%s", got, w)
			}
		})
	}
}
