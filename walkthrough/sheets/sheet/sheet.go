// Package sheet evaluates spreadsheets kept as files.
//
// A sheet is a directory, sheet/, with one file per cell, named for the cell:
// A1, B1, … up to column XFD and row 1048576. A cell file holds one line: a
// number, such as 3 or 2.5, or a formula, such as =B1*2. A formula combines
// numbers and references to other cells with + - * / and parentheses.
//
// Parse reads the cells, Resolve orders them so that each comes after the
// cells it refers to, and Graph.Evaluate computes their values. Eval does all
// three, and Calc prints the values as CSV. Format prints formulas back in
// canonical form, ImportCSV makes a sheet from a CSV file, and MoveDown moves
// a sheet one row down.
//
// Functions that read a sheet take a file system holding it at sheet/, and
// errors name files by their paths in it. Every error about a sheet's
// contents is an *Error, with a code from Codes or CSVCodes.
package sheet

import (
	"bytes"
	"errors"
	"io/fs"
	"slices"
	"strings"
)

// Parsed is a sheet whose cells have each been parsed.
type Parsed struct {
	cells []*cell // in cell order
}

type cell struct {
	pos     pos
	name    string
	path    string
	formula expr    // nil when the cell holds a number
	value   float64 // the number, when formula is nil
	refs    []string
}

func (c *cell) at() []Position { return []Position{{Cell: c.name, Path: c.path, Line: 1}} }

// Formulas returns the names of the cells that hold a formula, in cell order.
func (p *Parsed) Formulas() []string {
	var names []string
	for _, c := range p.cells {
		if c.formula != nil {
			names = append(names, c.name)
		}
	}
	return names
}

// Format prints each formula cell and its formula in canonical form, one per
// line, in cell order: A1 =B1 * 2.
func Format(p *Parsed) []byte {
	var b []byte
	for _, c := range p.cells {
		if c.formula == nil {
			continue
		}
		b = append(b, c.name...)
		b = append(b, " ="...)
		b = appendExpr(b, c.formula)
		b = append(b, '\n')
	}
	return b
}

// Parse reads and parses every cell of the sheet in fsys. A file system with
// no sheet/ holds an empty sheet. Parse fails with S101 on the first problem
// it meets, taking the cells in cell order, then whatever under sheet/ is not
// named for a cell, by name.
func Parse(fsys fs.FS) (*Parsed, error) {
	files, err := readCells(fsys)
	if err != nil {
		return nil, err
	}
	return parseFiles(files)
}

// EvalCells evaluates the sheet whose cells are given by name, each with what
// its file holds. Errors name each cell's file as sheet/<name>.
func EvalCells(cells map[string]string) (Values, error) {
	files := make([]cellFile, 0, len(cells))
	for name, data := range cells {
		files = append(files, cellFile{name: name, path: "sheet/" + name, data: []byte(data)})
	}
	sortFiles(files)
	p, err := parseFiles(files)
	if err != nil {
		return nil, err
	}
	return evalParsed(p)
}

// Eval parses, resolves and evaluates the sheet in fsys.
func Eval(fsys fs.FS) (Values, error) {
	p, err := Parse(fsys)
	if err != nil {
		return nil, err
	}
	return evalParsed(p)
}

func evalParsed(p *Parsed) (Values, error) {
	g, err := Resolve(p)
	if err != nil {
		return nil, err
	}
	return g.Evaluate(), nil
}

// HasFormula reports whether some cell of the sheet in fsys holds a formula,
// without parsing the sheet. A sheet it can't read counts as having one, so
// that whatever reads the sheet next reports the error.
func HasFormula(fsys fs.FS) bool {
	entries, err := fs.ReadDir(fsys, "sheet")
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	if err != nil {
		return true
	}
	for _, e := range entries {
		data, err := fs.ReadFile(fsys, "sheet/"+e.Name())
		if err != nil {
			return true
		}
		line, _, _ := bytes.Cut(data, []byte("\n"))
		if strings.HasPrefix(strings.TrimLeft(string(line), " \t"), "=") {
			return true
		}
	}
	return false
}

type cellFile struct {
	name, path string
	data       []byte
	dir        bool
}

// readCells reads every entry under sheet/, in cell order.
func readCells(fsys fs.FS) ([]cellFile, error) {
	entries, err := fs.ReadDir(fsys, "sheet")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	files := make([]cellFile, 0, len(entries))
	for _, e := range entries {
		f := cellFile{name: e.Name(), path: "sheet/" + e.Name(), dir: e.IsDir()}
		if !f.dir {
			if f.data, err = fs.ReadFile(fsys, f.path); err != nil {
				return nil, err
			}
		}
		files = append(files, f)
	}
	sortFiles(files)
	return files, nil
}

func sortFiles(files []cellFile) {
	slices.SortFunc(files, func(a, b cellFile) int { return compareNames(a.name, b.name) })
}

// checkFile reports a file that can't be a cell: one whose name is not a cell
// name, or a directory.
func checkFile(f cellFile) (pos, error) {
	p, ok := parseName(f.name)
	if !ok {
		return pos{}, newError(codeMalformed, []Position{{Path: f.path}}, "%s: not a cell name", f.name)
	}
	if f.dir {
		return pos{}, newError(codeMalformed, []Position{{Cell: f.name, Path: f.path}}, "%s: a directory, not a cell", f.name)
	}
	return p, nil
}

func parseFiles(files []cellFile) (*Parsed, error) {
	p := &Parsed{cells: make([]*cell, 0, len(files))}
	for _, f := range files {
		c, err := parseCell(f)
		if err != nil {
			return nil, err
		}
		p.cells = append(p.cells, c)
	}
	return p, nil
}

func parseCell(f cellFile) (*cell, error) {
	p, err := checkFile(f)
	if err != nil {
		return nil, err
	}
	c := &cell{pos: p, name: f.name, path: f.path}
	line, rest, _ := bytes.Cut(f.data, []byte("\n"))
	if len(rest) > 0 {
		return nil, newError(codeMalformed, []Position{{Cell: c.name, Path: c.path, Line: 2}}, "%s: more than one line", c.name)
	}
	text := strings.Trim(string(line), " \t")
	if src, ok := strings.CutPrefix(text, "="); ok {
		e, err := parseFormula(src)
		if err != nil {
			return nil, newError(codeMalformed, c.at(), "%s: %v", c.name, err)
		}
		c.formula, c.refs = e, refsOf(e)
		return c, nil
	}
	toks := lex(text)
	var bad error
	switch {
	case len(f.data) == 0:
		// An empty file has no line to point at.
		return nil, newError(codeMalformed, []Position{{Cell: c.name, Path: c.path}}, "%s: not a number or a formula: %q", c.name, text)
	case len(toks) == 0 || toks[0].kind == tWord || toks[0].kind == tRef:
		return nil, newError(codeMalformed, c.at(), "%s: not a number or a formula: %q", c.name, text)
	case toks[0].kind != tNumber:
		bad = (&parser{toks: toks}).unexpected()
	case len(toks) > 1:
		bad = (&parser{toks: toks, i: 1}).unexpected()
	default:
		c.value, bad = parseNumber(toks[0].text)
	}
	if bad != nil {
		return nil, newError(codeMalformed, c.at(), "%s: %v", c.name, bad)
	}
	return c, nil
}
