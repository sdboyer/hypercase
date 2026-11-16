// Package snapshot holds the primitives a project's own command uses to
// make a case from real input, and to work on a case's inputs as files:
// serialize trees of files as an archive's inputs, write a new case, load
// it back through its corpus type, extract a case's input view to a
// directory, and rewrite a case's inputs. What it writes reads back as the
// runner reads it.
//
// Which real files make a good case is the project's to say, so the
// command that takes them is the project's, and so is the update command
// it prints, which fills a new case's outputs. The command finds the
// corpus root it passes them with root.Find, as a run finds it.
//
// Create and RewriteInputs write a case's inputs, which no update writes,
// so each refuses while the project's update variable is set. Serialize,
// Load and Extract write no archive.
//
// The package doesn't import testing, so a project's command can import
// it.
package snapshot

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

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/hooks"
	"github.com/sdboyer/hypercase/internal/registry"
	"github.com/sdboyer/hypercase/root"
)

// Serialize turns each tree's files into the sections in/<tree>/<path>,
// with each file's bytes exactly as read, through the walk
// [archive.TreeSections]. The sections are sorted by name, so the same
// trees always serialize to the same bytes.
//
// A tree's name is one path element, not params, which is reserved, and
// one an archive's section names can hold, so no newline. Each
// tree holds at least one file. What the walk refuses, an empty directory,
// a file that isn't regular or one that wouldn't read back as written,
// fails Serialize, and so does an error reading any path. Each failure
// names the tree and the path, and returns no sections. The project's
// command filters out what it doesn't take before it calls Serialize.
func Serialize(trees map[string]fs.FS) ([]archive.Section, error) {
	if len(trees) == 0 {
		return nil, errors.New("there are no trees to serialize, and a case holds at least one.\nPass the case's input trees, by name.")
	}
	var ss []archive.Section
	for _, name := range slices.Sorted(maps.Keys(trees)) {
		if err := checkTree(name); err != nil {
			return nil, err
		}
		if trees[name] == nil {
			return nil, fmt.Errorf("the tree %s is nil.\nPass the tree's files as an fs.FS.", name)
		}
		got, err := archive.TreeSections(archive.In+"/"+name+"/", trees[name])
		if err != nil {
			at := ""
			var te *archive.TreeError
			if errors.As(err, &te) {
				at, err = ", at "+te.Path, te.Err
			}
			return nil, fmt.Errorf("the tree %s%s: %v.\nFix the file, or leave it out of the tree: the project's command chooses which files make the case.", name, at, err)
		}
		if len(got) == 0 {
			return nil, fmt.Errorf("the tree %s holds no files, and an archive holds a tree only as its files.\nGive the tree its files, or leave it out.", name)
		}
		ss = append(ss, got...)
	}
	slices.SortFunc(ss, func(a, b archive.Section) int { return strings.Compare(a.Name, b.Name) })
	return ss, nil
}

// checkTree checks a tree's name: one path element, not params, and one
// a section's name holds. Every section of the tree is named
// in/<name>/<path>, so the name holds where in/<name>/ does.
func checkTree(name string) error {
	switch {
	case name == "." || !fs.ValidPath(name) || strings.Contains(name, "/"):
		return fmt.Errorf("the tree name %q isn't one path element: a tree is the directory under in/ that holds it, such as sheet for in/sheet/.\nName the tree with one element.", name)
	case name == archive.ParamsTree:
		return errors.New("the tree name params is reserved: a column's parameters are fixed in Go, or derived from the archive's name, and an archive carries none.\nName the tree otherwise.")
	}
	if err := archive.HoldsName(archive.In + "/" + name + "/"); err != nil {
		return fmt.Errorf("the tree name %q can't be held in an archive, in the names of its sections: %v.\nName the tree otherwise.", name, err)
	}
	return nil
}

// notDuringUpdate refuses to run fn, a primitive that writes a case's
// inputs, while the variable the project's UpdateVar names is set: no
// update writes a case's inputs. what is what fn does, as "creates a
// case", and fix how a person does it otherwise.
func notDuringUpdate(fn string, project decl.Project, what, fix string) error {
	updateVar := project.UpdateVar
	if updateVar == "" {
		return fmt.Errorf("%s has no update variable to check: the project's UpdateVar is empty, and %s refuses during an update.\nPass the decl.Project the project registers.", fn, fn)
	}
	if os.Getenv(updateVar) != "" {
		return fmt.Errorf("%s is set, so this may be an update, and no update %s.\n%s from the project's command, or from a test run without %s.", updateVar, what, fix, updateVar)
	}
	return nil
}

