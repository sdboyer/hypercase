package hypercase_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/diff"
	"github.com/sdboyer/hypercase/internal/registry"
)

// Step 10: chaining, over the fixtures of fixtures_chain_test.go. The bytes
// lift import makes a Script of each export, committed under via/import/.

const (
	prices = "testdata/export/prices.txtar"
	names  = "testdata/export/names.txtar"
)

// under returns the names of an archive's sections that start with prefix.
func (s *sandbox) under(rel, prefix string) []string {
	s.t.Helper()
	var got []string
	for _, sec := range archive.Parse(s.read(rel)).Sections {
		if strings.HasPrefix(sec.Name, prefix) {
			got = append(got, sec.Name)
		}
	}
	return got
}

// move moves a section to before the first whose name starts with before.
func move(name, before string) func(a *archive.Archive) {
	return func(a *archive.Archive) {
		i := slices.IndexFunc(a.Sections, func(s archive.Section) bool { return s.Name == name })
		sec := a.Sections[i]
		a.Sections = slices.Delete(a.Sections, i, i+1)
		insert(sec.Name, string(sec.Data), before)(a)
	}
}

// TestChain: a bytes lift's product is committed under via/<lift>/, which
// holds nothing else, at every level of a case, and the lift's own files
// stay under out/<lift>/. A case it rejects has no via/<lift>/. A product
// may use any names, a cell's own files' included, and an empty one is
// committed as no section. The columns past the lift run on the exports,
// suites over Script and *Graph alike, and those over Script and *Graph
// run on the scripts too.
func TestChain(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	u := s.update("chain")
	s.passes(u)
	s.contains(u,
		"--- PASS: TestCorpus/export/prices/import",
		"--- PASS: TestCorpus/export/prices/evaluate",
		"--- PASS: TestCorpus/export/prices/list",
		"--- PASS: TestCorpus/script/plain/evaluate",
		"--- PASS: TestCorpus/script/plain/list",
		"--- PASS: TestCorpus/export/prices/variant/unclosed/import",
		"export/prices/variant/unclosed: evaluate is blocked: the lift import failed on the case, and testdata/export/prices.txtar, variant/unclosed/out/import/error.txt, records its error.")
	s.cat(prices)
	s.cat(names)
	s.cat("testdata/export/empty.txtar")
	s.cat("testdata/hypercase.json")

	if got, want := s.under(prices, "via/"), []string{"via/import/evaluate/out/values.txt", "via/import/quote/A1", "via/import/quote/B1"}; !slices.Equal(got, want) {
		t.Errorf("the base's via/ sections are %q, want %q", got, want)
	}
	s.hasSections(prices, map[string]string{
		"via/import/evaluate/out/values.txt":    "a1 3\nb1 6\n",
		"via/import/quote/B1":                   "=A1*2\n",
		"out/import/outcome":                    "accepted\n",
		"out/import/inputs":                     "via/import/evaluate/out/values.txt\n  in/import/out/evaluate/out/values.txt\nvia/import/quote/A1\n  in/import/out/quote/A1\nvia/import/quote/B1\n  in/import/out/quote/B1\n",
		"out/evaluate/values.txt":               "a1 3\nb1 6\n",
		"variant/unclosed/out/import/error.txt": "import: line 1: unclosed quote\n",
		"variant/unclosed/out/import/outcome":   "rejected\n",
		"variant/unclosed/out/evaluate/outcome": "blocked import\n",
		"variant/unclosed/out/list/outcome":     "blocked import\n",
	}, "variant/unclosed/via/", "out/import/", "via/import/outcome", "via/import/error.txt")
	// A hand-written variant's product is whole: the files equal to the
	// base's are there too, and its cells read it.
	if got, want := s.under(prices, "variant/doubled/via/"), []string{"variant/doubled/via/import/evaluate/out/values.txt", "variant/doubled/via/import/quote/A1", "variant/doubled/via/import/quote/B1"}; !slices.Equal(got, want) {
		t.Errorf("doubled's via/ sections are %q, want %q", got, want)
	}
	s.hasSections(prices, map[string]string{
		"variant/doubled/via/import/evaluate/out/values.txt":  "a1 6\nb1 12\n",
		"variant/doubled/via/import/quote/A1":                 "3\n",
		"variant/doubled/out/evaluate/values.txt":             "a1 6\nb1 12\n",
		"transform/louder/via/import/evaluate/out/values.txt": "A1 3\nB1 6\n",
		"transform/louder/via/import/quote/A1":                "3\n",
		"transform/louder/out/evaluate/values.txt":            "A1 3\nB1 6\n",
	})
	s.hasSections(names, map[string]string{
		"via/import/inputs/x":        "a tree named inputs\n",
		"via/import/names/error.txt": "not an error\n",
		"via/import/names/outcome":   "not an outcome\n",
		"out/list/files.txt":         "3 files\ninputs/x \"a tree named inputs\\n\"\nnames/error.txt \"not an error\\n\"\nnames/outcome \"not an outcome\\n\"\n",
		// A product tree named inputs is no cell's inputs file, which maps it.
		"out/import/inputs": "via/import/inputs/x\n  in/import/out/inputs/x\nvia/import/names/error.txt\n  in/import/out/names/error.txt\nvia/import/names/outcome\n  in/import/out/names/outcome\n",
		"out/list/inputs":   "files.txt\n  via/import/inputs/x\n  via/import/names/error.txt\n  via/import/names/outcome\n",
	})
	s.hasSections("testdata/export/empty.txtar", map[string]string{
		"out/import/outcome": "accepted\n",
		"out/list/files.txt": "0 files\n",
	}, "via/")
	s.hasSections("testdata/hypercase.json", map[string]string{})
	reg := string(s.read("testdata/hypercase.json"))
	for _, want := range []string{
		`"format": 9,`,
		"\"name\": \"import\",\n      \"description\": \"Imports the export as a script, as its import/ tree says.\",\n      \"input\": \"github.com/sdboyer/hypercase_test.Export\",\n      \"product\": \"github.com/sdboyer/hypercase_test.Script\",\n      \"owned\": true,\n      \"bytes\": true,",
		"\"productFiles\": [\n        {\n          \"pattern\": \"evaluate/*\",",
		"\"rejects\": [\n        \"unterminated\"\n      ]",
		"\"name\": \"parse\",\n      \"description\": \"Parses the script, as its parse/ tree says.\",\n      \"input\": \"github.com/sdboyer/hypercase_test.Script\",\n      \"product\": \"*github.com/sdboyer/hypercase_test.Parsed\",\n      \"owned\": true,\n      \"bytes\": false,",
	} {
		if !strings.Contains(reg, want) {
			t.Errorf("the registry lacks %q:\n%s", want, reg)
		}
	}
	s.passes(s.normal("chain"))
	s.done()
}

// TestChainExpectedErrors: a bytes lift's cell goes through the
// expected-error check, and a case's properties hold for the columns it
// reaches through the lift, as through any other. A cell past the lift
// that it blocks isn't checked.
func TestChainExpectedErrors(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	s.passes(s.update("chain"))
	clean := s.read(prices)

	unexpected := "export/prices/variant/unclosed: An unclosed quote, which the importer rejects.\nimport: Imports the export as a script, as its import/ tree says.\nimport returned an error, but the case states no property import rejects:\n  import: line 1: unclosed quote\nFix the code, or state the property on the case and in import's Rejects."
	s.edit(prices, "unclosed states nothing", drop("variant/unclosed/properties"))
	s.fails(s.normal("chain"), unexpected)
	s.write(prices, clean, "restore it")
	s.fails(s.normal("chain-norejects"), unexpected)

	s.edit(prices, "the base states unterminated, and imports", insert("properties", "unterminated\n", "via/"))
	s.fails(s.normal("chain"), "export/prices: One export of prices: import makes a script whose evaluate prints a1 and b1.\nimport: Imports the export as a script, as its import/ tree says.\nimport returned no error, but the case states unterminated, which import rejects.\nFix the code, or correct the case's properties or import's Rejects.")
	s.write(prices, clean, "restore it")

	// looped's product has a cycle, which looped states: import doesn't
	// err, and resolve does, as expected.
	n := s.normal("chain")
	s.passes(n)
	s.contains(n, "--- PASS: TestCorpus/export/looped/import", "--- PASS: TestCorpus/export/looped/resolve")
	s.hasSections("testdata/export/looped.txtar", map[string]string{"out/resolve/error.txt": "A1 -> A1: cycle\n", "out/import/outcome": "accepted\n"})
	s.edit("testdata/export/looped.txtar", "looped states nothing", drop("properties"))
	s.fails(s.normal("chain"), "export/looped: import makes a script with a cycle, which resolve rejects, and the export states.\nresolve: Resolves the parsed script, as its resolve/ tree says.\nresolve returned an error, but the case states no property resolve rejects:\n  A1 -> A1: cycle")

	// unclosed's evaluate is blocked, so the check doesn't run on it: a
	// gaps line that marks it marks a cell that runs nothing.
	s.edit(prices, "a gaps line marks unclosed's evaluate", insert("variant/unclosed/gaps", "evaluate The importer reads a quote to the line's end. Waits on sheets#9.\n", "variant/unclosed/in/"))
	s.fails(s.normal("chain"), "variant/unclosed/gaps, line 1, marks the cell, and the cell is blocked: the lift import, on evaluate's path, has the committed outcome \"rejected\", in variant/unclosed/out/import/outcome, so the cell runs nothing.\nRemove the line, or fix the lift or the case.")
	s.done()
}

// TestChainReadsCommitted: in a normal run, the cells past a bytes lift
// read its committed product, as its committed outcome decides, so a
// change in what the lift makes fails its own column alone. A cell that
// the committed outcome blocks, or a lift that fails on the committed
// product, may run in the update, and its committed files are then no
// orphans.
func TestChainReadsCommitted(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	s.passes(s.update("chain"))
	clean := s.read(prices)

	s.edit(prices, "import makes other values", set("in/import/out/evaluate/out/values.txt", "a1 4\nb1 8\n"))
	r := s.normal("chain")
	s.fails(r,
		"--- FAIL: TestCorpus/export/prices/import",
		"via/import/evaluate/out/values.txt differs (-golden +got):\n- a1 3\n- b1 6\n+ a1 4\n+ b1 8\nIf the change is intended, run FIXTURE_UPDATE_GOLDEN=1 go test ./... and read the diff before committing.")
	s.contains(r, "--- PASS: TestCorpus/export/prices/evaluate", "--- PASS: TestCorpus/export/prices/list")
	s.write(prices, clean, "restore it")

	// The committed outcome decides whether the cells past the lift run,
	// whatever else import's cell commits.
	s.edit(prices, "accepted, with an error committed", insert("out/import/error.txt", "a stale error\n", "out/import/outcome"))
	r = s.normal("chain")
	s.fails(r, "out/import/error.txt records an error, but the run returned none.")
	s.contains(r, "--- PASS: TestCorpus/export/prices/evaluate", "--- PASS: TestCorpus/export/prices/list")
	s.write(prices, clean, "restore it")
	// import accepts prices now, so the update runs the cells past it, and
	// their committed files are no orphans: the advice keeps them.
	keep := updateRuns("evaluate", "import")
	for _, outcome := range []string{"rejected", "panicked"} {
		s.edit(prices, "import's committed outcome is "+outcome, set("out/import/outcome", outcome+"\n"))
		r := s.normal("chain")
		s.fails(r,
			"out/import/outcome differs (-golden +got):\n- "+outcome+"\n+ accepted",
			"out/evaluate/outcome differs (-golden +got):\n- accepted\n+ blocked import",
			"out/evaluate/values.txt is recorded, and the cell is blocked: the lift import, on evaluate's path, has the committed outcome \""+outcome+"\", in out/import/outcome."+keep)
		s.write(prices, clean, "restore it")
	}
	s.edit(prices, "import's cell has no committed outcome", drop("out/import/outcome"))
	s.fails(s.normal("chain"),
		"out/import/outcome has no golden.",
		"out/evaluate/outcome differs (-golden +got):\n- accepted\n+ blocked import",
		"out/evaluate/values.txt is recorded, and the cell is blocked: the lift import, on evaluate's path, has no committed outcome, out/import/outcome, which a normal run reads."+keep)
	s.passes(s.update("chain"))
	s.same(prices, clean)

	// parse rejects the committed product, which import no longer makes,
	// and the update runs evaluate on what import makes now: the committed
	// files of evaluate's cell, which parse blocks, are no orphans either.
	s.edit(prices, "the committed product has parse reject it", insert("via/import/parse/error", "a stale error\n", "via/import/quote/"))
	s.fails(s.normal("chain"),
		"via/import/parse/error is a file of import's committed product, which this run of import doesn't make.",
		"parse returned an error, but the case states no property parse rejects:\n  a stale error\nFIXTURE_UPDATE_GOLDEN=1 go test ./... runs parse on another value than this run: it reruns import, whose outcome and product a normal run reads from the archive. Run it, fix what it reports, and read the diff before committing.",
		"out/evaluate/values.txt is recorded, and the cell is blocked: the lift parse, on evaluate's path, failed on the case."+keep)
	s.passes(s.update("chain"))
	s.same(prices, clean)
	s.done()
}

