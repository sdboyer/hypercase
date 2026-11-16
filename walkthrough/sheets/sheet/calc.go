package sheet

import (
	"fmt"
	"io"
	"io/fs"
)

// Calc evaluates the sheet in fsys and writes its values to stdout as CSV, one
// row of the sheet per line, from row 1 to the last row with a cell and from
// column A to the last column with one. An empty cell is an empty field. For
// each cell that divides by zero, in cell order, it writes a warning to stderr.
// It returns the error that stops the sheet being evaluated, and writes nothing
// when there is one.
func Calc(fsys fs.FS, stdout, stderr io.Writer) error {
	p, err := Parse(fsys)
	if err != nil {
		return err
	}
	g, err := Resolve(p)
	if err != nil {
		return err
	}
	vals, divs := g.evaluate()

	var last pos
	at := make(map[pos]float64, len(p.cells))
	for _, c := range p.cells {
		last.col = max(last.col, c.pos.col)
		last.row = max(last.row, c.pos.row)
		at[c.pos] = vals[c.name]
	}
	var out []byte
	for row := 1; row <= last.row; row++ {
		for col := 1; col <= last.col; col++ {
			if col > 1 {
				out = append(out, ',')
			}
			if v, ok := at[pos{col, row}]; ok {
				out = appendValue(out, v)
			}
		}
		out = append(out, '\n')
	}
	if _, err := stdout.Write(out); err != nil {
		return err
	}

	var warn []byte
	for _, c := range divs {
		warn = fmt.Appendf(warn, "%s: divides by zero\n", c.name)
	}
	if len(warn) > 0 {
		if _, err := stderr.Write(warn); err != nil {
			return err
		}
	}
	return nil
}
