package main

import (
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/gittest"
	"github.com/sdboyer/hypercase/internal/registry"
)

// TestRanks: changed outcomes come first, then cells that appeared, then
// those that disappeared, then cells whose other files changed under the
// same outcome. A cell exists where its outcome file does, a cell is
// listed once, at its first rank, and only the cells' files are changes.
func TestRanks(t *testing.T) {
	t.Parallel()
	const head = "A case.\n-- in/sheet/A1 --\n1\n"
	for _, tt := range []struct {
		name string
		a, b map[string]string
		want string
	}{
		{
			name: "unchanged",
			a:    map[string]string{"x": head + "-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n1\n"},
			b:    map[string]string{"x": head + "-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n1\n"},
			want: "",
		},
		{
			name: "the outcome alone",
			a:    map[string]string{"x": head + "-- out/calc/outcome --\naccepted\n"},
			b:    map[string]string{"x": head + "-- out/calc/outcome --\nrejected S101\n"},
			want: "outcome changed\n  sheet/x calc: accepted -> rejected S101\n",
		},
		{
			name: "the outcome's detail",
			a:    map[string]string{"x": head + "-- out/calc/outcome --\nrejected S101\n"},
			b:    map[string]string{"x": head + "-- out/calc/outcome --\nrejected S101 S102\n"},
			want: "outcome changed\n  sheet/x calc: rejected S101 -> rejected S101 S102\n",
		},
		{
			name: "the outcome and files, ranked once",
			a:    map[string]string{"x": head + "-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n1\n"},
			b:    map[string]string{"x": head + "-- out/calc/error.txt --\nno\n-- out/calc/outcome --\nrejected\n"},
			want: "outcome changed\n  sheet/x calc: accepted -> rejected\n    error.txt added\n    stdout.txt removed\n",
		},
		{
			name: "files alone",
			a: map[string]string{"x": head + "-- out/calc/outcome --\naccepted\n-- out/calc/stderr.txt --\nwarn\n" +
				"-- out/calc/stdout.txt --\n1\n-- out/calc/tables/a.csv --\n1\n"},
			b: map[string]string{"x": head + "-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n1\n" +
				"-- out/calc/tables/a.csv --\n2\n-- out/calc/values.txt --\nA1 1\n"},
			want: "bytes changed\n  sheet/x calc: accepted\n    stderr.txt removed\n    tables/a.csv changed\n    values.txt added\n",
		},
		{
			// A cell exists where its outcome file does, so one that gains
			// its outcome appears, with every file it has.
			name: "an outcome file added",
			a:    map[string]string{"x": head + "-- out/calc/stdout.txt --\n1\n"},
			b:    map[string]string{"x": head + "-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n1\n"},
			want: "appeared\n  sheet/x calc: accepted\n    stdout.txt added\n",
		},
		{
			name: "an outcome file removed",
			a:    map[string]string{"x": head + "-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n1\n"},
			b:    map[string]string{"x": head + "-- out/calc/stdout.txt --\n2\n"},
			want: "disappeared\n  sheet/x calc: accepted\n    stdout.txt removed\n",
		},
		{
			name: "files with no outcome file are no cell's",
			a:    map[string]string{"x": head + "-- out/calc/stdout.txt --\n1\n"},
			b:    map[string]string{"x": head + "-- out/calc/stdout.txt --\n2\n"},
			want: "",
		},
		{
			name: "a transform's inputs are no cell's files",
			a:    map[string]string{"x": head + "-- transform/doubled/in/sheet/A1 --\n2\n-- transform/doubled/out/calc/outcome --\naccepted\n"},
			b:    map[string]string{"x": head + "-- transform/doubled/delete --\nsheet/B1\n-- transform/doubled/in/sheet/A1 --\n3\n-- transform/doubled/out/calc/outcome --\naccepted\n"},
			want: "",
		},
		{
			name: "a column's cell appears in a case",
			a:    map[string]string{"x": head + "-- out/calc/outcome --\naccepted\n"},
			b:    map[string]string{"x": head + "-- out/calc/outcome --\naccepted\n-- out/evaluate/outcome --\naccepted\n-- out/evaluate/values.txt --\nA1 1\n"},
			want: "appeared\n  sheet/x evaluate: accepted\n    values.txt added\n",
		},
		{
			name: "a variant's cell disappears",
			a: map[string]string{"x": head + "-- out/calc/outcome --\naccepted\n" +
				"-- variant/two/about --\nA1 is 2.\n-- variant/two/in/sheet/A1 --\n2\n" +
				"-- variant/two/out/calc/outcome --\naccepted\n-- variant/two/out/calc/stdout.txt --\n2\n"},
			b:    map[string]string{"x": head + "-- out/calc/outcome --\naccepted\n"},
			want: "disappeared\n  sheet/x/variant/two calc: accepted\n    stdout.txt removed\n",
		},
		{
			name: "a transform's cell changes",
			a:    map[string]string{"x": head + "-- transform/doubled/in/sheet/A1 --\n2\n-- transform/doubled/out/calc/outcome --\naccepted\n"},
			b:    map[string]string{"x": head + "-- transform/doubled/in/sheet/A1 --\n2\n-- transform/doubled/out/calc/outcome --\nskipped no formulas\n"},
			want: "outcome changed\n  sheet/x/transform/doubled calc: accepted -> skipped no formulas\n",
		},
		{
			name: "by rank, then case, then column",
			a: map[string]string{
				"a": head + "-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n1\n",
				"m": head + "-- out/calc/outcome --\naccepted\n-- out/evaluate/outcome --\naccepted\n",
			},
			b: map[string]string{
				"a": head + "-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n2\n",
				"m": head + "-- out/evaluate/outcome --\nrejected\n",
				"z": head + "-- out/evaluate/outcome --\naccepted\n-- out/calc/outcome --\naccepted\n",
			},
			want: "outcome changed\n  sheet/m evaluate: accepted -> rejected\n" +
				"appeared\n  sheet/z calc: accepted\n  sheet/z evaluate: accepted\n" +
				"disappeared\n  sheet/m calc: accepted\n" +
				"bytes changed\n  sheet/a calc: accepted\n    stdout.txt changed\n",
		},
		{
			name: "an archive's cases sit together",
			a:    map[string]string{},
			b: map[string]string{
				"x":     head + "-- out/calc/outcome --\naccepted\n-- transform/t/in/sheet/A1 --\n2\n-- transform/t/out/calc/outcome --\naccepted\n-- variant/v/in/sheet/A1 --\n3\n-- variant/v/out/calc/outcome --\naccepted\n",
				"x-and": head + "-- out/calc/outcome --\naccepted\n",
			},
			want: "appeared\n  sheet/x calc: accepted\n  sheet/x/variant/v calc: accepted\n  sheet/x/transform/t calc: accepted\n  sheet/x-and calc: accepted\n",
		},
		{
			name: "a case's description, inputs, properties and gaps are no cell's files",
			a:    map[string]string{"x": head + "-- out/calc/outcome --\naccepted\n"},
			b: map[string]string{"x": "Another description.\n-- in/sheet/A1 --\n2\n-- properties --\ncircular\n" +
				"-- gaps --\ncalc Waits on sheets#1.\n-- out/calc/outcome --\naccepted\n"},
			want: "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := gittest.New(t)
			c := revisions(t, g, module(tt.a), module(tt.b))
			if got := ok(t, g, "", "report", "-text", c[0], c[1]); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// productModule returns a module whose corpus root is its root, with the
// registry productRegistry, and the archives of the corpus csv given by
// name.
func productModule(archives map[string]string) map[string]string {
	files := map[string]string{
		"go.mod":                  "module example.com/m\n\ngo 1.27\n",
		"testdata/hypercase.json": string(productRegistry().Marshal()),
	}
	for name, data := range archives {
		files["testdata/csv/"+name+".txtar"] = data
	}
	return files
}

// productRegistry declares the bytes lift import, which makes a sheet of
// a CSV, and the suite calc, which runs on sheets, the CSVs' among them.
func productRegistry() registry.Registry {
	csv := registry.Type{Dir: "csv", Description: "A CSV export.", GoType: "example.com/m.CSV"}
	sheet := registry.Type{Dir: "sheet", Description: "A sheet.", GoType: "example.com/m.Sheet"}
	return registry.Registry{
		UpdateVar: "M_UPDATE_GOLDEN", CorpusRoot: ".", RootPackage: true, Codes: true,
		Types: []registry.Type{csv, sheet},
		Lifts: []registry.Lift{{
			Name: "import", Description: "Imports the CSV as a sheet.", Input: csv.GoType, Product: sheet.GoType,
			Owned: true, Bytes: true, Corpora: []string{"csv"},
			Outputs:      []registry.Output{{Pattern: "error.txt", Description: "Why the importer rejects the CSV."}},
			ProductFiles: []registry.Output{{Pattern: "sheet/*", Description: "One file per cell, as a sheet's in/sheet/ holds them."}},
		}},
		Suites: []registry.Suite{{Name: "calc", Description: "Runs calc.", Consumes: sheet.GoType, Corpora: []string{"csv", "sheet"}}},
	}
}

// TestProducts: a bytes lift's product, under via/<lift>/, is files of the
// lift's cell, each shown as via/<lift>/<path>, after the cell's own
// files. A change to the product alone ranks the lift's cell bytes
// changed, and each cell past the lift ranks on its own files.
func TestProducts(t *testing.T) {
	t.Parallel()
	const head = "A CSV.\n-- in/csv/x.csv --\n4\n"
	accepted := func(product string) string {
		return head + "-- via/import/sheet/A1 --\n" + product + "\n-- out/import/outcome --\naccepted\n"
	}
	for _, tt := range []struct {
		name string
		a, b string
		want string
		// problem is what the page lists as b's problem, if anything.
		problem string
	}{
		{
			name: "the product alone",
			a:    accepted("4") + "-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n4\n",
			b:    accepted("=2*2") + "-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n4\n",
			want: "bytes changed\n  csv/x import: accepted\n    via/import/sheet/A1 changed\n",
		},
		{
			name: "the product and a cell past the lift, each its own cell's",
			a:    accepted("4") + "-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n4\n",
			b:    accepted("5") + "-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n5\n",
			want: "bytes changed\n  csv/x calc: accepted\n    stdout.txt changed\n  csv/x import: accepted\n    via/import/sheet/A1 changed\n",
		},
		{
			name: "a product's files added and removed",
			a:    accepted("4"),
			b:    head + "-- via/import/sheet/B1 --\n4\n-- out/import/outcome --\naccepted\n",
			want: "bytes changed\n  csv/x import: accepted\n    via/import/sheet/A1 removed\n    via/import/sheet/B1 added\n",
		},
		{
			name: "the outcome and the product, ranked once",
			a:    accepted("4") + "-- out/calc/outcome --\naccepted\n-- out/calc/stdout.txt --\n4\n",
			b: head + "-- out/calc/outcome --\nblocked import\n" +
				"-- out/import/error.txt --\nline 1: no\n-- out/import/outcome --\nrejected C101\n",
			want: "outcome changed\n  csv/x calc: accepted -> blocked import\n    stdout.txt removed\n" +
				"  csv/x import: accepted -> rejected C101\n    error.txt added\n    via/import/sheet/A1 removed\n",
		},
		{
			name: "the cell's own files, then its product's",
			a:    accepted("4"),
			b:    accepted("5") + "-- out/import/zz.txt --\nz\n",
			want: "bytes changed\n  csv/x import: accepted\n    zz.txt added\n    via/import/sheet/A1 changed\n",
		},
		{
			name: "a variant's product and a transform's",
			a: accepted("4") + "-- variant/v/in/csv/x.csv --\n5\n-- variant/v/via/import/sheet/A1 --\n5\n-- variant/v/out/import/outcome --\naccepted\n" +
				"-- transform/t/in/csv/x.csv --\n6\n-- transform/t/via/import/sheet/A1 --\n6\n-- transform/t/out/import/outcome --\naccepted\n",
			b: accepted("4") + "-- variant/v/in/csv/x.csv --\n5\n-- variant/v/via/import/sheet/A1 --\n=2+3\n-- variant/v/out/import/outcome --\naccepted\n" +
				"-- transform/t/in/csv/x.csv --\n6\n-- transform/t/via/import/sheet/A1 --\n=3+3\n-- transform/t/out/import/outcome --\naccepted\n",
			want: "bytes changed\n  csv/x/variant/v import: accepted\n    via/import/sheet/A1 changed\n  csv/x/transform/t import: accepted\n    via/import/sheet/A1 changed\n",
		},
		{
			name: "a lift's cell appears with its product",
			a:    head,
			b:    accepted("4"),
			want: "appeared\n  csv/x import: accepted\n    via/import/sheet/A1 added\n",
		},
		{
			// A cell exists where its outcome file does, its product's
			// files included.
			name:    "a product with no outcome file is no cell's",
			a:       head + "-- via/import/sheet/A1 --\n4\n",
			b:       head + "-- via/import/sheet/A1 --\n5\n",
			want:    "",
			problem: "<code>testdata/csv/x.txtar, via/import/</code>: the cell has its product and no outcome file.",
		},
		{
			name:    "nor are files and a product",
			a:       head + "-- out/import/error.txt --\nno\n-- via/import/sheet/A1 --\n4\n",
			b:       head + "-- out/import/error.txt --\nno\n-- via/import/sheet/A1 --\n5\n",
			want:    "",
			problem: "<code>testdata/csv/x.txtar, out/import/ and via/import/</code>: the cell has files and its product, and no outcome file.",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := gittest.New(t)
			c := revisions(t, g, productModule(map[string]string{"x": tt.a}), productModule(map[string]string{"x": tt.b}))
			if got := ok(t, g, "", "report", "-text", c[0], c[1]); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
			if tt.problem != "" {
				ps := section(t, ok(t, g, "", "report", c[0], c[1]), `<section id="problems">`, "</section>")
				holds(t, "the problems", ps, `<h3>At <span class="rev">`+c[1]+`</span></h3>`, tt.problem)
			}
		})
	}
}

// TestProductsLeftOut: a revision whose registries declare no bytes lift
// of a via/<lift>/ section's name leaves the section out, and the report
// compares no product of its cell with the other revision's, either way
// round, so a product only one revision places is no change of its cell.
// The report says so, as a problem at that revision, on the page and in
// the plain text.
func TestProductsLeftOut(t *testing.T) {
	t.Parallel()
	const x = "A CSV.\n-- in/csv/x.csv --\n4\n-- via/import/sheet/A1 --\nV\n-- out/import/outcome --\naccepted\n"
	plain := productRegistry()
	plain.Lifts[0].Bytes, plain.Lifts[0].ProductFiles = false, nil
	for _, tt := range []struct {
		name string
		at   int // the revision that leaves the product out
	}{{"at a", 0}, {"at b", 1}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mods := []map[string]string{
				productModule(map[string]string{"x": strings.ReplaceAll(x, "V", "4")}),
				productModule(map[string]string{"x": strings.ReplaceAll(x, "V", "5")}),
			}
			mods[tt.at]["testdata/hypercase.json"] = string(plain.Marshal())
			g := gittest.New(t)
			c := revisions(t, g, mods[0], mods[1])
			want := "problems\n  at " + c[tt.at] + "\n    testdata/csv/x.txtar, via/import/sheet/A1: the section is a product of import, and no registry declares a bytes lift of that name over testdata/csv/, so the report leaves it out, and doesn't compare its cell's product.\n"
			if got := ok(t, g, "", "report", "-text", c[0], c[1]); got != want {
				t.Errorf("got:\n%s\nwant the problem, and no change:\n%s", got, want)
			}
			ps := section(t, ok(t, g, "", "report", c[0], c[1]), `<section id="problems">`, "</section>")
			holds(t, "the problems", ps, `<h3>At <span class="rev">`+c[tt.at]+`</span></h3>`,
				"<code>testdata/csv/x.txtar, via/import/sheet/A1</code>: the section is a product of import, and no registry declares a bytes lift of that name over testdata/csv/, so the report leaves it out, and doesn&#39;t compare its cell&#39;s product.</li>")
			lacks(t, "the problems", ps, `<h3>At <span class="rev">`+c[1-tt.at]+`</span></h3>`)
		})
	}
}
