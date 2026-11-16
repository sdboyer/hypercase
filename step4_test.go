package hypercase_test

import (
	"bytes"
	"strings"
	"testing"
)

// Step 4: expected errors and known gaps.

func TestExpectedError(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "expected")
	loop := s.read("testdata/script/loop.txtar")
	r := s.normal("rejects")
	s.fails(r, "--- PASS: TestCorpus/script/loop/echo", "--- FAIL: TestCorpus/script/reworded/echo", "out/echo/error.txt differs (-golden +got):\n", "- A1 -> A1: cycle\n", "+ cycle: A1 -> A1\n")
	s.passes(s.update("rejects"))
	// The update leaves the properties section as it was, and so the case.
	s.same("testdata/script/loop.txtar", loop)
	s.passes(s.normal("rejects"))
	// With its outputs stripped, an update writes the same archive again.
	s.write("testdata/script/loop.txtar", loop[:bytes.Index(loop, []byte("-- out/"))], "strip the outputs")
	s.passes(s.update("rejects"))
	s.same("testdata/script/loop.txtar", loop)
	s.done()
}

func TestUnexpectedError(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "unexpected")
	want := []string{
		"echo returned an error, but the case states no property echo rejects:\n",
		"Fix the code, or state the property on the case and in echo's Rejects.",
	}
	r := s.normal("rejects")
	s.fails(r, append(want, "negative: A1 is a negative number", "out/echo/values.txt differs")...)
	s.lacks(r, "gaps")
	r = s.update("rejects")
	s.fails(r, want...)
	s.lacks(r, "gaps")
	s.cat("testdata/script/negative.txtar")
	if !bytes.Contains(s.read("testdata/script/negative.txtar"), []byte("-- out/echo/error.txt --\nA1: unexpected \"-\"\n")) {
		t.Error("the update didn't write the unexpected error's error.txt")
	}
	s.fails(s.normal("rejects"), want...)
	s.done()
}

func TestMissingError(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "missing-error")
	want := []string{
		"echo returned no error, but the case states circular and dangling, which echo rejects.\n",
		"Fix the code, or correct the case's properties or echo's Rejects.",
	}
	r := s.normal("rejects")
	s.fails(r, want...)
	s.lacks(r, "gaps")
	s.fails(s.update("rejects"), want...)
	s.done()
}

func TestPropertyNotRejected(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "unrejected")
	s.fails(s.normal("rejects"), "--- PASS: TestCorpus/script/calm/echo", "--- FAIL: TestCorpus/script/errs/echo", "echo returned an error, but the case states no property echo rejects:")
	s.done()
}

// TestWrongPropertyPasses records a known weakness of the expected-error
// check: it tests that a rejecting column errs, not why. A case with a
// cycle that states dangling passes a column that rejects both. #19 records
// this.
func TestWrongPropertyPasses(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "weakness")
	s.passes(s.normal("rejects"))
	s.done()
}

func TestUnknownProperty(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "unknown-property")
	want := "testdata/script/loopy.txtar: properties states loopy, and no registry under the module-root corpus lists a property of that name.\nDeclare the property, or correct the line."
	s.fails(s.normal("rejects"), want, "out/echo/values.txt has no golden.")
	s.passes(s.update("rejects"))
	s.cat("testdata/script/loopy.txtar")
	if !bytes.Contains(s.read("testdata/script/loopy.txtar"), []byte("-- out/echo/values.txt --")) {
		t.Error("the update didn't write the case's outputs")
	}
	s.done()
}

func TestPropertiesForm(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "properties-form")
	lines, sections := s.read("testdata/script/lines.txtar"), s.read("testdata/script/sections.txtar")
	want := []string{
		"testdata/script/lines.txtar has problems of form",
		"properties, line 2: the line is blank",
		"properties, line 3: circular is stated twice, on lines 1 and 3",
		`properties, line 4: "Circular" is not one property's name`,
		`properties, line 5: "circular dangling" is not one property's name`,
		"testdata/script/sections.txtar has problems of form",
		"properties: the section is empty",
		"properties/x: properties is one section, with nothing below it",
		"gaps/x: gaps is one section, with nothing below it",
	}
	s.fails(s.normal("rejects"), want...)
	s.contains(s.update("rejects"), "testdata/script/lines.txtar has problems of form, so none of its cells run, and no update writes it. The root-package check reports them.")
	s.same("testdata/script/lines.txtar", lines)
	s.same("testdata/script/sections.txtar", sections)
	s.done()
}

