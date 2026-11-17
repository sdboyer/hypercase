// Package cells reads an archive's cells from its sections' names, as
// hypercase strip reads them: the cell a section's name gives, whether the
// section is part of that cell, and the cells the archive's gaps lines
// mark. The root-package check reads them the same way, to say which gaps
// lines keep strip from removing a section. It doesn't import testing.
package cells

import (
	"strings"

	"github.com/sdboyer/hypercase/archive"
)

// A Cell names a cell by its case and its column. Kind and Name are "" for
// the base's cells.
type Cell struct{ Kind, Name, Column string }

// Of returns the cell the named section's name gives, that of a column's
// out/<column>/… or via/<column>/… in any case, and whether it gives one.
func Of(section string) (Cell, bool) {
	kind, name, rest := archive.SplitCase(section)
	top, below, ok := strings.Cut(rest, "/")
	if !ok || (top != archive.Out && top != archive.Via) {
		return Cell{}, false
	}
	col, _, _ := strings.Cut(below, "/")
	return Cell{kind, name, col}, true
}

// Part reports whether the named section, one whose name gives a cell, is
// part of that cell. An out/<column>/… is. A via/<column>/… is a bytes
// lift's product, part of the lift's cell and of no other column's, so it
// is unless bytesLifts gives its column false. bytesLifts says, of the
// columns over the archive's corpus that the caller knows, which are bytes
// lifts. A column it doesn't hold may be a bytes lift, so a via/ of one is
// part of its cell; nil knows no column.
func Part(section string, bytesLifts map[string]bool) bool {
	_, _, rest := archive.SplitCase(section)
	col, ok := strings.CutPrefix(rest, archive.Via+"/")
	if !ok {
		return true
	}
	col, _, _ = strings.Cut(col, "/")
	bytes, known := bytesLifts[col]
	return bytes || !known
}

// A Mark is the gaps line that marks a cell.
type Mark struct {
	// Case is "" for the base, or variant/<name> or transform/<name> for
	// one of its variants.
	Case string
	// Gaps is the section that holds the line: gaps, for the base's cells
	// and a transform's variant's, or variant/<name>/gaps, for a
	// hand-written variant's. Line is the line's number in it.
	Gaps string
	Line int
}

// Marks returns the cells the archive's gaps lines mark, with the line
// that marks each: the base's gaps mark the base's cells and, by lines
// that name a transform, its variant's; a hand-written variant's gaps
// mark its own.
func Marks(a *archive.Archive) map[Cell]Mark {
	m := map[Cell]Mark{}
	for _, g := range a.Gaps() {
		c, cs := Cell{Column: g.Column}, ""
		if g.Transform != "" {
			c.Kind, c.Name = archive.Transform, g.Transform
			cs = archive.Transform + "/" + g.Transform
		}
		m[c] = Mark{Case: cs, Gaps: archive.Gaps, Line: g.Line}
	}
	for _, v := range a.Variants() {
		if v.Kind != archive.Variant {
			continue
		}
		for _, g := range v.Gaps {
			m[Cell{archive.Variant, v.Name, g.Column}] = Mark{Case: v.Case(), Gaps: v.Prefix() + archive.Gaps, Line: g.Line}
		}
	}
	return m
}
