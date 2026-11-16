package hypercase_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/registry"
)

// Step 5: the root-package check, over the fixture shared.

func TestRootCheckRemovedSuite(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	before := s.read(twice)
	s.passes(s.run("b", "pkg-b-gone", true))
	want := "testdata/script/twice.txtar: no registry claims out/shout/outcome and out/shout/values.txt: no column named shout runs over the corpus script.\nIf a column should write them, register the column. If not, run hypercase strip -column shout at the module root, then FIXTURE_UPDATE_GOLDEN=1 go test ./... until it changes nothing."
	s.fails(s.normal("pkg-a"), want)
	s.passes(s.update("pkg-a"))
	s.same(twice, before)
	s.fails(s.normal("pkg-a"), want)
	// The command the message names, then the update, leave the check
	// passing.
	s.passes(s.strip("-column", "shout"))
	for _, r := range s.settle(childRun{"", "pkg-a", true}, childRun{"b", "pkg-b-gone", true}) {
		s.passes(r)
	}
	s.hasSections(twice, nil, "out/shout/")
	s.passes(s.normal("pkg-a"))
	s.done()
}

// TestRootCheckRemovedSuiteRoot: under a corpus root other than the
// module root, the message names it with -root, as the module root sees
// it, and the command it names, run there, strips that corpus root alone.
func TestRootCheckRemovedSuiteRoot(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	corp := "corp/testdata/script/twice.txtar"
	s.write(corp, s.read(twice), "a corpus under the root corp, whose package registers echo alone")
	s.passes(s.update("corp-a"))
	s.fails(s.normal("corp-a"), corp+": no registry claims out/shout/outcome and out/shout/values.txt: no column named shout runs over the corpus script.\nIf a column should write them, register the column. If not, run hypercase strip -root corp -column shout at the module root, then FIXTURE_UPDATE_GOLDEN=1 go test ./... until it changes nothing.")
	before := s.read(twice)
	s.passes(s.strip("-root", "corp", "-column", "shout"))
	for _, r := range s.settle(childRun{"", "corp-a", true}) {
		s.passes(r)
	}
	s.hasSections(corp, nil, "out/shout/")
	s.same(twice, before)
	s.passes(s.normal("corp-a"))
	s.done()
}

// TestRootCheckColumnTwice: a suite two packages own is one rename or
// removal from a unique name, and one three own is two.
func TestRootCheckColumnTwice(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	s.run("b", "pkg-b-echo", true)
	r := s.normal("pkg-a")
	s.fails(r, "the column echo is owned by . and ./b, each as a suite. Column names are unique per corpus root, and one package owns each column.\nRename one of the suites, or remove one.")
	s.lacks(r, "hypercase.Use")

	s.write("c/corpus_test.go", []byte(packageTestFile), "a package c over the corpus")
	s.run("c", "pkg-b-echo", true)
	r = s.normal("pkg-a")
	s.fails(r, "the column echo is owned by ., ./b and ./c, each as a suite. Column names are unique per corpus root, and one package owns each column.\nRename or remove all but one of the suites.")
	s.lacks(r, "hypercase.Use")
	s.done()
}

func TestRootCheckPropertyEntries(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	s.passes(s.update("pkg-a-circular"))
	s.passes(s.run("b", "pkg-b-circular", true))
	// One property declared once and registered by both is one property.
	s.passes(s.normal("pkg-a-circular"))
	s.passes(s.run("b", "pkg-b-loop", true))
	s.fails(s.normal("pkg-a-circular"), "the property circular has two different entries:\n  .: \"Some cell's formula refers back to itself, directly or through other cells.\"\n  ./b: \"A loop.\"\nDeclare the property once, in a package the others import, and register that declaration.")
	s.done()
}

// TestStatementsOfAnotherPackage: a package checks its cells against the
// properties a case states and the gaps lines naming its own columns, and
// leaves the rest to the package that registers them.
func TestStatementsOfAnotherPackage(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	s.write("testdata/script/loop.txtar", []byte("A1 refers to itself, which only b's shout rejects.\n-- in/do/out/values.txt --\nA1 0\n-- in/shout/error --\nA1 -> A1: cycle\n-- properties --\ncircular\n-- out/echo/outcome --\naccepted\n-- out/echo/values.txt --\nA1 0\n-- out/shout/error.txt --\nA1 -> A1: cycle\n-- out/shout/outcome --\nrejected\n"), "a case that states a property only b registers")
	s.write("testdata/script/gapped.txtar", []byte("shout prints A1 wrongly, a known gap in b's cell.\n-- in/do/out/values.txt --\nA1 6\n-- in/shout/out/values.txt --\nA1 SEVEN\n-- gaps --\nshout A1 is SIX. Waits on sheets#15.\n-- out/echo/outcome --\naccepted\n-- out/echo/values.txt --\nA1 6\n-- out/shout/outcome --\naccepted\n-- out/shout/values.txt --\nA1 SIX\n"), "a gaps line naming b's column, whose cell now differs")
	// b's update writes loop's cell, and fails on its frozen one, as committed.
	s.fails(s.run("b", "pkg-b-circular", true), "The cell is frozen by its known gap")
	s.cat("testdata/script/gapped.txtar")
	s.passes(s.normal("pkg-a"))
	s.fails(s.run("b", "pkg-b-circular", false), "--- PASS: TestCorpus/script/loop/shout", "--- FAIL: TestCorpus/script/gapped/shout", "The cell is frozen by its known gap")
	s.done()
}

