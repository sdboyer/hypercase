// walkthrough/sheets/step12/hypothesis_unit_test.go
package step12

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step12/sheetcorpus"
)

// properties are those the root package registers.
var properties = []decl.Property{sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling}

// TestHypothesesSay: each hypothesis names its property, one the package
// registers, describes itself, names its source, and claims at least one
// column, each once, with a polarity and predictions that fit it: a P or
// anti-P column predicts outcomes on a matching or a mismatching case, and
// a neutral one predicts over pairs. Each prediction is an outcome.
func TestHypothesesSay(t *testing.T) {
	for _, h := range hypotheses {
		t.Run(h.name, func(t *testing.T) {
			if !archive.ValidName(h.name) {
				t.Errorf("the name %q breaks the naming rule", h.name)
			}
			if h.description == "" {
				t.Error("no description")
			}
			if !slices.Contains(properties, h.property) {
				t.Errorf("the property %q is none the package registers", h.property.Name)
			}
			if h.source.name == "" || h.source.classify == nil {
				t.Error("its source has no name, or classifies nothing")
			}
			if len(h.claims) == 0 {
				t.Error("it claims no column")
			}
			seen := map[string]bool{}
			for _, cl := range h.claims {
				if seen[cl.column] {
					t.Errorf("it claims %s twice", cl.column)
				}
				seen[cl.column] = true
				polar := cl.match != "" || cl.mismatch != ""
				switch cl.polarity {
				case colP, colAnti:
					if !polar || len(cl.pair) > 0 {
						t.Errorf("%s is %s, and predicts nothing on a matching or a mismatching case, or predicts over pairs", cl.column, cl.polarity)
					}
				case colNeutral:
					if polar || len(cl.pair) == 0 {
						t.Errorf("%s is neutral, and predicts on a matching or a mismatching case, or nothing over pairs", cl.column)
					}
				default:
					t.Errorf("%s has the polarity %q, which is none of P, anti-P and neutral", cl.column, cl.polarity)
				}
				for _, p := range []string{cl.match, cl.mismatch} {
					if p == "" {
						continue
					}
					if _, err := archive.ParseOutcome([]byte(p + "\n")); err != nil {
						t.Errorf("%s predicts %q, which is no outcome: %v", cl.column, p, err)
					}
				}
			}
		})
	}
}

// cell makes a cell of a case of the archive twice, as a check sees it.
func cell(which, column, outcome string, properties ...string) hypercase.Cell {
	return hypercase.Cell{
		Type: "sheet", Archive: "twice", Case: which, Column: column,
		Outcome:    mustOutcome(outcome),
		Properties: properties,
		Files:      map[string][]byte{archive.OutcomeFile: []byte(outcome + "\n")},
	}
}

// TestRunsCounted: evaluating a hypothesis fails when it runs a lift, a
// suite or a transform it doesn't name, and passes when it names what it
// runs, once per case.
func TestRunsCounted(t *testing.T) {
	parses := source{name: "parses", classify: func(k *kase) (casePolarity, error) {
		view, err := k.inputView()
		if err != nil {
			return "", err
		}
		if _, err := sheetcorpus.Parse.Run(k.archive, sheetcorpus.Sheet{FS: view}, io.Discard); err != nil {
			return unclassified, nil
		}
		return anti, nil
	}}
	h := hypothesis{name: "runs-parse", property: sheetcorpus.Circular, source: parses, claims: []claim{{column: "resolve", polarity: colP, mismatch: "accepted"}}}
	cells := []hypercase.Cell{cell("", "resolve", "accepted"), cell("variant/cycle", "resolve", "rejected S201", "circular")}

	_, err := h.check().Run(cells)
	if err == nil || !strings.Contains(err.Error(), "ran parse 2 times; it may run nothing") {
		t.Errorf("a hypothesis that runs parse and names nothing: got %v, want it to fail naming parse", err)
	}
	h.source.runs = []string{"parse"}
	if _, err := h.check().Run(cells); err != nil {
		t.Errorf("a hypothesis that names what it runs: %v", err)
	}
}

// TestMisfit: a prediction that doesn't fit its column's polarity. A
// neutral column has no polarity for a case to match, so matching and
// mismatching predictions on one can't be evaluated. A P column that also
// predicts the base's outcome over a pair contradicts itself: on every
// pair whose cells meet its P predictions, the outcomes differ.
func TestMisfit(t *testing.T) {
	cells := []hypercase.Cell{cell("", "resolve", "accepted"), cell("variant/cycle", "resolve", "rejected S201", "circular")}

	neutral := hypothesis{name: "neutral-polar", property: sheetcorpus.Circular, source: stated, claims: []claim{
		{column: "resolve", polarity: colNeutral, match: "rejected"},
	}}
	if _, err := neutral.check().Run(cells); err == nil || !strings.Contains(err.Error(), "a neutral column has no polarity") {
		t.Errorf("matching predictions on a neutral column: got %v, want an error", err)
	}

	p := hypothesis{name: "p-pair", property: sheetcorpus.Circular, source: stated, claims: []claim{
		{column: "resolve", polarity: colP, match: "rejected", mismatch: "accepted", pair: []pairPrediction{sameOutcome}},
	}}
	got, err := p.check().Run(cells)
	want := []string{"sheet/twice/variant/cycle resolve: predicted the base's outcome, accepted; committed rejected S201"}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("the base's outcome on a P column: got %q, %v; want %q", got, err, want)
	}
}

// TestRelabelled: #20's question 5. circular and circular-relabelled are
// one claim, its P columns written as anti-P with their predictions
// swapped, and their committed findings are the same bytes. TestBreaks
// compares them after every break as well.
func TestRelabelled(t *testing.T) {
	dir := filepath.Join("testdata", "checks")
	a, err := os.ReadFile(filepath.Join(dir, "circular.txt"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "circular-relabelled.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(a) == 0 || string(a) != string(b) {
		t.Errorf("circular's findings:\n%s\ncircular-relabelled's:\n%s", a, b)
	}
}
