package strip

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
)

// tricky is an archive built to trick a strip that reads names loosely: a
// tree named out, a path through a directory named via, hand-written
// variants named out, transform and via, and a transform named in, beside
// ordinary update-written sections at every level.
const tricky = `A1 is twice B1, with names that read like an update's.
-- in/out/A1 --
=B1*2
-- in/sheet/via/B1 --
3
-- properties --
circular
-- out/evaluate/outcome --
accepted
-- out/evaluate/values.txt --
A1 6
-- variant/out/about --
A variant named out, whose A1 is three times B1.
-- variant/out/in/out/A1 --
=B1*3
-- variant/out/out/evaluate/outcome --
accepted
-- variant/transform/about --
A variant named transform, with no A1.
-- variant/transform/delete --
out/A1
-- variant/transform/out/evaluate/outcome --
accepted
-- variant/via/about --
A variant named via, whose B1 is four.
-- variant/via/properties --
circular
-- variant/via/in/sheet/via/B1 --
4
-- variant/via/via/import/sheet/A1 --
4
-- variant/via/out/import/outcome --
accepted
-- transform/in/in/sheet/A1 --
=A1
-- transform/in/delete --
out/A1
-- transform/in/via/import/sheet/A1 --
=A1
-- transform/in/out/evaluate/outcome --
blocked resolve
`

// trickyUpdateWritten are tricky's update-written sections, listed by
// hand, so the test doesn't read them as strip does.
var trickyUpdateWritten = []string{
	"out/evaluate/outcome",
	"out/evaluate/values.txt",
	"variant/out/out/evaluate/outcome",
	"variant/transform/out/evaluate/outcome",
	"variant/via/via/import/sheet/A1",
	"variant/via/out/import/outcome",
	"transform/in/in/sheet/A1",
	"transform/in/delete",
	"transform/in/via/import/sheet/A1",
	"transform/in/out/evaluate/outcome",
}

// stripped reports how after differs from before less exactly the named
// sections: every other section byte for byte, in its order, and the
// comment.
func stripped(before, after *archive.Archive, removed []string) error {
	if !bytes.Equal(before.Comment, after.Comment) {
		return fmt.Errorf("the comment changed: %q, then %q", before.Comment, after.Comment)
	}
	var want []archive.Section
	for _, s := range before.Sections {
		if !slices.Contains(removed, s.Name) {
			want = append(want, s)
		}
	}
	if !slices.EqualFunc(want, after.Sections, func(a, b archive.Section) bool { return a.Name == b.Name && bytes.Equal(a.Data, b.Data) }) {
		return fmt.Errorf("the sections are %s, not %s", names(after.Sections), names(want))
	}
	return nil
}

func names(ss []archive.Section) string {
	var ns []string
	for _, s := range ss {
		ns = append(ns, s.Name)
	}
	return "[" + strings.Join(ns, " ") + "]"
}

