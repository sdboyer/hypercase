package hypercase_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
)

// Step 8: outcomes, Codes, classifiers and corpus checks.

// TestOutcomes: every cell has an outcome, the code's verdict, whatever its
// state: accepted, rejected with the codes the project's Codes gives,
// sorted and each once, panicked, and skipped with the probe's reason. A
// classifier adds a detail to a cell that ran and returned, from what it
// consumed and what the run produced.
func TestOutcomes(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "outcomes")
	s.passes(s.update("outcomes"))
	for _, c := range []struct{ name, outcome string }{
		{"accepted", "accepted\n"},
		{"codes", "rejected S1 S2\n"},
		{"no-codes", "rejected\n"},
		{"panicked", "panicked\n"},
		{"skipped", "skipped the case is for no one\n"},
		{"detail", "accepted div-zero\n"},
		{"file-detail", "accepted big\n"},
		{"rejected-detail", "rejected S1; saw-error\n"},
	} {
		s.hasSections("testdata/script/"+c.name+".txtar", map[string]string{"out/echo/outcome": c.outcome})
	}
	s.hasSections("testdata/script/skipped.txtar", nil, "out/echo/")
	for _, name := range []string{"accepted", "codes", "skipped", "rejected-detail"} {
		s.cat("testdata/script/" + name + ".txtar")
	}
	// The registry records the classifier, and that the project gives
	// Codes.
	s.cat("testdata/hypercase.json")
	reg := string(s.read("testdata/hypercase.json"))
	for _, want := range []string{`"codes": true`, `"classifier": {`, `"name": "steered"`} {
		if !strings.Contains(reg, want) {
			t.Errorf("the registry doesn't hold %s:\n%s", want, reg)
		}
	}
	s.passes(s.normal("outcomes"))

	// The outcome is a golden.
	s.edit("testdata/script/codes.txtar", "record another code", set("out/echo/outcome", "rejected S1\n"))
	s.fails(s.normal("outcomes"), "script/codes: echo errs with two codes, one of them twice.\n    echo: Does what the case's do/ tree says.\n    out/echo/outcome differs (-golden +got):\n    - rejected S1\n    + rejected S1 S2")
	s.edit("testdata/script/skipped.txtar", "record another reason", set("out/echo/outcome", "skipped no reason\n"))
	s.fails(s.normal("outcomes"), "out/echo/outcome differs (-golden +got):\n    - skipped no reason\n    + skipped the case is for no one")
	s.done()
}

