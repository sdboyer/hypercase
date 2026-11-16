package snapshot

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/rogpeppe/go-internal/lockedfile"
	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/hooks"
)

// The tests' update variable, which none of them sets but
// TestRewriteDuringUpdate.
const updateVar = "SNAPSHOT_TEST_UPDATE_GOLDEN"

func section(name, data string) []archive.Section {
	return []archive.Section{{Name: name, Data: []byte(data)}}
}

// replaced returns twice with one section's bytes replaced, as text.
func replaced(t *testing.T, name, old, new string) []byte {
	t.Helper()
	marker := "-- " + name + " --\n"
	data := string(twice(t))
	if !strings.Contains(data, marker+old) {
		t.Fatalf("twice has no %s holding %q", name, old)
	}
	return []byte(strings.Replace(data, marker+old, marker+new, 1))
}

// TestRewriteBase: RewriteInputs replaces a base section, and every other
// byte of the archive is as it was: the comment, the properties and gaps
// sections, the other inputs, the variants, the transform's sections and
// the outputs, each in its place.
func TestRewriteBase(t *testing.T) {
	root, path := corpus(t)
	if err := RewriteInputs(root, path, "", updateVar, section("in/sheet/B1", "4\n")); err != nil {
		t.Fatal(err)
	}
	same(t, path, replaced(t, "in/sheet/B1", "3\n", "4\n"))
}

// TestRewriteVariant: RewriteInputs on cycle replaces
// variant/cycle/in/sheet/B1, passed as in/sheet/B1, and the base's
// in/sheet/B1 is as it was.
func TestRewriteVariant(t *testing.T) {
	root, path := corpus(t)
	if err := RewriteInputs(root, path, "cycle", updateVar, section("in/sheet/B1", "=C1\n")); err != nil {
		t.Fatal(err)
	}
	same(t, path, replaced(t, "variant/cycle/in/sheet/B1", "=A1\n", "=C1\n"))
}

// TestRewriteRefuses: a name the archive lacks, one outside in/, one
// passed twice, a variant the archive lacks, a transform's variant,
// content holding a line shaped like a section marker, a rewrite that
// leaves the archive with a problem of form, no sections, and an archive
// outside the corpus root: each fails, naming it, and the archive is
// byte-identical.
func TestRewriteRefuses(t *testing.T) {
	root, path := corpus(t)
	before := readFile(t, path)
	for _, c := range []struct {
		root, variant string
		sections      []archive.Section
		want          string
	}{
		{root, "", section("in/sheet/C1", "1\n"), path + " has no section in/sheet/C1"},
		{root, "cycle", section("in/sheet/A1", "1\n"), path + " has no section variant/cycle/in/sheet/A1"},
		{root, "", section("properties", "circular\n"), "the section properties isn't under in/"},
		{root, "cycle", section("variant/cycle/in/sheet/B1", "1\n"), "the section variant/cycle/in/sheet/B1 isn't under in/"},
		{root, "", append(section("in/sheet/B1", "4\n"), section("in/sheet/B1", "5\n")...), "the section in/sheet/B1 is passed twice"},
		{root, "gone", section("in/sheet/B1", "1\n"), path + " has no variant/gone/"},
		{root, "looped", section("in/sheet/A1", "=B1\n"), path + " has no variant/looped/, only transform/looped/"},
		{root, "", section("in/sheet/B1", "3\n-- in/sheet/C1 --\n4\n"), "the section in/sheet/B1 can't be held in an archive: its line 2 is shaped like a section marker"},
		{root, "", section("in/sheet/B1", "3"), "the section in/sheet/B1 can't be held in an archive: it has no final newline"},
		{root, "", nil, "there are no sections to rewrite in " + path},
		{t.TempDir(), "", section("in/sheet/B1", "4\n"), path + " isn't in a corpus's directory under"},
	} {
		fails(t, RewriteInputs(c.root, path, c.variant, updateVar, c.sections), c.want)
		same(t, path, before)
	}

	// A variant that only replaces B1, rewritten to hold the base's B1,
	// has the base's input view.
	four := filepath.Join(root, "testdata", "sheet", "four.txtar")
	data := []byte("A1 is twice B1.\n-- in/sheet/A1 --\n=B1*2\n-- in/sheet/B1 --\n3\n-- variant/four/about --\nB1 is 4.\n-- variant/four/in/sheet/B1 --\n4\n")
	writeFile(t, four, data)
	fails(t, RewriteInputs(root, four, "four", updateVar, section("in/sheet/B1", "3\n")), four+" would have problems of form once rewritten", "variant/four: the variant's input view equals the base's")
	same(t, four, data)
}

// TestRewriteRefusesUnplaced: an archive that isn't in the form an update
// writes, such as one with no final newline, is refused, since the rewrite
// would change its other bytes too, and it is byte-identical.
func TestRewriteRefusesUnplaced(t *testing.T) {
	root, path := corpus(t)
	before := bytes.TrimSuffix(twice(t), []byte("\n"))
	writeFile(t, path, before)
	fails(t, RewriteInputs(root, path, "", updateVar, section("in/sheet/B1", "4\n")), path+" isn't in the form an update writes", "Run the update, "+updateVar+"=1 go test ./..., then rewrite the inputs.")
	same(t, path, before)

	bad := filepath.Join(root, "testdata", "sheet", "typo.txtar")
	writeFile(t, bad, []byte("A typo.\n-- ni/sheet/A1 --\n1\n"))
	fails(t, RewriteInputs(root, bad, "", updateVar, section("in/sheet/A1", "2\n")), bad+" has problems of form")
}

