package hypercase_test

import (
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
)

// Step 7: variants and transforms.

// edit rewrites an archive in the sandbox, as a person's edit: fn changes
// its parsed form.
func (s *sandbox) edit(rel, why string, fn func(a *archive.Archive)) {
	s.t.Helper()
	a := archive.Parse(s.read(rel))
	fn(a)
	s.write(rel, a.Format(), why)
}

// drop removes the sections whose names start with prefix.
func drop(prefix string) func(a *archive.Archive) {
	return func(a *archive.Archive) {
		var kept []archive.Section
		for _, sec := range a.Sections {
			if !strings.HasPrefix(sec.Name, prefix) {
				kept = append(kept, sec)
			}
		}
		a.Sections = kept
	}
}

// set sets a section, adding it at the end when the archive lacks it.
func set(name, data string) func(a *archive.Archive) {
	return func(a *archive.Archive) {
		for i := range a.Sections {
			if a.Sections[i].Name == name {
				a.Sections[i].Data = []byte(data)
				return
			}
		}
		a.Sections = append(a.Sections, archive.Section{Name: name, Data: []byte(data)})
	}
}

// insert adds a section before the first whose name starts with before.
func insert(name, data, before string) func(a *archive.Archive) {
	return func(a *archive.Archive) {
		for i := range a.Sections {
			if strings.HasPrefix(a.Sections[i].Name, before) {
				a.Sections = append(a.Sections[:i], append([]archive.Section{{Name: name, Data: []byte(data)}}, a.Sections[i:]...)...)
				return
			}
		}
		a.Sections = append(a.Sections, archive.Section{Name: name, Data: []byte(data)})
	}
}

// TestVariants: each hand-written variant is a case, laid over the base,
// whose cells run as subtests named for it, and whose outputs an update
// writes beside its hand-written sections.
func TestVariants(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "variants")
	u := s.update("variants")
	s.passes(u)
	s.contains(u,
		"--- PASS: TestCorpus/script/twice/echo",
		"--- PASS: TestCorpus/script/twice/variant/cycle/echo",
		"--- PASS: TestCorpus/script/twice/variant/blank/echo",
		"--- PASS: TestCorpus/script/twice/variant/more/echo")
	s.cat(twice)
	s.hasSections(twice, map[string]string{
		"out/echo/values.txt":               "a1 six\n",
		"variant/cycle/out/echo/error.txt":  "A1 -> B1 -> A1: cycle\n",
		"variant/cycle/out/echo/values.txt": "a1 six\n",
		"variant/more/out/echo/b.txt":       "b1 three\n",
		"variant/more/out/echo/values.txt":  "a1 six\n",
	}, "variant/blank/out/")
	s.passes(s.normal("variants"))

	// -run picks a variant's cell by its name.
	r := s.normal("variants", "-test.run=TestCorpus/script/twice/variant/cycle/echo")
	s.passes(r)
	s.contains(r, "--- PASS: TestCorpus/script/twice/variant/cycle/echo")
	s.lacks(r, "TestCorpus/script/twice/echo", "variant/more")

	// A variant inherits none of the base's properties: each case states
	// its own.
	s.edit(twice, "move cycle's properties to the base", func(a *archive.Archive) {
		drop("variant/cycle/properties")(a)
		insert("properties", "circular\n", "out/")(a)
	})
	r = s.normal("variants")
	s.fails(r,
		"script/twice: A1 is twice B1, which echo prints in words.\n    echo: Does what the case's do/ tree says.\n    echo returned no error, but the case states circular, which echo rejects.",
		"script/twice/variant/cycle: The do/ tree errs, as on a cycle.\n    echo: Does what the case's do/ tree says.\n    echo returned an error, but the case states no property echo rejects:")
	s.done()
}

