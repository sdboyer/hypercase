// Package revmap reads and writes a cell's inputs file, out/<column>/inputs:
// its reverse maps, which give, for each of the cell's output files, the
// archive sections and lines it comes from, or that it depends on all of
// the cell's inputs. The framework writes it with Format, and a reader, the
// report among them, reads it with Parse. It doesn't import testing.
//
// The file holds, for each output file, in path order, bytewise, a line
// holding the file's path, then one line per range, indented two spaces:
// the range's section alone for the whole section, as in/sheet/A1, or the
// section and its lines, as in/sheet/A1:1-3. A file that depends on all
// of the cell's inputs has the one line "  all inputs" in their place, and
// a file with no ranges is its path line alone. A path or a section that
// isn't plain is written as a Go string literal, "like this", so that no
// name an archive holds reads back otherwise: a name is plain when it is
// printable ASCII, holds no colon, double quote or backslash, neither
// starts nor ends with a space, doesn't start with a hyphen, which a
// section's marker line starts with, and isn't "all inputs".
package revmap

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// A File is the reverse map of one output file.
type File struct {
	// Path names the output file: its path under its column's directory,
	// or, for a bytes lift's product file, its section below the case's
	// prefix, via/<lift>/<path>.
	Path string
	// All records that the file depends on all of the cell's inputs, as
	// its declaration's AllInputs states. Such a file has no Ranges.
	All bool
	// Ranges are the lines it comes from.
	Ranges []Range
}

// A Range is lines of one archive section.
type Range struct {
	// Section is the section's whole name, as in/sheet/A1.
	Section string
	// Start and End are the first and last lines, counted from 1, End
	// included. Start and End both 0 is the whole section.
	Start, End int
}

// whole reports whether the range is its whole section.
func (r Range) whole() bool { return r.Start == 0 && r.End == 0 }

// allLine is the range line of a file that depends on all inputs.
const allLine = "all inputs"

// Canonical returns the reverse maps as Format writes them: the files by
// path, bytewise, and each file's ranges sorted by section, then by first
// line, then by last, with repeats dropped, and the line ranges of one
// section that overlap or touch merged into one. A whole-section range
// absorbs that section's line ranges, and a range of every line of a
// section stays a line range: Canonical doesn't read the sections. A file
// that depends on all inputs keeps no range.
//
// It panics on two maps of one file, an empty path or section, and a
// range that is neither a whole section nor lines from 1 with its last at
// or after its first: the framework writes none, and checks every range
// before it maps it.
func Canonical(files []File) []File {
	out := make([]File, 0, len(files))
	for _, f := range files {
		if f.Path == "" {
			panic("revmap: a reverse map of a file with no path")
		}
		c := File{Path: f.Path, All: f.All}
		if !f.All {
			c.Ranges = canonicalRanges(f.Ranges)
		}
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	for i := 1; i < len(out); i++ {
		if out[i].Path == out[i-1].Path {
			panic(fmt.Sprintf("revmap: two reverse maps of %q", out[i].Path))
		}
	}
	return out
}

func canonicalRanges(rs []Range) []Range {
	for _, r := range rs {
		switch {
		case r.Section == "":
			panic("revmap: a range of no section")
		case !r.whole() && (r.Start < 1 || r.End < r.Start):
			panic(fmt.Sprintf("revmap: the range %d-%d of %q", r.Start, r.End, r.Section))
		}
	}
	sorted := slices.Clone(rs)
	slices.SortFunc(sorted, func(a, b Range) int {
		return cmp.Or(strings.Compare(a.Section, b.Section), cmp.Compare(a.Start, b.Start), cmp.Compare(a.End, b.End))
	})
	var out []Range
	for i := 0; i < len(sorted); {
		// One section's ranges: its whole, if it has one, absorbs the rest.
		j := i
		for j < len(sorted) && sorted[j].Section == sorted[i].Section {
			j++
		}
		section := sorted[i:j]
		if section[0].whole() {
			out = append(out, section[0])
			i = j
			continue
		}
		cur := section[0]
		for _, r := range section[1:] {
			if r.Start <= cur.End+1 {
				cur.End = max(cur.End, r.End)
				continue
			}
			out = append(out, cur)
			cur = r
		}
		out = append(out, cur)
		i = j
	}
	return out
}

// Format returns the bytes of the inputs file that holds the reverse maps:
// those of Canonical, every line ending with a newline. No maps give an
// empty file.
func Format(files []File) []byte {
	var b bytes.Buffer
	for _, f := range Canonical(files) {
		b.WriteString(name(f.Path))
		b.WriteByte('\n')
		if f.All {
			b.WriteString("  " + allLine + "\n")
			continue
		}
		for _, r := range f.Ranges {
			b.WriteString("  " + name(r.Section))
			if !r.whole() {
				fmt.Fprintf(&b, ":%d-%d", r.Start, r.End)
			}
			b.WriteByte('\n')
		}
	}
	return b.Bytes()
}

// name is a path or a section as the file writes it: as it is when it is
// plain, and quoted otherwise.
func name(s string) string {
	if plain(s) {
		return s
	}
	return strconv.Quote(s)
}

// plain reports whether a path or a section is written as it is.
func plain(s string) bool {
	if s == "" || s == allLine || s[0] == ' ' || s[0] == '-' || s[len(s)-1] == ' ' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < ' ' || c > '~' || c == '"' || c == '\\' || c == ':' {
			return false
		}
	}
	return true
}

