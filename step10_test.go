package hypercase_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/diff"
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
	})
	s.hasSections("testdata/export/empty.txtar", map[string]string{
		"out/import/outcome": "accepted\n",
		"out/list/files.txt": "0 files\n",
	}, "via/")
	s.hasSections("testdata/hypercase.json", map[string]string{})
	reg := string(s.read("testdata/hypercase.json"))
	for _, want := range []string{
		`"format": 6,`,
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
// change in what the lift makes fails its own column alone.
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
	for _, outcome := range []string{"rejected", "panicked"} {
		s.edit(prices, "import's committed outcome is "+outcome, set("out/import/outcome", outcome+"\n"))
		r := s.normal("chain")
		s.fails(r,
			"out/import/outcome differs (-golden +got):\n- "+outcome+"\n+ accepted",
			"out/evaluate/outcome differs (-golden +got):\n- accepted\n+ blocked import",
			"out/evaluate/values.txt is recorded, and the cell is blocked: the lift import, on evaluate's path, has the committed outcome \""+outcome+"\", in out/import/outcome.")
		s.write(prices, clean, "restore it")
	}
	s.edit(prices, "import's cell has no committed outcome", drop("out/import/outcome"))
	s.fails(s.normal("chain"),
		"out/import/outcome has no golden.",
		"out/evaluate/outcome differs (-golden +got):\n- accepted\n+ blocked import",
		"out/evaluate/values.txt is recorded, and the cell is blocked: the lift import, on evaluate's path, has no committed outcome, out/import/outcome, which a normal run reads.")
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
	blocked := "via/repack/evaluate/out/values.txt is recorded, and repack is blocked: the lift unpack, on repack's path, failed on the case, so it has no product.\nRun FIXTURE_UPDATE_GOLDEN=1 go test ./..., which removes it, and read the diff before committing."
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
	s.fails(s.normal("chain"), orphan+"the lift import, on list's path, has the committed outcome \"rejected\", in out/import/outcome.")

	s.edit(names, "a product beside the rejection", insert("via/import/inputs/x", "a tree named inputs\n", "out/"))
	r := s.normal("chain")
	s.fails(r, "via/import/inputs/x is recorded, and import failed on the case, so it has no product.\nIf it should make one, fix import or the case. If not, run FIXTURE_UPDATE_GOLDEN=1 go test ./..., which removes it, and read the diff before committing.", orphan)
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
	changed := "The cell is frozen by its known gap, gaps line 1: \"import The importer reads quoted fields. Waits on sheets#9.\"\nIts committed files record today's behavior, and the run now produces different ones:\nvia/import/evaluate/out/values.txt differs (-committed +got):\n- a1 3\n- b1 6\n+ a1 4\n+ b1 8\nThe cells past import fail in normal runs until the frozen cell is resolved: an update writes them from the product import makes now, and leaves its committed product in place, which a normal run reads.\nIf the gap is closed, remove the line, and the next update writes the cell. If the wrong behavior changed shape, delete the cell's files and its product, under out/import/ and via/import/, and the next update writes them."
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
		s.fails(s.update("chain"), tt.want, "export/names: import makes files named as a cell's own files are, which list reads.\nlist: Lists the files of the script's input view, with their bytes.\nThe lift import, on list's path, made a product an archive can't hold, so the cell runs nothing, and the update writes nothing for it. The lift's cell says why, in the package that owns it.\nFix import's jenny.")
		s.unchanged(before)
		s.write(names, clean, "restore it")
	}
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
		"testdata/export/prices.txtar: no registry claims via/nobody/quote/A1 and variant/doubled/via/nobody/quote/A1: no bytes lift named nobody runs over the corpus export.\nIf a bytes lift should write them, register it. If not, delete them by hand; FIXTURE_UPDATE_GOLDEN=1 go test ./... leaves them in place.",
		"testdata/export/prices.txtar: no registry claims via/parse/quote/A1: no bytes lift named parse runs over the corpus export.")
	s.contains(r, "--- PASS: TestCorpus/export/prices/import")
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

	// import stops making quote/B1. Its owner's column fails, naming the
	// file, and the module's update removes it.
	s.edit(prices, "import no longer makes quote/B1", drop("in/import/out/quote/B1"))
	s.fails(s.normal("chain-a"), "via/import/quote/B1 is a file of import's product, which import no longer makes.\nIf it should, fix import's jenny. If not, run FIXTURE_UPDATE_GOLDEN=1 go test ./..., which removes it, and read the diff before committing.")
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
