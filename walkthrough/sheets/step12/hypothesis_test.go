// walkthrough/sheets/step12/hypothesis_test.go
package step12

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step12/sheetcorpus"
)

// The hypotheses of #20, written by hand as corpus checks, as evidence for
// its design: nothing here is hypercase's. A hypothesis names a property,
// a source that classifies every case for it, and the columns it claims,
// each with a polarity and its predictions. It is evaluated at one
// revision, from what the archives and the registries commit, and its
// findings are the cells that break a prediction, one per line.

// A case's polarity for a property is a fact about the case.
type casePolarity string

const (
	pos     casePolarity = "P"
	anti    casePolarity = "anti-P"
	neither casePolarity = "neither" // no prediction can be made from the case
	// unclassified: the source couldn't classify the case, since the lift
	// its predicate reads rejected the case.
	unclassified casePolarity = "unclassified"
)

// A column's polarity for a property is a hypothesis's claim.
type columnPolarity string

const (
	colP       columnPolarity = "P"
	colAnti    columnPolarity = "anti-P"
	colNeutral columnPolarity = "neutral"
)

// A hypothesis is a claim about one property: how each column it lists
// behaves on the cases that have the property and on those that don't.
type hypothesis struct {
	name, description string
	property          decl.Property
	source            source
	claims            []claim
}

// A source classifies a case for a hypothesis's property.
type source struct {
	name string
	// runs names what classifying one case runs, as sheetcorpus.Runs
	// counts it: nothing, for a source that reads what the case states.
	runs     []string
	classify func(k *kase) (casePolarity, error)
}

// A claim is a column's polarity, and what it predicts.
type claim struct {
	column   string
	polarity columnPolarity
	// match and mismatch are what a P or anti-P column predicts of a case
	// of its own polarity, and of one of the opposite polarity: an outcome,
	// whose codes and detail, when it gives none, match any. "" predicts
	// nothing.
	match, mismatch string
	// pair is what a neutral column predicts of each pair: a base and its
	// variant, one P and the other anti-P.
	pair []pairPrediction
}

// A pairPrediction is a neutral column's prediction over a pair.
type pairPrediction struct {
	// what says what the variant's cell is predicted to hold.
	what string
	// check gives what was predicted for the variant's cell, from the
	// base's, what the variant's commits, and whether they agree.
	check func(base, variant hypercase.Cell) (predicted, committed string, ok bool)
}

// sameOutcome predicts that the variant's outcome is the base's.
var sameOutcome = pairPrediction{
	what: "the base's outcome",
	check: func(base, variant hypercase.Cell) (string, string, bool) {
		b, v := outcomeText(base), outcomeText(variant)
		return b, v, b == v
	},
}

// A kase is one case, with its cells, by column. (case is Go's.)
type kase struct {
	name                string // sheet/twice/transform/looped
	typ, archive, which string // which is "" for the base, or variant/<name> or transform/<name>
	properties          []string
	cells               map[string]hypercase.Cell
}

// cases groups cells by case, in the order the cells give them.
func cases(cells []hypercase.Cell) []*kase {
	var ks []*kase
	by := map[string]*kase{}
	for _, c := range cells {
		name := c.Type + "/" + c.Archive
		if c.Case != "" {
			name += "/" + c.Case
		}
		k, ok := by[name]
		if !ok {
			k = &kase{name: name, typ: c.Type, archive: c.Archive, which: c.Case, properties: c.Properties, cells: map[string]hypercase.Cell{}}
			by[name] = k
			ks = append(ks, k)
		}
		k.cells[c.Column] = c
	}
	return ks
}

// inputView reads the case's input view from its committed archive: the
// base's in/, or a variant's view of it. It runs no transform: a
// transform's variant is read from the inputs the archive commits for it.
// It is read relative to the corpus root, the root package's directory.
func (k *kase) inputView() (fs.FS, error) {
	p := filepath.Join("testdata", k.typ, k.archive+".txtar")
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	a := archive.Parse(data)
	if k.which == "" {
		return a.View(), nil
	}
	for _, v := range a.Variants() {
		if v.Case() != k.which {
			continue
		}
		if v.Kind == archive.Variant {
			return a.Base().VariantView(v), nil
		}
		view, ps := a.Base().InputView(v)
		if len(ps) > 0 {
			return nil, fmt.Errorf("%s: %s makes no input view: %v", p, k.which, ps)
		}
		return view, nil
	}
	return nil, fmt.Errorf("%s holds no %s", p, k.which)
}

// check is the hypothesis as a corpus check.
func (h hypothesis) check() hypercase.Check {
	return hypercase.Check{
		Name:        h.name,
		Description: h.description,
		Run: func(cells []hypercase.Cell) ([]string, error) {
			return counted(h.name, h.source.runs, func() ([]string, int, error) { return h.evaluate(cells) })
		},
	}
}

// counted runs eval, which gives findings and how many cases it read, and
// fails when it ran anything but what runs names, once per case. Every
// lift, suite and transform of this process counts its runs in
// sheetcorpus.Runs, and the cells have all run before any check does.
func counted(name string, runs []string, eval func() ([]string, int, error)) ([]string, error) {
	before := sheetcorpus.Runs.Counts()
	findings, n, err := eval()
	after := sheetcorpus.Runs.Counts()
	if err != nil {
		return nil, err
	}
	ran := map[string]int{}
	for what, m := range after {
		if d := m - before[what]; d != 0 {
			ran[what] = d
		}
	}
	want := map[string]int{}
	for _, what := range runs {
		want[what] = n
	}
	if !maps.Equal(ran, want) {
		return nil, fmt.Errorf("evaluating %s ran %s; it may run %s", name, runList(ran), runList(want))
	}
	return findings, nil
}