// noCorpus refuses the zero root.Corpus, which names no corpus root.
func noCorpus(fn string) error {
	return fmt.Errorf("%s was given no corpus root.\nFind it with root.Find, from the command's directory and the path the package passes to hypercase.Root.", fn)
}

// Create writes sections, under an empty comment, as the new case
// testdata/<typ.Dir>/<name>.txtar under the corpus root c, and returns its
// path. c is as root.Find finds it, and its directory exists; Create makes
// testdata/<typ.Dir>/ under it if it is missing. A registry under the
// module root records c as its package's corpus root, so some package's
// tests read the case: the path a package passes to hypercase.Root and
// the one a command passes to root.Find are written apart, and Create
// checks one against the other. A package's registry is written by its
// first update, so Create works once that has run. project is the project's
// declaration, as the project registers it: no update creates a case, so
// while the variable its UpdateVar names is set, Create fails, naming it,
// and writes nothing.
//
// name is the case's name, without .txtar, and follows the naming rule,
// which no path does. A new case holds only inputs, so every section is
// under in/, and there is at least one. The sections are an archive with
// no problem of form, and they read back as written. When any of these
// fails, nothing is written, and the error names what failed.
//
// The case is written with [archive.CreateFile], by a temp file linked to
// the case's name, so Create never replaces a case, even one another
// process creates between a check and the write, and no reader sees part
// of one. A write cut short leaves only a temp file, which blocks no later
// Create, and which the root-package check reports. A write that fails
// leaves no file, and removes the directories Create made for it, when
// they are empty.
//
// The comment is left empty, since the description is for a person to
// write: until they do, the root-package check fails on the case, and an
// update still writes its outputs. Nor does the new case state any
// property: a case states its properties by hand.
func Create[T ~struct{ fs.FS }](c root.Corpus, project decl.Project, typ decl.Type[T], name string, sections []archive.Section) (string, error) {
	if err := notDuringUpdate("Create", project, "creates a case", "Create the case"); err != nil {
		return "", err
	}
	if c == (root.Corpus{}) {
		return "", noCorpus("Create")
	}
	if !archive.ValidName(name) {
		return "", fmt.Errorf("the case's name, %q, breaks the naming rule: %s. A case's name is its archive's file name, less .txtar, and never a path.\nName the case by the rule.", name, archive.NameRule)
	}
	if err := checkDir(typ); err != nil {
		return "", err
	}
	if err := recorded(c, project.UpdateVar); err != nil {
		return "", err
	}
	dir := c.Dir()
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("the corpus root %s isn't a directory.\nPass the corpus root: the module root, or the directory a package passes to hypercase.Root.", dir)
	}
	if len(sections) == 0 {
		return "", fmt.Errorf("there are no sections for the case %s, and a new case holds its inputs.\nPass the sections Serialize makes from the case's trees.", name)
	}
	for _, s := range sections {
		if !strings.HasPrefix(s.Name, archive.In+"/") {
			return "", fmt.Errorf("the section %s isn't under in/, and a new case holds only inputs: the update writes its outputs, and a person its other sections.\nPass the case's inputs alone.", s.Name)
		}
	}
	a := &archive.Archive{Sections: slices.Clone(sections)}
	if ps := a.Check(); len(ps) > 0 {
		return "", fmt.Errorf("the sections for the case %s have problems of form, so none of its cells would run:\n%s\nFix the sections.", name, problemList(ps))
	}
	data, err := a.ReadBack()
	if err != nil {
		return "", fmt.Errorf("the sections for the case %s can't be written: %v.\nFix the section, or leave it out.", name, err)
	}
	made, err := mkdirs(dir, "testdata", typ.Dir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "testdata", typ.Dir, name+".txtar")
	if err := archive.CreateFile(path, data); err != nil {
		// Each directory Create made is removed only when it is empty: a
		// concurrent Create may have written into it.
		for _, d := range slices.Backward(made) {
			os.Remove(d)
		}
		if errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("%s exists, and Create never replaces a case.\nName the new case otherwise, or remove the archive first.", path)
		}
		return "", fmt.Errorf("creating %s: %v.\nFix what stopped the write, and create the case again.", path, err)
	}
	return path, nil
}

