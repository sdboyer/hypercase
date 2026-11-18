package main

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
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/diff"
	"github.com/sdboyer/hypercase/internal/hooks"
	"github.com/sdboyer/hypercase/internal/registry"
	"github.com/sdboyer/hypercase/internal/root"
	"github.com/sdboyer/hypercase/internal/walk"
)

// The strip fixture: a module whose corpus root is its root, with three
// packages over it, ., ./format and ./a/b/c, and a second corpus root,
// step09. Its archives are the sheets': twice, a sheet with a hand-written
// variant, cycle, and the variants of the transforms looped and renamed;
// and prices, a CSV that the bytes lift import makes a sheet of, with a
// hand-written variant, doubled, and the variant of the transform padded.
// The packages' registries record only what strip reads, and two hold
// findings.

const twiceFull = `A1 is twice B1.
-- in/sheet/A1 --
=B1*2
-- in/sheet/B1 --
3
-- out/evaluate/outcome --
accepted
-- out/evaluate/values.txt --
A1 6
B1 3
-- out/resolve/outcome --
accepted
-- variant/cycle/about --
B1 refers back to A1, so the sheet has a cycle.
-- variant/cycle/properties --
circular
-- variant/cycle/in/sheet/B1 --
=A1
-- variant/cycle/out/evaluate/outcome --
blocked resolve
-- variant/cycle/out/resolve/error.txt --
A1 -> B1 -> A1: cycle
-- variant/cycle/out/resolve/outcome --
rejected
-- transform/looped/in/sheet/A1 --
=A1
-- transform/looped/out/evaluate/outcome --
blocked resolve
-- transform/looped/out/resolve/error.txt --
A1 -> A1: cycle
-- transform/looped/out/resolve/outcome --
rejected
-- transform/renamed/in/sheet/A2 --
=B2*2
-- transform/renamed/in/sheet/B2 --
3
-- transform/renamed/delete --
sheet/A1
sheet/B1
-- transform/renamed/out/evaluate/outcome --
accepted
-- transform/renamed/out/evaluate/values.txt --
A2 6
B2 3
-- transform/renamed/out/resolve/outcome --
accepted
`

// twiceHand is twice's hand-written sections, written out by hand.
const twiceHand = `A1 is twice B1.
-- in/sheet/A1 --
=B1*2
-- in/sheet/B1 --
3
-- variant/cycle/about --
B1 refers back to A1, so the sheet has a cycle.
-- variant/cycle/properties --
circular
-- variant/cycle/in/sheet/B1 --
=A1
`

const pricesFull = `A CSV of one price, which import makes a sheet of.
-- in/csv/prices.csv --
A1,3
-- via/import/sheet/A1 --
3
-- out/evaluate/outcome --
accepted
-- out/evaluate/values.txt --
A1 3
-- out/import/outcome --
accepted
-- variant/doubled/about --
The price is doubled.
-- variant/doubled/in/csv/prices.csv --
A1,6
-- variant/doubled/via/import/sheet/A1 --
6
-- variant/doubled/out/evaluate/outcome --
accepted
-- variant/doubled/out/evaluate/values.txt --
A1 6
-- variant/doubled/out/import/outcome --
accepted
-- transform/padded/in/csv/prices.csv --
 A1 , 3
-- transform/padded/via/import/sheet/A1 --
3
-- transform/padded/out/evaluate/outcome --
accepted
-- transform/padded/out/evaluate/values.txt --
A1 3
-- transform/padded/out/import/outcome --
accepted
`

const pricesHand = `A CSV of one price, which import makes a sheet of.
-- in/csv/prices.csv --
A1,3
-- variant/doubled/about --
The price is doubled.
-- variant/doubled/in/csv/prices.csv --
A1,6
`

const sheetsVar = "SHEETS_UPDATE_GOLDEN"

func stripRegistry(root, updateVar string) string {
	return string(registry.Registry{UpdateVar: updateVar, CorpusRoot: root}.Marshal())
}

// stripModule returns the strip fixture's files, by slash path.
func stripModule() map[string]string {
	return map[string]string{
		"go.mod":                              "module example.com/sheets\n\ngo 1.27\n",
		"testdata/hypercase.json":             stripRegistry(".", sheetsVar),
		"testdata/checks/located.txt":         "sheet/twice evaluate\n",
		"testdata/sheet/twice.txtar":          twiceFull,
		"testdata/csv/prices.txtar":           pricesFull,
		"format/testdata/hypercase.json":      stripRegistry(".", sheetsVar),
		"format/testdata/checks/formulas.txt": "sheet/twice A1 =B1*2\n",
		"format/testdata/checks/notes.md":     "Not a check's findings.\n",
		"a/b/c/testdata/hypercase.json":       stripRegistry(".", sheetsVar),
		"step09/testdata/hypercase.json":      stripRegistry("step09", sheetsVar),
		"step09/testdata/sheet/twice.txtar":   twiceFull,
	}
}

// writeTree writes files, by slash path, under a new temp dir, and returns
// it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, data := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// tree reads every file under dir, by slash path, but the corpus roots'
// lock files, which strip creates as an update does.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == archive.LockFile {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		files[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// sameTree checks that dir holds exactly the files want holds.
func sameTree(t *testing.T, dir string, want map[string]string) {
	t.Helper()
	got := tree(t, dir)
	for _, p := range slices.Sorted(maps.Keys(want)) {
		g, ok := got[p]
		switch {
		case !ok:
			t.Errorf("%s is gone", p)
		case g != want[p]:
			t.Errorf("%s differs (-want +got):\n%s", p, diff.Lines([]byte(want[p]), []byte(g)))
		}
	}
	for _, p := range slices.Sorted(maps.Keys(got)) {
		if _, ok := want[p]; !ok {
			t.Errorf("%s is there, and shouldn't be", p)
		}
	}
}

// with returns files with the changes laid over them: a "" value removes a
// file.
func with(files map[string]string, changes map[string]string) map[string]string {
	out := maps.Clone(files)
	for p, data := range changes {
		if data == "" {
			delete(out, p)
		} else {
			out[p] = data
		}
	}
	return out
}

// stripIn runs hypercase strip in dir, a slash path under the tree.
func stripIn(t *testing.T, top, dir string, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(filepath.Join(top, filepath.FromSlash(dir)), nil, append([]string{"strip"}, args...), &out, &errb)
	return result{stdout: out.String(), stderr: errb.String(), code: code}
}

// stripOK runs strip and fails the test unless it exits 0 with nothing on
// stderr, and its stdout is want.
func stripOK(t *testing.T, top, dir string, want string, args ...string) {
	t.Helper()
	r := stripIn(t, top, dir, args...)
	if r.code != 0 || r.stderr != "" {
		t.Errorf("hypercase strip %s exited %d:\n%s", strings.Join(args, " "), r.code, r.stderr)
	}
	if r.stdout != want {
		t.Errorf("hypercase strip %s printed (-want +got):\n%s", strings.Join(args, " "), diff.Lines([]byte(want), []byte(r.stdout)))
	}
}

// stripFails runs strip, and fails the test unless it exits with code,
// with each of want on stderr and nothing on stdout.
func stripFails(t *testing.T, top, dir string, code int, args []string, want ...string) {
	t.Helper()
	r := stripIn(t, top, dir, args...)
	if r.code != code {
		t.Errorf("hypercase strip %s exited %d, want %d:\n%s", strings.Join(args, " "), r.code, code, r.stderr)
	}
	if r.stdout != "" {
		t.Errorf("hypercase strip %s wrote to stdout:\n%s", strings.Join(args, " "), r.stdout)
	}
	for _, w := range want {
		if !strings.Contains(r.stderr, w) {
			t.Errorf("hypercase strip %s: stderr doesn't hold %q:\n%s", strings.Join(args, " "), w, r.stderr)
		}
	}
}

// filtered returns the archive less each section whose name matches re.
func filtered(data, re string) string {
	a := archive.Parse([]byte(data))
	m := regexp.MustCompile(re)
	next := &archive.Archive{Comment: a.Comment}
	for _, s := range a.Sections {
		if !m.MatchString(s.Name) {
			next.Sections = append(next.Sections, s)
		}
	}
	return string(next.Format())
}

func sectionCount(data string) int { return len(archive.Parse([]byte(data)).Sections) }

const nextLine = "Next, run SHEETS_UPDATE_GOLDEN=1 go test ./... at the module root, and again until it changes nothing.\n"

// TestStripAll: with no flags, strip removes every update-written section
// of every archive under the corpus root, base, variants and transforms'
// variants, via/ included, and leaves every hand-written section's bytes,
// in their order. It removes the registries that record the corpus root
// and the findings files in their packages' testdata/checks/, and nothing
// of the corpus root step09.
func TestStripAll(t *testing.T) {
	t.Parallel()
	for _, hand := range []string{twiceHand, pricesHand} {
		if a := archive.Parse([]byte(hand)); len(a.Check()) > 0 || len(a.HandWrittenSections()) != len(a.Sections) {
			t.Fatalf("a hand-written form isn't one:\n%s", hand)
		}
	}
	files := stripModule()
	top := writeTree(t, files)
	stripOK(t, top, "", fmt.Sprintf("testdata/csv/prices.txtar: removed %d sections\ntestdata/sheet/twice.txtar: removed %d sections\n", sectionCount(pricesFull)-sectionCount(pricesHand), sectionCount(twiceFull)-sectionCount(twiceHand))+nextLine)
	sameTree(t, top, with(files, map[string]string{
		"testdata/sheet/twice.txtar":          twiceHand,
		"testdata/csv/prices.txtar":           pricesHand,
		"testdata/hypercase.json":             "",
		"testdata/checks/located.txt":         "",
		"format/testdata/hypercase.json":      "",
		"format/testdata/checks/formulas.txt": "",
		"a/b/c/testdata/hypercase.json":       "",
	}))

	// Again, with nothing left to strip and no registry to name the
	// update.
	stripOK(t, top, "", "Next, run the project's update over the whole module, at its root, and again until it changes nothing: no registry records the module-root corpus, so strip can't name its update variable.\n")
}

// TestStripNoRegistry: with no registry, strip removes the same sections
// from the archives, and its last line says to run the project's update.
func TestStripNoRegistry(t *testing.T) {
	t.Parallel()
	files := with(stripModule(), map[string]string{
		"testdata/hypercase.json":        "",
		"format/testdata/hypercase.json": "",
		"a/b/c/testdata/hypercase.json":  "",
	})
	top := writeTree(t, files)
	stripOK(t, top, "", fmt.Sprintf("testdata/csv/prices.txtar: removed %d sections\ntestdata/sheet/twice.txtar: removed %d sections\n", sectionCount(pricesFull)-sectionCount(pricesHand), sectionCount(twiceFull)-sectionCount(twiceHand))+
		"Next, run the project's update over the whole module, at its root, and again until it changes nothing: no registry records the module-root corpus, so strip can't name its update variable.\n")
	sameTree(t, top, with(files, map[string]string{
		"testdata/sheet/twice.txtar": twiceHand,
		"testdata/csv/prices.txtar":  pricesHand,
	}))
}

