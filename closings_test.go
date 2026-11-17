package hypercase_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/archive"
)

// TestClosings: every closing a normal run gives says what the update then
// does. Each grid takes a fixture whose archive one update has written,
// and runs scenarios on it: an edit of what a lift or the case makes now
// (the code's side, F) crossed with an edit of what the archive commits
// for a bytes lift, a transform's variant or a cell (the committed side,
// C). Each scenario runs a normal run, then an update, of each of the
// grid's packages, as go test ./... runs them, in children that record
// each cell's fate and files, in both runs, and each note's closing, in
// the normal run. Then every note's closing must hold:
//
//   - A default closing, the note's own, only where the update does with
//     the cell what the normal run does: its fate, its files and its
//     product are the normal run's, and it writes them or doesn't alike.
//   - A staged closing, which says what the update does, only with the
//     fate the update then has, in words that name it: it runs the cell,
//     on another value than the normal run's or not, blocks it by the lift
//     that fails, writes nothing for it past a lift whose cell it writes
//     nothing for, skips it for the probe's reason, or fails it with the
//     probe's fault.
//   - A closing that says the package can't tell only in a package that
//     doesn't own a bytes lift on the cell's path, where every closing of
//     such a cell says so.
//   - What a closing says the update writes or leaves, the update writes or
//     leaves: the outcome it names, a file it removes or leaves in place,
//     nothing for the cell.
//
// What each update record says it wrote is checked against the archive
// after the update, and what it didn't write is unchanged. The grids
// together must reach every pair of a note's kind and the form of its
// closing that a cell can give (closingPairs).
func TestClosings(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	reached := map[closingPair]string{} // each pair, and the first scenario that reached it
	count := map[closingPair]int{}      // how many notes give each pair
	total, ran := 0, 0
	t.Run("grid", func(t *testing.T) {
		for _, g := range closingGrids() {
			total += len(g.scenarios)
			t.Run(g.name, func(t *testing.T) {
				t.Parallel()
				clean := g.clean(t)
				for _, sc := range g.scenarios {
					t.Run(sc.name, func(t *testing.T) {
						t.Parallel()
						pairs := g.run(t, clean, sc)
						mu.Lock()
						defer mu.Unlock()
						ran++
						for _, p := range pairs {
							count[p]++
							if _, ok := reached[p]; !ok {
								reached[p] = g.name + "/" + sc.name
							}
						}
					})
				}
			})
		}
	})
	var b strings.Builder
	for _, p := range slices.SortedFunc(maps.Keys(count), comparePairs) {
		fmt.Fprintf(&b, "\n%s %s: %d, first in %s", p.kind, p.form, count[p], reached[p])
	}
	t.Logf("the notes the scenarios give, by kind and closing:%s", b.String())
	if ran < total {
		t.Logf("%d of the grids' %d scenarios ran, so the pairs they reach aren't checked.", ran, total)
		return
	}
	for _, p := range closingPairs() {
		if _, ok := reached[p]; !ok {
			t.Errorf("no scenario gives a note of kind %s a closing of the form %s.\nAdd a scenario that does to a grid in closingGrids, or, if no cell can, take the pair out of closingPairs, saying why.", p.kind, p.form)
		}
	}
	for _, p := range slices.SortedFunc(maps.Keys(reached), comparePairs) {
		if !slices.Contains(closingPairs(), p) {
			t.Errorf("%s gives a note of kind %s a closing of the form %s, which closingPairs says no cell can.", reached[p], p.kind, p.form)
		}
	}
}

// A closingPair is a note's kind, and the form of its closing: default,
// cantTell, or, for a staged closing, the update's fate: runs-another
// (runs the cell on another value than the normal run, which ran it),
// runs (runs a cell the normal run ran nothing in), blocked, broken (writes
// nothing for it), skipped, panicked or fault (its probe fails it).
type closingPair struct{ kind, form string }

func comparePairs(a, b closingPair) int {
	return strings.Compare(a.kind+" "+a.form, b.kind+" "+b.form)
}

