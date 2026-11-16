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

// TestFormats: the report reads every registry format from the oldest that
// hypercase's main branch wrote to its own, and refuses another, older or
// newer, saying to run the report from that revision's hypercase. It reads
// both revisions' registries before anything else, so an archive it can't
// read never hides the refusal. A registry of another corpus root is left
// unread, whatever its format.
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

	// An archive the report can't read, which it never reaches.
	broken := map[string]string{"x": "A case.\n-- out/calc/outcome --\nfine\n"}
	for _, other := range []struct {
		format int
		data   string
	}{
		{1, v1},
		{2, strings.Replace(v1, `"format": 1`, `"format": 2`, 1)},
		{later, fmt.Sprintf(`{"format": %d, "issueLinks": "a field this hypercase doesn't know"}`+"\n", later)},
		{later, fmt.Sprintf(`{"format": %d, "corpusRoot": ".", "issueLinks": "a field this hypercase doesn't know"}`+"\n", later)},
	} {
		g = gittest.New(t)
		c = revisions(t, g, module(broken), withRegistry(module(arch), other.data))
		want := []string{
			fmt.Sprintf("hypercase report: testdata/hypercase.json at %s holds registry format %d, and this hypercase reads formats %d to %d.\n", c[1], other.format, registry.Oldest, registry.Format) +
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

// format4 is a registry of format 4, the format before this hypercase's,
// as hypercase's main branch wrote it: no codes and no classifiers.
const format4 = `{
  "format": 4,
  "updateVar": "M_UPDATE_GOLDEN",
  "corpusRoot": ".",
  "rootPackage": true,
  "issueLinks": [{"pattern": "sheets#(\\d+)", "url": "https://example.com/sheets/issues/$1"}],
  "properties": [
    {"name": "circular", "description": "A loop, as format 4 says."},
    {"name": "retired", "description": "A property only format 4 lists."}
  ],
  "types": [{"dir": "sheet", "description": "A sheet, as format 4 says.", "goType": "example.com/m.Sheet", "transforms": [
    {"name": "looped", "description": "Loops, as format 4 says.", "gives": ["circular"], "keeps": ["retired"], "probe": false, "owned": true}
  ]}],
  "lifts": [],
  "suites": [
    {"name": "calc", "description": "Runs calc, as format 4 says.", "consumes": "io/fs.FS", "corpora": ["sheet"],
      "outputs": [{"pattern": "stdout.txt", "description": "What calc printed, as format 4 says."}], "rejects": []},
    {"name": "old", "description": "A column only format 4 has.", "consumes": "io/fs.FS", "corpora": ["sheet"], "outputs": [], "rejects": []}
  ]
}
`

// TestPreviousFormat: a registry of the format before the report's own at
// a, and one of its own at b. The page shows both revisions' cells,
// properties and descriptions, each from b, or from a for what b lacks.
func TestPreviousFormat(t *testing.T) {
	t.Parallel()
	const gone = "A case only a has.\n-- in/sheet/A1 --\n=A1\n-- properties --\nretired\n" +
		"-- out/calc/outcome --\nrejected\n-- out/calc/stdout.txt --\nloop\n-- out/old/outcome --\naccepted\n" +
		"-- transform/looped/in/sheet/A1 --\n=A1\n-- transform/looped/out/calc/outcome --\nrejected\n"
	a := module(map[string]string{"gone": gone, "kept": "A case both have.\n-- out/calc/outcome --\naccepted\n"})
	a["testdata/hypercase.json"] = format4
	b := module(map[string]string{"kept": "A case both have.\n-- out/calc/outcome --\nrejected\n", "new": "A case only b has.\n-- out/evaluate/outcome --\naccepted\n"})
	g := gittest.New(t)
	c := revisions(t, g, a, b)

	want := "outcome changed\n  sheet/kept calc: accepted -> rejected\n" +
		"appeared\n  sheet/new evaluate: accepted\n" +
		"disappeared\n  sheet/gone calc: rejected\n    stdout.txt removed\n  sheet/gone old: accepted\n  sheet/gone/transform/looped calc: rejected\n"
	if got := ok(t, g, "", "report", "-text", c[0], c[1]); got != want {
		t.Errorf("from format 4 to %d, the report is:\n%s\nwant:\n%s", registry.Format, got, want)
	}
	page := ok(t, g, "", "report", c[0], c[1])
	holds(t, "gone calc's change", change(t, page, "sheet/gone", "calc"), `<span class="prop" title="A property only format 4 lists.">retired</span>`)
	holds(t, "gone/transform/looped calc's change", change(t, page, "sheet/gone/transform/looped", "calc"),
		`<span class="prop given" title="A loop.">circular <span class="how">given</span></span>`,
		`<span class="prop kept" title="A property only format 4 lists.">retired <span class="how">kept</span></span>`)
	d := section(t, page, `<section class="corpus" id="describe-sheet">`, "</section>")
	only := `<span class="badge">only at ` + c[0] + `</span>`
	holds(t, "the descriptions of sheet", d,
		"<dt>calc</dt><dd>Runs calc.</dd>",
		"<dt>old "+only+"</dt><dd>A column only format 4 has.</dd>",
		"<dt>looped "+only+"</dt><dd>Loops, as format 4 says.<p>Gives <span class=\"prop\">circular</span>.</p><p>Keeps <span class=\"prop\">retired</span>.</p></dd>",
		`<dt><span class="case">sheet/gone</span></dt><dd>A case only a has.</dd>`)
	lacks(t, "the descriptions of sheet", d, "sheet/kept", "sheet/new", "transform/looped</span>")
	props := section(t, page, `<section class="properties" id="describe-properties">`, "</section>")
	holds(t, "the properties' descriptions", props, "<dt>circular</dt><dd>A loop.</dd>", "<dt>retired "+only+"</dt><dd>A property only format 4 lists.</dd>")

	// From b to a, the matrix is a's, at format 4.
	back := ok(t, g, "", "report", c[1], c[0])
	_, cells := matrixRow(t, back, "sheet", "sheet/gone")
	holds(t, "gone's calc at format 4", cells[0], `class="cell s-rejected appeared"`)
	holds(t, "gone's old at format 4", cells[1], `class="cell s-accepted appeared"`)
	head, _ := matrixRow(t, back, "sheet", "sheet/gone/transform/looped")
	holds(t, "gone/transform/looped's row at format 4", head, `<span class="prop given" title="A loop, as format 4 says.">circular <span class="how">given</span></span>`)
}
