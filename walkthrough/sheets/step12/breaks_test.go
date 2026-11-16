// walkthrough/sheets/step12/breaks_test.go
package step12

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step12/sheetcorpus"
)

// TestBreaks applies each deliberate break to a copy of the module and
// records what it moved, as a transcript compared with
// testdata/breaks/<name>.txt. Each copy is a git repository with two
// commits: the corpus as committed, which the module's update leaves as it
// is, and then the break with what the update writes after it, run until a
// pass writes nothing. The transcript holds the break, the diff of the
// findings between the two commits, each cell whose committed outcome
// moved, and each test the normal run then fails, with why. Each break
// also asserts what it shows. With SHEETS_UPDATE_GOLDEN=1, it writes the
// transcripts.
func TestBreaks(t *testing.T) {
	update := false
	switch v := os.Getenv(sheetcorpus.Project.UpdateVar); v {
	case "":
	case "1":
		update = true
	default:
		t.Fatalf("%s is %q; set it to 1 to update, or leave it unset", sheetcorpus.Project.UpdateVar, v)
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("the breaks run go in a copy of the module: %v", err)
	}
	base := baseline(t)
	for _, b := range breaks {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			r := b.run(t, base)
			if r.findings["circular.txt"] != r.findings["circular-relabelled.txt"] {
				t.Errorf("circular's findings and circular-relabelled's differ after the break:\n%s\n%s", r.findings["circular.txt"], r.findings["circular-relabelled.txt"])
			}
			b.assert(t, r)
			golden := filepath.Join("testdata", "breaks", b.name+".txt")
			if update {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(r.transcript), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v\nRun %s=1 go test ./... to write it, and read it before committing.", err, sheetcorpus.Project.UpdateVar)
			}
			if string(want) != r.transcript {
				t.Errorf("%s differs; got:\n%s\nIf the change is intended, run %s=1 go test ./... and read the diff before committing.", golden, r.transcript, sheetcorpus.Project.UpdateVar)
			}
		})
	}
}

// A brk is one deliberate break.
type brk struct {
	name, about string
	patches     []patch
	assert      func(t *testing.T, r result)
}

// A patch replaces old, which the file holds once, with new, in a file of
// the module by its slash path. An old of "" makes a new file.
type patch struct{ file, old, new string }

// A result is what a break moved.
type result struct {
	findings   map[string]string // each findings file after the break, by name
	diff       string            // the findings' diff between the two commits
	failing    []string          // what the normal run fails after the break
	transcript string
}

