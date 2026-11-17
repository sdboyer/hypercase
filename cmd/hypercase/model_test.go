package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/gittest"
	"github.com/sdboyer/hypercase/internal/registry"
)

// TestPageStates: each of an outcome's five states shows in the matrix,
// with its detail, panicked among them.
func TestPageStates(t *testing.T) {
	t.Parallel()
	reg := moduleRegistry()
	for _, name := range []string{"format", "parse", "resolve"} {
		reg.Suites = append(reg.Suites, registry.Suite{Name: name, Description: "Runs " + name + ".", Consumes: registry.RawInput, Corpora: []string{"sheet"}})
	}
	files := module(map[string]string{"x": "A case.\n-- in/sheet/A1 --\n1\n" +
		"-- out/calc/outcome --\naccepted div-zero\n-- out/evaluate/outcome --\nblocked resolve\n-- out/format/outcome --\nskipped no formulas\n" +
		"-- out/parse/error.txt --\nruntime error: index out of range\n-- out/parse/outcome --\npanicked\n-- out/resolve/outcome --\nrejected S201 S202\n"})
	files["testdata/hypercase.json"] = string(reg.Marshal())
	g := gittest.New(t)
	c := revisions(t, g, module(nil), files)
	_, cells := matrixRow(t, ok(t, g, "", "report", c[0], c[1]), "sheet", "sheet/x")
	for i, want := range []string{"accepted div-zero", "blocked resolve", "skipped no formulas", "panicked", "rejected S201 S202"} {
		state, _, _ := strings.Cut(want, " ")
		holds(t, "x's "+state+" cell", cells[i], `class="cell s-`+state+` appeared"><span class="outcome">`+want+`</span>`)
	}
}

// rowNames returns the cases of a corpus's matrix, in order.
func rowNames(t *testing.T, page, corpus string) []string {
	t.Helper()
	m := section(t, page, `<section class="matrix" id="corpus-`+corpus+`">`, "</section>")
	var rows []string
	for _, r := range regexp.MustCompile(`<th scope="row"><span class="case">([^<]*)</span>`).FindAllStringSubmatch(m, -1) {
		rows = append(rows, r[1])
	}
	return rows
}

// TestPageRows: an archive's base comes first, then its hand-written
// variants by name, then its transforms' variants by transform name, each
// named as a subtest is, and the next archive's cases after them.
func TestPageRows(t *testing.T) {
	t.Parallel()
	reg := moduleRegistry()
	reg.Types = []registry.Type{{Dir: "sheet", Description: "A sheet.", GoType: "example.com/m.Sheet", Transforms: []registry.Transform{
		{Name: "doubled", Description: "Doubles each number.", Owned: true},
		{Name: "blanked", Description: "Empties each cell.", Owned: true},
	}}}
	const out = "out/calc/outcome --\naccepted\n"
	files := module(map[string]string{
		"x": "A case.\n-- in/sheet/A1 --\n1\n-- " + out +
			"-- transform/doubled/in/sheet/A1 --\n2\n-- transform/doubled/" + out +
			"-- transform/blanked/in/sheet/A1 --\n-- transform/blanked/" + out +
			"-- variant/two/about --\nA1 is 2.\n-- variant/two/in/sheet/A1 --\n2\n-- variant/two/" + out +
			"-- variant/one/about --\nA1 is 1 again.\n-- variant/one/in/sheet/B1 --\n1\n-- variant/one/" + out,
		"x-and": "Another case.\n-- in/sheet/A1 --\n1\n-- " + out,
	})
	files["testdata/hypercase.json"] = string(reg.Marshal())
	g := gittest.New(t)
	c := revisions(t, g, module(nil), files)
	page := ok(t, g, "", "report", c[0], c[1])
	want := []string{"sheet/x", "sheet/x/variant/one", "sheet/x/variant/two", "sheet/x/transform/blanked", "sheet/x/transform/doubled", "sheet/x-and"}
	if got := rowNames(t, page, "sheet"); !slices.Equal(got, want) {
		t.Errorf("the matrix's rows are %q, want %q", got, want)
	}
	text := ok(t, g, "", "report", "-text", c[0], c[1])
	var listed []string
	for _, l := range strings.Split(strings.TrimSpace(text), "\n")[1:] {
		listed = append(listed, strings.TrimSuffix(strings.TrimSpace(l), " calc: accepted"))
	}
	if !slices.Equal(listed, want) {
		t.Errorf("the ranked changes list %q, want %q", listed, want)
	}
}

