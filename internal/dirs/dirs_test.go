package dirs

import (
	"reflect"
	"slices"
	"testing"
)

// TestIndex: every directory the paths imply is indexed, with its entries
// sorted, whatever order the paths come in, and the root always is.
func TestIndex(t *testing.T) {
	want := map[string][]string{
		".":      {"a", "b.txt", "do"},
		"a":      {"b", "c.txt"},
		"a/b":    {"d.txt", "e.txt"},
		"do":     {"out"},
		"do/out": {"x.txt"},
	}
	paths := []string{"do/out/x.txt", "a/b/e.txt", "b.txt", "a/c.txt", "a/b/d.txt"}
	for range 2 {
		if got := Index(slices.Values(paths)); !reflect.DeepEqual(got, want) {
			t.Errorf("Index(%q) = %v, want %v", paths, got, want)
		}
		slices.Reverse(paths)
	}
	if got := Index(slices.Values([]string(nil))); !reflect.DeepEqual(got, map[string][]string{".": nil}) {
		t.Errorf("Index of no paths = %v, want the root alone", got)
	}
}
