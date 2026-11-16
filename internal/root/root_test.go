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

func mustEval(t *testing.T, p string) string {
	t.Helper()
	e, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
