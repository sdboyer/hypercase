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
// its name, and an entry's value moves when its key's value does, and
// with no other key.
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
		var l registry.Lift
		reflect.ValueOf(&l).Elem().FieldByIndex(f.Index).Set(nonZero(f.Type))
		for _, e := range liftFields {
			if moved := e.value(l) != e.value(registry.Lift{}); moved != (e.name == key) {
				t.Errorf("liftFields' %s entry moves with %s: %v, want %v", e.name, key, moved, e.name == key)
			}
		}
	}
	for _, e := range liftFields {
		if !slices.Contains(keys, e.name) {
			t.Errorf("liftFields has an entry for %s, which no key of a lift's registry entry is", e.name)
		}
	}
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
