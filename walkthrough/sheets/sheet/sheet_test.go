package sheet_test

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
)

// describe renders an error as its code, its message and its positions, or as
// "plain: <message>" when it is not an *Error.
func describe(err error) string {
	if err == nil {
		return "<nil>"
	}
	var e *sheet.Error
	if !errors.As(err, &e) {
		return "plain: " + err.Error()
	}
	s := e.Code + " " + e.Error()
	for _, p := range e.Positions {
		s += fmt.Sprintf(" @%s %s:%d", p.Cell, p.Path, p.Line)
	}
	return s
}

// sheetFS lays cells out as a sheet's files.
func sheetFS(cells map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, data := range cells {
		fsys["sheet/"+name] = &fstest.MapFile{Data: []byte(data)}
	}
	return fsys
}

func TestEval(t *testing.T) {
	tests := []struct {
		name  string
		cells map[string]string
		want  string // Values.Format
	}{
		{"empty", map[string]string{}, ""},
		{"number", map[string]string{"A1": "3\n"}, "A1 3\n"},
		{"no newline", map[string]string{"A1": "3"}, "A1 3\n"},
		{"fraction", map[string]string{"A1": "2.50"}, "A1 2.5\n"},
		{"leading zeros", map[string]string{"A1": "007"}, "A1 7\n"},
		{"spaces around", map[string]string{"A1": " \t3 \t", "B1": "  = A1 * 2 "}, "A1 3\nB1 6\n"},
		{"twice", map[string]string{"A1": "=B1*2", "B1": "3"}, "A1 6\nB1 3\n"},
		{"prices", map[string]string{"A1": "4", "B1": "6", "C1": "=A1+B1"}, "A1 4\nB1 6\nC1 10\n"},
		{"precedence", map[string]string{"A1": "=2+3*4"}, "A1 14\n"},
		{"parentheses", map[string]string{"A1": "=(2+3)*4"}, "A1 20\n"},
		{"left to right", map[string]string{"A1": "=10-4-3", "B1": "=24/4/2"}, "A1 3\nB1 3\n"},
		{"negative result", map[string]string{"A1": "=1-4"}, "A1 -3\n"},
		{"negative zero", map[string]string{"A1": "=(0-1)*0"}, "A1 0\n"},
		{"inexact", map[string]string{"A1": "=0.1+0.2", "B1": "=1/3"}, "A1 0.30000000000000004\nB1 0.3333333333333333\n"},
		{"large", map[string]string{"A1": "1000000000000000000000"}, "A1 1000000000000000000000\n"},
		{"underflow rounds to zero", map[string]string{"A1": "0." + strings.Repeat("0", 400) + "1"}, "A1 0\n"},
		{"chain", map[string]string{"A1": "=B1+1", "B1": "=C1+1", "C1": "1"}, "A1 3\nB1 2\nC1 1\n"},
		{"repeated reference", map[string]string{"A1": "=B1*B1", "B1": "3"}, "A1 9\nB1 3\n"},
		{"cell order", map[string]string{"B2": "1", "A10": "2", "A2": "3", "AA1": "4", "B1": "5", "A1": "6"},
			"A1 6\nB1 5\nAA1 4\nA2 3\nB2 1\nA10 2\n"},
		{"divide by zero", map[string]string{"A1": "1", "B1": "0", "C1": "=A1/B1"}, "A1 1\nB1 0\nC1 #DIV/0!\n"},
		{"zero over zero", map[string]string{"A1": "=0/0"}, "A1 #DIV/0!\n"},
		{"division by zero passes through", map[string]string{"A1": "=1/0", "B1": "=A1*0+1"}, "A1 #DIV/0!\nB1 #DIV/0!\n"},
		{"overflow", map[string]string{"A1": "=" + bigNumber + "*10"}, "A1 #NUM!\n"},
		{"overflow passes through", map[string]string{"A1": "=" + bigNumber + "*10", "B1": "=A1-A1"}, "A1 #NUM!\nB1 #NUM!\n"},
		{"left fault wins", map[string]string{"A1": "=1/0", "B1": "=" + bigNumber + "*10", "C1": "=A1+B1", "D1": "=B1+A1"},
			"A1 #DIV/0!\nB1 #NUM!\nC1 #DIV/0!\nD1 #NUM!\n"},
		{"last cell", map[string]string{"XFD1048576": "1", "A1": "=XFD1048576+1"}, "A1 2\nXFD1048576 1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sheet.EvalCells(tt.cells)
			if err != nil {
				t.Fatalf("EvalCells: %v", err)
			}
			if s := string(got.Format()); s != tt.want {
				t.Errorf("EvalCells:\ngot:\n%s\nwant:\n%s", s, tt.want)
			}
			got, err = sheet.Eval(sheetFS(tt.cells))
			if err != nil {
				t.Fatalf("Eval: %v", err)
			}
			if s := string(got.Format()); s != tt.want {
				t.Errorf("Eval:\ngot:\n%s\nwant:\n%s", s, tt.want)
			}
		})
	}
}

