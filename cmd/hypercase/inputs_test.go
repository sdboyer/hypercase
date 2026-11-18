package main

import (
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/gittest"
	"github.com/sdboyer/hypercase/internal/registry"
	"github.com/sdboyer/hypercase/internal/revmap"
)

// cellOf returns a cell's sections, under the case prefix given: its
// inputs file, holding the maps given, its outcome, and its files.
func cellOf(prefix, column, outcome string, files [][2]string, maps ...revmap.File) string {
	dir := "-- " + prefix + "out/" + column + "/"
	s := dir + revmap.Name + " --\n" + string(revmap.Format(maps)) + dir + "outcome --\n" + outcome + "\n"
	for _, f := range files {
		s += dir + f[0] + " --\n" + f[1]
	}
	return s
}

// lines returns the range of a section's lines from start to end, or the
// whole section where both are 0.
func lines(section string, start, end int) revmap.Range {
	return revmap.Range{Section: section, Start: start, End: end}
}

// TestInputLines: beside each changed file, folded under "input lines",
// the page shows the lines its ranges point at, each numbered as its
// revision's section numbers it, with a line that stands for those left
// out between two lines apart: over B's map for a file at B, and over A's
// for a file only at A. Where the file's map changed, it shows both
// revisions' ranges, one excerpt marking whose ranges hold each line where
// the section and its note are the same at both, and each revision's over
// its own lines where they aren't. A section whose bytes changed shows its
// line diff, folded, after its last excerpt. A map of all inputs, or of no
// ranges, is shown as such. Ranges are shown into a hand variant's
// overlay, a transform's variant's own inputs, a bytes lift's product and
// a variant's product.
//
// Lines that may not be the ones the cell read carry a note: past a
// frozen bytes lift, and in the own inputs of a transform's variant no
// registry owns. A range into a section the archive doesn't hold, or past
// its last line, is a problem at its revision, and the page shows no lines
// of it. The text list shows no lines.
func TestInputLines(t *testing.T) {
	t.Parallel()
	reg := productRegistry()
	reg.Types[1].Transforms = []registry.Transform{
		{Name: "t", Description: "Owned.", Owned: true},
		{Name: "u", Description: "Owned by no registry."},
	}
	const w4 = "-- in/sheet/A1 --\nw\nx\ny\nz\n"
	out := func(data string) [][2]string { return [][2]string{{"stdout.txt", data}} }
	std := func(rs ...revmap.Range) revmap.File { return revmap.File{Path: "stdout.txt", Ranges: rs} }
	a1 := func(start, end int) revmap.Range { return lines("in/sheet/A1", start, end) }
	// both returns an archive at each revision, with V replaced by 1 at A
	// and 2 at B.
	both := func(s string) [2]string {
		return [2]string{strings.ReplaceAll(s, "V", "1"), strings.ReplaceAll(s, "V", "2")}
	}
	sheet := map[string][2]string{
		"later": both("A case.\n" + w4 + cellOf("", "calc", "accepted", out("V\n"), std(a1(1, 1), a1(3, 3)))),
		"gone": {
			"A case.\n" + w4 + cellOf("", "calc", "accepted", [][2]string{{"stderr.txt", "w\n"}, {"stdout.txt", "1\n"}}, revmap.File{Path: "stderr.txt", Ranges: []revmap.Range{a1(2, 2)}}, std()),
			"A case.\n" + w4 + cellOf("", "calc", "accepted", out("1\n"), std()),
		},
		"edited": {
			"A case.\n-- in/sheet/A1 --\n1\n2\n" + cellOf("", "calc", "accepted", out("1\n"), std(a1(0, 0))),
			"A case.\n-- in/sheet/A1 --\n1\n3\n" + cellOf("", "calc", "accepted", out("2\n"), std(a1(0, 0))),
		},
		"marked": {
			"A case.\n" + w4 + cellOf("", "calc", "accepted", out("1\n"), std(a1(1, 2))),
			"A case.\n" + w4 + cellOf("", "calc", "accepted", out("1\n"), std(a1(2, 3))),
		},
		"shifted": {
			"A case.\n-- in/sheet/A1 --\nw\nx\n" + cellOf("", "calc", "accepted", out("1\n"), std(a1(2, 2))),
			"A case.\n-- in/sheet/A1 --\nv\nw\nx\n" + cellOf("", "calc", "accepted", out("1\n"), std(a1(3, 3))),
		},
		"all":    both("A case.\n" + w4 + cellOf("", "calc", "accepted", out("V\n"), revmap.File{Path: "stdout.txt", All: true})),
		"allerr": both("A case.\n" + w4 + cellOf("", "calc", "rejected S201", [][2]string{{"error.txt", "V\n"}}, revmap.File{Path: "error.txt", All: true})),
		"none":   both("A case.\n" + w4 + cellOf("", "calc", "accepted", out("V\n"), std())),
		"overlay": both("A case.\n-- in/sheet/A1 --\n1\n" + cellOf("", "calc", "accepted", out("1\n"), std(a1(0, 0))) +
			"-- variant/v/about --\nA variant.\n-- variant/v/in/sheet/A1 --\n2\n" +
			cellOf("variant/v/", "calc", "accepted", out("V\n"), std(lines("variant/v/in/sheet/A1", 0, 0)))),
		"owned": both("A case.\n-- in/sheet/A1 --\n1\n" + cellOf("", "calc", "accepted", out("1\n"), std(a1(0, 0))) +
			"-- transform/t/in/sheet/A1 --\n=A1\n" +
			cellOf("transform/t/", "calc", "accepted", out("V\n"), std(lines("transform/t/in/sheet/A1", 0, 0)))),
		"unowned": both("A case.\n-- in/sheet/A1 --\n1\n" + cellOf("", "calc", "accepted", out("1\n"), std(a1(0, 0))) +
			"-- transform/u/in/sheet/A1 --\n=A1\n" +
			cellOf("transform/u/", "calc", "accepted", out("V\n"), std(lines("transform/u/in/sheet/A1", 0, 0)))),
		"bad": {
			"A case.\n" + w4 + cellOf("", "calc", "accepted", out("1\n"), std(a1(1, 1))),
			"A case.\n" + w4 + cellOf("", "calc", "accepted", out("2\n"), std(a1(1, 1), a1(3, 9), lines("in/sheet/Z9", 0, 0))),
		},
	}
	// A CSV's cell past its import reads the product import makes.
	csv := func(prefix, gap string) string {
		via := prefix + "via/import/sheet/A1"
		return "-- " + prefix + "in/csv/data --\na\n" + gap +
			cellOf(prefix, "import", "accepted", nil, revmap.File{Path: "via/import/sheet/A1"}) + "-- " + via + " --\n4\n" +
			cellOf(prefix, "calc", "accepted", out("V\n"), std(lines(via, 0, 0)))
	}
	const variant = "-- variant/v/about --\nA variant.\n"
	csvs := map[string][2]string{
		"via":      both("A CSV.\n" + csv("", "")),
		"frozen":   both("A CSV.\n" + csv("", "-- gaps --\nimport Waits on sheets#4.\n")),
		"vfrozen":  both("A CSV.\n" + csv("", "") + variant + csv("variant/v/", "-- variant/v/gaps --\nimport Waits on sheets#4.\n")),
		"vproduct": both("A CSV.\n" + csv("", "") + variant + csv("variant/v/", "")),
		// A gaps line freezes import at B alone, and calc's map changes.
		"freezing": {
			both("A CSV.\n" + csv("", ""))[0],
			strings.Replace(both("A CSV.\n" + csv("", "-- gaps --\nimport Waits on sheets#4.\n"))[1], "  via/import/sheet/A1\n", "  via/import/sheet/A1:1-1\n", 1),
		},
	}
	var revs [2]map[string]string
	for i := range 2 {
		revs[i] = map[string]string{"go.mod": "module example.com/m\n\ngo 1.27\n", "testdata/hypercase.json": string(reg.Marshal())}
		for name, a := range sheet {
			revs[i]["testdata/sheet/"+name+".txtar"] = a[i]
		}
		for name, a := range csvs {
			revs[i]["testdata/csv/"+name+".txtar"] = a[i]
		}
	}
	g := gittest.New(t)
	c := revisions(t, g, revs[0], revs[1])
	page := ok(t, g, "", "report", c[0], c[1])
	at := func(i int) string { return `<span class="rev">` + c[i] + `</span>` }
	excerpt := func(section string, revs ...int) string {
		s := `<div class="excerpt"><code>` + section + `</code> at ` + at(revs[0])
		for _, i := range revs[1:] {
			s += " and " + at(i)
		}
		return s + "\n" + `<pre class="lines">`
	}
	const fold = `<details class="inputs"><summary>input lines</summary>`

	later := change(t, page, "sheet/later", "calc")
	holds(t, "later calc's change", later, fold+"\n"+excerpt("in/sheet/A1", 1)+
		`<span class="ctx">   1  w</span><span class="skip">  ...</span><span class="ctx">   3  y</span></pre></div>`+"\n</details></li>")
	lacks(t, "later calc's change", later, at(0))

	holds(t, "gone calc's change", change(t, page, "sheet/gone", "calc"),
		`<li class="removed">`, excerpt("in/sheet/A1", 0)+`<span class="ctx">   2  x</span></pre></div>`)

	edited := change(t, page, "sheet/edited", "calc")
	holds(t, "edited calc's change", edited, excerpt("in/sheet/A1", 1)+`<span class="ctx">   1  1</span><span class="ctx">   2  3</span></pre>`+"\n"+
		`<details><summary><code>in/sheet/A1</code> changed <span class="stat"><span class="add">+1</span> <span class="del">-1</span></span></summary>`+"\n"+
		`<pre class="diff"><span class="ctx">  1</span><span class="del">- 2</span><span class="add">&#43; 3</span></pre></details></div>`)
	lacks(t, "edited calc's change", edited, excerpt("in/sheet/A1", 0))

	holds(t, "marked calc's change", change(t, page, "sheet/marked", "calc"),
		`<li class="ranges-changed">`, excerpt("in/sheet/A1", 0, 1)+
			`<span class="del">-    1  w</span><span class="ctx">     2  x</span><span class="add">&#43;    3  y</span></pre></div>`)
	shifted := change(t, page, "sheet/shifted", "calc")
	holds(t, "shifted calc's change", shifted,
		excerpt("in/sheet/A1", 0)+`<span class="ctx">   2  x</span></pre></div>`+"\n"+
			excerpt("in/sheet/A1", 1)+`<span class="ctx">   3  x</span></pre>`+"\n"+
			`<details><summary><code>in/sheet/A1</code> changed <span class="stat"><span class="add">+1</span> <span class="del">-0</span></span></summary>`)

	holds(t, "all calc's change", change(t, page, "sheet/all", "calc"), fold+"\n"+`<p class="map">At `+at(1)+`: all inputs.</p>`+"\n</details>")
	holds(t, "allerr calc's change", change(t, page, "sheet/allerr", "calc"), `<code>error.txt</code> changed`, `<p class="map">At `+at(1)+`: all inputs.</p>`)
	holds(t, "none calc's change", change(t, page, "sheet/none", "calc"), fold+"\n"+`<p class="map">At `+at(1)+`: no ranges.</p>`+"\n</details>")

	holds(t, "overlay calc's change", change(t, page, "sheet/overlay/variant/v", "calc"), excerpt("variant/v/in/sheet/A1", 1)+`<span class="ctx">   1  2</span></pre></div>`)
	owned := change(t, page, "sheet/owned/transform/t", "calc")
	holds(t, "owned calc's change", owned, excerpt("transform/t/in/sheet/A1", 1)+`<span class="ctx">   1  =A1</span></pre></div>`)
	lacks(t, "owned calc's change", owned, `<p class="note">`)
	holds(t, "unowned calc's change", change(t, page, "sheet/unowned/transform/u", "calc"),
		excerpt("transform/u/in/sheet/A1", 1)+`<span class="ctx">   1  =A1</span></pre>`+"\n"+
			`<p class="note">No registry owns the transform u, so no update writes its variant&#39;s own inputs, and these may not be the lines the cell read.</p></div>`)

	frozen := func(lift string) string {
		return `<p class="note">The cell of ` + lift + ` that makes these lines is frozen, so they stay as committed while the cells past it run on what ` + lift + ` makes now, and they may not be the lines this cell read.</p></div>`
	}
	via := change(t, page, "csv/via", "calc")
	holds(t, "via calc's change", via, excerpt("via/import/sheet/A1", 1)+`<span class="ctx">   1  4</span></pre></div>`)
	lacks(t, "via calc's change", via, `<p class="note">`)
	holds(t, "frozen calc's change", change(t, page, "csv/frozen", "calc"),
		excerpt("via/import/sheet/A1", 1)+`<span class="ctx">   1  4</span></pre>`+"\n"+frozen("import"))
	vproduct := change(t, page, "csv/vproduct/variant/v", "calc")
	holds(t, "vproduct's variant's calc's change", vproduct, excerpt("variant/v/via/import/sheet/A1", 1)+`<span class="ctx">   1  4</span></pre></div>`)
	lacks(t, "vproduct's variant's calc's change", vproduct, `<p class="note">`)
	holds(t, "vfrozen's variant's calc's change", change(t, page, "csv/vfrozen/variant/v", "calc"),
		excerpt("variant/v/via/import/sheet/A1", 1)+`<span class="ctx">   1  4</span></pre>`+"\n"+frozen("import"))
	lacks(t, "vfrozen calc's change", change(t, page, "csv/vfrozen", "calc"), `<p class="note">`)
	holds(t, "freezing calc's change", change(t, page, "csv/freezing", "calc"),
		excerpt("via/import/sheet/A1", 0)+`<span class="ctx">   1  4</span></pre></div>`+"\n"+
			excerpt("via/import/sheet/A1", 1)+`<span class="ctx">   1  4</span></pre>`+"\n"+frozen("import"))

	bad := change(t, page, "sheet/bad", "calc")
	holds(t, "bad calc's change", bad, excerpt("in/sheet/A1", 0, 1)+`<span class="ctx">     1  w</span></pre></div>`)
	lacks(t, "bad calc's change", bad, "<code>in/sheet/Z9</code>", `  3  y`)
	text := ok(t, g, "", "report", "-text", c[0], c[1])
	const x = "testdata/sheet/bad.txtar, out/calc/inputs: the inputs file maps stdout.txt to "
	if want := "problems\n  at " + c[1] + "\n" +
		"    " + x + "in/sheet/A1:3-9, and in/sheet/A1 has 4 lines, so the report shows no lines of the range.\n" +
		"    " + x + "in/sheet/Z9, and the archive holds no section in/sheet/Z9, so the report shows no lines of the range.\n"; !strings.HasPrefix(text, want+"bytes changed\n") {
		t.Errorf("the text list is\n%s\nwant it to start with\n%s", text, want)
	}
	lacks(t, "the text list", text, "   1  w", "input lines")
}
