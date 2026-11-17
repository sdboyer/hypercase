package step09

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/walkthrough/sheets/step09/sheetcorpus"
)

// TestReport runs hypercase report over a change to this corpus root, as a
// person does on a branch. In a scratch repository, it commits the toy as
// it is on main, makes the change on the branch change, runs the update
// there, commits again, and reports from the merge base to HEAD, in
// step09. The ranked changes, as plain text, are testdata/report.txt, and
// the page is testdata/report.html. The update rewrites both.
//
// The scratch repository's commits hold what the report reads, and nothing
// else: go.mod, which places the module root, and the corpus root's
// registries and archives. The Go source the update builds, and the corpus
// checks' findings, which the report doesn't read, are in its working tree,
// out of the commits, so the page's commits move only when what the report
// reads does.
//
// The toy is copied from this module's working tree. In the module's update,
// another package can write the corpus while this test copies it, so its
// goldens, like a corpus check's findings, can take a second update to
// settle.
func TestReport(t *testing.T) {
	update := updating(t)
	hypercase := buildHypercase(t)
	repo := t.TempDir()
	modfile := scratch(t, repo)
	commit(t, repo, "main")

	git(t, repo, "switch", "-q", "-c", "change")
	for _, e := range change {
		e.apply(t, repo)
	}
	runUpdate(t, repo, modfile)
	commit(t, repo, "change")

	base := strings.TrimSpace(git(t, repo, "merge-base", "main", "HEAD"))
	dir := filepath.Join(repo, "step09")
	golden(t, update, "testdata/report.txt", report(t, hypercase, dir, "-text", base, "HEAD"))
	golden(t, update, "testdata/report.html", report(t, hypercase, dir, base, "HEAD"))
}

// An edit is one change to a file of the scratch module, by slash path
// from its root: the file renamed to, or else old replaced by new, once.
type edit struct {
	path     string
	to       string
	old, new string
}

// change is the branch's change: three edits, each of which moves cells of
// one rank or two.
var change = []edit{
	// calc gains a classifier, which refines the outcome of each cell where
	// calc warns: outcome changed.
	{path: "step09/cli/corpus_test.go", old: calcRun, new: calcRun + warned},
	// blank is renamed empty: the cells of blank disappear, and the cells of
	// empty appear.
	{path: "step09/testdata/sheet/blank.txtar", to: "step09/testdata/sheet/empty.txtar"},
	// A file that is not a cell is worded otherwise, under the same
	// outcome: bytes changed.
	{path: "sheet/sheet.go", old: `"%s: not a cell name"`, new: `"%s: not the name of a cell"`},
}

const calcRun = `	Run: func(t testing.TB, archive string, in sheetcorpus.Sheet, stdout, stderr io.Writer) ([]hypercase.File, error) {
		return nil, sheet.Calc(in, stdout, stderr)
	},
`

const warned = `	Classifier: hypercase.Classifier[sheetcorpus.Sheet]{
		Name:        "warned",
		Description: "Adds warned when calc writes a warning.",
		Classify: func(in sheetcorpus.Sheet, got hypercase.Result) (string, error) {
			if len(got.File("stderr.txt")) > 0 {
				return "warned", nil
			}
			return "", nil
		},
	},
`