// TestTwoCorpora: two archives of one name in different corpus types are
// two archives, each in its own corpus's matrix, and two changes.
func TestTwoCorpora(t *testing.T) {
	t.Parallel()
	reg := moduleRegistry()
	reg.Suites = append(reg.Suites, registry.Suite{Name: "export", Description: "Exports a CSV.", Consumes: registry.RawInput, Corpora: []string{"csv"}})
	a := module(map[string]string{"x": "A sheet.\n-- in/sheet/A1 --\n1\n-- out/calc/outcome --\naccepted\n"})
	a["testdata/hypercase.json"] = string(reg.Marshal())
	a["testdata/csv/x.txtar"] = "A CSV.\n-- in/csv/x.csv --\n1\n-- out/export/outcome --\naccepted\n"
	b := module(map[string]string{"x": "A sheet.\n-- in/sheet/A1 --\n1\n-- out/calc/outcome --\nrejected\n"})
	b["testdata/hypercase.json"] = a["testdata/hypercase.json"]
	b["testdata/csv/x.txtar"] = "A CSV.\n-- in/csv/x.csv --\n1\n-- out/export/outcome --\nrejected\n"
	g := gittest.New(t)
	c := revisions(t, g, a, b)
	want := "outcome changed\n  csv/x export: accepted -> rejected\n  sheet/x calc: accepted -> rejected\n"
	if got := ok(t, g, "", "report", "-text", c[0], c[1]); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	page := ok(t, g, "", "report", c[0], c[1])
	for corpus, rows := range map[string][]string{"csv": {"csv/x"}, "sheet": {"sheet/x"}} {
		if got := rowNames(t, page, corpus); !slices.Equal(got, rows) {
			t.Errorf("the %s matrix's rows are %q, want %q", corpus, got, rows)
		}
	}
}

// TestTwoRoots: two corpus roots in one module. Each root's report merges
// only the registries that record it, and shows only its own cells. -root
// names the second root, relative to the current directory.
func TestTwoRoots(t *testing.T) {
	t.Parallel()
	root := func(dir string, archive, desc string) map[string]string {
		reg := moduleRegistry()
		reg.CorpusRoot = dir
		reg.Suites = []registry.Suite{{Name: "calc", Description: desc, Consumes: registry.RawInput, Corpora: []string{"sheet"}}}
		return map[string]string{
			dir + "/testdata/hypercase.json":   string(reg.Marshal()),
			dir + "/testdata/sheet/" + archive: "A case.\n-- in/sheet/A1 --\n1\n-- out/calc/outcome --\naccepted\n",
		}
	}
	files := map[string]string{"go.mod": "module example.com/m\n\ngo 1.27\n"}
	for _, r := range []map[string]string{root("one", "first.txtar", "Runs calc in one."), root("two", "second.txtar", "Runs calc in two.")} {
		for p, data := range r {
			files[p] = data
		}
	}
	g := gittest.New(t)
	c := revisions(t, g, map[string]string{"go.mod": files["go.mod"]}, files)
	for _, tt := range []struct{ dir, root, want, desc, other string }{
		{"one", "", "appeared\n  sheet/first calc: accepted\n", "Runs calc in one.", "two"},
		{"two", "", "appeared\n  sheet/second calc: accepted\n", "Runs calc in two.", "one"},
		{"one", "../two", "appeared\n  sheet/second calc: accepted\n", "Runs calc in two.", "one"},
		{"", "two", "appeared\n  sheet/second calc: accepted\n", "Runs calc in two.", "one"},
	} {
		args := []string{"report"}
		if tt.root != "" {
			args = append(args, "-root", tt.root)
		}
		if got := ok(t, g, tt.dir, append(append(args, "-text"), c[0], c[1])...); got != tt.want {
			t.Errorf("in %q with -root %q, the report is:\n%s\nwant:\n%s", tt.dir, tt.root, got, tt.want)
		}
		page := ok(t, g, tt.dir, append(args, c[0], c[1])...)
		holds(t, "the page", page, "<dt>calc</dt><dd>"+tt.desc+"</dd>")
		lacks(t, "the page", page, "Runs calc in "+tt.other+".", `<section id="problems">`)
	}
}

