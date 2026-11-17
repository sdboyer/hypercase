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
		"variant/blank/out/echo/outcome":    "accepted\n",
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
		"testdata/script/twice.txtar: no registry claims transform/nope/in/do/out/values.txt and transform/nope/out/echo/values.txt: no corpus type of testdata/script/ lists a transform named nope.\nIf a transform should make the variant, list it in its corpus type's Transforms. If not, remove gaps line 1, which marks a cell of theirs, so that strip removes them; then run hypercase strip -transform nope at the module root, then FIXTURE_UPDATE_GOLDEN=1 go test ./... until it changes nothing.")
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

// TestTransformInvalidInputs: a transform's committed inputs edited by
// hand into ones that make no input view are judged as inputs, never as
// problems of form: a normal run, in any package, would read them, so it
// fails the variant's cells, and the owner's update rewrites them. When
// the transform declines the base, no run reads them, and only the
// owner's fails, on their being there at all.
func TestTransformInvalidInputs(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.write("b/corpus_test.go", []byte(packageTestFile), "a package b over the corpus")
	s.passes(s.update("transforms"))
	s.passes(s.run("b", "tr-b", true))
	fresh := s.read(twice)
	const (
		failure = "script/twice/transform/looped: Adds do/error and resolve/error, so echo and resolve err as on a cycle.\n    The archive's inputs for looped's variant are invalid, and no cell runs on them:\n      "
		repair  = "\n    The module's update, FIXTURE_UPDATE_GOLDEN=1 go test ./..., rewrites them, in the package that owns looped."
	)
	for _, e := range []struct {
		desc, section, data, before, problem string
	}{
		{"lay a file over looped's view below the base's values.txt", "transform/looped/in/do/out/values.txt/x", "x\n", "transform/looped/in/resolve/",
			"transform/looped: the variant's input view holds do/out/values.txt, and do/out/values.txt/x below it"},
		{"give looped a delete line that is no path", "transform/looped/delete", "../x\n", "transform/looped/out/",
			`transform/looped/delete, line 1: "../x" is not a path in the input view`},
		{"give looped a delete line twice", "transform/looped/delete", "do/out/values.txt\ndo/out/values.txt\n", "transform/looped/out/",
			"transform/looped/delete, line 2: do/out/values.txt is listed twice, on lines 1 and 2"},
		{"give looped an empty delete", "transform/looped/delete", "", "transform/looped/out/",
			"transform/looped/delete: the section is empty; a variant that removes nothing has no delete section"},
	} {
		s.edit(twice, e.desc, insert(e.section, e.data, e.before))
		r := s.normal("transforms")
		s.fails(r, failure+e.problem+repair)
		s.lacks(r, "problems of form", "differs from its committed inputs", "If no package owns")
		s.fails(s.run("b", "tr-b", false), failure+e.problem+repair)
		// b's update reads what looped makes, and leaves the inputs to
		// looped's owner, whose update rewrites them.
		s.passes(s.run("b", "tr-b", true))
		s.passes(s.update("transforms"))
		s.same(twice, fresh)
		s.passes(s.normal("transforms"))
		s.passes(s.run("b", "tr-b", false))
	}

	// Inputs for a base the transform declines are read by no cell. The
	// owner's normal run fails on them, valid or not, and its update
	// removes them; another package's run passes the variant by.
	errs := "testdata/script/errs.txtar"
	before := s.read(errs)
	s.edit(errs, "give looped, which declines errs, an empty delete", insert("transform/looped/delete", "", "transform/upper/"))
	r := s.normal("transforms")
	s.fails(r, "script/errs/transform/looped: Adds do/error and resolve/error, so echo and resolve err as on a cycle.\n    transform/looped/delete is written, and looped declines the base: the base's do/ tree already errs\n    Run FIXTURE_UPDATE_GOLDEN=1 go test ./... to remove them.")
	s.lacks(r, "are invalid")
	b := s.run("b", "tr-b", false)
	s.passes(b)
	s.contains(b, "script/errs/transform/looped: looped declines the base: the base's do/ tree already errs")
	s.passes(s.update("transforms"))
	s.same(errs, before)
	s.passes(s.normal("transforms"))
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
		"script/twice/transform/looped: Adds do/error and resolve/error, so echo and resolve err as on a cycle.\n    echo: Does what the case's do/ tree says.\n    echo returned an error, but the variant states no property echo rejects:\n      A1 -> A1: cycle\n    The variant states what looped gives, and each property it keeps that the base states.\n    Fix the code or looped's jenny, or give the variant the property, in looped's Gives or Keeps, and list it in echo's Rejects.",
		"script/twice/transform/upper: Uppercases every file under do/out/.\n    echo: Does what the case's do/ tree says.\n    echo returned no error, but the variant states circular, which echo rejects.\n    The variant states what upper gives, and each property it keeps that the base states.\n    Fix the code or upper's jenny, or correct upper's Gives or Keeps, or echo's Rejects.")
	s.lacks(r, "the case's properties", "state the property on the case")
	s.done()
}

