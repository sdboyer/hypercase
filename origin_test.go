package hypercase

import (
	"fmt"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/revmap"
	"github.com/sdboyer/jenny"
)

// The types of TestOrigins' plan: a corpus type, and the products of two
// bytes lifts, a from the corpus type and b from a's product.
type (
	originIn struct{ fs.FS }
	originA  struct{ fs.FS }
	originB  struct{ fs.FS }
)

// The statuses of a path of a variant's view against its base's.
const (
	inherited = "inherited" // the base's file, untouched
	replaced  = "replaced"  // other bytes than the base's
	same      = "same"      // laid over the base with the base's bytes
	added     = "added"     // a path the base lacks
	deleted   = "deleted"   // a path of the base the variant removes
)

// The kinds of case: a base; a hand-written variant; a transform's
// variant staged on what the transform makes, as an update stages it and
// as a normal run does when the archive holds none of its inputs; and one
// staged on the inputs the archive commits, as a normal run does.
const (
	kindBase      = "base"
	kindVariant   = "variant"
	kindMade      = "made"
	kindCommitted = "committed"
)

// ownSection is where a case of each kind commits a file of each status,
// with no bytes lift on the path: the base's in/, or the case's own in/.
// A deleted file has none.
var ownSection = map[string]map[string]string{
	kindBase:      {inherited: "in/"},
	kindVariant:   {inherited: "in/", replaced: "variant/v/in/", same: "variant/v/in/", added: "variant/v/in/"},
	kindMade:      {inherited: "in/", replaced: "transform/t/in/", same: "in/", added: "transform/t/in/"},
	kindCommitted: {inherited: "in/", replaced: "transform/t/in/", same: "transform/t/in/", added: "transform/t/in/"},
}

// TestOrigins: the origin of the value a cell reads names, for each file
// of the view the value comes from, the section the archive commits it
// in: where a normal run reads it, and where an update writes it. It is
// generated over every kind of case, every status of a path against the
// base, zero, one and two bytes lifts on the path, and both runs, and
// checked against ownSection: past a bytes lift, every file is the last
// one's product's, under <prefix>via/<lift>/.
func TestOrigins(t *testing.T) {
	tIn, tA, tB := reflect.TypeFor[originIn](), reflect.TypeFor[originA](), reflect.TypeFor[originB]()
	copies := func(view fs.FS) (jenny.Files, error) {
		paths, err := archive.Paths(view)
		if err != nil {
			return nil, err
		}
		var out jenny.Files
		for _, p := range paths {
			data, err := fs.ReadFile(view, p)
			if err != nil {
				return nil, err
			}
			out = append(out, jenny.NewFile(p, data))
		}
		return out, nil
	}
	ja := jenny.For[originIn]().New("a", func(in originIn) (jenny.Files, error) { return copies(in) })
	jb := jenny.For[originA]().New("b", func(in originA) (jenny.Files, error) { return copies(in) })
	product := []decl.Output{{Pattern: "sheet/*", Description: "The copy."}}
	la := &lift{name: "a", in: tIn, out: tA, bytes: true, owned: true, product: product,
		generate: func(in any) (*jenny.FS, error) { return ja.GenerateFS(in.(originIn)) }}
	lb := &lift{name: "b", in: tA, out: tB, bytes: true, owned: true, product: product,
		generate: func(in any) (*jenny.FS, error) { return jb.GenerateFS(in.(originA)) }}
	p := &plan{
		root:     &corpusType{goType: tIn, new: func(v fs.FS) any { return originIn{v} }},
		producer: map[reflect.Type]*lift{tA: la, tB: lb},
	}
	products := map[*lift]*corpusType{
		la: {goType: tA, new: func(v fs.FS) any { return originA{v} }},
		lb: {goType: tB, new: func(v fs.FS) any { return originB{v} }},
	}
	consumes := []reflect.Type{tIn, tA, tB}
	lastLift := []string{"", "a", "b"}

	base := map[string]string{"sheet/a": "1\n", "sheet/b": "2\n"}
	cases := 0
	for _, kind := range []string{kindBase, kindVariant, kindMade, kindCommitted} {
		prefix := map[string]string{kindBase: "", kindVariant: "variant/v/", kindMade: "transform/t/", kindCommitted: "transform/t/"}[kind]
		for _, status := range []string{inherited, replaced, same, added, deleted} {
			if kind == kindBase && status != inherited {
				continue
			}
			// The view: the base's, with sheet/b replaced in every variant,
			// so that each is one, and the status applied to the path
			// under test, sheet/a, or, for an added path, sheet/c.
			view := maps.Clone(base)
			overlay := map[string]string{}
			var gone []string
			statusOf := map[string]string{"sheet/a": inherited, "sheet/b": inherited}
			if kind != kindBase {
				overlay["sheet/b"], statusOf["sheet/b"] = "3\n", replaced
			}
			switch status {
			case replaced:
				overlay["sheet/a"], statusOf["sheet/a"] = "9\n", replaced
			case same:
				overlay["sheet/a"], statusOf["sheet/a"] = base["sheet/a"], same
			case added:
				overlay["sheet/c"], statusOf["sheet/c"] = "5\n", added
			case deleted:
				gone, statusOf["sheet/a"] = []string{"sheet/a"}, deleted
			}
			maps.Copy(view, overlay)
			for _, g := range gone {
				delete(view, g)
			}
			for _, update := range []bool{false, true} {
				if kind == kindCommitted && update {
					continue // an update stages a transform's variant on what it makes
				}
				for n := range 3 {
					name := fmt.Sprintf("%s/%s/lifts%d/update%v", kind, status, n, update)
					cases++
					got := stageOrigin(t, name, p, products, consumes[n], kind, prefix, base, overlay, gone, view, update)
					if got == nil {
						continue
					}
					paths, err := archive.Paths(got.view)
					if err != nil {
						t.Fatalf("%s: %v", name, err)
					}
					if want := slices.Sorted(maps.Keys(view)); !slices.Equal(paths, want) {
						t.Errorf("%s: the view holds %q, want %q", name, paths, want)
					}
					for path, st := range statusOf {
						want := prefix + "via/" + lastLift[n] + "/" + path
						if n == 0 {
							want = ownSection[kind][st] + path
						}
						if st == deleted {
							if _, err := fs.Stat(got.view, path); err == nil {
								t.Errorf("%s: the view holds %s, which the variant deletes", name, path)
							}
							continue
						}
						if data, err := fs.ReadFile(got.view, path); err != nil || string(data) != view[path] {
							t.Errorf("%s: the view's %s is %q, %v, want %q", name, path, data, err, view[path])
						}
						if s := got.section(path); s != want {
							t.Errorf("%s: %s (%s) is committed in %s, want %s", name, path, st, s, want)
						}
					}
				}
			}
		}
	}
	if cases != 3*2*(1+5+5)+3*5 {
		t.Errorf("the grid has %d cases", cases)
	}
}

