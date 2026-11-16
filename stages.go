package hypercase

import (
	"fmt"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/erase"
	"github.com/sdboyer/jenny"
)

// A plan is how the package's columns reach the types they consume from
// one corpus type: along one path each, so each type reached on the way
// has one lift that produces it. The plan of a directory a suite's Corpus
// names has fs.FS as its root's type, and no lift.
type plan struct {
	root     *corpusType
	producer map[reflect.Type]*lift
	cols     []*column // the columns over the corpus type, by name
}

// planPaths finds, from each corpus type, the one path to each type a
// column or a lift consumes. It fails the run, before any case runs, on a
// type reached by more than one path, a cycle counting as more, and on a
// type nothing reaches.
func (r *runner) planPaths(t *testing.T) bool {
	t.Helper()
	ok := true
	types := slices.Clone(r.types)
	slices.SortFunc(types, func(a, b *corpusType) int { return strings.Compare(a.dir, b.dir) })
	lifts := slices.Clone(r.lifts)
	slices.SortFunc(lifts, func(a, b *lift) int { return strings.Compare(a.name, b.name) })

	// What consumes each type: the package's columns, and the lifts it uses.
	consumers := map[reflect.Type][]string{}
	var consumed []reflect.Type
	addConsumer := func(typ reflect.Type, who string) {
		if _, seen := consumers[typ]; !seen {
			consumed = append(consumed, typ)
		}
		consumers[typ] = append(consumers[typ], who)
	}
	for _, c := range r.columns {
		if c.corpus == "" {
			addConsumer(c.consumes, "the "+c.kind()+" "+c.name)
		}
	}
	for _, l := range lifts {
		if !l.owned {
			addConsumer(l.in, "the lift "+l.name)
		}
	}
	consume := func(typ reflect.Type) string {
		if who := consumers[typ]; len(who) > 1 {
			return andList(who) + " consume"
		}
		return consumers[typ][0] + " consumes"
	}

	reached := map[reflect.Type]bool{}
	for _, ct := range types {
		paths, cyclic := walkPaths(ct.goType, lifts)
		p := &plan{root: ct, producer: map[reflect.Type]*lift{}}
		for _, typ := range consumed {
			ps := paths[typ]
			if len(ps) == 0 {
				continue
			}
			reached[typ] = true
			if len(ps) > 1 || cyclic[typ] != "" {
				ok = false
				var lines []string
				for _, path := range ps {
					lines = append(lines, "  "+pathString(ct, path))
				}
				how := fmt.Sprintf("by %d paths", len(ps))
				if c := cyclic[typ]; c != "" {
					how = fmt.Sprintf("through the cycle %s, so by more than one path", c)
				}
				t.Errorf("from the corpus type %s, of testdata/%s/, %s, which %s, is reached %s:\n%s\nThere must be exactly one path to each type a column consumes. Remove a lift, or give the paths distinct product types.", ct.goType, ct.dir, typ, consume(typ), how, strings.Join(lines, "\n"))
				continue
			}
			for _, l := range ps[0] {
				p.producer[l.out] = l
			}
		}
		for _, c := range r.columns {
			if c.corpus == "" && len(paths[c.consumes]) == 1 && cyclic[c.consumes] == "" {
				p.cols = append(p.cols, c)
			}
		}
		for _, l := range lifts {
			if len(paths[l.in]) == 1 && cyclic[l.in] == "" {
				r.corpora[l.name] = append(r.corpora[l.name], ct.dir)
			}
		}
		for _, c := range p.cols {
			if c.lift == nil {
				r.corpora[c.name] = append(r.corpora[c.name], ct.dir)
			}
		}
		r.plans = append(r.plans, p)
	}
	r.planRaw()
	for _, typ := range consumed {
		if reached[typ] {
			continue
		}
		ok = false
		if typ == fsType {
			t.Errorf("%s fs.FS, which no corpus type is. A lift consumes a corpus type, for the raw trees, or a stage another lift reaches; only a suite runs over the raw input view, by naming a Corpus.\nDeclare a decl.Type for the archives' directory, and consume it.", consume(typ))
			continue
		}
		t.Errorf("%s %s, which no registered corpus type reaches.\nRegister the lift that produces it, or the corpus type.", consume(typ), typ)
	}
	if !ok {
		t.Log("No case runs until each type a column consumes is reached by one path.")
	}
	return ok
}