// closingPairs are the pairs a cell can give. A note of a cell the normal
// run ran on a value, its fault, a file it made or its frozen files, is no
// note of a cell that ran nothing, which the update runs. A gaps line's
// note, and a probe's fault, are of a cell that ran nothing, so the update
// never runs it on another value. A bytes lift's product is a lift's,
// which has no probe to skip it or fail it.
func closingPairs() []closingPair {
	all := []string{"default", "cantTell", "runs-another", "runs", "blocked", "broken", "skipped", "panicked", "fault"}
	not := map[string][]string{
		"fault":         {"runs"},
		"file":          {"runs"},
		"frozen":        {"runs"},
		"gap":           {"runs-another"},
		"probeFault":    {"runs-another"},
		"productOrphan": {"skipped", "panicked", "fault"},
	}
	var ps []closingPair
	for _, kind := range slices.Sorted(maps.Values(hypercase.KindNames)) {
		for _, form := range all {
			if !slices.Contains(not[kind], form) {
				ps = append(ps, closingPair{kind, form})
			}
		}
	}
	return ps
}

// A gridEdit is one side of a scenario: an edit of the grid's archive.
type gridEdit struct {
	name string
	fn   func(a *archive.Archive)
}

// A gridScenario is an edit of the archive, made of edits of each side.
type gridScenario struct {
	name  string
	edits []gridEdit
}

// cross is every scenario of one edit from each list, in order.
func cross(lists ...[]gridEdit) []gridScenario {
	scs := []gridScenario{{}}
	for _, l := range lists {
		var next []gridScenario
		for _, sc := range scs {
			for _, e := range l {
				next = append(next, gridScenario{
					name:  strings.TrimPrefix(sc.name+" + "+e.name, " + "),
					edits: append(slices.Clone(sc.edits), e),
				})
			}
		}
		scs = next
	}
	return scs
}

// editsOf returns the edits of a list whose names start with each prefix and
// a space, in the order given.
func editsOf(l []gridEdit, prefixes ...string) []gridEdit {
	var out []gridEdit
	for _, p := range prefixes {
		i := slices.IndexFunc(l, func(e gridEdit) bool { return strings.HasPrefix(e.name, p+" ") })
		if i < 0 {
			panic("no edit " + p)
		}
		out = append(out, l[i])
	}
	return out
}

func seq(fns ...func(a *archive.Archive)) func(a *archive.Archive) {
	return func(a *archive.Archive) {
		for _, f := range fns {
			f(a)
		}
	}
}

// gapOn marks a column's cell of the base with a gaps line.
func gapOn(col string) gridEdit {
	return gridEdit{"gap " + col, insert("gaps", col+" The importer reads quoted fields. Waits on sheets#9.\n", "in/")}
}

// A closingGrid is a fixture, the packages that run over it, and the
// scenarios run on its archive.
type closingGrid struct {
	name, fixture string
	remove        []string          // the fixture's archives left out
	prep          func(s *sandbox)  // what is done before the first update
	archive       string            // the archive the scenarios edit
	pkgs          []childRun        // the packages, as go test ./... runs them
	unowned       map[string]string // in a package that doesn't own a bytes lift: each column past it, "<pkg> <column>", and the lift
	scenarios     []gridScenario
}

// clean is the fixture, written by one update, by path.
func (g closingGrid) clean(t *testing.T) map[string][]byte {
	s := newSandbox(t, g.fixture)
	for _, p := range g.remove {
		s.remove(p, "leave the grid's archive")
	}
	if g.prep != nil {
		g.prep(s)
	}
	for i, r := range g.runAll(s, true, "") {
		if !r.ok() {
			t.Fatalf("the first update of %s failed:\n%s", g.pkgs[i].pkg, r.out)
		}
	}
	return s.snapshot()
}

// runAll runs each package at once, as go test ./... runs them, each
// recording what it observes to observe, then its index, if observe isn't
// "".
func (g closingGrid) runAll(s *sandbox, update bool, observe string) []result {
	results := make([]result, len(g.pkgs))
	var wg sync.WaitGroup
	for i, c := range g.pkgs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.update = update
			env := c.env()
			if observe != "" {
				env = append(env, fmt.Sprintf("%s=%s-%d.jsonl", observeEnv, observe, i))
			}
			results[i] = s.exec(c.pkg, c.registration, env)
		}()
	}
	wg.Wait()
	return results
}

