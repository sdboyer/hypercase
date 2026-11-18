package hypercase_test

import (
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
)

// TestInputsRefusals: registration refuses what an output declaration's
// Inputs states that no declaration may, naming the declaration and what
// to state: FromPositions on a pattern that isn't error.txt, or on a
// product file; an InputsFunc on error.txt, or on a pattern that matches
// it; and a value of the constants' kind that is none of them. Each holds
// for a suite's declarations and a lift's, owned or used.
func TestInputsRefusals(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	for _, tt := range []struct{ reg, want string }{
		{"inputs-positions-values", `panic: hypercase: the suite echo declares the output "values.txt" with Inputs: decl.FromPositions, which only a declaration whose pattern is error.txt states: Positions finds where an error points, and only error.txt holds one. Give it a decl.InputsFunc or decl.AllInputs.`},
		{"inputs-func-error", "panic: hypercase: the suite echo declares the output error.txt with a decl.InputsFunc. Its ranges come from where the error points, through the project's Positions, or from the whole input view: state decl.FromPositions or decl.AllInputs."},
		{"inputs-func-star", `panic: hypercase: the suite echo declares the output "*.txt" with a decl.InputsFunc, and the pattern matches error.txt, whose ranges come from the project's Positions or the whole input view, never from a function. State decl.AllInputs, or change the pattern so it doesn't match error.txt and declare error.txt on its own, with decl.FromPositions or decl.AllInputs.`},
		{"inputs-other", `panic: hypercase: the suite echo declares the output "values.txt" with an Inputs that is none of decl.InputsFunc, decl.AllInputs and decl.FromPositions. State one of them.`},
		{"inputs-lift-positions", `panic: hypercase: the lift parse declares the output "stderr.txt" with Inputs: decl.FromPositions, which only a declaration whose pattern is error.txt states: Positions finds where an error points, and only error.txt holds one. Give it a decl.InputsFunc or decl.AllInputs.`},
		{"inputs-lift-func", `panic: hypercase: the lift parse declares the output "*.txt" with a decl.InputsFunc, and the pattern matches error.txt, whose ranges come from the project's Positions or the whole input view, never from a function.`},
		{"inputs-product-positions", `panic: hypercase: the lift import declares the product file "parse/*" with Inputs: decl.FromPositions, and Positions finds where an error points, which a product file never holds. Give it a decl.InputsFunc or decl.AllInputs.`},
		{"inputs-product-other", `panic: hypercase: the lift import declares the product file "parse/*" with an Inputs that is none of decl.InputsFunc, decl.AllInputs and decl.FromPositions. State a decl.InputsFunc or decl.AllInputs.`},
	} {
		r := s.normal(tt.reg)
		s.fails(r, tt.want)
		s.lacks(r, "--- ")
	}
	s.done()
}

