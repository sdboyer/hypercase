package hypercase_test

import (
	"strings"
	"testing"
)

// Step 6: lifts across packages, over the fixture shared-ladder. The root
// package, ., owns parse, resolve and evaluate, and ./b uses parse for its
// suite outline.

func TestUse(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared-ladder")
	s.passes(s.update("ladder-a"))
	s.passes(s.run("b", "ladder-b", true))
	s.cat("testdata/script/x.txtar")
	s.passes(s.normal("ladder-a"))
	s.passes(s.run("b", "ladder-b", false))

	// parse errs where no property says it should. Its owner fails; b
	// neither judges nor writes parse's cell, drops what parse writes to
	// stderr, and its cell past parse is blocked.
	s.write("testdata/script/bad.txtar", []byte("parse rejects the script, and nothing says it should.\n-- in/parse/error --\nA1: not a number\n-- in/parse/stderr --\nparse: a warning\n-- in/outline/out/values.txt --\noutline 2\n"), "a case parse wrongly rejects")
	b := s.run("b", "ladder-b", true)
	s.passes(b)
	s.contains(b, "script/bad: outline is blocked: the lift parse failed on the case, and testdata/script/bad.txtar, out/parse/error.txt, records its error.")
	s.hasSections("testdata/script/bad.txtar", map[string]string{"out/outline/outcome": "blocked parse\n"}, "out/")
	s.passes(s.run("b", "ladder-b", false))
	s.fails(s.normal("ladder-a"), "parse returned an error, but the case states no property parse rejects:")
	// The owner's update fails on the cell too, and writes it.
	s.fails(s.update("ladder-a"), "parse returned an error, but the case states no property parse rejects:")
	s.hasSections("testdata/script/bad.txtar", map[string]string{"out/parse/error.txt": "A1: not a number\n", "out/parse/stderr.txt": "parse: a warning\n"}, "out/outline/")
	s.done()
}

func TestRootCheckLifts(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared-ladder")
	s.passes(s.update("ladder-a"))
	s.passes(s.run("b", "ladder-b-owns", true))
	s.fails(s.normal("ladder-a"), "the column parse is owned by . and ./b, each as a lift. Column names are unique per corpus root, and one package owns each column.\nRename one of the lifts, or remove one. Of . and ./b, which own the lift, one that only reads its product removes its column by giving the lift to hypercase.Use in place of hypercase.Own.")

	s.passes(s.run("b", "ladder-b", true))
	s.passes(s.update("ladder-a-useonly"))
	s.fails(s.normal("ladder-a-useonly"), "the lift parse is used by . and ./b, and no package owns it, so its error is recorded nowhere.\nOwn the lift in one of them: give it to hypercase.Own in place of hypercase.Use.")

	// ., ./b and ./c own parse: one rename or removal leaves two owners.
	s.passes(s.update("ladder-a"))
	s.passes(s.run("b", "ladder-b-owns", true))
	s.write("c/corpus_test.go", []byte(packageTestFile), "a package c over the corpus")
	s.passes(s.run("c", "ladder-b-owns", true))
	s.fails(s.normal("ladder-a"), "the column parse is owned by ., ./b and ./c, each as a lift. Column names are unique per corpus root, and one package owns each column.\nRename or remove all but one of the lifts. Of ., ./b and ./c, which own the lift, one that only reads its product removes its column by giving the lift to hypercase.Use in place of hypercase.Own.")
	s.done()
}

