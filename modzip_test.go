package hypercase_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sdboyer/hypercase/internal/gittest"
	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

// TestModuleZip: the module's commit at HEAD is one a download can zip, so
// a consumer can require it. The go command's download zips what git
// archive gives of the commit, in a repository whose attributes switch
// export-ignore and export-subst off: every file of its tree, symbolic
// links among them, and for a submodule a directory entry, which it drops.
// modzip.Create refuses the files as modzip.CheckFiles does: a path with
// an element module.CheckFilePath refuses, a symbolic link's among them,
// two paths that differ only in case, or files past the size limits.
// moduleZipProblems lists HEAD's tree with git ls-tree, which no attribute
// changes, and checks its files with modzip.CheckFiles. An untracked file
// is no part of the check, as it is none of a download. The test skips
// where there is no commit to check: where the module root is not the top
// of a git checkout, as in a module cache, whose module a download zipped
// by the rule, and where the checkout has no commit.
//
// go test replays a passing result while the files the test read under
// the module root are unchanged, so headFiles reads the files that name
// HEAD's commit. In a linked worktree those are outside its root, in the
// main checkout's .git, so there go test can replay a pass after a new
// commit: -count=1 gives the verdict. CI's fresh checkout always runs the
// check.
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
// link out, a path that differs from another only in case, and a file
// export-ignore marks, which a download zips all the same. It leaves out
// an untracked file, a nested module's and a submodule, as a download
// does. It gives a transcript's advice only for a transcript, and quotes
// each path. It finds the top of a checkout by a path in another case, on
// a volume that folds case. It skips where the directory is not the top of
// a git checkout, and where the checkout has no commit. And in a plain
// clone, a new commit changes a file headRefs names.
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
	}, {
		name: "a file whose name the rule refuses, which the committed attributes export-ignore",
		files: []indexEntry{
			{modeFile, "go.mod", goMod},
			{modeFile, ".gitattributes", "*'* export-ignore\n"},
			{modeFile, "it's.txt", ""},
		},
		want: []string{"\"it's.txt\" can't be in a module zip: invalid char '\\''.\nFix or rename it."},
	}, {
		name: "a file whose name the rule refuses, which the attributes of the repository export-ignore",
		files: []indexEntry{
			{modeFile, "go.mod", goMod},
			{modeFile, "it's.txt", ""},
		},
		after: map[string]string{".git/info/attributes": "* export-ignore\n"},
		want:  []string{"\"it's.txt\" can't be in a module zip: invalid char '\\''.\nFix or rename it."},
	}, {
		name: "a submodule whose name the rule refuses",
		files: []indexEntry{
			{modeFile, "go.mod", goMod},
			{modeGitlink, "the_user's_sheets", "0123456789012345678901234567890123456789"},
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
		if _, skip := moduleZipProblems(t, filepath.Join(r.Dir, "inner"), gittest.Env()); !strings.Contains(skip, " is inner/ in its git checkout, not its top, ") {
			t.Errorf("the skip is %q", skip)
		}
	})
	t.Run("a checkout with no commit", func(t *testing.T) {
		t.Parallel()
		r := gittest.New(t)
		if _, skip := moduleZipProblems(t, r.Dir, gittest.Env()); skip != "The checkout has no commit, so there is nothing to download." {
			t.Errorf("the skip is %q", skip)
		}
	})
	// A volume that folds case finds a path in any case, which keeps the
	// case it is given, and git gives the checkout's top in the case the
	// volume holds.
	t.Run("the top of a checkout, by a path in another case", func(t *testing.T) {
		t.Parallel()
		r := gittest.New(t)
		r.Write(map[string]string{"go.mod": goMod})
		r.Commit("one")
		parent := filepath.Dir(r.Dir)
		upper := filepath.Join(filepath.Dir(parent), strings.ToUpper(filepath.Base(parent)), filepath.Base(r.Dir))
		if upper == r.Dir {
			t.Fatalf("%s holds no letter to upper-case", parent)
		}
		if _, err := os.Stat(upper); err != nil {
			t.Skipf("the volume holds the checkout in one case only: %v", err)
		}
		if got, skip := moduleZipProblems(t, upper, gittest.Env()); skip != "" || got != nil {
			t.Errorf("problems %q, and skip %q, and want neither", got, skip)
		}
	})
	// go test replays a pass while the files the test read under the
	// module root are unchanged. In a plain clone, whose .git is under its
	// top, those include what headRefs names, so a new commit must rewrite
	// one of them, whichever ref store the clone keeps.
	for _, format := range []string{"files", "reftable"} {
		t.Run("a new commit rewrites a file of a plain clone that names HEAD, in "+format, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			git := func(args ...string) {
				t.Helper()
				if _, stderr, err := gitIn(dir, gittest.Env(), args...); err != nil {
					t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr)
				}
			}
			initArgs := []string{"init", "-q", "-b", "main", "--object-format=sha1"}
			if format != "files" {
				initArgs = append(initArgs, "--ref-format="+format)
			}
			if _, stderr, err := gitIn(dir, gittest.Env(), initArgs...); err != nil {
				if format != "files" && strings.Contains(stderr, "unknown option") {
					t.Skipf("this git has no --ref-format: %s", stderr)
				}
				t.Fatalf("git %s: %v\n%s", strings.Join(initArgs, " "), err, stderr)
			}
			named := func() string {
				var b strings.Builder
				for _, p := range headRefs(t, dir, gittest.Env()) {
					data, _ := os.ReadFile(p)
					fmt.Fprintf(&b, "%s\n%s\n", p, data)
				}
				return b.String()
			}
			commit := func(name, data string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
				git("add", name)
				git("commit", "-q", "-m", name)
			}
			commit("go.mod", goMod)
			one := named()
			commit("cell.txt", "A1\n")
			if two := named(); two == one {
				t.Errorf("a commit changed none of the files that name HEAD:\n%s", one)
			}
		})
	}
}