// TestTransformsShared: one package owns each transform, and writes and
// judges its variants' inputs. Every package that runs the type runs each
// variant's cells: an update on what the transform makes, and a normal run
// on the inputs the archive holds.
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
	s.fails(b, "transform/upper/out/copy/values.txt differs (-golden +got):",
		"A cell of the variant failed, and the archive's inputs for upper's variant differ from what upper makes now:\ntransform/upper/in/do/out/values.txt differs (-committed +made):\n- A1 SEVEN\n+ A1 SIX\nA normal run reads the inputs, and an update what upper makes.\nThe module's update, FIXTURE_UPDATE_GOLDEN=1 go test ./..., rewrites them, in the package that owns upper.")
	s.lacks(b, "differs from its committed inputs")
	// b's update runs the cells on what upper makes, and leaves the inputs
	// to upper's owner, so b's normal run fails until the owner's update
	// rewrites them.
	s.passes(s.run("b", "tr-b", true))
	s.hasSections(twice, map[string]string{"transform/upper/in/do/out/values.txt": "A1 SEVEN\n", "transform/upper/out/copy/values.txt": "A1 SIX\n"})
	s.fails(s.run("b", "tr-b", false), "transform/upper/out/copy/values.txt differs (-golden +got):")
	s.passes(s.update("transforms"))
	s.passes(s.run("b", "tr-b", false))
	s.done()
}

// TestTransformJennyChanges: when a transform's jenny changes, every
// package's update runs the cells on the variant it now makes, so the
// module's update settles in one pass, in either order or at once. Another
// package's update alone leaves its normal run failing on the inputs the
// archive holds, and the failure names the module's update.
func TestTransformJennyChanges(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.write("b/corpus_test.go", []byte(packageTestFile), "a package b over the corpus")
	s.passes(s.update("transforms"))
	s.passes(s.run("b", "tr-b", true))
	before := s.read(twice)

	// upper's jenny now doubles what it uppercases. b's update alone
	// writes its cells on what upper makes, and b's normal run, on the
	// inputs the archive holds, fails and names the module's update.
	s.passes(s.run("b", "tr-b-jenny", true))
	s.hasSections(twice, map[string]string{
		"transform/upper/in/do/out/values.txt": "A1 SIX\n",
		"transform/upper/out/copy/values.txt":  "A1 SIX\nA1 SIX\n",
	})
	s.fails(s.run("b", "tr-b-jenny", false), "transform/upper/out/copy/values.txt differs (-golden +got):",
		"A cell of the variant failed, and the archive's inputs for upper's variant differ from what upper makes now:\ntransform/upper/in/do/out/values.txt differs (-committed +made):\n  A1 SIX\n+ A1 SIX\nA normal run reads the inputs, and an update what upper makes.\nThe module's update, FIXTURE_UPDATE_GOLDEN=1 go test ./..., rewrites them, in the package that owns upper.")
	// The rest of the module's update, the owner's, settles it.
	s.passes(s.update("tr-jenny"))
	s.hasSections(twice, map[string]string{
		"transform/upper/in/do/out/values.txt": "A1 SIX\nA1 SIX\n",
		"transform/upper/out/echo/values.txt":  "A1 SIX\nA1 SIX\n",
		"transform/upper/out/copy/values.txt":  "A1 SIX\nA1 SIX\n",
	})
	settled := s.read(twice)
	s.passes(s.normal("tr-jenny"))
	s.passes(s.run("b", "tr-b-jenny", false))

	// From before the change, one pass of the module's update settles it
	// with the owner first too, and with both at once.
	s.write(twice, before, "restore the archive from before the jenny changed")
	s.passes(s.update("tr-jenny"))
	// The owner's update alone rewrites the inputs, so its normal run,
	// which reads them, passes.
	s.passes(s.normal("tr-jenny"))
	s.passes(s.run("b", "tr-b-jenny", true))
	s.same(twice, settled)
	s.write(twice, before, "restore the archive from before the jenny changed")
	for _, r := range s.parallel(childRun{".", "tr-jenny", true}, childRun{"b", "tr-b-jenny", true}) {
		s.passes(r)
	}
	s.same(twice, settled)
	// A further pass changes nothing.
	s.passes(s.update("tr-jenny"))
	s.passes(s.run("b", "tr-b-jenny", true))
	s.same(twice, settled)
	s.passes(s.normal("tr-jenny"))
	s.passes(s.run("b", "tr-b-jenny", false))
	s.done()
}

