package hypercase

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/diff"
	"github.com/sdboyer/hypercase/internal/root"
)

// Test runs every case through every column the package registered, as
// subtests of t: one per case, named for its archive, and one per cell
// within it, named for its column, so TestCorpus/pancakes/scale is the cell
// of the case pancakes in the column scale.
//
// Without the project's update variable, Test only reads: each output is
// compared with its golden. With it set to 1, Test writes the goldens.
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

	dir := filepath.Join(r.pkgDir, "testdata")
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("reading %s: %v", r.rel(dir), err)
	}
	var archives []string
	for _, e := range entries {
		switch {
		case archive.IsTemp(e.Name()):
			t.Errorf("%s is a temp file left by a write that was cut short.\nDelete it.", r.rel(filepath.Join(dir, e.Name())))
		case !e.IsDir() && strings.HasSuffix(e.Name(), ".txtar"):
			archives = append(archives, e.Name())
		}
	}
	for _, name := range archives {
		path := filepath.Join(dir, name)
		caseName := strings.TrimSuffix(name, ".txtar")
		if !validName(caseName) {
			t.Errorf("%s: the archive's name, %q, breaks the naming rule: %s.\nRename the archive.", r.rel(path), caseName, nameRule)
			continue
		}
		t.Run(caseName, func(t *testing.T) { r.runCase(t, path, caseName) })
	}
}

// runner is one call of Test.
type runner struct {
	update     bool
	updateVar  string
	moduleRoot string
	pkgDir     string
	columns    []*column // sorted by name
}

func newRunner(t *testing.T) (*runner, bool) {
	t.Helper()
	reg.mu.Lock()
	project, columns := reg.project, slices.Clone(reg.columns)
	reg.mu.Unlock()
	if project == nil {
		t.Errorf("the package registered no decl.Project.\nRegister the project, with its UpdateVar, from an init function.")
		return nil, false
	}
	r := &runner{updateVar: project.UpdateVar, columns: columns}
	slices.SortFunc(r.columns, func(a, b *column) int { return strings.Compare(a.name, b.name) })
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
	return r, true
}

// updateCmd is the command that runs an update.
func (r *runner) updateCmd() string {
	return r.updateVar + "=1 go test ./..."
}

// rel returns a path relative to the module root, with slashes, so a
// message reads the same wherever go test ran.
func (r *runner) rel(p string) string { return root.Rel(r.moduleRoot, p) }

func (r *runner) checkDescriptions(t *testing.T) {
	t.Helper()
	for _, c := range r.columns {
		if strings.TrimSpace(c.description) == "" {
			t.Errorf("the suite %s has no description.\nAdd one: it says what the suite does, in prose.", c.name)
		}
	}
}

// aCase is one case: an archive's base.
type aCase struct {
	name    string
	archive *archive.Archive
	view    fs.FS
}

// goldens returns a column's committed files, by path under its directory.
func (c *aCase) goldens(col string) map[string][]byte {
	g := map[string][]byte{}
	prefix := archive.Out + "/" + col + "/"
	for _, s := range c.archive.Sections {
		if p, ok := strings.CutPrefix(s.Name, prefix); ok {
			g[p] = s.Data
		}
	}
	return g
}

func (c *aCase) header() string {
	return c.name + ": " + indent(strings.TrimSpace(string(c.archive.Comment)), "  ")
}

func (r *runner) runCase(t *testing.T, path, name string) {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", r.rel(path), err)
	}
	a := archive.Parse(data)
	rel := r.rel(path)
	if ps := a.Check(); len(ps) > 0 {
		t.Errorf("%s has problems of form, so none of its cells run, and no update writes it:\n%s\nFix the archive by hand.", rel, problemList(ps))
		return
	}
	if ps := a.Description(); len(ps) > 0 {
		t.Errorf("%s: %s\nWrite the case's description, in prose, as the archive's comment, the text before its first section.", rel, problemLines(ps))
	}
	if !r.update && !archive.Placed(data) {
		t.Errorf("%s has update-written sections out of their place.\nRun %s to put them in place.", rel, r.updateCmd())
	}
	r.unclaimed(t, rel, a)

	c := &aCase{name: name, archive: a, view: a.View()}
	results := make([]*cellResult, len(r.columns))
	for i, col := range r.columns {
		res := &cellResult{}
		results[i] = res
		t.Run(col.name, func(t *testing.T) { r.runCell(t, c, col, res) })
	}
	if !r.update {
		return
	}
	next := a.Clone()
	for i, col := range r.columns {
		res := results[i]
		if !res.ran || !res.writable {
			continue
		}
		for _, p := range sortedKeys(res.files) {
			setSection(next, archive.Out+"/"+col.name+"/"+p, res.files[p])
		}
	}
	next = next.Place()
	if bytes.Equal(next.Format(), data) {
		return
	}
	if err := archive.Write(path, next); err != nil {
		t.Errorf("writing %s: %v", rel, err)
	}
}