// recorded refuses a corpus root that no registry under its module root
// records as its package's corpus root, naming the roots they do record.
// It reads every registry's head, and one it can't read fails it, since
// that registry may be the one that records the root.
func recorded(c root.Corpus, updateVar string) error {
	found, err := registry.Find(c.Module())
	if err != nil {
		return fmt.Errorf("finding the registries under the module root %s: %v.\nFix what stopped the walk, and create the case again.", c.Module(), err)
	}
	roots := map[string]bool{}
	for _, f := range found {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			return fmt.Errorf("reading %s: %v.\nFix what stopped the read, and create the case again.", f.Path, err)
		}
		h, err := registry.Peek(data)
		if err != nil {
			return fmt.Errorf("%s can't be read: %v, so it may record the corpus root %q.\nRun the update, %s=1 go test ./..., to write it again.", f.Path, err, c.Rel(), updateVar)
		}
		roots[h.CorpusRoot] = true
	}
	if roots[c.Rel()] {
		return nil
	}
	have := "no registry is under it"
	if len(roots) > 0 {
		var quoted []string
		for _, r := range slices.Sorted(maps.Keys(roots)) {
			quoted = append(quoted, fmt.Sprintf("%q", r))
		}
		have = "its registries record " + strings.Join(quoted, ", ")
	}
	return fmt.Errorf("no registry under the module root %s records the corpus root %q, so no package's tests would read the case: %s.\nPass root.Find the path a package passes to hypercase.Root, or . for one that passes none. A package's first update, %s=1 go test ./..., writes its registry.", c.Module(), c.Rel(), have, updateVar)
}

// mkdirs makes each missing directory of the path that the elements name
// under base, which exists, one at a time, and returns those it made, in
// order. A directory that another process makes meanwhile is not one it
// made.
func mkdirs(base string, elems ...string) ([]string, error) {
	var made []string
	dir := base
	for _, e := range elems {
		dir = filepath.Join(dir, e)
		err := os.Mkdir(dir, 0o755)
		if err == nil {
			made = append(made, dir)
			continue
		}
		if errors.Is(err, fs.ErrExist) {
			if fi, serr := os.Stat(dir); serr == nil && fi.IsDir() {
				continue
			}
			err = errors.New("it exists, and isn't a directory")
		}
		for _, d := range slices.Backward(made) {
			os.Remove(d)
		}
		return nil, fmt.Errorf("creating %s: %v.\nFix what stopped it, and create the case again.", dir, err)
	}
	return made, nil
}

// checkDir refuses a corpus type's directory that no corpus type can
// declare, as registration does: one the naming rule rejects, which a
// path is, and the directory of corpus checks' findings.
func checkDir[T ~struct{ fs.FS }](typ decl.Type[T]) error {
	switch {
	case !archive.ValidName(typ.Dir):
		return fmt.Errorf("the corpus type's directory, %q, breaks the naming rule: %s.\nPass the corpus type the project registers.", typ.Dir, archive.NameRule)
	case typ.Dir == archive.ChecksDir:
		return fmt.Errorf("the corpus type's directory, %s, is reserved for the findings of corpus checks.\nPass the corpus type the project registers.", typ.Dir)
	}
	return nil
}

// Load reads the case at path back through its corpus type, typ: it reads
// the archive as the runner does, makes the base's input view from its
// in/ sections as the runner does, and wraps the view in the type. So
// what a project checks after Load is what its tests see. Running lifts
// on the result is the project's choice.
//
// A read error fails Load, and so does an archive the runner doesn't run
// as a case of typ: one outside testdata/<typ.Dir>/, a file name that
// isn't <name>.txtar by the naming rule, and an archive with a problem of
// form, such as an in/params/ tree or a top-level name outside the closed
// set. Each failure names the path.
func Load[T ~struct{ fs.FS }](typ decl.Type[T], path string) (T, error) {
	var zero T
	if err := checkDir(typ); err != nil {
		return zero, err
	}
	a, ps, err := read(path)
	if err != nil {
		return zero, err
	}
	if len(ps) > 0 {
		return zero, formError(path, ps)
	}
	if dir := filepath.Base(filepath.Dir(path)); dir != typ.Dir {
		return zero, fmt.Errorf("%s is in testdata/%s/, and the corpus type's directory is %s, so no run reads it as a case of that type.\nLoad it through the corpus type of its directory.", path, dir, typ.Dir)
	}
	return T(struct{ fs.FS }{a.View()}), nil
}

