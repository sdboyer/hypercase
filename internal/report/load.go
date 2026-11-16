package report

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/registry"
)

// A snapshot is a corpus root as one revision commits it: its registries,
// and the cases and cells of the archives in the corpora they declare.
type snapshot struct {
	rev   Rev
	links []registry.IssueLink
	// columns holds each corpus's columns, by name.
	columns map[string]map[string]string // corpus → column → description
	props   map[string]string            // property → description
	// transforms holds each corpus's transforms, by name.
	transforms map[string]map[string]registry.Transform // corpus → transform → its declaration
	cases      map[caseKey]*caseAt
	cells      map[cellKey]*cellAt
	gaps       map[cellKey]string // the text of the gaps line that marks a cell
	regs       int                // the registries over the corpus root
}

// The kinds of case: the base, which an archive's in/ holds, and its
// variants, hand-written or a transform's.
const (
	base = iota
	variant
	transform
)

// A caseKey names a case: <corpus>/<archive>, then variant/<name> or
// transform/<name> for a variant.
type caseKey struct {
	corpus, archive string
	kind            int
	name            string
}

func (k caseKey) String() string {
	s := k.corpus + "/" + k.archive
	switch k.kind {
	case variant:
		s += "/" + archive.Variant + "/" + k.name
	case transform:
		s += "/" + archive.Transform + "/" + k.name
	}
	return s
}

// compare orders cases by corpus, then archive, then the base before its
// hand-written variants and those before its transforms' variants, each by
// name. So an archive's cases sit together.
func (k caseKey) compare(o caseKey) int {
	return cmp.Or(
		cmp.Compare(k.corpus, o.corpus),
		cmp.Compare(k.archive, o.archive),
		cmp.Compare(k.kind, o.kind),
		cmp.Compare(k.name, o.name),
	)
}

type cellKey struct {
	c      caseKey
	column string
}

func (k cellKey) compare(o cellKey) int {
	return cmp.Or(k.c.compare(o.c), cmp.Compare(k.column, o.column))
}

type caseAt struct {
	description string
	properties  []string
}

// A cellAt is a cell's committed files: its outcome, and the rest.
type cellAt struct {
	outcome []byte // nil when the cell has no outcome file
	files   map[string][]byte
}

// noOutcome is how a cell without an outcome file shows its outcome.
const noOutcome = "(no outcome)"

func (c *cellAt) outcomeLine() string {
	if c.outcome == nil {
		return noOutcome
	}
	return strings.TrimSuffix(string(c.outcome), "\n")
}

func (c *cellAt) state() string {
	if c.outcome == nil {
		return ""
	}
	state, _, _ := strings.Cut(c.outcomeLine(), " ")
	return state
}

// outcomeFile is a cell's outcome, in its column's directory.
const outcomeFile = "outcome"

