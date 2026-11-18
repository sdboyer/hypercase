package hypercase

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/registry"
	"github.com/sdboyer/hypercase/internal/registrytest"
)

// TestLiftFields: the root-package check compares each key of a lift's
// registry entry across the registries that hold it, but for those that
// may differ between them: liftFields has an entry for each other key, by
// its name, and an entry's value moves when any key under its key does,
// and with no other key. The keys are those encoding/json writes for an
// entry with every field set, and each holds a leaf, which moves it.
func TestLiftFields(t *testing.T) {
	// The entries are grouped by name. A package owns the lift or only
	// uses it, and the corpora that reach it, and the paths from them, are
	// each package's own.
	exempt := []string{"name", "owned", "corpora", "paths"}
	typ := reflect.TypeFor[registry.Lift]()
	full, ok := registrytest.Full(t, typ).(map[string]any)
	if !ok {
		t.Fatalf("encoding/json writes a lift's registry entry as no object")
	}
	keys := slices.Sorted(maps.Keys(full))
	ls := leaves(t, typ)
	unleaved(t, typ, ls)
	for _, l := range ls {
		base, set := l.base.Interface().(registry.Lift), l.set.Interface().(registry.Lift)
		for _, e := range liftFields {
			if moved := e.value(set) != e.value(base); moved != (e.name == l.top) {
				t.Errorf("liftFields' %s entry moves with %s: %v, want %v", e.name, l.path, moved, e.name == l.top)
			}
		}
	}
	for _, key := range keys {
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
	for _, e := range liftFields {
		if !slices.Contains(keys, e.name) {
			t.Errorf("liftFields has an entry for %s, which no key of a lift's registry entry is", e.name)
		}
	}
	for _, key := range exempt {
		if !slices.Contains(keys, key) {
			t.Errorf("%s is exempt, and no key of a lift's registry entry", key)
		}
	}
}

// TestTypeEntry: the root-package check compares a corpus type's
// registry entry across the registries that hold it as typeEntry shows
// it, which moves with each key under the entry but those that may differ
// between them. Each key encoding/json writes for an entry with every
// field set holds a leaf, which moves it.
func TestTypeEntry(t *testing.T) {
	// The entries are grouped by directory, and a package owns each
	// transform or doesn't.
	exempt := []string{"dir", "transforms[].owned"}
	var paths []string
	typ := reflect.TypeFor[registry.Type]()
	ls := leaves(t, typ)
	unleaved(t, typ, ls)
	for _, l := range ls {
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

// TestLeaves: leaves finds each key as encoding/json writes it, through
// nested structs, pointers, lists and maps, an embedded struct's keys
// among its parent's, a field with no tag by its Go name, one tagged only
// with options by its Go name too, and one tagged "-," as "-", and fails on
// a field encoding/json doesn't write.
func TestLeaves(t *testing.T) {
	type inner struct {
		A string `json:"a"`
		B []bool `json:"b"`
	}
	type Embedded struct {
		E string `json:"e"`
	}
	type entry struct {
		Embedded
		Plain   string
		Opts    string            `json:",omitempty"`
		Dash    string            `json:"-,"`
		Nested  inner             `json:"nested"`
		Ptr     *inner            `json:"ptr"`
		List    []inner           `json:"list"`
		Map     map[string]inner  `json:"map"`
		Scalars map[string]string `json:"scalars"`
		Count   int               `json:"count,omitempty"`
	}
	var got []string
	for _, l := range leaves(t, reflect.TypeFor[entry]()) {
		got = append(got, l.top+" "+l.path)
	}
	want := []string{
		"e e", "Plain Plain", "Opts Opts", "- -",
		"nested nested.a", "nested nested.b[]",
		"ptr ptr.a", "ptr ptr.b[]",
		"list list[].a", "list list[].b[]",
		"map map.*.a", "map map.*.b[]",
		"scalars scalars.*", "count count",
	}
	if !slices.Equal(got, want) {
		t.Errorf("leaves:\n got %q\nwant %q", got, want)
	}

	type hidden struct {
		Shown  string `json:"shown"`
		Hidden string `json:"-"`
	}
	ft := &fakeT{}
	leaves(ft, reflect.TypeFor[hidden]())
	if want := "the field Hidden, of hypercase.hidden, moves no key of what encoding/json writes, so it is no key of the registry's: give it one, or remove it"; !slices.Contains(ft.errs, want) {
		t.Errorf("leaves of a field encoding/json doesn't write: got %q, want %q", ft.errs, want)
	}
}

// unleaved fails on each key of what encoding/json writes for typ, with
// every field set, that holds no leaf: a key with no scalar under it, as a
// map of empty structs, which no leaf moves, so no comparison of leaves
// checks it.
func unleaved(t *testing.T, typ reflect.Type, ls []leaf) {
	t.Helper()
	for _, k := range registrytest.KeyPaths(t, typ) {
		held := slices.ContainsFunc(ls, func(l leaf) bool {
			return l.path == k || strings.HasPrefix(l.path, k+".") || strings.HasPrefix(l.path, k+"[]")
		})
		if !held {
			t.Errorf("the key %s, of what encoding/json writes for %s, holds no leaf, so no test of its leaves can tell whether the root-package check compares it.\nGive it a scalar to hold, as a string, a bool or a number, under it.", k, typ)
		}
	}
}

// A fakeT records what leaves reports.
type fakeT struct {
	testing.TB
	errs []string
}

func (f *fakeT) Helper() {}
func (f *fakeT) Errorf(format string, args ...any) {
	f.errs = append(f.errs, fmt.Sprintf(format, args...))
}

// A leaf is a key of a registry entry that holds no other keys, by its
// path in what encoding/json writes for the entry: parent.key, parent[].key
// under a list, and parent.*.key under a map. top is the entry's key it is
// under.
type leaf struct {
	path, top string
	// base is an entry whose lists, maps and pointers on the way to the
	// leaf hold one element each, with every field zero, and set is base
	// with the leaf, too, not zero. So set moves from base by the leaf
	// alone, and not by a container gaining its element.
	base, set reflect.Value
}

// leaves returns the leaves of a struct type: for each field it reaches
// through structs, pointers, lists and maps, the one key of what
// encoding/json writes that setting the field moves. It reports a field
// that moves no key, or more than one.
func leaves(t testing.TB, typ reflect.Type) []leaf {
	t.Helper()
	var ls []leaf
	for _, g := range goLeaves(typ) {
		moved := movedKeys(jsonOf(t, g.base), jsonOf(t, g.set), "")
		if len(moved) != 1 {
			if len(moved) == 0 {
				t.Errorf("the field %s, of %s, moves no key of what encoding/json writes, so it is no key of the registry's: give it one, or remove it", g.field, typ)
			} else {
				t.Errorf("the field %s, of %s, moves the keys %v of what encoding/json writes, and a leaf is one key", g.field, typ, moved)
			}
			continue
		}
		top, _, _ := strings.Cut(strings.Replace(moved[0], "[]", ".", 1), ".")
		ls = append(ls, leaf{path: moved[0], top: top, base: g.base, set: g.set})
	}
	return ls
}

// A goLeaf is a field a struct type reaches, by its Go path, with an entry
// of the type that holds it zero, base, and one that holds it set.
type goLeaf struct {
	field     string
	base, set reflect.Value
}

// goLeaves returns the fields a type reaches, in order, through structs,
// pointers, lists and maps with string keys, each with the values that
// hold it zero and set.
func goLeaves(typ reflect.Type) []goLeaf {
	switch typ.Kind() {
	case reflect.Struct:
		var out []goLeaf
		for f := range typ.Fields() {
			if !f.IsExported() {
				panic("the unexported field " + f.Name + " of " + typ.String() + ": a registry type holds none")
			}
			for _, sub := range goLeaves(f.Type) {
				with := func(v reflect.Value) reflect.Value {
					s := reflect.New(typ).Elem()
					s.FieldByIndex(f.Index).Set(v)
					return s
				}
				name := f.Name
				if sub.field != "" {
					name += "." + sub.field
				}
				out = append(out, goLeaf{name, with(sub.base), with(sub.set)})
			}
		}
		return out
	case reflect.Pointer:
		var out []goLeaf
		for _, sub := range goLeaves(typ.Elem()) {
			b, s := reflect.New(typ.Elem()), reflect.New(typ.Elem())
			b.Elem().Set(sub.base)
			s.Elem().Set(sub.set)
			out = append(out, goLeaf{sub.field, b, s})
		}
		return out
	case reflect.Slice:
		var out []goLeaf
		for _, sub := range goLeaves(typ.Elem()) {
			one := func(v reflect.Value) reflect.Value { return reflect.Append(reflect.New(typ).Elem(), v) }
			out = append(out, goLeaf{sub.field, one(sub.base), one(sub.set)})
		}
		return out
	case reflect.Map:
		if typ.Key().Kind() != reflect.String {
			panic("no registry key of " + typ.String())
		}
		var out []goLeaf
		for _, sub := range goLeaves(typ.Elem()) {
			one := func(v reflect.Value) reflect.Value {
				m := reflect.MakeMap(typ)
				m.SetMapIndex(reflect.ValueOf("*").Convert(typ.Key()), v)
				return m
			}
			out = append(out, goLeaf{sub.field, one(sub.base), one(sub.set)})
		}
		return out
	}
	return []goLeaf{{"", reflect.New(typ).Elem(), nonZero(typ)}}
}

// nonZero returns a value of a scalar type that isn't its zero value.
func nonZero(typ reflect.Type) reflect.Value {
	v := reflect.New(typ).Elem()
	switch typ.Kind() {
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1)
	default:
		panic("no value of " + typ.String())
	}
	return v
}

// jsonOf returns what encoding/json writes for v, read back as JSON
// values.
func jsonOf(t testing.TB, v reflect.Value) any {
	t.Helper()
	data, err := json.Marshal(v.Interface())
	if err != nil {
		t.Errorf("encoding %s: %v", v.Type(), err)
		return nil
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Errorf("decoding %s: %v", v.Type(), err)
	}
	return out
}

// movedKeys returns the paths of the deepest keys whose values differ
// between two JSON values, as leaves names them.
func movedKeys(a, b any, path string) []string {
	am, aok := a.(map[string]any)
	bm, bok := b.(map[string]any)
	if aok && bok {
		var out []string
		ks := slices.Sorted(maps.Keys(am))
		for k := range bm {
			if !slices.Contains(ks, k) {
				ks = append(ks, k)
			}
		}
		slices.Sort(ks)
		for _, k := range ks {
			p := k
			if path != "" {
				p = path + "." + k
			}
			av, ain := am[k]
			bv, bin := bm[k]
			if ain != bin {
				out = append(out, p)
				continue
			}
			out = append(out, movedKeys(av, bv, p)...)
		}
		return out
	}
	al, aok := a.([]any)
	bl, bok := b.([]any)
	if aok && bok && len(al) == len(bl) {
		var out []string
		for i := range al {
			for _, p := range movedKeys(al[i], bl[i], path+"[]") {
				if !slices.Contains(out, p) {
					out = append(out, p)
				}
			}
		}
		return out
	}
	if reflect.DeepEqual(a, b) {
		return nil
	}
	return []string{path}
}
