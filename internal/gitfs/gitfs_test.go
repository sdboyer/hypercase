package gitfs_test

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	g.Write(map[string]string{"sub/dir/f.txt": "g\n"})
	d := g.Commit("second")

	top := open(t, g.Dir)
	if got := top.Prefix(); got != "." {
		t.Errorf("Prefix at the top = %q, want .", got)
	}
	sub := open(t, filepath.Join(g.Dir, "sub", "dir"))
	if got := sub.Prefix(); got != "sub/dir" {
		t.Errorf("Prefix below the top = %q, want sub/dir", got)
	}
	for _, rev := range []string{"main~1", "HEAD~", c, c[:10]} {
		if got, err := sub.Resolve(rev); err != nil || got != c {
			t.Errorf("Resolve(%q) = %q, %v; want %s", rev, got, err, c)
		}
	}
	// A range is no commit, though rev-parse prints the commits it names
	// before it exits 1.
	if out := g.Git("rev-parse", c+".."+d); !strings.Contains(out, c) {
		t.Fatalf("git rev-parse %s..%s printed %q, and this test wants it to print the range's commits", c, d, out)
	}
	for _, rev := range []string{"nope", "--all", "-x", "HEAD~5", c + ".." + d, c + "..." + d, "main..HEAD"} {
		if _, err := sub.Resolve(rev); err == nil || err.Error() != rev+" names no commit in this repository" {
			t.Errorf("Resolve(%q): got %v", rev, err)
		}
	}
	if _, err := gitfs.Open(t.TempDir(), gittest.Env()); err == nil {
		t.Error("Open outside a repository: got no error")
	}
}

// TestMissingBlob: a blob the object store has lost is an error that says
// so, never an absent or empty file.
func TestMissingBlob(t *testing.T) {
	t.Parallel()
	g := gittest.New(t)
	g.Write(map[string]string{"kept.txt": "kept\n", "lost.txt": "lost\n"})
	c := g.Commit("first")
	hash := strings.TrimSpace(g.Git("rev-parse", c+":lost.txt"))
	if err := os.Remove(filepath.Join(g.Dir, ".git", "objects", hash[:2], hash[2:])); err != nil {
		t.Fatal(err)
	}
	tree, err := open(t, g.Dir).Tree(c)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := fs.ReadFile(tree, "kept.txt"); err != nil || string(got) != "kept\n" {
		t.Errorf("kept.txt: got %q, %v", got, err)
	}
	want := "readfile lost.txt: git cat-file finds no object " + hash + " in the repository"
	if _, err := fs.ReadFile(tree, "lost.txt"); err == nil || err.Error() != want {
		t.Errorf("lost.txt: got %v, want %q", err, want)
	}
	if _, err := fs.Stat(tree, "lost.txt"); err == nil || !strings.Contains(err.Error(), "finds no object "+hash) {
		t.Errorf("stat lost.txt: got %v", err)
	}
}

// wrapGit puts a git on PATH that runs script, a shell script given the
// real git as $GIT, before git's arguments.
func wrapGit(t *testing.T, script string) {
	if runtime.GOOS == "windows" {
		t.Skip("the wrapper is a shell script")
	}
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	body := "#!/bin/sh\nGIT='" + real + "'\n" + script
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestStderr: git's stderr never reaches a file's content, and a git
// command that fails is an error that names the command and holds its
// stderr. It sets PATH, so it can't run in parallel.
func TestStderr(t *testing.T) {
	g := gittest.New(t)
	files := map[string]string{"a.txt": "a\n", "dir/b.txt": "b\n", "empty.txt": ""}
	g.Write(files)
	c := g.Commit("first")

	t.Run("noise", func(t *testing.T) {
		wrapGit(t, `echo "noise on stderr from git $1" >&2
"$GIT" "$@"
status=$?
echo "more noise from git $1" >&2
exit $status
`)
		r := open(t, g.Dir)
		got, err := r.Resolve("HEAD")
		if err != nil || got != c {
			t.Fatalf("Resolve(HEAD) = %q, %v", got, err)
		}
		tree, err := r.Tree(c)
		if err != nil {
			t.Fatal(err)
		}
		for name, want := range files {
			if got, err := fs.ReadFile(tree, name); err != nil || string(got) != want {
				t.Errorf("%s: got %q, %v; want %q", name, got, err, want)
			}
		}
	})
	for _, cmd := range []string{"rev-parse", "ls-tree", "cat-file"} {
		t.Run(cmd, func(t *testing.T) {
			wrapGit(t, `if [ "$1" = `+cmd+` ]; then echo "the wrapper fails $1" >&2; exit 3; fi
exec "$GIT" "$@"
`)
			want := "git " + cmd + ": the wrapper fails " + cmd
			r, err := gitfs.Open(g.Dir, gittest.Env())
			if cmd == "rev-parse" {
				if err == nil || err.Error() != want {
					t.Errorf("Open: got %v, want %q", err, want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			tree, err := r.Tree(c)
			if cmd == "ls-tree" {
				if err == nil || err.Error() != want {
					t.Errorf("Tree: got %v, want %q", err, want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for range 2 { // the failure holds for every read after it
				if _, err := fs.ReadFile(tree, "a.txt"); err == nil || err.Error() != "readfile a.txt: "+want {
					t.Errorf("ReadFile: got %v, want %q", err, want)
				}
			}
		})
	}
	// A revision git resolves to no commit writes nothing on stderr, and a
	// rev-parse that fails otherwise is the failure, with its stderr.
	t.Run("resolve", func(t *testing.T) {
		wrapGit(t, `if [ "$1" = rev-parse ] && [ "$2" = --verify ]; then echo "the wrapper fails $1" >&2; exit 3; fi
exec "$GIT" "$@"
`)
		r := open(t, g.Dir)
		if _, err := r.Resolve("HEAD"); err == nil || err.Error() != "git rev-parse: the wrapper fails rev-parse" {
			t.Errorf("Resolve(HEAD): got %v, want %q", err, "git rev-parse: the wrapper fails rev-parse")
		}
	})
}