// TestChainPastAFailingLift: a bytes lift past a lift that fails is
// blocked by it, and so is every cell past the bytes lift, in an update
// and as the archive commits it.
func TestChainPastAFailingLift(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain-past")
	s.passes(s.update("chain-past"))
	s.cat("testdata/export/stuck.txtar")
	s.hasSections("testdata/export/stuck.txtar", map[string]string{
		"out/unpack/error.txt": "line 1: unclosed quote\n",
		"out/unpack/outcome":   "rejected\n",
		"out/repack/outcome":   "blocked unpack\n",
		"out/parse/outcome":    "blocked unpack\n",
		"out/resolve/outcome":  "blocked unpack\n",
		"out/evaluate/outcome": "blocked unpack\n",
		"out/list/outcome":     "blocked unpack\n",
	}, "via/", "out/repack/", "out/list/")
	s.hasSections("testdata/export/packed.txtar", map[string]string{"via/repack/evaluate/out/values.txt": "a1 3\n", "out/evaluate/values.txt": "a1 3\n"})
	n := s.normal("chain-past")
	s.passes(n)
	s.contains(n,
		"export/stuck: repack is blocked: the lift unpack failed on the case, and testdata/export/stuck.txtar, out/unpack/error.txt, records its error.",
		"export/stuck: evaluate is blocked: testdata/export/stuck.txtar, out/repack/outcome, records the lift repack's outcome as \"blocked unpack\".")

	// A bytes lift newly blocked has no product: its committed one fails
	// its column, and the update removes it.
	packed := "testdata/export/packed.txtar"
	s.edit(packed, "unpack now rejects packed, which says so", func(a *archive.Archive) {
		insert("properties", "unterminated\n", "in/")(a)
		insert("in/unpack/error", "line 1: unclosed quote\n", "via/")(a)
	})
	blocked := "via/repack/evaluate/out/values.txt is recorded, and repack is blocked: the lift unpack, on repack's path, failed on the case, so it has no product in this run.\nRun FIXTURE_UPDATE_GOLDEN=1 go test ./..., which removes it, and read the diff before committing."
	s.fails(s.normal("chain-past"), blocked)
	s.fails(s.update("chain-past"), "out/evaluate/values.txt is recorded, and the cell is blocked: the lift unpack, on evaluate's path, failed on the case.")
	s.hasSections(packed, map[string]string{"out/repack/outcome": "blocked unpack\n", "out/evaluate/values.txt": "a1 3\n"}, "via/")
	s.done()
}

// TestChainUpdate: an update's cells past a bytes lift read the product
// it makes, so one update moves the product and what is past it together,
// the normal run after it passes, and a second changes nothing.
func TestChainUpdate(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	s.passes(s.update("chain"))
	s.edit(prices, "import makes other values", set("in/import/out/evaluate/out/values.txt", "a1 4\nb1 8\n"))
	s.passes(s.update("chain"))
	s.hasSections(prices, map[string]string{
		"via/import/evaluate/out/values.txt": "a1 4\nb1 8\n",
		"out/evaluate/values.txt":            "a1 4\nb1 8\n",
	})
	s.passes(s.normal("chain"))
	before := s.snapshot()
	s.passes(s.update("chain"))
	s.unchanged(before)
	s.done()
}

// TestChainTwice: a grid reaches Script through two bytes lifts, export
// then import, each with its product under its via/. In a normal run, a
// committed outcome of export or import that blocks a cell past it may not
// be what the update does: the normal run runs export and import as the
// update does, so when export accepts the grid, the committed files of
// import's cell and list's are their own, which the update keeps, however
// many lifts back the committed outcome blocks them; when export rejects
// it, the update removes import's product.
func TestChainTwice(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain-twice")
	const sheet = "testdata/grid/sheet.txtar"
	s.passes(s.update("chain-twice"))
	s.cat(sheet)
	s.hasSections(sheet, map[string]string{
		"via/export/import/out/quote/A1": "3\n",
		"via/import/quote/A1":            "3\n",
		"out/export/outcome":             "accepted\n",
		"out/import/outcome":             "accepted\n",
	})
	s.passes(s.normal("chain-twice"))
	clean := s.read(sheet)

	s.edit(sheet, "export's cell has no committed outcome", drop("out/export/outcome"))
	s.fails(s.normal("chain-twice"),
		"out/export/outcome has no golden.",
		"out/import/outcome differs (-golden +got):\n- accepted\n+ blocked export",
		"via/import/quote/A1 is recorded, and import is blocked: the lift export, on import's path, has no committed outcome, out/export/outcome, which a normal run reads, so it has no product in this run."+updateRuns("import", "export"))
	s.passes(s.update("chain-twice"))
	s.same(sheet, clean)

	rejects := func(a *archive.Archive) {
		insert("properties", "unterminated\n", "in/")(a)
		insert("in/export/error", "line 1: unclosed quote\n", "in/export/out/")(a)
	}
	s.edit(sheet, "export now rejects the grid, which says so", rejects)
	s.fails(s.update("chain-twice"), "out/list/files.txt is recorded, and the cell is blocked: the lift export, on list's path, failed on the case.")
	s.hasSections(sheet, map[string]string{"out/export/outcome": "rejected\n", "out/import/outcome": "blocked export\n", "out/list/files.txt": "1 files\nquote/A1 \"3\\n\"\n"}, "via/")
	s.edit(sheet, "import's product back by hand", insert("via/import/quote/A1", "3\n", "out/"))
	s.fails(s.normal("chain-twice"), "via/import/quote/A1 is recorded, and import is blocked: the lift export, on import's path, has the committed outcome \"rejected\", in out/export/outcome, so it has no product in this run.\nRun FIXTURE_UPDATE_GOLDEN=1 go test ./..., which removes it, and read the diff before committing.")

	// export accepts the grid again. list is blocked by import's committed
	// outcome, which export's blocks in turn, and the update runs both.
	s.edit(sheet, "export accepts the grid again, and its error is deleted by hand", func(a *archive.Archive) {
		drop("properties")(a)
		drop("in/export/error")(a)
		drop("out/export/error.txt")(a)
	})
	s.fails(s.normal("chain-twice"),
		"out/export/outcome differs (-golden +got):\n- rejected\n+ accepted",
		"via/import/quote/A1 is recorded, and import is blocked: the lift export, on import's path, has the committed outcome \"rejected\", in out/export/outcome, so it has no product in this run."+updateRuns("import", "export"),
		"out/list/files.txt is recorded, and the cell is blocked: the lift import, on list's path, has the committed outcome \"blocked export\", in out/import/outcome."+updateRuns("list", "import"))
	s.passes(s.update("chain-twice"))
	s.same(sheet, clean)
	s.done()
}

// updateRuns is the advice a normal run gives on the committed files of
// a cell it finds blocked, past a bytes lift, when the update runs it.
func updateRuns(col, lift string) string {
	return "\nFIXTURE_UPDATE_GOLDEN=1 go test ./... runs " + col + ": it reruns " + lift + ", whose outcome and product a normal run reads from the archive. Delete nothing by hand before it has run."
}

// TestChainTwiceShared: a package that owns import but only uses export
// doesn't run export in a normal run, so it can't tell whether the update
// runs the cells past import that a committed outcome blocks, and says to
// run the update before deleting their files. The update runs them.
func TestChainTwiceShared(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain-twice")
	const sheet = "testdata/grid/sheet.txtar"
	s.write("b/corpus_test.go", []byte(packageTestFile), "./b, which owns import and list, and uses export")
	update := func() {
		for _, r := range s.parallel(childRun{"", "chain-twice-a", true}, childRun{"b", "chain-twice-b", true}) {
			s.passes(r)
		}
	}
	update()
	s.passes(s.normal("chain-twice-a"))
	s.passes(s.run("b", "chain-twice-b", false))
	clean := s.read(sheet)

	s.edit(sheet, "export now rejects the grid, which says so", func(a *archive.Archive) {
		insert("properties", "unterminated\n", "in/")(a)
		insert("in/export/error", "line 1: unclosed quote\n", "in/export/out/")(a)
	})
	s.passes(s.update("chain-twice-a"))
	s.fails(s.run("b", "chain-twice-b", true), "out/list/files.txt is recorded, and the cell is blocked: the lift export, on list's path, failed on the case.\nIf the cell should run, fix the lift or the case. If not, delete the section by hand; FIXTURE_UPDATE_GOLDEN=1 go test ./... leaves it in place.")
	s.hasSections(sheet, map[string]string{"out/export/outcome": "rejected\n", "out/import/outcome": "blocked export\n", "out/list/files.txt": "1 files\nquote/A1 \"3\\n\"\n"}, "via/")

	s.edit(sheet, "export accepts the grid again, and its error is deleted by hand", func(a *archive.Archive) {
		drop("properties")(a)
		drop("in/export/error")(a)
		drop("out/export/error.txt")(a)
	})
	b := s.run("b", "chain-twice-b", false)
	s.fails(b, "out/list/files.txt is recorded, and the cell is blocked: the lift import, on list's path, has the committed outcome \"blocked export\", in out/import/outcome.\nOnly the package that owns export runs it in a normal run, so this one can't tell what FIXTURE_UPDATE_GOLDEN=1 go test ./... does with list: a normal run reads export's outcome and product from the archive. Delete nothing by hand before it has run.")
	s.lacks(b, "export ran")
	update()
	s.same(sheet, clean)
	s.done()
}

// TestChainTwiceStaleProduct: in a normal run, import runs on export's
// committed product, which may be stale, where the update reruns export
// and runs import on what it makes. So what the normal run finds wrong
// with import's cell may not be what the update finds: it says what the
// update does with import's cell, past export, of import's product, its
// files, its outcome and its error. Here export's committed product lacks
// the import/out/quote/A1 that export makes, so import makes no quote/A1;
// or holds an import/error export doesn't make, so import rejects the
// grid. Each time the update reruns export, runs import as before, and
// changes nothing. When the update blocks import, where export now
// rejects the grid, it removes import's product; when it writes nothing
// for export's cell, where export's jenny now fails, it writes nothing
// for import's either, and leaves the product.
func TestChainTwiceStaleProduct(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain-twice")
	const sheet = "testdata/grid/sheet.txtar"
	s.passes(s.update("chain-twice"))
	clean := s.read(sheet)
	const reruns = "it reruns export, whose outcome and product a normal run reads from the archive"
	another := "FIXTURE_UPDATE_GOLDEN=1 go test ./... runs import on another value than this run: " + reruns + "."

	s.edit(sheet, "export's committed product lacks the import/out/quote/A1 export makes", drop("via/export/import/out/quote/A1"))
	r := s.normal("chain-twice")
	s.fails(r,
		"via/export/import/out/quote/A1 has no golden.",
		"via/import/quote/A1 is a file of import's committed product, which this run of import doesn't make.\n"+another+" Its run may still make the file: delete nothing by hand before it has run.")
	s.lacks(r, "fix import's jenny")
	s.passes(s.update("chain-twice"))
	s.same(sheet, clean)

	s.edit(sheet, "export's committed product holds an import/error export doesn't make", insert("via/export/import/error", "stale\n", "via/export/import/out/"))
	r = s.normal("chain-twice")
	s.fails(r,
		"via/export/import/error is a file of export's committed product, which this run of export doesn't make.\nIf it should, fix export's jenny.",
		"import returned an error, but the case states no property import rejects:\n  import: stale\n"+another+" Run it, fix what it reports, and read the diff before committing.",
		"out/import/error.txt has no golden.\n"+another+" Run it, fix what it reports, and read the diff before committing.",
		"out/import/outcome differs (-golden +got):\n- accepted\n+ rejected\n"+another+" What it writes for the cell, if anything, is its run's.",
		"via/import/quote/A1 is recorded, and import failed on the case in this run, so this run has no product.\n"+another+" Its run may still make the file: delete nothing by hand before it has run.")
	s.lacks(r, "Fix the code, or state the property", "fix import or the case", "If the change is intended")
	s.passes(s.update("chain-twice"))
	s.same(sheet, clean)

	s.edit(sheet, "export now rejects the grid, which says so, and import's committed product holds a quote/Z9", func(a *archive.Archive) {
		insert("properties", "unterminated\n", "in/")(a)
		insert("in/export/error", "line 1: unclosed quote\n", "in/export/out/")(a)
		insert("via/import/quote/Z9", "9\n", "out/")(a)
	})
	r = s.normal("chain-twice")
	s.fails(r,
		"import returned no error, but the case states unterminated, which import rejects.\nFIXTURE_UPDATE_GOLDEN=1 go test ./... blocks import: "+reruns+", and the lift export, on import's path, failed on the case. Run it, fix what it reports, and read the diff before committing.",
		"via/import/quote/Z9 is a file of import's committed product, which this run of import doesn't make.\nFIXTURE_UPDATE_GOLDEN=1 go test ./... blocks import: "+reruns+", and the lift export, on import's path, failed on the case. It removes the file: read the diff before committing.",
		"out/list/files.txt differs (-golden +got):\n- 1 files\n+ 2 files\n  quote/A1 \"3\\n\"\n+ quote/Z9 \"9\\n\"\nFIXTURE_UPDATE_GOLDEN=1 go test ./... blocks list: "+reruns+", and the lift export, on list's path, failed on the case. Run it, fix what it reports, and read the diff before committing.")
	s.lacks(r, "Fix the code, or correct the case's properties")
	s.fails(s.update("chain-twice"), "out/list/files.txt is recorded, and the cell is blocked: the lift export, on list's path, failed on the case.")
	s.hasSections(sheet, map[string]string{"out/import/outcome": "blocked export\n"}, "via/")
	s.write(sheet, clean, "restore it")

	s.edit(sheet, "export's jenny now fails on the grid, and import's committed product holds a quote/Z9", func(a *archive.Archive) {
		insert("in/export/fail", "the generator's error\n", "in/export/out/")(a)
		insert("via/import/quote/Z9", "9\n", "out/")(a)
	})
	r = s.normal("chain-twice")
	s.fails(r, "via/import/quote/Z9 is a file of import's committed product, which this run of import doesn't make.\nFIXTURE_UPDATE_GOLDEN=1 go test ./... writes nothing for import: "+reruns+", and it writes nothing for the cell of the lift export, on import's path. Fix what export's cell reports, and delete nothing by hand before FIXTURE_UPDATE_GOLDEN=1 go test ./... has run.")
	s.lacks(r, "which removes it")
	before := s.snapshot()
	s.fails(s.update("chain-twice"), "The cell of the lift export, on import's path")
	s.unchanged(before)
	s.done()
}

