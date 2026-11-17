package hypercase

import (
	"encoding/json"
	"os"
	"sync"
)

// The self-tests run a child with reverseEnv set to 1 to run each case's
// cells in reverse column-name order.
const reverseEnv = "HYPERCASE_SELFTEST_REVERSE"

// The self-tests run a child with observeEnv set to a file's path to have
// it append, one JSON object to a line, each cell's result and fate, and
// each note's closing: the records TestClosings reads.
const observeEnv = "HYPERCASE_SELFTEST_OBSERVE"

func init() {
	reverseCells = os.Getenv(reverseEnv) == "1"
	if p := os.Getenv(observeEnv); p != "" {
		observeTo(p)
	}
}

// An Observed is one record of a child's run: a cell's result and fate,
// or a note's closing.
type Observed struct {
	Record string `json:"record"` // "cell" or "note"
	Update bool   `json:"update"`
	Pkg    string `json:"pkg"` // the package's directory, from the module root
	Case   string `json:"case"`
	Prefix string `json:"prefix"` // what the case's sections' names start with
	Column string `json:"column"` // "" for a note of a variant its transform declines

	// A cell's.
	Fate     string            `json:"fate,omitempty"` // as FateString words it
	Ran      bool              `json:"ran,omitempty"`
	Writable bool              `json:"writable,omitempty"`
	Files    map[string]string `json:"files,omitempty"`
	Via      bool              `json:"via,omitempty"`
	Product  map[string]string `json:"product,omitempty"`
	Frozen   bool              `json:"frozen,omitempty"`

	// A note's.
	Kind    string `json:"kind,omitempty"`
	Body    string `json:"body,omitempty"`
	Closing string `json:"closing,omitempty"`
	Default string `json:"default,omitempty"`
	// Form is "default" where the closing is the note's default, "staged"
	// where it says what the update does, and "cantTell".
	Form string `json:"form,omitempty"`
	// Told is the update's fate the staging told, for a staged closing,
	// This the cell's fate in this run, and Unowned the bytes lift that
	// keeps the package from telling.
	Told    string `json:"told,omitempty"`
	This    string `json:"this,omitempty"`
	Unowned string `json:"unowned,omitempty"`
}

// KindNames name each noteKind in an Observed.
var KindNames = map[noteKind]string{
	aboutFault:         "fault",
	aboutFile:          "file",
	aboutOutcome:       "outcome",
	aboutOrphan:        "orphan",
	aboutProductOrphan: "productOrphan",
	aboutGap:           "gap",
	aboutProbeFault:    "probeFault",
	aboutFrozen:        "frozen",
}

// FateString words a fate in an Observed: runs, blocked <lift>, broken
// <lift>, skipped <why>, panicked <fault> or fault <fault>.
func FateString(f updateFate) string {
	switch {
	case f.block != nil && f.block.broken:
		return "broken " + f.block.lift.name
	case f.block != nil:
		return "blocked " + f.block.failed
	case f.skip != "":
		return "skipped " + f.skip
	case f.panicked:
		return "panicked " + f.fault
	case f.fault != "":
		return "fault " + f.fault
	}
	return "runs"
}

func observeTo(path string) {
	var mu sync.Mutex
	write := func(o Observed) {
		mu.Lock()
		defer mu.Unlock()
		data, err := json.Marshal(o)
		if err != nil {
			panic(err)
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		if _, err := f.Write(append(data, '\n')); err != nil {
			panic(err)
		}
	}
	strs := func(m map[string][]byte) map[string]string {
		if m == nil {
			return nil
		}
		out := map[string]string{}
		for k, v := range m {
			out[k] = string(v)
		}
		return out
	}
	observe.cell = func(c *aCase, col *column, res *cellResult) {
		r := c.state.r
		_, marked := c.gap(col.name)
		write(Observed{
			Record: "cell", Update: r.update, Pkg: r.rel(r.pkgDir), Case: c.name, Prefix: c.prefix, Column: col.name,
			Fate: FateString(res.fate), Ran: res.ran, Writable: res.writable, Files: strs(res.files),
			Via: res.via, Product: strs(res.product), Frozen: c.committed(col).frozen(marked),
		})
	}
	observe.note = func(st *staging, n note, closing string) {
		o := Observed{
			Record: "note", Update: st.r.update, Pkg: st.r.rel(st.r.pkgDir), Case: st.c.name, Prefix: st.c.prefix,
			Kind: KindNames[n.kind], Body: n.body, Closing: closing, Default: n.def, Frozen: st.frozen,
		}
		if st.col != nil {
			o.Column, o.This, o.Ran = st.col.name, FateString(st.this), st.ran
		}
		switch {
		case st.unowned != nil:
			o.Form, o.Unowned = "cantTell", st.unowned.name
		case st.does != "":
			o.Form, o.Told = "staged", FateString(st.f)
		default:
			o.Form = "default"
		}
		write(o)
	}
}
