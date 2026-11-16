package archive

import (
	"errors"
	"slices"
	"testing"
	"testing/fstest"
)

// TestTreeSections: the walk names each file's section by its path under
// the prefix, sorted by name, and an empty prefix leaves the paths alone.
// A tree with no files gives no sections.
func TestTreeSections(t *testing.T) {
	tree := fstest.MapFS{"b": {Data: []byte("2\n")}, "a/x": {Data: []byte("1\n")}, "a-z": {Data: nil}}
	for prefix, want := range map[string][]string{
		"in/sheet/": {"in/sheet/a-z", "in/sheet/a/x", "in/sheet/b"},
		"":          {"a-z", "a/x", "b"},
	} {
		ss, err := TreeSections(prefix, tree)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, s := range ss {
			got = append(got, s.Name)
		}
		if !slices.Equal(got, want) {
			t.Errorf("under %q, the walk gave %q, not %q", prefix, got, want)
		}
	}
	if ss, err := TreeSections("in/sheet/", fstest.MapFS{}); ss != nil || err != nil {
		t.Errorf("an empty tree gave %v, %v", ss, err)
	}
}

// TestTreeSectionsRefuses: a prefix that isn't a slash path ending in a
// slash is refused, and a file an archive can't hold is a TreeError
// naming its path in the tree.
func TestTreeSectionsRefuses(t *testing.T) {
	tree := fstest.MapFS{"a": {Data: []byte("1\n")}}
	for _, prefix := range []string{"in/sheet", "/in/", "in//", "./"} {
		if _, err := TreeSections(prefix, tree); err == nil {
			t.Errorf("the prefix %q was accepted", prefix)
		}
	}
	_, err := TreeSections("in/sheet/", fstest.MapFS{"x/a": {Data: []byte("1")}})
	var te *TreeError
	if !errors.As(err, &te) || te.Path != "x/a" {
		t.Errorf("the walk returned %v, not a TreeError naming x/a", err)
	}
}