// TestStripColumn: -column limits strip to the column's sections, at every
// level, a bytes lift's via/ among them, and leaves the registries and
// findings files.
func TestStripColumn(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		column         string
		twice, prices  string
		twiceN, priceN int
	}{
		{"evaluate", filtered(twiceFull, `^((variant|transform)/[^/]+/)?out/evaluate/`), filtered(pricesFull, `^((variant|transform)/[^/]+/)?out/evaluate/`), 6, 6},
		{"import", twiceFull, filtered(pricesFull, `^((variant|transform)/[^/]+/)?(via|out)/import/`), 0, 6},
	} {
		t.Run(tt.column, func(t *testing.T) {
			t.Parallel()
			files := stripModule()
			top := writeTree(t, files)
			var want string
			if tt.priceN > 0 {
				want += fmt.Sprintf("testdata/csv/prices.txtar: removed %d sections\n", tt.priceN)
			}
			if tt.twiceN > 0 {
				want += fmt.Sprintf("testdata/sheet/twice.txtar: removed %d sections\n", tt.twiceN)
			}
			stripOK(t, top, "", want+nextLine, "-column", tt.column)
			sameTree(t, top, with(files, map[string]string{
				"testdata/sheet/twice.txtar": tt.twice,
				"testdata/csv/prices.txtar":  tt.prices,
			}))
		})
	}
}

// TestStripTransform: -transform limits strip to the sections under the
// transform's variant, its inputs among them, and leaves the registries
// and findings files. With -column, only the column's sections under the
// variant go.
func TestStripTransform(t *testing.T) {
	t.Parallel()
	files := stripModule()
	top := writeTree(t, files)
	stripOK(t, top, "", "testdata/sheet/twice.txtar: removed 4 sections\n"+nextLine, "-transform", "looped")
	sameTree(t, top, with(files, map[string]string{"testdata/sheet/twice.txtar": filtered(twiceFull, `^transform/looped/`)}))

	top = writeTree(t, files)
	stripOK(t, top, "", "testdata/sheet/twice.txtar: removed 2 sections\n"+nextLine, "-transform", "looped", "-column", "resolve")
	sameTree(t, top, with(files, map[string]string{"testdata/sheet/twice.txtar": filtered(twiceFull, `^transform/looped/out/resolve/`)}))

	top = writeTree(t, files)
	stripOK(t, top, "", "testdata/csv/prices.txtar: removed 2 sections\n"+nextLine, "-column", "import", "-transform", "padded")
	sameTree(t, top, with(files, map[string]string{"testdata/csv/prices.txtar": filtered(pricesFull, `^transform/padded/(via|out)/import/`)}))
}

// TestStripNoMatch: a -column or -transform that matches no section under
// the corpus root fails, naming it, and nothing is written.
func TestStripNoMatch(t *testing.T) {
	t.Parallel()
	files := stripModule()
	top := writeTree(t, files)
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"-column", "nutrition"}, "hypercase strip: -column nutrition matches no section under the module-root corpus: no archive holds out/nutrition/… or via/nutrition/…, at any level.\nName a column or transform whose sections an archive holds.\n"},
		{[]string{"-transform", "nope"}, "hypercase strip: -transform nope matches no section under the module-root corpus: no archive holds transform/nope/…."},
		{[]string{"-column", "import", "-transform", "looped"}, "hypercase strip: -column import -transform looped matches no section under the module-root corpus: no archive holds transform/looped/out/import/… or transform/looped/via/import/…."},
		{[]string{"-column", "padded"}, "hypercase strip: -column padded matches no section"},
	} {
		stripFails(t, top, "", 1, tt.args, tt.want)
	}
	if _, err := os.Stat(filepath.Join(top, "testdata", archive.LockFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a strip that matched nothing took the lock: %v", err)
	}
	sameTree(t, top, files)
}

// TestStripNoMatchLeft: when only archives that strip leaves as they are
// hold the sections -column names, the failure lists those archives, each
// with why, and no other, and nothing is written. An archive that can't be
// read is listed with them, since it may hold them.
func TestStripNoMatchLeft(t *testing.T) {
	t.Parallel()
	const twin = "A1 is 3.\n-- in/sheet/A1 --\n3\n-- out/nutrition/outcome --\naccepted\n-- out/nutrition/outcome --\naccepted\n"
	const stray = "A1 is 3.\n-- in/sheet/A1 --\n3\n-- out/nutrition/outcome --\naccepted\n"
	files := with(stripModule(), map[string]string{
		"testdata/sheet/twin.txtar":  twin,
		"testdata/sheet/old/y.txtar": stray,
		"testdata/x.txtar":           strings.ReplaceAll(stray, "nutrition", "evaluate"),
	})
	top := writeTree(t, files)
	r := stripIn(t, top, "", "-column", "nutrition")
	want := `hypercase strip: -column nutrition matches no section that strip removes under the module-root corpus. Only archives that it leaves as they are hold out/nutrition/… or via/nutrition/…, at any level, or can't be read:
testdata/sheet/old/y.txtar: an archive belongs directly in a corpus's directory, testdata/<corpus>/, and this one is below testdata/sheet/, so it is no case. Move it into a corpus's directory, or delete it.
testdata/sheet/twin.txtar: the archive has problems of form, so no update writes it: out/nutrition/outcome: the section appears twice. Fix the archive by hand.
Fix them, then run hypercase strip -column nutrition again.
`
	if r.code != 1 || r.stdout != "" || r.stderr != want {
		t.Errorf("strip -column nutrition exited %d, printing %q, with stderr (-want +got):\n%s", r.code, r.stdout, diff.Lines([]byte(want), []byte(r.stderr)))
	}
	sameTree(t, top, files)

	// An archive that can't be read, one with no permissions, may hold
	// them, and is listed with why.
	files = with(stripModule(), map[string]string{"testdata/sheet/locked.txtar": stray})
	top = writeTree(t, files)
	locked := filepath.Join(top, "testdata", "sheet", "locked.txtar")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o644) })
	if _, err := os.ReadFile(locked); err == nil {
		t.Skip("a file with no permissions reads here, as it does for root, so no read fails on it")
	}
	r = stripIn(t, top, "", "-column", "nutrition")
	want = "hypercase strip: -column nutrition matches no section that strip removes under the module-root corpus. Only archives that it leaves as they are hold out/nutrition/… or via/nutrition/…, at any level, or can't be read:\ntestdata/sheet/locked.txtar: reading it failed: open " + locked + ": permission denied.\nFix them, then run hypercase strip -column nutrition again.\n"
	if r.code != 1 || r.stdout != "" || r.stderr != want {
		t.Errorf("strip -column nutrition exited %d, printing %q, with stderr (-want +got):\n%s", r.code, r.stdout, diff.Lines([]byte(want), []byte(r.stderr)))
	}
	if err := os.Chmod(locked, 0o644); err != nil {
		t.Fatal(err)
	}
	sameTree(t, top, files)
}

// TestStripWalk: the walk for archives skips a directory under testdata/
// that holds a go.mod, a nested module, and one that is another corpus
// root, which a registry records. Strip leaves their files as they are,
// and doesn't list them.
func TestStripWalk(t *testing.T) {
	t.Parallel()
	const stray = "A1 is 3.\n-- in/sheet/A1 --\n3\n-- out/evaluate/outcome --\naccepted\n"
	files := with(stripModule(), map[string]string{
		"testdata/nested/go.mod":                  "module example.com/nested\n\ngo 1.27\n",
		"testdata/nested/testdata/sheet/a.txtar":  stray,
		"testdata/nested/sheet/b.txtar":           stray,
		"other/testdata/hypercase.json":           stripRegistry("testdata/inner", sheetsVar),
		"testdata/inner/testdata/sheet/c.txtar":   stray,
		"testdata/inner/testdata/sheet/d/e.txtar": stray,
	})
	top := writeTree(t, files)
	stripOK(t, top, "", fmt.Sprintf("testdata/csv/prices.txtar: removed %d sections\ntestdata/sheet/twice.txtar: removed %d sections\n", sectionCount(pricesFull)-sectionCount(pricesHand), sectionCount(twiceFull)-sectionCount(twiceHand))+nextLine)
	sameTree(t, top, with(files, map[string]string{
		"testdata/sheet/twice.txtar":          twiceHand,
		"testdata/csv/prices.txtar":           pricesHand,
		"testdata/hypercase.json":             "",
		"testdata/checks/located.txt":         "",
		"format/testdata/hypercase.json":      "",
		"format/testdata/checks/formulas.txt": "",
		"a/b/c/testdata/hypercase.json":       "",
	}))
}

// TestStripNoVariable: a registry that records no update variable counts
// toward none, so the variable the others record names the update.
func TestStripNoVariable(t *testing.T) {
	t.Parallel()
	files := with(stripModule(), map[string]string{"format/testdata/hypercase.json": stripRegistry(".", "")})
	top := writeTree(t, files)
	stripOK(t, top, "", fmt.Sprintf("testdata/csv/prices.txtar: removed %d sections\ntestdata/sheet/twice.txtar: removed %d sections\n", sectionCount(pricesFull)-sectionCount(pricesHand), sectionCount(twiceFull)-sectionCount(twiceHand))+nextLine)
	sameTree(t, top, with(files, map[string]string{
		"testdata/sheet/twice.txtar":          twiceHand,
		"testdata/csv/prices.txtar":           pricesHand,
		"testdata/hypercase.json":             "",
		"testdata/checks/located.txt":         "",
		"format/testdata/hypercase.json":      "",
		"format/testdata/checks/formulas.txt": "",
		"a/b/c/testdata/hypercase.json":       "",
	}))
}

