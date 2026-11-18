package step11

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step11/sheetcorpus"
)

// TestReport runs hypercase report over a change to this corpus root, as a
// person does on a branch. In a scratch repository, it runs the update on
// the toy as it is on main and commits it, makes the change on the branch
// change, runs the update there, commits again, and reports from the merge
// base to HEAD, in step11. The ranked changes, as plain text, are
// testdata/report.txt, and the page is testdata/report.html. The update
// rewrites both.
//
// The scratch repository's commits hold what the report reads, and nothing
// else: go.mod, which places the module root, the corpus root's registries
// and archives, and its packages' corpus checks' findings. The Go source
// the update builds is in its working tree, out of the commits, so the
// page's commits move only when what the report reads does.
//
// The toy is copied from this module's working tree, which, in the module's
// update, another package can be writing while this test copies it. Each
// file is copied as it was before such a write or after it, and the update
// before each commit rewrites what the toy's code writes, so each revision
// holds what the code writes for it either way. The test makes a cell of the
// copy stale, as such a copy can find it, to show that main's update
// rewrites it. After a strip, the copy can hold none of that cell's
// sections, and the test then adds the one it makes stale.
func TestReport(t *testing.T) {
	update := updating(t)
	hypercase := buildHypercase(t)
	repo := t.TempDir()
	modfile := scratch(t, repo)
	stale(t, repo)
	runUpdate(t, repo, modfile)
	commit(t, repo, "main")

	git(t, repo, "switch", "-q", "-c", "change")
	for _, e := range change {
		e.apply(t, repo)
	}
	runUpdate(t, repo, modfile)
	commit(t, repo, "change")

	base := strings.TrimSpace(git(t, repo, "merge-base", "main", "HEAD"))
	dir := filepath.Join(repo, "step11")
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

// change is the branch's change: six edits. Each of the first three moves
// cells of one rank or two, as at step 9. The last three move what step 11
// adds to the report: input lines, ranges alone, and findings.
var change = []edit{
	// calc gains a classifier, which refines the outcome of each cell where
	// calc warns: outcome changed.
	{path: "step11/cli/corpus_test.go", old: calcRun, new: calcRun + warned},
	// blank is renamed empty: the cells of blank disappear, and the cells of
	// empty appear.
	{path: "step11/testdata/sheet/blank.txtar", to: "step11/testdata/sheet/empty.txtar"},
	// A file that is not a cell is worded otherwise, under the same
	// outcome: bytes changed.
	{path: "sheet/sheet.go", old: `"%s: not a cell name"`, new: `"%s: not the name of a cell"`},
	// wrapped's A1 becomes a formula the export wraps onto a second line,
	// as it wraps B1: import's A1 and the CSV lines it comes from change,
	// and B1 moves down a line while its file doesn't. The sheet's first
	// cell of more than one line is A1 now, so each error points there.
	{path: "step11/testdata/csv/wrapped.txtar",
		old: "A price and a formula the export wrapped onto a second line, in a quoted field.\n-- in/csv/wrapped.csv --\n4,\"=A1*\n2\"\n",
		new: "Two formulas the export wrapped onto a second line each, in quoted fields.\n-- in/csv/wrapped.csv --\n\"=1+\n3\",\"=A1*\n2\"\n"},
	// twice's variant cycle writes out A1 too, as the base has it: its
	// cells' files don't change, and each range that named the base's A1
	// names the variant's, so they change their ranges alone.
	{path: "step11/testdata/sheet/twice.txtar", old: "-- variant/cycle/in/sheet/B1 --\n", new: "-- variant/cycle/in/sheet/A1 --\n=B1*2\n-- variant/cycle/in/sheet/B1 --\n"},
	// The importer's code for a field past the last column or row is
	// renumbered: past-xfd's import is rejected under the new code, and
	// the findings of codes trade the old code for the new.
	{path: "sheet/error.go", old: `codeTooLarge     = "C103"`, new: `codeTooLarge     = "C104"`},
}

// stale makes a cell of the copy differ from what the code writes for it, as
// a copy taken in the module's update can find a cell the update has yet to
// rewrite: it puts a line before whatever prices' calc stdout.txt holds. In
// the update after a strip, the copy can be taken before step11/cli, the
// package that writes the section, has written it back, so where the copy
// has no such section, stale adds it in its place, holding that line alone.
// The update before main's commit rewrites it, so the report shows no change
// in it.
func stale(t *testing.T, repo string) {
	t.Helper()
	const section = "out/calc/stdout.txt"
	p := filepath.Join(repo, "step11", "testdata", "sheet", "prices.txtar")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	a := archive.Parse(data)
	i := slices.IndexFunc(a.Sections, func(s archive.Section) bool { return s.Name == section })
	if i < 0 {
		a.Sections = append(a.Sections, archive.Section{Name: section})
		i = len(a.Sections) - 1
	}
	a.Sections[i].Data = append([]byte("stale\n"), a.Sections[i].Data...)
	if err := archive.Write(p, a.Place()); err != nil {
		t.Fatal(err)
	}
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
// process's, with no GOFLAGS, the module system offline, on this toolchain
// and off any workspace, and the update variable only as updateVar sets it.
// The go env file applies, as it does to the caller's own go commands, and
// with this process's environment says where the toolchain keeps its
// caches. With the module system offline, a module missing from that
// module cache fails the build.
//
// The go command reads a variable that is empty from the go env file, so
// GOFLAGS is a space, which holds no flag and keeps the file's GOFLAGS out.
// GOWORK, GOPROXY and GOTOOLCHAIN are set, which keeps the file's out too.
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
	env = append(env, "GOFLAGS= ", "GOWORK=off", "GOPROXY=off", "GOTOOLCHAIN=local")
	if updateVar != "" {
		env = append(env, updateVar)
	}
	return env
}

// TestGoEnv: the go env file applies to the go commands the test runs, but
// for its GOFLAGS and the variables goEnv sets. The file, where go env -w
// writes it, holds a module cache's location, which they use, GOFLAGS with
// an overlay, which would build the probe from another source, and a
// GOWORK, GOPROXY and GOTOOLCHAIN, which goEnv's replace. A file that the
// caller's GOENV names elsewhere applies too.
func TestGoEnv(t *testing.T) {
	probe := t.TempDir()
	for name, data := range map[string]string{
		"go.mod":       "module probe\n\ngo 1.27.0\n",
		"main.go":      "package main\n\nfunc main() { println(\"main.go\") }\n",
		"alt/main.txt": "package main\n\nfunc main() { println(\"the overlay\") }\n",
		"overlay.json": `{"Replace": {"main.go": "alt/main.txt"}}`,
	} {
		if err := writeFile(filepath.Join(probe, filepath.FromSlash(name)), []byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	gocmd := func(env []string, args ...string) string {
		t.Helper()
		cmd := exec.Command("go", args...)
		cmd.Dir, cmd.Env = probe, env
		out, err := cmd.Output()
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, exit.Stderr)
			}
			t.Fatalf("go %s: %v", strings.Join(args, " "), err)
		}
		return strings.TrimSpace(string(out))
	}

	// The file's default location is in the user's config directory, which
	// HOME, XDG_CONFIG_HOME or AppData names, by platform. The build cache
	// stays this process's, which serves the probe's build. The module
	// cache, GOWORK, GOPROXY and GOTOOLCHAIN are left to the file, so only
	// goEnv keeps the file's out of them.
	cache := gocmd(nil, "env", "GOCACHE")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("AppData", filepath.Join(home, "AppData"))
	t.Setenv("GOENV", "")
	t.Setenv("GOCACHE", cache)
	for _, k := range []string{"GOMODCACHE", "GOWORK", "GOPROXY", "GOTOOLCHAIN"} {
		t.Setenv(k, "")
	}
	// Telemetry is kept in the same directory, and the first go command
	// there turns it off. Any other go command, run there first, takes the
	// upload token, and starts a process that writes there beside the
	// test's go commands.
	gocmd(nil, "telemetry", "off")
	envFile := gocmd(nil, "env", "GOENV")
	if !strings.HasPrefix(envFile, home+string(filepath.Separator)) {
		t.Fatalf("the go env file is %s, outside the test's HOME, %s", envFile, home)
	}
	modcache := filepath.Join(home, "modcache")
	file := "GOFLAGS=-overlay=overlay.json\nGOMODCACHE=" + modcache + "\n" +
		"GOWORK=" + filepath.Join(home, "go.work") + "\nGOPROXY=https://example.invalid\nGOTOOLCHAIN=path\n"
	if err := writeFile(envFile, []byte(file)); err != nil {
		t.Fatal(err)
	}

	if got := gocmd(goEnv(""), "env", "GOMODCACHE"); got != modcache {
		t.Errorf("the go commands use the module cache %s, not the go env file's, %s", got, modcache)
	}
	if got, want := gocmd(goEnv(""), "env", "GOWORK", "GOPROXY", "GOTOOLCHAIN"), "off\noff\nlocal"; got != want {
		t.Errorf("the go commands' GOWORK, GOPROXY and GOTOOLCHAIN are %q, not goEnv's, %q", got, want)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = probe
	cmd.Env = goEnv("")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run: %v\n%s", err, out)
	}
	if string(out) != "main.go\n" {
		t.Errorf("the probe built from the go env file's GOFLAGS, and wrote %q", out)
	}

	other := filepath.Join(t.TempDir(), "env")
	otherModcache := filepath.Join(home, "other-modcache")
	if err := writeFile(other, []byte("GOMODCACHE="+otherModcache+"\n")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOENV", other)
	if got := gocmd(goEnv(""), "env", "GOMODCACHE"); got != otherModcache {
		t.Errorf("with GOENV=%s, the go commands use the module cache %s, not that file's, %s", other, got, otherModcache)
	}
}

// buildHypercase builds the hypercase command into a temp dir, and returns
// its path. It builds in hypercase's own module, whose go.mod and go.sum
// list what the command needs, where this module's list what the toy needs.
//
// The command's source is outside this module, where go test doesn't look
// for what a test read, and the test's binary doesn't import the command or
// the report's packages. So go test can reuse a cached pass after a change
// to the report: run with -count=1 to see one.
func buildHypercase(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "hypercase")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", exe, "github.com/sdboyer/hypercase/cmd/hypercase")
	cmd.Dir = hypercaseRoot(t)
	cmd.Env = goEnv("")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building hypercase: %v\n%s", err, out)
	}
	return exe
}

