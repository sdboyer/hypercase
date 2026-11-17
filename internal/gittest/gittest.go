// Package gittest makes git repositories for tests, each in a temp dir,
// with commits whose hashes depend only on what the test writes. Git runs
// with none of the user's or the system's configuration, so no hook,
// signing key or default branch name of the machine changes what it does.
package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A Repo is a test's repository.
type Repo struct {
	t   testing.TB
	Dir string // the top of its working tree
}

// New makes an empty repository, on the branch main.
func New(t testing.TB) *Repo {
	t.Helper()
	r := &Repo{t: t, Dir: t.TempDir()}
	r.Git("init", "-q", "-b", "main")
	return r
}

// Env is the environment every git command of the test runs with: this
// process's, less every GIT_ variable and the locale, with git's
// configuration files switched off, the author, committer and dates fixed,
// messages in English, and no repository found above the temp dir.
func Env() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") && !strings.HasPrefix(kv, "LC_") && !strings.HasPrefix(kv, "LANG") {
			env = append(env, kv)
		}
	}
	return append(env,
		"LC_ALL=C",
		"GIT_CEILING_DIRECTORIES="+os.TempDir(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	)
}

// Git runs git in the repository, and returns its stdout. A failure fails
// the test.
func (r *Repo) Git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	cmd.Env = Env()
	out, err := cmd.Output()
	if err != nil {
		msg := err.Error()
		if ee, ok := err.(*exec.ExitError); ok {
			msg += ": " + string(ee.Stderr)
		}
		r.t.Fatalf("git %s: %s", strings.Join(args, " "), msg)
	}
	return string(out)
}

// Write writes files in the working tree, by slash path relative to its
// top.
func (r *Repo) Write(files map[string]string) {
	r.t.Helper()
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	for _, p := range paths {
		full := filepath.Join(r.Dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(files[p]), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
}

// Remove removes files from the working tree.
func (r *Repo) Remove(paths ...string) {
	r.t.Helper()
	for _, p := range paths {
		if err := os.RemoveAll(filepath.Join(r.Dir, filepath.FromSlash(p))); err != nil {
			r.t.Fatal(err)
		}
	}
}

// Commit commits everything in the working tree, and returns the commit.
func (r *Repo) Commit(msg string) string {
	r.t.Helper()
	r.Git("add", "-A")
	r.Git("commit", "-q", "--allow-empty", "-m", msg)
	return strings.TrimSpace(r.Git("rev-parse", "HEAD"))
}