var breaks = []brk{
	{
		name:  "resolve-accepts-cycles",
		about: "resolve accepts a sheet with a cycle: a cell it meets again while visiting the cells it refers to is no error.",
		patches: []patch{{
			file: "sheet/resolve.go",
			old:  "\t\t\treturn cycleError(stack[slices.Index(stack, c):])\n",
			new:  "\t\t\treturn nil // the break\n",
		}},
		assert: func(t *testing.T, r result) {
			for _, f := range []string{"circular.txt", "circular-parsed.txt"} {
				r.added(t, f,
					"sheet/loop resolve: predicted rejected, committed accepted",
					"sheet/loop evaluate: predicted blocked resolve, committed accepted",
					"sheet/twice/variant/cycle resolve: predicted rejected, committed accepted",
				)
			}
			r.unchanged(t, "circular-sources.txt", "first-row-empty.txt")
			// No check but the hypotheses judges evaluate's outcome: its
			// cells pass with the outcome the update wrote.
			r.passes(t, "/evaluate:")
		},
	},
	{
		name:  "format-skips-self-reference",
		about: "format's probe says no to a sheet whose first formula cell refers only to itself, as each looped variant's does.",
		patches: []patch{{
			file: "step12/format/corpus_test.go",
			old:  "\t\tif len(p.Formulas()) == 0 {\n\t\t\treturn \"no formulas\", false\n\t\t}\n",
			new:  "\t\tif len(p.Formulas()) == 0 {\n\t\t\treturn \"no formulas\", false\n\t\t}\n\t\tif f := p.Formulas()[0]; strings.HasPrefix(string(sheet.Format(p)), f+\" =\"+f+\"\\n\") { // the break\n\t\t\treturn \"a cell refers to itself\", false\n\t\t}\n",
		}, {
			file: "step12/format/corpus_test.go",
			old:  "import (\n\t\"io\"\n",
			new:  "import (\n\t\"io\"\n\t\"strings\"\n",
		}},
		assert: func(t *testing.T, r result) {
			for _, f := range []string{"circular.txt", "circular-parsed.txt"} {
				r.added(t, f,
					"sheet/twice/transform/looped format: predicted the base's outcome, accepted; committed skipped a cell refers to itself",
					"sheet/prices/transform/looped format: predicted the base's outcome, accepted; committed skipped a cell refers to itself",
				)
			}
			r.unchanged(t, "circular-sources.txt", "first-row-empty.txt")
		},
	},
	{
		name:  "parse-drops-product-refs",
		about: "parse reads a reference that starts a product as the number 1, so =B1*2 is 1 * 2: it stops seeing a class of formula. Question 2's scenario.",
		patches: []patch{{
			file: "sheet/formula.go",
			old:  "\t\top, ok := p.op(\"*/\")\n\t\tif !ok {\n\t\t\treturn l, nil\n\t\t}\n",
			new:  "\t\top, ok := p.op(\"*/\")\n\t\tif !ok {\n\t\t\treturn l, nil\n\t\t}\n\t\tif _, isRef := l.(ref); isRef {\n\t\t\tl = number(1) // the break\n\t\t}\n",
		}},
		assert: func(t *testing.T, r result) {
			// What each case states hasn't moved, and circular's prediction
			// for twice's cycle variant fails at resolve and at evaluate.
			r.added(t, "circular.txt",
				"sheet/twice/variant/cycle resolve: predicted rejected, committed accepted",
				"sheet/twice/variant/cycle evaluate: predicted blocked resolve, committed accepted",
			)
			// The predicate reads the same parse: the variant is anti-P to
			// it now, the prediction is accepted, and resolve accepts.
			r.unchanged(t, "circular-parsed.txt")
			r.added(t, "circular-sources.txt",
				"sheet/twice/variant/cycle: stated P, parsed anti-P, which disagree",
				"sheet/twice/variant/missing: stated neither, parsed anti-P, which disagree",
			)
			r.unchanged(t, "first-row-empty.txt")
		},
	},
	{
		name:  "values-row-one-only",
		about: "evaluate's values.txt holds only the cells on the first row.",
		patches: []patch{{
			file: "sheet/eval.go",
			old:  "\tfor _, name := range slices.SortedFunc(maps.Keys(v), compareNames) {\n",
			new:  "\tfor _, name := range slices.SortedFunc(maps.Keys(v), compareNames) {\n\t\tif name[len(name)-1] != '1' || name[len(name)-2] > 'Z' || name[len(name)-2] < 'A' {\n\t\t\tcontinue // the break\n\t\t}\n",
		}},
		assert: func(t *testing.T, r result) {
			r.added(t, "first-row-empty.txt",
				`sheet/twice/transform/renamed evaluate: predicted the base's values.txt one row down, "A2 6\nB2 3\n"; committed ""`,
				"sheet/divide/transform/renamed evaluate: predicted the base's outcome, accepted div-zero; committed accepted",
			)
			r.unchanged(t, "circular.txt", "circular-parsed.txt", "circular-sources.txt")
			r.passes(t, "")
		},
	},
	{
		name:  "low-states-first-row-empty",
		about: "A new base, low, starts on the second row, and states first-row-empty, which is true of it.",
		patches: []patch{{
			file: "step12/testdata/sheet/low.txtar",
			new:  "A2 is twice B2, and the first row is empty.\n-- in/sheet/A2 --\n=B2*2\n-- in/sheet/B2 --\n3\n-- properties --\nfirst-row-empty\n",
		}},
		assert: func(t *testing.T, r result) {
			r.noCase(t, "sheet/low/transform/looped")
			r.unchanged(t, "circular.txt", "circular-parsed.txt", "first-row-empty.txt")
			r.passes(t, "")
		},
	},
	{
		name:  "low-states-nothing",
		about: "The same base, low, states nothing, though its first row is empty.",
		patches: []patch{{
			file: "step12/testdata/sheet/low.txtar",
			new:  "A2 is twice B2, and the first row is empty.\n-- in/sheet/A2 --\n=B2*2\n-- in/sheet/B2 --\n3\n",
		}},
		assert: func(t *testing.T, r result) {
			r.added(t, "circular-sources.txt", "sheet/low/transform/looped: stated P, parsed P")
			r.unchanged(t, "circular.txt", "circular-parsed.txt", "first-row-empty.txt")
			r.passes(t, "")
		},
	},
	{
		name:  "format-omits-first-row-empty",
		about: "format's package doesn't register first-row-empty, which it neither rejects nor reads, and which renamed gives.",
		patches: []patch{{
			file: "step12/format/corpus_test.go",
			old:  "sheetcorpus.Circular, sheetcorpus.Dangling, sheetcorpus.FirstRowEmpty,\n",
			new:  "sheetcorpus.Circular, sheetcorpus.Dangling,\n",
		}},
		assert: func(t *testing.T, r result) {
			if !strings.Contains(r.transcript, "step12/format TestCorpus: unregistered property") {
				t.Error("format's run doesn't fail on the property it doesn't register")
			}
		},
	},
}

