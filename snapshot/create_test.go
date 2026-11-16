package snapshot

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/hooks"
	"github.com/sdboyer/hypercase/internal/registry"
	"github.com/sdboyer/hypercase/root"
)

var cells = map[string]fs.FS{"sheet": fstest.MapFS{"A1": {Data: []byte("=B1*2\n")}, "B1": {Data: []byte("3\n")}}}

// dirs returns the paths under dir, with slashes, in the walk's order.
func dirs(t *testing.T, dir string) []string {
	t.Helper()
	var got []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != dir {
			rel, _ := filepath.Rel(dir, p)
			got = append(got, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// noTemps fails the test if dir holds a temp file.
func noTemps(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if archive.IsTemp(e.Name()) {
			t.Errorf("%s holds the temp file %s", dir, e.Name())
		}
	}
}

// TestCreate: Create writes the sections, under an empty comment, as
// testdata/<dir>/<name>.txtar, making the missing testdata/sheet/, and
// returns the path.
func TestCreate(t *testing.T) {
	corp := newCorpus(t)
	path, err := Create(corp, project, sheetType, "twice", serialize(t, cells))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(corp.Dir(), "testdata", "sheet", "twice.txtar"); path != want {
		t.Errorf("Create returned %s, not %s", path, want)
	}
	if got, want := string(readFile(t, path)), "-- in/sheet/A1 --\n=B1*2\n-- in/sheet/B1 --\n3\n"; got != want {
		t.Errorf("Create wrote:\n%s\nnot:\n%s", got, want)
	}
}

// TestCreateExisting: Create refuses an existing case, naming it, leaves
// it byte-identical, and leaves no temp file beside it.
func TestCreateExisting(t *testing.T) {
	corp, path := corpus(t)
	before := readFile(t, path)
	_, err := Create(corp, project, sheetType, "twice", serialize(t, cells))
	fails(t, err, path+" exists, and Create never replaces a case.")
	same(t, path, before)
	noTemps(t, filepath.Dir(path))
}

// TestCreateConcurrent: of eight Creates of one name at once, exactly one
// succeeds, the others say the case exists, and no temp file is left.
func TestCreateConcurrent(t *testing.T) {
	corp := newCorpus(t)
	ss := serialize(t, cells)
	errs := make([]error, 8)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() { _, errs[i] = Create(corp, project, sheetType, "twice", ss) })
	}
	wg.Wait()
	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case !strings.Contains(err.Error(), "exists, and Create never replaces a case"):
			t.Errorf("a Create that lost failed otherwise: %v", err)
		}
	}
	if won != 1 {
		t.Errorf("%d Creates succeeded, not 1", won)
	}
	path := filepath.Join(corp.Dir(), "testdata", "sheet", "twice.txtar")
	if got := readFile(t, path); !bytes.Equal(got, format(ss)) {
		t.Errorf("the case holds:\n%s", got)
	}
	noTemps(t, filepath.Dir(path))
}

// TestCreateRefuses: a name that is a path, or that the naming rule
// rejects; a section outside in/; no sections, or sections with a problem
// of form; a corpus root that isn't a directory, and no corpus root; and
// a corpus type's directory no type can declare: each fails, naming it,
// and nothing is written.
func TestCreateRefuses(t *testing.T) {
	ss := serialize(t, cells)
	for _, c := range []struct {
		name     string
		typ      decl.Type[Sheet]
		sections []archive.Section
		want     string
	}{
		{"a/b", sheetType, ss, `the case's name, "a/b", breaks the naming rule`},
		{"..", sheetType, ss, `the case's name, "..", breaks the naming rule`},
		{"Open_Quote", sheetType, ss, `the case's name, "Open_Quote", breaks the naming rule`},
		{"twice", sheetType, append(ss[:1:1], archive.Section{Name: "out/evaluate/values.txt", Data: []byte("A1 6\n")}), "the section out/evaluate/values.txt isn't under in/"},
		{"twice", sheetType, append(ss[:1:1], archive.Section{Name: "properties", Data: []byte("circular\n")}), "the section properties isn't under in/"},
		{"twice", sheetType, nil, "there are no sections for the case twice"},
		{"twice", sheetType, []archive.Section{{Name: "in/params/x", Data: []byte("1\n")}}, "in/params/x: the tree name params is reserved"},
		{"twice", sheetType, append(ss[:1:1], ss[0]), "in/sheet/A1: the section appears twice"},
		{"twice", sheetType, []archive.Section{{Name: "in/sheet/A1", Data: []byte("3")}}, "section in/sheet/A1 can't be held: it has no final newline"},
		{"twice", decl.Type[Sheet]{Dir: "../sheet"}, ss, `the corpus type's directory, "../sheet", breaks the naming rule`},
		{"twice", decl.Type[Sheet]{Dir: "checks"}, ss, "the corpus type's directory, checks, is reserved for the findings of corpus checks"},
	} {
		corp := newCorpus(t)
		_, err := Create(corp, project, c.typ, c.name, c.sections)
		fails(t, err, c.want)
		if entries, _ := os.ReadDir(corp.Dir()); len(entries) > 0 {
			t.Errorf("a refused Create of %q wrote %v", c.name, entries)
		}
	}
	mod := newModule(t)
	file := filepath.Join(mod, "corp")
	writeFile(t, file, nil)
	_, err := Create(corpusIn(t, mod, "corp"), project, sheetType, "twice", ss)
	fails(t, err, "the corpus root "+file+" isn't a directory")
	_, err = Create(root.Corpus{}, project, sheetType, "twice", ss)
	fails(t, err, "Create was given no corpus root.\nFind it with root.Find")
}

