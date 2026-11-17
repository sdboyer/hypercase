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

// TestTestdataLinks: Testdata follows no symbolic link, and returns each
// one under testdata/, whatever it names and wherever it sits: a link to
// a directory, to a file, to another corpus root, under testdata/checks/,
// below a corpus's directory, one that names nothing, a loop, and one in
// place of testdata/. It calls file with none of them. Link, on a path
// through each, returns it, and on a path through none, nothing.
func TestTestdataLinks(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"shared/sheet/x.txtar", "shared/other/w.txtar", "td/a/z.txtar", "td/b.txtar", "td/checks/found.txt"} {
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
		{"td/other", "../shared/other"},
		{"td/checks/linked.txt", "found.txt"},
		{"td/a/old", "../../shared/sheet"},
		{"td/dangling", "../shared/missing"},
		{"td/loop", "loop"},
		{"linked", "td"},
	} {
		if err := os.Symlink(filepath.FromSlash(l.target), filepath.Join(dir, filepath.FromSlash(l.link))); err != nil {
			t.Skipf("making a symbolic link failed: %v", err)
		}
	}
	var got []string
	links, err := Testdata(td, map[string]bool{filepath.Join(td, "other"): true}, func(p string, d fs.DirEntry) {
		rel, _ := filepath.Rel(td, p)
		got = append(got, filepath.ToSlash(rel))
	})
	var rels []string
	for _, l := range links {
		rel, _ := filepath.Rel(td, l)
		rels = append(rels, filepath.ToSlash(rel))
	}
	want, wantLinks := []string{"a/z.txtar", "b.txtar", "checks/found.txt"}, []string{"a/old", "c.txtar", "checks/linked.txt", "dangling", "loop", "other", "sheet"}
	if err != nil || !slices.Equal(got, want) || !slices.Equal(rels, wantLinks) {
		t.Errorf("Testdata visited %q, with the links %q and the error %v; want %q, with the links %q", got, rels, err, want, wantLinks)
	}

	got = nil
	linked := filepath.Join(dir, "linked")
	links, err = Testdata(linked, nil, func(p string, d fs.DirEntry) { got = append(got, p) })
	if err != nil || got != nil || !slices.Equal(links, []string{linked}) {
		t.Errorf("Testdata of a link in place of testdata/ visited %q, with the links %q and the error %v; want none, with the link alone", got, links, err)
	}

	for _, tt := range []struct{ td, p, want string }{
		{"td", "td/a/z.txtar", ""},
		{"td", "td/b.txtar", ""},
		{"td", "td/sheet/x.txtar", "td/sheet"},
		{"td", "td/c.txtar", "td/c.txtar"},
		{"td", "td/checks/linked.txt", "td/checks/linked.txt"},
		{"td", "td/a/old/x.txtar", "td/a/old"},
		{"td", "td/dangling/x.txtar", "td/dangling"},
		{"td", "td/loop/x.txtar", "td/loop"},
		{"linked", "linked/a/z.txtar", "linked"},
	} {
		want := tt.want
		if want != "" {
			want = filepath.Join(dir, filepath.FromSlash(want))
		}
		got, err := Link(filepath.Join(dir, tt.td), filepath.Join(dir, filepath.FromSlash(tt.p)))
		if err != nil || got != want {
			t.Errorf("Link(%s, %s) = %q, %v; want %q", tt.td, tt.p, got, err, want)
		}
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
