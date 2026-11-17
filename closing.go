package hypercase

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
)

// A cell's failure is a list of notes, each one problem of the cell: its
// body says what is wrong, and its last line, its closing, what to do. One
// function, close, gives every closing, from what the package's update
// does with the cell, as the staging tells: the note's own default where
// the update does with the cell what this run does, what the update does
// where it does otherwise, and that the package can't tell where it
// doesn't own every bytes lift on the cell's path. A note's default is all
// a message site gives of its closing, and report, which takes the notes
// and the staging, is the one way a cell's notes fail its test.

// A noteKind is what a note is about, which decides what its closing says
// when the update does otherwise with the cell than this run.
type noteKind int

const (
	// aboutFault is what the cell's run, its code, its probe or the
	// project's hooks did wrong on the case, an error the case's
	// properties don't expect among them.
	aboutFault noteKind = iota
	// aboutFile is a file the cell made, or a bytes lift's product file,
	// that differs from its golden or has none.
	aboutFile
	// aboutOutcome is the cell's outcome, which differs from its golden or
	// has none.
	aboutOutcome
	// aboutOrphan is a file the cell commits under out/<column>/ that this
	// run doesn't produce.
	aboutOrphan
	// aboutProductOrphan is a file of a bytes lift's committed product that
	// this run of the lift doesn't make.
	aboutProductOrphan
	// aboutGap is a gaps line that marks a cell that ran nothing.
	aboutGap
	// aboutProbeFault is a probe's fault in a cell that ran nothing: it said
	// the case isn't for it with no reason, or with one an outcome can't
	// hold, so the cell has no outcome.
	aboutProbeFault
	// aboutFrozen is a frozen cell whose run differs from its committed
	// files, or that lacks an outcome.
	aboutFrozen
)

// A note is one problem of a cell: body says what is wrong, and def what
// to do when the update does with the cell what this run does.
type note struct {
	kind noteKind
	body string
	def  string
}

// notes collects a cell's notes, in the order its message gives them.
type notes []note

// add adds a note of the given kind, with its body and its default
// closing.
func (ns *notes) add(kind noteKind, body, def string) {
	*ns = append(*ns, note{kind: kind, body: body, def: def})
}

// A staging is what a normal run tells of what the package's update does
// with one cell, which the cell's closings say. The update may stage the
// case otherwise than this run: on what a transform makes, where this run
// reads a variant's committed inputs, and rerunning each bytes lift on the
// cell's path, where this run reads what the archive commits for it. It is
// told once, when a closing first needs it, since telling may run lifts
// again.
type staging struct {
	r   *runner
	c   *aCase
	col *column
	// this is the cell's fate in this run, and ran says whether this run
	// ran the cell on a value, so that the update does what this run does
	// only when it runs the cell on that value too.
	this updateFate
	ran  bool
	// frozen records that a gaps line freezes the cell, which no update
	// writes.
	frozen bool

	told bool
	// unowned is the last bytes lift on the cell's path that the package
	// doesn't own, which keeps it from telling: only the package that owns
	// a bytes lift runs it in a normal run.
	unowned *lift
	// f is the cell's fate in the update, and does says it. does is ""
	// when the update does with the cell what this run does.
	f    updateFate
	does string
}

// staging makes a cell's staging, untold: this is the cell's fate in this
// run, ran whether this run ran it on a value, and frozen whether a gaps
// line freezes it.
func (r *runner) staging(c *aCase, col *column, this updateFate, ran, frozen bool) *staging {
	return &staging{r: r, c: c, col: col, this: this, ran: ran, frozen: frozen}
}

// declinedStaging is the staging of the notes of a transform's variant
// that the transform declines: the update stages the variant from the same
// base, and declines it too.
func (r *runner) declinedStaging(c *aCase) *staging { return &staging{r: r, c: c, told: true} }

// tell tells, once, what the update does with the cell. An update, and a
// normal run that stages the case as its update does, tell nothing.
func (st *staging) tell() {
	if st.told {
		return
	}
	st.told = true
	if st.r.update {
		return
	}
	s := st.c.state
	last, unowned := s.plan.bytesOnPath(st.col.consumes)
	switch {
	case last == nil && s.made == nil:
		return
	case unowned != nil:
		st.unowned = unowned
		return
	}
	st.f = s.updateFate(st.col)
	// A cell this run ran meets another value in the update when the last
	// bytes lift on its path makes another product, or, with none, when
	// the update stages the variant on what the transform makes.
	if st.f.same(st.this) && !(st.ran && st.this.runs() && s.restages(st.col)) {
		return
	}
	st.does = st.r.does(st.col, st.f, st.c.staged(last), st.ran)
}