// run runs one scenario, judges it, and returns the pairs its notes give.
func (g closingGrid) run(t *testing.T, clean map[string][]byte, sc gridScenario) []closingPair {
	s := newSandbox(t, "")
	for _, p := range slices.Sorted(maps.Keys(clean)) {
		s.write(p, clean[p], "the clean fixture")
	}
	for _, e := range sc.edits {
		s.edit(g.archive, e.name, e.fn)
	}
	before := archive.Parse(s.read(g.archive))
	dir := t.TempDir()
	normal := g.runAll(s, false, filepath.Join(dir, "normal"))
	update := g.runAll(s, true, filepath.Join(dir, "update"))
	after := archive.Parse(s.read(g.archive))
	j := &judge{g: g, t: t, before: before, after: after}
	for i := range g.pkgs {
		j.normal = append(j.normal, readObserved(t, filepath.Join(dir, fmt.Sprintf("normal-%d.jsonl", i)))...)
		j.update = append(j.update, readObserved(t, filepath.Join(dir, fmt.Sprintf("update-%d.jsonl", i)))...)
	}
	j.judge()
	if t.Failed() {
		for i, c := range g.pkgs {
			t.Logf("the normal run of %s:\n%s\nits update:\n%s", c.registration, normal[i].out, update[i].out)
		}
	}
	return j.pairs
}

func readObserved(t *testing.T, path string) []hypercase.Observed {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []hypercase.Observed
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, 1<<24)
	for sc.Scan() {
		var o hypercase.Observed
		if err := json.Unmarshal(sc.Bytes(), &o); err != nil {
			t.Fatal(err)
		}
		out = append(out, o)
	}
	return out
}

// A judge checks one scenario's records.
type judge struct {
	g              closingGrid
	t              *testing.T
	before, after  *archive.Archive
	normal, update []hypercase.Observed
	pairs          []closingPair
}

type cellKey struct{ pkg, c, col string }

func cells(os []hypercase.Observed) map[cellKey]hypercase.Observed {
	m := map[cellKey]hypercase.Observed{}
	for _, o := range os {
		if o.Record == "cell" {
			m[cellKey{o.Pkg, o.Case, o.Column}] = o
		}
	}
	return m
}

func (j *judge) judge() {
	t := j.t
	t.Helper()
	nc, uc := cells(j.normal), cells(j.update)
	u := fixtureUpdate + "=1 go test ./..."
	for _, n := range j.normal {
		if n.Record != "note" || n.Update {
			continue
		}
		key := cellKey{n.Pkg, n.Case, n.Column}
		bad := func(format string, args ...any) {
			t.Errorf("%s, %s, in %s: %s\nthe note: %s\n%s\nthe normal run's cell: %+v\nthe update's: %+v", n.Case, n.Column, n.Pkg, fmt.Sprintf(format, args...), n.Body, n.Closing, nc[key], uc[key])
		}
		form := n.Form
		unowned := j.g.unowned[n.Pkg+" "+n.Column]
		switch {
		case n.Column == "":
			// A note of a variant its transform declines: the update
			// declines it too.
			if form != "default" {
				bad("a declined variant's note has a %s closing", form)
			}
			for k := range uc {
				if k.pkg == n.Pkg && k.c == n.Case {
					bad("the update runs %s's cell of the variant the normal run finds declined", k.col)
				}
			}
			j.pairs = append(j.pairs, closingPair{n.Kind, form})
			continue
		case unowned != "" && form != "cantTell":
			bad("the package doesn't own %s, on the cell's path, and the closing is %s", unowned, form)
		case unowned == "" && form == "cantTell":
			bad("the closing says the package can't tell, and it owns every bytes lift on the cell's path")
		}
		nr, nok := nc[key]
		ur, uok := uc[key]
		if !nok || !uok {
			bad("the normal run recorded the cell: %v, the update: %v", nok, uok)
			continue
		}
		if nr.Fate != n.This {
			bad("the note's staging has this run's fate as %q, and the cell's is %q", n.This, nr.Fate)
		}
		switch form {
		case "default":
			if n.Closing != n.Default {
				bad("the closing is the default, and differs from it")
			}
			if !sameCell(nr, ur) {
				bad("the closing is the note's default, and the update does otherwise with the cell")
			}
			j.pairs = append(j.pairs, closingPair{n.Kind, "default"})
		case "cantTell":
			want := fmt.Sprintf("Only the package that owns %s runs it in a normal run, so this one can't tell what %s does with %s: ", n.Unowned, u, n.Column)
			if !strings.HasPrefix(n.Closing, want) {
				bad("the closing doesn't start with %q", want)
			}
			if n.Unowned != unowned {
				bad("the closing names %s, and the grid has %s as the bytes lift the package doesn't own", n.Unowned, unowned)
			}
			j.pairs = append(j.pairs, closingPair{n.Kind, "cantTell"})
		case "staged":
			if n.Told != ur.Fate {
				bad("the closing says the update's fate is %q, and the update's is %q", n.Told, ur.Fate)
			}
			if !strings.HasPrefix(n.Closing, u+" ") {
				bad("the staged closing doesn't start with the update")
			}
			says := strings.TrimPrefix(n.Closing, u+" ")
			verb, what, _ := strings.Cut(n.Told, " ")
			class := verb
			switch verb {
			case "runs":
				w := "runs " + n.Column + ": "
				if n.Ran {
					w = "runs " + n.Column + " on another value than this run: "
					class = "runs-another"
				}
				if !strings.HasPrefix(says, w) {
					bad("the closing doesn't say %q", w)
				}
			case "blocked":
				if !strings.HasPrefix(says, "blocks "+n.Column+": ") || !strings.Contains(says, "the lift "+what+", on "+n.Column+"'s path, failed on the case") {
					bad("the closing doesn't say the update blocks the cell by %s", what)
				}
			case "broken":
				if !strings.HasPrefix(says, "writes nothing for "+n.Column+": ") || !strings.Contains(says, "the cell of the lift "+what+", on "+n.Column+"'s path") {
					bad("the closing doesn't say the update writes nothing for the cell past %s", what)
				}
			case "skipped":
				if !strings.HasPrefix(says, "skips "+n.Column+": ") || !strings.Contains(says, n.Column+"'s probe says the case isn't for it: "+what+"\n") {
					bad("the closing doesn't say the update skips the cell for %q", what)
				}
			case "panicked", "fault":
				if !strings.HasPrefix(says, "fails "+n.Column+": ") || !strings.Contains(says, ", and "+what+".") {
					bad("the closing doesn't say the update fails the cell with %q", what)
				}
			}
			j.pairs = append(j.pairs, closingPair{n.Kind, class})
		default:
			bad("the note's form is %q", form)
		}
		j.claims(n, ur)
	}
	j.written(uc)
}

