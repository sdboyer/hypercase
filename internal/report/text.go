package report

import (
	"bufio"
	"io"
	"strings"
)

// WriteText writes the ranked changes as plain text. Each rank that has
// changes has a heading, in rank order. Under it is one line per cell: its
// case, its column, and its outcome, or the change in it, so the outcome
// file is never listed as a file. Under each cell is one line per other
// file of the cell that was added, changed or removed.
func (r *Report) WriteText(w io.Writer) error {
	bw := bufio.NewWriter(w)
	for i, ch := range r.Changes {
		if i == 0 || r.Changes[i-1].Rank != ch.Rank {
			bw.WriteString(ch.Rank.String() + "\n")
		}
		bw.WriteString("  " + ch.Case + " " + ch.Column + ": " + strings.Join(ch.Outcomes(), " -> ") + "\n")
		for _, f := range ch.Files {
			bw.WriteString("    " + f.Path + " " + f.How + "\n")
		}
	}
	return bw.Flush()
}

// Outcomes are the outcomes the change shows: at A and at B when the
// outcome changed, at A when the cell disappeared, and at B otherwise.
func (ch Change) Outcomes() []string {
	switch ch.Rank {
	case OutcomeChanged:
		return []string{ch.Before, ch.After}
	case Disappeared:
		return []string{ch.Before}
	}
	return []string{ch.After}
}
