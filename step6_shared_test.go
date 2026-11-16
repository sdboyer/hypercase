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
	s.hasSections("testdata/script/bad.txtar", nil, "out/")
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
// another package own is the other's alone, and a lift it alone owns is
// owned by no package, which the failure says, naming the package.
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
