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
	return ParsePropertiesIn(Properties, data)
}

// ParsePropertiesIn parses the properties section of the given name: the
// base's, properties, or a hand-written variant's,
// variant/<name>/properties. Its problems name the section.
func ParsePropertiesIn(section string, data []byte) ([]string, []Problem) {
	var names []string
	var ps []Problem
	seen := map[string]int{}
	ls := lines(data)
	if len(ls) == 0 {
		return nil, []Problem{{Section: section, Msg: "the section is empty; a case with no properties has no properties section"}}
	}
	for i, l := range ls {
		n := i + 1
		switch {
		case l == "":
			ps = append(ps, Problem{Section: section, Line: n, Msg: "the line is blank"})
		case !ValidName(l):
			ps = append(ps, Problem{Section: section, Line: n, Msg: fmt.Sprintf("%q is not one property's name, which is %s", l, NameRule)})
		case seen[l] > 0:
			ps = append(ps, Problem{Section: section, Line: n, Msg: fmt.Sprintf("%s is stated twice, on lines %d and %d", l, seen[l], n)})
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
	// Transform names the transform whose variant holds the cell, for a
	// line of the base's gaps that starts transform/<name>. It is "" for a
	// line that marks the case's own cell.
	Transform string
	Column    string // the column whose cell the line marks
	Text      string // what is right, and the issue it waits on
	Line      int    // the line's number in the section
	Raw       string // the line as written
}

// Cell names the cell the line marks, in a message: the column, after
// transform/<name> for a cell of a transform's variant.
func (g Gap) Cell() string {
	if g.Transform != "" {
		return Transform + "/" + g.Transform + " " + g.Column
	}
	return g.Column
}

// ParseGaps parses the base's gaps section. Each line is a column's name,
// one space, then the text, to the end of the line; a line that marks a
// cell of a transform's variant starts with transform/<name> and one
// space. Text that is empty or starts with a space, a blank line, two lines
// for one cell and an empty section are each a problem of form.
func ParseGaps(data []byte) ([]Gap, []Problem) {
	return ParseGapsIn(Gaps, data)
}

// ParseGapsIn parses the gaps section of the given name: the base's, gaps,
// or a hand-written variant's, variant/<name>/gaps. Only the base's holds
// lines for transforms' variants: a hand-written variant's marks its own
// cells. Its problems name the section.
func ParseGapsIn(section string, data []byte) ([]Gap, []Problem) {
	kind, _, _ := SplitCase(section)
	transforms := kind == ""
	var gaps []Gap
	var ps []Problem
	seen := map[string]int{}
	ls := lines(data)
	if len(ls) == 0 {
		return nil, []Problem{{Section: section, Msg: "the section is empty; a case with no known gaps has no gaps section"}}
	}
	for i, l := range ls {
		n := i + 1
		head, text, _ := strings.Cut(l, " ")
		tr, isTransform := strings.CutPrefix(head, Transform+"/")
		col := head
		if isTransform {
			col, text, _ = strings.Cut(text, " ")
		} else {
			tr = ""
		}
		g := Gap{Transform: tr, Column: col, Text: text, Line: n, Raw: l}
		switch {
		case l == "":
			ps = append(ps, Problem{Section: section, Line: n, Msg: "the line is blank"})
		case isTransform && !transforms:
			ps = append(ps, Problem{Section: section, Line: n, Msg: fmt.Sprintf("the line starts with %s, and a hand-written variant's gaps mark its own cells. A known gap in a transform's variant is a line of the base's gaps", head)})
		case isTransform && !ValidName(tr):
			ps = append(ps, Problem{Section: section, Line: n, Msg: fmt.Sprintf("the line starts with %q, which is not transform/<name> for a transform's name", head)})
		case !ValidName(col):
			ps = append(ps, Problem{Section: section, Line: n, Msg: fmt.Sprintf("the line %s %q, which is not a column's name", gapLead(isTransform, head), col)})
		case text == "":
			ps = append(ps, Problem{Section: section, Line: n, Msg: fmt.Sprintf("the line names %s and has no text: say what is right, and the issue it waits on", g.Cell())})
		case text[0] == ' ':
			ps = append(ps, Problem{Section: section, Line: n, Msg: "the column's name is followed by more than one space"})
		case seen[g.Cell()] > 0:
			ps = append(ps, Problem{Section: section, Line: n, Msg: fmt.Sprintf("lines %d and %d both mark the cell of %s", seen[g.Cell()], n, g.Cell())})
		default:
			seen[g.Cell()] = n
			gaps = append(gaps, g)
		}
	}
	return gaps, ps
}

func gapLead(isTransform bool, head string) string {
	if isTransform {
		return "names " + head + ", then"
	}
	return "starts with"
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
