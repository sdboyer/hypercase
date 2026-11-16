package decl

import (
	"errors"
	"io"
	"io/fs"
	"reflect"

	"github.com/sdboyer/hypercase/internal/erase"
	"github.com/sdboyer/jenny"
)

func init() {
	erase.Of = of
	erase.IsRejection = func(err error) bool {
		var r *rejection
		return errors.As(err, &r)
	}
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
		e.Outputs = append(e.Outputs, erase.Output(o))
	}
	for _, o := range l.Product {
		e.Product = append(e.Product, erase.Output(o))
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