// sameCell reports whether the update did with a cell what the normal run
// did: the same fate, files and product, written alike. An update writes a
// frozen cell only to add the outcome it lacks, which the normal run
// says.
func sameCell(n, u hypercase.Observed) bool {
	if n.Fate != u.Fate || n.Frozen != u.Frozen || !maps.Equal(n.Product, u.Product) || n.Via != u.Via || n.Ran != u.Ran {
		return false
	}
	if u.Frozen && u.Writable {
		return maps.Equal(u.Files, map[string]string{"outcome": n.Files["outcome"]})
	}
	return n.Writable == u.Writable && maps.Equal(n.Files, u.Files)
}

// claims checks what a closing says the update writes or leaves against
// the archive after the update.
func (j *judge) claims(n hypercase.Observed, ur hypercase.Observed) {
	t := j.t
	t.Helper()
	bad := func(format string, args ...any) {
		t.Errorf("%s, %s, in %s: %s\nthe note: %s\n%s\nthe update's cell: %+v", n.Case, n.Column, n.Pkg, fmt.Sprintf(format, args...), n.Body, n.Closing, ur)
	}
	prefix := n.Prefix
	outcome := prefix + "out/" + n.Column + "/outcome"
	path, _, _ := strings.Cut(n.Body, " ")
	if i := strings.Index(n.Closing, "It writes the outcome \""); i >= 0 {
		want, _, _ := strings.Cut(n.Closing[i+len("It writes the outcome \""):], "\"")
		got, _ := j.after.Section(outcome)
		if string(got) != want+"\n" {
			bad("the closing says the update writes the outcome %q, and the archive has %q", want, got)
		}
	}
	if strings.Contains(n.Closing, "It writes nothing for the cell") && !j.cellSame(prefix, n.Column) {
		bad("the closing says the update writes nothing for the cell, and it changed")
	}
	if strings.Contains(n.Closing, "It removes the file") || strings.Contains(n.Closing, "which removes it") {
		if _, ok := j.after.Section(path); ok {
			bad("the closing says the update removes %s, and the archive still has it", path)
		}
	}
	if strings.Contains(n.Closing, "leaves it in place") {
		if _, ok := j.after.Section(path); !ok {
			bad("the closing says the update leaves %s in place, and the archive lacks it", path)
		}
	}
}