// planRaw adds a plan for each directory a suite's Corpus names: its
// archives' raw input views are fs.FS values, with no corpus type and no
// lift, and the suites that name it run over them. The plans join the
// corpus types', in the order of their directories.
func (r *runner) planRaw() {
	raw := map[string]*plan{}
	for _, c := range r.columns {
		if c.corpus == "" {
			continue
		}
		p := raw[c.corpus]
		if p == nil {
			p = &plan{
				root:     &corpusType{dir: c.corpus, goType: fsType, new: func(view fs.FS) any { return view }},
				producer: map[reflect.Type]*lift{},
			}
			raw[c.corpus] = p
			r.plans = append(r.plans, p)
		}
		p.cols = append(p.cols, c)
		r.corpora[c.name] = []string{c.corpus}
	}
	slices.SortFunc(r.plans, func(a, b *plan) int { return strings.Compare(a.root.dir, b.root.dir) })
}

// walkPaths returns every path of lifts from root to each type it reaches
// without a cycle, and, for each type a cycle reaches, the cycle.
func walkPaths(root reflect.Type, lifts []*lift) (paths map[reflect.Type][][]*lift, cyclic map[reflect.Type]string) {
	paths = map[reflect.Type][][]*lift{}
	var cycles []string
	var cycleAt []reflect.Type
	onPath := map[reflect.Type]bool{}
	var walk func(typ reflect.Type, path []*lift)
	walk = func(typ reflect.Type, path []*lift) {
		paths[typ] = append(paths[typ], slices.Clone(path))
		onPath[typ] = true
		for _, l := range lifts {
			if l.in != typ {
				continue
			}
			if onPath[l.out] {
				// The cycle runs from l.out along the path back to l.
				var names []string
				for i, pl := range path {
					if pl.in == l.out {
						for _, q := range path[i:] {
							names = append(names, q.name)
						}
						break
					}
				}
				names = append(names, l.name)
				cycles = append(cycles, strings.Join(names, ", "))
				cycleAt = append(cycleAt, l.out)
				continue
			}
			walk(l.out, append(path, l))
		}
		onPath[typ] = false
	}
	walk(root, nil)

	// Every type a cycle reaches is reached by more than one path.
	cyclic = map[reflect.Type]string{}
	for i, at := range cycleAt {
		queue := []reflect.Type{at}
		for len(queue) > 0 {
			typ := queue[0]
			queue = queue[1:]
			if cyclic[typ] != "" {
				continue
			}
			cyclic[typ] = cycles[i]
			for _, l := range lifts {
				if l.in == typ {
					queue = append(queue, l.out)
				}
			}
		}
	}
	return paths, cyclic
}

func pathString(ct *corpusType, path []*lift) string {
	s := ct.goType.String()
	for _, l := range path {
		s += " → " + l.name + " → " + l.out.String()
	}
	return s
}

// A caseState holds a case's values: its corpus type's, and each stage's,
// memoized, so a lift runs at most once per case however many cells read
// its product. Past a bytes lift, a normal run reads the product and the
// outcome that the archive commits for the case, and runs nothing before
// them; an update reads the product the lift makes.
type caseState struct {
	r     *runner
	c     *aCase
	plan  *plan
	root  any
	view  fs.FS // the case's input view
	runs  map[*lift]*liftRun
	reads map[*lift]*liftRead
}

