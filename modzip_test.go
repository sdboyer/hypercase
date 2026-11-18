package hypercase_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/gittest"
	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

// TestModuleZip: the module's commit at HEAD is one a download can zip, so
// a consumer can require it. The go command's download reads a commit's
// files through git archive, and modzip.Create refuses them as
// modzip.CheckFiles does: a path with an element module.CheckFilePath
// refuses, a symbolic link's among them, two paths that differ only in
// case, or files past the size limits. moduleZipProblems reads HEAD as the
// download does, and checks its files with modzip.CheckFiles, so an
// untracked file is no more part of the check than of a download. Where
// the module root is not the top of a git checkout, as in a module cache,
// there is no commit to check, and a module that was downloaded was zipped
// by the rule: the test skips.
func TestModuleZip(t *testing.T) {
	t.Parallel()
	problems, skip := moduleZipProblems(t, ".", os.Environ())
	if skip != "" {
		t.Skip(skip)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestModuleZipCases: moduleZipProblems refuses what a download of the
// commit would, in repositories gittest makes, whose commits are built in
// the index, so that no volume's case folding changes what they hold. It
// refuses a symbolic link, which a download checks before it leaves the
// link out, and a path that differs from another only in case. It
// leaves out an untracked file and a nested module's, as a download does.
// It gives a transcript's advice only for a transcript, and quotes each
// path. It skips where the directory is not the top of a git checkout. And
// a new commit changes a file headRefs names, which go test's cache of a
// result then sees.
func TestModuleZipCases(t *testing.T) {
	t.Parallel()
	const goMod = "module example.com/sheets\n"
	for _, c := range []struct {
		name  string
		files []indexEntry
		after map[string]string // written to the working tree after the commit
		want  []string
	}{{
		name: "a symbolic link whose name the rule refuses",
		files: []indexEntry{
			{modeFile, "go.mod", goMod},
			{modeFile, "cell/cell.go", "package cell\n"},
			{modeLink, "cell/the_user's_link", "cell.go"},
		},
		want: []string{"\"cell/the_user's_link\" can't be in a module zip: invalid char '\\''.\nFix or rename it."},
	}, {
		name: "a symbolic link that differs from a file only in case",
		files: []indexEntry{
			{modeFile, "go.mod", goMod},
			{modeFile, "cell/cell.go", "package cell\n"},
			{modeLink, "cell/Cell.go", "cell.go"},
		},
		want: []string{"\"cell/cell.go\" can't be in a module zip: case-insensitive file name collision: \"cell/Cell.go\" and \"cell/cell.go\".\nFix or rename it."},
	}, {
		name: "two transcripts that differ only in case",
		files: []indexEntry{
			{modeFile, "go.mod", goMod},
			{modeFile, "testdata/transcripts/TestSheet-Cell.txt", "$ go test\n"},
			{modeFile, "testdata/transcripts/TestSheet-cell.txt", "$ go test\n"},
		},
		want: []string{"\"testdata/transcripts/TestSheet-cell.txt\" can't be in a module zip: case-insensitive file name collision: \"testdata/transcripts/TestSheet-Cell.txt\" and \"testdata/transcripts/TestSheet-cell.txt\".\nA transcript is named from its test: rename the subtest, and the transcript with it."},
	}, {
		name: "a transcript whose name the rule refuses",
		files: []indexEntry{
			{modeFile, "go.mod", goMod},
			{modeFile, "testdata/transcripts/TestSheet-the_user's_sheet.txt", "$ go test\n"},
		},
		want: []string{"\"testdata/transcripts/TestSheet-the_user's_sheet.txt\" can't be in a module zip: invalid char '\\''.\nA transcript is named from its test: rename the subtest, and the transcript with it."},
	}, {
		name: "a name that holds a carriage return",
		files: []indexEntry{
			{modeFile, "go.mod", goMod},
			{modeFile, "Icon\r", ""},
		},
		want: []string{"\"Icon\\r\" can't be in a module zip: invalid char '\\r'.\nFix or rename it."},
	}, {
		name:  "an untracked file whose name the rule refuses",
		files: []indexEntry{{modeFile, "go.mod", goMod}},
		after: map[string]string{"it's.txt": "", "testdata/transcripts/TestSheet-it's.txt": ""},
	}, {
		name: "a file of a nested module whose name the rule refuses",
		files: []indexEntry{
			{modeFile, "go.mod", goMod},
			{modeFile, "inner/go.mod", "module example.com/sheets/inner\n"},
			{modeFile, "inner/it's.txt", ""},
		},
	}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := gittest.New(t)
			for _, f := range c.files {
				f.add(r)
			}
			r.Git("commit", "-q", "-m", c.name)
			r.Write(c.after)
			got, skip := moduleZipProblems(t, r.Dir, gittest.Env())
			if skip != "" {
				t.Fatalf("skipped: %s", skip)
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("problems:\n%q\nwant:\n%q", got, c.want)
			}
		})
	}

	t.Run("no git checkout", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if _, skip := moduleZipProblems(t, dir, gittest.Env()); !strings.Contains(skip, " is not in a git checkout, so there is no commit to check.") {
			t.Errorf("the skip is %q", skip)
		}
	})
	t.Run("a directory below the top of a git checkout", func(t *testing.T) {
		t.Parallel()
		r := gittest.New(t)
		r.Write(map[string]string{"inner/go.mod": "module example.com/inner\n"})
		r.Commit("inner")
		if _, skip := moduleZipProblems(t, filepath.Join(r.Dir, "inner"), gittest.Env()); !strings.Contains(skip, " is not the top of its git checkout, ") {
			t.Errorf("the skip is %q", skip)
		}
	})
	// go test replays a pass while the files the test read are unchanged,
	// and headFiles reads those headRefs names, so a new commit must
	// change one of them.
	t.Run("a new commit rewrites a file that names HEAD", func(t *testing.T) {
		t.Parallel()
		r := gittest.New(t)
		named := func() string {
			var b strings.Builder
			for _, p := range headRefs(t, r.Dir, gittest.Env()) {
				data, _ := os.ReadFile(p)
				fmt.Fprintf(&b, "%s\n%s\n", p, data)
			}
			return b.String()
		}
		r.Write(map[string]string{"go.mod": goMod})
		r.Commit("one")
		one := named()
		r.Write(map[string]string{"cell.txt": "A1\n"})
		r.Commit("two")
		if two := named(); two == one {
			t.Errorf("a commit changed none of the files that name HEAD:\n%s", one)
		}
	})
}

