package archive

import (
	"bytes"
	"io"
	"io/fs"
	"maps"
	"path"
	"slices"
	"time"

	"github.com/sdboyer/hypercase/internal/dirs"
)

// NewFS returns a read-only file system holding the given files, keyed by
// slash path. Directories are implied by the paths. The FS keeps its own
// copy of each file, so what a reader does to the bytes it reads changes
// nothing for the next reader.
func NewFS(files map[string][]byte) fs.FS {
	m := &memFS{files: map[string][]byte{}, dirs: dirs.Index(maps.Keys(files))}
	for name, data := range files {
		m.files[name] = bytes.Clone(data)
	}
	return m
}

type memFS struct {
	files map[string][]byte
	dirs  map[string][]string // each directory's entries, sorted
}

func (m *memFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if data, ok := m.files[name]; ok {
		return &memFile{info: fileInfo{name: path.Base(name), size: int64(len(data))}, r: bytes.NewReader(data)}, nil
	}
	if _, ok := m.dirs[name]; ok {
		entries, _ := m.ReadDir(name)
		return &memDir{info: fileInfo{name: path.Base(name), dir: true}, entries: entries}, nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (m *memFS) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrInvalid}
	}
	if data, ok := m.files[name]; ok {
		return bytes.Clone(data), nil
	}
	if _, ok := m.dirs[name]; ok {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: errIsDir}
	}
	return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrNotExist}
}

func (m *memFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	kids, ok := m.dirs[name]
	if !ok {
		if _, isFile := m.files[name]; isFile {
			return nil, &fs.PathError{Op: "readdir", Path: name, Err: errNotDir}
		}
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	entries := make([]fs.DirEntry, 0, len(kids))
	for _, k := range kids {
		full := path.Join(name, k)
		if data, isFile := m.files[full]; isFile {
			entries = append(entries, fs.FileInfoToDirEntry(fileInfo{name: k, size: int64(len(data))}))
		} else {
			entries = append(entries, fs.FileInfoToDirEntry(fileInfo{name: k, dir: true}))
		}
	}
	return entries, nil
}

func (m *memFS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	if data, ok := m.files[name]; ok {
		return fileInfo{name: path.Base(name), size: int64(len(data))}, nil
	}
	if _, ok := m.dirs[name]; ok {
		return fileInfo{name: path.Base(name), dir: true}, nil
	}
	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

// paths returns every file's path, sorted.
func (m *memFS) paths() []string {
	ps := make([]string, 0, len(m.files))
	for p := range m.files {
		ps = append(ps, p)
	}
	slices.Sort(ps)
	return ps
}

type pathErr string

func (e pathErr) Error() string { return string(e) }

var (
	errIsDir  = pathErr("is a directory")
	errNotDir = pathErr("not a directory")
)

type fileInfo struct {
	name string
	size int64
	dir  bool
}

func (fi fileInfo) Name() string { return fi.name }
func (fi fileInfo) Size() int64  { return fi.size }
func (fi fileInfo) Mode() fs.FileMode {
	if fi.dir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
func (fi fileInfo) ModTime() time.Time { return time.Time{} }
func (fi fileInfo) IsDir() bool        { return fi.dir }
func (fi fileInfo) Sys() any           { return nil }

type memFile struct {
	info fileInfo
	r    *bytes.Reader
}

func (f *memFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *memFile) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *memFile) Seek(off int64, whence int) (int64, error) {
	return f.r.Seek(off, whence)
}
func (f *memFile) ReadAt(p []byte, off int64) (int, error) { return f.r.ReadAt(p, off) }
func (f *memFile) Close() error                            { return nil }

type memDir struct {
	info    fileInfo
	entries []fs.DirEntry
	off     int
}

func (d *memDir) Stat() (fs.FileInfo, error) { return d.info, nil }
func (d *memDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.info.name, Err: errIsDir}
}
func (d *memDir) Close() error { return nil }
func (d *memDir) ReadDir(n int) ([]fs.DirEntry, error) {
	rest := d.entries[d.off:]
	if n <= 0 {
		d.off = len(d.entries)
		return slices.Clone(rest), nil
	}
	if len(rest) == 0 {
		return nil, io.EOF
	}
	if n > len(rest) {
		n = len(rest)
	}
	d.off += n
	return slices.Clone(rest[:n]), nil
}

// Paths returns the path of every file in a file system, sorted. It reads
// the FS with [fs.WalkDir], so it works on any FS.
func Paths(fsys fs.FS) ([]string, error) {
	if m, ok := fsys.(*memFS); ok {
		return m.paths(), nil
	}
	var ps []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			ps = append(ps, p)
		}
		return nil
	})
	slices.Sort(ps)
	return ps, err
}
