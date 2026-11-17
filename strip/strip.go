// Package strip removes update-written sections from archives: what an
// update writes, and never what a person writes. The command hypercase
// strip is built on it, and a project's own command can call it as that
// command does. A person runs it; no update does, and no package's tests
// run it as hypercase.Test runs them.
//
// It tells the sections apart by name alone, as [archive.HandWritten]
// does: it reads no registry and runs no Go. A frozen cell's sections,
// those of a cell a gaps line marks, are left in place unless the
// selection says to strip them too. Whether a via/<column>/… section is
// part of a cell turns on whether the column is a bytes lift, which no
// name tells, so the caller says what it knows in
// [Selection.BytesLifts].
//
// The package doesn't import testing.
package strip

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/cells"
)

// A Selection says which update-written sections [Archive] removes. The
// zero Selection is every one, frozen cells' aside.
type Selection struct {
	// Column limits it to the column's sections, at every level:
	// out/<Column>/… and via/<Column>/…, and the same under each
	// variant/<name>/ and transform/<name>/. "" is every column.
	Column string
	// Transform limits it to the sections under transform/<Transform>/:
	// its in/…, its delete, and its cells' sections. Given with Column, it
	// limits it to the column's sections under that transform's variant.
	// "" is no limit by transform.
	Transform string
	// Frozen strips frozen cells' sections too.
	Frozen bool
	// BytesLifts says, of the columns over the archive's corpus that the
	// caller knows, which are bytes lifts: true for a bytes lift, false
	// for any other column. A via/<column>/… section is a bytes lift's
	// product, part of the lift's cell, and of no other column's. So a
	// frozen cell keeps its via/<column>/… unless BytesLifts gives its
	// column false, and then that section is removed like one of no cell.
	// A column BytesLifts doesn't hold may be a bytes lift, so its cell
	// keeps its via/<column>/… when frozen. nil knows no column.
	BytesLifts map[string]bool
}

// Selects reports whether sel selects the named section: an
// update-written one within its limits, whether or not a frozen cell
// keeps it.
func (sel Selection) Selects(section string) bool { return selected(section, sel) }

// check reports a selection whose column or transform is no name, by the
// naming rule.
func (sel Selection) check() error {
	if sel.Column != "" && !archive.ValidName(sel.Column) {
		return fmt.Errorf("the column %q is no column's name, which is %s. Give a column's name", sel.Column, archive.NameRule)
	}
	if sel.Transform != "" && !archive.ValidName(sel.Transform) {
		return fmt.Errorf("the transform %q is no transform's name, which is %s. Give a transform's name", sel.Transform, archive.NameRule)
	}
	return nil
}

// A Result is what [Archive] did to one archive: the sections it removed,
// and the frozen cells it left.
type Result struct {
	// Removed are the sections removed, in the order the archive held
	// them.
	Removed []string
	// Frozen are the frozen cells left in place, in the order of their
	// first sections.
	Frozen []Frozen
}

// A Frozen is a frozen cell left in place: a cell that a gaps line marks,
// whose sections the selection would otherwise remove.
type Frozen struct {
	// Case is "" for the base, or variant/<name> or transform/<name> for
	// one of its variants.
	Case   string
	Column string
	// Gaps is the section that holds the line that marks the cell: gaps,
	// for the base's cells and a transform's variant's, or
	// variant/<name>/gaps, for a hand-written variant's. Line is the
	// line's number in it.
	Gaps string
	Line int
	// Sections are the cell's sections left, in the order the archive
	// holds them: its out/<Column>/…, and its via/<Column>/… unless the
	// selection's BytesLifts says the column is no bytes lift.
	Sections []string
}

// Plan returns what [Archive] removes from a, and the frozen cells it
// leaves, and writes nothing. It is meant for an archive
// [archive.Archive.Check] passes, the one kind Archive strips.
func Plan(a *archive.Archive, sel Selection) Result {
	marked := cells.Marks(a)
	var r Result
	frozen := map[cells.Cell]int{} // each frozen cell's index in r.Frozen
	for _, s := range a.Sections {
		if !selected(s.Name, sel) {
			continue
		}
		if c, ok := cells.Of(s.Name); ok && !sel.Frozen && cells.Part(s.Name, sel.BytesLifts) {
			if m, ok := marked[c]; ok {
				i, seen := frozen[c]
				if !seen {
					i = len(r.Frozen)
					frozen[c] = i
					r.Frozen = append(r.Frozen, Frozen{Case: m.Case, Column: c.Column, Gaps: m.Gaps, Line: m.Line})
				}
				r.Frozen[i].Sections = append(r.Frozen[i].Sections, s.Name)
				continue
			}
		}
		r.Removed = append(r.Removed, s.Name)
	}
	return r
}

// selected reports whether sel selects the section of the given name: an
// update-written one, within the selection's limits.
func selected(name string, sel Selection) bool {
	if archive.HandWritten(name) {
		return false
	}
	kind, cs, _ := archive.SplitCase(name)
	if sel.Transform != "" && (kind != archive.Transform || cs != sel.Transform) {
		return false
	}
	if sel.Column != "" {
		if c, ok := cells.Of(name); !ok || c.Column != sel.Column {
			return false
		}
	}
	return true
}

