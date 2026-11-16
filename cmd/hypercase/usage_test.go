package main

import (
	"testing"

	"github.com/sdboyer/hypercase/internal/gittest"
)

// TestUsage: a command line the command doesn't take exits 2, and a
// revision that names no commit exits 1, each saying why.
func TestUsage(t *testing.T) {
	t.Parallel()
	g := gittest.New(t)
	c := revisions(t, g, module(nil))
	const usage = "usage: hypercase report [-root dir] [-text] <a> <b>"
	fails(t, g, "", 2, nil, usage)
	fails(t, g, "", 2, []string{"strip"}, `hypercase: no command "strip"`, usage)
	fails(t, g, "", 2, []string{"report", c[0]}, "hypercase report takes two revisions, and was given 1.", usage)
	fails(t, g, "", 2, []string{"report", "-nope", c[0], c[0]}, "flag provided but not defined: -nope", usage)
	fails(t, g, "", 1, []string{"report", c[0], "nope"}, "hypercase report: nope names no commit in this repository\n")
	fails(t, g, "", 1, []string{"report", "--", "-x", c[0]}, "hypercase report: -x names no commit in this repository\n")
	if r := hypercase(t, g, "", "report", "-h"); r.code != 0 || r.stdout != "" {
		t.Errorf("report -h: exit %d, stdout %q", r.code, r.stdout)
	}
	outside := &gittest.Repo{Dir: t.TempDir()}
	fails(t, outside, "", 1, []string{"report", "a", "b"}, "hypercase report: git rev-parse: fatal: not a git repository")
}

// TestArchiveForm: the report fails on a section it reads whose form it
// can't read, naming the archive, the revision and the section.
func TestArchiveForm(t *testing.T) {
	t.Parallel()
	const head = "A case.\n-- in/sheet/A1 --\n1\n"
	for _, tt := range []struct{ name, archive, want string }{
		{"empty", head + "-- out/calc/outcome --\n-- out/calc/stdout.txt --\n1\n", "out/calc/outcome: an outcome is one line, ending with a newline"},
		{"two lines", head + "-- out/calc/outcome --\naccepted\naccepted\n", "out/calc/outcome: an outcome is one line, ending with a newline"},
		{"no state", head + "-- out/calc/outcome --\nfine\n", `out/calc/outcome: an outcome starts with its state, one of accepted, rejected, panicked, blocked, skipped, not "fine"`},
		{"panicked with detail", head + "-- out/calc/outcome --\npanicked boom\n", "out/calc/outcome: panicked stands alone"},
		{"blocked by nothing", head + "-- out/calc/outcome --\nblocked\n", "out/calc/outcome: blocked is followed by the lift that failed"},
		{"skipped for no reason", head + "-- out/calc/outcome --\nskipped\n", "out/calc/outcome: skipped is followed by the probe's reason"},
		{"codes out of order", head + "-- out/calc/outcome --\nrejected S2 S1\n", "out/calc/outcome: a rejected outcome's codes are sorted byte by byte, each once, and S1 follows S2"},
		{"a detail with a space at its end", head + "-- out/calc/outcome --\naccepted div-zero \n", `out/calc/outcome: "div-zero " can't be held in an outcome: it is one line, not empty, with no space at either end`},
		{"no column", head + "-- out/outcome --\naccepted\n", "out/outcome: an output belongs under its column, as out/<column>/<file>"},
		{"a variant with no name", head + "-- variant//out/calc/outcome --\naccepted\n", "variant//out/calc/outcome: a variant's section belongs under its name"},
		{"a section twice", head + "-- out/calc/outcome --\naccepted\n-- out/calc/outcome --\naccepted\n", "out/calc/outcome: the section appears twice"},
		{"a blank properties line", head + "-- properties --\ncircular\n\n", "properties, line 2: the line is blank"},
		{"a gaps line with no text", head + "-- gaps --\ncalc Waits on sheets#1.\nevaluate\n", "gaps, line 2: the line names evaluate and has no text"},
		{"two gaps lines for one cell", head + "-- gaps --\ncalc Waits on sheets#1.\ncalc Waits on sheets#2.\n", "gaps, line 2: lines 1 and 2 both mark the cell of calc"},
		{"a transform's gaps line with no column", head + "-- gaps --\ntransform/t Waits.\n", `gaps, line 1: the line names transform/t, then "Waits.", which is not a column's name`},
		{"a transform's line in a variant's gaps", head + "-- variant/v/gaps --\ntransform/t calc Waits.\n", "variant/v/gaps, line 1: the line starts with transform/t, and a hand-written variant's gaps mark its own cells"},
		{"a blank line in a variant's properties", head + "-- variant/v/properties --\ncircular\n\n", "variant/v/properties, line 2: the line is blank"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := gittest.New(t)
			c := revisions(t, g, module(nil), module(map[string]string{"x": tt.archive}))
			fails(t, g, "", 1, []string{"report", c[0], c[1]}, "hypercase report: testdata/sheet/x.txtar at "+c[1]+": "+tt.want)
		})
	}
}