// hypercaseRoot returns the directory of hypercase's own module, which the
// toy's go.mod replaces hypercase by.
func hypercaseRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// scratch copies the toy into repo, a new git repository on the branch
// main: the module's go.mod, the sheet package, and step11. The module's
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
	for _, dir := range []string{"sheet", "step11"} {
		copyDir(t, filepath.Join(module, dir), filepath.Join(repo, dir))
	}
	copyFile(t, filepath.Join(module, "go.mod"), filepath.Join(repo, "go.mod"))

	mod, err := os.ReadFile(filepath.Join(module, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	mod, err = scratchModfile(mod, hypercaseRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	build := t.TempDir()
	modfile = filepath.Join(build, "go.mod")
	if err := os.WriteFile(modfile, mod, 0o644); err != nil {
		t.Fatal(err)
	}
	copyFile(t, filepath.Join(module, "go.sum"), filepath.Join(build, "go.sum"))

	git(t, repo, gitInit...)
	return modfile
}

// scratchModfile returns the toy's go.mod, mod, with its replace of
// hypercase by a path relative to it rewritten to replace hypercase by root.
// The path is quoted, so that every character of root is part of it.
func scratchModfile(mod []byte, root string) ([]byte, error) {
	const replace = "replace github.com/sdboyer/hypercase => ../..\n"
	if n := bytes.Count(mod, []byte(replace)); n != 1 {
		return nil, fmt.Errorf("go.mod holds %q %d times, and the copy rewrites it once", replace, n)
	}
	abs := "replace github.com/sdboyer/hypercase => " + strconv.Quote(root) + "\n"
	return bytes.Replace(mod, []byte(replace), []byte(abs), 1), nil
}

// TestScratchModfile: the go command reads the scratch go.mod's replace of
// hypercase as the path it was given, a space in it included.
func TestScratchModfile(t *testing.T) {
	mod, err := os.ReadFile(filepath.Join("..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "a dir", "hypercase")
	mod, err = scratchModfile(mod, root)
	if err != nil {
		t.Fatal(err)
	}
	modfile := filepath.Join(t.TempDir(), "go.mod")
	if err := os.WriteFile(modfile, mod, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "mod", "edit", "-json", modfile)
	cmd.Env = goEnv("")
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			t.Fatalf("go mod edit -json: %v\n%s", err, exit.Stderr)
		}
		t.Fatalf("go mod edit -json: %v", err)
	}
	var got struct {
		Replace []struct {
			Old, New struct{ Path, Version string }
		}
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Replace) != 1 || got.Replace[0].Old.Path != "github.com/sdboyer/hypercase" || got.Replace[0].New.Path != root || got.Replace[0].New.Version != "" {
		t.Errorf("the go command reads the replaces as %+v, not hypercase replaced by %q", got.Replace, root)
	}
}

// gitInit makes the scratch repository, on main, with its objects named by
// SHA-1 whatever git's default, so that the page's commits are the same
// wherever the test runs.
var gitInit = []string{"init", "-q", "-b", "main", "--object-format=sha1"}

// TestGitInit: the scratch repository names its objects by SHA-1 where
// git's default is SHA-256.
func TestGitInit(t *testing.T) {
	repo := t.TempDir()
	cmd := exec.Command("git", gitInit...)
	cmd.Dir = repo
	cmd.Env = append(gitEnv(), "GIT_DEFAULT_HASH=sha256")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(gitInit, " "), err, out)
	}
	if got := git(t, repo, "rev-parse", "--show-object-format"); got != "sha1\n" {
		t.Errorf("the repository's object format is %q, not sha1", strings.TrimSpace(got))
	}
}

