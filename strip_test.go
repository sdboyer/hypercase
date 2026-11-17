package hypercase_test

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/registry"
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
	s.contains(r, "testdata/export/prices.txtar: left the cell list, frozen by its known gap, gaps line 1; hypercase strip -column list -frozen strips it, and list's other frozen cells\n")
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
	r := s.normal("strip-a-plain")
	s.fails(r, "testdata/export/prices.txtar: no registry claims transform/louder/in/import/out/evaluate/out/values.txt, ",
		": no corpus type of testdata/export/ lists a transform named louder.\nIf a transform should make the variant, list it in its corpus type's Transforms. If not, run hypercase strip -root . -transform louder at the module root, then FIXTURE_UPDATE_GOLDEN=1 go test ./... until it changes nothing.")
	s.passes(s.strip(s.printedStrip(r, "-transform", "louder")...))
	s.settle(plain...)
	s.hasSections(prices, nil, "transform/louder/")
	s.passes(s.normal("strip-a-plain"))
	s.passes(s.run("b", "strip-b-plain", false))
	s.done()
}

// TestStripRootCheckVia: via/ sections no bytes lift claims, as in
// TestChainRootCheck: via/nobody/…, of no column, and via/parse/…, of
// parse, a lift whose product is a stage, whose cell a gaps line marks.
// The root-package check's messages name hypercase strip -column nobody
// and -column parse, then the update. Running them leaves the check
// passing: via/parse/… is part of no cell, so strip removes it, and
// leaves parse's frozen out/parse/….
func TestStripRootCheckVia(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "strip")
	s.settle(stripModule...)
	s.edit(prices, "products no bytes lift makes, and a known gap in parse's cell", func(a *archive.Archive) {
		insert("gaps", "parse Parses every quote. Waits on sheets#2.\n", "via/")(a)
		insert("via/nobody/quote/A1", "3\n", "out/")(a)
		insert("via/parse/quote/A1", "3\n", "out/")(a)
		insert("variant/doubled/via/nobody/quote/A1", "3\n", "variant/doubled/out/")(a)
	})
	const update = " at the module root, then FIXTURE_UPDATE_GOLDEN=1 go test ./... until it changes nothing. The strip removes "
	r := s.normal("strip-a")
	s.fails(r,
		"testdata/export/prices.txtar: no registry claims via/nobody/quote/A1 and variant/doubled/via/nobody/quote/A1: no bytes lift named nobody runs over the corpus export.\nIf a bytes lift should write them, register it. If not, run hypercase strip -root . -column nobody"+update+"nobody's sections",
		"testdata/export/prices.txtar: no registry claims via/parse/quote/A1: no bytes lift named parse runs over the corpus export.\nIf a bytes lift should write them, register it. If not, run hypercase strip -root . -column parse"+update+"parse's sections")
	parse := s.printedStrip(r, "-column", "parse")
	s.passes(s.strip(s.printedStrip(r, "-column", "nobody")...))
	r = s.strip(parse...)
	s.passes(r)
	s.contains(r, "testdata/export/prices.txtar: left the cell parse, frozen by its known gap, gaps line 1; hypercase strip -column parse -frozen strips it, and parse's other frozen cells\n")
	for _, r := range s.settle(stripModule...) {
		s.passes(r)
	}
	s.hasSections(prices, nil, "via/nobody/", "via/parse/", "variant/doubled/via/nobody/")
	if _, ok := archive.Parse(s.read(prices)).Section("out/parse/outcome"); !ok {
		t.Errorf("strip removed parse's frozen cell:\n%s", s.read(prices))
	}
	s.passes(s.normal("strip-a"))
	s.passes(s.run("b", "strip-b", false))
	s.done()
}

// viaDisputed sets up TestStripRootCheckViaBytesDisagree and
// TestStripRootCheckViaUntested: a via/parse/… in parse's cell, which a
// gaps line marks, as in TestStripRootCheckVia, and a registry at reg, a
// copy of ./b's, that records parse, which ./b uses, as a bytes lift,
// where the root package, its owner, records none. One registry that says
// parse is a bytes lift is enough for strip to read the via/ as part of
// the frozen cell, and leave it.
func viaDisputed(t *testing.T, reg string) *sandbox {
	s := newSandbox(t, "strip")
	s.settle(stripModule...)
	const b = "b/testdata/hypercase.json"
	g, err := registry.Parse(s.read(b))
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(g.Lifts, func(l registry.Lift) bool { return l.Name == "parse" })
	if i < 0 || g.Lifts[i].Owned || g.Lifts[i].Bytes {
		t.Fatalf("%s records no parse that b uses and that is no bytes lift:\n%s", b, s.read(b))
	}
	g.Lifts[i].Bytes = true
	s.write(reg, g.Marshal(), "a copy of b's registry, recording parse as a bytes lift")
	s.edit(prices, "a product parse doesn't make, in parse's cell, which a gaps line marks", func(a *archive.Archive) {
		insert("gaps", viaGap, "via/")(a)
		insert("via/parse/quote/A1", "3\n", "out/")(a)
	})
	return s
}

