package decl

import (
	"io"
	"io/fs"
	"reflect"

	"github.com/sdboyer/hypercase/internal/erase"
	"github.com/sdboyer/jenny"
)

func init() {
	erase.Of = of
	erase.IsRejection = isRejection
}

// maxUnwraps bounds isRejection's walk: an error reached through more
// Unwrap calls than this makes the whole error a fault, so an error whose
// Unwrap leads back to itself ends the walk.
const maxUnwraps = 100

// isRejection reports whether Reject marks all of err. It walks err's
// tree, through Unwrap() error and Unwrap() []error, never an error's own
// As or Is, and stops at each error Reject made, whose own Unwrap leads to
// the unmarked error it marks: a path that reaches an error that wraps
// none without passing one is unmarked, and makes err a fault, and so is
// a path longer than maxUnwraps. So a join is a rejection when each of its
// errors is, a join of one included, as jenny joins the error of a jenny
// with one input.
func isRejection(err error) bool { return marked(err, 0) }

// marked is isRejection's walk, at an error reached through unwraps
// Unwrap calls.
func marked(err error, unwraps int) bool {
	if _, ok := err.(*rejection); ok {
		return true
	}
	if unwraps == maxUnwraps {
		return false
	}
	switch e := err.(type) {
	case interface{ Unwrap() error }:
		return marked(e.Unwrap(), unwraps+1)
	case interface{ Unwrap() []error }:
		errs := e.Unwrap()
		for _, err := range errs {
			if !marked(err, unwraps+1) {
				return false
			}
		}
		return len(errs) > 0
	}
	return false
}

// erasable is implemented by the generic declarations, and only by them.
type erasable interface{ erase() any }

func of(v any) (any, bool) {
	e, ok := v.(erasable)
	if !ok {
		return nil, false
	}
	return e.erase(), true
}

func (t Type[T]) erase() any {
	e := &erase.Type{
		Dir:         t.Dir,
		Description: t.Description,
		GoType:      reflect.TypeFor[T](),
		New:         func(view fs.FS) any { return T(struct{ fs.FS }{view}) },
	}
	for _, tr := range t.Transforms {
		e.Transforms = append(e.Transforms, tr.erase().(*erase.Transform))
	}
	return e
}

func (t Transform[T]) erase() any {
	e := &erase.Transform{
		Name:        t.Name,
		Description: t.Description,
		GoType:      reflect.TypeFor[T](),
	}
	for _, p := range t.Gives {
		e.Gives = append(e.Gives, erase.Property(p))
	}
	for _, p := range t.Keeps {
		e.Keeps = append(e.Keeps, erase.Property(p))
	}
	if t.Probe != nil {
		probe := t.Probe
		e.Probe = func(view fs.FS) (string, bool) { return probe(T(struct{ fs.FS }{view})) }
	}
	if !reflect.ValueOf(t.Jenny).IsZero() {
		j := t.Jenny
		e.Generate = func(view fs.FS) (*jenny.FS, error) { return j.GenerateFS(T(struct{ fs.FS }{view})) }
	}
	return e
}

func (l Lift[In, Out]) erase() any {
	e := &erase.Lift{
		Name:        l.Name,
		Description: l.Description,
		In:          reflect.TypeFor[In](),
		Out:         reflect.TypeFor[Out](),
		Jenny:       !reflect.ValueOf(l.Jenny).IsZero(),
	}
	for _, o := range l.Outputs {
		e.Outputs = append(e.Outputs, o.erased())
	}
	for _, o := range l.Product {
		e.Product = append(e.Product, o.erased())
	}
	if e.Jenny {
		j := l.Jenny
		e.Generate = func(in any) (*jenny.FS, error) { return j.GenerateFS(in.(In)) }
	}
	for _, p := range l.Rejects {
		e.Rejects = append(e.Rejects, erase.Property(p))
	}
	if l.Run != nil {
		run := l.Run
		e.Run = func(archive string, in any, stderr io.Writer) (any, error) {
			return run(archive, in.(In), stderr)
		}
	}
	return e
}

// erased is the output declaration as a lift's erased form holds it, with
// its Inputs as an opaque value: erase can't import decl, which imports it.
func (o Output) erased() erase.Output {
	return erase.Output{Pattern: o.Pattern, Description: o.Description, Inputs: o.Inputs}
}
