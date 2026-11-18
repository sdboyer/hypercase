package hypercase

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/revmap"
)

// A statement is what an output declaration states its files' ranges come
// from, in its Inputs.
type statement int

const (
	// statesNone is a declaration that states nothing: a nil Inputs, or a
	// nil InputsFunc.
	statesNone statement = iota
	// statesFunc maps each file with an InputsFunc.
	statesFunc
	// statesAll states AllInputs.
	statesAll
	// statesPositions states FromPositions.
	statesPositions
	// statesOther is a value of the kind of AllInputs and FromPositions
	// that is neither, as arithmetic on them makes.
	statesOther
)

// stated is what an Inputs states.
func stated(in decl.Inputs) statement {
	switch f := in.(type) {
	case nil:
		return statesNone
	case decl.InputsFunc:
		if f == nil {
			return statesNone
		}
		return statesFunc
	}
	switch in {
	case decl.AllInputs:
		return statesAll
	case decl.FromPositions:
		return statesPositions
	}
	return statesOther
}

// checkInputs refuses what a column's output declarations state in Inputs
// that no declaration may: a value that is none of an InputsFunc,
// AllInputs and FromPositions; FromPositions on a pattern that isn't
// error.txt; and an InputsFunc on a pattern that matches error.txt, whose
// ranges come from Positions or AllInputs. A declaration that states
// nothing is left to the run, which fails on every such declaration at
// once.
func checkInputs(kind, name string, outputs []decl.Output) {
	for _, o := range outputs {
		matches, _ := path.Match(o.Pattern, errorFile)
		switch stated(o.Inputs) {
		case statesOther:
			refuse(fmt.Sprintf("the %s %s declares the output %q with an Inputs that is none of decl.InputsFunc, decl.AllInputs and decl.FromPositions. State one of them.", kind, name, o.Pattern))
		case statesPositions:
			if o.Pattern != errorFile {
				refuse(fmt.Sprintf("the %s %s declares the output %q with Inputs: decl.FromPositions, which only a declaration whose pattern is %s states: Positions finds where an error points, and only %s holds one. Give it a decl.InputsFunc or decl.AllInputs.", kind, name, o.Pattern, errorFile, errorFile))
			}
		case statesFunc:
			switch {
			case o.Pattern == errorFile:
				refuse(fmt.Sprintf("the %s %s declares the output %s with a decl.InputsFunc. Its ranges come from where the error points, through the project's Positions, or from the whole input view: state decl.FromPositions or decl.AllInputs.", kind, name, errorFile))
			case matches:
				refuse(fmt.Sprintf("the %s %s declares the output %q with a decl.InputsFunc, and the pattern matches %s, whose ranges come from the project's Positions or the whole input view, never from a function. State decl.AllInputs, or change the pattern so it doesn't match %s and declare %s on its own, with decl.FromPositions or decl.AllInputs.", kind, name, o.Pattern, errorFile, errorFile, errorFile))
			}
		}
	}
}

// checkProductInputs refuses what a bytes lift's product declarations
// state in Inputs that none may: a value that is none of an InputsFunc,
// AllInputs and FromPositions, and FromPositions, since no product file is
// an error.txt.
func checkProductInputs(l *lift) {
	for _, o := range l.product {
		switch stated(o.Inputs) {
		case statesOther:
			refuse(fmt.Sprintf("the lift %s declares the product file %q with an Inputs that is none of decl.InputsFunc, decl.AllInputs and decl.FromPositions. State a decl.InputsFunc or decl.AllInputs.", l.name, o.Pattern))
		case statesPositions:
			refuse(fmt.Sprintf("the lift %s declares the product file %q with Inputs: decl.FromPositions, and Positions finds where an error points, which a product file never holds. Give it a decl.InputsFunc or decl.AllInputs.", l.name, o.Pattern))
		}
	}
}

// An origin is an input view, and where the archive commits each of its
// files: the section a range of the file names. A view's file is
// committed where a normal run reads it from, or, for a view an update
// stages, where the update writes it: a base's files under in/; a
// variant's own files, those its in/ sections hold, under the variant's
// in/, and the files it keeps of the base's under in/; and, past a bytes
// lift, every file of the product the cells read under the lift's via/.
type origin struct {
	view fs.FS
	// dir is the prefix of the sections of the view's files but its own:
	// in/, or, past a bytes lift, its product's, <prefix>via/<lift>/.
	dir string
	// over is the prefix of a variant's own in/ sections, <prefix>in/,
	// and own the paths of the files they hold.
	over string
	own  map[string]bool
}

// section names the section that holds a file of the view.
func (o *origin) section(p string) string {
	if o.own[p] {
		return o.over + p
	}
	return o.dir + p
}

// baseOrigin is the origin of a base's input view.
func baseOrigin(view fs.FS) *origin {
	return &origin{view: view, dir: archive.In + "/"}
}

// overlayOrigin is the origin of the view of a variant whose sections
// start with prefix, laid over the base: its own files are the paths of
// overlay. A hand-written variant's are its in/ sections' paths, a file
// that holds the base's bytes among them, and so are a transform's whose
// inputs a normal run reads.
func overlayOrigin[V any](view fs.FS, prefix string, overlay map[string]V) *origin {
	own := map[string]bool{}
	for p := range overlay {
		own[p] = true
	}
	return &origin{view: view, dir: archive.In + "/", over: prefix + archive.In + "/", own: own}
}

