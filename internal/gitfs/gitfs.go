// Package gitfs reads a git repository's revisions from its object store,
// never from its working tree. A revision's tree is an [fs.FS], so code
// that reads files on disk reads a revision the same way.
//
// It runs the git command: git rev-parse to find where it is and to
// resolve revisions, git ls-tree to list a tree, and one git cat-file
// --batch to read the files.
package gitfs

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os/exec"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sdboyer/hypercase/internal/dirs"
)

// A Repo is a git repository, as seen from a directory in its working
// tree.
type Repo struct {
	dir    string
	env    []string
	prefix string

	mu        sync.Mutex
	cat       *exec.Cmd
	catStderr bytes.Buffer // cat's stderr, read once it exits
	catErr    error        // the failure that stopped cat, which every later read gives
	in        io.WriteCloser
	outPipe   io.ReadCloser // cat's stdout, which out reads
	out       *bufio.Reader
	blobs     map[string][]byte
}

// Open opens the repository whose working tree holds dir. Each git
// command runs in dir with env as its environment, or with this process's
// environment when env is nil.
func Open(dir string, env []string) (*Repo, error) {
	r := &Repo{dir: dir, env: env, blobs: map[string][]byte{}}
	out, err := r.git("rev-parse", "--show-prefix")
	if err != nil {
		return nil, err
	}
	r.prefix = strings.TrimSuffix(strings.TrimSuffix(string(out), "\n"), "/")
	if r.prefix == "" {
		r.prefix = "."
	}
	return r, nil
}

// Prefix returns the directory the repository was opened in, as a slash
// path relative to the top of the working tree: "." for the top.
func (r *Repo) Prefix() string { return r.prefix }

// Resolve returns the commit a revision names, such as main or HEAD~2. A
// revision git can't resolve names no commit: git rev-parse exits 1,
// writing nothing on stderr. One that git fails on otherwise is an error
// that holds rev-parse's stderr.
func (r *Repo) Resolve(rev string) (string, error) {
	out, stderr, err := r.run("rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
	switch {
	case err != nil && stderr != "":
		return "", fmt.Errorf("git rev-parse: %s", stderr)
	case err != nil:
		return "", fmt.Errorf("%s names no commit in this repository", rev)
	}
	return strings.TrimSpace(string(out)), nil
}

// Tree returns a commit's tree as a read-only file system, whose root is
// the top of the working tree. Its files are read from the object store
// when they are first opened. A symbolic link is a file of mode
// [fs.ModeSymlink], which Open and ReadFile refuse, and a submodule is
// left out.
func (r *Repo) Tree(commit string) (fs.FS, error) {
	out, err := r.git("ls-tree", "-r", "-z", "--full-tree", commit)
	if err != nil {
		return nil, err
	}
	t := &tree{repo: r, files: map[string]entry{}}
	for _, rec := range bytes.Split(bytes.TrimSuffix(out, []byte{0}), []byte{0}) {
		if len(rec) == 0 {
			continue
		}
		meta, name, ok := strings.Cut(string(rec), "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 {
			return nil, fmt.Errorf("git ls-tree gave a line it doesn't document: %q", rec)
		}
		mode, typ, hash := fields[0], fields[1], fields[2]
		if typ != "blob" {
			continue // a submodule's commit, which this object store doesn't hold
		}
		var m fs.FileMode
		switch mode {
		case "100644":
			m = 0o444
		case "100755":
			m = 0o555
		case "120000":
			m = fs.ModeSymlink | 0o777
		default:
			return nil, fmt.Errorf("git ls-tree gave %s the mode %s, which it doesn't document", name, mode)
		}
		t.files[name] = entry{mode: m, hash: hash}
	}
	t.dirs = dirs.Index(maps.Keys(t.files))
	return t, nil
}

// Close stops the repository's git cat-file, if one runs.
func (r *Repo) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cat == nil {
		return nil
	}
	return r.stopCat()
}

