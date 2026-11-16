package root

import (
	"os"
	"path/filepath"
	"testing"
)

// TestModule: the module root is the nearest directory, from the one
// given upward, that holds a go.mod, a nested module's included.
func TestModule(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c")
	nested := filepath.Join(dir, "a", "nested", "d")
	for _, d := range []string{deep, nested} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range []string{dir, filepath.Join(dir, "a", "nested")} {
		if err := os.WriteFile(filepath.Join(m, "go.mod"), []byte("module x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range []struct{ from, want string }{
		{deep, dir},
		{dir, dir},
		{nested, filepath.Join(dir, "a", "nested")},
	} {
		got, err := module(tt.from)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := filepath.EvalSymlinks(got); got != mustEval(t, tt.want) {
			t.Errorf("module(%s) = %s, want %s", tt.from, got, tt.want)
		}
	}
}

// TestFind: the corpus root is the module root found from the directory,
// with the package's corpus root, relative to it, under it: Find returns
// both, and the path as passed. A path that isn't a slash path relative
// to the module root is refused, and so is a directory in no module.
func TestFind(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{".": dir, "corp": filepath.Join(dir, "corp"), "a/corp": filepath.Join(dir, "a", "corp")} {
		c, err := Find(deep, rel)
		if err != nil {
			t.Fatal(err)
		}
		if got := c.Dir(); mustEval(t, filepath.Dir(got)) != mustEval(t, filepath.Dir(want)) || filepath.Base(got) != filepath.Base(want) {
			t.Errorf("Find(%s, %q).Dir() = %s, want %s", deep, rel, got, want)
		}
		if got := c.Module(); mustEval(t, got) != mustEval(t, dir) {
			t.Errorf("Find(%s, %q).Module() = %s, want %s", deep, rel, got, dir)
		}
		if got := c.Rel(); got != rel {
			t.Errorf("Find(%s, %q).Rel() = %q", deep, rel, got)
		}
	}
	for _, rel := range []string{"/corp", "../corp", "corp/", "./corp", ""} {
		if _, err := Find(deep, rel); err == nil {
			t.Errorf("Find accepted the corpus root %q", rel)
		}
	}
	if _, err := Find(t.TempDir(), "."); err == nil {
		t.Error("Find found a corpus root outside any module")
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	e, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