// An indexEntry is a file a test commits through the index, by its mode,
// slash path and contents: a symbolic link's contents are its target.
type indexEntry struct{ mode, path, data string }

const (
	modeFile = "100644"
	modeLink = "120000"
)

func (f indexEntry) add(r *gittest.Repo) {
	r.Write(map[string]string{".blob": f.data})
	sha := strings.TrimSpace(r.Git("hash-object", "-w", ".blob"))
	r.Remove(".blob")
	r.Git("update-index", "--add", "--cacheinfo", f.mode+","+sha+","+f.path)
}

// moduleZipProblems returns what keeps the commit at HEAD in dir from
// being zipped by a download, each as a message, or why there is no commit
// to check. git runs with env.
func moduleZipProblems(t *testing.T, dir string, env []string) (problems []string, skip string) {
	t.Helper()
	files, skip := headFiles(t, dir, env)
	if skip != "" {
		return nil, skip
	}
	cf, err := modzip.CheckFiles(files)
	for _, e := range cf.Invalid {
		advice := "Fix or rename it."
		if strings.HasPrefix(e.Path, "testdata/transcripts/") {
			advice = "A transcript is named from its test: rename the subtest, and the transcript with it."
		}
		problems = append(problems, fmt.Sprintf("%q can't be in a module zip: %v.\n%s", e.Path, ruleError(e.Err), advice))
	}
	if cf.SizeError != nil {
		problems = append(problems, fmt.Sprintf("HEAD's files can't be a module zip: %v.\nRemove files, or make them smaller.", cf.SizeError))
	}
	if err != nil && len(problems) == 0 {
		problems = append(problems, fmt.Sprintf("HEAD's files can't be a module zip: %v.", err))
	}
	return problems, ""
}

