package hypercase_test

import (
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/snapshot"
)

// The snapshot primitives, with the runner: what Create writes is a case
// the runner reads as any other.

// create writes a case into the sandbox with snapshot.Create, as a
// project's command would, and notes it.
func (s *sandbox) create(name string, trees map[string]fs.FS) string {
	s.t.Helper()
	ss, err := snapshot.Serialize(trees)
	if err != nil {
		s.t.Fatal(err)
	}
	path, err := snapshot.Create(s.dir, scriptType, name, ss)
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