// A liftRun is what one call of a lift's function left. A bytes lift's
// also holds its product's files, and the faults of its jenny, if any: an
// error that is no rejection, or a product an archive can't hold. Only a
// product with no fault has a view and a value.
type liftRun struct {
	value    any
	out      *called
	product  map[string][]byte // a bytes lift's product, by path in its view; nil when it made none
	view     fs.FS
	problems []string
}

// A liftRead is what a normal run reads of a bytes lift on a case: the
// committed product, which the cells past it read, or what blocks them.
type liftRead struct {
	value any
	view  fs.FS
	block *blockage
}

// A blockage is what keeps a cell from running: a lift on its path that
// failed on the case.
type blockage struct {
	// lift is the lift that failed, or the bytes lift whose committed
	// outcome blocks the cell.
	lift *lift
	// failed names the lift that failed, which the cell's outcome names.
	failed string
	// outcome is, for a block a bytes lift's committed outcome makes, the
	// section that holds it, and committed its line; missing records that
	// the archive has no such section. outcome is "" for a lift that
	// failed in this run.
	outcome   string
	committed string
	missing   bool
	// broken records that, in an update, the bytes lift's jenny has a
	// fault, so its cell isn't written: the cell runs nothing, and the
	// update writes nothing for it.
	broken bool
}

// stage returns the value of a type the plan reaches, and the input view
// it comes from: the case's, or, past a bytes lift, the last one's
// product. It returns what blocks the type instead, when something does.
func (s *caseState) stage(typ reflect.Type) (any, fs.FS, *blockage) {
	if typ == s.plan.root.goType {
		return s.root, s.view, nil
	}
	l := s.plan.producer[typ]
	if l.bytes && !s.r.update {
		rd := s.read(l)
		return rd.value, rd.view, rd.block
	}
	in, view, b := s.stage(l.in)
	if b != nil {
		return nil, nil, b
	}
	lr := s.runLift(l, in)
	switch {
	case lr.out.panicked || lr.out.err != nil || lr.out.noProduct:
		return nil, nil, &blockage{lift: l, failed: l.name}
	case !l.bytes:
		return lr.value, view, nil
	case len(lr.problems) > 0:
		return nil, nil, &blockage{lift: l, failed: l.name, broken: true}
	}
	return lr.value, lr.view, nil
}

// read reads, once, what the archive commits for a bytes lift on the
// case. Its committed outcome decides: accepted gives the committed
// product, an empty one included; blocked by another lift blocks the cells
// past it by that lift; anything else, and no outcome, blocks them by the
// lift itself.
func (s *caseState) read(l *lift) *liftRead {
	if rd, ok := s.reads[l]; ok {
		return rd
	}
	rd := &liftRead{}
	s.reads[l] = rd
	section := s.c.out(l.name, archive.OutcomeFile)
	data, ok := s.c.a.Section(section)
	if !ok {
		rd.block = &blockage{lift: l, failed: l.name, outcome: section, missing: true}
		return rd
	}
	b := &blockage{lift: l, failed: l.name, outcome: section, committed: strings.TrimSuffix(string(data), "\n")}
	switch o, err := archive.ParseOutcome(data); {
	case err != nil:
		rd.block = b
	case o.State == archive.Accepted:
		rd.view = archive.NewFS(s.c.product(l))
		rd.value = s.r.products[l].new(rd.view)
	case o.State == archive.Blocked:
		b.failed = o.Detail
		rd.block = b
	default:
		rd.block = b
	}
	return rd
}

