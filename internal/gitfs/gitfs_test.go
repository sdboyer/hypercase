package gitfs_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/sdboyer/hypercase/internal/gitfs"
	"github.com/sdboyer/hypercase/internal/gittest"
)

func open(t *testing.T, dir string) *gitfs.Repo {
	t.Helper()
	r, err := gitfs.Open(dir, gittest.Env())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	})
	return r
}

// TestTreeIsTheCommit: a tree holds the files the commit holds, whatever
// the working tree holds now.
func TestTreeIsTheCommit(t *testing.T) {
	t.Parallel()
	g := gittest.New(t)
	g.Write(map[string]string{
		"go.mod":          "module x\n",
		"a/b/c.txt":       "c\n",
		"a/d.txt":         "d\n",
		"empty.txt":       "",
		"no-newline.txt":  "end",
		"a/b/deeper/e.go": "package deeper\n",
	})
	first := g.Commit("first")
	g.Write(map[string]string{"a/d.txt": "changed\n", "a/new.txt": "new\n"})
	g.Remove("a/b")
	second := g.Commit("second")
	g.Write(map[string]string{"a/d.txt": "uncommitted\n", "wt.txt": "only in the working tree\n"})

	r := open(t, g.Dir)
	tree, err := r.Tree(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := fstest.TestFS(tree, "go.mod", "a/b/c.txt", "a/d.txt", "empty.txt", "no-newline.txt", "a/b/deeper/e.go"); err != nil {
		t.Error(err)
	}
	for name, want := range map[string]string{"a/d.txt": "d\n", "no-newline.txt": "end", "empty.txt": ""} {
		got, err := fs.ReadFile(tree, name)
		if err != nil || string(got) != want {
			t.Errorf("%s at the first commit: got %q, %v; want %q", name, got, err, want)
		}
	}
	for _, name := range []string{"a/new.txt", "wt.txt"} {
		if _, err := fs.Stat(tree, name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s at the first commit: got %v, want it not to exist", name, err)
		}
	}

	tree, err = r.Tree(second)
	if err != nil {
		t.Fatal(err)
	}
	if err := fstest.TestFS(tree, "go.mod", "a/d.txt", "a/new.txt"); err != nil {
		t.Error(err)
	}
	if got, _ := fs.ReadFile(tree, "a/d.txt"); string(got) != "changed\n" {
		t.Errorf("a/d.txt at the second commit: got %q", got)
	}
	for _, name := range []string{"a/b", "a/b/c.txt", "wt.txt"} {
		if _, err := fs.Stat(tree, name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s at the second commit: got %v, want it not to exist", name, err)
		}
	}
}

// TestSymlink: a symbolic link is listed with its mode, and not followed.
func TestSymlink(t *testing.T) {
	t.Parallel()
	g := gittest.New(t)
	g.Write(map[string]string{"target.txt": "t\n"})
	if err := os.Symlink("target.txt", filepath.Join(g.Dir, "link.txt")); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	c := g.Commit("link")
	tree, err := open(t, g.Dir).Tree(c)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := fs.Stat(tree, "link.txt")
	if err != nil || fi.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("link.txt: got %v, %v; want a symbolic link", fi, err)
	}
	if _, err := fs.ReadFile(tree, "link.txt"); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("reading link.txt: got %v, want a refusal", err)
	}
}

// TestPrefixAndResolve: a repository opened below its top knows where it
// was opened, and a revision resolves to its commit, or fails by name.
func TestPrefixAndResolve(t *testing.T) {
	t.Parallel()
	g := gittest.New(t)
	g.Write(map[string]string{"sub/dir/f.txt": "f\n"})
	c := g.Commit("first")

	top := open(t, g.Dir)
	if got := top.Prefix(); got != "." {
		t.Errorf("Prefix at the top = %q, want .", got)
	}
	sub := open(t, filepath.Join(g.Dir, "sub", "dir"))
	if got := sub.Prefix(); got != "sub/dir" {
		t.Errorf("Prefix below the top = %q, want sub/dir", got)
	}
	for _, rev := range []string{"main", "HEAD", c, c[:10]} {
		if got, err := sub.Resolve(rev); err != nil || got != c {
			t.Errorf("Resolve(%q) = %q, %v; want %s", rev, got, err, c)
		}
	}
	for _, rev := range []string{"nope", "--all", "HEAD~5"} {
		if _, err := sub.Resolve(rev); err == nil || err.Error() != rev+" names no commit in this repository" {
			t.Errorf("Resolve(%q): got %v", rev, err)
		}
	}
	if _, err := gitfs.Open(t.TempDir(), gittest.Env()); err == nil {
		t.Error("Open outside a repository: got no error")
	}
}
