package hypercase_test

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/revmap"
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
// other cell gets its own. The update leaves an inputs file such a cell
// has as it is, as it leaves any output it doesn't write, while it
// rewrites the cell's other files.
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
	t.Run("kept", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "basic")
		want := "the suite echo declares the output values.txt with no Inputs."
		const twice = "testdata/script/twice.txtar"
		s.edit(twice, "echo's values.txt, for the update to write, beside the inputs file of an older map", func(a *archive.Archive) {
			drop("out/echo/values.txt")(a)
			set("out/echo/inputs", "values.txt\n  in/do/out/values.txt:1-1\n")(a)
		})
		s.fails(s.update("inputs-none-suite"), want)
		s.hasSections(twice, map[string]string{"out/echo/values.txt": "A1 6\nB1 3\n", "out/echo/inputs": "values.txt\n  in/do/out/values.txt:1-1\n"})
		s.fails(s.normal("inputs-none-suite"), want)
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

// TestInputsPositionsReadTheView: Positions receives the cell's input
// view, and can read it to turn where an error points into a path in the
// view. Here the error names its file by an absolute path, under the root
// that the view's file loader/root names, and a variant that moves the root
// overlays a root of its own, which its cell's Positions reads.
func TestInputsPositionsReadTheView(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "basic")
	const rooted = "testdata/script/rooted.txtar"
	s.write(rooted, []byte("The error names its file by an absolute path, under the root the view names.\n-- in/do/error --\nA1 -> A1: cycle\nat 2 2 /sheets@v1/do/error\n-- in/loader/root --\n/sheets@v1\n-- properties --\ncircular\n-- variant/moved/about --\nThe root moves, and the error names its file under the new one.\n-- variant/moved/properties --\ncircular\n-- variant/moved/in/do/error --\nA1 -> A1: cycle\nat 1 2 /sheets@v2/do/error\n-- variant/moved/in/loader/root --\n/sheets@v2\n"), "a case whose error names its file under the root its view names")
	s.passes(s.update("rejects"))
	s.hasSections(rooted, map[string]string{
		"out/echo/inputs":               "error.txt\n  in/do/error:2-2\n",
		"variant/moved/out/echo/inputs": "error.txt\n  variant/moved/in/do/error:1-2\n",
	})
	s.passes(s.normal("rejects"))
	s.done()
}

// TestInputsPositionsView: Positions receives the cell's input view, the
// one RenderError receives, at every cell of the chain, variant and
// transform fixtures. It runs each under recordingViews, in an update and
// then a normal run, and requires the calls of Positions and the cells
// whose error.txt takes its ranges from Positions to match one to one:
// each cell has a call whose error, rendered on the view the call
// received, is the cell's error.txt, and whose view is the cell's input
// view, as its Observed records them. That RenderError alone receiving
// another view is TestChainRenderError's to find. Among the cells it
// checks are a base's, a hand-written variant's, a transform's variant's,
// one past a bytes lift, and a bytes lift's own.
func TestInputsPositionsView(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	checked := map[string]bool{}
	t.Run("fixtures", func(t *testing.T) {
		for _, fixture := range []string{"chain", "variants", "transforms"} {
			t.Run(fixture, func(t *testing.T) {
				t.Parallel()
				s := newSandbox(t, fixture)
				dir := t.TempDir()
				for _, run := range []string{"update", "normal"} {
					hooks, observed := filepath.Join(dir, run+"-hooks.jsonl"), filepath.Join(dir, run+"-observed.jsonl")
					env := []string{hooksEnv + "=" + hooks, observeEnv + "=" + observed}
					if run == "update" {
						env = append(env, fixtureUpdate+"=1")
					}
					r := s.execRaw("", fixture, env)
					if !r.ok() {
						t.Fatalf("the %s run failed:\n%s", run, s.shown(r))
					}
					var cells []hypercase.Observed
					for _, o := range readObserved(t, observed) {
						if o.Record == "cell" && positioned(t, o) {
							cells = append(cells, o)
						}
					}
					uncalled, unclaimed := unmatched(readViewRecords(t, hooks), cells)
					for _, o := range uncalled {
						t.Errorf("in the %s run, %s: no call of Positions received the cell's error.txt and input view", run, cellName(o))
					}
					for _, rec := range unclaimed {
						t.Errorf("the %s run called Positions on the error %q, on the view %.12s, which no cell checked has with that error.txt", run, rec.Error, rec.Positions)
					}
					mu.Lock()
					for _, o := range cells {
						for _, k := range viewKinds(o) {
							checked[k] = true
						}
					}
					mu.Unlock()
				}
			})
		}
	})
	for _, k := range []string{"a base's", "a hand-written variant's", "a transform's variant's", "one past a bytes lift", "a bytes lift's own"} {
		if !checked[k] {
			t.Errorf("no cell checked is %s", k)
		}
	}
}

