package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/gittest"
	"github.com/sdboyer/hypercase/internal/registry"
)

// findingsModule returns a module as module does, with a registry at the
// format given, listing the checks given, and the findings files given, by
// check name, in the module root's testdata/checks/.
func findingsModule(format int, checks []registry.Check, findings map[string]string) map[string]string {
	reg := moduleRegistry()
	reg.Checks = checks
	files := module(nil)
	files["testdata/hypercase.json"] = string(reg.Marshal())
	if format < registry.Format {
		files["testdata/hypercase.json"] = registryAt(reg, format)
	}
	for name, data := range findings {
		files["testdata/checks/"+name+".txt"] = data
	}
	return files
}

// TestFindingsGenerated: over every pair of sets drawn from three findings,
// each the findings of one check at A and at B, the report lists, under
// findings changed, each check whose findings differ, by package and
// check, with the findings only at A marked -, then those only at B
// marked +, each in order, and with the check's description on the page;
// and no check whose findings are the same. With A's registry from before
// the format that lists checks, the report reads A's findings files by
// name, so a pair shows the same change. With A from before findings
// files, and so with none, B's findings all appear.
func TestFindingsGenerated(t *testing.T) {
	t.Parallel()
	all := []string{"A1 counts 1", "B1 counts 2", "C1 counts 3"}
	var sets [][]string
	for mask := range 1 << len(all) {
		var s []string
		for i, f := range all {
			if mask&(1<<i) != 0 {
				s = append(s, f)
			}
		}
		sets = append(sets, s)
	}
	file := func(s []string) string {
		if len(s) == 0 {
			return ""
		}
		return strings.Join(s, "\n") + "\n"
	}
	type pair struct {
		name string
		a, b []string
	}
	var pairs []pair
	for i, a := range sets {
		for j, b := range sets {
			pairs = append(pairs, pair{fmt.Sprintf("c-%d-%d", i, j), a, b})
		}
	}
	var checks []registry.Check
	files := [2]map[string]string{{}, {}}
	for _, p := range pairs {
		checks = append(checks, registry.Check{Name: p.name, Description: "Counts, as " + p.name + " does."})
		files[0][p.name], files[1][p.name] = file(p.a), file(p.b)
	}
	for _, tt := range []struct {
		name    string
		formatA int
		// none records that A has no findings files.
		none bool
	}{
		{name: "a lists its checks", formatA: registry.Format},
		{name: "a before checks are listed", formatA: registry.ChecksFormat - 1},
		{name: "a before findings files", formatA: registry.InputsFormat - 1, none: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			atA := files[0]
			if tt.none {
				atA = nil
			}
			g := gittest.New(t)
			c := revisions(t, g, findingsModule(tt.formatA, checks, atA), findingsModule(registry.Format, checks, files[1]))
			var want strings.Builder
			for _, p := range pairs {
				a := p.a
				if tt.none {
					a = nil
				}
				var lines []string
				for _, f := range a {
					if !slices.Contains(p.b, f) {
						lines = append(lines, "    - "+f)
					}
				}
				for _, f := range p.b {
					if !slices.Contains(a, f) {
						lines = append(lines, "    + "+f)
					}
				}
				if len(lines) > 0 {
					want.WriteString("  . " + p.name + "\n" + strings.Join(lines, "\n") + "\n")
				}
			}
			text := ok(t, g, "", "report", "-text", c[0], c[1])
			if w := "findings changed\n" + want.String(); text != w {
				t.Errorf("got:\n%s\nwant:\n%s", text, w)
			}
			page := ok(t, g, "", "report", c[0], c[1])
			findings := section(t, page, `<h3 class="rank findings-changed">`, "</ul>")
			holds(t, "the findings", findings, `<span class="pkg">.</span> <span class="check">c-0-7</span>: Counts, as c-0-7 does.</div>`+"\n"+
				`<pre class="findings"><span class="add">&#43; A1 counts 1</span><span class="add">&#43; B1 counts 2</span><span class="add">&#43; C1 counts 3</span></pre>`)
			lacks(t, "the findings", findings, `<span class="check">c-0-0</span>`)
			if !tt.none {
				holds(t, "the findings", findings, `<span class="check">c-1-2</span>: Counts, as c-1-2 does.</div>`+"\n"+
					`<pre class="findings"><span class="del">- A1 counts 1</span><span class="add">&#43; B1 counts 2</span></pre>`)
				lacks(t, "the findings", findings, `<span class="check">c-7-7</span>`)
			}
		})
	}
}

