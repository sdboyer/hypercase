package hypercase_test

import (
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
	s.fails(s.normal("ladder-a"), "the column parse is owned by . and ./b. Column names are unique per corpus root, and one package owns each column.\nRename one of the columns, or remove one. A package that only reads a lift's product gives the lift to hypercase.Use.")

	s.passes(s.run("b", "ladder-b", true))
	s.passes(s.update("ladder-a-useonly"))
	s.fails(s.normal("ladder-a-useonly"), "the lift parse is used by . and ./b, and no package owns it, so its error is recorded nowhere.\nOwn the lift in one of them: give it to hypercase.Own in place of hypercase.Use.")
	s.done()
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
	s.fails(s.normal("ladder-a"), "the column parse is owned by . and ./b.")

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
// is no owner of the lift, which no package then owns. A suite named like
// a lift another package owns is that column owned twice.
func TestRootCheckLiftSuiteName(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared-ladder")
	s.passes(s.update("ladder-a-useonly"))
	s.passes(s.run("b", "ladder-b-suite", true))
	s.fails(s.normal("ladder-a-useonly"),
		"the column parse is owned by ./b as a suite, and used by . as a lift. Lifts share the suites' column names, which are unique per corpus root.\nRename the suite or the lift.",
		"the lift parse is used by ., and no package owns it, so its error is recorded nowhere.\nOwn the lift in one of them: give it to hypercase.Own in place of hypercase.Use.")

	// The root package owns parse, and its update fails on what b's suite
	// wrote under the same name.
	s.fails(s.update("ladder-a"), "out/parse/values.txt is an output the run no longer produces.")
	r := s.normal("ladder-a")
	s.fails(r, "the column parse is owned by . and ./b. Column names are unique per corpus root, and one package owns each column.")
	s.lacks(r, "as a suite", "the lift parse is used by")
	s.done()
}

// TestRootCheckCorpusTypes: two corpus types of one directory under one
// corpus root fail the root-package check. Under two roots, neither
// refuses.
func TestRootCheckCorpusTypes(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared-ladder")
	s.passes(s.update("ladder-a"))
	s.passes(s.run("b", "ladder-b-grid", true))
	s.fails(s.normal("ladder-a"), "the corpus directory script has two different corpus types:\n  .: github.com/sdboyer/hypercase_test.Script: \"Trees of instructions, under in/<tree>/, that the fixtures' columns follow.\"\n  ./b: github.com/sdboyer/hypercase_test.Grid: \"A grid, in the script directory.\"\nAn archive's directory gives its type. Declare the type once, in a package the others import, and register that declaration.")
	s.run("b", "ladder-b-grid-other", true)
	s.lacks(s.normal("ladder-a"), "two different corpus types")
	s.done()
}
