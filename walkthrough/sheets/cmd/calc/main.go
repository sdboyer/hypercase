// Command calc prints a sheet's values.
//
// Usage:
//
//	calc DIR
//
// DIR is a sheet: a directory with one file per cell, named for the cell (A1,
// B1, …), holding a number or a formula such as =B1*2. calc writes the values
// to stdout as CSV, one row of the sheet per line, and a warning to stderr for
// each cell whose value is a division by zero of its own. When the sheet can't be evaluated, calc
// writes why to stderr and exits with status 1.
package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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
	if err := sheet.Calc(mount{dir, os.DirFS(dir)}, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "calc: %v\n", err)
		return 1
	}
	return 0
}

// mount presents a directory of cells as the sheet/ the sheet package reads.
// An error opening a cell names the cell's file by its path on disk.
type mount struct {
	dir   string
	cells fs.FS
}

func (m mount) Open(name string) (fs.File, error) {
	if name == "sheet" {
		return m.open(".")
	}
	if rest, ok := strings.CutPrefix(name, "sheet/"); ok && fs.ValidPath(name) {
		return m.open(rest)
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (m mount) open(name string) (fs.File, error) {
	f, err := m.cells.Open(name)
	if e, ok := err.(*fs.PathError); ok {
		e.Path = filepath.Join(m.dir, filepath.FromSlash(name))
	}
	return f, err
}
