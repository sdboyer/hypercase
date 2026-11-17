package archive

import (
	"errors"
	"slices"
	"strings"
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

// TestHoldsName: a section's name holds unless it is empty, holds a
// newline, or starts or ends with white space, and each of those is
// named.
func TestHoldsName(t *testing.T) {
	for _, tt := range []struct {
		name string
		want string // "" when it holds
	}{
		{"in/sheet/A1", ""},
		{"in/a b/c", ""},
		{"in/a -- b/c", ""},
		{"in/a\rb", ""},
		{"", "it is empty"},
		{"in/a\nb", "it holds a newline"},
		{"in/a\n", "it holds a newline"},
		{"in/a ", "it starts or ends with white space"},
		{" in/a", "it starts or ends with white space"},
		{"in/a\r", "it starts or ends with white space"},
	} {
		err := HoldsName(tt.name)
		switch {
		case tt.want == "" && err != nil:
			t.Errorf("HoldsName(%q) = %v, want nil", tt.name, err)
		case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
			t.Errorf("HoldsName(%q) = %v, want %q", tt.name, err, tt.want)
		}
	}
}

// TestHoldsNameIsTheReadBack: over every name of up to five runes from
// an alphabet of a letter, a slash, a dash and white space, HoldsName
// admits exactly the names that a one-section archive reads back.
func TestHoldsNameIsTheReadBack(t *testing.T) {
	alphabet := []rune{'a', '/', '-', ' ', '\t', '\n', '\r', '\u00a0', '\u2028'}
	var walk func(name string, n int)
	walk = func(name string, n int) {
		_, rerr := (&Archive{Sections: []Section{{Name: name}}}).ReadBack()
		if err := HoldsName(name); (err == nil) != (rerr == nil) {
			t.Errorf("HoldsName(%q) = %v, and the read-back says %v", name, err, rerr)
		}
		if n > 0 {
			for _, r := range alphabet {
				walk(name+string(r), n-1)
			}
		}
	}
	walk("", 5)
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
