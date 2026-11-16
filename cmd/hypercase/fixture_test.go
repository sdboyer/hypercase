package main

import (
	"testing"

	"github.com/sdboyer/hypercase/internal/gittest"
	"github.com/sdboyer/hypercase/internal/registry"
)

// The sheets fixture: a module whose corpus root is step09, with three
// packages over it, as the walkthrough's sheets toy has, and the change of
// the walkthrough's report: the archive ratio added, and a cycle worded
// cycle: A1 -> B1 -> A1. It adds what the walkthrough's sample leaves out:
// a cell whose outcome changes alone, one whose outcome and files change
// together, and an archive removed. It holds no Go source: the report
// reads registries and archives only.

const sheetsLink = "https://example.com/sheets/issues/$1"

func sheetsRegistries(link string) map[string]string {
	links := []registry.IssueLink{{Pattern: `sheets#(\d+)`, URL: link}}
	props := []registry.Property{
		{Name: "circular", Description: "Some cell's formula refers back to itself, directly or through other cells."},
		{Name: "dangling", Description: "Some formula refers to a cell the sheet doesn't have."},
		{Name: "malformed", Description: "Some cell holds neither a number nor a formula."},
	}
	sheet := registry.Type{Dir: "sheet", Description: "A sheet: one file per cell, under sheet/, named for the cell.", GoType: "example.com/sheets/sheetcorpus.Sheet"}
	const parsed, graph = "*example.com/sheets/sheet.Parsed", "*example.com/sheets/sheet.Graph"
	corpora := []string{"sheet"}
	parse := registry.Lift{Name: "parse", Description: "Parses every cell.", Input: sheet.GoType, Product: parsed, Owned: true, Corpora: corpora, Rejects: []string{"malformed"}}
	resolve := registry.Lift{Name: "resolve", Description: "Orders the cells so that each comes after the cells it refers to.", Input: parsed, Product: graph, Owned: true, Corpora: corpora, Rejects: []string{"circular", "dangling"}}
	used := parse
	used.Owned = false
	suite := func(name, desc, consumes string, rejects ...string) registry.Suite {
		return registry.Suite{Name: name, Description: desc, Consumes: consumes, Corpora: corpora, Rejects: rejects}
	}
	return map[string]string{
		"go.mod": "module example.com/sheets\n\ngo 1.27\n",
		"step09/testdata/hypercase.json": string(registry.Registry{
			UpdateVar: "SHEETS_UPDATE_GOLDEN", CorpusRoot: "step09", RootPackage: true,
			IssueLinks: links, Properties: props, Types: []registry.Type{sheet},
			Lifts:  []registry.Lift{parse, resolve},
			Suites: []registry.Suite{suite("evaluate", "Evaluates every cell of the sheet.", graph)},
		}.Marshal()),
		"step09/format/testdata/hypercase.json": string(registry.Registry{
			UpdateVar: "SHEETS_UPDATE_GOLDEN", CorpusRoot: "step09", IssueLinks: links, Properties: props, Types: []registry.Type{sheet},
			Lifts:  []registry.Lift{used},
			Suites: []registry.Suite{suite("format", "Prints each formula back in canonical form.", parsed)},
		}.Marshal()),
		"step09/cli/testdata/hypercase.json": string(registry.Registry{
			UpdateVar: "SHEETS_UPDATE_GOLDEN", CorpusRoot: "step09", IssueLinks: links, Properties: props, Types: []registry.Type{sheet},
			Suites: []registry.Suite{suite("calc", "Runs the calc command on the sheet; see sheets#12.", sheet.GoType, "circular", "dangling", "malformed")},
		}.Marshal()),
	}
}

const sheetDir = "step09/testdata/sheet/"

// sheets makes the fixture, and returns the repository and its two
// commits.
func sheets(t testing.TB) (g *gittest.Repo, a, b string) {
	g = gittest.New(t)
	files := sheetsRegistries("https://old.example.com/$1")
	for name, data := range map[string]string{
		"loop.txtar":     loopA,
		"twice.txtar":    twiceA,
		"negative.txtar": negative,
		"div.txtar":      divA,
		"blank.txtar":    blank,
	} {
		files[sheetDir+name] = data
	}
	g.Write(files)
	a = g.Commit("a")

	files = sheetsRegistries(sheetsLink)
	for name, data := range map[string]string{
		"loop.txtar":  loopB,
		"twice.txtar": twiceB,
		"div.txtar":   divB,
		"ratio.txtar": ratio,
	} {
		files[sheetDir+name] = data
	}
	g.Write(files)
	g.Remove(sheetDir + "blank.txtar")
	b = g.Commit("b")
	return g, a, b
}

const loopA = `A1 and B1 refer to each other.
-- in/sheet/A1 --
=B1
-- in/sheet/B1 --
=A1
-- properties --
circular
-- out/calc/error.txt --
A1 -> B1 -> A1: cycle
  sheet/A1:1: =B1
  sheet/B1:1: =A1
-- out/calc/outcome --
rejected S201
-- out/evaluate/outcome --
blocked resolve
-- out/format/formulas.txt --
A1 =B1
B1 =A1
-- out/format/outcome --
accepted
-- out/parse/outcome --
accepted
-- out/resolve/error.txt --
A1 -> B1 -> A1: cycle
  sheet/A1:1: =B1
  sheet/B1:1: =A1
-- out/resolve/outcome --
rejected S201
`

