// walkthrough/sheets/step05/cli/corpus_test.go
package cli

import (
	"io"
	"io/fs"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step05/sheetcorpus"
)

func init() {
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("step05"),
		sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling,
		calc,
	)
}

// doc:begin calc
var calc = hypercase.Suite[fs.FS]{
	Name:        "calc",
	Description: "Runs the calc command on the sheet.",
	Corpus:      "sheet",
	Outputs: []decl.Output{
		{Pattern: "stdout.txt", Description: "The sheet's values as CSV, one row per line."},
		{Pattern: "stderr.txt", Description: "A warning for each cell that divides by zero."},
		{Pattern: "error.txt", Description: "Why calc failed."},
	},
	Rejects: []decl.Property{sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling},
	Run: func(t testing.TB, archive string, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
		return nil, sheet.Calc(in, stdout, stderr)
	},
}

// doc:end calc

func TestCorpus(t *testing.T) { hypercase.Test(t) }