// TestVariantGaps: a hand-written variant's gaps mark its own cells, and
// freeze them as the base's do.
func TestVariantGaps(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "variants")
	s.passes(s.update("variants"))
	s.edit(twice, "mark more's cell, and change what it prints", func(a *archive.Archive) {
		insert("variant/more/gaps", "echo b1 is three, in digits. Waits on sheets#3.\n", "variant/more/out/")(a)
		set("variant/more/in/do/out/b.txt", "b1 3\n")(a)
	})
	r := s.normal("variants")
	s.fails(r, "script/twice/variant/more: The do/ tree gains a second output.\n    echo: Does what the case's do/ tree says.\n    The cell is frozen by its known gap, variant/more/gaps line 1: \"echo b1 is three, in digits. Waits on sheets#3.\"\n    Its committed files record today's behavior, and the run now produces different ones:\n    variant/more/out/echo/b.txt differs (-committed +got):")
	s.lacks(r, "--- FAIL: TestCorpus/script/twice/echo")
	// An update leaves the frozen cell as it is.
	before := s.read(twice)
	s.fails(s.update("variants"), "The cell is frozen by its known gap")
	s.same(twice, before)
	s.done()
}

// TestVariantRootCheck: the root-package check reads each variant's
// description, properties and gaps, and the sections no registry claims
// in any case.
func TestVariantRootCheck(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "variants")
	s.passes(s.update("variants"))
	s.edit(twice, "a variant with no about, states and lines nothing declares, and sections no registry claims", func(a *archive.Archive) {
		drop("variant/blank/about")(a)
		insert("variant/more/properties", "loopy\n", "variant/more/out/")(a)
		insert("variant/more/gaps", "format b1 prints wrongly. Waits on sheets#3.\n", "variant/more/out/")(a)
		insert("gaps", "transform/nope echo Waits on sheets#4.\n", "out/")(a)
		set("variant/more/out/shout/values.txt", "B1 THREE\n")(a)
		set("transform/nope/in/do/out/values.txt", "nope\n")(a)
		set("transform/nope/out/echo/values.txt", "nope\n")(a)
	})
	r := s.normal("variants")
	s.fails(r,
		"testdata/script/twice.txtar: variant/blank/about: the variant has no description: its about section is missing or empty\nWrite each variant's description, in prose, in its about section.",
		"testdata/script/twice.txtar: variant/more/properties states loopy, and no registry under the module-root corpus lists a property of that name.",
		"testdata/script/twice.txtar: variant/more/gaps, line 1, names format, and no registry has a column of that name over the corpus script.",
		"testdata/script/twice.txtar: gaps, line 1, names transform/nope, and no corpus type of testdata/script/ lists a transform of that name.\nList the transform, or remove the line.",
		"testdata/script/twice.txtar: no registry claims variant/more/out/shout/values.txt: no column named shout runs over the corpus script.",
		"testdata/script/twice.txtar: no registry claims transform/nope/in/do/out/values.txt and transform/nope/out/echo/values.txt: no corpus type of testdata/script/ lists a transform named nope.\nIf a transform should make the variant, list it in its corpus type's Transforms. If not, delete them by hand; FIXTURE_UPDATE_GOLDEN=1 go test ./... leaves them in place.")
	s.done()
}

// TestVariantForm: a variant's problems of form stop the archive's cells,
// as the base's do, and an update writes nothing to it.
func TestVariantForm(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "variants")
	s.passes(s.update("variants"))
	s.edit(twice, "delete a path the base lacks, and split cycle's hand-written sections", func(a *archive.Archive) {
		set("variant/blank/delete", "do/out/values.txt\ndo/out/gone.txt\n")(a)
		insert("variant/cycle/gaps", "echo Waits on sheets#5.\n", "variant/more/about")(a)
	})
	before := s.read(twice)
	s.fails(s.normal("variants"),
		"testdata/script/twice.txtar has problems of form, so none of its cells run, and no update writes it:\n  variant/blank/delete, line 2: the base has no do/out/gone.txt to remove\n  variant/cycle/gaps: the section sits apart from variant/cycle's other hand-written sections; each case's hand-written sections sit together")
	s.contains(s.update("variants"), "testdata/script/twice.txtar has problems of form, so none of its cells run, and no update writes it. The root-package check reports them.")
	s.same(twice, before)
	s.done()
}

