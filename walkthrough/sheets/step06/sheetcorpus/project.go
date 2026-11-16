// walkthrough/sheets/step06/sheetcorpus/project.go
package sheetcorpus

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"

	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
)

// doc:begin project
var Project = decl.Project{
	UpdateVar:   "SHEETS_UPDATE_GOLDEN",
	RenderError: renderError,
}

// renderError writes the error's text, then quotes each line it points at.
func renderError(in fs.FS, err error) []byte {
	out := []byte(err.Error() + "\n")
	for _, p := range sheet.Positions(err) {
		out = fmt.Appendf(out, "  %s:%d: %s\n", p.Path, p.Line, lineAt(in, p.Path, p.Line))
	}
	return out
}

// doc:end project

// lineAt is line n of the file at path in in, or its first line when n is
// 0.
func lineAt(in fs.FS, path string, n int) string {
	b, err := fs.ReadFile(in, path)
	if err != nil {
		return ""
	}
	s := bufio.NewScanner(bytes.NewReader(b))
	for i := 1; s.Scan(); i++ {
		if i == n || n == 0 {
			return s.Text()
		}
	}
	return ""
}

var Circular = decl.Property{
	Name:        "circular",
	Description: "Some cell's formula refers back to itself, directly or through other cells.",
}

var Malformed = decl.Property{
	Name:        "malformed",
	Description: "Some cell holds neither a number nor a formula.",
}

var Dangling = decl.Property{
	Name:        "dangling",
	Description: "Some formula refers to a cell the sheet doesn't have.",
}
