package hypercase_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/root"
	"github.com/sdboyer/hypercase/snapshot"
)

// The snapshot primitives, with the runner: what Create writes is a case
// the runner reads as any other.
//
// Create and RewriteInputs refuse while the fixtures' update variable is
// set, and these tests call them in this process, so each clears the
// variable with t.Setenv, and none runs in parallel.

// create writes a case into the sandbox's module-root corpus with
// snapshot.Create, as a project's command would, and notes it.
func (s *sandbox) create(name string, trees map[string]fs.FS) string {
	s.t.Helper()
	return s.createAt(s.dir, name, trees)
}

// createAt writes a case under the corpus root dir, an absolute path in
// the sandbox, with snapshot.Create, and notes it.
func (s *sandbox) createAt(dir, name string, trees map[string]fs.FS) string {
	s.t.Helper()
	ss, err := snapshot.Serialize(trees)
	if err != nil {
		s.t.Fatal(err)
	}
	path, err := snapshot.Create(dir, scriptType, name, fixtureUpdate, ss)
	if err != nil {
		s.t.Fatal(err)
	}
	rel, _ := filepath.Rel(s.dir, path)
	rel = filepath.ToSlash(rel)
	s.note("snapshot.Create wrote %s", rel)
	s.cat(rel)
	return rel
}

// TestSnapshotProperties: a case that Create wrote from a script whose
// resolve/ tree makes resolve err, as a sheet with a cycle does, states
// no property. After an update, resolve's cell fails as an unexpected
// error, and passes once a person adds a properties section stating
// circular.
func TestSnapshotProperties(t *testing.T) {
	t.Setenv(fixtureUpdate, "")
	s := newSandbox(t, "")
	rel := s.create("cycle", map[string]fs.FS{"resolve": fstest.MapFS{"error": {Data: []byte("A1 -> B1 -> A1: cycle\n")}}})
	unexpected := "resolve returned an error, but the case states no property resolve rejects:"
	s.fails(s.update("ladder"), unexpected)
	s.cat(rel)
	s.hasSections(rel, map[string]string{"out/resolve/error.txt": "A1 -> B1 -> A1: cycle\n"})
	s.edit(rel, "a person describes the case", func(a *archive.Archive) {
		a.Comment = []byte("resolve meets a cycle.\n")
	})
	r := s.normal("ladder")
	s.fails(r, unexpected, "--- FAIL: TestCorpus/script/cycle/resolve")
	s.edit(rel, "a person states circular", insert("properties", "circular\n", "out/"))
	r = s.normal("ladder")
	s.passes(r)
	s.contains(r, "--- PASS: TestCorpus/script/cycle/resolve")
	s.done()
}

// TestSnapshotDescription: a case that Create wrote has an empty comment.
// An update writes its outputs, and the root-package check fails on it,
// naming the missing description.
func TestSnapshotDescription(t *testing.T) {
	t.Setenv(fixtureUpdate, "")
	s := newSandbox(t, "")
	rel := s.create("six", map[string]fs.FS{"calc": fstest.MapFS{"out/values.txt": {Data: []byte("6\n")}}})
	s.passes(s.update("ladder"))
	s.cat(rel)
	s.hasSections(rel, map[string]string{"out/calc/values.txt": "6\n"})
	s.fails(s.normal("ladder"), rel+": the archive has no description: its comment, the text before the first section, is empty\nWrite the case's description, in prose, as the archive's comment, the text before its first section.")
	s.done()
}

// TestSnapshotRewriteAfterUpdate: a command loads a case that Create
// wrote, an update in another process writes the case's outputs, then the
// command's RewriteInputs runs: the update's sections survive beside the
// replaced input, and the next normal run compares the output against it.
func TestSnapshotRewriteAfterUpdate(t *testing.T) {
	t.Setenv(fixtureUpdate, "")
	s := newSandbox(t, "")
	rel := s.create("six", map[string]fs.FS{"calc": fstest.MapFS{"out/values.txt": {Data: []byte("6\n")}}})
	s.edit(rel, "a person describes the case", func(a *archive.Archive) {
		a.Comment = []byte("calc gives 6.\n")
	})
	if _, err := snapshot.Load(scriptType, s.path(rel)); err != nil {
		t.Fatal(err)
	}
	s.note("snapshot.Load read %s", rel)
	s.passes(s.update("ladder"))
	err := snapshot.RewriteInputs(s.dir, s.path(rel), "", fixtureUpdate, []archive.Section{{Name: "in/calc/out/values.txt", Data: []byte("7\n")}})
	if err != nil {
		t.Fatal(err)
	}
	s.note("snapshot.RewriteInputs replaced in/calc/out/values.txt in %s", rel)
	s.cat(rel)
	s.hasSections(rel, map[string]string{"in/calc/out/values.txt": "7\n", "out/calc/values.txt": "6\n"})
	s.fails(s.normal("ladder"), "out/calc/values.txt differs")
	s.done()
}

// TestSnapshotCorpusRoot: a command finds the corpus root with
// root.Corpus, from its own directory, and Create under that root writes
// where a run reads: the module root, for a package that passes no
// hypercase.Root, and corp under it, for one that passes
// hypercase.Root("corp").
func TestSnapshotCorpusRoot(t *testing.T) {
	t.Setenv(fixtureUpdate, "")
	for _, c := range []struct {
		name, rel, registration, column string
		trees                           map[string]fs.FS
		out                             map[string]string
	}{
		{"module", ".", "ladder", "calc", map[string]fs.FS{"calc": fstest.MapFS{"out/values.txt": {Data: []byte("6\n")}}}, map[string]string{"out/calc/values.txt": "6\n"}},
		{"corp", "corp", "corp-a", "echo", map[string]fs.FS{"do": fstest.MapFS{"out/values.txt": {Data: []byte("A1 6\n")}}}, map[string]string{"out/echo/values.txt": "A1 6\n"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := newSandbox(t, "")
			cmd := s.path("cmd/snapshot")
			for _, d := range []string{cmd, s.path(c.rel)} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			dir, err := root.Corpus(cmd, c.rel)
			if err != nil {
				t.Fatal(err)
			}
			s.note("root.Corpus, from cmd/snapshot, found the corpus root %s", filepath.ToSlash(mustRel(t, s.dir, dir)))
			rel := s.createAt(dir, "six", c.trees)
			s.edit(rel, "a person describes the case", func(a *archive.Archive) {
				a.Comment = []byte("The script gives 6.\n")
			})
			u := s.update(c.registration)
			s.passes(u)
			s.contains(u, "--- PASS: TestCorpus/script/six/"+c.column)
			s.cat(rel)
			s.hasSections(rel, c.out)
			s.passes(s.normal(c.registration))
			s.done()
		})
	}
}

// mustRel returns target relative to base, which holds it, failing the
// test if it doesn't.
func mustRel(t *testing.T, base, target string) string {
	t.Helper()
	b, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	tg, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	r, err := filepath.Rel(b, tg)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
