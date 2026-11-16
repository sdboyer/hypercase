package hypercase

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/diff"
	"github.com/sdboyer/hypercase/internal/registry"
	"github.com/sdboyer/hypercase/internal/root"
)

// Test runs every case through every column the package registered, as
// subtests of t: one per corpus, one per case within it, named for its
// archive, and one per cell within that, named for its column. So
// TestCorpus/recipe/pancakes/scale is the cell of the case recipe/pancakes
// in the column scale. In the package that registers [RootPackage], Test
// first runs the root-package check, on t itself.
//
// Without the project's update variable, Test only reads: each output is
// compared with its golden, and the package's registry with its own. With
// it set to 1, Test writes them.
func Test(t *testing.T) {
	t.Helper()
	reg.mu.Lock()
	reg.started = true
	reg.mu.Unlock()

	r, ok := newRunner(t)
	if !ok {
		return
	}
	r.checkDescriptions(t)
	r.checkRegistry(t)
	found, err := registry.Find(r.moduleRoot)
	if err != nil {
		t.Errorf("finding the module's registries: %v", err)
	}
	if !r.update {
		r.requireRootCheck(t, found)
	}
	if r.rootPackage {
		if r.update {
			t.Logf("The root-package check for %s runs in normal runs, so it runs after the update: go test ./...", r.rootName())
		} else {
			r.rootCheck(t, found)
		}
	}
	r.runCorpora(t)
	r.appliesToNoCase(t)
	r.runChecks(t, found)
}

// runner is one call of Test.
type runner struct {
	update      bool
	updateVar   string
	renderError func(in fs.FS, err error) []byte
	codes       func(err error) []string
	issueLinks  []decl.IssueLink
	moduleRoot  string
	pkgDir      string
	rootRel     string // the corpus root, relative to the module root
	corpusRoot  string
	rootPackage bool
	properties  []decl.Property
	types       []*corpusType
	lifts       []*lift             // owned and used
	columns     []*column           // sorted by name
	plans       []*plan             // one per corpus type and per suite's Corpus, by directory
	corpora     map[string][]string // each lift's and suite's corpus directories, sorted
	stats       map[string]*colStats
	owned       map[*transform]bool // the types' transforms the package owns
	checks      []Check             // sorted by name
}

// colStats counts a suite's cells, for the check that it applies to some
// case.
type colStats struct {
	total   int // the cells the suite has: its corpora's archives whose cells run
	ran     int // those that ran, which -run can make fewer
	applied int // those the suite's probe said were for it
}

func newRunner(t *testing.T) (*runner, bool) {
	t.Helper()
	reg.mu.Lock()
	project, properties, columns := reg.project, slices.Clone(reg.properties), slices.Clone(reg.columns)
	types, lifts := slices.Clone(reg.types), slices.Clone(reg.lifts)
	ownedTransforms, checks := slices.Clone(reg.ownedTransforms), slices.Clone(reg.checks)
	rootRel, rootPackage := reg.root, reg.rootPackage
	reg.mu.Unlock()
	if project == nil {
		t.Errorf("the package registered no decl.Project.\nRegister the project, with its UpdateVar, from an init function.")
		return nil, false
	}
	r := &runner{
		updateVar:   project.UpdateVar,
		renderError: project.RenderError,
		codes:       project.Codes,
		issueLinks:  slices.Clone(project.IssueLinks),
		rootRel:     rootRel,
		rootPackage: rootPackage,
		properties:  properties,
		types:       types,
		lifts:       lifts,
		columns:     columns,
		checks:      checks,
		corpora:     map[string][]string{},
		stats:       map[string]*colStats{},
	}
	slices.SortFunc(r.columns, func(a, b *column) int { return strings.Compare(a.name, b.name) })
	slices.SortFunc(r.checks, func(a, b Check) int { return strings.Compare(a.Name, b.Name) })
	for _, c := range r.columns {
		r.stats[c.name] = &colStats{}
	}
	switch v := os.Getenv(project.UpdateVar); v {
	case "":
	case "1":
		r.update = true
	default:
		t.Errorf("%s is set to %q.\nSet it to 1 to run an update, or unset it.", project.UpdateVar, v)
		return nil, false
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Errorf("finding the package's directory: %v", err)
		return nil, false
	}
	r.pkgDir = wd
	if r.moduleRoot, err = root.Module(wd); err != nil {
		t.Errorf("finding the module root: %v", err)
		return nil, false
	}
	r.corpusRoot = filepath.Join(r.moduleRoot, filepath.FromSlash(rootRel))
	if rootPackage && rootRel == "." && r.pkgDir != r.moduleRoot {
		t.Errorf("the package registers the root-package check for the module-root corpus, and is not the module's root package, %s.\nRegister hypercase.RootPackage() in the module's root package, or pass this package's corpus root with hypercase.Root.", r.moduleRoot)
		return nil, false
	}
	related := r.checkRelations(t, ownedTransforms)
	return r, r.planPaths(t) && related
}

// checkRelations checks what relates two registrations, once
// initialization is over: each property a column rejects is registered,
// and so is each a transform gives or keeps, and each transform the
// package owns is one a corpus type it registers lists.
func (r *runner) checkRelations(t *testing.T, ownedTransforms []*transform) bool {
	t.Helper()
	ok := true
	for _, c := range r.columns {
		ok = checkRegistered(t, fmt.Sprintf("the %s %s", c.kind(), c.name), "rejects", c.name+"'s Rejects", c.rejects) && ok
	}
	ok = r.checkTransformRelations(t, ownedTransforms) && ok
	if !ok {
		t.Log("No case runs until the registrations agree.")
	}
	return ok
}

