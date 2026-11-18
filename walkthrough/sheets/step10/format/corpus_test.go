// walkthrough/sheets/step10/format/corpus_test.go
package format

import (
	"io"
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
		sheetcorpus.Circular, sheetcorpus.Dangling,
		sheetcorpus.SheetType, sheetcorpus.CSVType,
		hypercase.Use(sheetcorpus.Import, sheetcorpus.Parse), // computed here; their sections are left alone
		hypercase.Own(format),
	)
}

// doc:end register

var format = hypercase.Suite[*sheet.Parsed]{
	Name:        "format",
	Description: "Prints each formula back in canonical form.",
	Probe: func(p *sheet.Parsed) (why string, ok bool) {
		if len(p.Formulas()) == 0 {
			return "no formulas", false
		}
		return "", true
	},
	Outputs: []decl.Output{
		{Pattern: "formulas.txt", Description: "Each formula cell and its formula, in cell order.", Inputs: sheetcorpus.Formulas},
	},
	Run: func(t testing.TB, archive string, p *sheet.Parsed, stdout, stderr io.Writer) ([]hypercase.File, error) {
		return []hypercase.File{{Path: "formulas.txt", Data: sheet.Format(p)}}, nil
	},
}

func TestCorpus(t *testing.T) { hypercase.Test(t) }
