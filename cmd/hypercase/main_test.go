package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/diff"
	"github.com/sdboyer/hypercase/internal/gittest"
	"github.com/sdboyer/hypercase/internal/registry"
	"github.com/sdboyer/hypercase/internal/revmap"
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
// registry, over the corpus sheet, and the archives given by name, each
// with the inputs files of mapped.
func module(archives map[string]string) map[string]string {
	files := map[string]string{
		"go.mod":                  "module example.com/m\n\ngo 1.27\n",
		"testdata/hypercase.json": string(moduleRegistry().Marshal()),
	}
	for name, data := range archives {
		files["testdata/sheet/"+name+".txtar"] = mapped(data)
	}
	return files
}

// mapped returns an archive with an inputs file for each cell that ran and
// has none, as the update writes one for a cell whose output declarations
// give no ranges: each of the cell's files, its product's named
// via/<lift>/<path>, by its path line alone. So a fixture written before
// inputs files, at a registry format that has them, holds what an update
// would commit, and the report finds no cell that ran without one. Each
// inputs file is added after the archive's last section, and an archive
// that doesn't end with a newline is returned as it is.
func mapped(data string) string {
	if !strings.HasSuffix(data, "\n") {
		return data
	}
	type cell struct {
		ran, has bool
		files    []revmap.File
	}
	cells := map[string]*cell{} // by the cell's directory, <prefix>out/<column>/
	var order []string
	cellOf := func(dir string) *cell {
		if cells[dir] == nil {
			cells[dir] = &cell{}
			order = append(order, dir)
		}
		return cells[dir]
	}
	for _, s := range archive.Parse([]byte(data)).Sections {
		kind, name, rest := archive.SplitCase(s.Name)
		prefix := ""
		switch {
		case kind != "" && name == "":
			continue // a section no case holds
		case kind != "":
			prefix = kind + "/" + name + "/"
		}
		top, below, _ := strings.Cut(rest, "/")
		col, file, _ := strings.Cut(below, "/")
		if col == "" || file == "" {
			continue
		}
		switch top {
		case archive.Out:
			c := cellOf(prefix + archive.Out + "/" + col + "/")
			switch file {
			case archive.OutcomeFile:
				o, err := archive.ParseOutcome(s.Data)
				c.ran = err == nil && o.State != archive.Skipped && o.State != archive.Blocked
			case revmap.Name:
				c.has = true
			default:
				c.files = append(c.files, revmap.File{Path: file})
			}
		case archive.Via:
			c := cellOf(prefix + archive.Out + "/" + col + "/")
			c.files = append(c.files, revmap.File{Path: rest})
		}
	}
	for _, dir := range order {
		if c := cells[dir]; c.ran && !c.has {
			data += "-- " + dir + revmap.Name + " --\n" + string(revmap.Format(c.files))
		}
	}
	return data
}

func moduleRegistry() registry.Registry {
	return registry.Registry{
		UpdateVar: "M_UPDATE_GOLDEN", CorpusRoot: ".", RootPackage: true,
		IssueLinks: []registry.IssueLink{{Pattern: `sheets#(\d+)`, URL: sheetsLink}},
		Properties: []registry.Property{{Name: "circular", Description: "A loop."}},
		Suites: []registry.Suite{
			{Name: "calc", Description: "Runs calc.", Consumes: registry.RawInput, Corpora: []string{"sheet"}, Paths: []registry.Path{{Corpus: "sheet"}}},
			{Name: "evaluate", Description: "Evaluates.", Consumes: registry.RawInput, Corpora: []string{"sheet"}, Paths: []registry.Path{{Corpus: "sheet"}}},
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
