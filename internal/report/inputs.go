package report

import (
	"bytes"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/diff"
	"github.com/sdboyer/hypercase/internal/registry"
	"github.com/sdboyer/hypercase/internal/revmap"
)

// The page's input lines grow with what each changed file maps, so they
// are capped, each cap with a note of what it leaves out.
const (
	// MaxRangeLines caps the lines of one range the page shows: its first
	// MaxRangeLines, then a line that says how many more it leaves out.
	MaxRangeLines = 20
	// MaxFileLines caps the input lines the page shows beside one changed
	// file, over all its sections and both revisions: past them, a note
	// says how many more lines its ranges hold, of how many sections.
	MaxFileLines = 4 * MaxRangeLines
)

// Inputs are the input lines a changed file comes from, as its
// revisions' reverse maps give them: B's map of a file at B, A's of a file
// only at A, and both where both revisions map the file, differently.
type Inputs struct {
	// Maps are what the shown maps state, A's then B's, each nil where the
	// page shows none of that revision's.
	Maps [2]*Map
	// Excerpts are the lines the shown maps' ranges point at, by section,
	// and a section's at A before its at B where the two are apart, up to
	// MaxFileLines of them.
	Excerpts []Excerpt
	// Cut is what MaxFileLines leaves out, or nil.
	Cut *Cut
}

// A Cut is what MaxFileLines leaves out of a file's input lines: how many
// lines its ranges hold past them, of how many sections, and the diffs of
// how many of those sections that changed.
type Cut struct {
	Lines, Sections, Diffs int
}

// A Map is what one revision's reverse map of a changed file states.
type Map struct {
	At string // the revision's name
	// All records that the file depends on all of its cell's inputs, and
	// Ranges that it comes from some ranges: a file with neither comes from
	// no input lines.
	All, Ranges bool
	// Note says why the map may not be what the cell's run gives, or is "".
	Note string
}

// An Excerpt is lines of one archive section that a changed file's
// ranges point at: at one revision, or at both where both shown maps name
// the section, its bytes are the same at each, and so is its note.
type Excerpt struct {
	Section string
	At      []string // the names of the revisions whose ranges point here
	// Lines are the lines the ranges hold, in order, each numbered as the
	// section numbers it at its revision, with a line that stands for those
	// left out between two lines apart.
	Lines []ExcerptLine
	// Diff, Added, Removed and Unaligned are the section's line diff from A
	// to B, as a changed file's are, where both revisions hold the section
	// with other bytes. Only the section's last excerpt has it, and only
	// the first file of a change that shows the section.
	Diff           []diff.Line
	Added, Removed int
	Unaligned      bool
	// DiffAt is the path of the file of the change that shows the
	// section's diff, where an earlier file does, or "".
	DiffAt string
	// Note says why the lines may not be the ones the cell read, or is "".
	Note string
}

// An ExcerptLine is one line of an excerpt.
type ExcerptLine struct {
	// N is the line's number, counted from 1, or 0 for a line that stands
	// for those left out between two others, or after the last.
	N int
	// Cut, in a line that stands for others, is how many of them a range
	// holds past its first MaxRangeLines.
	Cut int
	// Text is the line as a diff shows it, and NoNewline records that the
	// line is its section's last, and has no final newline.
	Text      string
	NoNewline bool
	// Mark says whose ranges hold the line in an excerpt at both
	// revisions: "-" for A's alone, "+" for B's alone, and " " for both.
	// It is "" in an excerpt at one revision.
	Mark string
}