// TestVariantsShared: a variant's cells are each column's, in the package
// that owns the column.
func TestVariantsShared(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "variants")
	s.write("b/corpus_test.go", []byte(packageTestFile), "a package b over the corpus")
	s.edit(twice, "give the variants shout/ trees", func(a *archive.Archive) {
		insert("variant/cycle/in/shout/out/values.txt", "A1 CYCLE\n", "variant/blank/about")(a)
	})
	s.passes(s.update("variants"))
	s.passes(s.run("b", "variants-b", true))
	s.hasSections(twice, map[string]string{"variant/cycle/out/shout/values.txt": "A1 CYCLE\n"}, "out/shout/", "variant/more/out/shout/")
	s.passes(s.normal("variants"))
	s.passes(s.run("b", "variants-b", false))
	s.done()
}

// TestTransforms: an update writes each transform's variant of each base
// it doesn't decline, and each column's cells on it; a normal run checks
// them like goldens.
func TestTransforms(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	u := s.update("transforms")
	s.passes(u)
	s.contains(u,
		"--- PASS: TestCorpus/script/twice/transform/looped/echo",
		"--- PASS: TestCorpus/script/twice/transform/upper/echo",
		"--- PASS: TestCorpus/script/errs/transform/upper/echo",
		"--- SKIP: TestCorpus/script/errs/transform/looped",
		"script/errs/transform/looped: looped declines the base: the base's do/ tree already errs",
		"--- SKIP: TestCorpus/script/garbled/transform/looped",
		"script/garbled/transform/looped: looped declines the base: the base states malformed, which looped neither gives nor keeps",
		"script/garbled/transform/upper: upper declines the base: the base states malformed, which upper neither gives nor keeps")
	for _, a := range []string{"twice", "errs", "garbled"} {
		s.cat("testdata/script/" + a + ".txtar")
	}
	s.cat("testdata/hypercase.json")
	s.passes(s.normal("transforms"))
	s.done()
}

// TestTransformInputs: the package that owns a transform judges its
// variants' inputs like goldens, and an update replaces them whole.
func TestTransformInputs(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.passes(s.update("transforms"))
	fresh := s.read(twice)

	s.edit(twice, "edit upper's input by hand, and add one it doesn't make", func(a *archive.Archive) {
		set("transform/upper/in/do/out/values.txt", "A1 SEVEN\n")(a)
		insert("transform/upper/in/do/out/z.txt", "Z\n", "transform/upper/out/")(a)
	})
	r := s.normal("transforms")
	s.fails(r,
		"script/twice/transform/upper: Uppercases every file under do/out/.\n    upper's variant of the base differs from its committed inputs:\n    transform/upper/in/do/out/values.txt differs (-committed +made):",
		"transform/upper/in/do/out/z.txt is no longer made\n    If the change is intended, run FIXTURE_UPDATE_GOLDEN=1 go test ./... and read the diff before committing.",
		// The cells read the committed inputs.
		"transform/upper/out/echo/values.txt differs (-golden +got):",
		"transform/upper/out/echo/z.txt has no golden.")
	s.passes(s.update("transforms"))
	s.same(twice, fresh)

	s.edit(twice, "remove upper's inputs, and leave its outputs", drop("transform/upper/in/"))
	s.fails(s.normal("transforms"), "script/twice/transform/upper: Uppercases every file under do/out/.\n    upper's variant of the base isn't written: the archive holds none of its inputs.\n    Run FIXTURE_UPDATE_GOLDEN=1 go test ./... to write them, and read them before committing.")
	s.passes(s.update("transforms"))
	s.same(twice, fresh)
	s.done()
}

