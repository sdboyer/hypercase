// walkthrough/sheets/step10/sheetcorpus/types.go
package sheetcorpus

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"unicode"

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

// view gives a sheet's every file as a jenny returns them, by its path in the
// sheet's input view.
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

// doc:begin import
type CSV struct{ fs.FS } // in/csv/<name>.csv

var CSVType = decl.Type[CSV]{
	Dir:         "csv",
	Description: "A CSV export: one file, in/csv/<name>.csv, with a name holding a character that is neither white space nor a format character; one row of the sheet per record, where a quoted field may hold a newline.",
}

var Unterminated = decl.Property{
	Name:        "unterminated",
	Description: "Some quoted field is never closed.",
}

var Import = decl.Lift[CSV, Sheet]{
	Name:        "import",
	Description: "Turns each CSV field into a cell: row 1, field 2 is B1.",
	Outputs: []decl.Output{
		{Pattern: "error.txt", Description: "The line the importer can't read, and why."},
	},
	Product: []decl.Output{
		{Pattern: "sheet/*", Description: "One file per cell, as a sheet's in/sheet/ holds them."},
	},
	Rejects: []decl.Property{Unterminated},
	Jenny: jenny.For[CSV]().New("import", func(in CSV) (jenny.Files, error) {
		var paths []string
		err := fs.WalkDir(in, ".", func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				paths = append(paths, p)
			}
			return err
		})
		if err != nil {
			return nil, err
		}
		var name string
		ok := len(paths) == 1 && path.Dir(paths[0]) == "csv"
		if ok {
			name, ok = strings.CutSuffix(path.Base(paths[0]), ".csv")
		}
		ok = ok && strings.ContainsFunc(name, func(r rune) bool {
			return !unicode.In(r, unicode.White_Space, unicode.Cf)
		})
		if !ok {
			return nil, fmt.Errorf("the input holds %q, and a CSV export is one file, csv/<name>.csv, with a name holding a character that is neither white space nor a format character", paths)
		}
		s, err := sheet.ImportCSV(in, paths[0])
		var e *sheet.Error
		if errors.As(err, &e) {
			return nil, decl.Reject(err)
		}
		if err != nil {
			return nil, err
		}
		return view(s, nil)
	}),
}

// doc:end import
