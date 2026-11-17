package hypercase

import (
	"bytes"
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
// and the staging, is the one way a cell's notes fail its test. The notes
// of a transform's variant that are about no one cell close on their
// defaults, and fail the variant's test through report too.

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
	// aboutInputs is the committed inputs of a transform's variant, which
	// differ from what the transform makes, are invalid or missing, or are
	// written where the transform declines the base. Such a note is about
	// no one cell.
	aboutInputs
)

// A note is one problem of a cell: body says what is wrong, and def what
// to do when the update does with the cell what this run does. file is
// the section a note about one file is about, which a closing may say the
// update leaves, removes or may still write.
type note struct {
	kind noteKind
	file string
	body string
	def  closing
}

// notes collects a cell's notes, in the order its message gives them.
type notes []note

// add adds a note of the given kind, with its body and its default
// closing.
func (ns *notes) add(kind noteKind, body string, def closing) {
	*ns = append(*ns, note{kind: kind, body: body, def: def})
}

// addFile adds a note about one section, file.
func (ns *notes) addFile(kind noteKind, file, body string, def closing) {
	*ns = append(*ns, note{kind: kind, file: file, body: body, def: def})
}

// A closing is what to do, its advice, which says nothing of what an
// update does, and what it claims the update does, writes and leaves.
// Every sentence of a closing that says what an update does comes from its
// claims, which render their own words, so TestClosings, which checks each
// claim against what the update then does, checks all a closing says of
// it.
type closing struct {
	advice string
	claims claims
}

// claims are what a closing says the update does with a cell, and writes
// or leaves. TestClosings checks each field against what the update did,
// and fails on a field it has no check for. A section is named whole, as
// the archive holds it.
type claims struct {
	// Does is what the update does with the cell, as fateWords words its
	// fate there, where it does otherwise than this run. It is "" in the
	// note's default, which the update does where it does what this run
	// does, and where the package can't tell.
	Does string `json:"does,omitempty"`
	// Another records that the update runs the cell on another value than
	// this run, which ran it.
	Another bool `json:"another,omitempty"`
	// Variant names the transform on whose output the update stages the
	// case, a variant of it, where this run reads its committed inputs.
	Variant string `json:"variant,omitempty"`
	// Reruns names the bytes lift the update reruns, whose outcome and
	// product this run reads from the archive: the last on the cell's path
	// that the update reaches.
	Reruns string `json:"reruns,omitempty"`
	// Reads names the last bytes lift on the cell's path, whose outcome
	// and product this run reads from the archive, where the update blocks
	// the cell by a lift before every bytes lift there, so it reaches none
	// of them, and stages no variant.
	Reads string `json:"reads,omitempty"`
	// CantTell names the bytes lift on the cell's path that the package
	// doesn't own, so it can't tell what the update does with the cell: a
	// normal run of the package reads the lift's outcome and product from
	// the archive, and only the package that owns it runs it. A lift before
	// it may fail in the update, which then reaches it in no package, so
	// CantTell says nothing of what the update reruns.
	CantTell string `json:"cantTell,omitempty"`
	// PastFresh records, of a frozen bytes lift's cell, that the update
	// writes the cells past the lift from what it makes now, and leaves
	// the cell's committed outcome and product in place.
	PastFresh bool `json:"pastFresh,omitempty"`
	// RunWrites records that what the update writes for the cell, if
	// anything, is its run's.
	RunWrites bool `json:"runWrites,omitempty"`
	// MayWrite is a section the update's run of the cell, on another
	// value, may still write.
	MayWrite string `json:"mayWrite,omitempty"`
	// Outcome is the outcome the update writes for the cell, as its file
	// holds it, less its newline. Differs records that this run's outcome
	// differs from what the update writes, and Panic that the update
	// writes the probe's panic in error.txt.
	Outcome string `json:"outcome,omitempty"`
	Differs bool   `json:"differs,omitempty"`
	Panic   bool   `json:"panic,omitempty"`
	// Nothing records that the update writes nothing for the cell, and
	// Frozen that a gaps line freezes it.
	Nothing bool `json:"nothing,omitempty"`
	Frozen  bool `json:"frozen,omitempty"`
	// Writes is a section the update writes, OnlyOutcome a cell's outcome
	// section, which the update adds, changing nothing else of the cell,
	// Leaves a section it leaves in place, and Removes one it removes.
	Writes      string `json:"writes,omitempty"`
	OnlyOutcome string `json:"onlyOutcome,omitempty"`
	Leaves      string `json:"leaves,omitempty"`
	Removes     string `json:"removes,omitempty"`
	// RemovesInputs names the transform whose variant's inputs the update
	// removes, and WritesInputs one whose variant's inputs, which the
	// archive lacks, the update writes. RewritesInputs names one this
	// package owns, whose variant's inputs its update rewrites, and
	// OwnerRewritesInputs one it doesn't, whose variant's inputs the update
	// of the package that owns it rewrites, if a package does.
	RemovesInputs       string `json:"removesInputs,omitempty"`
	WritesInputs        string `json:"writesInputs,omitempty"`
	RewritesInputs      string `json:"rewritesInputs,omitempty"`
	OwnerRewritesInputs string `json:"ownerRewritesInputs,omitempty"`
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
	// outcome is the cell's outcome in this run, as its file holds it, or
	// nil where it has none.
	outcome []byte
	// declined records the staging of a variant its transform declines,
	// which the update declines too.
	declined bool

	told bool
	// unowned is the last bytes lift on the cell's path that the package
	// doesn't own, which keeps it from telling: only the package that owns
	// a bytes lift runs it in a normal run.
	unowned *lift
	// f is the cell's fate in the update, and otherwise says whether the
	// update does otherwise with the cell than this run. reran is the last
	// bytes lift on the cell's path that the update runs, nil where it runs
	// none.
	f         updateFate
	otherwise bool
	reran     *lift
}

