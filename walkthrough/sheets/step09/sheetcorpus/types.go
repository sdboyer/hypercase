// walkthrough/sheets/step09/sheetcorpus/types.go
package sheetcorpus

import (
	"io"
	"io/fs"

	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
	"github.com/sdboyer/jenny"
)

type Sheet struct{ fs.FS } // in/sheet/A1, in/sheet/B1, …

var Looped = decl.Transform[Sheet]{
	Name:        "looped",
	Description: "The first cell in cell order, row by row then column by column, replaced by a reference to itself, so the sheet has a cycle.",
	Gives:       []decl.Property{Circular},
	Probe:       func(base Sheet) (why string, ok bool) { return sheet.Loopable(base) },
	Jenny: jenny.For[Sheet]().New("looped", func(base Sheet) (jenny.Files, error) {
		return view(sheet.Loop(base))
	}),
}

var Renamed = decl.Transform[Sheet]{
	Name:        "renamed",
	Description: "Every cell moved one row down, A1 to A2, with every reference rewritten to the moved cell.",
	Keeps:       []decl.Property{Circular, Dangling},
	Probe:       func(base Sheet) (why string, ok bool) { return sheet.Movable(base) },
	Jenny: jenny.For[Sheet]().New("renamed", func(base Sheet) (jenny.Files, error) {
		return view(sheet.MoveDown(base))
	}),
}

var SheetType = decl.Type[Sheet]{
	Dir:         "sheet",
	Description: "A spreadsheet: one file per cell under in/sheet/, holding a number or a formula.",
	Transforms:  []decl.Transform[Sheet]{Looped, Renamed},
}

// view gives a sheet's every file as a transform's jenny returns them, by its
// path in the sheet's input view.
func view(fsys fs.FS, err error) (jenny.Files, error) {
	if err != nil {
		return nil, err
	}
	var files jenny.Files
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		files = append(files, jenny.NewFile(p, data))
		return nil
	})
	return files, err
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