// written checks each cell the update records against the archive after
// it: a cell it wrote holds its files, and a bytes lift's its product, and
// a cell it didn't write is as it was.
func (j *judge) written(uc map[cellKey]hypercase.Observed) {
	t := j.t
	t.Helper()
	for _, k := range slices.SortedFunc(maps.Keys(uc), func(a, b cellKey) int { return strings.Compare(a.c+" "+a.col, b.c+" "+b.col) }) {
		ur := uc[k]
		prefix := ur.Prefix
		if !ur.Ran || !ur.Writable {
			if !j.cellSame(prefix, k.col) {
				t.Errorf("%s, %s, in %s: the update records it wrote nothing for the cell, and the cell changed: %+v", k.c, k.col, k.pkg, ur)
			}
			continue
		}
		for p, data := range ur.Files {
			if got, ok := j.after.Section(prefix + "out/" + k.col + "/" + p); !ok || string(got) != data {
				t.Errorf("%s, %s, in %s: the update records it wrote %s as %q, and the archive has %q", k.c, k.col, k.pkg, p, data, got)
			}
		}
		if ur.Via {
			got := map[string]string{}
			for _, s := range j.after.Sections {
				if rest, ok := strings.CutPrefix(s.Name, prefix+"via/"+k.col+"/"); ok {
					got[rest] = string(s.Data)
				}
			}
			want := ur.Product
			if want == nil {
				want = map[string]string{}
			}
			if !maps.Equal(got, want) {
				t.Errorf("%s, %s, in %s: the update records it wrote the product %v, and the archive has %v", k.c, k.col, k.pkg, want, got)
			}
		}
	}
}

// cellSame reports whether a cell's files and product are as they were
// before the update.
func (j *judge) cellSame(prefix, col string) bool {
	sections := func(a *archive.Archive) map[string]string {
		m := map[string]string{}
		for _, s := range a.Sections {
			if strings.HasPrefix(s.Name, prefix+"out/"+col+"/") || strings.HasPrefix(s.Name, prefix+"via/"+col+"/") {
				m[s.Name] = string(s.Data)
			}
		}
		return m
	}
	return maps.Equal(sections(j.before), sections(j.after))
}