// TestTransformJennyFrozen: a frozen cell is the exception to one pass of
// the module's update settling a jenny's change. No update writes it, so
// when the change moves its output, every update of the package that owns
// its column fails on it, and so does each of that package's normal runs
// once the transform's owner has rewritten the inputs. Before that, the
// cell reads the old inputs, and that package's run of the frozen cell
// passes, while the transform owner's normal run fails on the inputs,
// which differ from what the transform makes. It fails until a person
// deletes its files or removes its gaps line, and then the package that
// owns its column runs its update, which writes the cell. Until then,
// that package's normal run still fails on it, since no other package's
// update writes it.
func TestTransformJennyFrozen(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.remove("testdata/script/errs.txtar", "leave twice, whose frozen cells this follows")
	s.remove("testdata/script/garbled.txtar", "leave twice, whose frozen cells this follows")
	s.write("b/corpus_test.go", []byte(packageTestFile), "a package b over the corpus")
	s.passes(s.update("transforms"))
	s.passes(s.run("b", "tr-b", true))
	s.edit(twice, "mark upper's cells of echo and copy", insert("gaps", "transform/upper echo Waits on sheets#9.\ntransform/upper copy Waits on sheets#10.\n", "out/"))
	s.passes(s.normal("transforms"))
	s.passes(s.run("b", "tr-b", false))

	// upper's jenny now doubles what it uppercases. b's update fails on
	// its frozen cell, and b's normal run, on the old inputs, passes it.
	frozen := func(col, line string) string {
		return "script/twice/transform/upper: Uppercases every file under do/out/.\n    " + col + ": Does what the case's do/ tree says.\n    The cell is frozen by its known gap, gaps line " + line + ": \"transform/upper " + col + " Waits on sheets#"
	}
	s.fails(s.run("b", "tr-b-jenny", true), frozen("copy", "2"))
	r := s.run("b", "tr-b-jenny", false)
	s.passes(r)
	s.contains(r, "--- PASS: TestCorpus/script/twice/transform/upper/copy")
	// The owner's normal run, on the old inputs too, passes its frozen
	// cell, and fails on the inputs.
	r = s.normal("tr-jenny")
	s.fails(r, "upper's variant of the base differs from its committed inputs:", "--- PASS: TestCorpus/script/twice/transform/upper/echo")
	s.lacks(r, "The cell is frozen by its known gap")

	// The owner's update rewrites the inputs, and leaves its frozen cell.
	s.fails(s.update("tr-jenny"), frozen("echo", "1"))
	s.hasSections(twice, map[string]string{
		"transform/upper/in/do/out/values.txt": "A1 SIX\nA1 SIX\n",
		"transform/upper/out/echo/values.txt":  "A1 SIX\n",
		"transform/upper/out/copy/values.txt":  "A1 SIX\n",
	})
	s.fails(s.update("tr-jenny"), frozen("echo", "1"))
	s.fails(s.normal("tr-jenny"), frozen("echo", "1"))
	s.fails(s.run("b", "tr-b-jenny", false), frozen("copy", "2"))

	// A person deletes echo's frozen files alone. The root package's
	// normal run fails on the cell, and b's update, which fails on its own
	// frozen cell, leaves echo's unwritten too.
	echoMissing := "echo: Does what the case's do/ tree says.\n    transform/upper/out/echo/outcome has no golden.\n    Run FIXTURE_UPDATE_GOLDEN=1 go test ./... to write it, and read it before committing.\n    transform/upper/out/echo/values.txt has no golden."
	frozenBoth := s.read(twice)
	s.edit(twice, "delete upper's frozen cell of echo", drop("transform/upper/out/echo/"))
	s.fails(s.normal("tr-jenny"), echoMissing)
	s.fails(s.run("b", "tr-b-jenny", true), frozen("copy", "2"))
	s.hasSections(twice, nil, "transform/upper/out/echo/")
	s.write(twice, frozenBoth, "restore echo's frozen cell")

	// A person deletes echo's frozen files, and removes copy's gaps line,
	// so no package's update fails on a frozen cell. Each package's normal
	// run fails on its own cell until its update writes it, and the other
	// package's update, which passes, leaves that cell as it is: first
	// with the root package's update first, then with b's.
	s.edit(twice, "delete upper's frozen cell of echo, and close copy's gap", func(a *archive.Archive) {
		drop("transform/upper/out/echo/")(a)
		set("gaps", "transform/upper echo Waits on sheets#9.\n")(a)
	})
	edited := s.read(twice)
	copyDiffers := "transform/upper/out/copy/values.txt differs (-golden +got):"
	s.fails(s.normal("tr-jenny"), echoMissing)
	s.fails(s.run("b", "tr-b-jenny", false), copyDiffers)
	s.passes(s.update("tr-jenny"))
	s.hasSections(twice, map[string]string{
		"transform/upper/out/echo/values.txt": "A1 SIX\nA1 SIX\n",
		"transform/upper/out/copy/values.txt": "A1 SIX\n",
	})
	s.fails(s.run("b", "tr-b-jenny", false), copyDiffers)
	s.passes(s.run("b", "tr-b-jenny", true))
	s.hasSections(twice, map[string]string{"transform/upper/out/copy/values.txt": "A1 SIX\nA1 SIX\n"})
	s.passes(s.normal("tr-jenny"))
	s.passes(s.run("b", "tr-b-jenny", false))

	s.write(twice, edited, "restore the edited archive, for b's update first")
	s.passes(s.run("b", "tr-b-jenny", true))
	s.hasSections(twice, map[string]string{"transform/upper/out/copy/values.txt": "A1 SIX\nA1 SIX\n"}, "transform/upper/out/echo/")
	s.fails(s.normal("tr-jenny"), echoMissing)
	s.passes(s.update("tr-jenny"))
	s.hasSections(twice, map[string]string{"transform/upper/out/echo/values.txt": "A1 SIX\nA1 SIX\n"})
	s.passes(s.normal("tr-jenny"))
	s.passes(s.run("b", "tr-b-jenny", false))
	s.done()
}

