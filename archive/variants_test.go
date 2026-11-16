package archive

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// twiceWithVariants is the walkthrough's twice.txtar at step 7, in place.
const twiceWithVariants = `A1 is twice B1.
-- in/sheet/A1 --
=B1*2
-- in/sheet/B1 --
3
-- gaps --
transform/renamed evaluate A2 prints as 6.0. Waits on sheets#9.
-- out/evaluate/values.txt --
A1 6
B1 3
-- variant/cycle/about --
B1 refers back to A1, so the sheet has a cycle.
-- variant/cycle/properties --
circular
-- variant/cycle/in/sheet/B1 --
=A1
-- variant/cycle/out/resolve/error.txt --
A1 -> B1 -> A1: cycle
-- variant/missing/about --
B1 is gone, so A1 refers to a cell that doesn't exist.
-- variant/missing/properties --
dangling
-- variant/missing/delete --
sheet/B1
-- variant/missing/out/resolve/error.txt --
A1: B1 doesn't exist
-- transform/looped/in/sheet/A1 --
=A1
-- transform/looped/out/resolve/error.txt --
A1 -> A1: cycle
-- transform/renamed/in/sheet/A2 --
=B2*2
-- transform/renamed/in/sheet/B2 --
3
-- transform/renamed/delete --
sheet/A1
sheet/B1
-- transform/renamed/out/evaluate/values.txt --
A2 6
B2 3
`