// TestStripUsage: a repeated flag, an unknown flag, an argument and an
// empty or ill-formed name each fail with the usage, and nothing is
// written.
func TestStripUsage(t *testing.T) {
	t.Parallel()
	files := stripModule()
	top := writeTree(t, files)
	const usage = "usage: hypercase strip [-root dir] [-column name] [-transform name] [-frozen]"
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"-column", "evaluate", "-column", "resolve"}, `invalid value "resolve" for flag -column: the flag is given twice, and each flag may be given once`},
		{[]string{"-transform", "looped", "-transform", "looped"}, `invalid value "looped" for flag -transform: the flag is given twice`},
		{[]string{"-root", ".", "-root", "."}, `invalid value "." for flag -root: the flag is given twice`},
		{[]string{"-frozen", "-frozen"}, "invalid boolean flag frozen: the flag is given twice"},
		{[]string{"-nope"}, "flag provided but not defined: -nope"},
		{[]string{"testdata/sheet/twice.txtar"}, "hypercase strip takes no arguments, and was given 1: testdata/sheet/twice.txtar."},
		{[]string{"-column", "evaluate", "x", "y"}, "hypercase strip takes no arguments, and was given 2: x y."},
		{[]string{"-column", ""}, "hypercase strip: -column names a column, and was given an empty name."},
		{[]string{"-root="}, "hypercase strip: -root names the corpus root, and was given an empty name."},
		{[]string{"-column", "out/evaluate"}, "hypercase strip: -column out/evaluate is no column's name, which is lowercase ASCII letters and digits"},
		{[]string{"-transform", "Looped"}, "hypercase strip: -transform Looped is no transform's name, which is lowercase ASCII letters and digits"},
	} {
		stripFails(t, top, "", 2, tt.args, tt.want, usage)
	}
	if r := stripIn(t, top, "", "-h"); r.code != 0 || r.stdout != "" || !strings.Contains(r.stderr, usage) || !strings.Contains(r.stderr, "the corpus root dir, relative to the current directory or by an absolute path") {
		t.Errorf("strip -h: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	sameTree(t, top, files)
}

// The frozen fixture: the strip fixture with four frozen cells, evaluate's
// and transform/looped/resolve's in twice's gaps, variant/cycle/resolve's
// in its variant's, and import's in prices's.
const (
	twiceGaps  = "evaluate Evaluates A1 to 6, and B1 to 3. Waits on sheets#4.\ntransform/looped resolve Names the cycle's cells. Waits on sheets#5.\n"
	cycleGaps  = "resolve Says which cell to break the cycle at. Waits on sheets#6.\n"
	pricesGaps = "import Keeps the price's currency. Waits on sheets#7.\n"
)

func frozenModule() map[string]string {
	twice := strings.Replace(twiceFull, "-- out/evaluate/outcome --", "-- gaps --\n"+twiceGaps+"-- out/evaluate/outcome --", 1)
	twice = strings.Replace(twice, "-- variant/cycle/in/sheet/B1 --", "-- variant/cycle/gaps --\n"+cycleGaps+"-- variant/cycle/in/sheet/B1 --", 1)
	prices := strings.Replace(pricesFull, "-- via/import/sheet/A1 --", "-- gaps --\n"+pricesGaps+"-- via/import/sheet/A1 --", 1)
	return with(stripModule(), map[string]string{"testdata/sheet/twice.txtar": twice, "testdata/csv/prices.txtar": prices})
}

// TestStripFrozen: strip leaves a frozen cell's sections in place, a bytes
// lift's via/ among them, and lists the cell; -frozen strips them too. The
// freeze is the cell's: a transform's inputs and the cells the line
// doesn't mark are stripped. Every gaps section keeps its bytes.
func TestStripFrozen(t *testing.T) {
	t.Parallel()
	files := frozenModule()
	top := writeTree(t, files)
	stripOK(t, top, "", `testdata/csv/prices.txtar: removed 11 sections
testdata/csv/prices.txtar: left the cell import, frozen by its known gap, gaps line 1; hypercase strip -column import -frozen strips it, and import's other frozen cells
testdata/sheet/twice.txtar: removed 10 sections
testdata/sheet/twice.txtar: left the cell evaluate, frozen by its known gap, gaps line 1; hypercase strip -column evaluate -frozen strips it, and evaluate's other frozen cells
testdata/sheet/twice.txtar: left the cell variant/cycle/resolve, frozen by its known gap, variant/cycle/gaps line 1; hypercase strip -column resolve -frozen strips it, and resolve's other frozen cells
testdata/sheet/twice.txtar: left the cell transform/looped/resolve, frozen by its known gap, gaps line 2; hypercase strip -column resolve -transform looped -frozen strips it, and resolve's other frozen cells under transform/looped/
`+nextLine)
	got := tree(t, top)
	for _, tt := range []struct{ path, want string }{
		{"testdata/sheet/twice.txtar", `A1 is twice B1.
-- in/sheet/A1 --
=B1*2
-- in/sheet/B1 --
3
-- gaps --
` + twiceGaps + `-- out/evaluate/outcome --
accepted
-- out/evaluate/values.txt --
A1 6
B1 3
-- variant/cycle/about --
B1 refers back to A1, so the sheet has a cycle.
-- variant/cycle/properties --
circular
-- variant/cycle/gaps --
` + cycleGaps + `-- variant/cycle/in/sheet/B1 --
=A1
-- variant/cycle/out/resolve/error.txt --
A1 -> B1 -> A1: cycle
-- variant/cycle/out/resolve/outcome --
rejected
-- transform/looped/out/resolve/error.txt --
A1 -> A1: cycle
-- transform/looped/out/resolve/outcome --
rejected
`},
		{"testdata/csv/prices.txtar", `A CSV of one price, which import makes a sheet of.
-- in/csv/prices.csv --
A1,3
-- gaps --
` + pricesGaps + `-- via/import/sheet/A1 --
3
-- out/import/outcome --
accepted
-- variant/doubled/about --
The price is doubled.
-- variant/doubled/in/csv/prices.csv --
A1,6
`},
	} {
		if got[tt.path] != tt.want {
			t.Errorf("%s (-want +got):\n%s", tt.path, diff.Lines([]byte(tt.want), []byte(got[tt.path])))
		}
	}

	// -frozen strips the frozen cells too, and leaves every gaps section.
	stripOK(t, top, "", "testdata/csv/prices.txtar: removed 2 sections\ntestdata/sheet/twice.txtar: removed 6 sections\n"+
		"Next, run the project's update over the whole module, at its root, and again until it changes nothing: no registry records the module-root corpus, so strip can't name its update variable.\n", "-frozen")
	got = tree(t, top)
	for path, gaps := range map[string][]string{
		"testdata/sheet/twice.txtar": {"gaps", twiceGaps, "variant/cycle/gaps", cycleGaps},
		"testdata/csv/prices.txtar":  {"gaps", pricesGaps},
	} {
		a := archive.Parse([]byte(got[path]))
		if len(a.Sections) != len(a.HandWrittenSections()) {
			t.Errorf("%s keeps update-written sections:\n%s", path, got[path])
		}
		for i := 0; i < len(gaps); i += 2 {
			if data, _ := a.Section(gaps[i]); string(data) != gaps[i+1] {
				t.Errorf("%s's %s is %q, not %q", path, gaps[i], data, gaps[i+1])
			}
		}
	}
}

// TestStripThaw: the command a frozen cell's line gives strips that cell
// and the column's other frozen cells, and no other frozen cell. Each, run
// as its line says after a strip with no flags, leaves the frozen cells of
// the other columns. transform/looped/resolve's, with -transform, leaves
// variant/cycle/resolve; variant/cycle/resolve's, which no flag limits to
// its variant, strips transform/looped/resolve too.
func TestStripThaw(t *testing.T) {
	t.Parallel()
	top := writeTree(t, frozenModule())
	r := stripIn(t, top, "")
	if r.code != 0 || r.stderr != "" {
		t.Fatalf("strip exited %d:\n%s", r.code, r.stderr)
	}
	stripped := tree(t, top)
	cmds := map[string][]string{}
	for _, m := range regexp.MustCompile(`left the cell (\S+), frozen by its known gap, [^;]*; hypercase strip (.*) strips it, `).FindAllStringSubmatch(r.stdout, -1) {
		cmds[m[1]] = strings.Fields(m[2])
	}
	if len(cmds) != 4 {
		t.Fatalf("strip printed the commands %q for four frozen cells:\n%s", cmds, r.stdout)
	}
	for _, tt := range []struct {
		cell, archive string
		removed       []string
	}{
		{"evaluate", "testdata/sheet/twice.txtar", []string{"out/evaluate/outcome", "out/evaluate/values.txt"}},
		{"import", "testdata/csv/prices.txtar", []string{"via/import/sheet/A1", "out/import/outcome"}},
		{"variant/cycle/resolve", "testdata/sheet/twice.txtar", []string{"variant/cycle/out/resolve/error.txt", "variant/cycle/out/resolve/outcome", "transform/looped/out/resolve/error.txt", "transform/looped/out/resolve/outcome"}},
		{"transform/looped/resolve", "testdata/sheet/twice.txtar", []string{"transform/looped/out/resolve/error.txt", "transform/looped/out/resolve/outcome"}},
	} {
		t.Run(strings.ReplaceAll(tt.cell, "/", "_"), func(t *testing.T) {
			t.Parallel()
			dir := writeTree(t, stripped)
			if r := stripIn(t, dir, "", cmds[tt.cell]...); r.code != 0 || r.stderr != "" {
				t.Fatalf("hypercase strip %s exited %d:\n%s", strings.Join(cmds[tt.cell], " "), r.code, r.stderr)
			}
			var names []string
			for _, n := range tt.removed {
				names = append(names, regexp.QuoteMeta(n))
			}
			sameTree(t, dir, with(stripped, map[string]string{tt.archive: filtered(stripped[tt.archive], "^("+strings.Join(names, "|")+")$")}))
		})
	}
}

// resolveViaTwice is twice with a via/resolve/… in a cell a gaps line
// marks, resolve's, where resolve is a lift.
func resolveViaTwice() string {
	return strings.Replace(twiceFull, "-- out/evaluate/outcome --", "-- gaps --\nresolve Names the cycle's cells. Waits on sheets#5.\n-- via/resolve/sheet/A1 --\n6\n-- out/evaluate/outcome --", 1)
}

// liftsRegistry is the module-root package's registry, recording the
// lifts over the corpora: resolve, which makes no bytes, and import, a
// bytes lift.
func liftsRegistry() string {
	return string(registry.Registry{UpdateVar: sheetsVar, CorpusRoot: ".", Lifts: []registry.Lift{
		{Name: "resolve", Corpora: []string{"sheet"}, Paths: []registry.Path{{Corpus: "sheet"}}},
		{Name: "import", Bytes: true, Corpora: []string{"csv"}, Paths: []registry.Path{{Corpus: "csv"}}},
	}}.Marshal())
}

// TestStripFrozenVia: a via/<column>/… goes with a frozen cell only for a
// bytes lift, as the runner reads a cell. Where the registries say the
// column is a lift of no bytes, resolve here, a via/resolve/… is part of
// no cell, and -column resolve removes it while it leaves the frozen
// out/resolve/…. Where no registry says, it may be a bytes lift's, and is
// left with the cell: the registries record no column, or one of them is
// of another format. Where they disagree, which the root-package check
// fails on, one saying it is a bytes lift is enough to leave it. A
// registry that names the corpus root and can't be read fails strip, and
// nothing is written: one that isn't a registry, and one whose paths
// don't follow its corpora.
func TestStripFrozenVia(t *testing.T) {
	t.Parallel()
	twice := resolveViaTwice()
	lifts := liftsRegistry()
	older := strings.Replace(lifts, fmt.Sprintf(`"format": %d`, registry.Format), fmt.Sprintf(`"format": %d`, registry.Format-1), 1)
	if older == lifts {
		t.Fatalf("the registry holds no format %d", registry.Format)
	}
	bytesResolve := string(registry.Registry{UpdateVar: sheetsVar, CorpusRoot: ".", Lifts: []registry.Lift{{Name: "resolve", Bytes: true, Corpora: []string{"sheet"}, Paths: []registry.Path{{Corpus: "sheet"}}}}}.Marshal())
	const frozen = "testdata/sheet/twice.txtar: left the cell resolve, frozen by its known gap, gaps line 1; hypercase strip -column resolve -frozen strips it, and resolve's other frozen cells\n"
	for _, tt := range []struct {
		name, registry, format string
		out                    string
		removed                []string
	}{
		{"known", lifts, "", "testdata/sheet/twice.txtar: removed 6 sections\n" + frozen + nextLine, []string{"via/resolve/sheet/A1"}},
		{"unknown", stripRegistry(".", sheetsVar), "", "testdata/sheet/twice.txtar: removed 5 sections\n" + frozen + nextLine, nil},
		{"older", older, "", "testdata/sheet/twice.txtar: removed 5 sections\n" + frozen + nextLine, nil},
		{"disagreeing", lifts, bytesResolve, "testdata/sheet/twice.txtar: removed 5 sections\n" + frozen + nextLine, nil},
		{"disagreeing the other way", bytesResolve, lifts, "testdata/sheet/twice.txtar: removed 5 sections\n" + frozen + nextLine, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files := with(stripModule(), map[string]string{"testdata/hypercase.json": tt.registry, "testdata/sheet/twice.txtar": twice})
			if tt.format != "" {
				files["format/testdata/hypercase.json"] = tt.format
			}
			top := writeTree(t, files)
			stripOK(t, top, "", tt.out, "-column", "resolve")
			got := archive.Parse([]byte(tree(t, top)["testdata/sheet/twice.txtar"]))
			var left []string
			for _, s := range got.Sections {
				if !archive.HandWritten(s.Name) {
					left = append(left, s.Name)
				}
			}
			want := []string{"via/resolve/sheet/A1", "out/evaluate/outcome", "out/evaluate/values.txt", "out/resolve/outcome", "variant/cycle/out/evaluate/outcome", "transform/looped/in/sheet/A1", "transform/looped/out/evaluate/outcome", "transform/renamed/in/sheet/A2", "transform/renamed/in/sheet/B2", "transform/renamed/delete", "transform/renamed/out/evaluate/outcome", "transform/renamed/out/evaluate/values.txt"}
			want = slices.DeleteFunc(want, func(n string) bool { return slices.Contains(tt.removed, n) })
			if !slices.Equal(left, want) {
				t.Errorf("strip -column resolve left the update-written sections %q, not %q", left, want)
			}
		})
	}

	files := with(stripModule(), map[string]string{"testdata/hypercase.json": strings.Replace(lifts, "{\n", "{\n  \"nope\": 1,\n", 1), "testdata/sheet/twice.txtar": twice})
	top := writeTree(t, files)
	stripFails(t, top, "", 1, []string{"-column", "resolve"}, "hypercase strip: testdata/hypercase.json can't be read as a registry: it is not a registry: json: unknown field \"nope\". So strip can't tell which of the columns it registers are bytes lifts. Nothing was removed.\nFix the file or delete it, then run hypercase strip -column resolve again.\n")
	sameTree(t, top, files)

	noPaths := string(registry.Registry{UpdateVar: sheetsVar, CorpusRoot: ".", Lifts: []registry.Lift{{Name: "resolve", Corpora: []string{"sheet"}}}}.Marshal())
	files = with(stripModule(), map[string]string{"testdata/hypercase.json": noPaths, "testdata/sheet/twice.txtar": twice})
	top = writeTree(t, files)
	stripFails(t, top, "", 1, []string{"-column", "resolve"}, "hypercase strip: testdata/hypercase.json can't be read as a registry: the lift resolve runs over testdata/sheet/, and has paths from none, where a registry gives each column one path from each corpus it runs over, in the same order. So strip can't tell which of the columns it registers are bytes lifts. Nothing was removed.\nFix the file or delete it, then run hypercase strip -column resolve again.\n")
	sameTree(t, top, files)
}