// TestTransformStaleInputs: another package's normal run on a variant
// whose committed inputs differ from what its transform makes notes it
// once for the variant, however a cell fails: here a run that fails its
// test through its testing.TB, a probe that says no with no reason, and a
// frozen cell. The variant's note says what an update does with the
// inputs, and each cell's own note what it does with the cell, which it
// runs on what the transform makes, frozen or not, so the variant's note
// names no cell.
func TestTransformStaleInputs(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.write("b/corpus_test.go", []byte(packageTestFile), "a package b over the corpus")
	s.passes(s.update("transforms"))
	s.passes(s.run("b", "tr-b", true))
	settled := s.read(twice)
	const restaged = "FIXTURE_UPDATE_GOLDEN=1 go test ./... runs copy on another value than this run: it runs the variant on what upper makes now, where a normal run reads the inputs the archive holds. Run it, fix what it reports, and read the diff before committing."

	s.edit(twice, "add a fatal instruction to upper's input by hand", insert("transform/upper/in/do/fatal", "stop\n", "transform/upper/in/do/out/"))
	s.fails(s.run("b", "tr-b", false), "The run failed its test, through its testing.TB.\n"+restaged,
		"A cell of the variant failed, and the archive's inputs for upper's variant differ from what upper makes now:\ntransform/upper/in/do/fatal is no longer made\nA normal run reads the inputs, and an update what upper makes.\nThe module's update, FIXTURE_UPDATE_GOLDEN=1 go test ./..., rewrites them, in the package that owns upper.")

	s.write(twice, settled, "restore the settled archive")
	s.edit(twice, "add an empty probe instruction to upper's input by hand", insert("transform/upper/in/do/probe", "", "transform/upper/out/"))
	s.fails(s.run("b", "tr-b-probe", false), "copy's probe said the case isn't for it, and gave no reason.",
		"A cell of the variant failed, and the archive's inputs for upper's variant differ from what upper makes now:\ntransform/upper/in/do/probe is no longer made\nA normal run reads the inputs, and an update what upper makes.\nThe module's update, FIXTURE_UPDATE_GOLDEN=1 go test ./..., rewrites them, in the package that owns upper.")

	// A frozen cell's own note says the update runs it on what upper
	// makes.
	s.write(twice, settled, "restore the settled archive")
	s.edit(twice, "mark upper's cell of copy, and edit upper's input by hand", func(a *archive.Archive) {
		insert("gaps", "transform/upper copy Waits on sheets#9.\n", "out/")(a)
		set("transform/upper/in/do/out/values.txt", "A1 SEVEN\n")(a)
	})
	const rewrites = "A normal run reads the inputs, and an update what upper makes.\nThe module's update, FIXTURE_UPDATE_GOLDEN=1 go test ./..., rewrites them, in the package that owns upper. If no package owns upper, the root-package check says so, and no update rewrites the inputs.\n"
	r := s.run("b", "tr-b", false)
	s.fails(r, "The cell is frozen by its known gap, gaps line 1: \"transform/upper copy Waits on sheets#9.\"\nIts committed files record today's behavior, and the run now produces different ones:\ntransform/upper/out/copy/values.txt differs (-committed +got):\n- A1 SIX\n+ A1 SEVEN\n"+restaged,
		"A cell of the variant failed, and the archive's inputs for upper's variant differ from what upper makes now:\ntransform/upper/in/do/out/values.txt differs (-committed +made):\n- A1 SEVEN\n+ A1 SIX\n"+rewrites)
	s.lacks(r, "frozen ones", "copy's")

	// Only the cells that failed are noted: not a frozen one that passes.
	s.write(twice, settled, "restore the settled archive")
	s.passes(s.run("b", "tr-b-keep", true))
	s.edit(twice, "mark upper's cells of copy and keep, and add a probe instruction to upper's input by hand", func(a *archive.Archive) {
		insert("gaps", "transform/upper copy Waits on sheets#9.\ntransform/upper keep Waits on sheets#10.\n", "out/")(a)
		insert("transform/upper/in/do/probe", "not for copy\n", "transform/upper/out/")(a)
	})
	r = s.run("b", "tr-b-keep", false)
	s.fails(r, "gaps, line 1, marks the cell, and copy's probe says the case isn't for it: not for copy", rewrites)
	s.lacks(r, "keep:", "--- FAIL: TestCorpus/script/twice/transform/upper/keep")
	s.done()
}

