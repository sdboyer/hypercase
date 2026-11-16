// walkthrough/sheets/step04/evaluate_test.go
package step04

import (
	"io"
	"io/fs"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step04/sheetcorpus"
)

// doc:begin suite
func init() {
	hypercase.Register(sheetcorpus.Project, sheetcorpus.Circular, evaluate)
}

var evaluate = hypercase.Suite[fs.FS]{
	Name:        "evaluate",
	Description: "Evaluates every cell of the sheet.",
	Rejects:     []decl.Property{sheetcorpus.Circular},
	Run: func(t testing.TB, archive string, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
		values, err := sheet.Eval(in) // as in step 3
		if err != nil {
			return nil, err
		}
		return []hypercase.File{{Path: "values.txt", Data: values.Format()}}, nil
	},
}

// doc:end suite

func TestCorpus(t *testing.T) { hypercase.Test(t) }
