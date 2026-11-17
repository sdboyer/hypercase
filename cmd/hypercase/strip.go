package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/registry"
	"github.com/sdboyer/hypercase/internal/root"
	"github.com/sdboyer/hypercase/internal/shell"
	"github.com/sdboyer/hypercase/internal/walk"
	"github.com/sdboyer/hypercase/strip"
)

// once is a flag that may be given once.
type once struct {
	value string
	set   bool
}

func (o *once) String() string { return o.value }

func (o *once) Set(v string) error {
	if o.set {
		return errors.New("the flag is given twice, and each flag may be given once")
	}
	o.value, o.set = v, true
	return nil
}

// onceBool is a boolean flag that may be given once.
type onceBool struct{ once }

func (o *onceBool) IsBoolFlag() bool { return true }

func (o *onceBool) Set(v string) error {
	if _, err := strconv.ParseBool(v); err != nil {
		return err
	}
	return o.once.Set(v)
}

func (o *onceBool) on() bool {
	b, _ := strconv.ParseBool(o.value)
	return b
}

func runStrip(dir string, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("strip", flag.ContinueOnError)
	set.SetOutput(stderr)
	set.Usage = func() {
		fmt.Fprintln(stderr, "usage: "+stripUsage)
		set.PrintDefaults()
	}
	var rootFlag, column, transform once
	var frozen onceBool
	set.Var(&rootFlag, "root", "the corpus root `dir`, relative to the current directory or by an absolute path")
	set.Var(&column, "column", "strip only the sections of the column `name`, at every level")
	set.Var(&transform, "transform", "strip only the sections under the variant of the transform `name`")
	set.Var(&frozen, "frozen", "strip frozen cells' sections too")
	if err := set.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if set.NArg() > 0 {
		fmt.Fprintf(stderr, "hypercase strip takes no arguments, and was given %d: %s.\nusage: %s\n", set.NArg(), strings.Join(set.Args(), " "), stripUsage)
		return 2
	}
	for _, f := range []struct {
		name, what string
		o          once
	}{{"root", "the corpus root", rootFlag}, {"column", "a column", column}, {"transform", "a transform", transform}} {
		switch {
		case f.o.set && f.o.value == "":
			fmt.Fprintf(stderr, "hypercase strip: -%s names %s, and was given an empty name.\nusage: %s\n", f.name, f.what, stripUsage)
			return 2
		case f.name != "root" && f.o.value != "" && !archive.ValidName(f.o.value):
			fmt.Fprintf(stderr, "hypercase strip: -%s %s is no %s's name, which is %s.\nusage: %s\n", f.name, f.o.value, f.name, archive.NameRule, stripUsage)
			return 2
		}
	}
	sel := strip.Selection{Column: column.value, Transform: transform.value, Frozen: frozen.on()}
	return stripRoot(dir, rootFlag.value, sel, stdout, stderr)
}

// A stripped is what strip did to one file: a line of its output, which
// names the file by its path relative to the corpus root.
type stripped struct {
	path, text string
	problem    bool
}

// ownRegistry is a registry that records the corpus root being stripped.
type ownRegistry struct {
	path, pkg string
}