// copyDir copies the files under from to the same paths under to. In the
// module's update, another package can be writing them, each through a temp
// file that it renames over the file it replaces, so each file is as it was
// before such a write or after it. A listed temp file can be renamed away
// before it is read, and one copied is refused by the scratch update's root
// check, so copyDir skips temp files. It refuses what is neither a
// directory nor a regular file, a symlink among them, which the toy holds
// none of.
func copyDir(t *testing.T, from, to string) {
	t.Helper()
	if err := copyTree(from, to); err != nil {
		t.Fatal(err)
	}
}

// copyTree is copyDir, returning the error it fails on.
func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() || archive.IsTemp(d.Name()):
			return nil
		case !d.Type().IsRegular():
			return fmt.Errorf("%s is not a regular file, and the copy refuses it", p)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, p)
		if err != nil {
			return err
		}
		return writeFile(filepath.Join(to, rel), data)
	})
}

// TestCopyTree: the copy leaves out a temp file of a write and copies the
// rest, and refuses a symlink, whether to a file, to a directory or to
// nothing.
func TestCopyTree(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	tmp := filepath.Join("sheet", ".hypercase-tmp-divide.txtar-123")
	if !archive.IsTemp(tmp) {
		t.Fatalf("%s is not the name of a temp file", tmp)
	}
	for _, name := range []string{"hypercase.json", filepath.Join("sheet", "divide.txtar"), tmp} {
		if err := writeFile(filepath.Join(from, name), []byte(name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := copyTree(from, to); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	err := filepath.WalkDir(to, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(to, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		got[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"hypercase.json":                       "hypercase.json",
		filepath.Join("sheet", "divide.txtar"): filepath.Join("sheet", "divide.txtar"),
	}
	if !maps.Equal(got, want) {
		t.Errorf("copied %v, want %v", got, want)
	}

	for _, target := range []string{"divide.txtar", ".", "gone.txtar"} {
		from := t.TempDir()
		if err := writeFile(filepath.Join(from, "sheet", "divide.txtar"), []byte("divide\n")); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(from, "sheet", "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		err := copyTree(from, t.TempDir())
		if want := link + " is not a regular file"; err == nil || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("copying a symlink to %s: got %v, want an error that starts %q", target, err, want)
		}
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
	":(glob)step11/**/testdata/hypercase.json",
	":(glob)step11/testdata/*/*.txtar",
	":(glob)step11/**/testdata/checks/*",
}

// commit commits what the report reads, as it is in the working tree.
func commit(t *testing.T, repo, msg string) {
	t.Helper()
	git(t, repo, append([]string{"add", "-A", "--"}, read...)...)
	git(t, repo, "commit", "-q", "-m", msg)
}

// TestCommit: a commit holds go.mod, the registries, the archives and the
// corpus check's findings, and none of the rest of the toy: its Go source
// and the report's own goldens.
func TestCommit(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, gitInit...)
	for _, name := range []string{
		"go.mod",
		"sheet/sheet.go",
		"step11/corpus_test.go",
		"step11/cli/testdata/hypercase.json",
		"step11/testdata/hypercase.json",
		"step11/testdata/checks/codes.txt",
		"step11/testdata/report.txt",
		"step11/testdata/sheet/notes.txtar",
	} {
		if err := writeFile(filepath.Join(repo, filepath.FromSlash(name)), []byte(name+"\n")); err != nil {
			t.Fatal(err)
		}
	}
	commit(t, repo, "main")
	got := git(t, repo, "ls-tree", "-r", "--name-only", "HEAD")
	const want = "go.mod\nstep11/cli/testdata/hypercase.json\nstep11/testdata/checks/codes.txt\nstep11/testdata/hypercase.json\nstep11/testdata/sheet/notes.txtar\n"
	if got != want {
		t.Errorf("the commit holds:\n%s\nwant:\n%s", got, want)
	}
}

// runUpdate runs the update of step11's packages in the scratch module,
// then their normal run, which fails unless the update wrote everything.
// One pass is enough: the one code the change moves that the corpus check
// codes counts is past-xfd's, in a cell of import, and import's cells are
// this package's, as codes is, so its update writes the cell before its
// check reads it, and the findings don't depend on which package writes
// first. Both build with -trimpath, which keeps the scratch module's
// directory, new in each run, out of the build cache's keys, so that the
// cache serves them.
func runUpdate(t *testing.T, repo, modfile string) {
	t.Helper()
	gotest := func(env []string) ([]byte, error) {
		cmd := exec.Command("go", "test", "-modfile="+modfile, "-trimpath", "-count=1", "-run=^TestCorpus$", "./step11/...")
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