// inputsOf returns the input lines a changed file of a cell comes from, or
// nil where the page shows none: where no revision it shows maps the file,
// or each range it maps names lines the archive doesn't hold, which the
// problems list. at says whether the file is at A and at B, and cells are
// the cell at each, nil where it isn't.
func inputsOf(snaps [2]*snapshot, cells [2]*cellAt, k cellKey, fc FileChange, at [2]bool) *Inputs {
	shown := [2]bool{!at[1] || len(fc.Ranges) > 0, at[1]}
	in := &Inputs{}
	var ranges [2]map[string][]revmap.Range // by section
	for i := range 2 {
		var m *revmap.File
		if shown[i] {
			m = cells[i].mapOf(fc.Path)
		}
		if m == nil {
			continue
		}
		in.Maps[i] = &Map{At: snaps[i].rev.Name, All: m.All, Ranges: len(m.Ranges) > 0, Note: snaps[i].kept(k, cells[i])}
		ranges[i] = map[string][]revmap.Range{}
		for _, r := range m.Ranges {
			ranges[i][r.Section] = append(ranges[i][r.Section], r)
		}
	}
	if in.Maps == [2]*Map{} {
		return nil
	}
	ak := archiveKey{corpus: k.c.corpus, archive: k.c.archive}
	sections := map[string]bool{}
	for i := range 2 {
		for sec := range ranges[i] {
			sections[sec] = true
		}
	}
	for _, sec := range slices.Sorted(maps.Keys(sections)) {
		var data [2][]byte
		var has [2]bool
		var notes [2]string
		for i := range 2 {
			data[i], has[i] = snaps[i].sections[ak][sec]
			if ranges[i][sec] != nil {
				notes[i] = snaps[i].note(k.c, sec)
			}
		}
		start := len(in.Excerpts)
		add := func(e Excerpt) {
			if len(e.Lines) > 0 {
				in.Excerpts = append(in.Excerpts, e)
			}
		}
		if ranges[0][sec] != nil && ranges[1][sec] != nil && has[0] && has[1] && bytes.Equal(data[0], data[1]) && notes[0] == notes[1] {
			add(Excerpt{
				Section: sec,
				At:      []string{snaps[0].rev.Name, snaps[1].rev.Name},
				Lines:   excerptLines(data[0], [2][]revmap.Range{ranges[0][sec], ranges[1][sec]}, true),
				Note:    notes[0],
			})
		} else {
			for i := range 2 {
				if ranges[i][sec] == nil || !has[i] {
					continue
				}
				var held [2][]revmap.Range
				held[i] = ranges[i][sec]
				add(Excerpt{Section: sec, At: []string{snaps[i].rev.Name}, Lines: excerptLines(data[i], held, false), Note: notes[i]})
			}
		}
		if len(in.Excerpts) > start && has[0] && has[1] && !bytes.Equal(data[0], data[1]) {
			e := &in.Excerpts[len(in.Excerpts)-1]
			e.Diff, e.Added, e.Removed, e.Unaligned = lineDiff(data[0], data[1])
		}
	}
	if len(in.Excerpts) == 0 && !slices.ContainsFunc(in.Maps[:], func(m *Map) bool { return m != nil && (!m.Ranges || m.Note != "") }) {
		return nil
	}
	in.capLines()
	return in
}

// capLines cuts the excerpts after their first MaxFileLines lines, and
// records what that leaves out.
func (in *Inputs) capLines() {
	shown := 0
	for i := range in.Excerpts {
		e := &in.Excerpts[i]
		for j, l := range e.Lines {
			if l.N == 0 {
				continue
			}
			if shown < MaxFileLines {
				shown++
				continue
			}
			c := &Cut{}
			sections := map[string]bool{}
			leave := func(e Excerpt, lines []ExcerptLine) {
				for _, l := range lines {
					if l.N > 0 {
						c.Lines++
					}
					c.Lines += l.Cut
				}
				sections[e.Section] = true
			}
			leave(*e, e.Lines[j:])
			e.Lines = e.Lines[:j]
			if n := len(e.Lines); n > 0 && e.Lines[n-1].N == 0 {
				c.Lines += e.Lines[n-1].Cut
				e.Lines = e.Lines[:n-1]
			}
			kept := i + 1
			if len(e.Lines) == 0 { // the excerpt's first line is past the cap
				kept = i
				if e.Diff != nil {
					c.Diffs++
				}
			}
			for _, d := range in.Excerpts[i+1:] {
				leave(d, d.Lines)
				if d.Diff != nil {
					c.Diffs++
				}
			}
			in.Excerpts = in.Excerpts[:kept]
			c.Sections = len(sections)
			in.Cut = c
			return
		}
	}
}

// onceDiffs leaves each section's diff on the first file of a change
// whose input lines show it, and points the others' excerpts there.
func onceDiffs(files []FileChange) {
	at := map[string]string{} // section → the path of the file that shows its diff
	for _, f := range files {
		if f.Inputs == nil {
			continue
		}
		for i := range f.Inputs.Excerpts {
			e := &f.Inputs.Excerpts[i]
			if e.Diff == nil {
				continue
			}
			if p, ok := at[e.Section]; ok {
				e.Diff, e.Added, e.Removed, e.Unaligned, e.DiffAt = nil, 0, 0, false, p
				continue
			}
			at[e.Section] = f.Path
		}
	}
}

// excerptLines returns the lines of a section that ranges hold, A's and
// B's, numbered, each range's first MaxRangeLines of them, with a line
// that stands for those left out between two lines apart, or after the
// last, and each marked by whose ranges hold it when marks says to. A
// range past the section's last line holds none.
func excerptLines(data []byte, ranges [2][]revmap.Range, marks bool) []ExcerptLine {
	lines := sectionLines(data)
	var held [2][]bool
	shown := make([]bool, len(lines)+1)
	for i := range 2 {
		held[i] = make([]bool, len(lines)+1)
		for _, r := range ranges[i] {
			first, last := r.Start, r.End
			if first == 0 && last == 0 {
				first, last = 1, len(lines)
			}
			if last > len(lines) {
				continue
			}
			for n := first; n <= last; n++ {
				held[i][n] = true
				shown[n] = shown[n] || n < first+MaxRangeLines
			}
		}
	}
	var out []ExcerptLine
	prev, cut := 0, 0
	for n := 1; n <= len(lines); n++ {
		a, b := held[0][n], held[1][n]
		if !shown[n] {
			if a || b {
				cut++
			}
			continue
		}
		if prev > 0 && n > prev+1 {
			out = append(out, ExcerptLine{Cut: cut})
		}
		prev, cut = n, 0
		l := ExcerptLine{N: n, Text: diff.Display(lines[n-1]), NoNewline: !strings.HasSuffix(lines[n-1], "\n")}
		if marks {
			l.Mark = map[[2]bool]string{{true, false}: "-", {false, true}: "+", {true, true}: " "}[[2]bool{a, b}]
		}
		out = append(out, l)
	}
	if cut > 0 {
		out = append(out, ExcerptLine{Cut: cut})
	}
	return out
}

