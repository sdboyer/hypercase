package hypercase

import (
	"encoding/json"
	"io/fs"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/sdboyer/hypercase/archive"
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

// Claims are what a closing claims the update does, as an Observed
// records them.
type Claims = claims

// An Observed is one record of a child's run: a cell's result and fate,
// or a note's closing.
type Observed struct {
	Record string `json:"record"` // "cell" or "note"
	Update bool   `json:"update"`
	Pkg    string `json:"pkg"` // the package's directory, from the module root
	Case   string `json:"case"`
	Prefix string `json:"prefix"` // what the case's sections' names start with
	Column string `json:"column"` // "" for a note of a variant about no one cell

	// A cell's.
	Fate     string            `json:"fate,omitempty"` // as FateString words it
	Ran      bool              `json:"ran,omitempty"`
	Writable bool              `json:"writable,omitempty"`
	Files    map[string]string `json:"files,omitempty"`
	Via      bool              `json:"via,omitempty"`
	Product  map[string]string `json:"product,omitempty"`
	Frozen   bool              `json:"frozen,omitempty"`
	// OnValue records that the cell ran on a value, View the files of the
	// input view the value comes from, Root those of the case's input view,
	// and Path the bytes lifts on the cell's path, the last first.
	OnValue bool              `json:"onValue,omitempty"`
	View    map[string]string `json:"view,omitempty"`
	Root    map[string]string `json:"root,omitempty"`
	Path    []string          `json:"path,omitempty"`
	// Failed records that the cell's test failed.
	Failed bool `json:"failed,omitempty"`

	// A note's.
	Kind    int    `json:"kind,omitempty"` // a noteKind, which NoteKinds names
	File    string `json:"file,omitempty"` // the section a note about one file is about
	Body    string `json:"body,omitempty"`
	Closing string `json:"closing,omitempty"`
	Default string `json:"default,omitempty"`
	// Form is "default" where the closing is the note's default, "staged"
	// where it says what the update does, and "cantTell".
	Form string `json:"form,omitempty"`
	// This is the cell's fate in this run, Claims what the closing claims,
	// and Declined records a note of a variant its transform declines.
	This     string  `json:"this,omitempty"`
	Claims   *Claims `json:"claims,omitempty"`
	Declined bool    `json:"declined,omitempty"`
}

// NoteKinds names each noteKind the package declares, by its value, as
// its constant's name less "about", with a lower-case first letter:
// aboutProbeFault is probeFault. The type checker finds them, so a kind
// the package adds is named with no table to keep.
func NoteKinds(t *testing.T) map[int]string {
	t.Helper()
	out := map[int]string{}
	for v, name := range loadSource(t).constants("noteKind") {
		rest, ok := strings.CutPrefix(name, "about")
		if !ok || rest == "" {
			t.Fatalf("the noteKind %s isn't named about<Kind>", name)
		}
		out[int(v)] = strings.ToLower(rest[:1]) + rest[1:]
	}
	return out
}

// FateVerbs are the first words of each kind of fate, the string
// constants fateWords words fates with, as the type checker finds them.
func FateVerbs(t *testing.T) []string {
	t.Helper()
	return loadSource(t).stringsUsedIn(t, "fateWords")
}

// FateString words a fate in an Observed, as fateWords does.
func FateString(f updateFate) string { return fateWords(f) }

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
	files := func(view fs.FS) map[string]string {
		if view == nil {
			return nil
		}
		paths, err := archive.Paths(view)
		if err != nil {
			panic(err)
		}
		out := map[string]string{}
		for _, p := range paths {
			data, err := fs.ReadFile(view, p)
			if err != nil {
				panic(err)
			}
			out[p] = string(data)
		}
		return out
	}
	observe.cell = func(c *aCase, col *column, res *cellResult, failed bool) {
		r := c.state.r
		_, marked := c.gap(col.name)
		var path []string
		for typ := col.consumes; typ != c.state.plan.root.goType; {
			l := c.state.plan.producer[typ]
			if l.bytes {
				path = append(path, l.name)
			}
			typ = l.in
		}
		write(Observed{
			Record: "cell", Update: r.update, Pkg: r.rel(r.pkgDir), Case: c.name, Prefix: c.prefix, Column: col.name,
			Fate: FateString(res.fate), Ran: res.ran, Writable: res.writable, Files: strs(res.files),
			Via: res.via, Product: strs(res.product), Frozen: c.committed(col).frozen(marked),
			OnValue: res.onValue, View: files(res.view), Root: files(c.state.view), Path: path, Failed: failed,
		})
	}
	observe.note = func(st *staging, n note, closing string, cl claims) {
		o := Observed{
			Record: "note", Update: st.r.update, Pkg: st.r.rel(st.r.pkgDir), Case: st.c.name, Prefix: st.c.prefix,
			Kind: int(n.kind), File: n.file, Body: n.body, Closing: closing, Default: st.text(n, n.def), Frozen: st.frozen,
			Claims: &cl, Declined: st.declined,
		}
		if st.col != nil {
			o.Column, o.This, o.Ran = st.col.name, FateString(st.this), st.ran
		}
		switch {
		case cl.CantTell != "":
			o.Form = "cantTell"
		case cl.Does != "":
			o.Form = "staged"
		default:
			o.Form = "default"
		}
		write(o)
	}
}
