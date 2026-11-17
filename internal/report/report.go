// Package report builds hypercase report: what changed in a corpus root
// between two git revisions, read from the repository's object store.
//
// It reads registries and archives, never Go source, and works from the
// cells' committed outcomes alone: a cell exists at a revision when its
// outcome file does. It ranks the changes: changed outcomes first, then
// cells that appeared, then cells that disappeared, then cells whose files
// changed under the same outcome, each with a line diff of its files. It
// shows the matrix of each corpus at the second revision, each case with
// the properties it states and each known gap on the cell it marks, linked
// to its issue; the descriptions of what the registries and archives
// declare; and the problems it found, where it doesn't pick.
package report

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/diff"
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
	// Problems are what the report found that it can't show as it is: at
	// B, then at A.
	Problems []Problem
	// Changes are the cells that changed, in rank order, and within a rank
	// by case, then column.
	Changes []Change
	// Matrices are the corpora at B, by name.
	Matrices []Matrix
	// Corpora describe each corpus at B or at A, by name, and Properties
	// each property, by name.
	Corpora    []Corpus
	Properties []Described
}

// A Problem is what the report found at one revision that it can't show
// as it is. The report shows the problem, and doesn't pick.
type Problem struct {
	At string // the revision's name
	// Where is the file, then the section and line in it, from the top of
	// the repository, or "" for the corpus root as a whole.
	Where string
	Msg   string
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
	// holds it less its newline, Unknown where the cell's state is, or ""
	// where the cell isn't.
	Before, After string
	// Files are the cell's other files that were added, changed or removed,
	// by path, then, for a bytes lift's cell, its product's, by path.
	Files []FileChange
	// Properties are what the case states, at B, or at A for a cell that
	// disappeared.
	Properties []Property
	// Gap is the text of the gaps line that marks the cell, at the same
	// revision, or nil.
	Gap Text
}

// A FileChange is one file of a cell that changed, by its path under the
// column's directory, or, for a file of a bytes lift's product, by its
// path from the case's via/, as via/<lift>/<path>, with a line diff from A
// to B.
type FileChange struct {
	Path string
	How  string // added, changed or removed
	// Diff is the line diff, and Added and Removed count its lines only at
	// B and only at A. Unaligned records that the diff left out a stretch
	// it can't align, so Added and Removed don't count the file's change.
	Diff           []diff.Line
	Added, Removed int
	Unaligned      bool
}

// A Property is a property a case states, with its description at B, or
// at A for a property B lacks. How is "" for what the case's own
// properties section states, and Given or Kept for what a transform's
// variant states.
type Property struct {
	Name        string
	Description Text
	How         string
}

