package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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
// hypercase's main branch wrote to its own, and refuses another: a newer
// one saying to run the report from that revision's hypercase, and an
// older one saying no hypercase's report reads it. It reads both
// revisions' registries before anything else, so an archive it can't read
// never hides the refusal. A registry of another corpus root is left
// unread, whatever its format. A registry that is none, or that its
// format's update couldn't have written, is refused by name.
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
		// A newer format is read by the revision's own hypercase; an older
		// one than any report reads, by none.
		want := fmt.Sprintf("hypercase report: testdata/hypercase.json at %s holds registry format %d, and this hypercase reads formats %d to %d.\n", c[1], other.format, registry.Oldest, registry.Format) +
			"Run the report from " + c[1] + "'s hypercase, which reads it: in a checkout of " + c[1] + ", run the same command as go run github.com/sdboyer/hypercase/cmd/hypercase.\n"
		if other.format < registry.Oldest {
			want = fmt.Sprintf("hypercase report: testdata/hypercase.json at %s holds registry format %d, which no hypercase's report reads: the oldest format a report reads is %d.\n", c[1], other.format, registry.Oldest)
		}
		for _, args := range [][]string{{"report", c[0], c[1]}, {"report", "-text", c[1], c[0]}} {
			if r := hypercase(t, g, "", args...); r.code != 1 || r.stdout != "" || r.stderr != want {
				t.Errorf("hypercase %s: exit %d, stdout %q, stderr:\n%s\nwant exit 1, no stdout, and:\n%s", strings.Join(args, " "), r.code, r.stdout, r.stderr, want)
			}
		}
	}
	// A registry that is no registry fails by name, and so does one its
	// format's update couldn't have written.
	g = gittest.New(t)
	c = revisions(t, g, module(nil), withRegistry(module(arch), "not json\n"))
	fails(t, g, "", 1, []string{"report", c[0], c[1]}, "testdata/hypercase.json at "+c[1]+" can't be read: it is not a registry")
	stale := moduleRegistry()
	stale.Suites[0].Paths = nil
	g = gittest.New(t)
	c = revisions(t, g, module(nil), withRegistry(module(arch), string(stale.Marshal())))
	fails(t, g, "", 1, []string{"report", c[0], c[1]}, "testdata/hypercase.json at "+c[1]+" can't be read: the suite calc runs over testdata/sheet/, and has paths from none,")
}

