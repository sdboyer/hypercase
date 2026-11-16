package hypercase_test

import (
	"bytes"
	"strings"
	"testing"
)

// Step 6: corpus types and lifts, over the ladder of fixtures_lifts_test.go.

// hasSections checks an archive holds each section, with its bytes, and
// lacks each section named in lacking.
func (s *sandbox) hasSections(rel string, want map[string]string, lacking ...string) {
	s.t.Helper()
	data := s.read(rel)
	for name, body := range want {
		if !bytes.Contains(data, []byte("-- "+name+" --\n"+body)) {
			s.t.Errorf("%s lacks the section %s holding %q:\n%s", rel, name, body, data)
		}
	}
	for _, name := range lacking {
		if bytes.Contains(data, []byte("-- "+name)) {
			s.t.Errorf("%s holds %s, and shouldn't:\n%s", rel, name, data)
		}
	}
}

func TestLadder(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "ladder")
	s.passes(s.update("ladder"))
	for _, a := range []string{"fine", "stray", "loop", "both", "noisy"} {
		s.cat("testdata/script/" + a + ".txtar")
	}
	// A lift's cell records its function's error once, and its stderr.
	s.hasSections("testdata/script/stray.txtar", map[string]string{
		"out/parse/error.txt": "D1: not a number or a formula: \"total\"\n",
		"out/calc/error.txt":  "D1: not a number or a formula: \"total\"\n",
	}, "out/evaluate/", "out/format/", "out/resolve/")
	s.hasSections("testdata/script/loop.txtar", map[string]string{
		"out/resolve/error.txt": "A1 -> A1: cycle\n",
		"out/format/trail.txt":  "parse\n",
	}, "out/evaluate/", "out/parse/")
	s.hasSections("testdata/script/both.txtar", map[string]string{"out/parse/error.txt": "D1: not a number\n"}, "out/resolve/")
	s.hasSections("testdata/script/noisy.txtar", map[string]string{"out/parse/stderr.txt": "parse: a warning\n"}, "out/parse/error.txt")
	// A lift and a suite's run receive the archive's name.
	s.hasSections("testdata/script/fine.txtar", map[string]string{
		"out/parse/stderr.txt":     "archive: fine\n",
		"out/evaluate/archive.txt": "fine\n",
		"out/format/trail.txt":     "parse\n",
	})

	r := s.normal("ladder")
	s.passes(r)
	s.contains(r,
		"--- PASS: TestCorpus/script/stray/calc",
		"--- PASS: TestCorpus/script/stray/parse",
		"--- SKIP: TestCorpus/script/stray/evaluate",
		"script/stray: evaluate is blocked: the lift parse failed on the case, and testdata/script/stray.txtar, out/parse/error.txt, records its error.",
		"script/stray: format is blocked: the lift parse failed on the case",
		"script/stray: resolve is blocked: the lift parse failed on the case",
		"script/loop: evaluate is blocked: the lift resolve failed on the case, and testdata/script/loop.txtar, out/resolve/error.txt, records its error.",
		"--- PASS: TestCorpus/script/loop/format",
		"--- PASS: TestCorpus/script/loop/resolve",
		// Blocked, resolve's cell isn't checked for the error both states.
		"script/both: resolve is blocked: the lift parse failed on the case",
	)
	// A blocked suite's probe isn't called.
	s.lacks(r, "evaluate's probe says no")
	s.done()
}

// TestCellOrder: a case's cells run in column-name order, whatever order
// they were registered in, and each lift's function runs once per case,
// when the first cell that needs its product runs.
func TestCellOrder(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "ladder")
	s.passes(s.update("ladder"))
	cells := "--- PASS: TestCorpus/script/fine/calc\n--- PASS: TestCorpus/script/fine/evaluate\n--- PASS: TestCorpus/script/fine/format\n--- PASS: TestCorpus/script/fine/parse\n--- PASS: TestCorpus/script/fine/resolve\n"
	ran := "what ran, in order:\ncalc ran on fine\nparse ran on fine\nresolve ran on fine\nevaluate's probe ran\nevaluate ran on fine\nformat's probe ran\nformat ran on fine\n"
	for _, reg := range []string{"ladder", "ladder-shuffled"} {
		r := s.normal(reg, "-test.run=TestCorpus/script/fine/")
		s.passes(r)
		s.contains(r, ran, cells)
	}
	// The internal switch runs them in reverse.
	r := s.runEnv("", "ladder", []string{reverseEnv + "=1"}, "-test.run=TestCorpus/script/fine/")
	s.passes(r)
	s.contains(r, "what ran, in order:\nparse ran on fine\nresolve ran on fine\nformat's probe ran\nformat ran on fine\nevaluate's probe ran\nevaluate ran on fine\ncalc ran on fine\n",
		"--- PASS: TestCorpus/script/fine/resolve\n--- PASS: TestCorpus/script/fine/parse\n--- PASS: TestCorpus/script/fine/format\n--- PASS: TestCorpus/script/fine/evaluate\n--- PASS: TestCorpus/script/fine/calc\n")
	s.done()
}