// TestChainVariantAsUpdate: a normal run tells whether the update runs a
// cell it finds blocked on a transform's variant by staging the variant as
// the update does, on what the transform makes, where the normal run reads
// the committed inputs. Here louder's committed inputs differ from what it
// makes, each way: they hold an import/error louder doesn't make, so the
// update runs list, which the committed outcome blocks; or a stale delete
// hides the import/error louder makes, so the update blocks list too, and
// writes the import/error.txt for which the normal run, on the committed
// inputs, finds no error.
func TestChainVariantAsUpdate(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	s.passes(s.update("chain"))
	clean := s.read(prices)

	s.edit(prices, "louder's committed inputs hold an import/error louder doesn't make, and import's committed outcome is rejected", func(a *archive.Archive) {
		insert("transform/louder/in/import/error", "line 1: unclosed quote\n", "transform/louder/in/import/out/")(a)
		set("transform/louder/out/import/outcome", "rejected\n")(a)
	})
	runs := "FIXTURE_UPDATE_GOLDEN=1 go test ./... runs list: it runs the variant on what louder makes now, where a normal run reads the inputs the archive holds, and it reruns import, whose outcome and product a normal run reads from the archive."
	// import runs on the committed inputs, and rejects them, where the
	// update runs it on what louder makes.
	runsImport := "FIXTURE_UPDATE_GOLDEN=1 go test ./... runs import on another value than this run: it runs the variant on what louder makes now, where a normal run reads the inputs the archive holds."
	s.fails(s.normal("chain"),
		"louder's variant of the base differs from its committed inputs:\ntransform/louder/in/import/error is no longer made",
		"import returned an error, but the variant states no property import rejects:\n  import: line 1: unclosed quote\nThe variant states what louder gives, and each property it keeps that the base states.\n"+runsImport+" Run it, fix what it reports, and read the diff before committing.",
		"transform/louder/out/import/error.txt has no golden.\n"+runsImport+" Run it, fix what it reports, and read the diff before committing.",
		"transform/louder/via/import/quote/A1 is recorded, and import failed on the case in this run, so this run has no product.\n"+runsImport+" Its run may still make the file: delete nothing by hand before it has run.",
		"transform/louder/out/list/outcome differs (-golden +got):\n- accepted\n+ blocked import\n"+runs+" What it writes for the cell, if anything, is its run's.",
		"transform/louder/out/list/files.txt is recorded, and the cell is blocked: the lift import, on list's path, has the committed outcome \"rejected\", in transform/louder/out/import/outcome.\n"+runs+" Delete nothing by hand before it has run.")
	s.passes(s.update("chain"))
	s.same(prices, clean)

	s.edit(prices, "import now rejects the base, which says so, and so louder's variant", func(a *archive.Archive) {
		insert("properties", "unterminated\n", "in/")(a)
		insert("in/import/error", "line 2: unclosed quote\n", "in/import/out/")(a)
		insert("variant/doubled/properties", "unterminated\n", "variant/doubled/in/")(a)
	})
	orphan := "transform/louder/out/list/files.txt is recorded, and the cell is blocked: the lift import, on list's path, "
	deleteByHand := "\nIf the cell should run, fix the lift or louder's jenny, which makes the variant. If not, delete the section by hand; FIXTURE_UPDATE_GOLDEN=1 go test ./... leaves it in place."
	s.fails(s.update("chain"), orphan+"failed on the case."+deleteByHand)
	s.hasSections(prices, map[string]string{"transform/louder/out/import/outcome": "rejected\n", "transform/louder/out/list/outcome": "blocked import\n"}, "transform/louder/delete", "transform/louder/in/import/error")
	s.edit(prices, "a stale delete hides the import/error louder makes", insert("transform/louder/delete", "import/error\n", "transform/louder/out/"))
	r := s.normal("chain")
	s.fails(r,
		"louder's variant of the base differs from its committed inputs:\ntransform/louder/delete is no longer made",
		orphan+"has the committed outcome \"rejected\", in transform/louder/out/import/outcome."+deleteByHand,
		"transform/louder/out/import/error.txt records an error, but the run returned none.\nFIXTURE_UPDATE_GOLDEN=1 go test ./... runs import on another value than this run: it runs the variant on what louder makes now, where a normal run reads the inputs the archive holds. Its run may still write the file: delete nothing by hand before it has run.")
	s.lacks(r, "runs list")
	s.fails(s.update("chain"), orphan+"failed on the case."+deleteByHand)
	s.hasSections(prices, map[string]string{"transform/louder/out/import/error.txt": "import: line 2: unclosed quote\n"}, "transform/louder/delete")
	s.done()
}

// TestChainProbeAsUpdate: a normal run tells what the update does with a
// cell it finds blocked or skipped past a bytes lift by the cell's probe
// too, on the value the update stages, and says it of the cell's outcome
// and its files. Here import's committed outcome or product is stale.
// import now makes evaluate/probe, so the update skips evaluate, and
// writes that outcome; or evaluate/probe-panic, so the update fails
// evaluate, whose probe panics, and writes the outcome panicked; or an
// evaluate/probe that gives no reason, or one an outcome can't hold, so
// the update fails evaluate, and writes nothing for it. A stale
// evaluate/probe in the committed product skips evaluate in the normal
// run, or fails its probe there, and the update runs it; or, where import
// makes one too, it gives another reason than import's, and the update
// skips evaluate for import's, and writes that outcome.
func TestChainProbeAsUpdate(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	s.passes(s.update("chain"))
	clean := s.read(prices)
	const reruns = "it reruns import, whose outcome and product a normal run reads from the archive"

	s.edit(prices, "import's committed outcome is stale, and import now makes evaluate/probe", func(a *archive.Archive) {
		set("out/import/outcome", "rejected\n")(a)
		insert("in/import/out/evaluate/probe", "the script says evaluate is not for it\n", "in/import/out/quote/")(a)
	})
	const blocked = "the cell is blocked: the lift import, on evaluate's path, has the committed outcome \"rejected\", in out/import/outcome."
	skips := "FIXTURE_UPDATE_GOLDEN=1 go test ./... skips evaluate: " + reruns + ", and evaluate's probe says the case isn't for it: the script says evaluate is not for it"
	r := s.normal("chain")
	s.fails(r,
		"out/evaluate/outcome differs (-golden +got):\n- accepted\n+ blocked import\n"+skips+"\nIt writes the outcome \"skipped the script says evaluate is not for it\", not this one.",
		"out/evaluate/values.txt is recorded, and "+blocked+"\n"+skips+"\nIf the suite should run on the case, fix the probe. If not, delete the section by hand; FIXTURE_UPDATE_GOLDEN=1 go test ./... leaves it in place.",
		"out/list/files.txt is recorded, and the cell is blocked: the lift import, on list's path, has the committed outcome \"rejected\", in out/import/outcome."+updateRuns("list", "import"))
	s.lacks(r, "runs evaluate")
	s.fails(s.update("chain"), "out/evaluate/values.txt is recorded, and evaluate's probe says the case isn't for it: the script says evaluate is not for it\nIf the suite should run on the case, fix the probe. If not, delete the section by hand; FIXTURE_UPDATE_GOLDEN=1 go test ./... leaves it in place.")
	s.hasSections(prices, map[string]string{"out/evaluate/outcome": "skipped the script says evaluate is not for it\n", "out/evaluate/values.txt": "a1 3\nb1 6\n"})
	s.write(prices, clean, "restore it")

	s.edit(prices, "import's committed outcome is stale, and import now makes evaluate/probe-panic", func(a *archive.Archive) {
		set("out/import/outcome", "rejected\n")(a)
		insert("in/import/out/evaluate/probe-panic", "the probe gives up\n", "in/import/out/quote/")(a)
	})
	fails := "FIXTURE_UPDATE_GOLDEN=1 go test ./... fails evaluate: " + reruns + ", and evaluate's probe panics: the probe gives up. Fix the probe."
	r = s.normal("chain")
	s.fails(r,
		"out/evaluate/outcome differs (-golden +got):\n- accepted\n+ blocked import\n"+fails+" It writes the outcome \"panicked\", not this one, and the panic in error.txt.",
		"out/evaluate/values.txt is recorded, and "+blocked+"\n"+fails)
	s.lacks(r, "runs evaluate")
	s.fails(s.update("chain"),
		"evaluate's probe panicked: the probe gives up\nFix the probe.",
		"out/evaluate/values.txt is an output the run no longer produces.")
	s.hasSections(prices, map[string]string{"out/evaluate/outcome": "panicked\n", "out/evaluate/error.txt": "the probe gives up\n", "out/evaluate/values.txt": "a1 3\nb1 6\n"})
	s.write(prices, clean, "restore it")

	// A probe that says no with no reason, or with one an outcome can't
	// hold, fails its cell, and the update writes nothing for it.
	for _, tt := range []struct{ what, probe, fault string }{
		{"no reason", "\n", "evaluate's probe says the case isn't for it, and gives no reason"},
		{"a reason an outcome can't hold", "one\ntwo\n", "evaluate's probe says the case isn't for it, with a reason an outcome can't hold: \"one\\ntwo\" can't be held in an outcome: it is one line, not empty, with no space at either end"},
	} {
		s.edit(prices, "import's committed outcome is stale, and import now makes an evaluate/probe that gives "+tt.what, func(a *archive.Archive) {
			set("out/import/outcome", "rejected\n")(a)
			insert("in/import/out/evaluate/probe", tt.probe, "in/import/out/quote/")(a)
		})
		fails := "FIXTURE_UPDATE_GOLDEN=1 go test ./... fails evaluate: " + reruns + ", and " + tt.fault + ". Fix the probe."
		r = s.normal("chain")
		s.fails(r,
			"out/evaluate/outcome differs (-golden +got):\n- accepted\n+ blocked import\n"+fails+" It writes nothing for the cell.",
			"out/evaluate/values.txt is recorded, and "+blocked+"\n"+fails)
		s.lacks(r, "runs evaluate")
		s.fails(s.update("chain"), "evaluate's probe said the case isn't for it")
		s.hasSections(prices, map[string]string{"out/evaluate/outcome": "accepted\n", "out/evaluate/values.txt": "a1 3\nb1 6\n"}, "out/evaluate/error.txt")
		s.write(prices, clean, "restore it")
	}

	// A stale evaluate/probe in the committed product fails evaluate's
	// probe in the normal run, and the update runs evaluate.
	const badReason = "evaluate's probe said the case isn't for it, with a reason an outcome can't hold: \"one\\ntwo\" can't be held in an outcome: it is one line, not empty, with no space at either end."
	s.edit(prices, "import's committed product holds an evaluate/probe, which import doesn't make, with a reason an outcome can't hold", insert("via/import/evaluate/probe", "one\ntwo\n", "via/import/quote/"))
	r = s.normal("chain")
	s.fails(r,
		badReason+"\nFIXTURE_UPDATE_GOLDEN=1 go test ./... runs evaluate: "+reruns+". What it writes for the cell, if anything, is its run's.",
		"out/evaluate/values.txt is recorded, and evaluate's probe says the case isn't for it, with a reason an outcome can't hold: \"one\\ntwo\" can't be held in an outcome: it is one line, not empty, with no space at either end.\nFIXTURE_UPDATE_GOLDEN=1 go test ./... runs evaluate: "+reruns+". Delete nothing by hand before it has run.")
	s.lacks(r, "An update writes nothing for this cell")
	s.passes(s.update("chain"))
	s.same(prices, clean)
	s.edit(prices, "import's committed product holds an empty evaluate/probe, which import doesn't make, and evaluate's cell has no values.txt", func(a *archive.Archive) {
		insert("via/import/evaluate/probe", "\n", "via/import/quote/")(a)
		drop("out/evaluate/values.txt")(a)
	})
	r = s.normal("chain")
	s.fails(r, "evaluate's probe said the case isn't for it, and gave no reason.\nFIXTURE_UPDATE_GOLDEN=1 go test ./... runs evaluate: "+reruns+". What it writes for the cell, if anything, is its run's.")
	s.passes(s.update("chain"))
	s.same(prices, clean)
	// Where the update's probe fails the same way, it writes nothing for
	// the cell, as the normal run says.
	s.edit(prices, "import makes the evaluate/probe its committed product holds, with a reason an outcome can't hold", func(a *archive.Archive) {
		insert("in/import/out/evaluate/probe", "one\ntwo\n", "in/import/out/quote/")(a)
		insert("via/import/evaluate/probe", "one\ntwo\n", "via/import/quote/")(a)
	})
	r = s.normal("chain")
	s.fails(r, badReason+"\nFix the probe. An update writes nothing for this cell.")
	s.lacks(r, "runs evaluate")
	s.write(prices, clean, "restore it")

	s.edit(prices, "import's committed product holds an evaluate/probe import doesn't make", insert("via/import/evaluate/probe", "a stale reason\n", "via/import/quote/"))
	runs := "FIXTURE_UPDATE_GOLDEN=1 go test ./... runs evaluate: " + reruns + "."
	r = s.normal("chain")
	s.fails(r,
		"via/import/evaluate/probe is a file of import's committed product, which this run of import doesn't make.",
		"out/evaluate/outcome differs (-golden +got):\n- accepted\n+ skipped a stale reason\n"+runs+" What it writes for the cell, if anything, is its run's.",
		"out/evaluate/values.txt is recorded, and evaluate's probe says the case isn't for it: a stale reason\n"+runs+" Delete nothing by hand before it has run.")
	s.lacks(r, "delete the section by hand")
	s.passes(s.update("chain"))
	s.same(prices, clean)

	s.edit(prices, "import now makes evaluate/probe, and evaluate's cells have no values.txt or inputs", func(a *archive.Archive) {
		insert("in/import/out/evaluate/probe", "the new reason\n", "in/import/out/quote/")(a)
		for _, tree := range []string{"", "variant/doubled/", "transform/louder/"} {
			drop(tree + "out/evaluate/values.txt")(a)
			drop(tree + "out/evaluate/inputs")(a)
		}
	})
	s.passes(s.update("chain"))
	s.hasSections(prices, map[string]string{"out/evaluate/outcome": "skipped the new reason\n", "via/import/evaluate/probe": "the new reason\n"})
	probed := s.read(prices)
	s.edit(prices, "import's committed product gives evaluate/probe another reason", set("via/import/evaluate/probe", "a stale reason\n"))
	r = s.normal("chain")
	s.fails(r, "out/evaluate/outcome differs (-golden +got):\n- skipped the new reason\n+ skipped a stale reason\nFIXTURE_UPDATE_GOLDEN=1 go test ./... skips evaluate: "+reruns+", and evaluate's probe says the case isn't for it: the new reason\nIt writes the outcome \"skipped the new reason\", not this one.")
	s.lacks(r, "+ skipped a stale reason\nIf the change is intended")
	s.passes(s.update("chain"))
	s.same(prices, probed)
	s.done()
}