// TestOutcomeFailures: a cell whose code, classifier or probe gives what
// an outcome can't hold fails, naming it, and so does a classifier or a
// Codes that fails or panics. An update writes nothing for such a cell,
// and its committed outcome, if any, is left, not reported as an orphan.
func TestOutcomeFailures(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "outcome-failures")
	want := []string{
		"script/bad-code: Codes gives a code with a space in it.\n    echo: Does what the case's do/ tree says.\n    The project's Codes gave echo's error a code an outcome can't hold: the code \"a b\" can't be held in an outcome: a code is not empty, and holds no space, semicolon, carriage return or newline.\n    Fix Codes. An update writes nothing for this cell.",
		"script/codes-panic: Codes panics on echo's error.\n    echo: Does what the case's do/ tree says.\n    The project's Codes panicked on echo's error: codes boom\n    Fix Codes. An update writes nothing for this cell.",
		"script/cl-error: The classifier fails.\n    echo: Does what the case's do/ tree says.\n    echo's classifier steered failed: no detail today\n    Fix the classifier. An update writes nothing for this cell.",
		"script/cl-panic: The classifier panics.\n    echo: Does what the case's do/ tree says.\n    echo's classifier steered panicked: classifier boom\n    Fix the classifier. An update writes nothing for this cell.",
		"script/cl-bad: The classifier gives a detail with a space at its start.\n    echo: Does what the case's do/ tree says.\n    echo's classifier steered gave a detail an outcome can't hold: \" padded\" can't be held in an outcome: it is one line, not empty, with no space at either end.\n    Fix the classifier. An update writes nothing for this cell.",
		"script/reason: echo's probe says no with a reason of two lines.\n    echo: Does what the case's do/ tree says.\n    echo's probe said the case isn't for it, with a reason an outcome can't hold: \"two\\nlines\" can't be held in an outcome: it is one line, not empty, with no space at either end.\n    Fix the probe. An update writes nothing for this cell.",
	}
	s.fails(s.update("outcomes"), want...)
	for _, name := range []string{"bad-code", "codes-panic", "cl-error", "cl-panic", "cl-bad", "reason"} {
		s.hasSections("testdata/script/"+name+".txtar", nil, "out/")
		if s.exists("testdata/script/"+name+".txtar") && strings.Contains(string(s.read("testdata/script/"+name+".txtar")), "-- out/") {
			t.Errorf("the update wrote to %s, whose cell failed", name)
		}
	}
	s.hasSections("testdata/script/fine.txtar", map[string]string{"out/echo/outcome": "accepted\n"})
	r := s.normal("outcomes")
	s.fails(r, want...)
	// No update writes these cells, so no file of theirs is one to write,
	// and a file that differs from its golden says so.
	s.lacks(r, "has no golden")
	s.edit("testdata/script/cl-panic.txtar", "commit a values.txt the run no longer gives", set("out/echo/values.txt", "A1 7\n"))
	r = s.normal("outcomes")
	s.fails(r, "script/cl-panic: The classifier panics.\n    echo: Does what the case's do/ tree says.\n    echo's classifier steered panicked: classifier boom\n    Fix the classifier. An update writes nothing for this cell.\n    out/echo/values.txt differs (-golden +got):\n    - A1 7\n    + A1 6\n    An update writes nothing for this cell until the problems above are fixed.")
	s.lacks(r, "has no golden", "If the change is intended")

	// Such a cell has no outcome to judge, so a committed one is no orphan,
	// frozen or not, and the update leaves it.
	const clError = "testdata/script/cl-error.txtar"
	s.edit(clError, "commit the cell's files, as before the classifier failed", func(a *archive.Archive) {
		set("out/echo/outcome", "accepted\n")(a)
		set("out/echo/values.txt", "A1 6\n")(a)
	})
	committed := s.read(clError)
	clFails := "script/cl-error: The classifier fails.\n    echo: Does what the case's do/ tree says.\n    echo's classifier steered failed: no detail today\n    Fix the classifier. An update writes nothing for this cell."
	for _, update := range []bool{false, true} {
		r := s.run(".", "outcomes", update)
		s.fails(r, clFails)
		s.lacks(r, "out/echo/outcome is an output the run no longer produces", "has no golden")
	}
	s.same(clError, committed)
	s.edit(clError, "freeze the cell", insert("gaps", "echo The classifier gives a detail. Waits on sheets#10.\n", "out/"))
	committed = s.read(clError)
	for _, update := range []bool{false, true} {
		r := s.run(".", "outcomes", update)
		s.fails(r, clFails)
		s.lacks(r, "out/echo/outcome is no longer produced", "The cell is frozen")
	}
	s.same(clError, committed)
	s.done()
}

// TestClassifierRefusals: a classifier's name follows the naming rule, it
// has a Classify, and a description.
func TestClassifierRefusals(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "outcomes")
	for _, tt := range []struct{ reg, want string }{
		{"cl-bad-name", `panic: hypercase: the suite echo's classifier name "Steered" breaks the naming rule: lowercase ASCII letters and digits, in words joined by single hyphens, starting with a letter. Rename the classifier.`},
		{"cl-no-classify", "panic: hypercase: the suite echo's classifier steered has no Classify. Give it the function that gives a cell's detail."},
	} {
		r := s.normal(tt.reg)
		s.fails(r, tt.want)
		s.lacks(r, "--- ")
	}
	// The cells are written, so the run fails on the description alone.
	s.passes(s.update("outcomes"))
	s.fails(s.normal("cl-no-description"), "the suite echo's classifier steered has no description.\nAdd one: it says what the classifier's details mean, in prose.")
	s.done()
}