// TestInputsRequired: a run fails on each output declaration that states
// nothing in Inputs, naming it and what it may state, beside the check of
// descriptions: a suite's, a lift's, owned or used, and a product file's,
// owned or used; and, at once, on the declarations that state
// FromPositions in a project with no Positions. The cells still run: the
// update still writes their outputs. A cell with a file of such a
// declaration, a product file among them, gets no inputs file, and every
// other cell gets its own.
func TestInputsRequired(t *testing.T) {
	t.Parallel()
	t.Run("suite", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "basic")
		want := []string{
			"the suite echo declares the output values.txt with no Inputs.\nState where its files come from: a decl.InputsFunc, which maps each file to the lines of the input view it comes from, or decl.AllInputs.",
			"the suite echo declares the output error.txt with no Inputs.\nState where its files come from: decl.FromPositions, which takes its ranges from where the error points, through the project's Positions, or decl.AllInputs.",
			"the suite echo declares the output e* with no Inputs, and the pattern matches error.txt.\nState decl.AllInputs, or change the pattern so it doesn't match error.txt and declare error.txt on its own, with decl.FromPositions or decl.AllInputs, and the rest with a decl.InputsFunc or decl.AllInputs.",
		}
		s.edit("testdata/script/twice.txtar", "echo's outputs, for the update to write", drop("out/echo/"))
		s.fails(s.update("inputs-none-suite"), want...)
		s.hasSections("testdata/script/twice.txtar", map[string]string{"out/echo/outcome": "accepted\n", "out/echo/values.txt": "A1 6\nB1 3\n"}, "out/echo/inputs")
		s.fails(s.normal("inputs-none-suite"), want...)
		s.done()
	})
	t.Run("lift", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "shared-ladder")
		want := []string{
			"the lift parse declares the output error.txt with no Inputs.\nState where its files come from: decl.FromPositions, which takes its ranges from where the error points, through the project's Positions, or decl.AllInputs.",
			"the lift parse declares the output stderr.txt with no Inputs.\nState where its files come from: a decl.InputsFunc, which maps each file to the lines of the input view it comes from, or decl.AllInputs.",
		}
		const x, noisy = "testdata/script/x.txtar", "testdata/script/noisy.txtar"
		s.write(noisy, []byte("parse writes to stderr.\n-- in/parse/stderr --\nparse: a warning\n-- in/evaluate/out/values.txt --\nevaluate 2\n-- in/outline/out/values.txt --\noutline 2\n"), "a case whose parse cell has a file of a declaration that states nothing")
		s.fails(s.update("inputs-none-lift-a"), want...)
		s.hasSections(noisy, map[string]string{"out/parse/stderr.txt": "parse: a warning\n", "out/evaluate/values.txt": "evaluate 2\n", "out/evaluate/inputs": "values.txt\n"}, "out/parse/inputs")
		s.hasSections(x, map[string]string{"out/parse/inputs": "", "out/evaluate/values.txt": "evaluate 1\n", "out/evaluate/inputs": "values.txt\n"})
		s.fails(s.run("b", "inputs-none-lift-b", true), want...)
		s.hasSections(x, map[string]string{"out/outline/values.txt": "outline 1\n", "out/outline/inputs": "values.txt\n"})
		s.fails(s.normal("inputs-none-lift-a"), want...)
		s.fails(s.run("b", "inputs-none-lift-b", false), want...)
		s.done()
	})
	t.Run("product", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "chain-shared")
		want := []string{
			"the lift import declares the product file parse/* with no Inputs.\nState where its files come from: a decl.InputsFunc, which maps each file to the lines of the lift's input view it comes from, or decl.AllInputs.",
		}
		const prices = "testdata/export/prices.txtar"
		s.fails(s.update("inputs-none-product-a"), want...)
		s.hasSections(prices, map[string]string{"via/import/quote/A1": "3\n", "out/parse/inputs": ""}, "out/import/inputs")
		s.fails(s.run("b", "inputs-none-product-b", true), want...)
		s.hasSections(prices, map[string]string{"out/list/files.txt": "3 files\n", "out/list/inputs": "files.txt\n  via/import/evaluate/out/values.txt\n  via/import/quote/A1\n"})
		s.fails(s.normal("inputs-none-product-a"), want...)
		s.fails(s.run("b", "inputs-none-product-b", false), want...)
		s.done()
	})
	t.Run("no-positions", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "ladder")
		want := "the project has no Positions, and the suite calc's error.txt, the suite evaluate's error.txt, the suite format's error.txt, the lift parse's error.txt and the lift resolve's error.txt state decl.FromPositions, which takes error.txt's ranges from it.\nGive the decl.Project its Positions, which finds where an error points, or state decl.AllInputs in place of decl.FromPositions."
		const fine, both = "testdata/script/fine.txtar", "testdata/script/both.txtar"
		s.fails(s.update("inputs-no-positions"), want)
		s.hasSections(fine, map[string]string{"out/calc/values.txt": "calc 1\n", "out/calc/inputs": "values.txt\n"})
		s.hasSections(both, map[string]string{"out/parse/error.txt": "D1: not a number\n"}, "out/parse/inputs")
		s.fails(s.normal("inputs-no-positions"), want)
		s.done()
	})
}