// bigNumber is the largest float64, written out.
var bigNumber = "17976931348623157" + strings.Repeat("0", 292)

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name  string
		cells map[string]string
		want  string // describe
	}{
		{"word", map[string]string{"A1": "4", "D1": "total\n"}, `S101 D1: not a number or a formula: "total" @D1 sheet/D1:1`},
		{"empty file", map[string]string{"A1": ""}, `S101 A1: not a number or a formula: "" @A1 sheet/A1:1`},
		{"blank line", map[string]string{"A1": " \n"}, `S101 A1: not a number or a formula: "" @A1 sheet/A1:1`},
		{"reference without =", map[string]string{"A1": "B1"}, `S101 A1: not a number or a formula: "B1" @A1 sheet/A1:1`},
		{"number then word", map[string]string{"A1": "3x"}, `S101 A1: not a number or a formula: "3x" @A1 sheet/A1:1`},
		{"carriage return", map[string]string{"A1": "3\r\n"}, `S101 A1: not a number or a formula: "3\r" @A1 sheet/A1:1`},
		{"negative number", map[string]string{"A1": "-3"}, `S101 A1: unexpected "-" @A1 sheet/A1:1`},
		{"two numbers", map[string]string{"A1": "3 4"}, `S101 A1: unexpected "4" @A1 sheet/A1:1`},
		{"two lines", map[string]string{"A1": "3\n4\n"}, `S101 A1: more than one line @A1 sheet/A1:2`},
		{"trailing blank line", map[string]string{"A1": "3\n\n"}, `S101 A1: more than one line @A1 sheet/A1:2`},
		{"number out of range", map[string]string{"A1": "1" + strings.Repeat("0", 309)}, `S101 A1: number out of range: "1` + strings.Repeat("0", 309) + `" @A1 sheet/A1:1`},
		{"leading dot", map[string]string{"A1": ".5"}, `S101 A1: not a number or a formula: ".5" @A1 sheet/A1:1`},
		{"empty formula", map[string]string{"A1": "="}, `S101 A1: unexpected end of formula @A1 sheet/A1:1`},
		{"formula ends early", map[string]string{"A1": "=B1*"}, `S101 A1: unexpected end of formula @A1 sheet/A1:1`},
		{"unclosed parenthesis", map[string]string{"A1": "=(B1"}, `S101 A1: unexpected end of formula @A1 sheet/A1:1`},
		{"extra parenthesis", map[string]string{"A1": "=B1)"}, `S101 A1: unexpected ")" @A1 sheet/A1:1`},
		{"unary minus", map[string]string{"A1": "=-B1"}, `S101 A1: unexpected "-" @A1 sheet/A1:1`},
		{"two operands", map[string]string{"A1": "=B1 C1"}, `S101 A1: unexpected "C1" @A1 sheet/A1:1`},
		{"word in formula", map[string]string{"A1": "=total"}, `S101 A1: unexpected "total" @A1 sheet/A1:1`},
		{"lower-case reference", map[string]string{"A1": "=b1"}, `S101 A1: unexpected "b1" @A1 sheet/A1:1`},
		{"reference past the last column", map[string]string{"A1": "=XFE1"}, `S101 A1: unexpected "XFE1" @A1 sheet/A1:1`},
		{"reference past the last row", map[string]string{"A1": "=A1048577"}, `S101 A1: unexpected "A1048577" @A1 sheet/A1:1`},
		{"number out of range in formula", map[string]string{"A1": "=1" + strings.Repeat("0", 309)}, `S101 A1: number out of range: "1` + strings.Repeat("0", 309) + `" @A1 sheet/A1:1`},
		{"first in cell order", map[string]string{"B2": "x", "A10": "y", "C1": "z"}, `S101 C1: not a number or a formula: "z" @C1 sheet/C1:1`},
		{"cells before other names", map[string]string{"A2": "x", "a1": "1"}, `S101 A2: not a number or a formula: "x" @A2 sheet/A2:1`},
		{"lower-case name", map[string]string{"a1": "1"}, `S101 a1: not a cell name @ sheet/a1:0`},
		{"leading zero in name", map[string]string{"A01": "1"}, `S101 A01: not a cell name @ sheet/A01:0`},
		{"row zero", map[string]string{"A0": "1"}, `S101 A0: not a cell name @ sheet/A0:0`},
		{"no row", map[string]string{"A": "1"}, `S101 A: not a cell name @ sheet/A:0`},
		{"no column", map[string]string{"1": "1"}, `S101 1: not a cell name @ sheet/1:0`},
		{"four letters", map[string]string{"AAAA1": "1"}, `S101 AAAA1: not a cell name @ sheet/AAAA1:0`},
		{"past the last column", map[string]string{"XFE1": "1"}, `S101 XFE1: not a cell name @ sheet/XFE1:0`},
		{"past the last row", map[string]string{"A1048577": "1"}, `S101 A1048577: not a cell name @ sheet/A1048577:0`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sheet.Parse(sheetFS(tt.cells))
			if got := describe(err); got != tt.want {
				t.Errorf("Parse:\ngot  %s\nwant %s", got, tt.want)
			}
			_, err = sheet.EvalCells(tt.cells)
			if got := describe(err); got != tt.want {
				t.Errorf("EvalCells:\ngot  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestParseFS(t *testing.T) {
	tests := []struct {
		name string
		fsys fstest.MapFS
		want string // describe, or the formulas when Parse succeeds
	}{
		{"no sheet", fstest.MapFS{"other/A1": {Data: []byte("1")}}, ""},
		{"other trees ignored", fstest.MapFS{"sheet/A1": {Data: []byte("=1")}, "csv/x.csv": {}}, "A1"},
		{"directory", fstest.MapFS{"sheet/A1/x": {Data: []byte("1")}}, `S101 A1: a directory, not a cell @A1 sheet/A1:0`},
		{"directory not named for a cell", fstest.MapFS{"sheet/notes/x": {}}, `S101 notes: not a cell name @ sheet/notes:0`},
		{"sheet is a file", fstest.MapFS{"sheet": {Data: []byte("1")}}, `plain: readdir sheet: not implemented`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := sheet.Parse(tt.fsys)
			got := describe(err)
			if err == nil {
				got = strings.Join(p.Formulas(), " ")
			}
			if got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name  string
		cells map[string]string
		want  string // describe, or the order when Resolve succeeds
	}{
		{"numbers keep cell order", map[string]string{"B1": "1", "A1": "2", "A2": "3"}, "A1 B1 A2"},
		{"after what it refers to", map[string]string{"A1": "=B1*2", "B1": "3"}, "B1 A1"},
		{"chain", map[string]string{"A1": "=B1", "B1": "=C1", "C1": "1"}, "C1 B1 A1"},
		{"references in formula order", map[string]string{"A1": "=C1+B1", "B1": "1", "C1": "2"}, "C1 B1 A1"},
		{"shared", map[string]string{"A1": "=C1", "B1": "=C1", "C1": "1"}, "C1 A1 B1"},
		{"loop", map[string]string{"A1": "=B1", "B1": "=A1"},
			"S201 A1 -> B1 -> A1: cycle @A1 sheet/A1:1 @B1 sheet/B1:1"},
		{"self", map[string]string{"A1": "=A1"}, "S201 A1 -> A1: cycle @A1 sheet/A1:1"},
		{"cycle past a cell", map[string]string{"A1": "=B1", "B1": "=C1", "C1": "=B1*2"},
			"S201 B1 -> C1 -> B1: cycle @B1 sheet/B1:1 @C1 sheet/C1:1"},
		{"cycle starts at its first cell", map[string]string{"A1": "=C1", "C1": "=B1", "B1": "=C1"},
			"S201 B1 -> C1 -> B1: cycle @B1 sheet/B1:1 @C1 sheet/C1:1"},
		{"cycle in row order", map[string]string{"B1": "=A2", "A2": "=B1"},
			"S201 B1 -> A2 -> B1: cycle @B1 sheet/B1:1 @A2 sheet/A2:1"},
		{"dangling", map[string]string{"A1": "=B1*2"}, "S202 A1: B1 doesn't exist @A1 sheet/A1:1"},
		{"first dangling in cell order", map[string]string{"B1": "=D1", "A2": "=C1"}, "S202 B1: D1 doesn't exist @B1 sheet/B1:1"},
		{"first dangling in formula order", map[string]string{"A1": "=D1+C1"}, "S202 A1: D1 doesn't exist @A1 sheet/A1:1"},
		{"dangling before cycle", map[string]string{"A1": "=A1", "B1": "=C1"}, "S202 B1: C1 doesn't exist @B1 sheet/B1:1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := sheet.Parse(sheetFS(tt.cells))
			if err != nil {
				t.Fatal(err)
			}
			g, err := sheet.Resolve(p)
			got := describe(err)
			if err == nil {
				got = strings.Join(g.Order(), " ")
			}
			if got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
			if _, err := sheet.EvalCells(tt.cells); err != nil && describe(err) != tt.want {
				t.Errorf("EvalCells: got %s, want %s", describe(err), tt.want)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		formula string
		want    string
	}{
		{"=B1*2", "=B1 * 2"},
		{"=A1+B1", "=A1 + B1"},
		{"=  A1   +B1 ", "=A1 + B1"},
		{"=2.50+007", "=2.5 + 7"},
		{"=1000000000000000000000", "=1000000000000000000000"},
		{"=((A1))", "=A1"},
		{"=(A1+B1)*C1", "=(A1 + B1) * C1"},
		{"=A1+(B1*C1)", "=A1 + B1 * C1"},
		{"=(A1-B1)-C1", "=A1 - B1 - C1"},
		{"=A1-(B1-C1)", "=A1 - (B1 - C1)"},
		{"=A1-(B1+C1)", "=A1 - (B1 + C1)"},
		{"=A1+(B1+C1)", "=A1 + (B1 + C1)"},
		{"=A1/(B1*C1)", "=A1 / (B1 * C1)"},
		{"=(A1/B1)*C1", "=A1 / B1 * C1"},
		{"=A1*(B1/C1)", "=A1 * (B1 / C1)"},
		{"=(A1*B1)+(C1/D1)", "=A1 * B1 + C1 / D1"},
	}
	for _, tt := range tests {
		t.Run(tt.formula, func(t *testing.T) {
			p, err := sheet.Parse(sheetFS(map[string]string{"A2": tt.formula, "B2": "1"}))
			if err != nil {
				t.Fatal(err)
			}
			want := "A2 " + tt.want + "\n"
			if got := string(sheet.Format(p)); got != want {
				t.Errorf("got %q, want %q", got, want)
			}
			// The canonical form parses to itself.
			again, err := sheet.Parse(sheetFS(map[string]string{"A2": tt.want}))
			if err != nil {
				t.Fatal(err)
			}
			if got := string(sheet.Format(again)); got != want {
				t.Errorf("formatted again: got %q, want %q", got, want)
			}
		})
	}
}

func TestFormatCells(t *testing.T) {
	p, err := sheet.Parse(sheetFS(map[string]string{"B2": "=A1", "A1": "3", "C1": "=A1/2", "A2": "4"}))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(sheet.Format(p)), "C1 =A1 / 2\nB2 =A1\n"; got != want {
		t.Errorf("Format: got %q, want %q", got, want)
	}
	if got, want := strings.Join(p.Formulas(), " "), "C1 B2"; got != want {
		t.Errorf("Formulas: got %q, want %q", got, want)
	}
	p, err = sheet.Parse(sheetFS(map[string]string{"A1": "3"}))
	if err != nil {
		t.Fatal(err)
	}
	if got := sheet.Format(p); len(got) != 0 {
		t.Errorf("Format of a sheet with no formulas: got %q", got)
	}
	if got := p.Formulas(); len(got) != 0 {
		t.Errorf("Formulas of a sheet with no formulas: got %q", got)
	}
}

func TestValuesFormat(t *testing.T) {
	v := sheet.Values{"B1": 2, "A1": 1, "not a cell": 4, "a1": 3, "A2": 5}
	want := "A1 1\nB1 2\nA2 5\na1 3\nnot a cell 4\n"
	if got := string(v.Format()); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestCalc(t *testing.T) {
	tests := []struct {
		name   string
		cells  map[string]string
		stdout string
		stderr string
		err    string // describe
	}{
		{"empty", map[string]string{}, "", "", "<nil>"},
		{"twice", map[string]string{"A1": "=B1*2", "B1": "3"}, "6,3\n", "", "<nil>"},
		{"prices", map[string]string{"A1": "4", "B1": "6", "C1": "=A1+B1"}, "4,6,10\n", "", "<nil>"},
		{"gaps", map[string]string{"A1": "1", "C2": "=A1+1"}, "1,,\n,,2\n", "", "<nil>"},
		{"starts past A1", map[string]string{"B2": "1"}, ",\n,1\n", "", "<nil>"},
		{"divides by zero", map[string]string{"A1": "1", "B1": "0", "C1": "=A1/B1", "A2": "=C1+1", "B2": "=1/0+1/0"},
			"1,0,#DIV/0!\n#DIV/0!,#DIV/0!,\n", "C1: divides by zero\nB2: divides by zero\n", "<nil>"},
		{"overflow", map[string]string{"A1": "=" + bigNumber + "*2"}, "#NUM!\n", "", "<nil>"},
		{"malformed", map[string]string{"A1": "4", "D1": "total"}, "", "",
			`S101 D1: not a number or a formula: "total" @D1 sheet/D1:1`},
		{"cycle", map[string]string{"A1": "=B1*2", "B1": "=A1"}, "", "",
			"S201 A1 -> B1 -> A1: cycle @A1 sheet/A1:1 @B1 sheet/B1:1"},
		{"dangling", map[string]string{"A1": "=B1*2"}, "", "", "S202 A1: B1 doesn't exist @A1 sheet/A1:1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			err := sheet.Calc(sheetFS(tt.cells), &stdout, &stderr)
			if got := describe(err); got != tt.err {
				t.Errorf("error: got %s, want %s", got, tt.err)
			}
			if got := stdout.String(); got != tt.stdout {
				t.Errorf("stdout: got %q, want %q", got, tt.stdout)
			}
			if got := stderr.String(); got != tt.stderr {
				t.Errorf("stderr: got %q, want %q", got, tt.stderr)
			}
		})
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestCalcWriteErrors(t *testing.T) {
	cells := sheetFS(map[string]string{"A1": "=1/0"})
	if err := sheet.Calc(cells, failWriter{}, &strings.Builder{}); describe(err) != "plain: disk full" {
		t.Errorf("stdout: got %s", describe(err))
	}
	if err := sheet.Calc(cells, &strings.Builder{}, failWriter{}); describe(err) != "plain: disk full" {
		t.Errorf("stderr: got %s", describe(err))
	}
}

// cellsOf lists the files of a sheet in fsys, one per line: the cell's name,
// a space, and the file's contents quoted.
func cellsOf(t *testing.T, fsys fs.FS) string {
	t.Helper()
	var b strings.Builder
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %q\n", path, data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestImportCSV(t *testing.T) {
	tests := []struct {
		name string
		csv  string
		want string // cellsOf, or describe
	}{
		{"empty", "", ""},
		{"prices", "4,6,=A1+B1\n", "sheet/A1 \"4\\n\"\nsheet/B1 \"6\\n\"\nsheet/C1 \"=A1+B1\\n\"\n"},
		{"no final newline", "4,6", "sheet/A1 \"4\\n\"\nsheet/B1 \"6\\n\"\n"},
		{"rows", "1\n2,3\n", "sheet/A1 \"1\\n\"\nsheet/A2 \"2\\n\"\nsheet/B2 \"3\\n\"\n"},
		{"crlf", "1,2\r\n3\r\n", "sheet/A1 \"1\\n\"\nsheet/A2 \"3\\n\"\nsheet/B1 \"2\\n\"\n"},
		{"lone carriage return", "1\r2\n", "sheet/A1 \"1\\r2\\n\"\n"},
		{"empty fields", ",2,,4,\n", "sheet/B1 \"2\\n\"\nsheet/D1 \"4\\n\"\n"},
		{"empty line is an empty row", "1\n\n3\n", "sheet/A1 \"1\\n\"\nsheet/A3 \"3\\n\"\n"},
		{"spaces kept", " 4 ,x\n", "sheet/A1 \" 4 \\n\"\nsheet/B1 \"x\\n\"\n"},
		{"quoted", `"4","a,b",""` + "\n", "sheet/A1 \"4\\n\"\nsheet/B1 \"a,b\\n\"\n"},
		{"doubled quote", `"say ""hi"""` + "\n", "sheet/A1 \"say \\\"hi\\\"\\n\"\n"},
		{"newline in quotes", "\"1\n2\",3\n4\n", "sheet/A1 \"1\\n2\\n\"\nsheet/A2 \"4\\n\"\nsheet/B1 \"3\\n\"\n"},
		{"quoted then crlf", "\"1\"\r\n2", "sheet/A1 \"1\\n\"\nsheet/A2 \"2\\n\"\n"},
		{"last column", strings.Repeat(",", 16383) + "1\n", "sheet/XFD1 \"1\\n\"\n"},
		{"empty past the last column", strings.Repeat(",", 16384) + "\n", ""},
		{"last row", strings.Repeat("\n", 1048575) + "1\n", "sheet/A1048576 \"1\\n\"\n"},
		{"empty rows past the last", strings.Repeat("\n", 1048580), ""},

		{"open quote", `4,"six,=A1+B1` + "\n", "C101 line 1: quote never closes @ csv/in.csv:1"},
		{"open quote on a later line", "1\n2,\"x\n\ny\n", "C101 line 2: quote never closes @ csv/in.csv:2"},
		{"quote in an unquoted field", "1\n2,ab\"c\n", "C102 line 2: quote inside an unquoted field @ csv/in.csv:2"},
		{"text after a closing quote", "\"a\nb\"c,1\n", "C102 line 2: text after a closing quote @ csv/in.csv:2"},
		{"carriage return after a closing quote", "\"a\"\r", "C102 line 1: text after a closing quote @ csv/in.csv:1"},
		{"past the last column", strings.Repeat(",", 16384) + "1\n", "C103 line 1: past the last column, XFD @ csv/in.csv:1"},
		{"past the last row", strings.Repeat("\n", 1048576) + "1\n", "C103 line 1048577: past the last row, 1048576 @ csv/in.csv:1048577"},
		{"row counts records, not lines", "\"a\nb\"\n" + strings.Repeat("\n", 1048575) + "1\n",
			"C103 line 1048578: past the last row, 1048576 @ csv/in.csv:1048578"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := fstest.MapFS{"csv/in.csv": {Data: []byte(tt.csv)}}
			cells, err := sheet.ImportCSV(in, "csv/in.csv")
			got := describe(err)
			if err == nil {
				got = cellsOf(t, cells)
			}
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestImportCSVEvaluates(t *testing.T) {
	in := fstest.MapFS{"csv/prices.csv": {Data: []byte("4,6,=A1+B1\n")}}
	cells, err := sheet.ImportCSV(in, "csv/prices.csv")
	if err != nil {
		t.Fatal(err)
	}
	var stdout strings.Builder
	if err := sheet.Calc(cells, &stdout, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "4,6,10\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestImportCSVMissing(t *testing.T) {
	_, err := sheet.ImportCSV(fstest.MapFS{}, "csv/none.csv")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("got %v, want a not-exist error", err)
	}
	if sheet.Positions(err) != nil {
		t.Errorf("a read error has positions: %v", sheet.Positions(err))
	}
}

func TestMoveDown(t *testing.T) {
	tests := []struct {
		name  string
		cells map[string]string
		want  string // cellsOf, or describe
	}{
		{"empty", map[string]string{}, ""},
		{"twice", map[string]string{"A1": "=B1*2\n", "B1": "3\n"}, "sheet/A2 \"=B2*2\\n\"\nsheet/B2 \"3\\n\"\n"},
		{"spacing kept", map[string]string{"A1": "  = B1 *(C9+ XFD7) \n"}, "sheet/A2 \"  = B2 *(C10+ XFD8) \\n\"\n"},
		{"not a reference", map[string]string{"A1": "=b1+B1x+B1"}, "sheet/A2 \"=b1+B1x+B2\"\n"},
		{"value cell unchanged", map[string]string{"A1": "B1\n"}, "sheet/A2 \"B1\\n\"\n"},
		{"malformed kept", map[string]string{"A1": "=B1 +\n", "D1": "total\n"}, "sheet/A2 \"=B2 +\\n\"\nsheet/D2 \"total\\n\"\n"},
		{"later lines kept", map[string]string{"A1": "=B1\n=B1\n"}, "sheet/A2 \"=B2\\n=B1\\n\"\n"},
		{"cycle kept", map[string]string{"A1": "=A1"}, "sheet/A2 \"=A2\"\n"},
		{"dangling kept", map[string]string{"A1": "=Z9"}, "sheet/A2 \"=Z10\"\n"},
		{"cell on the last row", map[string]string{"A1048576": "1"}, "plain: A1048576 is on the last row, so it can't move down"},
		{"reference to the last row", map[string]string{"A1": "=B1048576"}, "plain: A1: B1048576 is on the last row, so it can't move down"},
		{"not a cell", map[string]string{"notes": "1"}, "S101 notes: not a cell name @ sheet/notes:0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			moved, err := sheet.MoveDown(sheetFS(tt.cells))
			got := describe(err)
			if err == nil {
				got = cellsOf(t, moved)
			}
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// Moving a sheet down moves its values with it.
func TestMoveDownEvaluates(t *testing.T) {
	moved, err := sheet.MoveDown(sheetFS(map[string]string{"A1": "=B1*2", "B1": "3", "C1": "=A1+B1"}))
	if err != nil {
		t.Fatal(err)
	}
	v, err := sheet.Eval(moved)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(v.Format()), "A2 6\nB2 3\nC2 9\n"; got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestHasFormula(t *testing.T) {
	tests := []struct {
		name string
		fsys fstest.MapFS
		want bool
	}{
		{"no sheet", fstest.MapFS{}, false},
		{"numbers", sheetFS(map[string]string{"A1": "1", "B1": "2"}), false},
		{"formula", sheetFS(map[string]string{"A1": "1", "B1": "=A1"}), true},
		{"indented formula", sheetFS(map[string]string{"A1": " \t=1"}), true},
		{"malformed formula", sheetFS(map[string]string{"A1": "=+"}), true},
		{"= on a later line", sheetFS(map[string]string{"A1": "1\n=2"}), false},
		{"= after text", sheetFS(map[string]string{"A1": "x=1"}), false},
		{"unreadable", fstest.MapFS{"sheet/A1/x": {}}, true},
		{"sheet is a file", fstest.MapFS{"sheet": {}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sheet.HasFormula(tt.fsys); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPositions(t *testing.T) {
	_, err := sheet.EvalCells(map[string]string{"A1": "=B1", "B1": "=A1"})
	wrapped := fmt.Errorf("evaluating: %w", err)
	want := []sheet.Position{{Cell: "A1", Path: "sheet/A1", Line: 1}, {Cell: "B1", Path: "sheet/B1", Line: 1}}
	got := sheet.Positions(wrapped)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	got[0].Line = 9
	if again := sheet.Positions(err); again[0].Line != 1 {
		t.Errorf("Positions shares its result with the error")
	}
	if got := sheet.Positions(errors.New("plain")); got != nil {
		t.Errorf("a plain error: got %v", got)
	}
	if got := sheet.Positions(nil); got != nil {
		t.Errorf("nil: got %v", got)
	}
}

func TestCodes(t *testing.T) {
	if got, want := strings.Join(sheet.Codes, " "), "S101 S201 S202"; got != want {
		t.Errorf("Codes: got %s, want %s", got, want)
	}
	if got, want := strings.Join(sheet.CSVCodes, " "), "C101 C102 C103"; got != want {
		t.Errorf("CSVCodes: got %s, want %s", got, want)
	}
}