// unmatched matches the calls of Positions the records give to the cells,
// each to one with its rendered error.txt and its input view's digest. It
// gives the cells left with no call of their own, in order of package,
// case, prefix and column, the order the cells take the calls in; and the
// records of the calls no cell takes, in order of what they rendered, then
// of their view.
func unmatched(records []viewRecord, cells []hypercase.Observed) (uncalled []hypercase.Observed, unclaimed []viewRecord) {
	type call struct{ rendered, view string }
	calls := map[call]int{}
	for _, rec := range records {
		calls[call{rec.Rendered, rec.Positions}]++
	}
	cells = slices.Clone(cells)
	slices.SortFunc(cells, func(a, b hypercase.Observed) int {
		return cmp.Or(strings.Compare(a.Pkg, b.Pkg), strings.Compare(a.Case, b.Case), strings.Compare(a.Prefix, b.Prefix), strings.Compare(a.Column, b.Column))
	})
	for _, o := range cells {
		c := call{o.Files["error.txt"], viewDigest(o.View)}
		if calls[c] == 0 {
			uncalled = append(uncalled, o)
			continue
		}
		calls[c]--
	}
	records = slices.Clone(records)
	slices.SortStableFunc(records, func(a, b viewRecord) int {
		return cmp.Or(strings.Compare(a.Rendered, b.Rendered), strings.Compare(a.Positions, b.Positions))
	})
	for _, rec := range records {
		if c := (call{rec.Rendered, rec.Positions}); calls[c] > 0 {
			unclaimed = append(unclaimed, rec)
			calls[c]--
		}
	}
	return uncalled, unclaimed
}

// cellName names a cell by its Observed, in a message.
func cellName(o hypercase.Observed) string {
	return fmt.Sprintf("%s %s%s, in the package %s", o.Case, o.Prefix, o.Column, o.Pkg)
}

// TestUnmatched: of two cells with one error.txt on one view, and one
// call of Positions on them, the second, in order of case, takes no call;
// and a call on another view is taken by no cell.
func TestUnmatched(t *testing.T) {
	t.Parallel()
	view, other := map[string]string{"do/error": "boom\n"}, map[string]string{"do/error": "boom\n", "do/x": "x\n"}
	cell := func(c string) hypercase.Observed {
		return hypercase.Observed{Pkg: ".", Case: c, Column: "echo", Files: map[string]string{"error.txt": "boom\n"}, View: view}
	}
	records := []viewRecord{
		{Error: "boom", Rendered: "boom\n", Positions: viewDigest(view)},
		{Error: "boom", Rendered: "boom\n", Positions: viewDigest(other)},
	}
	uncalled, unclaimed := unmatched(records, []hypercase.Observed{cell("script/b"), cell("script/a")})
	if len(uncalled) != 1 || uncalled[0].Case != "script/b" {
		t.Errorf("the cells with no call are %v, want script/b's alone", uncalled)
	}
	if want := records[1:2]; !slices.Equal(unclaimed, want) {
		t.Errorf("the calls no cell takes are %v, want %v", unclaimed, want)
	}
}

