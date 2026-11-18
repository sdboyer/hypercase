// walkthrough/sheets/step05/corpus_test.go
package step05

import (
	"io"
	"io/fs"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step05/sheetcorpus"
)

// doc:begin evaluate
func init() {
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("step05"),
		hypercase.RootPackage(),
		sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling,
		evaluate,
	)
}

var evaluate = hypercase.Suite[fs.FS]{
	Name:        "evaluate",
	Description: "Evaluates every cell of the sheet.",
	Corpus:      "sheet",
	Outputs: []decl.Output{
		{Pattern: "values.txt", Description: "Each cell's value, one per line, in cell order.", Inputs: sheetcorpus.Cells},
		{Pattern: "error.txt", Description: "Why the sheet can't be evaluated.", Inputs: decl.FromPositions},
	},
	Rejects: []decl.Property{sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling},
	Run: func(t testing.TB, archive string, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
		values, err := sheet.Eval(in)
		if err != nil {
			return nil, err
		}
		return []hypercase.File{{Path: "values.txt", Data: values.Format()}}, nil
	},
}

func TestCorpus(t *testing.T) { hypercase.Test(t) }

// doc:end evaluate