const (
	// viaGap is the gaps line that marks parse's cell.
	viaGap = "parse Parses every quote. Waits on sheets#2.\n"
	// viaLeft is strip's line for parse's frozen cell, which it leaves,
	// with its via/ while a registry says parse is a bytes lift.
	viaLeft = "testdata/export/prices.txtar: left the cell parse, frozen by its known gap, gaps line 1; hypercase strip -column parse -frozen strips it, and parse's other frozen cells\n"
)

// TestStripRootCheckViaBytesDisagree: ./b's registry records parse as a
// bytes lift (viaDisputed). The root-package check fails on the lift's
// entries, which differ, and says to run the update. Strip, run then,
// leaves via/parse/…, so the message for it names no strip: it says to
// settle the registries first. The update does, and then the message says
// to strip, and no gaps line keeps strip from the section, since parse is
// no bytes lift. Strip removes it and leaves parse's frozen cell, gaps
// line and all, and the update then leaves the check passing.
func TestStripRootCheckViaBytesDisagree(t *testing.T) {
	t.Parallel()
	s := viaDisputed(t, "b/testdata/hypercase.json")
	const claims = "testdata/export/prices.txtar: no registry claims via/parse/quote/A1: no bytes lift named parse runs over the corpus export.\nIf a bytes lift should write them, register it. If not, "
	r := s.normal("strip-a")
	s.fails(r, "the registries that own or use the lift parse differ in bytes:\n  bytes:\n    .: false\n    ./b: true\n",
		claims+"first settle whether parse is a bytes lift, on which the registries differ, as the check's failures on them say, then run the check again: until they agree, it can't tell whether gaps line 1, which marks a cell of theirs, keeps strip from removing them.")
	s.lacks(r, "remove gaps line")
	r = s.strip("-column", "parse")
	s.passes(r)
	s.contains(r, viaLeft)
	s.hasSections(prices, map[string]string{"via/parse/quote/A1": "3\n"})

	s.note("run the update, as the lift's failure says")
	s.settle(stripModule...)
	r = s.normal("strip-a")
	s.fails(r, claims+"run hypercase strip -root . -column parse at the module root, then FIXTURE_UPDATE_GOLDEN=1 go test ./... until it changes nothing.")
	s.lacks(r, "differ in bytes", "first settle", "remove gaps line")
	r = s.strip(s.printedStrip(r, "-column", "parse")...)
	s.passes(r)
	s.contains(r, viaLeft)
	s.hasSections(prices, map[string]string{"gaps": viaGap}, "via/parse/")
	for _, r := range s.settle(stripModule...) {
		s.passes(r)
	}
	s.hasSections(prices, map[string]string{"gaps": viaGap}, "via/parse/")
	s.passes(s.normal("strip-a"))
	s.passes(s.run("b", "strip-b", false))
	s.done()
}