// note says why lines of a section that a cell's ranges point at may not
// be the ones the cell read, where the report can tell from the
// revision's files, or is "": past a frozen bytes lift, whose product
// stays as committed while the cells past it run on what the lift makes
// now; and in the own inputs of a transform's variant that no registry
// owns, which no update writes. c is the cell's case.
func (s *snapshot) note(c caseKey, section string) string {
	kind, name, rest := archive.SplitCase(section)
	top, below, _ := strings.Cut(rest, "/")
	switch {
	case top == archive.Via:
		lift, _, _ := strings.Cut(below, "/")
		k := caseKey{corpus: c.corpus, archive: c.archive, kind: base}
		switch kind {
		case archive.Variant:
			k.kind, k.name = variant, name
		case archive.Transform:
			k.kind, k.name = transform, name
		}
		ck := cellKey{c: k, column: lift}
		if _, marked := s.gaps[ck]; marked && s.cells[ck] != nil {
			return fmt.Sprintf("The cell of %s that makes these lines is frozen, so they stay as committed while the cells past it run on what %s makes now, and they may not be the lines this cell read.", lift, lift)
		}
	case top == archive.In && kind == archive.Transform && !s.owned[c.corpus][name]:
		return fmt.Sprintf("No registry owns the transform %s, so no update writes its variant's own inputs, and these may not be the lines the cell read.", name)
	}
	return ""
}

// kept says why a cell's committed inputs file may not be the map its run
// gives, where its column's declarations tell, or is "": a file of the
// cell, its product's among them, is of a declaration that states no
// source of ranges, or that takes its ranges from Positions where the
// project doesn't supply Positions. Such a declaration fails the run, and
// the update leaves the cell's inputs file as it is. A file that more than
// one declaration matches can't be judged, so no declaration decides it.
func (s *snapshot) kept(k cellKey, c *cellAt) string {
	col, ok := s.column(k.c.corpus, k.column)
	if !ok {
		return ""
	}
	files := slices.Sorted(maps.Keys(c.files))
	for _, f := range slices.Sorted(maps.Keys(c.product)) {
		files = append(files, archive.Via+"/"+k.column+"/"+f)
	}
	for _, f := range files {
		var decls []registry.Output
		for _, o := range col.outputs {
			if ok, _ := path.Match(o.Pattern, f); ok {
				decls = append(decls, o)
			}
		}
		if len(decls) != 1 {
			continue
		}
		var why string
		switch {
		case decls[0].Inputs == registry.InputsNone:
			why = fmt.Sprintf("%s's declaration states no source of ranges", f)
		case decls[0].Inputs == registry.InputsPositions && !col.positions:
			why = fmt.Sprintf("%s takes its ranges from Positions, which the project doesn't supply", f)
		default:
			continue
		}
		return why + ", so no update writes the cell's inputs file, and its ranges may not be what the cell read."
	}
	return ""
}

// badRange says what keeps a range from naming lines of an archive's
// sections, or is "": a section the archive doesn't hold, or lines past
// the section's last.
func badRange(r revmap.Range, sections map[string][]byte) string {
	data, ok := sections[r.Section]
	switch {
	case !ok:
		return "the archive holds no section " + r.Section
	case r.End > lineCount(data):
		return fmt.Sprintf("%s has %s", r.Section, lines(lineCount(data)))
	}
	return ""
}

// sectionLines returns a section's lines, each with its newline, but the
// last where the section doesn't end with one.
func sectionLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := strings.SplitAfter(string(data), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// lineCount returns how many lines a section has, as sectionLines counts
// them.
func lineCount(data []byte) int {
	n := bytes.Count(data, []byte("\n"))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++
	}
	return n
}

// lineDiff returns the line diff from a to b, how many of its lines are
// only at B and only at A, and whether it left out a stretch it can't
// align, whose lines those don't count.
func lineDiff(a, b []byte) (lines []diff.Line, added, removed int, unaligned bool) {
	lines, _ = diff.Diff(a, b)
	for _, l := range lines {
		switch l.Kind {
		case '+':
			added++
		case '-':
			removed++
		case diff.Unaligned:
			unaligned = true
		}
	}
	return lines, added, removed, unaligned
}