// checkRegistered checks that each property of one of a registration's
// lists is one the package registers, as it registers it: what names the
// registration, as "the suite echo", verb says what it does with the
// list's properties, as "rejects", and list names the list, as "echo's
// Rejects".
func checkRegistered(t *testing.T, what, verb, list string, ps []decl.Property) bool {
	t.Helper()
	ok := true
	for _, p := range ps {
		q, found := reg.property(p.Name)
		switch {
		case !found:
			t.Errorf("%s %s %s, and the package registers no property of that name.\nRegister the property, or remove it from %s.", what, verb, p.Name, list)
			ok = false
		case q != p:
			t.Errorf("%s %s a property named %s, which the package registers with another description.\nKeep one declaration of %s, with one description: register it, and list it in %s.", what, verb, p.Name, p.Name, list)
			ok = false
		}
	}
	return ok
}

// updateCmd is the command that runs an update.
func (r *runner) updateCmd() string {
	return r.updateVar + "=1 go test ./..."
}

// rel returns a path relative to the module root, with slashes, so a
// message reads the same wherever go test ran.
func (r *runner) rel(p string) string { return root.Rel(r.moduleRoot, p) }

// rootName names the corpus root in a message.
func (r *runner) rootName() string {
	if r.rootRel == "." {
		return "the module-root corpus"
	}
	return "the corpus root " + r.rootRel
}

func (r *runner) lockPath() string {
	return filepath.Join(r.corpusRoot, "testdata", archive.LockFile)
}

func (r *runner) checkDescriptions(t *testing.T) {
	t.Helper()
	for _, p := range r.properties {
		if strings.TrimSpace(p.Description) == "" {
			t.Errorf("the property %s has no description.\nAdd one: it says what the fact about a case is, in prose.", p.Name)
		}
	}
	for _, ct := range r.types {
		if strings.TrimSpace(ct.description) == "" {
			t.Errorf("the corpus type %s, of testdata/%s/, has no description.\nAdd one: it says what the type's inputs are, in prose.", ct.goType, ct.dir)
		}
		for _, tr := range ct.transforms {
			if strings.TrimSpace(tr.description) == "" {
				t.Errorf("the transform %s, of the corpus type %s, has no description.\nAdd one: it says what the transform changes, in prose, and it describes each variant the transform makes.", tr.name, ct.goType)
			}
		}
	}
	for _, l := range r.lifts {
		if !l.owned {
			r.checkColumnDescriptions(t, "lift", l.name, l.description, l.outputs)
		}
	}
	for _, c := range r.columns {
		r.checkColumnDescriptions(t, c.kind(), c.name, c.description, c.outputs)
		if cl := c.classifier; cl != nil && strings.TrimSpace(cl.description) == "" {
			t.Errorf("the suite %s's classifier %s has no description.\nAdd one: it says what the classifier's details mean, in prose.", c.name, cl.name)
		}
	}
	for _, c := range r.checks {
		if strings.TrimSpace(c.Description) == "" {
			t.Errorf("the corpus check %s has no description.\nAdd one: it says what the check asserts, in prose.", c.Name)
		}
	}
}

func (r *runner) checkColumnDescriptions(t *testing.T, kind, name, description string, outputs []decl.Output) {
	t.Helper()
	if strings.TrimSpace(description) == "" {
		t.Errorf("the %s %s has no description.\nAdd one: it says what the %s does, in prose.", kind, name, kind)
	}
	for _, o := range outputs {
		if strings.TrimSpace(o.Description) == "" {
			t.Errorf("the %s %s declares the output %s with no description.\nAdd one: it says what the files hold, in prose.", kind, name, o.Pattern)
		}
	}
}

// requireRootCheck fails unless the registry of some package with tests
// records the root-package check for the package's corpus root, since
// nothing else reports what no column owns. A registry in a package with
// no _test.go file records a check no test runs, as the root-package check
// itself holds. Updates write registries, so only a normal run checks.
func (r *runner) requireRootCheck(t *testing.T, found []registry.Found) {
	t.Helper()
	var stale []registry.Found
	for _, f := range found {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			continue
		}
		if h, err := registry.Peek(data); err == nil && h.CorpusRoot == r.rootRel && h.RootPackage {
			if hasTestFile(f.Pkg) {
				return
			}
			stale = append(stale, f)
		}
	}
	if len(stale) == 0 {
		t.Errorf("no registry records a root-package check for %s, so nothing reports what no column there owns.\nRegister hypercase.RootPackage() in one package over it, then run %s to record it in the package's registry.", r.rootName(), r.updateCmd())
		return
	}
	for _, f := range stale {
		t.Errorf("%s records the root-package check for %s, and %s holds no _test.go file, so no test runs the check, and nothing reports what no column there owns.\nRestore the package's tests, or register hypercase.RootPackage() in another package over it, then run %s to record it in that package's registry.", r.rel(f.Path), r.rootName(), r.rel(f.Pkg), r.updateCmd())
	}
}

// A plannedCase is an archive whose cells run, with its variants.
type plannedCase struct {
	name     string // the archive's name
	path     string
	data     []byte
	a        *archive.Archive
	base     archive.Base // read once, for the base's cells and the variants'
	variants []*plannedVariant
}

// cases counts the archive's cases whose cells run: its base, and each
// variant a transform doesn't decline or fail on.
func (pc plannedCase) cases() int {
	n := 1
	for _, v := range pc.variants {
		if v.runs() {
			n++
		}
	}
	return n
}

