package snapshot

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoTesting: neither the package, nor package root, whose Find finds
// the corpus root, nor any of their dependencies imports testing, so a
// project's command can import them.
func TestNoTesting(t *testing.T) {
	for _, pkg := range []string{"github.com/sdboyer/hypercase/snapshot", "github.com/sdboyer/hypercase/root"} {
		out, err := exec.Command("go", "list", "-deps", pkg).Output()
		if err != nil {
			t.Fatal(err)
		}
		for _, dep := range strings.Fields(string(out)) {
			if dep == "testing" {
				t.Errorf("%s imports testing, directly or through a dependency", pkg)
			}
		}
	}
}
