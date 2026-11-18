package sheetcorpus_test

import (
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"slices"
	"testing"
	"testing/fstest"
	"unicode"

	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step11/sheetcorpus"
)

// TestImportInput: import finds its CSV by its tree, and errs on an input
// that breaks the corpus type's rule (one file, in/csv/<name>.csv, with a
// name holding a character that is neither white space nor a format
// character). White space is Unicode's White_Space, and a format character
// is one of its category Cf, such as a zero-width space. The error is its
// own, not the importer's, so it carries no code, and no mark of
// decl.Reject. It gives the path in the jenny's view, csv/<name>.csv, as it
// gives the files the input holds.
func TestImportInput(t *testing.T) {
	csv := &fstest.MapFile{Data: []byte("4,6\n")}
	type refusal struct {
		name string
		in   fstest.MapFS
		want string // the files the error names
	}
	refusals := []refusal{
		{"empty", fstest.MapFS{}, `[]`},
		{"none", fstest.MapFS{"notes/about": {Data: []byte("no CSV\n")}}, `["notes/about"]`},
		{"two", fstest.MapFS{"csv/a.csv": csv, "csv/b.csv": csv}, `["csv/a.csv" "csv/b.csv"]`},
		{"upper", fstest.MapFS{"csv/prices.CSV": csv}, `["csv/prices.CSV"]`},
		{"txt", fstest.MapFS{"csv/prices.txt": csv}, `["csv/prices.txt"]`},
		{"nested", fstest.MapFS{"csv/sub/a.csv": csv}, `["csv/sub/a.csv"]`},
		{"readme", fstest.MapFS{"csv/a.csv": csv, "csv/README": csv}, `["csv/README" "csv/a.csv"]`},
		{"outside", fstest.MapFS{"csv/a.csv": csv, "notes/about": csv}, `["csv/a.csv" "notes/about"]`},
		{"directory", fstest.MapFS{"csv/x.csv/y": csv}, `["csv/x.csv/y"]`},
		{"unnamed", fstest.MapFS{"csv/.csv": csv}, `["csv/.csv"]`},
		{"spaces", fstest.MapFS{"csv/   .csv": csv}, `["csv/   .csv"]`},
		{"mixed", fstest.MapFS{"csv/ \u200b\t.csv": csv}, `["csv/ \u200b\t.csv"]`},
	}
	// A name of one rune is refused for each rune of White_Space and of Cf,
	// every one taken from Unicode's tables, so none is left out.
	for _, table := range []*unicode.RangeTable{unicode.White_Space, unicode.Cf} {
		for _, r := range runes(table) {
			p := "csv/" + string(r) + ".csv"
			refusals = append(refusals, refusal{fmt.Sprintf("%U", r), fstest.MapFS{p: csv}, fmt.Sprintf("%q", []string{p})})
		}
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sheetcorpus.Import.Jenny.Generate(sheetcorpus.CSV{FS: tt.in})
			want := "import: the input holds " + tt.want + ", and a CSV export is one file, csv/<name>.csv, with a name holding a character that is neither white space nor a format character"
			if err == nil || err.Error() != want {
				t.Fatalf("got %v, want %s", err, want)
			}
			var e *sheet.Error
			if errors.As(err, &e) {
				t.Errorf("the error carries the importer's code %s", e.Code)
			}
			if marked(err) {
				t.Errorf("the error carries decl.Reject's mark: %v", err)
			}
		})
	}
	// A name holding a character that is neither white space nor a format
	// character imports, whatever else it holds: one dot does, and so do a
	// letter between spaces and a letter after a zero-width space.
	for _, tt := range []struct{ name, path string }{
		{"word", "csv/one.csv"},
		{"dot", "csv/..csv"},
		{"letter", "csv/ a .csv"},
		{"zwsp-letter", "csv/\u200ba.csv"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files, err := sheetcorpus.Import.Jenny.Generate(sheetcorpus.CSV{FS: fstest.MapFS{tt.path: csv}})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, f := range files {
				data, err := f.Render()
				if err != nil {
					t.Fatalf("rendering %s: %v", f.Path(), err)
				}
				got = append(got, f.Path()+"="+string(data))
			}
			if want := []string{"sheet/A1=4\n", "sheet/B1=6\n"}; !slices.Equal(got, want) {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

// runes lists every rune of table, its 16-bit ranges and its 32-bit ones.
func runes(table *unicode.RangeTable) []rune {
	var rs []rune
	for _, r := range table.R16 {
		for c := rune(r.Lo); c <= rune(r.Hi); c += rune(r.Stride) {
			rs = append(rs, c)
		}
	}
	for _, r := range table.R32 {
		for c := rune(r.Lo); c <= rune(r.Hi); c += rune(r.Stride) {
			rs = append(rs, c)
		}
	}
	return rs
}

// TestImportReadFault: import rejects a case only with the importer's own
// error. A CSV it can't read, or an input it can't walk, is a fault,
// returned unmarked.
func TestImportReadFault(t *testing.T) {
	for _, name := range []string{"csv/a.csv", "csv"} {
		t.Run(name, func(t *testing.T) {
			in := unopenable{fstest.MapFS{"csv/a.csv": {Data: []byte("4,6\n")}}, name}
			_, err := sheetcorpus.Import.Jenny.Generate(sheetcorpus.CSV{FS: in})
			if !errors.Is(err, fs.ErrPermission) {
				t.Fatalf("got %v, want the open's error", err)
			}
			if marked(err) {
				t.Errorf("the read's error carries decl.Reject's mark: %v", err)
			}
		})
	}
}

// unopenable is a view holding csv/a.csv, in which one name, a file's or a
// directory's, can't be opened.
type unopenable struct {
	files fstest.MapFS
	name  string
}

func (u unopenable) Open(name string) (fs.File, error) {
	if name == u.name {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return u.files.Open(name)
}

// marked reports whether some error in err's tree is one decl.Reject made.
// An error with no mark anywhere is no rejection, which needs a mark on
// every path through the tree, so a test that wants no rejection wants no
// mark. The mark's type is decl's own, so it is taken from a mark Reject
// makes.
func marked(err error) bool {
	mark := reflect.New(reflect.TypeOf(decl.Reject(errors.New("a rejection"))))
	return errors.As(err, mark.Interface())
}