func TestVariantsWellFormed(t *testing.T) {
	a := Parse([]byte(twiceWithVariants))
	if ps := a.Check(); len(ps) > 0 {
		t.Fatalf("a well-formed archive with variants has problems: %v", ps)
	}
	if ps := a.VariantDescriptions(); len(ps) > 0 {
		t.Errorf("its variants' descriptions have problems: %v", ps)
	}
	if !Placed([]byte(twiceWithVariants)) {
		t.Errorf("it isn't placed; in place it is:\n%s", a.Place().Format())
	}
	var cases []string
	for _, v := range a.Variants() {
		cases = append(cases, v.Case())
	}
	if want := []string{"variant/cycle", "variant/missing", "transform/looped", "transform/renamed"}; !slices.Equal(cases, want) {
		t.Errorf("the variants are %q, want %q", cases, want)
	}
	vs := a.Variants()
	if got := vs[0].Properties; !slices.Equal(got, []string{"circular"}) {
		t.Errorf("variant/cycle states %q", got)
	}
	if got := a.Properties(); got != nil {
		t.Errorf("the base states %q: a variant's properties are its own", got)
	}
	gaps := a.Gaps()
	if len(gaps) != 1 || gaps[0].Transform != "renamed" || gaps[0].Column != "evaluate" || gaps[0].Text != "A2 prints as 6.0. Waits on sheets#9." {
		t.Errorf("the base's gaps are %+v", gaps)
	}
	if got := gaps[0].Cell(); got != "transform/renamed evaluate" {
		t.Errorf("the gap's cell is %q", got)
	}

	views := map[string]map[string]string{
		"variant/cycle":     {"sheet/A1": "=B1*2\n", "sheet/B1": "=A1\n"},
		"variant/missing":   {"sheet/A1": "=B1*2\n"},
		"transform/looped":  {"sheet/A1": "=A1\n", "sheet/B1": "3\n"},
		"transform/renamed": {"sheet/A2": "=B2*2\n", "sheet/B2": "3\n"},
	}
	base := a.Base()
	for _, v := range vs {
		want := views[v.Case()]
		view := base.VariantView(v)
		var paths []string
		for p := range want {
			paths = append(paths, p)
		}
		if err := fstest.TestFS(view, paths...); err != nil {
			t.Errorf("%s's view: %v", v.Case(), err)
		}
		got := map[string]string{}
		fs.WalkDir(view, ".", func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				data, _ := fs.ReadFile(view, p)
				got[p] = string(data)
			}
			return nil
		})
		if !mapsEqual(got, want) {
			t.Errorf("%s's view is %q, want %q", v.Case(), got, want)
		}
		// The sections that record the view are those the archive holds.
		files := map[string][]byte{}
		for p, d := range want {
			files[p] = []byte(d)
		}
		var names, held []string
		for _, s := range base.VariantSections(v.Kind, v.Name, files) {
			names = append(names, s.Name)
			if d, ok := a.Section(s.Name); !ok || string(d) != string(s.Data) {
				t.Errorf("%s: VariantSections gives %s as %q, and the archive holds %q", v.Case(), s.Name, s.Data, d)
			}
		}
		for _, s := range a.Sections {
			if strings.HasPrefix(s.Name, v.Prefix()+In+"/") || s.Name == v.Prefix()+Delete {
				held = append(held, s.Name)
			}
		}
		if !slices.Equal(names, held) {
			t.Errorf("%s: VariantSections names %q, and the archive's inputs are %q", v.Case(), names, held)
		}
	}
	if got := base.VariantSections(Transform, "same", map[string][]byte{"sheet/A1": []byte("=B1*2\n"), "sheet/B1": []byte("3\n")}); len(got) != 0 {
		t.Errorf("a view equal to the base's gives sections %v", got)
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

func TestSplitCase(t *testing.T) {
	for _, tt := range []struct{ section, kind, name, rest string }{
		{"in/sheet/A1", "", "", "in/sheet/A1"},
		{"gaps", "", "", "gaps"},
		{"variant/cycle/in/sheet/B1", Variant, "cycle", "in/sheet/B1"},
		{"variant/cycle/about", Variant, "cycle", "about"},
		{"transform/looped/delete", Transform, "looped", "delete"},
		{"transform/looped", Transform, "looped", ""},
		{"variant", Variant, "", ""},
	} {
		kind, name, rest := SplitCase(tt.section)
		if kind != tt.kind || name != tt.name || rest != tt.rest {
			t.Errorf("SplitCase(%q) = %q, %q, %q, want %q, %q, %q", tt.section, kind, name, rest, tt.kind, tt.name, tt.rest)
		}
	}
}

func TestHandWritten(t *testing.T) {
	for name, want := range map[string]bool{
		"in/sheet/A1":                   true,
		"properties":                    true,
		"gaps":                          true,
		"out/calc/values.txt":           false,
		"via/parse/sheet.json":          false,
		"variant/cycle/about":           true,
		"variant/cycle/in/sheet/B1":     true,
		"variant/cycle/delete":          true,
		"variant/cycle/properties":      true,
		"variant/cycle/gaps":            true,
		"variant/cycle/out/calc/x":      false,
		"variant/cycle/via/parse/x":     false,
		"transform/looped/in/sheet/A1":  false,
		"transform/looped/delete":       false,
		"transform/looped/out/calc/x":   false,
		"transform/looped/via/parse/x":  false,
		"transform/looped/anything/odd": false,
	} {
		if got := HandWritten(name); got != want {
			t.Errorf("HandWritten(%q) = %v, want %v", name, got, want)
		}
	}
}

// TestVariantProblems: each problem of a variant's form, named by its
// section.
func TestVariantProblems(t *testing.T) {
	const base = "A case.\n-- in/sheet/A1 --\n=B1*2\n-- in/sheet/B1 --\n3\n"
	for _, tt := range []struct {
		name, sections string
		want           []string
	}{
		{"no name", "-- variant/about --\nx\n", []string{"variant/about: a variant's section belongs under its name, as variant/<name>/<section>"}},
		{"bad name", "-- variant/Cycle/about --\nx\n", []string{`variant/Cycle/about: the variant's name, "Cycle", breaks the naming rule`}},
		{"transform bad name", "-- transform/x_y/in/sheet/A1 --\n=A1\n", []string{`transform/x_y/in/sheet/A1: the variant's name, "x_y", breaks the naming rule`}},
		{"unknown section", "-- variant/cycle/notes --\nx\n", []string{"variant/cycle/notes: notes is not one of a hand-written variant's sections: about, in, delete, properties, gaps and out"}},
		{"unknown transform section", "-- transform/looped/about --\nx\n", []string{"transform/looped/about: about is not one of a transform's sections: in, delete and out, which an update writes"}},
		{"transform properties", "-- transform/looped/properties --\ncircular\n", []string{"transform/looped/properties: properties is not one of a transform's sections"}},
		{"about below", "-- variant/cycle/about/x --\nx\n", []string{"variant/cycle/about/x: about is one section, with nothing below it"}},
		{"via", "-- variant/cycle/in/sheet/B1 --\n=A1\n-- variant/cycle/via/parse/x --\nx\n", []string{"variant/cycle/via/parse/x: the name via is reserved for a later step"}},
		{"input with no tree", "-- variant/cycle/in/B1 --\n=A1\n", []string{"variant/cycle/in/B1: an input belongs in a tree, as variant/cycle/in/<tree>/<file>"}},
		{"params", "-- variant/cycle/in/params/x --\n1\n", []string{"variant/cycle/in/params/x: the tree name params is reserved"}},
		{"output with no column", "-- variant/cycle/in/sheet/B1 --\n=A1\n-- variant/cycle/out/x --\n1\n", []string{"variant/cycle/out/x: an output belongs under its column, as variant/cycle/out/<column>/<file>"}},
		{"bad properties", "-- variant/cycle/in/sheet/B1 --\n=A1\n-- variant/cycle/properties --\nCircular\n", []string{`variant/cycle/properties, line 1: "Circular" is not one property's name`}},
		{"empty properties", "-- variant/cycle/in/sheet/B1 --\n=A1\n-- variant/cycle/properties --\n", []string{"variant/cycle/properties: the section is empty"}},
		{"transform line in a variant's gaps", "-- variant/cycle/in/sheet/B1 --\n=A1\n-- variant/cycle/gaps --\ntransform/looped calc x. Waits on sheets#1.\n", []string{"variant/cycle/gaps, line 1: the line starts with transform/looped, and a hand-written variant's gaps mark its own cells"}},
		{"delete a missing path", "-- variant/missing/delete --\nsheet/C1\n", []string{"variant/missing/delete, line 1: the base has no sheet/C1 to remove"}},
		{"delete a bad path", "-- variant/missing/delete --\n/sheet/B1\n", []string{`variant/missing/delete, line 1: "/sheet/B1" is not a path in the input view`}},
		{"delete twice", "-- variant/missing/delete --\nsheet/B1\nsheet/B1\n", []string{"variant/missing/delete, line 2: sheet/B1 is listed twice, on lines 1 and 2"}},
		{"delete empty", "-- variant/missing/delete --\n", []string{"variant/missing/delete: the section is empty"}},
		{"overlay and delete", "-- variant/cycle/in/sheet/B1 --\n=A1\n-- variant/cycle/delete --\nsheet/B1\n", []string{"variant/cycle/delete, line 1: the variant both lays sheet/B1 over the base and removes it"}},
		{"equal view", "-- variant/same/about --\nThe same.\n-- variant/same/in/sheet/B1 --\n3\n", []string{"variant/same: the variant's input view equals the base's"}},
		{"no inputs", "-- variant/bare/about --\nNothing changed.\n", []string{"variant/bare: the variant's input view equals the base's"}},
		// A transform's in/ and delete are update-written, and how they
		// differ from the base is its owner's to judge: a base edited by
		// hand leaves none of these a problem of form.
		{"transform equal view", "-- transform/same/in/sheet/B1 --\n3\n", nil},
		{"transform delete a missing path", "-- transform/trim/in/sheet/A1 --\n1\n-- transform/trim/delete --\nsheet/C1\n", nil},
		{"transform delete twice", "-- transform/trim/delete --\nsheet/B1\nsheet/B1\n", nil},
		{"transform delete empty", "-- transform/trim/in/sheet/A1 --\n1\n-- transform/trim/delete --\n", nil},
		{"transform overlay and delete", "-- transform/trim/in/sheet/B1 --\n=A1\n-- transform/trim/delete --\nsheet/B1\n", nil},
		{"transform file below a base file", "-- transform/deep/in/sheet/B1/x --\n1\n", nil},
		{"file below a base file", "-- variant/deep/in/sheet/B1/x --\n1\n", []string{"variant/deep: the variant's input view holds sheet/B1, and sheet/B1/x below it"}},
		{"base after a variant", "-- variant/cycle/in/sheet/B1 --\n=A1\n-- properties --\ncircular\n", []string{"properties: the base's hand-written section sits after a variant's"}},
		{"variant split", "-- variant/cycle/in/sheet/B1 --\n=A1\n-- variant/missing/delete --\nsheet/B1\n-- variant/cycle/properties --\ncircular\n", []string{"variant/cycle/properties: the section sits apart from variant/cycle's other hand-written sections"}},
		// A transform's variant whose inputs an update removed leaves its
		// outputs, which are orphans, not a problem of form.
		{"transform leftovers", "-- transform/looped/out/calc/error.txt --\nx\n", nil},
		// Update-written sections between a case's hand-written ones don't
		// split it.
		{"variant around outputs", "-- variant/cycle/in/sheet/B1 --\n=A1\n-- variant/cycle/out/calc/x --\n1\n-- variant/cycle/properties --\ncircular\n", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := Parse([]byte(base + tt.sections))
			var got []string
			for _, p := range a.Check() {
				got = append(got, p.String())
			}
			if len(got) != len(tt.want) {
				t.Fatalf("problems:\n%s\nwant %d:\n%s", strings.Join(got, "\n"), len(tt.want), strings.Join(tt.want, "\n"))
			}
			for i, w := range tt.want {
				if !strings.HasPrefix(got[i], w) {
					t.Errorf("problem %d is %q, want it to start %q", i, got[i], w)
				}
			}
		})
	}
}

