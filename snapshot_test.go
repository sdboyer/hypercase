package hypercase_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/hooks"
	"github.com/sdboyer/hypercase/root"
	"github.com/sdboyer/hypercase/snapshot"
)

// The snapshot primitives, with the runner: what Create writes is a case
// the runner reads as any other.
//
// Create and RewriteInputs refuse while the fixtures' update variable is
// set, and these tests call them in this process, so each clears the
// variable with t.Setenv, and none runs in parallel. Create writes only
// under a corpus root a registry records, so each test's first update
// writes the package's registry before it creates a case.

// moduleCorpus returns the sandbox's module-root corpus.
func (s *sandbox) moduleCorpus() root.Corpus {
	s.t.Helper()
	c, err := root.Find(s.dir, ".")
	if err != nil {
		s.t.Fatal(err)
	}
	return c
}

// create writes a case into the sandbox's module-root corpus with
// snapshot.Create, as a project's command would, and notes it.
func (s *sandbox) create(name string, trees map[string]fs.FS) string {
	s.t.Helper()
	return s.createAt(s.moduleCorpus(), name, trees)
}

// createAt writes a case under a corpus root of the sandbox with
// snapshot.Create, and notes it.
func (s *sandbox) createAt(corpus root.Corpus, name string, trees map[string]fs.FS) string {
	s.t.Helper()
	path, err := snapshot.Create(corpus, project, scriptType, name, s.serialize(trees))
	if err != nil {
		s.t.Fatal(err)
	}
	rel, _ := filepath.Rel(s.dir, path)
	rel = filepath.ToSlash(rel)
	s.note("snapshot.Create wrote %s", rel)
	s.cat(rel)
	return rel
}

func (s *sandbox) serialize(trees map[string]fs.FS) []archive.Section {
	s.t.Helper()
	ss, err := snapshot.Serialize(trees)
	if err != nil {
		s.t.Fatal(err)
	}
	return ss
}

// registers runs the first update of a registration, which writes the
// package's registry, and notes it. With no case yet, the update fails on
// each suite, which applies to no case, and writes the registry still.
func (s *sandbox) registers(registration string) {
	s.t.Helper()
	s.note("the first update writes the package's registry, which records the corpus root Create writes under, and fails, since no suite has a case yet")
	s.fails(s.update(registration), "applies to no case: its corpus, ")
	if !s.exists("testdata/hypercase.json") {
		s.t.Fatal("the first update wrote no registry")
	}
}

