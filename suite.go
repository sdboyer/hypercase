package hypercase

import (
	"io"
	"io/fs"
	"reflect"
	"testing"
)

// A File is one output of a run: its path under the column's directory,
// out/<column>/, and its bytes.
type File struct {
	Path string
	Data []byte
}

// A Suite is a column of the matrix: every case runs through it, and its
// outputs for a case are that cell's files, under out/<Name>/. T is the
// type the suite consumes.
type Suite[T any] struct {
	// Name is the column's name, and names its directory, out/<Name>/.
	Name string
	// Description says what the suite does. It is required.
	Description string
	// Run runs one case. It receives the cell's testing.TB, the archive's
	// name, the case's input and one writer each for stdout and stderr, and
	// returns its output files and the error the code under test returned.
	// It reports its own failures, such as a fixture it can't set up,
	// through the testing.TB.
	Run func(t testing.TB, archive string, in T, stdout, stderr io.Writer) ([]File, error)
}

// column is a column with its type erased.
type column struct {
	name        string
	description string
	consumes    reflect.Type
	run         func(t testing.TB, archive string, in any, stdout, stderr io.Writer) ([]File, error)
}

// columnDecl is what Register accepts as a column.
type columnDecl interface {
	column() *column
}

func (s Suite[T]) column() *column {
	c := &column{
		name:        s.Name,
		description: s.Description,
		consumes:    reflect.TypeFor[T](),
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