// read reads the archive at path, as the runner reads one: a file
// testdata/<dir>/<name>.txtar, whose name follows the naming rule. It
// returns the archive and its problems of form, or why it can't read it.
func read(path string) (*archive.Archive, []archive.Problem, error) {
	if err := checkPath(path); err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %v.\nName an archive that exists, and that can be read.", path, err)
	}
	a := archive.Parse(data)
	return a, a.Check(), nil
}

// checkPath refuses a path the runner reads no case from: one that isn't
// testdata/<dir>/<name>.txtar, with a name the naming rule admits.
func checkPath(path string) error {
	name, ok := strings.CutSuffix(filepath.Base(path), ".txtar")
	if !ok {
		return fmt.Errorf("%s isn't an archive: an archive's file name ends in .txtar.\nName the case's archive.", path)
	}
	if !archive.ValidName(name) {
		return fmt.Errorf("%s: the archive's name, %q, breaks the naming rule: %s, so no run reads it.\nRename the archive.", path, name, archive.NameRule)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("finding %s: %v.\nName the case's archive.", path, err)
	}
	if filepath.Base(filepath.Dir(filepath.Dir(abs))) != "testdata" {
		return fmt.Errorf("%s isn't in a corpus's directory, testdata/<dir>/, so no run reads it.\nName an archive in its corpus's directory.", path)
	}
	return nil
}

func formError(path string, ps []archive.Problem) error {
	return fmt.Errorf("%s has problems of form, so none of its cells run:\n%s\nFix the archive by hand.", path, problemList(ps))
}

func problemList(ps []archive.Problem) string {
	lines := make([]string, len(ps))
	for i, p := range ps {
		lines[i] = "  " + p.String()
	}
	return strings.Join(lines, "\n")
}

// Extract writes the input view of the case at path under dir, which it
// creates: each file of the view at <dir>/<tree>/<path>, with its bytes
// exactly as read. The view is made as Load makes it. A non-empty variant
// names the hand-written variant variant/<variant>/, whose overlay and
// deletions Extract lays over the base, as the runner does. A transform's
// variant isn't extracted: its inputs are what the transform makes, which
// an update writes.
//
// dir must not exist, and its parent must. Each of these fails before dir
// is created, naming the path or the variant: what Load refuses, a variant
// the archive doesn't have, and one that defines no case. Extract reads
// back what it wrote, and a write that fails part-way, or files that read
// back otherwise, as on a filesystem that folds names, remove dir.
func Extract(path, variant, dir string) error {
	a, ps, err := read(path)
	if err != nil {
		return err
	}
	if len(ps) > 0 {
		if variant != "" && slices.ContainsFunc(ps, func(p archive.Problem) bool { return inVariant(p.Section, variant) }) {
			return fmt.Errorf("%s: the variant %s defines no case, and the archive has problems of form:\n%s\nFix the archive by hand.", path, variant, problemList(ps))
		}
		return formError(path, ps)
	}
	base := a.Base()
	view := base.View()
	if variant != "" {
		v, err := handWritten(path, a, variant)
		if err != nil {
			return err
		}
		view = base.VariantView(v)
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%s exists, and Extract writes a directory of its own.\nName a directory that doesn't exist yet.", dir)
		}
		return fmt.Errorf("creating %s: %v.\nName a directory whose parent exists.", dir, err)
	}
	if err := writeView(view, dir); err != nil {
		fix := "Fix what stopped the write, and extract the case again."
		if errors.Is(err, errReadBack) {
			fix = "Extract to a filesystem that keeps each name and each byte as written."
		}
		if rerr := os.RemoveAll(dir); rerr != nil {
			return fmt.Errorf("%v, and removing %s: %v.\nRemove the directory. %s", err, dir, rerr, fix)
		}
		return fmt.Errorf("%v, so %s is removed.\n%s", err, dir, fix)
	}
	return nil
}