// history reads the files of one of hypercase's own commits that
// testdata/history holds: the sheets toy's registries and archives, and
// its go.mod, as the commit has them. 20e9646 is main before the format
// that added outcome files, at format 4, and b7e82fe the merge that added
// them, at format 5.
func history(t *testing.T, commit string) map[string]string {
	t.Helper()
	fsys := os.DirFS(filepath.Join("testdata", "history", commit))
	files := map[string]string{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		files[p] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// toyRoots are the corpus roots of the sheets toy at both commits.
var toyRoots = []string{"walkthrough/sheets/step03", "walkthrough/sheets/step04", "walkthrough/sheets/step05", "walkthrough/sheets/step06"}

// TestPreviousFormat: the sheets toy across the format that added outcome
// files, as hypercase's own history holds it: format 4 at 20e9646, and 5
// at b7e82fe, whose archives differ only by an outcome file in every
// cell. Before outcome files, every cell of a case under each column the
// registries claim exists, its state unknown, so the report finds both
// revisions' cells, ranks none, since none has another file that
// changed, and finds no problem, in either direction. The matrix at
// format 4 shows each cell, its state unknown.
func TestPreviousFormat(t *testing.T) {
	t.Parallel()
	g := gittest.New(t)
	c := revisions(t, g, history(t, "20e9646"), history(t, "b7e82fe"))
	for _, root := range toyRoots {
		for _, revs := range [][2]string{{c[0], c[1]}, {c[1], c[0]}} {
			if got := ok(t, g, root, "report", "-text", revs[0], revs[1]); got != "" {
				t.Errorf("%s, from %s to %s: the report is:\n%s\nwant no change", root, revs[0], revs[1], got)
			}
			page := ok(t, g, root, "report", revs[0], revs[1])
			holds(t, root+"'s page", page, `<p class="none">No cell changed.</p>`)
			lacks(t, root+"'s page", page, `<section id="problems">`)
		}
	}
	page := ok(t, g, "walkthrough/sheets/step06", "report", c[1], c[0])
	_, cells := matrixRow(t, page, "sheet", "sheet/blank")
	if len(cells) != 5 {
		t.Fatalf("blank's row has %d cells, want 5", len(cells))
	}
	for i, cell := range cells {
		holds(t, fmt.Sprintf("blank's cell %d at format 4", i), cell, `class="cell s-unknown"><span class="outcome">unknown</span>`)
	}
	m := section(t, page, `<section class="matrix" id="corpus-sheet">`, "</section>")
	if n, rows := strings.Count(m, `<td class="cell s-unknown`), strings.Count(m, `<th scope="row">`); n != 5*rows || rows != 9 {
		t.Errorf("the matrix at format 4 has %d rows and %d cells of unknown state, want 9 and 45", rows, n)
	}
	holds(t, "blank's row at format 4", page, `<span class="case">sheet/blank</span> <span class="props"><span class="prop" title=`)
}

// TestOutcomeBoundary: across the format that added outcome files, a cell
// at both revisions ranks on its other files alone, showing the state it
// has at one revision as unknown at the other, and a cell only at one
// revision appears or disappears with its state, unknown before outcome
// files. An outcome file at a revision before them is a problem, and left
// out, even one that would fail the report at a later format, since the
// report doesn't parse it.
func TestOutcomeBoundary(t *testing.T) {
	t.Parallel()
	const blank = "walkthrough/sheets/step06/testdata/sheet/blank.txtar"
	const divide = "walkthrough/sheets/step06/testdata/sheet/divide.txtar"
	a, b := history(t, "20e9646"), history(t, "b7e82fe")
	a[blank] = strings.Replace(a[blank], "-- out/calc/error.txt --\nA1: not a number", "-- out/calc/error.txt --\nA1 is not a number", 1)
	a[blank] = strings.Replace(a[blank], "-- out/parse/error.txt --", "-- out/format/outcome --\nblocked parse\n-- out/parse/error.txt --", 1)
	a[divide] += "-- out/format/outcome --\nfine\n"
	delete(a, "walkthrough/sheets/step06/testdata/sheet/notes.txtar")
	delete(b, "walkthrough/sheets/step06/testdata/sheet/stray.txtar")
	g := gittest.New(t)
	c := revisions(t, g, a, b)
	const root = "walkthrough/sheets/step06"
	forward := ok(t, g, root, "report", "-text", c[0], c[1])
	problems := "problems\n  at " + c[0] + "\n" +
		"    " + blank + ", out/format/outcome: the section is an outcome file, and registry format 4 predates outcome files, so the report leaves it out.\n" +
		"    " + divide + ", out/format/outcome: the section is an outcome file, and registry format 4 predates outcome files, so the report leaves it out.\n"
	want := problems + "appeared\n" +
		"  sheet/notes calc: rejected\n    error.txt added\n" +
		"  sheet/notes evaluate: blocked parse\n" +
		"  sheet/notes format: blocked parse\n" +
		"  sheet/notes parse: rejected\n    error.txt added\n" +
		"  sheet/notes resolve: blocked parse\n" +
		"disappeared\n" +
		"  sheet/stray calc: unknown\n    error.txt removed\n" +
		"  sheet/stray evaluate: unknown\n" +
		"  sheet/stray format: unknown\n" +
		"  sheet/stray parse: unknown\n    error.txt removed\n" +
		"  sheet/stray resolve: unknown\n" +
		"bytes changed\n  sheet/blank calc: unknown -> rejected\n    error.txt changed\n"
	if forward != want {
		t.Errorf("from format 4 to 5, the report is:\n%s\nwant:\n%s", forward, want)
	}
	backward := ok(t, g, root, "report", "-text", c[1], c[0])
	want = problems + "appeared\n" +
		"  sheet/stray calc: unknown\n    error.txt added\n" +
		"  sheet/stray evaluate: unknown\n" +
		"  sheet/stray format: unknown\n" +
		"  sheet/stray parse: unknown\n    error.txt added\n" +
		"  sheet/stray resolve: unknown\n" +
		"disappeared\n" +
		"  sheet/notes calc: rejected\n    error.txt removed\n" +
		"  sheet/notes evaluate: blocked parse\n" +
		"  sheet/notes format: blocked parse\n" +
		"  sheet/notes parse: rejected\n    error.txt removed\n" +
		"  sheet/notes resolve: blocked parse\n" +
		"bytes changed\n  sheet/blank calc: rejected -> unknown\n    error.txt changed\n"
	if backward != want {
		t.Errorf("from format 5 to 4, the report is:\n%s\nwant:\n%s", backward, want)
	}
	page := ok(t, g, root, "report", c[1], c[0])
	holds(t, "the problems at format 4", section(t, page, `<section id="problems">`, "</section>"),
		`<h3>At <span class="rev">`+c[0]+`</span></h3>`,
		"<li><code>"+blank+", out/format/outcome</code>: the section is an outcome file, and registry format 4 predates outcome files, so the report leaves it out.</li>",
		"<li><code>"+divide+", out/format/outcome</code>: the section is an outcome file, and registry format 4 predates outcome files, so the report leaves it out.</li>")
	holds(t, "blank's calc change", change(t, page, "sheet/blank", "calc"),
		`<span class="outcome s-rejected">rejected</span> <span class="arrow">to</span> <span class="outcome s-unknown">unknown</span>`)
	_, cells := matrixRow(t, page, "sheet", "sheet/blank")
	holds(t, "blank's format at format 4", cells[2], `class="cell s-unknown"><span class="outcome">unknown</span>`)
}

// TestPreOutcomeTransforms: before outcome files, a transform's variant
// runs where the archive holds a section of it and a corpus type lists its
// transform, and so has a cell under every column, its state unknown. A
// variant its transform declined holds no section, and has no cells. One
// whose transform no corpus type lists doesn't run, and has a cell only
// under each column it has files of.
func TestPreOutcomeTransforms(t *testing.T) {
	t.Parallel()
	const root = "walkthrough/sheets/step06"
	a := history(t, "20e9646")
	a[root+"/testdata/hypercase.json"] = strings.Replace(a[root+"/testdata/hypercase.json"], `"transforms": []`,
		`"transforms": [{"name": "doubled", "description": "Doubles each number.", "gives": [], "keeps": [], "probe": false, "owned": true}]`, 1)
	a[root+"/testdata/sheet/blank.txtar"] += "-- transform/doubled/in/sheet/A1 --\n2\n-- transform/gone/in/sheet/A1 --\n2\n-- transform/gone/out/calc/error.txt --\nA1 is gone\n"
	g := gittest.New(t)
	c := revisions(t, g, history(t, "b7e82fe"), a)
	want := "problems\n  at " + c[1] + "\n    " + root + "/testdata/sheet/blank.txtar, transform/gone/: no corpus type of testdata/sheet/ lists the transform gone, so the report can't say what its variant states.\n" +
		"appeared\n"
	for _, col := range []string{"calc", "evaluate", "format", "parse", "resolve"} {
		want += "  sheet/blank/transform/doubled " + col + ": unknown\n"
	}
	want += "  sheet/blank/transform/gone calc: unknown\n    error.txt added\n"
	if got := ok(t, g, root, "report", "-text", c[0], c[1]); got != want {
		t.Errorf("to format 4, the report is:\n%s\nwant:\n%s", got, want)
	}
	page := ok(t, g, root, "report", c[0], c[1])
	rows := rowNames(t, page, "sheet")
	if !slices.Contains(rows, "sheet/blank/transform/doubled") || !slices.Contains(rows, "sheet/blank/transform/gone") || slices.Contains(rows, "sheet/divide/transform/doubled") {
		t.Errorf("the matrix at format 4 has the rows %q, want blank's doubled and gone variants and not divide's", rows)
	}
	_, cells := matrixRow(t, page, "sheet", "sheet/blank/transform/doubled")
	for i, cell := range cells {
		holds(t, fmt.Sprintf("blank's doubled variant's cell %d", i), cell, `class="cell s-unknown appeared"><span class="outcome">unknown</span>`)
	}
	if len(cells) != 5 {
		t.Errorf("blank's doubled variant has %d cells, want 5", len(cells))
	}
	_, cells = matrixRow(t, page, "sheet", "sheet/blank/transform/gone")
	for i, cell := range cells {
		if i == 0 {
			holds(t, "blank's gone variant's calc", cell, `class="cell s-unknown appeared"><span class="outcome">unknown</span>`)
			continue
		}
		lacks(t, fmt.Sprintf("blank's gone variant's cell %d", i), cell, "s-unknown")
	}
	holds(t, "the problems at format 4", section(t, page, `<section id="problems">`, "</section>"),
		"<li><code>"+root+"/testdata/sheet/blank.txtar, transform/gone/</code>: no corpus type of testdata/sheet/ lists the transform gone, so the report can&#39;t say what its variant states.</li>")
}
