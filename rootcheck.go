package hypercase

import (
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
		// tested records that the package's directory holds a file whose
		// name ends in _test.go. Any such file counts, even one with no
		// test that runs what the registry records.
		tested bool
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
		tested := hasTestFile(f.Pkg)
		regs = append(regs, pkgReg{pkg: r.pkgName(f.Pkg), reg: g, tested: tested})
		if !tested {
			t.Errorf("%s is a registry, and %s holds no _test.go file, so no test compares it, and it still claims sections.\nDelete it, or restore the package's tests.", rel, r.rel(f.Pkg))
		}
	}

	// Columns, lifts, types, properties and root-package checks, merged
	// across packages.
	columnPkgs := map[string][]string{}             // column → the packages that own it
	untestedCols := map[string][]string{}           // column → the packages with no _test.go file whose registries own it
	claimed := map[string]map[string]bool{}         // corpus → the columns over it
	usedPkgs := map[string][]string{}               // lift → the packages that only use it
	declared := map[string]bool{}                   // the corpus types' directories, and the suites' Corpus
	typeEntries := map[string]map[string][]string{} // directory → type entry → packages
	typeOrder := map[string][]string{}              // directory → type entries, in the order of the packages
	propEntries := map[string]map[string][]string{} // property → description → packages
	propOrder := map[string][]string{}              // property → descriptions, in the order of the packages
	transforms := map[string]map[string]bool{}      // directory → the transforms its corpus types list
	trPkgs := map[trKey][]string{}                  // transform → the packages that own it
	untestedTrs := map[trKey][]string{}             // transform → the packages with no _test.go file whose registries own it
	var rootPkgs []string
	claim := func(owners map[string][]string, name string, corpora []string, pkg string) {
		owners[name] = append(owners[name], pkg)
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
		// A registry in a package with no _test.go file records what no
		// test runs, so its package owns and uses nothing: no column, lift
		// or transform. Its sections stay claimed, since the registry's own
		// error reports them, and what it owns is kept apart, so a message
		// that finds no owner names it.
		owners, users, trOwners := columnPkgs, usedPkgs, trPkgs
		if !g.tested {
			owners, users, trOwners = untestedCols, map[string][]string{}, untestedTrs
		}
		if g.reg.RootPackage {
			rootPkgs = append(rootPkgs, g.pkg)
		}
		for _, s := range g.reg.Suites {
			claim(owners, s.Name, s.Corpora, g.pkg)
			if s.Consumes == registry.RawInput {
				for _, dir := range s.Corpora {
					declare(dir, rawEntry, g.pkg)
				}
			}
		}
		for _, l := range g.reg.Lifts {
			if l.Owned {
				claim(owners, l.Name, l.Corpora, g.pkg)
			} else {
				users[l.Name] = append(users[l.Name], g.pkg)
			}
		}
		for _, ct := range g.reg.Types {
			declare(ct.Dir, typeEntry(ct), g.pkg)
			if transforms[ct.Dir] == nil {
				transforms[ct.Dir] = map[string]bool{}
			}
			for _, tr := range ct.Transforms {
				transforms[ct.Dir][tr.Name] = true
				if tr.Owned {
					k := trKey{ct.Dir, tr.Name}
					trOwners[k] = append(trOwners[k], g.pkg)
				}
			}
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
	for _, name := range slices.Sorted(maps.Keys(columnPkgs)) {
		if pkgs := columnPkgs[name]; len(pkgs) > 1 {
			t.Errorf("the column %s is owned by %s. Column names are unique per corpus root, and one package owns each column.\nRename one of the columns, or remove one. A package that only reads a lift's product gives the lift to hypercase.Use.", name, andList(pkgs))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(usedPkgs)) {
		if _, owned := columnPkgs[name]; owned {
			continue
		}
		msg := fmt.Sprintf("the lift %s is used by %s, and no package owns it, so its error is recorded nowhere.", name, andList(usedPkgs[name]))
		fix := "Own the lift in one of them: give it to hypercase.Own in place of hypercase.Use."
		if pkgs := untestedCols[name]; len(pkgs) > 0 {
			why, restore := untestedOwners("lift", pkgs)
			msg += " " + why
			fix = restore + " own the lift in one of the packages that use it: give it to hypercase.Own in place of hypercase.Use."
		}
		t.Errorf("%s\n%s", msg, fix)
	}
	for _, dir := range slices.Sorted(maps.Keys(typeEntries)) {
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
	for _, dir := range slices.Sorted(maps.Keys(transforms)) {
		for _, name := range slices.Sorted(maps.Keys(transforms[dir])) {
			switch pkgs := trPkgs[trKey{dir, name}]; {
			case len(pkgs) == 0:
				msg := fmt.Sprintf("the transform %s, of testdata/%s/, is owned by no package, so nothing writes its variants' inputs, or judges them against what it makes.", name, dir)
				fix := "Give it to hypercase.Own in one package that registers its corpus type."
				if u := untestedTrs[trKey{dir, name}]; len(u) > 0 {
					why, restore := untestedOwners("transform", u)
					msg += " " + why
					fix = restore + " give the transform to hypercase.Own in another package that registers its corpus type."
				}
				t.Errorf("%s\n%s", msg, fix)
			case len(pkgs) > 1:
				t.Errorf("the transform %s, of testdata/%s/, is owned by %s. One package owns each transform.\nOwn it in one of them.", name, dir, andList(pkgs))
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(propEntries)) {
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
	for _, dir := range slices.Sorted(maps.Keys(undeclared)) {
		rel := r.rel(filepath.Join(td, filepath.FromSlash(dir))) + "/"
		if dir == "." {
			t.Errorf("%s holds archives directly, and an archive belongs in a corpus's directory, testdata/<corpus>/.\nMove the archives into one.", rel)
			continue
		}
		t.Errorf("%s holds archives, and no registry declares the directory: no corpus type has it as its Dir, and no suite names it as its Corpus.\nDeclare a decl.Type with that Dir, or move or delete the archives.", rel)
	}

	props := func(name string) bool { _, ok := propEntries[name]; return ok }
	for _, corpus := range slices.Sorted(maps.Keys(archives)) {
		cols := claimed[corpus]
		for _, file := range archives[corpus] {
			r.checkArchive(t, filepath.Join(td, corpus, file), corpus, cols, transforms[corpus], props)
		}
	}
}

// trKey names a transform by its corpus directory and its name.
type trKey struct{ dir, name string }

// untestedOwners words what the root-package check knows of a lift or
// transform that no package owns, and that the registries of pkgs, which
// hold no _test.go file, record their packages own: why no test runs it,
// and the first half of the fix, which the caller ends.
func untestedOwners(what string, pkgs []string) (why, restore string) {
	if len(pkgs) == 1 {
		return fmt.Sprintf("The registry of %s records that its package owns the %s, and %s holds no _test.go file, so no test runs the %s there.", pkgs[0], what, pkgs[0], what),
			fmt.Sprintf("Restore the tests of %s, or delete its registry and", pkgs[0])
	}
	return fmt.Sprintf("The registries of %s record that their packages own the %s, and they hold no _test.go file, so no test runs the %s there.", andList(pkgs), what, what),
		"Restore the tests of one of them and delete the others' registries, or delete all their registries and"
}

// rawEntry is how the root-package check names the raw input view, which
// a suite that names a Corpus consumes, as a directory's type.
const rawEntry = "the raw input view, fs.FS, which a suite's Corpus names"

// typeEntry is how the root-package check names a corpus type: its Go type
// and description, and its transforms, which every package that registers
// the type lists alike.
func typeEntry(ct registry.Type) string {
	e := fmt.Sprintf("%s: %q", ct.GoType, ct.Description)
	for _, tr := range ct.Transforms {
		e += fmt.Sprintf("; the transform %s: %q, gives [%s], keeps [%s]", tr.Name, tr.Description, strings.Join(tr.Gives, " "), strings.Join(tr.Keeps, " "))
		if tr.Probe {
			e += ", with a probe"
		}
	}
	return e
}

// checkArchive reports the problems of one archive as a whole. cols are
// the columns over its corpus, and transforms its corpus type's.
func (r *runner) checkArchive(t *testing.T, p, corpus string, cols map[string]bool, transforms map[string]bool, props func(string) bool) {
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
	if ps := a.VariantDescriptions(); len(ps) > 0 {
		t.Errorf("%s: %s\nWrite each variant's description, in prose, in its about section.", rel, problemLines(ps))
	}
	if !archive.Placed(data) {
		t.Errorf("%s has update-written sections out of their place.\nRun %s to put them in place.", rel, r.updateCmd())
	}
	stated := func(section string, names []string) {
		for _, name := range names {
			if !props(name) {
				t.Errorf("%s: %s states %s, and no registry under %s lists a property of that name.\nDeclare the property, or correct the line.", rel, section, name, r.rootName())
			}
		}
	}
	marked := func(section string, gaps []archive.Gap) {
		for _, g := range gaps {
			_, listed := transforms[g.Transform]
			switch {
			case g.Transform != "" && !listed:
				t.Errorf("%s: %s, line %d, names transform/%s, and no corpus type of testdata/%s/ lists a transform of that name.\nList the transform, or remove the line.", rel, section, g.Line, g.Transform, corpus)
			case !cols[g.Column]:
				t.Errorf("%s: %s, line %d, names %s, and no registry has a column of that name over the corpus %s.\nRegister the column, or remove the line.", rel, section, g.Line, g.Column, corpus)
			}
		}
	}
	stated(archive.Properties, a.Properties())
	marked(archive.Gaps, a.Gaps())
	for _, v := range a.Variants() {
		stated(v.Prefix()+archive.Properties, v.Properties)
		marked(v.Prefix()+archive.Gaps, v.Gaps)
	}

	// Sections no registry claims: a column's outputs, in any case, and a
	// transform's variant, whole, when no corpus type lists the transform.
	type claim struct{ what, why string }
	unclaimed := map[claim][]string{}
	for _, s := range a.Sections {
		kind, name, rest := archive.SplitCase(s.Name)
		if _, listed := transforms[name]; kind == archive.Transform && !listed {
			c := claim{archive.Transform + "/" + name, fmt.Sprintf("no corpus type of testdata/%s/ lists a transform named %s.\nIf a transform should make the variant, list it in its corpus type's Transforms", corpus, name)}
			unclaimed[c] = append(unclaimed[c], s.Name)
			continue
		}
		if out, ok := strings.CutPrefix(rest, archive.Out+"/"); ok {
			if col, _, _ := strings.Cut(out, "/"); !cols[col] {
				c := claim{col, fmt.Sprintf("no column named %s runs over the corpus %s.\nIf a column should write them, register the column", col, corpus)}
				unclaimed[c] = append(unclaimed[c], s.Name)
			}
		}
	}
	claims := make([]claim, 0, len(unclaimed))
	for c := range unclaimed {
		claims = append(claims, c)
	}
	slices.SortFunc(claims, func(a, b claim) int { return strings.Compare(a.what, b.what) })
	for _, c := range claims {
		t.Errorf("%s: no registry claims %s: %s. If not, delete them by hand; %s leaves them in place.", rel, andList(unclaimed[c]), c.why, r.updateCmd())
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
