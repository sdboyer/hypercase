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
	testedRegs := map[string]*registry.Registry{}   // package → its registry, for the packages with tests
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
		if g.tested {
			testedRegs[g.pkg] = g.reg
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
	r.usedPaths(t, testedRegs, liftEntries, liftPkgs)

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
			r.checkArchive(t, filepath.Join(td, corpus, file), corpus, cols, products[corpus], transforms[corpus], props)
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

// route shows a lift's path from a corpus, from the corpus to the lift:
// testdata/export/ → import → parse. lifts run from the corpus.
func route(corpus string, lifts []string, lift string) string {
	s := "testdata/" + corpus + "/"
	for _, l := range lifts {
		s += " → " + l
	}
	return s + " → " + lift
}

// usedPaths fails where a package that uses a lift reaches it from a
// corpus otherwise than the lift's owner does, so that the lift's cells
// the owner writes are the ones the user's cells past it are built
// through, as a corpus check reads them on a cell's Path. A lift with no
// owner, or two, has no one path to compare with, and rootCheck fails on
// it otherwise. regs are the registries of the packages with tests, by
// package, entries each lift's entries in them, and owners each lift's
// owners among them.
//
// It fails once for each divergence: a user, a corpus, and where the
// user's path from the corpus leaves the owner's, or that the owner
// doesn't run the lift over the corpus at all. The lifts past one
// divergence that the user uses share it, and the failure names them, and
// the paths to the nearest.
func (r *runner) usedPaths(t *testing.T, regs map[string]*registry.Registry, entries map[string][]liftEntry, owners map[string][]string) {
	t.Helper()
	// A divergence is where a user's path from a corpus leaves its owner's:
	// from the corpus, both take the lifts of prefix, then the user takes
	// mine and the owner theirs, and both go on alike. missing marks an
	// owner that doesn't run the lifts over the corpus.
	type divergence struct {
		user, owner, corpus  string
		missing              bool
		prefix, mine, theirs string // lift names, joined by spaces
	}
	type used struct {
		lift                  string
		route, theirs         []string        // the user's and the owner's lifts, from the corpus
		userPaths, ownerPaths []registry.Path // the user's and the owner's entries' paths
	}
	groups := map[divergence][]used{}
	var order []divergence
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		var owner *liftEntry
		var users []liftEntry
		for _, e := range entries[name] {
			if !e.lift.Owned {
				users = append(users, e)
			} else if len(owners[name]) == 1 {
				owner = &e
			}
		}
		if owner == nil {
			continue
		}
		owned := map[string][]string{} // corpus → the owner's lifts from it
		for _, p := range owner.lift.Paths {
			owned[p.Corpus] = reversed(p.Lifts)
		}
		for _, u := range users {
			for _, p := range u.lift.Paths {
				mine := reversed(p.Lifts)
				theirs, ok := owned[p.Corpus]
				if ok && slices.Equal(mine, theirs) {
					continue
				}
				d := divergence{user: u.pkg, owner: owner.pkg, corpus: p.Corpus, missing: !ok}
				if ok {
					i := 0
					for i < len(mine) && i < len(theirs) && mine[i] == theirs[i] {
						i++
					}
					j := 0
					for j < len(mine)-i && j < len(theirs)-i && mine[len(mine)-1-j] == theirs[len(theirs)-1-j] {
						j++
					}
					d.prefix = strings.Join(mine[:i], " ")
					d.mine = strings.Join(mine[i:len(mine)-j], " ")
					d.theirs = strings.Join(theirs[i:len(theirs)-j], " ")
				}
				if groups[d] == nil {
					order = append(order, d)
				}
				groups[d] = append(groups[d], used{name, mine, theirs, u.lift.Paths, owner.lift.Paths})
			}
		}
	}
	typeDirs := map[string]string{} // Go type → its corpus directory
	for _, g := range regs {
		for _, ct := range g.Types {
			typeDirs[ct.GoType] = ct.Dir
		}
	}
	for _, d := range order {
		us := groups[d]
		slices.SortStableFunc(us, func(a, b used) int { return len(a.route) - len(b.route) })
		var names []string
		for _, u := range us {
			names = append(names, u.lift)
		}
		near := us[0]
		lifts, it, its, past := "lift "+names[0]+" is", "it", "its", ""
		if len(us) > 1 {
			lifts, it, its, past = "lifts "+andList(names)+" are", "them", "their", " and past it"
		}
		if d.missing {
			// The owner takes the user's lifts from the corpus only as far
			// as a corpus both run the lift over: past it, its own lifts
			// take it on, and where the user's differ, the failure from
			// that corpus says so.
			userRuns, both := corpusSet(near.userPaths), map[string]bool{}
			for c := range corpusSet(near.ownerPaths) {
				both[c] = userRuns[c]
			}
			need := r.missingRegistrations(regs[d.owner], regs[d.user], d.corpus, near.route, both, typeDirs, owners)
			if need.empty() {
				t.Errorf("the %s used by %s on the path %s%s, and %s, which owns %s, doesn't run %s over testdata/%s/, so no package writes %s cells of that corpus's cases, which %s's cells past %s are built through.\nA package that uses a lift reaches it from each corpus by the path the lift's owner does, and the registry of %s records the corpus type of testdata/%s/ and the lifts on %s's path from it, so a registry is stale: run %s, which rewrites the registry of every package with tests.",
					lifts, d.user, route(d.corpus, near.route, near.lift), past, d.owner, it, it, d.corpus, its, d.user, it,
					d.owner, d.corpus, d.user, r.updateCmd())
				continue
			}
			t.Errorf("the %s used by %s on the path %s%s, and %s, which owns %s, doesn't run %s over testdata/%s/, so no package writes %s cells of that corpus's cases, which %s's cells past %s are built through.\nA package that uses a lift reaches it from each corpus by the path the lift's owner does. In %s, %s, so %s runs %s over testdata/%s/ too. If %s registers them already, or %s doesn't reach %s from testdata/%s/, a registry is stale: run %s, which rewrites the registry of every package with tests.",
				lifts, d.user, route(d.corpus, near.route, near.lift), past, d.owner, it, it, d.corpus, its, d.user, it,
				d.owner, need, d.owner, it, d.corpus, d.owner, d.user, it, d.corpus, r.updateCmd())
			continue
		}
		mine, theirs := strings.Fields(d.mine), strings.Fields(d.theirs)
		tail := near.route[len(strings.Fields(d.prefix))+len(mine):]
		replace := func(pkg string, add, drop []string, paths []registry.Path) string {
			return replaceLifts(pkg, add, drop, regs[pkg]) + r.adoptRegistrations(pkg, add, drop, tail, paths, regs, entries, owners, typeDirs)
		}
		t.Errorf("the %s used by %s on the path %s%s, and %s, which owns %s, runs %s on the path %s%s, so no package writes the cells of %s that %s's cells past %s are built through.\nA package that uses a lift reaches it from each corpus by the path the lift's owner does. Either %s, or %s. If they reach %s alike already, a registry is stale: run %s, which rewrites the registry of every package with tests.",
			lifts, d.user, route(d.corpus, near.route, near.lift), past, d.owner, it, it, route(d.corpus, near.theirs, near.lift), past, andList(names), d.user, it,
			replace(d.user, theirs, mine, near.userPaths), replace(d.owner, mine, theirs, near.ownerPaths), it, r.updateCmd())
	}
}

