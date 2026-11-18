package report

import (
	"bufio"
	"io"
	"strings"
)

// WriteText writes the problems, the changed findings and the ranked
// changes as plain text. The problems, when there are any, come first,
// under the heading problems: under it, a line per revision that has them,
// in the report's order, and under that, one line per problem, its place,
// where it has one, then what it is. The changed findings, when there are
// any, come next, under the heading findings changed: under it, a line per
// check whose findings changed, its package's directory, then its name,
// and under that, each finding only at A, marked -, then each only at B,
// marked +. Each rank that has changes has a heading, in rank order. Under it
// is one line per cell: its case, its column, and its outcome, or the
// change in it, so the outcome file is never listed as a file, and nor is
// the inputs file. Under each cell is one line per other file of the cell
// that was added, changed or removed, or whose reverse map alone changed,
// and under a file that both revisions map, what changed in its map: each
// line of A's that B's lacks, marked -, then each of B's that A's lacks,
// marked +.
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
	if len(r.Findings) > 0 {
		bw.WriteString("findings changed\n")
	}
	for _, fc := range r.Findings {
		bw.WriteString("  " + fc.Package + " " + fc.Check + "\n")
		for _, f := range fc.Disappeared {
			bw.WriteString("    - " + f + "\n")
		}
		for _, f := range fc.Appeared {
			bw.WriteString("    + " + f + "\n")
		}
	}
	for i, ch := range r.Changes {
		if i == 0 || r.Changes[i-1].Rank != ch.Rank {
			bw.WriteString(ch.Rank.String() + "\n")
		}
		bw.WriteString("  " + ch.Case + " " + ch.Column + ": " + strings.Join(ch.Outcomes(), " -> ") + "\n")
		for _, f := range ch.Files {
			bw.WriteString("    " + f.Path + " " + f.How + "\n")
			for _, l := range f.Ranges {
				bw.WriteString("      " + string(l.Kind) + " " + l.Text + "\n")
			}
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