// staging makes a cell's staging, untold: this is the cell's fate in this
// run, ran whether this run ran it on a value, frozen whether a gaps line
// freezes it, and outcome its outcome in this run, if it has one.
func (r *runner) staging(c *aCase, col *column, this updateFate, ran, frozen bool, outcome []byte) *staging {
	return &staging{r: r, c: c, col: col, this: this, ran: ran, frozen: frozen, outcome: outcome}
}

// variantStaging is the staging of the notes of a transform's variant
// that are about no one cell, which close on their defaults.
func (r *runner) variantStaging(c *aCase) *staging {
	return &staging{r: r, c: c, told: true}
}

// declinedStaging is the staging of the notes of a transform's variant
// that the transform declines: the update stages the variant from the same
// base, and declines it too.
func (r *runner) declinedStaging(c *aCase) *staging {
	return &staging{r: r, c: c, told: true, declined: true}
}

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
	st.otherwise, st.reran = true, s.plan.reran(st.col.consumes, st.f)
}

// differs reports whether the update does otherwise with the cell than
// this run, or the package can't tell.
func (st *staging) differs() bool {
	st.tell()
	return st.otherwise || st.unowned != nil
}

// close gives a note's closing, and what it claims: the note's default
// where the update does with the cell what this run does, what the update
// does where it does otherwise, and where the package can't tell, that it
// can't.
func (st *staging) close(n note) (string, claims) {
	st.tell()
	var c closing
	switch {
	case st.unowned != nil:
		c.claims = claims{CantTell: st.unowned.name}
		c.advice = "Run it, fix what it reports, and read the diff before committing."
		if n.kind == aboutOrphan || n.kind == aboutProductOrphan {
			c.advice = "Delete nothing by hand before it has run."
		}
	case st.otherwise:
		c = st.tail(n)
		c.claims.Does, c.claims.Another = fateWords(st.f), st.f.runs() && st.ran
		if st.reran != nil {
			c.claims.Reruns = st.reran.name
		} else if st.c.state.made == nil {
			// The update reaches no bytes lift on the cell's path, so a lift
			// before them all blocks the cell there, and this run differs
			// from it by reading the last one's outcome and product.
			last, _ := st.c.state.plan.bytesOnPath(st.col.consumes)
			c.claims.Reads = last.name
		}
	default:
		return st.text(n, n.def), n.def.claims
	}
	if st.c.state.made != nil {
		c.claims.Variant = st.c.tr.name
	}
	return st.text(n, c), c.claims
}

