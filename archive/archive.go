// Package archive reads and writes hypercase's archives. An archive is one
// txtar file holding one case: its description, its inputs and its goldens.
//
// The package doesn't import testing, so commands and a project's own code
// can read and write archives the way a test does.
package archive

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/txtar"
)

// An Archive is a parsed archive: its comment, which describes the case, and
// its sections in the order the file holds them.
type Archive struct {
	Comment  []byte
	Sections []Section
}

// A Section is one file of an archive.
type Section struct {
	Name string
	Data []byte
}

// Parse parses an archive. txtar has no syntax errors, so Parse always
// succeeds; [Archive.Check] reports problems of the archive's form.
func Parse(data []byte) *Archive {
	ta := txtar.Parse(data)
	a := &Archive{Comment: ta.Comment}
	for _, f := range ta.Files {
		a.Sections = append(a.Sections, Section{Name: f.Name, Data: f.Data})
	}
	return a
}

// Format returns the archive's bytes.
func (a *Archive) Format() []byte {
	ta := &txtar.Archive{Comment: a.Comment}
	for _, s := range a.Sections {
		ta.Files = append(ta.Files, txtar.File{Name: s.Name, Data: s.Data})
	}
	return txtar.Format(ta)
}

// Section returns the named section's data, and whether the archive has it.
func (a *Archive) Section(name string) ([]byte, bool) {
	for _, s := range a.Sections {
		if s.Name == name {
			return s.Data, true
		}
	}
	return nil, false
}

// Clone returns a copy of the archive that shares no slices with it.
func (a *Archive) Clone() *Archive {
	c := &Archive{Comment: bytes.Clone(a.Comment)}
	for _, s := range a.Sections {
		c.Sections = append(c.Sections, Section{Name: s.Name, Data: bytes.Clone(s.Data)})
	}
	return c
}

// The framework's top-level names. The top level of an archive is a closed
// set: any other name is a problem of the archive's form.
const (
	In         = "in"
	Out        = "out"
	Via        = "via"
	Properties = "properties"
	Gaps       = "gaps"
	Variant    = "variant"
	Transform  = "transform"
)

// topLevel is the closed set, each with whether this hypercase reads it yet.
// A section under a name it doesn't read yet is a problem of form, so it
// can't pass unread while an update rewrites the goldens beside it.
var topLevel = map[string]bool{
	In:         true,
	Out:        true,
	Via:        false,
	Properties: false,
	Gaps:       false,
	Variant:    false,
	Transform:  false,
}

// HandWritten reports whether a section of the given name is written by a
// person and never touched by an update. The answer depends on the name
// alone.
func HandWritten(name string) bool {
	top, _, _ := strings.Cut(name, "/")
	switch top {
	case Out, Via:
		return false
	}
	return true
}

// A Problem is a problem of an archive's form, or of its description.
type Problem struct {
	Section string // the section the problem is in, or "" for the comment or the archive as a whole
	Line    int    // the 1-based line within the section, or 0
	Msg     string
}

func (p Problem) String() string {
	switch {
	case p.Section == "":
		return p.Msg
	case p.Line > 0:
		return fmt.Sprintf("%s, line %d: %s", p.Section, p.Line, p.Msg)
	}
	return fmt.Sprintf("%s: %s", p.Section, p.Msg)
}

// Check reports the problems of the archive's form, in section order. An
// archive with a problem of form runs no cells, and no update writes it.
func (a *Archive) Check() []Problem {
	var ps []Problem
	seen := map[string]bool{}
	var names []string
	for _, s := range a.Sections {
		if seen[s.Name] {
			ps = append(ps, Problem{Section: s.Name, Msg: "the section appears twice"})
			continue
		}
		seen[s.Name] = true
		if !fs.ValidPath(s.Name) || s.Name == "." {
			ps = append(ps, Problem{Section: s.Name, Msg: "the name is not a valid slash path"})
			continue
		}
		names = append(names, s.Name)
		top, rest, _ := strings.Cut(s.Name, "/")
		read, ok := topLevel[top]
		switch {
		case !ok:
			ps = append(ps, Problem{Section: s.Name, Msg: fmt.Sprintf("%s is not one of the framework's top-level names (%s)", top, topLevelList())})
			continue
		case !read:
			ps = append(ps, Problem{Section: s.Name, Msg: fmt.Sprintf("the top-level name %s is reserved for a later step, and this hypercase doesn't read it yet", top)})
			continue
		}
		switch top {
		case In:
			if _, file, _ := strings.Cut(rest, "/"); file == "" {
				ps = append(ps, Problem{Section: s.Name, Msg: "an input belongs in a tree, as in/<tree>/<file>"})
			}
		case Out:
			col, file, _ := strings.Cut(rest, "/")
			if col == "" || file == "" {
				ps = append(ps, Problem{Section: s.Name, Msg: "an output belongs under its column, as out/<column>/<file>"})
			}
		}
	}
	ps = append(ps, conflicts(names)...)
	return ps
}

