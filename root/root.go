// Package root finds the corpus root a package's tests read their cases
// under, and the module root above it. It doesn't import testing, so a
// command finds them the same way a test does.
package root

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// A Corpus is a corpus root, as [Find] finds it: the module root, and the
// corpus root's path under it. The zero Corpus is none.
type Corpus struct {
	module string // the module root, an absolute path
	rel    string // the corpus root, a slash path relative to module
}

// Find returns the corpus root of a package in dir whose corpus root is
// rel: the path the package passes to hypercase.Root, a slash path
// relative to the module root, or "." for a package that passes none. The
// module root is the nearest directory, from dir upward, that holds a
// go.mod. A command that writes a case under the corpus root Find returns
// writes where the package's tests read.
func Find(dir, rel string) (Corpus, error) {
	if !fs.ValidPath(rel) {
		return Corpus{}, fmt.Errorf("the corpus root %q isn't a slash path relative to the module root, such as walkthrough/step05", rel)
	}
	m, err := module(dir)
	if err != nil {
		return Corpus{}, err
	}
	return Corpus{module: m, rel: rel}, nil
}

// Dir returns the corpus root's directory.
func (c Corpus) Dir() string { return filepath.Join(c.module, filepath.FromSlash(c.rel)) }

// Module returns the module root.
func (c Corpus) Module() string { return c.module }

// Rel returns the corpus root's path relative to the module root, as a
// package passes it to hypercase.Root: "." for the module root itself.
func (c Corpus) Rel() string { return c.rel }

// module returns the module root: the nearest directory, from dir upward,
// that holds a go.mod.
func module(dir string) (string, error) {
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
