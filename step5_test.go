package hypercase_test

import (
	"bytes"
	"strings"
	"testing"
)

// Step 5: suites and the matrix.

func TestTwoColumns(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "two-columns")
	s.passes(s.normal("echo-shout"))
	// A change to what one column writes fails only its cell.
	twice := string(s.read("testdata/script/twice.txtar"))
	s.write("testdata/script/twice.txtar", []byte(strings.Replace(twice, "-- in/shout/out/values.txt --\nA1 SIX\n", "-- in/shout/out/values.txt --\nA1 SEVEN\n", 1)), "shout now prints SEVEN")
	s.fails(s.normal("echo-shout"), "--- PASS: TestCorpus/script/twice/echo", "--- FAIL: TestCorpus/script/twice/shout", "out/shout/values.txt differs")
	s.done()
}

func TestGapFreezesOneCell(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "two-columns")
	frozen := string(s.read("testdata/script/frozen.txtar"))
	frozen = strings.Replace(frozen, "-- in/do/out/values.txt --\nA1 6\n", "-- in/do/out/values.txt --\nA1 7\n", 1)
	frozen = strings.Replace(frozen, "-- in/shout/out/values.txt --\nA1 SIX!\n", "-- in/shout/out/values.txt --\nA1 SEVEN!\n", 1)
	s.write("testdata/script/frozen.txtar", []byte(frozen), "both columns now print something else")
	want := []string{
		"--- FAIL: TestCorpus/script/frozen/echo",
		"out/echo/values.txt differs (-golden +got):",
		"--- FAIL: TestCorpus/script/frozen/shout",
		`The cell is frozen by its known gap, gaps line 1: "shout A1 is SIX, not SIX!. Waits on sheets#11."`,
	}
	s.fails(s.normal("echo-shout"), want...)
	// The update writes echo's cell, and leaves shout's as committed.
	s.fails(s.update("echo-shout"), `The cell is frozen by its known gap, gaps line 1: "shout A1 is SIX, not SIX!. Waits on sheets#11."`)
	s.cat("testdata/script/frozen.txtar")
	got := s.read("testdata/script/frozen.txtar")
	if !bytes.Contains(got, []byte("-- out/echo/values.txt --\nA1 7\n")) || !bytes.Contains(got, []byte("-- out/shout/values.txt --\nA1 SIX!\n")) {
		t.Error("the update should write echo's cell and leave shout's frozen one as committed")
	}
	r := s.normal("echo-shout")
	s.fails(r, "--- PASS: TestCorpus/script/frozen/echo", "--- FAIL: TestCorpus/script/frozen/shout")
	s.done()
}

func TestCaseNames(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "two-corpora")
	// Two archives with one name in two corpora are two cases.
	r := s.normal("two-corpora")
	s.passes(r)
	s.contains(r, "--- PASS: TestCorpus/script/twice/echo", "--- PASS: TestCorpus/other/twice/shout")
	s.lacks(r, "TestCorpus/script/twice/shout", "TestCorpus/other/twice/echo")
	s.done()
}

func TestProbes(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "probes")
	stale, no := s.read("testdata/script/stale.txtar"), s.read("testdata/script/no.txtar")
	want := []string{
		"--- PASS: TestCorpus/script/yes/echo",
		// A skipped cell isn't checked for an expected error: no states
		// circular, which echo rejects, and echo didn't run to err.
		"--- SKIP: TestCorpus/script/no/echo",
		"echo's probe says the case isn't for it: already serves 4",
		"--- FAIL: TestCorpus/script/noreason/echo",
		"echo's probe said the case isn't for it, and gave no reason.\nFix the probe: when it says no, it says why.",
		"out/echo/values.txt is recorded, and echo's probe says the case isn't for it: no formulas\nIf the suite should run on the case, fix the probe. If not, delete the section by hand; FIXTURE_UPDATE_GOLDEN=1 go test ./... leaves it in place.",
		"gaps, line 1, marks the cell, and echo's probe says the case isn't for it: no formulas\nRemove the line, or fix the probe.",
		"echo's probe panicked: probe overflow\nFix the probe. A panic is never an expected error.",
	}
	s.fails(s.normal("probe"), want...)
	s.fails(s.update("probe"), want[1:]...)
	s.same("testdata/script/stale.txtar", stale)
	s.same("testdata/script/no.txtar", no)
	// A panic in a probe is recorded like one in a run, as error.txt.
	s.cat("testdata/script/panics.txtar")
	panics := s.read("testdata/script/panics.txtar")
	if !bytes.Contains(panics, []byte("-- out/echo/error.txt --\nprobe overflow\n")) {
		t.Error("the update didn't write the probe's panic as error.txt")
	}
	s.write("testdata/script/panics.txtar", bytes.Replace(panics, []byte("-- out/echo/"), []byte("-- gaps --\necho The probe reads every cell. Waits on sheets#13.\n-- out/echo/"), 1), "mark the cell as a known gap")
	s.contains(s.normal("probe"), "--- PASS: TestCorpus/script/panics/echo")
	s.done()
}

// TestProbeSkipsTheCheck: without the probe, the case the probe skipped
// fails as a cell that didn't err.
func TestProbeSkipsTheCheck(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "probes")
	s.fails(s.normal("probe-none", "-test.run=TestCorpus/script/no$/"), "--- FAIL: TestCorpus/script/no/echo", "echo returned no error, but the case states circular, which echo rejects.")
	s.done()
}