// TestTransformBaseEdited: a base edited by hand can leave a transform's
// committed inputs ones no update would write: a delete line naming a path
// the base no longer has, or a view equal to the base's. Those are inputs
// that differ from what the transform makes, which its owner judges and
// its update rewrites, never problems of form, which stop the archive and
// which no update repairs.
func TestTransformBaseEdited(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.passes(s.update("trim"))
	s.hasSections(twice, map[string]string{"transform/trim/delete": "do/out/values.txt\n"})
	s.edit(twice, "rename the base's values.txt, which trim's delete names, and its golden", func(a *archive.Archive) {
		for i := range a.Sections {
			switch a.Sections[i].Name {
			case "in/do/out/values.txt":
				a.Sections[i].Name = "in/do/out/v.txt"
			case "out/echo/values.txt":
				a.Sections[i].Name = "out/echo/v.txt"
			}
		}
	})
	r := s.normal("trim")
	s.fails(r, "script/twice/transform/trim: Removes do/out/values.txt, and adds do/out/k.txt.\n    trim's variant of the base differs from its committed inputs:\n    transform/trim/delete is no longer made")
	s.lacks(r, "problems of form")
	s.passes(s.update("trim"))
	s.hasSections(twice, map[string]string{"transform/trim/out/echo/v.txt": "a1 six\n"}, "transform/trim/delete")
	s.passes(s.normal("trim"))

	for _, a := range []string{twice, "testdata/script/errs.txtar"} {
		s.edit(a, "drop trim's variant, which the next registrations don't list", drop("transform/trim/"))
	}
	s.passes(s.update("transforms"))
	s.edit(twice, "give the base what looped adds, so looped's committed variant equals it", func(a *archive.Archive) {
		insert("in/do/error", "A1 -> A1: cycle\n", "in/do/out/")(a)
		insert("in/resolve/error", "A1 -> A1: cycle\n", "out/")(a)
		insert("properties", "circular\n", "out/")(a)
	})
	r = s.normal("transforms")
	s.fails(r, "transform/looped/in/do/error and transform/looped/in/resolve/error are written, and looped declines the base: the base's do/ tree already errs\n    Run FIXTURE_UPDATE_GOLDEN=1 go test ./... to remove them.")
	s.lacks(r, "problems of form")
	s.fails(s.update("transforms"), "transform/looped/out/echo/error.txt is recorded, and looped declines the base")
	s.hasSections(twice, nil, "transform/looped/in/")
	s.done()
}

// TestTransformDeclines: when a transform no longer applies to a base, its
// owner's update removes the variant's inputs, and what is left under
// transform/<name>/out/ is each column's orphan.
func TestTransformDeclines(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.passes(s.update("transforms"))
	s.edit(twice, "make the base err, so looped declines it", func(a *archive.Archive) {
		insert("in/do/error", "A1 -> B1 -> A1: cycle\n", "out/")(a)
		insert("properties", "circular\n", "out/")(a)
	})
	r := s.normal("transforms")
	s.fails(r,
		"script/twice/transform/looped: Adds do/error and resolve/error, so echo and resolve err as on a cycle.\n    transform/looped/in/do/error and transform/looped/in/resolve/error are written, and looped declines the base: the base's do/ tree already errs\n    Run FIXTURE_UPDATE_GOLDEN=1 go test ./... to remove them.",
		"transform/looped/out/echo/error.txt is recorded, and looped declines the base: the base's do/ tree already errs\n    If the transform should make the variant, fix it. If not, delete the section by hand; FIXTURE_UPDATE_GOLDEN=1 go test ./... leaves it in place.")
	// The update removes the inputs, and fails on the orphans.
	s.fails(s.update("transforms"), "transform/looped/out/echo/error.txt is recorded, and looped declines the base")
	s.hasSections(twice, nil, "transform/looped/in/")
	s.lacks(s.normal("transforms"), "are written, and looped declines the base")
	s.edit(twice, "delete looped's outputs by hand", drop("transform/looped/"))
	s.passes(s.normal("transforms"))
	s.done()
}