// An indexEntry is a file a test commits through the index, by its mode,
// slash path and contents: a symbolic link's contents are its target, and
// a submodule's are the commit it names.
type indexEntry struct{ mode, path, data string }

const (
	modeFile    = "100644"
	modeLink    = "120000"
	modeGitlink = "160000"
)

func (f indexEntry) add(r *gittest.Repo) {
	sha := f.data
	if f.mode != modeGitlink {
		r.Write(map[string]string{".blob": f.data})
		sha = strings.TrimSpace(r.Git("hash-object", "-w", ".blob"))
		r.Remove(".blob")
	}
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
// command's download zips them: every file of its tree, by git ls-tree,
// less submodules. If there is no commit to check, it returns none, and
// says why.
func headFiles(t *testing.T, dir string, env []string) ([]modzip.File, string) {
	t.Helper()
	const downloaded = "A module that was downloaded, as into a module cache, was zipped by the rule."
	prefix, stderr, err := gitIn(dir, env, "rev-parse", "--show-prefix")
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return nil, fmt.Sprintf("git can't be run, so the commit at HEAD can't be read: %v.", err)
	case err != nil && strings.Contains(stderr, "not a git repository"):
		return nil, fmt.Sprintf("%s is not in a git checkout, so there is no commit to check. %s", dir, downloaded)
	case err != nil:
		t.Fatalf("git rev-parse --show-prefix: %v\n%s", err, stderr)
	}
	if p := strings.TrimSpace(string(prefix)); p != "" {
		return nil, fmt.Sprintf("%s is %s in its git checkout, not its top, so there is no commit of its own to check. %s", dir, p, downloaded)
	}
	if _, stderr, err := gitIn(dir, env, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return nil, "The checkout has no commit, so there is nothing to download."
		}
		t.Fatalf("git rev-parse --verify -q HEAD: %v\n%s", err, stderr)
	}
	// go test replays a passing result while the files the test read under
	// the module root are unchanged, and what git reads is not among them.
	// Reading the files that name HEAD's commit makes a new commit a new
	// input, where they are under the root.
	for _, p := range headRefs(t, dir, env) {
		_, _ = os.ReadFile(p)
	}
	out, stderr, err := gitIn(dir, env, "ls-tree", "-r", "-l", "-z", "--full-tree", "HEAD")
	if err != nil {
		t.Fatalf("git ls-tree HEAD: %v\n%s", err, stderr)
	}
	var files []modzip.File
	for _, entry := range strings.Split(string(out), "\x00") {
		if entry == "" {
			continue
		}
		meta, name, ok := strings.Cut(entry, "\t")
		f := strings.Fields(meta)
		if !ok || len(f) != 4 {
			t.Fatalf("git ls-tree gave %q", entry)
		}
		mode, err := strconv.ParseUint(f[0], 8, 32)
		if err != nil {
			t.Fatalf("git ls-tree gave %q: %v", entry, err)
		}
		var fm fs.FileMode
		switch mode & 0o170000 {
		case 0o100000:
			fm = fs.FileMode(mode & 0o777)
		case 0o120000:
			fm = fs.ModeSymlink | 0o777
		case 0o160000:
			// A submodule: git archive gives a directory entry for it,
			// which codeRepo.Zip drops.
			continue
		default:
			t.Fatalf("git ls-tree gave %q, of a mode no file has", entry)
		}
		// The size is the blob's. A download's git archive converts line
		// endings where the attributes say to, so its size for such a file
		// can differ by its line endings, which only the size limits read.
		size, err := strconv.ParseInt(f[3], 10, 64)
		if err != nil {
			t.Fatalf("git ls-tree gave %q: %v", entry, err)
		}
		files = append(files, treeFile{name: name, object: f[2], info: treeInfo{path.Base(name), size, fm}, dir: dir, env: env})
	}
	return files, ""
}