// TestChainOutcomeAsUpdate: when the update blocks a cell a normal run
// finds blocked past a bytes lift, by another lift, the normal run says
// what outcome the update writes. When the update runs the cell, it says
// that what the update writes is the run's, which may be nothing. Here
// import's committed outcome is stale, and import now makes a script that
// resolve rejects; or import's jenny now fails, so the update writes
// nothing for import's cell, nor for list's past it; or import makes a
// script whose evaluate fails its test, so the update writes nothing for
// evaluate's cell.
func TestChainOutcomeAsUpdate(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	s.passes(s.update("chain"))
	const names = "testdata/export/names.txtar"
	const reruns = "it reruns import, whose outcome and product a normal run reads from the archive"
	clean := s.read(names)

	s.edit(names, "import's committed outcome is stale, import now makes a script with a cycle, which the export states, and evaluate's inputs, which its blocked cell won't have, is deleted", func(a *archive.Archive) {
		set("out/import/outcome", "rejected\n")(a)
		drop("out/evaluate/inputs")(a)
		insert("properties", "circular\n", "in/")(a)
		insert("in/import/out/resolve/error", "A1 -> A1: cycle\n", "out/")(a)
	})
	r := s.normal("chain")
	s.fails(r, "out/evaluate/outcome differs (-golden +got):\n- accepted\n+ blocked import\nFIXTURE_UPDATE_GOLDEN=1 go test ./... blocks evaluate: "+reruns+", and the lift resolve, on evaluate's path, failed on the case. It writes the outcome \"blocked resolve\", not this one.")
	s.lacks(r, "runs evaluate")
	s.passes(s.update("chain"))
	s.hasSections(names, map[string]string{"out/evaluate/outcome": "blocked resolve\n", "out/resolve/outcome": "rejected\n"})
	s.write(names, clean, "restore it")

	s.edit(names, "import's committed outcome is stale, and import's jenny now fails on names", func(a *archive.Archive) {
		set("out/import/outcome", "rejected\n")(a)
		insert("properties", "unterminated\n", "in/")(a)
		insert("in/import/fail", "the generator's error\n", "via/")(a)
	})
	nothing := "FIXTURE_UPDATE_GOLDEN=1 go test ./... writes nothing for list: " + reruns + ", and it writes nothing for the cell of the lift import, on list's path."
	r = s.normal("chain")
	s.fails(r,
		"out/list/outcome differs (-golden +got):\n- accepted\n+ blocked import\n"+nothing+" Fix what import's cell reports.",
		"out/list/files.txt is recorded, and the cell is blocked: the lift import, on list's path, has the committed outcome \"rejected\", in out/import/outcome.\n"+nothing+" Fix what import's cell reports, and delete nothing by hand before FIXTURE_UPDATE_GOLDEN=1 go test ./... has run.")
	s.lacks(r, "runs list")
	before := s.snapshot()
	s.fails(s.update("chain"), listHeldBack)
	s.unchanged(before)
	s.write(names, clean, "restore it")

	s.edit(prices, "import's committed outcome is stale, and import now makes a script whose evaluate fails its test", func(a *archive.Archive) {
		set("out/import/outcome", "rejected\n")(a)
		insert("in/import/out/evaluate/fatal", "evaluate gives up\n", "in/import/out/quote/")(a)
	})
	s.fails(s.normal("chain"), "out/evaluate/outcome differs (-golden +got):\n- accepted\n+ blocked import\nFIXTURE_UPDATE_GOLDEN=1 go test ./... runs evaluate: "+reruns+". What it writes for the cell, if anything, is its run's.")
	s.fails(s.update("chain"), "The run failed its test, through its testing.TB.\nFix the code. An update writes nothing for this cell.")
	s.hasSections(prices, map[string]string{"out/import/outcome": "accepted\n", "out/evaluate/outcome": "accepted\n", "out/evaluate/values.txt": "a1 3\nb1 6\n"})
	s.done()
}

// TestChainOrphanAsUpdate: a cell past a bytes lift that runs on a stale
// committed product, and no longer produces a file it commits, may write
// it in the update, which runs the cell on what the lift makes: the
// normal run says to delete nothing by hand. When the committed product
// is what the lift makes, the update's run is this run, and the file is
// an orphan.
func TestChainOrphanAsUpdate(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	s.passes(s.update("chain"))
	clean := s.read(prices)

	s.edit(prices, "import's committed product lacks evaluate/out/values.txt, which import makes", drop("via/import/evaluate/out/values.txt"))
	r := s.normal("chain")
	s.fails(r,
		"via/import/evaluate/out/values.txt has no golden.",
		"out/evaluate/values.txt is an output the run no longer produces.\nFIXTURE_UPDATE_GOLDEN=1 go test ./... runs evaluate on another value than this run: it reruns import, whose outcome and product a normal run reads from the archive. Its run may still write the file: delete nothing by hand before it has run.")
	s.lacks(r, "delete the section by hand")
	s.passes(s.update("chain"))
	s.same(prices, clean)

	s.edit(prices, "evaluate's cell commits a file evaluate doesn't make", insert("out/evaluate/stray.txt", "a stray\n", "out/evaluate/values.txt"))
	orphan := "out/evaluate/stray.txt is an output the run no longer produces.\nIf the run should still write it, fix the code. If not, delete the section by hand; FIXTURE_UPDATE_GOLDEN=1 go test ./... leaves it in place."
	r = s.normal("chain")
	s.fails(r, orphan)
	s.lacks(r, "runs evaluate on another value")
	s.fails(s.update("chain"), orphan)
	s.hasSections(prices, map[string]string{"out/evaluate/stray.txt": "a stray\n"})
	s.done()
}

// TestChainLiftNowRejects: an update removes the product of a lift that
// now fails, and the cells past it are blocked, their committed files
// orphans that it leaves. A committed product beside a rejection fails
// the lift's column.
func TestChainLiftNowRejects(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	s.passes(s.update("chain"))
	s.edit(names, "import now rejects names, which says so", func(a *archive.Archive) {
		insert("properties", "unterminated\n", "in/")(a)
		insert("in/import/error", "line 2: unclosed quote\n", "in/import/out/")(a)
	})
	orphan := "out/list/files.txt is recorded, and the cell is blocked: "
	u := s.update("chain")
	s.fails(u, orphan+"the lift import, on list's path, failed on the case.\nIf the cell should run, fix the lift or the case. If not, delete the section by hand; FIXTURE_UPDATE_GOLDEN=1 go test ./... leaves it in place.")
	s.cat(names)
	s.hasSections(names, map[string]string{
		"out/import/error.txt": "import: line 2: unclosed quote\n",
		"out/list/outcome":     "blocked import\n",
		"out/list/files.txt":   "3 files\n",
	}, "via/")
	// The committed rejection is what import does now, so the files of the
	// cells past it are orphans, in a normal run as in the update.
	s.fails(s.normal("chain"), orphan+"the lift import, on list's path, has the committed outcome \"rejected\", in out/import/outcome.\nIf the cell should run, fix the lift or the case. If not, delete the section by hand; FIXTURE_UPDATE_GOLDEN=1 go test ./... leaves it in place.")

	s.edit(names, "a product beside the rejection", insert("via/import/inputs/x", "a tree named inputs\n", "out/"))
	r := s.normal("chain")
	s.fails(r, "via/import/inputs/x is recorded, and import failed on the case in this run, so this run has no product.\nIf it should make one, fix import or the case. If not, run FIXTURE_UPDATE_GOLDEN=1 go test ./..., which removes it, and read the diff before committing.", orphan)
	s.fails(s.update("chain"), orphan)
	s.hasSections(names, map[string]string{}, "via/")
	s.done()
}

// TestChainDeclined: a transform that now declines a base has no variant,
// so import's owner's update removes the variant's product, which a normal
// run fails, as louder's owner's does its inputs.
func TestChainDeclined(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	s.passes(s.update("chain"))
	s.edit(prices, "the base makes no output of evaluate, so louder declines it", drop("in/import/out/evaluate/"))
	s.fails(s.normal("chain"), "transform/louder/via/import/quote/A1 is recorded, and louder declines the base: the export makes no output of evaluate\nRun FIXTURE_UPDATE_GOLDEN=1 go test ./..., which removes it, and read the diff before committing.")
	s.fails(s.update("chain"), "transform/louder/out/evaluate/values.txt is recorded, and louder declines the base: the export makes no output of evaluate")
	s.hasSections(prices, map[string]string{"transform/louder/out/evaluate/values.txt": "A1 3\nB1 6\n"}, "transform/louder/via/", "transform/louder/in/")
	s.done()
}

// TestChainFrozen: a gaps line that marks a bytes lift's cell freezes its
// product with its files. No update writes either; one whose product
// changed writes the cells past it from the new product, which then fail
// in normal runs until the frozen cell is resolved.
func TestChainFrozen(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	s.passes(s.update("chain"))
	s.edit(prices, "a gaps line marks import's cell", insert("gaps", "import The importer reads quoted fields. Waits on sheets#9.\n", "via/"))
	s.passes(s.normal("chain"))
	frozen := s.read(prices)
	s.passes(s.update("chain"))
	s.same(prices, frozen)

	s.edit(prices, "import makes other values", set("in/import/out/evaluate/out/values.txt", "a1 4\nb1 8\n"))
	changed := "The cell is frozen by its known gap, gaps line 1: \"import The importer reads quoted fields. Waits on sheets#9.\"\nIts committed files record today's behavior, and the run now produces different ones:\nvia/import/evaluate/out/values.txt differs (-committed +got):\n- a1 3\n- b1 6\n+ a1 4\n+ b1 8\nThe cells past import fail in normal runs until the frozen cell is resolved: an update writes them from what import makes now, and leaves the cell's committed outcome and product in place, which a normal run reads.\nIf the gap is closed, remove the line. If the wrong behavior changed shape, delete the cell's files and its product, under out/import/ and via/import/."
	n := s.normal("chain")
	s.fails(n, changed)
	s.contains(n, "--- PASS: TestCorpus/export/prices/evaluate")
	s.fails(s.update("chain"), changed)
	s.hasSections(prices, map[string]string{
		"via/import/evaluate/out/values.txt": "a1 3\nb1 6\n",
		"out/evaluate/values.txt":            "a1 4\nb1 8\n",
	})
	s.fails(s.normal("chain"), changed, "--- FAIL: TestCorpus/export/prices/evaluate", "out/evaluate/values.txt differs (-golden +got):\n- a1 4\n- b1 8\n+ a1 3\n+ b1 6")

	s.edit(prices, "the gap is closed", drop("gaps"))
	s.passes(s.update("chain"))
	s.hasSections(prices, map[string]string{
		"via/import/evaluate/out/values.txt": "a1 4\nb1 8\n",
		"out/evaluate/values.txt":            "a1 4\nb1 8\n",
	})
	s.passes(s.normal("chain"))
	s.done()
}

// TestChainFrozenOutcome: the cells past a frozen bytes lift's cell part
// from normal runs whenever the cell's outcome accepts differently, as
// they do when its product changes: an update writes them from what the
// lift does now, and a normal run reads what the cell commits. empty's
// product is empty, so only the outcome tells the two apart, in either
// direction.
func TestChainFrozenOutcome(t *testing.T) {
	t.Parallel()
	const empty = "testdata/export/empty.txtar"
	const gapsLine = "import The importer reads quoted fields. Waits on sheets#9.\n"
	const note = "The cells past import fail in normal runs until the frozen cell is resolved: an update writes them from what import makes now, and leaves the cell's committed outcome and product in place, which a normal run reads."
	rejects := func(a *archive.Archive) {
		insert("properties", "unterminated\n", "in/")(a)
		insert("in/import/error", "line 1: unclosed quote\n", "in/import/note")(a)
	}

	t.Run("accepted to rejected", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "chain")
		s.passes(s.update("chain"))
		s.edit(empty, "a gaps line marks import's cell", insert("gaps", gapsLine, "in/"))
		s.passes(s.normal("chain"))
		s.edit(empty, "import now rejects empty", rejects)
		s.fails(s.normal("chain"), "out/import/outcome differs (-committed +got):\n- accepted\n+ rejected", note)
		s.fails(s.update("chain"), note)
		s.hasSections(empty, map[string]string{"out/import/outcome": "accepted\n", "out/list/outcome": "blocked import\n"})
		s.fails(s.normal("chain"), note, "--- FAIL: TestCorpus/export/empty/list")
		s.done()
	})

	t.Run("rejected to accepted", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "chain")
		s.edit(empty, "import rejects empty", rejects)
		s.passes(s.update("chain"))
		s.edit(empty, "a gaps line marks import's cell", insert("gaps", gapsLine, "in/"))
		s.passes(s.normal("chain"))
		s.edit(empty, "import accepts empty again, with an empty product", func(a *archive.Archive) {
			drop("properties")(a)
			drop("in/import/error")(a)
		})
		s.fails(s.normal("chain"), "out/import/outcome differs (-committed +got):\n- rejected\n+ accepted", note)
		s.fails(s.update("chain"), note)
		s.hasSections(empty, map[string]string{"out/import/outcome": "rejected\n", "out/list/outcome": "accepted\n"})
		s.fails(s.normal("chain"), note, "--- FAIL: TestCorpus/export/empty/list")
		s.done()
	})
}

