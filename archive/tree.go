package archive

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// TreeSections is the walk that turns a tree of files into sections: each
// regular file of tree becomes the section prefix + its path, holding its
// bytes exactly as read, whatever their size. prefix is empty, or a slash
// path ending in a slash, such as in/sheet/. The sections are sorted by
// name, so one tree always gives the same sections. A tree with no files
// gives none.
//
// It reads every file of the tree, and refuses what no archive can hold, so
// that what it returns reads back as the tree: an empty directory; a file
// that isn't regular, such as a symlink, a device or a socket; and a section
// that the archive's read-back check finds would read back differently,
// such as one holding a line shaped like a section marker. It fails on
// any error reading the tree. Each refusal and failure is a [*TreeError]
// naming the path in the tree, and it returns no sections with it.
func TreeSections(prefix string, tree fs.FS) ([]Section, error) {
	if dir, ok := strings.CutSuffix(prefix, "/"); prefix != "" && (!ok || dir == "." || !fs.ValidPath(dir)) {
		return nil, fmt.Errorf("the prefix %q is neither empty nor a slash path ending in a slash", prefix)
	}
	var ss []Section
	var dirs []string         // the directories below the root, as the walk visits them
	full := map[string]bool{} // the directories that hold an entry
	err := fs.WalkDir(tree, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return &TreeError{Path: p, Err: err}
		}
		if p != "." {
			full[path.Dir(p)] = true
		}
		switch {
		case d.IsDir():
			if p != "." {
				dirs = append(dirs, p)
			}
			return nil
		case !d.Type().IsRegular():
			return &TreeError{Path: p, Err: fmt.Errorf("it is %s, not a regular file, and an archive holds only regular files", kindOf(d.Type()))}
		}
		data, err := fs.ReadFile(tree, p)
		if err != nil {
			return &TreeError{Path: p, Err: err}
		}
		s := Section{Name: prefix + p, Data: data}
		if err := Holds(data); err != nil {
			return &TreeError{Path: p, Err: err}
		}
		if err := HoldsName(s.Name); err != nil {
			return &TreeError{Path: p, Err: fmt.Errorf("its section's name, %q, can't be held in an archive: %v", s.Name, err)}
		}
		ss = append(ss, s)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, d := range dirs {
		if !full[d] {
			return nil, &TreeError{Path: d, Err: fmt.Errorf("it is an empty directory, and an archive holds a directory only as the files in it")}
		}
	}
	slices.SortFunc(ss, func(a, b Section) int { return strings.Compare(a.Name, b.Name) })
	return ss, nil
}

// A TreeError is why [TreeSections] can't read a tree's file, or a
// directory, as sections.
type TreeError struct {
	Path string // the path in the tree
	Err  error
}

func (e *TreeError) Error() string { return e.Path + ": " + e.Err.Error() }

func (e *TreeError) Unwrap() error { return e.Err }

// HoldsName reports why a section of the given name can't be held in an
// archive, or nil when it can: a section's name is read from its marker
// line, trimmed of white space, so it reads back as written only when it
// is not empty, holds no newline, and neither starts nor ends with white
// space.
func HoldsName(name string) error {
	a := &Archive{Sections: []Section{{Name: name}}}
	if _, err := a.ReadBack(); err == nil {
		return nil
	}
	switch {
	case name == "":
		return errors.New("it is empty, and a marker line with no name starts no section")
	case strings.Contains(name, "\n"):
		return errors.New("it holds a newline, which ends the marker line that names a section")
	case strings.TrimSpace(name) != name:
		return errors.New("it starts or ends with white space, which reading the marker line trims")
	}
	return errors.New("it doesn't read back as written")
}

// kindOf names the kind of a file that isn't regular.
func kindOf(m fs.FileMode) string {
	switch {
	case m&fs.ModeSymlink != 0:
		return "a symlink"
	case m&fs.ModeNamedPipe != 0:
		return "a named pipe"
	case m&fs.ModeSocket != 0:
		return "a socket"
	case m&fs.ModeDevice != 0:
		return "a device"
	}
	return "an irregular file"
}
