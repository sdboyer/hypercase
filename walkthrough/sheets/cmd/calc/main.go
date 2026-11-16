// Command calc prints a sheet's values.
//
// Usage:
//
//	calc DIR
//
// DIR is a sheet: a directory with one file per cell, named for the cell (A1,
// B1, …), holding a number or a formula such as =B1*2. calc writes the values
// to stdout as CSV, one row of the sheet per line, and a warning to stderr for
// each cell that divides by zero. When the sheet can't be evaluated, calc
// writes why to stderr and exits with status 1.
package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: calc DIR")
		return 2
	}
	dir := args[0]
	if info, err := os.Stat(dir); err != nil {
		fmt.Fprintf(stderr, "calc: %v\n", err)
		return 1
	} else if !info.IsDir() {
		fmt.Fprintf(stderr, "calc: %s is not a directory\n", dir)
		return 1
	}
	if err := sheet.Calc(mount{os.DirFS(dir)}, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "calc: %v\n", err)
		return 1
	}
	return 0
}

// mount presents a directory of cells as the sheet/ the sheet package reads.
type mount struct{ cells fs.FS }

func (m mount) Open(name string) (fs.File, error) {
	if name == "sheet" {
		return m.cells.Open(".")
	}
	if rest, ok := strings.CutPrefix(name, "sheet/"); ok && fs.ValidPath(name) {
		return m.cells.Open(rest)
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}