// tail is a note's closing where the update does otherwise with the cell
// than this run, but for what the update does, which close claims: what
// to do, by what the note is about, and what the update writes or leaves.
func (st *staging) tail(n note) closing {
	u := st.r.updateCmd()
	f := st.f
	if f.block != nil && f.block.broken {
		// The update writes nothing for the cell, past the lift whose cell
		// it writes nothing for: what is left to do is that cell's.
		if n.kind == aboutOrphan || n.kind == aboutProductOrphan {
			return closing{advice: fmt.Sprintf("Fix what %s's cell reports, and delete nothing by hand before %s has run.", f.block.lift.name, u)}
		}
		return closing{advice: fmt.Sprintf("Fix what %s's cell reports.", f.block.lift.name)}
	}
	switch n.kind {
	case aboutOutcome:
		switch {
		case f.runs():
			// The normal run doesn't run the cell as the update does, so it
			// can't tell what outcome the update's run gives.
			return closing{claims: claims{RunWrites: true}}
		case f.fault != "":
			return closing{"Fix the probe.", st.faultWrites()}
		}
		return closing{claims: st.writesOutcome()}
	case aboutProbeFault:
		switch {
		case f.runs():
			return closing{claims: claims{RunWrites: true}}
		case st.frozen:
			return closing{claims: claims{Nothing: true, Frozen: true}}
		case f.fault != "":
			return closing{claims: st.faultWrites()}
		}
		return closing{claims: st.writesOutcome()}
	case aboutOrphan:
		switch {
		case f.runs() && st.ran:
			return closing{claims: claims{MayWrite: n.file}}
		case f.runs():
			return closing{advice: "Delete nothing by hand before it has run."}
		case f.fault != "":
			return closing{advice: "Fix the probe."}
		case f.skip != "":
			return closing{"If the suite should run on the case, fix the probe. If not, delete the section by hand;", claims{Leaves: n.file}}
		}
		return closing{fmt.Sprintf("If the cell should run, fix the lift or %s. If not, delete the section by hand;", st.c.inputsAt()), claims{Leaves: n.file}}
	case aboutProductOrphan:
		switch {
		case f.runs() && st.ran:
			return closing{claims: claims{MayWrite: n.file}}
		case f.runs():
			return closing{advice: "Delete nothing by hand before it has run."}
		}
		// A lift's column has no probe, so the update blocks the lift, and
		// replaces its product with none.
		return closing{claims: claims{Removes: n.file}}
	case aboutGap:
		switch {
		case f.skip != "":
			return closing{advice: "Remove the line, or fix the probe."}
		case f.block != nil:
			return closing{advice: fmt.Sprintf("Remove the line, or fix the lift or %s.", st.c.inputsAt())}
		}
	}
	return closing{advice: "Run it, fix what it reports, and read the diff before committing."}
}

// faultWrites is what the update writes for a cell whose probe fails
// there: for a panic, the outcome panicked, and the panic in error.txt;
// for any other fault, nothing.
func (st *staging) faultWrites() claims {
	if !st.f.panicked {
		return claims{Nothing: true}
	}
	cl := st.writes(archive.Outcome{State: archive.Panicked})
	cl.Panic = true
	return cl
}

// writesOutcome is what the update writes for a cell it skips or blocks
// otherwise than this run: that outcome. tail gives the closing past a
// lift whose cell the update writes nothing for.
func (st *staging) writesOutcome() claims {
	if st.f.skip != "" {
		return st.writes(archive.Outcome{State: archive.Skipped, Detail: st.f.skip})
	}
	return st.writes(archive.Outcome{State: archive.Blocked, Detail: st.f.block.failed})
}

// writes claims the update writes the outcome o for the cell, and whether
// this run's outcome, if it has one, differs from it.
func (st *staging) writes(o archive.Outcome) claims {
	return claims{Outcome: outcomeLine(o), Differs: st.outcome != nil && !bytes.Equal(st.outcome, o.Format())}
}