// TestRootCheckUsedPath: a package that uses a lift reaches it from each
// corpus by the path the lift's owner records. Here . owns parse and
// resolve, and ./b uses resolve, and reaches it through parse2, a copy of
// parse it owns, so the cells of resolve that ./b's cells past it are
// built through are cells no package writes, and the root-package check
// fails. The paths check finds resolve's cell of the case built through
// parse, where ./b's evaluate cell is built through parse2. The failure
// states the two ways the conflict ends, and applying either makes every
// run pass: ./b reaches resolve by .'s path, using parse in place of
// parse2, or . by ./b's, using parse2 in place of parse, and the cells of
// the copy given up, which no package writes then, are deleted. Until
// they are, the root-package check fails on them, as sections no registry
// claims.
func TestRootCheckUsedPath(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, a, b, drop string
	}{
		{"b gives up parse2", "ladder-a-paths", "ladder-b-shared-producer", "out/parse2/"},
		{"a gives up parse", "ladder-a-uses-producer", "ladder-b-producer", "out/parse/"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newSandbox(t, "shared-ladder")
			s.passes(s.run("b", "ladder-b-producer", true))
			s.passes(s.update("ladder-a-paths"))
			s.cat("testdata/checks/paths.txt")
			// A check that reads the cells sees the break, whatever the
			// root-package check says.
			const built = "script evaluate: resolve's cell of script/x is built through parse, and the cell through parse2 past it\n"
			if !strings.Contains(string(s.read("testdata/checks/paths.txt")), built) {
				t.Errorf("the paths check finds no cell built otherwise than the cell past it")
			}
			r := s.normal("ladder-a-paths")
			s.fails(r, "the lift resolve is used by ./b on the path testdata/script/ → parse2 → resolve, and ., which owns it, runs it on the path testdata/script/ → parse → resolve, so no package writes the cells of resolve that ./b's cells past it are built through.\nA package that uses a lift reaches it from each corpus by the path the lift's owner does: either ./b reaches resolve from testdata/script/ by .'s path, or . by ./b's. Making a way true may need other registrations to change, such as a lift that would then close a cycle or give a type a second path. If they reach it alike already, a registry is stale: run FIXTURE_UPDATE_GOLDEN=1 go test ./..., which rewrites the registry of every package with tests.")
			if n := strings.Count(string(r.out), " is used by "); n != 1 {
				t.Errorf("the root-package check fails %d times on lifts used otherwise than their owner reaches them, and should fail once", n)
			}
			s.passes(s.run("b", "ladder-b-producer", false))

			s.passes(s.run("b", tt.b, true))
			s.passes(s.update(tt.a))
			col := strings.TrimSuffix(strings.TrimPrefix(tt.drop, "out/"), "/")
			s.fails(s.normal(tt.a), "testdata/script/x.txtar: no registry claims "+tt.drop+"outcome: no column named "+col+" runs over the corpus script.")
			s.edit("testdata/script/x.txtar", "delete the cells no package writes now", drop(tt.drop))
			s.passes(s.update(tt.a))
			s.cat("testdata/checks/paths.txt")
			s.passes(s.normal(tt.a))
			s.passes(s.run("b", tt.b, false))
			s.done()
		})
	}
}

// TestRootCheckLiftOwnerUntested: a registry in a package with no _test.go
// file owns nothing, though it still claims its sections. A column it and
// another package own is the other's alone, and a lift it alone owns,
// which the root package uses, is owned by no package, which the failure
// says, naming the package, or each of them when two such packages own
// it. Nor does such a package use a lift, so the failure never sends a
// person to it to own the lift, and a suite it owns of the lift's name
// isn't named as the lift's owner.
func TestRootCheckLiftOwnerUntested(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared-ladder")
	s.passes(s.update("ladder-a"))
	s.passes(s.run("b", "ladder-b-owns", true))
	s.fails(s.normal("ladder-a"), "the column parse is owned by . and ./b, each as a lift.")

	s.remove("b/corpus_test.go", "remove b's tests, and leave its registry")
	untested := "b/testdata/hypercase.json is a registry, and b holds no _test.go file, so no test compares it, and it still claims sections."
	r := s.normal("ladder-a")
	s.fails(r, untested)
	s.lacks(r, "the column parse is owned by", "no registry claims")

	// The root package uses parse, which only b's registry owns.
	s.passes(s.update("ladder-a-useonly"))
	r = s.normal("ladder-a-useonly")
	s.fails(r, untested, "the lift parse is used by ., and no package owns it, so its error is recorded nowhere. The registry of ./b records that its package owns the lift, and ./b holds no _test.go file, so no test runs the lift there.\nRestore the tests of ./b, or delete its registry and own the lift in one of the packages that use it: give it to hypercase.Own in place of hypercase.Use.")
	s.lacks(r, "no registry claims")

	// A second package's registry owns parse too, and loses its tests.
	s.write("c/corpus_test.go", []byte(packageTestFile), "a package c over the corpus")
	s.passes(s.run("c", "ladder-b-owns", true))
	s.remove("c/corpus_test.go", "remove c's tests, and leave its registry")
	s.fails(s.normal("ladder-a-useonly"), "the lift parse is used by ., and no package owns it, so its error is recorded nowhere. The registries of ./b and ./c record that their packages own the lift, and they hold no _test.go file, so no test runs the lift there.\nRestore the tests of one of them and delete the others' registries, or delete all their registries and own the lift in one of the packages that use it: give it to hypercase.Own in place of hypercase.Use.")

	// b uses parse in place of owning it, and loses its tests again: only
	// c's registry owns parse, and b's use counts for nothing.
	s.write("b/corpus_test.go", []byte(packageTestFile), "restore b's tests")
	s.passes(s.run("b", "ladder-b", true))
	s.remove("b/corpus_test.go", "remove b's tests, and leave its registry")
	only := "the lift parse is used by ., and no package owns it, so its error is recorded nowhere. The registry of ./c records that its package owns the lift, and ./c holds no _test.go file, so no test runs the lift there.\nRestore the tests of ./c, or delete its registry and own the lift in one of the packages that use it: give it to hypercase.Own in place of hypercase.Use."
	s.fails(s.normal("ladder-a-useonly"), only)

	// b owns a suite named parse, and loses its tests again: the failure
	// still names only c as the lift's owner.
	s.write("b/corpus_test.go", []byte(packageTestFile), "restore b's tests")
	s.passes(s.run("b", "ladder-b-suite", true))
	s.remove("b/corpus_test.go", "remove b's tests, and leave its registry")
	s.fails(s.normal("ladder-a-useonly"), only)
	s.done()
}