// stopCat stops the repository's git cat-file, and returns its failure,
// holding what it wrote on stderr. It closes the pipe cat-file writes to
// before it waits, so a cat-file blocked writing what no read will take
// gets a broken pipe, and exits.
func (r *Repo) stopCat() error {
	r.in.Close()
	r.outPipe.Close()
	err := r.cat.Wait()
	r.cat = nil
	if err != nil {
		if msg := strings.TrimSpace(r.catStderr.String()); msg != "" {
			return fmt.Errorf("git cat-file: %s", msg)
		}
		return fmt.Errorf("git cat-file: %w", err)
	}
	return nil
}

// catFailed records that reading from git cat-file failed with err, and
// returns the error every later read gives: cat-file's own failure, with
// its stderr, when it exited with one.
func (r *Repo) catFailed(err error) error {
	if stop := r.stopCat(); stop != nil {
		r.catErr = stop
	} else {
		r.catErr = fmt.Errorf("git cat-file: %w", err)
	}
	return r.catErr
}

// git runs a git command in the repository's directory, and returns its
// stdout.
func (r *Repo) git(args ...string) ([]byte, error) {
	out, stderr, err := r.run(args...)
	if err != nil {
		if stderr != "" {
			return nil, fmt.Errorf("git %s: %s", args[0], stderr)
		}
		return nil, fmt.Errorf("git %s: %w", args[0], err)
	}
	return out, nil
}

// run runs a git command in the repository's directory, and returns its
// stdout, and its stderr less the space at either end.
func (r *Repo) run(args ...string) (stdout []byte, stderr string, err error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	cmd.Env = r.env
	var errb bytes.Buffer
	cmd.Stderr = &errb
	out, err := cmd.Output()
	return out, strings.TrimSpace(errb.String()), err
}

// blob returns an object's bytes, which the caller doesn't modify.
func (r *Repo) blob(hash string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if data, ok := r.blobs[hash]; ok {
		return data, nil
	}
	if r.catErr != nil {
		return nil, r.catErr
	}
	if r.cat == nil {
		cmd := exec.Command("git", "cat-file", "--batch")
		cmd.Dir = r.dir
		cmd.Env = r.env
		r.catStderr.Reset()
		cmd.Stderr = &r.catStderr
		in, err := cmd.StdinPipe()
		if err != nil {
			return nil, err
		}
		out, err := cmd.StdoutPipe()
		if err != nil {
			return nil, err
		}
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("git cat-file: %w", err)
		}
		r.cat, r.in, r.outPipe, r.out = cmd, in, out, bufio.NewReader(out)
	}
	if _, err := io.WriteString(r.in, hash+"\n"); err != nil {
		return nil, r.catFailed(err)
	}
	head, err := r.out.ReadString('\n')
	if err != nil {
		return nil, r.catFailed(err)
	}
	fields := strings.Fields(head)
	if len(fields) == 2 && fields[0] == hash && fields[1] == "missing" {
		return nil, fmt.Errorf("git cat-file finds no object %s in the repository", hash)
	}
	size := -1
	if len(fields) == 3 && fields[0] == hash {
		if n, err := strconv.Atoi(fields[2]); err == nil {
			size = n
		}
	}
	if size < 0 {
		// The header doesn't say how much content follows it, so no later
		// read can find its own header: each gives this error, with what
		// cat-file wrote on stderr, if anything. Stopping cat-file before it
		// has written what follows may fail it with a broken pipe, so its
		// exit status is left out.
		r.stopCat()
		r.catErr = fmt.Errorf("git cat-file gave %q for the blob %s", strings.TrimSpace(head), hash)
		if msg := strings.TrimSpace(r.catStderr.String()); msg != "" {
			r.catErr = fmt.Errorf("%w: %s", r.catErr, msg)
		}
		return nil, r.catErr
	}
	if fields[1] != "blob" {
		// Another kind of object, whose content is read past, so that the
		// next read finds its own header.
		if _, err := io.CopyN(io.Discard, r.out, int64(size)+1); err != nil {
			return nil, r.catFailed(err)
		}
		return nil, fmt.Errorf("git cat-file gives a %s, not a blob, for %s", fields[1], hash)
	}
	data := make([]byte, size+1) // the content, then a newline
	if _, err := io.ReadFull(r.out, data); err != nil {
		return nil, r.catFailed(err)
	}
	data = data[:size:size]
	r.blobs[hash] = data
	return data, nil
}

// tree is a commit's tree.
type tree struct {
	repo  *Repo
	files map[string]entry
	dirs  map[string][]string // each directory's entries, sorted
}