// stageOrigin stages a case of the kind as a run does, with the bytes
// lifts on the plan's path to typ, and returns the origin of the value it
// reaches. The archive holds the base and the variant's sections, and, for
// a normal run, what an update writes for each bytes lift: its accepted
// outcome, and its product, which copies its input view.
func stageOrigin(t *testing.T, name string, p *plan, products map[*lift]*corpusType, typ reflect.Type, kind, prefix string, base, overlay map[string]string, gone []string, view map[string]string, update bool) *origin {
	t.Helper()
	var sections []archive.Section
	for _, path := range slices.Sorted(maps.Keys(base)) {
		sections = append(sections, archive.Section{Name: "in/" + path, Data: []byte(base[path])})
	}
	if kind == kindVariant || kind == kindCommitted {
		for _, path := range slices.Sorted(maps.Keys(overlay)) {
			sections = append(sections, archive.Section{Name: prefix + "in/" + path, Data: []byte(overlay[path])})
		}
		if len(gone) > 0 {
			sections = append(sections, archive.Section{Name: prefix + "delete", Data: []byte(gone[0] + "\n")})
		}
	}
	if !update {
		for _, l := range []string{"a", "b"} {
			sections = append(sections, archive.Section{Name: prefix + "out/" + l + "/outcome", Data: archive.Outcome{State: archive.Accepted}.Format()})
			for _, path := range slices.Sorted(maps.Keys(view)) {
				sections = append(sections, archive.Section{Name: prefix + "via/" + l + "/" + path, Data: []byte(view[path])})
			}
		}
	}
	a := &archive.Archive{Sections: sections}
	if ps := a.Check(); len(ps) > 0 {
		t.Fatalf("%s: the archive has problems: %v", name, ps)
	}
	b := a.Base()
	var committed archive.VariantCase
	if vs := a.Variants(); len(vs) > 0 {
		committed = vs[0]
	}
	var from *origin
	switch kind {
	case kindBase:
		from = baseOrigin(b.View())
	case kindVariant:
		from = overlayOrigin(b.VariantView(committed), prefix, committed.Overlay)
	case kindMade:
		made := map[string][]byte{}
		for path, data := range view {
			made[path] = []byte(data)
		}
		from = madeOrigin(&plannedVariant{made: made, sections: b.VariantSections(archive.Transform, "t", made)}, prefix)
	case kindCommitted:
		v, problems := b.InputView(committed)
		if len(problems) > 0 {
			t.Fatalf("%s: the committed inputs give no view: %v", name, problems)
		}
		from = overlayOrigin(v, prefix, committed.Overlay)
	}
	r := &runner{update: update, products: products}
	c := &aCase{name: "origins/x", archive: "x", prefix: prefix, a: a}
	c.setView(r, p, from)
	_, got, block := c.state.stage(typ)
	if block != nil {
		t.Errorf("%s: the case is blocked by %s", name, block.failed)
		return nil
	}
	return got
}