// corpusSet is the set of the corpora paths start from.
func corpusSet(paths []registry.Path) map[string]bool {
	set := map[string]bool{}
	for _, p := range paths {
		set[p.Corpus] = true
	}
	return set
}

// needed is what a package registers to reach a lift from a corpus as
// another package does: the corpus types of dirs, the lifts of use, with
// hypercase.Use, each owned by a package, and the lifts of own, with
// hypercase.Own, owned by none.
type needed struct{ dirs, use, own []string }

func (g needed) empty() bool { return len(g.dirs)+len(g.use)+len(g.own) == 0 }

// and adds what o names to g, each once.
func (g *needed) and(o needed) {
	add := func(to *[]string, from []string) {
		for _, x := range from {
			if !slices.Contains(*to, x) {
				*to = append(*to, x)
			}
		}
	}
	add(&g.dirs, o.dirs)
	add(&g.use, o.use)
	add(&g.own, o.own)
}

func (g needed) String() string {
	var parts []string
	switch len(g.dirs) {
	case 0:
	case 1:
		parts = append(parts, "register the corpus type of "+dirList(g.dirs))
	default:
		parts = append(parts, "register the corpus types of "+dirList(g.dirs))
	}
	if len(g.use) > 0 {
		parts = append(parts, "give "+andList(g.use)+" to hypercase.Use")
	}
	if len(g.own) > 0 {
		parts = append(parts, "give "+andList(g.own)+" to hypercase.Own")
	}
	return strings.Join(parts, ", and ")
}

// missingRegistrations is what the package of the registry own lacks to
// reach a lift from corpus by the lifts of route, from the corpus, as the
// package of the registry user does, as far as the product of a bytes lift
// on route whose corpus is one of reached: the corpus types of the corpus
// and of each bytes lift's product before it, and those lifts, each with
// hypercase.Use where a package owns it, and hypercase.Own where none
// does.
func (r *runner) missingRegistrations(own, user *registry.Registry, corpus string, route []string, reached map[string]bool, typeDirs map[string]string, owners map[string][]string) needed {
	has := map[string]bool{}
	for _, ct := range own.Types {
		has["dir "+ct.Dir] = true
	}
	for _, l := range own.Lifts {
		has["lift "+l.Name] = true
	}
	entry := map[string]registry.Lift{}
	for _, l := range user.Lifts {
		entry[l.Name] = l
	}
	var need needed
	dirs := []string{corpus}
	for _, name := range route {
		switch {
		case has["lift "+name]:
		case len(owners[name]) > 0:
			need.use = append(need.use, name)
		default:
			need.own = append(need.own, name)
		}
		if l := entry[name]; l.Bytes {
			d := typeDirs[l.Product]
			if reached[d] {
				break
			}
			dirs = append(dirs, d)
		}
	}
	for _, d := range dirs {
		if !has["dir "+d] && !slices.Contains(need.dirs, d) {
			need.dirs = append(need.dirs, d)
		}
	}
	return need
}

