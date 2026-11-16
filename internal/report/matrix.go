package report

import (
	"maps"
	"slices"
	"strings"
)

// A Matrix is one corpus at B: its cases by its columns.
type Matrix struct {
	Corpus  string
	Columns []Column // by name
	Rows    []Row    // by case
}

// A Column is one of a corpus's columns: each the registries at B record
// over the corpus, and each that some cell or gaps line names.
type Column struct {
	Name        string
	Description Text // "" for a column no registry records
}

// A Row is one case of a corpus.
type Row struct {
	Case        string
	Description Text
	Properties  []Property
	Cells       []Cell // one per column, in the matrix's column order
}

// A Cell is one cell of the matrix.
type Cell struct {
	Column string
	// Present says the cell has committed files.
	Present bool
	// Outcome is its outcome, as the outcome file holds it less its
	// newline, and State its first word: "" for a cell with no outcome file.
	Outcome, State string
	// Gap is the text of the gaps line that marks the cell, or nil. A cell
	// that a line marks and that has committed files is frozen.
	Gap    Text
	Frozen bool
	// Changed says the cell changed between A and B, and Change how.
	Changed bool
	Change  Rank
}

// matrices returns each corpus the registries at B declare, by name.
func matrices(b *snapshot, changes []Change, links *linker) []Matrix {
	changed := map[[2]string]Rank{}
	for _, ch := range changes {
		if ch.Rank != Disappeared {
			changed[[2]string{ch.Case, ch.Column}] = ch.Rank
		}
	}
	var out []Matrix
	for _, corpus := range slices.Sorted(maps.Keys(b.columns)) {
		m := Matrix{Corpus: corpus}
		cols := map[string]bool{}
		for name := range b.columns[corpus] {
			cols[name] = true
		}
		for k := range b.cells {
			if k.c.corpus == corpus {
				cols[k.column] = true
			}
		}
		for k := range b.gaps {
			if k.c.corpus == corpus {
				cols[k.column] = true
			}
		}
		for _, name := range slices.Sorted(maps.Keys(cols)) {
			m.Columns = append(m.Columns, Column{Name: name, Description: links.text(b.columns[corpus][name])})
		}
		var cases []caseKey
		for k := range b.cases {
			if k.corpus == corpus {
				cases = append(cases, k)
			}
		}
		slices.SortFunc(cases, caseKey.compare)
		for _, k := range cases {
			row := Row{
				Case:        k.String(),
				Description: links.text(b.cases[k].description),
				Properties:  b.properties(k, links),
			}
			for _, col := range m.Columns {
				ck := cellKey{c: k, column: col.Name}
				cell := Cell{Column: col.Name}
				if c := b.cells[ck]; c != nil {
					cell.Present = true
					if c.outcome != nil {
						cell.Outcome = c.outcomeLine()
					}
					cell.State = c.state()
				}
				if text, ok := b.gaps[ck]; ok {
					cell.Gap = links.text(text)
					cell.Frozen = cell.Present
				}
				if r, ok := changed[[2]string{row.Case, col.Name}]; ok {
					cell.Changed, cell.Change = true, r
				}
				row.Cells = append(row.Cells, cell)
			}
			m.Rows = append(m.Rows, row)
		}
		out = append(out, m)
	}
	return out
}

// class names a rank for the page's style sheet.
func (r Rank) class() string { return strings.ReplaceAll(r.String(), " ", "-") }
