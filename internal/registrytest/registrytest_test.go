package registrytest

import (
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// TestKeyPaths: KeyPaths reads each key as encoding/json writes it,
// through nested structs, pointers, lists and maps: an embedded struct's
// keys among its parent's, a field with no tag by its Go name, one tagged
// only with options by its Go name too, and one tagged "-," as "-". A map
// of empty structs holds its key, and a number of each kind holds one.
func TestKeyPaths(t *testing.T) {
	type inner struct {
		A string `json:"a"`
	}
	type Embedded struct {
		E string `json:"e"`
	}
	type entry struct {
		Embedded
		Plain   string
		Opts    string              `json:",omitempty"`
		Dash    string              `json:"-,"`
		Nested  inner               `json:"nested"`
		Ptr     *inner              `json:"ptr"`
		List    []inner             `json:"list"`
		Map     map[string]inner    `json:"map"`
		Set     map[string]struct{} `json:"set"`
		Count   int                 `json:"count,omitempty"`
		I8      int8                `json:"i8"`
		I16     int16               `json:"i16"`
		I32     int32               `json:"i32"`
		I64     int64               `json:"i64"`
		U       uint                `json:"u"`
		U8      uint8               `json:"u8"`
		U16     uint16              `json:"u16"`
		U32     uint32              `json:"u32"`
		U64     uint64              `json:"u64"`
		Uintptr uintptr             `json:"uintptr"`
		F32     float32             `json:"f32"`
		F64     float64             `json:"f64"`
	}
	want := []string{"-", "Opts", "Plain", "count", "e", "f32", "f64", "i16", "i32", "i64", "i8", "list", "list[].a", "map", "map.*", "map.*.a", "nested", "nested.a", "ptr", "ptr.a", "set", "set.*", "u", "u16", "u32", "u64", "u8", "uintptr"}
	if got := KeyPaths(t, reflect.TypeFor[entry]()); !slices.Equal(got, want) {
		t.Errorf("KeyPaths:\n got %q\nwant %q", got, want)
	}
}

// TestPopulatedRefuses: Populated fails the test, and doesn't panic, on a
// field encoding/json doesn't write, unexported or tagged "-", through a
// pointer, a list or a map too, and on a kind a registry can't hold.
func TestPopulatedRefuses(t *testing.T) {
	type unexported struct {
		A string `json:"a"`
		b string
	}
	type hidden struct {
		A string `json:"a"`
		B string `json:"-"`
	}
	type deep struct {
		P *hidden            `json:"p"`
		L []hidden           `json:"l"`
		M map[string]*hidden `json:"m"`
	}
	type keyed struct {
		M map[int]string `json:"m"`
	}
	type complexed struct {
		C complex128 `json:"c"`
	}
	type anything struct {
		V any `json:"v"`
	}
	for _, c := range []struct {
		typ  reflect.Type
		want string
	}{
		{reflect.TypeFor[unexported](), "the unexported field b of registrytest.unexported: a registry type holds none"},
		{reflect.TypeFor[hidden](), `the field B of registrytest.hidden is tagged json:"-", so encoding/json doesn't write it: a registry type holds none`},
		{reflect.TypeFor[deep](), `the field B of registrytest.hidden is tagged json:"-", so encoding/json doesn't write it: a registry type holds none`},
		{reflect.TypeFor[keyed](), "no registry key of map[int]string"},
		{reflect.TypeFor[complexed](), "no registry value of complex128"},
		{reflect.TypeFor[anything](), "no registry value of interface {}"},
	} {
		if got := fatal(t, func(tb testing.TB) { Populated(tb, c.typ) }); got != c.want {
			t.Errorf("Populated of %s fails with %q, want %q", c.typ, got, c.want)
		}
	}
}

// fatal runs fn with a testing.TB whose Fatal and Fatalf record their
// message and end fn, as they end a test, and returns the message, or ""
// where fn ends without one. Any other method of the TB is t's.
func fatal(t *testing.T, fn func(testing.TB)) string {
	r := &recorder{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(r)
	}()
	<-done
	return r.msg
}

type recorder struct {
	testing.TB
	msg string
}

func (r *recorder) Helper() {}

func (r *recorder) Fatal(args ...any) {
	r.msg = strings.TrimSuffix(fmt.Sprintln(args...), "\n")
	runtime.Goexit()
}

func (r *recorder) Fatalf(format string, args ...any) {
	r.msg = fmt.Sprintf(format, args...)
	runtime.Goexit()
}
