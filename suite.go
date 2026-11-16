package hypercase

import (
	"io"
	"io/fs"
	"path"
	"reflect"
	"slices"
	"testing"

	"github.com/sdboyer/hypercase/decl"
)

// A File is one output of a run: its path under the column's directory,
// out/<column>/, and its bytes.
type File struct {
	Path string
	Data []byte
}

// A Suite is a column of the matrix: it covers a class of outputs, or one
// command. Each case of its corpus runs through it, and its outputs for a
// case are that cell's files, under out/<Name>/. T is the type the suite
// consumes.
type Suite[T any] struct {
	// Name is the column's name, and names its directory, out/<Name>/.
	// Column names are unique per corpus root.
	Name string
	// Description says what the suite does. It is required.
	Description string
	// Corpus names the directory of the archives the suite runs over,
	// testdata/<Corpus>/ under the corpus root.
	Corpus string
	// Probe decides whether a case is for the suite, from the case's input
	// alone. When it says no, it gives the reason, and the cell is skipped:
	// it runs nothing. A suite with no probe is for every case.
	Probe func(in T) (why string, ok bool)
	// Outputs declares every file the suite's cells may write, stdout.txt,
	// stderr.txt and error.txt included. A file no declaration matches fails
	// its cell, and so does one that two declarations match.
	Outputs []decl.Output
	// Rejects lists the properties the suite rejects. A cell is expected to
	// err exactly when its case states one of them.
	Rejects []decl.Property
	// Run runs one case. It receives the cell's testing.TB, the archive's
	// name, the case's input and one writer each for stdout and stderr, and
	// returns its output files and the error the code under test returned.
	// What it writes to stdout and stderr is compared as stdout.txt and
	// stderr.txt. It reports its own failures, such as a fixture it can't
	// set up, through the testing.TB.
	Run func(t testing.TB, archive string, in T, stdout, stderr io.Writer) ([]File, error)
}

// column is a column with its type erased.
type column struct {
	name        string
	description string
	corpus      string
	outputs     []decl.Output
	rejects     []decl.Property
	consumes    reflect.Type
	probe       func(in any) (string, bool)
	run         func(t testing.TB, archive string, in any, stdout, stderr io.Writer) ([]File, error)
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

// columnDecl is what Register accepts as a column.
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

var fsType = reflect.TypeFor[fs.FS]()
