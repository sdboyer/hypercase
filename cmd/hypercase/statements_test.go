package main

import (
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/gittest"
	"github.com/sdboyer/hypercase/internal/registry"
	"github.com/sdboyer/hypercase/internal/revmap"
)

// statementSentence is what the page says of an output declaration's
// statement, as a reader is meant to read it: where its ranges come from,
// and, for one that takes them from Positions, whether the project
// supplies it. A statement a registry doesn't record says nothing.
func statementSentence(inputs string, positions bool) string {
	switch {
	case inputs == registry.InputsFunc:
		return "Its ranges come from a function."
	case inputs == registry.InputsPositions && positions:
		return "Its ranges come from Positions, which the project supplies."
	case inputs == registry.InputsPositions:
		return "Its ranges come from Positions, which the project doesn't supply."
	case inputs == registry.InputsAll:
		return "It depends on all inputs."
	case inputs == registry.InputsNone:
		return "It states no source of ranges."
	}
	return ""
}

// TestPageStatements: the descriptions show what each output declaration
// states its ranges come from, and for one that takes them from Positions,
// whether its column's registry records that the project supplies it. A
// bytes lift's product declarations show theirs too. A registry from
// before the format that records statements shows none.
//
// The declarations are generated: each statement a registry records for
// an output, each one it records for a product file, and each in a
// registry that records Positions supplied and one that doesn't. An
// output only A declares shows its statement at A, with whether A's
// registry records Positions supplied.
func TestPageStatements(t *testing.T) {
	t.Parallel()
	csv := registry.Type{Dir: "csv", Description: "A CSV export.", GoType: "example.com/m.CSV"}
	sheet := registry.Type{Dir: "sheet", Description: "A sheet.", GoType: "example.com/m.Sheet"}
	type want struct{ corpus, column, pattern, sentence string }
	var wants []want
	files := map[string]string{"go.mod": "module example.com/m\n\ngo 1.27\n"}
	for _, supplied := range []bool{true, false} {
		suffix := map[bool]string{true: "supplied", false: "unsupplied"}[supplied]
		suite := registry.Suite{Name: "calc-" + suffix, Description: "Runs calc.", Consumes: sheet.GoType, Corpora: []string{"sheet"}, Paths: []registry.Path{{Corpus: "sheet"}}}
		lift := registry.Lift{
			Name: "import-" + suffix, Description: "Imports the CSV as a sheet.", Input: csv.GoType, Product: sheet.GoType,
			Owned: true, Bytes: true, Corpora: []string{"csv"}, Paths: []registry.Path{{Corpus: "csv"}},
		}
		for _, s := range []string{registry.InputsFunc, registry.InputsPositions, registry.InputsAll, registry.InputsNone} {
			pattern := s + ".txt"
			if s == registry.InputsPositions {
				pattern = "error.txt"
			}
			suite.Outputs = append(suite.Outputs, registry.Output{Pattern: pattern, Description: "An output.", Inputs: s})
			wants = append(wants, want{"sheet", suite.Name, pattern, statementSentence(s, supplied)})
			if s == registry.InputsPositions {
				continue // a product file never takes its ranges from Positions
			}
			lift.ProductFiles = append(lift.ProductFiles, registry.Output{Pattern: "sheet/" + s + "-*", Description: "A product file.", Inputs: s})
			wants = append(wants, want{"csv", lift.Name, "via/" + lift.Name + "/sheet/" + s + "-*", statementSentence(s, supplied)})
		}
		reg := registry.Registry{
			UpdateVar: "M_UPDATE_GOLDEN", CorpusRoot: ".", RootPackage: supplied, Positions: supplied,
			Types: []registry.Type{csv, sheet}, Lifts: []registry.Lift{lift}, Suites: []registry.Suite{suite},
		}
		dir := map[bool]string{true: "", false: "unsupplied/"}[supplied]
		files[dir+"testdata/hypercase.json"] = string(reg.Marshal())
	}
	// A registry from before statements, whose output records none.
	old := registry.Registry{
		UpdateVar: "M_UPDATE_GOLDEN", CorpusRoot: ".", Types: []registry.Type{sheet},
		Suites: []registry.Suite{{Name: "old", Description: "Runs an old suite.", Consumes: sheet.GoType, Corpora: []string{"sheet"}, Paths: []registry.Path{{Corpus: "sheet"}},
			Outputs: []registry.Output{{Pattern: "old.txt", Description: "An output.", Inputs: registry.InputsFunc}}}},
	}
	files["old/testdata/hypercase.json"] = registryAt(old, registry.InputsFormat-1)
	wants = append(wants, want{"sheet", "old", "old.txt", ""})
	// A column whose error.txt only A declares, where only A's registry
	// records Positions supplied.
	moved := func(positions bool, outputs ...registry.Output) string {
		return string(registry.Registry{
			UpdateVar: "M_UPDATE_GOLDEN", CorpusRoot: ".", Positions: positions, Types: []registry.Type{sheet},
			Suites: []registry.Suite{{Name: "moved", Description: "Runs a moved suite.", Consumes: sheet.GoType, Corpora: []string{"sheet"}, Paths: []registry.Path{{Corpus: "sheet"}}, Outputs: outputs}},
		}.Marshal())
	}
	stay := registry.Output{Pattern: "stay.txt", Description: "An output.", Inputs: registry.InputsFunc}
	atA := module(nil)
	atA["moved/testdata/hypercase.json"] = moved(true, registry.Output{Pattern: "error.txt", Description: "An output.", Inputs: registry.InputsPositions}, stay)
	files["moved/testdata/hypercase.json"] = moved(false, stay)
	wants = append(wants, want{"sheet", "moved", "error.txt", statementSentence(registry.InputsPositions, true)}, want{"sheet", "moved", "stay.txt", statementSentence(registry.InputsFunc, false)})

	g := gittest.New(t)
	c := revisions(t, g, atA, files)
	page := ok(t, g, "", "report", c[0], c[1])
	for _, w := range wants {
		desc := section(t, page, `<section class="corpus" id="describe-`+w.corpus+`">`, "</section>")
		item := section(t, section(t, desc, "<dt>"+w.column+"</dt>", "</dd>"), "<li><code>"+w.pattern+"</code>", "</li>")
		if w.sentence == "" {
			lacks(t, w.pattern+"'s description", item, `class="statement"`)
			continue
		}
		holds(t, w.pattern+"'s description", item, `<span class="statement">`+strings.ReplaceAll(w.sentence, "'", "&#39;")+`</span>`)
	}
}