type entry struct {
	mode fs.FileMode
	hash string
}

var errSymlink = errors.New("is a symbolic link, which a revision's tree doesn't follow")

func (t *tree) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if e, ok := t.files[name]; ok {
		data, err := t.read("open", name, e)
		if err != nil {
			return nil, err
		}
		return &file{info: info{name: path.Base(name), size: int64(len(data)), mode: e.mode}, r: bytes.NewReader(data)}, nil
	}
	if _, ok := t.dirs[name]; ok {
		entries, err := t.ReadDir(name)
		if err != nil {
			return nil, err
		}
		return &dir{info: info{name: path.Base(name), mode: fs.ModeDir | 0o555}, entries: entries}, nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (t *tree) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrInvalid}
	}
	e, ok := t.files[name]
	if !ok {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrNotExist}
	}
	data, err := t.read("readfile", name, e)
	if err != nil {
		return nil, err
	}
	return bytes.Clone(data), nil
}

func (t *tree) read(op, name string, e entry) ([]byte, error) {
	if e.mode&fs.ModeSymlink != 0 {
		return nil, &fs.PathError{Op: op, Path: name, Err: errSymlink}
	}
	data, err := t.repo.blob(e.hash)
	if err != nil {
		return nil, &fs.PathError{Op: op, Path: name, Err: err}
	}
	return data, nil
}

func (t *tree) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	if e, ok := t.files[name]; ok {
		return t.fileInfo(name, e)
	}
	if _, ok := t.dirs[name]; ok {
		return info{name: path.Base(name), mode: fs.ModeDir | 0o555}, nil
	}
	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

// fileInfo describes a file. A regular file's size is its blob's, read
// from the object store; a symbolic link's is that of its target's name.
func (t *tree) fileInfo(name string, e entry) (fs.FileInfo, error) {
	data, err := t.repo.blob(e.hash)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}
	return info{name: path.Base(name), size: int64(len(data)), mode: e.mode}, nil
}

func (t *tree) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	kids, ok := t.dirs[name]
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	entries := make([]fs.DirEntry, 0, len(kids))
	for _, k := range kids {
		full := path.Join(name, k)
		if e, isFile := t.files[full]; isFile {
			entries = append(entries, dirEntry{t: t, path: full, e: e})
		} else {
			entries = append(entries, fs.FileInfoToDirEntry(info{name: k, mode: fs.ModeDir | 0o555}))
		}
	}
	return entries, nil
}

// dirEntry is a file's entry in a directory, whose Info reads the blob's
// size only when asked.
type dirEntry struct {
	t    *tree
	path string
	e    entry
}

func (d dirEntry) Name() string               { return path.Base(d.path) }
func (d dirEntry) IsDir() bool                { return false }
func (d dirEntry) Type() fs.FileMode          { return d.e.mode.Type() }
func (d dirEntry) Info() (fs.FileInfo, error) { return d.t.fileInfo(d.path, d.e) }

type info struct {
	name string
	size int64
	mode fs.FileMode
}

func (fi info) Name() string       { return fi.name }
func (fi info) Size() int64        { return fi.size }
func (fi info) Mode() fs.FileMode  { return fi.mode }
func (fi info) ModTime() time.Time { return time.Time{} }
func (fi info) IsDir() bool        { return fi.mode.IsDir() }
func (fi info) Sys() any           { return nil }

type file struct {
	info info
	r    *bytes.Reader
}

func (f *file) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *file) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *file) Close() error               { return nil }

type dir struct {
	info    info
	entries []fs.DirEntry
	off     int
}

func (d *dir) Stat() (fs.FileInfo, error) { return d.info, nil }
func (d *dir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.info.name, Err: errors.New("is a directory")}
}
func (d *dir) Close() error { return nil }
func (d *dir) ReadDir(n int) ([]fs.DirEntry, error) {
	rest := d.entries[d.off:]
	if n <= 0 {
		d.off = len(d.entries)
		return slices.Clone(rest), nil
	}
	if len(rest) == 0 {
		return nil, io.EOF
	}
	n = min(n, len(rest))
	d.off += n
	return slices.Clone(rest[:n]), nil
}
