package hypercase_test

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
)

// Strip's tests run hypercase strip as a person does, from the command
// built from ./cmd/hypercase, over the fixture strip: a module of two
// packages over one corpus root. The root package owns import, parse and
// resolve, and the check cells; ./b uses them, and owns evaluate, list,
// the transform louder and the check products. Its archives hold only
// hand-written sections: an export, prices, with the hand-written variant
// doubled, which import makes a script of, and a script, plain, with the
// hand-written variant more.

var stripRegistrations = map[string]func(){
	"strip-a": func() { chain(project, exportType, own(importLift, parse, resolve, cellsCheck)) },
	"strip-b": func() { stripB(exportType, own(evaluate, list, louder, exportCheck)) },
	// louder dropped from the export type, as both packages register it.
	"strip-a-plain": func() { chain(project, plainExportType, own(importLift, parse, resolve, cellsCheck)) },
	"strip-b-plain": func() { stripB(plainExportType, own(evaluate, list, exportCheck)) },
}

func stripB(export decl.Type[Export], owned hypercase.Option) {
	hypercase.Register(append([]any{project, scriptType, export, hypercase.Use(importLift, parse, resolve), owned}, chainProps...)...)
}

// stripModule is the update over the whole fixture strip, as go test ./...
// runs it: both packages at once.
var stripModule = []childRun{{"", "strip-a", true}, {"b", "strip-b", true}}

// hypercaseCmd is the hypercase command, built once for the tests that run
// it, in a temp dir TestMain removes.
var hypercaseCmd struct {
	once      sync.Once
	dir, path string
	err       error
}

func TestMain(m *testing.M) {
	code := m.Run()
	if hypercaseCmd.dir != "" {
		os.RemoveAll(hypercaseCmd.dir)
	}
	os.Exit(code)
}

func hypercaseBinary(t *testing.T) string {
	t.Helper()
	hypercaseCmd.once.Do(func() {
		dir, err := os.MkdirTemp("", "hypercase-cmd-")
		if err != nil {
			hypercaseCmd.err = err
			return
		}
		hypercaseCmd.dir, hypercaseCmd.path = dir, filepath.Join(dir, "hypercase")
		if out, err := exec.Command("go", "build", "-o", hypercaseCmd.path, "./cmd/hypercase").CombinedOutput(); err != nil {
			hypercaseCmd.err = fmt.Errorf("go build ./cmd/hypercase: %v\n%s", err, out)
		}
	})
	if hypercaseCmd.err != nil {
		t.Fatal(hypercaseCmd.err)
	}
	return hypercaseCmd.path
}

// strip runs hypercase strip at the sandbox's module root, as a person
// does, and records it.
func (s *sandbox) strip(args ...string) result {
	s.t.Helper()
	cmd := exec.Command(hypercaseBinary(s.t), append([]string{"strip"}, args...)...)
	cmd.Dir = s.dir
	out, err := cmd.CombinedOutput()
	r := result{out: s.normalize(out)}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		r.code = exit.ExitCode()
	} else if err != nil {
		s.t.Fatalf("running hypercase strip: %v", err)
	}
	fmt.Fprintf(&s.transcript, "$ %s\n%s", strings.Join(append([]string{"hypercase", "strip"}, args...), " "), r.out)
	if r.code != 0 {
		fmt.Fprintf(&s.transcript, "[exit %d]\n", r.code)
	}
	return r
}

// settle runs the update, its packages at once, until a pass changes
// nothing, and returns that pass's results. How many passes write depends
// on how the packages' runs interleave, so only the last pass, which
// changes nothing, is recorded.
func (s *sandbox) settle(runs ...childRun) []result {
	s.t.Helper()
	for pass := 1; ; pass++ {
		before := s.snapshot()
		rs := s.execParallel(runs...)
		if maps.EqualFunc(before, s.snapshot(), bytes.Equal) {
			s.note("the update, run until a pass changes nothing; that pass:")
			s.recordAll(runs, rs)
			return rs
		}
		if pass == 5 {
			s.t.Fatalf("the update still changes files after %d passes", pass)
		}
	}
}

// stale leaves files that no update removes: a removed column's sections,
// a dropped transform's, a dropped bytes lift's, an output the run no
// longer produces, and a findings file no check writes.
func (s *sandbox) stale() {
	s.t.Helper()
	s.edit(prices, "a removed column's sections", func(a *archive.Archive) {
		set("out/nutrition/outcome", "accepted\n")(a)
		set("variant/doubled/out/nutrition/outcome", "accepted\n")(a)
	})
	s.edit(prices, "a dropped transform's sections", set("transform/gone/in/import/out/quote/A1", "3\n"))
	s.edit(prices, "a dropped bytes lift's product", set("via/gone/quote/A1", "3\n"))
	s.edit(prices, "an output list no longer produces", set("out/list/old.txt", "stale\n"))
	s.write("b/testdata/checks/old.txt", []byte("export/prices old\n"), "a findings file no check writes")
}

