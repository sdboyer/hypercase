package report

import (
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
	"github.com/sdboyer/hypercase/internal/revmap"
)

// A snapshot is a corpus root as one revision commits it: its registries,
// and the cases and cells of the archives in the corpora they declare.
type snapshot struct {
	rev   Rev
	root  string // the corpus root, as a slash path from the top of the repository
	links []registry.IssueLink
	// linkRegs holds the registries that give each link pattern, by path.
	linkRegs map[string][]string
	// format is the newest format version of the registries, outcomes
	// whether the cells commit outcome files, as they do from
	// registry.OutcomeFormat on, and ranges whether they commit inputs
	// files, as they do from registry.InputsFormat on. Before the one, a
	// cell's state is unknown, and before the other, its ranges are.
	format   int
	outcomes bool
	ranges   bool
	// types holds the description of each corpus type, by its directory.
	types map[string]string
	// columns holds each column the registries claim, by name, which is
	// unique per corpus root.
	columns map[string]*column
	props   map[string]string // property → description
	// transforms holds each corpus's transforms, by name.
	transforms map[string]map[string]registry.Transform // corpus → transform → its declaration
	// owned holds the transforms over each corpus that some registry owns,
	// whose package writes their variants' own inputs.
	owned map[string]map[string]bool
	// bytesLifts holds the names of the bytes lifts any registry declares
	// over each corpus, whose cells commit a product under via/<lift>/.
	bytesLifts map[string]map[string]bool
	cases      map[caseKey]*caseAt
	cells      map[cellKey]*cellAt
	gaps       map[cellKey]string // the text of the gaps line that marks a cell
	// sections holds each archive's sections, by their whole names, which
	// a range names.
	sections map[archiveKey]map[string][]byte
	problems []Problem
	// leftOut holds the cells whose product the report leaves out: those
	// with a via/<lift>/ section whose lift no registry declares as a
	// bytes lift over the corpus.
	leftOut map[cellKey]bool
}

// An archiveKey names an archive: its corpus, and its name.
type archiveKey struct {
	corpus, archive string
}

// A column is one that the registries claim: a suite, or a lift its
// package owns.
type column struct {
	description string
	// outputs are its output declarations, and a bytes lift's product
	// declarations, each as via/<lift>/<pattern>.
	outputs    []registry.Output
	classifier *registry.Classifier
	corpora    map[string]bool
	// registries are the registries that claim the column, by path. When
	// two do, the column is in conflict, and the report shows neither one's
	// description, outputs or classifier.
	registries []string
	conflict   bool
}

// column returns the column of the name that the registries claim over
// the corpus, if they do.
func (s *snapshot) column(corpus, name string) (*column, bool) {
	c, ok := s.columns[name]
	if !ok || !c.corpora[corpus] {
		return nil, false
	}
	return c, true
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
	return k.corpus + "/" + k.archive + strings.TrimSuffix("/"+k.prefix(), "/")
}

// prefix is the prefix of the case's sections in its archive:
// variant/<name>/ or transform/<name>/, and "" for the base.
func (k caseKey) prefix() string {
	switch k.kind {
	case variant:
		return archive.Variant + "/" + k.name + "/"
	case transform:
		return archive.Transform + "/" + k.name + "/"
	}
	return ""
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
	path        string // the archive, from the top of the repository
	description string
	properties  []stated
}

// stated is a property a case states. how is "" for one its own section
// states, and Given or Kept for a transform's variant's.
type stated struct {
	name, how string
}

// How a transform's variant comes to state a property.
const (
	Given = "given" // the transform gives it
	Kept  = "kept"  // the transform keeps it, and the base states it
)

// A cellAt is a cell's committed files: its outcome, its inputs file, the
// rest of its files under out/<column>/, and, for a bytes lift's cell, its
// product under via/<column>/. A cell exists at a revision when its
// outcome file does. Before registry.OutcomeFormat, every cell of a case
// that runs under a column the registries claim exists, and so does every
// cell with files, whether or not its case runs or a registry claims its
// column; and its state is unknown.
type cellAt struct {
	known   bool // whether its state is: whether it has an outcome
	outcome archive.Outcome
	line    string            // the outcome file, less its newline, or Unknown
	files   map[string][]byte // by path under out/<column>/, less outcome and inputs
	product map[string][]byte // by path under via/<column>/
	// inputs is the cell's inputs file, or nil where it has none, or its
	// revision predates registry.InputsFormat and the report leaves it out.
	inputs *inputsAt
}