// The chain fixture's edits of what import makes now, each under
// in/import/, and of what prices commits.
var (
	chainF = []gridEdit{
		{"F0 none", func(*archive.Archive) {}},
		{"F1 probe r1", insert("in/import/out/evaluate/probe", "r1\n", "in/import/out/quote/")},
		{"F2 probe-panic", insert("in/import/out/evaluate/probe-panic", "pp\n", "in/import/out/quote/")},
		{"F3 probe empty", insert("in/import/out/evaluate/probe", "\n", "in/import/out/quote/")},
		{"F4 probe bad", insert("in/import/out/evaluate/probe", "one\ntwo\n", "in/import/out/quote/")},
		{"F5 resolve error", insert("in/import/out/resolve/error", "boom\n", "in/import/out/quote/")},
		{"F6 parse error", insert("in/import/out/parse/error", "bad\n", "in/import/out/quote/")},
		{"F7 import rejects, stated", seq(insert("properties", "unterminated\n", "in/"), insert("in/import/error", "line 1\n", "in/import/out/"))},
		{"F8 import fault", insert("in/import/fail", "gen\n", "in/import/out/")},
		{"F9 values change", set("in/import/out/evaluate/out/values.txt", "a1 4\nb1 8\n")},
		{"F10 evaluate error", insert("in/import/out/evaluate/error", "e\n", "in/import/out/quote/")},
		{"F11 evaluate panic", insert("in/import/out/evaluate/panic", "ep\n", "in/import/out/quote/")},
		{"F12 import rejects", insert("in/import/error", "line 1\n", "in/import/out/")},
	}
	chainC = []gridEdit{
		{"C0 none", func(*archive.Archive) {}},
		{"C1 import outcome rejected", set("out/import/outcome", "rejected\n")},
		{"C2 import outcome missing", drop("out/import/outcome")},
		{"C3 import outcome blocked foo", set("out/import/outcome", "blocked foo\n")},
		{"C4 via probe stale", insert("via/import/evaluate/probe", "stale\n", "via/import/quote/")},
		{"C5 via probe empty", insert("via/import/evaluate/probe", "\n", "via/import/quote/")},
		{"C6 via probe bad", insert("via/import/evaluate/probe", "one\ntwo\n", "via/import/quote/")},
		{"C7 via probe-panic", insert("via/import/evaluate/probe-panic", "sp\n", "via/import/quote/")},
		{"C8 via resolve error", insert("via/import/resolve/error", "stale\n", "via/import/quote/")},
		{"C9 via parse error", insert("via/import/parse/error", "stale\n", "via/import/quote/")},
		{"C10 via values", set("via/import/evaluate/out/values.txt", "a1 9\n")},
		{"C11 via extra Z9", insert("via/import/quote/Z9", "9\n", "out/")},
		{"C12 via drop A1", drop("via/import/quote/A1")},
		{"C13 out stray", insert("out/evaluate/stray.txt", "s\n", "out/evaluate/values.txt")},
		{"C14 out drop values", drop("out/evaluate/values.txt")},
		{"C15 via evaluate panic", insert("via/import/evaluate/panic", "sp\n", "via/import/quote/")},
		{"C16 via undeclared out", insert("via/import/evaluate/out/zz.json", "x\n", "via/import/evaluate/out/values.txt")},
		{"C17 via evaluate error", insert("via/import/evaluate/error", "stale\n", "via/import/quote/")},
		{"C18 evaluate outcome skipped", set("out/evaluate/outcome", "skipped x\n")},
		{"C19 via undeclared out and values", seq(insert("via/import/evaluate/out/zz.json", "x\n", "via/import/evaluate/out/values.txt"), set("via/import/evaluate/out/values.txt", "a1 9\n"))},
	}
	// louderC edits louder's variant of prices: its committed inputs, and
	// what its cells commit.
	louderC = []gridEdit{
		{"V0 none", func(*archive.Archive) {}},
		{"V1 inputs import/error", insert("transform/louder/in/import/error", "line 1\n", "transform/louder/in/import/out/")},
		{"V2 inputs values stale", set("transform/louder/in/import/out/evaluate/out/values.txt", "A1 9\n")},
		{"V3 via probe-panic", insert("transform/louder/via/import/evaluate/probe-panic", "x\n", "transform/louder/via/import/evaluate/out/")},
		{"V4 via probe stale", insert("transform/louder/via/import/evaluate/probe", "stale\n", "transform/louder/via/import/evaluate/out/")},
		{"V5 delete-only inputs, and a stray", seq(insert("transform/louder/delete", "nothing/here\n", "transform/louder/via/"), insert("transform/louder/out/evaluate/stray.txt", "s\n", "transform/louder/out/evaluate/values.txt"))},
		{"V6 inputs undeclared", insert("transform/louder/in/import/out/undeclared/x", "x\n", "transform/louder/in/import/out/evaluate/")},
		{"V7 inputs import/fail", insert("transform/louder/in/import/fail", "x\n", "transform/louder/in/import/out/")},
		{"V8 inputs import/panic", insert("transform/louder/in/import/panic", "x\n", "transform/louder/in/import/out/")},
		{"V9 inputs probe", insert("transform/louder/in/import/out/evaluate/probe", "r9\n", "transform/louder/in/import/out/evaluate/out/")},
		{"V10 inputs import/error, and via drops A1", seq(insert("transform/louder/in/import/error", "line 1\n", "transform/louder/in/import/out/"), drop("transform/louder/via/import/quote/A1"))},
	}
)