// TestRootCheckLiftUsedUntested: a lift that only packages with no
// _test.go file use needs no owner, as a transform that only they list
// doesn't: nothing beyond their registries' failures is reported.
func TestRootCheckLiftUsedUntested(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared-ladder")
	s.edit("testdata/script/x.txtar", "remove the outcomes of the root package's lifts, which it owns no longer", drop("out/"))
	// The root package registers no corpus type, and b uses parse, which
	// no package owns.
	s.passes(s.update("root-only"))
	s.passes(s.run("b", "ladder-b", true))
	s.fails(s.normal("root-only"), "the lift parse is used by ./b, and no package owns it, so its error is recorded nowhere.")

	s.remove("b/corpus_test.go", "remove b's tests, and leave its registry")
	r := s.normal("root-only")
	s.fails(r, "b/testdata/hypercase.json is a registry, and b holds no _test.go file, so no test compares it, and it still claims sections.")
	s.lacks(r, "the lift parse", "no registry claims")
	s.done()
}

// TestRootCheckLiftSuiteName: lifts share the suites' column names, so a
// suite named like a lift that another package uses collides with it, and
// is no owner of the lift, which no package then owns. The collision names
// the lift's users in place of its owners, says that owning the lift would
// add a column of the name, and that removing the lift is dropping it from
// every user's hypercase.Use. The users together count as one column, so
// two of them leave the fix at one rename or removal. Owning the lift
// while the suite holds its name would make the column owned twice, so
// the no-owner failure says to own it once no suite shares its name. A
// suite named like a lift another package owns is that column owned
// twice: the failure says which owner holds which, and offers
// hypercase.Use to no one, since the lift's one owner is the owner it
// needs.
func TestRootCheckLiftSuiteName(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared-ladder")
	s.passes(s.update("ladder-a-useonly"))
	s.passes(s.run("b", "ladder-b-suite", true))
	r := s.normal("ladder-a-useonly")
	s.fails(r,
		"the column parse is owned by ./b as a suite, and used by . as a lift. Lifts share the suites' column names, which are unique per corpus root. No package owns the lift, and owning it would add a column of the name.\nRename the suite or the lift, or remove one. To remove the lift, drop it from hypercase.Use in every package that uses it.",
		"the lift parse is used by ., and no package owns it, so its error is recorded nowhere.\nOnce no suite shares the lift's name, own the lift in one of them: give it to hypercase.Own in place of hypercase.Use.")
	s.lacks(r, "\nOwn the lift", "Lifts share the suites' column names, which are unique per corpus root, and one package owns each column.")

	// ./c uses the lift too.
	s.write("c/corpus_test.go", []byte(packageTestFile), "a package c over the corpus")
	s.passes(s.run("c", "ladder-b", true))
	r = s.normal("ladder-a-useonly")
	s.fails(r,
		"the column parse is owned by ./b as a suite, and used by . and ./c, each as a lift. Lifts share the suites' column names, which are unique per corpus root. No package owns the lift, and owning it would add a column of the name.\nRename the suite or the lift, or remove one. To remove the lift, drop it from hypercase.Use in every package that uses it.",
		"the lift parse is used by . and ./c, and no package owns it, so its error is recorded nowhere.\nOnce no suite shares the lift's name, own the lift in one of them: give it to hypercase.Own in place of hypercase.Use.")
	s.lacks(r, "all but one")

	// The root package owns parse, and its update fails on what b's suite
	// wrote under the same name.
	s.fails(s.update("ladder-a"), "out/parse/values.txt is an output the run no longer produces.")
	r = s.normal("ladder-a")
	s.fails(r, "the column parse is owned by ./b as a suite, and by . as a lift. Lifts share the suites' column names, which are unique per corpus root, and one package owns each column.\nRename the suite or the lift, or remove one.")
	s.lacks(r, "hypercase.Use", "the lift parse is used by")
	s.done()
}