// TestCorpusMissing: a revision before the corpus root's registries, at a,
// makes every cell at b appear. At b, the root is wrong, and the report
// fails, saying so.
func TestCorpusMissing(t *testing.T) {
	t.Parallel()
	g := gittest.New(t)
	c := revisions(t, g,
		map[string]string{"README": "Before hypercase.\n"},
		module(map[string]string{"x": "A case.\n-- out/calc/outcome --\naccepted\n-- out/evaluate/outcome --\nrejected\n"}))
	want := "appeared\n  sheet/x calc: accepted\n  sheet/x evaluate: rejected\n"
	if got := ok(t, g, "", "report", "-text", c[0], c[1]); got != want {
		t.Errorf("from a revision with no corpus, the report is:\n%s\nwant:\n%s", got, want)
	}
	fails(t, g, "", 1, []string{"report", c[1], c[0]},
		"hypercase report: no registry records the corpus root . at "+c[0]+", though some do at "+c[1]+", and the report shows the corpus at its second revision.\n")
	g = gittest.New(t)
	c = revisions(t, g, module(nil), map[string]string{"go.mod": "module example.com/m\n\ngo 1.27\n"})
	fails(t, g, "", 1, []string{"report", c[0], c[1]}, "hypercase report: no registry records the corpus root . at "+c[1]+", though some do at "+c[0]+",")
}

