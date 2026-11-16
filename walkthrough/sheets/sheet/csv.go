package sheet

import (
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
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	out := fstest.MapFS{}
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
			out["sheet/"+pos{col, row}.String()] = &fstest.MapFile{Data: append(field, '\n')}
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
