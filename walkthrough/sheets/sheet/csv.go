package sheet

import (
	"fmt"
	"io/fs"
	"testing/fstest"
)

// ImportCSV reads the CSV file at path in fsys and returns a sheet holding one
// cell per field that is not empty: record 1's field 2 is B1. Each cell's file
// holds the field and a newline. A field may be quoted, "like this", to hold a
// comma, a newline or a doubled quote; records end with \n or \r\n. The sheet
// is at sheet/ in the returned file system. Its cells are not parsed, so a
// field that is neither a number nor a formula is a malformed cell, and fails
// whatever parses the sheet.
//
// An error about the file's contents is an *Error with a code from CSVCodes,
// whose message and position give the line of the file it is on.
func ImportCSV(fsys fs.FS, path string) (fs.FS, error) {
	fields, err := readCSV(fsys, path)
	if err != nil {
		return nil, err
	}
	out := fstest.MapFS{}
	for name, f := range fields {
		out["sheet/"+name] = &fstest.MapFile{Data: f.data}
	}
	return out, nil
}

// FieldLines reads the CSV file at path in fsys, as ImportCSV does, and
// returns the lines of the file, counted from 1, that the field of the
// named cell spans: one line, or more for a quoted field that holds a
// newline. It returns the error ImportCSV returns for the file, and an
// error for a cell ImportCSV makes no file for.
func FieldLines(fsys fs.FS, path, cell string) (first, last int, err error) {
	fields, err := readCSV(fsys, path)
	if err != nil {
		return 0, 0, err
	}
	f, ok := fields[cell]
	if !ok {
		return 0, 0, fmt.Errorf("%s has no field for the cell %s", path, cell)
	}
	return f.first, f.last, nil
}

// A csvField is a field of a CSV file that is not empty: its cell's file,
// and the lines of the CSV file it spans.
type csvField struct {
	data        []byte
	first, last int
}

// readCSV reads the CSV file at path in fsys into its fields that are not
// empty, by the name of each one's cell.
func readCSV(fsys fs.FS, path string) (map[string]csvField, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	out := map[string]csvField{}
	line, row, col := 1, 1, 1
	fail := func(code string, line int, msg string) error {
		return newError(code, []Position{{Path: path, Line: line}}, "line %d: %s", line, msg)
	}
	for i := 0; ; {
		start := line
		var field []byte
		if i < len(data) && data[i] == '"' {
			for i++; ; i++ {
				if i == len(data) {
					return nil, fail(codeUnterminated, start, "quote never closes")
				}
				if data[i] == '"' {
					if i+1 < len(data) && data[i+1] == '"' {
						i++
					} else {
						i++
						break
					}
				} else if data[i] == '\n' {
					line++
				}
				field = append(field, data[i])
			}
			if i < len(data) && data[i] != ',' && !endsRecord(data, i) {
				return nil, fail(codeStrayQuote, line, "text after a closing quote")
			}
		} else {
			for ; i < len(data) && data[i] != ',' && !endsRecord(data, i); i++ {
				if data[i] == '"' {
					return nil, fail(codeStrayQuote, line, "quote inside an unquoted field")
				}
				field = append(field, data[i])
			}
		}

		if len(field) > 0 {
			switch {
			case col > lastCol:
				return nil, fail(codeTooLarge, start, "past the last column, XFD")
			case row > lastRow:
				return nil, fail(codeTooLarge, start, "past the last row, 1048576")
			}
			out[pos{col, row}.String()] = csvField{data: append(field, '\n'), first: start, last: line}
		}

		switch {
		case i == len(data):
			return out, nil
		case data[i] == ',':
			i++
			col++
			continue
		case data[i] == '\r':
			i++
		}
		i++ // the \n
		line++
		row++
		col = 1
		if i == len(data) {
			return out, nil
		}
	}
}

// endsRecord reports whether data[i:] starts with \n or \r\n.
func endsRecord(data []byte, i int) bool {
	return data[i] == '\n' || data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n'
}