func TestGapsForm(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "gaps-form")
	lines := s.read("testdata/script/lines.txtar")
	want := []string{
		"gaps, line 1: the line names echo and has no text: say what is right, and the issue it waits on",
		"gaps, line 2: the column's name is followed by more than one space",
		"gaps, line 3: the line is blank",
		"gaps, line 5: lines 4 and 5 both mark the cell of echo",
		`gaps, line 6: the line starts with "transform/Looped", which is not transform/<name> for a transform's name`,
		"gaps, line 7: the line names transform/looped echo and has no text",
		"testdata/script/empty.txtar has problems of form",
		"gaps: the section is empty",
	}
	s.fails(s.normal("rejects"), want...)
	s.contains(s.update("rejects"), "testdata/script/lines.txtar has problems of form, so none of its cells run, and no update writes it. The root-package check reports them.")
	s.same("testdata/script/lines.txtar", lines)
	s.done()
}

func TestKnownGaps(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "gaps")
	r := s.normal("rejects")
	// A marked cell passes while its run reproduces its files: one that errs
	// wrongly, one that doesn't err though the case states circular, and one
	// whose output is wrong.
	s.contains(r, "--- PASS: TestCorpus/script/negative/echo", "--- PASS: TestCorpus/script/accepted-loop/echo", "--- PASS: TestCorpus/script/div-zero/echo")
	s.fails(r,
		"testdata/script/other-column.txtar: gaps, line 1, names format, and no registry has a column of that name over the corpus script.\nRegister the column, or remove the line.",
		"testdata/script/out-of-place.txtar has update-written sections out of their place.",
		"out/echo/values.txt has no golden.") // unwritten: a marked cell with no files is written like any other
	negative, divZero := s.read("testdata/script/negative.txtar"), s.read("testdata/script/div-zero.txtar")
	outOfPlace := s.read("testdata/script/out-of-place.txtar")

	s.passes(s.update("rejects"))
	s.same("testdata/script/negative.txtar", negative)
	s.same("testdata/script/div-zero.txtar", divZero)
	// The update moves a frozen cell's sections into place, and never
	// changes their bytes.
	s.cat("testdata/script/out-of-place.txtar")
	if got := s.read("testdata/script/out-of-place.txtar"); bytes.Equal(got, outOfPlace) || !bytes.HasSuffix(got, []byte("-- out/echo/a.txt --\na\n-- out/echo/b.txt --\nb\n")) {
		t.Errorf("the update didn't put the frozen cell's sections in place, as they were")
	}
	// A marked cell with no committed files is written by an update.
	s.cat("testdata/script/unwritten.txtar")
	if !bytes.Contains(s.read("testdata/script/unwritten.txtar"), []byte("-- out/echo/values.txt --\nA1 0.3333\n")) {
		t.Error("the update didn't write the marked cell that had no files")
	}
	s.done()
}

