// walkthrough/sheets/step06/sheetcorpus/types.go
package sheetcorpus

import (
	"io"
	"io/fs"

	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
)

// doc:begin types
type Sheet struct{ fs.FS } // in/sheet/A1, in/sheet/B1, …

var SheetType = decl.Type[Sheet]{
	Dir:         "sheet", // testdata/sheet/*.txtar
	Description: "A spreadsheet: one file per cell under in/sheet/, holding a number or a formula.",
}

var Parse = decl.Lift[Sheet, *sheet.Parsed]{
	Name:        "parse",
	Description: "Parses every cell.",
	Outputs: []decl.Output{
		{Pattern: "error.txt", Description: "The cell that is neither a number nor a formula."},
	},
	Rejects: []decl.Property{Malformed},
	Run: func(archive string, in Sheet, stderr io.Writer) (*sheet.Parsed, error) {
		return sheet.Parse(in)
	},
}

var Resolve = decl.Lift[*sheet.Parsed, *sheet.Graph]{
	Name:        "resolve",
	Description: "Orders the cells so that each comes after the cells it refers to.",
	Outputs: []decl.Output{
		{Pattern: "error.txt", Description: "The cycle, or the reference to a missing cell."},
	},
	Rejects: []decl.Property{Circular, Dangling},
	Run: func(archive string, p *sheet.Parsed, stderr io.Writer) (*sheet.Graph, error) {
		return sheet.Resolve(p)
	},
}

// doc:end types
