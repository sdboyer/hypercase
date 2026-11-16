package hypercase_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Step 3: archives.

func TestMatchingGoldenPasses(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "basic")
	s.passes(s.normal("echo"))
	s.done()
}

func TestNewlineAndLineEndingDiffer(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "newline")
	r := s.normal("echo")
	s.fails(r,
		"--- FAIL: TestCorpus/script/extra-newline/echo",
		"--- FAIL: TestCorpus/script/line-ending/echo",
		`+ "A1 6\r"`)
	s.done()
}

func TestMissingGolden(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "missing-golden")
	before := s.read("testdata/script/twice.txtar")
	s.fails(s.normal("echo"), "out/echo/values.txt has no golden.", "Run FIXTURE_UPDATE_GOLDEN=1 go test ./... to write it")
	s.same("testdata/script/twice.txtar", before)
	s.passes(s.update("echo"))
	s.cat("testdata/script/twice.txtar")
	s.passes(s.normal("echo"))
	s.done()
}

func TestChangedOutput(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "changed")
	before := s.read("testdata/script/twice.txtar")
	r := s.normal("echo")
	s.fails(r, "out/echo/values.txt differs (-golden +got):", "- B1 3\n", "+ B1 3.0\n",
		"If the change is intended, run FIXTURE_UPDATE_GOLDEN=1 go test ./... and read the diff before committing.")
	// The message names the case and the column, with both descriptions.
	s.contains(r, "script/twice: A1 is twice B1, and the code now prints B1 as a decimal.\n", "echo: Does what the case's do/ tree says.\n")
	// The message reads the same in another process.
	if again := s.normal("echo"); again.out != r.out {
		t.Errorf("a second process printed another message:\n%s\nthen:\n%s", r.out, again.out)
	}
	s.passes(s.update("echo"))
	after := s.read("testdata/script/twice.txtar")
	wantPrefix := before[:bytes.Index(before, []byte("-- out/"))]
	if !bytes.HasPrefix(after, wantPrefix) {
		t.Errorf("the update changed the comment or the inputs:\n%s", after)
	}
	s.cat("testdata/script/twice.txtar")
	s.passes(s.normal("echo"))
	s.done()
}

func TestErrorFailsItsCase(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "error")
	s.fails(s.normal("echo"), "echo returned an error, but the case states no property echo rejects:\n", `A1: unexpected "-"`, "out/echo/values.txt differs", "out/echo/error.txt has no golden.")
	s.fails(s.update("echo"), "echo returned an error, but the case states no property echo rejects:\n")
	s.cat("testdata/script/negative.txtar")
	if !bytes.Contains(s.read("testdata/script/negative.txtar"), []byte("-- out/echo/error.txt --\nA1: unexpected \"-\"\n")) {
		t.Error("the update didn't write error.txt with the error's text and a final newline")
	}
	s.fails(s.normal("echo"), "echo returned an error, but the case states no property echo rejects:\n")
	s.done()
}

func TestPanicFailsItsCase(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "panic")
	r := s.normal("echo")
	s.fails(r, "--- FAIL: TestCorpus/script/boom/echo", "echo panicked: index out of range", "--- PASS: TestCorpus/script/calm/echo")
	s.fails(s.update("echo"), "echo panicked: index out of range")
	s.cat("testdata/script/boom.txtar")
	got := s.read("testdata/script/boom.txtar")
	if !bytes.Contains(got, []byte("-- out/echo/error.txt --\nindex out of range\n")) || bytes.Contains(got, []byte("goroutine")) {
		t.Error("error.txt should hold the panic value and no stack")
	}
	again := newSandbox(t, "panic")
	again.update("echo")
	if a, b := got, again.read("testdata/script/boom.txtar"); !bytes.Equal(a, b) {
		t.Errorf("two processes wrote different archives:\n%s\nand:\n%s", a, b)
	}
	s.done()
}