// TestCreateUnrecordedRoot: a corpus root that no registry under the
// module records is refused, naming the roots the registries record, and
// nothing is written; so is a module with no registry. Any package's
// registry records a root, not only the module root's. A registry Create
// can't read is refused, naming it, even beside one that records the
// root.
func TestCreateUnrecordedRoot(t *testing.T) {
	mod := newModule(t)
	before := dirs(t, mod)
	_, err := Create(corpusIn(t, mod, "."), project, sheetType, "twice", serialize(t, cells))
	fails(t, err, `no registry under the module root `+mod+` records the corpus root ".", so no package's tests would read the case: its registries record "corp".`, "Pass root.Find the path a package passes to hypercase.Root")
	_, err = Create(corpusIn(t, mod, "other"), project, sheetType, "twice", serialize(t, cells))
	fails(t, err, `records the corpus root "other"`, `its registries record "corp".`)
	if got := dirs(t, mod); !slices.Equal(got, before) {
		t.Errorf("a refused Create left %q, not %q", got, before)
	}

	bare := newBareModule(t)
	_, err = Create(corpusIn(t, bare, "."), project, sheetType, "twice", serialize(t, cells))
	fails(t, err, `records the corpus root ".", so no package's tests would read the case: no registry of a package with tests is under it.`, "A new package's first update, "+updateVar+"=1 go test ./..., writes its registry: it fails, since the package's suites apply to no case yet, and writes the registry still.")
	if got := dirs(t, bare); !slices.Equal(got, []string{"go.mod"}) {
		t.Errorf("a refused Create left %q", got)
	}

	// ./pkg records the module root.
	writeRegistry(t, filepath.Join(mod, "pkg"), ".")
	if _, err := Create(corpusIn(t, mod, "."), project, sheetType, "twice", serialize(t, cells)); err != nil {
		t.Errorf("Create refused a root ./pkg's registry records: %v", err)
	}
	reg := filepath.Join(mod, "pkg", "testdata", registry.Name)
	writeFile(t, reg, []byte("{"))
	_, err = Create(corpusIn(t, mod, "corp"), project, sheetType, "twice", serialize(t, cells))
	fails(t, err, reg+" can't be read: it is not a registry", ", and Create writes nothing while a registry under the module root can't be read.\nRun the update, "+updateVar+"=1 go test ./..., to write it again.")
}

// TestCreateStaleRegistry: a registry in a package with no _test.go file
// records what no test runs, so a corpus root that only such a registry
// records is refused, naming it, and nothing is written. Once a tested
// package's registry records the root, Create writes.
func TestCreateStaleRegistry(t *testing.T) {
	mod := newModule(t)
	if err := os.Remove(filepath.Join(mod, testFile)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(mod, "corp"), 0o755); err != nil {
		t.Fatal(err)
	}
	before := dirs(t, mod)
	reg := filepath.Join(mod, "testdata", registry.Name)
	_, err := Create(corpusIn(t, mod, "corp"), project, sheetType, "twice", serialize(t, cells))
	fails(t, err, `the corpus root "corp" is recorded only by registries in packages with no _test.go file, `+reg+`, so no test reads cases under it.`, "Restore the package's tests, or pass root.Find the corpus root of a package that has them.")
	if got := dirs(t, mod); !slices.Equal(got, before) {
		t.Errorf("a refused Create left %q, not %q", got, before)
	}
	writeRegistry(t, filepath.Join(mod, "pkg"), "corp")
	if _, err := Create(corpusIn(t, mod, "corp"), project, sheetType, "twice", serialize(t, cells)); err != nil {
		t.Errorf("Create refused a root a tested package's registry records: %v", err)
	}
}

