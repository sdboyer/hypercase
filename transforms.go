package hypercase

import (
	"bytes"
	"fmt"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/diff"
	"github.com/sdboyer/hypercase/internal/erase"
	"github.com/sdboyer/hypercase/internal/registry"
	"github.com/sdboyer/jenny"
)

// A transform is one of a registered corpus type's decl.Transform values.
type transform struct {
	name        string
	description string
	gives       []decl.Property
	keeps       []decl.Property
	goType      reflect.Type // the corpus type it is of
	probe       func(view fs.FS) (string, bool)
	generate    func(view fs.FS) (*jenny.FS, error)
}

func newTransform(e *erase.Transform) *transform {
	tr := &transform{
		name:        e.Name,
		description: e.Description,
		goType:      e.GoType,
		probe:       e.Probe,
		generate:    e.Generate,
	}
	for _, p := range e.Gives {
		tr.gives = append(tr.gives, decl.Property(p))
	}
	for _, p := range e.Keeps {
		tr.keeps = append(tr.keeps, decl.Property(p))
	}
	return tr
}

// sameDeclaration reports whether two transforms are one declaration, as
// far as anything but their functions shows.
func (tr *transform) sameDeclaration(o *transform) bool {
	return tr.name == o.name && tr.description == o.description && tr.goType == o.goType &&
		slices.Equal(tr.gives, o.gives) && slices.Equal(tr.keeps, o.keeps) &&
		(tr.probe == nil) == (o.probe == nil) && (tr.generate == nil) == (o.generate == nil)
}

// checkTransforms refuses a corpus type's transforms that break a rule of
// their own, as the type is registered.
func checkTransforms(t *corpusType) {
	for i, tr := range t.transforms {
		if !validName(tr.name) {
			refuse(fmt.Sprintf("the corpus type %s lists a transform named %q, which breaks the naming rule: %s. Rename the transform.", t.goType, tr.name, nameRule))
		}
		for _, o := range t.transforms[:i] {
			if o.name == tr.name {
				refuse(fmt.Sprintf("the corpus type %s lists two transforms named %s. Transform names are unique per corpus type; rename one.", t.goType, tr.name))
			}
		}
		if tr.generate == nil {
			refuse(fmt.Sprintf("the transform %s, of the corpus type %s, has no Jenny. Give it the jenny that makes its variant's input view from the base's.", tr.name, t.goType))
		}
		checkListed("the transform "+tr.name, "Gives", tr.gives)
		checkListed("the transform "+tr.name, "Keeps", tr.keeps)
		for _, p := range tr.gives {
			for _, q := range tr.keeps {
				if p.Name == q.Name {
					refuse(fmt.Sprintf("the transform %s both gives and keeps %s. Every variant has a property the transform gives: list it in Gives alone.", tr.name, p.Name))
				}
			}
		}
	}
}

// ownTransform registers a transform the package owns. Whether a corpus
// type the package registers lists it waits until initialization is over.
func (r *registrations) ownTransform(tr *transform) {
	for _, o := range r.ownedTransforms {
		if o.goType == tr.goType && o.name == tr.name {
			refuse(fmt.Sprintf("the transform %s is given to hypercase.Own twice. Own it once.", tr.name))
		}
	}
	r.ownedTransforms = append(r.ownedTransforms, tr)
}