// TestFindingsProblems: what the report can't read of a revision's findings
// is a problem there, and the check shows no change: a check the registry
// lists with no findings file, one whose findings file's name names a
// directory, and a findings file not as an update writes it, out of order,
// with a line twice or empty, or with no final newline. Each other file in
// testdata/checks/, the findings of no check the registry lists, is a
// problem too, left out. Before the format that lists checks, a findings
// file is read by its name, so neither listing problem arises, and a file
// whose name is no check's, or a directory, is left out. A revision's
// problems are listed by path. Two packages' checks of one name stay
// apart, and a check that only A's registry lists shows A's description. A
// finding with a carriage return is shown quoted, as a diff shows a line.
func TestFindingsProblems(t *testing.T) {
	t.Parallel()
	check := func(name string) registry.Check {
		return registry.Check{Name: name, Description: "Checks " + name + "."}
	}
	bad := map[string]string{
		"unsorted":   "b\na\n",
		"repeated":   "a\na\n",
		"empty-line": "a\n\nb\n",
		"no-newline": "a",
	}
	whys := map[string]string{
		"unsorted":   "line 2 sorts before line 1",
		"repeated":   "line 2 repeats line 1",
		"empty-line": "line 2 is empty",
		"no-newline": "its last line has no newline",
	}
	var badChecks []registry.Check
	for name := range bad {
		badChecks = append(badChecks, check(name))
	}
	a := findingsModule(registry.Format, append([]registry.Check{check("missing"), check("dir"), check("cr"), check("gone"), check("shared")}, badChecks...),
		map[string]string{"cr": "", "gone": "x\n", "shared": "root\n"})
	b := findingsModule(registry.Format, append([]registry.Check{check("missing"), check("dir"), check("cr"), check("shared")}, badChecks...),
		map[string]string{"cr": "a\rb\n", "shared": "root\nroot 2\n", "orphan": "o\n"})
	// A directory where dir's findings file would be.
	a["testdata/checks/dir.txt/x"], b["testdata/checks/dir.txt/x"] = "x\n", "y\n"
	for name, data := range bad {
		a["testdata/checks/"+name+".txt"] = data
		b["testdata/checks/"+name+".txt"] = "z\n" // a change the report can't show
	}
	b["testdata/checks/README"] = "Not findings.\n"
	// Another package, with a check of one name, and its findings changed.
	pkg := moduleRegistry()
	pkg.RootPackage = false
	pkg.Suites = nil
	pkg.Checks = []registry.Check{check("shared")}
	// A third, before checks are listed, with a findings file read by its
	// name, and a file whose name is no check's.
	old := pkg
	old.Checks = nil
	for i, files := range []map[string]string{a, b} {
		files["p/testdata/hypercase.json"] = string(pkg.Marshal())
		files["p/testdata/checks/shared.txt"] = []string{"p\n", "p\np 2\n"}[i]
		files["q/testdata/hypercase.json"] = registryAt(old, registry.ChecksFormat-1)
		files["q/testdata/checks/found.txt"] = []string{"", "found\n"}[i]
		files["q/testdata/checks/Not_A_Check.txt"] = []string{"", "n\n"}[i]
		files["q/testdata/checks/qdir.txt/x"] = []string{"x\n", "y\n"}[i]
	}
	g := gittest.New(t)
	c := revisions(t, g, a, b)
	text := ok(t, g, "", "report", "-text", c[0], c[1])

	const missing = "testdata/hypercase.json lists the corpus check missing, and the package has no findings file for it, which an update writes for each check it lists. So the report shows no change in its findings here."
	const orphan = "the file is the findings of no corpus check testdata/hypercase.json lists, so the report leaves it out."
	const dir = "testdata/hypercase.json lists the corpus check dir, and this isn't a regular file, as the findings file an update writes for it is. So the report shows no change in its findings here."
	problems := [2][][2]string{
		{{"testdata/checks/missing.txt", missing}, {"testdata/checks/dir.txt", dir}},
		{{"testdata/checks/missing.txt", missing}, {"testdata/checks/dir.txt", dir}, {"testdata/checks/orphan.txt", orphan}, {"testdata/checks/README", orphan}},
	}
	for name := range bad {
		problems[0] = append(problems[0], [2]string{"testdata/checks/" + name + ".txt", "the findings file isn't as an update writes it: " + whys[name] + ". So the report shows no change in the check's findings here."})
	}
	var want strings.Builder
	want.WriteString("problems\n")
	for _, i := range []int{1, 0} {
		slices.SortFunc(problems[i], func(x, y [2]string) int { return strings.Compare(x[0], y[0]) })
		want.WriteString("  at " + c[i] + "\n")
		for _, p := range problems[i] {
			want.WriteString("    " + p[0] + ": " + p[1] + "\n")
		}
	}
	want.WriteString("findings changed\n" +
		"  . cr\n    + \"a\\rb\"\n" +
		"  . gone\n    - x\n" +
		"  . shared\n    + root 2\n" +
		"  p shared\n    + p 2\n" +
		"  q found\n    + found\n")
	if text != want.String() {
		t.Errorf("got:\n%s\nwant:\n%s", text, want.String())
	}
	page := ok(t, g, "", "report", c[0], c[1])
	holds(t, "the findings", section(t, page, `<h3 class="rank findings-changed">`, "</ul>"),
		`<span class="pkg">.</span> <span class="check">gone</span>: Checks gone.</div>`,
		`<span class="pkg">q</span> <span class="check">found</span></div>`,
		`<span class="pkg">.</span> <span class="check">cr</span>: Checks cr.</div>`+"\n"+`<pre class="findings"><span class="add">&#43; &#34;a\rb&#34;</span></pre>`)
}