// TestStripProblems: an archive with problems of form, a sheet with a
// section twice or with variant/cycle/notes, is left byte for byte as it
// was and listed with the reason, and so is an archive no corpus holds,
// testdata/x.txtar and testdata/sheet/old/y.txtar, each with an out/
// section. The other archives are stripped, and strip exits 1. The
// registries stay while any of them is left.
func TestStripProblems(t *testing.T) {
	t.Parallel()
	const twin = "A1 is 3.\n-- in/sheet/A1 --\n3\n-- out/evaluate/outcome --\naccepted\n-- out/evaluate/outcome --\naccepted\n"
	const notes = "A1 is 3.\n-- in/sheet/A1 --\n3\n-- out/evaluate/outcome --\naccepted\n-- variant/cycle/about --\nA cycle.\n-- variant/cycle/notes --\nA note.\n"
	const stray = "A1 is 3.\n-- in/sheet/A1 --\n3\n-- out/evaluate/outcome --\naccepted\n"
	files := with(stripModule(), map[string]string{
		"testdata/sheet/twin.txtar":   twin,
		"testdata/sheet/notes.txtar":  notes,
		"testdata/x.txtar":            stray,
		"testdata/sheet/old/y.txtar":  stray,
		"testdata/sheet/old/y.golden": stray,
	})
	top := writeTree(t, files)
	r := stripIn(t, top, "")
	const kept = ": left in place, since this run left a *.txtar file as it was, so that strip, run again as that file's line says, reads the registry as this run did.\n"
	want := "a/b/c/testdata/hypercase.json" + kept + "format/testdata/hypercase.json" + kept + fmt.Sprintf(`testdata/csv/prices.txtar: removed %d sections
testdata/hypercase.json`+kept+`testdata/sheet/notes.txtar: left as it was: the archive has problems of form, so no update writes it: variant/cycle/notes: notes is not one of a hand-written variant's sections: about, in, delete, properties, gaps, via and out. Fix the archive by hand, then run hypercase strip again.
testdata/sheet/old/y.txtar: left as it was: an archive belongs directly in a corpus's directory, testdata/<corpus>/, and this one is below testdata/sheet/, so it is no case. Move it into a corpus's directory, or delete it, then run hypercase strip again.
testdata/sheet/twice.txtar: removed %d sections
testdata/sheet/twin.txtar: left as it was: the archive has problems of form, so no update writes it: out/evaluate/outcome: the section appears twice. Fix the archive by hand, then run hypercase strip again.
testdata/x.txtar: left as it was: an archive belongs in a corpus's directory, testdata/<corpus>/, and this one is directly in testdata/. Move it into one, or delete it, then run hypercase strip again.
`, sectionCount(pricesFull)-sectionCount(pricesHand), sectionCount(twiceFull)-sectionCount(twiceHand)) + nextLine
	if r.code != 1 || r.stderr != "" || r.stdout != want {
		t.Errorf("strip exited %d, with stderr %q, and printed (-want +got):\n%s", r.code, r.stderr, diff.Lines([]byte(want), []byte(r.stdout)))
	}
	// The registries are kept for a strip run again, once the archives
	// are fixed or moved; the findings files go.
	sameTree(t, top, with(files, map[string]string{
		"testdata/sheet/twice.txtar":          twiceHand,
		"testdata/csv/prices.txtar":           pricesHand,
		"testdata/checks/located.txt":         "",
		"format/testdata/checks/formulas.txt": "",
	}))
}

// TestStripRegistries: strip reads every registry under the module before
// it removes anything. Registries that record different update variables
// for the corpus root, or one that can't be read, fail it, naming them,
// and nothing is written.
func TestStripRegistries(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		changes map[string]string
		want    []string
	}{
		{"two variables", map[string]string{"format/testdata/hypercase.json": stripRegistry(".", "FORMAT_UPDATE_GOLDEN")}, []string{
			"hypercase strip: the registries that record the module-root corpus record different update variables: FORMAT_UPDATE_GOLDEN, in format/testdata/hypercase.json; SHEETS_UPDATE_GOLDEN, in testdata/hypercase.json and a/b/c/testdata/hypercase.json. Nothing was removed.\n",
			"give every package over the corpus root the same decl.Project, run its update, then run hypercase strip again.",
		}},
		{"unreadable", map[string]string{"a/b/c/testdata/hypercase.json": "{\n"}, []string{
			"hypercase strip: a/b/c/testdata/hypercase.json can't be read as a registry: ",
			"Fix the file or delete it, then run hypercase strip again.",
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files := with(stripModule(), tt.changes)
			top := writeTree(t, files)
			stripFails(t, top, "", 1, nil, tt.want...)
			sameTree(t, top, files)
		})
	}
}

