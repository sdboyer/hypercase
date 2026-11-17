package hypercase_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/registry"
)

// TestClosings: every closing a normal run gives says what the update then
// does. Each grid takes a fixture whose archive one update has written,
// and runs scenarios on it: an edit of what a lift or the case makes now
// (the code's side, F) crossed with an edit of what the archive commits
// for a bytes lift, a transform's variant or a cell (the committed side,
// C). Each scenario runs a normal run, then an update, of each of the
// grid's packages, as go test ./... runs them, in children that record
// each cell's fate and files and whether its test failed, and each run of
// a lift's function, in both runs, and each note's closing and what it
// claims. Then every note's closing
// in the normal run must hold:
//
//   - A default closing, the note's own, only where the update does with
//     the cell what the normal run does: its fate, its files and its
//     product are the normal run's, and it writes them or doesn't alike.
//   - A staged closing, which says what the update does, only where the
//     update does otherwise, with the fate the update then has, in words
//     that name it: it runs the cell, on another value than the normal
//     run's or not, blocks it by the lift that fails, writes nothing for
//     it past a lift whose cell it writes nothing for, skips it for the
//     probe's reason, or fails it with the probe's fault.
//   - A closing that says the package can't tell only in a package that
//     doesn't own a bytes lift on the cell's path, where every closing of
//     such a cell says so.
//   - Each of its claims, which give every sentence it says of the update,
//     holds of what the update did (claimChecks): the outcome it writes, a
//     file it writes, removes or leaves in place, nothing for the cell, the
//     lift it reruns, a variant's inputs it writes, rewrites or removes.
//   - A note of a variant about no one cell closes on its default.
//
// In each run, a cell's test fails just where the run gives notes of the
// cell. What each update record says it wrote is checked against the
// archive after the update, and what it didn't write is unchanged. The
// grids together must reach every pair of a note's kind and the form of
// its closing that a note can give (closingPairs).
func TestClosings(t *testing.T) {
	t.Parallel()
	kinds := hypercase.NoteKinds(t)
	pairs := closingPairs(t, kinds)
	var mu sync.Mutex
	reached := map[closingPair]string{} // each pair, and the scenario first by name that reached it
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
						got := g.run(t, clean, sc, kinds)
						mu.Lock()
						defer mu.Unlock()
						ran++
						name := g.name + "/" + sc.name
						for _, p := range got {
							count[p]++
							if first, ok := reached[p]; !ok || name < first {
								reached[p] = name
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
	for _, p := range pairs {
		if _, ok := reached[p]; !ok {
			t.Errorf("no scenario gives a note of kind %s a closing of the form %s.\nAdd a scenario that does to a grid in closingGrids, or, if no note can, exclude the pair in closingPairs, saying why.", p.kind, p.form)
		}
	}
	for _, p := range slices.SortedFunc(maps.Keys(reached), comparePairs) {
		if !slices.Contains(pairs, p) {
			t.Errorf("%s gives a note of kind %s a closing of the form %s, which closingPairs says no note can.", reached[p], p.kind, p.form)
		}
	}
}

// A closingPair is a note's kind, and the form of its closing: default,
// cantTell, or, for a staged closing, the first word of the update's fate,
// as fateWords words it, and runs-another for a fate of runs on another
// value than the normal run's, which ran the cell on one.
type closingPair struct{ kind, form string }

func comparePairs(a, b closingPair) int {
	return strings.Compare(a.kind+" "+a.form, b.kind+" "+b.form)
}

// closingPairs are the pairs a note can give: each kind the package
// declares, as NoteKinds names them, with each form, but those an
// exclusion takes out, each saying why no note gives them. The staged
// forms are the words fateWords begins a fate with, which the type checker
// finds. It fails on an exclusion of a kind or a form that is none.
func closingPairs(t *testing.T, kinds map[int]string) []closingPair {
	t.Helper()
	verbs := hypercase.FateVerbs(t)
	forms := slices.Concat([]string{"default", "cantTell", "runs-another"}, verbs)
	var staged []string
	for _, f := range forms {
		if f != "default" {
			staged = append(staged, f)
		}
	}
	exclusions := []struct {
		kind  string
		forms []string
		why   string
	}{
		{"fault", []string{"runs"}, "a fault is of a cell the normal run ran on a value, which the update doesn't run as a cell that ran nothing"},
		{"file", []string{"runs"}, "a file is one the normal run's run of the cell made, so the cell ran on a value"},
		{"frozen", []string{"runs"}, "a frozen cell's note is of its run on a value, or its committed files, which a cell that ran nothing has none of"},
		{"gap", []string{"runs-another"}, "a gaps line's note is of a cell that ran nothing, which the update never runs on another value"},
		{"probeFault", []string{"runs-another"}, "a probe's fault is of a cell that ran nothing, which the update never runs on another value"},
		{"productOrphan", []string{"skipped", "panicked", "fault"}, "a bytes lift's product is a lift's, which has no probe to skip it or fail it"},
		{"inputs", staged, "a note of a variant's inputs is about no one cell, so it closes on its default"},
	}
	names := slices.Sorted(maps.Values(kinds))
	out := map[string][]string{}
	for _, e := range exclusions {
		if !slices.Contains(names, e.kind) {
			t.Errorf("closingPairs excludes forms of the kind %s, which is no kind of note's: %v", e.kind, names)
		}
		for _, f := range e.forms {
			if !slices.Contains(forms, f) {
				t.Errorf("closingPairs excludes the form %s of %s notes, which is no form of a closing's: %v", f, e.kind, forms)
			}
		}
		if strings.TrimSpace(e.why) == "" {
			t.Errorf("closingPairs excludes %v of %s notes, and says no reason", e.forms, e.kind)
		}
		out[e.kind] = append(out[e.kind], e.forms...)
	}
	var ps []closingPair
	for _, kind := range names {
		for _, form := range forms {
			if !slices.Contains(out[kind], form) {
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
func (g closingGrid) run(t *testing.T, clean map[string][]byte, sc gridScenario, kinds map[int]string) []closingPair {
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
	j := &judge{g: g, t: t, kinds: kinds, before: before, after: after, owners: g.owners(t, s)}
	for i := range g.pkgs {
		j.normalOut = append(j.normalOut, normal[i].out)
		j.normal = append(j.normal, readObserved(t, filepath.Join(dir, fmt.Sprintf("normal-%d.jsonl", i)))...)
		j.update = append(j.update, readObserved(t, filepath.Join(dir, fmt.Sprintf("update-%d.jsonl", i)))...)
	}
	j.lifts = liftRuns(j.update)
	j.judge()
	if t.Failed() {
		for i, c := range g.pkgs {
			t.Logf("the normal run of %s:\n%s\nits update:\n%s", c.registration, normal[i].out, update[i].out)
		}
	}
	return j.pairs
}

// owners are the transforms that some package's registry, after the
// update, records it owns.
func (g closingGrid) owners(t *testing.T, s *sandbox) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, c := range g.pkgs {
		reg, err := registry.Parse(s.read(filepath.Join(c.pkg, "testdata", registry.Name)))
		if err != nil {
			t.Fatalf("reading %s's registry: %v", c.registration, err)
		}
		for _, typ := range reg.Types {
			for _, tr := range typ.Transforms {
				if tr.Owned {
					out[tr.Name] = true
				}
			}
		}
	}
	return out
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
	kinds          map[int]string
	before, after  *archive.Archive
	normal, update []hypercase.Observed
	lifts          map[liftKey]bool // the lifts whose functions the update ran
	owners         map[string]bool  // the transforms some package's registry, after the update, records it owns
	normalOut      []string         // each package's normal run's output
	pairs          []closingPair
}

type cellKey struct{ pkg, c, col string }

// A liftKey is a lift whose function ran on a case, in a package.
type liftKey struct{ pkg, c, lift string }

// liftRuns are the lifts whose functions a run ran, on each case, in each
// package, but in a normal run's staging of a case as its update stages
// it.
func liftRuns(os []hypercase.Observed) map[liftKey]bool {
	m := map[liftKey]bool{}
	for _, o := range os {
		if o.Record == "lift" && !o.AsUpdate {
			m[liftKey{o.Pkg, o.Case, o.Lift}] = true
		}
	}
	return m
}

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
	for _, n := range j.normal {
		if n.Record != "note" || n.Update {
			continue
		}
		key := cellKey{n.Pkg, n.Case, n.Column}
		bad := func(format string, args ...any) {
			t.Errorf("%s, %s, in %s: %s\nthe note: %s\n%s\nits claims: %+v\nthe normal run's cell: %+v\nthe update's: %+v", n.Case, n.Column, n.Pkg, fmt.Sprintf(format, args...), n.Body, n.Closing, *n.Claims, nc[key], uc[key])
		}
		c := &noteCheck{j: j, n: n, cl: *n.Claims, uc: uc}
		if n.Column != "" {
			var nok, uok bool
			c.nr, nok = nc[key]
			c.ur, uok = uc[key]
			if !nok || !uok {
				bad("the normal run recorded the cell: %v, the update: %v", nok, uok)
				continue
			}
			c.cell = true
			if c.nr.Fate != n.This {
				bad("the note's staging has this run's fate as %q, and the cell's is %q", n.This, c.nr.Fate)
			}
		}
		form := n.Form
		switch form {
		case "default":
			if n.Closing != n.Default {
				bad("the closing is the default, and differs from it")
			}
		case "staged":
			verb, _, _ := strings.Cut(c.cl.Does, " ")
			form = verb
			if c.cl.Another {
				form += "-another"
			}
		case "cantTell":
		default:
			bad("the note's form is %q", form)
		}
		if n.Column == "" && form != "default" {
			bad("a variant's note about no one cell has a %s closing", form)
		}
		if n.Declined {
			// The update declines the variant too.
			for k := range uc {
				if k.pkg == n.Pkg && k.c == n.Case {
					bad("the update runs %s's cell of the variant the normal run finds declined", k.col)
				}
			}
		}
		for _, name := range slices.Sorted(maps.Keys(claimChecks)) {
			if msg := claimChecks[name](c); msg != "" {
				bad("%s: %s", name, msg)
			}
		}
		kind, ok := j.kinds[n.Kind]
		if !ok {
			bad("the note's kind is %d, which no noteKind is", n.Kind)
		}
		j.pairs = append(j.pairs, closingPair{kind, form})
	}
	j.written(uc)
	j.failedWithNotes()
}

// failedWithNotes checks that, in each run, a cell's test fails where the
// run gives notes of the cell, which report closes, and only there.
func (j *judge) failedWithNotes() {
	for _, run := range [][]hypercase.Observed{j.normal, j.update} {
		noted := map[cellKey]bool{}
		for _, o := range run {
			if o.Record == "note" && o.Column != "" {
				noted[cellKey{o.Pkg, o.Case, o.Column}] = true
			}
		}
		for _, o := range run {
			if o.Record != "cell" {
				continue
			}
			switch k := (cellKey{o.Pkg, o.Case, o.Column}); {
			case o.Failed && !noted[k]:
				j.t.Errorf("%s, %s, in %s: the cell's test fails in the %s with no note of the cell: %+v", o.Case, o.Column, o.Pkg, runName(o.Update), o)
			case !o.Failed && noted[k]:
				j.t.Errorf("%s, %s, in %s: the %s gives notes of the cell, and its test passes: %+v", o.Case, o.Column, o.Pkg, runName(o.Update), o)
			}
		}
	}
}

func runName(update bool) string {
	if update {
		return "update"
	}
	return "normal run"
}

// A noteCheck is what a claim's check reads: the note, what its closing
// claims, and, for a note of a cell, the cell's records in the normal run
// and the update.
type noteCheck struct {
	j      *judge
	n      hypercase.Observed
	cl     hypercase.Claims
	cell   bool
	nr, ur hypercase.Observed
	uc     map[cellKey]hypercase.Observed
}

// staged reports whether the closing says what the update does, or that
// the package can't tell.
func (c *noteCheck) staged() bool { return c.cl.Does != "" || c.cl.CantTell != "" }

// section returns a section of an archive, and whether it has it.
func section(a *archive.Archive, name string) (string, bool) {
	data, ok := a.Section(name)
	return string(data), ok
}

// out names a file of the note's cell, by its path under out/<column>/.
func (c *noteCheck) out(p string) string { return c.n.Prefix + "out/" + c.n.Column + "/" + p }

// claimChecks check each field of a closing's claims against what the
// update did: each returns what is wrong, or "". TestClaimChecks fails on
// a field of the claims with no check here, so no closing claims what
// TestClosings doesn't check.
var claimChecks = map[string]func(c *noteCheck) string{
	"Does": func(c *noteCheck) string {
		switch {
		case !c.cell && c.cl.Does != "":
			return "a variant's note about no one cell says what the update does with a cell"
		case !c.cell:
		case c.cl.Does != "" && c.cl.Does != c.ur.Fate:
			return fmt.Sprintf("the closing says the update's fate is %q, and it is %q", c.cl.Does, c.ur.Fate)
		case c.cl.Does != "" && sameCell(c.nr, c.ur):
			return "the closing says the update does otherwise with the cell than the normal run, and it does the same"
		case !c.staged() && !sameCell(c.nr, c.ur):
			return "the closing is the note's default, and the update does otherwise with the cell"
		}
		return ""
	},
	"Another": func(c *noteCheck) string {
		want := c.cell && c.cl.Does == "runs" && c.nr.OnValue
		switch {
		case c.cl.Another != want:
			return fmt.Sprintf("the closing says the update runs the cell on another value: %v, and the update runs a cell the normal run ran on a value: %v", c.cl.Another, want)
		case c.cl.Another && maps.Equal(c.nr.View, c.ur.View):
			return "the closing says the update runs the cell on another value, and it ran the cell on the input view the normal run did"
		}
		return ""
	},
	"Variant": func(c *noteCheck) string {
		differs := c.cell && !maps.Equal(c.nr.Root, c.ur.Root)
		switch {
		case !c.staged() && c.cl.Variant != "":
			return "the note's default says the update stages the variant otherwise"
		case !c.staged():
		case (c.cl.Variant != "") != differs:
			return fmt.Sprintf("the closing says the update stages the variant on what %q makes, and the update's input view of the case differs from the normal run's: %v", c.cl.Variant, differs)
		case c.cl.Variant != "" && c.n.Prefix != "transform/"+c.cl.Variant+"/":
			return fmt.Sprintf("the closing names the transform %s, and the case is %s", c.cl.Variant, c.n.Case)
		}
		return ""
	},
	// The lift a closing says the update reruns is one whose function the
	// update ran on the case, as its records of each run of a lift show:
	// where the package can't tell, the lift it doesn't own, which the
	// package that owns it runs, and otherwise the last bytes lift on the
	// cell's path that the package's update ran.
	"Reruns": func(c *noteCheck) string {
		var want string
		switch {
		case c.cl.CantTell != "":
			want = c.cl.CantTell
			ran := false
			for k := range c.j.lifts {
				ran = ran || k.c == c.n.Case && k.lift == want
			}
			if !ran {
				return fmt.Sprintf("the closing says the update reruns %s, and no package's update ran its function on the case", want)
			}
		case c.cl.Does != "":
			for _, l := range c.ur.Path {
				if c.j.lifts[liftKey{c.n.Pkg, c.n.Case, l}] {
					want = l
					break
				}
			}
		}
		if c.cl.Reruns != want {
			return fmt.Sprintf("the closing says the update reruns %q, and the last bytes lift on the cell's path, %v, whose function the update ran on the case is %q", c.cl.Reruns, c.ur.Path, want)
		}
		return ""
	},
	"CantTell": func(c *noteCheck) string {
		var want string
		if c.cell {
			want = c.j.g.unowned[c.n.Pkg+" "+c.n.Column]
		}
		if c.cl.CantTell != want {
			return fmt.Sprintf("the closing says the package can't tell, as it doesn't own %q, and the bytes lift on the cell's path it doesn't own is %q", c.cl.CantTell, want)
		}
		return ""
	},
	"PastFresh": func(c *noteCheck) string {
		if !c.cl.PastFresh {
			return ""
		}
		if !c.cell || !c.j.cellSame(c.n.Prefix, c.n.Column) {
			return "the closing says the update leaves the cell's committed outcome and product in place, and they changed"
		}
		for _, k := range slices.SortedFunc(maps.Keys(c.uc), compareCells) {
			u := c.uc[k]
			if k.pkg == c.n.Pkg && k.c == c.n.Case && len(u.Path) > 0 && u.Path[0] == c.n.Column && u.OnValue && !maps.Equal(u.View, c.ur.Product) {
				return fmt.Sprintf("the closing says the update runs the cells past %s on what it makes now, and it runs %s on %v, where %s makes %v", c.n.Column, k.col, u.View, c.n.Column, c.ur.Product)
			}
		}
		return ""
	},
	"RunWrites": func(c *noteCheck) string {
		if c.cl.RunWrites && (c.ur.Fate != "runs" || !c.ur.OnValue) {
			return "the closing says what the update writes for the cell is its run's, and the update didn't run it"
		}
		return ""
	},
	"MayWrite": func(c *noteCheck) string {
		switch {
		case c.cl.MayWrite == "":
		case c.cl.MayWrite != c.n.File:
			return fmt.Sprintf("the closing says the update's run may still write %s, and the note is about %s", c.cl.MayWrite, c.n.File)
		case !c.cl.Another:
			return "the closing says the update's run of the cell may still write the file, and doesn't say it runs the cell on another value"
		}
		return ""
	},
	"Outcome": func(c *noteCheck) string {
		if c.cl.Outcome == "" {
			return ""
		}
		if got, _ := section(c.j.after, c.out("outcome")); got != c.cl.Outcome+"\n" {
			return fmt.Sprintf("the closing says the update writes the outcome %q, and the archive has %q", c.cl.Outcome, got)
		}
		return ""
	},
	"Differs": func(c *noteCheck) string {
		// What the closing says the update writes, if it says.
		var writes string
		switch {
		case c.cl.Outcome != "":
			writes = c.cl.Outcome + "\n"
		case c.cl.RunWrites && c.ur.Ran && c.ur.Writable:
			writes = c.ur.Files["outcome"]
		}
		this, ok := c.nr.Files["outcome"]
		switch {
		case c.cl.Differs && writes != "" && (!ok || this == writes):
			return fmt.Sprintf("the closing says the outcome the update writes, %q, isn't this run's, which is %q", writes, this)
		case !c.cl.Differs && c.cl.Outcome != "" && ok && this != writes:
			return fmt.Sprintf("the closing doesn't say the outcome the update writes, %q, isn't this run's, which is %q", writes, this)
		}
		return ""
	},
	"Panic": func(c *noteCheck) string {
		if !c.cl.Panic {
			return ""
		}
		_, v, ok := strings.Cut(c.ur.Fate, "'s probe panics: ")
		got, _ := section(c.j.after, c.out("error.txt"))
		switch {
		case c.cl.Outcome != "panicked":
			return fmt.Sprintf("the closing says the update writes a panic, and the outcome %q", c.cl.Outcome)
		case !ok:
			return fmt.Sprintf("the closing says the update writes the probe's panic, and its fate is %q", c.ur.Fate)
		case got != v+"\n":
			return fmt.Sprintf("the closing says the update writes the panic %q in error.txt, and the archive has %q", v, got)
		}
		return ""
	},
	"Nothing": func(c *noteCheck) string {
		switch {
		case !c.cl.Nothing:
		case !c.cell:
			return "a variant's note about no one cell says the update writes nothing for a cell"
		case c.ur.Ran && c.ur.Writable:
			return "the closing says the update writes nothing for the cell, and it records that it wrote it"
		case !c.j.cellSame(c.n.Prefix, c.n.Column):
			return "the closing says the update writes nothing for the cell, and it changed"
		}
		return ""
	},
	"Frozen": func(c *noteCheck) string {
		if c.cl.Frozen && !c.ur.Frozen {
			return "the closing says a gaps line freezes the cell, and the update has it unfrozen"
		}
		return ""
	},
	"Writes": func(c *noteCheck) string {
		if c.cl.Writes == "" {
			return ""
		}
		got, ok := section(c.j.after, c.cl.Writes)
		// A file of the cell's, or of its bytes lift's product.
		want, made := "", false
		if rel, ok := strings.CutPrefix(c.cl.Writes, c.n.Prefix+"out/"+c.n.Column+"/"); ok {
			want, made = c.ur.Files[rel]
		} else if rel, ok := strings.CutPrefix(c.cl.Writes, c.n.Prefix+"via/"+c.n.Column+"/"); ok && c.ur.Via {
			want, made = c.ur.Product[rel]
		}
		if !ok || !made || !c.ur.Ran || !c.ur.Writable || got != want {
			return fmt.Sprintf("the closing says the update writes %s, and the archive has %q", c.cl.Writes, got)
		}
		return ""
	},
	"OnlyOutcome": func(c *noteCheck) string {
		if c.cl.OnlyOutcome == "" {
			return ""
		}
		was, is := c.j.cellSections(c.j.before, c.n.Prefix, c.n.Column), c.j.cellSections(c.j.after, c.n.Prefix, c.n.Column)
		_, had := was[c.cl.OnlyOutcome]
		was[c.cl.OnlyOutcome] = c.ur.Files["outcome"]
		if had || c.cl.OnlyOutcome != c.out("outcome") || !maps.Equal(was, is) {
			return fmt.Sprintf("the closing says the update adds %s and changes nothing else of the cell, and the cell had it: %v, or the cell is %v", c.cl.OnlyOutcome, had, is)
		}
		return ""
	},
	"Leaves": func(c *noteCheck) string {
		if c.cl.Leaves == "" {
			return ""
		}
		was, ok := section(c.j.before, c.cl.Leaves)
		is, still := section(c.j.after, c.cl.Leaves)
		if !ok || !still || was != is {
			return fmt.Sprintf("the closing says the update leaves %s in place, and the archive had it: %v, and has it as it was: %v", c.cl.Leaves, ok, still && was == is)
		}
		return ""
	},
	"Removes": func(c *noteCheck) string {
		if c.cl.Removes == "" {
			return ""
		}
		_, ok := section(c.j.before, c.cl.Removes)
		if _, still := section(c.j.after, c.cl.Removes); !ok || still {
			return fmt.Sprintf("the closing says the update removes %s, and the archive had it: %v, and still has it: %v", c.cl.Removes, ok, still)
		}
		return ""
	},
	"RemovesInputs": func(c *noteCheck) string {
		if c.cl.RemovesInputs == "" {
			return ""
		}
		if was, is := c.j.inputs(c.j.before, c.cl.RemovesInputs), c.j.inputs(c.j.after, c.cl.RemovesInputs); len(was) == 0 || len(is) > 0 {
			return fmt.Sprintf("the closing says the update removes %s's inputs, and the archive had %v, and has %v", c.cl.RemovesInputs, slices.Sorted(maps.Keys(was)), slices.Sorted(maps.Keys(is)))
		}
		return ""
	},
	"WritesInputs": func(c *noteCheck) string {
		if c.cl.WritesInputs == "" {
			return ""
		}
		if was := c.j.inputs(c.j.before, c.cl.WritesInputs); len(was) > 0 {
			return fmt.Sprintf("the closing says the update writes %s's inputs, which the archive lacks, and it had %v", c.cl.WritesInputs, slices.Sorted(maps.Keys(was)))
		}
		return c.inputsMade(c.cl.WritesInputs)
	},
	// The package that owns the transform rewrites its variant's inputs,
	// and its registry, after the update, records that it owns it.
	"RewritesInputs": func(c *noteCheck) string {
		tr := c.cl.RewritesInputs
		switch {
		case tr == "":
			return ""
		case !c.j.owners[tr]:
			return fmt.Sprintf("the closing says this package's update rewrites %s's inputs, and no registry records a package owns it", tr)
		case maps.Equal(c.j.inputs(c.j.before, tr), c.j.inputs(c.j.after, tr)):
			return fmt.Sprintf("the closing says this package's update rewrites %s's inputs, and they are as they were", tr)
		}
		return c.inputsMade(tr)
	},
	// In a package that doesn't own the transform, the closing says the
	// owner's update rewrites the inputs, and, if no package owns it, that
	// the root-package check says so, and no update rewrites them: which
	// holds is what the registries the update wrote record.
	"OwnerRewritesInputs": func(c *noteCheck) string {
		tr := c.cl.OwnerRewritesInputs
		if tr == "" {
			return ""
		}
		same := maps.Equal(c.j.inputs(c.j.before, tr), c.j.inputs(c.j.after, tr))
		switch {
		case c.j.owners[tr] && same:
			return fmt.Sprintf("the closing says the update of the package that owns %s rewrites its inputs, and they are as they were", tr)
		case c.j.owners[tr]:
			return c.inputsMade(tr)
		case !same:
			return fmt.Sprintf("the closing says that if no package owns %s, no update rewrites its inputs, and no registry records a package owns it, and the update changed them", tr)
		}
		says := regexp.MustCompile(regexp.QuoteMeta("the transform "+tr+", of testdata/") + `\S*, is owned by no package`)
		for _, out := range c.j.normalOut {
			if says.MatchString(out) {
				return ""
			}
		}
		return fmt.Sprintf("the closing says that if no package owns %s, the root-package check says so, and no normal run says %q", tr, says)
	},
}

// inputsMade checks that, after the update, the inputs the archive holds
// for the note's variant, of the transform tr, give the input view the
// update ran the variant's cells on: what the transform makes.
func (c *noteCheck) inputsMade(tr string) string {
	if c.n.Prefix != archive.Transform+"/"+tr+"/" {
		return fmt.Sprintf("the closing names %s's inputs, and the note is of %s", tr, c.n.Case)
	}
	var vc *archive.VariantCase
	for _, v := range c.j.after.Variants() {
		if v.Kind == archive.Transform && v.Name == tr {
			vc = &v
		}
	}
	if vc == nil || !vc.Inputs {
		return fmt.Sprintf("the closing says the update writes %s's inputs, and the archive holds none", tr)
	}
	view, ps := c.j.after.Base().InputView(*vc)
	if len(ps) > 0 {
		return fmt.Sprintf("the closing says the update writes %s's inputs, and those the archive holds are invalid: %v", tr, ps)
	}
	files, err := viewFiles(view)
	if err != nil {
		return err.Error()
	}
	ran := false
	for _, k := range slices.SortedFunc(maps.Keys(c.uc), compareCells) {
		if u := c.uc[k]; k.c == c.n.Case && u.OnValue {
			ran = true
			if !maps.Equal(u.Root, files) {
				return fmt.Sprintf("the closing says the update writes %s's inputs, which give %v, and it ran %s, in %s, on %v", tr, files, k.col, k.pkg, u.Root)
			}
		}
	}
	if !ran {
		return fmt.Sprintf("the closing says the update writes %s's inputs, and it ran no cell of the variant on a value", tr)
	}
	return ""
}

// viewFiles returns an input view's files, by path.
func viewFiles(view fs.FS) (map[string]string, error) {
	paths, err := archive.Paths(view)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, p := range paths {
		data, err := fs.ReadFile(view, p)
		if err != nil {
			return nil, err
		}
		out[p] = string(data)
	}
	return out, nil
}

// TestClaimChecks: claimChecks checks each field of a closing's claims,
// and names no other.
func TestClaimChecks(t *testing.T) {
	var fields []string
	for f := range reflect.TypeFor[hypercase.Claims]().Fields() {
		fields = append(fields, f.Name)
		if _, ok := claimChecks[f.Name]; !ok {
			t.Errorf("a closing's claims hold %s, and claimChecks has no check of it.\nGive claimChecks a check of %s against what the update does, so that TestClosings checks every claim a closing makes.", f.Name, f.Name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(claimChecks)) {
		if !slices.Contains(fields, name) {
			t.Errorf("claimChecks checks %s, which no field of a closing's claims is", name)
		}
	}
}

// sameCell reports whether the update did with a cell what the normal run
// did: the same fate, files and product, written alike, and where both ran
// it, on the same input view. A probe that fails the cell in both, alike,
// fails it alike on any value. An update writes a frozen cell only to add
// the outcome it lacks, which the normal run says.
func sameCell(n, u hypercase.Observed) bool {
	if n.Fate != u.Fate || n.Frozen != u.Frozen || !maps.Equal(n.Product, u.Product) || n.Via != u.Via || n.Ran != u.Ran {
		return false
	}
	if n.Fate == "runs" && n.OnValue && u.OnValue && !maps.Equal(n.View, u.View) {
		return false
	}
	if u.Frozen && u.Writable {
		return maps.Equal(u.Files, map[string]string{"outcome": n.Files["outcome"]})
	}
	return n.Writable == u.Writable && maps.Equal(n.Files, u.Files)
}

func compareCells(a, b cellKey) int {
	return strings.Compare(a.pkg+" "+a.c+" "+a.col, b.pkg+" "+b.c+" "+b.col)
}

// cellSections returns a cell's files and product in an archive, by
// section.
func (j *judge) cellSections(a *archive.Archive, prefix, col string) map[string]string {
	m := map[string]string{}
	for _, s := range a.Sections {
		if strings.HasPrefix(s.Name, prefix+"out/"+col+"/") || strings.HasPrefix(s.Name, prefix+"via/"+col+"/") {
			m[s.Name] = string(s.Data)
		}
	}
	return m
}

// inputs returns a transform's variant's committed inputs in an archive,
// by section.
func (j *judge) inputs(a *archive.Archive, tr string) map[string]string {
	m := map[string]string{}
	prefix := "transform/" + tr + "/"
	for _, s := range a.Sections {
		if strings.HasPrefix(s.Name, prefix+"in/") || s.Name == prefix+"delete" {
			m[s.Name] = string(s.Data)
		}
	}
	return m
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
		{"F13 evaluate errorf", insert("in/import/out/evaluate/errorf", "e\n", "in/import/out/quote/")},
		{"F14 evaluate makes no output", drop("in/import/out/evaluate/out/values.txt")},
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
		{"C20 via evaluate errorf", insert("via/import/evaluate/errorf", "stale\n", "via/import/quote/")},
		{"C21 via evaluate fatal", insert("via/import/evaluate/fatal", "stale\n", "via/import/quote/")},
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
		{"U11 inputs delete empty", insert("transform/upper/delete", "", "transform/upper/out/")},
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
	// louder: the transform's variant past import, which it declines where
	// evaluate makes no output.
	add("louder", chain, cross(editsOf(chainF, "F0", "F1", "F5", "F9", "F12", "F14"), louderC))
	// frozen: a gaps line freezes a cell of import, or past it.
	var frozenF []gridEdit
	for _, col := range []string{"import", "evaluate", "list"} {
		frozenF = append(frozenF, gapOn(col))
	}
	add("frozen", chain, cross(frozenF, editsOf(chainF, "F0", "F1", "F2", "F3", "F5", "F8", "F9", "F12"), editsOf(chainC, "C0", "C1", "C4", "C5", "C8", "C10", "C11")))
	// chain-render: import rejects quoted, and the project's RenderError
	// panics on its error, so the update writes nothing for import's cell,
	// and holds back the cells past it.
	render := closingGrid{fixture: "chain-render", archive: "testdata/export/quoted.txtar", pkgs: []childRun{{pkg: "", registration: "chain-render"}}}
	add("chain-render", render, cross([]gridEdit{
		{"R0 none", func(*archive.Archive) {}},
		{"R1 import rejects, and the renderer panics", seq(set("properties", "circular\nunterminated\n"), insert("in/import/error", "line 1: unclosed quote\n", "in/quote/"), insert("in/render/panic", "the renderer panicked\n", "via/"))},
	}))
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
	// transforms-unowned: no package owns upper, or looped. The root
	// package owned them for the first update, which wrote their variants'
	// inputs.
	unowned := closingGrid{
		fixture: "transforms", archive: "testdata/script/twice.txtar",
		remove: []string{"testdata/script/errs.txtar", "testdata/script/garbled.txtar"},
		prep: func(s *sandbox) {
			writeB(s)
			s.passes(s.update("transforms"))
		},
		pkgs: []childRun{{pkg: "", registration: "tr-unowned"}, {pkg: "b", registration: "tr-b"}},
	}
	add("transforms-unowned", unowned, cross([]gridEdit{
		{"W0 none", func(*archive.Archive) {}},
		{"W1 inputs values stale", set("transform/upper/in/do/out/values.txt", "A1 SEVEN\n")},
		{"W2 inputs delete empty", insert("transform/upper/delete", "", "transform/upper/out/")},
	}))
	return gs
}
