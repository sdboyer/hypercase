package hypercase

import (
	"io"
	"io/fs"
	"path"
	"reflect"
	"slices"
	"testing"

	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/erase"
)

// A File is one output of a run: its path under the column's directory,
// out/<column>/, and its bytes.
type File struct {
	Path string
	Data []byte
}

// A Suite is a column of the matrix: it covers a class of outputs, or one
// command. T is the type the suite consumes: a corpus type, for the raw
// trees, or a stage, the product of a lift. The suite runs over every
// archive whose corpus type reaches T through the registered lifts, and
// its outputs for a case are that cell's files, under out/<Name>/. A
// Suite[fs.FS] runs over the archives of the directory its Corpus names,
// on their raw input views, with no corpus type.
type Suite[T any] struct {
	// Name is the column's name, and names its directory, out/<Name>/.
	// Column names are unique per corpus root, lifts' and suites' together.
	Name string
	// Description says what the suite does. It is required.
	Description string
	// Corpus names the directory of the archives a Suite[fs.FS] runs over,
	// testdata/<Corpus>/ under the corpus root, which no corpus type
	// declares. A suite that consumes any other type names none: the type
	// says which archives it runs over.
	Corpus string
	// Probe decides whether a case is for the suite, from the value of the
	// type it consumes alone. When it says no, it gives the reason, and the
	// cell is skipped: it runs nothing. A suite with no probe is for every
	// case.
	Probe func(in T) (why string, ok bool)
	// Outputs declares every file the suite's cells may write, stdout.txt,
	// stderr.txt and error.txt included. A file no declaration matches fails
	// its cell, and so does one that two declarations match.
	Outputs []decl.Output
	// Rejects lists the properties the suite rejects. A cell is expected to
	// err exactly when its case states one of them.
	Rejects []decl.Property
	// Run runs one case. It receives the cell's testing.TB, the archive's
	// name, the value of the type the suite consumes and one writer each
	// for stdout and stderr, and returns its output files and the error the
	// code under test returned. What it writes to stdout and stderr is
	// compared as stdout.txt and stderr.txt. It reports its own failures,
	// such as a fixture it can't set up, through the testing.TB.
	Run func(t testing.TB, archive string, in T, stdout, stderr io.Writer) ([]File, error)
	// Classifier refines the outcomes of the suite's cells that ran and
	// returned. A suite with none leaves them unrefined.
	Classifier Classifier[T]
}

// column is a column with its type erased: a suite's, or a lift's.
type column struct {
	name        string
	description string
	corpus      string // a Suite[fs.FS]'s Corpus
	outputs     []decl.Output
	rejects     []decl.Property
	consumes    reflect.Type
	probe       func(in any) (string, bool)
	run         func(t testing.TB, archive string, in any, stdout, stderr io.Writer) ([]File, error)
	classifier  *classifier // a suite's, or nil
	lift        *lift       // the column's lift, for a lift's column
}

// kind names what the column is, in a message.
func (c *column) kind() string {
	if c.lift != nil {
		return "lift"
	}
	return "suite"
}

// describe names the column and what it consumes, in a message.
func (c *column) describe() string {
	if c.lift != nil {
		return c.lift.describe()
	}
	if c.corpus != "" {
		return "the suite " + c.name + ", over the raw input views of testdata/" + c.corpus + "/"
	}
	return "the suite " + c.name + ", over " + c.consumes.String()
}

// rejectsAny returns the properties of names the column rejects, in the
// order the names are given.
func (c *column) rejectsAny(names []string) []string {
	var hit []string
	for _, n := range names {
		for _, p := range c.rejects {
			if p.Name == n {
				hit = append(hit, n)
			}
		}
	}
	return hit
}

// declared returns the patterns of the column's output declarations that
// match a file's path.
func (c *column) declared(p string) []string {
	var hit []string
	for _, o := range c.outputs {
		if ok, _ := path.Match(o.Pattern, p); ok {
			hit = append(hit, o.Pattern)
		}
	}
	return hit
}

// columnDecl is what Own accepts as a suite.
type columnDecl interface {
	column() *column
}

func (s Suite[T]) column() *column {
	c := &column{
		name:        s.Name,
		description: s.Description,
		corpus:      s.Corpus,
		outputs:     slices.Clone(s.Outputs),
		rejects:     slices.Clone(s.Rejects),
		consumes:    reflect.TypeFor[T](),
		classifier:  s.Classifier.erase(),
	}
	if s.Probe != nil {
		probe := s.Probe
		c.probe = func(in any) (string, bool) { return probe(in.(T)) }
	}
	if s.Run != nil {
		run := s.Run
		c.run = func(t testing.TB, archive string, in any, stdout, stderr io.Writer) ([]File, error) {
			return run(t, archive, in.(T), stdout, stderr)
		}
	}
	return c
}

// A corpusType is a registered decl.Type.
type corpusType struct {
	dir         string
	description string
	goType      reflect.Type
	new         func(view fs.FS) any
	transforms  []*transform
}

// A lift is a registered decl.Lift, owned or used.
type lift struct {
	name        string
	description string
	outputs     []decl.Output
	rejects     []decl.Property
	in, out     reflect.Type
	run         func(archive string, in any, stderr io.Writer) (any, error)
	jenny       bool
	owned       bool
}

func newLift(e *erase.Lift, owned bool) *lift {
	l := &lift{
		name:        e.Name,
		description: e.Description,
		in:          e.In,
		out:         e.Out,
		run:         e.Run,
		jenny:       e.Jenny,
		owned:       owned,
	}
	for _, o := range e.Outputs {
		l.outputs = append(l.outputs, decl.Output(o))
	}
	for _, p := range e.Rejects {
		l.rejects = append(l.rejects, decl.Property(p))
	}
	return l
}

func (l *lift) describe() string {
	return "the lift " + l.name + ", from " + l.in.String() + " to " + l.out.String()
}

// column is the lift's column, in the package that owns it.
func (l *lift) column() *column {
	return &column{
		name:        l.name,
		description: l.description,
		outputs:     l.outputs,
		rejects:     l.rejects,
		consumes:    l.in,
		lift:        l,
	}
}

var fsType = reflect.TypeFor[fs.FS]()

// isCorpusShape reports whether t is shaped as a corpus type: a struct
// whose one field is an embedded fs.FS.
func isCorpusShape(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && t.NumField() == 1 && t.Field(0).Anonymous && t.Field(0).Type == fsType
}