// errReadBack marks the failure of what Extract wrote to read back as the
// case's input view.
var errReadBack = errors.New("what was written doesn't read back as the case's input view")

// inVariant reports whether a problem's section is a hand-written
// variant's, or the variant as a whole.
func inVariant(section, variant string) bool {
	c := archive.Variant + "/" + variant
	return section == c || strings.HasPrefix(section, c+"/")
}

// handWritten returns the archive's hand-written variant of the given
// name, or why it has none.
func handWritten(path string, a *archive.Archive, variant string) (archive.VariantCase, error) {
	var names []string
	transform := false
	for _, v := range a.Variants() {
		switch {
		case v.Kind == archive.Variant && v.Name == variant:
			return v, nil
		case v.Kind == archive.Variant:
			names = append(names, v.Name)
		case v.Name == variant:
			transform = true
		}
	}
	if transform {
		return archive.VariantCase{}, fmt.Errorf("%s has no variant/%s/, only transform/%s/, and a transform's variant isn't extracted: its inputs are what the transform makes, which an update writes.\nExtract the base, or name a hand-written variant.", path, variant, variant)
	}
	if len(names) == 0 {
		return archive.VariantCase{}, fmt.Errorf("%s has no variant/%s/, and no hand-written variant at all.\nExtract the base, with no variant named.", path, variant)
	}
	return archive.VariantCase{}, fmt.Errorf("%s has no variant/%s/.\nName one of its hand-written variants: %s.", path, variant, strings.Join(names, ", "))
}

// writeView writes a view's files under dir, then reads them back.
func writeView(view fs.FS, dir string) error {
	paths, err := archive.Paths(view)
	if err != nil {
		return fmt.Errorf("reading the case's input view: %v", err)
	}
	for _, p := range paths {
		data, err := fs.ReadFile(view, p)
		if err != nil {
			return fmt.Errorf("reading %s of the case's input view: %v", p, err)
		}
		dst := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("writing %s: %v", dst, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("writing %s: %v", dst, err)
		}
		if hooks.ExtractFile != nil {
			if err := hooks.ExtractFile(dst); err != nil {
				return fmt.Errorf("writing %s: %v", dst, err)
			}
		}
	}
	written := os.DirFS(dir)
	got, err := archive.Paths(written)
	if err != nil {
		return fmt.Errorf("reading back %s: %v", dir, err)
	}
	if !slices.Equal(got, paths) {
		return fmt.Errorf("%w: the files under %s are %s, and the view's are %s, as when a filesystem folds names", errReadBack, dir, strings.Join(got, ", "), strings.Join(paths, ", "))
	}
	for _, p := range paths {
		want, _ := fs.ReadFile(view, p)
		data, err := fs.ReadFile(written, p)
		if err != nil {
			return fmt.Errorf("reading back %s: %v", filepath.Join(dir, filepath.FromSlash(p)), err)
		}
		if !bytes.Equal(data, want) {
			return fmt.Errorf("%w: %s holds other bytes than the view's %s", errReadBack, filepath.Join(dir, filepath.FromSlash(p)), p)
		}
	}
	return nil
}