// TestProblems: what the report can't show as it is, it lists as a problem
// at its revision, and doesn't pick: a stated name no registry lists; a
// gaps line naming a column or a transform no registry has; a column two
// registries claim; a committed column no registry claims; a section it
// can't place, such as via/ before a later step teaches it; a cell with
// files and no outcome; and a transform's variant no corpus type lists.
func TestProblems(t *testing.T) {
	t.Parallel()
	reg := moduleRegistry()
	reg.Types = []registry.Type{{Dir: "sheet", Description: "A sheet.", GoType: "example.com/m.Sheet"}}
	other := registry.Registry{UpdateVar: "M_UPDATE_GOLDEN", CorpusRoot: ".", Types: reg.Types,
		Suites: []registry.Suite{{Name: "evaluate", Description: "Evaluates, as another package says.", Consumes: registry.RawInput, Corpora: []string{"sheet"}}}}
	files := module(map[string]string{
		"x": "A case.\n-- in/sheet/A1 --\n1\n-- properties --\ncircular\nlooped\n" +
			"-- gaps --\nnope Waits on sheets#1.\ntransform/gone calc Waits on sheets#2.\n" +
			"-- out/calc/outcome --\naccepted\n-- out/old/outcome --\naccepted\n-- out/format/stdout.txt --\n1\n" +
			"-- via/import/sheet/A1 --\n1\n-- delete --\nsheet/A1\n-- out/outcome --\naccepted\n" +
			"-- variant//out/calc/outcome --\naccepted\n-- variant/v/in/sheet/A1 --\n2\n-- variant/v/properties --\nmissing\n" +
			"-- transform/t/in/sheet/A1 --\n2\n-- transform/t/out/calc/outcome --\naccepted\n-- transform/t/about --\nNo.\n",
	})
	files["testdata/hypercase.json"] = string(reg.Marshal())
	files["pkg/testdata/hypercase.json"] = string(other.Marshal())
	g := gittest.New(t)
	c := revisions(t, g, module(nil), files)
	page := ok(t, g, "", "report", c[0], c[1])
	ps := section(t, page, `<section id="problems">`, "</section>")
	const x = "<code>testdata/sheet/x.txtar"
	want := []string{
		`<h2>Problems <span class="count">13</span></h2>`,
		`<h3>At <span class="rev">` + c[1] + `</span></h3>`,
		"<li>testdata/hypercase.json and pkg/testdata/hypercase.json each claim the column evaluate, and a column&#39;s name is unique per corpus root, so the report shows neither one&#39;s description of it.</li>",
		x + ", properties, line 2</code>: the case states looped, and no registry lists a property of that name.</li>",
		x + ", gaps, line 1</code>: the line names the column nope, and no registry claims a column of that name over testdata/sheet/.</li>",
		x + ", gaps, line 2</code>: the line names transform/gone, and no corpus type of testdata/sheet/ lists a transform of that name.</li>",
		x + ", via/import/sheet/A1</code>: the section is a product of import, and no registry declares a bytes lift of that name over testdata/sheet/, so the report leaves it out, and doesn&#39;t compare its cell&#39;s product.</li>",
		x + ", delete</code>: the report can&#39;t place the section",
		x + ", out/outcome</code>: the report can&#39;t place the section",
		x + ", variant//out/calc/outcome</code>: the report can&#39;t place the section",
		x + ", variant/v/properties, line 1</code>: the case states missing, and no registry lists a property of that name.</li>",
		x + ", transform/t/about</code>: the report can&#39;t place the section",
		x + ", out/format/</code>: the cell has files and no outcome file. A cell exists where its outcome file does, so the report shows no cell here.</li>",
		x + ", transform/t/</code>: no corpus type of testdata/sheet/ lists the transform t, so the report can&#39;t say what its variant states.</li>",
		"<code>testdata/sheet/</code>: the directory holds 1 cell of the column old, and no registry claims a column of that name over it.</li>",
	}
	holds(t, "the problems", ps, want...)
	if n := strings.Count(ps, "<li>"); n != 13 {
		t.Errorf("the page lists %d problems, want 13:\n%s", n, ps)
	}
	// The report doesn't pick a description for evaluate, and shows what
	// it placed.
	holds(t, "evaluate's description", page, "<dt>evaluate</dt><dd></dd>")
	_, cells := matrixRow(t, page, "sheet", "sheet/x")
	var cols []string
	m := section(t, page, `<section class="matrix" id="corpus-sheet">`, "</section>")
	for _, c := range regexp.MustCompile(`<th scope="col"[^>]*>([^<]*)</th>`).FindAllStringSubmatch(m, -1) {
		cols = append(cols, c[1])
	}
	if want := []string{"case", "calc", "evaluate", "nope", "old"}; !slices.Equal(cols, want) {
		t.Errorf("the matrix's columns are %q, want %q", cols, want)
	}
	holds(t, "x's old", cells[3], `class="cell s-accepted appeared"`)
	holds(t, "x's nope", cells[2], `<div class="gap">Waits on`)
	if got := rowNames(t, page, "sheet"); !slices.Equal(got, []string{"sheet/x", "sheet/x/variant/v", "sheet/x/transform/gone", "sheet/x/transform/t"}) {
		t.Errorf("the matrix's rows are %q", got)
	}

	// Problems at both revisions are listed at b, then at a.
	later := module(map[string]string{"x": "A case.\n-- properties --\nlater\n-- out/calc/outcome --\naccepted\n"})
	c = append(c, revisions(t, g, later)...)
	both := section(t, ok(t, g, "", "report", c[1], c[2]), `<section id="problems">`, "</section>")
	atB, atA := `<h3>At <span class="rev">`+c[2]+`</span></h3>`, `<h3>At <span class="rev">`+c[1]+`</span></h3>`
	holds(t, "the problems at both", both, `<h2>Problems <span class="count">14</span></h2>`, atB, atA, "the case states later")
	if strings.Index(both, atB) > strings.Index(both, atA) {
		t.Errorf("the problems at a come before b's:\n%s", both)
	}
	if strings.Index(both, "the case states later") > strings.Index(both, atA) {
		t.Errorf("b's problem is listed at a:\n%s", both)
	}
}