func TestAppliesToNoCase(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "no-case")
	want := "the suite echo applies to no case: its probe says no to every case of script.\nA suite that applies to no case checks nothing. Add a case it applies to, or fix the probe."
	s.fails(s.normal("probe"), "--- SKIP: TestCorpus/script/a/echo", "--- SKIP: TestCorpus/script/b/echo", want)
	s.fails(s.update("probe"), want)
	// With -run leaving a cell out, the verdict isn't given.
	r := s.normal("probe", "-test.run=TestCorpus/script/a/")
	s.passes(r)
	s.lacks(r, "applies to no case")

	empty := newSandbox(t, "empty-corpus")
	empty.fails(empty.normal("echo"), "the suite echo applies to no case: its corpus, testdata/script/, holds no archive whose cells run.\nA suite that applies to no case checks nothing. Add a case it applies to.")
	s.transcript.WriteString(empty.transcript.String())
	s.done()
}

func TestDeclaredOutputs(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "outputs")
	before := map[string][]byte{}
	for _, n := range []string{"undeclared", "streams", "matched-twice"} {
		before[n] = s.read("testdata/script/" + n + ".txtar")
	}
	want := []string{
		"--- PASS: TestCorpus/script/declared/echo",
		"out/echo/notes.txt is not declared.\nDeclare it in echo's Outputs, with a description, or fix the code. An update writes nothing for this cell.",
		"out/echo/stdout.txt, which holds what the run wrote to stdout, is not declared.",
		"out/echo/stderr.txt, which holds what the run wrote to stderr, is not declared.",
		"out/echo/error.txt, which holds the error the run returned, is not declared.",
		"out/echo/a.txt matches two of echo's output declarations, ?.txt and a.txt.\nChange the patterns so that one declaration matches each file. An update writes nothing for this cell.",
	}
	r := s.normal("narrow")
	s.fails(r, want...)
	// An undeclared file isn't compared, so it has no golden to miss.
	s.lacks(r, "has no golden")
	s.fails(s.update("narrow"), want[1:]...)
	for n, data := range before {
		s.same("testdata/script/"+n+".txtar", data)
	}
	s.done()
}

func TestOutputWithoutDescription(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "missing-golden")
	want := "the suite echo declares the output values.txt with no description.\nAdd one: it says what the files hold, in prose."
	s.fails(s.update("output-nodesc"), want)
	s.cat("testdata/script/twice.txtar")
	if !bytes.Contains(s.read("testdata/script/twice.txtar"), []byte("-- out/echo/values.txt --")) {
		t.Error("the update didn't write outputs")
	}
	s.fails(s.normal("output-nodesc"), want)
	s.done()
}

func TestSuiteRefusals(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "basic")
	for _, tt := range []struct{ reg, want string }{
		{"bad-pattern", `panic: hypercase: the suite echo declares an output with the pattern "values[.txt", which is not a pattern of slash paths under out/echo/ in the syntax of path.Match.`},
		{"bad-pattern-path", `panic: hypercase: the suite echo declares an output with the pattern "../values.txt", which is not a pattern`},
		{"corpus-set", `panic: hypercase: the suite echo names the Corpus "script". A suite runs over the archives whose corpus type reaches the type it consumes. Declare a decl.Type with Dir "script", make the suite consume it or a stage it reaches, and remove Corpus.`},
		{"bad-dir", `panic: hypercase: the corpus type hypercase_test.Script declares the directory "Script", which breaks the naming rule`},
		{"two-roots", `panic: hypercase: Register was given hypercase.Root a second time, with "b".`},
		{"bad-root", `panic: hypercase: hypercase.Root was given "/abs", which is not a slash path relative to the module root`},
		{"two-root-packages", "panic: hypercase: Register was given hypercase.RootPackage a second time."},
	} {
		r := s.normal(tt.reg)
		s.fails(r, tt.want)
		s.lacks(r, "--- ")
	}
	s.done()
}

func TestRegistry(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "basic")
	s.passes(s.normal("echo"))
	s.cat("testdata/hypercase.json")
	// A new suite changes what the package registers.
	s.fails(s.normal("echo-shout"), "testdata/hypercase.json differs from what the package registers (-committed +registered):", `+       "name": "shout",`, "Run FIXTURE_UPDATE_GOLDEN=1 go test ./... to write it, and read the diff before committing.")
	s.passes(s.update("echo-shout"))
	s.passes(s.normal("echo-shout"))
	full := s.read("testdata/hypercase.json")

	s.remove("testdata/hypercase.json", "the registry is lost")
	s.fails(s.normal("echo-shout"), "testdata/hypercase.json is missing. It records what the package registers, for tools and for the root-package check.\nRun FIXTURE_UPDATE_GOLDEN=1 go test ./... to write it.")
	// The registry holds what was registered, whichever cells -run selects.
	s.passes(s.update("echo-shout", "-test.run=TestCorpus/script/twice/shout"))
	s.same("testdata/hypercase.json", full)

	s.write("testdata/.hypercase-tmp-hypercase.json-99", []byte("{\n"), "a write of the registry was killed")
	want := "testdata/.hypercase-tmp-hypercase.json-99 is a temp file left by a write that was cut short.\nDelete it."
	s.fails(s.normal("echo-shout"), want)
	s.fails(s.update("echo-shout"), want)
	s.done()
}

// TestModuleRootCheckPlace: the module-root corpus's check runs in the
// module's root package.
func TestModuleRootCheckPlace(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	s.fails(s.run("b", "pkg-b-root", false), "the package registers the root-package check for the module-root corpus, and is not the module's root package")
	s.done()
}