// The chain-twice fixture's edits of what export makes now, under
// in/export/, and of what sheet commits.
var (
	twiceF = []gridEdit{
		{"T0 none", func(*archive.Archive) {}},
		{"T1 export rejects, stated", seq(insert("properties", "unterminated\n", "in/"), insert("in/export/error", "x\n", "in/export/out/"))},
		{"T2 export rejects", insert("in/export/error", "x\n", "in/export/out/")},
		{"T3 export fault", insert("in/export/fail", "gen\n", "in/export/out/")},
		{"T4 export panic", insert("in/export/panic", "xp\n", "in/export/out/")},
		{"T5 export makes import/error", insert("in/export/out/import/error", "ie\n", "in/export/out/import/out/")},
		{"T6 export makes import/fail", insert("in/export/out/import/fail", "if\n", "in/export/out/import/out/")},
		{"T7 export's A1 changed", set("in/export/out/import/out/quote/A1", "4\n")},
		{"T8 export makes import/panic", insert("in/export/out/import/panic", "ip\n", "in/export/out/import/out/")},
		{"T9 export undeclared", set("in/export/out/zz", "z\n")},
		{"T10 import undeclared", set("in/export/out/import/out/undeclared/x", "x\n")},
	}
	twiceC = []gridEdit{
		{"D0 none", func(*archive.Archive) {}},
		{"D1 via export drops A1", drop("via/export/import/out/quote/A1")},
		{"D2 via export import/error", insert("via/export/import/error", "stale\n", "via/export/import/out/")},
		{"D3 via export import/fail", insert("via/export/import/fail", "stale\n", "via/export/import/out/")},
		{"D4 via export import/panic", insert("via/export/import/panic", "stale\n", "via/export/import/out/")},
		{"D5 export outcome rejected", set("out/export/outcome", "rejected\n")},
		{"D6 export outcome missing", drop("out/export/outcome")},
		{"D7 via import Z9", insert("via/import/quote/Z9", "9\n", "out/")},
		{"D8 via import drops A1", drop("via/import/quote/A1")},
		{"D9 import outcome rejected", set("out/import/outcome", "rejected\n")},
		{"D10 list files stale", set("out/list/files.txt", "0 files\n")},
		{"D11 via export A1 changed", set("via/export/import/out/quote/A1", "4\n")},
		{"D12 via export import undeclared", insert("via/export/import/out/undeclared/x", "x\n", "via/import/")},
		{"D13 export outcome blocked foo", set("out/export/outcome", "blocked foo\n")},
	}
)

// The transforms fixture's edits of twice's base, and of what upper's
// variant commits, on a path with no bytes lift.
var (
	transformsF = []gridEdit{
		{"B0 none", func(*archive.Archive) {}},
		{"B1 evaluate probe", insert("in/evaluate/probe", "r1\n", "in/evaluate/out/")},
		{"B2 resolve error", insert("in/resolve/error", "A1 -> A1: cycle\n", "in/evaluate/out/")},
		{"B3 values change", set("in/evaluate/out/values.txt", "a1 seven\n")},
		{"B4 evaluate probe-panic", insert("in/evaluate/probe-panic", "pp\n", "in/evaluate/out/")},
		{"B5 evaluate probe empty", insert("in/evaluate/probe", "\n", "in/evaluate/out/")},
		{"B6 evaluate error", insert("in/evaluate/error", "e\n", "in/evaluate/out/")},
	}
	transformsC = []gridEdit{
		{"U0 none", func(*archive.Archive) {}},
		{"U1 inputs resolve error", insert("transform/upper/in/resolve/error", "A1 -> A1: cycle\n", "transform/upper/out/")},
		{"U2 inputs evaluate probe", insert("transform/upper/in/evaluate/probe", "a stale reason\n", "transform/upper/out/")},
		{"U3 inputs delete values", insert("transform/upper/delete", "evaluate/out/values.txt\n", "transform/upper/out/")},
		{"U4 delete-only inputs, and a stray", seq(insert("transform/upper/delete", "nothing/here\n", "transform/upper/out/"), insert("transform/upper/out/evaluate/stray.txt", "a stray\n", "transform/upper/out/evaluate/values.txt"))},
		{"U5 inputs evaluate probe-panic", insert("transform/upper/in/evaluate/probe-panic", "sp\n", "transform/upper/out/")},
		{"U6 inputs evaluate probe empty", insert("transform/upper/in/evaluate/probe", "\n", "transform/upper/out/")},
		{"U7 inputs values stale", set("transform/upper/in/evaluate/out/values.txt", "A1 NINE\n")},
		{"U8 evaluate outcome blocked foo", set("transform/upper/out/evaluate/outcome", "blocked foo\n")},
		{"U9 inputs evaluate error", insert("transform/upper/in/evaluate/error", "stale\n", "transform/upper/out/")},
		{"U10 inputs delete resolve/error, and evaluate outcome blocked foo", seq(insert("transform/upper/delete", "resolve/error\n", "transform/upper/out/"), set("transform/upper/out/evaluate/outcome", "blocked foo\n"))},
	}
)

