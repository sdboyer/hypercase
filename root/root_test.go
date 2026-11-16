package root

import (
	"os"
	"path/filepath"
	"testing"
)

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
		got, err := Module(tt.from)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := filepath.EvalSymlinks(got); got != mustEval(t, tt.want) {
			t.Errorf("Module(%s) = %s, want %s", tt.from, got, tt.want)
		}
	}
	if got := Rel(dir, filepath.Join(deep, "x.txtar")); got != "a/b/c/x.txtar" {
		t.Errorf("Rel = %s", got)
	}
}

// TestCorpus: the corpus root is the module root found from the
// directory, with the package's corpus root, relative to it, under it. A
// path that isn't a slash path relative to the module root is refused.
func TestCorpus(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{".": dir, "corp": filepath.Join(dir, "corp"), "a/corp": filepath.Join(dir, "a", "corp")} {
		got, err := Corpus(deep, rel)
		if err != nil {
			t.Fatal(err)
		}
		if mustEval(t, filepath.Dir(got)) != mustEval(t, filepath.Dir(want)) || filepath.Base(got) != filepath.Base(want) {
			t.Errorf("Corpus(%s, %q) = %s, want %s", deep, rel, got, want)
		}
	}
	for _, rel := range []string{"/corp", "../corp", "corp/", ""} {
		if _, err := Corpus(deep, rel); err == nil {
			t.Errorf("Corpus accepted the corpus root %q", rel)
		}
	}
	if _, err := Corpus(t.TempDir(), "."); err == nil {
		t.Error("Corpus found a corpus root outside any module")
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
