package archive

import (
	"bytes"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
)

// The sections of a variant below its name, beside in/ and out/: about
// describes a hand-written variant, and delete lists the base's paths a
// variant removes.
const (
	About  = "about"
	Delete = "delete"
)

// A VariantCase is a case an archive holds beside its base: a hand-written
// variant, under variant/<name>/, or a transform's, under
// transform/<name>/.
type VariantCase struct {
	Kind string // Variant or Transform
	Name string
	// Overlay holds the variant's in/ files, by their paths in the input
	// view, and Delete the base's paths it removes, in the order written.
	// EmptyDelete records a delete section that lists no path.
	Overlay     map[string][]byte
	Delete      []string
	EmptyDelete bool
	// Inputs records that the archive holds the variant's in/ or delete. A
	// transform's variant without them is one its transform no longer
	// makes, whose outputs are left.
	Inputs bool
	// About, Properties and Gaps are a hand-written variant's own. A
	// transform's variant has none of them.
	About      []byte
	Properties []string
	Gaps       []Gap
}

// Case names the variant as the subtests and the report do, after its
// archive's name: variant/<name> or transform/<name>.
func (v VariantCase) Case() string { return v.Kind + "/" + v.Name }

// Prefix is the prefix of the variant's sections: variant/<name>/ or
// transform/<name>/.
func (v VariantCase) Prefix() string { return v.Case() + "/" }

// SplitCase splits a section's name into the case it belongs to and its
// name within the case. For the base's sections, kind and name are "".
func SplitCase(section string) (kind, name, rest string) {
	top, r, _ := strings.Cut(section, "/")
	if top == Variant || top == Transform {
		name, rest, _ = strings.Cut(r, "/")
		return top, name, rest
	}
	return "", "", section
}

// baseFiles returns the base's input view, as files by path.
func (a *Archive) baseFiles() map[string][]byte {
	files := map[string][]byte{}
	for _, s := range a.Sections {
		if kind, _, rest := SplitCase(s.Name); kind == "" {
			if p, ok := strings.CutPrefix(rest, In+"/"); ok {
				files[p] = s.Data
			}
		}
	}
	return files
}

// Variants returns the archive's variants: the hand-written ones in the
// order written, then the transforms' by name. It is meant for an archive
// [Archive.Check] passes.
func (a *Archive) Variants() []VariantCase {
	byCase := map[string]*VariantCase{}
	var order []string
	for _, kind := range []string{Variant, Transform} {
		var names []string
		for _, s := range a.Sections {
			k, name, _ := SplitCase(s.Name)
			if k == kind && byCase[kind+"/"+name] == nil {
				byCase[kind+"/"+name] = &VariantCase{Kind: kind, Name: name, Overlay: map[string][]byte{}}
				names = append(names, name)
			}
		}
		if kind == Transform {
			slices.Sort(names)
		} else {
			names = a.variantOrder()
		}
		for _, n := range names {
			order = append(order, kind+"/"+n)
		}
	}
	for _, s := range a.Sections {
		kind, name, rest := SplitCase(s.Name)
		v := byCase[kind+"/"+name]
		if v == nil {
			continue
		}
		switch top, _, _ := strings.Cut(rest, "/"); {
		case top == In:
			v.Overlay[strings.TrimPrefix(rest, In+"/")] = s.Data
			v.Inputs = true
		case rest == Delete:
			v.Delete = lines(s.Data)
			v.EmptyDelete = len(v.Delete) == 0
			v.Inputs = true
		case rest == About:
			v.About = s.Data
		case rest == Properties:
			v.Properties, _ = ParsePropertiesIn(s.Name, s.Data)
		case rest == Gaps:
			v.Gaps, _ = ParseGapsIn(s.Name, s.Data)
		}
	}
	vs := make([]VariantCase, 0, len(order))
	for _, c := range order {
		vs = append(vs, *byCase[c])
	}
	return vs
}

