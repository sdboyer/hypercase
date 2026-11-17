package main

import (
	"testing"

	"github.com/sdboyer/hypercase/internal/gittest"
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
