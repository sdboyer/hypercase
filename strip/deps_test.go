package strip

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// deps lists a package's dependencies, as go list -deps does.
func deps(t *testing.T, pkg string) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", pkg).Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	return strings.Fields(string(out))
}

// TestNoTesting: neither strip nor the command built on it imports
// testing, so a project's own command can call strip as hypercase strip
// does.
func TestNoTesting(t *testing.T) {
	t.Parallel()
	for _, pkg := range []string{"github.com/sdboyer/hypercase/strip", "github.com/sdboyer/hypercase/cmd/hypercase"} {
		if slices.Contains(deps(t, pkg), "testing") {
			t.Errorf("%s imports testing", pkg)
		}
	}
}

// TestNoUpdateStrips: the runner imports neither strip nor the command,
// so no update, and no project's test, runs strip.
func TestNoUpdateStrips(t *testing.T) {
	t.Parallel()
	got := deps(t, "github.com/sdboyer/hypercase")
	if !slices.Contains(got, "github.com/sdboyer/hypercase/archive") {
		t.Fatalf("go list -deps of the runner lists no archive package, so it can't show what the runner imports:\n%s", strings.Join(got, "\n"))
	}
	for _, pkg := range []string{"github.com/sdboyer/hypercase/strip", "github.com/sdboyer/hypercase/cmd/hypercase"} {
		if slices.Contains(got, pkg) {
			t.Errorf("the runner imports %s", pkg)
		}
	}
}