// TestTransformUnowned: a package that doesn't own a transform can't know
// whether another does, so a message that names the update in the package
// that owns it adds what happens when none does: the root-package check
// says so, and no update rewrites the inputs. Here the root package gives
// its transforms up, its check reports them, and neither package's update
// rewrites the inputs. Both halves also hold when the only owner's package
// has no tests (TestTransformOwnerUntested), and when the owner dropped
// the transform from Own (TestTransformOwnerDropped), once its update
// runs; until then that package's normal run fails on its registry. With
// no root-package check registered, nothing says so, and every normal run
// fails for the missing check instead, until the check reports the
// transform (TestTransformUnownedNoRootCheck).
func TestTransformUnowned(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.write("b/corpus_test.go", []byte(packageTestFile), "a package b over the corpus")
	s.passes(s.update("transforms"))
	s.passes(s.run("b", "tr-b", true))
	// The root package gives up its transforms, and its update records it.
	s.passes(s.update("tr-unowned"))
	s.fails(s.normal("tr-unowned"), "the transform upper, of testdata/script/, is owned by no package")
	settled := s.read(twice)

	s.edit(twice, "edit upper's input by hand", set("transform/upper/in/do/out/values.txt", "A1 SEVEN\n"))
	s.fails(s.run("b", "tr-b", false), "transform/upper/out/copy/values.txt differs (-golden +got):",
		"A normal run reads the inputs, and an update what upper makes.\nThe module's update, FIXTURE_UPDATE_GOLDEN=1 go test ./..., rewrites them, in the package that owns upper. If no package owns upper, the root-package check says so, and no update rewrites the inputs.")
	s.passes(s.update("tr-unowned"))
	s.passes(s.run("b", "tr-b", true))
	s.hasSections(twice, map[string]string{"transform/upper/in/do/out/values.txt": "A1 SEVEN\n"})

	s.write(twice, settled, "restore the archive")
	s.edit(twice, "give upper an empty delete by hand", insert("transform/upper/delete", "", "transform/upper/out/"))
	s.fails(s.run("b", "tr-b", false), "script/twice/transform/upper: Uppercases every file under do/out/.\n    The archive's inputs for upper's variant are invalid, and no cell runs on them:\n      transform/upper/delete: the section is empty; a variant that removes nothing has no delete section\n    The module's update, FIXTURE_UPDATE_GOLDEN=1 go test ./..., rewrites them, in the package that owns upper. If no package owns upper, the root-package check says so, and no update rewrites the inputs.")
	s.done()
}

