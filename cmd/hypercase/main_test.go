package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/diff"
	"github.com/sdboyer/hypercase/internal/gittest"
	"github.com/sdboyer/hypercase/internal/registry"
)

// revisions commits each set of files in turn, as the whole working tree,
// and returns the commits.
func revisions(t *testing.T, g *gittest.Repo, revs ...map[string]string) []string {
	t.Helper()
	var commits []string
	for i, files := range revs {
		entries, err := os.ReadDir(g.Dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.Name() != ".git" {
				g.Remove(e.Name())
			}
		}
		g.Write(files)
		commits = append(commits, g.Commit(fmt.Sprint("revision ", i)))
	}
	return commits
}

// module returns a module whose corpus root is its root, with one
// registry, over the corpus sheet, and the archives given by name.
func module(archives map[string]string) map[string]string {
	files := map[string]string{
		"go.mod":                  "module example.com/m\n\ngo 1.27\n",
		"testdata/hypercase.json": string(moduleRegistry().Marshal()),
	}
	for name, data := range archives {
		files["testdata/sheet/"+name+".txtar"] = data
	}
	return files
}

func moduleRegistry() registry.Registry {
	return registry.Registry{
		UpdateVar: "M_UPDATE_GOLDEN", CorpusRoot: ".", RootPackage: true,
		IssueLinks: []registry.IssueLink{{Pattern: `sheets#(\d+)`, URL: sheetsLink}},
		Properties: []registry.Property{{Name: "circular", Description: "A loop."}},
		Suites: []registry.Suite{
			{Name: "calc", Description: "Runs calc.", Corpus: "sheet"},
			{Name: "evaluate", Description: "Evaluates.", Corpus: "sheet"},
		},
	}
}

// goldenVar rewrites the goldens, as it does for the module's other tests.
const goldenVar = "HYPERCASE_UPDATE_GOLDEN"

type result struct {
	stdout, stderr string
	code           int
}

// hypercase runs the command in a directory of the repository, relative
// to its top.
func hypercase(t *testing.T, g *gittest.Repo, dir string, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(filepath.Join(g.Dir, filepath.FromSlash(dir)), gittest.Env(), args, &out, &errb)
	return result{stdout: out.String(), stderr: errb.String(), code: code}
}

// ok runs the command and fails the test unless it succeeds.
func ok(t *testing.T, g *gittest.Repo, dir string, args ...string) string {
	t.Helper()
	r := hypercase(t, g, dir, args...)
	if r.code != 0 {
		t.Fatalf("hypercase %s exited %d:\n%s", strings.Join(args, " "), r.code, r.stderr)
	}
	if r.stderr != "" {
		t.Errorf("hypercase %s wrote to stderr:\n%s", strings.Join(args, " "), r.stderr)
	}
	return r.stdout
}

// fails runs the command and fails the test unless it exits with code,
// saying each of want on stderr, and writing nothing on stdout.
func fails(t *testing.T, g *gittest.Repo, dir string, code int, args []string, want ...string) {
	t.Helper()
	r := hypercase(t, g, dir, args...)
	if r.code != code {
		t.Errorf("hypercase %s exited %d, want %d:\n%s", strings.Join(args, " "), r.code, code, r.stderr)
	}
	if r.stdout != "" {
		t.Errorf("hypercase %s wrote to stdout:\n%s", strings.Join(args, " "), r.stdout)
	}
	for _, w := range want {
		if !strings.Contains(r.stderr, w) {
			t.Errorf("hypercase %s: stderr doesn't hold %q:\n%s", strings.Join(args, " "), w, r.stderr)
		}
	}
}

func golden(t *testing.T, name string, got string) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if os.Getenv(goldenVar) == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%v\nRun %s=1 go test ./... to write it, and read it.", err, goldenVar)
	}
	if string(want) != got {
		t.Errorf("%s differs (-golden +got):\n%s\nIf the change is intended, run %s=1 go test ./... and read the diff.", p, diff.Lines(want, []byte(got)), goldenVar)
	}
}

// TestText: the plain-text report of the sheets fixture, whose change is
// the walkthrough's, with one change of each rank.
func TestText(t *testing.T) {
	t.Parallel()
	g, a, b := sheets(t)
	golden(t, "sheets.txt", ok(t, g, "step09", "report", "-text", a, b))
}
