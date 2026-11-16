// Package erase holds the type-erased forms of decl's generic
// declarations. decl builds them, and hypercase reads them, so decl's
// declarations need no exported method for the framework.
package erase

import (
	"io"
	"io/fs"
	"reflect"
)

// An Output mirrors decl.Output.
type Output struct {
	Pattern     string
	Description string
}

// A Property mirrors decl.Property.
type Property struct {
	Name        string
	Description string
}

// A Type is a decl.Type with its type argument erased.
type Type struct {
	Dir         string
	Description string
	GoType      reflect.Type
	// New wraps an input view as a value of GoType.
	New func(view fs.FS) any
}

// A Lift is a decl.Lift with its type arguments erased.
type Lift struct {
	Name        string
	Description string
	Outputs     []Output
	Rejects     []Property
	In, Out     reflect.Type
	// Run calls the lift's Run, or is nil when it has none.
	Run func(archive string, in any, stderr io.Writer) (any, error)
	// Jenny reports that the lift gives a jenny.
	Jenny bool
}

// Of returns the erased form of a decl declaration, a *Type or a *Lift, and
// false for anything else. decl sets it when it is initialized.
var Of func(v any) (any, bool)