// TestInputsFaults: a range that names no lines of the view the cell ran
// on, an Inputs that fails or panics, and a Positions that panics or
// gives such a range, each fail the cell in a note that names the file,
// what gave its ranges, and the range, and says to fix it. An update
// writes nothing for the cell, and writes the others. Each holds for a
// suite's output, a stage lift's error.txt and a bytes lift's product
// file.
func TestInputsFaults(t *testing.T) {
	t.Parallel()
	t.Run("suite", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "basic")
		const twice = "testdata/script/twice.txtar"
		s.edit(twice, "echo's outputs, for the update to write", drop("out/echo/"))
		faults := []struct{ name, archive, want string }{
			{"range", "A range past its file's last line, after one that holds.\n-- in/do/out/values.txt --\nA1 6\n-- in/do/ranges/values.txt --\n1 1 do/out/values.txt\n1 2 do/out/values.txt\n",
				"The Inputs of echo's output values.txt gave out/echo/values.txt the range decl.Range{Path: \"do/out/values.txt\", Start: 1, End: 2}, and its End, 2, is past the last line of do/out/values.txt, which has 1 line.\n    Fix values.txt's Inputs. An update writes nothing for this cell."},
			{"failing", "An Inputs that fails.\n-- in/do/out/values.txt --\nA1 6\n-- in/do/ranges/values.txt --\nerror the map is lost\n",
				"The Inputs of echo's output values.txt failed on out/echo/values.txt: the map is lost\n    Fix values.txt's Inputs. An update writes nothing for this cell."},
			{"panicking", "An Inputs that panics.\n-- in/do/out/a.txt --\na\n-- in/do/ranges/a.txt --\npanic the map panicked\n",
				"The Inputs of echo's output ?.txt panicked on out/echo/a.txt: the map panicked\n    Fix ?.txt's Inputs. An update writes nothing for this cell."},
			{"finder", "A Positions that panics.\n-- in/do/error --\nA1 -> A1: cycle\nat panic the finder panicked\n-- properties --\ncircular\n",
				"The project's Positions panicked on echo's error: the finder panicked\n    Fix Positions. An update writes nothing for this cell."},
			{"pointing", "A Positions that gives a range past its file's last line.\n-- in/do/error --\nA1 -> A1: cycle\nat 1 3 do/error\n-- properties --\ncircular\n",
				"The project's Positions, on echo's error, gave out/echo/error.txt the range decl.Range{Path: \"do/error\", Start: 1, End: 3}, and its End, 3, is past the last line of do/error, which has 2 lines.\n    Fix Positions. An update writes nothing for this cell."},
		}
		var want []string
		for _, f := range faults {
			s.write("testdata/script/"+f.name+".txtar", []byte(f.archive), "a case whose echo cell has a fault in its inputs")
			want = append(want, f.want)
		}
		s.fails(s.update("rejects"), want...)
		for _, f := range faults {
			s.same("testdata/script/"+f.name+".txtar", []byte(f.archive))
		}
		s.hasSections(twice, map[string]string{"out/echo/outcome": "accepted\n", "out/echo/inputs": "values.txt\n  in/do/out/values.txt\n"})
		s.fails(s.normal("rejects"), want...)
		s.done()
	})
	t.Run("lift", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "ladder")
		const finder = "testdata/script/finder.txtar"
		data := []byte("A Positions that panics on parse's error.\n-- in/parse/error --\nD1: not a number\nat panic the finder panicked\n-- properties --\nmalformed\n")
		s.write(finder, data, "a case whose parse cell's Positions panics")
		want := "The project's Positions panicked on parse's error: the finder panicked\n    Fix Positions. An update writes nothing for this cell."
		s.fails(s.update("ladder"), want)
		s.hasSections(finder, map[string]string{"out/calc/outcome": "accepted\n", "out/resolve/outcome": "blocked parse\n"})
		if strings.Contains(string(s.read(finder)), "-- out/parse/") {
			t.Errorf("the update wrote parse's cell, whose Positions panicked")
		}
		s.fails(s.normal("ladder"), want)
		s.done()
	})
	t.Run("product", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "chain")
		s.passes(s.update("chain"))
		const prices = "testdata/export/prices.txtar"
		s.edit(prices, "import's map of quote/A1 gives a range past its last line", insert("in/import/ranges/quote/A1", "1 5 import/out/quote/A1\n", "in/import/out/quote/"))
		before := s.read(prices)
		want := "The Inputs of import's product file quote/* gave via/import/quote/A1 the range decl.Range{Path: \"import/out/quote/A1\", Start: 1, End: 5}, and its End, 5, is past the last line of import/out/quote/A1, which has 1 line.\n    Fix quote/*'s Inputs. An update writes nothing for this cell."
		s.fails(s.update("chain"), want)
		s.same(prices, before)
		s.fails(s.normal("chain"), want)
		s.done()
	})
}

