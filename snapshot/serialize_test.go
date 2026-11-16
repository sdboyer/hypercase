package snapshot

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/sdboyer/hypercase/archive"
)

// TestSerializeSorted: trees serialize to in/a/…, in/a-b/… and in/b/…,
// sorted by name: not in the walk's order, which visits x/ before x-z, nor
// tree by tree, which puts in/a/ before in/a-b/. Serializing them again
// gives the same bytes.
func TestSerializeSorted(t *testing.T) {
	trees := func() map[string]fs.FS {
		return map[string]fs.FS{
			"b":   fstest.MapFS{"B1": {Data: []byte("3\n")}},
			"a-b": fstest.MapFS{"C1": {Data: []byte("5\n")}},
			"a":   fstest.MapFS{"x/y": {Data: []byte("=B1*2\n")}, "x-z": {Data: []byte("4\n")}, "A1": {Data: []byte("1\n")}},
		}
	}
	ss := serialize(t, trees())
	if want := []string{"in/a-b/C1", "in/a/A1", "in/a/x-z", "in/a/x/y", "in/b/B1"}; !slices.Equal(names(ss), want) {
		t.Errorf("Serialize gave the sections %q, not %q", names(ss), want)
	}
	if again := serialize(t, trees()); !bytes.Equal(format(again), format(ss)) {
		t.Errorf("serializing the trees again gave other bytes:\n%s\nnot\n%s", format(again), format(ss))
	}
}

// TestSerializeMarker: a file holding a line shaped like a section marker
// is refused, naming its tree and path. Without the walk's read-back
// check, the archive would split the file in two.
func TestSerializeMarker(t *testing.T) {
	data := []byte("=B1*2\n-- in/sheet/B1 --\n3\n")
	_, err := Serialize(map[string]fs.FS{"sheet": fstest.MapFS{"A1": {Data: data}}})
	fails(t, err, "the tree sheet, at A1:", "its line 2 is shaped like a section marker")

	split := archive.Parse(format([]archive.Section{{Name: "in/sheet/A1", Data: data}}))
	if len(split.Sections) != 2 {
		t.Errorf("written unchecked, the file reads back as %d sections, not the 2 it splits into", len(split.Sections))
	}
}

// TestSerializeIsTheWalk: Serialize's sections for a tree are those the
// walk, archive.TreeSections, gives for it under in/<tree>/.
func TestSerializeIsTheWalk(t *testing.T) {
	tree := fstest.MapFS{"A1": {Data: []byte("=B1*2\n")}, "B1": {Data: []byte("3\n")}, "notes/x": {Data: nil}}
	ss := serialize(t, map[string]fs.FS{"sheet": tree})
	walked, err := archive.TreeSections("in/sheet/", tree)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(format(ss), format(walked)) {
		t.Errorf("Serialize gave:\n%s\nand the walk:\n%s", format(ss), format(walked))
	}
}

// TestSerializeFinalNewline: a file without a final newline either reads
// back byte for byte, through Create and Load, or is refused by Serialize,
// naming it. It never reads back changed.
func TestSerializeFinalNewline(t *testing.T) {
	for _, data := range []string{"", "3", "=B1*2\n3", "3\n\n"} {
		tree := fstest.MapFS{"A1": {Data: []byte(data)}}
		ss, err := Serialize(map[string]fs.FS{"sheet": tree})
		if err != nil {
			fails(t, err, "the tree sheet, at A1:", "it has no final newline, which an archive adds")
			if strings.HasSuffix(data, "\n") || data == "" {
				t.Errorf("Serialize refused %q, which an archive holds", data)
			}
			continue
		}
		path, err := Create(newCorpus(t), project, sheetType, "nonl", ss)
		if err != nil {
			t.Fatal(err)
		}
		s, err := Load(sheetType, path)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := fs.ReadFile(s, "sheet/A1"); err != nil || string(got) != data {
			t.Errorf("%q reads back as %q (%v)", data, got, err)
		}
	}
}

// failFS is a tree whose Open fails for one path.
type failFS struct {
	fs.FS
	path string
}

func (f failFS) Open(name string) (fs.File, error) {
	if name == f.path {
		return nil, &fs.PathError{Op: "open", Path: name, Err: errors.New("the disk is unreadable here")}
	}
	return f.FS.Open(name)
}