// Parse reads an inputs file. It refuses any bytes Format doesn't write,
// so a file it reads is canonical: no line out of order, no range two
// ranges would merge into, and no name quoted that needn't be.
func Parse(data []byte) ([]File, error) {
	if len(data) > 0 && data[len(data)-1] != '\n' {
		return nil, errors.New("it has no final newline")
	}
	var files []File
	text := string(data)
	for n := 1; text != ""; n++ {
		line, rest, _ := strings.Cut(text, "\n")
		text = rest
		if r, ok := strings.CutPrefix(line, "  "); ok {
			if len(files) == 0 {
				return nil, fmt.Errorf("line %d is a range, and no file's path comes before it", n)
			}
			f := &files[len(files)-1]
			if r == allLine {
				f.All = true
				continue
			}
			rg, err := parseRange(r)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", n, err)
			}
			f.Ranges = append(f.Ranges, rg)
			continue
		}
		p, rest, err := parseName(line)
		switch {
		case err != nil:
			return nil, fmt.Errorf("line %d: %v", n, err)
		case rest != "":
			return nil, fmt.Errorf("line %d: the path is followed by %q", n, rest)
		}
		files = append(files, File{Path: p})
	}
	if err := canonical(files); err != nil {
		return nil, err
	}
	if got := Format(files); !bytes.Equal(got, data) {
		return nil, errors.New("it isn't as Format writes it: its files, or a file's ranges, are out of order, or two of a file's ranges are one")
	}
	return Canonical(files), nil
}

// canonical reports what keeps Canonical from taking the maps a file
// holds, which it would panic on: two maps of one file, and a file both
// with ranges and depending on all inputs.
func canonical(files []File) error {
	seen := map[string]bool{}
	for _, f := range files {
		if seen[f.Path] {
			return fmt.Errorf("it maps %q twice", f.Path)
		}
		seen[f.Path] = true
		if f.All && len(f.Ranges) > 0 {
			return fmt.Errorf("%q depends on all inputs, and has ranges", f.Path)
		}
	}
	return nil
}

// parseRange reads a range line, less its indent.
func parseRange(s string) (Range, error) {
	section, rest, err := parseName(s)
	if err != nil {
		return Range{}, err
	}
	r := Range{Section: section}
	if rest == "" {
		return r, nil
	}
	lines, ok := strings.CutPrefix(rest, ":")
	first, last, dash := strings.Cut(lines, "-")
	if !ok || !dash {
		return Range{}, fmt.Errorf("the section is followed by %q, not :<first>-<last>", rest)
	}
	if r.Start, err = number(first); err == nil {
		r.End, err = number(last)
	}
	switch {
	case err != nil:
		return Range{}, err
	case r.End < r.Start:
		return Range{}, fmt.Errorf("its last line, %d, is before its first, %d", r.End, r.Start)
	}
	return r, nil
}

// number reads a line number: decimal, from 1, with no sign and no
// leading zero.
func number(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || strconv.Itoa(n) != s {
		return 0, fmt.Errorf("%q is no line number", s)
	}
	return n, nil
}

// parseName reads a name at the start of s, quoted or plain, and returns
// what follows it. A plain name runs to the first colon.
func parseName(s string) (string, string, error) {
	if strings.HasPrefix(s, `"`) {
		q, err := strconv.QuotedPrefix(s)
		if err != nil {
			return "", "", fmt.Errorf("%q opens a quote it doesn't close", s)
		}
		n, _ := strconv.Unquote(q)
		return n, s[len(q):], nil
	}
	n, rest, _ := strings.Cut(s, ":")
	if rest != "" || strings.HasSuffix(s, ":") {
		rest = ":" + rest
	}
	if !plain(n) {
		return "", "", fmt.Errorf("%q is neither a plain name nor a quoted one", n)
	}
	return n, rest, nil
}