// TestPageDiffs: each file of a changed cell that changed shows its line
// diff, with how many lines it adds and removes: an added file as all
// added, a removed one as all removed.
func TestPageDiffs(t *testing.T) {
	t.Parallel()
	g, a, b := sheets(t)
	page := ok(t, g, "step09", "report", a, b)
	holds(t, "loop calc's change", change(t, page, "sheet/loop", "calc"),
		`<li class="changed"><details><summary><code>error.txt</code> changed <span class="stat"><span class="add">+1</span> <span class="del">-1</span></span></summary>`+"\n"+
			`<pre class="diff"><span class="del">- A1 -&gt; B1 -&gt; A1: cycle</span><span class="add">&#43; cycle: A1 -&gt; B1 -&gt; A1</span><span class="ctx">    sheet/A1:1: =B1</span><span class="ctx">    sheet/B1:1: =A1</span></pre></details></li>`)
	holds(t, "div calc's change", change(t, page, "sheet/div", "calc"),
		`<code>error.txt</code> added <span class="stat"><span class="add">+1</span> <span class="del">-0</span></span></summary>`+"\n"+
			`<pre class="diff"><span class="add">&#43; C1: division by zero</span></pre>`,
		`<code>stdout.txt</code> removed <span class="stat"><span class="add">+0</span> <span class="del">-1</span></span></summary>`+"\n"+
			`<pre class="diff"><span class="del">- 1,0,#DIV/0!</span></pre>`)
}

// TestPageLargeDiff: a 100,000-line output with six lines changed far
// apart shows exactly those lines and their context, counted, so the page
// grows with the change, not with the file. A stretch the diff can't align
// is left out, marked, and its file shows no counts, which wouldn't count
// its change.
func TestPageLargeDiff(t *testing.T) {
	t.Parallel()
	big := func(change ...int) string {
		var b strings.Builder
		for i := range 100_000 {
			if slices.Contains(change, i) {
				fmt.Fprintf(&b, "line %d, changed\n", i)
				continue
			}
			fmt.Fprintf(&b, "line %d\n", i)
		}
		return b.String()
	}
	// Ten thousand lines of a and b, in an order no edit script within the
	// diff's budget aligns, and no line either holds once.
	unaligned := func(first string, mul, mod, below int) string {
		var b strings.Builder
		b.WriteString(first + "\n")
		for i := range 9998 {
			if i*mul%mod < below {
				b.WriteString("a\n")
			} else {
				b.WriteString("b\n")
			}
		}
		b.WriteString(first + " again\n")
		return b.String()
	}
	const head = "A case.\n-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n"
	g := gittest.New(t)
	c := revisions(t, g,
		module(map[string]string{"x": head + big(), "y": head + unaligned("first of a", 7919, 13, 6)}),
		module(map[string]string{"x": head + big(3, 17_000, 33_333, 50_001, 81_234, 99_990), "y": head + unaligned("first of b", 104_729, 17, 8)}))
	page := ok(t, g, "", "report", c[0], c[1])
	x := change(t, page, "sheet/x", "calc")
	holds(t, "x calc's change", x, `<code>stdout.txt</code> changed <span class="stat"><span class="add">+6</span> <span class="del">-6</span></span>`,
		`<span class="ctx">  line 2</span><span class="del">- line 3</span><span class="add">&#43; line 3, changed</span><span class="ctx">  line 4</span>`)
	if n := strings.Count(x, `<span class="skip">`); n != 5 {
		t.Errorf("x calc's diff has %d skips between its hunks, want 5", n)
	}
	y := change(t, page, "sheet/y", "calc")
	holds(t, "y calc's change", y, `<code>stdout.txt</code> changed <span class="stat">partly unaligned</span>`,
		`<pre class="diff"><span class="unaligned">  ... left out: 10000 lines before and 10000 lines after, which the diff can&#39;t align</span></pre>`)
	if len(x) > 4<<10 || len(y) > 1<<10 || len(page) > 24<<10 {
		t.Errorf("the page is %d bytes, x calc's change %d and y calc's %d, want at most 24 KiB, 4 KiB and 1 KiB", len(page), len(x), len(y))
	}
}

// TestPageGolden: the page of the sheets fixture is the same on every run,
// and the golden records it.
func TestPageGolden(t *testing.T) {
	t.Parallel()
	g, a, b := sheets(t)
	page := ok(t, g, "step09", "report", a, b)
	if again := ok(t, g, "step09", "report", a, b); again != page {
		t.Error("two runs over the same revisions wrote different pages")
	}
	golden(t, "sheets.html", page)
}