// corpusRoot writes the archives, by path under testdata/, in a temp dir,
// and returns it.
func corpusRoot(t *testing.T, archives map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, data := range archives {
		full := filepath.Join(root, "testdata", filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func read(t *testing.T, p string) *archive.Archive {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return archive.Parse(data)
}

// TestRecognition: strip removes exactly the update-written sections of
// an archive built to trick it, by name alone, and leaves every other
// section byte for byte, in its order. A strip that reads /out/ anywhere
// in a name as an output fails the same comparison.
func TestRecognition(t *testing.T) {
	t.Parallel()
	before := archive.Parse([]byte(tricky))
	if ps := before.Check(); len(ps) > 0 {
		t.Fatalf("the tricky archive has problems of form: %v", ps)
	}
	root := corpusRoot(t, map[string]string{"sheet/tricky.txtar": tricky})
	p := filepath.Join(root, "testdata", "sheet", "tricky.txtar")
	r, err := Archive(root, p, Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.Removed, trickyUpdateWritten) {
		t.Errorf("Archive removed %q, not %q", r.Removed, trickyUpdateWritten)
	}
	if len(r.Frozen) > 0 {
		t.Errorf("Archive left frozen cells, and no gaps line marks one: %+v", r.Frozen)
	}
	if err := stripped(before, read(t, p), trickyUpdateWritten); err != nil {
		t.Error(err)
	}

	// The same comparison fails a strip that matches out/, via/ and
	// transform/ anywhere in a name.
	var loose []string
	for _, s := range before.Sections {
		n := "/" + s.Name
		if strings.Contains(n, "/out/") || strings.Contains(n, "/via/") || strings.Contains(n, "/transform/") {
			loose = append(loose, s.Name)
		}
	}
	if err := stripped(before, without(before, loose), trickyUpdateWritten); err == nil {
		t.Errorf("a strip that matches /out/ anywhere in a name passes the comparison, removing %q", loose)
	}
}

// TestFinalNewline: an archive whose last section has no final newline
// reads as though it had one, as txtar reads every archive. Stripping it
// leaves that section's bytes as they read, and writes the archive in
// txtar's form, which has the newline, as an update writes it.
func TestFinalNewline(t *testing.T) {
	t.Parallel()
	const unended = "A1 is 3.\n-- out/evaluate/outcome --\naccepted\n-- in/sheet/A1 --\n3"
	root := corpusRoot(t, map[string]string{"sheet/unended.txtar": unended})
	p := filepath.Join(root, "testdata", "sheet", "unended.txtar")
	if data, _ := archive.Parse([]byte(unended)).Section("in/sheet/A1"); string(data) != "3\n" {
		t.Fatalf("in/sheet/A1 reads as %q, not %q", data, "3\n")
	}
	if _, err := Archive(root, p, Selection{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(p); string(got) != "A1 is 3.\n-- in/sheet/A1 --\n3\n" {
		t.Errorf("strip left:\n%q", got)
	}
}

// TestPlanWritesNothing: Plan says what Archive removes, and an archive
// with nothing to remove gives an empty Result.
func TestPlanWritesNothing(t *testing.T) {
	t.Parallel()
	a := archive.Parse([]byte(tricky))
	if r := Plan(a, Selection{}); !slices.Equal(r.Removed, trickyUpdateWritten) {
		t.Errorf("Plan removes %q, not %q", r.Removed, trickyUpdateWritten)
	}
	if got := a.Format(); string(got) != tricky {
		t.Errorf("Plan changed the archive:\n%s", got)
	}
	hand := without(a, trickyUpdateWritten)
	if r := Plan(hand, Selection{}); len(r.Removed) > 0 || len(r.Frozen) > 0 {
		t.Errorf("Plan of an archive with only hand-written sections = %+v", r)
	}
}

// TestRemove: Remove removes the named update-written sections and leaves
// every other byte as it was. It refuses a hand-written name, one the
// archive lacks, and one named twice, naming each, and writes nothing.
func TestRemove(t *testing.T) {
	t.Parallel()
	root := corpusRoot(t, map[string]string{"sheet/tricky.txtar": tricky})
	p := filepath.Join(root, "testdata", "sheet", "tricky.txtar")
	names := []string{"variant/out/out/evaluate/outcome", "transform/in/in/sheet/A1"}
	if err := Remove(root, p, names); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.NewReplacer(
		"-- variant/out/out/evaluate/outcome --\naccepted\n", "",
		"-- transform/in/in/sheet/A1 --\n=A1\n", "",
	).Replace(tricky)
	if string(data) != want {
		t.Errorf("Remove left:\n%s\nwant:\n%s", data, want)
	}

	for _, tt := range []struct {
		names []string
		want  string
	}{
		{[]string{"variant/out/in/out/A1"}, "variant/out/in/out/A1 is hand-written, and only update-written sections are removed"},
		{[]string{"in/sheet/via/B1", "properties"}, "in/sheet/via/B1 is hand-written, and only update-written sections are removed; properties is hand-written"},
		{[]string{"out/evaluate/stdout.txt"}, "the archive has no section out/evaluate/stdout.txt"},
		{[]string{"out/evaluate/outcome", "out/evaluate/outcome"}, "out/evaluate/outcome is named twice"},
		{[]string{"out/evaluate/outcome", "variant/out/about", "transform/in/out/gone"}, "variant/out/about is hand-written, and only update-written sections are removed; the archive has no section transform/in/out/gone. Nothing was removed"},
	} {
		err := Remove(root, p, tt.names)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("Remove(%q) = %v, want it refused with %q", tt.names, err, tt.want)
		}
		if got, _ := os.ReadFile(p); !bytes.Equal(got, data) {
			t.Errorf("a refused Remove(%q) wrote the archive:\n%s", tt.names, got)
		}
	}
}

// TestRefusals: Archive and Remove refuse an archive with problems of
// form, and a path that is no archive of the corpus root, naming why, and
// write nothing. A selection that names no column or transform by the
// naming rule is refused too.
func TestRefusals(t *testing.T) {
	t.Parallel()
	const notes = "A1 is twice B1.\n-- in/sheet/A1 --\n=B1*2\n-- out/evaluate/outcome --\naccepted\n-- variant/cycle/about --\nA cycle.\n-- variant/cycle/notes --\nA note.\n"
	const twice = "A1 is twice B1.\n-- in/sheet/A1 --\n=B1*2\n-- out/evaluate/outcome --\naccepted\n-- out/evaluate/outcome --\naccepted\n"
	archives := map[string]string{
		"sheet/notes.txtar":  notes,
		"sheet/twice.txtar":  twice,
		"x.txtar":            notes,
		"sheet/old/y.txtar":  notes,
		"checks/z.txtar":     notes,
		"Sheet/a.txtar":      notes,
		"sheet/Bad.txtar":    notes,
		"sheet/notes.golden": notes,
	}
	root := corpusRoot(t, archives)
	for _, tt := range []struct{ path, want string }{
		{"sheet/notes.txtar", "the archive has problems of form, so no update writes it: variant/cycle/notes: notes is not one of a hand-written variant's sections: about, in, delete, properties, gaps, via and out. Fix the archive by hand"},
		{"sheet/twice.txtar", "the archive has problems of form, so no update writes it: out/evaluate/outcome: the section appears twice."},
		{"x.txtar", "an archive belongs in a corpus's directory, testdata/<corpus>/, and this one is directly in testdata/. Move it into one, or delete it"},
		{"sheet/old/y.txtar", "an archive belongs directly in a corpus's directory, testdata/<corpus>/, and this one is below testdata/sheet/, so it is no case."},
		{"checks/z.txtar", "testdata/checks/ holds corpus checks' findings, so no corpus has the directory."},
		{"Sheet/a.txtar", `the directory's name, "Sheet", breaks the naming rule`},
		{"sheet/Bad.txtar", `the archive's name, "Bad", breaks the naming rule`},
		{"sheet/notes.golden", "an archive's file name ends in .txtar, and notes.golden doesn't."},
	} {
		p := filepath.Join(root, "testdata", filepath.FromSlash(tt.path))
		if _, err := Archive(root, p, Selection{}); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("Archive(%s) = %v, want it refused with %q", tt.path, err, tt.want)
		}
		if err := Remove(root, p, []string{"out/evaluate/outcome"}); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("Remove(%s) = %v, want it refused with %q", tt.path, err, tt.want)
		}
		if got, _ := os.ReadFile(p); string(got) != archives[tt.path] {
			t.Errorf("a refused strip of %s wrote it:\n%s", tt.path, got)
		}
	}
	outside := filepath.Join(t.TempDir(), "testdata", "sheet", "x.txtar")
	if _, err := Archive(root, outside, Selection{}); err == nil || !strings.Contains(err.Error(), "it is not under the corpus root's testdata/") {
		t.Errorf("Archive of a path outside the corpus root = %v", err)
	}
	p := filepath.Join(root, "testdata", "sheet", "notes.txtar")
	for _, tt := range []struct {
		sel  Selection
		want string
	}{
		{Selection{Column: "out/evaluate"}, `the column "out/evaluate" is no column's name, which is lowercase ASCII letters`},
		{Selection{Transform: "Looped"}, `the transform "Looped" is no transform's name, which is lowercase ASCII letters`},
	} {
		if _, err := Archive(root, p, tt.sel); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("Archive with %+v = %v, want it refused with %q", tt.sel, err, tt.want)
		}
	}
}

// TestFrozenVia: a via/<column>/… section is part of a frozen cell when
// BytesLifts says the column is a bytes lift, or doesn't know the column,
// and of no cell when BytesLifts says it is no bytes lift: then the
// section is removed, and the cell's out/<column>/… is left.
func TestFrozenVia(t *testing.T) {
	t.Parallel()
	a := archive.Parse([]byte(`A CSV that import makes a sheet of, with a via/ for resolve too.
-- in/csv/prices.csv --
A1,3
-- gaps --
import Keeps the price's currency. Waits on sheets#7.
resolve Names the cycle's cells. Waits on sheets#5.
-- via/import/sheet/A1 --
3
-- via/resolve/sheet/A1 --
3
-- out/import/outcome --
accepted
-- out/resolve/outcome --
accepted
`))
	if ps := a.Check(); len(ps) > 0 {
		t.Fatalf("the archive has problems of form: %v", ps)
	}
	importCell := []string{"via/import/sheet/A1", "out/import/outcome"}
	for _, tt := range []struct {
		name    string
		lifts   map[string]bool
		removed []string
		resolve []string // resolve's frozen cell's sections
	}{
		{"known", map[string]bool{"import": true, "resolve": false}, []string{"via/resolve/sheet/A1"}, []string{"out/resolve/outcome"}},
		{"unknown", nil, nil, []string{"via/resolve/sheet/A1", "out/resolve/outcome"}},
		{"import alone", map[string]bool{"import": true}, nil, []string{"via/resolve/sheet/A1", "out/resolve/outcome"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, sel := range []Selection{{BytesLifts: tt.lifts}, {Column: "resolve", BytesLifts: tt.lifts}} {
				r := Plan(a, sel)
				if !slices.Equal(r.Removed, tt.removed) {
					t.Errorf("Plan(%+v) removes %q, not %q", sel, r.Removed, tt.removed)
				}
				frozen := map[string][]string{}
				for _, f := range r.Frozen {
					frozen[f.Column] = f.Sections
				}
				if !slices.Equal(frozen["resolve"], tt.resolve) {
					t.Errorf("Plan(%+v) leaves resolve's frozen cell %q, not %q", sel, frozen["resolve"], tt.resolve)
				}
				if want := importCell; sel.Column != "" {
					if _, ok := frozen["import"]; ok {
						t.Errorf("Plan(%+v) leaves import's cell, which the selection doesn't select", sel)
					}
				} else if !slices.Equal(frozen["import"], want) {
					t.Errorf("Plan(%+v) leaves import's frozen cell %q, not %q", sel, frozen["import"], want)
				}
			}
		})
	}
}

// TestFrozenInputs: a cell's inputs file is one of its update-written
// sections: a frozen cell keeps it with its other sections, and lists it
// among them, and a selection that strips frozen cells removes it, as one
// of a cell that isn't frozen is removed.
func TestFrozenInputs(t *testing.T) {
	t.Parallel()
	a := archive.Parse([]byte(`A sheet whose evaluate cell a gaps line freezes.
-- in/sheet/A1 --
3
-- gaps --
evaluate Prints A1 wrongly. Waits on sheets#3.
-- out/evaluate/inputs --
values.txt
  in/sheet/A1
-- out/evaluate/outcome --
accepted
-- out/evaluate/values.txt --
A1 3
-- out/list/files.txt --
1 file
-- out/list/inputs --
files.txt
  all inputs
-- out/list/outcome --
accepted
`))
	if ps := a.Check(); len(ps) > 0 {
		t.Fatalf("the archive has problems of form: %v", ps)
	}
	evaluate := []string{"out/evaluate/inputs", "out/evaluate/outcome", "out/evaluate/values.txt"}
	list := []string{"out/list/files.txt", "out/list/inputs", "out/list/outcome"}
	for _, tt := range []struct {
		sel     Selection
		removed []string
		frozen  []string
	}{
		{Selection{}, list, evaluate},
		{Selection{Column: "evaluate"}, nil, evaluate},
		{Selection{Column: "evaluate", Frozen: true}, evaluate, nil},
		{Selection{Frozen: true}, slices.Concat(evaluate, list), nil},
	} {
		r := Plan(a, tt.sel)
		if !slices.Equal(r.Removed, tt.removed) {
			t.Errorf("Plan(%+v) removes %q, not %q", tt.sel, r.Removed, tt.removed)
		}
		var frozen []string
		for _, f := range r.Frozen {
			frozen = append(frozen, f.Sections...)
		}
		if !slices.Equal(frozen, tt.frozen) {
			t.Errorf("Plan(%+v) leaves the frozen sections %q, not %q", tt.sel, frozen, tt.frozen)
		}
	}
}
