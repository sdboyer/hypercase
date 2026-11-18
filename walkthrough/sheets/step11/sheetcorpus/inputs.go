// walkthrough/sheets/step11/sheetcorpus/inputs.go
package sheetcorpus

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
)

// Cells maps an output file to every cell of the sheet, whole: a cell's
// value depends on the cells its formula refers to, and theirs on others.
var Cells = decl.InputsFunc(func(in fs.FS, _ string) ([]decl.Range, error) {
	cells, err := fs.Glob(in, "sheet/*")
	if err != nil {
		return nil, err
	}
	rs := make([]decl.Range, len(cells))
	for i, c := range cells {
		rs[i] = decl.Range{Path: c} // 0, 0: the whole file
	}
	return rs, nil
})

// Formulas maps an output file to the cells that hold a formula, whole:
// the cells format prints.
var Formulas = decl.InputsFunc(func(in fs.FS, _ string) ([]decl.Range, error) {
	p, err := sheet.Parse(in)
	if err != nil {
		return nil, err
	}
	var rs []decl.Range
	for _, name := range p.Formulas() {
		rs = append(rs, decl.Range{Path: "sheet/" + name})
	}
	return rs, nil
})

// Field maps a product file of import, one cell of the sheet, to the lines
// of the CSV export its field spans.
var Field = decl.InputsFunc(func(in fs.FS, path string) ([]decl.Range, error) {
	csvs, err := fs.Glob(in, "csv/*.csv")
	if err != nil {
		return nil, err
	}
	if len(csvs) != 1 {
		return nil, fmt.Errorf("the input holds %q, and a CSV export is one file", csvs)
	}
	first, last, err := sheet.FieldLines(in, csvs[0], strings.TrimPrefix(path, "sheet/"))
	if err != nil {
		return nil, err
	}
	return []decl.Range{{Path: csvs[0], Start: first, End: last}}, nil
})

// positions finds the lines an error points at, sheet.Positions's: each
// line it names, or the whole file for a position about the file as a
// whole, whose Line is 0. Its paths are in the input view already, so it
// doesn't read the view.
func positions(_ fs.FS, err error) []decl.Range {
	var rs []decl.Range
	for _, p := range sheet.Positions(err) {
		rs = append(rs, decl.Range{Path: p.Path, Start: p.Line, End: p.Line})
	}
	return rs
}