// TestPageEscapes: what an archive holds is escaped on the page, so a
// </script> in a file, a description or a gaps line is text, never markup.
func TestPageEscapes(t *testing.T) {
	t.Parallel()
	const evil = `</script><script>alert(1)</script>`
	g := gittest.New(t)
	c := revisions(t, g,
		module(map[string]string{"x": "A case.\n-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\nfine\n"}),
		module(map[string]string{"x": "A case " + evil + ".\n-- gaps --\ncalc Waits on " + evil + ".\n-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n" + evil + "\n"}))
	page := ok(t, g, "", "report", c[0], c[1])
	lacks(t, "the page", page, "<script", "</script")
	const escaped = `&lt;/script&gt;&lt;script&gt;alert(1)&lt;/script&gt;`
	head, cells := matrixRow(t, page, "sheet", "sheet/x")
	holds(t, "x's row", head, "A case "+escaped+".")
	holds(t, "x's calc", cells[0], "Waits on "+escaped+".")
	holds(t, "x calc's change", change(t, page, "sheet/x", "calc"), `<span class="add">&#43; `+escaped+`</span>`)
}

// TestPageDescriptions: the page describes each corpus type, column,
// output declaration, classifier, transform and property, from b, or from
// a for what b lacks, which it marks; and each archive and hand-written
// variant only at a, from a.
func TestPageDescriptions(t *testing.T) {
	t.Parallel()
	reg := func(version string) registry.Registry {
		r := moduleRegistry()
		r.Properties = append(r.Properties, registry.Property{Name: "dangling", Description: "A reference to nothing, " + version + "."})
		r.Types = []registry.Type{{Dir: "sheet", Description: "A sheet, " + version + ".", GoType: "example.com/m.Sheet", Transforms: []registry.Transform{
			{Name: "looped", Description: "Loops, " + version + ".", Gives: []string{"circular"}, Owned: true},
		}}}
		r.Suites[0].Description = "Runs calc, " + version + "."
		r.Suites[0].Outputs = []registry.Output{{Pattern: "stdout.txt", Description: "What calc prints, " + version + "."}}
		r.Suites[1].Classifier = &registry.Classifier{Name: "div-zero", Description: "Adds div-zero, " + version + "."}
		return r
	}
	ra, rb := reg("at a"), reg("at b")
	// b drops calc's stdout.txt and evaluate's classifier; a's describe
	// them. calc's classifier is at both, and b's describes it.
	ra.Suites[0].Outputs = append(ra.Suites[0].Outputs, registry.Output{Pattern: "stderr.txt", Description: "What calc warns, at a."})
	rb.Suites[0].Outputs = []registry.Output{{Pattern: "stderr.txt", Description: "What calc warns, at b."}}
	ra.Suites[0].Classifier = &registry.Classifier{Name: "slow", Description: "Adds slow, at a."}
	rb.Suites[0].Classifier = &registry.Classifier{Name: "slow", Description: "Adds slow, at b."}
	rb.Suites[1].Classifier = nil
	ra.Suites = append(ra.Suites, registry.Suite{Name: "old", Description: "A column only a has.", Consumes: registry.RawInput, Corpora: []string{"sheet"},
		Outputs: []registry.Output{{Pattern: "old.txt", Description: "What old wrote."}}})
	ra.Types[0].Transforms = append(ra.Types[0].Transforms, registry.Transform{Name: "doubled", Description: "A transform only a has.", Keeps: []string{"dangling"}, Owned: true})
	ra.Properties = append(ra.Properties, registry.Property{Name: "retired", Description: "A property only a lists."})
	rb.Types = append(rb.Types, registry.Type{Dir: "csv", Description: "A CSV, only at b.", GoType: "example.com/m.CSV"})
	ra.Types = append(ra.Types, registry.Type{Dir: "tsv", Description: "A TSV, only at a.", GoType: "example.com/m.TSV"})

	const kept = "A case both have.\n-- in/sheet/A1 --\n1\n-- out/calc/outcome --\naccepted\n" +
		"-- variant/v/about --\nA variant both have, at %s.\n-- variant/v/in/sheet/A1 --\n2\n-- variant/v/out/calc/outcome --\naccepted\n"
	a := module(map[string]string{
		"kept": strings.ReplaceAll(kept, "%s", "a"),
		"gone": "A case only a has.\n-- in/sheet/A1 --\n1\n-- out/calc/outcome --\naccepted\n" +
			"-- variant/w/about --\nA variant only a has.\n-- variant/w/in/sheet/A1 --\n2\n-- variant/w/out/calc/outcome --\naccepted\n",
	})
	a["testdata/hypercase.json"] = string(ra.Marshal())
	b := module(map[string]string{"kept": strings.ReplaceAll(kept, "%s", "b")})
	b["testdata/hypercase.json"] = string(rb.Marshal())
	g := gittest.New(t)
	c := revisions(t, g, a, b)
	page := ok(t, g, "", "report", c[0], c[1])
	only := ` <span class="badge">only at ` + c[0] + `</span>`
	d := section(t, page, `<section class="corpus" id="describe-sheet">`, "</section>")
	holds(t, "the sheet corpus's descriptions", d,
		"<h3>sheet</h3>\n<p>A sheet, at b.</p>",
		"<dt>calc</dt><dd>Runs calc, at b.<ul class=\"outputs\"><li><code>stderr.txt</code>: What calc warns, at b.</li><li><code>stdout.txt</code>"+only+": What calc prints, at a.</li></ul>"+
			"<p class=\"classifier\">Its classifier, <code>slow</code>: Adds slow, at b.</p></dd>",
		"<dt>evaluate</dt><dd>Evaluates.<p class=\"classifier\">Its classifier, <code>div-zero</code>"+only+": Adds div-zero, at a.</p></dd>",
		"<dt>old"+only+"</dt><dd>A column only a has.<ul class=\"outputs\"><li><code>old.txt</code>: What old wrote.</li></ul></dd>",
		"<dt>doubled"+only+"</dt><dd>A transform only a has.<p>Keeps <span class=\"prop\">dangling</span>.</p></dd>",
		"<dt>looped</dt><dd>Loops, at b.<p>Gives <span class=\"prop\">circular</span>.</p></dd>",
		`<dt><span class="case">sheet/gone</span></dt><dd>A case only a has.</dd>`,
		`<dt><span class="case">sheet/gone/variant/w</span></dt><dd>A variant only a has.</dd>`)
	lacks(t, "the sheet corpus's descriptions", d, "A sheet, at a.", "Runs calc, at a.", "What calc warns, at a.", "Adds slow, at a.", "Loops, at a.", "sheet/kept")
	holds(t, "the csv corpus's description", section(t, page, `<section class="corpus" id="describe-csv">`, "</section>"), "<h3>csv</h3>\n<p>A CSV, only at b.</p>")
	holds(t, "the tsv corpus's description", section(t, page, `<section class="corpus" id="describe-tsv">`, "</section>"), "<h3>tsv"+only+"</h3>\n<p>A TSV, only at a.</p>")
	holds(t, "the properties' descriptions", section(t, page, `<section class="properties" id="describe-properties">`, "</section>"),
		"<dt>dangling</dt><dd>A reference to nothing, at b.</dd>", "<dt>retired"+only+"</dt><dd>A property only a lists.</dd>")
	head, _ := matrixRow(t, page, "sheet", "sheet/kept/variant/v")
	holds(t, "kept/variant/v's row", head, `<div class="desc">A variant both have, at b.</div>`)
}