func TestFrozenCellChanges(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "gaps")
	negative := s.read("testdata/script/negative.txtar")
	edits := []struct{ name, old, new, why string }{
		{"negative", "-- in/do/error --\nA1: unexpected \"-\"\n", "-- in/do/error --\nA1: bad number\n", "the code's error is worded differently"},
		{"div-zero", "-- in/do/out/values.txt --\nA1 0\n", "-- in/do/out/values.txt --\nA1 0.0\n", "the code's wrong output changes"},
		{"accepted-loop", "-- in/do/out/values.txt --\n", "-- in/do/out/c.txt --\nx\n-- in/do/out/values.txt --\n", "the code writes a new file"},
	}
	for _, e := range edits {
		rel := "testdata/script/" + e.name + ".txtar"
		s.write(rel, []byte(strings.Replace(string(s.read(rel)), e.old, e.new, 1)), e.why)
	}
	edited := map[string][]byte{}
	for _, e := range edits {
		edited[e.name] = s.read("testdata/script/" + e.name + ".txtar")
	}
	want := []string{
		"--- FAIL: TestCorpus/script/negative/echo",
		"--- FAIL: TestCorpus/script/div-zero/echo",
		"--- FAIL: TestCorpus/script/accepted-loop/echo",
		`The cell is frozen by its known gap, gaps line 1: "echo A negative number is valid, and evaluates to itself. Waits on sheets#3."`,
		"out/echo/error.txt differs (-committed +got):",
		"out/echo/values.txt differs (-committed +got):",
		"out/echo/c.txt is new",
		"If the gap is closed, remove the line, and the next update writes the cell. If the wrong behavior changed shape, delete the cell's files, and the next update writes them.",
	}
	s.fails(s.normal("rejects", "-test.run=TestCorpus/script/(negative|div-zero|accepted-loop)/"), want...)
	s.fails(s.update("rejects", "-test.run=TestCorpus/script/(negative|div-zero|accepted-loop)/"), want...)
	for _, e := range edits {
		s.same("testdata/script/"+e.name+".txtar", edited[e.name])
	}

	// A cell that stops writing a file it wrote fails too.
	s.write("testdata/script/negative.txtar", negative, "restore negative")
	s.write("testdata/script/negative.txtar", []byte(strings.Replace(string(negative), "-- in/do/error --\nA1: unexpected \"-\"\n", "", 1)), "the code stops erring")
	stopped := s.read("testdata/script/negative.txtar")
	s.fails(s.update("rejects", "-test.run=TestCorpus/script/negative/"), "out/echo/error.txt is no longer produced")
	s.same("testdata/script/negative.txtar", stopped)

	// The gap closed: with the line removed, the next update writes the cell.
	noGap := strings.Replace(string(stopped), "-- gaps --\necho A negative number is valid, and evaluates to itself. Waits on sheets#3.\n", "", 1)
	s.write("testdata/script/negative.txtar", []byte(noGap), "remove the gaps line")
	s.fails(s.update("rejects", "-test.run=TestCorpus/script/negative/"), "out/echo/error.txt records an error, but the run returned none.")
	s.note("the orphaned error.txt is a person's to remove")

	// The wrong behavior changed shape: with the files deleted and the line
	// kept, the next update writes them, and the cell is frozen again.
	divZero := strings.Replace(string(edited["div-zero"]), "-- out/echo/values.txt --\nA1 0\n", "", 1)
	s.write("testdata/script/div-zero.txtar", []byte(divZero), "delete the frozen cell's files")
	s.passes(s.update("rejects", "-test.run=TestCorpus/script/div-zero/"))
	s.cat("testdata/script/div-zero.txtar")
	// The root-package check fails on the fixture's other archives, whatever -run selects.
	s.contains(s.normal("rejects", "-test.run=TestCorpus/script/div-zero/"), "--- PASS: TestCorpus/script/div-zero/echo")
	s.done()
}

func TestPanicIsNeverExpected(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "panic-states")
	want := []string{"echo panicked: stack overflow", "Fix the code. A panic is never an expected error."}
	s.fails(s.normal("rejects"), want...)
	s.fails(s.update("rejects"), want...)
	s.cat("testdata/script/loop.txtar")
	loop := s.read("testdata/script/loop.txtar")
	if !bytes.Contains(loop, []byte("-- out/echo/error.txt --\nstack overflow\n")) {
		t.Error("the update didn't write the panic's value as error.txt")
	}
	// With a gaps line, the cell passes while its error.txt matches.
	marked := strings.Replace(string(loop), "-- out/echo/", "-- gaps --\necho A cycle is rejected, not a crash. Waits on sheets#9.\n-- out/echo/", 1)
	s.write("testdata/script/loop.txtar", []byte(marked), "mark the cell as a known gap")
	s.passes(s.normal("rejects"))
	s.done()
}

func TestRejectsRelations(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "expected")
	r := s.normal("rejects-unregistered")
	s.fails(r, "the suite echo rejects circular, and the package registers no property of that name.\nRegister the property, or remove it from echo's Rejects.", "No case runs until the registrations agree.")
	s.lacks(r, "TestCorpus/script/loop")
	s.fails(s.normal("rejects-other-description"), "the suite echo rejects a property named circular, which the package registers with another description.\n    Keep one declaration of circular, with one description: register it, and list it in echo's Rejects.")
	// A property registered in a later Register call, as from a file
	// initialized after the suite's, is accepted.
	s.contains(s.normal("rejects-registered-later"), "--- PASS: TestCorpus/script/loop/echo")
	s.done()
}

func TestPropertyRefusals(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "expected")
	for _, tt := range []struct{ reg, want string }{
		{"two-properties", "panic: hypercase: two properties are named circular. Register each property once."},
		{"bad-property-name", `panic: hypercase: the property name "Circular" breaks the naming rule`},
		{"rejects-twice", "panic: hypercase: the suite echo rejects circular twice."},
	} {
		s.fails(s.normal(tt.reg), tt.want)
	}
	s.done()
}

func TestPropertyWithoutDescription(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "unknown-property")
	want := "the property circular has no description."
	s.fails(s.update("property-nodesc"), want)
	if !bytes.Contains(s.read("testdata/script/loopy.txtar"), []byte("-- out/echo/values.txt --")) {
		t.Error("the update didn't write outputs")
	}
	s.fails(s.normal("property-nodesc"), want)
	s.done()
}
