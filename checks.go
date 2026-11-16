package hypercase

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/diff"
	"github.com/sdboyer/hypercase/internal/registry"
)

// A Check is a corpus check: an assertion about a group of cells, read
// from their committed files. It never runs a suite, a lift or a
// transform. The package that registers it, with [Register] or [Own],
// owns it, and [Test] runs it as the subtest checks/<Name>, once the
// package's cases have run.
//
// Its findings are a golden, testdata/checks/<Name>.txt in the package's
// directory: one finding per line, sorted byte by byte, each once. A
// normal run compares them, and an update writes them.
type Check struct {
	// Name names the check, and its findings' file, under the naming rule.
	// Names are unique per package.
	Name string
	// Description says what the check asserts. It is required.
	Description string
	// Run reads every cell over the corpus root, and returns the check's
	// findings, each one line, not empty. An error is a requirement the
	// cells violate: it fails every run, updates included, and an update
	// then writes no findings. So does a panic.
	Run func(cells []Cell) ([]string, error)
}

// A Cell is a cell as a corpus check sees it: what the archives and the
// registries under the corpus root commit, read from disk.
type Cell struct {
	// Type names the cell's corpus by its directory, testdata/<Type>/
	// under the corpus root.
	Type string
	// Archive is the archive's name, less .txtar.
	Archive string
	// Case is "" for the archive's base, or variant/<name> or
	// transform/<name> for one of its variants.
	Case string
	// Column is the column's name.
	Column string
	// Outcome is the cell's committed outcome. It is the zero Outcome
	// when the cell has no outcome file.
	Outcome archive.Outcome
	// Entry is the cell's column, as its package's registry records it.
	Entry Entry
	// Properties are what the case states: the base's properties, a
	// hand-written variant's own, or what a transform gives and each
	// property it keeps that the base states.
	Properties []string
	// Files are the cell's committed files, the outcome among them, by
	// their paths under out/<Column>/.
	Files map[string][]byte
}

// An Entry is a column as its package's registry records it.
type Entry struct {
	// Kind is "suite" or "lift".
	Kind        string
	Name        string
	Description string
	// Package is the package that owns the column, by its directory
	// relative to the module root, as go test names it: ., ./format.
	Package string
	// Rejects names the properties the column rejects.
	Rejects []string
	// Classifier names a suite's classifier, or is "" for none.
	Classifier string
}

// addCheck registers a check the package owns.
func (r *registrations) addCheck(c Check) {
	switch {
	case !validName(c.Name):
		refuse(fmt.Sprintf("the corpus check name %q breaks the naming rule: %s. Rename the check.", c.Name, nameRule))
	case c.Run == nil:
		refuse(fmt.Sprintf("the corpus check %s has no Run. Give it the function that reads the cells and gives its findings.", c.Name))
	}
	for _, o := range r.checks {
		if o.Name == c.Name {
			refuse(fmt.Sprintf("two corpus checks are named %s. Check names are unique per package; rename one.", c.Name))
		}
	}
	r.checks = append(r.checks, c)
}

// findingsDir is the directory of the package's findings.
func (r *runner) findingsDir() string {
	return filepath.Join(r.pkgDir, "testdata", checksDir)
}

// runChecks runs the package's corpus checks, each the subtest
// checks/<name>, over the cells as the files on disk commit them, and
// fails each findings file no check of the package owns.
func (r *runner) runChecks(t *testing.T, found []registry.Found) {
	t.Helper()
	r.orphanFindings(t)
	if len(r.checks) == 0 {
		return
	}
	cells, err := r.readCells(found)
	t.Run(checksDir, func(t *testing.T) {
		for _, c := range r.checks {
			t.Run(c.Name, func(t *testing.T) {
				if err != nil {
					t.Errorf("the corpus check %s can't read its cells: %v\nFix what the message names. No check runs, and an update writes no findings.", c.Name, err)
					return
				}
				r.runCheck(t, c, cloneCells(cells))
			})
		}
	})
}