// added fails unless the diff of the findings file name adds each line.
func (r result) added(t *testing.T, name string, lines ...string) {
	t.Helper()
	for _, l := range lines {
		if !strings.Contains(r.diff, "\n+"+l+"\n") {
			t.Errorf("the break adds no line %q to %s", l, name)
		}
	}
}

// unchanged fails unless the break leaves each findings file as it was.
func (r result) unchanged(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		if strings.Contains(r.diff, "/testdata/checks/"+n+"\n") {
			t.Errorf("the break moves %s", n)
		}
	}
}

// passes fails if the normal run after the break fails a test whose line
// holds s: any, for "".
func (r result) passes(t *testing.T, s string) {
	t.Helper()
	for _, f := range r.failing {
		if strings.Contains(f, s) {
			t.Errorf("the normal run after the break fails %s", f)
		}
	}
}

// noCase fails if a cell of the named case has an outcome after the
// break.
func (r result) noCase(t *testing.T, name string) {
	t.Helper()
	if strings.Contains(r.transcript, "\n"+name+" ") {
		t.Errorf("the break leaves cells of %s", name)
	}
}

// run applies the break to a copy of the baseline, and records what moved.
func (b brk) run(t *testing.T, base string) result {
	dir := t.TempDir()
	copyTree(t, base, dir)
	var applied strings.Builder
	for _, p := range b.patches {
		path := filepath.Join(dir, filepath.FromSlash(p.file))
		if p.old == "" {
			if err := os.WriteFile(path, []byte(p.new), 0o644); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&applied, "+++ %s\n%s", p.file, prefixLines("+", p.new))
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if n := bytes.Count(data, []byte(p.old)); n != 1 {
			t.Fatalf("%s holds the patch's old text %d times, not once:\n%s", p.file, n, p.old)
		}
		if err := os.WriteFile(path, bytes.Replace(data, []byte(p.old), []byte(p.new), 1), 0o644); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&applied, "--- %s\n%s", p.file, replacement(p.old, p.new))
	}
	before := outcomes(t, base)
	settle(t, dir)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "the break")
	diff := git(t, dir, "diff", "--no-color", "--no-ext-diff", "HEAD~1", "HEAD", "--", "step12/testdata/checks")
	after := outcomes(t, dir)

	r := result{findings: map[string]string{}, diff: diff}
	entries, err := os.ReadDir(filepath.Join(dir, "step12", "testdata", "checks"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, "step12", "testdata", "checks", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		r.findings[e.Name()] = string(data)
	}

	var tr strings.Builder
	fmt.Fprintf(&tr, "%s\n\n== break\n%s\n== findings\n%s\n== outcomes\n", b.about, applied.String(), diff)
	for _, c := range slices.Sorted(maps.Keys(after)) {
		if o, ok := before[c]; !ok || o != after[c] {
			if !ok {
				o = "none"
			}
			fmt.Fprintf(&tr, "%s: %s -> %s\n", c, o, after[c])
		}
	}
	for _, c := range slices.Sorted(maps.Keys(before)) {
		if _, ok := after[c]; !ok {
			fmt.Fprintf(&tr, "%s: %s -> none\n", c, before[c])
		}
	}
	tr.WriteString("\n== failing\n")
	r.failing = failing(t, dir)
	for _, f := range r.failing {
		tr.WriteString(f + "\n")
	}
	r.transcript = tr.String()
	return r
}