// An inputsAt is a cell's inputs file: its bytes, and the reverse maps it
// holds, by the file each maps, a product's file as via/<column>/<path>.
// maps is nil where the report shows none of them: where the file doesn't
// parse, or the cell didn't run.
type inputsAt struct {
	data []byte
	maps map[string]revmap.File
}

// ran reports whether the cell ran: whether it is accepted, rejected or
// panicked, and so has an inputs file from registry.InputsFormat on.
func (c *cellAt) ran() bool {
	return c.known && c.outcome.State != archive.Skipped && c.outcome.State != archive.Blocked
}

// mapOf returns the reverse map of one of the cell's files, by its path
// under out/<column>/, or as via/<column>/<path> for its product's, or nil
// where the cell's inputs file gives none: where the cell has none, the
// report shows none of its maps, or it doesn't map the file. The cell may
// be nil.
func (c *cellAt) mapOf(path string) *revmap.File {
	if c == nil || c.inputs == nil {
		return nil
	}
	f, ok := c.inputs.maps[path]
	if !ok {
		return nil
	}
	return &f
}

// Unknown is the outcome of a cell whose state is unknown: at a revision
// whose registries predate registry.OutcomeFormat. It is no outcome a file
// can hold, whose first word is a state.
const Unknown = "unknown"

// A found registry is one of the corpus root's, as a revision commits it.
type found struct {
	path string // from the top of the repository
	reg  *registry.Registry
}

// readRegistries reads the registries of a revision's module that record
// the corpus root, which a snapshot is loaded from. It reads each of a
// format from registry.Oldest to registry.Format, and refuses another,
// saying which hypercase reads it. Another corpus root's registry it
// leaves unread, whatever its format. mod is the module root and root the
// corpus root, as slash paths from the top of the repository.
func readRegistries(rev Rev, tree fs.FS, mod, root string) ([]found, error) {
	sub, err := fs.Sub(tree, mod)
	if err != nil {
		return nil, err
	}
	pkgs, err := registry.FindFS(sub)
	if err != nil {
		return nil, fmt.Errorf("finding the registries at %s: %w", rev.Name, err)
	}
	rel := relTo(mod, root)
	var out []found
	for _, pkg := range pkgs {
		p := path.Join(mod, pkg, "testdata", registry.Name)
		data, err := fs.ReadFile(tree, p)
		if err != nil {
			return nil, fmt.Errorf("reading %s at %s: %w", p, rev.Name, err)
		}
		if h, err := registry.Peek(data); err == nil && h.CorpusRoot != rel {
			continue
		}
		g, err := registry.ParseAny(data)
		if err != nil {
			return nil, unreadable(p, rev, err)
		}
		out = append(out, found{path: p, reg: g})
	}
	return out, nil
}

// unreadable is the error of a registry that can't be read. One of a newer
// format says which hypercase reads it, and one older than any report
// reads says so.
func unreadable(p string, rev Rev, err error) error {
	var fe *registry.FormatError
	switch {
	case errors.As(err, &fe) && fe.Format < registry.Oldest:
		return fmt.Errorf("%s at %s holds registry format %d, which no hypercase's report reads: the oldest format a report reads is %d.",
			p, rev.Name, fe.Format, registry.Oldest)
	case errors.As(err, &fe):
		return fmt.Errorf("%s at %s holds registry format %d, and this hypercase reads formats %d to %d.\n"+
			"Run the report from %s's hypercase, which reads it: in a checkout of %s, run the same command as go run github.com/sdboyer/hypercase/cmd/hypercase.",
			p, rev.Name, fe.Format, registry.Oldest, registry.Format, rev.Name, rev.Name)
	}
	return fmt.Errorf("%s at %s can't be read: %w", p, rev.Name, err)
}