// stripRoot strips the corpus root given, relative to dir, or else the one
// found from dir, and returns the exit code.
func stripRoot(dir, given string, sel strip.Selection, stdout, stderr io.Writer) int {
	fail := func(format string, args ...any) int {
		fmt.Fprintf(stderr, "hypercase strip: "+format+"\n", args...)
		return 1
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return fail("finding the current directory failed: %v.\nRun strip from a directory that exists.", err)
	}
	corpusRoot, err := findCorpusRoot(dir, given)
	if err != nil {
		return fail("%v", err)
	}
	if fi, err := os.Stat(filepath.Join(corpusRoot, "testdata")); err != nil || !fi.IsDir() {
		return fail("%s holds no testdata/ directory, so it is no corpus root.\nName the corpus root with -root, relative to the current directory or by an absolute path.", corpusRoot)
	}
	mod, err := root.Module(corpusRoot)
	if err != nil {
		return fail("finding the module root of %s failed: %v.\nName a corpus root inside a Go module with -root, relative to the current directory or by an absolute path.", corpusRoot, err)
	}
	rootRel := root.Rel(mod, corpusRoot)
	rootName := "the corpus root " + rootRel
	if rootRel == "." {
		rootName = "the module-root corpus"
	}
	rel := func(p string) string { return root.Rel(corpusRoot, p) }
	// A line that says to run strip again names the root with -root,
	// unless strip run again from dir with no -root finds the same root
	// whether or not this run removes the registry there: dir is the module
	// root, the corpus root the module-root corpus, and the registry there,
	// if any, records it.
	bare, err := findCorpusRoot(dir, "")
	named := dir != mod || corpusRoot != mod || err != nil || bare != corpusRoot
	again := rerun(dir, corpusRoot, named, sel)

	// The registries, read before anything is removed: those that record
	// the corpus root, and the corpus roots the others record.
	found, err := registry.Find(mod)
	if err != nil {
		return fail("finding the module's registries failed: %v. Nothing was removed.\nFix what failed, then run %s again.", err, again)
	}
	var own []ownRegistry
	others := map[string]bool{}
	vars := map[string][]string{} // update variable → the registries that record it
	var regs []*registry.Registry // those that record the corpus root, read whole
	for _, f := range found {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			return fail("reading %s failed: %v. Nothing was removed.\nFix what failed, then run %s again.", rel(f.Path), err, again)
		}
		h, err := registry.Peek(data)
		if err != nil {
			return fail("%s can't be read as a registry: %v. So strip can't tell which corpus root it records, or the update to run. Nothing was removed.\nFix the file or delete it, then run %s again.", rel(f.Path), err, again)
		}
		if h.CorpusRoot != rootRel {
			others[filepath.Join(mod, filepath.FromSlash(h.CorpusRoot))] = true
			continue
		}
		own = append(own, ownRegistry{path: f.Path, pkg: f.Pkg})
		if h.UpdateVar != "" {
			vars[h.UpdateVar] = append(vars[h.UpdateVar], rel(f.Path))
		}
		// The columns it registers over each corpus, so that strip can
		// tell a bytes lift's via/ from a section of no cell. A registry
		// of another format holds them in a form this strip doesn't read,
		// and tells it of no column.
		reg, err := registry.Parse(data)
		var format *registry.FormatError
		switch {
		case errors.As(err, &format):
			continue
		case err != nil:
			return fail("%s can't be read as a registry: %v. So strip can't tell which of the columns it registers are bytes lifts. Nothing was removed.\nFix the file or delete it, then run %s again.", rel(f.Path), err, again)
		}
		regs = append(regs, reg)
	}
	lifts := registry.BytesLifts(regs) // corpus → column → whether it is a bytes lift
	if len(vars) > 1 {
		var each []string
		for _, v := range slices.Sorted(maps.Keys(vars)) {
			each = append(each, fmt.Sprintf("%s, in %s", v, strings.Join(vars[v], " and ")))
		}
		return fail("the registries that record %s record different update variables: %s. Nothing was removed.\nOne project's registries record its one update variable: give every package over the corpus root the same decl.Project, run its update, then run %s again.", rootName, strings.Join(each, "; "), again)
	}

	// The archives: every *.txtar under the root's testdata/, as the
	// root-package check walks it, past nested modules and other corpus
	// roots. A link in place of testdata/ or of a corpus's directory
	// fails strip, since it follows none, where a run reads through it.
	td := filepath.Join(corpusRoot, "testdata")
	var paths []string
	links, err := walk.Testdata(td, others, func(p string, d fs.DirEntry) {
		if strings.HasSuffix(d.Name(), ".txtar") {
			paths = append(paths, p)
		}
	})
	if err != nil {
		return fail("reading %s failed: %v. Nothing was removed.\nFix what failed, then run %s again.", rel(td), err, again)
	}
	if len(links) > 0 {
		var each []string
		for _, l := range links {
			each = append(each, rel(l))
		}
		are := each[0] + " is a symbolic link"
		if len(each) > 1 {
			are = strings.Join(each[:len(each)-1], ", ") + " and " + each[len(each)-1] + " are symbolic links"
		}
		return fail("%s, and a corpus root's testdata/, and a corpus's directory in it, are read as directories: a normal run and the update read archives through a link, and strip follows none, so it would strip none of the archives there. Nothing was removed.\nReplace each link with the directory it names, then run %s again.", are, again)
	}
	if sel.Column != "" || sel.Transform != "" {
		if ok, left := matches(corpusRoot, paths, sel); !ok && len(left) == 0 {
			return fail("%s matches no section under %s: no archive holds %s.\nName a column or transform whose sections an archive holds.", flags(sel), rootName, what(sel))
		} else if !ok {
			var each []string
			for _, l := range left {
				each = append(each, l.path+": "+l.text)
			}
			return fail("%s matches no section that strip removes under %s. Only archives that it leaves as they are hold %s, or can't be read:\n%s\nFix them, then run %s again.", flags(sel), rootName, what(sel), strings.Join(each, "\n"), again)
		}
	}

	var lines []stripped
	add := func(p, text string, problem bool) {
		lines = append(lines, stripped{path: rel(p), text: text, problem: problem})
	}
	// Whether a *.txtar file was left as it was, so that strip must run
	// again: one that Archive refuses, as no archive of the corpus root by
	// its path or for problems of form, or one whose write failed.
	leftArchive := false
	for _, p := range paths {
		// What the registries say of the bytes lifts over the corpus.
		each := sel
		corpus, _, _ := strings.Cut(root.Rel(td, p), "/")
		each.BytesLifts = lifts[corpus]
		r, err := strip.Archive(corpusRoot, p, each)
		if err != nil {
			add(p, fmt.Sprintf("left as it was: %v, then run %s again.", err, again), true)
			leftArchive = true
			continue
		}
		if n := len(r.Removed); n == 1 {
			add(p, "removed 1 section", false)
		} else if n > 1 {
			add(p, fmt.Sprintf("removed %d sections", n), false)
		}
		for _, f := range r.Frozen {
			// The command that strips the cell: its column's sections,
			// under its transform for a transform's variant's cell. No flag
			// limits strip to one archive or to a hand-written variant, so
			// it strips the column's other frozen cells too.
			name := f.Column
			thaw := strip.Selection{Column: f.Column, Frozen: true}
			reach := f.Column + "'s other frozen cells"
			if f.Case != "" {
				name = f.Case + "/" + f.Column
				if kind, t, _ := strings.Cut(f.Case, "/"); kind == archive.Transform {
					thaw.Transform = t
					reach += " under " + archive.Transform + "/" + t + "/"
				}
			}
			add(p, fmt.Sprintf("left the cell %s, frozen by its known gap, %s line %d; %s strips it, and %s", name, f.Gaps, f.Line, rerun(dir, corpusRoot, named, thaw), reach), false)
		}
	}
	if sel.Column == "" && sel.Transform == "" {
		// A registry goes only once its package's findings files have, so
		// that a strip run again finds those left, and only once no
		// *.txtar file is left as it was, so that a strip run again, once
		// it is fixed, moved into a corpus's directory or deleted, reads
		// the registries as this one did.
		lock := filepath.Join(td, archive.LockFile)
		for _, g := range own {
			left := false
			ps, err := findings(g.pkg)
			if err != nil {
				add(filepath.Join(g.pkg, "testdata", archive.ChecksDir), fmt.Sprintf("left as it was: reading it failed: %v. Fix what failed, then run %s again.", err, again), true)
				left = true
			}
			for _, p := range ps {
				if err := archive.RemoveFile(lock, p); err != nil {
					add(p, fmt.Sprintf("left in place: removing it failed: %v. Fix what failed, then run %s again.", err, again), true)
					left = true
				}
			}
			switch {
			case left:
				add(g.path, "left in place, so that strip run again finds the findings files of its package that this run left.", false)
				continue
			case leftArchive:
				add(g.path, "left in place, since this run left a *.txtar file as it was, so that strip, run again as that file's line says, reads the registry as this run did.", false)
				continue
			}
			if err := archive.RemoveFile(lock, g.path); err != nil {
				add(g.path, fmt.Sprintf("left in place: removing it failed: %v. Fix what failed, then run %s again.", err, again), true)
			}
		}
	}

	slices.SortStableFunc(lines, func(a, b stripped) int { return strings.Compare(a.path, b.path) })
	code := 0
	for _, l := range lines {
		fmt.Fprintf(stdout, "%s: %s\n", l.path, l.text)
		if l.problem {
			code = 1
		}
	}
	if len(vars) == 1 {
		for v := range vars {
			fmt.Fprintf(stdout, "Next, run %s=1 go test ./... at the module root, and again until it changes nothing.\n", v)
		}
	} else {
		fmt.Fprintf(stdout, "Next, run the project's update over the whole module, at its root, and again until it changes nothing: no registry records %s, so strip can't name its update variable.\n", rootName)
	}
	return code
}

