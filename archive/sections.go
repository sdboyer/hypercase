package archive

import (
	"fmt"
	"regexp"
	"strings"
)

// nameRE is hypercase's naming rule: lowercase ASCII letters and digits, in
// words joined by single hyphens, starting with a letter.
var nameRE = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// ValidName reports whether s follows hypercase's naming rule, which covers
// archives, columns and properties.
func ValidName(s string) bool { return nameRE.MatchString(s) }

// NameRule says the naming rule in words.
const NameRule = "lowercase ASCII letters and digits, in words joined by single hyphens, starting with a letter"

// lines splits a section into its lines, less their newlines.
func lines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// ParseProperties parses a properties section: each line is exactly one
// property's name. A blank line, a name stated twice and an empty section
// are each a problem of form.
func ParseProperties(data []byte) ([]string, []Problem) {
	var names []string
	var ps []Problem
	seen := map[string]int{}
	ls := lines(data)
	if len(ls) == 0 {
		return nil, []Problem{{Section: Properties, Msg: "the section is empty; a case with no properties has no properties section"}}
	}
	for i, l := range ls {
		n := i + 1
		switch {
		case l == "":
			ps = append(ps, Problem{Section: Properties, Line: n, Msg: "the line is blank"})
		case !ValidName(l):
			ps = append(ps, Problem{Section: Properties, Line: n, Msg: fmt.Sprintf("%q is not one property's name, which is %s", l, NameRule)})
		case seen[l] > 0:
			ps = append(ps, Problem{Section: Properties, Line: n, Msg: fmt.Sprintf("%s is stated twice, on lines %d and %d", l, seen[l], n)})
		default:
			seen[l] = n
			names = append(names, l)
		}
	}
	return names, ps
}

// A Gap is one line of a gaps section: a known gap, which marks one cell as
// known to be wrong, and freezes it.
type Gap struct {
	Column string // the column whose cell the line marks
	Text   string // what is right, and the issue it waits on
	Line   int    // the line's number in the section
	Raw    string // the line as written
}

// ParseGaps parses a gaps section: each line is a column's name, one space,
// then the text, to the end of the line. Text that is empty or starts with a
// space, a blank line, two lines for one cell and an empty section are each
// a problem of form.
func ParseGaps(data []byte) ([]Gap, []Problem) {
	var gaps []Gap
	var ps []Problem
	seen := map[string]int{}
	ls := lines(data)
	if len(ls) == 0 {
		return nil, []Problem{{Section: Gaps, Msg: "the section is empty; a case with no known gaps has no gaps section"}}
	}
	for i, l := range ls {
		n := i + 1
		col, text, _ := strings.Cut(l, " ")
		switch {
		case l == "":
			ps = append(ps, Problem{Section: Gaps, Line: n, Msg: "the line is blank"})
		case !ValidName(col):
			ps = append(ps, Problem{Section: Gaps, Line: n, Msg: fmt.Sprintf("the line starts with %q, which is not a column's name", col)})
		case text == "":
			ps = append(ps, Problem{Section: Gaps, Line: n, Msg: fmt.Sprintf("the line names %s and has no text: say what is right, and the issue it waits on", col)})
		case text[0] == ' ':
			ps = append(ps, Problem{Section: Gaps, Line: n, Msg: "the column's name is followed by more than one space"})
		case seen[col] > 0:
			ps = append(ps, Problem{Section: Gaps, Line: n, Msg: fmt.Sprintf("lines %d and %d both mark the cell of %s", seen[col], n, col)})
		default:
			seen[col] = n
			gaps = append(gaps, Gap{Column: col, Text: text, Line: n, Raw: l})
		}
	}
	return gaps, ps
}

// Properties returns the properties the case states, in the order written.
// It is meant for an archive [Archive.Check] passes.
func (a *Archive) Properties() []string {
	data, ok := a.Section(Properties)
	if !ok {
		return nil
	}
	names, _ := ParseProperties(data)
	return names
}

// Gaps returns the case's known gaps, in the order written. It is meant
// for an archive [Archive.Check] passes.
func (a *Archive) Gaps() []Gap {
	data, ok := a.Section(Gaps)
	if !ok {
		return nil
	}
	gaps, _ := ParseGaps(data)
	return gaps
}