// TestChainSectionOrder: a case's via/ sections sit between its inputs and
// its outputs. A normal run fails on one out of place, and an update puts
// it in place, changing no section's bytes.
func TestChainSectionOrder(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	s.passes(s.update("chain"))
	placed := s.read(prices)
	for _, tt := range []struct{ why, section, before string }{
		{"the base's product after its outputs", "via/import/quote/A1", "variant/"},
		{"a variant's product before its hand-written sections", "variant/doubled/via/import/quote/A1", "variant/doubled/about"},
		{"a transform's variant's product before its inputs", "transform/louder/via/import/quote/A1", "transform/louder/in/"},
	} {
		s.edit(prices, tt.why, move(tt.section, tt.before))
		s.fails(s.normal("chain"), "testdata/export/prices.txtar has update-written sections out of their place.\nRun FIXTURE_UPDATE_GOLDEN=1 go test ./... to put them in place.")
		s.passes(s.update("chain"))
		s.same(prices, placed)
	}
	s.done()
}

// TestChainRenderError: past a bytes lift, RenderError receives the
// lift's product as the view: the one it makes in an update, and the one
// committed in a normal run.
func TestChainRenderError(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain-render")
	quoted := "testdata/export/quoted.txtar"
	s.passes(s.update("chain-render"))
	s.hasSections(quoted, map[string]string{"out/resolve/error.txt": "rendered: A1 -> A1: cycle\n  quote: the product's text\n"})
	s.passes(s.normal("chain-render"))
	s.edit(quoted, "the committed product's text differs", set("via/import/quote/text", "the committed text\n"))
	s.fails(s.normal("chain-render"), "out/resolve/error.txt differs (-golden +got):\nrendered: A1 -> A1: cycle\n-   quote: the product's text\n+   quote: the committed text")
	s.done()
}

// TestChainHeldBack: an update writes the cells past a bytes lift only
// when it writes the lift's cell, since a normal run reads what that cell
// commits. Here import now rejects quoted, and the renderer panics on its
// error, so no update writes import's cell, and its committed product and
// outcome stay: the cells past it run nothing, and the update writes
// nothing for them.
func TestChainHeldBack(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain-render")
	quoted := "testdata/export/quoted.txtar"
	s.passes(s.update("chain-render"))
	s.edit(quoted, "import now rejects quoted, and the renderer panics on its error", func(a *archive.Archive) {
		set("properties", "circular\nunterminated\n")(a)
		insert("in/import/error", "line 1: unclosed quote\n", "in/quote/")(a)
		insert("in/render/panic", "the renderer panicked\n", "via/")(a)
	})
	before := s.snapshot()
	u := s.update("chain-render")
	s.fails(u, "The project's RenderError panicked on import's error: the renderer panicked\nFix RenderError. An update writes nothing for this cell.")
	for _, col := range []string{"evaluate", "parse", "resolve"} {
		s.fails(u, "The cell of the lift import, on "+col+"'s path, has a problem, which that cell reports, in the package that owns it. A normal run reads the product and the outcome the archive commits for import, so this cell runs nothing.\nFix what import's cell reports.")
	}
	s.unchanged(before)
	s.done()
}

// TestChainFrozenHeldBack: a frozen bytes lift's cell whose run differs,
// and that an update couldn't write even unfrozen, holds the cells past
// it back, so it doesn't say an update writes them.
func TestChainFrozenHeldBack(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain-render")
	quoted := "testdata/export/quoted.txtar"
	s.passes(s.update("chain-render"))
	s.edit(quoted, "a gaps line marks import's cell, import now rejects quoted, and the renderer panics on its error", func(a *archive.Archive) {
		insert("gaps", "import The importer reads quoted fields. Waits on sheets#9.\n", "in/")(a)
		insert("in/import/error", "line 1: unclosed quote\n", "in/quote/")(a)
		insert("in/render/panic", "the renderer panicked\n", "via/")(a)
	})
	before := s.snapshot()
	u := s.update("chain-render")
	s.fails(u, "via/import/resolve/error is no longer produced\nFix the problems above. An update writes nothing for this cell, which a gaps line freezes.",
		"The cell of the lift import, on parse's path")
	s.lacks(u, "The cells past import fail in normal runs")
	s.unchanged(before)
	s.done()
}

// TestChainTwoPaths: from a corpus type, a path through a bytes lift
// counts, so a second path to a type a column consumes fails the run
// before any case runs.
func TestChainTwoPaths(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	r := s.normal("chain-two-paths")
	s.fails(r, "from the corpus type hypercase_test.Export, of testdata/export/, *hypercase_test.Parsed, which the suite format consumes, is reached by 2 paths:\n  hypercase_test.Export → import → hypercase_test.Script → parse → *hypercase_test.Parsed\n  hypercase_test.Export → import-parse → *hypercase_test.Parsed",
		"No case runs until each type a column consumes is reached by one path.")
	s.lacks(r, "--- PASS: TestCorpus/", "--- FAIL: TestCorpus/export/")
	s.done()
}

// TestChainProductProblems: a product an archive can't hold fails the
// lift's cell, naming the file. A normal run's cells past the lift read
// the committed product still; an update writes nothing for the lift, nor
// for the cells past it, which fail.
func TestChainProductProblems(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	s.passes(s.update("chain"))
	clean := s.read(names)
	for _, tt := range []struct{ why, section, want string }{
		{"an undeclared file", "in/import/out/stray/x", "via/import/stray/x is not declared.\nDeclare it in import's Product, with a description, or fix import's jenny. An update writes nothing for this cell."},
		{"a file in the tree params", "in/import/out/params/x", "via/import/params/x: the tree name params is reserved in every input view, a product's included.\nFix import's jenny: what it makes is its product's input view, with the trees as its top-level directories. An update writes nothing for this cell."},
		{"a file outside any tree", "in/import/out/x", "via/import/x: a product's file belongs in a tree, as via/import/<tree>/<file>.\nFix import's jenny"},
		{"a file below a file", "in/import/nest/quote/x", "via/import/quote/x: the section is also the directory of via/import/quote/x/below.\nFix import's jenny"},
		{"a file with no final newline", "in/import/nonl/quote/x", "via/import/quote/x can't be held in an archive: it has no final newline, which an archive adds.\nFix import's jenny so the file can be held. An update writes nothing for this cell."},
		{"a line shaped like a section marker", "in/import/marker/quote/x", "via/import/quote/x can't be held in an archive: its line 2 is shaped like a section marker, which starts another section when the archive is read.\nFix import's jenny so the file can be held. An update writes nothing for this cell."},
		{"a name with a space after it", "in/import/space/quote/x", "\"via/import/quote/x \" can't be held in an archive: its name reads back as \"via/import/quote/x\", since an archive trims the spaces around a section's name.\nFix import's jenny so the file's name can be held. An update writes nothing for this cell."},
		{"a name with a newline", "in/import/newline/quote/x", "\"via/import/quote/x\\nx\" can't be held in an archive: its name holds a newline, which ends a section's marker line.\nFix import's jenny so the file's name can be held. An update writes nothing for this cell."},
	} {
		s.edit(names, "import makes "+tt.why, insert(tt.section, "x\n", "via/"))
		n := s.normal("chain")
		s.fails(n, "--- FAIL: TestCorpus/export/names/import", tt.want)
		s.contains(n, "--- PASS: TestCorpus/export/names/list")
		before := s.snapshot()
		s.fails(s.update("chain"), tt.want, listHeldBack)
		s.unchanged(before)
		s.write(names, clean, "restore it")
	}
	s.done()
}

// listHeldBack is how an update fails names' list, which it holds back
// when it writes nothing for import's cell.
const listHeldBack = "export/names: import makes files named as a cell's own files are, which list reads.\nlist: Lists the files of the script's input view, with their bytes.\nThe cell of the lift import, on list's path, has a problem, which that cell reports, in the package that owns it. A normal run reads the product and the outcome the archive commits for import, so this cell runs nothing.\nFix what import's cell reports."

// TestChainJennyFaults: a bytes lift's jenny rejects a case only by an
// error decl.Reject marks. Any other error, jenny's own included, is a
// fault of the jenny: it fails the lift's cell even on a case that states
// a property the lift rejects, and an update writes nothing for the cell,
// nor for the cells past it.
func TestChainJennyFaults(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	s.passes(s.update("chain"))
	clean := s.read(names)
	fault := func(err string) string {
		return "import's jenny failed on the case, with an error that is no rejection:\n  " + err + "\nIf import rejects the case, return the error as decl.Reject(err). If not, fix import's jenny. An update writes nothing for this cell."
	}
	for _, tt := range []struct{ why, section, want string }{
		{"two files at one path", "in/import/dup/quote/x", fault("import returned invalid Files: multiple files at path quote/x from jennies: 'import', 'import'")},
		{"two files whose paths differ only by case", "in/import/case/quote/x", fault("import returned invalid Files: multiple files at path quote/x from jennies: 'import', 'import'")},
		{"a tree that fails to render", "in/import/render/quote/x", fault(`rendering quote/x from "import" failed: the tree won't render`)},
		{"an error decl.Reject doesn't mark", "in/import/fail", fault("import: the generator's error")},
	} {
		s.edit(names, "names states unterminated, and import makes "+tt.why, func(a *archive.Archive) {
			insert("properties", "unterminated\n", "in/")(a)
			insert(tt.section, "the generator's error\n", "via/")(a)
		})
		n := s.normal("chain")
		s.fails(n, "--- FAIL: TestCorpus/export/names/import", tt.want)
		s.contains(n, "--- PASS: TestCorpus/export/names/list")
		before := s.snapshot()
		s.fails(s.update("chain"), tt.want, listHeldBack)
		s.unchanged(before)
		s.write(names, clean, "restore it")
	}
	// A rejection joined with a fault is a fault: decl.Reject marks only
	// part of the error.
	s.edit(names, "names states unterminated, and import rejects it, joined with an error decl.Reject doesn't mark", func(a *archive.Archive) {
		insert("properties", "unterminated\n", "in/")(a)
		insert("in/import/error", "line 2: unclosed quote\n", "via/")(a)
		insert("in/import/fail", "the generator's error\n", "via/")(a)
	})
	joined := fault("import: line 2: unclosed quote\n  the generator's error")
	n := s.normal("chain")
	s.fails(n, "--- FAIL: TestCorpus/export/names/import", joined)
	s.contains(n, "--- PASS: TestCorpus/export/names/list")
	before := s.snapshot()
	s.fails(s.update("chain"), joined, listHeldBack)
	s.unchanged(before)
	s.done()
}

// TestChainCodesThroughJenny: a bytes lift's rejection wraps the error its
// generator returned, through decl.Reject and the jenny's own wrapping, so
// the project's Codes finds it with errors.As.
func TestChainCodesThroughJenny(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	coded := "testdata/export/coded.txtar"
	s.write(coded, []byte("import rejects the export with an error that has a code.\n-- properties --\nunterminated\n-- in/import/coded --\nC101\n"), "an export import rejects with a *codedError")
	s.passes(s.update("chain-codes"))
	s.cat(coded)
	s.hasSections(coded, map[string]string{
		"out/import/outcome":   "rejected C101\n",
		"out/import/error.txt": "import: an error with the code C101\n",
	}, "via/")
	s.passes(s.normal("chain-codes"))

	// Rejections joined are one rejection, and Codes reaches the code
	// through the join.
	joined := "testdata/export/joined.txtar"
	s.write(joined, []byte("import rejects the export with two errors decl.Reject marks, joined, one with a code.\n-- properties --\nunterminated\n-- in/import/coded --\nC101\n-- in/import/error --\nline 1: unclosed quote\n"), "an export import rejects with two marked errors, joined")
	s.passes(s.update("chain-codes"))
	s.hasSections(joined, map[string]string{
		"out/import/outcome":   "rejected C101\n",
		"out/import/error.txt": "import: line 1: unclosed quote\nan error with the code C101\n",
	}, "via/")
	s.passes(s.normal("chain-codes"))
	s.done()
}

// TestChainCheck: a corpus check's Cell holds a bytes lift's committed
// product, which it reads when the lift now makes another.
func TestChainCheck(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	s.passes(s.update("chain-check"))
	findings := "testdata/checks/products.txt"
	s.cat(findings)
	committed := s.read(findings)
	for _, want := range []string{
		"export/prices import evaluate/out/values.txt \"a1 3\\nb1 6\\n\"\n",
		"export/prices/variant/doubled import evaluate/out/values.txt \"a1 6\\nb1 12\\n\"\n",
		"export/prices/transform/louder import quote/B1 \"=A1*2\\n\"\n",
		"export/names import names/outcome \"not an outcome\\n\"\n",
	} {
		if !strings.Contains(string(committed), want) {
			t.Errorf("the findings lack %q:\n%s", want, committed)
		}
	}
	if strings.Contains(string(committed), " list ") || strings.Contains(string(committed), " parse ") {
		t.Errorf("a column that isn't a bytes lift has a product:\n%s", committed)
	}
	s.edit(prices, "import makes other values", set("in/import/out/evaluate/out/values.txt", "a1 4\nb1 8\n"))
	r := s.normal("chain-check")
	s.fails(r, "--- FAIL: TestCorpus/export/prices/import")
	s.contains(r, "--- PASS: TestCorpus/checks/products")
	s.same(findings, committed)
	s.done()
}

