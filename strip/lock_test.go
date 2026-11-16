package strip

import (
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

// These tests set the hooks, so none runs in parallel.

// steps records the steps of the writes the hooks see, in order.
type steps struct {
	mu   sync.Mutex
	seen []string
}

func (s *steps) add(step string) {
	s.mu.Lock()
	s.seen = append(s.seen, step)
	s.mu.Unlock()
}

func (s *steps) got() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.seen)
}

const twice = `A1 is twice B1.
-- in/sheet/A1 --
=B1*2
-- in/sheet/B1 --
3
-- out/evaluate/outcome --
accepted
-- out/evaluate/values.txt --
A1 6
B1 3
`

const twiceStripped = `A1 is twice B1.
-- in/sheet/A1 --
=B1*2
-- in/sheet/B1 --
3
`

// TestLockAroundEachArchive: strip takes the corpus root's lock before it
// reads an archive, and releases it only after the rename. What it reads
// is the archive as it is once the lock is held, and another writer, B,
// that asks for the lock while strip holds it gets it only after strip's
// rename.
func TestLockAroundEachArchive(t *testing.T) {
	root := corpusRoot(t, map[string]string{"sheet/twice.txtar": "A1 is twice B1.\n-- in/sheet/A1 --\n=B1*2\n"})
	p := filepath.Join(root, "testdata", "sheet", "twice.txtar")
	lock := filepath.Join(root, "testdata", archive.LockFile)
	var s steps
	held := make(chan *lockedfile.File, 1)
	hooks.Modify = func(step string) {
		s.add("strip " + step)
		switch step {
		case "locked":
			// The archive an update wrote just before strip took the lock.
			if err := os.WriteFile(p, []byte(twice), 0o644); err != nil {
				t.Error(err)
			}
		case "read":
			go func() {
				f, err := lockedfile.OpenFile(lock, os.O_RDWR|os.O_CREATE, 0o666)
				if err != nil {
					t.Error(err)
				}
				data, _ := os.ReadFile(p)
				s.add("B locked, and reads the stripped archive: " + boolText(string(data) == twiceStripped))
				held <- f
			}()
		case "renamed":
			if data, _ := os.ReadFile(p); string(data) != twiceStripped {
				t.Errorf("at the rename, the archive is:\n%s", data)
			}
		}
	}
	r, err := Archive(root, p, Selection{})
	hooks.Modify = nil
	if err != nil {
		t.Fatal(err)
	}
	if f := <-held; f != nil {
		f.Close()
	}
	if want := []string{"out/evaluate/outcome", "out/evaluate/values.txt"}; !slices.Equal(r.Removed, want) {
		t.Errorf("Archive removed %q, not %q: it didn't read the archive as it was once the lock was held", r.Removed, want)
	}
	want := []string{"strip lock", "strip locked", "strip read", "strip renamed", "strip unlock", "B locked, and reads the stripped archive: true"}
	if got := s.got(); !slices.Equal(got, want) {
		t.Errorf("the steps ran in the order %q, not %q", got, want)
	}
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// TestWaitsForTheLock: while an update holds the corpus root's lock,
// strip waits for it. Strip asks for the lock, the update releases it, and
// only then does strip hold it and read the archive.
func TestWaitsForTheLock(t *testing.T) {
	root := corpusRoot(t, map[string]string{"sheet/twice.txtar": twice})
	p := filepath.Join(root, "testdata", "sheet", "twice.txtar")
	unlock, err := lockedfile.MutexAt(filepath.Join(root, "testdata", archive.LockFile)).Lock()
	if err != nil {
		t.Fatal(err)
	}
	var s steps
	asked := make(chan struct{})
	hooks.Modify = func(step string) {
		s.add("strip " + step)
		if step == "lock" {
			close(asked)
		}
	}
	defer func() { hooks.Modify = nil }()
	done := make(chan error)
	go func() {
		_, err := Archive(root, p, Selection{})
		done <- err
	}()
	<-asked
	if data, _ := os.ReadFile(p); string(data) != twice {
		t.Errorf("strip changed the archive while the update held the lock:\n%s", data)
	}
	s.add("the update unlocks")
	unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	want := []string{"strip lock", "the update unlocks", "strip locked", "strip read", "strip renamed", "strip unlock"}
	if got := s.got(); !slices.Equal(got, want) {
		t.Errorf("the steps ran in the order %q, not %q", got, want)
	}
	if data, _ := os.ReadFile(p); string(data) != twiceStripped {
		t.Errorf("once the lock was free, strip left:\n%s", data)
	}
}

// TestNothingToStrip: an archive with nothing to strip is read under the
// lock and never opened for writing: no temp file is written, no rename
// is made, and the file is the one that was there.
func TestNothingToStrip(t *testing.T) {
	root := corpusRoot(t, map[string]string{"sheet/twice.txtar": twiceStripped})
	p := filepath.Join(root, "testdata", "sheet", "twice.txtar")
	before, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	var s steps
	hooks.Modify = s.add
	hooks.AfterTemp = func(tmp string) error {
		s.add("temp " + filepath.Base(tmp))
		return nil
	}
	defer func() { hooks.Modify, hooks.AfterTemp = nil, nil }()
	r, err := Archive(root, p, Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Removed) > 0 || len(r.Frozen) > 0 {
		t.Errorf("Archive = %+v, of an archive with nothing to strip", r)
	}
	if err := Remove(root, p, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := s.got(), []string{"lock", "locked", "read", "unlock"}; !slices.Equal(got, want) {
		t.Errorf("the steps were %q, not %q", got, want)
	}
	after, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Error("the archive's file was replaced")
	}
}

// TestFailedWrite: a write that fails once its temp file is written
// leaves the archive as it was, with no temp file, and releases the lock,
// so the next strip completes.
func TestFailedWrite(t *testing.T) {
	root := corpusRoot(t, map[string]string{"sheet/twice.txtar": twice})
	p := filepath.Join(root, "testdata", "sheet", "twice.txtar")
	var s steps
	hooks.Modify = s.add
	hooks.AfterTemp = func(tmp string) error { return errors.New("disk full") }
	_, err := Archive(root, p, Selection{})
	hooks.Modify, hooks.AfterTemp = nil, nil
	if err == nil || !strings.Contains(err.Error(), "stripping it failed, and the archive is as it was: ") || !strings.Contains(err.Error(), "disk full. Fix what failed") {
		t.Fatalf("Archive = %v, want the failed write", err)
	}
	if data, _ := os.ReadFile(p); string(data) != twice {
		t.Errorf("a failed write changed the archive:\n%s", data)
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	for _, e := range entries {
		if archive.IsTemp(e.Name()) {
			t.Errorf("a failed write left its temp file, %s", e.Name())
		}
	}
	if got, want := s.got(), []string{"lock", "locked", "read", "unlock"}; !slices.Equal(got, want) {
		t.Errorf("the steps were %q, not %q", got, want)
	}
	if _, err := Archive(root, p, Selection{}); err != nil {
		t.Fatalf("the next strip failed: %v", err)
	}
	if data, _ := os.ReadFile(p); string(data) != twiceStripped {
		t.Errorf("the next strip left:\n%s", data)
	}
}
