package archive

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/rogpeppe/go-internal/lockedfile"
	"github.com/sdboyer/hypercase/internal/hooks"
)

// modifyFixture writes the well-formed archive in a temp dir, and returns
// its path and its corpus root's lock path.
func modifyFixture(t *testing.T) (name, lock string) {
	t.Helper()
	dir := t.TempDir()
	name = filepath.Join(dir, "x.txtar")
	if err := os.WriteFile(name, []byte(wellFormed), 0o644); err != nil {
		t.Fatal(err)
	}
	return name, filepath.Join(dir, LockFile)
}

func scaled(a *Archive, _ []byte) (*Archive, error) {
	a.Sections[1].Data = []byte("serves: 8\n")
	return a, nil
}

// TestModifyHoldsTheLockAcrossTheRename: another update, B, that asks for
// the lock while Modify, A, holds it, gets it only after A's rename, and
// holds it on the file at the lock path, the one every later update opens.
func TestModifyHoldsTheLockAcrossTheRename(t *testing.T) {
	name, lock := modifyFixture(t)
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
		record("A " + step)
		if step == "read" {
			go func() {
				f, err := lockedfile.OpenFile(lock, os.O_RDWR|os.O_CREATE, 0o666)
				if err != nil {
					t.Error(err)
				}
				record("B locked")
				held <- f
			}()
		}
	}
	err := Modify(lock, name, scaled)
	hooks.Modify = nil
	if err != nil {
		t.Fatal(err)
	}
	f := <-held
	if f == nil {
		return
	}
	defer f.Close()
	mu.Lock()
	got := slices.Clone(steps)
	mu.Unlock()
	if want := []string{"A lock", "A locked", "A read", "A renamed", "A unlock", "B locked"}; !slices.Equal(got, want) {
		t.Errorf("the steps ran in the order %q, not %q", got, want)
	}
	holds, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	atPath, err := os.Stat(lock)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(holds, atPath) {
		t.Error("B holds the lock on a file that is no longer at the lock path")
	}
	if data, _ := os.ReadFile(name); !bytes.Contains(data, []byte("serves: 8\n")) {
		t.Errorf("A's write didn't land:\n%s", data)
	}
}

// TestModifyFailedWrite: a write that fails after its temp file is written
// leaves the archive as it was, with no temp file, and releases the lock,
// so the next update completes.
func TestModifyFailedWrite(t *testing.T) {
	name, lock := modifyFixture(t)
	hooks.AfterTemp = func(string) error { return errors.New("disk full") }
	err := Modify(lock, name, scaled)
	hooks.AfterTemp = nil
	if err == nil {
		t.Fatal("Modify succeeded, and its write failed")
	}
	if data, _ := os.ReadFile(name); string(data) != wellFormed {
		t.Errorf("a failed write changed the archive:\n%s", data)
	}
	entries, _ := os.ReadDir(filepath.Dir(name))
	for _, e := range entries {
		if IsTemp(e.Name()) {
			t.Errorf("a failed write left its temp file, %s", e.Name())
		}
	}
	if err := Modify(lock, name, scaled); err != nil {
		t.Fatalf("the next update failed: %v", err)
	}
	if data, _ := os.ReadFile(name); !bytes.Contains(data, []byte("serves: 8\n")) {
		t.Errorf("the next update didn't land:\n%s", data)
	}
}

// TestModifyWritesNothing: Modify writes nothing when fn returns nil, an
// error, or the bytes it read.
func TestModifyWritesNothing(t *testing.T) {
	name, lock := modifyFixture(t)
	hooks.AfterTemp = func(string) error {
		t.Error("Modify wrote a temp file")
		return nil
	}
	defer func() { hooks.AfterTemp = nil }()
	if err := Modify(lock, name, func(*Archive, []byte) (*Archive, error) { return nil, nil }); err != nil {
		t.Error(err)
	}
	refused := errors.New("refused")
	if err := Modify(lock, name, func(*Archive, []byte) (*Archive, error) { return nil, refused }); !errors.Is(err, refused) {
		t.Errorf("Modify returned %v, not fn's error", err)
	}
	if err := Modify(lock, name, func(a *Archive, _ []byte) (*Archive, error) { return a, nil }); err != nil {
		t.Error(err)
	}
}