func TestFrameworkNamesFail(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "framework-names")
	before := s.read("testdata/script/names.txtar")
	r := s.normal("echo")
	for _, name := range []string{"error.txt", "inputs", "outcome", "stderr.txt", "stdout.txt"} {
		s.contains(r, "The run returned an output at "+name+", a name the framework owns under out/echo/.")
	}
	// No update writes the cell, so no file of it is one to write.
	s.lacks(r, "has no golden")
	s.fails(s.update("echo"))
	s.same("testdata/script/names.txtar", before)
	s.done()
}

// TestUnwritableCell: a cell whose outputs an update won't write, for any
// of the reasons the run gives, fails on that reason first. A file of it
// with no golden isn't asked for, and one that differs from its golden
// says the update writes nothing until the problems above are fixed. An
// orphan is reported as in any other cell.
func TestUnwritableCell(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "basic")
	s.remove("testdata/script/twice.txtar", "leave only the unwritable cells")
	causes := []struct{ name, comment, input, want string }{
		{"framework", "The run returns an output at a name the framework owns.", "-- in/do/out/outcome --\nx\n", "The run returned an output at outcome, a name the framework owns under out/echo/."},
		{"doubled", "The run returns two outputs at one path.", "-- in/do/twice/b.txt --\nx\n", "The run returned two outputs at b.txt."},
		{"up", "The run returns an output outside its cell's directory.", "-- in/do/up/b.txt --\nx\n", `The run returned an output at "../b.txt", which is not a valid slash path under out/echo/.`},
		{"render", "RenderError panics on the run's error.", "-- in/do/error --\nboom\n-- in/render/panic --\nrender boom\n", "The project's RenderError panicked on echo's error: render boom"},
		{"render-nonl", "RenderError gives the run's error with no final newline.", "-- in/do/error --\nboom\n-- in/render/nonl --\n", `The project's RenderError returned "rendered: boom" for echo's error, which doesn't end with a newline.`},
		{"conflict", "The run returns c.txt, and c.txt/d below it.", "-- in/do/extranl/c.txt/d --\nx\n-- in/do/out/c.txt --\nx\n", "out/echo/c.txt can't be held in an archive: the section is also the directory of out/echo/c.txt/d."},
		{"undeclared", "The run returns an output no declaration matches.", "-- in/do/out/notes.txt --\nx\n", "out/echo/notes.txt is not declared."},
		{"matched", "The run returns an output two declarations match.", "-- in/do/out/a.txt --\nx\n", "out/echo/a.txt matches two of echo's output declarations, ?.txt and a.txt."},
		{"unheld", "The run returns an output with no final newline.", "-- in/do/nonl/b.txt --\nx\n", "out/echo/b.txt can't be held in an archive: it has no final newline, which an archive adds."},
	}
	var want []string
	for _, c := range causes {
		s.write("testdata/script/"+c.name+".txtar", []byte(c.comment+"\n"+c.input+"-- in/do/out/values.txt --\nA1 6\n"), "a cell the run can't write, which also gives values.txt")
		want = append(want, "script/"+c.name+": "+c.comment+"\n    echo: Does what the case's do/ tree says.")
		want = append(want, c.want)
	}
	committed := map[string][]byte{}
	for _, c := range causes {
		committed[c.name] = s.read("testdata/script/" + c.name + ".txtar")
	}
	// The update writes the registry, and nothing for these cells.
	s.fails(s.update("unwritable"), want...)
	for _, c := range causes {
		s.same("testdata/script/"+c.name+".txtar", committed[c.name])
	}
	r := s.normal("unwritable")
	s.fails(r, want...)
	s.lacks(r, "has no golden")
	for _, c := range causes {
		s.edit("testdata/script/"+c.name+".txtar", "commit a values.txt the run no longer gives", set("out/echo/values.txt", "A1 7\n"))
		committed[c.name] = s.read("testdata/script/" + c.name + ".txtar")
	}
	differs := "out/echo/values.txt differs (-golden +got):\n- A1 7\n+ A1 6\nAn update writes nothing for this cell until the problems above are fixed."
	r = s.normal("unwritable")
	s.fails(r, want...)
	if n := strings.Count(flat(r.out), flat(differs)); n != len(causes) {
		t.Errorf("%d cells say their values.txt differs, and the update writes nothing for them, want %d:\n%s", n, len(causes), r.out)
	}
	s.lacks(r, "has no golden", "If the change is intended")
	s.fails(s.update("unwritable"), want...)
	for _, c := range causes {
		s.same("testdata/script/"+c.name+".txtar", committed[c.name])
	}
	// An orphan is still reported, in every run: no update removes a file,
	// so a person deletes it, whatever keeps the update from the cell.
	for _, c := range causes {
		s.edit("testdata/script/"+c.name+".txtar", "commit a file the run doesn't give", insert("out/echo/g.txt", "x\n", "out/echo/values.txt"))
		committed[c.name] = s.read("testdata/script/" + c.name + ".txtar")
	}
	orphan := "out/echo/g.txt is an output the run no longer produces."
	for _, update := range []bool{false, true} {
		r := s.run(".", "unwritable", update)
		s.fails(r, want...)
		if n := strings.Count(flat(r.out), orphan); n != len(causes) {
			t.Errorf("%d cells say g.txt is an output the run no longer produces, want %d:\n%s", n, len(causes), r.out)
		}
	}
	for _, c := range causes {
		s.same("testdata/script/"+c.name+".txtar", committed[c.name])
	}
	s.done()
}

