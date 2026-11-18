// walkthrough/sheets/step03/sheetcorpus/inputs.go
package sheetcorpus

import (
	"io/fs"

	"github.com/sdboyer/hypercase/decl"
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