// differs reports whether the update does otherwise with the cell than
// this run, or the package can't tell.
func (st *staging) differs() bool {
	st.tell()
	return st.does != "" || st.unowned != nil
}

// close gives a note's closing: the note's default where the update does
// with the cell what this run does, what the update does where it does
// otherwise, and where the package can't tell, that it can't.
func (st *staging) close(n note) string {
	st.tell()
	switch {
	case st.unowned != nil:
		return st.cantTell(n)
	case st.does == "":
		return n.def
	}
	return then(st.f, st.does, st.tail(n.kind))
}

// tail is what a closing says after does, for a note of the given kind,
// where the update does otherwise with the cell than this run.
func (st *staging) tail(kind noteKind) string {
	u := st.r.updateCmd()
	f := st.f
	if f.block != nil && f.block.broken {
		// does says the update writes nothing for the cell, past the lift
		// whose cell it writes nothing for: what is left to do is that
		// cell's.
		if kind == aboutOrphan || kind == aboutProductOrphan {
			return fmt.Sprintf("Fix what %s's cell reports, and delete nothing by hand before %s has run.", f.block.lift.name, u)
		}
		return fmt.Sprintf("Fix what %s's cell reports.", f.block.lift.name)
	}
	switch kind {
	case aboutOutcome:
		return outcomeWrites(f)
	case aboutProbeFault:
		return st.probeFaultWrites()
	case aboutOrphan:
		switch {
		case f.runs() && st.ran:
			return "Its run may still write the file: delete nothing by hand before it has run."
		case f.runs():
			return "Delete nothing by hand before it has run."
		case f.fault != "":
			return "Fix the probe."
		case f.skip != "":
			return fmt.Sprintf("If the suite should run on the case, fix the probe. If not, delete the section by hand; %s leaves it in place.", u)
		}
		return fmt.Sprintf("If the cell should run, fix the lift or %s. If not, delete the section by hand; %s leaves it in place.", st.c.inputsAt(), u)
	case aboutProductOrphan:
		switch {
		case f.runs() && st.ran:
			return "Its run may still make the file: delete nothing by hand before it has run."
		case f.runs():
			return "Delete nothing by hand before it has run."
		}
		// A lift's column has no probe, so the update blocks the lift, and
		// replaces its product with none.
		return "It removes the file: read the diff before committing."
	case aboutGap:
		switch {
		case f.skip != "":
			return "Remove the line, or fix the probe."
		case f.block != nil:
			return fmt.Sprintf("Remove the line, or fix the lift or %s.", st.c.inputsAt())
		}
	}
	return "Run it, fix what it reports, and read the diff before committing."
}

// cantTell is a note's closing in a package that doesn't own every bytes
// lift on the cell's path: it doesn't run such a lift in a normal run, so
// it can't tell what its update, which reruns it, does with the cell.
func (st *staging) cantTell(n note) string {
	s := fmt.Sprintf("Only the package that owns %s runs it in a normal run, so this one can't tell what %s does with %s: %s.", st.unowned.name, st.r.updateCmd(), st.col.name, st.c.staged(st.unowned))
	switch n.kind {
	case aboutOrphan, aboutProductOrphan:
		return s + " Delete nothing by hand before it has run."
	}
	return s + " Run it, fix what it reports, and read the diff before committing."
}

// probeFaultWrites says, of a probe's fault in a cell that ran nothing,
// which has no outcome in this run, what the update writes for the cell.
// No update writes a frozen cell. tail gives the closing past a lift whose
// cell the update writes nothing for.
func (st *staging) probeFaultWrites() string {
	f := st.f
	switch {
	case f.runs():
		return "What it writes for the cell, if anything, is its run's."
	case st.frozen:
		return "It writes nothing for the cell, which a gaps line freezes."
	case f.fault != "":
		return faultWrites(f, false)
	case f.skip != "":
		return fmt.Sprintf("It writes the outcome %q.", outcomeLine(archive.Outcome{State: archive.Skipped, Detail: f.skip}))
	}
	return fmt.Sprintf("It writes the outcome %q.", outcomeLine(archive.Outcome{State: archive.Blocked, Detail: f.block.failed}))
}