// TestRangeGrid: a range of a cell's input view holds when its Path names
// a file of the view and it is the whole file, Start and End both 0, or
// lines of it, from 1, End at or after Start and at or before the last.
// Otherwise its fault is the first of: no path, an invalid path, a
// directory, a path the view lacks, as a deleted one, a negative line, one
// of Start and End 0 and not both, End before Start, and End past the last
// line. Generated over Start and End in {-1, 0, 1, 2, n, n+1}, files of 0,
// 1 and 3 lines, and every kind of path, and checked against an oracle
// that holds the lines it reads.
func TestRangeGrid(t *testing.T) {
	paths := map[string][]string{
		"file":      {"sheet/x"},
		"deleted":   {"sheet/gone"},
		"directory": {"sheet", "."},
		"invalid":   {"./sheet/x", "sheet/x/", "/sheet/x", "sheet//x", "..", "sheet/../sheet/x"},
		"empty":     {""},
	}
	for _, n := range []int{0, 1, 3} {
		data := ""
		for i := range n {
			data += fmt.Sprintf("line %d\n", i+1)
		}
		// A variant that keeps the base's sheet/x and deletes sheet/gone.
		a := &archive.Archive{Sections: []archive.Section{
			{Name: "in/sheet/x", Data: []byte(data)},
			{Name: "in/sheet/gone", Data: []byte("gone\n")},
			{Name: "variant/v/in/sheet/y", Data: []byte("y\n")},
			{Name: "variant/v/delete", Data: []byte("sheet/gone\n")},
		}}
		v := a.Variants()[0]
		o := overlayOrigin(a.Base().VariantView(v), "variant/v/", v.Overlay)
		values := []int{-1, 0, 1, 2, n, n + 1}
		for kind, ps := range paths {
			for _, path := range ps {
				for _, start := range values {
					for _, end := range values {
						r := decl.Range{Path: path, Start: start, End: end}
						got, fault, lines := o.check(r)
						want := wantFault(kind, start, end, n)
						if fault != want {
							t.Errorf("%d lines: %+v (%s): fault %d, want %d", n, r, kind, fault, want)
							continue
						}
						var wantRange revmap.Range
						if want == rangeHeld {
							wantRange = revmap.Range{Section: "in/sheet/x", Start: start, End: end}
						}
						if got != wantRange {
							t.Errorf("%d lines: %+v: the range is %+v, want %+v", n, r, got, wantRange)
						}
						if kind == "file" && lines != n {
							t.Errorf("%d lines: %+v: the file has %d lines, by check", n, r, lines)
						}
					}
				}
			}
		}
	}
}

// wantFault is TestRangeGrid's oracle: the lines of a file of n lines it
// holds, and the fault of any other range, in check's order.
func wantFault(kind string, start, end, n int) rangeFault {
	switch kind {
	case "empty":
		return rangeNoPath
	case "invalid":
		return rangeInvalid
	case "directory":
		return rangeDir
	case "deleted":
		return rangeAbsent
	}
	holds := map[[2]int]bool{{0, 0}: true}
	for s := 1; s <= n; s++ {
		for e := s; e <= n; e++ {
			holds[[2]int{s, e}] = true
		}
	}
	switch {
	case holds[[2]int{start, end}]:
		return rangeHeld
	case start < 0 || end < 0:
		return rangeNegative
	case start == 0 || end == 0:
		return rangeHalf
	case end < start:
		return rangeBackward
	}
	return rangePast
}

// TestRangeFaultWords: every fault has its own words, and none names a
// range the view holds.
func TestRangeFaultWords(t *testing.T) {
	r := decl.Range{Path: "sheet/x", Start: 3, End: 2}
	seen := map[string]rangeFault{}
	for f := rangeNoPath; f <= rangePast; f++ {
		w := f.says(r, 1)
		if w == "" {
			t.Errorf("fault %d has no words", f)
		}
		if g, dup := seen[w]; dup {
			t.Errorf("faults %d and %d have the same words: %s", g, f, w)
		}
		seen[w] = f
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("a range the view holds has words")
			}
		}()
		rangeHeld.says(r, 1)
	}()
	if got, want := rangePast.says(decl.Range{Path: "sheet/x", Start: 1, End: 4}, 3), "its End, 4, is past the last line of sheet/x, which has 3 lines"; got != want {
		t.Errorf("past the end says %q, want %q", got, want)
	}
}

// TestLineCount: a file's lines are those that end with a newline, and a
// last one that doesn't.
func TestLineCount(t *testing.T) {
	for data, want := range map[string]int{
		"":       0,
		"\n":     1,
		"\n\n":   2,
		"a":      1,
		"a\n":    1,
		"a\nb":   2,
		"a\nb\n": 2,
		"a\n\nb": 3,
	} {
		if got := lineCount([]byte(data)); got != want {
			t.Errorf("%q has %d lines, want %d", data, got, want)
		}
	}
}
