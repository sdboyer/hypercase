package sheet

import (
	"bytes"
	"fmt"
	"io/fs"
	"testing/fstest"
)

// MoveDown returns the sheet in fsys moved one row down: A1 becomes A2, and
// every reference in a formula is rewritten to the moved cell, so =B1*2
// becomes =B2*2. Everything else in each cell's file is kept byte for byte,
// and a cell that is not a formula moves unchanged, however malformed. A
// reference to a missing cell moves too. The sheet is at sheet/ in the
// returned file system. MoveDown fails on anything under sheet/ that is not a
// cell file, and on a cell or reference on the last row.
func MoveDown(fsys fs.FS) (fs.FS, error) {
	files, err := readCells(fsys)
	if err != nil {
		return nil, err
	}
	out := fstest.MapFS{}
	for _, f := range files {
		p, err := checkFile(f)
		if err != nil {
			return nil, err
		}
		if p.row == lastRow {
			return nil, fmt.Errorf("%s is on the last row, so it can't move down", f.name)
		}
		data, err := moveRefs(f.name, f.data)
		if err != nil {
			return nil, err
		}
		out["sheet/"+pos{p.col, p.row + 1}.String()] = &fstest.MapFile{Data: data}
	}
	return out, nil
}

// moveRefs rewrites each reference in the formula on a cell file's first line
// to the cell one row down.
func moveRefs(name string, data []byte) ([]byte, error) {
	line, _, _ := bytes.Cut(data, []byte("\n"))
	eq := len(line) - len(bytes.TrimLeft(line, " \t"))
	if eq == len(line) || line[eq] != '=' {
		return bytes.Clone(data), nil
	}
	src := eq + 1
	var out []byte
	done := 0
	for _, t := range lex(string(line[src:])) {
		if t.kind != tRef {
			continue
		}
		p, _ := parseName(t.text)
		if p.row == lastRow {
			return nil, fmt.Errorf("%s: %s is on the last row, so it can't move down", name, t.text)
		}
		off := src + t.off
		out = append(out, data[done:off]...)
		out = append(out, pos{p.col, p.row + 1}.String()...)
		done = off + len(t.text)
	}
	return append(out, data[done:]...), nil
}
