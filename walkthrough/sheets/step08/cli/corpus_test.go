// walkthrough/sheets/step08/cli/corpus_test.go
package cli

import (
	"io"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step08/sheetcorpus"
)

func init() {
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("step08"),
		sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling,
		sheetcorpus.SheetType,
		hypercase.Own(calc),
	)
}

var calc = hypercase.Suite[sheetcorpus.Sheet]{
	Name:        "calc",
	Description: "Runs the calc command on the sheet.",
	Outputs: []decl.Output{
		{Pattern: "stdout.txt", Description: "The sheet's values as CSV, one row per line.", Inputs: decl.AllInputs},
		{Pattern: "stderr.txt", Description: "A warning for each cell that divides by zero.", Inputs: decl.AllInputs},
		{Pattern: "error.txt", Description: "Why calc failed.", Inputs: decl.FromPositions},
	},
	Rejects: []decl.Property{sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling},
	Run: func(t testing.TB, archive string, in sheetcorpus.Sheet, stdout, stderr io.Writer) ([]hypercase.File, error) {
		return nil, sheet.Calc(in, stdout, stderr)
	},
}

func TestCorpus(t *testing.T) { hypercase.Test(t) }