// TestFrozenOutcome: a cell frozen before outcomes were written has none.
// A normal run fails on it, and says to run the update, which adds the
// outcome and changes nothing else, when the cell's other files match the
// run. A frozen cell whose files differ gains nothing, and fails as
// before, and so does one whose run fails in a way that keeps an update
// from writing a cell.
func TestFrozenOutcome(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "gaps")
	const negative = "testdata/script/negative.txtar"
	before := s.read(negative)
	s.edit(negative, "drop the frozen cell's outcome, as before outcomes were written", drop("out/echo/outcome"))
	s.fails(s.normal("rejects"), "script/negative: A1 is a negative number, which the code wrongly rejects.\n    echo: Does what the case's do/ tree says.\n    The cell is frozen by its known gap, gaps line 1: \"echo A negative number is valid, and evaluates to itself. Waits on sheets#3.\"\n    Its committed files match the run, and it has no out/echo/outcome, which every cell has.\n    Run FIXTURE_UPDATE_GOLDEN=1 go test ./... to add it. The update adds the outcome and changes nothing else.")
	s.passes(s.update("rejects"))
	s.same(negative, before)

	s.edit(negative, "drop the outcome, and change the recorded error", func(a *archive.Archive) {
		drop("out/echo/outcome")(a)
		set("out/echo/error.txt", "A1: bad number\n")(a)
	})
	edited := s.read(negative)
	want := "Its committed files record today's behavior, and the run now produces different ones:\n    out/echo/error.txt differs (-committed +got):"
	r := s.normal("rejects")
	s.fails(r, want)
	s.lacks(r, "which every cell has")
	s.fails(s.update("rejects"), want)
	s.same(negative, edited)

	// A frozen cell whose run fails otherwise gains no outcome: an update
	// writes nothing for it, and no run says to add one.
	s.write(negative, before, "restore the frozen cell")
	s.edit(negative, "drop the outcome, and give the run an undeclared output, committed too", func(a *archive.Archive) {
		drop("out/echo/outcome")(a)
		insert("in/do/out/zz.undeclared", "z\n", "gaps")(a)
		set("out/echo/zz.undeclared", "z\n")(a)
	})
	edited = s.read(negative)
	undeclared := "out/echo/zz.undeclared is not declared.\n    Declare it in echo's Outputs, with a description, or fix the code. An update writes nothing for this cell."
	r = s.update("rejects")
	s.fails(r, undeclared)
	s.lacks(r, "which every cell has")
	s.same(negative, edited)
	r = s.normal("rejects")
	s.fails(r, undeclared)
	s.lacks(r, "which every cell has")
	s.done()
}

// TestFrozenVariantOutcome: the upgrade adds a missing outcome to a frozen
// cell of a hand-written variant, and of a transform's variant, as it does
// to a base's, and changes nothing else.
func TestFrozenVariantOutcome(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "check-gaps")
	const loop = "testdata/script/loop.txtar"
	before := s.read(loop)
	s.edit(loop, "drop the frozen variants' outcomes, as before outcomes were written", func(a *archive.Archive) {
		drop("variant/neg/out/echo/outcome")(a)
		drop("transform/looped/out/echo/outcome")(a)
	})
	s.fails(s.normal("check-gaps"),
		"script/loop/variant/neg: A1 is a negative number, which the code wrongly rejects.\n    echo: Does what the case's do/ tree says.\n    The cell is frozen by its known gap, variant/neg/gaps line 1: \"echo A negative number is valid, and evaluates to itself. Waits on sheets#3.\"\n    Its committed files match the run, and it has no variant/neg/out/echo/outcome, which every cell has.\n    Run FIXTURE_UPDATE_GOLDEN=1 go test ./... to add it. The update adds the outcome and changes nothing else.",
		"script/loop/transform/looped: Adds do/error and resolve/error, so echo and resolve err as on a cycle.\n    echo: Does what the case's do/ tree says.\n    The cell is frozen by its known gap, gaps line 1: \"transform/looped echo A cycle's error starts with the cell where it starts. Waits on sheets#4.\"\n    Its committed files match the run, and it has no transform/looped/out/echo/outcome, which every cell has.\n    Run FIXTURE_UPDATE_GOLDEN=1 go test ./... to add it. The update adds the outcome and changes nothing else.")
	s.passes(s.update("check-gaps"))
	s.same(loop, before)
	s.passes(s.normal("check-gaps"))
	s.done()
}