// unclaimed reports each output section under a column the package doesn't
// register.
func (r *runner) unclaimed(t *testing.T, rel string, a *archive.Archive) {
	t.Helper()
	for _, s := range a.Sections {
		rest, ok := strings.CutPrefix(s.Name, archive.Out+"/")
		if !ok {
			continue
		}
		col, _, _ := strings.Cut(rest, "/")
		if !slices.ContainsFunc(r.columns, func(c *column) bool { return c.name == col }) {
			t.Errorf("%s: %s is under out/%s/, and no column of that name is registered.\nIf a column should write it, register the column. If not, delete the section by hand; %s leaves it in place.", rel, s.Name, col, r.updateCmd())
		}
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
}

// The names the framework owns directly under a column's directory.
const (
	errorFile   = "error.txt"
	stdoutFile  = "stdout.txt"
	stderrFile  = "stderr.txt"
	outcomeFile = "outcome"
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
	res.ran = true
	finished := false
	header := c.header() + "\n" + col.name + ": " + col.description
	failedTest := func() {
		what := "The run failed its test, through its testing.TB."
		if r.update {
			what += " The update writes nothing for this cell."
		}
		t.Logf("%s\n%s", header, what)
	}
	defer func() {
		if finished {
			return
		}
		// The run called FailNow or SkipNow, through its testing.TB.
		if t.Skipped() {
			t.Errorf("%s\nThe run skipped its cell. A run can't skip a case; a probe decides which cases are for a suite.", header)
			return
		}
		failedTest()
	}()
	out := call(func(o *called) {
		o.files, o.err = col.run(t, c.name, c.view, &o.stdout, &o.stderr)
	})
	finished = true
	if t.Failed() {
		failedTest()
		return
	}

	m := &msg{}
	files := map[string][]byte{}
	writable := true
	for _, f := range out.files {
		switch {
		case !fs.ValidPath(f.Path) || f.Path == ".":
			m.add("The run returned an output at %q, which is not a valid slash path under out/%s/.\nFix the code. An update writes nothing for this cell.", f.Path, col.name)
			writable = false
		case slices.Contains(frameworkFiles, f.Path):
			m.add("The run returned an output at %s, a name the framework owns under out/%s/.\nRename the output. An update writes nothing for this cell.", f.Path, col.name)
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
	if p, q, ok := fileDirConflict(files); ok {
		m.add("The run returned an output at %s and another at %s, below it.\nFix the code. An update writes nothing for this cell.", p, q)
		writable = false
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
		m.add("%s panicked: %v\nFix the code. A panic is never an expected error.", col.name, out.panicValue)
		defer t.Logf("stack of the panic:\n%s", out.stack)
	case out.err != nil:
		files[errorFile] = []byte(out.err.Error() + "\n")
		m.add("%s returned an error:\n  %s\nFix the code.", col.name, indent(strings.TrimSuffix(out.err.Error(), "\n"), "  "))
	}
	unheld := map[string]bool{}
	for _, p := range sortedKeys(files) {
		if err := archive.Holds(files[p]); err != nil {
			m.add("out/%s/%s can't be held in an archive: %v.\nFix the code so the output can be held. An update writes nothing for this cell.", col.name, p, err)
			writable = false
			unheld[p] = true
		}
	}

	goldens := c.goldens(col.name)
	if !r.update {
		for _, p := range sortedKeys(files) {
			g, ok := goldens[p]
			switch {
			case unheld[p]:
			case !ok:
				m.add("out/%s/%s has no golden.\nRun %s to write it, and read it before committing.", col.name, p, r.updateCmd())
			case !bytes.Equal(g, files[p]):
				m.add("out/%s/%s differs (-golden +got):\n%s\nIf the change is intended, run %s and read the diff before committing.", col.name, p, strings.TrimSuffix(diff.Lines(g, files[p]), "\n"), r.updateCmd())
			}
		}
	}
	for _, p := range sortedKeys(goldens) {
		if _, ok := files[p]; ok {
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
		m.add("out/%s/%s %s.\nIf the run should still write it, fix the code. If not, delete the section by hand; %s leaves it in place.", col.name, p, what, r.updateCmd())
	}
	if !m.empty() {
		t.Errorf("%s\n%s", header, m)
	}
	res.writable = writable
	res.files = files
}

// fileDirConflict reports an output that is also the directory of another.
func fileDirConflict(files map[string][]byte) (string, string, bool) {
	for _, p := range sortedKeys(files) {
		for d := path.Dir(p); d != "."; d = path.Dir(d) {
			if _, ok := files[d]; ok {
				return d, p, true
			}
		}
	}
	return "", "", false
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

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}
