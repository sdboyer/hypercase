// Package registrytest gives what encoding/json writes for a registry
// type with every field set, for the tests that read a registry's keys
// from the encoder rather than from a list kept by hand.
package registrytest

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// KeyPaths returns the key paths of what encoding/json writes for a value
// of type typ with every field set, sorted: a key, under its parent's as
// parent.key, parent[].key under a list, and parent.*.key under a map.
func KeyPaths(t testing.TB, typ reflect.Type) []string {
	t.Helper()
	var out []string
	var walk func(v any, prefix string)
	walk = func(v any, prefix string) {
		switch v := v.(type) {
		case map[string]any:
			for k, c := range v {
				out = append(out, prefix+k)
				walk(c, prefix+k+".")
			}
		case []any:
			for _, c := range v {
				walk(c, strings.TrimSuffix(prefix, ".")+"[].")
			}
		}
	}
	walk(Full(t, typ), "")
	slices.Sort(out)
	return slices.Compact(out)
}

// Full returns what encoding/json writes for a value of type typ with
// every field set, read back as JSON values: every key, with one element
// in each list and map.
func Full(t testing.TB, typ reflect.Type) any {
	t.Helper()
	data, err := json.Marshal(Populated(typ).Interface())
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// Populated returns a value of type typ with every field set: each string
// "x", each map's one key "*", each bool true and each number 1, each list
// and map holding one element, and each pointer pointing at one. It panics
// on a field encoding/json doesn't write, and on a kind a registry can't
// hold.
func Populated(typ reflect.Type) reflect.Value {
	v := reflect.New(typ).Elem()
	switch typ.Kind() {
	case reflect.Struct:
		for f := range typ.Fields() {
			if !f.IsExported() {
				panic("the unexported field " + f.Name + " of " + typ.String() + ": a registry type holds none")
			}
			v.FieldByIndex(f.Index).Set(Populated(f.Type))
		}
	case reflect.Pointer:
		v.Set(reflect.New(typ.Elem()))
		v.Elem().Set(Populated(typ.Elem()))
	case reflect.Slice:
		v.Set(reflect.Append(v, Populated(typ.Elem())))
	case reflect.Map:
		if typ.Key().Kind() != reflect.String {
			panic("no registry key of " + typ.String())
		}
		v.Set(reflect.MakeMap(typ))
		v.SetMapIndex(reflect.ValueOf("*").Convert(typ.Key()), Populated(typ.Elem()))
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int:
		v.SetInt(1)
	default:
		panic("no registry value of " + typ.String())
	}
	return v
}
