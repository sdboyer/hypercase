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
		for _, list := range []struct {
			verb string
			ps   []decl.Property
		}{{"gives", tr.gives}, {"keeps", tr.keeps}} {
			for j, p := range list.ps {
				for _, q := range list.ps[:j] {
					if p.Name == q.Name {
						refuse(fmt.Sprintf("the transform %s %s %s twice. List each property once.", tr.name, list.verb, p.Name))
					}
				}
			}
		}
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
			for _, list := range []struct {
				verb, field string
				ps          []decl.Property
			}{{"gives", "Gives", tr.gives}, {"keeps", "Keeps", tr.keeps}} {
				for _, p := range list.ps {
					q, found := reg.property(p.Name)
					switch {
					case !found:
						t.Errorf("the transform %s, of the corpus type %s, %s %s, and the package registers no property of that name.\nRegister the property, or remove it from %s's %s.", tr.name, ct.goType, list.verb, p.Name, tr.name, list.field)
						ok = false
					case q != p:
						t.Errorf("the transform %s %s a property named %s, which the package registers with another description.\nList the registered property: declare it once, and use that declaration in both places.", tr.name, list.verb, p.Name)
						ok = false
					}
				}
			}
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
	view       fs.FS // a hand-written variant's

	// A transform's variant.
	tr       *transform
	declined string            // why the transform declines the base
	problem  string            // what went wrong making the variant
	made     map[string][]byte // the input view the transform made
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
// order written, then one per transform of its corpus type, by name.
func (r *runner) planVariants(p *plan, a *archive.Archive) []*plannedVariant {
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
			view:       a.VariantView(v),
		})
	}
	trs := slices.Clone(p.root.transforms)
	slices.SortFunc(trs, func(a, b *transform) int { return strings.Compare(a.name, b.name) })
	base := a.Properties()
	for _, tr := range trs {
		v := &plannedVariant{
			kind:      archive.Transform,
			name:      tr.name,
			about:     []byte(tr.description),
			gapsAt:    archive.Gaps,
			committed: committed[tr.name],
			tr:        tr,
		}
		for _, g := range a.Gaps() {
			if g.Transform == tr.name {
				v.gaps = append(v.gaps, g)
			}
		}
		for _, p := range tr.gives {
			v.properties = append(v.properties, p.Name)
		}
		var neither []string
		for _, name := range base {
			switch {
			case slices.ContainsFunc(tr.keeps, func(p decl.Property) bool { return p.Name == name }):
				if !slices.Contains(v.properties, name) {
					v.properties = append(v.properties, name)
				}
			case !slices.ContainsFunc(tr.gives, func(p decl.Property) bool { return p.Name == name }):
				neither = append(neither, name)
			}
		}
		if len(neither) > 0 {
			v.declined = fmt.Sprintf("the base states %s, which %s neither gives nor keeps", andList(neither), tr.name)
		} else {
			r.makeVariant(v, a)
		}
		vs = append(vs, v)
	}
	return vs
}