// variantOrder returns the names of the hand-written variants: those with
// hand-written sections in the order of their first, then any others, which
// hold only update-written sections, by name.
func (a *Archive) variantOrder() []string {
	var names []string
	seen := map[string]bool{}
	for _, s := range a.Sections {
		if kind, name, _ := SplitCase(s.Name); kind == Variant && HandWritten(s.Name) && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	var rest []string
	for _, s := range a.Sections {
		if kind, name, _ := SplitCase(s.Name); kind == Variant && !seen[name] {
			seen[name] = true
			rest = append(rest, name)
		}
	}
	slices.Sort(rest)
	return append(names, rest...)
}

// A Base is an archive's base input view, read once, from which the views
// of its variants and the sections that record them are made.
type Base struct {
	files map[string][]byte
}

// Base reads the archive's base input view.
func (a *Archive) Base() Base { return Base{files: a.baseFiles()} }

// View returns the base's input view.
func (b Base) View() fs.FS { return NewFS(b.files) }

// VariantView returns a variant's input view: the base's, with the overlay
// laid over it path by path and the deleted paths removed.
func (b Base) VariantView(v VariantCase) fs.FS {
	return NewFS(variantFiles(b.files, v))
}

func variantFiles(base map[string][]byte, v VariantCase) map[string][]byte {
	files := maps.Clone(base)
	for p, data := range v.Overlay {
		files[p] = data
	}
	for _, p := range v.Delete {
		delete(files, p)
	}
	return files
}

// VariantSections returns the sections that record a variant of the base
// whose input view is view: its in/ sections, for each path whose bytes
// differ from the base's or that the base lacks, sorted, then its delete
// section, for the base's paths the view lacks. Equal views give no
// sections.
func (b Base) VariantSections(kind, name string, view map[string][]byte) []Section {
	base := b.files
	prefix := kind + "/" + name + "/"
	var ss []Section
	for _, p := range slices.Sorted(maps.Keys(view)) {
		if b, ok := base[p]; !ok || !bytes.Equal(b, view[p]) {
			ss = append(ss, Section{Name: prefix + In + "/" + p, Data: view[p]})
		}
	}
	var gone []string
	for _, p := range slices.Sorted(maps.Keys(base)) {
		if _, ok := view[p]; !ok {
			gone = append(gone, p)
		}
	}
	if len(gone) > 0 {
		ss = append(ss, Section{Name: prefix + Delete, Data: []byte(strings.Join(gone, "\n") + "\n")})
	}
	return ss
}

// checkVariant reports the problems of one section of a variant: rest is
// its name below variant/<name>/ or transform/<name>/.
func checkVariant(s Section, kind, name, rest string) []Problem {
	if !ValidName(name) {
		return []Problem{{Section: s.Name, Msg: fmt.Sprintf("the variant's name, %q, breaks the naming rule: %s", name, NameRule)}}
	}
	top, below, _ := strings.Cut(rest, "/")
	switch {
	case rest == "":
		return []Problem{{Section: s.Name, Msg: fmt.Sprintf("a variant's section belongs under its name, as %s/<name>/<section>", kind)}}
	case top == In:
		return checkIn(s.Name, below)
	case top == Out:
		return checkOut(s.Name, below)
	case top == Via:
		return []Problem{{Section: s.Name, Msg: "the name via is reserved for a later step, and this hypercase doesn't read it yet"}}
	case rest == Delete:
		return nil // checked against the base, with the variant's in/
	case kind == Variant && rest == About:
		return nil
	case kind == Variant && rest == Properties:
		_, ps := ParsePropertiesIn(s.Name, s.Data)
		return ps
	case kind == Variant && rest == Gaps:
		_, ps := ParseGapsIn(s.Name, s.Data)
		return ps
	case kind == Variant && (top == About || top == Properties || top == Gaps || top == Delete):
		return []Problem{{Section: s.Name, Msg: fmt.Sprintf("%s is one section, with nothing below it", top)}}
	case kind == Variant:
		return []Problem{{Section: s.Name, Msg: fmt.Sprintf("%s is not one of a hand-written variant's sections: about, in, delete, properties, gaps and out", top)}}
	case top == Delete:
		return []Problem{{Section: s.Name, Msg: "delete is one section, with nothing below it"}}
	}
	return []Problem{{Section: s.Name, Msg: fmt.Sprintf("%s is not one of a transform's sections: in, delete and out, which an update writes", top)}}
}

// checkVariants reports the problems of the variants as wholes: a
// hand-written variant's delete line the base can't satisfy, and its view
// equal to the base's or one no file tree holds; and hand-written sections
// that don't sit with their case's. A transform's in/ and delete are
// update-written: how they differ from what the base now gives, and
// whether they make an input view at all, is for the runs to judge and the
// update of the package that owns the transform to rewrite, never a
// problem of form, which no update repairs. See [Base.InputView].
func (a *Archive) checkVariants() []Problem {
	var ps []Problem
	base := a.baseFiles()
	for _, v := range a.Variants() {
		if !ValidName(v.Name) || v.Kind == Transform {
			continue
		}
		if dps := checkDelete(v, base); len(dps) > 0 {
			ps = append(ps, dps...)
			continue // a delete it can't satisfy
		}
		view := variantFiles(base, v)
		if maps.EqualFunc(view, base, bytes.Equal) {
			ps = append(ps, Problem{Section: v.Case(), Msg: "the variant's input view equals the base's; a variant changes a file or two of the base"})
		}
		ps = append(ps, checkNested(v, view)...)
	}

	// Each case's hand-written sections sit together, the base's first, so
	// an update never moves one.
	seen := map[string]bool{}
	last := ""
	for _, s := range a.Sections {
		if !HandWritten(s.Name) {
			continue
		}
		kind, name, _ := SplitCase(s.Name)
		c := ""
		if kind != "" {
			c = kind + "/" + name
		}
		switch {
		case c == last:
		case seen[c] && c == "":
			ps = append(ps, Problem{Section: s.Name, Msg: "the base's hand-written section sits after a variant's; the base's hand-written sections come first"})
		case seen[c]:
			ps = append(ps, Problem{Section: s.Name, Msg: fmt.Sprintf("the section sits apart from %s's other hand-written sections; each case's hand-written sections sit together", c)})
		}
		seen[c], last = true, c
	}
	return ps
}

// InputView returns the input view the committed inputs of a transform's
// variant, v, make: the base's, with the inputs laid over it and the
// deleted paths removed. If they make none, it returns what leaves them
// none instead: a delete section that is empty, or that holds a line that
// is no path of the view or a path twice, and a view that no file tree
// holds. A delete line naming a path the base lacks, or one the inputs lay
// over, still leaves a view, which the package that owns the transform
// judges against what the transform makes.
func (b Base) InputView(v VariantCase) (fs.FS, []Problem) {
	if ps := checkDelete(v, b.files); len(ps) > 0 {
		return nil, ps
	}
	files := variantFiles(b.files, v)
	if ps := checkNested(v, files); len(ps) > 0 {
		return nil, ps
	}
	return NewFS(files), nil
}

// checkDelete reports the problems of a variant's delete section, if it
// has one: an empty section, and each line that is no path of the input
// view or that repeats one. For a hand-written variant, it also reports
// each line the base can't satisfy: a path the base lacks, or one the
// variant lays over it.
func checkDelete(v VariantCase, base map[string][]byte) []Problem {
	var ps []Problem
	section := v.Prefix() + Delete
	seen := map[string]int{}
	if v.EmptyDelete {
		ps = append(ps, Problem{Section: section, Msg: "the section is empty; a variant that removes nothing has no delete section"})
	}
	for i, p := range v.Delete {
		n := i + 1
		_, overlaid := v.Overlay[p]
		_, inBase := base[p]
		switch {
		case !fs.ValidPath(p) || p == ".":
			ps = append(ps, Problem{Section: section, Line: n, Msg: fmt.Sprintf("%q is not a path in the input view", p)})
		case seen[p] > 0:
			ps = append(ps, Problem{Section: section, Line: n, Msg: fmt.Sprintf("%s is listed twice, on lines %d and %d", p, seen[p], n)})
		case v.Kind == Variant && !inBase:
			ps = append(ps, Problem{Section: section, Line: n, Msg: fmt.Sprintf("the base has no %s to remove", p)})
		case v.Kind == Variant && overlaid:
			ps = append(ps, Problem{Section: section, Line: n, Msg: fmt.Sprintf("the variant both lays %s over the base and removes it", p)})
		}
		seen[p] = n
	}
	return ps
}

// checkNested reports each path of a variant's input view that is the
// directory of another.
func checkNested(v VariantCase, view map[string][]byte) []Problem {
	var ps []Problem
	for _, n := range Nested(view) {
		ps = append(ps, Problem{Section: v.Case(), Msg: "the variant's input view holds " + n})
	}
	return ps
}

// Nested reports each path of an input view that is the directory of
// another, as "<path>, and <other> below it", by the other's path. No file
// tree holds such a view.
func Nested(view map[string][]byte) []string {
	var ns []string
	for _, p := range slices.Sorted(maps.Keys(view)) {
		for d := path.Dir(p); d != "."; d = path.Dir(d) {
			if _, ok := view[d]; ok {
				ns = append(ns, fmt.Sprintf("%s, and %s below it", d, p))
				break
			}
		}
	}
	return ns
}

// VariantDescriptions checks each hand-written variant's about, which
// describes it.
func (a *Archive) VariantDescriptions() []Problem {
	var ps []Problem
	for _, v := range a.Variants() {
		if v.Kind != Variant {
			continue
		}
		if len(bytes.TrimSpace(v.About)) == 0 {
			ps = append(ps, Problem{Section: v.Prefix() + About, Msg: "the variant has no description: its about section is missing or empty"})
			continue
		}
		for _, p := range CheckDescription(v.About) {
			p.Section = v.Prefix() + About
			p.Msg = strings.Replace(p.Msg, "of the comment", "of the about section", 1)
			p.Msg = strings.Replace(p.Msg, "the comment is prose", "a description is prose", 1)
			ps = append(ps, p)
		}
	}
	return ps
}