// TestPageDescribeConflict: a column two registries claim at b has no
// declarations there, and the page takes none of a's for it, whose outputs
// and classifier b doesn't lack but declares twice: it shows neither
// description, and none of a's declarations marked only at a.
func TestPageDescribeConflict(t *testing.T) {
	t.Parallel()
	ra := moduleRegistry()
	ra.Suites[0].Outputs = []registry.Output{{Pattern: "stdout.txt", Description: "What calc prints, at a."}}
	ra.Suites[0].Classifier = &registry.Classifier{Name: "slow", Description: "Adds slow, at a."}
	other := registry.Registry{UpdateVar: "M_UPDATE_GOLDEN", CorpusRoot: ".",
		Suites: []registry.Suite{{Name: "calc", Description: "Runs calc, as another package says.", Consumes: registry.RawInput, Corpora: []string{"sheet"}}}}
	const x = "A case.\n-- in/sheet/A1 --\n1\n-- out/calc/outcome --\naccepted\n"
	a := module(map[string]string{"x": x})
	a["testdata/hypercase.json"] = string(ra.Marshal())
	b := module(map[string]string{"x": x})
	b["pkg/testdata/hypercase.json"] = string(other.Marshal())
	g := gittest.New(t)
	c := revisions(t, g, a, b)
	page := ok(t, g, "", "report", c[0], c[1])
	d := section(t, page, `<section class="corpus" id="describe-sheet">`, "</section>")
	holds(t, "the sheet corpus's descriptions", d, "<dt>calc</dt><dd></dd>")
	lacks(t, "the sheet corpus's descriptions", d, "at a.", "only at")
	holds(t, "the problems", section(t, page, `<section id="problems">`, "</section>"),
		"each claim the column calc, and a column&#39;s name is unique per corpus root")
}