// TestTwoOrders: the self-test corpus, updated in column-name order and in
// reverse, gives every cell the same bytes. A suite that changes the
// product it shares makes them differ.
func TestTwoOrders(t *testing.T) {
	t.Parallel()
	for _, fixture := range []string{"ladder", "lift-errors", "render"} {
		reg := map[string]string{"ladder": "ladder", "lift-errors": "ladder", "render": "ladder-render"}[fixture]
		forward, reverse := newSandbox(t, fixture), newSandbox(t, fixture)
		forward.update(reg)
		reverse.runEnv("", reg, []string{fixtureUpdate + "=1", reverseEnv + "=1"})
		if got, want := reverse.snapshot(), forward.snapshot(); !equalSnapshots(got, want) {
			t.Errorf("%s: the update in reverse order wrote other bytes than in column-name order", fixture)
		}
	}
	forward, reverse := newSandbox(t, "ladder"), newSandbox(t, "ladder")
	forward.update("ladder-mutate")
	reverse.runEnv("", "ladder-mutate", []string{fixtureUpdate + "=1", reverseEnv + "=1"})
	if equalSnapshots(reverse.snapshot(), forward.snapshot()) {
		t.Error("a suite that appends to the shared *Parsed should make the two orders differ")
	}
	if got := string(reverse.read("testdata/script/fine.txtar")); !strings.Contains(got, "-- out/format/trail.txt --\nparse mutate\n") {
		t.Errorf("in reverse, format should see mutate's change:\n%s", got)
	}
}

func equalSnapshots(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || !bytes.Equal(v, w) {
			return false
		}
	}
	return true
}

// TestLiftExpectedErrors: the expected-error check judges a lift's cell as
// it does a suite's, and leaves a blocked cell unchecked.
func TestLiftExpectedErrors(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "lift-errors")
	gapped := s.read("testdata/script/gapped.txtar")
	panicsGapped := s.read("testdata/script/panics-gapped.txtar")
	orphan := s.read("testdata/script/blocked-orphan.txtar")
	unstated := "script/unstated: parse rejects a script that states nothing.\nparse: Parses the script, as its parse/ tree says.\nparse returned an error, but the case states no property parse rejects:\n  A1: not a number\nFix the code, or state the property on the case and in parse's Rejects."
	quiet := "parse returned no error, but the case states malformed, which parse rejects."
	blockedGap := "gaps, line 1, marks the cell, and the cell is blocked: the lift resolve, on evaluate's path, failed on the case, so the cell runs nothing.\nRemove the line, or fix the lift or the case."
	blockedOrphan := "out/evaluate/values.txt is recorded, and the cell is blocked: the lift resolve, on evaluate's path, failed on the case.\nIf the cell should run, fix the lift or the case. If not, delete the section by hand; FIXTURE_UPDATE_GOLDEN=1 go test ./... leaves it in place."
	panics := "parse panicked: parse: boom\nFix the code. A panic is never an expected error."
	for _, update := range []bool{false, true} {
		r := s.run("", "ladder", update)
		s.fails(r, unstated, quiet, blockedGap, blockedOrphan, panics,
			"--- PASS: TestCorpus/script/gapped/parse",
			"--- PASS: TestCorpus/script/panics-gapped/parse",
			"script/panics: evaluate is blocked: the lift parse failed on the case")
		s.lacks(r, "--- FAIL: TestCorpus/script/gapped/", "--- FAIL: TestCorpus/script/panics-gapped/")
	}
	// The update writes parse's error, keeps the frozen cells' files, and
	// leaves the blocked cell's orphan in place.
	s.hasSections("testdata/script/unstated.txtar", map[string]string{"out/parse/error.txt": "A1: not a number\n"})
	s.hasSections("testdata/script/panics.txtar", map[string]string{"out/parse/error.txt": "parse: boom\n"})
	s.same("testdata/script/gapped.txtar", gapped)
	s.same("testdata/script/panics-gapped.txtar", panicsGapped)
	s.same("testdata/script/blocked-orphan.txtar", orphan)

	// The frozen cell fails once its error text changes.
	s.write("testdata/script/gapped.txtar", bytes.Replace(gapped, []byte("-- in/parse/error --\nA1: wrongly rejected\n"), []byte("-- in/parse/error --\nA1: wrongly rejected, reworded\n"), 1), "parse's wrong error is reworded")
	for _, update := range []bool{false, true} {
		s.fails(s.run("", "ladder", update, "-test.run=TestCorpus/script/gapped/"), "The cell is frozen by its known gap, gaps line 1")
	}
	s.done()
}