// TestCreateUndeclaredDir: a corpus type's directory that no tested
// registry of the corpus root declares is refused, naming the directories
// they declare, and nothing is written. A suite over the raw input
// declares its corpus, as a corpus type declares its directory. A
// registry in a package with no _test.go file declares nothing, and nor
// does one of another corpus root.
func TestCreateUndeclaredDir(t *testing.T) {
	corp := newCorpus(t)
	ss := serialize(t, cells)
	_, err := Create(corp, project, decl.Type[Sheet]{Dir: "undeclared"}, "twice", ss)
	fails(t, err, `no registry of the corpus root "corp" declares the directory undeclared, as a corpus type's directory or the corpus of a suite over the raw input, so no package's tests would read the case: they declare "sheet".`, "Pass the corpus type a package of the corpus root registers.")
	if got := dirs(t, corp.Dir()); len(got) > 0 {
		t.Errorf("a refused Create wrote %q", got)
	}

	writeRegistryOf(t, filepath.Join(corp.Module(), "raw"), registry.Registry{CorpusRoot: "corp", Suites: []registry.Suite{{Name: "count", Consumes: registry.RawInput, Corpora: []string{"undeclared"}}}}, true)
	if _, err := Create(corp, project, decl.Type[Sheet]{Dir: "undeclared"}, "twice", ss); err != nil {
		t.Errorf("Create refused a raw-input suite's corpus: %v", err)
	}

	writeRegistryOf(t, filepath.Join(corp.Module(), "stale"), registry.Registry{CorpusRoot: "corp", Types: []registry.Type{{Dir: "loose"}}}, false)
	_, err = Create(corp, project, decl.Type[Sheet]{Dir: "loose"}, "twice", ss)
	fails(t, err, `declares the directory loose`, `they declare "sheet", "undeclared".`)
	writeRegistryOf(t, filepath.Join(corp.Module(), "elsewhere"), registry.Registry{CorpusRoot: "other", Types: []registry.Type{{Dir: "away"}}}, true)
	_, err = Create(corp, project, decl.Type[Sheet]{Dir: "away"}, "twice", ss)
	fails(t, err, `declares the directory away`, `they declare "sheet", "undeclared".`)
}

// TestCreateDuringUpdate: while the variable the project's UpdateVar
// names is set, Create fails, naming it, and writes nothing. A project
// with no UpdateVar fails, and nothing is written.
func TestCreateDuringUpdate(t *testing.T) {
	corp := newCorpus(t)
	t.Setenv(updateVar, "1")
	_, err := Create(corp, project, sheetType, "twice", serialize(t, cells))
	fails(t, err, updateVar+" is set, so this may be an update, and no update creates a case.\nCreate the case from the project's command, or from a test run without "+updateVar+".")
	_, err = Create(corp, decl.Project{}, sheetType, "twice", serialize(t, cells))
	fails(t, err, "Create has no update variable to check: the project's UpdateVar is empty, and Create refuses during an update.\nPass the decl.Project the project registers.")
	if got := dirs(t, corp.Dir()); len(got) > 0 {
		t.Errorf("a refused Create wrote %q", got)
	}
}

// TestCreateNoPartialFile: Create writes the case to a temp file beside
// it, whose name no run reads as an archive, and gives the file the case's
// name only once every byte is written and synced: a reader at that
// moment finds no case, never part of one. No temp file is left after.
func TestCreateNoPartialFile(t *testing.T) {
	corp := newCorpus(t)
	ss := serialize(t, cells)
	path := filepath.Join(corp.Dir(), "testdata", "sheet", "twice.txtar")
	var temp string
	hooks.AfterTemp = func(tmp string) error {
		temp = tmp
		if !archive.IsTemp(tmp) || strings.HasSuffix(tmp, ".txtar") || filepath.Dir(tmp) != filepath.Dir(path) {
			t.Errorf("the temp file is %s, which a run may read as an archive, or which isn't beside the case", tmp)
		}
		if got := readFile(t, tmp); !bytes.Equal(got, format(ss)) {
			t.Errorf("the temp file holds:\n%s", got)
		}
		absent(t, path)
		return nil
	}
	_, err := Create(corp, project, sheetType, "twice", ss)
	hooks.AfterTemp = nil
	if err != nil {
		t.Fatal(err)
	}
	if temp == "" {
		t.Fatal("Create wrote no temp file")
	}
	same(t, path, format(ss))
	absent(t, temp)
}

