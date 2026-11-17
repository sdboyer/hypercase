// Package walk walks a corpus root's testdata/ as the root-package check
// and hypercase strip both read it, so the two find the same files, and
// tells by its path which file is an archive of the corpus root, so the
// two agree on which archives strip strips. It doesn't import testing.
package walk

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sdboyer/hypercase/archive"
)

// Testdata calls file with each file under td, a corpus root's testdata/,
// in lexical order. It skips each directory below td that is another
// corpus root, as others holds it by its path, or holds a go.mod, the root
// of a nested module. It returns the walk's error, which ends it.
//
// No file under a corpus root's testdata/, td included, is a symbolic
// link. Testdata follows none: it returns, in links, each one it reaches,
// whatever it names, and calls file with none of them.
func Testdata(td string, others map[string]bool, file func(p string, d fs.DirEntry)) (links []string, err error) {
	err = filepath.WalkDir(td, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			links = append(links, p)
			return nil
		}
		if d.IsDir() {
			if p != td && (others[p] || IsFile(filepath.Join(p, "go.mod"))) {
				return filepath.SkipDir
			}
			return nil
		}
		file(p, d)
		return nil
	})
	return links, err
}

// IsFile reports whether p names a regular file.
func IsFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// CheckPath reports why path is no archive of the corpus root, or nil
// when it is one: a file of a corpus's directory,
// testdata/<corpus>/<archive>.txtar under the root, where the corpus's
// directory is not testdata/checks/, which holds corpus checks' findings,
// and each name follows the naming rule. Only such an archive's cells
// run, so only it is one an update writes. The error ends with the fix:
// it is a [*PathError], but where the path can't be made relative to the
// root.
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
	if e := ArchivePath(filepath.ToSlash(rel)); e != nil {
		return e
	}
	return nil
}

// A PathError says why a path is no archive of the corpus root, and what
// to do about it.
type PathError struct {
	Reason, Fix string
}

func (e *PathError) Error() string { return e.Reason + ". " + e.Fix }

// ArchivePath is [CheckPath]'s rule for rel, a slash path relative to the
// corpus root.
func ArchivePath(rel string) *PathError {
	parts := strings.Split(rel, "/")
	file := parts[len(parts)-1]
	name, isArchive := strings.CutSuffix(file, ".txtar")
	const nameOne = "Name an archive of the corpus root, testdata/<corpus>/<archive>.txtar under it"
	switch {
	case parts[0] == ".." || parts[0] != "testdata" || len(parts) < 2:
		return &PathError{"it is not under the corpus root's testdata/, where a corpus's archives are", nameOne}
	case !isArchive:
		return &PathError{fmt.Sprintf("an archive's file name ends in .txtar, and %s doesn't", file), nameOne}
	case len(parts) == 2:
		return &PathError{"an archive belongs in a corpus's directory, testdata/<corpus>/, and this one is directly in testdata/", "Move it into one, or delete it"}
	case len(parts) > 3:
		return &PathError{fmt.Sprintf("an archive belongs directly in a corpus's directory, testdata/<corpus>/, and this one is below testdata/%s/, so it is no case", parts[1]), "Move it into a corpus's directory, or delete it"}
	case parts[1] == archive.ChecksDir:
		return &PathError{fmt.Sprintf("testdata/%s/ holds corpus checks' findings, so no corpus has the directory", archive.ChecksDir), "Move the archive into a corpus's directory, or delete it"}
	case !archive.ValidName(parts[1]):
		return &PathError{fmt.Sprintf("the directory's name, %q, breaks the naming rule: %s, so no corpus has it", parts[1], archive.NameRule), "Rename the directory"}
	case !archive.ValidName(name):
		return &PathError{fmt.Sprintf("the archive's name, %q, breaks the naming rule: %s, so none of its cells run, and no update writes it", name, archive.NameRule), "Rename the archive"}
	}
	return nil
}
