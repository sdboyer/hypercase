package cells

import (
	"maps"
	"testing"

	"github.com/sdboyer/hypercase/archive"
)

// TestOf: a column's out/ or via/ section gives its cell, in the base, a
// hand-written variant and a transform's variant, and no other section
// gives one.
func TestOf(t *testing.T) {
	for _, tt := range []struct {
		section string
		want    Cell
		ok      bool
	}{
		{"out/evaluate/outcome", Cell{Column: "evaluate"}, true},
		{"via/import/sheet/A1", Cell{Column: "import"}, true},
		{"variant/cycle/out/resolve/error.txt", Cell{archive.Variant, "cycle", "resolve"}, true},
		{"transform/looped/via/import/sheet/A1", Cell{archive.Transform, "looped", "import"}, true},
		{"in/sheet/A1", Cell{}, false},
		{"gaps", Cell{}, false},
		{"transform/looped/in/sheet/A1", Cell{}, false},
		{"variant/cycle/about", Cell{}, false},
	} {
		if got, ok := Of(tt.section); got != tt.want || ok != tt.ok {
			t.Errorf("Of(%q) = %v, %v, want %v, %v", tt.section, got, ok, tt.want, tt.ok)
		}
	}
}

// TestPart: an out/ section is part of its cell. A via/ section is, unless
// what is known says its column is no bytes lift; a column nothing says
// may be one.
func TestPart(t *testing.T) {
	known := map[string]bool{"import": true, "resolve": false}
	for _, tt := range []struct {
		section string
		lifts   map[string]bool
		want    bool
	}{
		{"out/resolve/outcome", known, true},
		{"via/import/sheet/A1", known, true},
		{"via/resolve/sheet/A1", known, false},
		{"variant/cycle/via/resolve/sheet/A1", known, false},
		{"via/nobody/A1", known, true},
		{"via/resolve/sheet/A1", nil, true},
	} {
		if got := Part(tt.section, tt.lifts); got != tt.want {
			t.Errorf("Part(%q, %v) = %v, want %v", tt.section, tt.lifts, got, tt.want)
		}
	}
}

// TestMarks: the base's gaps mark the base's cells, and by a line that
// names a transform, its variant's; a hand-written variant's gaps mark its
// own. Each mark names its case, its section and its line.
func TestMarks(t *testing.T) {
	a := archive.Parse([]byte(`A1 is twice B1.
-- in/sheet/A1 --
=B1*2
-- gaps --
evaluate Evaluates A1 to 6. Waits on sheets#4.
transform/looped resolve Names the cycle's cells. Waits on sheets#5.
-- variant/cycle/about --
B1 refers back to A1.
-- variant/cycle/gaps --
resolve Says which cell to break the cycle at. Waits on sheets#6.
-- variant/cycle/in/sheet/B1 --
=A1
`))
	want := map[Cell]Mark{
		{Column: "evaluate"}:                     {Case: "", Gaps: "gaps", Line: 1},
		{archive.Transform, "looped", "resolve"}: {Case: "transform/looped", Gaps: "gaps", Line: 2},
		{archive.Variant, "cycle", "resolve"}:    {Case: "variant/cycle", Gaps: "variant/cycle/gaps", Line: 1},
	}
	if got := Marks(a); !maps.Equal(got, want) {
		t.Errorf("Marks = %v, want %v", got, want)
	}
}