func TestRenderError(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "render")
	before := map[string][]byte{}
	for _, a := range []string{"render-panic", "render-nonl"} {
		before[a] = s.read("testdata/script/" + a + ".txtar")
	}
	for _, update := range []bool{true, false} {
		r := s.run("", "ladder-render", update)
		s.fails(r,
			"The project's RenderError panicked on parse's error: render: boom\nFix RenderError. An update writes nothing for this cell.",
			"The project's RenderError returned \"rendered: A1: not a number\" for parse's error, which doesn't end with a newline.\nIts output must end with a newline: fix RenderError. An update writes nothing for this cell.",
			"--- PASS: TestCorpus/script/quoted/parse",
			"--- PASS: TestCorpus/script/quoted-other/parse",
			"--- PASS: TestCorpus/script/suite-error/calc")
	}
	// RenderError gets each case's own view, renders suites' errors too,
	// and never a panic's.
	s.hasSections("testdata/script/quoted.txtar", map[string]string{"out/parse/error.txt": "rendered: A1: not a number\n  quote: A1 holds one\n"})
	s.hasSections("testdata/script/quoted-other.txtar", map[string]string{"out/parse/error.txt": "rendered: A1: not a number\n  quote: A1 holds two\n"})
	s.hasSections("testdata/script/suite-error.txtar", map[string]string{
		"out/calc/error.txt":  "rendered: D1: not a number\n",
		"out/parse/error.txt": "parse: boom\n",
	})
	for a, data := range before {
		s.same("testdata/script/"+a+".txtar", data)
	}
	s.done()
}

// TestPaths: there is exactly one path to each type a column consumes, or
// the run fails before any case runs.
func TestPaths(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "ladder")
	twoPaths := "from the corpus type hypercase_test.Script, of testdata/script/, *hypercase_test.Graph, which the suite evaluate consumes, is reached by 2 paths:\n  hypercase_test.Script → direct → *hypercase_test.Graph\n  hypercase_test.Script → parse → *hypercase_test.Parsed → resolve → *hypercase_test.Graph\nThere must be exactly one path to each type a column consumes. Remove a lift, or give the paths distinct product types."
	for _, reg := range []string{"two-paths", "split"} {
		r := s.normal(reg)
		s.fails(r, twoPaths, "No case runs until each type a column consumes is reached by one path.")
		s.lacks(r, "--- PASS: TestCorpus/")
	}
	s.passes(s.update("split-second"))
	s.passes(s.update("ladder"))
	r := s.normal("cycle")
	s.fails(r, "*hypercase_test.Graph, which the lift back and the suite evaluate consume, is reached through the cycle resolve, back, so by more than one path:\n  hypercase_test.Script → parse → *hypercase_test.Parsed → resolve → *hypercase_test.Graph")
	s.lacks(r, "--- PASS: TestCorpus/")
	s.fails(s.normal("unreached"), "the suite evaluate consumes *hypercase_test.Graph, which no registered corpus type reaches.\nRegister the lift that produces it, or the corpus type.")
	s.fails(s.normal("fs-suite"), "the suite raw consumes fs.FS, which no corpus type is. A column consumes a corpus type, for the raw trees, or a stage a lift reaches.\nDeclare a decl.Type for the archives' directory, and consume it.")
	s.done()
}

// TestTwoCorpusTypes: two corpus types may each reach one type by a path
// of their own, and a suite over it runs over both directories.
func TestTwoCorpusTypes(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "two-types")
	s.passes(s.update("two-types"))
	r := s.normal("two-types")
	s.passes(r)
	s.contains(r, "--- PASS: TestCorpus/grid/b/format", "--- PASS: TestCorpus/grid/b/grid-parse", "--- PASS: TestCorpus/script/a/format", "--- PASS: TestCorpus/script/a/parse")
	s.hasSections("testdata/grid/b.txtar", map[string]string{"out/format/trail.txt": "grid-parse\n"})
	s.cat("testdata/hypercase.json")
	s.done()
}