// TestStripRootCheckViaUntested: c, a package with no tests, holds a
// registry that records parse as a bytes lift (viaDisputed), and the
// hand-written variant doubled holds a via/parse/… too, in its parse
// cell, which its own gaps line marks. The root-package check doesn't
// compare c's entries, and fails on the registry itself, saying to delete
// it. Strip reads every registry, c's too, and leaves both via/parse/…,
// and so does the check, so the message for them names no strip: it says
// to settle the registries first, naming both lines. With c's registry
// deleted, the message says to strip, and no gaps line keeps strip from
// the sections. Strip removes them and leaves both frozen cells, gaps
// lines and all, and the update then leaves the check passing.
func TestStripRootCheckViaUntested(t *testing.T) {
	t.Parallel()
	s := viaDisputed(t, "c/testdata/hypercase.json")
	s.edit(prices, "a product parse doesn't make, in the variant's parse cell, which its gaps line marks", func(a *archive.Archive) {
		insert("variant/doubled/gaps", viaGap, "variant/doubled/via/")(a)
		insert("variant/doubled/via/parse/quote/A1", "6\n", "variant/doubled/out/")(a)
	})
	const (
		claims      = "testdata/export/prices.txtar: no registry claims via/parse/quote/A1 and variant/doubled/via/parse/quote/A1: no bytes lift named parse runs over the corpus export.\nIf a bytes lift should write them, register it. If not, "
		variantLeft = "testdata/export/prices.txtar: left the cell variant/doubled/parse, frozen by its known gap, variant/doubled/gaps line 1; hypercase strip -column parse -frozen strips it, and parse's other frozen cells\n"
	)
	gaps := map[string]string{"gaps": viaGap, "variant/doubled/gaps": viaGap}
	r := s.normal("strip-a")
	s.fails(r, "c/testdata/hypercase.json is a registry, and c holds no _test.go file, so no test compares it, and it still claims sections.\nDelete it, or restore the package's tests.",
		claims+"first settle whether parse is a bytes lift, on which the registries differ, as the check's failures on them say, then run the check again: until they agree, it can't tell whether gaps line 1 and variant/doubled/gaps line 1, which mark cells of theirs, keep strip from removing them.")
	s.lacks(r, "differ in bytes", "remove gaps line")
	r = s.strip("-column", "parse")
	s.passes(r)
	s.contains(r, viaLeft, variantLeft)
	s.hasSections(prices, map[string]string{"via/parse/quote/A1": "3\n", "variant/doubled/via/parse/quote/A1": "6\n"})

	s.remove("c/testdata/hypercase.json", "delete c's registry, as its failure says")
	r = s.normal("strip-a")
	s.fails(r, claims+"run hypercase strip -root . -column parse at the module root, then FIXTURE_UPDATE_GOLDEN=1 go test ./... until it changes nothing.")
	s.lacks(r, "first settle", "remove gaps line")
	r = s.strip(s.printedStrip(r, "-column", "parse")...)
	s.passes(r)
	s.contains(r, viaLeft, variantLeft)
	s.hasSections(prices, gaps, "via/parse/", "variant/doubled/via/parse/")
	for _, r := range s.settle(stripModule...) {
		s.passes(r)
	}
	s.hasSections(prices, gaps, "via/parse/", "variant/doubled/via/parse/")
	s.passes(s.normal("strip-a"))
	s.passes(s.run("b", "strip-b", false))
	s.done()
}

// printedStrip returns the arguments of the hypercase strip command that a
// root-package check's message in r says to run at the module root, for
// the sections the flag given selects, as a person pastes it. The tests
// that read it name roots no shell quotes.
func (s *sandbox) printedStrip(r result, flag ...string) []string {
	s.t.Helper()
	for _, m := range regexp.MustCompile(`run hypercase strip (-[^\n]*?) at the module root`).FindAllStringSubmatch(r.out, -1) {
		if args := strings.Fields(m[1]); slices.Equal(args[max(len(args)-len(flag), 0):], flag) {
			return args
		}
	}
	s.t.Fatalf("no message says to run hypercase strip %s at the module root:\n%s", strings.Join(flag, " "), r.out)
	return nil
}

// TestStripRootCheckNamesRoot: the module root's registry is stale, and
// records another corpus root, step09, whose archive holds the same
// out/nobody/… as the module-root corpus's. A strip at the module root
// with no -root reads that registry, and strips step09. So the
// root-package check's strip command names the module-root corpus with
// -root: run as printed, it strips that corpus's sections, and leaves
// step09's.
func TestStripRootCheckNamesRoot(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "strip")
	s.settle(stripModule...)
	const (
		reg     = "testdata/hypercase.json"
		step09  = "step09/" + prices
		section = "out/nobody/outcome"
	)
	g, err := registry.Parse(s.read(reg))
	if err != nil {
		t.Fatal(err)
	}
	g.CorpusRoot = "step09"
	s.write(reg, g.Marshal(), "the module root's registry, recording another corpus root, step09")
	s.write(step09, s.read(prices), "a copy of prices under step09")
	for _, p := range []string{prices, step09} {
		s.edit(p, "a removed column's outcome", insert(section, "accepted\n", "out/parse/"))
	}
	r := s.normal("strip-a")
	s.fails(r, "testdata/export/prices.txtar: no registry claims out/nobody/outcome: no column named nobody runs over the corpus export.\nIf a column should write them, register the column. If not, run hypercase strip ")
	s.passes(s.strip(s.printedStrip(r, "-column", "nobody")...))
	if _, ok := archive.Parse(s.read(prices)).Section(section); ok {
		t.Errorf("the printed strip left %s in %s:\n%s", section, prices, s.read(prices))
	}
	if _, ok := archive.Parse(s.read(step09)).Section(section); !ok {
		t.Errorf("the printed strip removed %s from %s, under the corpus root the registry records:\n%s", section, step09, s.read(step09))
	}
	s.done()
}