// runLift runs a lift on its input, once. A lift whose product type is an
// interface can return neither a product nor an error, which is the lift's
// own failure. A bytes lift's jenny makes its product's files, and rejects
// the case only by an error decl.Reject marks: any other error is a fault
// of the jenny, as a product an archive can't hold is.
func (s *caseState) runLift(l *lift, in any) *liftRun {
	if lr, ok := s.runs[l]; ok {
		return lr
	}
	lr := &liftRun{}
	s.runs[l] = lr
	if !l.bytes {
		lr.out = call(func(o *called) {
			lr.value, o.err = l.run(s.c.archive, in, &o.stderr)
		})
		lr.out.noProduct = !lr.out.panicked && lr.out.err == nil && lr.value == nil
		return lr
	}
	var made *jenny.FS
	lr.out = call(func(o *called) { made, o.err = l.generate(in) })
	switch {
	case lr.out.panicked:
		return lr
	case lr.out.err != nil && erase.IsRejection(lr.out.err):
		return lr
	case lr.out.err != nil:
		lr.out.jennyErr, lr.out.err = lr.out.err, nil
		lr.problems = []string{fmt.Sprintf("%s's jenny failed on the case, with an error that is no rejection:\n  %s\nIf %s rejects the case, return the error as decl.Reject(err). If not, fix %s's jenny. An update writes nothing for this cell.", l.name, indent(lr.out.jennyErr.Error(), "  "), l.name, l.name)}
		return lr
	}
	lr.product, lr.problems = productOf(l, s.c.prefix, made)
	if len(lr.problems) == 0 {
		lr.view = archive.NewFS(lr.product)
		lr.value = s.r.products[l].new(lr.view)
	}
	return lr
}

// productOf reads a bytes lift's product from what its jenny made, for a
// case whose sections start with prefix, and says why an archive can't
// hold it, if it can't: a file whose name doesn't read back as written; a
// file outside any tree, or in the tree params; a file that is also a
// directory; a file no declaration matches, or two do; and a file an
// archive would read back differently.
func productOf(l *lift, prefix string, made *jenny.FS) (map[string][]byte, []string) {
	files := map[string][]byte{}
	var problems []string
	for _, f := range made.AsFiles() {
		// A jenny's FS renders each file it adds, and AsFiles gives each
		// one as Bytes, as for a transform's, so no test reaches this.
		data, ok := jenny.BytesOf(f)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s made %s, which its FS gave as a %T, not as rendered bytes. An FS renders each file it adds, so this is a bug in hypercase or jenny: report it to hypercase.", l.name, f.Path(), f))
			continue
		}
		files[f.Path()] = data
	}
	dir := l.viaDir(prefix)
	// A name that doesn't read back is quoted, since it may hold a
	// newline, and isn't checked further.
	var paths []string
	for _, p := range slices.Sorted(maps.Keys(files)) {
		if err := archive.HoldsName(dir + p); err != nil {
			problems = append(problems, fmt.Sprintf("%q can't be held in an archive: %v.\nFix %s's jenny so the file's name can be held. An update writes nothing for this cell.", dir+p, err, l.name))
			continue
		}
		paths = append(paths, p)
	}
	for _, p := range archive.CheckVia(prefix, l.name, paths) {
		problems = append(problems, fmt.Sprintf("%s.\nFix %s's jenny: what it makes is its product's input view, with the trees as its top-level directories. An update writes nothing for this cell.", p, l.name))
	}
	for _, p := range paths {
		switch hit := l.declaredProduct(p); len(hit) {
		case 0:
			problems = append(problems, fmt.Sprintf("%s is not declared.\nDeclare it in %s's Product, with a description, or fix %s's jenny. An update writes nothing for this cell.", dir+p, l.name, l.name))
		case 1:
		default:
			problems = append(problems, fmt.Sprintf("%s matches two of %s's product declarations, %s and %s.\nChange the patterns so that one declaration matches each file. An update writes nothing for this cell.", dir+p, l.name, hit[0], hit[1]))
		}
		if err := archive.Holds(files[p]); err != nil {
			problems = append(problems, fmt.Sprintf("%s can't be held in an archive: %v.\nFix %s's jenny so the file can be held. An update writes nothing for this cell.", dir+p, err, l.name))
		}
	}
	return files, problems
}
