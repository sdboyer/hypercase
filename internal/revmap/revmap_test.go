package revmap

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
)

// TestFormat: one case per rule of the format. Files are in path order,
// bytewise, and each file's ranges by section, then first line, then last.
// A repeated range appears once, overlapping and adjacent line ranges of
// one section merge, a whole section absorbs its line ranges, and a range
// of a section's first lines stays one. A file with no ranges is its path
// line alone, a cell with no files an empty file, and a file that depends
// on all inputs reads "all inputs". A name that isn't plain is quoted.
func TestFormat(t *testing.T) {
	for _, tt := range []struct {
		name  string
		files []File
		want  string
	}{
		{"no files", nil, ""},
		{"a file with no ranges", []File{{Path: "error.txt"}}, "error.txt\n"},
		{"files by path", []File{{Path: "values.txt"}, {Path: "error.txt"}, {Path: "via/import/sheet/A1"}}, "error.txt\nvalues.txt\nvia/import/sheet/A1\n"},
		{"bytewise", []File{{Path: "b"}, {Path: "B"}, {Path: "a"}}, "B\na\nb\n"},
		{"all inputs", []File{{Path: "stdout.txt", All: true}}, "stdout.txt\n  all inputs\n"},
		{"all inputs absorbs ranges", []File{{Path: "stdout.txt", All: true, Ranges: []Range{{Section: "in/x"}}}}, "stdout.txt\n  all inputs\n"},
		{"ranges by section", []File{{Path: "v", Ranges: []Range{{Section: "in/sheet/B1"}, {Section: "in/sheet/A1"}}}}, "v\n  in/sheet/A1\n  in/sheet/B1\n"},
		{"ranges by first line", []File{{Path: "v", Ranges: []Range{{Section: "in/x", Start: 5, End: 5}, {Section: "in/x", Start: 2, End: 3}}}}, "v\n  in/x:2-3\n  in/x:5-5\n"},
		{"a repeat appears once", []File{{Path: "v", Ranges: []Range{{Section: "in/x", Start: 2, End: 3}, {Section: "in/x", Start: 2, End: 3}}}}, "v\n  in/x:2-3\n"},
		{"a whole repeat appears once", []File{{Path: "v", Ranges: []Range{{Section: "in/x"}, {Section: "in/x"}}}}, "v\n  in/x\n"},
		{"overlapping merge", []File{{Path: "v", Ranges: []Range{{Section: "in/x", Start: 2, End: 4}, {Section: "in/x", Start: 3, End: 6}}}}, "v\n  in/x:2-6\n"},
		{"contained merge", []File{{Path: "v", Ranges: []Range{{Section: "in/x", Start: 2, End: 9}, {Section: "in/x", Start: 3, End: 4}}}}, "v\n  in/x:2-9\n"},
		{"adjacent merge", []File{{Path: "v", Ranges: []Range{{Section: "in/x", Start: 4, End: 5}, {Section: "in/x", Start: 2, End: 3}}}}, "v\n  in/x:2-5\n"},
		{"a gap keeps two", []File{{Path: "v", Ranges: []Range{{Section: "in/x", Start: 1, End: 2}, {Section: "in/x", Start: 4, End: 5}}}}, "v\n  in/x:1-2\n  in/x:4-5\n"},
		{"other sections don't merge", []File{{Path: "v", Ranges: []Range{{Section: "in/x", Start: 1, End: 2}, {Section: "in/y", Start: 3, End: 4}}}}, "v\n  in/x:1-2\n  in/y:3-4\n"},
		{"a whole section absorbs its lines", []File{{Path: "v", Ranges: []Range{{Section: "in/x", Start: 2, End: 3}, {Section: "in/x"}, {Section: "in/y", Start: 1, End: 1}}}}, "v\n  in/x\n  in/y:1-1\n"},
		{"a section's first lines stay lines", []File{{Path: "v", Ranges: []Range{{Section: "in/x", Start: 1, End: 3}}}}, "v\n  in/x:1-3\n"},
		{"a section that reads as a range is quoted", []File{{Path: "v", Ranges: []Range{{Section: "in/sheet/A1:1-1"}, {Section: "in/sheet/A1", Start: 1, End: 1}}}}, "v\n  in/sheet/A1:1-1\n  \"in/sheet/A1:1-1\"\n"},
		{"a quoted section's lines", []File{{Path: "v", Ranges: []Range{{Section: "in/a b ", Start: 1, End: 2}}}}, "v\n  \"in/a b \":1-2\n"},
		{"a path that reads as a range is quoted", []File{{Path: "  x"}}, "\"  x\"\n"},
		{"a path shaped as a marker is quoted", []File{{Path: "-- x --"}}, "\"-- x --\"\n"},
		{"a path reading all inputs is quoted", []File{{Path: "x", Ranges: []Range{{Section: "all inputs"}}}}, "x\n  \"all inputs\"\n"},
		{"a name holding a quote is quoted", []File{{Path: `a"b`}}, "\"a\\\"b\"\n"},
		{"non-ASCII is quoted", []File{{Path: "café"}}, "\"café\"\n"},
		{"a space inside is plain", []File{{Path: "a b.txt"}}, "a b.txt\n"},
	} {
		got := string(Format(tt.files))
		if got != tt.want {
			t.Errorf("%s: Format gives\n%q\nwant\n%q", tt.name, got, tt.want)
			continue
		}
		back, err := Parse([]byte(got))
		if err != nil {
			t.Errorf("%s: Parse(%q): %v", tt.name, got, err)
			continue
		}
		if canon := Canonical(tt.files); !reflect.DeepEqual(back, canon) {
			t.Errorf("%s: Parse(Format) is %+v, want %+v", tt.name, back, canon)
		}
	}
}