// TestStripRoot: -root names the corpus root, and strip changes nothing
// outside it. Without -root, strip finds the root from the package it runs
// in, by its registry, or else, at the module root, takes the module
// root's: run from a/b/c, several levels down, it strips the module-root
// corpus, and from step09, the corpus root step09. From a directory with
// no registry that isn't the module root, a/b, it asks for -root, and so
// it does when the registry there can't say which root it records, or
// records one inside a module nested in the package's.
func TestStripRoot(t *testing.T) {
	t.Parallel()
	files := stripModule()
	module := with(files, map[string]string{
		"testdata/sheet/twice.txtar":          twiceHand,
		"testdata/csv/prices.txtar":           pricesHand,
		"testdata/hypercase.json":             "",
		"testdata/checks/located.txt":         "",
		"format/testdata/hypercase.json":      "",
		"format/testdata/checks/formulas.txt": "",
		"a/b/c/testdata/hypercase.json":       "",
	})
	step09 := with(files, map[string]string{
		"step09/testdata/sheet/twice.txtar": twiceHand,
		"step09/testdata/hypercase.json":    "",
	})
	moduleOut := fmt.Sprintf("testdata/csv/prices.txtar: removed %d sections\ntestdata/sheet/twice.txtar: removed %d sections\n", sectionCount(pricesFull)-sectionCount(pricesHand), sectionCount(twiceFull)-sectionCount(twiceHand)) + nextLine
	step09Out := fmt.Sprintf("testdata/sheet/twice.txtar: removed %d sections\n", sectionCount(twiceFull)-sectionCount(twiceHand)) + nextLine
	for _, tt := range []struct {
		dir  string
		args []string
		out  string
		want map[string]string
	}{
		{"", []string{"-root", "step09"}, step09Out, step09},
		{"format", []string{"-root", "../step09"}, step09Out, step09},
		{"step09", nil, step09Out, step09},
		{"step09/testdata", []string{"-root", ".."}, step09Out, step09},
		{"a/b/c", nil, moduleOut, module},
		{"a/b", []string{"-root", "../.."}, moduleOut, module},
		{"step09", []string{"-root", ".."}, moduleOut, module},
	} {
		top := writeTree(t, files)
		stripOK(t, top, tt.dir, tt.out, tt.args...)
		sameTree(t, top, tt.want)
	}
	// An absolute -root names the root as a relative one does.
	abs := writeTree(t, files)
	stripOK(t, abs, "format", step09Out, "-root", filepath.Join(abs, "step09"))
	sameTree(t, abs, step09)

	top := writeTree(t, files)
	for _, tt := range []struct {
		dir  string
		args []string
		want string
	}{
		{"", []string{"-root", "nope"}, "hypercase strip: -root nope names no directory.\nName the corpus root relative to the current directory or by an absolute path.\n"},
		{"", []string{"-root", "a"}, "hypercase strip: " + filepath.Join(top, "a") + " holds no testdata/ directory, so it is no corpus root.\nName the corpus root with -root, relative to the current directory or by an absolute path.\n"},
		{"a/b", nil, "hypercase strip: the current directory holds no registry, testdata/hypercase.json, to say which corpus root its package's tests read, and it isn't the module root, ../.., where strip finds the module-root corpus.\nName the corpus root with -root, relative to the current directory or by an absolute path, or run strip at the module root.\n"},
		{"step09/testdata", nil, "hypercase strip: the current directory holds no registry, testdata/hypercase.json, to say which corpus root its package's tests read, and it isn't the module root, ../.., where strip finds the module-root corpus.\n"},
	} {
		r := stripIn(t, top, tt.dir, tt.args...)
		if r.code != 1 || r.stdout != "" || !strings.HasPrefix(r.stderr, tt.want) {
			t.Errorf("strip %s from %q exited %d, printing %q, with stderr (-want +got):\n%s", strings.Join(tt.args, " "), tt.dir, r.code, r.stdout, diff.Lines([]byte(tt.want), []byte(r.stderr)))
		}
		if strings.Contains(r.stderr, "usage:") {
			t.Errorf("strip %s from %q gave the usage, for no usage error:\n%s", strings.Join(tt.args, " "), tt.dir, r.stderr)
		}
	}
	sameTree(t, top, files)

	// The registry of the package strip runs in, when it can't say which
	// root it records.
	for _, tt := range []struct{ registry, want string }{
		{"{\n", "hypercase strip: testdata/hypercase.json can't be read as a registry: it is not a registry: unexpected end of JSON input, so strip can't tell which corpus root it records.\nName the corpus root with -root, relative to the current directory or by an absolute path.\n"},
		{stripRegistry("../x", sheetsVar), "hypercase strip: testdata/hypercase.json records the corpus root \"../x\", which isn't a slash path relative to the module root.\nName the corpus root with -root, relative to the current directory or by an absolute path.\n"},
	} {
		files := with(stripModule(), map[string]string{"step09/testdata/hypercase.json": tt.registry})
		top := writeTree(t, files)
		stripFails(t, top, "step09", 1, nil, tt.want)
		sameTree(t, top, files)
	}
	// One that can't be read at all: a directory in its place.
	files = with(stripModule(), map[string]string{"step09/testdata/hypercase.json": "", "step09/testdata/hypercase.json/x": "x\n"})
	top = writeTree(t, files)
	stripFails(t, top, "step09", 1, nil, "hypercase strip: reading testdata/hypercase.json failed: ", ", so strip can't tell which corpus root it records.\nFix what failed, or name the corpus root with -root, relative to the current directory or by an absolute path.\n")
	sameTree(t, top, files)

	// One whose corpus root lies inside a module nested in the package's:
	// strip would read that module's registries, and not the package's.
	files = with(stripModule(), map[string]string{
		"step09/testdata/hypercase.json":      stripRegistry("sub/corp", sheetsVar),
		"sub/go.mod":                          "module example.com/sub\n\ngo 1.27\n",
		"sub/corp/testdata/sheet/twice.txtar": twiceFull,
	})
	top = writeTree(t, files)
	stripFails(t, top, "step09", 1, nil, "hypercase strip: testdata/hypercase.json records the corpus root sub/corp, which lies inside the module at ../sub, and not in the module at .., the package's.\nMove the corpus root out of the module at ../sub, or name it with -root, relative to the current directory or by an absolute path, to strip it as a corpus root of that module.\n")
	sameTree(t, top, files)

	// A directory in no module, run from or named with -root.
	out := t.TempDir()
	if err := os.MkdirAll(filepath.Join(out, "testdata", "sheet"), 0o755); err != nil {
		t.Fatal(err)
	}
	stripFails(t, out, "", 1, nil, "hypercase strip: finding the module root failed: no go.mod in ", ".\nRun strip inside a Go module, or name the corpus root with -root, relative to the current directory or by an absolute path.\n")
	stripFails(t, top, "", 1, []string{"-root", out}, "hypercase strip: finding the module root of "+out+" failed: no go.mod in ", ".\nName a corpus root inside a Go module with -root, relative to the current directory or by an absolute path.\n")
}

// TestStripRerun: a frozen cell's line gives the command to strip the
// cell, and that command, run from the same directory, strips the same
// corpus root, and no other. It names the root with -root unless strip,
// run there without it, finds the same root whether or not the first run
// removed the registry there: run from step09, or from a/b/c over the
// module-root corpus, the first run removed the registry that named the
// root; at the module root, whose registry records step09, -root . named
// another root than that registry does. Only at the module root on the
// module-root corpus, with no registry there to name another, is -root
// left out.
func TestStripRerun(t *testing.T) {
	t.Parallel()
	const gaps = "-- gaps --\nevaluate Evaluates A1 to 6. Waits on sheets#4.\n"
	twice := strings.Replace(twiceFull, "-- out/evaluate/outcome --", gaps+"-- out/evaluate/outcome --", 1)
	for _, tt := range []struct {
		name    string
		changes map[string]string
		dir     string
		args    []string
		cmd     string // the frozen cell's command, past hypercase strip
		archive string // the archive whose frozen cell the command strips
	}{
		{"from step09", nil, "step09", nil, "-root . -column evaluate -frozen", "step09/testdata/sheet/twice.txtar"},
		{"from a/b/c", nil, "a/b/c", nil, "-root ../../.. -column evaluate -frozen", "testdata/sheet/twice.txtar"},
		{"the module root's registry records step09", map[string]string{"testdata/hypercase.json": stripRegistry("step09", sheetsVar)}, "", []string{"-root", "."}, "-root . -column evaluate -frozen", "testdata/sheet/twice.txtar"},
		{"at the module root", nil, "", nil, "-column evaluate -frozen", "testdata/sheet/twice.txtar"},
	} {
		t.Run(strings.ReplaceAll(tt.name, "/", "_"), func(t *testing.T) {
			t.Parallel()
			files := with(stripModule(), map[string]string{"step09/testdata/sheet/twice.txtar": twice, "testdata/sheet/twice.txtar": twice})
			top := writeTree(t, with(files, tt.changes))
			r := stripIn(t, top, tt.dir, tt.args...)
			line := "testdata/sheet/twice.txtar: left the cell evaluate, frozen by its known gap, gaps line 1; hypercase strip " + tt.cmd + " strips it, and evaluate's other frozen cells\n"
			if r.code != 0 || r.stderr != "" || !strings.Contains(r.stdout, line) {
				t.Fatalf("strip %s from %q exited %d, with stderr %q, and printed no line %q:\n%s", strings.Join(tt.args, " "), tt.dir, r.code, r.stderr, line, r.stdout)
			}
			after := tree(t, top)
			r = stripIn(t, top, tt.dir, strings.Fields(tt.cmd)...)
			if r.code != 0 || r.stderr != "" || !strings.HasPrefix(r.stdout, "testdata/sheet/twice.txtar: removed 2 sections\n") {
				t.Errorf("hypercase strip %s from %q exited %d, printing:\n%s%s", tt.cmd, tt.dir, r.code, r.stdout, r.stderr)
			}
			sameTree(t, top, with(after, map[string]string{tt.archive: filtered(after[tt.archive], "^out/evaluate/")}))
		})
	}

	// From step09, with its registry gone, strip without -root asks for it.
	files := with(stripModule(), map[string]string{"step09/testdata/hypercase.json": ""})
	top := writeTree(t, files)
	stripFails(t, top, "step09", 1, []string{"-frozen"}, "hypercase strip: the current directory holds no registry, testdata/hypercase.json, to say which corpus root its package's tests read, and it isn't the module root, ..,")
	sameTree(t, top, files)
}

// TestStripRerunQuoted: a line that says to run strip again quotes a
// corpus root whose path has a space, so that a POSIX shell reads the
// command as the arguments strip takes, and the command, run with them,
// strips the root. It runs sh to read the command, so it skips where there
// is none.
func TestStripRerunQuoted(t *testing.T) {
	t.Parallel()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("no sh to read the command: %v", err)
	}
	twice := strings.Replace(twiceFull, "-- out/evaluate/outcome --", "-- gaps --\nevaluate Evaluates A1 to 6. Waits on sheets#4.\n-- out/evaluate/outcome --", 1)
	files := with(stripModule(), map[string]string{
		"my corp/testdata/hypercase.json":    stripRegistry("my corp", sheetsVar),
		"my corp/testdata/sheet/twice.txtar": twice,
	})
	top := writeTree(t, files)
	r := stripIn(t, top, "", "-root", "my corp")
	m := regexp.MustCompile(`left the cell evaluate, frozen by its known gap, gaps line 1; hypercase strip (.*) strips it, `).FindStringSubmatch(r.stdout)
	if r.code != 0 || r.stderr != "" || m == nil || m[1] != "-root 'my corp' -column evaluate -frozen" {
		t.Fatalf("strip -root 'my corp' exited %d, printing no command with the root quoted:\n%s%s", r.code, r.stdout, r.stderr)
	}
	out, err := exec.Command(sh, "-c", "set -- "+m[1]+`; for a; do printf '%s\n' "$a"; done`).Output()
	if err != nil {
		t.Fatalf("sh reading %s: %v", m[1], err)
	}
	args := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if want := []string{"-root", "my corp", "-column", "evaluate", "-frozen"}; !slices.Equal(args, want) {
		t.Fatalf("sh reads %s as %q, not %q", m[1], args, want)
	}
	after := tree(t, top)
	stripOK(t, top, "", "testdata/sheet/twice.txtar: removed 2 sections\nNext, run the project's update over the whole module, at its root, and again until it changes nothing: no registry records the corpus root my corp, so strip can't name its update variable.\n", args...)
	const p = "my corp/testdata/sheet/twice.txtar"
	sameTree(t, top, with(after, map[string]string{p: filtered(after[p], "^out/evaluate/")}))
}