// checkTransformRelations checks what relates a transform to the rest of
// the registrations, once initialization is over: each transform the
// package owns is one a corpus type it registers lists, and each property
// a transform gives or keeps is registered. It records which of the
// types' transforms the package owns.
func (r *runner) checkTransformRelations(t *testing.T, owned []*transform) bool {
	t.Helper()
	ok := true
	r.owned = map[*transform]bool{}
	for _, o := range owned {
		var ct *corpusType
		for _, c := range r.types {
			if c.goType == o.goType {
				ct = c
			}
		}
		if ct == nil {
			t.Errorf("the package owns the transform %s, of the corpus type %s, and registers no such corpus type.\nRegister the corpus type, which lists the transform in its Transforms.", o.name, o.goType)
			ok = false
			continue
		}
		i := slices.IndexFunc(ct.transforms, func(tr *transform) bool { return tr.name == o.name })
		switch {
		case i < 0:
			t.Errorf("the package owns the transform %s, and the corpus type %s lists no transform of that name.\nList the transform in the type's Transforms, or remove it from hypercase.Own.", o.name, ct.goType)
			ok = false
		case !ct.transforms[i].sameDeclaration(o):
			t.Errorf("the package owns a transform named %s that differs from the one the corpus type %s lists.\nOwn the declaration the type lists: declare the transform once, and use that declaration in both places.", o.name, ct.goType)
			ok = false
		default:
			r.owned[ct.transforms[i]] = true
		}
	}
	for _, ct := range r.types {
		for _, tr := range ct.transforms {
			what := fmt.Sprintf("the transform %s, of the corpus type %s,", tr.name, ct.goType)
			ok = checkRegistered(t, what, "gives", tr.name+"'s Gives", tr.gives) && ok
			ok = checkRegistered(t, what, "keeps", tr.name+"'s Keeps", tr.keeps) && ok
		}
	}
	return ok
}

// A plannedVariant is a variant of an archive's base, with what its cells
// run on. A transform's is made when its archive is read, so a transform
// runs once per base, outside the subtests.
type plannedVariant struct {
	kind, name string
	about      []byte
	gapsAt     string // the section that holds the variant's gaps lines
	properties []string
	gaps       []archive.Gap
	committed  archive.VariantCase

	// A transform's variant.
	tr       *transform
	declined string            // why the transform declines the base
	problem  string            // what went wrong making the variant
	fix      string            // what to do about it
	made     map[string][]byte // the input view the transform made
	sections []archive.Section // the sections that record it
}

func (v *plannedVariant) caseName() string { return v.kind + "/" + v.name }

// states says what the transform's variants state, for a message about
// what one of them states.
func (tr *transform) states() string {
	return fmt.Sprintf("The variant states what %s gives, and each property it keeps that the base states.", tr.name)
}

// runs reports whether the variant's cells run.
func (v *plannedVariant) runs() bool {
	return v.tr == nil || v.declined == "" && v.problem == ""
}

// planVariants plans an archive's variants: its hand-written ones, in the
// order written, then one per transform of its corpus type, by name. Each
// transform runs here, on the base's input view, since whether its variant
// has cells sets the count of each suite's cells, which doesn't depend on
// -run. It makes the view if a transform runs, and drops it once the
// archive is planned; runCase makes it again for the base's cells.
func (r *runner) planVariants(p *plan, a *archive.Archive, base archive.Base) []*plannedVariant {
	var vs []*plannedVariant
	committed := map[string]archive.VariantCase{}
	for _, v := range a.Variants() {
		if v.Kind == archive.Transform {
			committed[v.Name] = v
			continue
		}
		vs = append(vs, &plannedVariant{
			kind:       archive.Variant,
			name:       v.Name,
			about:      v.About,
			gapsAt:     v.Prefix() + archive.Gaps,
			properties: v.Properties,
			gaps:       v.Gaps,
			committed:  v,
		})
	}
	trs := slices.Clone(p.root.transforms)
	slices.SortFunc(trs, func(a, b *transform) int { return strings.Compare(a.name, b.name) })
	stated, gaps := a.Properties(), a.Gaps()
	var view fs.FS
	for _, tr := range trs {
		v := &plannedVariant{
			kind:      archive.Transform,
			name:      tr.name,
			about:     []byte(tr.description),
			gapsAt:    archive.Gaps,
			committed: committed[tr.name],
			tr:        tr,
		}
		for _, g := range gaps {
			if g.Transform == tr.name {
				v.gaps = append(v.gaps, g)
			}
		}
		var neither []string
		v.properties, neither = registry.VariantProperties(propertyNames(tr.gives), propertyNames(tr.keeps), stated)
		if len(neither) > 0 {
			v.declined = fmt.Sprintf("the base states %s, which %s neither gives nor keeps", andList(neither), tr.name)
		} else {
			if view == nil {
				view = base.View()
			}
			r.makeVariant(v, base, view)
		}
		vs = append(vs, v)
	}
	return vs
}