// TestFrozenRanNothing: a skipped or blocked cell that a gaps line marks,
// with committed files, is frozen like any other: an update leaves its
// outcome as committed, though the cell now runs nothing.
func TestFrozenRanNothing(t *testing.T) {
	t.Parallel()
	t.Run("skipped", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "probes")
		for _, a := range []string{"marked", "no", "noreason", "panics", "stale"} {
			s.remove("testdata/script/"+a+".txtar", "leave yes, which echo runs on")
		}
		const frozen = "testdata/script/frozen.txtar"
		s.write(frozen, []byte("The probe now says no to a cell a gaps line freezes.\n-- in/do/probe --\nno formulas\n-- gaps --\necho A1 should be 7. Waits on sheets#13.\n-- out/echo/outcome --\naccepted\n"), "a frozen cell the probe now skips")
		before := s.read(frozen)
		skipped := "gaps, line 1, marks the cell, and echo's probe says the case isn't for it: no formulas\n    Remove the line, or fix the probe."
		s.fails(s.update("probe"), skipped)
		s.same(frozen, before)
		r := s.normal("probe")
		s.fails(r, skipped)
		s.lacks(r, "out/echo/outcome differs")
		s.done()
	})
	t.Run("blocked", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "transforms")
		s.remove("testdata/script/errs.txtar", "leave twice, whose cases the lifts run on as they should")
		s.remove("testdata/script/garbled.txtar", "leave twice, whose cases the lifts run on as they should")
		s.passes(s.update("tr-ladder-eval"))
		s.edit(twice, "mark looped's blocked cell, and record another outcome for it", func(a *archive.Archive) {
			insert("gaps", "transform/looped evaluate Waits on sheets#9.\n", "out/")(a)
			set("transform/looped/out/evaluate/outcome", "accepted\n")(a)
		})
		before := s.read(twice)
		blocked := "gaps, line 1, marks the cell, and the cell is blocked: the lift resolve, on evaluate's path, failed on the case, so the cell runs nothing."
		s.fails(s.update("tr-ladder-eval"), blocked)
		s.same(twice, before)
		r := s.normal("tr-ladder-eval")
		s.fails(r, blocked)
		s.lacks(r, "transform/looped/out/evaluate/outcome differs")
		s.done()
	})
}

// TestOutcomeClash: a column's output under outcome/ is one no archive
// can hold beside the cell's outcome.
func TestOutcomeClash(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "file-dir")
	const clash = "testdata/script/clash.txtar"
	s.write(clash, []byte("echo writes a file under outcome/.\n-- in/do/out/outcome/x --\nx\n"), "a case whose output is outcome/x")
	s.fails(s.update("nested"), "script/clash: echo writes a file under outcome/.\n    echo: Does what the case's do/ tree says.\n    out/echo/outcome can't be held in an archive: the section is also the directory of out/echo/outcome/x.\n    Fix the code. An update writes nothing for this cell.")
	s.hasSections(clash, nil, "out/")
	s.done()
}