// headRefs returns the files that name the commit at HEAD in dir, whether
// each exists or not: HEAD, the branch it names, if it names one, the
// packed refs, and the reftable's list of tables. A new commit rewrites
// one of them.
func headRefs(t *testing.T, dir string, env []string) []string {
	t.Helper()
	out, stderr, err := gitIn(dir, env, "rev-parse", "--symbolic-full-name", "HEAD")
	if err != nil {
		t.Fatalf("git rev-parse --symbolic-full-name HEAD: %v\n%s", err, stderr)
	}
	args := []string{"rev-parse", "--git-path", "HEAD", "--git-path", "packed-refs", "--git-path", "reftable/tables.list"}
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

// A treeFile is a file of a commit's tree, as git ls-tree lists it: its
// path, its blob, and what a download's zip entry for it says.
type treeFile struct {
	name, object string
	info         treeInfo
	dir          string
	env          []string
}

func (f treeFile) Path() string                { return f.name }
func (f treeFile) Lstat() (fs.FileInfo, error) { return f.info, nil }

func (f treeFile) Open() (io.ReadCloser, error) {
	out, stderr, err := gitIn(f.dir, f.env, "cat-file", "blob", f.object)
	if err != nil {
		return nil, fmt.Errorf("git cat-file blob %s: %v\n%s", f.object, err, stderr)
	}
	return io.NopCloser(bytes.NewReader(out)), nil
}

// A treeInfo is a treeFile's fs.FileInfo.
type treeInfo struct {
	name string
	size int64
	mode fs.FileMode
}

func (i treeInfo) Name() string       { return i.name }
func (i treeInfo) Size() int64        { return i.size }
func (i treeInfo) Mode() fs.FileMode  { return i.mode }
func (i treeInfo) ModTime() time.Time { return time.Time{} }
func (i treeInfo) IsDir() bool        { return false }
func (i treeInfo) Sys() any           { return nil }

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
