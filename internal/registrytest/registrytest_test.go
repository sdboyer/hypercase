package registrytest

import (
	"reflect"
	"slices"
	"testing"
)

// TestKeyPaths: KeyPaths reads each key as encoding/json writes it,
// through nested structs, pointers, lists and maps: an embedded struct's
// keys among its parent's, a field with no tag by its Go name, one tagged
// only with options by its Go name too, and one tagged "-," as "-". A field
// tagged "-" holds no key, and a map of empty structs holds its key.
func TestKeyPaths(t *testing.T) {
	type inner struct {
		A string `json:"a"`
	}
	type Embedded struct {
		E string `json:"e"`
	}
	type entry struct {
		Embedded
		Plain  string
		Opts   string              `json:",omitempty"`
		Dash   string              `json:"-,"`
		Hidden string              `json:"-"`
		Nested inner               `json:"nested"`
		Ptr    *inner              `json:"ptr"`
		List   []inner             `json:"list"`
		Map    map[string]inner    `json:"map"`
		Set    map[string]struct{} `json:"set"`
		Count  int                 `json:"count,omitempty"`
	}
	want := []string{"-", "Opts", "Plain", "count", "e", "list", "list[].a", "map", "map.*", "map.*.a", "nested", "nested.a", "ptr", "ptr.a", "set", "set.*"}
	if got := KeyPaths(t, reflect.TypeFor[entry]()); !slices.Equal(got, want) {
		t.Errorf("KeyPaths:\n got %q\nwant %q", got, want)
	}
}