// makeVariant runs a transform on a base, whose input view is view: its
// probe, then its jenny.
func (r *runner) makeVariant(v *plannedVariant, base archive.Base, view fs.FS) {
	tr := v.tr
	if tr.probe != nil {
		var why string
		var ok bool
		c := call(func(*called) { why, ok = tr.probe(view) })
		switch {
		case c.panicked:
			v.problem, v.fix = fmt.Sprintf("%s's probe panicked on the base: %v", tr.name, c.panicValue), "Fix the probe."
			return
		case !ok && strings.TrimSpace(why) == "":
			v.problem, v.fix = fmt.Sprintf("%s's probe said the base isn't for it, and gave no reason.", tr.name), "Fix the probe: when it says no, it says why."
			return
		case !ok:
			v.declined = why
			return
		}
	}
	var made *jenny.FS
	var err error
	c := call(func(*called) { made, err = tr.generate(view) })
	switch {
	case c.panicked:
		v.problem, v.fix = fmt.Sprintf("%s panicked on the base: %v", tr.name, c.panicValue), "Fix its jenny."
		return
	case err != nil:
		v.problem, v.fix = fmt.Sprintf("%s failed on the base: %v", tr.name, err), "Fix its jenny, or make its probe decline such a base."
		return
	}
	files := map[string][]byte{}
	var held []string
	for _, f := range made.AsFiles() {
		// A jenny's FS renders each file it adds, and AsFiles gives each
		// one as Bytes, so no transform reaches this, and no test does: it
		// guards against a change to jenny, which would otherwise record
		// an unrendered file as empty bytes.
		data, ok := jenny.BytesOf(f)
		if !ok {
			v.problem, v.fix = fmt.Sprintf("%s made %s, which its FS gave as a %T, not as rendered bytes.", tr.name, f.Path(), f), "An FS renders each file it adds, so this is a bug in hypercase or jenny: report it to hypercase."
			return
		}
		if err := archive.Holds(data); err != nil {
			held = append(held, fmt.Sprintf("  %s: %v", f.Path(), err))
		}
		files[f.Path()] = data
	}
	if len(held) > 0 {
		v.problem, v.fix = fmt.Sprintf("%s made files an archive can't hold:\n%s", tr.name, strings.Join(held, "\n")), "Fix its jenny."
		return
	}
	if ns := archive.Nested(files); len(ns) > 0 {
		v.problem, v.fix = fmt.Sprintf("%s made an input view no file tree holds: it holds %s.", tr.name, strings.Join(ns, "; ")), "Fix its jenny."
		return
	}
	sections := base.VariantSections(archive.Transform, tr.name, files)
	if len(sections) == 0 {
		v.problem, v.fix = fmt.Sprintf("%s made an input view equal to the base's. A variant changes the base.", tr.name), "Make its probe decline such a base."
		return
	}
	// The variant's sections, as an archive of their own, have no problem
	// of form. The base's are left out: the archive's check passed them,
	// and it reads a transform's variant apart from the base.
	if ps := (&archive.Archive{Sections: sections}).Check(); len(ps) > 0 {
		v.problem, v.fix = fmt.Sprintf("%s made a variant an archive can't hold:\n%s", tr.name, problemList(ps)), "Fix its jenny: what it makes is the variant's whole input view, with the trees as its top-level directories."
		return
	}
	v.made, v.sections = files, sections
}

// inputs returns the committed sections that record a transform's
// variant: its in/ and delete.
func (v *plannedVariant) inputs(a *archive.Archive) []archive.Section {
	var ss []archive.Section
	prefix := archive.Transform + "/" + v.name + "/"
	for _, s := range a.Sections {
		if strings.HasPrefix(s.Name, prefix+archive.In+"/") || s.Name == prefix+archive.Delete {
			ss = append(ss, s)
		}
	}
	return ss
}