// replacement shows old replaced by new: the lines they share at either
// end once, as context, and the rest as removed and added.
func replacement(old, new string) string {
	o, n := strings.SplitAfter(old, "\n"), strings.SplitAfter(new, "\n")
	pre := 0
	for pre < len(o) && pre < len(n) && o[pre] == n[pre] && o[pre] != "" {
		pre++
	}
	suf := 0
	for suf < len(o)-pre && suf < len(n)-pre && o[len(o)-1-suf] == n[len(n)-1-suf] {
		suf++
	}
	return prefixLines(" ", strings.Join(o[:pre], "")) +
		prefixLines("-", strings.Join(o[pre:len(o)-suf], "")) +
		prefixLines("+", strings.Join(n[pre:len(n)-suf], "")) +
		prefixLines(" ", strings.Join(o[len(o)-suf:], ""))
}

func prefixLines(prefix, s string) string {
	var b strings.Builder
	for _, l := range strings.SplitAfter(s, "\n") {
		if l != "" {
			b.WriteString(prefix + l)
		}
	}
	return b.String()
}

// baseline copies the module's sheet package and step12 to a git
// repository in a temp dir, settles them with the module's update, and
// commits them. The copy builds against this checkout of hypercase.
func baseline(t *testing.T) string {
	t.Helper()
	module, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	hypercaseRoot := filepath.Join(module, "..", "..")
	if data, err := os.ReadFile(filepath.Join(hypercaseRoot, "go.mod")); err != nil || !bytes.HasPrefix(data, []byte("module github.com/sdboyer/hypercase\n")) {
		t.Fatalf("%s is not hypercase's checkout: %v", hypercaseRoot, err)
	}
	dir := t.TempDir()
	for _, name := range []string{"go.sum", ".gitignore", ".gitattributes"} {
		copyFile(t, filepath.Join(module, name), filepath.Join(dir, name))
	}
	gomod, err := os.ReadFile(filepath.Join(module, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	const replace = "replace github.com/sdboyer/hypercase => ../..\n"
	if bytes.Count(gomod, []byte(replace)) != 1 {
		t.Fatalf("go.mod has no line %q", replace)
	}
	gomod = bytes.Replace(gomod, []byte(replace), []byte("replace github.com/sdboyer/hypercase => "+hypercaseRoot+"\n"), 1)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), gomod, 0o644); err != nil {
		t.Fatal(err)
	}
	copyTree(t, filepath.Join(module, "sheet"), filepath.Join(dir, "sheet"))
	copyTree(t, filepath.Join(module, "step12"), filepath.Join(dir, "step12"))
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "committed")
	settle(t, dir)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "settled")
	return dir
}

// settle runs the module's update until a pass writes nothing.
func settle(t *testing.T, dir string) {
	t.Helper()
	gotest(t, dir, false, "-run", "^$") // a patch that doesn't build fails here
	prev := tree(t, dir)
	for range 5 {
		gotest(t, dir, true, "-run", "^TestCorpus$")
		next := tree(t, dir)
		if next == prev {
			return
		}
		prev = next
	}
	t.Fatal("five passes of the update didn't settle")
}

// tree is the hash of the working tree's contents.
func tree(t *testing.T, dir string) string {
	git(t, dir, "add", "-A")
	return git(t, dir, "write-tree")
}

// gotest runs go test on step12's packages in dir, as an update or a
// normal run, and returns its output. Only a run that leaves an update
// mode's tests unrun or fails to build fails the test: the breaks fail
// cells on purpose.
func gotest(t *testing.T, dir string, update bool, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("go", append(append([]string{"test", "-count=1", "-trimpath"}, args...), "./step12/...")...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		switch k, _, _ := strings.Cut(kv, "="); k {
		case sheetcorpus.Project.UpdateVar, "GOFLAGS", "GOWORK", "GOTOOLCHAIN", "GOPROXY":
			continue
		}
		cmd.Env = append(cmd.Env, kv)
	}
	cmd.Env = append(cmd.Env, "GOFLAGS=-mod=readonly", "GOWORK=off", "GOTOOLCHAIN=local", "GOPROXY=off")
	if update {
		cmd.Env = append(cmd.Env, sheetcorpus.Project.UpdateVar+"=1")
	}
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if err != nil && !errors.As(err, &ee) {
		t.Fatalf("go test: %v", err)
	}
	if bytes.Contains(out, []byte("[build failed]")) || bytes.Contains(out, []byte("[setup failed]")) || (len(args) > 1 && args[1] == "^$" && err != nil) {
		t.Fatalf("go test %s in the copy fails to build:\n%s", strings.Join(args, " "), out)
	}
	return out
}