// TestStripFailedWrite: a write made to fail once its temp file is
// written leaves that archive as it was, with no temp file, and strip goes
// on to strip the next archive, keeps the registries, and exits 1. It sets
// a hook, so it doesn't run in parallel.
func TestStripFailedWrite(t *testing.T) {
	files := stripModule()
	top := writeTree(t, files)
	var temps []string
	hooks.AfterTemp = func(tmp string) error {
		temps = append(temps, filepath.Base(tmp))
		if len(temps) == 1 {
			return errors.New("disk full")
		}
		return nil
	}
	r := stripIn(t, top, "")
	hooks.AfterTemp = nil
	if len(temps) != 2 || !strings.HasPrefix(temps[0], ".hypercase-tmp-prices.txtar-") {
		t.Errorf("the writes' temp files were %q, not prices's and then twice's", temps)
	}
	const kept = ": left in place, since this run left a *.txtar file as it was, so that strip, run again as that file's line says, reads the registry as this run did.\n"
	want := "a/b/c/testdata/hypercase.json" + kept + "format/testdata/hypercase.json" + kept +
		"testdata/csv/prices.txtar: left as it was: stripping it failed, and the archive is as it was: disk full. Fix what failed, then run hypercase strip again.\n" +
		"testdata/hypercase.json" + kept +
		fmt.Sprintf("testdata/sheet/twice.txtar: removed %d sections\n", sectionCount(twiceFull)-sectionCount(twiceHand)) + nextLine
	if r.code != 1 || r.stderr != "" || r.stdout != want {
		t.Errorf("strip exited %d, with stderr %q, and printed (-want +got):\n%s", r.code, r.stderr, diff.Lines([]byte(want), []byte(r.stdout)))
	}
	// tree reads every file, so a temp file left behind is one more.
	sameTree(t, top, with(files, map[string]string{
		"testdata/sheet/twice.txtar":          twiceHand,
		"testdata/checks/located.txt":         "",
		"format/testdata/checks/formulas.txt": "",
	}))
}

// TestStripFailedWriteRerun: after a failed write, strip run again as its
// line says ends where one strip that fails nothing does. The registries
// the first run kept tell the second that resolve makes no bytes, so it
// removes the via/resolve/… of resolve's frozen cell, and it names the
// update by their variable. The test sets a hook, so it doesn't run in
// parallel.
func TestStripFailedWriteRerun(t *testing.T) {
	files := with(stripModule(), map[string]string{"testdata/hypercase.json": liftsRegistry(), "testdata/sheet/twice.txtar": resolveViaTwice()})
	once := writeTree(t, files)
	want := stripIn(t, once, "")
	if want.code != 0 || want.stderr != "" || !strings.Contains(want.stdout, "testdata/sheet/twice.txtar: left the cell resolve") {
		t.Fatalf("strip exited %d, printing:\n%s%s", want.code, want.stdout, want.stderr)
	}
	if _, ok := archive.Parse([]byte(tree(t, once)["testdata/sheet/twice.txtar"])).Section("via/resolve/sheet/A1"); ok {
		t.Fatalf("strip left via/resolve/sheet/A1, a section of no cell")
	}

	top := writeTree(t, files)
	failed := false
	hooks.AfterTemp = func(tmp string) error {
		if !failed && strings.HasPrefix(filepath.Base(tmp), ".hypercase-tmp-twice.txtar-") {
			failed = true
			return errors.New("disk full")
		}
		return nil
	}
	r := stripIn(t, top, "")
	hooks.AfterTemp = nil
	m := regexp.MustCompile(`(?m)^testdata/sheet/twice\.txtar: left as it was: .*disk full\. Fix what failed, then run hypercase strip(.*) again\.$`).FindStringSubmatch(r.stdout)
	if r.code != 1 || m == nil {
		t.Fatalf("strip exited %d, printing no failed write of twice:\n%s%s", r.code, r.stdout, r.stderr)
	}
	again := stripIn(t, top, "", strings.Fields(m[1])...)
	if again.code != 0 || again.stderr != "" || !strings.HasSuffix(again.stdout, nextLine) {
		t.Errorf("hypercase strip%s exited %d, printing:\n%s%s", m[1], again.code, again.stdout, again.stderr)
	}
	sameTree(t, top, tree(t, once))
}

// TestStripMisplacedRerun: an archive strip leaves for its place, below a
// corpus's directory, keeps every registry, as one left for its form or a
// failed write does. Moved into the corpus's directory, and stripped again
// as its line says, it ends where one strip of it in place does: the
// registries the first run kept tell the second that resolve makes no
// bytes, so it removes the via/resolve/… of resolve's frozen cell, and it
// names the update by their variable.
func TestStripMisplacedRerun(t *testing.T) {
	t.Parallel()
	files := with(stripModule(), map[string]string{"testdata/hypercase.json": liftsRegistry(), "testdata/sheet/twice.txtar": resolveViaTwice()})
	once := writeTree(t, files)
	if r := stripIn(t, once, ""); r.code != 0 || r.stderr != "" {
		t.Fatalf("strip exited %d, printing:\n%s%s", r.code, r.stdout, r.stderr)
	}
	if _, ok := archive.Parse([]byte(tree(t, once)["testdata/sheet/twice.txtar"])).Section("via/resolve/sheet/A1"); ok {
		t.Fatalf("strip left via/resolve/sheet/A1, a section of no cell")
	}

	top := writeTree(t, with(files, map[string]string{"testdata/sheet/twice.txtar": "", "testdata/sheet/old/twice.txtar": resolveViaTwice()}))
	r := stripIn(t, top, "")
	const kept = ": left in place, since this run left a *.txtar file as it was, so that strip, run again as that file's line says, reads the registry as this run did.\n"
	for _, reg := range []string{"testdata/hypercase.json", "format/testdata/hypercase.json", "a/b/c/testdata/hypercase.json"} {
		if !strings.Contains(r.stdout, reg+kept) {
			t.Errorf("strip printed no line keeping %s:\n%s", reg, r.stdout)
		}
	}
	m := regexp.MustCompile(`(?m)^testdata/sheet/old/twice\.txtar: left as it was: .*, so it is no case\. Move it into a corpus's directory, or delete it, then run hypercase strip(.*) again\.$`).FindStringSubmatch(r.stdout)
	var args []string
	if r.code != 1 || m == nil {
		t.Errorf("strip exited %d, printing no line that leaves testdata/sheet/old/twice.txtar and says to run strip again:\n%s%s", r.code, r.stdout, r.stderr)
	} else {
		args = strings.Fields(m[1])
	}
	old, moved := filepath.Join(top, "testdata", "sheet", "old", "twice.txtar"), filepath.Join(top, "testdata", "sheet", "twice.txtar")
	if err := os.Rename(old, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Dir(old)); err != nil {
		t.Fatal(err)
	}
	again := stripIn(t, top, "", args...)
	if again.code != 0 || again.stderr != "" || !strings.HasSuffix(again.stdout, nextLine) {
		t.Errorf("hypercase strip %s exited %d, printing:\n%s%s", strings.Join(args, " "), again.code, again.stdout, again.stderr)
	}
	sameTree(t, top, tree(t, once))
}

// TestStripFailedRemoval: a registry whose removal fails is left in place
// and listed, the others are removed, and strip exits 1. The removal is
// made to fail by putting a directory with a file in it where the
// registry was, the first time strip holds the lock, which is for the
// first archive it strips, testdata/csv/prices.txtar, before it removes
// any registry. So the test sets a hook, and doesn't run in parallel.
func TestStripFailedRemoval(t *testing.T) {
	files := stripModule()
	top := writeTree(t, files)
	reg := filepath.Join(top, "format", "testdata", "hypercase.json")
	hooks.Modify = func(step string) {
		if step != "locked" || !walk.IsFile(reg) {
			return
		}
		if err := os.Remove(reg); err != nil {
			t.Error(err)
		}
		if err := os.MkdirAll(filepath.Join(reg, "x"), 0o755); err != nil {
			t.Error(err)
		}
	}
	r := stripIn(t, top, "")
	hooks.Modify = nil
	if r.code != 1 || r.stderr != "" {
		t.Errorf("strip exited %d, with stderr %q", r.code, r.stderr)
	}
	line := "format/testdata/hypercase.json: left in place: removing it failed: remove " + reg + ": directory not empty. Fix what failed, then run hypercase strip again.\n"
	if !strings.Contains(r.stdout, line) || !strings.HasSuffix(r.stdout, nextLine) {
		t.Errorf("strip printed:\n%s\nwhich lacks the line %q, or doesn't end with the next step", r.stdout, line)
	}
	got := tree(t, top)
	for _, p := range []string{"testdata/hypercase.json", "a/b/c/testdata/hypercase.json", "format/testdata/checks/formulas.txt"} {
		if _, ok := got[p]; ok {
			t.Errorf("%s is left, and should be removed", p)
		}
	}
}

// strippedModule is the strip fixture as a strip with no flags leaves it.
func strippedModule() map[string]string {
	return with(stripModule(), map[string]string{
		"testdata/sheet/twice.txtar":          twiceHand,
		"testdata/csv/prices.txtar":           pricesHand,
		"testdata/hypercase.json":             "",
		"testdata/checks/located.txt":         "",
		"format/testdata/hypercase.json":      "",
		"format/testdata/checks/formulas.txt": "",
		"a/b/c/testdata/hypercase.json":       "",
	})
}

// TestStripFailedFindings: a package's findings file whose removal fails
// keeps the package's registry, so that strip run again, as its line
// says, finds the file and removes it, and then the registry. The removal
// is made to fail by putting a directory with a file in it where the
// findings file was, once strip holds the lock to remove it. Strip
// removes the registries' packages' files in the registries' path order,
// so that is the first time it holds the lock once a/b/c's registry is
// gone. The test sets a hook, so it doesn't run in parallel.
func TestStripFailedFindings(t *testing.T) {
	files := stripModule()
	top := writeTree(t, files)
	f := filepath.Join(top, "format", "testdata", "checks", "formulas.txt")
	abc := filepath.Join(top, "a", "b", "c", "testdata", registry.Name)
	hooks.Modify = func(step string) {
		if step != "locked" || walk.IsFile(abc) || !walk.IsFile(f) {
			return
		}
		if err := os.Remove(f); err != nil {
			t.Error(err)
		}
		if err := os.MkdirAll(filepath.Join(f, "x"), 0o755); err != nil {
			t.Error(err)
		}
	}
	r := stripIn(t, top, "")
	hooks.Modify = nil
	want := "format/testdata/checks/formulas.txt: left in place: removing it failed: remove " + f + ": directory not empty. Fix what failed, then run hypercase strip again.\n" +
		"format/testdata/hypercase.json: left in place, so that strip run again finds the findings files of its package that this run left.\n" +
		fmt.Sprintf("testdata/csv/prices.txtar: removed %d sections\ntestdata/sheet/twice.txtar: removed %d sections\n", sectionCount(pricesFull)-sectionCount(pricesHand), sectionCount(twiceFull)-sectionCount(twiceHand)) + nextLine
	if r.code != 1 || r.stderr != "" || r.stdout != want {
		t.Fatalf("strip exited %d, with stderr %q, and printed (-want +got):\n%s", r.code, r.stderr, diff.Lines([]byte(want), []byte(r.stdout)))
	}
	// tree reads files alone, so formulas.txt, a directory, is no file.
	sameTree(t, top, with(strippedModule(), map[string]string{
		"format/testdata/hypercase.json": files["format/testdata/hypercase.json"],
	}))

	// What failed is fixed, and strip run again.
	if err := os.RemoveAll(f); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f, []byte(files["format/testdata/checks/formulas.txt"]), 0o644); err != nil {
		t.Fatal(err)
	}
	stripOK(t, top, "", nextLine)
	sameTree(t, top, strippedModule())
}

