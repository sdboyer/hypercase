// walkthrough/sheets/step05/format/corpus_test.go
package format

import (
	"io"
	"io/fs"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step05/sheetcorpus"
)

// doc:begin format
func init() {
	hypercase.Register(sheetcorpus.Project, hypercase.Root("step05"), sheetcorpus.Malformed, format)
}

var format = hypercase.Suite[fs.FS]{
	Name:        "format",
	Description: "Prints each formula back in canonical form.",
	Corpus:      "sheet",
	Probe: func(in fs.FS) (why string, ok bool) {
		if !sheet.HasFormula(in) {
			return "no formulas", false
		}
		return "", true
	},
	Outputs: []decl.Output{
		{Pattern: "formulas.txt", Description: "Each formula cell and its formula, in cell order.", Inputs: sheetcorpus.Formulas},
		{Pattern: "error.txt", Description: "Why the sheet doesn't parse.", Inputs: decl.FromPositions},
	},
	Rejects: []decl.Property{sheetcorpus.Malformed},
	Run: func(t testing.TB, archive string, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
		p, err := sheet.Parse(in)
		if err != nil {
			return nil, err
		}
		return []hypercase.File{{Path: "formulas.txt", Data: sheet.Format(p)}}, nil
	},
}

// doc:end format

func TestCorpus(t *testing.T) { hypercase.Test(t) }