// TestChainCheckPaths: a cell's path runs through a bytes lift whose
// product is another corpus type, on to the cell's own corpus type: an
// export's cell of evaluate is built through resolve, parse and import,
// and a script's, of the type import makes, through resolve and parse.
// Past two bytes lifts, the path runs through both. The registry records
// each path, and a check reads it there.
func TestChainCheckPaths(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, fixture, registration string
		findings                    string
		paths                       map[string][]registry.Path
	}{
		{
			"through import", "chain", "chain-paths",
			"export evaluate: through resolve, parse, import\n" +
				"export import: through no lift\n" +
				"export list: through import\n" +
				"export parse: through import\n" +
				"export resolve: through parse, import\n" +
				"script evaluate: through resolve, parse\n" +
				"script list: through no lift\n" +
				"script parse: through no lift\n" +
				"script resolve: through parse\n",
			map[string][]registry.Path{
				"suite evaluate": {{Corpus: "export", Lifts: []string{"resolve", "parse", "import"}}, {Corpus: "script", Lifts: []string{"resolve", "parse"}}},
				"suite list":     {{Corpus: "export", Lifts: []string{"import"}}, {Corpus: "script", Lifts: []string{}}},
				"lift import":    {{Corpus: "export", Lifts: []string{}}},
				"lift resolve":   {{Corpus: "export", Lifts: []string{"parse", "import"}}, {Corpus: "script", Lifts: []string{"parse"}}},
			},
		},
		{
			"through export and import", "chain-twice", "chain-twice-paths",
			"grid export: through no lift\n" +
				"grid import: through export\n" +
				"grid list: through import, export\n",
			map[string][]registry.Path{
				"suite list":  {{Corpus: "export", Lifts: []string{"import"}}, {Corpus: "grid", Lifts: []string{"import", "export"}}, {Corpus: "script", Lifts: []string{}}},
				"lift import": {{Corpus: "export", Lifts: []string{}}, {Corpus: "grid", Lifts: []string{"export"}}},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newSandbox(t, tt.fixture)
			s.passes(s.update(tt.registration))
			s.passes(s.normal(tt.registration))
			const findings = "testdata/checks/paths.txt"
			s.cat(findings)
			if got := string(s.read(findings)); got != tt.findings {
				t.Errorf("the cells' paths are\n%s\nwant\n%s", got, tt.findings)
			}
			s.wantPaths("testdata/hypercase.json", tt.paths)
			s.done()
		})
	}
}

