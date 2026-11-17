package hypercase_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/registry"
)

// TestClosings: every closing a run gives says what the update then does.
// Each grid takes a fixture whose archive one update has written, and runs
// scenarios on it: an edit of what a lift or the case makes now (the
// code's side, F) crossed with an edit of what the archive commits for a
// bytes lift, a transform's variant or a cell (the committed side, C).
// The hand-picked grids, closingGrids, each take one fixture's chain of
// lifts and its packages. The topology grids, topologyGrids, take every
// chain of up to maxChain lifts, bytes lifts or stages, with every
// assignment of its lifts and its suite to two packages, and every way the
// hand-picked grids make one lift on the chain fail, crossed with each edit
// of what the archive commits that they make.
// Each scenario runs a normal run, then an update, of each of the grid's
// packages, as go test ./... runs them, in children that record each
// cell's fate and files and whether its test failed, each run of a lift's
// function, and each note's closing and what it claims, in both runs. A
// topology grid's scenarios are variants of one archive, which each run
// reads at once.
// Where a note is of a frozen cell with cells past it, or its closing
// claims what normal runs after the update do, the scenario runs one.
// Then every note's closing must hold:
//
//   - In a normal run, a default closing, the note's own, only where the
//     update does with the cell what the normal run does: its fate, its
//     files and its product are the normal run's, and it writes them or
//     doesn't alike.
//   - In a normal run, a staged closing, which says what the update does,
//     only where the update does otherwise, with the fate the update then
//     has, in words that name it: it runs the cell, on another value than
//     the normal run's or not, blocks it by the lift that fails, writes
//     nothing for it past a lift whose cell it writes nothing for, skips it
//     for the probe's reason, or fails it with the probe's fault.
//   - In a normal run, a closing that says the package can't tell only in a
//     package that doesn't own a bytes lift on the cell's path, where every
//     closing of such a cell says so.
//   - Each of its claims, which give every sentence it says of the update,
//     holds of what the update did (claimChecks): the outcome it writes, a
//     file it writes, removes or leaves in place, nothing for the cell, the
//     lift it reruns, the cells past a frozen lift that then fail, a
//     variant's inputs it writes, rewrites or removes. An update's own
//     notes close on their defaults, and each claim one of them makes is
//     checked as a normal run's is.
//   - A note of a variant about no one cell closes on its default.
//   - No note's body mentions the update, which is its closing's to name:
//     not the update command, its variable, or the word "update", in any
//     case.
//
// In each run, a cell's test fails just where the run gives notes of the
// cell. What each update record says it wrote is checked against the
// archive after the update, and what it didn't write is unchanged. The
// grids together must reach every pair of a note's kind and the form of
// its closing that a note can give (closingPairs), and every field of a
// closing's claims, set in some note and unset in another.
func TestClosings(t *testing.T) {
	t.Parallel()
	kinds := hypercase.NoteKinds(t)
	pairs := closingPairs(t, kinds)
	fields := claimFields()
	var mu sync.Mutex
	reached := map[closingPair]string{} // each pair, and the scenario first by name that reached it
	count := map[closingPair]int{}      // how many notes give each pair
	set, unset := map[string]string{}, map[string]string{}
	total, ran := 0, 0
	first := func(m map[string]string, k, name string) {
		if was, ok := m[k]; !ok || name < was {
			m[k] = name
		}
	}
	record := func(name string, got reach) {
		mu.Lock()
		defer mu.Unlock()
		ran++
		for _, p := range got.pairs {
			count[p]++
			if first, ok := reached[p]; !ok || name < first {
				reached[p] = name
			}
		}
		for f := range got.set {
			first(set, f, name)
		}
		for f := range got.unset {
			first(unset, f, name)
		}
	}
	t.Run("grid", func(t *testing.T) {
		for _, g := range slices.Concat(closingGrids(), topologyGrids()) {
			total += len(g.scenarios)
			t.Run(g.name, func(t *testing.T) {
				t.Parallel()
				clean := g.clean(t)
				if g.packed {
					for name, got := range g.runPacked(t, clean, kinds) {
						record(g.name+"/"+name, got)
					}
					return
				}
				for _, sc := range g.scenarios {
					t.Run(sc.name, func(t *testing.T) {
						t.Parallel()
						record(g.name+"/"+sc.name, g.run(t, clean, sc, kinds))
					})
				}
			})
		}
	})
	var b strings.Builder
	for _, p := range slices.SortedFunc(maps.Keys(count), comparePairs) {
		fmt.Fprintf(&b, "\n%s %s: %d, first in %s", p.kind, p.form, count[p], reached[p])
	}
	t.Logf("the notes the %d scenarios give, by kind and closing:%s", ran, b.String())
	b.Reset()
	for _, f := range fields {
		fmt.Fprintf(&b, "\n%s: first set in %s", f, set[f])
	}
	t.Logf("the fields of the claims their closings make:%s", b.String())
	if ran < total {
		t.Logf("%d of the grids' %d scenarios ran, so the pairs and the claims they reach aren't checked.", ran, total)
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
	for _, f := range fields {
		if _, ok := set[f]; !ok {
			t.Errorf("no scenario gives a note whose closing claims %s, so no check of it runs.\nAdd a scenario that does to a grid in closingGrids.", f)
		}
		if _, ok := unset[f]; !ok {
			t.Errorf("every note the scenarios give claims %s, so none shows a closing without it.\nAdd a scenario whose note doesn't to a grid in closingGrids.", f)
		}
	}
}

// claimFields are the names of the fields of a closing's claims.
func claimFields() []string {
	var out []string
	for f := range reflect.TypeFor[hypercase.Claims]().Fields() {
		out = append(out, f.Name)
	}
	return out
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
// scenarios run on its archive. A packed grid's scenarios are hand-written
// variants of its archive, which its prep writes and the first update
// writes the cells of, each named for its scenario and then edited by it,
// so that each run reads them all at once, and each update writes the
// archive once: an update's every write of an archive syncs it to disk.
type closingGrid struct {
	name, fixture string
	remove        []string          // the fixture's archives left out
	prep          func(s *sandbox)  // what is done before the first update
	archive       string            // the archive the scenarios edit
	pkgs          []childRun        // the packages, as go test ./... runs them
	unowned       map[string]string // in a package that doesn't own a bytes lift: each column past it, "<pkg> <column>", and the lift
	scenarios     []gridScenario
	packed        bool
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

// What a scenario's notes reach: the pairs of a normal run's notes, and
// the fields of the claims some note sets, and some note leaves unset.
type reach struct {
	pairs      []closingPair
	set, unset map[string]bool
}

// run runs one scenario, and the runs after it that its closings' claims
// need, judges it, and returns what its notes reach.
func (g closingGrid) run(t *testing.T, clean map[string][]byte, sc gridScenario, kinds map[int]string) reach {
	s := newSandbox(t, "")
	for _, p := range slices.Sorted(maps.Keys(clean)) {
		s.write(p, clean[p], "the clean fixture")
	}
	for _, e := range sc.edits {
		s.edit(g.archive, e.name, e.fn)
	}
	before := archive.Parse(s.read(g.archive))
	dir := t.TempDir()
	var logs []string
	runs := func(name string, update bool) []hypercase.Observed {
		rs := g.runAll(s, update, filepath.Join(dir, name))
		var obs []hypercase.Observed
		for i, r := range rs {
			logs = append(logs, fmt.Sprintf("the %s run of %s:\n%s", name, g.pkgs[i].registration, r.out))
			obs = append(obs, readObserved(t, filepath.Join(dir, fmt.Sprintf("%s-%d.jsonl", name, i)))...)
		}
		return obs
	}
	j := &judge{g: g, t: t, kinds: kinds, before: before}
	j.normal = runs("normal", false)
	j.update = runs("update", true)
	j.u = &updated{after: archive.Parse(s.read(g.archive)), cells: cells(j.update), lifts: liftRuns(j.update), owners: g.owners(t, s)}
	if j.needsPost() {
		j.u.post = cells(runs("post", false))
	}
	j.judge()
	if t.Failed() {
		for _, l := range logs {
			t.Log(l)
		}
	}
	return j.reach
}

// runPacked runs a packed grid's scenarios, and the runs after them that
// their closings' claims need, judges each scenario on the records of its
// variant, as run judges its one, and returns what the notes of each that
// ran reach, by its name.
func (g closingGrid) runPacked(t *testing.T, clean map[string][]byte, kinds map[int]string) map[string]reach {
	s := newSandbox(t, "")
	for _, p := range slices.Sorted(maps.Keys(clean)) {
		s.write(p, clean[p], "the clean fixture")
	}
	s.edit(g.archive, "the scenarios", func(a *archive.Archive) {
		for _, sc := range g.scenarios {
			for _, e := range sc.edits {
				e.fn(a)
			}
		}
		*a = *a.Place()
		if ps := a.Check(); len(ps) > 0 {
			t.Fatalf("the scenarios make an archive with problems of form: %v", ps)
		}
	})
	before := archive.Parse(s.read(g.archive))
	dir := t.TempDir()
	runs := func(name string, update bool) []hypercase.Observed {
		var obs []hypercase.Observed
		for i, r := range g.runAll(s, update, filepath.Join(dir, name)) {
			got := readObserved(t, filepath.Join(dir, fmt.Sprintf("%s-%d.jsonl", name, i)))
			if len(got) == 0 {
				t.Fatalf("the %s run of %s recorded nothing:\n%s", name, g.pkgs[i].registration, r.out)
			}
			obs = append(obs, got...)
		}
		return obs
	}
	normal, update := runs("normal", false), runs("update", true)
	u := updated{after: archive.Parse(s.read(g.archive)), owners: g.owners(t, s)}
	base := strings.TrimSuffix(strings.TrimPrefix(g.archive, "testdata/"), ".txtar")
	judges := make([]*judge, len(g.scenarios))
	post := false
	for i, sc := range g.scenarios {
		c := base + "/" + packedPrefix(sc.name)
		j := &judge{g: g, kinds: kinds, before: before, normal: ofCase(normal, c), update: ofCase(update, c)}
		ju := u
		ju.cells, ju.lifts = cells(j.update), liftRuns(j.update)
		j.u = &ju
		post = post || j.needsPost()
		judges[i] = j
	}
	if post {
		obs := runs("post", false)
		for i, sc := range g.scenarios {
			judges[i].u.post = cells(ofCase(obs, base+"/"+packedPrefix(sc.name)))
		}
	}
	out := map[string]reach{}
	for i, sc := range g.scenarios {
		t.Run(sc.name, func(t *testing.T) {
			j := judges[i]
			j.t = t
			j.judge()
			out[sc.name] = j.reach
		})
	}
	return out
}

// packedPrefix names the variant that holds a packed grid's scenario.
func packedPrefix(scenario string) string { return "variant/" + scenario }

// ofCase returns the records of the case c.
func ofCase(os []hypercase.Observed, c string) []hypercase.Observed {
	var out []hypercase.Observed
	for _, o := range os {
		if o.Case == c {
			out = append(out, o)
		}
	}
	return out
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

// marked reports whether a gaps line marks a cell, by its case's prefix
// and its column, as the judge reads the gaps itself, apart from the
// archive package: the base's gaps marks the base's cells and, with lines
// that start with transform/<name>, a transform's variant's, and a
// hand-written variant's own gaps mark its cells.
func marked(a *archive.Archive, prefix, col string) bool {
	section, lead := "gaps", col+" "
	switch kind, name, _ := strings.Cut(strings.TrimSuffix(prefix, "/"), "/"); kind {
	case "transform":
		lead = "transform/" + name + " " + col + " "
	case "variant":
		section = prefix + "gaps"
	}
	data, _ := a.Section(section)
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, lead) {
			return true
		}
	}
	return false
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
	before         *archive.Archive
	normal, update []hypercase.Observed
	u              *updated
	reach          reach
}

// updated is what the update did, which a claim's check observes: every
// cell's record in the update, and the note's cell's, the archive after
// it, the lifts whose functions it ran, the transforms the registries it
// wrote record a package owns, and each cell's record in the normal run
// after it, where the scenario runs one.
type updated struct {
	cell   hypercase.Observed
	cells  map[cellKey]hypercase.Observed
	after  *archive.Archive
	lifts  map[liftKey]bool
	owners map[string]bool
	post   map[cellKey]hypercase.Observed
}

type cellKey struct{ pkg, c, prefix, col string }

func cells(os []hypercase.Observed) map[cellKey]hypercase.Observed {
	m := map[cellKey]hypercase.Observed{}
	for _, o := range os {
		if o.Record == "cell" {
			m[cellKey{o.Pkg, o.Case, o.Prefix, o.Column}] = o
		}
	}
	return m
}

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

// needsPost reports whether the scenario's claims need a normal run after
// the update: for a frozen note of a cell with cells past it, whose
// closing says whether they fail in normal runs, or a note that claims
// they do.
func (j *judge) needsPost() bool {
	for _, n := range slices.Concat(j.normal, j.update) {
		if n.Record == "note" && (n.Claims.PastFresh || j.kinds[n.Kind] == "frozen" && len(past(j.u.cells, n)) > 0) {
			return true
		}
	}
	return false
}

// past returns the cells past a note's cell, in its package and case, by
// the update's records: those whose path holds the cell's column.
func past(cells map[cellKey]hypercase.Observed, n hypercase.Observed) []cellKey {
	var out []cellKey
	for _, k := range slices.SortedFunc(maps.Keys(cells), compareCells) {
		if n.Column != "" && k.pkg == n.Pkg && k.c == n.Case && slices.Contains(cells[k].Path, n.Column) {
			out = append(out, k)
		}
	}
	return out
}

func (j *judge) judge() {
	t := j.t
	t.Helper()
	nc := cells(j.normal)
	fields := claimFields()
	j.reach.set, j.reach.unset = map[string]bool{}, map[string]bool{}
	for _, n := range slices.Concat(j.normal, j.update) {
		if n.Record != "note" {
			continue
		}
		key := cellKey{n.Pkg, n.Case, n.Prefix, n.Column}
		u := *j.u
		c := &noteCheck{g: j.g, n: n, cl: *n.Claims, kind: j.kinds[n.Kind], before: j.before}
		bad := func(format string, args ...any) {
			t.Errorf("%s, %s, in %s, in the %s: %s\nthe note: %s\n%s\nits claims: %+v\nthe normal run's cell: %+v\nthe update's: %+v", n.Case, n.Column, n.Pkg, runName(n.Update), fmt.Sprintf(format, args...), n.Body, n.Closing, *n.Claims, c.nr, u.cell)
		}
		if n.Column != "" {
			// A normal run's note is of a cell both runs record, and an
			// update's of one the update records: a normal run that finds a
			// variant's inputs invalid runs none of its cells.
			var nok, uok bool
			c.nr, nok = nc[key]
			u.cell, uok = j.u.cells[key]
			if !uok || !nok && !n.Update {
				bad("the normal run recorded the cell: %v, the update: %v", nok, uok)
				continue
			}
			c.cell = true
			if this := map[bool]hypercase.Observed{false: c.nr, true: u.cell}[n.Update]; this.Fate != n.This {
				bad("the note's staging has this run's fate as %q, and the cell's is %q", n.This, this.Fate)
			}
		}
		if how := updateInBody(n.Body); how != "" {
			bad("the note's body mentions the update, by %s, which is its closing's to name", how)
		}
		cl := reflect.ValueOf(c.cl)
		for _, f := range fields {
			if cl.FieldByName(f).IsZero() {
				j.reach.unset[f] = true
			} else {
				j.reach.set[f] = true
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
		for _, name := range slices.Sorted(maps.Keys(claimChecks)) {
			if msg := claimChecks[name].check(c, &u); msg != "" {
				bad("%s: %s", name, msg)
			}
		}
		if n.Update {
			// An update tells nothing of itself: each note closes on its
			// default, whose claims hold of the update, as checked.
			if form != "default" {
				bad("an update's note has a %s closing", form)
			}
			continue
		}
		if n.Column == "" && form != "default" {
			bad("a variant's note about no one cell has a %s closing", form)
		}
		if n.Declined {
			// The update declines the variant too.
			for k := range j.u.cells {
				if k.pkg == n.Pkg && k.c == n.Case {
					bad("the update runs %s's cell of the variant the normal run finds declined", k.col)
				}
			}
		}
		kind, ok := j.kinds[n.Kind]
		if !ok {
			bad("the note's kind is %d, which no noteKind is", n.Kind)
		}
		j.reach.pairs = append(j.reach.pairs, closingPair{kind, form})
	}
	j.written()
	j.failedWithNotes()
}

// updateInBody says how a note's body mentions the update, which is its
// closing's to name, or "": by the update command, its variable, or the
// word "update", in any case.
func updateInBody(body string) string {
	cmd := fixtureUpdate + "=1 go test ./..."
	switch {
	case strings.Contains(body, cmd):
		return fmt.Sprintf("the update command, %s", cmd)
	case strings.Contains(body, fixtureUpdate):
		return fmt.Sprintf("the update variable, %s", fixtureUpdate)
	case strings.Contains(strings.ToLower(body), "update"):
		return `the word "update"`
	}
	return ""
}

// failedWithNotes checks that, in each run, a cell's test fails where the
// run gives notes of the cell, which report closes, and only there.
func (j *judge) failedWithNotes() {
	for _, run := range [][]hypercase.Observed{j.normal, j.update} {
		noted := map[cellKey]bool{}
		for _, o := range run {
			if o.Record == "note" && o.Column != "" {
				noted[cellKey{o.Pkg, o.Case, o.Prefix, o.Column}] = true
			}
		}
		for _, o := range run {
			if o.Record != "cell" {
				continue
			}
			switch k := (cellKey{o.Pkg, o.Case, o.Prefix, o.Column}); {
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

// A noteCheck is what a claim's check reads of the run that gave the
// note: the note, what its closing claims, the note's kind, as NoteKinds
// names it, the archive before the runs, and, for a note of a cell, the
// cell's record in the normal run.
// What the update did is the check's other argument, so a check that
// observes nothing of it names no such argument.
type noteCheck struct {
	g      closingGrid
	n      hypercase.Observed
	cl     hypercase.Claims
	kind   string
	cell   bool
	nr     hypercase.Observed
	before *archive.Archive
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

// A claimCheck checks one field of a closing's claims against what the
// update did, u, and returns what is wrong, or "". unobserved says why
// the check observes nothing the update did, where it doesn't, and only
// there: TestClaimChecks requires it of a check that never reads u.
type claimCheck struct {
	check      func(c *noteCheck, u *updated) string
	unobserved string
}

// claimChecks check each field of a closing's claims. TestClaimChecks
// fails on a field of the claims with no check here, so no closing claims
// what TestClosings doesn't check.
var claimChecks = map[string]claimCheck{
	"Does": {check: func(c *noteCheck, u *updated) string {
		switch {
		case !c.cell && c.cl.Does != "":
			return "a variant's note about no one cell says what the update does with a cell"
		case c.n.Update && c.cl.Does != "":
			return "an update's note says what the update does otherwise than itself"
		case !c.cell || c.n.Update:
		case c.cl.Does != "" && c.cl.Does != u.cell.Fate:
			return fmt.Sprintf("the closing says the update's fate is %q, and it is %q", c.cl.Does, u.cell.Fate)
		case c.cl.Does != "" && sameCell(c.nr, u.cell):
			return "the closing says the update does otherwise with the cell than the normal run, and it does the same"
		case !c.staged() && !sameCell(c.nr, u.cell):
			return "the closing is the note's default, and the update does otherwise with the cell"
		}
		return ""
	}},
	"Another": {check: func(c *noteCheck, u *updated) string {
		want := c.cell && c.cl.Does == "runs" && c.nr.OnValue
		switch {
		case c.cl.Another != want:
			return fmt.Sprintf("the closing says the update runs the cell on another value: %v, and the update runs a cell the normal run ran on a value: %v", c.cl.Another, want)
		case c.cl.Another && maps.Equal(c.nr.View, u.cell.View):
			return "the closing says the update runs the cell on another value, and it ran the cell on the input view the normal run did"
		}
		return ""
	}},
	"Variant": {check: func(c *noteCheck, u *updated) string {
		differs := c.cell && !maps.Equal(c.nr.Root, u.cell.Root)
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
	}},
	// The lift a closing says the update reruns is the last bytes lift on
	// the cell's path whose function the package's update ran on the case,
	// as its records of each run of a lift show, and none where the
	// closing says nothing of what the update does with the cell: in its
	// default, and where the package can't tell.
	"Reruns": {check: func(c *noteCheck, u *updated) string {
		var want string
		if c.cl.Does != "" {
			for _, l := range u.cell.Path {
				if u.lifts[liftKey{c.n.Pkg, c.n.Case, l}] {
					want = l
					break
				}
			}
		}
		if c.cl.Reruns != want {
			return fmt.Sprintf("the closing says the update reruns %q, and the last bytes lift on the cell's path, %v, whose function the update ran on the case is %q", c.cl.Reruns, u.cell.Path, want)
		}
		return ""
	}},
	// Where the closing says what the update does with the cell, and the
	// update stages no variant and runs no bytes lift on the cell's path,
	// as its records of each run of a lift show, the closing names the
	// last of them, and this run's cell took what the archive commits for
	// it. An outcome other than accepted blocks the cell, by the lift a
	// blocked one names, or by the lift itself. An accepted one gives its
	// product, which is the cell's input view, or which a lift past it
	// fails on: one that isn't a bytes lift, and isn't the lift the
	// update fails before them.
	"Reads": {check: func(c *noteCheck, u *updated) string {
		var want string
		reached := slices.ContainsFunc(u.cell.Path, func(l string) bool { return u.lifts[liftKey{c.n.Pkg, c.n.Case, l}] })
		if c.cl.Does != "" && c.cl.Variant == "" && len(u.cell.Path) > 0 && !reached {
			want = u.cell.Path[0]
		}
		switch {
		case c.cl.Reads != want:
			return fmt.Sprintf("the closing says a normal run reads %q's outcome and product, where the update reaches no bytes lift on the cell's path, and the last of %v, which the update reaches none of: %v, is %q", c.cl.Reads, u.cell.Path, !reached, want)
		case want == "":
			return ""
		}
		data, _ := c.before.Section(c.n.Prefix + "out/" + want + "/" + archive.OutcomeFile)
		o, err := archive.ParseOutcome(data)
		accepted := err == nil && o.State == archive.Accepted
		product := map[string]string{}
		for _, s := range c.before.Sections {
			if p, ok := strings.CutPrefix(s.Name, c.n.Prefix+"via/"+want+"/"); ok {
				product[p] = string(s.Data)
			}
		}
		wantFate := "blocked " + want
		if err == nil && o.State == archive.Blocked {
			wantFate = "blocked " + o.Detail
		}
		blocker, blocked := strings.CutPrefix(c.nr.Fate, "blocked ")
		switch {
		case !accepted && c.nr.Fate != wantFate:
			return fmt.Sprintf("the closing says a normal run reads %s's outcome, which the archive commits as %q, and the normal run's cell's fate is %q", want, data, c.nr.Fate)
		case !accepted:
		case !blocked && !maps.Equal(c.nr.View, product):
			return fmt.Sprintf("the closing says a normal run reads %s's product, whose committed outcome is accepted, and the normal run's cell's input view isn't the committed product: %v", want, c.nr.View)
		case blocked && (slices.Contains(u.cell.Path, blocker) || c.nr.Fate == c.cl.Does):
			return fmt.Sprintf("the closing says a normal run reads %s's product, whose committed outcome is accepted, and the normal run's cell is blocked by %s, a bytes lift on its path or the lift the update fails", want, blocker)
		}
		return ""
	}},
	// Where the update writes nothing for a cell past a bytes lift whose
	// cell it writes nothing for, its fate there, a closing that says what
	// the update does with the cell names the lift, as does the update's
	// own note of the cell. The update wrote neither cell, as each of its
	// records of them, in every package, says, and the archive holds both
	// as they were.
	"Unwritten": {check: func(c *noteCheck, u *updated) string {
		var want string
		if l, ok := strings.CutPrefix(u.cell.Fate, "broken "); ok && c.cell && (c.n.Update || c.cl.Does != "") {
			want = l
		}
		switch {
		case c.cl.Unwritten != want:
			return fmt.Sprintf("the closing says the update writes nothing for the cell of the lift %q, and the lift whose cell the update's fate of the cell, %q, says it writes nothing for is %q", c.cl.Unwritten, u.cell.Fate, want)
		case want == "":
			return ""
		}
		var recs []cellKey
		for _, k := range slices.SortedFunc(maps.Keys(u.cells), compareCells) {
			if k.c == c.n.Case && k.prefix == c.n.Prefix && (k.col == want || k.col == c.n.Column && k.pkg == c.n.Pkg) {
				recs = append(recs, k)
			}
		}
		var lift bool
		for _, k := range recs {
			lift = lift || k.col == want
			if o := u.cells[k]; o.Ran && o.Writable {
				return fmt.Sprintf("the closing says the update writes nothing for the cell of the lift %s, nor for %s, and the update of %s wrote %s's", want, c.n.Column, k.pkg, k.col)
			}
		}
		for _, col := range []string{want, c.n.Column} {
			if !maps.Equal(cellSections(c.before, c.n.Prefix, col), cellSections(u.after, c.n.Prefix, col)) {
				return fmt.Sprintf("the closing says the update writes nothing for the cell of the lift %s, nor for %s, and %s's changed", want, c.n.Column, col)
			}
		}
		if !lift {
			return fmt.Sprintf("the closing says the update writes nothing for the cell of the lift %s, and no package's update records the cell", want)
		}
		return ""
	}},
	"CantTell": {check: func(c *noteCheck, _ *updated) string {
		var want string
		if c.cell && !c.n.Update {
			want = c.g.unowned[c.n.Pkg+" "+c.n.Column]
		}
		if c.cl.CantTell != want {
			return fmt.Sprintf("the closing says the package can't tell, as it doesn't own %q, and the bytes lift on the cell's path it doesn't own is %q", c.cl.CantTell, want)
		}
		return ""
	}, unobserved: "the claim is that this package doesn't own the lift, so it can't tell what the update does, and reads the lift's outcome and product from the archive: which package owns which lift is the grid's registrations, which unowned records, and no run of the update shows what a package can't tell. It says nothing the update does, but how it stages a variant, which is Variant's"},
	// The cells past a frozen lift fail in normal runs when the update
	// writes them from what the lift makes now, and a normal run reads the
	// frozen cell's committed outcome and product, which the update leaves
	// in place: a normal run after the update fails such a cell, and gives
	// it another fate, other files or another product than the update
	// wrote. A frozen note of a cell with cells past it, closing on its
	// default, says so just where they do. A staged closing says what the
	// update does with the cell, and to run it and fix what it reports: the
	// update's own note of the cell closes on its default, and says it.
	"PastFresh": {check: func(c *noteCheck, u *updated) string {
		switch {
		case !c.cell && c.cl.PastFresh:
			return "a variant's note about no one cell says the cells past it fail"
		case !c.cell, !c.cl.PastFresh && (c.kind != "frozen" || c.staged()):
			return ""
		case c.cl.PastFresh && !maps.Equal(cellSections(c.before, c.n.Prefix, c.n.Column), cellSections(u.after, c.n.Prefix, c.n.Column)):
			return "the closing says the update leaves the cell's committed outcome and product in place, and they changed"
		}
		var parted []string
		for _, k := range slices.SortedFunc(maps.Keys(u.cells), compareCells) {
			uk := u.cells[k]
			if k.pkg != c.n.Pkg || k.c != c.n.Case || !slices.Contains(uk.Path, c.n.Column) || !uk.Ran || !uk.Writable {
				continue
			}
			pk, ok := u.post[k]
			if !ok {
				return fmt.Sprintf("the normal run after the update recorded no cell of %s", k.col)
			}
			if pk.Failed && (pk.Fate != uk.Fate || !maps.Equal(pk.Files, uk.Files) || !maps.Equal(pk.Product, uk.Product)) {
				parted = append(parted, k.col)
			}
		}
		switch {
		case c.cl.PastFresh && len(parted) == 0:
			return fmt.Sprintf("the closing says the cells past %s fail in normal runs, and a normal run after the update fails none the update wrote, giving it other than the update wrote", c.n.Column)
		case !c.cl.PastFresh && len(parted) > 0:
			return fmt.Sprintf("the closing doesn't say the cells past %s fail in normal runs, and a normal run after the update fails %v, giving them other than the update wrote", c.n.Column, parted)
		}
		return ""
	}},
	"RunWrites": {check: func(c *noteCheck, u *updated) string {
		if c.cl.RunWrites && (u.cell.Fate != "runs" || !u.cell.OnValue) {
			return "the closing says what the update writes for the cell is its run's, and the update didn't run it"
		}
		return ""
	}},
	// That the update's run may still write the file rests on the update
	// running the cell on another value than the normal run's, which the
	// check observes. That it may is all the claim says: whether the run
	// writes the file or not, no observation refutes it.
	"MayWrite": {check: func(c *noteCheck, u *updated) string {
		switch {
		case c.cl.MayWrite == "":
		case c.cl.MayWrite != c.n.File:
			return fmt.Sprintf("the closing says the update's run may still write %s, and the note is about %s", c.cl.MayWrite, c.n.File)
		case u.cell.Fate != "runs" || !u.cell.OnValue || maps.Equal(c.nr.View, u.cell.View):
			return fmt.Sprintf("the closing says the update's run of the cell may still write the file, and the update didn't run the cell on another value than the normal run: its fate is %q, on a value: %v, on the normal run's view: %v", u.cell.Fate, u.cell.OnValue, maps.Equal(c.nr.View, u.cell.View))
		}
		return ""
	}},
	"Outcome": {check: func(c *noteCheck, u *updated) string {
		if c.cl.Outcome == "" {
			return ""
		}
		if got, _ := section(u.after, c.out("outcome")); got != c.cl.Outcome+"\n" {
			return fmt.Sprintf("the closing says the update writes the outcome %q, and the archive has %q", c.cl.Outcome, got)
		}
		return ""
	}},
	"Differs": {check: func(c *noteCheck, u *updated) string {
		// What the closing says the update writes, if it says.
		var writes string
		switch {
		case c.cl.Outcome != "":
			writes = c.cl.Outcome + "\n"
		case c.cl.RunWrites && u.cell.Ran && u.cell.Writable:
			writes = u.cell.Files["outcome"]
		}
		this, ok := c.nr.Files["outcome"]
		switch {
		case c.cl.Differs && writes != "" && (!ok || this == writes):
			return fmt.Sprintf("the closing says the outcome the update writes, %q, isn't this run's, which is %q", writes, this)
		case !c.cl.Differs && c.cl.Outcome != "" && ok && this != writes:
			return fmt.Sprintf("the closing doesn't say the outcome the update writes, %q, isn't this run's, which is %q", writes, this)
		}
		return ""
	}},
	"Panic": {check: func(c *noteCheck, u *updated) string {
		if !c.cl.Panic {
			return ""
		}
		_, v, ok := strings.Cut(u.cell.Fate, "'s probe panics: ")
		got, _ := section(u.after, c.out("error.txt"))
		switch {
		case c.cl.Outcome != "panicked":
			return fmt.Sprintf("the closing says the update writes a panic, and the outcome %q", c.cl.Outcome)
		case !ok:
			return fmt.Sprintf("the closing says the update writes the probe's panic, and its fate is %q", u.cell.Fate)
		case got != v+"\n":
			return fmt.Sprintf("the closing says the update writes the panic %q in error.txt, and the archive has %q", v, got)
		}
		return ""
	}},
	"Nothing": {check: func(c *noteCheck, u *updated) string {
		switch {
		case !c.cl.Nothing:
		case !c.cell:
			return "a variant's note about no one cell says the update writes nothing for a cell"
		case u.cell.Ran && u.cell.Writable:
			return "the closing says the update writes nothing for the cell, and it records that it wrote it"
		case !maps.Equal(cellSections(c.before, c.n.Prefix, c.n.Column), cellSections(u.after, c.n.Prefix, c.n.Column)):
			return "the closing says the update writes nothing for the cell, and it changed"
		}
		return ""
	}},
	// A gaps line freezes a cell that it marks, as the judge reads the
	// gaps itself, and that has committed files: the update leaves each of
	// them as it was.
	"Frozen": {check: func(c *noteCheck, u *updated) string {
		if !c.cl.Frozen {
			return ""
		}
		was := cellSections(c.before, c.n.Prefix, c.n.Column)
		switch {
		case !c.cell:
			return "a variant's note about no one cell says a gaps line freezes a cell"
		case !marked(c.before, c.n.Prefix, c.n.Column):
			return "the closing says a gaps line freezes the cell, and no gaps line marks it"
		case len(was) == 0:
			return "the closing says a gaps line freezes the cell, and the cell has no committed files"
		case !maps.Equal(was, cellSections(u.after, c.n.Prefix, c.n.Column)):
			return "the closing says a gaps line freezes the cell, and the update changed it"
		}
		return ""
	}},
	"Writes": {check: func(c *noteCheck, u *updated) string {
		if c.cl.Writes == "" {
			return ""
		}
		got, ok := section(u.after, c.cl.Writes)
		// A file of the cell's, or of its bytes lift's product.
		want, made := "", false
		if rel, ok := strings.CutPrefix(c.cl.Writes, c.n.Prefix+"out/"+c.n.Column+"/"); ok {
			want, made = u.cell.Files[rel]
		} else if rel, ok := strings.CutPrefix(c.cl.Writes, c.n.Prefix+"via/"+c.n.Column+"/"); ok && u.cell.Via {
			want, made = u.cell.Product[rel]
		}
		if !ok || !made || !u.cell.Ran || !u.cell.Writable || got != want {
			return fmt.Sprintf("the closing says the update writes %s, and the archive has %q", c.cl.Writes, got)
		}
		return ""
	}},
	"OnlyOutcome": {check: func(c *noteCheck, u *updated) string {
		if c.cl.OnlyOutcome == "" {
			return ""
		}
		was, is := cellSections(c.before, c.n.Prefix, c.n.Column), cellSections(u.after, c.n.Prefix, c.n.Column)
		_, had := was[c.cl.OnlyOutcome]
		was[c.cl.OnlyOutcome] = u.cell.Files["outcome"]
		if had || c.cl.OnlyOutcome != c.out("outcome") || !maps.Equal(was, is) {
			return fmt.Sprintf("the closing says the update adds %s and changes nothing else of the cell, and the cell had it: %v, or the cell is %v", c.cl.OnlyOutcome, had, is)
		}
		return ""
	}},
	"Leaves": {check: func(c *noteCheck, u *updated) string {
		if c.cl.Leaves == "" {
			return ""
		}
		was, ok := section(c.before, c.cl.Leaves)
		is, still := section(u.after, c.cl.Leaves)
		if !ok || !still || was != is {
			return fmt.Sprintf("the closing says the update leaves %s in place, and the archive had it: %v, and has it as it was: %v", c.cl.Leaves, ok, still && was == is)
		}
		return ""
	}},
	"Removes": {check: func(c *noteCheck, u *updated) string {
		if c.cl.Removes == "" {
			return ""
		}
		_, ok := section(c.before, c.cl.Removes)
		if _, still := section(u.after, c.cl.Removes); !ok || still {
			return fmt.Sprintf("the closing says the update removes %s, and the archive had it: %v, and still has it: %v", c.cl.Removes, ok, still)
		}
		return ""
	}},
	"RemovesInputs": {check: func(c *noteCheck, u *updated) string {
		if c.cl.RemovesInputs == "" {
			return ""
		}
		if was, is := inputs(c.before, c.cl.RemovesInputs), inputs(u.after, c.cl.RemovesInputs); len(was) == 0 || len(is) > 0 {
			return fmt.Sprintf("the closing says the update removes %s's inputs, and the archive had %v, and has %v", c.cl.RemovesInputs, slices.Sorted(maps.Keys(was)), slices.Sorted(maps.Keys(is)))
		}
		return ""
	}},
	"WritesInputs": {check: func(c *noteCheck, u *updated) string {
		if c.cl.WritesInputs == "" {
			return ""
		}
		if was := inputs(c.before, c.cl.WritesInputs); len(was) > 0 {
			return fmt.Sprintf("the closing says the update writes %s's inputs, which the archive lacks, and it had %v", c.cl.WritesInputs, slices.Sorted(maps.Keys(was)))
		}
		return inputsMade(c, u, c.cl.WritesInputs)
	}},
	// The package that owns the transform rewrites its variant's inputs,
	// and its registry, after the update, records that it owns it.
	"RewritesInputs": {check: func(c *noteCheck, u *updated) string {
		tr := c.cl.RewritesInputs
		switch {
		case tr == "":
			return ""
		case !u.owners[tr]:
			return fmt.Sprintf("the closing says this package's update rewrites %s's inputs, and no registry records a package owns it", tr)
		case maps.Equal(inputs(c.before, tr), inputs(u.after, tr)):
			return fmt.Sprintf("the closing says this package's update rewrites %s's inputs, and they are as they were", tr)
		}
		return inputsMade(c, u, tr)
	}},
	// In a package that doesn't own the transform, the closing says the
	// owner's update rewrites the inputs, and, if no package owns it, that
	// no update rewrites them: which holds is what the registries the
	// update wrote record.
	"OwnerRewritesInputs": {check: func(c *noteCheck, u *updated) string {
		tr := c.cl.OwnerRewritesInputs
		if tr == "" {
			return ""
		}
		same := maps.Equal(inputs(c.before, tr), inputs(u.after, tr))
		switch {
		case u.owners[tr] && same:
			return fmt.Sprintf("the closing says the update of the package that owns %s rewrites its inputs, and they are as they were", tr)
		case u.owners[tr]:
			return inputsMade(c, u, tr)
		case !same:
			return fmt.Sprintf("the closing says that if no package owns %s, no update rewrites its inputs, and no registry records a package owns it, and the update changed them", tr)
		}
		return ""
	}},
}

// inputsMade checks that, after the update, the inputs the archive holds
// for the note's variant, of the transform tr, give the input view the
// update ran the variant's cells on: what the transform makes.
func inputsMade(c *noteCheck, u *updated, tr string) string {
	if c.n.Prefix != archive.Transform+"/"+tr+"/" {
		return fmt.Sprintf("the closing names %s's inputs, and the note is of %s", tr, c.n.Case)
	}
	var vc *archive.VariantCase
	for _, v := range u.after.Variants() {
		if v.Kind == archive.Transform && v.Name == tr {
			vc = &v
		}
	}
	if vc == nil || !vc.Inputs {
		return fmt.Sprintf("the closing says the update writes %s's inputs, and the archive holds none", tr)
	}
	view, ps := u.after.Base().InputView(*vc)
	if len(ps) > 0 {
		return fmt.Sprintf("the closing says the update writes %s's inputs, and those the archive holds are invalid: %v", tr, ps)
	}
	files, err := viewFiles(view)
	if err != nil {
		return err.Error()
	}
	ran := false
	for _, k := range slices.SortedFunc(maps.Keys(u.cells), compareCells) {
		if uk := u.cells[k]; k.c == c.n.Case && uk.OnValue {
			ran = true
			if !maps.Equal(uk.Root, files) {
				return fmt.Sprintf("the closing says the update writes %s's inputs, which give %v, and it ran %s, in %s, on %v", tr, files, k.col, k.pkg, uk.Root)
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
// and names no other. A check observes what the update did through its
// second argument alone, so one that names it _, or never reads it, as
// reads tells, observes nothing the update did: it, and only it, says why
// in unobserved. The type checker reads the checks' source in this file.
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
	observes := checksObserving(t)
	for _, name := range slices.Sorted(maps.Keys(claimChecks)) {
		o, ok := observes[name]
		unobserved := strings.TrimSpace(claimChecks[name].unobserved)
		switch {
		case !ok:
			t.Errorf("claimChecks' check of %s is no func literal in claimChecks' source, so TestClaimChecks can't tell what it observes", name)
		case !o && unobserved == "":
			t.Errorf("claimChecks' check of %s observes nothing the update did: it never reads its second argument.\nCheck the claim against what the update did, or say in unobserved why no observation of the update can check it.", name)
		case o && unobserved != "":
			t.Errorf("claimChecks' check of %s says it observes nothing the update did, and reads its second argument", name)
		}
	}
}

// checksObserving reports, for each check of claimChecks, by its field,
// whether its func literal reads its second parameter, what the update
// did, as reads tells, in claimChecks' source in this file, which the type
// checker reads with the rest of this package.
func checksObserving(t *testing.T) map[string]bool {
	t.Helper()
	tf := xtestTypes(t)
	out := map[string]bool{}
	for _, f := range tf.files {
		if filepath.Base(tf.fset.Position(f.Pos()).Filename) != "closings_test.go" {
			continue
		}
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, sp := range gd.Specs {
				vs := sp.(*ast.ValueSpec)
				if len(vs.Names) != 1 || vs.Names[0].Name != "claimChecks" || len(vs.Values) != 1 {
					continue
				}
				for _, el := range vs.Values[0].(*ast.CompositeLit).Elts {
					kv := el.(*ast.KeyValueExpr)
					name, err := strconv.Unquote(kv.Key.(*ast.BasicLit).Value)
					if err != nil {
						t.Fatal(err)
					}
					for _, fel := range kv.Value.(*ast.CompositeLit).Elts {
						fkv, ok := fel.(*ast.KeyValueExpr)
						if !ok || fkv.Key.(*ast.Ident).Name != "check" {
							continue
						}
						if lit, ok := fkv.Value.(*ast.FuncLit); ok {
							out[name] = readsParam(tf.info, lit, 1)
						}
					}
				}
			}
		}
	}
	return out
}

// readsParam reports whether a func literal's body reads its parameter i,
// as reads tells. A parameter with no name, or named _, is read nowhere.
func readsParam(info *types.Info, lit *ast.FuncLit, i int) bool {
	var params []*ast.Ident
	for _, p := range lit.Type.Params.List {
		params = append(params, p.Names...)
	}
	if i >= len(params) || params[i].Name == "_" {
		return false
	}
	return reads(info, lit.Body, info.Defs[params[i]])
}

// reads reports whether body reads the object v, as the type checker
// resolves each name in it, so a name that shadows v is another object:
// whether it names v otherwise than as what an assignment assigns to,
// directly or through its fields, elements or pointer, and otherwise than
// in what an assignment to blank identifiers alone assigns, which keeps
// nothing it reads.
func reads(info *types.Info, body ast.Node, v types.Object) bool {
	read := false
	var stack []ast.Node
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if id, ok := n.(*ast.Ident); ok && info.Uses[id] == v && !discarded(stack, id) {
			read = true
		}
		stack = append(stack, n)
		return true
	})
	return read
}

// discarded reports whether an identifier, under the nodes of stack,
// outermost first, is what an assignment assigns to, or what it reaches
// through, or is in what an assignment to blank identifiers alone
// assigns.
func discarded(stack []ast.Node, id *ast.Ident) bool {
	var child ast.Node = id
	target := true // child is id, or what id reaches into
	for i := len(stack) - 1; i >= 0; i-- {
		switch p := stack[i].(type) {
		case *ast.AssignStmt:
			if target && slices.Contains(p.Lhs, child.(ast.Expr)) {
				return true
			}
			return slices.IndexFunc(p.Lhs, func(e ast.Expr) bool { return !isBlank(e) }) < 0 && slices.Contains(p.Rhs, child.(ast.Expr))
		case *ast.IncDecStmt:
			return target
		case *ast.ValueSpec:
			return slices.IndexFunc(p.Names, func(n *ast.Ident) bool { return n.Name != "_" }) < 0
		case *ast.SelectorExpr:
			target = target && p.X == child
		case *ast.IndexExpr:
			target = target && p.X == child
		case *ast.StarExpr, *ast.ParenExpr:
		default:
			target = false
		}
		child = stack[i]
	}
	return false
}

func isBlank(e ast.Expr) bool {
	id, ok := ast.Unparen(e).(*ast.Ident)
	return ok && id.Name == "_"
}

// TestReads: whether a claim's check observes what the update did is
// whether its func literal reads its second parameter, which reads tells:
// a name that shadows the parameter is another object, and an assignment
// to it, or to blank identifiers alone, reads nothing it keeps.
func TestReads(t *testing.T) {
	t.Parallel()
	const src = `package p

type updated struct{ n int }

var sink int

var checks = map[string]func(c int, u *updated) string{
	"a field":        func(c int, u *updated) string { sink = u.n; return "" },
	"an index":       func(c int, u *updated) string { var m map[int]int; m[u.n] = c; return "" },
	"a target's":     func(c int, u *updated) string { var m map[int]int; m[c] = u.n; return "" },
	"nothing":        func(c int, u *updated) string { return "" },
	"blank":          func(c int, u *updated) string { _ = u; return "" },
	"blank field":    func(c int, u *updated) string { _, _ = u.n, c; return "" },
	"blank var":      func(c int, u *updated) string { var _ = u.n; return "" },
	"shadowed":       func(c int, u *updated) string { if c > 0 { u := &updated{}; sink = u.n }; return "" },
	"a literal's":    func(c int, u *updated) string { return func(u *updated) string { sink = u.n; return "" }(nil) },
	"assigned":       func(c int, u *updated) string { u = nil; return "" },
	"field assigned": func(c int, u *updated) string { u.n = c; return "" },
	"incremented":    func(c int, u *updated) string { u.n++; return "" },
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}}
	if _, err := (&types.Config{}).Check("p", fset, []*ast.File{f}, info); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"a field": true, "an index": true, "a target's": true}
	got := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if kv, ok := n.(*ast.KeyValueExpr); ok {
			name, _ := strconv.Unquote(kv.Key.(*ast.BasicLit).Value)
			if readsParam(info, kv.Value.(*ast.FuncLit), 1) {
				got[name] = true
			}
		}
		return true
	})
	if !maps.Equal(got, want) {
		t.Errorf("the checks that read u are %v, want %v", slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)))
	}
}

// typedFiles are a package's files, as the type checker reads them.
type typedFiles struct {
	fset  *token.FileSet
	files []*ast.File
	info  *types.Info
}

// xtestTypes type-checks this package, the external tests, against its
// imports' export data, which go list builds, the package's own test
// build among them, as the package's tests type-check its source.
func xtestTypes(t *testing.T) typedFiles {
	t.Helper()
	cmd := exec.Command("go", "list", "-test", "-export", "-deps", "-json=ImportPath,Name,Dir,GoFiles,Export,ImportMap,ForTest", ".")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, stderr.Bytes())
	}
	type listed struct {
		ImportPath, Name, Dir, Export, ForTest string
		GoFiles                                []string
		ImportMap                              map[string]string
	}
	exports := map[string]string{}
	var self *listed
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p listed
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		exports[p.ImportPath] = p.Export
		if p.Name == "hypercase_test" && p.ForTest != "" {
			self = &p
		}
	}
	if self == nil {
		t.Fatal("go list gave no external test package")
	}
	tf := typedFiles{fset: token.NewFileSet(), info: &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}}}
	for _, name := range self.GoFiles {
		f, err := parser.ParseFile(tf.fset, filepath.Join(self.Dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		tf.files = append(tf.files, f)
	}
	imp := importer.ForCompiler(tf.fset, "gc", func(path string) (io.ReadCloser, error) {
		if m, ok := self.ImportMap[path]; ok {
			path = m
		}
		if exports[path] == "" {
			return nil, fmt.Errorf("go list gave no export data for %s", path)
		}
		return os.Open(exports[path])
	})
	if _, err := (&types.Config{Importer: imp}).Check(self.ImportPath, tf.fset, tf.files, tf.info); err != nil {
		t.Fatalf("type-checking the external tests: %v", err)
	}
	return tf
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
func cellSections(a *archive.Archive, prefix, col string) map[string]string {
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
func inputs(a *archive.Archive, tr string) map[string]string {
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
func (j *judge) written() {
	t := j.t
	t.Helper()
	for _, k := range slices.SortedFunc(maps.Keys(j.u.cells), func(a, b cellKey) int { return strings.Compare(a.c+" "+a.col, b.c+" "+b.col) }) {
		ur := j.u.cells[k]
		prefix := ur.Prefix
		if !ur.Ran || !ur.Writable {
			if !maps.Equal(cellSections(j.before, prefix, k.col), cellSections(j.u.after, prefix, k.col)) {
				t.Errorf("%s, %s, in %s: the update records it wrote nothing for the cell, and the cell changed: %+v", k.c, k.col, k.pkg, ur)
			}
			continue
		}
		if msg := holdsCell(j.u.after, ur); msg != "" {
			t.Errorf("%s, %s, in %s: %s", k.c, k.col, k.pkg, msg)
		}
	}
}

// holdsCell says how an archive doesn't hold what an update records it
// wrote for a cell: its files, and a bytes lift's product, or "".
func holdsCell(a *archive.Archive, ur hypercase.Observed) string {
	for _, p := range slices.Sorted(maps.Keys(ur.Files)) {
		data := ur.Files[p]
		if got, ok := a.Section(ur.Prefix + "out/" + ur.Column + "/" + p); !ok || string(got) != data {
			return fmt.Sprintf("the update records it wrote %s as %q, and the archive has %q", p, data, got)
		}
	}
	if ur.Via {
		got := map[string]string{}
		for _, s := range a.Sections {
			if rest, ok := strings.CutPrefix(s.Name, ur.Prefix+"via/"+ur.Column+"/"); ok {
				got[rest] = string(s.Data)
			}
		}
		want := ur.Product
		if want == nil {
			want = map[string]string{}
		}
		if !maps.Equal(got, want) {
			return fmt.Sprintf("the update records it wrote the product %v, and the archive has %v", want, got)
		}
	}
	return ""
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
		{"V11 no inputs", drop("transform/louder/in/")},
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

// gapNoOutcome marks a column's cell of the base with a gaps line, and
// removes the outcome it commits.
func gapNoOutcome(col string) gridEdit {
	return gridEdit{"gap " + col + ", no outcome", seq(gapOn(col).fn, drop("out/"+col+"/outcome"))}
}

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
	// frozen-outcome: a gaps line freezes a cell that commits no outcome.
	add("frozen-outcome", chain, cross([]gridEdit{gapNoOutcome("evaluate"), gapNoOutcome("import")}, editsOf(chainF, "F0", "F9")))
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
	inputsEdits := []gridEdit{
		{"W0 none", func(*archive.Archive) {}},
		{"W1 inputs values stale", set("transform/upper/in/do/out/values.txt", "A1 SEVEN\n")},
		{"W2 inputs delete empty", insert("transform/upper/delete", "", "transform/upper/out/")},
	}
	add("transforms-unowned", unowned, cross(inputsEdits))
	// transforms-unowned-noroot: transforms-unowned, where no package
	// declares the root package, so no root-package check runs.
	noroot := unowned
	noroot.pkgs = []childRun{{pkg: "", registration: "tr-unowned-noroot"}, {pkg: "b", registration: "tr-b"}}
	add("transforms-unowned-noroot", noroot, cross(inputsEdits))
	return gs
}