// TestInputLinesKept: a cell's map at a revision carries a note where its
// column's declarations there say no update writes the cell's inputs file,
// so its ranges may not be what the cell read: the cell has a file of a
// declaration that states no source of ranges, or one that takes its
// ranges from Positions where the project doesn't supply it. The note is
// shown with the map's statement, all inputs or no ranges among them, and
// where the map's ranges show no lines. A bytes lift's product file is a
// file of its cell. A cell with neither, an error.txt whose project
// supplies Positions, and a file two declarations match, carry none.
func TestInputLinesKept(t *testing.T) {
	t.Parallel()
	out := func(pattern, inputs string) registry.Output {
		return registry.Output{Pattern: pattern, Description: "An output.", Inputs: inputs}
	}
	supplied := moduleRegistry()
	supplied.Positions = true
	supplied.Suites = supplied.Suites[:1] // calc
	supplied.Suites[0].Outputs = []registry.Output{out("kept.txt", registry.InputsNone), out("error.txt", registry.InputsPositions), out("stdout.txt", registry.InputsFunc),
		out("dup*", registry.InputsNone), out("dup.txt", registry.InputsNone)}
	unsupplied := moduleRegistry()
	unsupplied.RootPackage = false
	unsupplied.Suites = unsupplied.Suites[1:] // evaluate
	unsupplied.Suites[0].Outputs = []registry.Output{out("error.txt", registry.InputsPositions), out("stdout.txt", registry.InputsFunc)}
	// A bytes lift whose product files state no source of ranges.
	lift := productRegistry()
	lift.RootPackage, lift.Codes, lift.Suites = false, false, nil
	lift.Lifts[0].ProductFiles[0].Inputs = registry.InputsNone

	const a1 = "-- in/sheet/A1 --\n1\n"
	std := func(rs ...revmap.Range) revmap.File { return revmap.File{Path: "stdout.txt", Ranges: rs} }
	whole := lines("in/sheet/A1", 0, 0)
	archives := func(v string) map[string]string {
		return map[string]string{
			"kept": "A case.\n" + a1 + cellOf("", "calc", "accepted", [][2]string{{"kept.txt", "k\n"}, {"stdout.txt", v}}, revmap.File{Path: "kept.txt"}, std(whole)),
			"unsupplied": "A case.\n" + a1 + cellOf("", "evaluate", "rejected", [][2]string{{"error.txt", v}},
				revmap.File{Path: "error.txt", Ranges: []revmap.Range{lines("in/sheet/A1", 1, 1)}}),
			"all-kept": "A case.\n" + a1 + cellOf("", "evaluate", "rejected", [][2]string{{"error.txt", v}}, revmap.File{Path: "error.txt", All: true}),
			"supplied": "A case.\n" + a1 + cellOf("", "calc", "rejected", [][2]string{{"error.txt", v}},
				revmap.File{Path: "error.txt", Ranges: []revmap.Range{lines("in/sheet/A1", 1, 1)}}),
			"plain": "A case.\n" + a1 + cellOf("", "evaluate", "accepted", [][2]string{{"stdout.txt", v}}, std(whole)),
			"dup":   "A case.\n" + a1 + cellOf("", "calc", "accepted", [][2]string{{"dup.txt", v}}, revmap.File{Path: "dup.txt", Ranges: []revmap.Range{whole}}),
			// kept.txt's one range is into a section the archive lacks.
			"kept-bad": "A case.\n" + a1 + cellOf("", "calc", "accepted", [][2]string{{"kept.txt", v}}, revmap.File{Path: "kept.txt", Ranges: []revmap.Range{lines("in/sheet/Z9", 0, 0)}}),
		}
	}
	var revs [2]map[string]string
	for i, v := range []string{"1\n", "2\n"} {
		revs[i] = map[string]string{
			"go.mod":                    "module example.com/m\n\ngo 1.27\n",
			"testdata/hypercase.json":   string(supplied.Marshal()),
			"p/testdata/hypercase.json": string(unsupplied.Marshal()),
			"q/testdata/hypercase.json": string(lift.Marshal()),
			"testdata/csv/product.txtar": "A CSV.\n-- in/csv/data --\na\n" + cellOf("", "import", "accepted", nil, revmap.File{Path: "via/import/sheet/A1"}) +
				"-- via/import/sheet/A1 --\n" + v,
		}
		for name, data := range archives(v) {
			revs[i]["testdata/sheet/"+name+".txtar"] = data
		}
	}
	g := gittest.New(t)
	c := revisions(t, g, revs[0], revs[1])
	page := ok(t, g, "", "report", c[0], c[1])
	at := `<p class="note">At <span class="rev">` + c[1] + `</span>: `
	const tail = `, so no update writes the cell&#39;s inputs file, and its ranges may not be what the cell read.</p>`

	holds(t, "kept calc's change", change(t, page, "sheet/kept", "calc"),
		`<details class="inputs"><summary>input lines</summary>`+"\n"+at+`kept.txt&#39;s declaration states no source of ranges`+tail+"\n"+`<div class="excerpt"><code>in/sheet/A1</code>`)
	holds(t, "unsupplied evaluate's change", change(t, page, "sheet/unsupplied", "evaluate"),
		at+`error.txt takes its ranges from Positions, which the project doesn&#39;t supply`+tail)
	holds(t, "all-kept evaluate's change", change(t, page, "sheet/all-kept", "evaluate"),
		`<p class="map">At <span class="rev">`+c[1]+`</span>: all inputs.</p>`+"\n"+at+`error.txt takes its ranges from Positions`)
	holds(t, "product import's change", change(t, page, "csv/product", "import"),
		`<p class="map">At <span class="rev">`+c[1]+`</span>: no ranges.</p>`+"\n"+at+`via/import/sheet/A1&#39;s declaration states no source of ranges`+tail)
	holds(t, "kept-bad calc's change", change(t, page, "sheet/kept-bad", "calc"),
		`<details class="inputs"><summary>input lines</summary>`+"\n"+at+`kept.txt&#39;s declaration states no source of ranges`+tail+"\n</details>")
	for _, cell := range [][2]string{{"sheet/supplied", "calc"}, {"sheet/plain", "evaluate"}, {"sheet/dup", "calc"}} {
		entry := change(t, page, cell[0], cell[1])
		holds(t, cell[0]+" "+cell[1]+"'s change", entry, `<div class="excerpt"><code>in/sheet/A1</code>`)
		lacks(t, cell[0]+" "+cell[1]+"'s change", entry, `<p class="note">`)
	}
}
