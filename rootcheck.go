package hypercase

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/registry"
)

// rootCheck is the root-package check: it reads every registry over the
// corpus root and every archive in the directories they declare, and
// reports what no column owns, and the problems of an archive as a whole.
// It reads only files on disk and runs nothing, so its result doesn't
// depend on which packages ran, or in what order.
func (r *runner) rootCheck(t *testing.T, found []registry.Found) {
	t.Helper()
	type pkgReg struct {
		pkg string
		reg *registry.Registry
	}
	var regs []pkgReg
	otherRoots := map[string]bool{}
	for _, f := range found {
		rel := r.rel(f.Path)
		data, err := os.ReadFile(f.Path)
		if err != nil {
			t.Errorf("reading %s: %v", rel, err)
			continue
		}
		// Another corpus root's registry is left to that root's check,
		// whatever its format.
		if h, err := registry.Peek(data); err == nil && h.CorpusRoot != r.rootRel {
			otherRoots[filepath.Join(r.moduleRoot, filepath.FromSlash(h.CorpusRoot))] = true
			continue
		}
		g, err := registry.Parse(data)
		if err != nil {
			t.Errorf("%s can't be read: %v.\nRun %s to write it.", rel, err, r.updateCmd())
			continue
		}
		regs = append(regs, pkgReg{pkg: r.pkgName(f.Pkg), reg: g})
		if !hasTestFile(f.Pkg) {
			t.Errorf("%s is a registry, and %s holds no _test.go file, so no test compares it, and it still claims sections.\nDelete it, or restore the package's tests.", rel, r.rel(f.Pkg))
		}
	}

	// Columns, lifts, types, properties and root-package checks, merged
	// across packages.
	columnPkgs := map[string][]string{}             // column → the packages that own it
	claimed := map[string]map[string]bool{}         // corpus → the columns over it
	usedPkgs := map[string][]string{}               // lift → the packages that only use it
	declared := map[string]bool{}                   // the corpus types' directories, and the suites' Corpus
	typeEntries := map[string]map[string][]string{} // directory → type entry → packages
	typeOrder := map[string][]string{}              // directory → type entries, in the order of the packages
	propEntries := map[string]map[string][]string{} // property → description → packages
	propOrder := map[string][]string{}              // property → descriptions, in the order of the packages
	var rootPkgs []string
	claim := func(name string, corpora []string, pkg string) {
		columnPkgs[name] = append(columnPkgs[name], pkg)
		for _, corpus := range corpora {
			if claimed[corpus] == nil {
				claimed[corpus] = map[string]bool{}
			}
			claimed[corpus][name] = true
		}
	}
	declare := func(dir, entry, pkg string) {
		declared[dir] = true
		if typeEntries[dir] == nil {
			typeEntries[dir] = map[string][]string{}
		}
		if typeEntries[dir][entry] == nil {
			typeOrder[dir] = append(typeOrder[dir], entry)
		}
		if pkgs := typeEntries[dir][entry]; len(pkgs) == 0 || pkgs[len(pkgs)-1] != pkg {
			typeEntries[dir][entry] = append(pkgs, pkg)
		}
	}
	for _, g := range regs {
		if g.reg.RootPackage {
			rootPkgs = append(rootPkgs, g.pkg)
		}
		for _, s := range g.reg.Suites {
			claim(s.Name, s.Corpora, g.pkg)
			if s.Consumes == registry.RawInput {
				for _, dir := range s.Corpora {
					declare(dir, rawEntry, g.pkg)
				}
			}
		}
		for _, l := range g.reg.Lifts {
			if l.Owned {
				claim(l.Name, l.Corpora, g.pkg)
			} else {
				usedPkgs[l.Name] = append(usedPkgs[l.Name], g.pkg)
			}
		}
		for _, ct := range g.reg.Types {
			declare(ct.Dir, fmt.Sprintf("%s: %q", ct.GoType, ct.Description), g.pkg)
		}
		for _, p := range g.reg.Properties {
			if propEntries[p.Name] == nil {
				propEntries[p.Name] = map[string][]string{}
			}
			if propEntries[p.Name][p.Description] == nil {
				propOrder[p.Name] = append(propOrder[p.Name], p.Description)
			}
			propEntries[p.Name][p.Description] = append(propEntries[p.Name][p.Description], g.pkg)
		}
	}
	if len(rootPkgs) > 1 {
		t.Errorf("%s register the root-package check for %s. It runs once per corpus root.\nRegister it in one package.", andList(rootPkgs), r.rootName())
	}
	for _, name := range sortedKeys(columnPkgs) {
		if pkgs := columnPkgs[name]; len(pkgs) > 1 {
			t.Errorf("the column %s is owned by %s. Column names are unique per corpus root, and one package owns each column.\nRename one of the columns, or remove one. A package that only reads a lift's product gives the lift to hypercase.Use.", name, andList(pkgs))
		}
	}
	for _, name := range sortedKeys(usedPkgs) {
		if _, owned := columnPkgs[name]; !owned {
			t.Errorf("the lift %s is used by %s, and no package owns it, so its error is recorded nowhere.\nOwn the lift in one of them: give it to hypercase.Own in place of hypercase.Use.", name, andList(usedPkgs[name]))
		}
	}
	for _, dir := range sortedKeys(typeEntries) {
		entries := typeEntries[dir]
		if len(entries) < 2 {
			continue
		}
		var lines []string
		raw := false
		for _, e := range typeOrder[dir] {
			lines = append(lines, fmt.Sprintf("  %s: %s", andList(entries[e]), e))
			raw = raw || e == rawEntry
		}
		fix := "Declare the type once, in a package the others import, and register that declaration."
		if raw {
			fix = "A suite that names the directory as its Corpus runs over its archives' raw input views, with no type. Make the suite consume the corpus type and remove its Corpus, or move one set of archives to another directory."
		}
		t.Errorf("the corpus directory %s has two different corpus types:\n%s\nAn archive's directory gives its type. %s", dir, strings.Join(lines, "\n"), fix)
	}
	for _, name := range sortedKeys(propEntries) {
		entries := propEntries[name]
		if len(entries) < 2 {
			continue
		}
		var lines []string
		for _, d := range propOrder[name] {
			lines = append(lines, fmt.Sprintf("  %s: %q", andList(entries[d]), d))
		}
		t.Errorf("the property %s has two different entries:\n%s\nDeclare the property once, in a package the others import, and register that declaration.", name, strings.Join(lines, "\n"))
	}

	// The archives, and the directories that hold them.
	td := filepath.Join(r.corpusRoot, "testdata")
	archives := map[string][]string{} // corpus → archive file names
	undeclared := map[string]bool{}   // directories, relative to td
	err := filepath.WalkDir(td, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == td && errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			if p != td && (otherRoots[p] || isFile(filepath.Join(p, "go.mod"))) {
				return filepath.SkipDir
			}
			return nil
		}
		dir := filepath.ToSlash(mustRel(td, filepath.Dir(p)))
		isCorpus := declared[dir]
		switch {
		case archive.IsTemp(d.Name()) && isCorpus:
			t.Errorf("%s is a temp file left by a write that was cut short.\nDelete it.", r.rel(p))
		case strings.HasSuffix(d.Name(), ".txtar") && isCorpus:
			archives[dir] = append(archives[dir], d.Name())
		case strings.HasSuffix(d.Name(), ".txtar"):
			undeclared[dir] = true
		}
		return nil
	})
	if err != nil {
		t.Errorf("reading %s: %v", r.rel(td), err)
	}
	for _, dir := range sortedKeys(undeclared) {
		rel := r.rel(filepath.Join(td, filepath.FromSlash(dir))) + "/"
		if dir == "." {
			t.Errorf("%s holds archives directly, and an archive belongs in a corpus's directory, testdata/<corpus>/.\nMove the archives into one.", rel)
			continue
		}
		t.Errorf("%s holds archives, and no registry declares the directory: no corpus type has it as its Dir, and no suite names it as its Corpus.\nDeclare a decl.Type with that Dir, or move or delete the archives.", rel)
	}

	props := func(name string) bool { _, ok := propEntries[name]; return ok }
	for _, corpus := range sortedKeys(archives) {
		cols := claimed[corpus]
		for _, file := range archives[corpus] {
			r.checkArchive(t, filepath.Join(td, corpus, file), corpus, cols, props)
		}
	}
}