// load reads the corpus root at a revision: every registry under its
// module root that records it, and every archive in the corpora they
// declare. mod is the module root and root the corpus root, as slash paths
// from the top of the repository; mod is "" when the revision has no
// module there, and holds nothing.
func load(rev Rev, tree fs.FS, mod, root string) (*snapshot, error) {
	s := &snapshot{
		rev:        rev,
		columns:    map[string]map[string]string{},
		props:      map[string]string{},
		transforms: map[string]map[string]registry.Transform{},
		cases:      map[caseKey]*caseAt{},
		cells:      map[cellKey]*cellAt{},
		gaps:       map[cellKey]string{},
	}
	if mod == "" {
		return s, nil
	}
	sub, err := fs.Sub(tree, mod)
	if err != nil {
		return nil, err
	}
	pkgs, err := registry.FindFS(sub)
	if err != nil {
		return nil, fmt.Errorf("finding the registries at %s: %w", rev.Name, err)
	}
	rel := relTo(mod, root)
	for _, pkg := range pkgs {
		p := path.Join(mod, pkg, "testdata", registry.Name)
		g, err := readRegistry(tree, p, rel, rev)
		if err != nil {
			return nil, err
		}
		if g == nil {
			continue
		}
		s.regs++
		s.links = append(s.links, g.IssueLinks...)
		for _, p := range g.Properties {
			if _, ok := s.props[p.Name]; !ok {
				s.props[p.Name] = p.Description
			}
		}
		for _, ty := range g.Types {
			if s.transforms[ty.Dir] == nil {
				s.transforms[ty.Dir] = map[string]registry.Transform{}
			}
			for _, tr := range ty.Transforms {
				if _, ok := s.transforms[ty.Dir][tr.Name]; !ok {
					s.transforms[ty.Dir][tr.Name] = tr
				}
			}
		}
		addColumn := func(name, description string, corpora []string) {
			for _, corpus := range corpora {
				if s.columns[corpus] == nil {
					s.columns[corpus] = map[string]string{}
				}
				s.columns[corpus][name] = description
			}
		}
		for _, suite := range g.Suites {
			addColumn(suite.Name, suite.Description, suite.Corpora)
		}
		for _, l := range g.Lifts {
			if l.Owned {
				addColumn(l.Name, l.Description, l.Corpora)
			}
		}
	}
	for _, corpus := range slices.Sorted(maps.Keys(s.columns)) {
		dir := path.Join(root, "testdata", corpus)
		entries, err := fs.ReadDir(tree, dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue // a declared directory that doesn't exist yet
		}
		if err != nil {
			return nil, fmt.Errorf("reading %s at %s: %w", dir, rev.Name, err)
		}
		for _, e := range entries {
			name, ok := strings.CutSuffix(e.Name(), ".txtar")
			if !ok || e.IsDir() {
				continue
			}
			p := path.Join(dir, e.Name())
			if !e.Type().IsRegular() {
				return nil, fmt.Errorf("%s at %s is not a regular file, and an archive is one", p, rev.Name)
			}
			data, err := fs.ReadFile(tree, p)
			if err != nil {
				return nil, fmt.Errorf("reading %s at %s: %w", p, rev.Name, err)
			}
			if err := s.addArchive(corpus, name, data); err != nil {
				return nil, fmt.Errorf("%s at %s: %w", p, rev.Name, err)
			}
		}
	}
	return s, nil
}

// readRegistry reads a registry of the corpus root rel, and refuses one of
// another format, saying which hypercase reads it. Another corpus root's
// registry it leaves unread, whatever its format, and returns nil.
func readRegistry(tree fs.FS, p, rel string, rev Rev) (*registry.Registry, error) {
	data, err := fs.ReadFile(tree, p)
	if err != nil {
		return nil, fmt.Errorf("reading %s at %s: %w", p, rev.Name, err)
	}
	if h, err := registry.Peek(data); err == nil && h.CorpusRoot != rel {
		return nil, nil
	}
	g, err := registry.Parse(data)
	if err != nil {
		return nil, unreadable(p, rev, err)
	}
	return g, nil
}

// unreadable is the error of a registry that can't be read: one of
// another format says which hypercase reads it.
func unreadable(p string, rev Rev, err error) error {
	var fe *registry.FormatError
	if errors.As(err, &fe) {
		return fmt.Errorf("%s at %s holds registry format %d, and this hypercase reads format %d.\n"+
			"Run the report from %s's hypercase, which reads it: in a checkout of %s, run the same command as go run github.com/sdboyer/hypercase/cmd/hypercase.",
			p, rev.Name, fe.Format, registry.Format, rev.Name, rev.Name)
	}
	return fmt.Errorf("%s at %s can't be read: %w", p, rev.Name, err)
}