// runCorpora runs the cases of each corpus type the package's columns run
// over, or whose transforms it owns. It reads every archive first, outside
// the subtests, and makes each transform's variants, so the count of each
// suite's cells doesn't depend on -run.
func (r *runner) runCorpora(t *testing.T) {
	t.Helper()
	for _, p := range r.plans {
		if len(p.cols) == 0 && !slices.ContainsFunc(p.root.transforms, func(tr *transform) bool { return r.owned[tr] }) {
			continue
		}
		cases := r.planCorpus(t, p)
		for _, col := range p.cols {
			if col.lift == nil {
				for _, pc := range cases {
					r.stats[col.name].total += pc.cases()
				}
			}
		}
		t.Run(p.root.dir, func(t *testing.T) {
			for _, pc := range cases {
				t.Run(pc.name, func(t *testing.T) { r.runCase(t, p, pc) })
			}
		})
	}
}

// planCorpus reads a corpus's archives. One the package can't run is left
// out, with a line in the log: the root-package check reports it.
func (r *runner) planCorpus(t *testing.T, p *plan) []plannedCase {
	t.Helper()
	dir := filepath.Join(r.corpusRoot, "testdata", p.root.dir)
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("reading %s: %v", r.rel(dir), err)
		return nil
	}
	var cases []plannedCase
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txtar") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		name := strings.TrimSuffix(e.Name(), ".txtar")
		if !validName(name) {
			t.Logf("%s: the archive's name breaks the naming rule, so none of its cells run. The root-package check reports it.", r.rel(path))
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("reading %s: %v", r.rel(path), err)
			continue
		}
		a := archive.Parse(data)
		if len(a.Check()) > 0 {
			t.Logf("%s has problems of form, so none of its cells run, and no update writes it. The root-package check reports them.", r.rel(path))
			continue
		}
		base := a.Base()
		cases = append(cases, plannedCase{name: name, path: path, data: data, a: a, base: base, variants: r.planVariants(p, a, base)})
	}
	return cases
}

// aCase is one case: an archive's base, or one of its variants.
type aCase struct {
	name       string // <corpus>/<archive>, then /variant/<name> or /transform/<name> for a variant
	archive    string // the archive's name
	path       string // the archive's file
	prefix     string // what the case's sections' names start with: "" for the base, variant/<name>/ or transform/<name>/
	about      []byte // the case's description
	gapsAt     string // the section that holds the case's gaps lines
	a          *archive.Archive
	view       fs.FS
	properties []string
	gaps       []archive.Gap // the lines that mark the case's cells
	state      *caseState
	tr         *transform // the transform whose variant the case is, if any
}

// inputsAt names what to fix for the case's inputs to change: the case,
// or, for a transform's variant, which an update rewrites, the jenny that
// makes it.
func (c *aCase) inputsAt() string {
	if c.tr != nil {
		return c.tr.name + "'s jenny, which makes the variant"
	}
	return "the case"
}

// newCase makes a case of an archive, with no input view yet.
func (r *runner) newCase(p *plan, pc plannedCase, prefix string, about []byte, gapsAt string, properties []string, gaps []archive.Gap) *aCase {
	name := p.root.dir + "/" + pc.name
	if prefix != "" {
		name += "/" + strings.TrimSuffix(prefix, "/")
	}
	return &aCase{
		name:       name,
		archive:    pc.name,
		path:       pc.path,
		prefix:     prefix,
		about:      about,
		gapsAt:     gapsAt,
		a:          pc.a,
		properties: properties,
		gaps:       gaps,
	}
}

// setView gives the case its input view, and the values its cells read.
func (c *aCase) setView(p *plan, view fs.FS) {
	c.view = view
	c.state = &caseState{
		archive: c.archive,
		plan:    p,
		root:    p.root.new(view),
		runs:    map[*lift]*liftRun{},
	}
}

// gap returns the known gap that marks a column's cell, if any.
func (c *aCase) gap(col string) (archive.Gap, bool) {
	for _, g := range c.gaps {
		if g.Column == col {
			return g, true
		}
	}
	return archive.Gap{}, false
}

// gapAt names a gaps line in a message.
func (c *aCase) gapAt(g archive.Gap) string {
	return fmt.Sprintf("%s, line %d,", c.gapsAt, g.Line)
}

// outDir is the prefix of a column's files in the case.
func (c *aCase) outDir(col string) string {
	return c.prefix + archive.Out + "/" + col + "/"
}

// out names one of a column's files in the case.
func (c *aCase) out(col, p string) string { return c.outDir(col) + p }

// goldens returns a column's committed files, by path under its directory.
func (c *aCase) goldens(col string) map[string][]byte {
	g := map[string][]byte{}
	prefix := c.outDir(col)
	for _, s := range c.a.Sections {
		if p, ok := strings.CutPrefix(s.Name, prefix); ok {
			g[p] = s.Data
		}
	}
	return g
}

func (c *aCase) header() string {
	return c.name + ": " + indent(strings.TrimSpace(string(c.about)), "  ")
}

// reverseCells runs each case's cells in reverse column-name order. Only
// hypercase's own tests set it, to show that no cell's bytes depend on the
// cells before it.
var reverseCells bool

// runCase runs an archive's cases: its base's cells, then each variant's,
// as a subtest named for the variant. An update then writes what they made
// to the archive, at once.
func (r *runner) runCase(t *testing.T, p *plan, pc plannedCase) {
	var gaps []archive.Gap
	for _, g := range pc.a.Gaps() {
		if g.Transform == "" {
			gaps = append(gaps, g)
		}
	}
	base := r.newCase(p, pc, "", pc.a.Comment, archive.Gaps, pc.a.Properties(), gaps)
	// The view copies the base's files, so it is made here, where the
	// base's cells read it, and held no longer than they run.
	base.setView(p, pc.base.View())
	w := &caseWrites{set: map[string][]byte{}}
	r.runCells(t, base, p.cols, w)
	for _, v := range pc.variants {
		t.Run(v.caseName(), func(t *testing.T) { r.runVariant(t, p, pc, v, w) })
	}
	if r.update {
		r.write(t, pc, w)
	}
}

