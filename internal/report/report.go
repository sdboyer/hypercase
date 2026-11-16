// Package report builds hypercase report: what changed in a corpus root
// between two git revisions, read from the repository's object store.
//
// It reads registries and archives, never Go source, and works from the
// cells' committed outcomes alone. It ranks the changes: changed outcomes
// first, then cells that appeared, then cells that disappeared, then cells
// whose files changed under the same outcome. And it shows the matrix of
// each corpus at the second revision, each case with the properties it
// states and each known gap on the cell it marks, linked to its issue.
package report

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sdboyer/hypercase/internal/gitfs"
	"github.com/sdboyer/hypercase/internal/registry"
)

// Options are what a report is asked for.
type Options struct {
	// Dir is the directory the report runs in, in a git working tree.
	Dir string
	// Env is git's environment, or nil for this process's.
	Env []string
	// Root is the corpus root, relative to Dir. When it is empty, the
	// corpus root is found as a test finds it: the one the registry of the
	// package in Dir records, or else the module root.
	Root string
	// A and B are the revisions to compare.
	A, B string
}

// A Rev is a revision: the name it was given by, and its commit.
type Rev struct {
	Name, Commit string
}

// A Report is what changed in a corpus root between two revisions.
type Report struct {
	A, B Rev
	// Root is the corpus root, as a slash path from the top of the
	// repository.
	Root string
	// Changes are the cells that changed, in rank order, and within a rank
	// by case, then column.
	Changes []Change
	// Matrices are the corpora at B, by name.
	Matrices []Matrix
}

// A Rank is a class of change to a cell.
type Rank int

const (
	OutcomeChanged Rank = iota // the cell's outcome differs
	Appeared                   // the cell is only at B
	Disappeared                // the cell is only at A
	BytesChanged               // another of its files differs, under the same outcome
)

func (r Rank) String() string {
	return [...]string{"outcome changed", "appeared", "disappeared", "bytes changed"}[r]
}

// A Change is one cell's change.
type Change struct {
	Rank   Rank
	Case   string
	Column string
	// Before and After are the outcome at A and at B, as the outcome file
	// holds it less its newline, or "" where the cell isn't.
	Before, After string
	// Files are the cell's other files that were added, changed or removed,
	// by path.
	Files []FileChange
	// Properties are what the case states, at B, or at A for a cell that
	// disappeared.
	Properties []Property
	// Gap is the text of the gaps line that marks the cell, at the same
	// revision, or nil.
	Gap Text
}

// A FileChange is one file of a cell that changed, by its path under the
// column's directory.
type FileChange struct {
	Path string
	How  string // added, changed or removed
}

// A Property is a property a case states.
type Property struct {
	Name        string
	Description Text
}

// Build reads both revisions and compares them.
func Build(opts Options) (*Report, error) {
	repo, err := gitfs.Open(opts.Dir, opts.Env)
	if err != nil {
		return nil, err
	}
	defer repo.Close()
	var revs [2]Rev
	var trees [2]fs.FS
	for i, name := range []string{opts.A, opts.B} {
		commit, err := repo.Resolve(name)
		if err != nil {
			return nil, err
		}
		revs[i] = Rev{Name: name, Commit: commit}
		if trees[i], err = repo.Tree(commit); err != nil {
			return nil, err
		}
	}
	root, err := corpusRoot(opts.Root, repo.Prefix(), revs, trees)
	if err != nil {
		return nil, err
	}
	var snaps [2]*snapshot
	for i := range revs {
		mod, _ := findModule(trees[i], root)
		if snaps[i], err = load(revs[i], trees[i], mod, root); err != nil {
			return nil, err
		}
	}
	a, b := snaps[0], snaps[1]
	if a.regs == 0 && b.regs == 0 {
		return nil, fmt.Errorf("no registry records the corpus root %s, at %s or at %s.\nName the corpus root with -root, relative to the current directory, or run the report from the directory of a package that registers it", root, a.rev.Name, b.rev.Name)
	}
	links, err := newLinker(b.links)
	if err != nil {
		return nil, fmt.Errorf("the registries at %s: %w", b.rev.Name, err)
	}
	r := &Report{A: a.rev, B: b.rev, Root: root}
	r.Changes = changes(a, b, links)
	r.Matrices = matrices(b, r.Changes, links)
	return r, nil
}