// adoptRegistrations words the rest of one way to end a divergence, in
// which the package pkg gives add to hypercase.Use in place of drop.
// paths are pkg's paths of the lift, and those that reach it through drop,
// then tail, start from the corpora pkg reaches add from then. Where the
// owner of add's first lift doesn't run it over such a corpus, the owner
// registers what it lacks to, as missingRegistrations words it, as far as
// a corpus both run the lifts over, so that the way is whole: following
// it ends the failure, even where another failure names the same
// registrations.
func (r *runner) adoptRegistrations(pkg string, add, drop, tail []string, paths []registry.Path, regs map[string]*registry.Registry, entries map[string][]liftEntry, owners map[string][]string, typeDirs map[string]string) string {
	if len(add) == 0 || len(owners[add[0]]) != 1 || owners[add[0]][0] == pkg {
		return ""
	}
	owner := owners[add[0]][0]
	var runs map[string]bool
	for _, e := range entries[add[0]] {
		if e.pkg == owner && e.lift.Owned {
			runs = corpusSet(e.lift.Paths)
		}
	}
	reaches := corpusSet(paths)
	both := map[string]bool{}
	for c := range runs {
		both[c] = reaches[c]
	}
	after := append(slices.Clone(drop), tail...)
	var need needed
	for _, p := range paths {
		route := reversed(p.Lifts)
		if runs[p.Corpus] || len(route) < len(after) || !slices.Equal(route[len(route)-len(after):], after) {
			continue
		}
		need.and(r.missingRegistrations(regs[owner], regs[pkg], p.Corpus, route[:len(route)-len(after)], both, typeDirs, owners))
	}
	if need.empty() {
		return ""
	}
	return ", and in " + owner + ", " + need.String()
}

// replaceLifts words one way to end a divergence: the package pkg, whose
// registry is reg, registers add, with hypercase.Use, in place of drop,
// and the cells of each lift of drop it owns, which no package writes
// then, are deleted.
func replaceLifts(pkg string, add, drop []string, reg *registry.Registry) string {
	s := "in " + pkg + ", give " + andList(add) + " to hypercase.Use"
	if len(add) == 0 {
		s = "in " + pkg + ", remove " + andList(drop)
	} else if len(drop) > 0 {
		s += " in place of " + andList(drop)
	}
	var owned, dirs []string
	for _, l := range reg.Lifts {
		if l.Owned && slices.Contains(drop, l.Name) {
			owned = append(owned, l.Name)
			dirs = append(dirs, "out/"+l.Name+"/")
		}
	}
	switch len(owned) {
	case 0:
	case 1:
		s += ", and delete " + owned[0] + "'s cells, under " + dirs[0] + " in the archives, which no package writes then"
	default:
		s += ", and delete the cells of " + andList(owned) + ", under " + andList(dirs) + " in the archives, which no package writes then"
	}
	return s
}

// reversed returns a reversed copy of s.
func reversed(s []string) []string {
	out := slices.Clone(s)
	slices.Reverse(out)
	return out
}

// dirList names corpus directories, as testdata/<dir>/ each, in prose.
func dirList(dirs []string) string {
	var names []string
	for _, d := range dirs {
		names = append(names, "testdata/"+d+"/")
	}
	return andList(names)
}

// liftFields are the fields of a lift's registry entry that every registry
// that owns or uses the lift records alike, each by its name in the
// registry and as the root-package check shows it. Whether a package owns
// the lift differs between them, and so may the corpora that reach it,
// and the paths from them, where the owner runs the lift over each corpus
// a user reaches it from, by the user's path, which rootCheck compares
// apart.
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
// the columns over its corpus, products the bytes lifts among them, and
// transforms its corpus type's.
func (r *runner) checkArchive(t *testing.T, p, corpus string, cols, products, transforms map[string]bool, props func(string) bool) {
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
	// corpus type lists the transform.
	type claim struct{ what, why string }
	unclaimed := map[claim][]string{}
	for _, s := range a.Sections {
		kind, name, rest := archive.SplitCase(s.Name)
		if kind == archive.Transform && !transforms[name] {
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
		if via, ok := strings.CutPrefix(rest, archive.Via+"/"); ok {
			if l, _, _ := strings.Cut(via, "/"); !products[l] {
				c := claim{archive.Via + "/" + l, fmt.Sprintf("no bytes lift named %s runs over the corpus %s.\nIf a bytes lift should write them, register it", l, corpus)}
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
