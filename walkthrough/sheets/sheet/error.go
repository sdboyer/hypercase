package sheet

import (
	"errors"
	"fmt"
	"slices"
)

// Codes are the codes of the errors Parse and Resolve return:
//
//   - S101: a cell holds neither a number nor a formula, or the sheet holds
//     something that is not a cell.
//   - S201: a cycle, some cell's formula referring back to itself, directly
//     or through other cells.
//   - S202: a formula refers to a cell the sheet doesn't have.
var Codes = []string{codeMalformed, codeCycle, codeDangling}

// CSVCodes are the codes of the errors ImportCSV returns:
//
//   - C101: a quoted field never closes.
//   - C102: a quote inside an unquoted field, or text after a closing quote.
//   - C103: a field past the last column, XFD, or the last row, 1048576.
var CSVCodes = []string{codeUnterminated, codeStrayQuote, codeTooLarge}

const (
	codeMalformed    = "S101"
	codeCycle        = "S201"
	codeDangling     = "S202"
	codeUnterminated = "C101"
	codeStrayQuote   = "C102"
	codeTooLarge     = "C103"
)

// An Error is a problem with a sheet or a CSV file. Its message names the cell,
// or the line of the CSV file, and its positions give each line it is about.
type Error struct {
	Code      string     // one of Codes or CSVCodes
	Positions []Position // in the order the message names them
	msg       string
}

func (e *Error) Error() string { return e.msg }

// A Position is a line of a file that an error is about.
type Position struct {
	Cell string // the cell whose file Path is, or "" for a line of a CSV file
	Path string // the file, as the file system the sheet was read from names it: sheet/A1
	Line int    // counted from 1; 0 when the error is about the file as a whole
}

// Positions returns the positions of the first *Error in err's tree, or nil if
// it has none.
func Positions(err error) []Position {
	var e *Error
	if errors.As(err, &e) {
		return slices.Clone(e.Positions)
	}
	return nil
}

func newError(code string, at []Position, format string, args ...any) *Error {
	return &Error{Code: code, Positions: at, msg: fmt.Sprintf(format, args...)}
}
