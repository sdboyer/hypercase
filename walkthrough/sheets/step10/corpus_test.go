// walkthrough/sheets/step10/corpus_test.go
package step10

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step10/sheetcorpus"
)

// doc:begin register
func init() {
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("step10"),
		hypercase.RootPackage(),
		sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling,
		sheetcorpus.Unterminated, sheetcorpus.StrayQuote, sheetcorpus.OutOfRange,
		sheetcorpus.SheetType, sheetcorpus.CSVType,
		hypercase.Own(sheetcorpus.Import, sheetcorpus.Parse, sheetcorpus.Resolve, evaluate),
		hypercase.Own(sheetcorpus.Looped, sheetcorpus.Renamed),
		hypercase.Own(codes),
	)
}

// doc:end register

var evaluate = hypercase.Suite[*sheet.Graph]{
	Name:        "evaluate",
	Description: "Evaluates every cell of the sheet.",
	Outputs: []decl.Output{
		{Pattern: "values.txt", Description: "Each cell's value, one per line, in cell order.", Inputs: sheetcorpus.Cells},
	},
	Run: func(t testing.TB, archive string, g *sheet.Graph, stdout, stderr io.Writer) ([]hypercase.File, error) {
		return []hypercase.File{{Path: "values.txt", Data: g.Evaluate().Format()}}, nil
	},
	Classifier: hypercase.Classifier[*sheet.Graph]{
		Name:        "div-zero",
		Description: "Adds div-zero when some cell divides by zero.",
		Classify: func(g *sheet.Graph, got hypercase.Result) (string, error) {
			if bytes.Contains(got.File("values.txt"), []byte("#DIV/0!")) {
				return "div-zero", nil
			}
			return "", nil
		},
	},
}

var codes = hypercase.Check{
	Name:        "codes",
	Description: "Every code the formula evaluator and the CSV importer define is the code of some rejected cell.",
	Run: func(cells []hypercase.Cell) ([]string, error) {
		seen := map[string]bool{}
		for _, c := range cells {
			for _, code := range c.Outcome.Codes { // empty unless rejected
				seen[code] = true
			}
		}
		var found, missing []string
		for _, code := range slices.Concat(sheet.Codes, sheet.CSVCodes) {
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

func TestCorpus(t *testing.T) { hypercase.Test(t) }
