package snapshot

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
)

var cells = map[string]fs.FS{"sheet": fstest.MapFS{"A1": {Data: []byte("=B1*2\n")}, "B1": {Data: []byte("3\n")}}}

// TestCreate: Create writes the sections, under an empty comment, as
// testdata/<dir>/<name>.txtar, making the missing testdata/sheet/, and
// returns the path.
func TestCreate(t *testing.T) {
	root := t.TempDir()
	path, err := Create(root, sheetType, "twice", serialize(t, cells))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "testdata", "sheet", "twice.txtar"); path != want {
		t.Errorf("Create returned %s, not %s", path, want)
	}
	if got, want := string(readFile(t, path)), "-- in/sheet/A1 --\n=B1*2\n-- in/sheet/B1 --\n3\n"; got != want {
		t.Errorf("Create wrote:\n%s\nnot:\n%s", got, want)
	}
}

// TestCreateExisting: Create refuses an existing case, naming it, and
// leaves it byte-identical.
func TestCreateExisting(t *testing.T) {
	root, path := corpus(t)
	before := readFile(t, path)
	_, err := Create(root, sheetType, "twice", serialize(t, cells))
	fails(t, err, path+" exists, and Create never replaces a case.")
	same(t, path, before)
}

// TestCreateConcurrent: of eight Creates of one name at once, exactly one
// succeeds, and the others say the case exists.
func TestCreateConcurrent(t *testing.T) {
	root := t.TempDir()
	ss := serialize(t, cells)
	errs := make([]error, 8)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() { _, errs[i] = Create(root, sheetType, "twice", ss) })
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
	if got := readFile(t, filepath.Join(root, "testdata", "sheet", "twice.txtar")); !bytes.Equal(got, format(ss)) {
		t.Errorf("the case holds:\n%s", got)
	}
}

// TestCreateRefuses: a name that is a path, or that the naming rule
// rejects; a section outside in/; no sections, or sections with a problem
// of form; a corpus root that isn't a directory; and a corpus type's
// directory no type can declare: each fails, naming it, and nothing is
// written.
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
		root := t.TempDir()
		_, err := Create(root, c.typ, c.name, c.sections)
		fails(t, err, c.want)
		if entries, _ := os.ReadDir(root); len(entries) > 0 {
			t.Errorf("a refused Create of %q wrote %v", c.name, entries)
		}
	}
	file := filepath.Join(t.TempDir(), "root")
	writeFile(t, file, nil)
	_, err := Create(file, sheetType, "twice", ss)
	fails(t, err, "the corpus root "+file+" isn't a directory")
}

// TestCreateFailsPartWay: a Create whose write fails after some of the
// bytes are written leaves no file.
func TestCreateFailsPartWay(t *testing.T) {
	root := t.TempDir()
	var saw []byte
	afterCreateWrite = func(path string) error {
		saw = readFile(t, path)
		return errors.New("the disk is full")
	}
	_, err := Create(root, sheetType, "twice", serialize(t, cells))
	afterCreateWrite = nil
	path := filepath.Join(root, "testdata", "sheet", "twice.txtar")
	fails(t, err, "writing "+path+": the disk is full. The partial file is removed.")
	if len(saw) == 0 {
		t.Error("the hook saw no bytes written")
	}
	absent(t, path)
	if _, err := Create(root, sheetType, "twice", serialize(t, cells)); err != nil {
		t.Errorf("the next Create failed: %v", err)
	}
}

// TestCreateNoProperties: a new case states no property, and its comment
// is empty.
func TestCreateNoProperties(t *testing.T) {
	path, err := Create(t.TempDir(), sheetType, "twice", serialize(t, cells))
	if err != nil {
		t.Fatal(err)
	}
	a := archive.Parse(readFile(t, path))
	if len(a.Comment) != 0 || a.Properties() != nil {
		t.Errorf("the new case has the comment %q and the properties %q", a.Comment, a.Properties())
	}
}
