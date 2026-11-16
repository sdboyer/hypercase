package archive

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// tempPrefix starts the name of every temp file a write creates. A temp file
// sits beside the file it replaces, and its name never ends in .txtar, so no
// run reads one as an archive.
const tempPrefix = ".hypercase-tmp-"

// IsTemp reports whether a file name is that of a temp file a write
// creates. One that is still there was left by a write that was cut short.
func IsTemp(name string) bool {
	return strings.HasPrefix(filepath.Base(name), tempPrefix)
}

// afterTemp, when set, runs once a write's temp file is written and before
// it is renamed. Tests set it to make a write fail at that point.
var afterTemp func(tmp string) error

// WriteFile writes data to the named file through a temp file in the same
// directory, synced and renamed over it, so a reader sees the old file or
// the new one, never part of one. The file keeps its mode; a new one is
// 0644. A failed write leaves the file as it was and removes the temp file.
func WriteFile(name string, data []byte) (err error) {
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(name); err == nil {
		mode = fi.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(name), tempPrefix+filepath.Base(name)+"-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if afterTemp != nil {
		if err := afterTemp(tmp); err != nil {
			return err
		}
	}
	return os.Rename(tmp, name)
}

// Write formats the archive and writes it to the named file with
// [WriteFile]. Before anything is written, it reads the bytes back and
// compares each section with what it meant to write, so no section is ever
// written in a form that reads back differently.
func Write(name string, a *Archive) error {
	data := a.Format()
	if err := readsBack(a, data); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return WriteFile(name, data)
}

func readsBack(a *Archive, data []byte) error {
	b := Parse(data)
	if !bytes.Equal(a.Comment, b.Comment) {
		return fmt.Errorf("the comment would read back differently")
	}
	for i, s := range a.Sections {
		if err := Holds(s.Data); err != nil {
			return fmt.Errorf("section %s can't be held: %w", s.Name, err)
		}
		if i >= len(b.Sections) || b.Sections[i].Name != s.Name || !bytes.Equal(b.Sections[i].Data, s.Data) {
			return fmt.Errorf("section %s would read back differently", s.Name)
		}
	}
	if len(b.Sections) != len(a.Sections) {
		return fmt.Errorf("the archive would read back with %d sections, not %d", len(b.Sections), len(a.Sections))
	}
	return nil
}
