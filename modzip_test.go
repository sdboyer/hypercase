package hypercase_test

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/module"
	"golang.org/x/mod/zip"
)

// TestModuleZip: every file of the module is one a module zip can hold, so
// a consumer can require the module at any commit. zip.CheckDir applies
// the go command's own rule to the files under the module root, which is
// the root package's directory: each element of a file's path is one
// module.CheckFilePath admits, no two paths differ only in case, and the
// files fit the size limits. A file it refuses fails the module's download,
// so no consumer can require that commit.
func TestModuleZip(t *testing.T) {
	t.Parallel()
	cf, err := zip.CheckDir(".")
	for _, e := range cf.Invalid {
		t.Errorf("%s can't be in a module zip: %v.\nRename it. A transcript is named from its test: rename the test, and the transcript with it.", e.Path, ruleError(e.Err))
	}
	if cf.SizeError != nil {
		t.Errorf("the module's files can't be a module zip: %v.", cf.SizeError)
	}
	if err != nil && len(cf.Invalid) == 0 && cf.SizeError == nil {
		t.Fatalf("checking the module's files: %v", err)
	}
}

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
		for _, kv := range os.Environ() {
			k, _, _ := strings.Cut(kv, "=")
			if k != fixtureEnv && k != transcriptsVar && k != transcriptChildEnv && k != "PWD" {
				cmd.Env = append(cmd.Env, kv)
			}
		}
		cmd.Env = append(cmd.Env, transcriptChildEnv+"=1")
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