func (e edit) apply(t *testing.T, repo string) {
	t.Helper()
	p := filepath.Join(repo, filepath.FromSlash(e.path))
	if e.to != "" {
		if err := os.Rename(p, filepath.Join(repo, filepath.FromSlash(e.to))); err != nil {
			t.Fatal(err)
		}
		return
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(data, []byte(e.old)); n != 1 {
		t.Fatalf("%s holds %q %d times, and the change replaces it once", e.path, e.old, n)
	}
	if err := os.WriteFile(p, bytes.Replace(data, []byte(e.old), []byte(e.new), 1), 0o644); err != nil {
		t.Fatal(err)
	}
}

// updating reports whether the test runs in the project's update.
func updating(t *testing.T) bool {
	t.Helper()
	switch v := os.Getenv(sheetcorpus.Project.UpdateVar); v {
	case "":
		return false
	case "1":
		return true
	default:
		t.Fatalf("%s is set to %q; set it to 1 to run an update, or unset it", sheetcorpus.Project.UpdateVar, v)
		return false
	}
}

// golden compares got with the file at name, or writes it there in an
// update.
func golden(t *testing.T, update bool, name string, got []byte) {
	t.Helper()
	if update {
		if err := os.WriteFile(name, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("%v\nRun %s=1 go test ./... to write it, and read it.", err, sheetcorpus.Project.UpdateVar)
	}
	if !bytes.Equal(got, want) {
		n, g, w := firstDiff(got, want)
		t.Errorf("%s differs from the report, first at line %d:\n\treport: %s\n\tgolden: %s\nIf the change is intended, run %s=1 go test ./... and read the diff.", name, n, g, w, sheetcorpus.Project.UpdateVar)
	}
}

// firstDiff returns the first line, counted from 1, at which got and want
// differ, and that line of each, quoted, or <end> where one has no such
// line.
func firstDiff(got, want []byte) (n int, g, w string) {
	gl, wl := strings.SplitAfter(string(got), "\n"), strings.SplitAfter(string(want), "\n")
	for n = 0; n < len(gl) && n < len(wl) && gl[n] == wl[n]; n++ {
	}
	line := func(lines []string) string {
		if n < len(lines) && lines[n] != "" {
			return strconv.Quote(lines[n])
		}
		return "<end>"
	}
	return n + 1, line(gl), line(wl)
}

// goEnv is the environment of each go command the test runs: this
// process's, which holds where the toolchain keeps its caches, with no
// GOFLAGS, the module system offline, on this toolchain and off any
// workspace, and the update variable only as updateVar sets it.
func goEnv(updateVar string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case sheetcorpus.Project.UpdateVar, "GOFLAGS", "GOWORK", "GOPROXY", "GOTOOLCHAIN":
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "GOFLAGS=", "GOWORK=off", "GOPROXY=off", "GOTOOLCHAIN=local")
	if updateVar != "" {
		env = append(env, updateVar)
	}
	return env
}

// buildHypercase builds the hypercase command into a temp dir, and returns
// its path.
//
// The command's source is outside this module, where go test doesn't look
// for what a test read, and the test's binary doesn't import the command or
// the report's packages. So go test can reuse a cached pass after a change
// to the report: run with -count=1 to see one.
func buildHypercase(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "hypercase")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", exe, "github.com/sdboyer/hypercase/cmd/hypercase")
	cmd.Env = goEnv("")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building hypercase: %v\n%s", err, out)
	}
	return exe
}