// TestPageProducts: the page shows a change to a bytes lift's product in
// the lift's cell, with its line diff, and describes the lift's product
// declarations with its outputs, each as via/<lift>/<pattern>. A
// via/<lift>/ section whose lift no registry declares as a bytes lift over
// its corpus is a problem, which the report leaves out: a suite's, or a
// bytes lift's over another corpus.
func TestPageProducts(t *testing.T) {
	t.Parallel()
	const x = "A CSV.\n-- in/csv/x.csv --\n4\n-- via/import/sheet/A1 --\nV\n-- via/calc/sheet/A1 --\nV\n" +
		"-- out/calc/outcome --\naccepted\n-- out/import/outcome --\naccepted\n"
	ra := productRegistry()
	ra.Lifts[0].ProductFiles = append(ra.Lifts[0].ProductFiles, registry.Output{Pattern: "notes/*", Description: "A note, only at a."})
	a := productModule(map[string]string{"x": strings.ReplaceAll(x, "V", "4")})
	a["testdata/hypercase.json"] = string(ra.Marshal())
	b := productModule(map[string]string{"x": strings.ReplaceAll(x, "V", "5")})
	b["testdata/sheet/y.txtar"] = "A sheet.\n-- in/sheet/A1 --\n1\n-- via/import/sheet/A1 --\n1\n-- out/calc/outcome --\naccepted\n"
	g := gittest.New(t)
	c := revisions(t, g, a, b)
	page := ok(t, g, "", "report", c[0], c[1])

	holds(t, "x import's change", change(t, page, "csv/x", "import"),
		`<li class="changed"><details><summary><code>via/import/sheet/A1</code> changed <span class="stat"><span class="add">+1</span> <span class="del">-1</span></span></summary>`+"\n"+
			`<pre class="diff"><span class="del">- 4</span><span class="add">&#43; 5</span></pre></details></li>`)
	lacks(t, "the changes", section(t, page, `<section id="changes">`, "</section>"), `<span class="case">csv/x</span> <span class="col">calc</span>`)

	only := ` <span class="badge">only at ` + c[0] + `</span>`
	holds(t, "the csv corpus's descriptions", section(t, page, `<section class="corpus" id="describe-csv">`, "</section>"),
		`<dt>import</dt><dd>Imports the CSV as a sheet.<ul class="outputs"><li><code>error.txt</code>: Why the importer rejects the CSV.</li>`+
			`<li><code>via/import/notes/*</code>`+only+`: A note, only at a.</li>`+
			`<li><code>via/import/sheet/*</code>: One file per cell, as a sheet&#39;s in/sheet/ holds them.</li></ul></dd>`)

	holds(t, "the problems", section(t, page, `<section id="problems">`, "</section>"),
		"<code>testdata/csv/x.txtar, via/calc/sheet/A1</code>: the section is a product of calc, and no registry declares a bytes lift of that name over testdata/csv/, so the report leaves it out, and doesn&#39;t compare its cell&#39;s product.</li>",
		"<code>testdata/sheet/y.txtar, via/import/sheet/A1</code>: the section is a product of import, and no registry declares a bytes lift of that name over testdata/sheet/, so the report leaves it out, and doesn&#39;t compare its cell&#39;s product.</li>")
}