// TestFrozenInputs: a frozen cell that ran and lacks its inputs file, as
// a cell frozen before inputs files were written does, fails each normal
// run, which says to run the update; when the cell's other files match
// the run, the update adds it, as the run made it, and changes nothing
// else. A cell that lacks the outcome too gains both in one update. A
// frozen cell whose inputs file differs from the run's, as one does when
// an edit moves the lines it maps, fails as a frozen cell whose files
// differ does, and gains nothing, until a person deletes its files.
func TestFrozenInputs(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "gaps")
	const loop = "testdata/script/accepted-loop.txtar"
	const head = "script/accepted-loop: A1 and B1 refer to each other, and the code wrongly accepts the sheet.\n    echo: Does what the case's do/ tree says.\n    The cell is frozen by its known gap, gaps line 1: \"echo A cycle is an error. Waits on sheets#4.\"\n    "
	before := s.read(loop)

	s.edit(loop, "drop the frozen cell's inputs, as before inputs files were written", drop("out/echo/inputs"))
	s.fails(s.normal("rejects"), head+"Its committed files match the run, and it has no out/echo/inputs, which every cell that ran has.\n    Run FIXTURE_UPDATE_GOLDEN=1 go test ./... to add it. The update adds the inputs file and changes nothing else.")
	s.passes(s.update("rejects"))
	s.same(loop, before)

	s.edit(loop, "drop the frozen cell's outcome and inputs, as before outcomes were written", func(a *archive.Archive) {
		drop("out/echo/inputs")(a)
		drop("out/echo/outcome")(a)
	})
	s.fails(s.normal("rejects"), head+"Its committed files match the run, and it has no out/echo/outcome or out/echo/inputs, which every cell that ran has.\n    Run FIXTURE_UPDATE_GOLDEN=1 go test ./... to add them. The update adds the outcome and the inputs file and changes nothing else.")
	s.passes(s.update("rejects"))
	s.same(loop, before)

	s.edit(loop, "the code now maps values.txt to its first line alone", insert("in/do/ranges/values.txt", "1 1 do/out/values.txt\n", "properties"))
	edited := s.read(loop)
	want := head + "Its committed files record today's behavior, and the run now produces different ones:\n    out/echo/inputs differs (-committed +got):\n      values.txt\n    -   in/do/out/values.txt\n    +   in/do/out/values.txt:1-1\n    If the gap is closed, remove the line. If the wrong behavior changed shape, delete the cell's files."
	s.fails(s.normal("rejects"), want)
	s.fails(s.update("rejects"), want)
	s.same(loop, edited)
	s.edit(loop, "delete the frozen cell's files", drop("out/echo/"))
	s.passes(s.update("rejects"))
	s.hasSections(loop, map[string]string{"out/echo/inputs": "values.txt\n  in/do/out/values.txt:1-1\n", "out/echo/outcome": "accepted\n"})
	s.done()
}