// TestCreateFailsPartWay: a Create whose write fails once the temp file is
// written leaves neither the case nor the temp file, and keeps
// testdata/sheet/, whether it made the directories or found them: from an
// empty corpus root, with testdata/ there, and with both there. The next
// Create completes.
func TestCreateFailsPartWay(t *testing.T) {
	want := []string{"testdata", "testdata/sheet"}
	for _, pre := range []string{"", "testdata", "testdata/sheet"} {
		corp := newCorpus(t)
		if pre != "" {
			if err := os.MkdirAll(filepath.Join(corp.Dir(), pre), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		var temp string
		hooks.AfterTemp = func(tmp string) error {
			temp = tmp
			return errors.New("the disk is full")
		}
		_, err := Create(corp, project, sheetType, "twice", serialize(t, cells))
		hooks.AfterTemp = nil
		path := filepath.Join(corp.Dir(), "testdata", "sheet", "twice.txtar")
		fails(t, err, "creating "+path+": the disk is full.\nFix what stopped the write, and create the case again.")
		if temp == "" {
			t.Fatal("Create wrote no temp file")
		}
		if got := dirs(t, corp.Dir()); !slices.Equal(got, want) {
			t.Errorf("with %q there, a failed Create left %q, not %q", pre, got, want)
		}
		if _, err := Create(corp, project, sheetType, "twice", serialize(t, cells)); err != nil {
			t.Errorf("the next Create failed: %v", err)
		}
	}
}

// TestCreateOpenFails: a Create whose temp file can't be opened, here
// because the temp file's name, longer than the case's, is longer than the
// filesystem holds, writes no file, and keeps the directories it made.
func TestCreateOpenFails(t *testing.T) {
	corp := newCorpus(t)
	name := strings.Repeat("a", 240)
	_, err := Create(corp, project, sheetType, name, serialize(t, cells))
	fails(t, err, "creating "+filepath.Join(corp.Dir(), "testdata", "sheet", name+".txtar")+": open ", "file name too long")
	if got, want := dirs(t, corp.Dir()), []string{"testdata", "testdata/sheet"}; !slices.Equal(got, want) {
		t.Errorf("a Create that couldn't open its temp file left %q, not %q", got, want)
	}
}

// TestCreateFailsBesideAnother: a Create, A, that fails after another, B,
// has run to the end in the directories A made leaves B's case as B
// wrote it. A's hook runs B once A's temp file is written, then fails A.
func TestCreateFailsBesideAnother(t *testing.T) {
	corp := newCorpus(t)
	ss := serialize(t, cells)
	aTemp := ".hypercase-tmp-first.txtar-"
	var bPath string
	var bErr error
	hooks.AfterTemp = func(tmp string) error {
		if !strings.HasPrefix(filepath.Base(tmp), aTemp) {
			return nil
		}
		bPath, bErr = Create(corp, project, sheetType, "second", ss)
		return errors.New("the disk is full")
	}
	_, err := Create(corp, project, sheetType, "first", ss)
	hooks.AfterTemp = nil
	if bErr != nil || bPath == "" {
		t.Fatalf("B, run from A's hook, failed: %v", bErr)
	}
	dir := filepath.Join(corp.Dir(), "testdata", "sheet")
	fails(t, err, "creating "+filepath.Join(dir, "first.txtar")+": the disk is full.")
	same(t, bPath, format(ss))
	absent(t, filepath.Join(dir, "first.txtar"))
	noTemps(t, dir)
}

// TestCreateAfterACrash: a Create cut short once its temp file is written,
// here by a panic in the hook, which no cleanup undoes, leaves only the
// temp file, which no run reads as an archive. It blocks no later Create.
func TestCreateAfterACrash(t *testing.T) {
	corp := newCorpus(t)
	ss := serialize(t, cells)
	var temp string
	hooks.AfterTemp = func(tmp string) error {
		temp = tmp
		panic("the process dies")
	}
	func() {
		defer func() { recover() }()
		Create(corp, project, sheetType, "twice", ss)
	}()
	hooks.AfterTemp = nil
	path := filepath.Join(corp.Dir(), "testdata", "sheet", "twice.txtar")
	absent(t, path)
	if _, err := os.Stat(temp); err != nil {
		t.Fatalf("the cut-short Create left no temp file: %v", err)
	}
	if _, err := Create(corp, project, sheetType, "twice", ss); err != nil {
		t.Fatalf("the next Create failed: %v", err)
	}
	same(t, path, format(ss))
}

// TestCreateNoProperties: a new case states no property, and its comment
// is empty.
func TestCreateNoProperties(t *testing.T) {
	path, err := Create(newCorpus(t), project, sheetType, "twice", serialize(t, cells))
	if err != nil {
		t.Fatal(err)
	}
	a := archive.Parse(readFile(t, path))
	if len(a.Comment) != 0 || a.Properties() != nil {
		t.Errorf("the new case has the comment %q and the properties %q", a.Comment, a.Properties())
	}
}