// TestRewriteLocks: observed through the archive's hooks, RewriteInputs
// takes the corpus root's lock before it re-reads the archive, and
// releases it after the rename: another update that asks for the lock at
// the re-read gets it only once RewriteInputs has released it.
func TestRewriteLocks(t *testing.T) {
	root, path := corpus(t)
	lock := filepath.Join(root, "testdata", archive.LockFile)
	var (
		mu    sync.Mutex
		steps []string
	)
	record := func(s string) {
		mu.Lock()
		steps = append(steps, s)
		mu.Unlock()
	}
	held := make(chan *lockedfile.File, 1)
	hooks.Modify = func(step string) {
		record("rewrite " + step)
		if step == "read" {
			go func() {
				f, err := lockedfile.OpenFile(lock, os.O_RDWR|os.O_CREATE, 0o666)
				if err != nil {
					t.Error(err)
				}
				record("update locked")
				held <- f
			}()
		}
	}
	err := RewriteInputs(root, path, "", updateVar, section("in/sheet/B1", "4\n"))
	hooks.Modify = nil
	if err != nil {
		t.Fatal(err)
	}
	if f := <-held; f != nil {
		f.Close()
	}
	mu.Lock()
	got := slices.Clone(steps)
	mu.Unlock()
	if want := []string{"rewrite locked", "rewrite read", "rewrite renamed", "rewrite unlock", "update locked"}; !slices.Equal(got, want) {
		t.Errorf("the steps ran in the order %q, not %q", got, want)
	}
}

// TestRewriteRereads: a command reads the archive, an update writes it,
// then the command's RewriteInputs runs: the update's sections survive,
// since RewriteInputs replaces sections of the archive it re-reads.
func TestRewriteRereads(t *testing.T) {
	root, path := corpus(t)
	if _, err := Load(sheetType, path); err != nil {
		t.Fatal(err)
	}
	err := archive.Modify(filepath.Join(root, "testdata", archive.LockFile), path, func(a *archive.Archive) (*archive.Archive, error) {
		for i, s := range a.Sections {
			if s.Name == "out/resolve/error.txt" {
				a.Sections[i].Data = []byte("D1: Z9 is missing\n")
			}
		}
		return a, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := RewriteInputs(root, path, "", updateVar, section("in/sheet/B1", "4\n")); err != nil {
		t.Fatal(err)
	}
	want := replaced(t, "in/sheet/B1", "3\n", "4\n")
	want = bytes.Replace(want, []byte("-- out/resolve/error.txt --\nD1: Z9 doesn't exist\n"), []byte("-- out/resolve/error.txt --\nD1: Z9 is missing\n"), 1)
	same(t, path, want)
}

// TestRewriteDuringUpdate: while the variable updateVar names is set,
// RewriteInputs fails, naming it, takes no lock, and leaves the archive
// byte-identical. With it unset, the same call replaces the section, with
// no registry under the corpus root. An empty updateVar fails, and the
// archive is byte-identical.
func TestRewriteDuringUpdate(t *testing.T) {
	root, path := corpus(t)
	before := readFile(t, path)
	var steps []string
	hooks.Modify = func(step string) { steps = append(steps, step) }
	defer func() { hooks.Modify = nil }()

	t.Setenv(updateVar, "1")
	fails(t, RewriteInputs(root, path, "", updateVar, section("in/sheet/B1", "4\n")), updateVar+" is set, so this may be an update")
	same(t, path, before)
	absent(t, filepath.Join(root, "testdata", archive.LockFile))
	if len(steps) > 0 {
		t.Errorf("RewriteInputs took steps %q during an update", steps)
	}

	fails(t, RewriteInputs(root, path, "", "", section("in/sheet/B1", "4\n")), "RewriteInputs has no update variable to check")
	same(t, path, before)

	os.Unsetenv(updateVar)
	if err := RewriteInputs(root, path, "", updateVar, section("in/sheet/B1", "4\n")); err != nil {
		t.Fatal(err)
	}
	same(t, path, replaced(t, "in/sheet/B1", "3\n", "4\n"))
	for _, reg := range []string{"hypercase.json", filepath.Join("sheet", "hypercase.json")} {
		absent(t, filepath.Join(root, "testdata", reg))
	}
}

// TestRewriteFailedWrite: a RewriteInputs whose write fails after the
// temp file is written leaves the archive as it was, with no temp file
// left over, and releases the lock, so the next rewrite completes.
func TestRewriteFailedWrite(t *testing.T) {
	root, path := corpus(t)
	before := readFile(t, path)
	var temp string
	hooks.AfterTemp = func(tmp string) error {
		temp = tmp
		return errors.New("the disk is full")
	}
	err := RewriteInputs(root, path, "", updateVar, section("in/sheet/B1", "4\n"))
	hooks.AfterTemp = nil
	fails(t, err, "rewriting "+path+": the disk is full.\nThe archive is as it was.")
	same(t, path, before)
	if temp == "" {
		t.Fatal("the write made no temp file")
	}
	absent(t, temp)
	if err := RewriteInputs(root, path, "", updateVar, section("in/sheet/B1", "4\n")); err != nil {
		t.Errorf("the next rewrite failed: %v", err)
	}
}
