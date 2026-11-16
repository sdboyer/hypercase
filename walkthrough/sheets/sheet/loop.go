package sheet

import (
	"io/fs"
	"testing/fstest"
)

// Loopable reports whether Loop would change the sheet in fsys, and why not
// when it wouldn't: the sheet has no cells, or its first cell, in cell order,
// already holds a reference to itself and nothing else, however it is spelled
// (=A1, = (A1)). A sheet holding something that is not a cell file, or one
// that can't be read, is loopable: Loop reports what is wrong with it.
func Loopable(fsys fs.FS) (why string, ok bool) {
	_, err := Loop(fsys)
	return probe(err)
}

// Loop returns the sheet in fsys with its first cell, in cell order, replaced
// by a reference to itself, so the sheet has a cycle: A1 becomes =A1. Every
// other cell is kept byte for byte, and the sheet is at sheet/ in the
// returned file system. Loop fails on anything under sheet/ that is not a
// cell file, and on a sheet Loopable declines, since Loop would then change
// nothing.
func Loop(fsys fs.FS) (fs.FS, error) {
	files, err := readCells(fsys)
	if err != nil {
		return nil, err
	}
	if _, err := checkFiles(files); err != nil {
		return nil, err
	}
	if why := loopRefusal(files); why != "" {
		return nil, refusal(why)
	}
	out := fstest.MapFS{}
	for i, f := range files {
		data := f.data
		if i == 0 {
			data = []byte("=" + f.name + "\n")
		}
		out[f.path] = &fstest.MapFile{Data: data}
	}
	return out, nil
}

// loopRefusal says why Loop would leave a sheet's cells, in cell order,
// unchanged, or is "" when it wouldn't.
func loopRefusal(files []cellFile) string {
	if len(files) == 0 {
		return "the sheet has no cells"
	}
	c, err := parseCell(files[0])
	if err != nil {
		return ""
	}
	if r, ok := c.formula.(ref); ok && string(r) == c.name {
		return c.name + " already refers to itself"
	}
	return ""
}