// runCells runs a case's cells, one subtest each, named for its column, and
// adds what an update writes for them to w. It returns each cell's result,
// in the order of cols.
func (r *runner) runCells(t *testing.T, c *aCase, cols []*column, w *caseWrites) []*cellResult {
	results := make([]*cellResult, len(cols))
	for i := range cols {
		results[i] = &cellResult{}
	}
	order := make([]int, len(cols))
	for i := range order {
		order[i] = i
	}
	if reverseCells {
		slices.Reverse(order)
	}
	for _, i := range order {
		col, res := cols[i], results[i]
		t.Run(col.name, func(t *testing.T) { r.runCell(t, c, col, res) })
	}
	for i, col := range cols {
		if res := results[i]; res.ran && res.writable {
			for p, data := range res.files {
				w.set[c.out(col.name, p)] = data
			}
		}
	}
	return results
}

// caseWrites is what an update writes to one archive: the sections it
// sets, and the transforms whose inputs it replaces whole.
type caseWrites struct {
	set     map[string][]byte
	replace []string // transforms' names
}

// replaceInputs replaces a transform's inputs, its in/ and delete, with
// the given sections: a file it no longer makes is removed.
func (w *caseWrites) replaceInputs(name string, sections []archive.Section) {
	w.replace = append(w.replace, name)
	for _, s := range sections {
		w.set[s.Name] = s.Data
	}
}

// replaced reports whether a section is one of the inputs w replaces.
func (w *caseWrites) replaced(section string) bool {
	kind, name, rest := archive.SplitCase(section)
	if kind != archive.Transform || !slices.Contains(w.replace, name) {
		return false
	}
	return strings.HasPrefix(rest, archive.In+"/") || rest == archive.Delete
}

var errChanged = errors.New("the archive's hand-written sections changed")

// write writes the package's own sections of one archive: a
// read-modify-write under the corpus root's lock. The bytes of every
// section the package doesn't own come back as they were, though a
// section may move to its place.
func (r *runner) write(t *testing.T, pc plannedCase, w *caseWrites) {
	t.Helper()
	apply := func(a *archive.Archive) *archive.Archive {
		next := &archive.Archive{Comment: bytes.Clone(a.Comment)}
		for _, s := range a.Sections {
			if !w.replaced(s.Name) {
				next.Sections = append(next.Sections, archive.Section{Name: s.Name, Data: bytes.Clone(s.Data)})
			}
		}
		for _, name := range slices.Sorted(maps.Keys(w.set)) {
			setSection(next, name, w.set[name])
		}
		return next.Place()
	}
	if bytes.Equal(apply(pc.a).Format(), pc.data) {
		return
	}
	rel := r.rel(pc.path)
	err := archive.Modify(r.lockPath(), pc.path, func(current *archive.Archive) (*archive.Archive, error) {
		// The cells ran on the hand-written sections first read. Outputs
		// written beside other inputs would make an update that passed and
		// a normal run that fails.
		if !archive.HandWrittenEqual(pc.a, current) {
			return nil, errChanged
		}
		if ps := current.Check(); len(ps) > 0 {
			return nil, fmt.Errorf("it gained problems of form while the update ran:\n%s", problemList(ps))
		}
		return apply(current), nil
	})
	switch {
	case errors.Is(err, errChanged):
		t.Errorf("%s changed while the update ran: its comment or hand-written sections differ from those its cells ran on, so the update wrote nothing to it.\nRun %s again.", rel, r.updateCmd())
	case err != nil:
		t.Errorf("writing %s: %v", rel, err)
	}
}

func setSection(a *archive.Archive, name string, data []byte) {
	for i := range a.Sections {
		if a.Sections[i].Name == name {
			a.Sections[i].Data = data
			return
		}
	}
	a.Sections = append(a.Sections, archive.Section{Name: name, Data: data})
}

// cellResult is what a cell leaves for its case's update.
type cellResult struct {
	ran      bool
	writable bool              // whether an update writes the cell's files
	files    map[string][]byte // the cell's outputs, by path under out/<column>/
	// frozenDiffers records a frozen cell whose run produced files other
	// than its committed ones.
	frozenDiffers bool
}

// The names the framework owns directly under a column's directory.
const (
	errorFile   = "error.txt"
	stdoutFile  = "stdout.txt"
	stderrFile  = "stderr.txt"
	outcomeFile = archive.OutcomeFile
	inputsFile  = "inputs"
)

var frameworkFiles = []string{errorFile, stdoutFile, stderrFile, outcomeFile, inputsFile}

// called is what one call of a column's function left.
type called struct {
	files          []File
	err            error
	stdout, stderr bytes.Buffer
	panicked       bool
	panicValue     any
	stack          []byte
	inProbe        bool // the panic was the probe's
	noProduct      bool // a lift returned neither a product nor an error
}

// call runs fn, recovering a panic. A runtime.Goexit, as from t.FailNow,
// goes on through it.
func call(fn func(c *called)) (c *called) {
	c = &called{}
	returned := false
	defer func() {
		if returned {
			return
		}
		if v := recover(); v != nil {
			c.panicked, c.panicValue, c.stack = true, v, debug.Stack()
		}
	}()
	fn(c)
	returned = true
	return c
}