// TestRecordingViews: recordingViews records each call of Positions with
// the error, the error rendered on the view the call received, by the
// project's RenderError or, where the project has none, as the framework
// renders it, and a digest of that view; and it refuses a project with no
// Positions.
func TestRecordingViews(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	finds := func(fs.FS, error) []decl.Range { return nil }
	view := fstest.MapFS{"do/a": {Data: []byte("a\n")}}
	for _, tt := range []struct {
		name   string
		render func(fs.FS, error) []byte
		want   string
	}{
		{"framework", nil, "boom\n"},
		{"project", func(in fs.FS, err error) []byte {
			data, _ := fs.ReadFile(in, "do/a")
			return []byte(err.Error() + " on " + string(data))
		}, "boom on a\n"},
	} {
		path := filepath.Join(dir, tt.name+".jsonl")
		recordingViews(decl.Project{Positions: finds, RenderError: tt.render}, path).Positions(view, errors.New("boom"))
		if got, want := readViewRecords(t, path), []viewRecord{{Error: "boom", Rendered: tt.want, Positions: digestOf(view)}}; !slices.Equal(got, want) {
			t.Errorf("with the %s's rendering, recordingViews recorded %+v, want %+v", tt.name, got, want)
		}
	}

	defer func() {
		if recover() == nil {
			t.Error("recordingViews took a project with no Positions")
		}
	}()
	recordingViews(decl.Project{}, filepath.Join(dir, "none.jsonl"))
}

// positioned reports whether a cell's Observed records a call of
// Positions: the cell has an error.txt, which no panic gave it, and an
// inputs file, which doesn't map error.txt to all inputs.
func positioned(t *testing.T, o hypercase.Observed) bool {
	t.Helper()
	inputs, ok := o.Files["inputs"]
	if _, has := o.Files["error.txt"]; !has || !ok || o.Files["outcome"] == archive.Panicked+"\n" {
		return false
	}
	files, err := revmap.Parse([]byte(inputs))
	if err != nil {
		t.Fatalf("%s %s%s: its inputs file: %v", o.Case, o.Prefix, o.Column, err)
	}
	i := slices.IndexFunc(files, func(f revmap.File) bool { return f.Path == "error.txt" })
	return i >= 0 && !files[i].All
}

// viewKinds names the kinds of input view a cell's Observed records, as
// TestInputsPositionsView requires some cell of each.
func viewKinds(o hypercase.Observed) []string {
	var kinds []string
	switch {
	case strings.HasPrefix(o.Prefix, "variant/"):
		kinds = append(kinds, "a hand-written variant's")
	case strings.HasPrefix(o.Prefix, "transform/"):
		kinds = append(kinds, "a transform's variant's")
	case len(o.Path) == 0 && !o.Via:
		kinds = append(kinds, "a base's")
	}
	if len(o.Path) > 0 {
		kinds = append(kinds, "one past a bytes lift")
	}
	if o.Via {
		kinds = append(kinds, "a bytes lift's own")
	}
	return kinds
}

// readViewRecords reads what recordingViews wrote to the file at path.
func readViewRecords(t *testing.T, path string) []viewRecord {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []viewRecord
	for line := range bytes.Lines(data) {
		var rec viewRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatal(err)
		}
		out = append(out, rec)
	}
	return out
}

// TestInputsClash: a column's output under inputs/ is one no archive can
// hold beside the cell's inputs file, as one under outcome/ is beside its
// outcome (TestOutcomeClash).
func TestInputsClash(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "file-dir")
	const clash = "testdata/script/clash.txtar"
	s.write(clash, []byte("echo writes a file under inputs/.\n-- in/do/out/inputs/x --\nx\n"), "a case whose output is inputs/x")
	s.fails(s.update("nested"), "script/clash: echo writes a file under inputs/.\n    echo: Does what the case's do/ tree says.\n    out/echo/inputs can't be held in an archive: the section is also the directory of out/echo/inputs/x.\n    Fix the code. An update writes nothing for this cell.")
	s.hasSections(clash, nil, "out/")
	s.done()
}