// rawEntry is how the root-package check names the raw input view, which
// a suite that names a Corpus consumes, as a directory's type.
const rawEntry = "the raw input view, fs.FS, which a suite's Corpus names"

// checkArchive reports the problems of one archive as a whole.
func (r *runner) checkArchive(t *testing.T, p, corpus string, cols map[string]bool, props func(string) bool) {
	t.Helper()
	rel := r.rel(p)
	if name := strings.TrimSuffix(filepath.Base(p), ".txtar"); !validName(name) {
		t.Errorf("%s: the archive's name, %q, breaks the naming rule: %s.\nRename the archive.", rel, name, nameRule)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Errorf("reading %s: %v", rel, err)
		return
	}
	a := archive.Parse(data)
	if ps := a.Check(); len(ps) > 0 {
		t.Errorf("%s has problems of form, so none of its cells run, and no update writes it:\n%s\nFix the archive by hand.", rel, problemList(ps))
		return
	}
	if ps := a.Description(); len(ps) > 0 {
		t.Errorf("%s: %s\nWrite the case's description, in prose, as the archive's comment, the text before its first section.", rel, problemLines(ps))
	}
	if !archive.Placed(data) {
		t.Errorf("%s has update-written sections out of their place.\nRun %s to put them in place.", rel, r.updateCmd())
	}
	for _, name := range a.Properties() {
		if !props(name) {
			t.Errorf("%s: properties states %s, and no registry under %s lists a property of that name.\nDeclare the property, or correct the line.", rel, name, r.rootName())
		}
	}
	for _, g := range a.Gaps() {
		if !cols[g.Column] {
			t.Errorf("%s: gaps, line %d, names %s, and no registry has a column of that name over the corpus %s.\nRegister the column, or remove the line.", rel, g.Line, g.Column, corpus)
		}
	}
	unclaimed := map[string][]string{} // column → its sections
	for _, s := range a.Sections {
		if rest, ok := strings.CutPrefix(s.Name, archive.Out+"/"); ok {
			if col, _, _ := strings.Cut(rest, "/"); !cols[col] {
				unclaimed[col] = append(unclaimed[col], s.Name)
			}
		}
	}
	for _, col := range sortedKeys(unclaimed) {
		t.Errorf("%s: no registry claims %s: no column named %s runs over the corpus %s.\nIf a column should write them, register the column. If not, delete them by hand; %s leaves them in place.", rel, andList(unclaimed[col]), col, corpus, r.updateCmd())
	}
}

// pkgName names a package by its directory, as go test does: ., ./format.
func (r *runner) pkgName(dir string) string {
	if rel := r.rel(dir); rel != "." {
		return "./" + rel
	}
	return "."
}

func hasTestFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), "_test.go") {
			return true
		}
	}
	return false
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

func mustRel(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return target
	}
	return rel
}