func TestBaseGapsWithTransforms(t *testing.T) {
	gaps, ps := ParseGaps([]byte("calc x. Waits on sheets#1.\ntransform/looped calc y. Waits on sheets#2.\ntransform/renamed calc z.\n"))
	if len(ps) > 0 {
		t.Fatal(ps)
	}
	var cells []string
	for _, g := range gaps {
		cells = append(cells, g.Cell()+": "+g.Text)
	}
	if want := []string{"calc: x. Waits on sheets#1.", "transform/looped calc: y. Waits on sheets#2.", "transform/renamed calc: z."}; !slices.Equal(cells, want) {
		t.Errorf("the cells are %q, want %q", cells, want)
	}
	for _, tt := range []struct{ data, want string }{
		{"transform/looped calc x.\ntransform/looped calc y.\n", "gaps, line 2: lines 1 and 2 both mark the cell of transform/looped calc"},
		{"transform/Looped calc x.\n", `gaps, line 1: the line starts with "transform/Looped", which is not transform/<name> for a transform's name`},
		{"transform/looped Calc x.\n", `gaps, line 1: the line names transform/looped, then "Calc", which is not a column's name`},
		{"transform/looped calc\n", "gaps, line 1: the line names transform/looped calc and has no text"},
		{"transform/looped\n", `gaps, line 1: the line names transform/looped, then "", which is not a column's name`},
	} {
		_, ps := ParseGaps([]byte(tt.data))
		if len(ps) != 1 || ps[0].String() != tt.want && !strings.HasPrefix(ps[0].String(), tt.want) {
			t.Errorf("ParseGaps(%q) = %v, want %q", tt.data, ps, tt.want)
		}
	}
	// The base's cells and a transform's are distinct.
	if _, ps := ParseGaps([]byte("calc x.\ntransform/looped calc x.\n")); len(ps) > 0 {
		t.Errorf("a base line and a transform line for one column: %v", ps)
	}
}