// TestFormatPanics: Canonical, and so Format, refuses what the framework
// never gives it: two maps of one file, an empty path or section, and a
// range that is neither a whole section nor lines from 1, last at or
// after first.
func TestFormatPanics(t *testing.T) {
	for _, files := range [][]File{
		{{Path: "a"}, {Path: "a"}},
		{{Path: ""}},
		{{Path: "a", Ranges: []Range{{Section: ""}}}},
		{{Path: "a", Ranges: []Range{{Section: "in/x", Start: 0, End: 1}}}},
		{{Path: "a", Ranges: []Range{{Section: "in/x", Start: 2, End: 1}}}},
		{{Path: "a", Ranges: []Range{{Section: "in/x", Start: -1, End: -1}}}},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Format(%+v) didn't panic", files)
				}
			}()
			Format(files)
		}()
	}
}

// TestParseRefuses: Parse refuses every bytes Format doesn't write.
func TestParseRefuses(t *testing.T) {
	for _, data := range []string{
		"v",                           // no final newline
		"  in/x\n",                    // a range before any path
		"v\n  in/x:\n",                // no lines after the colon
		"v\n  in/x:1\n",               // no last line
		"v\n  in/x:2-1\n",             // last before first
		"v\n  in/x:0-1\n",             // no line 0
		"v\n  in/x:01-1\n",            // a leading zero
		"v\n  in/x:+1-1\n",            // a sign
		"v\n  in/x:1-1x\n",            // trailing text
		"v\n  in/x\n  in/x\n",         // a repeat
		"v\n  in/x:1-2\n  in/x:3-4\n", // adjacent, unmerged
		"v\n  in/y\n  in/x\n",         // out of order
		"w\nv\n",                      // files out of order
		"v\nv\n",                      // one file twice
		"v\n  all inputs\n  in/x\n",   // all inputs, and a range
		"\"v\"\n",                     // a plain name quoted
		"\"v\n",                       // an unclosed quote
		"v w:x\n",                     // a colon in a plain path
		" v\n",                        // a leading space
		"v \n",                        // a trailing space
		"\n",                          // an empty path
		"\"\"\n",                      // an empty path, quoted
		"v\n  \"\"\n",                 // an empty section, quoted
		"v\n  \"\":1-1\n",             // an empty section's lines
		"v\n   in/x\n",                // three spaces
		"v\n  \"in/x\" \n",            // text after a quoted section
	} {
		if files, err := Parse([]byte(data)); err == nil {
			t.Errorf("Parse(%q) = %+v, and should refuse it", data, files)
		}
	}
}

// TestPermutations: the bytes don't depend on the order of the files or
// of their ranges: every permutation of a set of ranges, and of a set of
// files, gives the bytes of one.
func TestPermutations(t *testing.T) {
	ranges := []Range{
		{Section: "in/x", Start: 1, End: 2},
		{Section: "in/x", Start: 3, End: 3},
		{Section: "in/x", Start: 5, End: 7},
		{Section: "in/y"},
		{Section: "in/y", Start: 2, End: 2},
		{Section: "via/l/y", Start: 6, End: 9},
	}
	want := string(Format([]File{{Path: "v", Ranges: ranges}}))
	n := 0
	permute(len(ranges), func(order []int) {
		rs := make([]Range, len(order))
		for i, j := range order {
			rs[i] = ranges[j]
		}
		n++
		if got := string(Format([]File{{Path: "v", Ranges: rs}})); got != want {
			t.Errorf("the ranges in the order %v give\n%s\nwant\n%s", order, got, want)
		}
	})
	if n != 720 {
		t.Errorf("%d permutations of %d ranges, want 720", n, len(ranges))
	}
	files := []File{{Path: "error.txt"}, {Path: "values.txt", Ranges: ranges[:2]}, {Path: "stdout.txt", All: true}, {Path: "via/l/a"}}
	want = string(Format(files))
	permute(len(files), func(order []int) {
		fs := make([]File, len(order))
		for i, j := range order {
			fs[i] = files[j]
		}
		if got := string(Format(fs)); got != want {
			t.Errorf("the files in the order %v give\n%s\nwant\n%s", order, got, want)
		}
	})
}

