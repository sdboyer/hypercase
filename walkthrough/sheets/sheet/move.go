package sheet

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"testing/fstest"
)

// Movable reports whether MoveDown would move the sheet in fsys, and why not
// when it wouldn't: the sheet has no cells, or a cell, or a reference in a
// formula, is on the last row, with no row below it. A sheet holding
// something that is not a cell file, or one that can't be read, is movable:
// MoveDown reports what is wrong with it.
func Movable(fsys fs.FS) (why string, ok bool) {
	_, err := MoveDown(fsys)
	return probe(err)
}

// MoveDown returns the sheet in fsys moved one row down: A1 becomes A2, and
// every reference in a formula is rewritten to the moved cell, so =B1*2
// becomes =B2*2. Everything else in each cell's file is kept byte for byte,
// and a cell that is not a formula moves unchanged, however malformed. A
// reference to a missing cell moves too. The sheet is at sheet/ in the
// returned file system. MoveDown fails on anything under sheet/ that is not a
// cell file, and on a sheet Movable declines.
func MoveDown(fsys fs.FS) (fs.FS, error) {
	files, err := readCells(fsys)
	if err != nil {
		return nil, err
	}
	at, err := checkFiles(files)
	if err != nil {
		return nil, err
	}
	return moveDown(files, at)
}

// checkFiles returns each cell file's position, and fails on the first file,
// in cell order, that can't be a cell.
func checkFiles(files []cellFile) ([]pos, error) {
	at := make([]pos, len(files))
	for i, f := range files {
		p, err := checkFile(f)
		if err != nil {
			return nil, err
		}
		at[i] = p
	}
	return at, nil
}

// A refusal is why a transform can't apply to a sheet: a fact about the
// sheet, not something wrong with it.
type refusal string

func (r refusal) Error() string { return string(r) }

// probe answers a transform's probe from the error the transform gave on the
// sheet: it declines the sheet, with the reason, on a refusal, and lets the
// sheet through on any other error, for the transform to report.
func probe(err error) (why string, ok bool) {
	var r refusal
	if errors.As(err, &r) {
		return string(r), false
	}
	return "", true
}

// moveDown moves cell files, at their positions, one row down. It refuses a
// sheet with no cells, which it would leave unchanged, and one with a cell or
// reference on the last row.
func moveDown(files []cellFile, at []pos) (fs.FS, error) {
	if len(files) == 0 {
		return nil, refusal("the sheet has no cells")
	}
	out := fstest.MapFS{}
	for i, f := range files {
		if at[i].row == lastRow {
			return nil, refusal(fmt.Sprintf("%s is on the last row, so it can't move down", f.name))
		}
		data, err := moveRefs(f.name, f.data)
		if err != nil {
			return nil, err
		}
		out["sheet/"+pos{at[i].col, at[i].row + 1}.String()] = &fstest.MapFile{Data: data}
	}
	return out, nil
}

// moveRefs rewrites each reference in the formula on a cell file's first line
// to the cell one row down. It refuses a reference on the last row.
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
			return nil, refusal(fmt.Sprintf("%s: %s is on the last row, so it can't move down", name, t.text))
		}
		off := src + t.off
		out = append(out, data[done:off]...)
		out = append(out, pos{p.col, p.row + 1}.String()...)
		done = off + len(t.text)
	}
	return append(out, data[done:]...), nil
}