// TestChainSharedCheckPaths: a check reads the path of each column over
// the corpus root from the registry of the package that owns it, so a
// cell of a column that another package runs through lifts it only uses
// carries the path that package's registry records. A registry entry that
// doesn't give one path from each corpus its column runs over, in the
// order of its corpora, can't be read, and so can't one whose path names
// a lift the registry doesn't list, or the column itself, whether the
// column is the package's or a lift it uses, and so can't one that
// records an output's inputs as no declaration registration takes would,
// or lists a corpus check twice: every check fails, and so does the
// root-package check. Each hand edit is
// written as the JSON it makes, not by Marshal, which would sort the
// paths.
func TestChainSharedCheckPaths(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain-shared")
	s.passes(s.update("chain-a-paths"))
	s.passes(s.run("b", "chain-b", true))
	s.passes(s.normal("chain-a-paths"))
	s.passes(s.run("b", "chain-b", false))
	const findings, reg = "testdata/checks/paths.txt", "b/testdata/hypercase.json"
	s.cat(findings)
	const want = "export evaluate: through resolve, parse, import\n" +
		"export import: through no lift\n" +
		"export list: through import\n" +
		"export parse: through import\n" +
		"export resolve: through parse, import\n"
	if got := string(s.read(findings)); got != want {
		t.Errorf("the cells' paths are\n%s\nwant\n%s", got, want)
	}
	s.wantPaths(reg, map[string][]registry.Path{
		"suite evaluate": {{Corpus: "export", Lifts: []string{"resolve", "parse", "import"}}, {Corpus: "script", Lifts: []string{"resolve", "parse"}}},
		"lift resolve":   {{Corpus: "export", Lifts: []string{"parse", "import"}}, {Corpus: "script", Lifts: []string{"parse"}}},
	})

	clean := s.read(reg)
	edit := func(why string, change func(*registry.Registry)) {
		g, err := registry.Parse(clean)
		if err != nil {
			t.Fatal(err)
		}
		change(g)
		data, err := json.MarshalIndent(g, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		s.write(reg, append(data, '\n'), why)
	}
	evaluate := func(change func(*registry.Suite)) func(*registry.Registry) {
		return func(g *registry.Registry) {
			change(&g.Suites[slices.IndexFunc(g.Suites, func(c registry.Suite) bool { return c.Name == "evaluate" })])
		}
	}
	// refused checks that the root package's normal run fails on b's
	// registry, which neither the corpus check nor the root-package check
	// can read, for why, and that the findings stay as they were.
	refused := func(why string) {
		s.fails(s.normal("chain-a-paths"),
			"the corpus check paths can't read its cells: b/testdata/hypercase.json can't be read: "+why+". No check runs, and an update writes no findings.\nRun FIXTURE_UPDATE_GOLDEN=1 go test ./... to write it.",
			"b/testdata/hypercase.json can't be read: "+why+".\nRun FIXTURE_UPDATE_GOLDEN=1 go test ./... to write it.")
		s.same(findings, []byte(want))
	}
	const evaluateRuns = "the suite evaluate runs over testdata/export/ and testdata/script/, and has paths from "
	const rule = ", where a registry gives each column one path from each corpus it runs over, in the same order"
	edit("drop evaluate's path from export from b's registry, by hand", evaluate(func(c *registry.Suite) { c.Paths = c.Paths[1:] }))
	refused(evaluateRuns + "testdata/script/" + rule)
	s.fails(s.update("chain-a-paths"), "the corpus check paths can't read its cells: b/testdata/hypercase.json can't be read: "+evaluateRuns+"testdata/script/"+rule+". No check runs, and an update writes no findings.\nRun FIXTURE_UPDATE_GOLDEN=1 go test ./... to write it.")
	s.same(findings, []byte(want))
	edit("give evaluate a second path from script in b's registry, by hand", evaluate(func(c *registry.Suite) { c.Paths[0].Corpus = "script" }))
	refused(evaluateRuns + "testdata/script/ and testdata/script/" + rule)
	edit("give evaluate no paths in b's registry, by hand", evaluate(func(c *registry.Suite) { c.Paths = nil }))
	refused(evaluateRuns + "none" + rule)
	edit("list evaluate's paths in the other order from its corpora in b's registry, by hand", evaluate(func(c *registry.Suite) { slices.Reverse(c.Paths) }))
	refused(evaluateRuns + "testdata/script/ and testdata/export/" + rule)
	edit("give evaluate a third path, from script, after its two in b's registry, by hand", evaluate(func(c *registry.Suite) { c.Paths = append(c.Paths, c.Paths[1]) }))
	refused(evaluateRuns + "testdata/export/, testdata/script/ and testdata/script/" + rule)
	edit("name a lift b's registry doesn't list on evaluate's path from export, by hand", evaluate(func(c *registry.Suite) { c.Paths[0].Lifts = []string{"resolve", "nosuch"} }))
	refused("the suite evaluate's path from testdata/export/ names nosuch, and the registry lists no lift of that name, where every lift on a column's path is one its package registers")
	edit("record evaluate's values.txt with inputs no declaration states in b's registry, by hand", evaluate(func(c *registry.Suite) {
		c.Outputs[slices.IndexFunc(c.Outputs, func(o registry.Output) bool { return o.Pattern == "values.txt" })].Inputs = "lines"
	}))
	refused(`the suite evaluate declares the output values.txt with inputs "lines", where a registry records "func", "positions", "all" or "none"`)
	edit("list the corpus check codes twice in b's registry, by hand", func(g *registry.Registry) {
		codes := registry.Check{Name: "codes", Description: "Lists the codes."}
		g.Checks = append(g.Checks, codes, codes)
	})
	refused("the registry lists the corpus check codes twice, where each corpus check a package registers has a name of its own")

	// The lifts b uses are held to the same rules as its columns.
	edit("drop the paths from export of the lifts b uses, parse and resolve, from b's registry, by hand", func(g *registry.Registry) {
		for i, l := range g.Lifts {
			if l.Name == "parse" || l.Name == "resolve" {
				g.Lifts[i].Paths = slices.DeleteFunc(g.Lifts[i].Paths, func(p registry.Path) bool { return p.Corpus == "export" })
			}
		}
	})
	refused("the lift parse runs over testdata/export/ and testdata/script/, and has paths from testdata/script/" + rule)
	edit("name resolve on its own path from script in b's registry, by hand", func(g *registry.Registry) {
		l := &g.Lifts[slices.IndexFunc(g.Lifts, func(l registry.Lift) bool { return l.Name == "resolve" })]
		l.Paths[1].Lifts = []string{"resolve"}
	})
	refused("the lift resolve's path from testdata/script/ names the lift itself, where a column's path names the lifts before it")
	s.same(findings, []byte(want))
	s.write(reg, clean, "restore it")
	s.passes(s.normal("chain-a-paths"))
	s.done()
}

// TestChainRootCheckUsedPath: a package that uses a lift reaches it from
// each corpus by the path the lift's owner records. Here . owns parse and
// resolve over script alone, and ./b, which uses them, reaches them from
// export too, through import, which it owns. No package writes the cells
// of parse and resolve that ./b's cells of export are built through, as
// the paths check finds, and the root-package check fails once for each
// lift. Applying the first way each failure states, . running the lift
// over export by ./b's path, makes every run pass: . registers export's
// corpus type and uses import. A registry edited by hand back to the
// conflict brings the failures back, and the update, which they name
// where a registry is stale, ends them.
func TestChainRootCheckUsedPath(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain-shared")
	s.passes(s.run("b", "used-fewer-b", true))
	s.passes(s.update("used-fewer-a"))
	s.cat("testdata/checks/paths.txt")
	failures := []string{
		"the lift parse is used by ./b on the path testdata/export/ → import → parse, and ., which owns it, doesn't run it over testdata/export/, so no package writes its cells of that corpus's cases, which ./b's cells past it are built through.\nA package that uses a lift reaches it from each corpus by the path the lift's owner does: either . runs parse over testdata/export/ by that path, or ./b doesn't reach parse from testdata/export/. Making a way true may need other registrations to change, such as a lift that would then close a cycle or give a type a second path. If one holds already, a registry is stale: run FIXTURE_UPDATE_GOLDEN=1 go test ./..., which rewrites the registry of every package with tests.",
		"the lift resolve is used by ./b on the path testdata/export/ → import → parse → resolve, and ., which owns it, doesn't run it over testdata/export/, so no package writes its cells of that corpus's cases, which ./b's cells past it are built through.\nA package that uses a lift reaches it from each corpus by the path the lift's owner does: either . runs resolve over testdata/export/ by that path, or ./b doesn't reach resolve from testdata/export/. Making a way true may need other registrations to change, such as a lift that would then close a cycle or give a type a second path. If one holds already, a registry is stale: run FIXTURE_UPDATE_GOLDEN=1 go test ./..., which rewrites the registry of every package with tests.",
	}
	r := s.normal("used-fewer-a")
	s.fails(r, failures...)
	if n := strings.Count(string(r.out), " is used by "); n != len(failures) {
		t.Errorf("the root-package check fails %d times on lifts used otherwise than their owner reaches them, and should fail %d times", n, len(failures))
	}
	s.passes(s.run("b", "used-fewer-b", false))

	// . registers export and uses import, so it runs parse and resolve over
	// export by ./b's path.
	s.passes(s.update("used-fewer-a-fixed"))
	s.cat("testdata/checks/paths.txt")
	s.passes(s.normal("used-fewer-a-fixed"))
	s.passes(s.run("b", "used-fewer-b", false))

	// .'s registry, edited by hand so that . runs parse and resolve over
	// script alone, though it registers what it takes to reach them from
	// export as ./b does: the failures are back, and the update ends them.
	clean := s.read("testdata/hypercase.json")
	g, err := registry.Parse(clean)
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range g.Lifts {
		if l.Name == "parse" || l.Name == "resolve" {
			g.Lifts[i].Corpora = slices.DeleteFunc(l.Corpora, func(c string) bool { return c == "export" })
			g.Lifts[i].Paths = slices.DeleteFunc(l.Paths, func(p registry.Path) bool { return p.Corpus == "export" })
		}
	}
	s.write("testdata/hypercase.json", g.Marshal(), "drop export from the corpora and paths of parse and resolve in .'s registry, by hand")
	s.fails(s.normal("used-fewer-a-fixed"), failures...)
	s.passes(s.update("used-fewer-a-fixed"))
	s.same("testdata/hypercase.json", clean)
	s.passes(s.normal("used-fewer-a-fixed"))
	s.passes(s.run("b", "used-fewer-b", false))
	s.done()
}

// TestChainRootCheckUsedUnowned: ./b uses parse and resolve, which .
// owns over script alone, and reaches them from export through import,
// which no package owns. The root-package check fails over import, which
// no package owns, and over parse and resolve, from export; import, with
// no owner, has no path to compare with ./b's. Where . registers export
// and owns import, it runs parse and resolve over export by ./b's path,
// which ends every failure.
func TestChainRootCheckUsedUnowned(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain-shared")
	s.passes(s.run("b", "used-unowned-b", true))
	s.passes(s.update("used-fewer-a"))
	r := s.normal("used-fewer-a")
	s.fails(r,
		"the lift import is used by ./b, and no package owns it, so its error is recorded nowhere.",
		"the lift parse is used by ./b on the path testdata/export/ → import → parse, and ., which owns it, doesn't run it over testdata/export/, so no package writes its cells of that corpus's cases, which ./b's cells past it are built through.\nA package that uses a lift reaches it from each corpus by the path the lift's owner does: either . runs parse over testdata/export/ by that path, or ./b doesn't reach parse from testdata/export/. Making a way true may need other registrations to change, such as a lift that would then close a cycle or give a type a second path. If one holds already, a registry is stale: run FIXTURE_UPDATE_GOLDEN=1 go test ./..., which rewrites the registry of every package with tests.",
		"the lift resolve is used by ./b on the path testdata/export/ → import → parse → resolve, and ., which owns it, doesn't run it over testdata/export/, so no package writes its cells of that corpus's cases, which ./b's cells past it are built through.\nA package that uses a lift reaches it from each corpus by the path the lift's owner does: either . runs resolve over testdata/export/ by that path, or ./b doesn't reach resolve from testdata/export/. Making a way true may need other registrations to change, such as a lift that would then close a cycle or give a type a second path. If one holds already, a registry is stale: run FIXTURE_UPDATE_GOLDEN=1 go test ./..., which rewrites the registry of every package with tests.")
	if n := strings.Count(string(r.out), " is used by "); n != 3 {
		t.Errorf("the root-package check fails %d times on used lifts, and should fail 3 times: once for import, and once for each of parse and resolve", n)
	}

	// . registers export and owns import.
	s.passes(s.update("used-unowned-a-fixed"))
	s.passes(s.run("b", "used-unowned-b", true))
	s.passes(s.normal("used-unowned-a-fixed"))
	s.passes(s.run("b", "used-unowned-b", false))
	s.done()
}

// TestUpdateUnreadableUntested: ./b's tests are removed, and its registry,
// edited by hand, gives its lifts no paths, so it can't be read. The root
// package's corpus check fails on it, in the update and in a normal run,
// and so does its root-package check, in a normal run, and each failure
// says that b holds no _test.go file, so no update writes the registry,
// and to delete it: the update fails alike, twice. Once the registry is
// deleted, the failure left is over ./b's cells, which no registry claims
// then, and once they are deleted too, every run passes. Where the
// package has tests, the failures say to run the update, as
// TestUpdateMalformedPaths shows.
func TestUpdateUnreadableUntested(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared-ladder")
	s.passes(s.run("b", "ladder-b-shared-producer", true))
	s.passes(s.update("ladder-a-paths"))
	s.passes(s.normal("ladder-a-paths"))
	s.remove("b/corpus_test.go", "remove b's tests, and leave its registry")
	g, err := registry.Parse(s.read("b/testdata/hypercase.json"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range g.Lifts {
		g.Lifts[i].Paths = nil
	}
	s.write("b/testdata/hypercase.json", g.Marshal(), "drop the paths of b's lifts, by hand")
	const why = "the lift parse runs over testdata/script/, and has paths from none, where a registry gives each column one path from each corpus it runs over, in the same order"
	const check = "the corpus check paths can't read its cells: b/testdata/hypercase.json can't be read: " + why + ", and b holds no _test.go file, so no update writes it. No check runs, and an update writes no findings.\nDelete b/testdata/hypercase.json."
	s.fails(s.normal("ladder-a-paths"), check, "b/testdata/hypercase.json can't be read: "+why+", and b holds no _test.go file, so no update writes it.\nDelete it.")
	s.fails(s.update("ladder-a-paths"), check)
	s.fails(s.update("ladder-a-paths"), check)
	s.remove("b/testdata/hypercase.json", "delete b's registry, which no update writes")
	s.passes(s.update("ladder-a-paths"))
	s.fails(s.normal("ladder-a-paths"), "testdata/script/x.txtar: no registry claims out/evaluate/inputs, out/evaluate/outcome and out/evaluate/values.txt: no column named evaluate runs over the corpus script.")
	s.edit("testdata/script/x.txtar", "delete ./b's cells, which no package writes", drop("out/evaluate/"))
	s.passes(s.normal("ladder-a-paths"))
	s.passes(s.update("ladder-a-paths"))
	s.done()
}

// TestUpdateMalformedPaths: over the strip fixture, the registries of
// both packages give parse, which . owns and ./b uses, no path from
// export, as edited by hand, so neither can be read. The update walks for
// symbolic links before it writes anything, the package's own registry
// included, and before a corpus check reads a registry, so with a link in
// place of testdata/export it refuses the link and leaves both registries
// as they were. Without the link, the root package's update rewrites its
// own registry, and its corpus check cells can't read ./b's, which only
// ./b's update rewrites. Then every run passes.
func TestUpdateMalformedPaths(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "strip")
	s.settle(stripModule...)
	const regA, regB = "testdata/hypercase.json", "b/testdata/hypercase.json"
	cleanA, cleanB := s.read(regA), s.read(regB)
	for _, reg := range []string{regA, regB} {
		g, err := registry.Parse(s.read(reg))
		if err != nil {
			t.Fatal(err)
		}
		l := &g.Lifts[slices.IndexFunc(g.Lifts, func(l registry.Lift) bool { return l.Name == "parse" })]
		l.Paths = slices.DeleteFunc(l.Paths, func(p registry.Path) bool { return p.Corpus == "export" })
		s.write(reg, g.Marshal(), "drop parse's path from export, by hand")
	}
	const why = "the lift parse runs over testdata/export/ and testdata/script/, and has paths from testdata/script/, where a registry gives each column one path from each corpus it runs over, in the same order"
	s.remove("testdata/.hypercase.lock", "the lock, which the update takes to write an archive")
	before := s.snapshot()
	s.link("testdata/export", "")
	s.fails(s.update("strip-a"), "testdata/ holds no symbolic links: testdata/export is one, so the update wrote nothing.\nRemove it, or replace it with a copy of what it names, if it names anything, then run FIXTURE_UPDATE_GOLDEN=1 go test ./... again.")
	s.unlink("testdata/export")
	s.unchanged(before)

	s.fails(s.update("strip-a"), "the corpus check cells can't read its cells: b/testdata/hypercase.json can't be read: "+why+". No check runs, and an update writes no findings.\nRun FIXTURE_UPDATE_GOLDEN=1 go test ./... to write it.")
	s.same(regA, cleanA)
	s.passes(s.run("b", "strip-b", true))
	s.same(regB, cleanB)
	s.passes(s.update("strip-a"))
	s.passes(s.normal("strip-a"))
	s.passes(s.run("b", "strip-b", false))
	s.done()
}

// TestChainRootCheck: the root-package check claims via/<lift>/ for each
// bytes lift over the corpus, and reports any other via/ section, naming
// the archive and the section.
func TestChainRootCheck(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	s.passes(s.update("chain"))
	s.edit(prices, "products no bytes lift makes", func(a *archive.Archive) {
		insert("via/nobody/quote/A1", "3\n", "out/")(a)
		insert("via/parse/quote/A1", "3\n", "out/")(a)
		insert("variant/doubled/via/nobody/quote/A1", "3\n", "variant/doubled/out/")(a)
	})
	r := s.normal("chain")
	s.fails(r,
		"testdata/export/prices.txtar: no registry claims via/nobody/quote/A1 and variant/doubled/via/nobody/quote/A1: no bytes lift named nobody runs over the corpus export.\nIf a bytes lift should write them, register it. If not, run hypercase strip -root . -column nobody at the module root.",
		"testdata/export/prices.txtar: no registry claims via/parse/quote/A1: no bytes lift named parse runs over the corpus export.")
	s.contains(r, "--- PASS: TestCorpus/export/prices/import")
	s.done()
}

// TestSameViewUnownedTransform: in a package that doesn't own a transform,
// committed inputs that differ from what the transform makes only in a
// delete line naming a path the base lacks give the input view the
// transform makes, so a cell of the variant that fails, fails for its own
// reason, and the run doesn't put the failure down to the inputs. Here the
// root package, which doesn't own louder, runs parse on louder's variant,
// whose committed outcome is stale.
func TestSameViewUnownedTransform(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain-shared")
	prices := "testdata/export/prices.txtar"
	s.passes(s.update("chain-a"))
	s.passes(s.run("b", "chain-b", true))
	s.edit(prices, "louder's committed inputs delete a path the base lacks, and parse's cell of louder's variant commits a stale outcome", func(a *archive.Archive) {
		insert("transform/louder/delete", "nothing/here\n", "transform/louder/via/")(a)
		set("transform/louder/out/parse/outcome", "rejected\n")(a)
	})
	r := s.normal("chain-a")
	s.fails(r, "transform/louder/out/parse/outcome differs (-golden +got):\n- rejected\n+ accepted\nIf the change is intended, run FIXTURE_UPDATE_GOLDEN=1 go test ./... and read the diff before committing.")
	s.lacks(r, "A cell of the variant failed, and the archive's inputs", "on another value")
	s.done()
}

// TestChainShared: across packages. The root package owns import, parse
// and resolve; ./b uses them, and owns evaluate, list and the transform
// louder. A package that only uses a bytes lift reads its committed
// product in a normal run, and doesn't run it; its update runs it, and
// writes only its own sections. import's owner writes its product in
// louder's variant, and louder's owner the variant's inputs.
func TestChainShared(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain-shared")
	prices := "testdata/export/prices.txtar"
	s.passes(s.update("chain-a"))
	s.hasSections(prices, map[string]string{
		"via/import/quote/B1": "=A1*2\n",
		"transform/louder/via/import/evaluate/out/values.txt": "A1 3\n",
	}, "transform/louder/in/", "out/evaluate/", "out/list/", "transform/louder/out/list/")
	b := s.run("b", "chain-b", true)
	s.passes(b)
	s.contains(b, "import ran")
	s.hasSections(prices, map[string]string{
		"transform/louder/in/import/out/evaluate/out/values.txt": "A1 3\n",
		"out/list/files.txt": "3 files\nevaluate/out/values.txt \"a1 3\\n\"\nquote/A1 \"3\\n\"\nquote/B1 \"=A1*2\\n\"\n",
	})
	s.cat(prices)
	s.passes(s.normal("chain-a"))
	b = s.run("b", "chain-b", false)
	s.passes(b)
	s.lacks(b, "import ran")
	clean := s.read(prices)

	// Committed bytes that differ from what import makes fail its owner's
	// column, and b's cells read them.
	s.edit(prices, "the committed product differs", set("via/import/quote/B1", "=A1*3\n"))
	s.fails(s.normal("chain-a"), "via/import/quote/B1 differs (-golden +got):\n- =A1*3\n+ =A1*2")
	b = s.run("b", "chain-b", false)
	s.fails(b, "out/list/files.txt differs (-golden +got):", "- quote/B1 \"=A1*2\\n\"\n+ quote/B1 \"=A1*3\\n\"")
	s.lacks(b, "import ran")
	s.write(prices, clean, "restore it")

	// b doesn't run import in a normal run, so it can't tell whether
	// import accepts prices now, and its advice keeps the files of its
	// cells that import's missing outcome blocks.
	s.edit(prices, "import's cell has no committed outcome", drop("out/import/outcome"))
	b = s.run("b", "chain-b", false)
	s.fails(b, "out/list/files.txt is recorded, and the cell is blocked: the lift import, on list's path, has no committed outcome, out/import/outcome, which a normal run reads.\nOnly the package that owns import runs it in a normal run, so this one can't tell what FIXTURE_UPDATE_GOLDEN=1 go test ./... does with list: a normal run reads import's outcome and product from the archive. Delete nothing by hand before it has run.")
	s.lacks(b, "import ran")
	s.write(prices, clean, "restore it")

	// import stops making quote/B1. Its owner's column fails, naming the
	// file, and the module's update removes it.
	s.edit(prices, "import no longer makes quote/B1", drop("in/import/out/quote/B1"))
	s.fails(s.normal("chain-a"), "via/import/quote/B1 is a file of import's committed product, which this run of import doesn't make.\nIf it should, fix import's jenny. If not, run FIXTURE_UPDATE_GOLDEN=1 go test ./..., which removes it, and read the diff before committing.")
	before := s.read(prices)
	for _, r := range s.parallel(childRun{"", "chain-a", true}, childRun{"b", "chain-b", true}) {
		s.passes(r)
	}
	after := s.read(prices)
	s.note("the update's diff:\n%s", diff.Lines(before, after))
	s.hasSections(prices, map[string]string{}, "via/import/quote/B1", "transform/louder/via/import/quote/B1")
	s.passes(s.normal("chain-a"))
	s.passes(s.run("b", "chain-b", false))
	s.done()
}

// TestChainSharedProjects: in an update, a package that only uses a bytes
// lift holds the cells past it back only on what keeps the lift's cell
// unwritten under any Project: its own Project doesn't judge the lift's
// cell, and the owner's may differ. So what the owner's Project alone
// does to the lift's cell, the using package's update can't follow.
func TestChainSharedProjects(t *testing.T) {
	t.Parallel()

	// b's renderer panics on import's error, and import's owner's doesn't:
	// the update writes import's cell, and b's cells past it.
	t.Run("the renderer of the user panics", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "chain-shared")
		const quoted = "testdata/export/quoted.txtar"
		s.write(quoted, []byte("import rejects the export, and projectRender panics on its error.\n-- properties --\nunterminated\n-- in/import/error --\nline 1: unclosed quote\n-- in/render/panic --\nthe renderer panicked\n"), "an export import rejects, on whose error projectRender panics")
		for _, r := range s.parallel(childRun{"", "chain-a", true}, childRun{"b", "chain-b-render", true}) {
			s.passes(r)
		}
		s.hasSections(quoted, map[string]string{
			"out/import/outcome":   "rejected\n",
			"out/import/error.txt": "import: line 1: unclosed quote\n",
			"out/evaluate/outcome": "blocked import\n",
			"out/list/outcome":     "blocked import\n",
		})
		s.passes(s.normal("chain-a"))
		s.passes(s.run("b", "chain-b-render", false))
		s.done()
	})

	// import's owner's renderer panics on its error, and b's doesn't: no
	// update writes import's cell, whose committed outcome stays accepted,
	// and b's update writes its cells past import as blocked by it, which
	// b's normal run then contradicts, reading the committed outcome.
	t.Run("the renderer of the owner panics", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "chain-shared")
		const prices = "testdata/export/prices.txtar"
		for _, r := range s.parallel(childRun{"", "chain-a-render", true}, childRun{"b", "chain-b", true}) {
			s.passes(r)
		}
		s.edit(prices, "import now rejects prices, and projectRender panics on its error", func(a *archive.Archive) {
			insert("properties", "unterminated\n", "in/")(a)
			insert("in/import/error", "line 1: unclosed quote\n", "in/import/out/")(a)
			insert("in/render/panic", "the renderer panicked\n", "via/")(a)
		})
		s.fails(s.update("chain-a-render"),
			"The project's RenderError panicked on import's error: the renderer panicked\nFix RenderError. An update writes nothing for this cell.",
			"The cell of the lift import, on parse's path")
		s.fails(s.run("b", "chain-b", true), "out/list/files.txt is recorded, and the cell is blocked: the lift import, on list's path, failed on the case.")
		s.hasSections(prices, map[string]string{
			"out/import/outcome":   "accepted\n",
			"out/evaluate/outcome": "blocked import\n",
			"out/list/outcome":     "blocked import\n",
		})
		s.fails(s.run("b", "chain-b", false), "out/list/outcome differs (-golden +got):\n- blocked import\n+ accepted")

		// Once the owner's update writes import's cell, b's normal run reads
		// its rejection, and passes.
		s.edit(prices, "projectRender no longer panics, and the cells' orphans are deleted by hand", func(a *archive.Archive) {
			drop("in/render/panic")(a)
			for _, tree := range []string{"", "transform/louder/"} {
				for _, f := range []string{"parse/inputs", "resolve/inputs", "evaluate/values.txt", "evaluate/inputs", "list/files.txt", "list/inputs"} {
					drop(tree + "out/" + f)(a)
				}
			}
		})
		for _, r := range s.parallel(childRun{"", "chain-a-render", true}, childRun{"b", "chain-b", true}) {
			s.passes(r)
		}
		s.hasSections(prices, map[string]string{"out/import/outcome": "rejected\n", "out/list/outcome": "blocked import\n"}, "via/import/")
		s.passes(s.normal("chain-a-render"))
		s.passes(s.run("b", "chain-b", false))
		s.done()
	})

	// import's jenny faults on one export, and makes a product an archive
	// can't hold of another: no Project's update writes import's cell on
	// either, so b's update holds the cells past it back, as the owner's
	// does.
	t.Run("a fault or a product an archive cannot hold", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "chain-shared")
		for _, r := range s.parallel(childRun{"", "chain-a", true}, childRun{"b", "chain-b", true}) {
			s.passes(r)
		}
		s.write("testdata/export/faulty.txtar", []byte("import's jenny fails on the export, with an error decl.Reject doesn't mark.\n-- in/import/fail --\nthe generator's error\n"), "an export on which import's jenny faults")
		s.write("testdata/export/stray.txtar", []byte("import makes a file its Product doesn't declare.\n-- in/import/out/stray/x --\nx\n"), "an export of which import makes an undeclared file")
		before := s.snapshot()
		heldBack := func(name, about, col, desc string) string {
			return "export/" + name + ": " + about + "\n" + col + ": " + desc + "\nThe cell of the lift import, on " + col + "'s path, has a problem, which that cell reports, in the package that owns it."
		}
		const (
			faultyAbout = "import's jenny fails on the export, with an error decl.Reject doesn't mark."
			strayAbout  = "import makes a file its Product doesn't declare."
			listDesc    = "Lists the files of the script's input view, with their bytes."
		)
		s.fails(s.run("b", "chain-b", true), heldBack("faulty", faultyAbout, "list", listDesc), heldBack("stray", strayAbout, "list", listDesc))
		s.fails(s.update("chain-a"),
			"import's jenny failed on the case, with an error that is no rejection:\n  import: the generator's error",
			"via/import/stray/x is not declared.",
			"The cell of the lift import, on parse's path")
		s.unchanged(before)
		s.done()
	})
}