// runCheck runs one check, and compares its findings with their golden,
// or writes them in an update.
func (r *runner) runCheck(t *testing.T, c Check, cells []Cell) {
	var findings []string
	var err error
	p := call(func(*called) { findings, err = c.Run(cells) })
	switch {
	case p.panicked:
		t.Errorf("the corpus check %s panicked: %v\nFix the check. An update writes no findings.", c.Name, p.panicValue)
		return
	case err != nil:
		t.Errorf("the corpus check %s fails: %v\nThe cells violate what the check requires. Fix the code or the corpus, or the check. An update writes no findings.", c.Name, err)
		return
	}
	for _, f := range findings {
		switch {
		case f == "":
			t.Errorf("the corpus check %s gave an empty finding.\nFix the check: a finding says something, on one line. An update writes no findings.", c.Name)
			return
		case strings.Contains(f, "\n"):
			t.Errorf("the corpus check %s gave a finding that holds a newline: %q.\nFix the check: a finding is one line. An update writes no findings.", c.Name, f)
			return
		}
	}
	var data []byte
	for _, f := range slices.Compact(slices.Sorted(slices.Values(findings))) {
		data = append(data, f+"\n"...)
	}
	if data == nil {
		data = []byte{}
	}
	golden := filepath.Join(r.findingsDir(), c.Name+".txt")
	got, rerr := os.ReadFile(golden)
	if rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
		t.Errorf("reading %s: %v", r.rel(golden), rerr)
		return
	}
	if r.update {
		if rerr == nil && bytes.Equal(got, data) {
			return
		}
		if err := os.MkdirAll(r.findingsDir(), 0o755); err != nil {
			t.Errorf("writing %s: %v", r.rel(golden), err)
			return
		}
		if err := archive.WriteFile(golden, data); err != nil {
			t.Errorf("writing %s: %v", r.rel(golden), err)
		}
		return
	}
	switch {
	case rerr != nil:
		t.Errorf("%s, the corpus check %s's findings, has no golden.\nRun %s to write it, and read it before committing.", r.rel(golden), c.Name, r.updateCmd())
	case !bytes.Equal(got, data):
		t.Errorf("%s, the corpus check %s's findings, differs (-golden +got):\n%s\nIf the change is intended, run %s and read the diff before committing.", r.rel(golden), c.Name, strings.TrimSuffix(diff.Lines(got, data), "\n"), r.updateCmd())
	}
}

// orphanFindings fails each file under the package's testdata/checks/
// that is no check's findings, in a normal run. An update leaves it.
func (r *runner) orphanFindings(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(r.findingsDir())
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("reading %s: %v", r.rel(r.findingsDir()), err)
		}
		return
	}
	for _, e := range entries {
		p := filepath.Join(r.findingsDir(), e.Name())
		name, ok := strings.CutSuffix(e.Name(), ".txt")
		switch {
		case archive.IsTemp(e.Name()):
			t.Errorf("%s is a temp file left by a write that was cut short.\nDelete it.", r.rel(p))
		case r.update:
		case !ok || e.IsDir() || !slices.ContainsFunc(r.checks, func(c Check) bool { return c.Name == name }):
			t.Errorf("%s is the findings of no corpus check the package registers.\nIf a check should write it, register the check. If not, delete it by hand; %s leaves it in place.", r.rel(p), r.updateCmd())
		}
	}
}

