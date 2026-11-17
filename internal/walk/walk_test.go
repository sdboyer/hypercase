package walk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestTestdata: Testdata calls file with each file under testdata/, in
// lexical order, past each directory below it that holds a go.mod or is
// another corpus root, but not testdata/ itself, and returns the walk's
// error, as for a testdata/ that doesn't exist.
func TestTestdata(t *testing.T) {
	td := t.TempDir()
	for _, f := range []string{"a/x.txtar", "a/y.txt", "b.txtar", "nested/go.mod", "nested/z.txtar", "other/w.txtar", "other-not/v.txtar"} {
		p := filepath.Join(td, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	links, err := Testdata(td, map[string]bool{filepath.Join(td, "other"): true}, func(p string, d fs.DirEntry) {
		rel, _ := filepath.Rel(td, p)
		got = append(got, filepath.ToSlash(rel))
	})
	if want := []string{"a/x.txtar", "a/y.txt", "b.txtar", "other-not/v.txtar"}; err != nil || links != nil || !slices.Equal(got, want) {
		t.Errorf("Testdata visited %q, with the links %q and the error %v; want %q", got, links, err, want)
	}

	// A go.mod directly in testdata/ makes no nested module of testdata/
	// itself, which the walk reads whole.
	got = nil
	_, err = Testdata(filepath.Join(td, "nested"), nil, func(p string, d fs.DirEntry) {
		got = append(got, filepath.Base(p))
	})
	if want := []string{"go.mod", "z.txtar"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("Testdata of a testdata/ that holds a go.mod visited %q, with the error %v; want %q", got, err, want)
	}

	got = nil
	_, err = Testdata(filepath.Join(td, "missing"), nil, func(p string, d fs.DirEntry) { got = append(got, p) })
	if !errors.Is(err, fs.ErrNotExist) || got != nil {
		t.Errorf("Testdata of a missing directory visited %q, with the error %v; want none, and an error that it doesn't exist", got, err)
	}
}

// TestTestdataLinks: Testdata follows no symbolic link. It returns each
// link in place of testdata/, or directly in it and naming a directory,
// where a run reads a corpus's archives through it. It visits any other
// link as a file: one to a file, one that names nothing, and one below a
// corpus's directory, where no run reads.
func TestTestdataLinks(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"shared/sheet/x.txtar", "shared/old/y.txtar", "td/a/z.txtar", "td/b.txtar"} {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	td := filepath.Join(dir, "td")
	for _, l := range []struct{ link, target string }{
		{"td/sheet", "../shared/sheet"},
		{"td/c.txtar", "b.txtar"},
		{"td/a/old", "../../shared/old"},
		{"td/dangling", "../shared/missing"},
		{"linked", "td"},
	} {
		if err := os.Symlink(filepath.FromSlash(l.target), filepath.Join(dir, filepath.FromSlash(l.link))); err != nil {
			t.Skipf("making a symbolic link failed: %v", err)
		}
	}
	var got []string
	links, err := Testdata(td, nil, func(p string, d fs.DirEntry) {
		rel, _ := filepath.Rel(td, p)
		got = append(got, filepath.ToSlash(rel))
	})
	want, wantLinks := []string{"a/old", "a/z.txtar", "b.txtar", "c.txtar", "dangling"}, []string{filepath.Join(td, "sheet")}
	if err != nil || !slices.Equal(got, want) || !slices.Equal(links, wantLinks) {
		t.Errorf("Testdata visited %q, with the links %q and the error %v; want %q, with the links %q", got, links, err, want, wantLinks)
	}

	got = nil
	linked := filepath.Join(dir, "linked")
	links, err = Testdata(linked, nil, func(p string, d fs.DirEntry) { got = append(got, p) })
	if err != nil || got != nil || !slices.Equal(links, []string{linked}) {
		t.Errorf("Testdata of a link in place of testdata/ visited %q, with the links %q and the error %v; want none, with the link alone", got, links, err)
	}
}

// TestIsFile: a regular file is one, and a directory and a missing path
// aren't.
func TestIsFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]bool{f: true, dir: false, filepath.Join(dir, "missing"): false} {
		if got := IsFile(p); got != want {
			t.Errorf("IsFile(%s) = %v, want %v", p, got, want)
		}
	}
}