// runVariant runs one variant's cells, and, in the package that owns its
// transform, judges the variant's inputs.
func (r *runner) runVariant(t *testing.T, p *plan, pc plannedCase, v *plannedVariant, w *caseWrites) {
	c := r.newCase(p, pc, v.kind+"/"+v.name+"/", v.about, v.gapsAt, v.properties, v.gaps)
	c.tr = v.tr
	owner := v.tr != nil && r.owned[v.tr]
	// stale is whether a failing cell may be one the module's update
	// fixes: another package's normal run reads committed inputs that may
	// differ from what the transform makes. diffs says how they differ.
	stale := false
	var diffs string
	switch {
	case v.tr == nil:
		c.setView(r, p, pc.base.VariantView(v.committed))
	case v.problem != "" && owner:
		var ns notes
		ns.add(aboutFault, v.problem, closing{advice: v.fix})
		report(t, c.header(), ns, r.variantStaging(c))
		return
	case v.problem != "":
		t.Skipf("%s: the transform %s failed on the base, and the package that owns it reports why.", c.name, v.name)
	case v.declined != "":
		r.declined(t, c, p, pc, v, owner, w)
		return
	case r.update || !v.committed.Inputs:
		// An update runs the cells on what the transform makes, in every
		// package: what it writes depends on the inputs and the code
		// alone, never on sections an update wrote.
		if owner {
			r.judgeInputs(t, c, pc, v, w)
		}
		c.setView(r, p, archive.NewFS(v.made))
	default:
		// A normal run reads the inputs the archive holds.
		view, invalid := pc.base.InputView(v.committed)
		if len(invalid) > 0 {
			// An update reads what the transform makes instead, and its
			// owner's rewrites them.
			var ns notes
			ns.add(aboutInputs, fmt.Sprintf("The archive's inputs for %s's variant are invalid, and no cell runs on them:\n%s", v.name, problemList(invalid)), closing{claims: r.rewritesInputs(v.tr)})
			report(t, c.header(), ns, r.variantStaging(c))
			return
		}
		if owner {
			r.judgeInputs(t, c, pc, v, w)
		}
		c.setView(r, p, view)
		// A cell the inputs block may be one the update, which stages the
		// variant on what the transform makes, runs. Inputs that differ
		// only in a delete line naming a path the base lacks give the same
		// view, on which the update runs each cell as this run does, so no
		// failure is theirs.
		if diffs = v.inputDiffs(pc.a); diffs != "" && !holdsExactly(view, v.made) {
			c.state.made = archive.NewFS(v.made)
			stale = !owner
		}
	}
	r.runCells(t, c, p.cols, w)
	if !stale || !t.Failed() {
		return
	}
	// However its cells failed, the failures may be the inputs'. Each
	// cell's notes say what the update does with it.
	var ns notes
	ns.add(aboutInputs, fmt.Sprintf("A cell of the variant failed, and the archive's inputs for %s's variant, which a normal run reads, differ from what %s makes now:\n%s", v.name, v.name, diffs), closing{claims: r.rewritesInputs(v.tr)})
	report(t, c.header(), ns, r.variantStaging(c))
}

// rewritesInputs is what a package's update does with a variant's inputs
// that differ from what the transform tr makes: in the package that owns
// tr, its update rewrites them, and in one that doesn't, the update of the
// package that owns it does, if a package does.
func (r *runner) rewritesInputs(tr *transform) claims {
	if r.owned[tr] {
		return claims{RewritesInputs: tr.name}
	}
	return claims{OwnerRewritesInputs: tr.name}
}

// inputDiffs says how the committed inputs of a transform's variant differ
// from those it made, one difference to a line, or "" when they don't.
func (v *plannedVariant) inputDiffs(a *archive.Archive) string {
	committed := map[string][]byte{}
	for _, s := range v.inputs(a) {
		committed[s.Name] = s.Data
	}
	var lines []string
	for _, s := range v.sections {
		g, ok := committed[s.Name]
		switch {
		case !ok:
			lines = append(lines, fmt.Sprintf("%s is new", s.Name))
		case !bytes.Equal(g, s.Data):
			lines = append(lines, fmt.Sprintf("%s differs (-committed +made):\n%s", s.Name, strings.TrimSuffix(diff.Lines(g, s.Data), "\n")))
		}
		delete(committed, s.Name)
	}
	for _, name := range slices.Sorted(maps.Keys(committed)) {
		lines = append(lines, fmt.Sprintf("%s is no longer made", name))
	}
	return strings.Join(lines, "\n")
}