// TestChainLiftEntries: the root-package check fails when the registries
// that own or use a lift record it differently, once per lift, naming the
// packages and each field that differs. A package that only uses a bytes
// lift tells by its own declaration whether an update writes the lift's
// cell, and so the cells past it, and one that uses any lift stages the
// cells past it by its own declaration, so it must declare the lift as
// the owner does. A registry in a package with no _test.go file isn't
// compared.
func TestChainLiftEntries(t *testing.T) {
	t.Parallel()
	const breakage = "the registries that own or use the lift import differ in "
	const fix = "\nEvery package that owns or uses a lift registers one declaration of it: a package that only uses a bytes lift tells by its own declaration whether an update writes the lift's cell, and so the cells past it. Declare the lift once, in a package the others import, and register that declaration." + staleRegistry

	// ./b uses a copy of import that declares no outputs, so its update
	// holds the cells past import back on an error.txt it doesn't declare,
	// where the owner's update writes import's cell.
	t.Run("a copy with no outputs", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "chain-shared")
		const quoted = "testdata/export/quoted.txtar"
		s.write(quoted, []byte("import rejects the export.\n-- properties --\nunterminated\n-- in/import/error --\nline 1: unclosed quote\n"), "an export import rejects")
		rs := s.parallel(childRun{"", "chain-a", true}, childRun{"b", "chain-b-nooutputs", true})
		s.passes(rs[0])
		s.fails(rs[1], "The cell of the lift import, on list's path, has a problem, which that cell reports, in the package that owns it.")
		s.hasSections(quoted, map[string]string{"out/import/outcome": "rejected\n"}, "out/list/", "out/evaluate/")
		s.fails(s.normal("chain-a"), breakage+"outputs:\n  outputs:\n    .: error.txt \"Why the importer rejects the export.\" (inputs: positions)\n    ./b: none"+fix)
		s.done()
	})

	// Each field of b's entry, changed in its registry by hand, outputs and
	// productFiles each by a declaration's description or presence and by
	// what one states in Inputs, then two at once, and an ordinary lift's.
	// The module's update rewrites the stale registry.
	t.Run("each field", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "chain-shared")
		update := func() {
			for _, r := range s.parallel(childRun{"", "chain-a", true}, childRun{"b", "chain-b", true}) {
				s.passes(r)
			}
		}
		update()
		s.passes(s.normal("chain-a"))
		const reg = "b/testdata/hypercase.json"
		clean := s.read(reg)
		edit := func(lift, why string, change func(l *registry.Lift)) result {
			g, err := registry.Parse(clean)
			if err != nil {
				t.Fatal(err)
			}
			i := slices.IndexFunc(g.Lifts, func(l registry.Lift) bool { return l.Name == lift })
			change(&g.Lifts[i])
			s.write(reg, g.Marshal(), why)
			return s.normal("chain-a")
		}
		description := func(l *registry.Lift) { l.Description = "Imports the export." }
		// products shows import's product declarations, the first, evaluate/*,
		// stating first.
		products := func(first string) string {
			return "evaluate/* \"What evaluate does, as a script's evaluate/ tree says.\" (inputs: " + first + "), evaluate/out/* \"An output evaluate returns.\" (inputs: func), inputs/* \"A tree named as a cell's inputs file is.\" (inputs: func), names/* \"Files named as a cell's own files are.\" (inputs: func), parse/* \"What parse does, as a script's parse/ tree says.\" (inputs: func), quote/* \"What a renderer of errors quotes.\" (inputs: func), resolve/* \"What resolve does, as a script's resolve/ tree says.\" (inputs: func)"
		}
		for _, tt := range []struct {
			field  string
			change func(l *registry.Lift)
			want   string
		}{
			{"description", description, "description:\n  .: \"Imports the export as a script, as its import/ tree says.\"\n  ./b: \"Imports the export.\""},
			{"input", func(l *registry.Lift) { l.Input = "github.com/sdboyer/hypercase_test.Grid" }, "input:\n  .: github.com/sdboyer/hypercase_test.Export\n  ./b: github.com/sdboyer/hypercase_test.Grid"},
			{"product", func(l *registry.Lift) { l.Product = "*github.com/sdboyer/hypercase_test.Parsed" }, "product:\n  .: github.com/sdboyer/hypercase_test.Script\n  ./b: *github.com/sdboyer/hypercase_test.Parsed"},
			{"bytes", func(l *registry.Lift) { l.Bytes = false }, "bytes:\n  .: true\n  ./b: false"},
			{"outputs", func(l *registry.Lift) { l.Outputs[0].Description = "Why." }, "outputs:\n  .: error.txt \"Why the importer rejects the export.\" (inputs: positions)\n  ./b: error.txt \"Why.\" (inputs: positions)"},
			{"outputs", func(l *registry.Lift) { l.Outputs[0].Inputs = registry.InputsAll }, "outputs:\n  .: error.txt \"Why the importer rejects the export.\" (inputs: positions)\n  ./b: error.txt \"Why the importer rejects the export.\" (inputs: all)"},
			{"productFiles", func(l *registry.Lift) { l.ProductFiles = l.ProductFiles[:1] }, "productFiles:\n  .: " + products("func") + "\n  ./b: evaluate/* \"What evaluate does, as a script's evaluate/ tree says.\" (inputs: func)"},
			{"productFiles", func(l *registry.Lift) { l.ProductFiles[0].Inputs = registry.InputsAll }, "productFiles:\n  .: " + products("func") + "\n  ./b: " + products("all")},
			{"rejects", func(l *registry.Lift) { l.Rejects = nil }, "rejects:\n  .: unterminated\n  ./b: none"},
		} {
			r := edit("import", "b's registry records another "+tt.field+" for import", tt.change)
			s.fails(r, breakage+tt.field+":\n"+tt.want+fix)
			if n := strings.Count(r.out, breakage); n != 1 {
				t.Errorf("%s: the check reports import %d times, want once:\n%s", tt.field, n, r.out)
			}
		}

		// Two fields that differ give one error, which names both.
		r := edit("import", "b's registry records another description and other rejects for import", func(l *registry.Lift) {
			description(l)
			l.Rejects = nil
		})
		s.fails(r, breakage+"description and rejects:\n  description:\n    .: \"Imports the export as a script, as its import/ tree says.\"\n    ./b: \"Imports the export.\"\n  rejects:\n    .: unterminated\n    ./b: none"+fix)
		if n := strings.Count(r.out, breakage); n != 1 {
			t.Errorf("two fields: the check reports import %d times, want once:\n%s", n, r.out)
		}

		// parse is an ordinary lift: ./b stages the cells past it by its own
		// declaration.
		s.fails(edit("parse", "b's registry records another description for parse", func(l *registry.Lift) { l.Description = "Parses." }),
			"the registries that own or use the lift parse differ in description:\n  description:\n    .: \"Parses the script, as its parse/ tree says.\"\n    ./b: \"Parses.\"\nEvery package that owns or uses a lift registers one declaration of it: a package that only uses a lift runs its own declaration of it to stage the cells past it, and the lift's cell, which the package that owns it writes, records what the owner's declaration does. Declare the lift once, in a package the others import, and register that declaration."+staleRegistry)

		// The packages register one declaration of each lift, so the
		// registry is stale, and the module's update rewrites it.
		update()
		s.same(reg, clean)
		s.passes(s.normal("chain-a"))
		s.done()
	})

	// ./b has no _test.go file, so no test runs what its registry records:
	// the registry fails on its own, and its entry of import isn't
	// compared with the owner's.
	t.Run("an untested package", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "chain-shared")
		for _, r := range s.parallel(childRun{"", "chain-a", true}, childRun{"b", "chain-b", true}) {
			s.passes(r)
		}
		const reg = "b/testdata/hypercase.json"
		g, err := registry.Parse(s.read(reg))
		if err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(g.Lifts, func(l registry.Lift) bool { return l.Name == "import" })
		g.Lifts[i].Description = "Imports the export."
		s.write(reg, g.Marshal(), "b's registry records another description for import")
		s.remove("b/corpus_test.go", "remove b's tests, and leave its registry")
		r := s.normal("chain-a")
		s.fails(r, "b/testdata/hypercase.json is a registry, and b holds no _test.go file, so no test compares it, and it still claims sections.\nDelete it, or restore the package's tests.")
		s.lacks(r, "the registries that own or use the lift")
		s.done()
	})
}

// TestBytesLiftRefusals: a bytes lift gives a jenny, and declares its
// product's files, each in a tree other than params, and its cell's
// error.txt alone. No other lift declares a product.
func TestBytesLiftRefusals(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	for _, tt := range []struct{ reg, want string }{
		{"bytes-run", "panic: hypercase: the lift import's product, hypercase_test.Script, is a corpus type, and the lift gives Run. Give it Jenny in place of Run: the files the jenny generates from the input are the product's input view."},
		{"bytes-both", "panic: hypercase: the lift import gives both Run and Jenny. Its product, hypercase_test.Script, is a corpus type, whose files the jenny makes: remove Run."},
		{"bytes-no-jenny", "panic: hypercase: the lift import's product, hypercase_test.Script, is a corpus type, and the lift gives no Jenny. Give it the jenny that makes the product's files."},
		{"bytes-no-product", "panic: hypercase: the lift import's product, hypercase_test.Script, is a corpus type, and the lift declares no Product. Declare the product's files in Product, each with a pattern and a description, as a suite declares its outputs."},
		{"bytes-stderr", `panic: hypercase: the lift import declares an output with the pattern "stderr.txt", which doesn't match error.txt, the only file a bytes lift's cell writes: its jenny has no stderr. Remove the declaration.`},
		{"bytes-product-flat", `panic: hypercase: the lift import declares a product file with the pattern "*", which matches only files outside any tree. A product's file is in a tree, a top-level directory of its input view, as sheet/A1 is: fix the pattern.`},
		{"bytes-product-params", `panic: hypercase: the lift import declares a product file with the pattern "params/*", in the tree params, which every input view reserves. Fix the pattern.`},
		{"bytes-product-pattern", `panic: hypercase: the lift import declares a product file with the pattern "parse/[", which is not a pattern of slash paths in its product's input view, in the syntax of path.Match. Fix the pattern.`},
		{"stage-product", "panic: hypercase: the lift parse declares a Product, and its product, *hypercase_test.Parsed, isn't a corpus type, so it makes no files. Remove Product."},
	} {
		r := s.normal(tt.reg)
		s.fails(r, tt.want)
		s.lacks(r, "--- ")
	}
	s.fails(s.normal("bytes-unregistered"), "the lift import's product, hypercase_test.Script, is shaped as a corpus type, and the package registers no such corpus type.\nRegister the corpus type, whose input view the lift's product is, or make the product a stage, a type that isn't a named fs.FS wrapper.")
	s.fails(s.normal("bytes-product-nodesc"), "the lift import declares the product file parse/* with no description.\nAdd one: it says what the files hold, in prose.")
	s.done()
}
