// Package root finds the module root a test runs under. It doesn't import
// testing, so a command finds the root the same way a test does.
package root

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

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