// TestSnapshotProperties: a case that Create wrote from a script whose
// resolve/ tree makes resolve err, as a sheet with a cycle does, states
// no property. After an update, resolve's cell fails as an unexpected
// error, and passes once a person adds a properties section stating
// circular.
func TestSnapshotProperties(t *testing.T) {
	t.Setenv(fixtureUpdate, "")
	s := newSandbox(t, "")
	s.registers("ladder")
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
	s.registers("ladder")
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
	s.registers("ladder")
	rel := s.create("six", map[string]fs.FS{"calc": fstest.MapFS{"out/values.txt": {Data: []byte("6\n")}}})
	s.edit(rel, "a person describes the case", func(a *archive.Archive) {
		a.Comment = []byte("calc gives 6.\n")
	})
	if _, err := snapshot.Load(scriptType, s.path(rel)); err != nil {
		t.Fatal(err)
	}
	s.note("snapshot.Load read %s", rel)
	s.passes(s.update("ladder"))
	err := snapshot.RewriteInputs(s.moduleCorpus(), project, s.path(rel), "", []archive.Section{{Name: "in/calc/out/values.txt", Data: []byte("7\n")}})
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
// root.Find, from its own directory, and Create under that root writes
// where a run reads: the module root, for a package that passes no
// hypercase.Root, and corp under it, for one that passes
// hypercase.Root("corp"). Create under the other root, which the
// package's registry doesn't record, is refused, and writes nothing.
func TestSnapshotCorpusRoot(t *testing.T) {
	t.Setenv(fixtureUpdate, "")
	for _, c := range []struct {
		name, rel, other, registration, column string
		trees                                  map[string]fs.FS
		out                                    map[string]string
	}{
		{"module", ".", "corp", "ladder", "calc", map[string]fs.FS{"calc": fstest.MapFS{"out/values.txt": {Data: []byte("6\n")}}}, map[string]string{"out/calc/values.txt": "6\n"}},
		{"corp", "corp", ".", "corp-a", "echo", map[string]fs.FS{"do": fstest.MapFS{"out/values.txt": {Data: []byte("A1 6\n")}}}, map[string]string{"out/echo/values.txt": "A1 6\n"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := newSandbox(t, "")
			cmd := s.path("cmd/snapshot")
			for _, d := range []string{cmd, s.path(c.rel), s.path(c.other)} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			s.registers(c.registration)
			other, err := root.Find(cmd, c.other)
			if err != nil {
				t.Fatal(err)
			}
			before := s.snapshot()
			_, err = snapshot.Create(other, project, scriptType, "six", s.serialize(c.trees))
			if err == nil {
				t.Fatalf("Create wrote under %s, which no registry records", c.other)
			}
			s.note("snapshot.Create under the corpus root %s, which the registry doesn't record, is refused:\n%s", c.other, strings.TrimSuffix(s.normalize([]byte(err.Error())), "\n"))
			s.unchanged(before)
			corpus, err := root.Find(cmd, c.rel)
			if err != nil {
				t.Fatal(err)
			}
			s.note("root.Find, from cmd/snapshot, found the corpus root %s", filepath.ToSlash(mustRel(t, s.dir, corpus.Dir())))
			rel := s.createAt(corpus, "six", c.trees)
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

// TestSnapshotFailedCreate: a Create that fails once its temp file is
// written keeps the directories it made, corp/testdata/ and
// corp/testdata/script/, and a normal run reads the empty corpus
// directory as it read the missing one: its output is the same.
func TestSnapshotFailedCreate(t *testing.T) {
	t.Setenv(fixtureUpdate, "")
	s := newSandbox(t, "")
	cmd := s.path("cmd/snapshot")
	for _, d := range []string{cmd, s.path("corp")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s.registers("corp-a")
	if s.exists("corp/testdata") {
		t.Fatal("corp/testdata/ exists before any Create")
	}
	s.note("a normal run, with no corp/testdata/")
	missing := s.normal("corp-a")
	corpus, err := root.Find(cmd, "corp")
	if err != nil {
		t.Fatal(err)
	}
	hooks.AfterTemp = func(string) error { return errors.New("the disk is full") }
	_, err = snapshot.Create(corpus, project, scriptType, "six", s.serialize(map[string]fs.FS{"do": fstest.MapFS{"out/values.txt": {Data: []byte("A1 6\n")}}}))
	hooks.AfterTemp = nil
	if err == nil {
		t.Fatal("Create succeeded, and its write should fail")
	}
	s.note("snapshot.Create fails:\n%s", strings.TrimSuffix(s.normalize([]byte(err.Error())), "\n"))
	entries, err := os.ReadDir(s.path("corp/testdata/script"))
	if err != nil || len(entries) > 0 {
		t.Fatalf("the failed Create left corp/testdata/script/ holding %v (%v), not empty", entries, err)
	}
	s.note("the failed Create keeps corp/testdata/script/, empty; a normal run")
	empty := s.normal("corp-a")
	if empty != missing {
		t.Errorf("a run reads an empty corpus directory otherwise than a missing one:\n%s\nnot:\n%s", empty.out, missing.out)
	}
	s.done()
}

// TestSnapshotUntestedRegistry: a package, b, that registers a corpus type
// and a lift over it, and no suite, passes its first update with no case
// yet, and the update writes its registry, so Create then writes a case
// into the type's directory, testdata/grid/. Once b holds no _test.go
// file, Create refuses that directory, naming b's registry, and writes
// nothing. The root-package check still counts the directory as declared:
// it fails on b's registry, and not on the archive in testdata/grid/, as
// it does once b's registry is gone.
func TestSnapshotUntestedRegistry(t *testing.T) {
	t.Setenv(fixtureUpdate, "")
	s := newSandbox(t, "")
	s.registers("ladder")
	s.write("b/corpus_test.go", []byte(packageTestFile), "give b its tests")
	s.passes(s.run("b", "grid-lift", true))
	if !s.exists("b/testdata/hypercase.json") {
		t.Fatal("b's first update wrote no registry")
	}
	trees := map[string]fs.FS{"cells": fstest.MapFS{"A1": {Data: []byte("6\n")}}}
	path, err := snapshot.Create(s.moduleCorpus(), project, gridType, "six", s.serialize(trees))
	if err != nil {
		t.Fatal(err)
	}
	rel, _ := filepath.Rel(s.dir, path)
	rel = filepath.ToSlash(rel)
	s.note("snapshot.Create wrote %s", rel)
	s.edit(rel, "a person describes the case", func(a *archive.Archive) {
		a.Comment = []byte("A grid of one cell.\n")
	})
	s.remove("b/corpus_test.go", "remove b's tests, and leave its registry")
	before := s.snapshot()
	_, err = snapshot.Create(s.moduleCorpus(), project, gridType, "seven", s.serialize(trees))
	if err == nil {
		t.Fatal("Create wrote into testdata/grid/, which only a registry in a package with no _test.go file declares")
	}
	s.note("snapshot.Create into testdata/grid/ is refused:\n%s", strings.TrimSuffix(s.normalize([]byte(err.Error())), "\n"))
	s.unchanged(before)
	undeclared := "testdata/grid/ holds archives, and no registry declares the directory"
	r := s.normal("ladder")
	s.fails(r, "b/testdata/hypercase.json is a registry, and b holds no _test.go file")
	s.lacks(r, undeclared)
	s.remove("b/testdata/hypercase.json", "delete b's registry")
	s.fails(s.normal("ladder"), undeclared)
	s.done()
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