func TestLiftRefusals(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "ladder")
	for _, tt := range []struct{ reg, want string }{
		{"fs-product", "panic: hypercase: the lift copy's product, hypercase_test.Other, is a corpus type, and this hypercase doesn't run lifts whose product is a corpus type. Make the product a stage, a type that isn't a named fs.FS wrapper."},
		{"jenny-lift", "panic: hypercase: the lift parse gives a Jenny, which makes a product that is a corpus type, and its product is *hypercase_test.Parsed. Give it Run, which makes the product."},
		{"lift-no-run", "panic: hypercase: the lift parse has no Run. Give it the function that makes its product."},
		{"lift-bad-output", `panic: hypercase: the lift parse declares an output with the pattern "values.txt", which matches neither error.txt nor stderr.txt, the only files a lift's cell writes. Remove the declaration.`},
		{"lift-bad-name", `panic: hypercase: the lift name "Parse" breaks the naming rule`},
		{"checks-dir", "panic: hypercase: the corpus type hypercase_test.Script declares the directory checks, a name reserved for the findings of corpus checks. Name another directory."},
		{"type-twice", "panic: hypercase: the corpus type hypercase_test.Script is registered twice. Register it once."},
		{"type-two-dirs", "panic: hypercase: the corpus type hypercase_test.Script is declared twice, with the directories script and grid. A corpus type has one directory; register one declaration."},
		{"dir-twice", "panic: hypercase: two corpus types declare the directory script: hypercase_test.Script and hypercase_test.Grid. An archive's directory gives its type; declare another directory for one of them."},
		{"dir-twice-calls", "panic: hypercase: two corpus types declare the directory script: hypercase_test.Script and hypercase_test.Grid."},
		{"lift-suite-name", "panic: hypercase: two columns are named parse: the lift parse, from hypercase_test.Script to *hypercase_test.Parsed, and the suite parse, over hypercase_test.Script. Column names are unique; rename one."},
		{"lift-suite-name-calls", "panic: hypercase: two columns are named parse: the suite parse, over hypercase_test.Script, and the lift parse, from hypercase_test.Script to *hypercase_test.Parsed. Column names are unique; rename one."},
		{"two-lifts-name", "panic: hypercase: two columns are named parse: the lift parse, from hypercase_test.Script to *hypercase_test.Parsed, and the lift parse, from hypercase_test.Grid to *hypercase_test.Parsed. Column names are unique; rename one."},
		{"own-and-use", "panic: hypercase: the lift parse is given to both hypercase.Own and hypercase.Use. A package owns a lift, or only uses it: pass it to one."},
		{"own-and-use-calls", "panic: hypercase: the lift parse is given to both hypercase.Own and hypercase.Use."},
		{"bare-suite", "panic: hypercase: Register was given the suite calc. A package owns the suites it runs: pass it to hypercase.Own."},
		{"bare-lift", "panic: hypercase: Register was given the lift parse. Pass it to hypercase.Own, in the package that owns its column, or to hypercase.Use, in a package that only reads its product."},
		{"use-suite", "panic: hypercase: hypercase.Use was given the suite calc. It takes lifts only: a package runs only its own suites. Own the suite in the package that runs it."},
		{"own-other", "panic: hypercase: hypercase.Own was given a decl.Property. It takes the columns the package owns: hypercase.Suite and decl.Lift values."},
	} {
		r := s.normal(tt.reg)
		s.fails(r, tt.want)
		s.lacks(r, "--- ")
	}
	s.done()
}

func TestLiftRelations(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "no-archive")
	r := s.normal("lift-rejects-unregistered")
	s.fails(r, "the lift parse rejects malformed, and the package registers no property of that name.\nRegister the property, or remove it from parse's Rejects.", "No case runs until the registrations agree.")
	s.passes(s.normal("lift-rejects-later"))
	// A lift's column that no archive reaches is no failure; a suite's is.
	s.passes(s.normal("lift-only"))
	s.fails(s.update("calc-only"), "the suite calc applies to no case: its corpus, testdata/script/, holds no archive whose cells run.")
	s.cat("testdata/hypercase.json")
	s.done()
}

func TestLiftDescriptions(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "ladder")
	want := []string{
		"the corpus type hypercase_test.Script, of testdata/script/, has no description.\nAdd one: it says what the type's inputs are, in prose.",
		"the lift parse has no description.\nAdd one: it says what the lift does, in prose.",
		"the lift parse declares the output error.txt with no description.\nAdd one: it says what the files hold, in prose.",
	}
	s.fails(s.normal("lift-nodesc"), want...)
	// An update fails too, and still writes the outputs.
	s.fails(s.update("lift-nodesc"), want...)
	s.hasSections("testdata/script/stray.txtar", map[string]string{"out/parse/error.txt": "D1: not a number or a formula: \"total\"\n"})
	s.done()
}

// TestParamsReserved: an archive with an in/params/ tree is a problem of
// the archive itself.
func TestParamsReserved(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "params")
	r := s.normal("calc-only")
	s.fails(r, "testdata/script/params.txtar has problems of form, so none of its cells run, and no update writes it:\n  in/params/x: the tree name params is reserved: a column's parameters are fixed in Go, or derived from the archive's name, and an archive carries none\nFix the archive by hand.",
		"--- PASS: TestCorpus/script/fine/calc")
	s.lacks(r, "TestCorpus/script/params/")
	s.done()
}
