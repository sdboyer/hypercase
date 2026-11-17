package hypercase

import (
	"bytes"
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
// its product, but for a normal run's asUpdate. Past a bytes lift, a
// normal run reads the product and the outcome that the archive commits
// for the case, and runs nothing before them; an update reads the product
// the lift makes.
type caseState struct {
	r     *runner
	c     *aCase
	plan  *plan
	root  any
	view  fs.FS // the case's input view
	runs  map[*lift]*liftRun
	reads map[*lift]*liftRead
	// update says how the state stages the types past a bytes lift: by
	// running the lift, as an update does, or by reading what the archive
	// commits for it, as a normal run does.
	update bool
	// made is, in a normal run of a transform's variant whose committed
	// inputs give another input view than the one the transform makes,
	// what it makes: the input view the update stages the case on. It is
	// nil when the update stages the case on view, or on a view that holds
	// the same files.
	made fs.FS
	// asUpdate is, in a normal run, the case staged as the package's
	// update stages it, made when a cell first asks what the update does
	// with it. Its normal is the normal run's state, whose runs it shares
	// for each lift no bytes lift precedes: such a lift's input is the same
	// in both. A lift past a bytes lift runs once in each. When the update
	// stages the case on made, no lift's input need be the same in both,
	// so it has no normal, and shares no run.
	asUpdate *caseState
	normal   *caseState
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
	problems notes
	verdict  *liftVerdict // a bytes lift's cell's, once judged
	bare     *liftVerdict // a bytes lift's cell's under a bare Project, once judged
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
	// broken records that, in an update, the bytes lift's cell isn't
	// written, as a fault of its jenny or any other problem of the cell
	// keeps it from being: the cell runs nothing, and the update writes
	// nothing for it.
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
	if l.bytes && !s.update {
		rd := s.read(l)
		return rd.value, rd.view, rd.block
	}
	in, view, b := s.stage(l.in)
	if b != nil {
		return nil, nil, b
	}
	lr := s.runLift(l, in)
	// Past a bytes lift, which runs here only in an update, or in a normal
	// run's staging of the case as its update stages it, a cell is written
	// only with the lift's cell: a normal run reads what that writes.
	if l.bytes && s.heldBack(l, lr, in, view) {
		return nil, nil, &blockage{lift: l, failed: l.name, broken: true}
	}
	switch {
	case lr.out.panicked || lr.out.err != nil || lr.out.noProduct:
		return nil, nil, &blockage{lift: l, failed: l.name}
	case !l.bytes:
		return lr.value, view, nil
	}
	return lr.value, lr.view, nil
}

// heldBack reports whether, in an update, the cells past the bytes lift l
// wait for its cell: whether the update writes nothing for the cell, whose
// outcome and product a normal run reads. The package that owns l holds
// them back on the cell's verdict. Another package's Project doesn't judge
// l's cell, and the owner's may differ, so it holds them back on the
// cell's verdict under a bare Project, which fails no cell: on what keeps
// the update from writing the cell whatever the owner's Project, such as a
// fault of the jenny or a product an archive can't hold, when it registers
// the owner's declaration of l. The root-package check fails on two
// declarations whose registry entries differ.
func (s *caseState) heldBack(l *lift, lr *liftRun, in any, view fs.FS) bool {
	if l.owned {
		return !s.liftVerdict(l, lr, in, view).writable
	}
	if lr.bare == nil {
		lr.bare = s.judgeLift(s.r.bare(), l, lr, in, view)
	}
	return !lr.bare.writable
}

// An updateFate is what the package's update does with a cell, which a
// normal run tells by staging the case as the update stages it. The
// update runs the cell when nothing blocks it and its probe, if it has
// one, says the case is for it, with no fault.
type updateFate struct {
	// block is what blocks the cell in the update, when something does.
	block *blockage
	// skip is the reason the cell's probe gives, on the value the update
	// stages, for saying the case isn't for it, when it gives one an
	// outcome can hold.
	skip string
	// fault says how the cell's probe fails the cell on the value the
	// update stages, when it does: it panics, or says the case isn't for
	// it with no reason, or with one an outcome can't hold.
	fault string
	// panicked records that the fault is the probe's panic, for which the
	// update writes the outcome panicked, and the panic in error.txt. For
	// the other faults, it writes nothing for the cell.
	panicked bool
}

// runs reports whether the update runs the cell.
func (f updateFate) runs() bool { return f.block == nil && f.skip == "" && f.fault == "" }

// same reports whether two fates of a cell are one: both block it by the
// failure of one lift, skip it for one reason, fail it with one fault of
// its probe, or run it, on whatever value.
func (f updateFate) same(g updateFate) bool {
	if f.block != nil || g.block != nil {
		return f.block != nil && g.block != nil && !f.block.broken && !g.block.broken && f.block.failed == g.block.failed
	}
	return f.skip == g.skip && f.fault == g.fault
}

// probePanics is the fault of col's probe that panics with v.
func probePanics(col *column, v any) string {
	return fmt.Sprintf("%s's probe panics: %v", col.name, v)
}

// updateFate tells, in a normal run, what the package's update does with
// col's cell: whether the case, staged as the update stages it, reaches
// the type col consumes with nothing to block it, and if it does, what
// col's probe, if it has one, says of the value it reaches. The update
// stages a transform's variant on what the transform makes, where a normal
// run reads the committed inputs, and runs each bytes lift, where a normal
// run reads what the archive commits for it. A normal run runs only the
// bytes lifts the package owns, so only a package that owns every bytes
// lift on the way asks.
func (s *caseState) updateFate(col *column) updateFate {
	if s.asUpdate == nil {
		s.asUpdate = &caseState{
			r:      s.r,
			c:      s.c,
			plan:   s.plan,
			root:   s.root,
			view:   s.view,
			runs:   map[*lift]*liftRun{},
			reads:  map[*lift]*liftRead{},
			update: true,
			normal: s,
		}
		if s.made != nil {
			s.asUpdate.root, s.asUpdate.view, s.asUpdate.normal = s.plan.root.new(s.made), s.made, nil
		}
	}
	in, _, b := s.asUpdate.stage(col.consumes)
	if b != nil {
		return updateFate{block: b}
	}
	if col.probe == nil {
		return updateFate{}
	}
	var why string
	var ok bool
	p := call(func(*called) { why, ok = col.probe(in) })
	switch {
	case p.panicked:
		return updateFate{fault: probePanics(col, p.panicValue), panicked: true}
	case ok:
		return updateFate{}
	}
	if fault := probeFault(col, why); fault != "" {
		return updateFate{fault: fault}
	}
	return updateFate{skip: why}
}

// probeFault says how col's probe, which said the case isn't for it, with
// the reason why, fails the cell: with no reason, or with one an outcome
// can't hold. It returns "" for a reason an outcome holds.
func probeFault(col *column, why string) string {
	if strings.TrimSpace(why) == "" {
		return fmt.Sprintf("%s's probe says the case isn't for it, and gives no reason", col.name)
	}
	if err := archive.CheckDetail(why); err != nil {
		return fmt.Sprintf("%s's probe says the case isn't for it, with a reason an outcome can't hold: %v", col.name, err)
	}
	return ""
}

// restages reports, in a normal run, once updateFate says the update runs
// col's cell, whether the update runs it on another value than this run
// does. Past a bytes lift, the cell's value comes from the last bytes
// lift's product alone, so it differs when that lift makes another
// product than the one the archive commits. With no bytes lift on the
// path, it differs when the update stages a transform's variant on what
// the transform makes, whose input view differs from the committed
// inputs'.
func (s *caseState) restages(col *column) bool {
	last, _ := s.plan.bytesOnPath(col.consumes)
	if last == nil {
		return s.made != nil
	}
	lr, ok := s.asUpdate.runs[last]
	if !ok && s.asUpdate.normal != nil {
		lr, ok = s.asUpdate.normal.runs[last]
	}
	return !ok || !maps.EqualFunc(lr.product, s.c.product(last), bytes.Equal)
}

// bytesOnPath returns the last bytes lift on the plan's path to typ, and
// the last there that the package doesn't own, each nil when there is
// none.
func (p *plan) bytesOnPath(typ reflect.Type) (last, unowned *lift) {
	for typ != p.root.goType {
		l := p.producer[typ]
		if l.bytes && last == nil {
			last = l
		}
		if l.bytes && !l.owned && unowned == nil {
			unowned = l
		}
		typ = l.in
	}
	return last, unowned
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
	if s.normal != nil {
		if last, _ := s.plan.bytesOnPath(l.in); last == nil {
			return s.normal.runLift(l, in)
		}
	}
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
		lr.problems.add(aboutFault,
			fmt.Sprintf("%s's jenny failed on the case, with an error that is no rejection:\n  %s", l.name, indent(lr.out.jennyErr.Error(), "  ")),
			fmt.Sprintf("If %s rejects the case, return the error as decl.Reject(err). If not, fix %s's jenny. An update writes nothing for this cell.", l.name, l.name))
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
func productOf(l *lift, prefix string, made *jenny.FS) (map[string][]byte, notes) {
	files := map[string][]byte{}
	var problems notes
	for _, f := range made.AsFiles() {
		// A jenny's FS renders each file it adds, and AsFiles gives each
		// one as Bytes, as for a transform's, so no test reaches this.
		data, ok := jenny.BytesOf(f)
		if !ok {
			problems.add(aboutFault,
				fmt.Sprintf("%s made %s, which its FS gave as a %T, not as rendered bytes.", l.name, f.Path(), f),
				"An FS renders each file it adds, so this is a bug in hypercase or jenny: report it to hypercase.")
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
			problems.add(aboutFault,
				fmt.Sprintf("%q can't be held in an archive: %v.", dir+p, err),
				fmt.Sprintf("Fix %s's jenny so the file's name can be held. An update writes nothing for this cell.", l.name))
			continue
		}
		paths = append(paths, p)
	}
	for _, p := range archive.CheckVia(prefix, l.name, paths) {
		problems.add(aboutFault, p.String()+".",
			fmt.Sprintf("Fix %s's jenny: what it makes is its product's input view, with the trees as its top-level directories. An update writes nothing for this cell.", l.name))
	}
	for _, p := range paths {
		switch hit := l.declaredProduct(p); len(hit) {
		case 0:
			problems.add(aboutFault, fmt.Sprintf("%s is not declared.", dir+p),
				fmt.Sprintf("Declare it in %s's Product, with a description, or fix %s's jenny. An update writes nothing for this cell.", l.name, l.name))
		case 1:
		default:
			problems.add(aboutFault, fmt.Sprintf("%s matches two of %s's product declarations, %s and %s.", dir+p, l.name, hit[0], hit[1]),
				"Change the patterns so that one declaration matches each file. An update writes nothing for this cell.")
		}
		if err := archive.Holds(files[p]); err != nil {
			problems.add(aboutFault, fmt.Sprintf("%s can't be held in an archive: %v.", dir+p, err),
				fmt.Sprintf("Fix %s's jenny so the file can be held. An update writes nothing for this cell.", l.name))
		}
	}
	return files, problems
}