func TestFailedTestWritesNothing(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "fatal")
	before := s.read("testdata/script/fatal.txtar")
	s.fails(s.normal("echo"), "the fixture's database won't start")
	s.fails(s.update("echo"), "The run failed its test, through its testing.TB. The update writes nothing for this cell.")
	s.same("testdata/script/fatal.txtar", before)
	s.done()
}

func TestNothingScrubsOutput(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "tempdir")
	s.passes(s.update("echo"))
	s.fails(s.normal("echo"), "out/echo/dir.txt differs")
	// The transcript can't hold the temp paths, which change on every run.
	s.transcript.Reset()
	s.note("an update, then a normal run that fails on dir.txt")
	s.done()
}

func TestOrphans(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "orphans")
	gone, quiet := s.read("testdata/script/gone.txtar"), s.read("testdata/script/quiet.txtar")
	want := []string{
		"out/echo/old.txt is an output the run no longer produces.",
		"out/echo/error.txt records an error, but the run returned none.",
		"out/echo/stdout.txt records output, but the run wrote nothing to stdout.",
		"out/echo/stderr.txt records output, but the run wrote nothing to stderr.",
		"If not, delete the section by hand; FIXTURE_UPDATE_GOLDEN=1 go test ./... leaves it in place.",
	}
	// A section under a column nothing registers is the root-package
	// check's to report, in a normal run.
	unclaimed := "testdata/script/gone.txtar: no registry claims out/other/values.txt: no column named other runs over the corpus script.\nIf a column should write them, register the column. If not, delete them by hand; FIXTURE_UPDATE_GOLDEN=1 go test ./... leaves them in place."
	s.fails(s.normal("echo"), append(want, unclaimed)...)
	s.fails(s.update("echo"), want...)
	s.same("testdata/script/gone.txtar", gone)
	s.same("testdata/script/quiet.txtar", quiet)
	s.done()
}

func TestSectionOrder(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "order")
	inputs := func(rel string) []string {
		var ins []string
		for _, line := range strings.Split(string(s.read(rel)), "\n") {
			if strings.HasPrefix(line, "-- in/") {
				ins = append(ins, line)
			}
		}
		return ins
	}
	betweenIn := inputs("testdata/script/between.txtar")
	r := s.normal("echo")
	s.fails(r,
		"testdata/script/unsorted.txtar has update-written sections out of their place.\n",
		"testdata/script/between.txtar has update-written sections out of their place.\n",
		"Run FIXTURE_UPDATE_GOLDEN=1 go test ./... to put them in place.")
	s.passes(s.update("echo"))
	s.cat("testdata/script/unsorted.txtar")
	s.cat("testdata/script/between.txtar")
	if got := inputs("testdata/script/between.txtar"); strings.Join(got, "\n") != strings.Join(betweenIn, "\n") {
		t.Errorf("the update moved inputs: %q, then %q", betweenIn, got)
	}
	unsorted, between := s.read("testdata/script/unsorted.txtar"), s.read("testdata/script/between.txtar")
	s.passes(s.update("echo"))
	s.same("testdata/script/unsorted.txtar", unsorted)
	s.same("testdata/script/between.txtar", between)
	s.passes(s.normal("echo"))
	s.done()
}

