package hypercase

import (
	"bytes"
	"maps"
	"slices"
)

// A Classifier refines a suite's outcomes: it reads what a cell's run
// produced, and gives a detail that follows the outcome's state, as in
// accepted div-zero. It can't change the state, and runs only on cells
// that ran and returned, accepted or rejected. A suite has at most one.
type Classifier[T any] struct {
	// Name names the classifier, under the naming rule.
	Name string
	// Description says what the classifier's details mean. It is required.
	Description string
	// Classify returns the cell's detail, or "" for none, from the value
	// the suite consumed and what its run produced. A detail is one line,
	// with no space at either end. An error, or a panic, fails the cell,
	// and an update then writes nothing for it.
	Classify func(in T, got Result) (string, error)
}

// A Result is what a cell's run produced, as a classifier reads it: the
// cell's files, the framework's among them, and the error the run
// returned.
type Result struct {
	files map[string][]byte
	// Err is the error the run returned, or nil for an accepted cell.
	Err error
}

// File returns a copy of the cell's file at a path under out/<column>/,
// such as values.txt or stdout.txt, or nil when the run produced none
// there.
func (r Result) File(path string) []byte {
	data, ok := r.files[path]
	if !ok {
		return nil
	}
	return append([]byte{}, data...)
}

// Paths returns the paths of the cell's files, sorted.
func (r Result) Paths() []string { return slices.Sorted(maps.Keys(r.files)) }

// classifier is a suite's classifier, with its type erased.
type classifier struct {
	name        string
	description string
	classify    func(in any, got Result) (string, error)
}

func (c Classifier[T]) erase() *classifier {
	if c.Name == "" && c.Description == "" && c.Classify == nil {
		return nil
	}
	e := &classifier{name: c.Name, description: c.Description}
	if c.Classify != nil {
		classify := c.Classify
		e.classify = func(in any, got Result) (string, error) { return classify(in.(T), got) }
	}
	return e
}

// newResult is a cell's result, from its files.
func newResult(files map[string][]byte, err error) Result {
	r := Result{files: map[string][]byte{}, Err: err}
	for p, data := range files {
		r.files[p] = bytes.Clone(data)
	}
	return r
}
