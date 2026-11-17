package report

import (
	"testing"
	"testing/fstest"

	"github.com/sdboyer/hypercase/internal/registry"
)

// TestChangeClosedGap records a known gap, #51: a changed cell that a gaps
// line marks at A and none marks at B shows no gap, since a change's gap is
// read at B. A change that closes the gap is such a change, since it
// removes the line as it fixes the cell. What is right is to show A's line,
// marked as closed, with its link.
func TestChangeClosedGap(t *testing.T) {
	reg := &registry.Registry{
		Format:     registry.Format,
		IssueLinks: []registry.IssueLink{{Pattern: `sheets#(\d+)`, URL: "https://example.com/sheets/issues/$1"}},
		Types:      []registry.Type{{Dir: "sheet", Description: "A sheet."}},
		Suites:     []registry.Suite{{Name: "parse", Description: "Parses the sheet.", Corpora: []string{"sheet"}}},
	}
	at := func(name, negative string) *snapshot {
		t.Helper()
		tree := fstest.MapFS{"testdata/sheet/negative.txtar": {Data: []byte(negative)}}
		s, err := load(Rev{Name: name}, tree, ".", []found{{path: "testdata/hypercase.json", reg: reg}})
		if err != nil {
			t.Fatal(err)
		}
		if len(s.problems) > 0 {
			t.Fatalf("at %s: %v", name, s.problems)
		}
		return s
	}
	a := at("a", "A1 is a negative number.\n-- in/sheet/A1 --\n-3\n-- gaps --\nparse A negative number is valid. Waits on sheets#3.\n-- out/parse/error.txt --\nA1: unexpected \"-\"\n-- out/parse/outcome --\nrejected S101\n")
	b := at("b", "A1 is a negative number.\n-- in/sheet/A1 --\n-3\n-- out/parse/outcome --\naccepted\n")
	marked := cellKey{c: caseKey{corpus: "sheet", archive: "negative"}, column: "parse"}
	if _, ok := a.gaps[marked]; len(a.gaps) != 1 || !ok {
		t.Fatalf("at a, the gaps lines mark %v, not negative's parse cell alone", a.gaps)
	}
	links, bad := newLinker(b.links)
	if bad != nil {
		t.Fatal(bad)
	}
	got := changes(a, b, b.props, links)
	if len(got) != 1 || got[0].Rank != OutcomeChanged || got[0].Case != "sheet/negative" || got[0].Column != "parse" {
		t.Fatalf("the changes are %+v, not the change of outcome of negative's parse cell", got)
	}
	if got[0].Gap != nil {
		t.Errorf("the change shows the gap %q, where today it shows none (#51). If #51 is fixed, make this test show A's line, marked as closed.", got[0].Gap)
	}
}