// report fails a cell's test with its notes, under the cell's header: each
// note's body, then its closing.
func report(t *testing.T, header string, ns notes, st *staging) {
	t.Helper()
	parts := make([]string, len(ns))
	for i, n := range ns {
		closing := st.close(n)
		if observe.note != nil {
			observe.note(st, n, closing)
		}
		parts[i] = n.body + "\n" + closing
	}
	t.Errorf("%s\n%s", header, strings.Join(parts, "\n"))
}

// observe is nil but in hypercase's own tests, which set its functions in
// the test binaries they run as children, to see each cell's result and
// fate in every run, and each note's closing in a normal run, and check
// what a normal run says the update does against what the update did.
var observe struct {
	cell func(c *aCase, col *column, res *cellResult)
	note func(st *staging, n note, closing string)
}

// does words what the package's update does with col's cell, whose fate
// there is f, where staged says how the update stages the case otherwise
// than this normal run. ran says whether this run ran the cell, so that
// the update's run of it is on another value. A skip's sentence ends with
// the probe's reason.
func (r *runner) does(col *column, f updateFate, staged string, ran bool) string {
	u := r.updateCmd()
	switch {
	case f.runs() && ran:
		return fmt.Sprintf("%s runs %s on another value than this run: %s.", u, col.name, staged)
	case f.runs():
		return fmt.Sprintf("%s runs %s: %s.", u, col.name, staged)
	case f.fault != "":
		return fmt.Sprintf("%s fails %s: %s, and %s.", u, col.name, staged, f.fault)
	case f.skip != "":
		return fmt.Sprintf("%s skips %s: %s, and %s's probe says the case isn't for it: %s", u, col.name, staged, col.name, f.skip)
	case f.block.broken:
		return fmt.Sprintf("%s writes nothing for %s: %s, and it writes nothing for the cell of the lift %s, on %s's path, whose outcome and product a normal run reads.", u, col.name, staged, f.block.lift.name, col.name)
	}
	return fmt.Sprintf("%s blocks %s: %s, and %s.", u, col.name, staged, f.block.why(col))
}

// outcomeWrites says what the update writes for a cell's outcome, whose
// fate there is f: against the outcome this normal run gives it. tail
// gives the closing past a lift whose cell the update writes nothing for.
func outcomeWrites(f updateFate) string {
	switch {
	case f.runs():
		return "What it writes for the cell, if anything, is its run's, not this outcome."
	case f.fault != "":
		return "Fix the probe. " + faultWrites(f, true)
	case f.skip != "":
		return fmt.Sprintf("It writes the outcome %q, not this one.", outcomeLine(archive.Outcome{State: archive.Skipped, Detail: f.skip}))
	}
	return fmt.Sprintf("It writes the outcome %q, not this one.", outcomeLine(archive.Outcome{State: archive.Blocked, Detail: f.block.failed}))
}

// faultWrites says what the update writes for a cell whose probe fails
// there, as f says: for a panic, the outcome panicked, against this run's
// when against is set, and the panic in error.txt; for any other fault,
// nothing.
func faultWrites(f updateFate, against bool) string {
	if !f.panicked {
		return "It writes nothing for the cell."
	}
	o := outcomeLine(archive.Outcome{State: archive.Panicked})
	if against {
		return fmt.Sprintf("It writes the outcome %q, not this one, and the panic in %s.", o, errorFile)
	}
	return fmt.Sprintf("It writes the outcome %q, and the panic in %s.", o, errorFile)
}

// outcomeLine is an outcome as its file holds it, less its newline.
func outcomeLine(o archive.Outcome) string {
	return strings.TrimSuffix(string(o.Format()), "\n")
}

// then joins what follows a sentence of does, which says what the update
// does with a cell whose fate there is f: on a new line after a skip,
// whose sentence ends with the probe's reason, and after a space
// otherwise.
func then(f updateFate, does, more string) string {
	if f.skip != "" {
		return does + "\n" + more
	}
	return does + " " + more
}
