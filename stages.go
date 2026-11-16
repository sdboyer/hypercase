package hypercase

import (
	"fmt"
	"io/fs"
	"reflect"
	"slices"
	"strings"
	"testing"
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
// its product.
type caseState struct {
	archive string
	plan    *plan
	root    any
	runs    map[*lift]*liftRun
}

// A liftRun is what one call of a lift's function left.
type liftRun struct {
	value any
	out   *called
}

// stage returns the value of a type the plan reaches, or the lift on its
// path that failed.
func (s *caseState) stage(typ reflect.Type) (any, *lift) {
	if typ == s.plan.root.goType {
		return s.root, nil
	}
	l := s.plan.producer[typ]
	in, by := s.stage(l.in)
	if by != nil {
		return nil, by
	}
	lr := s.runLift(l, in)
	if lr.out.panicked || lr.out.err != nil {
		return nil, l
	}
	return lr.value, nil
}

// runLift runs a lift on its input, once.
func (s *caseState) runLift(l *lift, in any) *liftRun {
	if lr, ok := s.runs[l]; ok {
		return lr
	}
	lr := &liftRun{}
	lr.out = call(func(o *called) {
		lr.value, o.err = l.run(s.archive, in, &o.stderr)
	})
	s.runs[l] = lr
	return lr
}
