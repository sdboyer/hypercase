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
	"github.com/sdboyer/hypercase/internal/cells"
	"github.com/sdboyer/hypercase/internal/registry"
	"github.com/sdboyer/hypercase/internal/walk"
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
		// name ends in _test.go, and doesn't start with _ or ., which Go
		// ignores. Any such file counts, even one with no test that runs
		// what the registry records.
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
	suitePkgs := map[string][]string{}              // suite → the packages that own it
	liftPkgs := map[string][]string{}               // lift → the packages that own it
	untestedLifts := map[string][]string{}          // lift → the packages with no _test.go file whose registries own it
	claimed := map[string]map[string]bool{}         // corpus → the columns over it
	products := map[string]map[string]bool{}        // corpus → the bytes lifts whose columns are over it
	usedPkgs := map[string][]string{}               // lift → the packages that only use it
	declared := map[string]bool{}                   // the corpus types' directories, and the suites' Corpus
	typeEntries := map[string]map[string][]string{} // directory → type entry → packages
	typeOrder := map[string][]string{}              // directory → type entries, in the order of the packages
	propEntries := map[string]map[string][]string{} // property → description → packages
	propOrder := map[string][]string{}              // property → descriptions, in the order of the packages
	liftEntries := map[string][]liftEntry{}         // lift → the entries of the registries with tests that own or use it, in the order of the packages
	transforms := map[string]map[string]bool{}      // directory → the transforms its corpus types list
	needOwner := map[string]map[string]bool{}       // directory → the transforms its corpus types list in packages with tests
	trPkgs := map[trKey][]string{}                  // transform → the packages that own it
	untestedTrs := map[trKey][]string{}             // transform → the packages with no _test.go file whose registries own it
	var rootPkgs []string
	claim := func(name string, corpora []string) {
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
		// or transform. Nor does a transform it lists need an owner, nor
		// is its entry of a lift compared with the others', which are
		// compared for what each package's runs do with the lift. Its
		// sections stay claimed, since the registry's own error reports
		// them, and the lifts and transforms it owns are kept apart, so a
		// message that finds no owner names it. What still counts where
		// registries are compared is its root-package check, its corpus
		// types and its properties, the stricter choice: a second
		// root-package check, or a corpus type or property another package
		// declares differently, still fails.
		if g.reg.RootPackage {
			rootPkgs = append(rootPkgs, g.pkg)
		}
		for _, s := range g.reg.Suites {
			claim(s.Name, s.Corpora)
			if g.tested {
				suitePkgs[s.Name] = append(suitePkgs[s.Name], g.pkg)
			}
			if s.Consumes == registry.RawInput {
				for _, dir := range s.Corpora {
					declare(dir, rawEntry, g.pkg)
				}
			}
		}
		for _, l := range g.reg.Lifts {
			if g.tested {
				liftEntries[l.Name] = append(liftEntries[l.Name], liftEntry{g.pkg, l})
			}
			if !l.Owned {
				if g.tested {
					usedPkgs[l.Name] = append(usedPkgs[l.Name], g.pkg)
				}
				continue
			}
			claim(l.Name, l.Corpora)
			if l.Bytes {
				for _, corpus := range l.Corpora {
					if products[corpus] == nil {
						products[corpus] = map[string]bool{}
					}
					products[corpus][l.Name] = true
				}
			}
			if g.tested {
				liftPkgs[l.Name] = append(liftPkgs[l.Name], g.pkg)
			} else {
				untestedLifts[l.Name] = append(untestedLifts[l.Name], g.pkg)
			}
		}
		for _, ct := range g.reg.Types {
			declare(ct.Dir, typeEntry(ct), g.pkg)
			if transforms[ct.Dir] == nil {
				transforms[ct.Dir] = map[string]bool{}
			}
			if g.tested && needOwner[ct.Dir] == nil {
				needOwner[ct.Dir] = map[string]bool{}
			}
			for _, tr := range ct.Transforms {
				transforms[ct.Dir][tr.Name] = true
				if g.tested {
					needOwner[ct.Dir][tr.Name] = true
				}
				if !tr.Owned {
					continue
				}
				k := trKey{ct.Dir, tr.Name}
				if g.tested {
					trPkgs[k] = append(trPkgs[k], g.pkg)
				} else {
					untestedTrs[k] = append(untestedTrs[k], g.pkg)
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
	// Lifts and suites draw their names from one set, the column names,
	// so the duplicate-column check runs over both. A lift that no package
	// owns draws from it too, since owning it would make it a column.
	columns := map[string]bool{}
	for name := range suitePkgs {
		columns[name] = true
	}
	for name := range liftPkgs {
		columns[name] = true
	}
	for _, name := range slices.Sorted(maps.Keys(columns)) {
		if holders, why, fix := duplicateColumn(suitePkgs[name], liftPkgs[name], usedPkgs[name]); holders != "" {
			t.Errorf("the column %s is %s. %s\n%s", name, holders, why, fix)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(usedPkgs)) {
		if len(liftPkgs[name]) > 0 {
			continue
		}
		var untested string
		fix := "Own the lift in one of them: give it to hypercase.Own in place of hypercase.Use."
		if pkgs := untestedLifts[name]; len(pkgs) > 0 {
			why, restore := untestedOwners("lift", pkgs)
			untested = " " + why
			fix = restore + " own the lift in one of the packages that use it: give it to hypercase.Own in place of hypercase.Use."
		}
		// Owning the lift while a suite holds its name would make the
		// column owned twice, so the fix waits on the collision's, which
		// the duplicate-column check reports.
		if len(suitePkgs[name]) > 0 {
			fix = "Once no suite shares the lift's name, " + lowerFirst(fix)
		}
		t.Errorf("the lift %s is used by %s, and no package owns it, so its error is recorded nowhere.%s\n%s", name, andList(usedPkgs[name]), untested, fix)
	}
	// Registries that differ where the packages' code agrees are stale,
	// such as one edited by hand, so each comparison's fix says how the
	// update ends that.
	stale := fmt.Sprintf("If they register one declaration already, a registry is stale: run %s, which rewrites the registry of every package with tests.", r.updateCmd())
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
		fix := "Declare the type once, in a package the others import, and register that declaration. " + stale
		if raw {
			fix = fmt.Sprintf("A suite that names the directory as its Corpus runs over its archives' raw input views, with no type. Make the suite consume the corpus type and remove its Corpus, or move one set of archives to another directory. If the code does one of these already, a registry is stale: run %s, which rewrites the registry of every package with tests.", r.updateCmd())
		}
		t.Errorf("the corpus directory %s has two different corpus types:\n%s\nAn archive's directory gives its type. %s", dir, strings.Join(lines, "\n"), fix)
	}
	for _, dir := range slices.Sorted(maps.Keys(needOwner)) {
		for _, name := range slices.Sorted(maps.Keys(needOwner[dir])) {
			switch pkgs := trPkgs[trKey{dir, name}]; {
			case len(pkgs) == 0:
				var untested string
				fix := "Give it to hypercase.Own in one package that registers its corpus type."
				if u := untestedTrs[trKey{dir, name}]; len(u) > 0 {
					why, restore := untestedOwners("transform", u)
					untested = " " + why
					fix = restore + " give the transform to hypercase.Own in another package that registers its corpus type."
				}
				t.Errorf("the transform %s, of testdata/%s/, is owned by no package, so nothing writes its variants' inputs, or judges them against what it makes.%s\n%s", name, dir, untested, fix)
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
		t.Errorf("the property %s has two different entries:\n%s\nDeclare the property once, in a package the others import, and register that declaration. %s", name, strings.Join(lines, "\n"), stale)
	}
	for _, name := range slices.Sorted(maps.Keys(liftEntries)) {
		var differ, blocks []string
		bytesLift := false
		for _, e := range liftEntries[name] {
			bytesLift = bytesLift || e.lift.Bytes
		}
		for _, f := range liftFields {
			pkgs := map[string][]string{} // value → packages
			var order []string
			for _, e := range liftEntries[name] {
				v := f.value(e.lift)
				if pkgs[v] == nil {
					order = append(order, v)
				}
				pkgs[v] = append(pkgs[v], e.pkg)
			}
			if len(order) < 2 {
				continue
			}
			differ = append(differ, f.name)
			blocks = append(blocks, "  "+f.name+":")
			for _, v := range order {
				blocks = append(blocks, fmt.Sprintf("    %s: %s", andList(pkgs[v]), v))
			}
		}
		if len(differ) == 0 {
			continue
		}
		why := "a package that only uses a lift runs its own declaration of it to stage the cells past it, and the lift's cell, which the package that owns it writes, records what the owner's declaration does"
		if bytesLift {
			why = "a package that only uses a bytes lift tells by its own declaration whether an update writes the lift's cell, and so the cells past it"
		}
		t.Errorf("the registries that own or use the lift %s differ in %s:\n%s\nEvery package that owns or uses a lift registers one declaration of it: %s. Declare the lift once, in a package the others import, and register that declaration. %s", name, andList(differ), strings.Join(blocks, "\n"), why, stale)
	}

	// The archives, and the directories that hold them, as hypercase
	// strip walks them. A corpus root with no testdata/ holds none.
	td := filepath.Join(r.corpusRoot, "testdata")
	archives := map[string][]string{} // corpus → archive file names
	undeclared := map[string]bool{}   // directories, relative to td
	if _, err := os.Lstat(td); !errors.Is(err, fs.ErrNotExist) {
		links, err := walk.Testdata(td, otherRoots, func(p string, d fs.DirEntry) {
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
		})
		if err != nil {
			t.Errorf("reading %s: %v", r.rel(td), err)
		}
		if len(links) > 0 {
			reason, it := walk.LinkReason(td, links, r.rel)
			t.Errorf("%s.\n"+walk.LinkFix+".", reason, it)
		}
	}
	for _, dir := range slices.Sorted(maps.Keys(undeclared)) {
		rel := r.rel(filepath.Join(td, filepath.FromSlash(dir))) + "/"
		if dir == "." {
			t.Errorf("%s holds archives directly, and an archive belongs in a corpus's directory, testdata/<corpus>/.\nMove the archives into one, or delete them.", rel)
			continue
		}
		t.Errorf("%s holds archives, and no registry declares the directory: no corpus type has it as its Dir, and no suite names it as its Corpus.\nDeclare a decl.Type with that Dir, or move or delete the archives.", rel)
	}

	props := func(name string) bool { _, ok := propEntries[name]; return ok }
	// What the registries say of the bytes lifts over each corpus, read as
	// strip reads them, to say which gaps lines keep it from removing a
	// section. And the lifts whose entries in them, owned or used, in a
	// package with tests or not, differ on whether the lift is a bytes
	// lift. The check fails on the registries that make them differ, by
	// the lift's entries, which it compares across the packages with
	// tests, or by a registry's own failure, in a package with none. Their
	// fix may change what strip reads, so the check's advice on which
	// gaps lines keep strip from a lift's via/ waits on them agreeing.
	var parsed []*registry.Registry
	for _, g := range regs {
		parsed = append(parsed, g.reg)
	}
	bytesLifts := registry.BytesLifts(parsed)
	says := map[string]map[bool]bool{} // lift → what its entries say of whether it is a bytes lift
	for _, g := range parsed {
		for _, l := range g.Lifts {
			if says[l.Name] == nil {
				says[l.Name] = map[bool]bool{}
			}
			says[l.Name][l.Bytes] = true
		}
	}
	disputed := map[string]bool{}
	for name, s := range says {
		disputed[name] = len(s) > 1
	}
	for _, corpus := range slices.Sorted(maps.Keys(archives)) {
		cols := claimed[corpus]
		for _, file := range archives[corpus] {
			r.checkArchive(t, filepath.Join(td, corpus, file), corpus, cols, products[corpus], bytesLifts[corpus], disputed, transforms[corpus], props)
		}
	}
}

// duplicateColumn words the failure of a name that two or more columns
// hold: who holds it, why that fails, and the fix, which rootCheck's
// message gives in turn. It returns "" for each when fewer hold it. suites
// are the packages that own the name as a suite, lifts those that own it
// as a lift, and users those that use the lift. Each suite's owner counts
// as one column, and each lift's owner as one. When no package owns the
// lift, its users together count as one column, the one owning the lift
// would make; when one does, they count for nothing. The fix leaves one
// column of the name. Only a lift's owners are offered hypercase.Use, and
// only when more than one owns the lift, since a lift that one package
// owns has an owner it needs.
func duplicateColumn(suites, lifts, users []string) (holders, why, fix string) {
	if len(lifts) > 0 {
		users = nil
	}
	n := len(suites) + len(lifts)
	if len(users) > 0 {
		n++
	}
	if n < 2 {
		return "", "", ""
	}
	// A lift owner that only reads the lift's product removes its column
	// with hypercase.Use, so that is how the fix says to remove one.
	useHint := ""
	if len(lifts) > 1 {
		useHint = fmt.Sprintf(" Of %s, which own the lift, one that only reads its product removes its column by giving the lift to hypercase.Use in place of hypercase.Own.", andList(lifts))
	}
	switch {
	case len(lifts) == 0 && len(users) == 0:
		holders = "owned by " + pkgsAs(suites, "suite")
		why = "Column names are unique per corpus root, and one package owns each column."
		fix = leaveOne(n, "one of the suites", "the suites")
	case len(suites) == 0:
		holders = "owned by " + pkgsAs(lifts, "lift")
		why = "Column names are unique per corpus root, and one package owns each column."
		fix = leaveOne(n, "one of the lifts", "the lifts") + useHint
	case len(lifts) > 0:
		holders = "owned by " + pkgsAs(suites, "suite") + ", and by " + pkgsAs(lifts, "lift")
		why = "Lifts share the suites' column names, which are unique per corpus root, and one package owns each column."
		fix = leaveOne(n, "the suite or the lift", "the columns") + useHint
	default:
		holders = "owned by " + pkgsAs(suites, "suite") + ", and used by " + pkgsAs(users, "lift")
		why = "Lifts share the suites' column names, which are unique per corpus root. No package owns the lift, and owning it would add a column of the name."
		fix = leaveOne(n, "the suite or the lift", "the suites and the lift") + " To remove the lift, drop it from hypercase.Use in every package that uses it."
	}
	return holders, why, fix
}

// leaveOne words the fix that leaves one of n columns of a name: of two,
// rename or remove one, which two names; of more, every one but one, of
// those many names.
func leaveOne(n int, two, many string) string {
	if n == 2 {
		return "Rename " + two + ", or remove one."
	}
	return "Rename or remove all but one of " + many + "."
}

// pkgsAs names packages, and what each holds a column's name as: "./b as
// a suite", ". and ./b, each as a lift".
func pkgsAs(pkgs []string, kind string) string {
	if len(pkgs) == 1 {
		return pkgs[0] + " as a " + kind
	}
	return andList(pkgs) + ", each as a " + kind
}

// lowerFirst lowers a sentence's first letter, so another can lead it.
func lowerFirst(s string) string { return strings.ToLower(s[:1]) + s[1:] }

// trKey names a transform by its corpus directory and its name.
type trKey struct{ dir, name string }

// A liftEntry is one registry's entry of a lift, and the registry's
// package.
type liftEntry struct {
	pkg  string
	lift registry.Lift
}

// liftFields are the fields of a lift's registry entry that every registry
// that owns or uses the lift records alike, each by its name in the
// registry and as the root-package check shows it. Whether a package owns
// the lift differs between them, and so may the corpora that reach it.
var liftFields = []struct {
	name  string
	value func(registry.Lift) string
}{
	{"description", func(l registry.Lift) string { return fmt.Sprintf("%q", l.Description) }},
	{"input", func(l registry.Lift) string { return l.Input }},
	{"product", func(l registry.Lift) string { return l.Product }},
	{"bytes", func(l registry.Lift) string { return fmt.Sprint(l.Bytes) }},
	{"outputs", func(l registry.Lift) string { return outputsEntry(l.Outputs) }},
	{"productFiles", func(l registry.Lift) string { return outputsEntry(l.ProductFiles) }},
	{"rejects", func(l registry.Lift) string {
		if len(l.Rejects) == 0 {
			return "none"
		}
		return strings.Join(l.Rejects, ", ")
	}},
}

// outputsEntry shows a registry's output declarations: each pattern, with
// its description.
func outputsEntry(outs []registry.Output) string {
	if len(outs) == 0 {
		return "none"
	}
	var es []string
	for _, o := range outs {
		es = append(es, fmt.Sprintf("%s %q", o.Pattern, o.Description))
	}
	return strings.Join(es, ", ")
}

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
// the columns over its corpus, products the bytes lifts among them that a
// registry owns, in a package with tests or not, bytesLifts what the
// registries say of the columns over it as strip reads them (see
// [registry.BytesLifts]), disputed the lifts whose entries in the
// registries differ on whether they are bytes lifts, and transforms its
// corpus type's.
func (r *runner) checkArchive(t *testing.T, p, corpus string, cols, products, bytesLifts, disputed, transforms map[string]bool, props func(string) bool) {
	t.Helper()
	rel := r.rel(p)
	// An archive hypercase strip refuses by its path runs no cell, and
	// strip strips none of its sections, so the check reports that alone.
	if e := walk.ArchivePath(filepath.ToSlash(mustRel(r.corpusRoot, p))); e != nil {
		t.Errorf("%s is no archive a run reads: %s.\n%s.", rel, e.Reason, e.Fix)
		return
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
			switch {
			case g.Transform != "" && !transforms[g.Transform]:
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

	// Sections no registry claims: a column's outputs and a bytes lift's
	// product, in any case, and a transform's variant, whole, when no
	// corpus type lists the transform. A claim's why says why no registry
	// claims them, and its strip is the flag that limits hypercase strip
	// to them, or "" where strip would refuse the flag; whose names them.
	// A claim of a bytes lift's product names the lift.
	type claim struct{ what, why, strip, whose, lift string }
	unclaimed := map[claim][]string{}
	// columnClaim builds a claim of the sections of the column col, of
	// which fact says why no registry claims them, and register what to
	// do if one should. A name that breaks the naming rule, by
	// archive.ValidName, no column or lift can have, and hypercase strip
	// refuses it as -column, so such a claim has no register and no strip.
	columnClaim := func(what, fact, register, col, whose, lift string) claim {
		if !archive.ValidName(col) {
			return claim{what, fact + ", and none can, since the name breaks the naming rule: " + nameRule, "", whose, lift}
		}
		return claim{what, fact + ".\n" + register, "-column " + col, whose, lift}
	}
	// The gaps lines that keep strip from removing a claim's sections, as
	// strip reads the archive's cells, knowing what the registries say of
	// the columns over the corpus.
	kept := map[claim][]string{}
	marks := cells.Marks(a)
	keep := func(c claim, section string) {
		if l, ok := stripKeeps(marks, section, bytesLifts); ok && !slices.Contains(kept[c], l) {
			kept[c] = append(kept[c], l)
		}
	}
	for _, s := range a.Sections {
		kind, name, rest := archive.SplitCase(s.Name)
		if kind == archive.Transform && !transforms[name] {
			// archive.Check refuses a transform's name that breaks the
			// naming rule, so strip takes this one as -transform.
			c := claim{archive.Transform + "/" + name, fmt.Sprintf("no corpus type of testdata/%s/ lists a transform named %s.\nIf a transform should make the variant, list it in its corpus type's Transforms", corpus, name), "-transform " + name, "the sections under transform/" + name + "/", ""}
			unclaimed[c] = append(unclaimed[c], s.Name)
			keep(c, s.Name)
			continue
		}
		if out, ok := strings.CutPrefix(rest, archive.Out+"/"); ok {
			if col, _, _ := strings.Cut(out, "/"); !cols[col] {
				c := columnClaim(col, fmt.Sprintf("no column named %s runs over the corpus %s", col, corpus), "If a column should write them, register the column", col, col+"'s sections", "")
				unclaimed[c] = append(unclaimed[c], s.Name)
				keep(c, s.Name)
			}
		}
		if via, ok := strings.CutPrefix(rest, archive.Via+"/"); ok {
			if l, _, _ := strings.Cut(via, "/"); !products[l] {
				c := columnClaim(archive.Via+"/"+l, fmt.Sprintf("no bytes lift named %s runs over the corpus %s", l, corpus), "If a bytes lift should write them, register it", l, l+"'s sections", l)
				unclaimed[c] = append(unclaimed[c], s.Name)
				keep(c, s.Name)
			}
		}
	}
	claims := make([]claim, 0, len(unclaimed))
	for c := range unclaimed {
		claims = append(claims, c)
	}
	slices.SortFunc(claims, func(a, b claim) int { return strings.Compare(a.what, b.what) })
	for _, c := range claims {
		if c.strip == "" {
			t.Errorf("%s: no registry claims %s: %s.\n%s leaves them in place. Delete them by hand, or run %s at the module root: with no flags, it removes every section an update writes, from every archive under %s but a frozen cell's, and the registries and findings files of the root, and lists what it leaves. Then run %s until it changes nothing, and read what it reports.", rel, andList(unclaimed[c]), c.why, r.updateCmd(), r.stripCmd(""), r.rootName(), r.updateCmd())
			continue
		}
		// Whether a gaps line keeps strip from removing a lift's product
		// turns on whether the lift is a bytes lift, so while the
		// registries differ on that, the advice waits on them.
		if ls := kept[c]; len(ls) > 0 && disputed[c.lift] {
			keeps := fmt.Sprintf("%s, which marks a cell of theirs, keeps", ls[0])
			if len(ls) > 1 {
				keeps = fmt.Sprintf("%s, which mark cells of theirs, keep", andList(ls))
			}
			t.Errorf("%s: no registry claims %s: %s. If not, first settle whether %s is a bytes lift, on which the registries differ, as the check's failures on them say, then run the check again: until they agree, it can't tell whether %s strip from removing them.", rel, andList(unclaimed[c]), c.why, c.lift, keeps)
			continue
		}
		first := ""
		if ls := kept[c]; len(ls) == 1 {
			first = fmt.Sprintf("remove %s, which marks a cell of theirs, so that strip removes them; then ", ls[0])
		} else if len(ls) > 1 {
			first = fmt.Sprintf("remove %s, which mark cells of theirs, so that strip removes them; then ", andList(ls))
		}
		t.Errorf("%s: no registry claims %s: %s. If not, %srun %s at the module root. It removes %s from every archive under %s, but a frozen cell's, and lists what it leaves. Then run %s until it changes nothing, and read what it reports.", rel, andList(unclaimed[c]), c.why, first, r.stripCmd(c.strip), c.whose, r.rootName(), r.updateCmd())
	}
}

// stripKeeps returns the gaps line that keeps hypercase strip, run without
// -frozen, from removing an archive's section, as strip reads the
// archive's cells, and whether one does: the line that marks the cell the
// section is part of. marks are the cells the archive's gaps lines mark,
// and bytesLifts says which columns over its corpus are bytes lifts.
func stripKeeps(marks map[cells.Cell]cells.Mark, section string, bytesLifts map[string]bool) (string, bool) {
	c, ok := cells.Of(section)
	if !ok || !cells.Part(section, bytesLifts) {
		return "", false
	}
	m, ok := marks[c]
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%s line %d", m.Gaps, m.Line), true
}

// pkgName names a package by its directory, as go test does: ., ./format.
func (r *runner) pkgName(dir string) string {
	if rel := r.rel(dir); rel != "." {
		return "./" + rel
	}
	return "."
}

// hasTestFile reports whether dir holds a file whose name ends in
// _test.go, and doesn't start with _ or ., the names Go ignores. It reads
// no build constraints, so a file they exclude still counts.
func hasTestFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasSuffix(name, "_test.go") && !strings.HasPrefix(name, "_") && !strings.HasPrefix(name, ".") {
			return true
		}
	}
	return false
}

func mustRel(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return target
	}
	return rel
}