func TestVariantDescriptions(t *testing.T) {
	a := Parse([]byte("A case.\n-- in/sheet/A1 --\n1\n" +
		"-- variant/a/in/sheet/A1 --\n2\n" +
		"-- variant/b/about --\n\n-- variant/b/in/sheet/A1 --\n3\n" +
		"-- variant/c/about --\nnote: a key and a value.\n-- variant/c/in/sheet/A1 --\n4\n" +
		"-- transform/t/in/sheet/A1 --\n5\n"))
	if ps := a.Check(); len(ps) > 0 {
		t.Fatal(ps)
	}
	var got []string
	for _, p := range a.VariantDescriptions() {
		got = append(got, p.String())
	}
	want := []string{
		"variant/a/about: the variant has no description: its about section is missing or empty",
		"variant/b/about: the variant has no description: its about section is missing or empty",
		`variant/c/about: line 1 of the about section is a key: value line, and a description is prose: "note: a key and a value."`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("the problems are:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestPlaceVariants: Place puts every update-written section of every
// case in its place, and leaves the hand-written sections in the order
// written.
func TestPlaceVariants(t *testing.T) {
	a := Parse([]byte(twiceWithVariants))
	// Reverse the update-written sections, and move them all to the end.
	var hand, written []Section
	for _, s := range a.Sections {
		if HandWritten(s.Name) {
			hand = append(hand, s)
		} else {
			written = append(written, s)
		}
	}
	slices.Reverse(written)
	scrambled := &Archive{Comment: a.Comment, Sections: append(hand, written...)}
	if Placed(scrambled.Format()) {
		t.Fatal("the scrambled archive is placed")
	}
	if got := string(scrambled.Place().Format()); got != twiceWithVariants {
		t.Errorf("in place, the scrambled archive is:\n%s\nwant:\n%s", got, twiceWithVariants)
	}
	// A hand-written variant with only outputs left goes after those with
	// hand-written sections, by name.
	b := Parse([]byte("A case.\n-- in/x/a --\n1\n-- variant/z/in/x/a --\n2\n-- variant/b/out/c/f --\n1\n-- variant/a/out/c/f --\n1\n-- out/c/f --\n0\n"))
	want := "A case.\n-- in/x/a --\n1\n-- out/c/f --\n0\n-- variant/z/in/x/a --\n2\n-- variant/a/out/c/f --\n1\n-- variant/b/out/c/f --\n1\n"
	if got := string(b.Place().Format()); got != want {
		t.Errorf("in place:\n%s\nwant:\n%s", got, want)
	}
}
