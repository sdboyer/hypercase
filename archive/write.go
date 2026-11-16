package archive

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sdboyer/hypercase/internal/hooks"
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

// WriteFile writes data to the named file through a temp file in the same
// directory, synced and renamed over it, so a reader sees the old file or
// the new one, never part of one. The file keeps its mode; a new one is
// 0644. A failed write leaves the file as it was and removes the temp file.
func WriteFile(name string, data []byte) error {
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(name); err == nil {
		mode = fi.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return viaTemp(name, data, mode, func(tmp string) error { return os.Rename(tmp, name) })
}

// CreateFile writes data as the named file, which doesn't exist yet,
// through a temp file in the same directory, synced and then linked to the
// name, so a reader sees no file or the whole one, never part of one, and
// a write cut short leaves only a temp file. The link refuses a name that
// exists, even one another process creates during the write: then the
// error is fs.ErrExist, and that file is as it was. The new file's mode is
// 0644. The temp file is removed, whether the write fails or not.
func CreateFile(name string, data []byte) error {
	var tmp string
	err := viaTemp(name, data, 0o644, func(t string) error {
		tmp = t
		return os.Link(t, name)
	})
	if err == nil {
		// The file is created. A temp file this leaves, the root-package
		// check reports.
		os.Remove(tmp)
	}
	return err
}

// viaTemp writes data, with the given mode, to a temp file beside the
// named file, syncs and closes it, and calls finish with the temp file's
// path, to put it in the file's place. When any step fails, it removes
// the temp file.
func viaTemp(name string, data []byte, mode fs.FileMode, finish func(tmp string) error) (err error) {
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
	if hooks.AfterTemp != nil {
		if err := hooks.AfterTemp(tmp); err != nil {
			return err
		}
	}
	return finish(tmp)
}

// Write formats the archive and writes it to the named file with
// [WriteFile]. Before anything is written, it reads the bytes back and
// compares each section with what it meant to write, so no section is ever
// written in a form that reads back differently.
func Write(name string, a *Archive) error {
	data, err := a.ReadBack()
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return WriteFile(name, data)
}

// ReadBack formats the archive, reads the bytes back, and compares the
// comment and each section, its name and its data, with what it meant to
// write. It returns the bytes when every part reads back as meant, and
// otherwise why not, naming the first section that doesn't. It is the
// check every write makes before it writes.
func (a *Archive) ReadBack() ([]byte, error) {
	data := a.Format()
	if err := readsBack(a, data); err != nil {
		return nil, err
	}
	return data, nil
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
