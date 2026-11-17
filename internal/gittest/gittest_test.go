package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestNewObjectFormat: a repository New makes is SHA-1 under a git whose
// default is SHA-256, so no golden that holds a commit's name depends on
// git's default (#50). The git on PATH is a wrapper that makes SHA-256 its
// default, as GIT_DEFAULT_HASH does, since Env drops every GIT_ variable a
// caller sets. It sets PATH, so it can't run in parallel.
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

	// The wrapper's git makes a repository SHA-256 where nothing says
	// otherwise, or the test below shows nothing.
	plain := &Repo{t: t, Dir: t.TempDir()}
	plain.Git("init", "-q")
	if got := strings.TrimSpace(plain.Git("rev-parse", "--show-object-format")); got != "sha256" {
		t.Fatalf("under the wrapper, git init makes a %s repository, want sha256", got)
	}

	r := New(t)
	if got := strings.TrimSpace(r.Git("rev-parse", "--show-object-format")); got != "sha1" {
		t.Errorf("New made a %s repository, want sha1", got)
	}
}