// TestRootCheckLiftSuiteNameUntested: a suite named like a used lift, and
// an owner of the lift with no _test.go file, fail together: the
// collision, and the lift owned by no package, naming the untested owner,
// whose fix waits on the collision's. So they do with two untested
// owners, whose fix the wait leads too.
func TestRootCheckLiftSuiteNameUntested(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared-ladder")
	s.passes(s.update("ladder-a-useonly"))
	s.write("c/corpus_test.go", []byte(packageTestFile), "a package c over the corpus")
	s.passes(s.run("c", "ladder-b-owns", true))
	s.remove("c/corpus_test.go", "remove c's tests, and leave its registry")
	s.passes(s.run("b", "ladder-b-suite", true))
	collision := "the column parse is owned by ./b as a suite, and used by . as a lift. Lifts share the suites' column names, which are unique per corpus root. No package owns the lift, and owning it would add a column of the name.\nRename the suite or the lift, or remove one. To remove the lift, drop it from hypercase.Use in every package that uses it."
	s.fails(s.normal("ladder-a-useonly"),
		collision,
		"the lift parse is used by ., and no package owns it, so its error is recorded nowhere. The registry of ./c records that its package owns the lift, and ./c holds no _test.go file, so no test runs the lift there.\nOnce no suite shares the lift's name, restore the tests of ./c, or delete its registry and own the lift in one of the packages that use it: give it to hypercase.Own in place of hypercase.Use.")

	s.write("d/corpus_test.go", []byte(packageTestFile), "a package d over the corpus")
	s.fails(s.run("d", "ladder-b-owns", true), "out/parse/values.txt is an output the run no longer produces.")
	s.remove("d/corpus_test.go", "remove d's tests, and leave its registry")
	s.fails(s.normal("ladder-a-useonly"),
		collision,
		"the lift parse is used by ., and no package owns it, so its error is recorded nowhere. The registries of ./c and ./d record that their packages own the lift, and they hold no _test.go file, so no test runs the lift there.\nOnce no suite shares the lift's name, restore the tests of one of them and delete the others' registries, or delete all their registries and own the lift in one of the packages that use it: give it to hypercase.Own in place of hypercase.Use.")
	s.done()
}