// rerun returns the command that strips sel from the corpus root again,
// run from dir, naming the root with -root when named says to. It quotes
// the root's path for a shell where the path needs it.
func rerun(dir, corpusRoot string, named bool, sel strip.Selection) string {
	cmd := []string{"hypercase", "strip"}
	if named {
		cmd = append(cmd, "-root", shell.Quote(root.Rel(dir, corpusRoot)))
	}
	if f := flags(sel); f != "" {
		cmd = append(cmd, f)
	}
	if sel.Frozen {
		cmd = append(cmd, "-frozen")
	}
	return strings.Join(cmd, " ")
}

// findCorpusRoot returns the corpus root, from dir, an absolute path:
// given, relative to dir; or else the one that the registry of the package
// in dir records; or else, when dir is the module root, the module root.
// Anywhere else with no registry, strip can't tell the package's corpus
// root, and asks for -root. A registry's corpus root inside another
// module, one nested in the package's, is refused: strip reads a corpus
// root's registries in its own module, and the package's is another.
func findCorpusRoot(dir, given string) (string, error) {
	if given != "" {
		p := given
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			return "", fmt.Errorf("-root %s names no directory.\nName the corpus root relative to the current directory or by an absolute path.", given)
		}
		return p, nil
	}
	mod, err := root.Module(dir)
	if err != nil {
		return "", fmt.Errorf("finding the module root failed: %v.\nRun strip inside a Go module, or name the corpus root with -root, relative to the current directory or by an absolute path.", err)
	}
	p := filepath.Join(dir, "testdata", registry.Name)
	data, err := os.ReadFile(p)
	switch {
	case errors.Is(err, fs.ErrNotExist) && dir == mod:
		return mod, nil
	case errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("the current directory holds no registry, testdata/%s, to say which corpus root its package's tests read, and it isn't the module root, %s, where strip finds the module-root corpus.\nName the corpus root with -root, relative to the current directory or by an absolute path, or run strip at the module root.", registry.Name, root.Rel(dir, mod))
	case err != nil:
		return "", fmt.Errorf("reading %s failed: %v, so strip can't tell which corpus root it records.\nFix what failed, or name the corpus root with -root, relative to the current directory or by an absolute path.", root.Rel(dir, p), err)
	}
	h, err := registry.Peek(data)
	if err != nil {
		return "", fmt.Errorf("%s can't be read as a registry: %v, so strip can't tell which corpus root it records.\nName the corpus root with -root, relative to the current directory or by an absolute path.", root.Rel(dir, p), err)
	}
	if !fs.ValidPath(h.CorpusRoot) {
		return "", fmt.Errorf("%s records the corpus root %q, which isn't a slash path relative to the module root.\nName the corpus root with -root, relative to the current directory or by an absolute path.", root.Rel(dir, p), h.CorpusRoot)
	}
	corpusRoot := filepath.Join(mod, filepath.FromSlash(h.CorpusRoot))
	if m, err := root.Module(corpusRoot); err == nil && m != mod {
		return "", fmt.Errorf("%s records the corpus root %s, which lies inside the module at %s, and not in the module at %s, the package's.\nMove the corpus root out of the module at %s, or name it with -root, relative to the current directory or by an absolute path, to strip it as a corpus root of that module.", root.Rel(dir, p), h.CorpusRoot, root.Rel(dir, m), root.Rel(dir, mod), root.Rel(dir, m))
	}
	return corpusRoot, nil
}