// staleSections are the sections stale leaves.
var staleSections = map[string]string{
	"out/nutrition/outcome":                 "accepted\n",
	"variant/doubled/out/nutrition/outcome": "accepted\n",
	"transform/gone/in/import/out/quote/A1": "3\n",
	"via/gone/quote/A1":                     "3\n",
	"out/list/old.txt":                      "stale\n",
}

// TestStripRegenerates: strip, then the module's update until it changes
// nothing, reaches what the update reaches from the archives' hand-written
// sections alone, the fixture as committed: every archive, registry and
// findings file is byte for byte the same. The stale files a person left,
// which the update alone leaves in place, are gone.
func TestStripRegenerates(t *testing.T) {
	t.Parallel()
	fresh := newSandbox(t, "strip")
	for _, r := range fresh.settle(stripModule...) {
		fresh.passes(r)
	}
	want := fresh.snapshot()

	alone := newSandbox(t, "strip")
	alone.settle(stripModule...)
	alone.stale()
	alone.settle(stripModule...)
	alone.hasSections(prices, staleSections)
	if !alone.exists("b/testdata/checks/old.txt") {
		t.Error("the update removed b/testdata/checks/old.txt")
	}

	s := newSandbox(t, "strip")
	s.note("two other copies of the fixture: one updated as it is committed, the fresh copy, and one left stale like this one and updated alone, which keeps the stale files")
	s.settle(stripModule...)
	s.stale()
	s.passes(s.strip())
	for _, r := range s.settle(stripModule...) {
		s.passes(r)
	}
	s.unchanged(want)
	s.done()
}

// TestStripFrozen: a frozen cell whose committed files differ from what
// the code now produces keeps them through strip and the update, and
// fails as frozen. With -frozen, strip removes them, and the update writes
// what the code produces, as in the fresh copy. The gaps line stays, and
// the cell it still marks passes, frozen with files that match.
func TestStripFrozen(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "strip")
	s.settle(stripModule...)
	fresh := s.snapshot()
	s.edit(prices, "a known gap freezes list's cell, whose committed files record an older behavior", func(a *archive.Archive) {
		insert("gaps", "list Lists one file too few. Waits on sheets#1.\n", "variant/")(a)
		set("out/list/files.txt", "2 files\n")(a)
	})
	const frozen = "The cell is frozen by its known gap, gaps line 1: \"list Lists one file too few. Waits on sheets#1.\""
	s.fails(s.run("b", "strip-b", false), frozen)

	r := s.strip()
	s.passes(r)
	s.contains(r, "testdata/export/prices.txtar: left the cell list, frozen by its known gap, gaps line 1; hypercase strip -frozen strips it too\n")
	s.settle(stripModule...)
	s.hasSections(prices, map[string]string{"out/list/files.txt": "2 files\n"})
	s.fails(s.run("b", "strip-b", false), frozen)

	s.passes(s.strip("-frozen"))
	for _, r := range s.settle(stripModule...) {
		s.passes(r)
	}
	s.passes(s.normal("strip-a"))
	s.passes(s.run("b", "strip-b", false))
	got := archive.Parse(s.read(prices))
	if data, _ := got.Section("gaps"); string(data) != "list Lists one file too few. Waits on sheets#1.\n" {
		t.Errorf("the gaps section is %q", data)
	}
	drop("gaps")(got)
	if !bytes.Equal(got.Format(), fresh[prices]) {
		t.Errorf("past its gaps section, %s differs from the fresh copy's:\n%s", prices, got.Format())
	}
	for p, data := range fresh {
		if p != prices {
			s.same(p, data)
		}
	}
	s.done()
}

// TestStripRootCheck: once the transform louder is dropped, the
// root-package check's message for its sections names hypercase strip
// -transform louder, then the update. Running them leaves the check
// passing.
func TestStripRootCheck(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "strip")
	s.settle(stripModule...)
	plain := []childRun{{"", "strip-a-plain", true}, {"b", "strip-b-plain", true}}
	s.note("louder is dropped from the export type")
	s.settle(plain...)
	s.fails(s.normal("strip-a-plain"), "testdata/export/prices.txtar: no registry claims transform/louder/in/import/out/evaluate/out/values.txt, ",
		": no corpus type of testdata/export/ lists a transform named louder.\nIf a transform should make the variant, list it in its corpus type's Transforms. If not, run hypercase strip -transform louder at the module root, then FIXTURE_UPDATE_GOLDEN=1 go test ./... until it changes nothing.")
	s.passes(s.strip("-transform", "louder"))
	s.settle(plain...)
	s.hasSections(prices, nil, "transform/louder/")
	s.passes(s.normal("strip-a-plain"))
	s.passes(s.run("b", "strip-b-plain", false))
	s.done()
}