// TestVariantOutcomes: a variant's cells have outcomes too, under its
// prefix. A transform's variant the transform declines has no cells, so
// no outcome.
func TestVariantOutcomes(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.passes(s.update("transforms"))
	s.hasSections(twice, map[string]string{
		"out/echo/outcome":                  "accepted\n",
		"transform/looped/out/echo/outcome": "rejected\n",
		"transform/upper/out/echo/outcome":  "accepted\n",
	})
	s.hasSections("testdata/script/errs.txtar", map[string]string{"transform/upper/out/echo/outcome": "rejected\n"}, "transform/looped/")
	s.done()
}

// TestBlockedTransformVariant: a blocked cell of a transform's variant
// says to fix the lift or the transform's jenny, which makes the
// variant's inputs, where a case's says to fix the case.
func TestBlockedTransformVariant(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.remove("testdata/script/errs.txtar", "leave twice, whose cases the lifts run on as they should")
	s.remove("testdata/script/garbled.txtar", "leave twice, whose cases the lifts run on as they should")
	s.passes(s.update("tr-ladder-eval"))
	s.hasSections(twice, map[string]string{"transform/looped/out/evaluate/outcome": "blocked resolve\n"})
	s.edit(twice, "mark looped's blocked cell, and record a file for it", func(a *archive.Archive) {
		insert("gaps", "transform/looped evaluate Waits on sheets#9.\n", "out/")(a)
		insert("transform/looped/out/evaluate/a.txt", "a\n", "transform/looped/out/evaluate/outcome")(a)
	})
	r := s.normal("tr-ladder-eval")
	s.fails(r,
		"gaps, line 1, marks the cell, and the cell is blocked: the lift resolve, on evaluate's path, failed on the case, so the cell runs nothing.\n    Remove the line, or fix the lift or looped's jenny, which makes the variant.",
		"transform/looped/out/evaluate/a.txt is recorded, and the cell is blocked: the lift resolve, on evaluate's path, failed on the case.\n    If the cell should run, fix the lift or looped's jenny, which makes the variant. If not, delete the section by hand;")
	s.lacks(r, "fix the lift or the case", "out of their place")
	s.done()
}