// linkIn puts a symbolic link in place of rel, under top, moving what it
// replaces to shared/<name>.d. The link names target, relative to its
// directory, or, for "", what it replaces. unlinkIn moves rel back, and
// removes shared/ once it is empty.
func linkIn(t *testing.T, top, rel, target string) {
	t.Helper()
	p := filepath.Join(top, filepath.FromSlash(rel))
	moved := filepath.Join(top, "shared", filepath.Base(rel)+".d")
	if err := os.MkdirAll(filepath.Dir(moved), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(p, moved); err != nil {
		t.Fatal(err)
	}
	target = filepath.FromSlash(target)
	if target == "" {
		var err error
		if target, err = filepath.Rel(filepath.Dir(p), moved); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(target, p); err != nil {
		t.Skipf("making a symbolic link failed: %v", err)
	}
}

func unlinkIn(t *testing.T, top, rel string) {
	t.Helper()
	p := filepath.Join(top, filepath.FromSlash(rel))
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(top, "shared", filepath.Base(rel)+".d"), p); err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(top, "shared")
	entries, err := os.ReadDir(shared)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		if err := os.Remove(shared); err != nil {
			t.Fatal(err)
		}
	}
}

// TestStripLinks: nothing under the corpus root's testdata/ is a symbolic
// link, so one there fails strip, exit 1, before it removes anything, the
// registries included, whatever it names: a corpus's directory, an
// archive, testdata/checks/, nothing or itself. TestStripTestdataLink
// covers testdata/ itself.
func TestStripLinks(t *testing.T) {
	t.Parallel()
	const one = ". Nothing was removed.\nRemove it, or replace it with a copy of what it names, if it names anything, then run hypercase strip again.\n"
	type link struct{ rel, target string } // a target of "" names what the link replaces
	for _, tt := range []struct {
		links []link
		want  string
	}{
		{[]link{{"testdata/sheet", ""}}, "hypercase strip: testdata/ holds no symbolic links: testdata/sheet is one" + one},
		{[]link{{"testdata/sheet/twice.txtar", ""}}, "hypercase strip: testdata/ holds no symbolic links: testdata/sheet/twice.txtar is one" + one},
		{[]link{{"testdata/checks", ""}}, "hypercase strip: testdata/ holds no symbolic links: testdata/checks is one" + one},
		{[]link{{"testdata/sheet", "../shared/missing"}}, "hypercase strip: testdata/ holds no symbolic links: testdata/sheet is one" + one},
		{[]link{{"testdata/sheet", "sheet"}}, "hypercase strip: testdata/ holds no symbolic links: testdata/sheet is one" + one},
		{[]link{{"testdata/csv", ""}, {"testdata/sheet", ""}}, "hypercase strip: testdata/ holds no symbolic links: testdata/csv and testdata/sheet are links. Nothing was removed.\nRemove each, or replace it with a copy of what it names, if it names anything, then run hypercase strip again.\n"},
	} {
		files := stripModule()
		top := writeTree(t, files)
		for _, l := range tt.links {
			linkIn(t, top, l.rel, l.target)
		}
		stripFails(t, top, "", 1, nil, tt.want)
		for _, l := range tt.links {
			unlinkIn(t, top, l.rel)
		}
		sameTree(t, top, files)
	}
}

// TestStripTestdataLink: strip reads the registry that records the corpus
// root from -root's testdata/, or else from the current directory's, so a
// symbolic link in place of that testdata/ fails it, exit 1, before it
// reads anything through it, whatever the link names: a directory,
// nothing, itself or a file. That holds at the module root and in a
// subdirectory, with -root and without, and the line that says to run
// strip again names -root where it was given. Away from the module root
// with no -root, strip run again finds the corpus root only by a registry
// in testdata/, so the line also says to name the root with -root if
// there is none, and with -root it doesn't. Outside any module, a
// registry is no help, and the line doesn't name -root: strip run again
// once the link is gone says what is. A link in place of the corpus
// root's testdata/, which a
// registry read first names, fails it the same way, before it reads
// another registry.
func TestStripTestdataLink(t *testing.T) {
	t.Parallel()
	const more = " If the current directory then holds no registry, testdata/hypercase.json, name the corpus root with -root, relative to the current directory or by an absolute path."
	for _, at := range []struct {
		link, dir string
		args      []string
		again     string
	}{
		{"testdata", "", nil, "hypercase strip again."},
		{"testdata", "", []string{"-root", "."}, "hypercase strip -root . again."},
		{"step09/testdata", "step09", nil, "hypercase strip again." + more},
		{"step09/testdata", "step09", []string{"-root", "."}, "hypercase strip -root . again."},
		{"step09/testdata", "", []string{"-root", "step09"}, "hypercase strip -root step09 again."},
		{"a/b/c/testdata", "a/b/c", nil, "hypercase strip again." + more},
		{"testdata", "a/b/c", nil, "hypercase strip -root ../../.. again."},
	} {
		for _, names := range []string{"a directory", "nothing", "itself", "a file"} {
			t.Run(fmt.Sprintf("%s naming %s, from %s, by %s", at.link, names, filepath.ToSlash(filepath.Join(".", at.dir)), strings.Join(append([]string{"hypercase strip"}, at.args...), " ")), func(t *testing.T) {
				files := stripModule()
				top := writeTree(t, files)
				target := map[string]string{"a directory": "", "nothing": "missing", "itself": "testdata"}[names]
				if names == "a file" {
					rel, err := filepath.Rel(filepath.Dir(filepath.Join(top, filepath.FromSlash(at.link))), filepath.Join(top, "go.mod"))
					if err != nil {
						t.Fatal(err)
					}
					target = filepath.ToSlash(rel)
				}
				linkIn(t, top, at.link, target)
				stripFails(t, filepath.Join(top, filepath.FromSlash(at.dir)), "", 1, at.args, "hypercase strip: testdata is a symbolic link, and a corpus root's testdata/ is a directory that holds none. Nothing was removed.\nRemove it, or replace it with a copy of what it names, if it names anything, then run "+at.again+"\n")
				unlinkIn(t, top, at.link)
				sameTree(t, top, files)
			})
		}
	}
	t.Run("testdata naming a directory, in no module", func(t *testing.T) {
		files := map[string]string{"testdata/sheet/twice.txtar": twiceFull}
		top := writeTree(t, files)
		linkIn(t, top, "testdata", "")
		stripFails(t, top, "", 1, nil, "hypercase strip: testdata is a symbolic link, and a corpus root's testdata/ is a directory that holds none. Nothing was removed.\nRemove it, or replace it with a copy of what it names, if it names anything, then run hypercase strip again.\n")
		unlinkIn(t, top, "testdata")
		sameTree(t, top, files)
		stripFails(t, top, "", 1, nil, "hypercase strip: finding the module root failed: no go.mod in ", ".\nRun strip inside a Go module, or name the corpus root with -root, relative to the current directory or by an absolute path.\n")
	})
}

// TestStripRootPastLink: with -root, the testdata/ strip reads first is
// -root's, so a symbolic link in place of the current directory's, which
// isn't the corpus root, fails nothing before strip reads the registries.
// Strip reads it as it reads any package's testdata/, for a registry,
// through the link (#57): from the module root, by -root step09, with the
// module root's testdata/ a link to a directory or to nothing, strip
// strips step09 and leaves the link and its target as they were. With one
// to itself or a file, finding the registries fails, and strip exits 1
// having removed nothing. With one to a directory whose registry records
// step09, strip removes that registry and its package's findings file
// through the link, in its target.
func TestStripRootPastLink(t *testing.T) {
	t.Parallel()
	const failed = "hypercase strip: finding the module's registries failed: "
	stripped := fmt.Sprintf("testdata/sheet/twice.txtar: removed %d sections\n", sectionCount(twiceFull)-sectionCount(twiceHand))
	for _, tt := range []struct {
		names, target string
		registry      string // the module root's registry, if not the fixture's
		out           string // what strip prints, or, when it fails, what its stderr holds
		want          map[string]string
	}{
		{names: "a directory", out: stripped + nextLine},
		{names: "nothing", target: "missing", out: stripped + nextLine},
		{names: "itself", target: "testdata", out: failed},
		{names: "a file", target: "go.mod", out: failed},
		{
			names:    "a directory whose registry records step09",
			registry: stripRegistry("step09", sheetsVar),
			out:      stripped + nextLine,
			want:     map[string]string{"testdata/hypercase.json": "", "testdata/checks/located.txt": ""},
		},
	} {
		t.Run(tt.names, func(t *testing.T) {
			files := stripModule()
			if tt.registry != "" {
				files = with(files, map[string]string{"testdata/hypercase.json": tt.registry})
			}
			top := writeTree(t, files)
			linkIn(t, top, "testdata", tt.target)
			want := files
			if strings.HasPrefix(tt.out, failed) {
				stripFails(t, top, "", 1, []string{"-root", "step09"}, tt.out, ". Nothing was removed.\nFix what failed, then run hypercase strip -root step09 again.\n")
			} else {
				stripOK(t, top, "", tt.out, "-root", "step09")
				want = with(files, map[string]string{"step09/testdata/sheet/twice.txtar": twiceHand, "step09/testdata/hypercase.json": ""})
			}
			switch fi, err := os.Lstat(filepath.Join(top, "testdata")); {
			case err != nil:
				t.Errorf("testdata is gone after strip: %v", err)
			case fi.Mode()&os.ModeSymlink == 0:
				t.Errorf("testdata is no symbolic link after strip")
			}
			unlinkIn(t, top, "testdata")
			sameTree(t, top, with(want, tt.want))
		})
	}
}