func (r *runner) runCell(t *testing.T, c *aCase, col *column, res *cellResult) {
	st := r.stats[col.name]
	st.ran++
	header := c.header() + "\n" + col.name + ": " + col.description
	gap, marked := c.gap(col.name)
	goldens := c.goldens(col.name)

	in, by := c.state.stage(col.consumes)
	if by != nil {
		st.applied++
		r.blocked(t, c, header, col, by, gap, marked, goldens, res)
		return
	}

	var out *called
	if col.lift != nil {
		// A lift's cell records its function's one run on the case, which
		// an earlier cell may have made.
		out = c.state.runLift(col.lift, in).out
	} else if col.probe != nil {
		var why string
		var ok bool
		p := call(func(*called) { why, ok = col.probe(in) })
		switch {
		case p.panicked:
			p.inProbe = true
			out = p
		case !ok && strings.TrimSpace(why) == "":
			st.applied++
			t.Errorf("%s\n%s's probe said the case isn't for it, and gave no reason.\nFix the probe: when it says no, it says why.", header, col.name)
			return
		case !ok:
			if err := archive.CheckDetail(why); err != nil {
				st.applied++
				t.Errorf("%s\n%s's probe said the case isn't for it, with a reason an outcome can't hold: %v.\nFix the probe. An update writes nothing for this cell.", header, col.name, err)
				return
			}
			r.skipped(t, c, header, col, why, gap, marked, goldens, res)
			return
		}
	}
	st.applied++
	res.ran = true

	if out == nil {
		finished := false
		defer func() {
			if finished {
				return
			}
			// The run called FailNow or SkipNow, through its testing.TB.
			if t.Skipped() {
				t.Errorf("%s\nThe run skipped its cell. A run can't skip a case; a probe decides which cases are for a suite.", header)
				return
			}
			r.failedTest(t, header)
		}()
		out = call(func(o *called) {
			o.files, o.err = col.run(t, c.archive, in, &o.stdout, &o.stderr)
		})
		finished = true
		if t.Failed() {
			r.failedTest(t, header)
			return
		}
	}

	m := &msg{}
	files, unjudged, writable := r.outputs(m, c, col, out)
	if o, ok := r.outcome(m, col, in, out, files); ok {
		files[outcomeFile] = o.Format()
	} else {
		// The cell has no outcome, so a committed one is no orphan.
		writable = false
		unjudged[outcomeFile] = true
	}
	if _, ok := goldens[outcomeFile]; !ok && !writable {
		// No update writes the cell, so a missing outcome isn't one to
		// write: the cell's problems come first.
		unjudged[outcomeFile] = true
	}

	if !marked {
		r.expectedError(m, c, col, out)
	}
	if marked && len(goldens) > 0 {
		// The known gap freezes the cell: its committed files record
		// today's behavior, and no update writes to it, but to add the
		// outcome of a cell frozen before outcomes were written, when
		// nothing else keeps an update from writing the cell.
		upgradable := writable
		writable = false
		_, hasOutcome := goldens[outcomeFile]
		var diffs []string
		for _, p := range slices.Sorted(maps.Keys(files)) {
			g, ok := goldens[p]
			switch {
			case unjudged[p]:
			case !ok && p == outcomeFile:
			case !ok:
				diffs = append(diffs, fmt.Sprintf("%s is new", c.out(col.name, p)))
			case !bytes.Equal(g, files[p]):
				diffs = append(diffs, fmt.Sprintf("%s differs (-committed +got):\n%s", c.out(col.name, p), strings.TrimSuffix(diff.Lines(g, files[p]), "\n")))
			}
		}
		for _, p := range slices.Sorted(maps.Keys(goldens)) {
			if _, ok := files[p]; !ok && !unjudged[p] {
				diffs = append(diffs, fmt.Sprintf("%s is no longer produced", c.out(col.name, p)))
			}
		}
		_, made := files[outcomeFile]
		switch {
		case len(diffs) > 0:
			res.frozenDiffers = true
			m.add("The cell is frozen by its known gap, %s line %d: %q\nIts committed files record today's behavior, and the run now produces different ones:\n%s\nIf the gap is closed, remove the line, and the next update writes the cell. If the wrong behavior changed shape, delete the cell's files, and the next update writes them.", c.gapsAt, gap.Line, gap.Raw, strings.Join(diffs, "\n"))
		case hasOutcome || !made || !upgradable:
		case r.update:
			// The upgrade, the one write an update makes to a frozen cell:
			// it adds the outcome, and changes nothing the cell has.
			writable = true
			files = map[string][]byte{outcomeFile: files[outcomeFile]}
		default:
			m.add("The cell is frozen by its known gap, %s line %d: %q\nIts committed files match the run, and it has no %s, which every cell has.\nRun %s to add it. The update adds the outcome and changes nothing else.", c.gapsAt, gap.Line, gap.Raw, c.out(col.name, outcomeFile), r.updateCmd())
		}
	} else {
		r.compare(m, c, col, files, goldens, unjudged)
	}
	if !m.empty() {
		t.Errorf("%s\n%s", header, m)
		if out.panicked && !marked {
			t.Logf("stack of the panic:\n%s", out.stack)
		}
	}
	res.writable = writable
	res.files = files
}

func (r *runner) failedTest(t *testing.T, header string) {
	what := "The run failed its test, through its testing.TB."
	if r.update {
		what += " The update writes nothing for this cell."
	}
	t.Logf("%s\n%s", header, what)
}

