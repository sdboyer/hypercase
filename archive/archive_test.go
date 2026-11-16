package archive

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

const wellFormed = `Pancakes for two.
-- in/recipe/pancakes.txt --
serves: 2
flour: 100 g
-- out/scale/card.txt --
serves: 4
flour: 200 g
-- out/scale/empty --
`

func TestRoundTrip(t *testing.T) {
	a := Parse([]byte(wellFormed))
	if got := string(a.Format()); got != wellFormed {
		t.Errorf("formatting a parsed archive changed it:\n%s", got)
	}
	if ps := a.Check(); len(ps) > 0 {
		t.Errorf("a well-formed archive has problems: %v", ps)
	}
	if !Placed([]byte(wellFormed)) {
		t.Error("a well-formed archive isn't placed")
	}
}

// TestParseDoesNotAlias: an append to the comment, or to a section, leaves
// every other section as it was.
func TestParseDoesNotAlias(t *testing.T) {
	a := Parse([]byte(wellFormed))
	a.Comment = append(a.Comment, "A longer comment, long enough to run over the next section.\n"...)
	if got, want := string(a.Sections[0].Data), "serves: 2\nflour: 100 g\n"; got != want {
		t.Errorf("an append to the comment changed the first section to %q", got)
	}
	a.Sections[0].Data = append(a.Sections[0].Data, "A longer line, long enough to run over the next section.\n"...)
	if got, want := string(a.Sections[1].Data), "serves: 4\nflour: 200 g\n"; got != want {
		t.Errorf("an append to the first section changed the second to %q", got)
	}
}

func TestHolds(t *testing.T) {
	for _, tt := range []struct {
		data string
		want string // "" when it holds
	}{
		{"", ""},
		{"a\n", ""},
		{"a\n\n", ""},
		{"a\r\n", ""},
		{"a", "it has no final newline"},
		{"a\n-- b --\n", "its line 2 is shaped like a section marker"},
		{"-- b --\n", "its line 1 is shaped like a section marker"},
		{"a\n-- b --\r\n", "its line 2 is shaped like a section marker"},
		{"a\n--  --\n", ""},   // an empty name is no marker
		{"a\n-- b -\n", ""},   // nor is a line without the closing --
		{"a\n -- b --\n", ""}, // nor one that doesn't start the line
	} {
		err := Holds([]byte(tt.data))
		switch {
		case tt.want == "" && err != nil:
			t.Errorf("Holds(%q) = %v, want nil", tt.data, err)
		case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
			t.Errorf("Holds(%q) = %v, want %q", tt.data, err, tt.want)
		}
	}
}

func TestWriteRefusesWhatReadsBackDifferently(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "x.txtar")
	if err := os.WriteFile(name, []byte(wellFormed), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"no newline", "a\n-- b --\n"} {
		a := Parse([]byte(wellFormed))
		a.Sections = append(a.Sections, Section{Name: "out/scale/x", Data: []byte(data)})
		if err := Write(name, a); err == nil {
			t.Errorf("Write of %q succeeded", data)
		}
		if got, _ := os.ReadFile(name); string(got) != wellFormed {
			t.Errorf("a refused write changed the file:\n%s", got)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("a refused write left files: %v", entries)
	}
}

func TestFailedWriteLeavesTheFile(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "x.txtar")
	if err := os.WriteFile(name, []byte(wellFormed), 0o644); err != nil {
		t.Fatal(err)
	}
	a := Parse([]byte(wellFormed))
	a.Sections[1].Data = []byte("serves: 8\n")
	var sawTemp string
	afterTemp = func(tmp string) error {
		sawTemp = tmp
		if !IsTemp(tmp) || strings.HasSuffix(tmp, ".txtar") {
			t.Errorf("the temp file %s isn't named as a temp file", tmp)
		}
		return errors.New("disk full")
	}
	err := Write(name, a)
	afterTemp = nil
	if err == nil || sawTemp == "" {
		t.Fatalf("Write = %v, and the hook saw %q", err, sawTemp)
	}
	if got, _ := os.ReadFile(name); string(got) != wellFormed {
		t.Errorf("a failed write changed the file:\n%s", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("a failed write left its temp file: %v", entries)
	}
	if err := Write(name, a); err != nil {
		t.Fatalf("the next write failed: %v", err)
	}
	if got, _ := os.ReadFile(name); !bytes.Contains(got, []byte("serves: 8\n")) {
		t.Errorf("the next write didn't land:\n%s", got)
	}
}