// RewriteInputs replaces named in/ sections of the case at path, under the
// corpus root c: the base's in/<tree>/<path>, or, with a non-empty
// variant, those of the overlay of variant/<variant>/. Each section is
// passed by its in/ name either way, and each must be in the archive
// already. Every other byte of the archive comes back as it was, and a
// replaced section keeps its place.
//
// No update changes a case's inputs, and RewriteInputs is the explicit way
// to change them. It refuses during an update, so who writes a section
// stays readable from its name: project is the project's declaration, and
// while the variable its UpdateVar names is set, RewriteInputs fails,
// naming it, before it takes any lock, and writes nothing. It reads no
// registry, so it works before a project's first update.
//
// It writes as an update does: it takes the corpus root's lock,
// testdata/.hypercase.lock under the corpus root, re-reads the archive,
// replaces the sections, writes the result to a temp file beside the
// archive, renames it over the archive, and releases the lock. Each
// refusal writes nothing: a project with no UpdateVar; no corpus root; a
// section outside in/, named twice, or that an archive can't hold; an
// archive outside testdata/<dir>/ under the corpus root, or
// one with a problem of form, before or after the rewrite; one that isn't
// in the form an update writes, since rewriting it would change other
// bytes; a variant it lacks, and a transform's variant, whose inputs are
// update-written; and a section it lacks. A write that fails leaves the
// archive as it was.
func RewriteInputs(c root.Corpus, project decl.Project, path, variant string, sections []archive.Section) error {
	if err := notDuringUpdate("RewriteInputs", project, "changes a case's inputs", "Rewrite the inputs"); err != nil {
		return err
	}
	if c == (root.Corpus{}) {
		return noCorpus("RewriteInputs")
	}
	if len(sections) == 0 {
		return fmt.Errorf("there are no sections to rewrite in %s.\nPass the in/ sections to replace.", path)
	}
	seen := map[string]bool{}
	for _, s := range sections {
		switch {
		case !strings.HasPrefix(s.Name, archive.In+"/"):
			return fmt.Errorf("the section %s isn't under in/, and RewriteInputs rewrites only a case's inputs.\nPass in/ sections alone, a variant's by their in/ names too.", s.Name)
		case seen[s.Name]:
			return fmt.Errorf("the section %s is passed twice.\nPass each section once.", s.Name)
		}
		seen[s.Name] = true
		if err := archive.Holds(s.Data); err != nil {
			return fmt.Errorf("the section %s can't be held in an archive: %v.\nChange its content.", s.Name, err)
		}
	}
	dir := c.Dir()
	if err := checkUnder(dir, path); err != nil {
		return err
	}
	prefix := ""
	if variant != "" {
		prefix = archive.Variant + "/" + variant + "/"
	}
	// refused is the refusal the rewrite returns, which Modify returns as
	// it is: anything else Modify returns stopped the write.
	var refused error
	refuse := func(err error) (*archive.Archive, error) {
		refused = err
		return nil, err
	}
	err := archive.Modify(filepath.Join(dir, "testdata", archive.LockFile), path, func(current *archive.Archive, read []byte) (*archive.Archive, error) {
		if ps := current.Check(); len(ps) > 0 {
			return refuse(formError(path, ps))
		}
		// The form judged is that of the bytes current was parsed from.
		if !archive.Placed(read) {
			return refuse(fmt.Errorf("%s isn't in the form an update writes, so rewriting its inputs would change other bytes too.\nRun the update, %s=1 go test ./..., then rewrite the inputs.", path, project.UpdateVar))
		}
		if variant != "" {
			if _, err := handWritten(path, current, variant); err != nil {
				return refuse(err)
			}
		}
		next := current.Clone()
		for _, s := range sections {
			name := prefix + s.Name
			i := slices.IndexFunc(next.Sections, func(x archive.Section) bool { return x.Name == name })
			if i < 0 {
				return refuse(fmt.Errorf("%s has no section %s, and RewriteInputs replaces only inputs the case has.\nName a section the case has, or add the input by hand.", path, name))
			}
			next.Sections[i].Data = bytes.Clone(s.Data)
		}
		if ps := next.Check(); len(ps) > 0 {
			return refuse(fmt.Errorf("%s would have problems of form once rewritten, so none of its cells would run:\n%s\nChange what the sections hold.", path, problemList(ps)))
		}
		return next, nil
	})
	if err != nil && err != refused {
		return fmt.Errorf("rewriting %s: %v.\nThe archive is as it was. Fix what stopped the write, and rewrite again.", path, err)
	}
	return err
}

// checkUnder refuses an archive that isn't in a corpus directory of the
// corpus root dir, dir/testdata/<corpus>/, since dir's lock covers no
// other.
func checkUnder(dir, path string) error {
	if err := checkPath(path); err != nil {
		return err
	}
	want, err := os.Stat(filepath.Join(dir, "testdata"))
	if err == nil {
		var got fs.FileInfo
		if got, err = os.Stat(filepath.Dir(filepath.Dir(path))); err == nil && os.SameFile(want, got) {
			return nil
		}
	}
	return fmt.Errorf("%s isn't in a corpus's directory under %s, testdata/<dir>/, so the corpus root's lock doesn't cover it.\nPass the corpus root the archive is under.", path, dir)
}
