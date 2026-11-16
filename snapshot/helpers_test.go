package snapshot

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/registry"
	"github.com/sdboyer/hypercase/root"
)

// The tests' update variable, which none of them sets but
// TestCreateDuringUpdate and TestRewriteDuringUpdate.
const updateVar = "SNAPSHOT_TEST_UPDATE_GOLDEN"

// project is the tests' project, whose update variable is updateVar.
var project = decl.Project{UpdateVar: updateVar}

// Sheet is the walkthrough's corpus type: a spreadsheet, one file per cell
// under in/sheet/.
type Sheet struct{ fs.FS }

var sheetType = decl.Type[Sheet]{
	Dir:         "sheet",
	Description: "A spreadsheet: one file per cell under in/sheet/, holding a number or a formula.",
}

// twice is the archive the tests work on, testdata/sheet/twice.txtar: a
// base with its comment, properties, gaps and outputs; the hand-written
// variants cycle, which replaces B1, adds C1 and deletes D1, and missing;
// and the transform looped's variant.
func twice(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "sheet", "twice.txtar"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// newModule makes a module whose root package's registry records the
// corpus root corp, and returns the module root.
func newModule(t *testing.T) string {
	t.Helper()
	mod := t.TempDir()
	writeFile(t, filepath.Join(mod, "go.mod"), []byte("module example.com/m\n"))
	writeRegistry(t, mod, "corp")
	return mod
}

// writeRegistry writes, in the package directory pkg, a registry that
// records the corpus root rel.
func writeRegistry(t *testing.T, pkg, rel string) {
	t.Helper()
	writeFile(t, filepath.Join(pkg, "testdata", registry.Name), fmt.Appendf(nil, `{"format": %d, "corpusRoot": %q}`, registry.Format, rel))
}

// corpusIn returns the corpus root rel of the module mod.
func corpusIn(t *testing.T, mod, rel string) root.Corpus {
	t.Helper()
	c, err := root.Find(mod, rel)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// newCorpus makes a module holding the empty corpus root corp, and
// returns that root.
func newCorpus(t *testing.T) root.Corpus {
	t.Helper()
	mod := newModule(t)
	if err := os.Mkdir(filepath.Join(mod, "corp"), 0o755); err != nil {
		t.Fatal(err)
	}
	return corpusIn(t, mod, "corp")
}

// corpus makes a corpus root holding twice, and returns the root and the
// archive's path.
func corpus(t *testing.T) (root.Corpus, string) {
	t.Helper()
	c := newCorpus(t)
	path := filepath.Join(c.Dir(), "testdata", "sheet", "twice.txtar")
	writeFile(t, path, twice(t))
	return c, path
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// same checks a file holds what it held.
func same(t *testing.T, path string, before []byte) {
	t.Helper()
	if got := readFile(t, path); !bytes.Equal(got, before) {
		t.Errorf("%s changed:\n%s", path, got)
	}
}

// absent checks nothing is at a path.
func absent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err == nil {
		t.Errorf("%s exists, and nothing should", path)
	}
}

// fails checks err is a failure whose message holds each of want, and
// whose last line says what to do: a sentence of its own.
func fails(t *testing.T, err error, want ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error, and want one holding %q", want)
	}
	msg := err.Error()
	for _, w := range want {
		if !strings.Contains(msg, w) {
			t.Errorf("the error doesn't hold %q:\n%s", w, msg)
		}
	}
	lines := strings.Split(msg, "\n")
	last := lines[len(lines)-1]
	if len(lines) < 2 || last == "" || !unicode.IsUpper([]rune(last)[0]) || !strings.HasSuffix(last, ".") {
		t.Errorf("the error doesn't end with what to do, a sentence on a line of its own:\n%s", msg)
	}
}

func format(ss []archive.Section) []byte { return (&archive.Archive{Sections: ss}).Format() }

func serialize(t *testing.T, trees map[string]fs.FS) []archive.Section {
	t.Helper()
	ss, err := Serialize(trees)
	if err != nil {
		t.Fatal(err)
	}
	return ss
}

func names(ss []archive.Section) []string {
	var ns []string
	for _, s := range ss {
		ns = append(ns, s.Name)
	}
	return ns
}