// load reads the corpus root at a revision, from the registries that
// record it: every archive in the corpora they declare. root is the corpus
// root, as a slash path from the top of the repository.
func load(rev Rev, tree fs.FS, root string, regs []found) (*snapshot, error) {
	s := &snapshot{
		rev:        rev,
		root:       root,
		linkRegs:   map[string][]string{},
		types:      map[string]string{},
		columns:    map[string]*column{},
		props:      map[string]string{},
		transforms: map[string]map[string]registry.Transform{},
		owned:      map[string]map[string]bool{},
		bytesLifts: map[string]map[string]bool{},
		leftOut:    map[cellKey]bool{},
		cases:      map[caseKey]*caseAt{},
		cells:      map[cellKey]*cellAt{},
		gaps:       map[cellKey]string{},
		sections:   map[archiveKey]map[string][]byte{},
	}
	for _, f := range regs {
		s.format = max(s.format, f.reg.Format)
	}
	s.outcomes = s.format >= registry.OutcomeFormat
	s.ranges = s.format >= registry.InputsFormat
	declared := map[string]bool{} // the corpora's directories
	for _, f := range regs {
		g := f.reg
		s.links = append(s.links, g.IssueLinks...)
		for _, l := range g.IssueLinks {
			if !slices.Contains(s.linkRegs[l.Pattern], f.path) {
				s.linkRegs[l.Pattern] = append(s.linkRegs[l.Pattern], f.path)
			}
		}
		for _, p := range g.Properties {
			if _, ok := s.props[p.Name]; !ok {
				s.props[p.Name] = p.Description
			}
		}
		for _, ty := range g.Types {
			declared[ty.Dir] = true
			if _, ok := s.types[ty.Dir]; !ok {
				s.types[ty.Dir] = ty.Description
			}
			if s.transforms[ty.Dir] == nil {
				s.transforms[ty.Dir] = map[string]registry.Transform{}
				s.owned[ty.Dir] = map[string]bool{}
			}
			for _, tr := range ty.Transforms {
				if _, ok := s.transforms[ty.Dir][tr.Name]; !ok {
					s.transforms[ty.Dir][tr.Name] = tr
				}
				s.owned[ty.Dir][tr.Name] = s.owned[ty.Dir][tr.Name] || tr.Owned
			}
		}
		claim := func(name, description string, outputs []registry.Output, classifier *registry.Classifier, corpora []string) {
			c := s.columns[name]
			if c == nil {
				c = &column{description: description, outputs: outputs, classifier: classifier, corpora: map[string]bool{}}
				s.columns[name] = c
			}
			if !slices.Contains(c.registries, f.path) {
				c.registries = append(c.registries, f.path)
			}
			for _, corpus := range corpora {
				declared[corpus] = true
				c.corpora[corpus] = true
			}
		}
		for _, suite := range g.Suites {
			claim(suite.Name, suite.Description, suite.Outputs, suite.Classifier, suite.Corpora)
		}
		for _, l := range g.Lifts {
			if l.Bytes {
				for _, corpus := range l.Corpora {
					if s.bytesLifts[corpus] == nil {
						s.bytesLifts[corpus] = map[string]bool{}
					}
					s.bytesLifts[corpus][l.Name] = true
				}
			}
			if l.Owned {
				// A bytes lift's product declarations are described with
				// its outputs, each named via/<lift>/<pattern>, as its
				// product's files are shown.
				outputs := slices.Clone(l.Outputs)
				for _, o := range l.ProductFiles {
					outputs = append(outputs, registry.Output{Pattern: archive.Via + "/" + l.Name + "/" + o.Pattern, Description: o.Description})
				}
				claim(l.Name, l.Description, outputs, nil, l.Corpora)
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(s.columns)) {
		if c := s.columns[name]; len(c.registries) > 1 {
			s.problem("", fmt.Sprintf("%s each claim the column %s, and a column's name is unique per corpus root, so the report shows neither one's description of it.", andList(c.registries), name))
			c.description, c.outputs, c.classifier, c.conflict = "", nil, nil, true
		}
	}
	for _, corpus := range slices.Sorted(maps.Keys(declared)) {
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
			if err := s.addArchive(corpus, name, p, data); err != nil {
				return nil, fmt.Errorf("%s at %s: %w", p, rev.Name, err)
			}
		}
	}
	s.unclaimedColumns()
	return s, nil
}

// addArchive reads the cases and cells of the archive at p. It places each
// section by its name: a case's inputs and its other hand-written
// sections, and a transform's in/ and delete, belong to the case, its
// out/<column>/ sections to its cells, and its via/<lift>/ sections, a
// bytes lift's product, to the lift's cell. A section it can't place is a
// problem. From registry.OutcomeFormat on, so is a cell with no outcome
// file, which doesn't exist; before it, an outcome file is, and the case's
// cells are those of [cellAt]. It reads what a case states and what a
// cell's outcome is with the archive package's parsers, and fails on a
// section of those it can't read.
func (s *snapshot) addArchive(corpus, name, p string, data []byte) error {
	a := archive.Parse(data)
	baseKey := caseKey{corpus: corpus, archive: name, kind: base}
	s.cases[baseKey] = &caseAt{path: p, description: strings.TrimSpace(string(a.Comment))}
	var transforms []caseKey           // the archive's transforms' variants
	withSections := map[caseKey]bool{} // the cases a section is placed in
	var cells []cellKey                // the archive's cells, in the order met
	caseOf := func(k caseKey) *caseAt {
		if s.cases[k] == nil {
			s.cases[k] = &caseAt{path: p}
			if k.kind == transform {
				transforms = append(transforms, k)
			}
		}
		return s.cases[k]
	}
	place := func(k caseKey) *caseAt {
		withSections[k] = true
		return caseOf(k)
	}
	cantPlace := func(section string) {
		s.problem(p+", "+section, "the report can't place the section in a case or a cell, and leaves it out.")
	}
	cellOf := func(ck cellKey) *cellAt {
		c := s.cells[ck]
		if c == nil {
			c = &cellAt{line: Unknown, files: map[string][]byte{}, product: map[string][]byte{}}
			s.cells[ck] = c
			cells = append(cells, ck)
		}
		return c
	}
	sections := map[string][]byte{}
	s.sections[archiveKey{corpus: corpus, archive: name}] = sections
	for _, sec := range a.Sections {
		if _, seen := sections[sec.Name]; seen {
			return problem(sec.Name, 0, "the section appears twice")
		}
		sections[sec.Name] = sec.Data
		kind, vname, rest := archive.SplitCase(sec.Name)
		k := baseKey
		if kind != "" {
			if vname == "" || rest == "" {
				cantPlace(sec.Name)
				continue
			}
			k = caseKey{corpus: corpus, archive: name, kind: variant, name: vname}
			if kind == archive.Transform {
				k.kind = transform
			}
		}
		top, below, _ := strings.Cut(rest, "/")
		switch {
		case top == archive.Out:
			col, file, _ := strings.Cut(below, "/")
			if col == "" || file == "" || !fs.ValidPath(below) {
				cantPlace(sec.Name)
				continue
			}
			if file == archive.OutcomeFile && !s.outcomes {
				s.problem(p+", "+sec.Name, fmt.Sprintf("the section is an outcome file, and registry format %d predates outcome files, so the report leaves it out.", s.format))
				continue
			}
			if file == revmap.Name && !s.ranges {
				s.problem(p+", "+sec.Name, fmt.Sprintf("the section is an inputs file, and registry format %d predates inputs files, so the report leaves it out.", s.format))
				continue
			}
			place(k)
			c := cellOf(cellKey{c: k, column: col})
			switch file {
			case revmap.Name:
				c.inputs = &inputsAt{data: sec.Data}
				parsed, err := revmap.Parse(sec.Data)
				if err != nil {
					s.problem(p+", "+sec.Name, fmt.Sprintf("the inputs file can't be read: %v. So the report shows no ranges for the cell's files.", err))
					continue
				}
				c.inputs.maps = map[string]revmap.File{}
				for _, f := range parsed {
					c.inputs.maps[f.Path] = f
				}
				continue
			case archive.OutcomeFile:
			default:
				c.files[file] = sec.Data
				continue
			}
			o, err := archive.ParseOutcome(sec.Data)
			if err != nil {
				return problem(sec.Name, 0, err.Error())
			}
			c.known, c.outcome, c.line = true, o, strings.TrimSuffix(string(sec.Data), "\n")
		case top == archive.Via:
			// A file of a bytes lift's product, which belongs to the lift's
			// cell.
			lift, file, _ := strings.Cut(below, "/")
			if lift == "" || file == "" || !fs.ValidPath(below) {
				cantPlace(sec.Name)
				continue
			}
			if !s.bytesLifts[corpus][lift] {
				s.problem(p+", "+sec.Name, fmt.Sprintf("the section is a product of %s, and no registry declares a bytes lift of that name over testdata/%s/, so the report leaves it out, and doesn't compare its cell's product.", lift, corpus))
				s.leftOut[cellKey{c: k, column: lift}] = true
				continue
			}
			place(k)
			cellOf(cellKey{c: k, column: lift}).product[file] = sec.Data
		case top == archive.In && below != "", kind != "" && rest == archive.Delete:
			// An input, or a variant's deleted inputs, which the report
			// doesn't read.
			place(k)
		case kind == archive.Variant && rest == archive.About:
			place(k).description = strings.TrimSpace(string(sec.Data))
		case kind != archive.Transform && rest == archive.Properties:
			names, ps := archive.ParsePropertiesIn(sec.Name, sec.Data)
			if len(ps) > 0 {
				return errors.New(ps[0].String())
			}
			c := place(k)
			for i, n := range names {
				if _, ok := s.props[n]; !ok {
					s.problem(fmt.Sprintf("%s, %s, line %d", p, sec.Name, i+1), fmt.Sprintf("the case states %s, and no registry lists a property of that name.", n))
				}
				c.properties = append(c.properties, stated{name: n})
			}
		case kind != archive.Transform && rest == archive.Gaps:
			gaps, ps := archive.ParseGapsIn(sec.Name, sec.Data)
			if len(ps) > 0 {
				return errors.New(ps[0].String())
			}
			place(k)
			for _, g := range gaps {
				where := fmt.Sprintf("%s, %s, line %d", p, sec.Name, g.Line)
				ck := cellKey{c: k, column: g.Column}
				if g.Transform != "" {
					// The line is the base's, and marks a cell of the
					// transform's variant, which is shown even where the
					// variant has no sections.
					ck.c = caseKey{corpus: corpus, archive: name, kind: transform, name: g.Transform}
					caseOf(ck.c)
					if _, ok := s.transforms[corpus][g.Transform]; !ok {
						s.problem(where, fmt.Sprintf("the line names transform/%s, and no corpus type of testdata/%s/ lists a transform of that name.", g.Transform, corpus))
					}
				}
				if _, ok := s.column(corpus, g.Column); !ok {
					s.problem(where, fmt.Sprintf("the line names the column %s, and no registry claims a column of that name over testdata/%s/.", g.Column, corpus))
				}
				s.gaps[ck] = g.Text
			}
		default:
			cantPlace(sec.Name)
		}
	}
	if s.outcomes {
		for _, ck := range cells {
			c := s.cells[ck]
			if c.known {
				continue
			}
			// The cell's files are under out/<column>/, and a bytes lift's
			// product under via/<column>/.
			var dirs []string
			has := "files"
			if len(c.files) > 0 || c.inputs != nil {
				dirs = append(dirs, ck.c.prefix()+archive.Out+"/"+ck.column+"/")
			}
			if len(c.product) > 0 {
				dirs = append(dirs, ck.c.prefix()+archive.Via+"/"+ck.column+"/")
				has = "files and its product,"
				if len(c.files) == 0 && c.inputs == nil {
					has = "its product"
				}
			}
			s.problem(p+", "+strings.Join(dirs, " and "), "the cell has "+has+" and no outcome file. A cell exists where its outcome file does, so the report shows no cell here.")
			delete(s.cells, ck)
		}
		if s.ranges {
			for _, ck := range cells {
				if c := s.cells[ck]; c != nil {
					s.checkInputs(p, ck, c)
				}
			}
		}
	} else {
		// Before outcome files, a case runs under every column the
		// registries claim over its corpus, and a cell that wrote nothing
		// committed nothing: so each of its cells exists, its state
		// unknown. A case runs when the archive holds a section of it: the
		// base always, and a transform's variant only when its corpus type
		// lists the transform, which may decline a base.
		runs := map[caseKey]bool{baseKey: true}
		for k := range withSections {
			if _, listed := s.transforms[corpus][k.name]; k.kind != transform || listed {
				runs[k] = true
			}
		}
		for k := range runs {
			for col := range s.columnsOver(corpus) {
				if ck := (cellKey{c: k, column: col}); s.cells[ck] == nil {
					s.cells[ck] = &cellAt{line: Unknown, files: map[string][]byte{}}
				}
			}
		}
	}
	var baseStates []string
	for _, st := range s.cases[baseKey].properties {
		baseStates = append(baseStates, st.name)
	}
	for _, k := range transforms {
		tr, ok := s.transforms[corpus][k.name]
		if !ok {
			if withSections[k] {
				s.problem(p+", "+k.prefix(), fmt.Sprintf("no corpus type of testdata/%s/ lists the transform %s, so the report can't say what its variant states.", corpus, k.name))
			}
			continue
		}
		names, _ := registry.VariantProperties(tr.Gives, tr.Keeps, baseStates)
		for _, n := range names {
			how := Kept
			if slices.Contains(tr.Gives, n) {
				how = Given
			}
			s.cases[k].properties = append(s.cases[k].properties, stated{name: n, how: how})
		}
	}
	return nil
}

// checkInputs records a problem for each way a cell's inputs file
// disagrees with the cell, at a revision whose cells commit inputs files:
// a cell that ran and has none; one beside a cell that didn't run, whose
// maps the report shows none of; and, in one the report reads, each file
// it maps that the cell lacks, each of the cell's files it doesn't map,
// and each range that names a section the archive doesn't hold, or lines
// past its last. A product's files are mapped as via/<column>/<path>,
// except where the report leaves the product out, whose files it doesn't
// compare. p is the archive's path.
func (s *snapshot) checkInputs(p string, ck cellKey, c *cellAt) {
	dir := p + ", " + ck.c.prefix() + archive.Out + "/" + ck.column + "/"
	switch {
	case c.inputs == nil && c.ran():
		s.problem(dir, fmt.Sprintf("the cell is %s, and has no inputs file, which every cell that ran has from registry format %d on. So the report shows no ranges for its files.", c.outcome.State, registry.InputsFormat))
		return
	case c.inputs == nil:
		return
	case !c.ran():
		s.problem(dir+revmap.Name, fmt.Sprintf("the cell is %s, and a cell that didn't run has no inputs file, so the report shows no ranges from it.", c.outcome.State))
		c.inputs.maps = nil
		return
	case c.inputs.maps == nil:
		return // it doesn't parse, which is a problem already
	}
	via := archive.Via + "/" + ck.column + "/"
	files := map[string]bool{}
	for f := range c.files {
		files[f] = true
	}
	for f := range c.product {
		files[via+f] = true
	}
	sections := s.sections[archiveKey{corpus: ck.c.corpus, archive: ck.c.archive}]
	for _, f := range slices.Sorted(maps.Keys(c.inputs.maps)) {
		if !files[f] && !(s.leftOut[ck] && strings.HasPrefix(f, via)) {
			s.problem(dir+revmap.Name, fmt.Sprintf("the inputs file maps %s, and the cell has no such file.", f))
		}
		for _, r := range c.inputs.maps[f].Ranges {
			if why := badRange(r, sections); why != "" {
				s.problem(dir+revmap.Name, fmt.Sprintf("the inputs file maps %s to %s, and %s, so the report shows no lines of the range.", f, r, why))
			}
		}
	}
	for _, f := range slices.Sorted(maps.Keys(files)) {
		if _, ok := c.inputs.maps[f]; !ok {
			s.problem(dir+revmap.Name, fmt.Sprintf("the inputs file doesn't map %s, a file of the cell, so the report shows no ranges for it.", f))
		}
	}
}

// unclaimedColumns records a problem for each column that has committed
// cells over a corpus, and that no registry claims over it.
func (s *snapshot) unclaimedColumns() {
	count := map[[2]string]int{} // corpus, column → cells
	for k := range s.cells {
		if _, ok := s.column(k.c.corpus, k.column); !ok {
			count[[2]string{k.c.corpus, k.column}]++
		}
	}
	keys := slices.SortedFunc(maps.Keys(count), func(a, b [2]string) int {
		return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1]))
	})
	for _, k := range keys {
		cells := "1 cell"
		if n := count[k]; n > 1 {
			cells = fmt.Sprintf("%d cells", n)
		}
		s.problem(path.Join(s.root, "testdata", k[0])+"/", fmt.Sprintf("the directory holds %s of the column %s, and no registry claims a column of that name over it.", cells, k[1]))
	}
}

func (s *snapshot) problem(where, msg string) {
	s.problems = append(s.problems, Problem{At: s.rev.Name, Where: where, Msg: msg})
}

func problem(section string, line int, msg string) error {
	return errors.New(archive.Problem{Section: section, Line: line, Msg: msg}.String())
}

// andList joins names as a list in prose: a, b and c.
func andList(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
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