// TestWriteFileKeepsMode: a write replaces a file's bytes and keeps its
// mode, and a new file is 0644.
func TestWriteFileKeepsMode(t *testing.T) {
	dir := t.TempDir()
	for _, mode := range []fs.FileMode{0o600, 0o664, 0o755} {
		name := filepath.Join(dir, mode.String()+".txtar")
		if err := os.WriteFile(name, []byte(wellFormed), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(name, mode); err != nil {
			t.Fatal(err)
		}
		if err := WriteFile(name, []byte("A case.\n")); err != nil {
			t.Fatal(err)
		}
		if fi, err := os.Stat(name); err != nil {
			t.Fatal(err)
		} else if fi.Mode() != mode {
			t.Errorf("a write over a file of mode %v left it %v", mode, fi.Mode())
		}
	}
	name := filepath.Join(dir, "new.txtar")
	if err := WriteFile(name, []byte("A case.\n")); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(name); err != nil {
		t.Fatal(err)
	} else if fi.Mode() != 0o644 {
		t.Errorf("a new file's mode is %v, not %v", fi.Mode(), fs.FileMode(0o644))
	}
}

func TestView(t *testing.T) {
	a := Parse([]byte(`A sheet.
-- in/sheet/A1 --
=B1*2
-- in/sheet/B1 --
3
-- in/recipe/pancakes.txt --
serves: 2
-- in/deep/a/b/c --
c
-- out/evaluate/values.txt --
A1 6
`))
	v := a.View()
	if err := fstest.TestFS(v, "sheet/A1", "sheet/B1", "recipe/pancakes.txt", "deep/a/b/c"); err != nil {
		t.Fatal(err)
	}
	paths, err := Paths(v)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths, " "); got != "deep/a/b/c recipe/pancakes.txt sheet/A1 sheet/B1" {
		t.Errorf("Paths = %s", got)
	}
	if _, err := fs.Stat(v, "out"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the view holds out/: %v", err)
	}
	// A reader that changes what it read changes nothing for the next.
	data, _ := fs.ReadFile(v, "sheet/B1")
	data[0] = '9'
	if again, _ := fs.ReadFile(v, "sheet/B1"); string(again) != "3\n" {
		t.Errorf("the view changed under its reader: %q", again)
	}
}

func TestEmptyView(t *testing.T) {
	if err := fstest.TestFS(Parse([]byte("Nothing.\n")).View()); err != nil {
		t.Fatal(err)
	}
}

func TestPlace(t *testing.T) {
	a := Parse([]byte(`Out of place.
-- out/b/x --
b
-- in/t/1 --
1
-- out/a/y --
a
-- in/t/2 --
2
`))
	want := `Out of place.
-- in/t/1 --
1
-- in/t/2 --
2
-- out/a/y --
a
-- out/b/x --
b
`
	if got := string(a.Place().Format()); got != want {
		t.Errorf("Place:\n%s", got)
	}
	if Placed(a.Format()) {
		t.Error("an archive out of place reads as placed")
	}
}

// TestPlaceVia: a case's via/ sections sit between its inputs and its
// outputs, sorted by name: in the base and in a hand-written variant after
// the hand-written sections, and in a transform's variant after its in/
// and delete.
func TestPlaceVia(t *testing.T) {
	a := Parse([]byte(`Products out of place.
-- via/import/sheet/B1 --
b
-- in/csv/x.csv --
1
-- out/import/outcome --
accepted
-- via/import/sheet/A1 --
a
-- variant/v/about --
A variant.
-- variant/v/out/import/outcome --
accepted
-- variant/v/via/import/sheet/A1 --
va
-- variant/v/in/csv/x.csv --
2
-- transform/t/via/import/sheet/A1 --
ta
-- transform/t/out/import/outcome --
accepted
-- transform/t/delete --
csv/y.csv
-- transform/t/in/csv/x.csv --
3
`))
	want := `Products out of place.
-- in/csv/x.csv --
1
-- via/import/sheet/A1 --
a
-- via/import/sheet/B1 --
b
-- out/import/outcome --
accepted
-- variant/v/about --
A variant.
-- variant/v/in/csv/x.csv --
2
-- variant/v/via/import/sheet/A1 --
va
-- variant/v/out/import/outcome --
accepted
-- transform/t/in/csv/x.csv --
3
-- transform/t/delete --
csv/y.csv
-- transform/t/via/import/sheet/A1 --
ta
-- transform/t/out/import/outcome --
accepted
`
	if got := string(a.Place().Format()); got != want {
		t.Errorf("Place:\n%s", got)
	}
	if Placed(a.Format()) {
		t.Error("an archive out of place reads as placed")
	}
	if !Placed([]byte(want)) {
		t.Error("the placed archive doesn't read as placed")
	}
}

func TestNoTesting(t *testing.T) {
	for _, pkg := range []string{"github.com/sdboyer/hypercase/archive", "github.com/sdboyer/hypercase/decl"} {
		out, err := exec.Command("go", "list", "-deps", pkg).Output()
		if err != nil {
			t.Fatal(err)
		}
		for _, dep := range strings.Fields(string(out)) {
			if dep == "testing" {
				t.Errorf("%s imports testing", pkg)
			}
		}
	}
}