// addArchive reads one archive's cases and cells. It reads the sections
// that say what a case is and what its cells hold, with the archive
// package's parsers, and fails on one of them whose form it can't read.
func (s *snapshot) addArchive(corpus, name string, data []byte) error {
	a := archive.Parse(data)
	baseKey := caseKey{corpus: corpus, archive: name, kind: base}
	s.cases[baseKey] = &caseAt{description: strings.TrimSpace(string(a.Comment))}
	seen := map[string]bool{}
	for _, sec := range a.Sections {
		if seen[sec.Name] {
			return problem(sec.Name, 0, "the section appears twice")
		}
		seen[sec.Name] = true
		kind, vname, rest := archive.SplitCase(sec.Name)
		k := baseKey
		if kind != "" {
			if vname == "" || rest == "" {
				return problem(sec.Name, 0, fmt.Sprintf("a variant's section belongs under its name, as %s/<name>/<section>", kind))
			}
			k = caseKey{corpus: corpus, archive: name, kind: variant, name: vname}
			if kind == archive.Transform {
				k.kind = transform
			}
			if s.cases[k] == nil {
				s.cases[k] = &caseAt{}
			}
		}
		top, file, _ := strings.Cut(rest, "/")
		switch {
		case top == archive.Out:
			if err := s.addCellFile(k, sec.Name, file, sec.Data); err != nil {
				return err
			}
		case kind == archive.Transform:
			// A transform's inputs; its description is in the registry.
		case kind == archive.Variant && rest == archive.About:
			s.cases[k].description = strings.TrimSpace(string(sec.Data))
		case rest == archive.Properties:
			names, ps := archive.ParsePropertiesIn(sec.Name, sec.Data)
			if len(ps) > 0 {
				return errors.New(ps[0].String())
			}
			s.cases[k].properties = names
		case rest == archive.Gaps:
			gaps, ps := archive.ParseGapsIn(sec.Name, sec.Data)
			if len(ps) > 0 {
				return errors.New(ps[0].String())
			}
			for _, g := range gaps {
				ck := cellKey{c: k, column: g.Column}
				if g.Transform != "" {
					// The line is the base's, and marks a cell of the
					// transform's variant, which is shown even where the
					// variant has no sections.
					ck.c = caseKey{corpus: corpus, archive: name, kind: transform, name: g.Transform}
					if s.cases[ck.c] == nil {
						s.cases[ck.c] = &caseAt{}
					}
				}
				s.gaps[ck] = g.Text
			}
		}
	}
	for k, c := range s.cases {
		if k.kind == transform && k.corpus == corpus && k.archive == name {
			c.properties = s.transformProperties(corpus, k.name, s.cases[baseKey].properties)
		}
	}
	return nil
}

// transformProperties returns what a transform's variant states, as the
// registries declare the transform: what it gives, then each property it
// keeps that the base states, in the base's order. A transform no
// registry declares gives its variant none.
func (s *snapshot) transformProperties(corpus, name string, base []string) []string {
	tr, ok := s.transforms[corpus][name]
	if !ok {
		return nil
	}
	props := slices.Clone(tr.Gives)
	for _, p := range base {
		if slices.Contains(tr.Keeps, p) && !slices.Contains(props, p) {
			props = append(props, p)
		}
	}
	return props
}

// addCellFile reads one file of a cell: rest is its path under out/, as
// <column>/<file>.
func (s *snapshot) addCellFile(k caseKey, section, rest string, data []byte) error {
	col, file, _ := strings.Cut(rest, "/")
	if col == "" || file == "" || !fs.ValidPath(rest) {
		return problem(section, 0, "an output belongs under its column, as out/<column>/<file>")
	}
	ck := cellKey{c: k, column: col}
	c := s.cells[ck]
	if c == nil {
		c = &cellAt{files: map[string][]byte{}}
		s.cells[ck] = c
	}
	if file == outcomeFile {
		if err := checkOutcome(data); err != nil {
			return problem(section, 0, err.Error())
		}
		c.outcome = data
		return nil
	}
	c.files[file] = data
	return nil
}

// states are an outcome's states, its first word.
var states = []string{"accepted", "rejected", "panicked", "blocked", "skipped"}

// checkOutcome checks an outcome file is one the report can show: one
// line, ending with a newline, whose first word is a state. panicked
// stands alone, and blocked and skipped say what blocked or why.
func checkOutcome(data []byte) error {
	line, ok := bytes.CutSuffix(data, []byte("\n"))
	if !ok || bytes.IndexByte(line, '\n') >= 0 {
		return errors.New("an outcome is one line, ending with a newline")
	}
	state, detail, _ := strings.Cut(string(line), " ")
	switch {
	case !slices.Contains(states, state):
		return fmt.Errorf("an outcome starts with its state, one of %s, not %q", strings.Join(states, ", "), state)
	case state == "panicked" && detail != "":
		return errors.New("panicked stands alone")
	case state == "blocked" && detail == "":
		return errors.New("blocked is followed by the lift that failed")
	case state == "skipped" && detail == "":
		return errors.New("skipped is followed by the probe's reason")
	}
	return nil
}

func problem(section string, line int, msg string) error {
	return errors.New(archive.Problem{Section: section, Line: line, Msg: msg}.String())
}

// relTo returns p relative to dir, an ancestor of p or p itself.
func relTo(dir, p string) string {
	switch {
	case dir == p:
		return "."
	case dir == ".":
		return p
	}
	return strings.TrimPrefix(p, dir+"/")
}
