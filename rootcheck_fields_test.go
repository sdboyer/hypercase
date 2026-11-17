package hypercase

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/registry"
)

// TestLiftFields: the root-package check compares each key of a lift's
// registry entry across the registries that hold it, but for those that
// may differ between them: liftFields has an entry for each other key, by
// its name, and an entry's value moves when any key under its key does,
// and with no other key.
func TestLiftFields(t *testing.T) {
	// The entries are grouped by name. A package owns the lift or only
	// uses it, and the corpora that reach it are each package's own.
	exempt := []string{"name", "owned", "corpora"}
	var keys []string
	for f := range reflect.TypeFor[registry.Lift]().Fields() {
		key, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		keys = append(keys, key)
		compared := false
		for _, e := range liftFields {
			compared = compared || e.name == key
		}
		switch {
		case compared && slices.Contains(exempt, key):
			t.Errorf("a lift's %s is compared, and exempt from comparison", key)
		case !compared && !slices.Contains(exempt, key):
			t.Errorf("a lift's %s is neither compared nor exempt from comparison.\nGive liftFields an entry for it, or exempt it here, saying why its entries may differ.", key)
		}
	}
	for _, l := range leaves(reflect.TypeFor[registry.Lift]()) {
		base, set := l.base.Interface().(registry.Lift), l.set.Interface().(registry.Lift)
		for _, e := range liftFields {
			if moved := e.value(set) != e.value(base); moved != (e.name == l.top) {
				t.Errorf("liftFields' %s entry moves with %s: %v, want %v", e.name, l.path, moved, e.name == l.top)
			}
		}
	}
	for _, e := range liftFields {
		if !slices.Contains(keys, e.name) {
			t.Errorf("liftFields has an entry for %s, which no key of a lift's registry entry is", e.name)
		}
	}
}

// TestTypeEntry: the root-package check compares a corpus type's
// registry entry across the registries that hold it as typeEntry shows
// it, which moves with each key under the entry but those that may differ
// between them.
func TestTypeEntry(t *testing.T) {
	// The entries are grouped by directory, and a package owns each
	// transform or doesn't.
	exempt := []string{"dir", "transforms[].owned"}
	var paths []string
	for _, l := range leaves(reflect.TypeFor[registry.Type]()) {
		paths = append(paths, l.path)
		base, set := l.base.Interface().(registry.Type), l.set.Interface().(registry.Type)
		if moved, want := typeEntry(set) != typeEntry(base), !slices.Contains(exempt, l.path); moved != want {
			t.Errorf("typeEntry moves with a corpus type's %s: %v, want %v.\nShow the key in typeEntry, or exempt it here, saying why its entries may differ.", l.path, moved, want)
		}
	}
	for _, p := range exempt {
		if !slices.Contains(paths, p) {
			t.Errorf("%s is exempt, and no key of a corpus type's registry entry", p)
		}
	}
}

// A leaf is a key of a registry entry that holds no other keys: a field,
// under its parent's key as parent.key, or as parent[].key where the
// parent is a list of structs.
type leaf struct {
	path, top string // the key's path, and the entry's key it is under
	// base is an entry whose lists on the way to the leaf hold one element
	// each, with every field zero, and set is base with the leaf, too, not
	// zero. So set moves from base by the leaf alone, and not by a list
	// gaining its element.
	base, set reflect.Value
}

// leaves returns the leaves of a struct type t.
func leaves(t reflect.Type) []leaf {
	var ls []leaf
	for f := range t.Fields() {
		key, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		with := func(v reflect.Value) reflect.Value {
			s := reflect.New(t).Elem()
			s.FieldByIndex(f.Index).Set(v)
			return s
		}
		if f.Type.Kind() != reflect.Slice || f.Type.Elem().Kind() != reflect.Struct {
			ls = append(ls, leaf{path: key, top: key, base: reflect.New(t).Elem(), set: with(nonZero(f.Type))})
			continue
		}
		for _, sub := range leaves(f.Type.Elem()) {
			one := func(v reflect.Value) reflect.Value { return reflect.Append(reflect.New(f.Type).Elem(), v) }
			ls = append(ls, leaf{path: key + "[]." + sub.path, top: key, base: with(one(sub.base)), set: with(one(sub.set))})
		}
	}
	return ls
}

// nonZero returns a value of type t that isn't its zero value, nor holds
// one.
func nonZero(t reflect.Type) reflect.Value {
	v := reflect.New(t).Elem()
	switch t.Kind() {
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Slice:
		v = reflect.Append(v, nonZero(t.Elem()))
	case reflect.Struct:
		for i := range t.NumField() {
			v.Field(i).Set(nonZero(t.Field(i).Type))
		}
	default:
		panic("no value of " + t.String())
	}
	return v
}