func runList(runs map[string]int) string {
	if len(runs) == 0 {
		return "nothing"
	}
	var parts []string
	for _, what := range slices.Sorted(maps.Keys(runs)) {
		parts = append(parts, fmt.Sprintf("%s %d times", what, runs[what]))
	}
	return strings.Join(parts, ", ")
}

// evaluate classifies every case, compares each listed column's committed
// outcomes with what the claim predicts, and gives each violation as a
// finding: the case, the column, the prediction and what is committed. It
// also gives how many cases it classified.
func (h hypothesis) evaluate(cells []hypercase.Cell) ([]string, int, error) {
	columns := map[string]bool{}
	for _, c := range cells {
		columns[c.Column] = true
	}
	for _, cl := range h.claims {
		switch {
		case !columns[cl.column]:
			return nil, 0, fmt.Errorf("%s claims the column %s, which no registry under the corpus root has", h.name, cl.column)
		case cl.polarity == colNeutral && (cl.match != "" || cl.mismatch != ""):
			return nil, 0, fmt.Errorf("%s claims %s neutral, and predicts what it does on a matching or a mismatching case; a neutral column has no polarity for a case to match", h.name, cl.column)
		}
	}
	ks := cases(cells)
	pol := map[*kase]casePolarity{}
	bases := map[string]*kase{}
	for _, k := range ks {
		p, err := h.source.classify(k)
		if err != nil {
			return nil, 0, fmt.Errorf("classifying %s for %s: %w", k.name, h.property.Name, err)
		}
		pol[k] = p
		if k.which == "" {
			bases[k.typ+"/"+k.archive] = k
		}
	}
	var findings []string
	for _, cl := range h.claims {
		for _, k := range ks {
			cell, ok := k.cells[cl.column]
			if !ok {
				continue
			}
			if cl.match != "" || cl.mismatch != "" {
				findings = append(findings, polarFindings(cl, k, pol[k], cell)...)
			}
			if b := bases[k.typ+"/"+k.archive]; k.which != "" && b != nil && opposite(pol[b], pol[k]) {
				if bc, ok := b.cells[cl.column]; ok {
					for _, pp := range cl.pair {
						if predicted, committed, ok := pp.check(bc, cell); !ok {
							findings = append(findings, fmt.Sprintf("%s %s: predicted %s, %s; committed %s%s", k.name, cl.column, pp.what, predicted, committed, gapNote(cellOf{b, bc}, cellOf{k, cell})))
						}
					}
				}
			}
		}
	}
	return findings, len(ks), nil
}

// polarFindings judges one cell against a P or anti-P column's prediction.
func polarFindings(cl claim, k *kase, p casePolarity, cell hypercase.Cell) []string {
	if p != pos && p != anti {
		return nil
	}
	want := cl.mismatch
	if (p == pos) == (cl.polarity == colP) {
		want = cl.match
	}
	if want == "" || matches(cell.Outcome, want) {
		return nil
	}
	return []string{fmt.Sprintf("%s %s: predicted %s, committed %s%s", k.name, cl.column, want, outcomeText(cell), gapNote(cellOf{k, cell}))}
}

// opposite reports whether two cases make a pair: one P and the other
// anti-P.
func opposite(a, b casePolarity) bool {
	return (a == pos && b == anti) || (a == anti && b == pos)
}

// matches reports whether an outcome is what a prediction says: its state,
// and its codes and detail where the prediction gives them.
func matches(o archive.Outcome, prediction string) bool {
	want := mustOutcome(prediction)
	return o.State == want.State &&
		(len(want.Codes) == 0 || slices.Equal(o.Codes, want.Codes)) &&
		(want.Detail == "" || o.Detail == want.Detail)
}

func mustOutcome(s string) archive.Outcome {
	o, err := archive.ParseOutcome([]byte(s + "\n"))
	if err != nil {
		panic(fmt.Sprintf("the prediction %q is no outcome: %v", s, err))
	}
	return o
}

// outcomeText is a cell's committed outcome, as its file holds it, less
// the newline.
func outcomeText(c hypercase.Cell) string {
	data, ok := c.Files[archive.OutcomeFile]
	if !ok {
		return "no outcome"
	}
	return strings.TrimSuffix(string(data), "\n")
}

// A cellOf is a cell with its case.
type cellOf struct {
	k *kase
	c hypercase.Cell
}

// gapNote names each known gap behind the cells a finding compares: a gaps
// line on one of them, or on the lift that blocked one.
func gapNote(cells ...cellOf) string {
	var gaps []string
	for _, kc := range cells {
		k, c := kc.k, kc.c
		if c.Gap != "" {
			gaps = append(gaps, k.name+" "+c.Column)
		}
		if c.Outcome.State == archive.Blocked {
			if l, ok := k.cells[c.Outcome.Detail]; ok && l.Gap != "" {
				gaps = append(gaps, k.name+" "+l.Column)
			}
		}
	}
	switch len(gaps) {
	case 0:
		return ""
	case 1:
		return " (known gap: " + gaps[0] + ")"
	}
	return " (known gaps: " + strings.Join(gaps, ", ") + ")"
}
