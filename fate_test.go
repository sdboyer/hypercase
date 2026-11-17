package hypercase

import (
	"reflect"
	"testing"
)

// TestFateWords: fateWords words a fate as the one kind its fields give,
// and refuses, by panicking, a fate whose fields no one kind gives. A fate
// with any one field of updateFate set, or any two, is never worded as a
// run, which only the zero fate is, so a field fateWords doesn't know of
// is refused, never guessed.
func TestFateWords(t *testing.T) {
	if got := fateWords(updateFate{}); got != fateRuns {
		t.Errorf("the zero fate: got %q, want %q", got, fateRuns)
	}
	words := func(f updateFate) (w string, refused bool) {
		defer func() {
			if v := recover(); v != nil {
				refused = true
			}
		}()
		return fateWords(f), false
	}
	typ := reflect.TypeFor[updateFate]()
	set := func(f *updateFate, i int) {
		v := reflect.ValueOf(f).Elem().Field(i)
		v = reflect.NewAt(v.Type(), v.Addr().UnsafePointer()).Elem()
		switch v.Kind() {
		case reflect.String:
			v.SetString("x")
		case reflect.Bool:
			v.SetBool(true)
		case reflect.Pointer:
			v.Set(reflect.New(v.Type().Elem()))
		default:
			t.Fatalf("updateFate's %s is a %s, which TestFateWords can't set", typ.Field(i).Name, v.Kind())
		}
	}
	for i := range typ.NumField() {
		for j := i; j < typ.NumField(); j++ {
			var f updateFate
			set(&f, i)
			set(&f, j)
			what := typ.Field(i).Name
			if j != i {
				what += " and " + typ.Field(j).Name
			}
			if w, refused := words(f); !refused && w == fateRuns {
				t.Errorf("a fate with %s set is worded %q, as a run", what, w)
			}
		}
	}
	for _, f := range []updateFate{{skip: "x", fault: "y"}, {fault: "y", skip: "x", panicked: true}, {block: &blockage{}, skip: "x"}} {
		if w, refused := words(f); !refused {
			t.Errorf("%+v is worded %q, and is no one kind's", f, w)
		}
	}
}