// Build reads both revisions and compares them. It reads every registry
// of the corpus root at both, and refuses one of a format it doesn't read,
// before it reads anything else.
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
	var regs [2][]found
	for i := range revs {
		if mod, ok := findModule(trees[i], root); ok {
			if regs[i], err = readRegistries(revs[i], trees[i], mod, root); err != nil {
				return nil, err
			}
		}
	}
	const advice = "Name the corpus root with -root, relative to the current directory, or run the report from the directory of a package that registers it"
	switch {
	case len(regs[0]) == 0 && len(regs[1]) == 0:
		return nil, fmt.Errorf("no registry records the corpus root %s, at %s or at %s.\n%s", root, revs[0].Name, revs[1].Name, advice)
	case len(regs[1]) == 0:
		return nil, fmt.Errorf("no registry records the corpus root %s at %s, though some do at %s, and the report shows the corpus at its second revision.\n%s", root, revs[1].Name, revs[0].Name, advice)
	}
	var snaps [2]*snapshot
	for i := range revs {
		if snaps[i], err = load(revs[i], trees[i], root, regs[i]); err != nil {
			return nil, err
		}
	}
	a, b := snaps[0], snaps[1]
	links, bad := newLinker(b.links)
	for _, bl := range bad {
		regs := b.linkRegs[bl.pattern]
		verb := "gives"
		if len(regs) > 1 {
			verb = "give"
		}
		b.problem("", fmt.Sprintf("%s %s the issue link pattern %q, which is not a regular expression (%v), so the report links no text through it.", andList(regs), verb, bl.pattern, bl.err))
	}
	r := &Report{A: a.rev, B: b.rev, Root: root}
	r.Problems = append(slices.Clone(b.problems), a.problems...)
	// A property's description is B's, or A's where B lists no property of
	// its name.
	props := maps.Clone(a.props)
	maps.Copy(props, b.props)
	r.Changes = changes(a, b, props, links)
	r.Matrices = matrices(b, r.Changes, props, links)
	r.Corpora, r.Properties = describe(a, b, links)
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
// changed. A cell whose state is unknown at either revision has no outcome
// to compare, so it ranks on its files alone when it is at both, and one
// whose product either revision leaves out has no product to compare.
// props holds each property's description.
func changes(a, b *snapshot, props map[string]string, links *linker) []Change {
	keys := map[cellKey]bool{}
	for k := range a.cells {
		keys[k] = true
	}
	for k := range b.cells {
		keys[k] = true
	}
	sorted := slices.SortedFunc(maps.Keys(keys), cellKey.compare)

	var out []Change
	for _, k := range sorted {
		ca, cb := a.cells[k], b.cells[k]
		ch := Change{Case: k.c.String(), Column: k.column}
		at := b
		switch {
		case ca == nil:
			ch.Rank, ch.After = Appeared, cb.line
		case cb == nil:
			ch.Rank, ch.Before = Disappeared, ca.line
			at = a
		case ca.known && cb.known && ca.line != cb.line:
			ch.Rank, ch.Before, ch.After = OutcomeChanged, ca.line, cb.line
		default:
			ch.Rank, ch.Before, ch.After = BytesChanged, ca.line, cb.line
		}
		ch.Files = fileChanges(ca, cb, k.column, !a.leftOut[k] && !b.leftOut[k])
		if ch.Rank == BytesChanged && len(ch.Files) == 0 {
			continue
		}
		ch.Properties = at.properties(k.c, props, links)
		if text, ok := at.gaps[k]; ok {
			ch.Gap = links.text(text)
		}
		out = append(out, ch)
	}
	slices.SortStableFunc(out, func(x, y Change) int { return int(x.Rank) - int(y.Rank) })
	return out
}

// fileChanges returns a cell's files, other than its outcome, that differ
// between two revisions, by path, then its product's, by path, each with
// its diff, when products says to compare them. Either cell may be nil.
func fileChanges(a, b *cellAt, column string, products bool) []FileChange {
	var fa, fb, pa, pb map[string][]byte
	if a != nil {
		fa, pa = a.files, a.product
	}
	if b != nil {
		fb, pb = b.files, b.product
	}
	if !products {
		pa, pb = nil, nil
	}
	return append(changedFiles(fa, fb, ""), changedFiles(pa, pb, archive.Via+"/"+column+"/")...)
}

// changedFiles returns the files that differ between two revisions' sets
// of a cell's files, by path, each with its diff, and each path after the
// prefix.
func changedFiles(fa, fb map[string][]byte, prefix string) []FileChange {
	paths := map[string]bool{}
	for p := range fa {
		paths[p] = true
	}
	for p := range fb {
		paths[p] = true
	}
	var out []FileChange
	for _, p := range slices.Sorted(maps.Keys(paths)) {
		da, inA := fa[p]
		db, inB := fb[p]
		fc := FileChange{Path: prefix + p}
		switch {
		case !inA:
			fc.How = "added"
		case !inB:
			fc.How = "removed"
		case !bytes.Equal(da, db):
			fc.How = "changed"
		default:
			continue
		}
		fc.Diff, _ = diff.Diff(da, db)
		for _, l := range fc.Diff {
			switch l.Kind {
			case '+':
				fc.Added++
			case '-':
				fc.Removed++
			case diff.Unaligned:
				fc.Unaligned = true
			}
		}
		out = append(out, fc)
	}
	return out
}

// properties returns what a case states, each with its description from
// props.
func (s *snapshot) properties(k caseKey, props map[string]string, links *linker) []Property {
	c := s.cases[k]
	if c == nil {
		return nil
	}
	var out []Property
	for _, st := range c.properties {
		out = append(out, Property{Name: st.name, Description: links.text(props[st.name]), How: st.how})
	}
	return out
}
