package sheet

import (
	"bytes"
	"fmt"
	"io/fs"
	"testing/fstest"
)

// Loop returns the sheet in fsys with its first cell, in cell order, replaced
// by a reference to itself, so the sheet has a cycle: A1 becomes =A1. Every
// other cell is kept byte for byte, and the sheet is at sheet/ in the
// returned file system. Loop fails on anything under sheet/ that is not a
// cell file, on a sheet with no cells, and on one whose first cell is already
// that reference, since Loop would then change nothing.
func Loop(fsys fs.FS) (fs.FS, error) {
	files, err := readCells(fsys)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("the sheet has no cells")
	}
	out := fstest.MapFS{}
	for i, f := range files {
		if _, err := checkFile(f); err != nil {
			return nil, err
		}
		data := bytes.Clone(f.data)
		if i == 0 {
			data = []byte("=" + f.name + "\n")
			if bytes.Equal(data, f.data) {
				return nil, fmt.Errorf("%s already refers to itself", f.name)
			}
		}
		out[f.path] = &fstest.MapFile{Data: data}
	}
	return out, nil
}