// TestTransformGaps: a known gap in a cell of a transform's variant is a
// line of the base's gaps that names the transform.
func TestTransformGaps(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.passes(s.update("transforms"))
	errs := "testdata/script/errs.txtar"
	s.edit(errs, "mark upper's cell of echo, and record what is right", func(a *archive.Archive) {
		insert("gaps", "transform/upper echo partial stays lowercase. Waits on sheets#7.\n", "out/")(a)
		set("transform/upper/out/echo/values.txt", "partial\n")(a)
	})
	r := s.normal("transforms")
	s.fails(r, "script/errs/transform/upper: Uppercases every file under do/out/.\n    echo: Does what the case's do/ tree says.\n    The cell is frozen by its known gap, gaps line 1: \"transform/upper echo partial stays lowercase. Waits on sheets#7.\"")
	s.lacks(r, "--- FAIL: TestCorpus/script/errs/echo")

	// A line that marks a cell of a variant its transform declines fails.
	s.edit("testdata/script/garbled.txtar", "mark looped's cell of echo", insert("gaps", "transform/looped echo Waits on sheets#8.\n", "out/"))
	s.fails(s.normal("transforms"), "script/garbled/transform/looped: Adds do/error and resolve/error, so echo and resolve err as on a cycle.\n    gaps, line 1, marks the cell of transform/looped echo, and looped declines the base: the base states malformed, which looped neither gives nor keeps\n    Remove the line, or fix the transform.")
	s.done()
}

// TestTransformLifts: a lift is a column on every variant too: resolve
// errs on each of looped's variants, which give circular, and a variant
// parse rejects blocks the cells past it.
func TestTransformLifts(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.remove("testdata/script/errs.txtar", "")
	s.remove("testdata/script/garbled.txtar", "")
	s.edit(twice, "a hand-written variant parse rejects", func(a *archive.Archive) {
		set("variant/bad/about", "parse rejects the variant.\n")(a)
		set("variant/bad/properties", "malformed\n")(a)
		set("variant/bad/in/parse/error", "A1: not a number\n")(a)
	})
	r := s.update("tr-ladder")
	s.passes(r)
	s.contains(r, "script/twice/variant/bad: resolve is blocked: the lift parse failed on the case, and testdata/script/twice.txtar, variant/bad/out/parse/error.txt, records its error.")
	s.hasSections(twice, map[string]string{
		"variant/bad/out/parse/error.txt":        "A1: not a number\n",
		"transform/looped/out/resolve/error.txt": "A1 -> A1: cycle\n",
	}, "transform/upper/out/", "out/")
	s.passes(s.normal("tr-ladder"))
	s.done()
}

// TestTransformExpectedErrors: a transform's variant states what the
// transform gives and keeps, so the expected-error check on one of its
// cells names those, and the transform's jenny, where a case's names its
// properties.
func TestTransformExpectedErrors(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	r := s.update("tr-props")
	s.fails(r,
		"script/twice/transform/looped: Adds do/error and resolve/error, so echo and resolve err as on a cycle.\n    echo: Does what the case's do/ tree says.\n    echo returned an error, but the variant states no property echo rejects:\n      A1 -> A1: cycle\n    The variant states what looped gives, and each property it keeps that the base states. Fix the code or looped's jenny, or give the variant the property, in looped's Gives or Keeps, and list it in echo's Rejects.",
		"script/twice/transform/upper: Uppercases every file under do/out/.\n    echo: Does what the case's do/ tree says.\n    echo returned no error, but the variant states circular, which echo rejects.\n    The variant states what upper gives, and each property it keeps that the base states. Fix the code or upper's jenny, or correct upper's Gives or Keeps, or echo's Rejects.")
	s.lacks(r, "the case's properties", "state the property on the case")
	s.done()
}

