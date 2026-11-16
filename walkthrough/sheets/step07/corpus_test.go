// walkthrough/sheets/step07/corpus_test.go
package step07

import (
	"io"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step07/sheetcorpus"
)

// doc:begin evaluate
func init() {
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("step07"),
		hypercase.RootPackage(),
		sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling,
		sheetcorpus.SheetType,
		hypercase.Own(sheetcorpus.Parse, sheetcorpus.Resolve, evaluate),
		hypercase.Own(sheetcorpus.Looped, sheetcorpus.Renamed),
	)
}

var evaluate = hypercase.Suite[*sheet.Graph]{
	Name:        "evaluate",
	Description: "Evaluates every cell of the sheet.",
	Outputs: []decl.Output{
		{Pattern: "values.txt", Description: "Each cell's value, one per line, in cell order."},
	},
	Run: func(t testing.TB, archive string, g *sheet.Graph, stdout, stderr io.Writer) ([]hypercase.File, error) {
		return []hypercase.File{{Path: "values.txt", Data: g.Evaluate().Format()}}, nil
	},
}

func TestCorpus(t *testing.T) { hypercase.Test(t) }

// doc:end evaluate