// TestRootCheckArchives: the problems of an archive as a whole fail the
// root-package check alone, and no package's cells.
func TestRootCheckArchives(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	data := string(s.read(twice))
	s.write("testdata/script/nameless.txtar", []byte(data[strings.Index(data, "-- "):]), "an archive with no description")
	s.write("testdata/script/loopy.txtar", []byte("A case that states a property nothing declares.\n-- in/do/out/values.txt --\nA1 6\n-- properties --\nloopy\n-- gaps --\nformat A1 prints as =6. Waits on sheets#8.\n-- out/echo/outcome --\naccepted\n-- out/echo/values.txt --\nA1 6\n-- out/shout/outcome --\naccepted\n"), "a case with a property and a gaps line nothing declares")
	s.write("testdata/script/typo.txtar", []byte("A section outside the closed set.\n-- ni/do/out/values.txt --\nA1 6\n"), "an archive with a problem of form")
	s.write("testdata/script/old/x.txtar", []byte("An archive in a directory below a corpus's.\n"), "an archive in no corpus's directory")
	s.write("testdata/renamed/x.txtar", []byte("An archive of a corpus whose directory was renamed in Go.\n"), "an archive in a directory no registry declares")
	s.write("testdata/x.txtar", []byte("An archive directly in testdata/.\n"), "an archive directly in testdata/")
	typo := s.read("testdata/script/typo.txtar")
	b := s.run("b", "pkg-b", false)
	s.passes(b)
	s.contains(b, "testdata/script/typo.txtar has problems of form, so none of its cells run, and no update writes it. The root-package check reports them.")
	s.fails(s.normal("pkg-a"),
		"testdata/script/nameless.txtar: the archive has no description",
		"testdata/script/loopy.txtar: properties states loopy, and no registry under the module-root corpus lists a property of that name.",
		"testdata/script/loopy.txtar: gaps, line 1, names format, and no registry has a column of that name over the corpus script.",
		"testdata/script/typo.txtar has problems of form, so none of its cells run, and no update writes it:\n  ni/do/out/values.txt: ni is not one of the framework's top-level names",
		"testdata/script/old/ holds archives, and no registry declares the directory: no corpus type has it as its Dir, and no suite names it as its Corpus.",
		"testdata/renamed/ holds archives, and no registry declares the directory: no corpus type has it as its Dir, and no suite names it as its Corpus.\nDeclare a decl.Type with that Dir, or move or delete the archives.",
		"testdata/ holds archives directly, and an archive belongs in a corpus's directory, testdata/<corpus>/.\nMove the archives into one.",
		"--- PASS: TestCorpus/script/nameless/echo",
		"--- PASS: TestCorpus/script/loopy/echo")
	s.passes(s.run("b", "pkg-b", true))
	s.same("testdata/script/typo.txtar", typo)
	s.done()
}

// TestRootCheckDeclaredDirectory: a directory a registry declares, and
// that doesn't exist yet, is fine.
func TestRootCheckDeclaredDirectory(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	s.run("b", "pkg-b-later", true)
	s.remove("testdata/script/twice.txtar", "")
	s.write(twice, []byte("A1 is twice B1, and only echo prints it.\n-- in/do/out/values.txt --\nA1 6\n-- out/echo/outcome --\naccepted\n-- out/echo/values.txt --\nA1 6\n"), "")
	r := s.normal("pkg-a")
	s.passes(r)
	s.done()
}

