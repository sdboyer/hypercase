package gittest

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNewObjectFormat records a known gap, #50: the git init that New runs
// names the repository's objects by git's default object format. So under
// a git whose default is SHA-256, as GIT_DEFAULT_HASH=sha256 makes it for
// that git init, every commit a test makes has another name, and so does
// every golden that holds one. What is right is SHA-1, whatever git's
// default.
func TestNewObjectFormat(t *testing.T) {
	r := &Repo{t: t, Dir: t.TempDir()}
	cmd := exec.Command("git", initArgs...)
	cmd.Dir = r.Dir
	cmd.Env = append(Env(), "GIT_DEFAULT_HASH=sha256")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(initArgs, " "), err, out)
	}
	if got := strings.TrimSpace(r.Git("rev-parse", "--show-object-format")); got != "sha256" {
		t.Errorf("the repository's object format is %s, where today it is git's default, sha256 (#50). If #50 is fixed, make this test show it is sha1.", got)
	}
}