// failing runs step12's packages normally, and gives each test that fails
// with messages of its own, and the kinds of failure its messages name.
func failing(t *testing.T, dir string) []string {
	t.Helper()
	out := gotest(t, dir, false, "-run", "^TestCorpus$", "-json")
	type event struct{ Action, Package, Test, Output string }
	failed := map[[2]string]bool{}
	own := map[[2]string]*strings.Builder{}
	for _, line := range bytes.Split(out, []byte("\n")) {
		var e event
		if len(line) == 0 || json.Unmarshal(line, &e) != nil || e.Test == "" {
			continue
		}
		key := [2]string{strings.TrimPrefix(e.Package, "github.com/sdboyer/hypercase/walkthrough/sheets/"), e.Test}
		switch e.Action {
		case "fail":
			failed[key] = true
		case "output":
			trimmed := strings.TrimSpace(e.Output)
			if strings.HasPrefix(trimmed, "=== ") || strings.HasPrefix(trimmed, "--- ") {
				continue
			}
			if own[key] == nil {
				own[key] = &strings.Builder{}
			}
			own[key].WriteString(e.Output)
		}
	}
	var lines []string
	for key := range failed {
		if own[key] == nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s %s: %s", key[0], key[1], strings.Join(failureKinds(own[key].String()), ", ")))
	}
	slices.Sort(lines)
	return lines
}

// failureKinds names the kinds of failure a test's messages hold.
func failureKinds(msg string) []string {
	kinds := []struct{ kind, phrase string }{
		{"expected error missing", "returned no error, but the"},
		{"unexpected error", "returned an error, but the"},
		{"orphan", "records an error, but the run returned none"},
		{"orphan", "records output, but the run wrote nothing"},
		{"orphan", "is an output the run no longer produces"},
		{"orphan", "is recorded, and "},
		{"golden differs", "differs (-golden +got)"},
		{"frozen cell differs", "frozen by its known gap"},
		{"corpus check", "the corpus check"},
		{"unregistered property", "registers no property of that name"},
		{"panic", "panicked"},
	}
	var got []string
	for _, k := range kinds {
		if strings.Contains(msg, k.phrase) && !slices.Contains(got, k.kind) {
			got = append(got, k.kind)
		}
	}
	if len(got) == 0 {
		got = append(got, "other")
	}
	slices.Sort(got)
	return got
}

// outcomes reads each cell's committed outcome from step12's archives, by
// case and column.
func outcomes(t *testing.T, dir string) map[string]string {
	t.Helper()
	sheets := filepath.Join(dir, "step12", "testdata", "sheet")
	entries, err := os.ReadDir(sheets)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".txtar")
		if !ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sheets, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range archive.Parse(data).Sections {
			kind, vname, rest := archive.SplitCase(s.Name)
			col, ok := strings.CutPrefix(rest, "out/")
			if !ok {
				continue
			}
			col, ok = strings.CutSuffix(col, "/"+archive.OutcomeFile)
			if !ok {
				continue
			}
			c := "sheet/" + name
			if kind != "" {
				c += "/" + kind + "/" + vname
			}
			m[c+" "+col] = strings.TrimSuffix(string(s.Data), "\n")
		}
	}
	return m
}

// git runs git in dir with none of the machine's configuration, and fixed
// names and dates, and returns its output.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") && !strings.HasPrefix(kv, "LC_") && !strings.HasPrefix(kv, "LANG") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env,
		"LC_ALL=C",
		"GIT_CEILING_DIRECTORIES="+filepath.Dir(dir),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	)
	out, err := cmd.Output()
	if err != nil {
		msg := err.Error()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			msg += ": " + string(ee.Stderr)
		}
		t.Fatalf("git %s: %s", strings.Join(args, " "), msg)
	}
	return string(out)
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Name() == archive.LockFile || archive.IsTemp(d.Name()) {
			return nil
		}
		copyFile(t, p, target)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