func TestRootCheckRegistries(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	s.remove("b/corpus_test.go", "b's tests are deleted, and its registry left behind")
	untested := "b/testdata/hypercase.json is a registry, and b holds no _test.go file, so no test compares it, and it still claims sections.\nDelete it, or restore the package's tests."
	s.fails(s.normal("pkg-a"), untested)
	// Go ignores a file whose name starts with _ or ., so neither counts.
	s.write("b/_corpus_test.go", []byte(packageTestFile), "a test file Go ignores, by its leading _")
	s.write("b/.corpus_test.go", []byte(packageTestFile), "a test file Go ignores, by its leading .")
	s.fails(s.normal("pkg-a"), untested)
	s.remove("b/_corpus_test.go", "remove the file Go ignores")
	s.remove("b/.corpus_test.go", "remove the file Go ignores")
	// The check reads no build constraints, so a test file they exclude
	// still counts.
	s.write("b/x_test.go", []byte("//go:build ignore\n\npackage fixture\n"), "a test file a build constraint excludes")
	s.passes(s.normal("pkg-a"))
	s.remove("b/x_test.go", "remove the excluded file")
	s.write("b/corpus_test.go", []byte(packageTestFile), "restore b's tests")
	s.write("b/testdata/hypercase.json", []byte("{\n  \"format\": 9\n}\n"), "a registry from a later hypercase")
	s.fails(s.normal("pkg-a"), fmt.Sprintf("b/testdata/hypercase.json can't be read: it holds format 9, and this hypercase reads format %d.\nRun FIXTURE_UPDATE_GOLDEN=1 go test ./... to write it.", registry.Format))
	s.done()
}

// TestRootCheckOtherRoots: a registry of another corpus root is that
// root's check's to read, whatever its format: an older one, as an earlier
// step's example commits, or a newer one. This root's update and its
// normal run agree on it, and pass.
func TestRootCheckOtherRoots(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	s.write("sib/corpus_test.go", []byte(packageTestFile), "a package over the corpus root sib")
	for _, tt := range []struct{ why, data string }{
		{"its registry, in format 1", `{"format": 1, "updateVar": "FIXTURE_UPDATE_GOLDEN", "corpusRoot": "sib", "rootPackage": true, "properties": [],` +
			` "suites": [{"name": "calc", "description": "Runs calc.", "corpus": "sheet", "outputs": [], "rejects": []}]}` + "\n"},
		{"its registry, from a later hypercase", fmt.Sprintf(`{"format": %d, "corpusRoot": "sib", "a field of a later hypercase": {}}`+"\n", registry.Format+1)},
	} {
		s.write("sib/testdata/hypercase.json", []byte(tt.data), tt.why)
		s.passes(s.update("pkg-a"))
		s.passes(s.normal("pkg-a"))
		s.passes(s.run("b", "pkg-b", false))
	}
	s.done()
}

// TestRootCheckRequired: every package's normal run over a corpus root
// fails while no registry records a root-package check for it.
func TestRootCheckRequired(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	want := "no registry records a root-package check for the module-root corpus, so nothing reports what no column there owns.\nRegister hypercase.RootPackage() in one package over it, then run FIXTURE_UPDATE_GOLDEN=1 go test ./... to record it in the package's registry."
	s.passes(s.update("pkg-a-noroot"))
	s.fails(s.normal("pkg-a-noroot"), want)
	s.fails(s.run("b", "pkg-b", false), want)
	s.passes(s.run("b", "pkg-b", true))

	// A package over another corpus root needs that root's check.
	s.write("other/testdata/script/twice.txtar", s.read(twice), "a corpus under the root other")
	s.passes(s.update("pkg-a"))
	s.passes(s.run("b", "pkg-b-other", true))
	s.fails(s.run("b", "pkg-b-other", false), "no registry records a root-package check for the corpus root other")
	// a's root has its check. What it reports is b's sections, which no column over it claims now.
	r := s.normal("pkg-a")
	s.fails(r, "no registry claims out/shout/outcome and out/shout/values.txt")
	s.lacks(r, "no registry records a root-package check")
	s.done()
}

// TestRootCheckStale: a registry that records the root-package check, in a
// package with no tests, records a check that no test runs, so every
// other package's normal run fails, naming it.
func TestRootCheckStale(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	s.passes(s.normal("pkg-a"))
	s.passes(s.run("b", "pkg-b", false))
	s.remove("corpus_test.go", "the root package's tests are deleted, and its registry left behind")
	s.fails(s.run("b", "pkg-b", false), "testdata/hypercase.json records the root-package check for the module-root corpus, and . holds no _test.go file, so no test runs the check, and nothing reports what no column there owns.\nRestore the package's tests, or register hypercase.RootPackage() in another package over it, then run FIXTURE_UPDATE_GOLDEN=1 go test ./... to record it in that package's registry.")
	s.done()
}

func TestTwoRootPackages(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	s.write("corp/testdata/script/twice.txtar", s.read(twice), "a corpus under the root corp")
	s.passes(s.update("corp-a"))
	s.passes(s.run("b", "corp-b", true))
	s.fails(s.normal("corp-a"), ". and ./b register the root-package check for the corpus root corp. It runs once per corpus root.\nRegister it in one package.")
	s.done()
}
