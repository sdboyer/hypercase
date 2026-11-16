// walkthrough/sheets/step12/firstrow_test.go
package step12

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step12/sheetcorpus"
)

// firstRowEmpty is the sheets' analog of #20's imperial hypothesis over
// the recipes toy, since dropped: a property of valid inputs, which a
// transform gives, claimed neutral in a column whose prediction is
// strengthened beyond the outcome. renamed moves a sheet one row down, so
// its variant has first-row-empty, which renamed gives, and its base,
// whose cells start on the first row, has not: the two make a pair. Every
// column is neutral, and evaluate's prediction is strengthened as
// shopping-list's was to "equals base": no column's bytes survive the move
// unchanged, so the variant's values are the base's, each one row down.
var firstRowEmpty = hypothesis{
	name:        "first-row-empty",
	description: "Moving a sheet one row down changes no column's outcome, and evaluate's values are the base's, each one row down. A case's polarity is what it states.",
	property:    sheetcorpus.FirstRowEmpty,
	source: source{
		name: "stated",
		// A closed world: a case that doesn't state first-row-empty has a
		// cell on the first row. No column rejects first-row-empty, so no
		// run checks a statement of it either way.
		classify: func(k *kase) (casePolarity, error) {
			if slices.Contains(k.properties, sheetcorpus.FirstRowEmpty.Name) {
				return pos, nil
			}
			return anti, nil
		},
	},
	claims: []claim{
		{column: "calc", polarity: colNeutral, pair: []pairPrediction{sameOutcome}},
		{column: "evaluate", polarity: colNeutral, pair: []pairPrediction{sameOutcome, valuesOneRowDown}},
		{column: "format", polarity: colNeutral, pair: []pairPrediction{sameOutcome}},
		{column: "parse", polarity: colNeutral, pair: []pairPrediction{sameOutcome}},
		{column: "resolve", polarity: colNeutral, pair: []pairPrediction{sameOutcome}},
	},
}

// valuesOneRowDown predicts that the variant's values.txt is the base's
// with each cell one row down: A1 6 becomes A2 6.
var valuesOneRowDown = pairPrediction{
	what: "the base's values.txt one row down",
	check: func(base, variant hypercase.Cell) (string, string, bool) {
		b, bok := base.Files["values.txt"]
		v, vok := variant.Files["values.txt"]
		if !bok && !vok {
			return "", "", true
		}
		want, got := "no values.txt", "no values.txt"
		if bok {
			want = strconv.Quote(oneRowDown(string(b)))
		}
		if vok {
			got = strconv.Quote(string(v))
		}
		return want, got, want == got
	},
}

// oneRowDown moves the cell that starts each line of values one row down.
func oneRowDown(values string) string {
	var b strings.Builder
	for _, line := range strings.SplitAfter(values, "\n") {
		name, rest, ok := strings.Cut(line, " ")
		i := strings.IndexFunc(name, func(r rune) bool { return r >= '0' && r <= '9' })
		row, err := strconv.Atoi(name[max(i, 0):])
		if !ok || i <= 0 || err != nil {
			b.WriteString(line)
			continue
		}
		fmt.Fprintf(&b, "%s%d %s", name[:i], row+1, rest)
	}
	return b.String()
}