const loopB = `A1 and B1 refer to each other.
-- in/sheet/A1 --
=B1
-- in/sheet/B1 --
=A1
-- properties --
circular
-- out/calc/error.txt --
cycle: A1 -> B1 -> A1
  sheet/A1:1: =B1
  sheet/B1:1: =A1
-- out/calc/outcome --
rejected S201
-- out/evaluate/outcome --
blocked resolve
-- out/format/formulas.txt --
A1 =B1
B1 =A1
-- out/format/outcome --
accepted
-- out/parse/outcome --
accepted
-- out/resolve/error.txt --
cycle: A1 -> B1 -> A1
  sheet/A1:1: =B1
  sheet/B1:1: =A1
-- out/resolve/outcome --
rejected S201
`

const twiceHead = `A1 is twice B1.
-- in/sheet/A1 --
=B1*2
-- in/sheet/B1 --
3
-- gaps --
transform/looped format Prints the reference to itself as =A1. Waits on sheets#7.
-- out/calc/outcome --
accepted
-- out/calc/stdout.txt --
6,3
-- out/evaluate/outcome --
accepted
-- out/evaluate/values.txt --
A1 6
B1 3
-- out/format/formulas.txt --
A1 =B1 * 2
-- out/format/outcome --
accepted
-- out/parse/outcome --
accepted
-- out/resolve/outcome --
accepted
-- variant/cycle/about --
B1 refers back to A1, so the sheet has a cycle.
-- variant/cycle/properties --
circular
-- variant/cycle/in/sheet/B1 --
=A1
`

const twiceTail = `-- variant/cycle/out/evaluate/outcome --
blocked resolve
-- variant/cycle/out/format/formulas.txt --
A1 =B1 * 2
B1 =A1
-- variant/cycle/out/format/outcome --
accepted
-- variant/cycle/out/parse/outcome --
accepted
-- transform/looped/in/sheet/A1 --
=A1
-- transform/looped/out/format/formulas.txt --
A1 = A1
-- transform/looped/out/format/outcome --
accepted
-- transform/looped/out/resolve/outcome --
rejected S201
-- transform/looped/out/resolve/error.txt --
A1 -> A1: cycle
  sheet/A1:1: =A1
`

const twiceA = twiceHead + `-- variant/cycle/out/calc/error.txt --
A1 -> B1 -> A1: cycle
  sheet/A1:1: =B1*2
  sheet/B1:1: =A1
-- variant/cycle/out/calc/outcome --
rejected S201
` + twiceTail + `-- variant/cycle/out/resolve/error.txt --
A1 -> B1 -> A1: cycle
  sheet/A1:1: =B1*2
  sheet/B1:1: =A1
-- variant/cycle/out/resolve/outcome --
rejected S201
`

const twiceB = twiceHead + `-- variant/cycle/out/calc/error.txt --
cycle: A1 -> B1 -> A1
  sheet/A1:1: =B1*2
  sheet/B1:1: =A1
-- variant/cycle/out/calc/outcome --
rejected S201
` + twiceTail + `-- variant/cycle/out/resolve/error.txt --
cycle: A1 -> B1 -> A1
  sheet/A1:1: =B1*2
  sheet/B1:1: =A1
-- variant/cycle/out/resolve/outcome --
rejected S201
`

// negative's evaluate is a known gap, frozen; its calc gap marks a cell
// with no files.
const negative = `A1 is a negative number, which sheets#3 is about.
-- in/sheet/A1 --
-3
-- gaps --
evaluate A negative number is valid, and evaluates to itself. Waits on sheets#3.
calc A negative number is valid. Waits on sheets#3.
-- out/evaluate/error.txt --
A1: unexpected "-"
-- out/evaluate/outcome --
rejected S101
`

const divA = `C1 divides A1 by B1, which is zero.
-- in/sheet/A1 --
1
-- in/sheet/B1 --
0
-- in/sheet/C1 --
=A1/B1
-- out/calc/outcome --
accepted
-- out/calc/stdout.txt --
1,0,#DIV/0!
-- out/evaluate/outcome --
accepted
-- out/evaluate/values.txt --
A1 1
B1 0
C1 #DIV/0!
`

const divB = `C1 divides A1 by B1, which is zero.
-- in/sheet/A1 --
1
-- in/sheet/B1 --
0
-- in/sheet/C1 --
=A1/B1
-- out/calc/error.txt --
C1: division by zero
-- out/calc/outcome --
rejected S202
-- out/evaluate/outcome --
accepted div-zero
-- out/evaluate/values.txt --
A1 1
B1 0
C1 #DIV/0!
`

const blank = `A1 is empty.
-- in/sheet/A1 --
-- out/format/outcome --
skipped no formulas
-- out/parse/outcome --
accepted
`

const ratio = `A1 over B1, a ratio <1.
-- in/sheet/A1 --
=B1/C1
-- in/sheet/B1 --
1
-- in/sheet/C1 --
4
-- out/calc/outcome --
accepted
-- out/calc/stdout.txt --
0.25,1,4
-- out/evaluate/outcome --
accepted
-- out/evaluate/values.txt --
A1 0.25
B1 1
C1 4
-- out/format/formulas.txt --
A1 =B1 / C1
-- out/format/outcome --
accepted
-- out/parse/outcome --
accepted
-- out/resolve/outcome --
accepted
`
