package hypercase_test

import (
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/sdboyer/hypercase/internal/diff"
)

// The form of steps 3 to 5 keeps running: a suite over the raw input view,
// fs.FS, names its Corpus and runs over that directory's archives with no
// corpus type, and a suite given to Register bare is the package's own.

// TestRawForm runs every fixture of steps 3 to 5 in that form, and in the
// form of step 6, each in a sandbox of its own: each run's output and exit
// code are the step 6 form's, and so is every file the runs leave, the
// registries aside, and the paths of the temp dirs the runs make. It runs
// three more pairs: a suite over the raw input view beside a corpus type,
// and suites over corpus types given to Register bare.
func TestRawForm(t *testing.T) {
	t.Parallel()
	tempDirs := regexp.MustCompile(regexp.QuoteMeta(filepath.Clean(os.TempDir())) + `/TestCorpus\S*`)
	scrub := func(s string) string { return tempDirs.ReplaceAllString(s, "$$TMP") }
	type pair struct {
		fixture    string
		typed, raw []fixturePkg
	}
	var pairs []pair
	for _, fixture := range slices.Sorted(maps.Keys(fixturePackages)) {
		pkgs := fixturePackages[fixture]
		var raw []fixturePkg
		for _, p := range pkgs {
			if r, ok := rawForms[p.registration]; ok {
				raw = append(raw, fixturePkg{p.dir, r})
			}
		}
		if len(raw) == len(pkgs) {
			pairs = append(pairs, pair{fixture, pkgs, raw})
		}
	}
	if len(pairs) < 30 {
		t.Fatalf("only %d fixtures have a raw form; the fixtures of steps 3 to 5 are more", len(pairs))
	}
	pairs = append(pairs,
		pair{"two-corpora", []fixturePkg{{".", "two-corpora"}}, []fixturePkg{{".", "raw-mixed"}}},
		pair{"basic", []fixturePkg{{".", "echo"}}, []fixturePkg{{".", "bare-echo"}}},
		pair{"params", []fixturePkg{{".", "calc-only"}}, []fixturePkg{{".", "bare-calc"}}},
	)
	for _, p := range pairs {
		t.Run(p.fixture+"/"+p.raw[0].registration, func(t *testing.T) {
			t.Parallel()
			typed, raw := newSandbox(t, p.fixture), newSandbox(t, p.fixture)
			update := []string{fixtureUpdate + "=1"}
			// The raw form's registries: an update that runs no cell writes
			// them, and nothing else.
			for _, pkg := range p.raw {
				raw.exec(pkg.dir, pkg.registration, update, "-test.run=^TestCorpus$/^$")
			}
			for _, step := range []struct {
				what string
				env  []string
			}{{"a normal run", nil}, {"an update", update}, {"a normal run after the update", nil}} {
				for i := range p.typed {
					a := typed.exec(p.typed[i].dir, p.typed[i].registration, step.env)
					b := raw.exec(p.raw[i].dir, p.raw[i].registration, step.env)
					a.out, b.out = scrub(a.out), scrub(b.out)
					if a != b {
						t.Errorf("%s of %s, in %s: the run of %s differs from the run of %s (-%s +%s):\n%s[exit %d, then %d]", step.what, p.fixture, p.typed[i].dir, p.raw[i].registration, p.typed[i].registration, p.typed[i].registration, p.raw[i].registration, diff.Lines([]byte(a.out), []byte(b.out)), a.code, b.code)
					}
				}
			}
			before, after := typed.snapshot(), raw.snapshot()
			for _, f := range slices.Sorted(maps.Keys(before)) {
				if path.Base(f) == "hypercase.json" {
					continue
				}
				if got, ok := after[f]; !ok {
					t.Errorf("%s: the typed form's runs leave %s, and the raw form's don't", p.fixture, f)
				} else if scrub(string(got)) != scrub(string(before[f])) {
					t.Errorf("%s: the runs leave %s different (-%s +%s):\n%s", p.fixture, f, p.typed[0].registration, p.raw[0].registration, diff.Lines(before[f], got))
				}
			}
			for _, f := range slices.Sorted(maps.Keys(after)) {
				if _, ok := before[f]; !ok {
					t.Errorf("%s: the raw form's runs leave %s, and the typed form's don't", p.fixture, f)
				}
			}
		})
	}
}

// TestRawRegistry: the registry records a suite over the raw input view as
// consuming fs.FS, over the directory its Corpus names, and the
// root-package check takes that directory as declared.
func TestRawRegistry(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "basic")
	s.passes(s.update("raw-echo"))
	s.cat("testdata/hypercase.json")
	s.passes(s.normal("raw-echo"))
	s.write("testdata/stray/a.txtar", []byte("A stray.\n-- in/do/out/values.txt --\nA1 1\n"), "add archives in a directory nothing declares")
	s.fails(s.normal("raw-echo"), "testdata/stray/ holds archives, and no registry declares the directory: no corpus type has it as its Dir, and no suite names it as its Corpus.")
	s.done()
}

// TestRawCorpusOfType: a directory is the Corpus of a suite over the raw
// input view, or a corpus type's, not both.
func TestRawCorpusOfType(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	s.passes(s.update("raw-pkg-a"))
	s.passes(s.run("b", "pkg-b", true))
	s.fails(s.normal("raw-pkg-a"),
		"the corpus directory script has two different corpus types:\n  .: the raw input view, fs.FS, which a suite's Corpus names\n  ./b: github.com/sdboyer/hypercase_test.Script: ",
		"An archive's directory gives its type. A suite that names the directory as its Corpus runs over its archives' raw input views, with no type. Make the suite consume the corpus type and remove its Corpus, or move one set of archives to another directory. If the code does one of these already, a registry is stale: run FIXTURE_UPDATE_GOLDEN=1 go test ./..., which rewrites the registry of every package with tests.")
	s.done()
}

func TestRawRefusals(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "basic")
	for _, tt := range []struct{ reg, want string }{
		{"raw-no-corpus", "panic: hypercase: the suite echo consumes fs.FS and names no Corpus. Name the directory of the archives it runs over, testdata/<corpus>/ under the corpus root, or consume a corpus type."},
		{"raw-bad-corpus", `panic: hypercase: the suite echo names the corpus "Script", which breaks the naming rule`},
		{"raw-checks", "panic: hypercase: the suite echo names the corpus checks, a name reserved for the findings of corpus checks. Name another directory."},
		{"raw-of-type", "panic: hypercase: the suite echo names the Corpus script, and the corpus type hypercase_test.Script declares that directory. An archive's directory gives its type: make the suite consume hypercase_test.Script, or a stage it reaches, and remove Corpus."},
		{"raw-of-type-calls", "panic: hypercase: the suite echo names the Corpus script, and the corpus type hypercase_test.Script declares that directory."},
		{"raw-two-names", "panic: hypercase: two columns are named echo: the suite echo, over the raw input views of testdata/script/, and the suite echo, over the raw input views of testdata/script/. Column names are unique; rename one."},
	} {
		r := s.normal(tt.reg)
		s.fails(r, tt.want)
		s.lacks(r, "--- ")
	}
	s.done()
}