// skipped finishes a cell whose probe said the case isn't for its column.
// The cell runs nothing, so it isn't checked for an expected error; its
// outcome is its one file, and its other committed files are orphans.
func (r *runner) skipped(t *testing.T, c *aCase, header string, col *column, why string, gap archive.Gap, marked bool, goldens map[string][]byte, res *cellResult) {
	m := &msg{}
	if marked {
		m.add("%s marks the cell, and %s's probe says the case isn't for it: %s\nRemove the line, or fix the probe.", c.gapAt(gap), col.name, why)
	}
	r.ranNothing(m, c, col, archive.Outcome{State: archive.Skipped, Detail: why}, marked, goldens, res)
	for _, p := range slices.Sorted(maps.Keys(goldens)) {
		if p == outcomeFile {
			continue
		}
		m.add("%s is recorded, and %s's probe says the case isn't for it: %s\nIf the suite should run on the case, fix the probe. If not, delete the section by hand; %s leaves it in place.", c.out(col.name, p), col.name, why, r.updateCmd())
	}
	if !m.empty() {
		t.Errorf("%s\n%s", header, m)
		return
	}
	t.Skipf("%s: %s's probe says the case isn't for it: %s", c.name, col.name, why)
}

// blocked finishes a cell that a lift on its path blocked, by failing on
// the case. The cell runs nothing, so it isn't checked for an expected
// error; its outcome is its one file, and its other committed files are
// orphans.
func (r *runner) blocked(t *testing.T, c *aCase, header string, col *column, by *lift, gap archive.Gap, marked bool, goldens map[string][]byte, res *cellResult) {
	m := &msg{}
	if marked {
		m.add("%s marks the cell, and the cell is blocked: the lift %s, on %s's path, failed on the case, so the cell runs nothing.\nRemove the line, or fix the lift or %s.", c.gapAt(gap), by.name, col.name, c.inputsAt())
	}
	r.ranNothing(m, c, col, archive.Outcome{State: archive.Blocked, Detail: by.name}, marked, goldens, res)
	for _, p := range slices.Sorted(maps.Keys(goldens)) {
		if p == outcomeFile {
			continue
		}
		m.add("%s is recorded, and the cell is blocked: the lift %s, on %s's path, failed on the case.\nIf the cell should run, fix the lift or %s. If not, delete the section by hand; %s leaves it in place.", c.out(col.name, p), by.name, col.name, c.inputsAt(), r.updateCmd())
	}
	if !m.empty() {
		t.Errorf("%s\n%s", header, m)
		return
	}
	if c.state.runs[by].out.noProduct {
		t.Skipf("%s: %s is blocked: the lift %s failed on the case: it returned no product and no error.", c.name, col.name, by.name)
	}
	t.Skipf("%s: %s is blocked: the lift %s failed on the case, and %s, %s, records its error.", c.name, col.name, by.name, r.rel(c.path), c.out(by.name, errorFile))
}

// ranNothing records the outcome of a cell that ran nothing, its one file:
// a normal run compares it with its golden, and an update writes it. A
// frozen cell, one a gaps line marks that has committed files, keeps them
// as they are: its line fails it already, and since the run made none of
// its files but the outcome, the upgrade that adds a missing outcome to a
// frozen cell whose files match the run's doesn't apply.
func (r *runner) ranNothing(m *msg, c *aCase, col *column, o archive.Outcome, marked bool, goldens map[string][]byte, res *cellResult) {
	data := o.Format()
	res.ran, res.writable, res.files = true, true, map[string][]byte{outcomeFile: data}
	if marked && len(goldens) > 0 {
		res.writable = false
		return
	}
	if r.update {
		return
	}
	switch g, ok := goldens[outcomeFile]; {
	case !ok:
		m.add("%s has no golden.\nRun %s to write it, and read it before committing.", c.out(col.name, outcomeFile), r.updateCmd())
	case !bytes.Equal(g, data):
		m.add("%s differs (-golden +got):\n%s\nIf the change is intended, run %s and read the diff before committing.", c.out(col.name, outcomeFile), strings.TrimSuffix(diff.Lines(g, data), "\n"), r.updateCmd())
	}
}

// outcome is a cell's outcome, from what its run did, which its column's
// classifier, if any, refines. It reports a code, a classifier or a detail
// that fails, and a lift that returned neither a product nor an error; the
// cell then has no outcome, and an update writes nothing for it.
func (r *runner) outcome(m *msg, col *column, in any, out *called, files map[string][]byte) (archive.Outcome, bool) {
	var o archive.Outcome
	switch {
	case out.panicked:
		return archive.Outcome{State: archive.Panicked}, true
	case out.noProduct:
		m.add("%s returned no product and no error, so the cell has no outcome.\nFix the lift: it returns its product, or an error that says why it has none. Returning neither is never an expected error, and an update writes nothing for this cell.", col.name)
		return o, false
	case out.err != nil:
		o.State = archive.Rejected
		codes, ok := r.errorCodes(m, col, out.err)
		if !ok {
			return o, false
		}
		o.Codes = codes
	default:
		o.State = archive.Accepted
	}
	cl := col.classifier
	if cl == nil {
		return o, true
	}
	var detail string
	var err error
	p := call(func(*called) { detail, err = cl.classify(in, newResult(files, out.err)) })
	switch {
	case p.panicked:
		m.add("%s's classifier %s panicked: %v\nFix the classifier. An update writes nothing for this cell.", col.name, cl.name, p.panicValue)
		return o, false
	case err != nil:
		m.add("%s's classifier %s failed: %v\nFix the classifier. An update writes nothing for this cell.", col.name, cl.name, err)
		return o, false
	case detail == "":
	default:
		if err := archive.CheckDetail(detail); err != nil {
			m.add("%s's classifier %s gave a detail an outcome can't hold: %v.\nFix the classifier. An update writes nothing for this cell.", col.name, cl.name, err)
			return o, false
		}
		o.Detail = detail
	}
	return o, true
}

