package report

import (
	"bufio"
	"io"
	"strings"
)

// WriteText writes the problems and the ranked changes as plain text. The
// problems, when there are any, come first, under the heading problems:
// under it, a line per revision that has them, in the report's order, and
// under that, one line per problem, its place, where it has one, then what
// it is. Each rank that has changes has a heading, in rank order. Under it
// is one line per cell: its case, its column, and its outcome, or the
// change in it, so the outcome file is never listed as a file. Under each
// cell is one line per other file of the cell that was added, changed or
// removed.
func (r *Report) WriteText(w io.Writer) error {
	bw := bufio.NewWriter(w)
	if len(r.Problems) > 0 {
		bw.WriteString("problems\n")
	}
	for _, g := range problemGroups(r.Problems) {
		bw.WriteString("  at " + g.At + "\n")
		for _, p := range g.Problems {
			line := p.Msg
			if p.Where != "" {
				line = p.Where + ": " + line
			}
			bw.WriteString("    " + line + "\n")
		}
	}
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
// outcome changed, or when only its bytes did and its state is unknown at
// one revision; at A when the cell disappeared; and at B otherwise.
func (ch Change) Outcomes() []string {
	switch {
	case ch.Rank == OutcomeChanged, ch.Rank == BytesChanged && ch.Before != ch.After:
		return []string{ch.Before, ch.After}
	case ch.Rank == Disappeared:
		return []string{ch.Before}
	}
	return []string{ch.After}
}