// readCells reads every cell over the corpus root: each case of each
// archive in each corpus a registry under the corpus root has columns
// over, and each such column. They are sorted by corpus, then archive,
// then case, the base first, then its hand-written variants in the order
// written, then its transforms' by name, then column. A registry or an
// archive that can't be read, or a cell's outcome, is an error, and so is
// a transform's variant whose properties can't be known: one no corpus
// type lists, or one its transform declines.
func (r *runner) readCells(found []registry.Found) ([]Cell, error) {
	columns := map[string][]Entry{}                          // corpus → its columns
	transforms := map[string]map[string]registry.Transform{} // corpus → transform → its declaration
	for _, f := range found {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, err
		}
		if h, err := registry.Peek(data); err == nil && h.CorpusRoot != r.rootRel {
			continue
		}
		g, err := registry.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", r.rel(f.Path), err)
		}
		pkg := r.pkgName(f.Pkg)
		for _, s := range g.Suites {
			e := Entry{Kind: "suite", Name: s.Name, Description: s.Description, Package: pkg, Rejects: s.Rejects}
			if s.Classifier != nil {
				e.Classifier = s.Classifier.Name
			}
			for _, corpus := range s.Corpora {
				columns[corpus] = append(columns[corpus], e)
			}
		}
		for _, l := range g.Lifts {
			if !l.Owned {
				continue
			}
			e := Entry{Kind: "lift", Name: l.Name, Description: l.Description, Package: pkg, Rejects: l.Rejects}
			for _, corpus := range l.Corpora {
				columns[corpus] = append(columns[corpus], e)
			}
		}
		for _, ct := range g.Types {
			if transforms[ct.Dir] == nil {
				transforms[ct.Dir] = map[string]registry.Transform{}
			}
			for _, tr := range ct.Transforms {
				if _, ok := transforms[ct.Dir][tr.Name]; !ok {
					transforms[ct.Dir][tr.Name] = tr
				}
			}
		}
	}
	var cells []Cell
	for _, corpus := range slices.Sorted(maps.Keys(columns)) {
		cols := columns[corpus]
		slices.SortStableFunc(cols, func(a, b Entry) int { return strings.Compare(a.Name, b.Name) })
		dir := filepath.Join(r.corpusRoot, "testdata", corpus)
		entries, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue // a declared directory with no archive yet
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			name, ok := strings.CutSuffix(e.Name(), ".txtar")
			if !ok || e.IsDir() {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if !validName(name) {
				return nil, fmt.Errorf("%s: the archive's name breaks the naming rule", r.rel(p))
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			a := archive.Parse(data)
			if ps := a.Check(); len(ps) > 0 {
				return nil, fmt.Errorf("%s has problems of form:\n%s", r.rel(p), problemList(ps))
			}
			type checkedCase struct {
				name, prefix string
				properties   []string
			}
			stated := a.Properties()
			cases := []checkedCase{{properties: stated}}
			for _, v := range a.Variants() {
				c := checkedCase{name: v.Case(), prefix: v.Prefix(), properties: v.Properties}
				if v.Kind == archive.Transform {
					tr, ok := transforms[corpus][v.Name]
					if !ok {
						return nil, fmt.Errorf("%s holds %s, and no corpus type of testdata/%s/ lists a transform named %s", r.rel(p), v.Case(), corpus, v.Name)
					}
					var neither []string
					if c.properties, neither = registry.VariantProperties(tr.Gives, tr.Keeps, stated); len(neither) > 0 {
						return nil, fmt.Errorf("%s holds %s, and %s declines the base: the base states %s, which %s neither gives nor keeps", r.rel(p), v.Case(), v.Name, andList(neither), v.Name)
					}
				}
				cases = append(cases, c)
			}
			for _, c := range cases {
				for _, col := range cols {
					cell := Cell{Type: corpus, Archive: name, Case: c.name, Column: col.Name, Entry: col, Properties: c.properties, Files: map[string][]byte{}}
					for _, s := range a.Sections {
						if f, ok := strings.CutPrefix(s.Name, c.prefix+archive.Out+"/"+col.Name+"/"); ok {
							cell.Files[f] = s.Data
						}
					}
					if data, ok := cell.Files[archive.OutcomeFile]; ok {
						if cell.Outcome, err = archive.ParseOutcome(data); err != nil {
							return nil, fmt.Errorf("%s, %s%s/%s/%s: %v", r.rel(p), c.prefix, archive.Out, col.Name, archive.OutcomeFile, err)
						}
					}
					cells = append(cells, cell)
				}
			}
		}
	}
	return cells, nil
}

// cloneCells copies cells deeply, so no check sees what another changed.
func cloneCells(cells []Cell) []Cell {
	out := make([]Cell, len(cells))
	for i, c := range cells {
		c.Outcome.Codes = slices.Clone(c.Outcome.Codes)
		c.Entry.Rejects = slices.Clone(c.Entry.Rejects)
		c.Properties = slices.Clone(c.Properties)
		files := make(map[string][]byte, len(c.Files))
		for p, data := range c.Files {
			files[p] = bytes.Clone(data)
		}
		c.Files = files
		out[i] = c
	}
	return out
}
