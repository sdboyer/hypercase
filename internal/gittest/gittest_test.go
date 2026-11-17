package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestNewObjectFormat records a known gap, #50: the repository New makes
// names its objects by git's default object format. So under a git whose
// default is SHA-256, as the git first on PATH here is, every commit a test
// makes has another name, and so does every golden that holds one. What is
// right is SHA-1, whatever git's default. The git on PATH is a wrapper that
// defaults GIT_DEFAULT_HASH to sha256 and keeps one that is set, so a fix
// in New's git init or in Env fails the test. It sets PATH, so it can't run
// in parallel.
func TestNewObjectFormat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the wrapper is a shell script")
	}
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	body := "#!/bin/sh\n: \"${GIT_DEFAULT_HASH:=sha256}\"\nexport GIT_DEFAULT_HASH\nexec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// A plain git init under the wrapper, with no GIT_ variable set, makes a
	// SHA-256 repository, so the test can't pass with a wrapper that
	// doesn't.
	plain := t.TempDir()
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"rev-parse", "--show-object-format"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = plain, env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		if args[0] == "rev-parse" && strings.TrimSpace(string(out)) != "sha256" {
			t.Fatalf("a plain git init under the wrapper makes a %s repository, not sha256", strings.TrimSpace(string(out)))
		}
	}

	r := New(t)
	if got := strings.TrimSpace(r.Git("rev-parse", "--show-object-format")); got != "sha256" {
		t.Errorf("the repository's object format is %s, where today it is git's default, sha256 (#50). If #50 is fixed, make this test show it is sha1.", got)
	}
}