// TestSerializeReadError: a tree whose Open fails for one file fails
// Serialize, naming the tree and the path.
func TestSerializeReadError(t *testing.T) {
	tree := failFS{fstest.MapFS{"A1": {Data: []byte("1\n")}, "B1": {Data: []byte("3\n")}}, "B1"}
	_, err := Serialize(map[string]fs.FS{"sheet": tree})
	fails(t, err, "the tree sheet, at B1:", "the disk is unreadable here")
}

// TestSerializeRefusesWhatCantRoundTrip: an empty directory, a symlink, a
// file whose name an archive doesn't hold as written, and a tree with no
// files are each refused, naming the tree and the path.
func TestSerializeRefusesWhatCantRoundTrip(t *testing.T) {
	a1 := &fstest.MapFile{Data: []byte("1\n")}
	for _, c := range []struct {
		tree fstest.MapFS
		want []string
	}{
		{fstest.MapFS{"A1": a1, "notes": {Mode: fs.ModeDir}}, []string{"the tree sheet, at notes:", "an empty directory"}},
		{fstest.MapFS{"A1": a1, "x/notes": {Mode: fs.ModeDir}}, []string{"the tree sheet, at x/notes:", "an empty directory"}},
		{fstest.MapFS{"A1": a1, "B1": {Data: []byte("A1"), Mode: fs.ModeSymlink}}, []string{"the tree sheet, at B1:", "a symlink, not a regular file"}},
		{fstest.MapFS{"A1 ": a1}, []string{"the tree sheet, at A1 :", `"in/sheet/A1 "`, "can't be held in an archive: it starts or ends with white space"}},
		{fstest.MapFS{}, []string{"the tree sheet holds no files"}},
	} {
		ss, err := Serialize(map[string]fs.FS{"sheet": c.tree})
		fails(t, err, c.want...)
		if ss != nil {
			t.Errorf("Serialize failed, and gave sections: %q", names(ss))
		}
	}
}

// TestSerializeTreeNames: params, a/b, ., .. and the empty name are each
// refused, naming the tree; so is a nil tree, and no trees at all.
func TestSerializeTreeNames(t *testing.T) {
	tree := fstest.MapFS{"A1": {Data: []byte("1\n")}}
	for _, name := range []string{"params", "a/b", ".", "..", ""} {
		_, err := Serialize(map[string]fs.FS{name: tree})
		if name == "params" {
			fails(t, err, "the tree name params is reserved")
		} else {
			fails(t, err, `the tree name "`+name+`" isn't one path element`)
		}
	}
	// A name the section markers can't hold is the tree's to blame, not
	// its files'.
	for _, name := range []string{"a\nb", "sheet\n"} {
		_, err := Serialize(map[string]fs.FS{name: tree})
		fails(t, err, "the tree name "+strconv.Quote(name)+" can't be held in an archive, in the names of its sections: it holds a newline, which ends the marker line that names a section.")
	}
	_, err := Serialize(map[string]fs.FS{"sheet": nil})
	fails(t, err, "the tree sheet is nil")
	_, err = Serialize(nil)
	fails(t, err, "there are no trees to serialize")
}

// TestSerializeLarge: a 1 MiB file is written whole, and reads back
// identical.
func TestSerializeLarge(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789abcde\n"), 1<<16)
	ss := serialize(t, map[string]fs.FS{"sheet": fstest.MapFS{"A1": {Data: data}}})
	path, err := Create(newCorpus(t), project, sheetType, "large", ss)
	if err != nil {
		t.Fatal(err)
	}
	if got := archive.Parse(readFile(t, path)); len(got.Sections) != 1 || !bytes.Equal(got.Sections[0].Data, data) {
		t.Errorf("the 1 MiB file doesn't read back whole")
	}
}

// TestSerializeDirFS: a tree on disk serializes as its files, and a
// symlink in it is refused.
func TestSerializeDirFS(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/A1", []byte("=B1*2\n"))
	writeFile(t, dir+"/B1", []byte("3\n"))
	ss := serialize(t, map[string]fs.FS{"sheet": os.DirFS(dir)})
	if want := "-- in/sheet/A1 --\n=B1*2\n-- in/sheet/B1 --\n3\n"; string(format(ss)) != want {
		t.Errorf("the tree on disk serializes as:\n%s", format(ss))
	}
	if err := os.Symlink("A1", dir+"/C1"); err != nil {
		t.Fatal(err)
	}
	_, err := Serialize(map[string]fs.FS{"sheet": os.DirFS(dir)})
	fails(t, err, "the tree sheet, at C1:", "a symlink")
}