// TestTransformUnownedNoRootCheck: with no root-package check registered,
// nothing reports a transform no package owns, and every normal run fails
// for the missing check instead, beside the note that says the check
// reports it. Once the check is registered, it does. No update rewrites
// the variant's inputs in either state.
func TestTransformUnownedNoRootCheck(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.write("b/corpus_test.go", []byte(packageTestFile), "a package b over the corpus")
	s.passes(s.update("transforms"))
	s.passes(s.run("b", "tr-b", true))
	// The root package gives up its transforms and the root-package check.
	s.passes(s.update("tr-unowned-noroot"))
	missing := "no registry records a root-package check for the module-root corpus, so nothing reports what no column there owns."
	unowned := "the transform upper, of testdata/script/, is owned by no package"
	r := s.normal("tr-unowned-noroot")
	s.fails(r, missing)
	s.lacks(r, unowned)

	s.edit(twice, "edit upper's input by hand", set("transform/upper/in/do/out/values.txt", "A1 SEVEN\n"))
	r = s.run("b", "tr-b", false)
	s.fails(r, missing, "transform/upper/out/copy/values.txt differs (-golden +got):",
		"If no package owns upper, the root-package check says so, and no update rewrites the inputs.")
	s.lacks(r, unowned)
	s.passes(s.update("tr-unowned-noroot"))
	s.passes(s.run("b", "tr-b", true))
	s.hasSections(twice, map[string]string{"transform/upper/in/do/out/values.txt": "A1 SEVEN\n"})

	// The root package registers the check again, and it reports upper.
	s.passes(s.update("tr-unowned"))
	r = s.normal("tr-unowned")
	s.fails(r, unowned)
	s.lacks(r, missing)
	s.hasSections(twice, map[string]string{"transform/upper/in/do/out/values.txt": "A1 SEVEN\n"})
	s.done()
}

// TestTransformOwnerUntested: a registry in a package with no _test.go
// records what no test runs, so its package owns no transform: the
// root-package check reports the transform as owned by no package, beside
// the registry no test compares, and names each package whose registry
// records that it owns the transform, so the fix restores its tests or
// moves the Own, and makes no second owner. No update rewrites the
// variant's inputs.
func TestTransformOwnerUntested(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.write("b/corpus_test.go", []byte(packageTestFile), "a package b over the corpus")
	s.passes(s.update("tr-owns-looped"))
	s.passes(s.run("b", "tr-b-owns", true))
	s.passes(s.normal("tr-owns-looped"))
	s.passes(s.run("b", "tr-b-owns", false))

	s.remove("b/corpus_test.go", "remove b's tests, and leave its registry")
	r := s.normal("tr-owns-looped")
	s.fails(r,
		"b/testdata/hypercase.json is a registry, and b holds no _test.go file, so no test compares it, and it still claims sections.",
		"the transform upper, of testdata/script/, is owned by no package, so nothing writes its variants' inputs, or judges them against what it makes. The registry of ./b records that its package owns the transform, and ./b holds no _test.go file, so no test runs the transform there.\nRestore the tests of ./b, or delete its registry and give the transform to hypercase.Own in another package that registers its corpus type.")
	s.lacks(r, "the transform looped, of testdata/script/, is owned by no package")
	s.edit(twice, "edit upper's input by hand", set("transform/upper/in/do/out/values.txt", "A1 SEVEN\n"))
	s.passes(s.update("tr-owns-looped"))
	s.hasSections(twice, map[string]string{"transform/upper/in/do/out/values.txt": "A1 SEVEN\n"})

	// A second package's registry records it too, and loses its tests.
	s.write("c/corpus_test.go", []byte(packageTestFile), "a package c over the corpus")
	s.passes(s.run("c", "tr-b-owns", true))
	s.remove("c/corpus_test.go", "remove c's tests, and leave its registry")
	s.fails(s.normal("tr-owns-looped"), "the transform upper, of testdata/script/, is owned by no package, so nothing writes its variants' inputs, or judges them against what it makes. The registries of ./b and ./c record that their packages own the transform, and they hold no _test.go file, so no test runs the transform there.\nRestore the tests of one of them and delete the others' registries, or delete all their registries and give the transform to hypercase.Own in another package that registers its corpus type.")
	s.done()
}