func TestRegeneration(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "regen")
	s.passes(s.update("echo"))
	s.cat("testdata/script/stale.txtar")
	if a, b := s.read("testdata/script/stale.txtar"), s.read("testdata/script/stripped.txtar"); !bytes.Equal(a, b) {
		t.Errorf("updating stale goldens and updating none gave two archives:\n%s\nand:\n%s", a, b)
	}
	s.done()
}

func TestDescriptions(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "descriptions")
	want := []string{
		"testdata/script/empty.txtar: the archive has no description",
		"testdata/script/blank.txtar: the archive has no description",
		`testdata/script/key-value.txtar: line 2 of the comment is a key: value line, and the comment is prose: "cells: 1"`,
	}
	s.fails(s.normal("echo"), want...)
	// An update writes the cells, and the root-package check, which reports
	// the descriptions, runs in the normal run after it.
	s.passes(s.update("echo"))
	for _, name := range []string{"empty", "blank", "key-value"} {
		if !bytes.Contains(s.read("testdata/script/"+name+".txtar"), []byte("-- out/echo/values.txt --\nA1 6\n")) {
			t.Errorf("the update didn't write %s's outputs", name)
		}
	}
	s.done()
}

func TestSuiteWithoutDescription(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "missing-golden")
	s.fails(s.update("echo-nodesc"), "the suite echo has no description.")
	s.cat("testdata/script/twice.txtar")
	s.fails(s.normal("echo-nodesc"), "the suite echo has no description.")
	s.done()
}

func TestProblemsOfForm(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "form")
	names := []string{"duplicate", "paths", "products", "shape", "typo"}
	before := map[string][]byte{}
	for _, n := range names {
		before[n] = s.read("testdata/script/" + n + ".txtar")
	}
	want := []string{
		"testdata/script/typo.txtar has problems of form, so none of its cells run, and no update writes it:\n",
		"ni/do/out/values.txt: ni is not one of the framework's top-level names",
		"via/A1: a product belongs under its lift, as via/<lift>/<tree>/<file>",
		"via/import/A1: a product's file belongs in a tree, as via/import/<tree>/<file>",
		"via/import/params/x: the tree name params is reserved in every input view, a product's included",
		"in/do/out/values.txt: the section appears twice",
		"in/../x: the name is not a valid slash path",
		"in//x: the name is not a valid slash path",
		"in/A1: an input belongs in a tree",
		"out/values.txt: an output belongs under its column",
		"in/do/out/values.txt: the section is also the directory of in/do/out/values.txt/x",
	}
	r := s.normal("echo")
	s.fails(r, want...)
	s.lacks(r, "=== RUN   TestCorpus/script/typo/echo", "TestCorpus/script/typo/echo")
	// The package runs no cell of an archive with a problem of form, and
	// leaves the report to the root-package check.
	u := s.update("echo")
	s.passes(u)
	s.contains(u, "testdata/script/typo.txtar has problems of form, so none of its cells run, and no update writes it. The root-package check reports them.")
	for _, n := range names {
		s.same("testdata/script/"+n+".txtar", before[n])
	}
	s.done()
}

func TestOutputsAnArchiveCantHold(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "cant-hold")
	before := map[string][]byte{"no-newline": s.read("testdata/script/no-newline.txtar"), "marker": s.read("testdata/script/marker.txtar")}
	want := []string{
		"out/echo/values.txt can't be held in an archive: it has no final newline, which an archive adds.",
		"out/echo/values.txt can't be held in an archive: its line 2 is shaped like a section marker",
	}
	r := s.normal("echo")
	s.fails(r, want...)
	s.lacks(r, "has no golden")
	s.fails(s.update("echo"), want...)
	s.same("testdata/script/no-newline.txtar", before["no-newline"])
	s.same("testdata/script/marker.txtar", before["marker"])
	s.done()
}