// TestTransformsShared: one package owns each transform, and writes and
// judges its variants' inputs. Every package that runs the type runs each
// variant's cells, on the committed inputs, but in the owner's update.
func TestTransformsShared(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.write("b/corpus_test.go", []byte(packageTestFile), "a package b over the corpus")
	s.passes(s.update("transforms"))
	s.passes(s.run("b", "tr-b", true))
	s.hasSections(twice, map[string]string{
		"transform/upper/out/copy/values.txt": "A1 SIX\n",
		"transform/looped/out/copy/error.txt": "A1 -> A1: cycle\n",
	})
	s.passes(s.normal("transforms"))
	s.passes(s.run("b", "tr-b", false))

	s.edit(twice, "edit upper's input by hand", set("transform/upper/in/do/out/values.txt", "A1 SEVEN\n"))
	b := s.run("b", "tr-b", false)
	s.fails(b, "transform/upper/out/copy/values.txt differs (-golden +got):")
	s.lacks(b, "differs from its committed inputs")
	// b's update runs the cells on the committed inputs too, and leaves
	// them to upper's owner.
	s.passes(s.run("b", "tr-b", true))
	s.hasSections(twice, map[string]string{"transform/upper/in/do/out/values.txt": "A1 SEVEN\n", "transform/upper/out/copy/values.txt": "A1 SEVEN\n"})
	s.passes(s.run("b", "tr-b", false))
	// The owner's update rewrites them, and b's next update follows.
	s.passes(s.update("transforms"))
	s.fails(s.run("b", "tr-b", false), "transform/upper/out/copy/values.txt differs (-golden +got):")
	s.passes(s.run("b", "tr-b", true))
	s.passes(s.run("b", "tr-b", false))
	s.done()
}

// TestTransformJennyChanges: when a transform's jenny changes, only its
// owner's update runs the cells on the variant it now makes. Another
// package's update runs them on the committed inputs, so its next normal
// run passes, and a whole-module update settles over passes: the owner
// rewrites the inputs, and on the next pass the others follow.
func TestTransformJennyChanges(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.write("b/corpus_test.go", []byte(packageTestFile), "a package b over the corpus")
	s.passes(s.update("transforms"))
	s.passes(s.run("b", "tr-b", true))
	before := s.read(twice)

	// upper's jenny now doubles what it uppercases. b's update alone
	// writes nothing, and b's normal run passes.
	s.passes(s.run("b", "tr-b-jenny", true))
	s.same(twice, before)
	s.passes(s.run("b", "tr-b-jenny", false))

	// The rest of a whole-module update that ran b first: the owner's
	// update rewrites upper's inputs after b ran its cells, so b's are
	// stale.
	s.passes(s.update("tr-jenny"))
	s.hasSections(twice, map[string]string{
		"transform/upper/in/do/out/values.txt": "A1 SIX\nA1 SIX\n",
		"transform/upper/out/echo/values.txt":  "A1 SIX\nA1 SIX\n",
		"transform/upper/out/copy/values.txt":  "A1 SIX\n",
	})
	s.fails(s.run("b", "tr-b-jenny", false), "transform/upper/out/copy/values.txt differs (-golden +got):")
	// The next pass settles it, and the one after changes nothing.
	s.passes(s.run("b", "tr-b-jenny", true))
	s.passes(s.update("tr-jenny"))
	settled := s.read(twice)
	s.hasSections(twice, map[string]string{"transform/upper/out/copy/values.txt": "A1 SIX\nA1 SIX\n"})
	s.passes(s.run("b", "tr-b-jenny", true))
	s.passes(s.update("tr-jenny"))
	s.same(twice, settled)
	s.passes(s.normal("tr-jenny"))
	s.passes(s.run("b", "tr-b-jenny", false))
	s.done()
}