// makeVariant runs a transform on a base: its probe, then its jenny.
func (r *runner) makeVariant(v *plannedVariant, a *archive.Archive) {
	tr := v.tr
	view := a.View()
	if tr.probe != nil {
		var why string
		var ok bool
		c := call(func(*called) { why, ok = tr.probe(view) })
		switch {
		case c.panicked:
			v.problem = fmt.Sprintf("%s's probe panicked on the base: %v\nFix the probe.", tr.name, c.panicValue)
			return
		case !ok && strings.TrimSpace(why) == "":
			v.problem = fmt.Sprintf("%s's probe said the base isn't for it, and gave no reason.\nFix the probe: when it says no, it says why.", tr.name)
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
		v.problem = fmt.Sprintf("%s panicked on the base: %v\nFix its jenny.", tr.name, c.panicValue)
		return
	case err != nil:
		v.problem = fmt.Sprintf("%s failed on the base: %v\nFix its jenny, or make its probe decline such a base.", tr.name, err)
		return
	}
	files := map[string][]byte{}
	var held []string
	for _, f := range made.AsFiles() {
		data, err := f.Render()
		if err != nil {
			v.problem = fmt.Sprintf("%s made %s, which doesn't render: %v\nFix its jenny.", tr.name, f.Path(), err)
			return
		}
		if err := archive.Holds(data); err != nil {
			held = append(held, fmt.Sprintf("  %s: %v", f.Path(), err))
		}
		files[f.Path()] = data
	}
	if len(held) > 0 {
		v.problem = fmt.Sprintf("%s made files an archive can't hold:\n%s\nFix its jenny.", tr.name, strings.Join(held, "\n"))
		return
	}
	if ns := archive.Nested(files); len(ns) > 0 {
		v.problem = fmt.Sprintf("%s made an input view no file tree holds: it holds %s.\nFix its jenny.", tr.name, strings.Join(ns, "; "))
		return
	}
	sections := a.VariantSections(archive.Transform, tr.name, files)
	if len(sections) == 0 {
		v.problem = fmt.Sprintf("%s made an input view equal to the base's. A variant changes the base.\nMake its probe decline such a base.", tr.name)
		return
	}
	// The variant's sections, beside the base's, are an archive of their
	// own, with no problem of form.
	scratch := &archive.Archive{}
	for _, s := range a.Sections {
		if kind, _, _ := archive.SplitCase(s.Name); kind == "" {
			scratch.Sections = append(scratch.Sections, s)
		}
	}
	scratch.Sections = append(scratch.Sections, sections...)
	if ps := scratch.Check(); len(ps) > 0 {
		v.problem = fmt.Sprintf("%s made a variant an archive can't hold:\n%s\nFix its jenny: what it makes is the variant's whole input view, with the trees as its top-level directories.", tr.name, problemList(ps))
		return
	}
	v.made = files
}

// madeSections are the sections that record the transform's variant.
func (v *plannedVariant) madeSections(a *archive.Archive) []archive.Section {
	return a.VariantSections(archive.Transform, v.name, v.made)
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
	switch {
	case v.tr == nil:
		c.setView(p, v.view)
	case v.problem != "" && owner:
		t.Errorf("%s\n%s", c.header(), v.problem)
		return
	case v.problem != "":
		t.Skipf("%s: the transform %s failed on the base, and the package that owns it reports why.", c.name, v.name)
	case v.declined != "":
		r.declined(t, c, p, pc, v, owner, w)
		return
	default:
		if owner {
			r.judgeInputs(t, c, pc, v, w)
		}
		// Only the owner's update runs the cells on what the transform
		// makes, as it writes those inputs. Every other run reads the
		// inputs the archive holds, so no package writes outputs of inputs
		// the archive doesn't hold; after the owner's update, the others'
		// next updates follow.
		if owner && r.update || !v.committed.Inputs {
			c.setView(p, archive.NewFS(v.made))
		} else {
			c.setView(p, pc.a.VariantView(v.committed))
		}
	}
	r.runCells(t, c, p.cols, w)
}

// judgeInputs compares a transform's committed inputs with those it made,
// as goldens, in the package that owns it. An update writes them whole.
func (r *runner) judgeInputs(t *testing.T, c *aCase, pc plannedCase, v *plannedVariant, w *caseWrites) {
	t.Helper()
	made := v.madeSections(pc.a)
	if r.update {
		w.replaceInputs(v.name, made)
		return
	}
	if !v.committed.Inputs {
		t.Errorf("%s\n%s's variant of the base isn't written: the archive holds none of its inputs.\nRun %s to write them, and read them before committing.", c.header(), v.name, r.updateCmd())
		return
	}
	committed := map[string][]byte{}
	for _, s := range v.inputs(pc.a) {
		committed[s.Name] = s.Data
	}
	m := &msg{}
	for _, s := range made {
		g, ok := committed[s.Name]
		switch {
		case !ok:
			m.add("%s is new", s.Name)
		case !bytes.Equal(g, s.Data):
			m.add("%s differs (-committed +made):\n%s", s.Name, strings.TrimSuffix(diff.Lines(g, s.Data), "\n"))
		}
		delete(committed, s.Name)
	}
	for _, name := range slices.Sorted(maps.Keys(committed)) {
		m.add("%s is no longer made", name)
	}
	if !m.empty() {
		t.Errorf("%s\n%s's variant of the base differs from its committed inputs:\n%s\nIf the change is intended, run %s and read the diff before committing.", c.header(), v.name, m, r.updateCmd())
	}
}

// declined finishes a transform's variant that the transform declines:
// no cell runs. Its owner's update removes the variant's inputs, and what
// is left under transform/<name>/out/ is each column's orphan.
func (r *runner) declined(t *testing.T, c *aCase, p *plan, pc plannedCase, v *plannedVariant, owner bool, w *caseWrites) {
	t.Helper()
	m := &msg{}
	if owner {
		if r.update {
			w.replaceInputs(v.name, nil)
		} else if ins := v.inputs(pc.a); len(ins) > 0 {
			var names []string
			for _, s := range ins {
				names = append(names, s.Name)
			}
			m.add("%s %s written, and %s declines the base: %s\nRun %s to remove them.", andList(names), isAre(len(names)), v.name, v.declined, r.updateCmd())
		}
	}
	for _, col := range p.cols {
		if g, ok := c.gap(col.name); ok {
			m.add("%s marks the cell of %s, and %s declines the base: %s\nRemove the line, or fix the transform.", c.gapAt(g), g.Cell(), v.name, v.declined)
		}
		for _, f := range slices.Sorted(maps.Keys(c.goldens(col.name))) {
			m.add("%s is recorded, and %s declines the base: %s\nIf the transform should make the variant, fix it. If not, delete the section by hand; %s leaves it in place.", c.out(col.name, f), v.name, v.declined, r.updateCmd())
		}
	}
	if !m.empty() {
		t.Errorf("%s\n%s", c.header(), m)
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