// TestRootCheckLiftSuiteNameMany: a name that three or four columns hold
// fails once, naming each package's kind: a suite and two owners of the
// lift; two suites and one owner; two suites and a lift only used, whose
// user counts as one column; and two suites and two owners. One rename or
// removal leaves two columns of the name or more, so the fix is to rename
// or remove all but one. When two or more packages own the lift, they are
// named, and told that one that only reads its product removes its column
// with hypercase.Use.
func TestRootCheckLiftSuiteNameMany(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared-ladder")
	s.passes(s.update("ladder-a"))
	s.write("c/corpus_test.go", []byte(packageTestFile), "a package c over the corpus")

	// ./b owns a suite named parse, and . and ./c own the lift.
	s.passes(s.run("b", "ladder-b-suite", true))
	s.fails(s.run("c", "ladder-b-owns", true), "out/parse/values.txt is an output the run no longer produces.")
	s.fails(s.normal("ladder-a"), "the column parse is owned by ./b as a suite, and by . and ./c, each as a lift. Lifts share the suites' column names, which are unique per corpus root, and one package owns each column.\nRename or remove all but one of the columns. Of . and ./c, which own the lift, one that only reads its product removes its column by giving the lift to hypercase.Use in place of hypercase.Own.")

	// ./b and ./c own suites named parse, and . owns the lift.
	s.passes(s.run("c", "ladder-b-suite", true))
	r := s.normal("ladder-a")
	s.fails(r, "the column parse is owned by ./b and ./c, each as a suite, and by . as a lift. Lifts share the suites' column names, which are unique per corpus root, and one package owns each column.\nRename or remove all but one of the columns.")
	s.lacks(r, "hypercase.Use", "Column names are unique per corpus root, and one package owns each column.")

	// . uses the lift, which no package owns, and ./b and ./c own suites
	// of its name: the same failure, with the lift's user in place of its
	// owner.
	s.passes(s.update("ladder-a-useonly"))
	r = s.normal("ladder-a-useonly")
	s.fails(r,
		"the column parse is owned by ./b and ./c, each as a suite, and used by . as a lift. Lifts share the suites' column names, which are unique per corpus root. No package owns the lift, and owning it would add a column of the name.\nRename or remove all but one of the suites and the lift. To remove the lift, drop it from hypercase.Use in every package that uses it.",
		"the lift parse is used by ., and no package owns it, so its error is recorded nowhere.\nOnce no suite shares the lift's name, own the lift in one of them: give it to hypercase.Own in place of hypercase.Use.")
	s.lacks(r, "Column names are unique per corpus root, and one package owns each column.", "hypercase.Use in place of hypercase.Own")

	// ./b and ./c own suites named parse, and . and ./d own the lift.
	s.write("d/corpus_test.go", []byte(packageTestFile), "a package d over the corpus")
	s.fails(s.run("d", "ladder-b-owns", true), "out/parse/values.txt is an output the run no longer produces.")
	s.fails(s.update("ladder-a"), "out/parse/values.txt is an output the run no longer produces.")
	s.fails(s.normal("ladder-a"), "the column parse is owned by ./b and ./c, each as a suite, and by . and ./d, each as a lift. Lifts share the suites' column names, which are unique per corpus root, and one package owns each column.\nRename or remove all but one of the columns. Of . and ./d, which own the lift, one that only reads its product removes its column by giving the lift to hypercase.Use in place of hypercase.Own.")
	s.done()
}

// TestRootCheckCorpusTypes: two corpus types of one directory under one
// corpus root fail the root-package check, even when one is a registry's
// in a package with no _test.go file. Under two roots, neither refuses.
func TestRootCheckCorpusTypes(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared-ladder")
	s.passes(s.update("ladder-a"))
	s.passes(s.run("b", "ladder-b-grid", true))
	twoTypes := "the corpus directory script has two different corpus types:\n  .: github.com/sdboyer/hypercase_test.Script: \"Trees of instructions, under in/<tree>/, that the fixtures' columns follow.\"\n  ./b: github.com/sdboyer/hypercase_test.Grid: \"A grid, in the script directory.\"\nAn archive's directory gives its type. Declare the type once, in a package the others import, and register that declaration." + staleRegistry
	s.fails(s.normal("ladder-a"), twoTypes)
	s.remove("b/corpus_test.go", "remove b's tests, and leave its registry")
	s.fails(s.normal("ladder-a"), "b/testdata/hypercase.json is a registry, and b holds no _test.go file, so no test compares it, and it still claims sections.", twoTypes)
	s.write("b/corpus_test.go", []byte(packageTestFile), "restore b's tests")
	s.run("b", "ladder-b-grid-other", true)
	s.lacks(s.normal("ladder-a"), "two different corpus types")
	s.done()
}