// corpusRoot returns the corpus root, as a slash path from the top of the
// repository. Given one, it is relative to the directory the report runs
// in, whose path from the top is prefix. Without one, it is found as a test
// finds it, at B, or at A when B has no module there: the corpus root that
// the registry of the package in that directory records, or else the
// module root.
func corpusRoot(given, prefix string, revs [2]Rev, trees [2]fs.FS) (string, error) {
	if given != "" {
		if filepath.IsAbs(given) {
			return "", fmt.Errorf("-root %s is an absolute path. Name the corpus root relative to the current directory", given)
		}
		root := path.Join(prefix, filepath.ToSlash(given))
		if root == ".." || strings.HasPrefix(root, "../") {
			return "", fmt.Errorf("-root %s is outside the repository", given)
		}
		return root, nil
	}
	for _, i := range []int{1, 0} {
		mod, ok := findModule(trees[i], prefix)
		if !ok {
			continue
		}
		p := path.Join(prefix, "testdata", registry.Name)
		if _, err := fs.Stat(trees[i], p); errors.Is(err, fs.ErrNotExist) {
			return mod, nil
		}
		data, err := fs.ReadFile(trees[i], p)
		if err != nil {
			return "", fmt.Errorf("reading %s at %s: %w", p, revs[i].Name, err)
		}
		h, err := registry.Peek(data)
		if err != nil {
			return "", unreadable(p, revs[i], err)
		}
		return path.Join(mod, h.CorpusRoot), nil
	}
	return "", fmt.Errorf("no go.mod is in %s or any directory above it, at %s or at %s", prefix, revs[0].Name, revs[1].Name)
}

// findModule returns the module root of a directory in a tree: the nearest
// directory, from dir upward, that holds a go.mod.
func findModule(tree fs.FS, dir string) (string, bool) {
	for d := dir; ; d = path.Dir(d) {
		if fi, err := fs.Stat(tree, path.Join(d, "go.mod")); err == nil && fi.Mode().IsRegular() {
			return d, true
		}
		if d == "." {
			return "", false
		}
	}
}

// changes compares every cell of the two snapshots, and ranks those that
// changed.
func changes(a, b *snapshot, links *linker) []Change {
	keys := map[cellKey]bool{}
	for k := range a.cells {
		keys[k] = true
	}
	for k := range b.cells {
		keys[k] = true
	}
	sorted := make([]cellKey, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	slices.SortFunc(sorted, cellKey.compare)

	var out []Change
	for _, k := range sorted {
		ca, cb := a.cells[k], b.cells[k]
		ch := Change{Case: k.c.String(), Column: k.column}
		at := b
		switch {
		case ca == nil:
			ch.Rank, ch.After = Appeared, cb.outcomeLine()
		case cb == nil:
			ch.Rank, ch.Before = Disappeared, ca.outcomeLine()
			at = a
		case !bytes.Equal(ca.outcome, cb.outcome):
			ch.Rank, ch.Before, ch.After = OutcomeChanged, ca.outcomeLine(), cb.outcomeLine()
		default:
			ch.Rank, ch.Before, ch.After = BytesChanged, ca.outcomeLine(), cb.outcomeLine()
		}
		ch.Files = fileChanges(ca, cb)
		if ch.Rank == BytesChanged && len(ch.Files) == 0 {
			continue
		}
		ch.Properties = at.properties(k.c, links)
		if text, ok := at.gaps[k]; ok {
			ch.Gap = links.text(text)
		}
		out = append(out, ch)
	}
	slices.SortStableFunc(out, func(x, y Change) int { return int(x.Rank) - int(y.Rank) })
	return out
}

// fileChanges returns a cell's files, other than its outcome, that differ
// between two revisions, by path. Either cell may be nil.
func fileChanges(a, b *cellAt) []FileChange {
	var fa, fb map[string][]byte
	if a != nil {
		fa = a.files
	}
	if b != nil {
		fb = b.files
	}
	paths := map[string]bool{}
	for p := range fa {
		paths[p] = true
	}
	for p := range fb {
		paths[p] = true
	}
	var out []FileChange
	for _, p := range sortedKeys(paths) {
		da, inA := fa[p]
		db, inB := fb[p]
		switch {
		case !inA:
			out = append(out, FileChange{Path: p, How: "added"})
		case !inB:
			out = append(out, FileChange{Path: p, How: "removed"})
		case !bytes.Equal(da, db):
			out = append(out, FileChange{Path: p, How: "changed"})
		}
	}
	return out
}

// properties returns what a case states, each with its description.
func (s *snapshot) properties(k caseKey, links *linker) []Property {
	c := s.cases[k]
	if c == nil {
		return nil
	}
	var out []Property
	for _, name := range c.properties {
		out = append(out, Property{Name: name, Description: links.text(s.props[name])})
	}
	return out
}
