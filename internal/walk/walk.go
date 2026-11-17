// Package walk walks a corpus root's testdata/ as the root-package check
// and hypercase strip both read it, so the two find the same files. It
// doesn't import testing.
package walk

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Testdata calls file with each file under td, a corpus root's testdata/,
// in lexical order. It skips each directory below td that is another
// corpus root, as others holds it by its path, or holds a go.mod, the root
// of a nested module. It returns the walk's error, which ends it.
func Testdata(td string, others map[string]bool, file func(p string, d fs.DirEntry)) error {
	return filepath.WalkDir(td, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
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
}

// IsFile reports whether p names a regular file.
func IsFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}
