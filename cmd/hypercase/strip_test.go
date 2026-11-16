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
	if r := stripIn(t, top, "", "-h"); r.code != 0 || r.stdout != "" || !strings.Contains(r.stderr, usage) {
		t.Errorf("strip -h: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	sameTree(t, top, files)
}

// TestStripFrozen: strip leaves a frozen cell's sections in place, a bytes
// lift's via/ among them, and lists the cell; -frozen strips them too. The
// freeze is the cell's: a transform's inputs and the cells the line
// doesn't mark are stripped. Every gaps section keeps its bytes.
func TestStripFrozen(t *testing.T) {
	t.Parallel()
	const twiceGaps = "evaluate Evaluates A1 to 6, and B1 to 3. Waits on sheets#4.\ntransform/looped resolve Names the cycle's cells. Waits on sheets#5.\n"
	const cycleGaps = "resolve Says which cell to break the cycle at. Waits on sheets#6.\n"
	const pricesGaps = "import Keeps the price's currency. Waits on sheets#7.\n"
	twice := strings.Replace(twiceFull, "-- out/evaluate/outcome --", "-- gaps --\n"+twiceGaps+"-- out/evaluate/outcome --", 1)
	twice = strings.Replace(twice, "-- variant/cycle/in/sheet/B1 --", "-- variant/cycle/gaps --\n"+cycleGaps+"-- variant/cycle/in/sheet/B1 --", 1)
	prices := strings.Replace(pricesFull, "-- via/import/sheet/A1 --", "-- gaps --\n"+pricesGaps+"-- via/import/sheet/A1 --", 1)
	files := with(stripModule(), map[string]string{"testdata/sheet/twice.txtar": twice, "testdata/csv/prices.txtar": prices})
	top := writeTree(t, files)
	stripOK(t, top, "", `testdata/csv/prices.txtar: removed 11 sections
testdata/csv/prices.txtar: left the cell import, frozen by its known gap, gaps line 1; -frozen strips it too
testdata/sheet/twice.txtar: removed 10 sections
testdata/sheet/twice.txtar: left the cell evaluate, frozen by its known gap, gaps line 1; -frozen strips it too
testdata/sheet/twice.txtar: left the cell variant/cycle/resolve, frozen by its known gap, variant/cycle/gaps line 1; -frozen strips it too
testdata/sheet/twice.txtar: left the cell transform/looped/resolve, frozen by its known gap, gaps line 2; -frozen strips it too
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

// TestStripProblems: an archive with problems of form, a sheet with a
// section twice or with variant/cycle/notes, is left byte for byte as it
// was and listed with the reason, and so is an archive no corpus holds,
// testdata/x.txtar and testdata/sheet/old/y.txtar, each with an out/
// section. The other archives are stripped, and strip exits 1.
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
	want := fmt.Sprintf(`testdata/csv/prices.txtar: removed %d sections
testdata/sheet/notes.txtar: left as it was: the archive has problems of form, so no update writes it: variant/cycle/notes: notes is not one of a hand-written variant's sections: about, in, delete, properties, gaps, via and out. Fix the archive by hand, and strip it again.
testdata/sheet/old/y.txtar: left as it was: an archive belongs directly in a corpus's directory, testdata/<corpus>/, and this one is below testdata/sheet/, so it is no case. Move it into a corpus's directory, or delete it.
testdata/sheet/twice.txtar: removed %d sections
testdata/sheet/twin.txtar: left as it was: the archive has problems of form, so no update writes it: out/evaluate/outcome: the section appears twice. Fix the archive by hand, and strip it again.
testdata/x.txtar: left as it was: an archive belongs in a corpus's directory, testdata/<corpus>/, and this one is directly in testdata/. Move it into one.
`, sectionCount(pricesFull)-sectionCount(pricesHand), sectionCount(twiceFull)-sectionCount(twiceHand)) + nextLine
	if r.code != 1 || r.stderr != "" || r.stdout != want {
		t.Errorf("strip exited %d, with stderr %q, and printed (-want +got):\n%s", r.code, r.stderr, diff.Lines([]byte(want), []byte(r.stdout)))
	}
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
			"give every package over the corpus root the same decl.Project, run its update, and run strip again.",
		}},
		{"unreadable", map[string]string{"a/b/c/testdata/hypercase.json": "{\n"}, []string{
			"hypercase strip: a/b/c/testdata/hypercase.json can't be read as a registry: ",
			"Fix the file or delete it, and run strip again.",
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
// in, by its registry, or else takes the module root's: run from a/b/c,
// several levels down, it strips the module-root corpus, and from step09,
// the corpus root step09.
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
		{"a/b", nil, moduleOut, module},
		{"step09", []string{"-root", ".."}, moduleOut, module},
	} {
		top := writeTree(t, files)
		stripOK(t, top, tt.dir, tt.out, tt.args...)
		sameTree(t, top, tt.want)
	}

	top := writeTree(t, files)
	stripFails(t, top, "", 1, []string{"-root", "nope"}, "hypercase strip: -root nope names no directory.\nName the corpus root relative to the current directory.\n")
	stripFails(t, top, "", 1, []string{"-root", "a"}, "hypercase strip: "+filepath.Join(top, "a")+" holds no testdata/ directory, so it is no corpus root.\n")
	sameTree(t, top, files)
}

// TestStripFailedWrite: a write made to fail once its temp file is
// written leaves that archive as it was, with no temp file, and strip goes
// on to strip the next archive, and exits 1. It sets a hook, so it doesn't
// run in parallel.
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
	want := "testdata/csv/prices.txtar: left as it was: stripping it failed, and the archive is as it was: disk full. Fix what failed, and strip it again.\n" +
		fmt.Sprintf("testdata/sheet/twice.txtar: removed %d sections\n", sectionCount(twiceFull)-sectionCount(twiceHand)) + nextLine
	if r.code != 1 || r.stderr != "" || r.stdout != want {
		t.Errorf("strip exited %d, with stderr %q, and printed (-want +got):\n%s", r.code, r.stderr, diff.Lines([]byte(want), []byte(r.stdout)))
	}
	// tree reads every file, so a temp file left behind is one more.
	sameTree(t, top, with(files, map[string]string{
		"testdata/sheet/twice.txtar":          twiceHand,
		"testdata/hypercase.json":             "",
		"testdata/checks/located.txt":         "",
		"format/testdata/hypercase.json":      "",
		"format/testdata/checks/formulas.txt": "",
		"a/b/c/testdata/hypercase.json":       "",
	}))
}

// TestStripFailedRemoval: a registry whose removal fails is left in place
// and listed, the others are removed, and strip exits 1. The removal is
// made to fail by putting a directory with a file in it where the
// registry was, once strip holds the lock to remove it, so the test sets a
// hook and doesn't run in parallel.
func TestStripFailedRemoval(t *testing.T) {
	files := stripModule()
	top := writeTree(t, files)
	reg := filepath.Join(top, "format", "testdata", "hypercase.json")
	hooks.Modify = func(step string) {
		if step != "locked" || !isFile(reg) {
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
	line := "format/testdata/hypercase.json: left in place: removing it failed: remove " + reg + ": directory not empty. Fix what failed, and run strip again.\n"
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

// TestStripToys: on a copy of the walkthrough's toy module, stripping each
// of its corpus roots, then running the update over the whole module until
// it changes nothing, gives back every file as it was. Stale files a person
// left are gone: a removed suite's sections, in an archive of each root,
// and a findings file no check writes. The toy's frozen cells keep their
// committed files.
func TestStripToys(t *testing.T) {
	t.Parallel()
	hc, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(hc, "walkthrough", "sheets")
	dir := t.TempDir()
	err = filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == archive.LockFile {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	var env []string
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); k != "GOFLAGS" && k != "GOWORK" && k != sheetsVar && k != goldenVar && k != "PWD" {
			env = append(env, kv)
		}
	}
	env = append(env, "GOWORK=off")
	goCmd := func(extra []string, args ...string) {
		t.Helper()
		cmd := exec.Command("go", args...)
		cmd.Dir, cmd.Env = dir, append(slices.Clone(env), extra...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	goCmd(nil, "mod", "edit", "-replace=github.com/sdboyer/hypercase="+hc)
	want := tree(t, dir)

	// The toy's corpus roots, as its registries record them.
	found, err := registry.Find(dir)
	if err != nil {
		t.Fatal(err)
	}
	var toyRoots []string
	for _, f := range found {
		h, err := registry.Peek([]byte(want[root.Rel(dir, f.Path)]))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(toyRoots, h.CorpusRoot) {
			toyRoots = append(toyRoots, h.CorpusRoot)
		}
	}
	slices.Sort(toyRoots)
	if len(toyRoots) < 2 {
		t.Fatalf("the toy's registries record the corpus roots %q, and it has more", toyRoots)
	}

	stale := map[string]string{"step06/testdata/checks/gone.txt": "sheet/twice gone\n"}
	for _, r := range toyRoots {
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
	for _, r := range toyRoots {
		res := stripIn(t, dir, "", "-root", r)
		if res.code != 0 || res.stderr != "" || !strings.HasSuffix(res.stdout, "Next, run "+sheetsVar+"=1 go test ./... at the module root, and again until it changes nothing.\n") {
			t.Fatalf("strip -root %s exited %d, printing:\n%s%s", r, res.code, res.stdout, res.stderr)
		}
	}
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