// matches reports whether sel selects a section of an archive of the
// corpus root that strip strips. When it selects none, it also returns,
// with why, each archive that strip leaves as it is and that holds a
// section sel selects, or that it can't read.
func matches(corpusRoot string, paths []string, sel strip.Selection) (bool, []stripped) {
	var left []stripped
	for _, p := range paths {
		rel := root.Rel(corpusRoot, p)
		data, err := os.ReadFile(p)
		if err != nil {
			left = append(left, stripped{path: rel, text: fmt.Sprintf("reading it failed: %v.", err)})
			continue
		}
		a := archive.Parse(data)
		if !slices.ContainsFunc(a.Sections, func(s archive.Section) bool { return sel.Selects(s.Name) }) {
			continue
		}
		why := strip.CheckPath(corpusRoot, p)
		if why == nil {
			why = strip.CheckForm(a)
		}
		if why == nil {
			return true, nil
		}
		left = append(left, stripped{path: rel, text: why.Error() + "."})
	}
	return false, left
}

// flags gives a selection as the flags that limit it.
func flags(sel strip.Selection) string {
	var given []string
	if sel.Column != "" {
		given = append(given, "-column "+sel.Column)
	}
	if sel.Transform != "" {
		given = append(given, "-transform "+sel.Transform)
	}
	return strings.Join(given, " ")
}

// what says which sections a limited selection selects.
func what(sel strip.Selection) string {
	switch {
	case sel.Transform == "":
		return fmt.Sprintf("out/%s/… or via/%s/…, at any level", sel.Column, sel.Column)
	case sel.Column == "":
		return fmt.Sprintf("transform/%s/…", sel.Transform)
	}
	return fmt.Sprintf("transform/%s/out/%s/… or transform/%s/via/%s/…", sel.Transform, sel.Column, sel.Transform, sel.Column)
}

// findings returns the findings files in a package's testdata/checks/:
// each regular file whose name is a check's, under the naming rule, then
// .txt. A package with no testdata/checks/ has none.
func findings(pkg string) ([]string, error) {
	dir := filepath.Join(pkg, "testdata", archive.ChecksDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	var ps []string
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".txt"); ok && e.Type().IsRegular() && archive.ValidName(name) {
			ps = append(ps, filepath.Join(dir, e.Name()))
		}
	}
	return ps, err
}
