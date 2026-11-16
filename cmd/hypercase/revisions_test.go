package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/gittest"
	"github.com/sdboyer/hypercase/internal/registry"
)

// TestObjectStore: the report reads both revisions from the repository's
// object store, never the working tree. A working tree that holds other
// archives and registries, or none, changes nothing.
func TestObjectStore(t *testing.T) {
	t.Parallel()
	g, a, b := sheets(t)
	want := ok(t, g, "step09", "report", "-text", a, b)
	g.Write(map[string]string{
		sheetDir + "loop.txtar":              "Rewritten.\n-- out/calc/outcome --\npanicked\n",
		sheetDir + "new.txtar":               "Uncommitted.\n-- out/calc/outcome --\naccepted\n",
		"step09/testdata/hypercase.json":     `{"format": 99}`,
		"step09/cli/testdata/hypercase.json": "not json",
	})
	g.Remove(sheetDir+"div.txtar", "step09/format")
	if got := ok(t, g, "step09", "report", "-text", a, b); got != want {
		t.Errorf("with the working tree changed, the report is:\n%s\nwant:\n%s", got, want)
	}
	// HEAD is b, whatever the working tree holds.
	if got := ok(t, g, "step09", "report", "-text", a, "HEAD"); got != want {
		t.Errorf("to HEAD, the report is:\n%s\nwant:\n%s", got, want)
	}
}

// TestRoot: -root names the corpus root relative to the current directory.
// Without it, the corpus root is found as a test finds it: the one that
// the registry of the package in the current directory records, or else
// the module root.
func TestRoot(t *testing.T) {
	t.Parallel()
	g, a, b := sheets(t)
	want := ok(t, g, "step09", "report", "-text", a, b)
	for _, tt := range []struct{ dir, root string }{
		{"", "step09"},
		{"step09", ""},
		{"step09", "."},
		{"step09/cli", ""},
		{"step09/format", ".."},
	} {
		args := []string{"report", "-text"}
		if tt.root != "" {
			args = append(args, "-root", tt.root)
		}
		if got := ok(t, g, tt.dir, append(args, a, b)...); got != want {
			t.Errorf("in %q with -root %q, the report is:\n%s\nwant:\n%s", tt.dir, tt.root, got, want)
		}
	}
	// The module root holds no registry over itself.
	if err := os.MkdirAll(filepath.Join(g.Dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"", "docs"} {
		fails(t, g, dir, 1, []string{"report", a, b}, "no registry records the corpus root ., at "+a+" or at "+b+".")
	}
	fails(t, g, "step09", 1, []string{"report", "-root", "cli", a, b}, "no registry records the corpus root step09/cli,")
	fails(t, g, "step09", 1, []string{"report", "-root", g.Dir, a, b}, "is an absolute path. Name the corpus root relative to the current directory")
	fails(t, g, "step09", 1, []string{"report", "-root", "../..", a, b}, "-root ../.. is outside the repository")
}

// TestNestedModule: a module below the top of the repository has its own
// module root, which its registries' corpus roots are relative to.
func TestNestedModule(t *testing.T) {
	t.Parallel()
	g := gittest.New(t)
	arch := map[string]string{"x": "A case.\n-- out/calc/outcome --\naccepted\n"}
	nest := func(files map[string]string) map[string]string {
		out := map[string]string{"go.mod": "module example.com/top\n\ngo 1.27\n"}
		for p, data := range files {
			out["tools/"+p] = data
		}
		return out
	}
	c := revisions(t, g, nest(module(nil)), nest(module(arch)))
	want := "appeared\n  sheet/x calc: accepted\n"
	for _, tt := range []struct{ dir, root string }{{"tools", ""}, {"", "tools"}, {"tools/testdata", ".."}} {
		args := []string{"report", "-text"}
		if tt.root != "" {
			args = append(args, "-root", tt.root)
		}
		if got := ok(t, g, tt.dir, append(args, c[0], c[1])...); got != want {
			t.Errorf("in %q with -root %q, the report is:\n%s\nwant:\n%s", tt.dir, tt.root, got, want)
		}
	}
	// From the top, the module root is the top's, and its corpus has no
	// registry.
	fails(t, g, "", 1, []string{"report", c[0], c[1]}, "no registry records the corpus root .,")
}

// TestFormats: the report reads its own registry format, and refuses
// another, older or newer, saying to run the report from that revision's
// hypercase. A registry of another corpus root is left unread, whatever
// its format.
func TestFormats(t *testing.T) {
	t.Parallel()
	arch := map[string]string{"x": "A case about sheets#4.\n-- out/calc/outcome --\naccepted\n"}
	v1 := `{"format": 1, "updateVar": "M_UPDATE_GOLDEN", "corpusRoot": ".", "rootPackage": true, "properties": [],` +
		` "suites": [{"name": "calc", "description": "Runs calc.", "corpus": "sheet", "outputs": [], "rejects": []}]}` + "\n"
	withRegistry := func(files map[string]string, data string) map[string]string {
		files["testdata/hypercase.json"] = data
		return files
	}
	later := registry.Format + 1

	sibling := func(files map[string]string, data string) map[string]string {
		files["sib/testdata/hypercase.json"] = data
		return files
	}
	g := gittest.New(t)
	c := revisions(t, g,
		sibling(module(nil), strings.Replace(v1, `"corpusRoot": "."`, `"corpusRoot": "sib"`, 1)),
		sibling(module(arch), fmt.Sprintf(`{"format": %d, "corpusRoot": "sib", "a field of a later hypercase": {}}`+"\n", later)))
	if got, want := ok(t, g, "", "report", "-text", c[0], c[1]), "appeared\n  sheet/x calc: accepted\n"; got != want {
		t.Errorf("beside another corpus root's registries, the report is:\n%s\nwant:\n%s", got, want)
	}

	for _, other := range []struct {
		format int
		data   string
	}{
		{1, v1},
		{later, fmt.Sprintf(`{"format": %d, "issueLinks": "a field this hypercase doesn't know"}`+"\n", later)},
		{later, fmt.Sprintf(`{"format": %d, "corpusRoot": ".", "issueLinks": "a field this hypercase doesn't know"}`+"\n", later)},
	} {
		g = gittest.New(t)
		c = revisions(t, g, module(nil), withRegistry(module(arch), other.data))
		want := []string{
			fmt.Sprintf("hypercase report: testdata/hypercase.json at %s holds registry format %d, and this hypercase reads format %d.\n", c[1], other.format, registry.Format) +
				"Run the report from " + c[1] + "'s hypercase, which reads it: in a checkout of " + c[1] + ", run the same command as go run github.com/sdboyer/hypercase/cmd/hypercase.\n",
		}
		fails(t, g, "", 1, []string{"report", c[0], c[1]}, want...)
		fails(t, g, "", 1, []string{"report", "-text", c[1], c[0]}, want...)
	}
	// A registry that is no registry fails by name.
	g = gittest.New(t)
	c = revisions(t, g, module(nil), withRegistry(module(arch), "not json\n"))
	fails(t, g, "", 1, []string{"report", c[0], c[1]}, "testdata/hypercase.json at "+c[1]+" can't be read: it is not a registry")
}