// TestChecks: a corpus check reads every cell over the corpus root, from
// the committed files, whichever package owns the column, and its
// findings are a golden: sorted, each once, written by an update and
// compared by a normal run. No check sees what another changed.
func TestChecks(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.write("b/corpus_test.go", []byte(packageTestFile), "a package b over the corpus")
	s.passes(s.update("tr-checks"))
	s.passes(s.run("b", "tr-b-checks", true))
	// A check reads what is committed when it runs: the root's ran before
	// b wrote its cells, and the next update settles it.
	s.fails(s.normal("tr-checks"), "testdata/checks/cells.txt, the corpus check cells's findings, differs (-golden +got):")
	s.passes(s.update("tr-checks"))
	s.passes(s.normal("tr-checks"))
	s.passes(s.run("b", "tr-b-checks", false))
	s.cat("testdata/checks/cells.txt")
	if root, b := s.read("testdata/checks/cells.txt"), s.read("b/testdata/checks/cells.txt"); !bytes.Equal(root, b) {
		t.Errorf("two packages' cells checks found different cells:\n%s\n%s", root, b)
	}
	for _, f := range []struct{ name, want string }{
		{"unsorted", "a\nb\n"},
		{"alters", ""},
	} {
		if got := string(s.read("testdata/checks/" + f.name + ".txt")); got != f.want {
			t.Errorf("%s's findings are %q, want %q", f.name, got, f.want)
		}
	}

	// The findings are a golden.
	cells := s.read("testdata/checks/cells.txt")
	s.write("testdata/checks/cells.txt", append([]byte("a finding by hand\n"), cells...), "add a finding by hand")
	s.fails(s.normal("tr-checks"), "testdata/checks/cells.txt, the corpus check cells's findings, differs (-golden +got):\n    - a finding by hand")
	s.passes(s.update("tr-checks"))
	s.same("testdata/checks/cells.txt", cells)
	s.remove("testdata/checks/unsorted.txt", "remove a check's findings")
	s.fails(s.normal("tr-checks"), "testdata/checks/unsorted.txt, the corpus check unsorted's findings, has no golden.\n    Run FIXTURE_UPDATE_GOLDEN=1 go test ./... to write it, and read it before committing.")
	s.passes(s.update("tr-checks"))
	s.passes(s.normal("tr-checks"))

	// A findings file no check writes fails a normal run, and an update
	// leaves it.
	s.write("testdata/checks/gone.txt", []byte("x\n"), "leave the findings of a check that is gone")
	s.fails(s.normal("tr-checks"), "testdata/checks/gone.txt is the findings of no corpus check the package registers.\n    If a check should write it, register the check. If not, delete it by hand; FIXTURE_UPDATE_GOLDEN=1 go test ./... leaves it in place.")
	s.passes(s.update("tr-checks"))
	if !s.exists("testdata/checks/gone.txt") {
		t.Error("the update removed testdata/checks/gone.txt")
	}
	s.remove("testdata/checks/gone.txt", "delete it by hand")

	// A check reads committed files: a hand edit to an outcome moves the
	// findings, as well as failing the cell.
	s.edit(twice, "record another outcome by hand", set("out/echo/outcome", "rejected\n"))
	r := s.normal("tr-checks")
	s.fails(r, "out/echo/outcome differs (-golden +got):", "testdata/checks/cells.txt, the corpus check cells's findings, differs (-golden +got):",
		"- script/twice echo: accepted; the suite echo of .", "+ script/twice echo: rejected; the suite echo of .")
	s.passes(s.update("tr-checks"))
	s.passes(s.normal("tr-checks"))

	// An archive no check can read fails every check, and an update writes
	// no findings.
	findings := s.read("testdata/checks/cells.txt")
	s.write("testdata/script/broken.txtar", []byte("A section twice.\n-- in/do/out/values.txt --\nx\n-- in/do/out/values.txt --\ny\n"), "an archive with a problem of form")
	const unreadable = "can't read its cells: testdata/script/broken.txtar has problems of form:\n      in/do/out/values.txt: the section appears twice\n    Fix what the message names. No check runs, and an update writes no findings."
	r = s.update("tr-checks")
	s.fails(r, "the corpus check alters "+unreadable, "the corpus check cells "+unreadable, "the corpus check unsorted "+unreadable)
	s.same("testdata/checks/cells.txt", findings)
	s.remove("testdata/script/broken.txtar", "remove the broken archive")

	// So does a transform's variant whose properties can't be known: one
	// no corpus type lists, or one its transform declines.
	before := s.read(twice)
	s.edit(twice, "leave the cells of a transform no corpus type lists", insert("transform/gone/out/echo/outcome", "accepted\n", "transform/looped/"))
	gone := "can't read its cells: testdata/script/twice.txtar holds transform/gone, and no corpus type of testdata/script/ lists a transform named gone\n    Fix what the message names. No check runs, and an update writes no findings."
	for _, update := range []bool{false, true} {
		s.fails(s.run(".", "tr-checks", update), "the corpus check alters "+gone, "the corpus check cells "+gone, "the corpus check unsorted "+gone)
		s.same("testdata/checks/cells.txt", findings)
	}
	s.write(twice, before, "remove the transform's cells")
	s.edit(twice, "state a property each transform declines", insert("properties", "malformed\n", "out/"))
	declined := "can't read its cells: testdata/script/twice.txtar holds transform/looped, and looped declines the base: the base states malformed, which looped neither gives nor keeps\n    Fix what the message names. No check runs, and an update writes no findings."
	s.fails(s.normal("tr-checks"), "the corpus check alters "+declined, "the corpus check cells "+declined, "the corpus check unsorted "+declined)
	s.same("testdata/checks/cells.txt", findings)
	s.done()
}

