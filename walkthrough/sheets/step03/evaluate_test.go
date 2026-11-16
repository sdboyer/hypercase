// walkthrough/sheets/step03/evaluate_test.go
package step03

import (
	"io"
	"io/fs"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step03/sheetcorpus"
)

// doc:begin suite
func init() {
	hypercase.Register(sheetcorpus.Project, hypercase.Root("step03"), hypercase.RootPackage(), evaluate)
}

var evaluate = hypercase.Suite[fs.FS]{
	Name:        "evaluate",
	Description: "Evaluates every cell of the sheet.",
	Corpus:      "sheet",
	Outputs: []decl.Output{
		{Pattern: "values.txt", Description: "Each cell's value, one per line, in cell order."},
	},
	Run: func(t testing.TB, archive string, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
		values, err := sheet.Eval(in) // the cells under sheet/
		if err != nil {
			return nil, err
		}
		return []hypercase.File{{Path: "values.txt", Data: values.Format()}}, nil
	},
}

func TestCorpus(t *testing.T) { hypercase.Test(t) }

// doc:end suite