// text renders a closing of a note: what the update does, where the
// closing claims it does otherwise than this run, then the advice, then
// what the update writes or leaves. Each sentence that says what an update
// does is its claims' own. One that follows advice ending with a comma goes
// on from it, in the form it takes there.
func (st *staging) text(n note, c closing) string {
	u := st.r.updateCmd()
	cl := c.claims
	var lead []string
	sep := " "
	switch {
	case cl.CantTell != "":
		how := fmt.Sprintf("a normal run reads %s's outcome and product from the archive", cl.CantTell)
		if s := staged(cl); s != "" {
			how = s + ", and " + how
		}
		lead = append(lead, fmt.Sprintf("Only the package that owns %s runs it in a normal run, so this one can't tell what %s does with %s: %s.", cl.CantTell, u, st.col.name, how))
	case cl.Does != "":
		lead = append(lead, st.does(cl))
		if strings.HasPrefix(cl.Does, fateSkipped+" ") {
			// Its sentence ends with the probe's reason.
			sep = "\n"
		}
	}
	if cl.PastFresh {
		lead = append(lead, fmt.Sprintf("The cells past %s fail in normal runs until the frozen cell is resolved: an update writes them from what %s makes now, and leaves the cell's committed outcome and product in place, which a normal run reads.", st.col.name, st.col.name))
		sep = "\n"
	}
	staged := cl.Does != "" || cl.CantTell != ""
	var says []said
	if cl.RunWrites {
		says = append(says, said{"What it writes for the cell, if anything, is its run's.", "what it writes for the cell, if anything, is its run's."})
	}
	if cl.MayWrite != "" {
		made := "write"
		if n.kind == aboutProductOrphan {
			made = "make"
		}
		says = append(says, said{fmt.Sprintf("Its run may still %s the file: delete nothing by hand before it has run.", made), fmt.Sprintf("its run may still %s the file: delete nothing by hand before it has run.", made)})
	}
	if cl.Outcome != "" {
		s := said{fmt.Sprintf("It writes the outcome %q", cl.Outcome), fmt.Sprintf("it writes the outcome %q", cl.Outcome)}
		if cl.Differs {
			s = s.then(", not this one")
		}
		if cl.Panic {
			s = s.then(", and the panic in " + errorFile)
		}
		says = append(says, s.then("."))
	}
	if cl.Nothing {
		s := said{"An update writes nothing for this cell", "an update writes nothing for this cell"}
		if staged {
			s = said{"It writes nothing for the cell", "it writes nothing for the cell"}
		}
		if cl.Frozen {
			s = s.then(", which a gaps line freezes")
		}
		says = append(says, s.then("."))
	}
	if cl.Writes != "" {
		says = append(says, said{fmt.Sprintf("Run %s to write it, and read it before committing.", u), fmt.Sprintf("run %s to write it, and read it before committing.", u)})
	}
	if cl.OnlyOutcome != "" {
		says = append(says, said{fmt.Sprintf("Run %s to add it. The update adds the outcome and changes nothing else.", u), fmt.Sprintf("run %s to add it. The update adds the outcome and changes nothing else.", u)})
	}
	if cl.Leaves != "" {
		s := fmt.Sprintf("%s leaves it in place.", u)
		says = append(says, said{s, s})
	}
	switch {
	case cl.Removes != "" && staged:
		says = append(says, said{"It removes the file: read the diff before committing.", "it removes the file: read the diff before committing."})
	case cl.Removes != "":
		says = append(says, said{fmt.Sprintf("Run %s, which removes it, and read the diff before committing.", u), fmt.Sprintf("run %s, which removes it, and read the diff before committing.", u)})
	}
	if cl.RemovesInputs != "" {
		says = append(says, said{fmt.Sprintf("Run %s to remove them.", u), fmt.Sprintf("run %s to remove them.", u)})
	}
	if cl.WritesInputs != "" {
		says = append(says, said{fmt.Sprintf("Run %s to write them, and read them before committing.", u), fmt.Sprintf("run %s to write them, and read them before committing.", u)})
	}
	if cl.RewritesInputs != "" {
		says = append(says, said{fmt.Sprintf("The module's update, %s, rewrites them, in the package that owns %s.", u, cl.RewritesInputs), fmt.Sprintf("the module's update, %s, rewrites them, in the package that owns %s.", u, cl.RewritesInputs)})
	}
	if cl.OwnerRewritesInputs != "" {
		// This package can't know some package owns the transform.
		tr := cl.OwnerRewritesInputs
		says = append(says, said{fmt.Sprintf("The module's update, %s, rewrites them, in the package that owns %s. If no package owns %s, no update rewrites the inputs.", u, tr, tr), fmt.Sprintf("the module's update, %s, rewrites them, in the package that owns %s. If no package owns %s, no update rewrites the inputs.", u, tr, tr)})
	}
	var rest []string
	if c.advice != "" {
		rest = append(rest, c.advice)
	}
	for i, s := range says {
		if i == 0 && strings.HasSuffix(c.advice, ",") {
			rest = append(rest, s.joined)
			continue
		}
		rest = append(rest, s.own)
	}
	all := strings.Join(lead, "\n")
	switch {
	case all == "":
		return strings.Join(rest, " ")
	case len(rest) == 0:
		return all
	}
	return all + sep + strings.Join(rest, " ")
}

// A said is a sentence a closing's claims say: own, as it reads on its
// own, and joined, as it reads going on from advice that ends with a
// comma. Each is written whole, and neither is made from the other by
// changing a letter's case, which would change a name or the update
// command that opens it.
type said struct{ own, joined string }

// then is the sentence s, going on with more.
func (s said) then(more string) said { return said{s.own + more, s.joined + more} }

// staged says how the update stages the case otherwise than a normal run,
// as cl claims.
func staged(cl claims) string {
	var how []string
	if cl.Variant != "" {
		how = append(how, fmt.Sprintf("it runs the variant on what %s makes now, where a normal run reads the inputs the archive holds", cl.Variant))
	}
	if cl.Reruns != "" {
		how = append(how, fmt.Sprintf("it reruns %s, whose outcome and product a normal run reads from the archive", cl.Reruns))
	}
	return strings.Join(how, ", and ")
}