// closingGrids are the grids TestClosings runs.
func closingGrids() []closingGrid {
	const prices = "testdata/export/prices.txtar"
	const sheet = "testdata/grid/sheet.txtar"
	chainRemove := []string{"testdata/export/empty.txtar", "testdata/export/names.txtar", "testdata/export/looped.txtar", "testdata/script/plain.txtar"}
	chain := closingGrid{fixture: "chain", remove: chainRemove, archive: prices, pkgs: []childRun{{pkg: "", registration: "chain"}}}
	twice := closingGrid{fixture: "chain-twice", archive: sheet, pkgs: []childRun{{pkg: "", registration: "chain-twice"}}}
	writeB := func(s *sandbox) { s.write("b/corpus_test.go", []byte(packageTestFile), "./b is a package with tests") }

	var gs []closingGrid
	add := func(name string, g closingGrid, scs []gridScenario) {
		g.name, g.scenarios = name, scs
		gs = append(gs, g)
	}
	// chain: one package owns import, a bytes lift, and the ladder and
	// suites past it.
	add("chain", chain, cross(chainF, chainC))
	// louder: the transform's variant past import.
	add("louder", chain, cross(editsOf(chainF, "F0", "F1", "F5", "F9", "F12"), louderC))
	// frozen: a gaps line freezes a cell of import, or past it.
	var frozenF []gridEdit
	for _, col := range []string{"import", "evaluate", "list"} {
		frozenF = append(frozenF, gapOn(col))
	}
	add("frozen", chain, cross(frozenF, editsOf(chainF, "F0", "F1", "F2", "F3", "F5", "F8", "F9", "F12"), editsOf(chainC, "C0", "C1", "C4", "C5", "C8", "C10", "C11")))
	// chain-twice: export, then import, two bytes lifts in sequence.
	add("chain-twice", twice, cross(twiceF, twiceC))
	add("chain-twice-frozen", twice, cross([]gridEdit{gapOn("export"), gapOn("import"), gapOn("list")}, editsOf(twiceF, "T0", "T1", "T3", "T7"), editsOf(twiceC, "D0", "D1", "D2", "D5", "D7", "D11")))
	// chain-twice-shared: ./b owns import and list, and only uses export.
	shared := twice
	shared.prep = writeB
	shared.pkgs = []childRun{{pkg: "", registration: "chain-twice-a"}, {pkg: "b", registration: "chain-twice-b"}}
	shared.unowned = map[string]string{"b import": "export", "b list": "export"}
	add("chain-twice-shared", shared, cross(slices.Concat(twiceF, []gridEdit{gapOn("import"), gapOn("list")}), editsOf(twiceC, "D0", "D1", "D2", "D5", "D7", "D8", "D10", "D11", "D12")))
	// chain-shared: ./b owns evaluate, whose probe runs past import, which
	// it only uses, and louder, which the root package only uses.
	cs := closingGrid{
		fixture: "chain-shared", archive: prices,
		pkgs:    []childRun{{pkg: "", registration: "chain-a"}, {pkg: "b", registration: "chain-b"}},
		unowned: map[string]string{"b evaluate": "import", "b list": "import"},
	}
	add("chain-shared", cs, cross([]gridEdit{{"no gap", func(*archive.Archive) {}}, gapOn("evaluate")}, editsOf(chainF, "F0", "F1", "F3", "F5", "F9", "F12"), slices.Concat(editsOf(chainC, "C0", "C1", "C4", "C5", "C10", "C13"), editsOf(louderC, "V1", "V5"))))
	// transforms: upper's variant, with no bytes lift on any path.
	tr := closingGrid{
		fixture: "transforms", archive: "testdata/script/twice.txtar",
		remove: []string{"testdata/script/errs.txtar", "testdata/script/garbled.txtar"},
		prep: func(s *sandbox) {
			s.edit("testdata/script/twice.txtar", "evaluate writes a file", set("in/evaluate/out/values.txt", "a1 six\n"))
		},
		pkgs: []childRun{{pkg: "", registration: "tr-ladder-eval"}},
	}
	add("transforms", tr, cross(transformsF, transformsC))
	return gs
}