// scratch copies the toy into repo, a new git repository on the branch
// main: the module's go.mod, the sheet package, and step09. The module's
// go.mod replaces hypercase with a path relative to it, which the copy
// can't follow, so the copy builds with the go.mod scratch returns, which
// replaces hypercase by an absolute path, with the module's go.sum beside
// it in a temp dir outside the repository.
func scratch(t *testing.T, repo string) (modfile string) {
	t.Helper()
	module, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"sheet", "step09"} {
		copyDir(t, filepath.Join(module, dir), filepath.Join(repo, dir))
	}
	copyFile(t, filepath.Join(module, "go.mod"), filepath.Join(repo, "go.mod"))

	const replace = "replace github.com/sdboyer/hypercase => ../..\n"
	mod, err := os.ReadFile(filepath.Join(module, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(mod, []byte(replace)); n != 1 {
		t.Fatalf("go.mod holds %q %d times, and the copy rewrites it once", replace, n)
	}
	abs := "replace github.com/sdboyer/hypercase => " + filepath.Join(module, "..", "..") + "\n"
	build := t.TempDir()
	modfile = filepath.Join(build, "go.mod")
	if err := os.WriteFile(modfile, bytes.Replace(mod, []byte(replace), []byte(abs), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	copyFile(t, filepath.Join(module, "go.sum"), filepath.Join(build, "go.sum"))

	git(t, repo, "init", "-q", "-b", "main")
	return modfile
}

func copyDir(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(from, p)
		if err != nil {
			return err
		}
		copyFile(t, p, filepath.Join(to, rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFile(to, data); err != nil {
		t.Fatal(err)
	}
}

// writeFile writes data to the file at name, making its directory first.
func writeFile(name string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	return os.WriteFile(name, data, 0o644)
}

// read is what the report reads, as pathspecs: everything the commits hold.
var read = []string{
	"go.mod",
	":(glob)step09/**/testdata/hypercase.json",
	":(glob)step09/testdata/*/*.txtar",
}

// commit commits what the report reads, as it is in the working tree.
func commit(t *testing.T, repo, msg string) {
	t.Helper()
	git(t, repo, append([]string{"add", "-A", "--"}, read...)...)
	git(t, repo, "commit", "-q", "-m", msg)
}

// TestCommit: a commit holds go.mod, the registries and the archives, and
// none of the rest of the toy: its Go source, its corpus check's findings,
// which the report doesn't read, and the report's own goldens.
func TestCommit(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	for _, name := range []string{
		"go.mod",
		"sheet/sheet.go",
		"step09/corpus_test.go",
		"step09/cli/testdata/hypercase.json",
		"step09/testdata/hypercase.json",
		"step09/testdata/checks/codes.txt",
		"step09/testdata/report.txt",
		"step09/testdata/sheet/notes.txtar",
	} {
		if err := writeFile(filepath.Join(repo, filepath.FromSlash(name)), []byte(name+"\n")); err != nil {
			t.Fatal(err)
		}
	}
	commit(t, repo, "main")
	got := git(t, repo, "ls-tree", "-r", "--name-only", "HEAD")
	const want = "go.mod\nstep09/cli/testdata/hypercase.json\nstep09/testdata/hypercase.json\nstep09/testdata/sheet/notes.txtar\n"
	if got != want {
		t.Errorf("the commit holds:\n%s\nwant:\n%s", got, want)
	}
}

// runUpdate runs the update of step09's packages in the scratch module,
// then their normal run, which fails unless the update wrote everything.
// One pass is enough: the change moves no code that the corpus check codes
// counts, so its findings don't depend on which package writes first. Both
// build with -trimpath, which keeps the scratch module's directory, new in
// each run, out of the build cache's keys, so that the cache serves them.
func runUpdate(t *testing.T, repo, modfile string) {
	t.Helper()
	gotest := func(env []string) ([]byte, error) {
		cmd := exec.Command("go", "test", "-modfile="+modfile, "-trimpath", "-count=1", "-run=^TestCorpus$", "./step09/...")
		cmd.Dir = repo
		cmd.Env = env
		return cmd.CombinedOutput()
	}
	if out, err := gotest(goEnv(sheetcorpus.Project.UpdateVar + "=1")); err != nil {
		t.Fatalf("the update: %v\n%s", err, out)
	}
	if out, err := gotest(goEnv("")); err != nil {
		t.Fatalf("the normal run after the update: %v\n%s", err, out)
	}
}

// gitEnv is the environment of each git command, the report's included:
// git's configuration files switched off, the author, committer and dates
// fixed, and messages in English, so the commits, and what the report
// writes, are the same wherever the test runs.
func gitEnv() []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"LC_ALL=C",
		"GIT_CEILING_DIRECTORIES=" + os.TempDir(),
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Sheets",
		"GIT_AUTHOR_EMAIL=sheets@example.com",
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z",
		"GIT_COMMITTER_NAME=Sheets",
		"GIT_COMMITTER_EMAIL=sheets@example.com",
		"GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, exit.Stderr)
		}
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}

// report runs hypercase report in dir, and returns what it writes on
// stdout. It fails the test unless the command exits 0 and writes nothing
// on stderr.
func report(t *testing.T, hypercase, dir string, args ...string) []byte {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(hypercase, append([]string{"report"}, args...)...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("hypercase report %s: %v\n%s", strings.Join(args, " "), err, stderr.Bytes())
	}
	if stderr.Len() > 0 {
		t.Errorf("hypercase report %s wrote to stderr:\n%s", strings.Join(args, " "), stderr.Bytes())
	}
	return stdout.Bytes()
}