// TestStripTestdataLinkRegistry: the module root's testdata/ a symbolic
// link to a directory whose registry records another corpus root, step09.
// Read through the link, the registry would send strip to step09, which
// would remove the registry and the findings file through the link, in its
// target. Strip refuses the link first, and leaves the target as it was.
// So it does when the registry can't be read, with -root ., and the line
// that says to run strip again keeps -root .: without it, strip run again
// once the link is replaced would read the registry and strip step09.
func TestStripTestdataLinkRegistry(t *testing.T) {
	t.Parallel()
	const want = "hypercase strip: testdata is a symbolic link, and a corpus root's testdata/ is a directory that holds none. Nothing was removed.\nRemove it, or replace it with a copy of what it names, if it names anything, then run "
	files := with(stripModule(), map[string]string{"testdata/hypercase.json": stripRegistry("step09", sheetsVar)})
	t.Run("readable", func(t *testing.T) {
		top := writeTree(t, files)
		linkIn(t, top, "testdata", "")
		stripFails(t, top, "", 1, nil, want+"hypercase strip again.\n")
		unlinkIn(t, top, "testdata")
		sameTree(t, top, files)
	})
	t.Run("unreadable", func(t *testing.T) {
		top := writeTree(t, files)
		linkIn(t, top, "testdata", "")
		reg := filepath.Join(top, "shared", "testdata.d", "hypercase.json")
		if err := os.Chmod(reg, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(reg, 0o644) })
		if _, err := os.ReadFile(reg); err == nil {
			t.Skip("a file with no read permission is still readable here")
		}
		stripFails(t, top, "", 1, []string{"-root", "."}, want+"hypercase strip -root . again.\n")
		if err := os.Chmod(reg, 0o644); err != nil {
			t.Fatal(err)
		}
		unlinkIn(t, top, "testdata")
		sameTree(t, top, files)
	})
}

// TestStripRegistryUnread: at the module root, a registry that can't be
// read, in a testdata/ that is no link, fails strip, which can't tell the
// corpus root it records, and doesn't take the module-root corpus.
func TestStripRegistryUnread(t *testing.T) {
	t.Parallel()
	files := with(stripModule(), map[string]string{"testdata/hypercase.json": "", "testdata/hypercase.json/x": "x\n"})
	top := writeTree(t, files)
	stripFails(t, top, "", 1, nil, "hypercase strip: reading testdata/hypercase.json failed: ")
	sameTree(t, top, files)
}

// TestStripUnreadTestdata: a directory under the corpus root's testdata/
// that can't be read fails strip's walk for archives, and strip exits 1
// having removed nothing, as reading the corpus root's testdata/ fails.
func TestStripUnreadTestdata(t *testing.T) {
	t.Parallel()
	files := with(stripModule(), map[string]string{"testdata/sheet/old/y.txtar": "A1 is 3.\n"})
	top := writeTree(t, files)
	locked := filepath.Join(top, "testdata", "sheet", "old")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	if _, err := os.ReadDir(locked); err == nil {
		t.Skip("a directory with no permissions reads here, as it does for root, so no walk fails on it")
	}
	stripFails(t, top, "", 1, nil, "hypercase strip: reading testdata failed: ", "permission denied. Nothing was removed.\nFix what failed, then run hypercase strip again.\n")
	if err := os.Chmod(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	sameTree(t, top, files)
}

// TestStripUnreadFindings: a package whose testdata/checks/ can't be read,
// a file in its place, keeps its registry, so that strip run again, once
// it is a directory, removes the findings files in it, and then the
// registry.
func TestStripUnreadFindings(t *testing.T) {
	t.Parallel()
	files := with(stripModule(), map[string]string{
		"format/testdata/checks/formulas.txt": "",
		"format/testdata/checks/notes.md":     "",
		"format/testdata/checks":              "not a directory\n",
	})
	top := writeTree(t, files)
	r := stripIn(t, top, "")
	const line = "format/testdata/checks: left as it was: reading it failed: "
	const fix = ". Fix what failed, then run hypercase strip again.\nformat/testdata/hypercase.json: left in place, so that strip run again finds the findings files of its package that this run left.\n"
	if r.code != 1 || r.stderr != "" || !strings.Contains(r.stdout, line) || !strings.Contains(r.stdout, fix) || !strings.HasSuffix(r.stdout, nextLine) {
		t.Fatalf("strip exited %d, with stderr %q, and printed:\n%s", r.code, r.stderr, r.stdout)
	}
	sameTree(t, top, with(strippedModule(), map[string]string{
		"format/testdata/hypercase.json":  files["format/testdata/hypercase.json"],
		"format/testdata/checks":          files["format/testdata/checks"],
		"format/testdata/checks/notes.md": "",
	}))

	checks := filepath.Join(top, "format", "testdata", "checks")
	if err := os.Remove(checks); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(checks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checks, "formulas.txt"), []byte("sheet/twice A1 =B1*2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stripOK(t, top, "", nextLine)
	sameTree(t, top, with(strippedModule(), map[string]string{"format/testdata/checks/notes.md": ""}))
}

// TestStripToys: on a copy of the walkthrough's toy module, stripping each
// of its corpus roots, then running the update over the whole module until
// it changes nothing, gives back every file as it was. Stale files a person
// left are gone: a removed suite's sections, in an archive of each root,
// and a findings file no check writes. The toy's frozen cells keep their
// committed files.
func TestStripToys(t *testing.T) {
	t.Parallel()
	dir, goCmd := toyCopy(t)
	want := tree(t, dir)
	roots := recordedRoots(t, dir, want)

	stale := map[string]string{"step06/testdata/checks/gone.txt": "sheet/twice gone\n"}
	for _, r := range roots {
		p := r + "/testdata/sheet/twice.txtar"
		stale[p] = want[p] + "-- out/gone/outcome --\naccepted\n"
	}
	for p, data := range stale {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stripToy(t, dir, roots)
	stripped := tree(t, dir)
	for p := range want {
		if strings.HasSuffix(p, "/"+registry.Name) {
			if _, ok := stripped[p]; ok {
				t.Errorf("strip left the registry %s", p)
			}
		}
	}
	if got, w := stripped["step04/testdata/sheet/negative.txtar"], want["step04/testdata/sheet/negative.txtar"]; got != w {
		t.Errorf("strip changed step04's negative.txtar, whose only update-written cell is frozen (-want +got):\n%s", diff.Lines([]byte(w), []byte(got)))
	}

	for pass := 1; ; pass++ {
		before := tree(t, dir)
		goCmd([]string{sheetsVar + "=1"}, "test", "-count=1", "./...")
		if maps.Equal(tree(t, dir), before) {
			break
		}
		if pass == 5 {
			t.Fatalf("the update still changes files after %d passes", pass)
		}
	}
	sameTree(t, dir, want)
}

// TestStripToysReportFirst: on a copy of the walkthrough's toy module, with
// each of its corpus roots stripped and its committed reports removed, the
// report tests, run alone in the update, write back every report as it was.
// A report test copies its corpus root as it finds it, and in the update of
// the whole module, the other packages over the root write their sections
// back while it runs. Run first, it finds none of them written (#74).
func TestStripToysReportFirst(t *testing.T) {
	t.Parallel()
	dir, goCmd := toyCopy(t)
	want := tree(t, dir)
	stripToy(t, dir, recordedRoots(t, dir, want))
	var reports []string
	for _, p := range slices.Sorted(maps.Keys(want)) {
		if strings.HasSuffix(p, "/testdata/report.txt") || strings.HasSuffix(p, "/testdata/report.html") {
			reports = append(reports, p)
			if err := os.Remove(filepath.Join(dir, filepath.FromSlash(p))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(reports) == 0 {
		t.Fatal("the toy commits no report")
	}

	goCmd([]string{sheetsVar + "=1"}, "test", "-count=1", "-run=^TestReport$", "./...")
	got := tree(t, dir)
	for _, p := range reports {
		g, ok := got[p]
		switch {
		case !ok:
			t.Errorf("the report tests didn't write %s", p)
		case g != want[p]:
			t.Errorf("%s differs (-want +got):\n%s", p, diff.Lines([]byte(want[p]), []byte(g)))
		}
	}
}

// toyCopy copies the walkthrough's toy module, and returns the toy's
// directory in the copy and a function that runs go there, which fails the
// test unless go exits 0. Each go command runs off any workspace, with no
// GOFLAGS, and with neither update variable unless extra sets one.
//
// The toy's go.mod replaces hypercase by ../.., hypercase's own module,
// where step 9's report test also builds the hypercase command. So the
// toy is copied to walkthrough/sheets in a copy of that module, which
// leaves out .git, and, outside the toy, the testdata/ directories: no
// build reads them, and the module's own tests can be writing them while
// this one copies.
func toyCopy(t *testing.T) (dir string, goCmd func(extra []string, args ...string)) {
	t.Helper()
	hc, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	toy := filepath.Join("walkthrough", "sheets")
	top := t.TempDir()
	err = filepath.WalkDir(hc, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(hc, p)
		inToy := rel == toy || strings.HasPrefix(rel, toy+string(filepath.Separator))
		switch {
		case d.IsDir() && (d.Name() == ".git" || d.Name() == "testdata" && !inToy):
			return filepath.SkipDir
		case d.IsDir() || d.Name() == ".git" || d.Name() == archive.LockFile:
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(top, filepath.Dir(rel)), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(top, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	dir = filepath.Join(top, toy)
	var env []string
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); k != "GOFLAGS" && k != "GOWORK" && k != sheetsVar && k != goldenVar && k != "PWD" {
			env = append(env, kv)
		}
	}
	env = append(env, "GOWORK=off")
	goCmd = func(extra []string, args ...string) {
		t.Helper()
		cmd := exec.Command("go", args...)
		cmd.Dir, cmd.Env = dir, append(slices.Clone(env), extra...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	return dir, goCmd
}

// recordedRoots returns the corpus roots that the toy's registries record,
// sorted, reading each from files, the toy's files at dir by slash path. It
// fails the test unless there is more than one.
func recordedRoots(t *testing.T, dir string, files map[string]string) []string {
	t.Helper()
	found, err := registry.Find(dir)
	if err != nil {
		t.Fatal(err)
	}
	var roots []string
	for _, f := range found {
		h, err := registry.Peek([]byte(files[root.Rel(dir, f.Path)]))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(roots, h.CorpusRoot) {
			roots = append(roots, h.CorpusRoot)
		}
	}
	slices.Sort(roots)
	if len(roots) < 2 {
		t.Fatalf("the toy's registries record the corpus roots %q, and it has more", roots)
	}
	return roots
}

// stripToy runs hypercase strip -root on each of roots, in the toy at dir,
// and fails the test unless each exits 0 and says to run the toy's update
// next.
func stripToy(t *testing.T, dir string, roots []string) {
	t.Helper()
	for _, r := range roots {
		res := stripIn(t, dir, "", "-root", r)
		if res.code != 0 || res.stderr != "" || !strings.HasSuffix(res.stdout, "Next, run "+sheetsVar+"=1 go test ./... at the module root, and again until it changes nothing.\n") {
			t.Fatalf("strip -root %s exited %d, printing:\n%s%s", r, res.code, res.stdout, res.stderr)
		}
	}
}