// TestCheckGaps: a cell carries the text of the gaps line that marks it:
// the case's own, or for a transform's variant, the base's line that names
// the transform. The check's findings record every cell's gap, so no line
// reaches a cell it doesn't mark: not the base's, nor another transform's
// variant's. The framework exempts no frozen cell from a check, and a
// check that excuses a known gap passes while each cell that violates it
// is frozen, and fails once a line is removed.
func TestCheckGaps(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "check-gaps")
	s.passes(s.normal("check-gaps"))
	s.cat("testdata/checks/located.txt")
	for _, tt := range []struct {
		archive, why string
		edit         func(*archive.Archive)
		cell         string
	}{
		{"negative", "remove the line that marks the base's cell", drop("gaps"), "script/negative echo"},
		{"loop", "remove the line that marks the hand-written variant's cell", drop("variant/neg/gaps"), "script/loop/variant/neg echo"},
		{"loop", "remove the base's line that marks looped's cell", drop("gaps"), "script/loop/transform/looped echo"},
	} {
		p := "testdata/script/" + tt.archive + ".txtar"
		before := s.read(p)
		s.edit(p, tt.why, tt.edit)
		s.fails(s.normal("check-gaps"), "the corpus check located fails: the error.txt of "+tt.cell+" starts with no cell\n    The cells violate what the check requires.")
		s.write(p, before, "restore the line")
	}
	s.passes(s.normal("check-gaps"))
	s.done()
}

// TestCheckFailures: a check that errs or panics, or gives a finding that
// is empty or holds a newline, fails every run, and an update writes no
// findings for it. A check that finds nothing writes an empty file.
func TestCheckFailures(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "basic")
	for _, tt := range []struct{ reg, check, want string }{
		{"check-error", "fails", "the corpus check fails fails: no cell is rejected\n    The cells violate what the check requires. Fix the code or the corpus, or the check. An update writes no findings."},
		{"check-panic", "boom", "the corpus check boom panicked: check boom\n    Fix the check. An update writes no findings."},
		{"check-newline", "lines", "the corpus check lines gave a finding that holds a newline: \"one\\ntwo\".\n    Fix the check: a finding is one line. An update writes no findings."},
		{"check-empty", "empty", "the corpus check empty gave an empty finding.\n    Fix the check: a finding says something, on one line. An update writes no findings."},
	} {
		s.fails(s.update(tt.reg), tt.want, "--- FAIL: TestCorpus/checks/"+tt.check)
		if s.exists("testdata/checks/" + tt.check + ".txt") {
			t.Errorf("%s's update wrote findings", tt.reg)
		}
		s.fails(s.normal(tt.reg), tt.want)
	}
	s.passes(s.update("check-none"))
	if got := string(s.read("testdata/checks/none.txt")); got != "" {
		t.Errorf("none's findings are %q, want an empty file", got)
	}
	s.passes(s.normal("check-none"))
	s.done()
}

// TestCheckRefusals: a check's name follows the naming rule, and is the
// only one of its name in the package; it has a Run, and a description.
func TestCheckRefusals(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "basic")
	for _, tt := range []struct{ reg, want string }{
		{"check-bad-name", `panic: hypercase: the corpus check name "Cells" breaks the naming rule: lowercase ASCII letters and digits, in words joined by single hyphens, starting with a letter. Rename the check.`},
		{"check-no-run", "panic: hypercase: the corpus check idle has no Run. Give it the function that reads the cells and gives its findings."},
		{"check-twice", "panic: hypercase: two corpus checks are named none. Check names are unique per package; rename one."},
	} {
		r := s.normal(tt.reg)
		s.fails(r, tt.want)
		s.lacks(r, "--- ")
	}
	s.fails(s.normal("check-no-description"), "the corpus check none has no description.\nAdd one: it says what the check asserts, in prose.")
	s.done()
}