// TestTransformOwners: the root-package check fails a transform no package
// owns, or two own, and a corpus type two packages list differently.
func TestTransformOwners(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.write("b/corpus_test.go", []byte(packageTestFile), "a package b over the corpus")
	s.passes(s.update("tr-unowned"))
	s.passes(s.run("b", "tr-b", true))
	r := s.normal("tr-unowned")
	s.fails(r,
		"the transform looped, of testdata/script/, is owned by no package, so nothing writes or judges its variants' inputs.\nGive it to hypercase.Own in one package that registers its corpus type.",
		"the transform upper, of testdata/script/, is owned by no package")
	s.passes(s.update("transforms"))
	s.passes(s.run("b", "tr-b-owns", true))
	s.fails(s.normal("transforms"), "the transform upper, of testdata/script/, is owned by . and ./b. One package owns each transform.\nOwn it in one of them.")
	s.passes(s.run("b", "tr-b-differs", true))
	s.fails(s.normal("transforms"), "the corpus directory script has two different corpus types:\n  .: github.com/sdboyer/hypercase_test.Script: \"Trees of instructions, under in/<tree>/, that the fixtures' columns follow.\"; the transform looped: \"Adds do/error and resolve/error, so echo and resolve err as on a cycle.\", gives [circular], keeps [], with a probe; the transform upper: \"Uppercases every file under do/out/.\", gives [], keeps [circular dangling], with a probe\n  ./b: github.com/sdboyer/hypercase_test.Script: \"Trees of instructions, under in/<tree>/, that the fixtures' columns follow.\"; the transform looped: \"Adds do/error and resolve/error, so echo and resolve err as on a cycle.\", gives [circular], keeps [], with a probe\n")
	s.done()
}

// TestTransformFailures: a transform that fails on a base, or makes a
// variant that can't be one, fails in the package that owns it, and is
// skipped in the others.
func TestTransformFailures(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transform-failures")
	s.write("b/corpus_test.go", []byte(packageTestFile), "a package b over the corpus")
	want := []string{
		"script/error/transform/poke: Adds do/out/p.txt.\n    poke failed on the base: poke: the base is too big\n    Fix its jenny, or make its probe decline such a base.",
		"script/panic/transform/poke: Adds do/out/p.txt.\n    poke panicked on the base: boom\n    Fix its jenny.",
		"script/same/transform/poke: Adds do/out/p.txt.\n    poke made an input view equal to the base's. A variant changes the base.\n    Make its probe decline such a base.",
		"script/flat/transform/poke: Adds do/out/p.txt.\n    poke made a variant an archive can't hold:\n      transform/poke/in/flat: an input belongs in a tree, as transform/poke/in/<tree>/<file>",
		"script/marker/transform/poke: Adds do/out/p.txt.\n    poke made files an archive can't hold:\n      do/out/m.txt: its line 2 is shaped like a section marker",
		"script/nest/transform/poke: Adds do/out/p.txt.\n    poke made an input view no file tree holds: it holds tr/nest, and tr/nest/x below it.\n    Fix its jenny.",
		"script/probe-panic/transform/poke: Adds do/out/p.txt.\n    poke's probe panicked on the base: probe boom\n    Fix the probe.",
		"script/probe-none/transform/poke: Adds do/out/p.txt.\n    poke's probe said the base isn't for it, and gave no reason.\n    Fix the probe: when it says no, it says why.",
	}
	s.fails(s.update("poke"), want...)
	s.fails(s.normal("poke"), append(want, "script/probe-no/transform/poke: poke declines the base: the base is too small")...)
	for _, a := range []string{"error", "same", "probe-no"} {
		s.cat("testdata/script/" + a + ".txtar")
	}
	b := s.run("b", "poke-b", true)
	s.passes(b)
	s.contains(b, "script/error/transform/poke: the transform poke failed on the base, and the package that owns it reports why.")
	s.done()
}

