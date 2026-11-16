package snapshot

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoTesting: neither the package nor any of its dependencies imports
// testing, so a project's command can import it.
func TestNoTesting(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/sdboyer/hypercase/snapshot").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if dep == "testing" {
			t.Error("the snapshot package imports testing, directly or through a dependency")
		}
	}
}
