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
	"github.com/sdboyer/hypercase/internal/shell"
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
	owned       map[*transform]bool   // the types' transforms the package owns
	products    map[*lift]*corpusType // each bytes lift's product's corpus type
	checks      []Check               // sorted by name
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
// and so is each a transform gives or keeps, each transform the package
// owns is one a corpus type it registers lists, and each bytes lift's
// product is a corpus type it registers.
func (r *runner) checkRelations(t *testing.T, ownedTransforms []*transform) bool {
	t.Helper()
	ok := true
	for _, c := range r.columns {
		ok = checkRegistered(t, fmt.Sprintf("the %s %s", c.kind(), c.name), "rejects", c.name+"'s Rejects", c.rejects) && ok
	}
	ok = r.checkTransformRelations(t, ownedTransforms) && ok
	r.products = map[*lift]*corpusType{}
	for _, l := range r.lifts {
		if !l.bytes {
			continue
		}
		i := slices.IndexFunc(r.types, func(ct *corpusType) bool { return ct.goType == l.out })
		if i < 0 {
			t.Errorf("the lift %s's product, %s, is shaped as a corpus type, and the package registers no such corpus type.\nRegister the corpus type, whose input view the lift's product is, or make the product a stage, a type that isn't a named fs.FS wrapper.", l.name, l.out)
			ok = false
			continue
		}
		r.products[l] = r.types[i]
	}
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

// stripCmd is the hypercase strip command, run at the module root, that
// removes the sections the flag given selects under the corpus root, or,
// with no flag, every section an update writes there. It names the root
// with -root, the module-root corpus too: without it, strip takes the
// root the module root's registry records, which a stale one may record
// otherwise. It quotes the root's path for a shell where the path needs
// it.
func (r *runner) stripCmd(flag string) string {
	cmd := "hypercase strip -root " + shell.Quote(r.rootRel)
	if flag != "" {
		cmd += " " + flag
	}
	return cmd
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
		for _, o := range l.product {
			if strings.TrimSpace(o.Description) == "" {
				t.Errorf("the lift %s declares the product file %s with no description.\nAdd one: it says what the files hold, in prose.", l.name, o.Pattern)
			}
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
func (c *aCase) setView(r *runner, p *plan, view fs.FS) {
	c.state = &caseState{
		r:      r,
		c:      c,
		plan:   p,
		root:   p.root.new(view),
		view:   view,
		runs:   map[*lift]*liftRun{},
		reads:  map[*lift]*liftRead{},
		update: r.update,
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
	return committedUnder(c.a, c.outDir(col))
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
	base.setView(r, p, pc.base.View())
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
	passed := make([]bool, len(cols))
	for _, i := range order {
		col, res := cols[i], results[i]
		passed[i] = t.Run(col.name, func(t *testing.T) { r.runCell(t, c, col, res) })
	}
	for i, col := range cols {
		if observe.cell != nil {
			observe.cell(c, col, results[i], !passed[i])
		}
		if res := results[i]; res.ran && res.writable {
			for p, data := range res.files {
				w.set[c.out(col.name, p)] = data
			}
			if res.via {
				w.replaceProduct(col.lift.viaDir(c.prefix), res.product)
			}
		}
	}
	return results
}

// caseWrites is what an update writes to one archive: the sections it
// sets, the transforms whose inputs it replaces whole, and the bytes lifts'
// products it replaces whole.
type caseWrites struct {
	set      map[string][]byte
	replace  []string // transforms' names
	products []string // the prefixes of the products' sections, via/<lift>/ after a case's prefix
}

// replaceInputs replaces a transform's inputs, its in/ and delete, with
// the given sections: a file it no longer makes is removed.
func (w *caseWrites) replaceInputs(name string, sections []archive.Section) {
	w.replace = append(w.replace, name)
	for _, s := range sections {
		w.set[s.Name] = s.Data
	}
}

// replaceProduct replaces a bytes lift's product in one case, the sections
// under dir, with the given files, by their paths in the product: a file
// it no longer makes is removed, and so is every file, for a lift with no
// product.
func (w *caseWrites) replaceProduct(dir string, files map[string][]byte) {
	w.products = append(w.products, dir)
	for p, data := range files {
		w.set[dir+p] = data
	}
}

// replaced reports whether a section is one of the inputs or products w
// replaces.
func (w *caseWrites) replaced(section string) bool {
	for _, dir := range w.products {
		if strings.HasPrefix(section, dir) {
			return true
		}
	}
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
	// fate is the cell's fate in this run, as updateFate words the
	// update's: blocked, skipped, failed by its probe, or run.
	fate     updateFate
	ran      bool
	writable bool              // whether an update writes the cell's files
	files    map[string][]byte // the cell's outputs, by path under out/<column>/
	// onValue records that the cell ran on a value: its run, or its
	// lift's, past its probe, if it has one. view is the input view the
	// value comes from, or nil where a lift on the cell's path blocks it.
	onValue bool
	view    fs.FS
	// via records that an update that writes the cell replaces its bytes
	// lift's product, under via/<column>/, with product: the files of the
	// product, by path in it, or none, for a lift with no product.
	via     bool
	product map[string][]byte
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
	inProbe        bool  // the panic was the probe's
	noProduct      bool  // a lift returned neither a product nor an error
	jennyErr       error // a bytes lift's jenny returned this error, which is no rejection, in place of err
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
	cf := c.committed(col)
	goldens := cf.goldens

	in, view, by := c.state.stage(col.consumes)
	res.view = view
	if by != nil {
		st.applied++
		res.fate = updateFate{block: by}
		if by.broken {
			// Only an update runs a bytes lift past which it finds this.
			var ns notes
			ns.add(aboutFault,
				fmt.Sprintf("The cell of the lift %s, on %s's path, has a problem, which that cell reports, in the package that owns it. A normal run reads the product and the outcome the archive commits for %s, so this cell runs nothing.", by.lift.name, col.name, by.lift.name),
				closing{advice: fmt.Sprintf("Fix what %s's cell reports.", by.lift.name), claims: claims{Unwritten: by.lift.name}})
			report(t, header, ns, r.staging(c, col, res.fate, false, cf.frozen(marked), nil))
			return
		}
		r.blocked(t, c, header, col, by, gap, marked, cf, res)
		return
	}

	var out *called
	var lr *liftRun
	if col.lift != nil {
		// A lift's cell records its function's one run on the case, which
		// an earlier cell may have made.
		lr = c.state.runLift(col.lift, in)
		out = lr.out
	} else if col.probe != nil {
		var why string
		var ok bool
		p := call(func(*called) { why, ok = col.probe(in) })
		switch {
		case p.panicked:
			p.inProbe = true
			out = p
			res.fate = updateFate{fault: probePanics(col, p.panicValue), panicked: true}
		case !ok:
			if fault := probeFault(col, why); fault != "" {
				st.applied++
				res.fate = updateFate{fault: fault}
				r.probeFailed(t, c, header, col, why, marked, cf, res)
				return
			}
			res.fate = updateFate{skip: why}
			r.skipped(t, c, header, col, why, gap, marked, goldens, res)
			return
		}
	}
	st.applied++
	res.ran, res.onValue = true, true
	frozen := cf.frozen(marked)
	// What the update does with the cell, which a normal run tells once,
	// when a closing first needs it.
	stg := r.staging(c, col, res.fate, true, frozen, nil)

	if out == nil {
		finished := false
		defer func() {
			if finished {
				return
			}
			// The run called FailNow or SkipNow, through its testing.TB.
			if t.Skipped() {
				var ns notes
				ns.add(aboutFault, "The run skipped its cell.", closing{advice: "A run can't skip a case; a probe decides which cases are for a suite."})
				report(t, header, ns, stg)
				return
			}
			failedTest(t, header, stg)
		}()
		out = call(func(o *called) {
			o.files, o.err = col.run(t, c.archive, in, &o.stdout, &o.stderr)
		})
		finished = true
		if t.Failed() {
			failedTest(t, header, stg)
			return
		}
	}

	var ns notes
	bytesLift := col.lift != nil && col.lift.bytes
	var files map[string][]byte
	var unjudged map[string]bool
	var writable bool
	if bytesLift {
		// In an update, the cells past the lift may have asked for the
		// verdict first, to learn whether the update writes this cell.
		v := c.state.liftVerdict(col.lift, lr, in, view)
		ns = append(ns, v.notes...)
		files, unjudged, writable = maps.Clone(v.files), maps.Clone(v.unjudged), v.writable
	} else {
		files, unjudged, writable = r.verdict(&ns, c, col, in, view, out)
	}
	// A bytes lift's cell includes its product. A fault of its jenny
	// leaves the product uncompared: the fault comes first.
	var product map[string][]byte
	held := true
	if bytesLift {
		product, held = lr.product, len(lr.problems) == 0
	}

	if !marked {
		r.expectedError(&ns, c, col, out)
	}
	if frozen {
		// The known gap freezes the cell: its committed files record
		// today's behavior, and no update writes to it, but to add the
		// outcome of a cell frozen before outcomes were written, when
		// nothing else keeps an update from writing the cell. couldWrite
		// is true when nothing else does.
		couldWrite := writable
		writable = false
		_, hasOutcome := goldens[outcomeFile]
		diffs := frozenDiffs(c.outDir(col.name), goldens, files, unjudged)
		var productDiffs []string
		if bytesLift && held {
			productDiffs = frozenDiffs(col.lift.viaDir(c.prefix), cf.product, product, nil)
			diffs = append(diffs, productDiffs...)
		}
		_, made := files[outcomeFile]
		switch {
		case len(diffs) > 0:
			var cl closing
			cl.advice = "If the gap is closed, remove the line. If the wrong behavior changed shape, delete the cell's files."
			if bytesLift {
				cl.advice = fmt.Sprintf("If the gap is closed, remove the line. If the wrong behavior changed shape, delete the cell's files and its product, under %s and %s.", c.outDir(col.name), col.lift.viaDir(c.prefix))
			}
			if !couldWrite {
				cl = closing{"Fix the problems above.", claims{Nothing: true, Frozen: true}}
			}
			// A normal run gives the cells past a bytes lift what its cell
			// commits, and an update, which writes them unless it writes
			// nothing for the lift's cell, what the lift makes now. They
			// part when one outcome accepts and the other doesn't, or when
			// both accept and the products differ.
			if bytesLift && couldWrite {
				was, is := accepts(goldens[outcomeFile]), accepts(files[outcomeFile])
				cl.claims.PastFresh = was != is || is && len(productDiffs) > 0
			}
			ns.add(aboutFrozen,
				fmt.Sprintf("The cell is frozen by its known gap, %s line %d: %q\nIts committed files record today's behavior, and the run now produces different ones:\n%s", c.gapsAt, gap.Line, gap.Raw, strings.Join(diffs, "\n")),
				cl)
		case hasOutcome || !made || !couldWrite:
		case r.update:
			// The upgrade, the one write an update makes to a frozen cell:
			// it adds the outcome, and changes nothing the cell has.
			writable = true
			files = map[string][]byte{outcomeFile: files[outcomeFile]}
		default:
			ns.add(aboutFrozen,
				fmt.Sprintf("The cell is frozen by its known gap, %s line %d: %q\nIts committed files match the run, and it has no %s, which every cell has.", c.gapsAt, gap.Line, gap.Raw, c.out(col.name, outcomeFile)),
				closing{claims: claims{OnlyOutcome: c.out(col.name, outcomeFile)}})
		}
	} else {
		r.compare(&ns, c, col, files, goldens, unjudged, writable)
		if bytesLift && held {
			r.compareProduct(&ns, c, col.lift, out, product, cf.product, writable)
		}
	}
	if len(ns) > 0 {
		stg.outcome = files[outcomeFile]
		report(t, header, ns, stg)
		if out.panicked && !marked {
			t.Logf("stack of the panic:\n%s", out.stack)
		}
	}
	res.writable = writable
	res.files = files
	// An update writes a frozen cell only to add its outcome.
	res.via = bytesLift && !frozen
	res.product = product
}

// verdict is a cell's files, what it made and what the framework writes
// for it, with its outcome, the files left unjudged, and whether an update
// may write the cell, its freeze aside. It adds what is wrong to ns.
func (r *runner) verdict(ns *notes, c *aCase, col *column, in any, view fs.FS, out *called) (files map[string][]byte, unjudged map[string]bool, writable bool) {
	files, unjudged, writable = r.outputs(ns, c, col, view, out)
	if o, ok := r.outcome(ns, col, in, out, files); ok {
		files[outcomeFile] = o.Format()
	} else {
		// The cell has no outcome, so a committed one is no orphan.
		writable = false
		unjudged[outcomeFile] = true
	}
	return files, unjudged, writable
}

// A liftVerdict is a bytes lift's cell's verdict on a case, and what the
// cell reports of it.
type liftVerdict struct {
	files    map[string][]byte
	unjudged map[string]bool
	writable bool
	notes    notes
}

// liftVerdict judges a bytes lift's cell on the case once, from its one
// run, under the package's Project: the lift's own cell reports it, and in
// the package that owns the lift, an update runs the cells past the lift
// only when it writes the lift's cell, whose product they read. A fault of
// the jenny keeps the update from writing the cell. A package that only
// uses the lift holds the cells past it back on the cell's verdict under a
// bare Project instead, which heldBack gives.
func (s *caseState) liftVerdict(l *lift, lr *liftRun, in any, view fs.FS) *liftVerdict {
	if lr.verdict == nil {
		lr.verdict = s.judgeLift(s.r, l, lr, in, view)
	}
	return lr.verdict
}

// judgeLift judges a bytes lift's cell on the case, from its one run, as
// the runner r judges a cell.
func (s *caseState) judgeLift(r *runner, l *lift, lr *liftRun, in any, view fs.FS) *liftVerdict {
	v := &liftVerdict{}
	v.files, v.unjudged, v.writable = r.verdict(&v.notes, s.c, l.column(), in, view, lr.out)
	if len(lr.problems) > 0 {
		v.notes = append(v.notes, lr.problems...)
		v.writable = false
	}
	return v
}

// bare is the runner as it judges a cell under a bare Project: one whose
// RenderError renders every error as an empty line, and which gives no
// Codes, so neither fails a cell. What keeps an update from writing a cell
// under the bare Project keeps it from writing the cell, as the same
// declaration gives it, under any Project.
func (r *runner) bare() *runner {
	b := *r
	b.renderError = func(fs.FS, error) []byte { return []byte("\n") }
	b.codes = nil
	return &b
}

// cellFiles is what a case commits for a column's cell: its files, by path
// under out/<column>/, and a bytes lift's product, by path under
// via/<column>/, which is nil for any other column.
type cellFiles struct{ goldens, product map[string][]byte }

// committed reads what the case commits for a column's cell.
func (c *aCase) committed(col *column) cellFiles {
	cf := cellFiles{goldens: c.goldens(col.name)}
	if col.lift != nil && col.lift.bytes {
		cf.product = c.product(col.lift)
	}
	return cf
}

// frozen reports whether a gaps line freezes the cell: marked says whether
// one marks it, and the cell has committed files, a bytes lift's product
// included.
func (cf cellFiles) frozen(marked bool) bool {
	return marked && (len(cf.goldens) > 0 || len(cf.product) > 0)
}

// product returns a bytes lift's committed product in the case, by path in
// it.
func (c *aCase) product(l *lift) map[string][]byte {
	return committedUnder(c.a, l.viaDir(c.prefix))
}

// committedUnder returns an archive's sections under dir, by path under it.
func committedUnder(a *archive.Archive, dir string) map[string][]byte {
	files := map[string][]byte{}
	for _, s := range a.Sections {
		if p, ok := strings.CutPrefix(s.Name, dir); ok {
			files[p] = s.Data
		}
	}
	return files
}

// accepts reports whether an outcome file's data says its cell accepted
// the case. No data, or data that isn't an outcome, doesn't.
func accepts(data []byte) bool {
	o, err := archive.ParseOutcome(data)
	return err == nil && o.State == archive.Accepted
}

// frozenDiffs says how the files a frozen cell's run made, got, differ from
// its committed ones, each named by its path under dir: its files under
// out/<column>/, or a bytes lift's product under via/<lift>/. A path that
// unjudged holds is left out, and so is an outcome the cell lacks, which
// the upgrade adds.
func frozenDiffs(dir string, committed, got map[string][]byte, unjudged map[string]bool) []string {
	var diffs []string
	for _, p := range slices.Sorted(maps.Keys(got)) {
		g, ok := committed[p]
		switch {
		case unjudged[p]:
		case !ok && p == outcomeFile:
		case !ok:
			diffs = append(diffs, fmt.Sprintf("%s is new", dir+p))
		case !bytes.Equal(g, got[p]):
			diffs = append(diffs, fmt.Sprintf("%s differs (-committed +got):\n%s", dir+p, strings.TrimSuffix(diff.Lines(g, got[p]), "\n")))
		}
	}
	for _, p := range slices.Sorted(maps.Keys(committed)) {
		if _, ok := got[p]; !ok && !unjudged[p] {
			diffs = append(diffs, fmt.Sprintf("%s is no longer produced", dir+p))
		}
	}
	return diffs
}

// failedTest finishes a cell whose run failed its test, through its
// testing.TB, with a note, which the staging closes as it closes every
// note of a cell.
func failedTest(t *testing.T, header string, stg *staging) {
	var ns notes
	ns.add(aboutFault, "The run failed its test, through its testing.TB.", closing{"Fix the code.", claims{Nothing: true}})
	report(t, header, ns, stg)
}

// skipped finishes a cell whose probe said the case isn't for its column.
// The cell runs nothing, so it isn't checked for an expected error; its
// outcome is its one file, and its other committed files are orphans.
func (r *runner) skipped(t *testing.T, c *aCase, header string, col *column, why string, gap archive.Gap, marked bool, goldens map[string][]byte, res *cellResult) {
	var ns notes
	frozen := marked && len(goldens) > 0
	if marked {
		ns.add(aboutGap, fmt.Sprintf("%s marks the cell, and %s's probe says the case isn't for it: %s", c.gapAt(gap), col.name, why), closing{advice: "Remove the line, or fix the probe."})
	}
	o := archive.Outcome{State: archive.Skipped, Detail: why}
	r.ranNothing(&ns, c, col, o, frozen, goldens, res)
	for _, p := range slices.Sorted(maps.Keys(goldens)) {
		if p != outcomeFile {
			ns.addFile(aboutOrphan, c.out(col.name, p),
				fmt.Sprintf("%s is recorded, and %s's probe says the case isn't for it: %s", c.out(col.name, p), col.name, why),
				closing{"If the suite should run on the case, fix the probe. If not, delete the section by hand;", claims{Leaves: c.out(col.name, p)}})
		}
	}
	if len(ns) > 0 {
		report(t, header, ns, r.staging(c, col, res.fate, false, frozen, o.Format()))
		return
	}
	t.Skipf("%s: %s's probe says the case isn't for it: %s", c.name, col.name, why)
}

// probeFailed finishes a cell whose probe said the case isn't for its
// column with no reason, or with one an outcome can't hold. The cell has
// no outcome, and an update writes nothing for it. When the update does
// otherwise with the cell, as it may past a bytes lift, or on a
// transform's variant whose committed inputs give another input view than
// the one the transform makes, where the probe reads another value, a
// normal run says so of the cell's committed files too.
func (r *runner) probeFailed(t *testing.T, c *aCase, header string, col *column, why string, marked bool, cf cellFiles, res *cellResult) {
	stg := r.staging(c, col, res.fate, false, cf.frozen(marked), nil)
	var ns notes
	if strings.TrimSpace(why) == "" {
		ns.add(aboutProbeFault, fmt.Sprintf("%s's probe said the case isn't for it, and gave no reason.", col.name), closing{advice: "Fix the probe: when it says no, it says why."})
	} else {
		ns.add(aboutProbeFault, fmt.Sprintf("%s's probe said the case isn't for it, with a reason an outcome can't hold: %v.", col.name, archive.CheckDetail(why)), closing{"Fix the probe.", claims{Nothing: true}})
	}
	if !r.update && stg.differs() {
		for _, p := range slices.Sorted(maps.Keys(cf.goldens)) {
			if p != outcomeFile {
				ns.addFile(aboutOrphan, c.out(col.name, p), fmt.Sprintf("%s is recorded, and %s.", c.out(col.name, p), res.fate.fault), closing{claims: claims{Nothing: true, Leaves: c.out(col.name, p)}})
			}
		}
	}
	report(t, header, ns, stg)
}

// blocked finishes a cell that a lift on its path blocked, by failing on
// the case, in this run or as the archive commits it. The cell runs
// nothing, so it isn't checked for an expected error; its outcome is its
// one file, and its other committed files are orphans. A blocked bytes
// lift has no product, so its owner's update removes the committed one,
// which other cells read.
func (r *runner) blocked(t *testing.T, c *aCase, header string, col *column, b *blockage, gap archive.Gap, marked bool, cf cellFiles, res *cellResult) {
	var ns notes
	why := b.why(col)
	goldens := cf.goldens
	if marked {
		ns.add(aboutGap, fmt.Sprintf("%s marks the cell, and the cell is blocked: %s, so the cell runs nothing.", c.gapAt(gap), why), closing{advice: fmt.Sprintf("Remove the line, or fix the lift or %s.", c.inputsAt())})
	}
	frozen := cf.frozen(marked)
	o := archive.Outcome{State: archive.Blocked, Detail: b.failed}
	r.ranNothing(&ns, c, col, o, frozen, goldens, res)
	for _, p := range slices.Sorted(maps.Keys(goldens)) {
		if p != outcomeFile {
			ns.addFile(aboutOrphan, c.out(col.name, p),
				fmt.Sprintf("%s is recorded, and the cell is blocked: %s.", c.out(col.name, p), why),
				closing{fmt.Sprintf("If the cell should run, fix the lift or %s. If not, delete the section by hand;", c.inputsAt()), claims{Leaves: c.out(col.name, p)}})
		}
	}
	if col.lift != nil && col.lift.bytes && !frozen {
		res.via = true
		if !r.update {
			dir := col.lift.viaDir(c.prefix)
			for _, p := range slices.Sorted(maps.Keys(cf.product)) {
				ns.addFile(aboutProductOrphan, dir+p,
					fmt.Sprintf("%s is recorded, and %s is blocked: %s, so it has no product in this run.", dir+p, col.name, why),
					closing{claims: claims{Removes: dir + p}})
			}
		}
	}
	if len(ns) > 0 {
		report(t, header, ns, r.staging(c, col, res.fate, false, frozen, o.Format()))
		return
	}
	switch {
	case b.missing:
		t.Skipf("%s: %s is blocked: %s holds no %s, the outcome of the lift %s, which the cells past it read in a normal run.", c.name, col.name, r.rel(c.path), b.outcome, b.lift.name)
	case b.outcome != "":
		t.Skipf("%s: %s is blocked: %s, %s, records the lift %s's outcome as %q.", c.name, col.name, r.rel(c.path), b.outcome, b.lift.name, b.committed)
	case c.state.runs[b.lift].out.noProduct:
		t.Skipf("%s: %s is blocked: the lift %s failed on the case: it returned no product and no error.", c.name, col.name, b.failed)
	default:
		t.Skipf("%s: %s is blocked: the lift %s failed on the case, and %s, %s, records its error.", c.name, col.name, b.failed, r.rel(c.path), c.out(b.failed, errorFile))
	}
}

// why says, in a message about a cell, what blocks it.
func (b *blockage) why(col *column) string {
	switch {
	case b.missing:
		return fmt.Sprintf("the lift %s, on %s's path, has no committed outcome, %s, which a normal run reads", b.lift.name, col.name, b.outcome)
	case b.outcome != "":
		return fmt.Sprintf("the lift %s, on %s's path, has the committed outcome %q, in %s", b.lift.name, col.name, b.committed, b.outcome)
	}
	return fmt.Sprintf("the lift %s, on %s's path, failed on the case", b.failed, col.name)
}

// ranNothing records the outcome of a cell that ran nothing, its one file:
// a normal run compares it with its golden, and an update writes it. A
// frozen cell, one a gaps line marks that has committed files, keeps them
// as they are: its line fails it already, and since the run made none of
// its files but the outcome, the upgrade that adds a missing outcome to a
// frozen cell whose files match the run's doesn't apply.
func (r *runner) ranNothing(ns *notes, c *aCase, col *column, o archive.Outcome, frozen bool, goldens map[string][]byte, res *cellResult) {
	data := o.Format()
	res.ran, res.writable, res.files = true, true, map[string][]byte{outcomeFile: data}
	if frozen {
		res.writable = false
		return
	}
	if r.update {
		return
	}
	u := r.updateCmd()
	switch g, ok := goldens[outcomeFile]; {
	case !ok:
		ns.addFile(aboutOutcome, c.out(col.name, outcomeFile), fmt.Sprintf("%s has no golden.", c.out(col.name, outcomeFile)), closing{claims: claims{Writes: c.out(col.name, outcomeFile)}})
	case !bytes.Equal(g, data):
		ns.addFile(aboutOutcome, c.out(col.name, outcomeFile), fmt.Sprintf("%s differs (-golden +got):\n%s", c.out(col.name, outcomeFile), strings.TrimSuffix(diff.Lines(g, data), "\n")), closing{advice: fmt.Sprintf("If the change is intended, run %s and read the diff before committing.", u)})
	}
}

// outcome is a cell's outcome, from what its run did, which its column's
// classifier, if any, refines. It reports a code, a classifier or a detail
// that fails, and a lift that returned neither a product nor an error; the
// cell then has no outcome, and an update writes nothing for it. Each fails
// the cell whether or not a gaps line marks it: the line freezes the code's
// verdict, and such a cell has none.
func (r *runner) outcome(ns *notes, col *column, in any, out *called, files map[string][]byte) (archive.Outcome, bool) {
	var o archive.Outcome
	switch {
	case out.panicked:
		return archive.Outcome{State: archive.Panicked}, true
	case out.noProduct:
		ns.add(aboutFault, fmt.Sprintf("%s returned no product and no error, so the cell has no outcome.", col.name), closing{"Fix the lift: it returns its product, or an error that says why it has none. Returning neither is never an expected error.", claims{Nothing: true}})
		return o, false
	case out.jennyErr != nil:
		// The cell's problems say why: its jenny's fault.
		return o, false
	case out.err != nil:
		o.State = archive.Rejected
		codes, ok := r.errorCodes(ns, col, out.err)
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
		ns.add(aboutFault, fmt.Sprintf("%s's classifier %s panicked: %v", col.name, cl.name, p.panicValue), closing{"Fix the classifier.", claims{Nothing: true}})
		return o, false
	case err != nil:
		ns.add(aboutFault, fmt.Sprintf("%s's classifier %s failed: %v", col.name, cl.name, err), closing{"Fix the classifier.", claims{Nothing: true}})
		return o, false
	case detail == "":
	default:
		if err := archive.CheckDetail(detail); err != nil {
			ns.add(aboutFault, fmt.Sprintf("%s's classifier %s gave a detail an outcome can't hold: %v.", col.name, cl.name, err), closing{"Fix the classifier.", claims{Nothing: true}})
			return o, false
		}
		o.Detail = detail
	}
	return o, true
}

// errorCodes gives an error's codes, through the project's Codes: sorted,
// each once. It reports a Codes that panics, or gives a code an outcome
// can't hold.
func (r *runner) errorCodes(ns *notes, col *column, err error) ([]string, bool) {
	if r.codes == nil {
		return nil, true
	}
	var codes []string
	p := call(func(*called) { codes = r.codes(err) })
	if p.panicked {
		ns.add(aboutFault, fmt.Sprintf("The project's Codes panicked on %s's error: %v", col.name, p.panicValue), closing{"Fix Codes.", claims{Nothing: true}})
		return nil, false
	}
	for _, code := range codes {
		if err := archive.CheckCode(code); err != nil {
			ns.add(aboutFault, fmt.Sprintf("The project's Codes gave %s's error a code an outcome can't hold: %v.", col.name, err), closing{"Fix Codes.", claims{Nothing: true}})
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
// may write the cell. view is the cell's input view, which a renderer of
// the error reads.
func (r *runner) outputs(ns *notes, c *aCase, col *column, view fs.FS, out *called) (files map[string][]byte, unjudged map[string]bool, writable bool) {
	files, unjudged, writable = map[string][]byte{}, map[string]bool{}, true
	for _, f := range out.files {
		switch {
		case !fs.ValidPath(f.Path) || f.Path == ".":
			ns.add(aboutFault, fmt.Sprintf("The run returned an output at %q, which is not a valid slash path under %s.", f.Path, c.outDir(col.name)), closing{"Fix the code.", claims{Nothing: true}})
			writable = false
		case slices.Contains(frameworkFiles, f.Path):
			ns.add(aboutFault, fmt.Sprintf("The run returned an output at %s, a name the framework owns under %s.", f.Path, c.outDir(col.name)), closing{"Rename the output.", claims{Nothing: true}})
			writable = false
		case files[f.Path] != nil:
			ns.add(aboutFault, fmt.Sprintf("The run returned two outputs at %s.", f.Path), closing{"Fix the code.", claims{Nothing: true}})
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
		data, ok := r.render(ns, col, view, out.err)
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
		ns.add(aboutFault, fmt.Sprintf("%s can't be held in an archive: %s.", p.Section, p.Msg), closing{"Fix the code.", claims{Nothing: true}})
		writable = false
	}
	for _, p := range slices.Sorted(maps.Keys(files)) {
		if unjudged[p] {
			continue
		}
		switch hit := col.declared(p); len(hit) {
		case 0:
			ns.add(aboutFault, fmt.Sprintf("%s%s is not declared.", c.out(col.name, p), holds(p)), closing{fmt.Sprintf("Declare it in %s's Outputs, with a description, or fix the code.", col.name), claims{Nothing: true}})
			writable = false
			unjudged[p] = true
		case 1:
		default:
			ns.add(aboutFault, fmt.Sprintf("%s matches two of %s's output declarations, %s and %s.", c.out(col.name, p), col.name, hit[0], hit[1]), closing{"Change the patterns so that one declaration matches each file.", claims{Nothing: true}})
			writable = false
			unjudged[p] = true
		}
	}
	for _, p := range slices.Sorted(maps.Keys(files)) {
		if err := archive.Holds(files[p]); err != nil && !unjudged[p] {
			ns.add(aboutFault, fmt.Sprintf("%s can't be held in an archive: %v.", c.out(col.name, p), err), closing{"Fix the code so the output can be held.", claims{Nothing: true}})
			writable = false
			unjudged[p] = true
		}
	}
	return files, unjudged, writable
}

// render renders an error a column's function returned, for error.txt:
// through the project's RenderError, which receives the cell's input view,
// or as the error's text. It reports a RenderError that panics, or whose
// bytes don't end with a newline.
func (r *runner) render(ns *notes, col *column, view fs.FS, err error) ([]byte, bool) {
	if r.renderError == nil {
		return []byte(err.Error() + "\n"), true
	}
	var data []byte
	p := call(func(*called) { data = r.renderError(view, err) })
	switch {
	case p.panicked:
		ns.add(aboutFault, fmt.Sprintf("The project's RenderError panicked on %s's error: %v", col.name, p.panicValue), closing{"Fix RenderError.", claims{Nothing: true}})
		return []byte{}, false
	case !bytes.HasSuffix(data, []byte("\n")):
		ns.add(aboutFault, fmt.Sprintf("The project's RenderError returned %q for %s's error, which doesn't end with a newline.", data, col.name), closing{"Its output must end with a newline: fix RenderError.", claims{Nothing: true}})
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
func (r *runner) expectedError(ns *notes, c *aCase, col *column, out *called) {
	hit := col.rejectsAny(c.properties)
	errText := func() string { return indent(strings.TrimSuffix(out.err.Error(), "\n"), "  ") }
	switch {
	case out.panicked && out.inProbe:
		ns.add(aboutFault, fmt.Sprintf("%s's probe panicked: %v", col.name, out.panicValue), closing{advice: "Fix the probe. A panic is never an expected error."})
	case out.panicked:
		ns.add(aboutFault, fmt.Sprintf("%s panicked: %v", col.name, out.panicValue), closing{advice: "Fix the code. A panic is never an expected error."})
	case out.noProduct || out.jennyErr != nil:
	case out.err != nil && len(hit) == 0 && c.tr != nil:
		ns.add(aboutFault,
			fmt.Sprintf("%s returned an error, but the variant states no property %s rejects:\n  %s\n%s", col.name, col.name, errText(), c.tr.states()),
			closing{advice: fmt.Sprintf("Fix the code or %s's jenny, or give the variant the property, in %s's Gives or Keeps, and list it in %s's Rejects.", c.tr.name, c.tr.name, col.name)})
	case out.err != nil && len(hit) == 0:
		ns.add(aboutFault,
			fmt.Sprintf("%s returned an error, but the case states no property %s rejects:\n  %s", col.name, col.name, errText()),
			closing{advice: fmt.Sprintf("Fix the code, or state the property on the case and in %s's Rejects.", col.name)})
	case out.err == nil && len(hit) > 0 && c.tr != nil:
		ns.add(aboutFault,
			fmt.Sprintf("%s returned no error, but the variant states %s, which %s rejects.\n%s", col.name, andList(hit), col.name, c.tr.states()),
			closing{advice: fmt.Sprintf("Fix the code or %s's jenny, or correct %s's Gives or Keeps, or %s's Rejects.", c.tr.name, c.tr.name, col.name)})
	case out.err == nil && len(hit) > 0:
		ns.add(aboutFault,
			fmt.Sprintf("%s returned no error, but the case states %s, which %s rejects.", col.name, andList(hit), col.name),
			closing{advice: fmt.Sprintf("Fix the code, or correct the case's properties or %s's Rejects.", col.name)})
	}
}

// compare compares a cell's files with its goldens, and reports orphans.
// In a cell no update writes, a file with no golden isn't reported, and
// one that differs says the cell's problems come first.
func (r *runner) compare(ns *notes, c *aCase, col *column, files, goldens map[string][]byte, unjudged map[string]bool, writable bool) {
	if !r.update {
		for _, p := range slices.Sorted(maps.Keys(files)) {
			if !unjudged[p] {
				kind := aboutFile
				if p == outcomeFile {
					kind = aboutOutcome
				}
				g, ok := goldens[p]
				r.compareFile(ns, kind, c.out(col.name, p), g, ok, files[p], writable)
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
		ns.addFile(aboutOrphan, c.out(col.name, p), fmt.Sprintf("%s %s.", c.out(col.name, p), what), closing{"If the run should still write it, fix the code. If not, delete the section by hand;", claims{Leaves: c.out(col.name, p)}})
	}
}

// compareFile compares one file a cell made with its golden, in a normal
// run, as a note of the given kind: name is its section, and ok says
// whether the case commits it. In a cell no update writes, a file with no
// golden isn't one to write, so it isn't reported, and one that differs
// says the cell's problems come first.
func (r *runner) compareFile(ns *notes, kind noteKind, name string, golden []byte, ok bool, got []byte, writable bool) {
	u := r.updateCmd()
	switch {
	case !ok && !writable:
	case !ok:
		ns.addFile(kind, name, fmt.Sprintf("%s has no golden.", name), closing{claims: claims{Writes: name}})
	case !bytes.Equal(golden, got) && !writable:
		ns.addFile(kind, name, fmt.Sprintf("%s differs (-golden +got):\n%s", name, strings.TrimSuffix(diff.Lines(golden, got), "\n")), closing{"Fix the problems above first.", claims{Nothing: true}})
	case !bytes.Equal(golden, got):
		ns.addFile(kind, name, fmt.Sprintf("%s differs (-golden +got):\n%s", name, strings.TrimSuffix(diff.Lines(golden, got), "\n")), closing{advice: fmt.Sprintf("If the change is intended, run %s and read the diff before committing.", u)})
	}
}

// compareProduct compares a bytes lift's product with the one the case
// commits, under via/<lift>/, in a normal run, as compare does a cell's
// files with its goldens. An update that writes the lift's cell replaces
// the committed product whole, so a committed file the lift doesn't make
// is no orphan: the update removes it.
func (r *runner) compareProduct(ns *notes, c *aCase, l *lift, out *called, product, committed map[string][]byte, writable bool) {
	if r.update {
		return
	}
	dir := l.viaDir(c.prefix)
	for _, p := range slices.Sorted(maps.Keys(product)) {
		g, ok := committed[p]
		r.compareFile(ns, aboutFile, dir+p, g, ok, product[p], writable)
	}
	failed := out.panicked || out.err != nil
	for _, p := range slices.Sorted(maps.Keys(committed)) {
		if _, ok := product[p]; ok {
			continue
		}
		should := fmt.Sprintf("If it should, fix %s's jenny.", l.name)
		if failed {
			should = fmt.Sprintf("If it should make one, fix %s or %s.", l.name, c.inputsAt())
		}
		cl := closing{should + " If not,", claims{Removes: dir + p}}
		if !writable {
			cl = closing{should + " If not, fix the problems above first.", claims{Nothing: true}}
		}
		if failed {
			ns.addFile(aboutProductOrphan, dir+p,
				fmt.Sprintf("%s is recorded, and %s failed on the case in this run, so this run has no product.", dir+p, l.name),
				cl)
			continue
		}
		ns.addFile(aboutProductOrphan, dir+p,
			fmt.Sprintf("%s is a file of %s's committed product, which this run of %s doesn't make.", dir+p, l.name, l.name),
			cl)
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