func TestTransformRefusals(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	for _, tt := range []struct{ reg, want string }{
		{"tr-bare", "panic: hypercase: Register was given the transform looped. List it in its corpus type's Transforms, and pass it to hypercase.Own in the package that owns it."},
		{"tr-use", "panic: hypercase: hypercase.Use was given the transform looped. It takes lifts only: every package that runs a corpus type runs its transforms, from the type's declaration. Pass the transform to hypercase.Own in the package that owns it."},
		{"tr-own-twice", "panic: hypercase: the transform looped is given to hypercase.Own twice. Own it once."},
		{"tr-bad-name", `panic: hypercase: the corpus type hypercase_test.Script lists a transform named "Looped", which breaks the naming rule: lowercase ASCII letters and digits, in words joined by single hyphens, starting with a letter. Rename the transform.`},
		{"tr-two-names", "panic: hypercase: the corpus type hypercase_test.Script lists two transforms named looped. Transform names are unique per corpus type; rename one."},
		{"tr-no-jenny", "panic: hypercase: the transform looped, of the corpus type hypercase_test.Script, has no Jenny. Give it the jenny that makes its variant's input view from the base's."},
		{"tr-gives-keeps", "panic: hypercase: the transform looped both gives and keeps circular. Every variant has a property the transform gives: list it in Gives alone."},
		{"tr-gives-twice", "panic: hypercase: the transform looped gives circular twice. List each property once."},
		{"tr-keeps-twice", "panic: hypercase: the transform upper keeps dangling twice. List each property once."},
		{"col-variant", "panic: hypercase: the suite name variant is reserved: a case's subtests name its variants variant/<name> and transform/<name>. Rename the suite."},
		{"col-transform", "panic: hypercase: the suite name transform is reserved: a case's subtests name its variants variant/<name> and transform/<name>. Rename the suite."},
		{"lift-variant", "panic: hypercase: the lift name variant is reserved: a case's subtests name its variants variant/<name> and transform/<name>. Rename the lift."},
	} {
		r := s.normal(tt.reg)
		s.fails(r, tt.want)
		s.lacks(r, "--- ")
	}
	s.done()
}

// TestTransformRelations: what relates a transform to the rest of the
// registrations is checked once initialization is over, before any case
// runs.
func TestTransformRelations(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	for _, tt := range []struct{ reg, want string }{
		{"tr-own-unlisted", "the package owns the transform upper, and the corpus type hypercase_test.Script lists no transform of that name.\nList the transform in the type's Transforms, or remove it from hypercase.Own."},
		{"tr-own-no-type", "the package owns the transform looped, of the corpus type hypercase_test.Script, and registers no such corpus type.\nRegister the corpus type, which lists the transform in its Transforms."},
		{"tr-own-differs", "the package owns a transform named looped that differs from the one the corpus type hypercase_test.Script lists.\nOwn the declaration the type lists: declare the transform once, and use that declaration in both places."},
		{"tr-unregistered-gives", "the transform looped, of the corpus type hypercase_test.Script, gives circular, and the package registers no property of that name.\nRegister the property, or remove it from looped's Gives."},
		{"tr-other-description", "the transform upper keeps a property named circular, which the package registers with another description.\nList the registered property: declare it once, and use that declaration in both places."},
	} {
		r := s.normal(tt.reg)
		s.fails(r, tt.want, "No case runs until the registrations agree.")
		s.lacks(r, "TestCorpus/script/")
	}
	// A transform with no description fails the run, and its variants run.
	r := s.normal("tr-no-description")
	s.fails(r, "the transform looped, of the corpus type hypercase_test.Script, has no description.\nAdd one: it says what the transform changes, in prose, and it describes each variant the transform makes.")
	s.contains(r, "--- FAIL: TestCorpus/script/twice/transform/looped/echo")
	s.done()
}