// madeOrigin is the origin of the view a transform makes, which an update
// stages its variant on, as do the normal runs of a variant whose archive
// holds none of its inputs: its own files are those the sections that
// record it hold, the paths whose bytes differ from the base's or that
// the base lacks. A file the transform makes with the base's bytes is the
// base's.
func madeOrigin(v *plannedVariant, prefix string) *origin {
	own := map[string]bool{}
	for _, s := range v.sections {
		if p, ok := strings.CutPrefix(s.Name, prefix+archive.In+"/"); ok {
			own[p] = true
		}
	}
	return &origin{view: archive.NewFS(v.made), dir: archive.In + "/", over: prefix + archive.In + "/", own: own}
}

// viaOrigin is the origin of a bytes lift's product, in a case whose
// sections start with prefix: every file is under its via/.
func viaOrigin(l *lift, prefix string, product map[string][]byte) *origin {
	return &origin{view: archive.NewFS(product), dir: l.viaDir(prefix)}
}

// A rangeFault is what keeps a range from naming lines of the input view.
type rangeFault int

const (
	rangeHeld     rangeFault = iota // no fault: the view holds the lines
	rangeNoPath                     // the range names no file
	rangeInvalid                    // its path isn't a valid slash path
	rangeDir                        // its path is a directory of the view
	rangeAbsent                     // the view has no file at its path
	rangeNegative                   // a line number is negative
	rangeHalf                       // one of Start and End is 0, and not both
	rangeBackward                   // End is before Start
	rangePast                       // End is past the file's last line
)

// says is the fault, in a message about the range r, whose file has
// lines lines.
func (f rangeFault) says(r decl.Range, lines int) string {
	switch f {
	case rangeNoPath:
		return "it names no file: its Path is empty"
	case rangeInvalid:
		return fmt.Sprintf("its Path, %q, is no valid slash path of a view: one that is unrooted, with no empty, . or .. element", r.Path)
	case rangeDir:
		return fmt.Sprintf("%s is a directory of the input view the cell ran on, not a file", r.Path)
	case rangeAbsent:
		return fmt.Sprintf("the input view the cell ran on has no file %s", r.Path)
	case rangeNegative:
		return fmt.Sprintf("its Start, %d, or its End, %d, is negative", r.Start, r.End)
	case rangeHalf:
		return fmt.Sprintf("its Start is %d and its End %d: Start and End both 0 is the whole file, and lines count from 1", r.Start, r.End)
	case rangeBackward:
		return fmt.Sprintf("its End, %d, is before its Start, %d", r.End, r.Start)
	case rangePast:
		return fmt.Sprintf("its End, %d, is past the last line of %s, which has %s", r.End, r.Path, lineWords(lines))
	}
	panic(fmt.Sprintf("hypercase: no words for the range fault %d", f))
}

// lineWords is a number of lines, in words.
func lineWords(n int) string {
	switch n {
	case 0:
		return "none"
	case 1:
		return "1 line"
	}
	return fmt.Sprintf("%d lines", n)
}

// check checks a range against the view, the view the cell ran on, never
// the archive's bytes, and returns it as the inputs file records it, in
// the section that holds its file. When the view holds no such lines, it
// returns the fault, with the number of lines of the range's file, where
// it has one.
func (o *origin) check(r decl.Range) (revmap.Range, rangeFault, int) {
	switch {
	case r.Path == "":
		return revmap.Range{}, rangeNoPath, 0
	case !fs.ValidPath(r.Path):
		return revmap.Range{}, rangeInvalid, 0
	}
	info, err := fs.Stat(o.view, r.Path)
	switch {
	case err != nil:
		return revmap.Range{}, rangeAbsent, 0
	case info.IsDir():
		return revmap.Range{}, rangeDir, 0
	}
	data, err := fs.ReadFile(o.view, r.Path)
	if err != nil {
		panic(fmt.Sprintf("hypercase: an input view states the file %s, and can't read it: %v", r.Path, err))
	}
	n := lineCount(data)
	switch {
	case r.Start < 0 || r.End < 0:
		return revmap.Range{}, rangeNegative, n
	case r.Start == 0 && r.End == 0:
	case r.Start == 0 || r.End == 0:
		return revmap.Range{}, rangeHalf, n
	case r.End < r.Start:
		return revmap.Range{}, rangeBackward, n
	case r.End > n:
		return revmap.Range{}, rangePast, n
	}
	return revmap.Range{Section: o.section(r.Path), Start: r.Start, End: r.End}, rangeHeld, n
}

// lineCount is the number of lines of a file: those that end with a
// newline, and a last one that doesn't, if any. No file of an input view
// lacks a final newline, since an archive's last section gains one when
// it is read, and an archive must hold every file a jenny makes, but the
// count doesn't depend on that.
func lineCount(data []byte) int {
	n := bytes.Count(data, []byte("\n"))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++
	}
	return n
}