func topLevelList() string {
	names := make([]string, 0, len(topLevel))
	for n := range topLevel {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// conflicts reports a section that is also a directory of another, which no
// file tree can hold.
func conflicts(names []string) []Problem {
	set := map[string]bool{}
	for _, n := range names {
		set[n] = true
	}
	var ps []Problem
	for _, n := range names {
		for d := path.Dir(n); d != "."; d = path.Dir(d) {
			if set[d] {
				ps = append(ps, Problem{Section: d, Msg: fmt.Sprintf("the section is also the directory of %s", n)})
				break
			}
		}
	}
	return ps
}

// Description checks the archive's comment, which describes its case. It is
// required, and holds prose only.
func (a *Archive) Description() []Problem {
	return CheckDescription(a.Comment)
}

// CheckDescription checks a description written in an archive: it is
// required, and holds no key: value lines.
func CheckDescription(text []byte) []Problem {
	if len(bytes.TrimSpace(text)) == 0 {
		return []Problem{{Msg: "the archive has no description: its comment, the text before the first section, is empty"}}
	}
	var ps []Problem
	for i, line := range strings.Split(strings.TrimSuffix(string(text), "\n"), "\n") {
		if keyValue(line) {
			ps = append(ps, Problem{Msg: fmt.Sprintf("line %d of the comment is a key: value line, and the comment is prose: %q", i+1, line)})
		}
	}
	return ps
}

// keyValue reports whether a line reads as a key: value line, a lowercase
// key and a colon at its start.
func keyValue(line string) bool {
	key, _, ok := strings.Cut(line, ":")
	if !ok || key == "" {
		return false
	}
	if rest := line[len(key)+1:]; rest != "" && rest[0] != ' ' {
		return false
	}
	for i, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
		case i > 0 && (r >= '0' && r <= '9' || r == '-' || r == '_'):
		default:
			return false
		}
	}
	return true
}

// View returns the case's input view: every section under in/, with the
// prefix removed.
func (a *Archive) View() fs.FS {
	files := map[string][]byte{}
	for _, s := range a.Sections {
		if rest, ok := strings.CutPrefix(s.Name, In+"/"); ok {
			files[rest] = s.Data
		}
	}
	return NewFS(files)
}

// HandWrittenSections returns the archive's hand-written sections, in the
// order the file holds them.
func (a *Archive) HandWrittenSections() []Section {
	var hs []Section
	for _, s := range a.Sections {
		if HandWritten(s.Name) {
			hs = append(hs, s)
		}
	}
	return hs
}

// Place returns the archive with every update-written section in its place:
// the hand-written sections in the order written, then via/ and out/, each
// sorted by name. Where a section goes depends only on names and on the
// hand-written sections, never on where it sat.
func (a *Archive) Place() *Archive {
	p := &Archive{Comment: a.Comment}
	var via, out []Section
	for _, s := range a.Sections {
		switch top, _, _ := strings.Cut(s.Name, "/"); {
		case top == Via:
			via = append(via, s)
		case top == Out:
			out = append(out, s)
		default:
			p.Sections = append(p.Sections, s)
		}
	}
	byName := func(a, b Section) int { return strings.Compare(a.Name, b.Name) }
	slices.SortStableFunc(via, byName)
	slices.SortStableFunc(out, byName)
	p.Sections = append(p.Sections, via...)
	p.Sections = append(p.Sections, out...)
	return p
}

// Placed reports whether the archive's bytes are what an update writes: its
// update-written sections in their place, in txtar's canonical form.
func Placed(data []byte) bool {
	return bytes.Equal(Parse(data).Place().Format(), data)
}

// HandWrittenEqual reports whether two archives have the same comment and
// the same hand-written sections, in the same order, byte for byte.
func HandWrittenEqual(a, b *Archive) bool {
	if !bytes.Equal(a.Comment, b.Comment) {
		return false
	}
	ha, hb := a.HandWrittenSections(), b.HandWrittenSections()
	return slices.EqualFunc(ha, hb, func(x, y Section) bool {
		return x.Name == y.Name && bytes.Equal(x.Data, y.Data)
	})
}

// Holds reports why a section's data can't be held in an archive, or nil
// when it can: what an archive holds reads back byte for byte.
func Holds(data []byte) error {
	const name = "x"
	got := Parse(txtar.Format(&txtar.Archive{Files: []txtar.File{{Name: name, Data: data}}}))
	if len(got.Sections) == 1 && got.Sections[0].Name == name && bytes.Equal(got.Sections[0].Data, data) {
		return nil
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		return fmt.Errorf("it has no final newline, which an archive adds")
	}
	for i, line := range bytes.SplitAfter(data, []byte("\n")) {
		if a := Parse(append([]byte("\n"), line...)); len(a.Sections) > 0 {
			return fmt.Errorf("its line %d is shaped like a section marker, which starts another section when the archive is read", i+1)
		}
	}
	return fmt.Errorf("it doesn't read back as written")
}