// errorCodes gives an error's codes, through the project's Codes: sorted,
// each once. It reports a Codes that panics, or gives a code an outcome
// can't hold.
func (r *runner) errorCodes(m *msg, col *column, err error) ([]string, bool) {
	if r.codes == nil {
		return nil, true
	}
	var codes []string
	p := call(func(*called) { codes = r.codes(err) })
	if p.panicked {
		m.add("The project's Codes panicked on %s's error: %v\nFix Codes. An update writes nothing for this cell.", col.name, p.panicValue)
		return nil, false
	}
	for _, code := range codes {
		if err := archive.CheckCode(code); err != nil {
			m.add("The project's Codes gave %s's error a code an outcome can't hold: %v.\nFix Codes. An update writes nothing for this cell.", col.name, err)
			return nil, false
		}
	}
	codes = slices.Compact(slices.Sorted(slices.Values(codes)))
	if len(codes) == 0 {
		return nil, true
	}
	return codes, true
}

// outputs collects a cell's files: what its function returned, and what the
// framework writes for it. It reports outputs whose shape is wrong or that
// no declaration matches, which are left unjudged, and whether an update
// may write the cell.
func (r *runner) outputs(m *msg, c *aCase, col *column, out *called) (files map[string][]byte, unjudged map[string]bool, writable bool) {
	files, unjudged, writable = map[string][]byte{}, map[string]bool{}, true
	for _, f := range out.files {
		switch {
		case !fs.ValidPath(f.Path) || f.Path == ".":
			m.add("The run returned an output at %q, which is not a valid slash path under %s.\nFix the code. An update writes nothing for this cell.", f.Path, c.outDir(col.name))
			writable = false
		case slices.Contains(frameworkFiles, f.Path):
			m.add("The run returned an output at %s, a name the framework owns under %s.\nRename the output. An update writes nothing for this cell.", f.Path, c.outDir(col.name))
			writable = false
		case files[f.Path] != nil:
			m.add("The run returned two outputs at %s.\nFix the code. An update writes nothing for this cell.", f.Path)
			writable = false
		default:
			files[f.Path] = bytes.Clone(f.Data)
			if files[f.Path] == nil {
				files[f.Path] = []byte{}
			}
		}
	}
	if out.stdout.Len() > 0 {
		files[stdoutFile] = out.stdout.Bytes()
	}
	if out.stderr.Len() > 0 {
		files[stderrFile] = out.stderr.Bytes()
	}
	switch {
	case out.panicked:
		files[errorFile] = fmt.Appendf(nil, "%v\n", out.panicValue)
	case out.err != nil:
		data, ok := r.render(m, c, col, out.err)
		files[errorFile] = data
		if !ok {
			writable = false
			unjudged[errorFile] = true
		}
	}
	names := []string{c.out(col.name, outcomeFile)} // every cell has one
	for _, p := range slices.Sorted(maps.Keys(files)) {
		names = append(names, c.out(col.name, p))
	}
	for _, p := range archive.Conflicts(names) {
		m.add("%s can't be held in an archive: %s.\nFix the code. An update writes nothing for this cell.", p.Section, p.Msg)
		writable = false
	}
	for _, p := range slices.Sorted(maps.Keys(files)) {
		if unjudged[p] {
			continue
		}
		switch hit := col.declared(p); len(hit) {
		case 0:
			m.add("%s%s is not declared.\nDeclare it in %s's Outputs, with a description, or fix the code. An update writes nothing for this cell.", c.out(col.name, p), holds(p), col.name)
			writable = false
			unjudged[p] = true
		case 1:
		default:
			m.add("%s matches two of %s's output declarations, %s and %s.\nChange the patterns so that one declaration matches each file. An update writes nothing for this cell.", c.out(col.name, p), col.name, hit[0], hit[1])
			writable = false
			unjudged[p] = true
		}
	}
	for _, p := range slices.Sorted(maps.Keys(files)) {
		if err := archive.Holds(files[p]); err != nil && !unjudged[p] {
			m.add("%s can't be held in an archive: %v.\nFix the code so the output can be held. An update writes nothing for this cell.", c.out(col.name, p), err)
			writable = false
			unjudged[p] = true
		}
	}
	return files, unjudged, writable
}

// render renders an error a column's function returned, for error.txt:
// through the project's RenderError, or as the error's text. It reports a
// RenderError that panics, or whose bytes don't end with a newline.
func (r *runner) render(m *msg, c *aCase, col *column, err error) ([]byte, bool) {
	if r.renderError == nil {
		return []byte(err.Error() + "\n"), true
	}
	var data []byte
	p := call(func(*called) { data = r.renderError(c.view, err) })
	switch {
	case p.panicked:
		m.add("The project's RenderError panicked on %s's error: %v\nFix RenderError. An update writes nothing for this cell.", col.name, p.panicValue)
		return []byte{}, false
	case !bytes.HasSuffix(data, []byte("\n")):
		m.add("The project's RenderError returned %q for %s's error, which doesn't end with a newline.\nIts output must end with a newline: fix RenderError. An update writes nothing for this cell.", data, col.name)
		return []byte{}, false
	}
	return bytes.Clone(data), true
}

// holds says what a file the framework writes holds.
func holds(p string) string {
	switch p {
	case errorFile:
		return ", which holds the error the run returned,"
	case stdoutFile:
		return ", which holds what the run wrote to stdout,"
	case stderrFile:
		return ", which holds what the run wrote to stderr,"
	}
	return ""
}

