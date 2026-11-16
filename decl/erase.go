package decl

import (
	"io"
	"io/fs"
	"reflect"

	"github.com/sdboyer/hypercase/internal/erase"
)

func init() { erase.Of = of }

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
	return &erase.Type{
		Dir:         t.Dir,
		Description: t.Description,
		GoType:      reflect.TypeFor[T](),
		New:         func(view fs.FS) any { return T(struct{ fs.FS }{view}) },
	}
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
