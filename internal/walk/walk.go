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
//
// It follows no symbolic link, where a run reads a corpus's archives
// through one. So it returns, in links, each link in place of td, or
// directly in td and naming a directory, where a corpus's directory is,
// and reads nothing through it.
func Testdata(td string, others map[string]bool, file func(p string, d fs.DirEntry)) (links []string, err error) {
	err = filepath.WalkDir(td, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 && (p == td || filepath.Dir(p) == td && isDir(p)) {
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

// isDir reports whether p names a directory, through a link.
func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