// Archive removes from the archive at path every update-written section
// sel selects, under the corpus root's lock, and returns what it removed
// and the frozen cells it left. It takes the lock, reads the archive,
// writes what is left to a temp file in the archive's directory, through
// the archive package's read-back check, renames it over the archive, and
// only then releases the lock. The sections left keep their bytes and
// their order. An archive with nothing to remove isn't rewritten.
//
// It refuses, and writes nothing to, a path that isn't an archive of the
// corpus root (see [CheckPath]), and an archive with problems of form,
// which no update writes. A write that fails leaves the archive as it was,
// and removes its temp file.
func Archive(root, path string, sel Selection) (Result, error) {
	if err := sel.check(); err != nil {
		return Result{}, err
	}
	if err := CheckPath(root, path); err != nil {
		return Result{}, err
	}
	var r Result
	var refused error
	err := archive.Modify(lockPath(root), path, func(a *archive.Archive) (*archive.Archive, error) {
		if refused = CheckForm(a); refused != nil {
			return nil, refused
		}
		r = Plan(a, sel)
		if len(r.Removed) == 0 {
			return nil, nil
		}
		return without(a, r.Removed), nil
	})
	if err != nil {
		return Result{}, failed(err, refused)
	}
	return r, nil
}

// failed words an error of [archive.Modify]: a refusal as it is, and
// anything else as a failure that left the archive as it was.
func failed(err, refused error) error {
	if err == nil || err == refused {
		return err
	}
	return fmt.Errorf("stripping it failed, and the archive is as it was: %w. Fix what failed", err)
}

// Remove removes the named sections from the archive at path, under the
// corpus root's lock, as [Archive] writes. Each must be update-written by
// name: a hand-written name, one the archive lacks, and one named twice
// are each refused, naming it, and nothing is written. A frozen cell's
// section is removed like any other, since naming it is the choice. Like
// Archive, it refuses a path that isn't an archive of the corpus root, and
// an archive with problems of form.
func Remove(root, path string, names []string) error {
	if err := CheckPath(root, path); err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}
	var refused error
	err := archive.Modify(lockPath(root), path, func(a *archive.Archive) (*archive.Archive, error) {
		if refused = CheckForm(a); refused != nil {
			return nil, refused
		}
		have := map[string]bool{}
		for _, s := range a.Sections {
			have[s.Name] = true
		}
		var why []string
		seen := map[string]bool{}
		for _, n := range names {
			switch {
			case seen[n]:
				why = append(why, n+" is named twice")
			case archive.HandWritten(n):
				why = append(why, n+" is hand-written, and only update-written sections are removed")
			case !have[n]:
				why = append(why, "the archive has no section "+n)
			}
			seen[n] = true
		}
		if len(why) > 0 {
			refused = fmt.Errorf("%s. Nothing was removed: name each update-written section to remove once", strings.Join(why, "; "))
			return nil, refused
		}
		return without(a, names), nil
	})
	return failed(err, refused)
}

// without returns the archive less the named sections, the rest in their
// order.
func without(a *archive.Archive, names []string) *archive.Archive {
	gone := map[string]bool{}
	for _, n := range names {
		gone[n] = true
	}
	next := &archive.Archive{Comment: a.Comment}
	for _, s := range a.Sections {
		if !gone[s.Name] {
			next.Sections = append(next.Sections, s)
		}
	}
	return next
}

// CheckForm reports why [Archive] and [Remove] refuse an archive for its
// form, or nil when they don't: an archive with problems of form, none of
// whose cells run, so that no update writes it, and no strip does either.
func CheckForm(a *archive.Archive) error {
	ps := a.Check()
	if len(ps) == 0 {
		return nil
	}
	var lines []string
	for _, p := range ps {
		lines = append(lines, p.String())
	}
	return fmt.Errorf("the archive has problems of form, so no update writes it: %s. Fix the archive by hand", strings.Join(lines, "; "))
}

// lockPath is the corpus root's lock file, which every update under the
// root takes around each read-modify-write of an archive.
func lockPath(root string) string {
	return filepath.Join(root, "testdata", archive.LockFile)
}

// CheckPath reports why path is no archive of the corpus root, or nil
// when it is one: a file of a corpus's directory,
// testdata/<corpus>/<archive>.txtar under the root, where the corpus's
// directory is not testdata/checks/, which holds corpus checks' findings,
// and each name follows the naming rule. Only such an archive's cells
// run, so only it is one an update writes.
func CheckPath(root, path string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return err
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	file := parts[len(parts)-1]
	name, isArchive := strings.CutSuffix(file, ".txtar")
	switch {
	case parts[0] == ".." || parts[0] != "testdata" || len(parts) < 2:
		return errors.New("it is not under the corpus root's testdata/, where a corpus's archives are. Name an archive of the corpus root, testdata/<corpus>/<archive>.txtar under it")
	case !isArchive:
		return fmt.Errorf("an archive's file name ends in .txtar, and %s doesn't. Name an archive of the corpus root, testdata/<corpus>/<archive>.txtar under it", file)
	case len(parts) == 2:
		return errors.New("an archive belongs in a corpus's directory, testdata/<corpus>/, and this one is directly in testdata/. Move it into one")
	case len(parts) > 3:
		return fmt.Errorf("an archive belongs directly in a corpus's directory, testdata/<corpus>/, and this one is below testdata/%s/, so it is no case. Move it into a corpus's directory, or delete it", parts[1])
	case parts[1] == archive.ChecksDir:
		return fmt.Errorf("testdata/%s/ holds corpus checks' findings, so no corpus has the directory. Move the archive into a corpus's directory, or delete it", archive.ChecksDir)
	case !archive.ValidName(parts[1]):
		return fmt.Errorf("the directory's name, %q, breaks the naming rule: %s, so no corpus has it. Rename the directory", parts[1], archive.NameRule)
	case !archive.ValidName(name):
		return fmt.Errorf("the archive's name, %q, breaks the naming rule: %s, so none of its cells run, and no update writes it. Rename the archive", name, archive.NameRule)
	}
	return nil
}