// holdsExactly reports whether an input view holds files, and no other
// file.
func holdsExactly(view fs.FS, files map[string][]byte) bool {
	paths, err := archive.Paths(view)
	if err != nil || len(paths) != len(files) {
		return false
	}
	for _, p := range paths {
		data, err := fs.ReadFile(view, p)
		if want, ok := files[p]; err != nil || !ok || !bytes.Equal(data, want) {
			return false
		}
	}
	return true
}

// judgeInputs compares a transform's committed inputs with those it made,
// as goldens, in the package that owns it. An update writes them whole.
func (r *runner) judgeInputs(t *testing.T, c *aCase, pc plannedCase, v *plannedVariant, w *caseWrites) {
	t.Helper()
	if r.update {
		w.replaceInputs(v.name, v.sections)
		return
	}
	var ns notes
	if !v.committed.Inputs {
		ns.add(aboutInputs, fmt.Sprintf("%s's variant of the base isn't written: the archive holds none of its inputs.", v.name), closing{claims: claims{WritesInputs: v.name}})
	} else if d := v.inputDiffs(pc.a); d != "" {
		ns.add(aboutInputs, fmt.Sprintf("%s's variant of the base differs from its committed inputs:\n%s", v.name, d), closing{advice: fmt.Sprintf("If the change is intended, run %s and read the diff before committing.", r.updateCmd())})
	}
	if len(ns) > 0 {
		report(t, c.header(), ns, r.variantStaging(c))
	}
}

// declined finishes a transform's variant that the transform declines:
// no cell runs. Its owner's update removes the variant's inputs, and what
// is left under transform/<name>/out/ is each column's orphan. A bytes
// lift has no product on the variant, so its owner's update removes the
// committed one, which other cells read, unless a gaps line marks its cell.
func (r *runner) declined(t *testing.T, c *aCase, p *plan, pc plannedCase, v *plannedVariant, owner bool, w *caseWrites) {
	t.Helper()
	var ns notes
	if owner {
		if r.update {
			w.replaceInputs(v.name, nil)
		} else if ins := v.inputs(pc.a); len(ins) > 0 {
			var names []string
			for _, s := range ins {
				names = append(names, s.Name)
			}
			ns.add(aboutInputs, fmt.Sprintf("%s %s written, and %s declines the base: %s", andList(names), isAre(len(names)), v.name, v.declined), closing{claims: claims{RemovesInputs: v.name}})
		}
	}
	for _, col := range p.cols {
		g, marked := c.gap(col.name)
		if marked {
			ns.add(aboutGap, fmt.Sprintf("%s marks the cell of %s, and %s declines the base: %s", c.gapAt(g), g.Cell(), v.name, v.declined), closing{advice: "Remove the line, or fix the transform."})
		}
		for _, f := range slices.Sorted(maps.Keys(c.goldens(col.name))) {
			ns.addFile(aboutOrphan, c.out(col.name, f), fmt.Sprintf("%s is recorded, and %s declines the base: %s", c.out(col.name, f), v.name, v.declined), closing{"If the transform should make the variant, fix it. If not, delete the section by hand;", claims{Leaves: c.out(col.name, f)}})
		}
		if col.lift == nil || !col.lift.bytes || marked {
			continue
		}
		dir := col.lift.viaDir(c.prefix)
		if r.update {
			w.replaceProduct(dir, nil)
			continue
		}
		for _, f := range slices.Sorted(maps.Keys(c.product(col.lift))) {
			ns.addFile(aboutProductOrphan, dir+f, fmt.Sprintf("%s is recorded, and %s declines the base: %s", dir+f, v.name, v.declined), closing{claims: claims{Removes: dir + f}})
		}
	}
	if len(ns) > 0 {
		report(t, c.header(), ns, r.declinedStaging(c))
		return
	}
	t.Skipf("%s: %s declines the base: %s", c.name, v.name, v.declined)
}

func isAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}