// expectedError is the expected-error check: a cell is expected to err
// exactly when its case states a property its column rejects. A panic is
// never an expected error.
func (r *runner) expectedError(m *msg, c *aCase, col *column, out *called) {
	hit := col.rejectsAny(c.properties)
	switch {
	case out.panicked && out.inProbe:
		m.add("%s's probe panicked: %v\nFix the probe. A panic is never an expected error.", col.name, out.panicValue)
	case out.panicked:
		m.add("%s panicked: %v\nFix the code. A panic is never an expected error.", col.name, out.panicValue)
	case out.noProduct:
	case out.err != nil && len(hit) == 0 && c.tr != nil:
		m.add("%s returned an error, but the variant states no property %s rejects:\n  %s\n%s Fix the code or %s's jenny, or give the variant the property, in %s's Gives or Keeps, and list it in %s's Rejects.", col.name, col.name, indent(strings.TrimSuffix(out.err.Error(), "\n"), "  "), c.tr.states(), c.tr.name, c.tr.name, col.name)
	case out.err != nil && len(hit) == 0:
		m.add("%s returned an error, but the case states no property %s rejects:\n  %s\nFix the code, or state the property on the case and in %s's Rejects.", col.name, col.name, indent(strings.TrimSuffix(out.err.Error(), "\n"), "  "), col.name)
	case out.err == nil && len(hit) > 0 && c.tr != nil:
		m.add("%s returned no error, but the variant states %s, which %s rejects.\n%s Fix the code or %s's jenny, or correct %s's Gives or Keeps, or %s's Rejects.", col.name, andList(hit), col.name, c.tr.states(), c.tr.name, c.tr.name, col.name)
	case out.err == nil && len(hit) > 0:
		m.add("%s returned no error, but the case states %s, which %s rejects.\nFix the code, or correct the case's properties or %s's Rejects.", col.name, andList(hit), col.name, col.name)
	}
}

// compare compares a cell's files with its goldens, and reports orphans.
func (r *runner) compare(m *msg, c *aCase, col *column, files, goldens map[string][]byte, unjudged map[string]bool) {
	if !r.update {
		for _, p := range slices.Sorted(maps.Keys(files)) {
			g, ok := goldens[p]
			switch {
			case unjudged[p]:
			case !ok:
				m.add("%s has no golden.\nRun %s to write it, and read it before committing.", c.out(col.name, p), r.updateCmd())
			case !bytes.Equal(g, files[p]):
				m.add("%s differs (-golden +got):\n%s\nIf the change is intended, run %s and read the diff before committing.", c.out(col.name, p), strings.TrimSuffix(diff.Lines(g, files[p]), "\n"), r.updateCmd())
			}
		}
	}
	for _, p := range slices.Sorted(maps.Keys(goldens)) {
		if _, ok := files[p]; ok || unjudged[p] {
			continue
		}
		var what string
		switch p {
		case errorFile:
			what = "records an error, but the run returned none"
		case stdoutFile:
			what = "records output, but the run wrote nothing to stdout"
		case stderrFile:
			what = "records output, but the run wrote nothing to stderr"
		default:
			what = "is an output the run no longer produces"
		}
		m.add("%s %s.\nIf the run should still write it, fix the code. If not, delete the section by hand; %s leaves it in place.", c.out(col.name, p), what, r.updateCmd())
	}
}

// appliesToNoCase fails each suite that applies to no case: one whose
// corpora hold no archive whose cells run, or whose probe said no to every
// case. The verdict needs every one of the suite's cells to have run, so
// under a -run that leaves some out, it isn't given. A lift's column that
// no archive reaches is no failure: a lift is declared with its type,
// often before the type's first archive.
func (r *runner) appliesToNoCase(t *testing.T) {
	t.Helper()
	for _, c := range r.columns {
		if c.lift != nil {
			continue
		}
		st := r.stats[c.name]
		switch {
		case st.ran < st.total || st.applied > 0:
		case st.total == 0:
			var dirs []string
			for _, d := range r.corpora[c.name] {
				dirs = append(dirs, r.rel(filepath.Join(r.corpusRoot, "testdata", d))+"/")
			}
			what := "its corpus, " + andList(dirs) + ", holds"
			if len(dirs) > 1 {
				what = "its corpora, " + andList(dirs) + ", hold"
			}
			t.Errorf("the suite %s applies to no case: %s no archive whose cells run.\nA suite that applies to no case checks nothing. Add a case it applies to.", c.name, what)
		default:
			t.Errorf("the suite %s applies to no case: its probe says no to every case of %s.\nA suite that applies to no case checks nothing. Add a case it applies to, or fix the probe.", c.name, andList(r.corpora[c.name]))
		}
	}
}

func andList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// msg collects the problems of one failure, each ending with what to do.
type msg struct{ parts []string }

func (m *msg) add(format string, args ...any) {
	m.parts = append(m.parts, fmt.Sprintf(format, args...))
}
func (m *msg) empty() bool    { return len(m.parts) == 0 }
func (m *msg) String() string { return strings.Join(m.parts, "\n") }

func indent(s, prefix string) string {
	return strings.ReplaceAll(s, "\n", "\n"+prefix)
}

func problemList(ps []archive.Problem) string {
	lines := make([]string, len(ps))
	for i, p := range ps {
		lines[i] = "  " + p.String()
	}
	return strings.Join(lines, "\n")
}

func problemLines(ps []archive.Problem) string {
	lines := make([]string, len(ps))
	for i, p := range ps {
		lines[i] = p.String()
	}
	return strings.Join(lines, "\n")
}