// does words what the package's update does with the cell, as cl claims:
// its fate there, and how it stages the case otherwise than this normal
// run. A skip's sentence ends with the probe's reason.
func (st *staging) does(cl claims) string {
	u, col, how := st.r.updateCmd(), st.col.name, staged(cl)
	verb, detail, _ := strings.Cut(cl.Does, " ")
	switch verb {
	case fateRuns:
		if cl.Another {
			return fmt.Sprintf("%s runs %s on another value than this run: %s.", u, col, how)
		}
		return fmt.Sprintf("%s runs %s: %s.", u, col, how)
	case fateFault, fatePanicked:
		return fmt.Sprintf("%s fails %s: %s, and %s.", u, col, how, detail)
	case fateSkipped:
		return fmt.Sprintf("%s skips %s: %s, and %s's probe says the case isn't for it: %s", u, col, how, col, detail)
	case fateBroken:
		return fmt.Sprintf("%s writes nothing for %s: %s, and it writes nothing for the cell of the lift %s, on %s's path, whose outcome and product a normal run reads.", u, col, how, detail, col)
	case fateBlocked:
		// The update runs every lift on the cell's path, so a lift that
		// blocks the cell there failed on the case. One that fails before
		// every bytes lift on the path, with no variant, leaves the update
		// nothing it stages otherwise than this run: what differs is the
		// outcome and product of the last one, which this run reads from
		// the archive, as Reads names it.
		if cl.Reads != "" {
			return fmt.Sprintf("%s blocks %s: the lift %s, on %s's path, failed on the case, before %s, whose outcome and product a normal run reads from the archive.", u, col, detail, col, cl.Reads)
		}
		return fmt.Sprintf("%s blocks %s: %s, and the lift %s, on %s's path, failed on the case.", u, col, how, detail, col)
	}
	panic(fmt.Sprintf("hypercase: no words for what an update does with a cell, %q", cl.Does))
}

// The first word of each kind of fate, as fateWords words it.
const (
	fateRuns     = "runs"
	fateBlocked  = "blocked"
	fateBroken   = "broken"
	fateSkipped  = "skipped"
	fatePanicked = "panicked"
	fateFault    = "fault"
)

// fateWords words a fate: runs, blocked <lift>, broken <lift>, skipped
// <why>, panicked <fault> or fault <fault>. It panics on a fate its words
// don't hold whole, one whose fields no one kind of fate gives.
func fateWords(f updateFate) string {
	var words string
	var whole updateFate
	switch {
	case f.block != nil && f.block.broken:
		words, whole = fateBroken+" "+f.block.lift.name, updateFate{block: f.block}
	case f.block != nil:
		words, whole = fateBlocked+" "+f.block.failed, updateFate{block: f.block}
	case f.skip != "":
		words, whole = fateSkipped+" "+f.skip, updateFate{skip: f.skip}
	case f.panicked:
		words, whole = fatePanicked+" "+f.fault, updateFate{fault: f.fault, panicked: true}
	case f.fault != "":
		words, whole = fateFault+" "+f.fault, updateFate{fault: f.fault}
	default:
		words = fateRuns
	}
	if f != whole {
		panic(fmt.Sprintf("hypercase: a fate whose words, %q, don't hold it whole: %+v", words, f))
	}
	return words
}

// report fails a test with notes, under the header: each note's body, then
// its closing.
func report(t *testing.T, header string, ns notes, st *staging) {
	t.Helper()
	parts := make([]string, len(ns))
	for i, n := range ns {
		closing, cl := st.close(n)
		if observe.note != nil {
			observe.note(st, n, closing, cl)
		}
		parts[i] = n.body + "\n" + closing
	}
	t.Errorf("%s\n%s", header, strings.Join(parts, "\n"))
}

// observe is nil but in hypercase's own tests, which set its functions in
// the test binaries they run as children, to see each cell's result and
// fate in every run, each note's closing, and each run of a lift's
// function, and check what a normal run says the update does against what
// the update did.
var observe struct {
	cell func(c *aCase, col *column, res *cellResult, failed bool)
	note func(st *staging, n note, closing string, cl claims)
	lift func(s *caseState, l *lift)
}

// outcomeLine is an outcome as its file holds it, less its newline.
func outcomeLine(o archive.Outcome) string {
	return strings.TrimSuffix(string(o.Format()), "\n")
}
