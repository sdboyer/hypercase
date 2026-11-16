// walkthrough/sheets/step10/sheetcorpus/project.go
package sheetcorpus

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"

	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
)

// doc:begin project
var Project = decl.Project{
	UpdateVar:   "SHEETS_UPDATE_GOLDEN",
	RenderError: renderError,
	Codes: func(err error) []string {
		var e *sheet.Error
		if errors.As(err, &e) {
			return []string{e.Code}
		}
		return nil
	},
}

// doc:end project

// renderError writes the error's text, then quotes each line it points at. A
// position about a file as a whole names only the file.
func renderError(in fs.FS, err error) []byte {
	out := []byte(err.Error() + "\n")
	for _, p := range sheet.Positions(err) {
		if p.Line == 0 {
			out = fmt.Appendf(out, "  %s\n", p.Path)
			continue
		}
		if line := lineAt(in, p.Path, p.Line); line != "" {
			out = fmt.Appendf(out, "  %s:%d: %s\n", p.Path, p.Line, line)
		} else {
			out = fmt.Appendf(out, "  %s:%d\n", p.Path, p.Line)
		}
	}
	return out
}

// lineAt is line n, counted from 1, of the file at path in in, or "" when
// there is no such line.
func lineAt(in fs.FS, path string, n int) string {
	b, err := fs.ReadFile(in, path)
	if err != nil {
		return ""
	}
	for i := 1; ; i++ {
		line, rest, found := bytes.Cut(b, []byte("\n"))
		if i == n {
			return string(line)
		}
		if !found {
			return ""
		}
		b = rest
	}
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