// headFiles returns the files of the commit at HEAD in dir as the go
// command's download reads them: git archive's zip entries, less its
// directories. If dir is not the top of a git checkout, it returns none,
// and says why.
func headFiles(t *testing.T, dir string, env []string) ([]modzip.File, string) {
	t.Helper()
	const none = "A module that was downloaded, as into a module cache, was zipped by the rule."
	top, stderr, err := gitIn(dir, env, "rev-parse", "--show-toplevel")
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return nil, fmt.Sprintf("git can't be run, so there is no commit to check: %v. %s", err, none)
	case err != nil && strings.Contains(stderr, "not a git repository"):
		return nil, fmt.Sprintf("%s is not in a git checkout, so there is no commit to check. %s", dir, none)
	case err != nil:
		t.Fatalf("git rev-parse --show-toplevel: %v\n%s", err, stderr)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatal(err)
	}
	got, err := filepath.EvalSymlinks(strings.TrimSpace(string(top)))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		return nil, fmt.Sprintf("%s is not the top of its git checkout, %s, so there is no commit of its own to check. %s", dir, got, none)
	}
	// go test replays a passing result while the files the test read are
	// unchanged, and what git reads is not among them. Reading the files
	// that name HEAD's commit makes a new commit a new input.
	for _, p := range headRefs(t, dir, env) {
		_, _ = os.ReadFile(p)
	}
	out, stderr, err := gitIn(dir, env, "-c", "core.autocrlf=input", "-c", "core.eol=lf", "archive", "--format=zip", "HEAD")
	if err != nil {
		t.Fatalf("git archive HEAD: %v\n%s", err, stderr)
	}
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatal(err)
	}
	var files []modzip.File
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, "/") {
			files = append(files, archived{f})
		}
	}
	return files, ""
}

// headRefs returns the files that name the commit at HEAD in dir, whether
// each exists or not: HEAD, the branch it names, if it names one, and the
// packed refs. A new commit rewrites one of them.
func headRefs(t *testing.T, dir string, env []string) []string {
	t.Helper()
	out, stderr, err := gitIn(dir, env, "rev-parse", "--symbolic-full-name", "HEAD")
	if err != nil {
		t.Fatalf("git rev-parse --symbolic-full-name HEAD: %v\n%s", err, stderr)
	}
	args := []string{"rev-parse", "--git-path", "HEAD", "--git-path", "packed-refs"}
	if ref := strings.TrimSpace(string(out)); ref != "HEAD" {
		args = append(args, "--git-path", ref)
	}
	out, stderr, err = gitIn(dir, env, args...)
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr)
	}
	var paths []string
	for _, p := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		paths = append(paths, p)
	}
	return paths
}

// gitIn runs git in dir, with env and messages in English, and returns its
// stdout and stderr.
func gitIn(dir string, env []string, args ...string) ([]byte, string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(slices.Clone(env), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	return out, stderr.String(), err
}

// archived is a file of git archive's zip, as the download reads it.
type archived struct{ f *zip.File }

func (a archived) Path() string                 { return a.f.Name }
func (a archived) Lstat() (fs.FileInfo, error)  { return a.f.FileInfo(), nil }
func (a archived) Open() (io.ReadCloser, error) { return a.f.Open() }

// ruleError is what of a path the module-zip rule refuses: the reason
// module.CheckFilePath gives, without the path it repeats.
func ruleError(err error) error {
	var p *module.InvalidPathError
	if errors.As(err, &p) {
		return p.Err
	}
	return err
}

// transcriptChildEnv makes TestTranscriptRefused the child it runs.
const transcriptChildEnv = "HYPERCASE_SELFTEST_TRANSCRIPT"

// TestTranscriptRefused: a self-test whose transcript a module zip can't
// hold, as its name gives it, fails at its transcript, saying to rename
// it, and reads and writes none, in a normal run and in an update. It runs
// this package's test binary as a child, in a temp dir, whose one test is
// such a self-test.
func TestTranscriptRefused(t *testing.T) {
	if os.Getenv(transcriptChildEnv) == "1" {
		t.Run("a subtest's name", func(t *testing.T) {
			newSandbox(t, "").done()
		})
		return
	}
	t.Parallel()
	const want = "testdata/transcripts/TestTranscriptRefused-a_subtest's_name.txt can't be in a module zip: invalid char '\\''.\nA transcript is named from its test: rename the subtest, TestTranscriptRefused/a_subtest's_name."
	for _, update := range []bool{false, true} {
		dir := t.TempDir()
		cmd := exec.Command(os.Args[0], "-test.run=^TestTranscriptRefused$", "-test.count=1")
		cmd.Dir = dir
		cmd.Env = append(childEnv(), transcriptChildEnv+"=1")
		if update {
			cmd.Env = append(cmd.Env, transcriptsVar+"=1")
		}
		out, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			t.Errorf("update %v: the child's test should fail, and it ended with %v:\n%s", update, err, out)
		}
		if !strings.Contains(flat(string(out)), flat(want)) {
			t.Errorf("update %v: the child's output doesn't hold %q:\n%s", update, want, out)
		}
		if _, err := os.Stat(filepath.Join(dir, "testdata")); err == nil {
			t.Errorf("update %v: the child wrote testdata/, and should write nothing.", update)
		} else if !errors.Is(err, fs.ErrNotExist) {
			t.Error(err)
		}
	}
}
