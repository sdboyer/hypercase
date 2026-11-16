// Package root finds the module root a test runs under, and the corpus
// root under it. It doesn't import testing, so a command finds the roots
// the same way a test does.
package root

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Corpus returns the corpus root of a package in dir whose corpus root is
// rel: the path the package passes to hypercase.Root, a slash path
// relative to the module root, or "." for a package that passes none. It
// is the module root, found from dir as [Module] finds it, with rel
// under it. A command that writes a case under the corpus root it
// returns writes where the package's tests read.
func Corpus(dir, rel string) (string, error) {
	if !fs.ValidPath(rel) {
		return "", fmt.Errorf("the corpus root %q isn't a slash path relative to the module root, such as walkthrough/step05", rel)
	}
	m, err := Module(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(m, filepath.FromSlash(rel)), nil
}

// Module returns the module root: the nearest directory, from dir upward,
// that holds a go.mod.
func Module(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if filepath.Dir(d) == d {
			return "", fmt.Errorf("no go.mod in %s or any directory above it", dir)
		}
	}
}

// Rel returns target relative to base, with slashes.
func Rel(base, target string) string {
	r, err := filepath.Rel(base, target)
	if err != nil {
		return filepath.ToSlash(target)
	}
	return filepath.ToSlash(r)
}
