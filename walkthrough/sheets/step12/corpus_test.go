// walkthrough/sheets/step12/corpus_test.go
package step12

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step12/sheetcorpus"
)

// doc:begin evaluate
func init() {
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("step12"),
		hypercase.RootPackage(),
		sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling, sheetcorpus.FirstRowEmpty,
		sheetcorpus.SheetType,
		hypercase.Own(sheetcorpus.Parse, sheetcorpus.Resolve, evaluate),
		hypercase.Own(sheetcorpus.Looped, sheetcorpus.Renamed),
		hypercase.Own(codes),
		hypercase.Own(checksOf(hypotheses)...),
		hypercase.Own(circularSources),
	)
}

// hypotheses are #20's, written by hand, each owned as a corpus check.
var hypotheses = []hypothesis{circular, circularRelabelled, circularParsed, firstRowEmpty}

func checksOf(hs []hypothesis) []any {
	var checks []any
	for _, h := range hs {
		checks = append(checks, h.check())
	}
	return checks
}

var evaluate = hypercase.Suite[*sheet.Graph]{
	Name:        "evaluate",
	Description: "Evaluates every cell of the sheet.",
	Outputs: []decl.Output{
		{Pattern: "values.txt", Description: "Each cell's value, one per line, in cell order."},
	},
	Run: func(t testing.TB, archive string, g *sheet.Graph, stdout, stderr io.Writer) ([]hypercase.File, error) {
		sheetcorpus.Runs.Add("evaluate")
		return []hypercase.File{{Path: "values.txt", Data: g.Evaluate().Format()}}, nil
	},
	Classifier: hypercase.Classifier[*sheet.Graph]{
		Name:        "div-zero",
		Description: "Adds div-zero when some cell divides by zero.",
		Classify: func(g *sheet.Graph, got hypercase.Result) (string, error) {
			sheetcorpus.Runs.Add("evaluate")
			if bytes.Contains(got.File("values.txt"), []byte("#DIV/0!")) {
				return "div-zero", nil
			}
			return "", nil
		},
	},
}

// doc:end evaluate

// doc:begin codes
var codes = hypercase.Check{
	Name:        "codes",
	Description: "Every code the formula evaluator defines is the code of some rejected cell.",
	Run: func(cells []hypercase.Cell) ([]string, error) {
		seen := map[string]bool{}
		for _, c := range cells {
			for _, code := range c.Outcome.Codes { // empty unless rejected
				seen[code] = true
			}
		}
		var found, missing []string
		for _, code := range sheet.Codes {
			if seen[code] {
				found = append(found, code)
			} else {
				missing = append(missing, code)
			}
		}
		if len(missing) > 0 {
			return nil, fmt.Errorf("no rejected cell has %s", strings.Join(missing, ", "))
		}
		return found, nil
	},
}

// doc:end codes

func TestCorpus(t *testing.T) { hypercase.Test(t) }