// permute calls f with each permutation of 0 to n-1.
func permute(n int, f func([]int)) {
	order := make([]int, n)
	used := make([]bool, n)
	var walk func(i int)
	walk = func(i int) {
		if i == n {
			f(order)
			return
		}
		for j := range n {
			if !used[j] {
				used[j] = true
				order[i] = j
				walk(i + 1)
				used[j] = false
			}
		}
	}
	walk(0)
}

// nameAtoms are what the generated names are made of: plain words and
// path separators, and each kind of byte that makes a name not plain.
var nameAtoms = []string{"a", "in/sheet/A1", "/", " ", ":", "1-1", `"`, `\`, "-", "-- x --", "all inputs", "é", "\u200b", "\t", "\n"}

// names returns every name of one to three atoms, the empty one first.
func names() []string {
	out := []string{""}
	var walk func(prefix string, depth int)
	walk = func(prefix string, depth int) {
		if depth == 3 {
			return
		}
		for _, a := range nameAtoms {
			n := prefix + a
			out = append(out, n)
			walk(n, depth+1)
		}
	}
	walk("", 0)
	return out
}

// TestRoundTrip: over generated names holding colons, leading and
// trailing spaces, quotes, a marker's shape, "all inputs" and bytes that
// aren't printable ASCII, every name as a file's path and as a range's
// section reads back from Format's bytes as written, so no name reads as
// another, or as a range. Every file Format writes for names an archive
// holds is one an archive holds, and the plain names are just those that
// need no quotes.
func TestRoundTrip(t *testing.T) {
	ns := names()
	held := 0
	for i, n := range ns {
		if n == "" {
			continue
		}
		other := ns[(i*7+3)%len(ns)]
		if other == "" || other == n {
			other = "in/x"
		}
		files := []File{
			{Path: n, Ranges: []Range{{Section: n}, {Section: other, Start: 1, End: 2}}},
			{Path: n + "/b", Ranges: []Range{{Section: n, Start: 2, End: 3}}},
			{Path: n + "/c", All: true},
			{Path: n + "/d"},
		}
		data := Format(files)
		back, err := Parse(data)
		if err != nil {
			t.Errorf("%q: Parse(%q): %v", n, data, err)
			continue
		}
		if canon := Canonical(files); !reflect.DeepEqual(back, canon) {
			t.Errorf("%q: Parse(Format) is %+v, want %+v", n, back, canon)
		}
		if holdable(n) && holdable(other) {
			held++
			if err := archive.Holds(data); err != nil {
				t.Errorf("%q: an archive can't hold the inputs file\n%s\n%v", n, data, err)
			}
		}
		if got := !strings.HasPrefix(string(Format([]File{{Path: n}})), `"`); got != plain(n) {
			t.Errorf("%q: written unquoted %v, plain %v", n, got, plain(n))
		}
	}
	if held < 100 {
		t.Errorf("only %d of %d names are ones an archive holds", held, len(ns))
	}
	t.Logf("%d names, %d of them ones an archive holds", len(ns), held)
}

// holdable reports whether an archive holds a section of the name, as an
// output's or an input's.
func holdable(n string) bool {
	return archive.HoldsName("out/c/"+n) == nil && archive.HoldsName(n) == nil
}

// TestPlainNames: the walkthrough's names, and the framework's sections,
// are written as they are.
func TestPlainNames(t *testing.T) {
	for _, n := range []string{"values.txt", "error.txt", "stdout.txt", "in/sheet/A1", "variant/cycle/in/sheet/B1", "transform/looped/in/sheet/A1", "via/import/sheet/A1", "in/csv/prices.csv", "tables/a b.csv"} {
		if !plain(n) {
			t.Errorf("%q isn't plain", n)
		}
	}
	for _, n := range []string{fmt.Sprintf("%s:%d-%d", "in/x", 1, 1), " x", "x ", "-x", `a"b`, `a\b`, "all inputs", "é", ""} {
		if plain(n) {
			t.Errorf("%q is plain", n)
		}
	}
}