// TestTransformListedUntested: a transform that only packages with no
// _test.go file list, in their corpus types, needs no owner, as a lift
// that only they use doesn't: nothing beyond their registries' failures
// is reported.
func TestTransformListedUntested(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.write("b/corpus_test.go", []byte(packageTestFile), "a package b over the corpus")
	// The root package registers no corpus type, and b lists upper and
	// looped, and owns neither.
	s.passes(s.update("root-only"))
	s.passes(s.run("b", "tr-b", true))
	unowned := "the transform upper, of testdata/script/, is owned by no package"
	s.fails(s.normal("root-only"), unowned)

	s.remove("b/corpus_test.go", "remove b's tests, and leave its registry")
	r := s.normal("root-only")
	s.fails(r, "b/testdata/hypercase.json is a registry, and b holds no _test.go file, so no test compares it, and it still claims sections.")
	s.lacks(r, "is owned by no package", "no registry claims", "lists a transform")
	s.done()
}

// TestTransformOwnerDropped: a package that drops a transform from Own,
// with no update, leaves its registry recording that it owns it, so the
// root-package check, which reads the registries, still counts it as the
// owner. The state reports itself: the package's own normal run fails on
// its registry, and names the update, after which the root-package check
// reports the transform owned by no package, and no update rewrites the
// variant's inputs.
func TestTransformOwnerDropped(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "transforms")
	s.write("b/corpus_test.go", []byte(packageTestFile), "a package b over the corpus")
	s.passes(s.update("tr-owns-looped"))
	s.passes(s.run("b", "tr-b-owns", true))
	s.passes(s.normal("tr-owns-looped"))

	// b drops upper from Own, and runs no update.
	s.fails(s.run("b", "tr-b", false), "b/testdata/hypercase.json differs from what the package registers (-committed +registered):",
		"-           \"owned\": true\n+           \"owned\": false",
		"Run FIXTURE_UPDATE_GOLDEN=1 go test ./... to write it, and read the diff before committing.")
	s.passes(s.normal("tr-owns-looped"))
	s.passes(s.run("b", "tr-b", true))
	s.fails(s.normal("tr-owns-looped"), "the transform upper, of testdata/script/, is owned by no package")
	s.edit(twice, "edit upper's input by hand", set("transform/upper/in/do/out/values.txt", "A1 SEVEN\n"))
	s.passes(s.update("tr-owns-looped"))
	s.passes(s.run("b", "tr-b", true))
	s.hasSections(twice, map[string]string{"transform/upper/in/do/out/values.txt": "A1 SEVEN\n"})
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
		"the transform looped, of testdata/script/, is owned by no package, so nothing writes its variants' inputs, or judges them against what it makes.\nGive it to hypercase.Own in one package that registers its corpus type.",
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
// skipped in the others, in their updates and their normal runs.
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
	// A package that doesn't own the transform skips the variant on every
	// failure, in its update and in its normal run, and leaves the owner to
	// report why.
	var skipped []string
	for _, a := range []string{"error", "panic", "same", "flat", "marker", "nest", "probe-panic", "probe-none"} {
		skipped = append(skipped, "script/"+a+"/transform/poke: the transform poke failed on the base, and the package that owns it reports why.")
	}
	for _, update := range []bool{true, false} {
		b := s.run("b", "poke-b", update)
		s.passes(b)
		s.contains(b, skipped...)
	}
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
		{"lift-transform", "panic: hypercase: the lift name transform is reserved: a case's subtests name its variants variant/<name> and transform/<name>. Rename the lift."},
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
		{"tr-unregistered-keeps", "the transform upper, of the corpus type hypercase_test.Script, keeps circular, and the package registers no property of that name.\nRegister the property, or remove it from upper's Keeps."},
		{"tr-other-description", "the transform upper, of the corpus type hypercase_test.Script, keeps a property named circular, which the package registers with another description.\nKeep one declaration of circular, with one description: register it, and list it in upper's Keeps."},
		{"tr-other-gives", "the transform looped, of the corpus type hypercase_test.Script, gives a property named circular, which the package registers with another description.\nKeep one declaration of circular, with one description: register it, and list it in looped's Gives."},
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
