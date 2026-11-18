package report

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/diff"
	"github.com/sdboyer/hypercase/internal/revmap"
)

// Inputs are the input lines a changed file comes from, as its
// revisions' reverse maps give them: B's map of a file at B, A's of a file
// only at A, and both where both revisions map the file, differently.
type Inputs struct {
	// Maps are what the shown maps state, A's then B's, each nil where the
	// page shows none of that revision's.
	Maps [2]*Map
	// Excerpts are the lines the shown maps' ranges point at, by section,
	// and a section's at A before its at B where the two are apart.
	Excerpts []Excerpt
}

// A Map is what one revision's reverse map of a changed file states.
type Map struct {
	At string // the revision's name
	// All records that the file depends on all of its cell's inputs, and
	// Ranges that it comes from some ranges: a file with neither comes from
	// no input lines.
	All, Ranges bool
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
	// with other bytes. Only the section's last excerpt has it.
	Diff           []diff.Line
	Added, Removed int
	Unaligned      bool
	// Note says why the lines may not be the ones the cell read, or is "".
	Note string
}

// An ExcerptLine is one line of an excerpt.
type ExcerptLine struct {
	// N is the line's number, counted from 1, or 0 for a line that stands
	// for those left out between two others.
	N int
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
		in.Maps[i] = &Map{At: snaps[i].rev.Name, All: m.All, Ranges: len(m.Ranges) > 0}
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
	if len(in.Excerpts) == 0 && !slices.ContainsFunc(in.Maps[:], func(m *Map) bool { return m != nil && !m.Ranges }) {
		return nil
	}
	return in
}

// excerptLines returns the lines of a section that ranges hold, A's and
// B's, numbered, with a line that stands for those left out between two
// lines apart, and each marked by whose ranges hold it when marks says to.
// A range past the section's last line holds none.
func excerptLines(data []byte, ranges [2][]revmap.Range, marks bool) []ExcerptLine {
	lines := sectionLines(data)
	var held [2][]bool
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
			}
		}
	}
	var out []ExcerptLine
	prev := 0
	for n := 1; n <= len(lines); n++ {
		a, b := held[0][n], held[1][n]
		if !a && !b {
			continue
		}
		if prev > 0 && n > prev+1 {
			out = append(out, ExcerptLine{})
		}
		prev = n
		l := ExcerptLine{N: n, Text: diff.Display(lines[n-1]), NoNewline: !strings.HasSuffix(lines[n-1], "\n")}
		if marks {
			l.Mark = map[[2]bool]string{{true, false}: "-", {false, true}: "+", {true, true}: " "}[[2]bool{a, b}]
		}
		out = append(out, l)
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