// TestInputsTwoPackages: each package's update writes the inputs files
// of its own columns' cells in an archive both run over, and leaves the
// other's; each normal run fails on its own cells' missing inputs alone.
func TestInputsTwoPackages(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	const twice = "testdata/script/twice.txtar"
	before := s.read(twice)
	s.edit(twice, "drop both cells' inputs", func(a *archive.Archive) {
		drop("out/echo/inputs")(a)
		drop("out/shout/inputs")(a)
	})
	r := s.normal("pkg-a")
	s.fails(r, "out/echo/inputs has no golden.")
	s.lacks(r, "out/shout/inputs")
	r = s.run("b", "pkg-b", false)
	s.fails(r, "out/shout/inputs has no golden.")
	s.lacks(r, "out/echo/inputs")
	s.passes(s.update("pkg-a"))
	s.hasSections(twice, map[string]string{"out/echo/inputs": "values.txt\n  in/do/out/values.txt\n"}, "out/shout/inputs")
	s.passes(s.run("b", "pkg-b", true))
	s.same(twice, before)
	s.passes(s.normal("pkg-a"))
	s.passes(s.run("b", "pkg-b", false))
	s.done()
}

// TestInputsShared: one output declaration, of a lift and of a suite,
// maps each one's files against the view its own cell ran on: the base's
// in/, and a variant's own in/ where it replaces the file.
func TestInputsShared(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "ladder")
	for _, a := range []string{"both", "fine", "loop", "noisy", "stray"} {
		s.remove("testdata/script/"+a+".txtar", "")
	}
	const shared = "testdata/script/shared.txtar"
	s.write(shared, []byte("parse and chatty each write parse/stderr to stderr.\n-- in/parse/stderr --\nparse: a warning\n-- variant/quiet/about --\nAnother warning.\n-- variant/quiet/in/parse/stderr --\nparse: another warning\n"), "a case whose parse/ tree writes to stderr, and a variant with another warning")
	s.passes(s.update("inputs-shared"))
	s.hasSections(shared, map[string]string{
		"out/parse/inputs":                "stderr.txt\n  in/parse/stderr\n",
		"out/chatty/inputs":               "stderr.txt\n  in/parse/stderr\n",
		"variant/quiet/out/parse/inputs":  "stderr.txt\n  variant/quiet/in/parse/stderr\n",
		"variant/quiet/out/chatty/inputs": "stderr.txt\n  variant/quiet/in/parse/stderr\n",
	})
	s.passes(s.normal("inputs-shared"))
	s.done()
}

// TestInputsPositionsOfAnotherPackage: a package that uses a bytes lift
// holds the cells past it back on what keeps the update from writing the
// lift's cell under any Project, and its own Positions is no part of that.
// Here Positions panics on import's error in both packages: the owner's
// update writes nothing for import's cell, whose committed outcome stays
// accepted, and b's writes its cells past import as blocked by it, as it
// does when the owner's renderer panics.
func TestInputsPositionsOfAnotherPackage(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain-shared")
	const prices = "testdata/export/prices.txtar"
	for _, r := range s.parallel(childRun{"", "chain-a", true}, childRun{"b", "chain-b", true}) {
		s.passes(r)
	}
	s.edit(prices, "import now rejects prices, with an error whose positions panic", func(a *archive.Archive) {
		insert("properties", "unterminated\n", "in/")(a)
		insert("in/import/error", "line 1: unclosed quote\nat panic the finder panicked\n", "in/import/out/")(a)
	})
	s.fails(s.update("chain-a"), "The project's Positions panicked on import's error: the finder panicked\n    Fix Positions. An update writes nothing for this cell.")
	s.fails(s.run("b", "chain-b", true), "out/list/files.txt is recorded, and the cell is blocked: the lift import, on list's path, failed on the case.")
	s.hasSections(prices, map[string]string{
		"out/import/outcome":   "accepted\n",
		"out/evaluate/outcome": "blocked import\n",
		"out/list/outcome":     "blocked import\n",
	})
	s.done()
}
