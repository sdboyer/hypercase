package report

import (
	"strings"
	"testing"
)

// TestWriteText: the plain text gives the problems first, under one
// heading, by revision in the report's order, each with its place where
// it has one, then the ranked changes. With no problems, it gives the
// changes alone.
func TestWriteText(t *testing.T) {
	changes := []Change{{Rank: Appeared, Case: "sheet/x", Column: "calc", After: "accepted"}}
	for _, tt := range []struct {
		name     string
		problems []Problem
		want     string
	}{
		{"none", nil, "appeared\n  sheet/x calc: accepted\n"},
		{"both revisions", []Problem{
			{At: "b", Where: "testdata/sheet/x.txtar, gaps, line 1", Msg: "the line names the column y."},
			{At: "b", Msg: "two registries claim calc."},
			{At: "a", Where: "testdata/sheet/", Msg: "the directory holds 1 cell of z."},
		}, "problems\n  at b\n    testdata/sheet/x.txtar, gaps, line 1: the line names the column y.\n    two registries claim calc.\n  at a\n    testdata/sheet/: the directory holds 1 cell of z.\nappeared\n  sheet/x calc: accepted\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			r := &Report{Problems: tt.problems, Changes: changes}
			if err := r.WriteText(&b); err != nil {
				t.Fatal(err)
			}
			if got := b.String(); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