// TestOutputsFileAndDirectory: a cell's files, the framework's included,
// can't hold a file that is also the directory of another, which no archive
// can hold.
func TestOutputsFileAndDirectory(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "file-dir")
	before := map[string][]byte{"stream": s.read("testdata/script/stream.txtar"), "outputs": s.read("testdata/script/outputs.txtar")}
	want := []string{
		"out/echo/stdout.txt can't be held in an archive: the section is also the directory of out/echo/stdout.txt/a.\nFix the code. An update writes nothing for this cell.",
		"out/echo/a.txt can't be held in an archive: the section is also the directory of out/echo/a.txt/b.\nFix the code. An update writes nothing for this cell.",
	}
	r := s.normal("nested")
	s.fails(r, want...)
	s.lacks(r, "has no golden")
	s.fails(s.update("nested"), want...)
	s.same("testdata/script/stream.txtar", before["stream"])
	s.same("testdata/script/outputs.txtar", before["outputs"])
	s.fails(s.normal("nested"), want...)
	s.done()
}

func TestRunCantSkip(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "skip")
	s.fails(s.normal("echo"), "--- FAIL: TestCorpus/script/skip/echo", "The run skipped its cell.")
	s.done()
}

func TestLeftoverTempFile(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "temp-left")
	want := "testdata/script/.hypercase-tmp-twice.txtar-1234 is a temp file left by a write that was cut short.\n"
	s.fails(s.normal("echo"), want, "--- PASS: TestCorpus/script/twice/echo")
	// Other packages' writes are in flight during an update, so only a
	// normal run's root-package check reports a temp file beside an archive.
	s.passes(s.update("echo"))
	s.done()
}

func TestArchiveNameRule(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "bad-archive-name")
	s.fails(s.normal("echo"), `testdata/script/Twice.txtar: the archive's name, "Twice", breaks the naming rule`)
	s.done()
}

func TestRunSelectsOneCell(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "panic")
	r := s.normal("echo", "-test.run=TestCorpus/script/calm/echo")
	s.passes(r)
	s.lacks(r, "boom")
	s.done()
}

func TestUpdateVarValue(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "basic")
	r := s.runEnv("", "echo", []string{fixtureUpdate + "=yes"})
	s.fails(r, `FIXTURE_UPDATE_GOLDEN is set to "yes".`, "Set it to 1 to run an update, or unset it.")
	s.done()
}

func TestRegistrationRefusals(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "basic")
	for _, tt := range []struct{ reg, want string }{
		{"bad-name", `panic: hypercase: the suite name "Echo" breaks the naming rule`},
		{"two-names", "panic: hypercase: two columns are named echo: the suite echo, over hypercase_test.Script, and the suite echo, over hypercase_test.Script. Column names are unique; rename one."},
		{"no-run", "panic: hypercase: the suite echo has no Run."},
		{"two-projects", "panic: hypercase: Register was given a decl.Project a second time."},
		{"unknown", "panic: hypercase: Register was given a int, which is nothing hypercase registers."},
	} {
		r := s.normal(tt.reg)
		s.fails(r, tt.want)
		s.lacks(r, "--- ")
		// The refusal doesn't depend on -run.
		s.fails(s.normal(tt.reg, "-test.run=^$"), tt.want)
	}
	s.fails(s.normal("late"), "Register was called after a test started.")
	s.fails(s.normal("no-project"), "the package registered no decl.Project.")
	s.done()
}

func TestFixturesAreNeverCorpora(t *testing.T) {
	// A fixture fails on purpose, so it sits under testdata/fixtures/<fixture>/,
	// never directly in a directory under testdata/, where a run reads archives.
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join("testdata", e.Name(), "*.txtar"))
		for _, m := range matches {
			t.Errorf("%s is an archive directly in a directory under testdata/; move it under testdata/fixtures/<fixture>/", m)
		}
	}
}
