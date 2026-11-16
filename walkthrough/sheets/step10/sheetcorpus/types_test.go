package sheetcorpus_test

import (
	"errors"
	"io/fs"
	"reflect"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step10/sheetcorpus"
)

// TestImportCount: import finds its CSV by its tree, and errs on a CSV
// archive without exactly one, with an error of its own, not the
// importer's, so the error carries no code.
func TestImportCount(t *testing.T) {
	csv := &fstest.MapFile{Data: []byte("4,6\n")}
	for _, tt := range []struct {
		name string
		in   fstest.MapFS
		want string
	}{
		{"none", fstest.MapFS{"notes/about": {Data: []byte("no CSV\n")}}, "import: 0 files under csv/, not one"},
		{"two", fstest.MapFS{"csv/a.csv": csv, "csv/b.csv": csv}, "import: 2 files under csv/, not one"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sheetcorpus.Import.Jenny.Generate(sheetcorpus.CSV{FS: tt.in})
			if err == nil || err.Error() != tt.want {
				t.Fatalf("got %v, want %s", err, tt.want)
			}
			var e *sheet.Error
			if errors.As(err, &e) {
				t.Errorf("the error carries the importer's code %s", e.Code)
			}
		})
	}
	files, err := sheetcorpus.Import.Jenny.Generate(sheetcorpus.CSV{FS: fstest.MapFS{"csv/one.csv": csv}})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range files {
		data, err := f.Render()
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, f.Path()+"="+string(data))
	}
	if want := []string{"sheet/A1=4\n", "sheet/B1=6\n"}; !slices.Equal(got, want) {
		t.Errorf("one CSV: got %q, want %q", got, want)
	}
}

// TestImportReadFault: import rejects a case only with the importer's own
// error. A CSV it can't read is a fault, returned unmarked.
func TestImportReadFault(t *testing.T) {
	in := unreadable{fstest.MapFS{"csv/a.csv": {Data: []byte("4,6\n")}}}
	_, err := sheetcorpus.Import.Jenny.Generate(sheetcorpus.CSV{FS: in})
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("got %v, want the read's error", err)
	}
	if rejected(err) {
		t.Errorf("import rejects the case with a read error: %v", err)
	}
}

// unreadable is a view that lists csv/a.csv, and can't open it.
type unreadable struct{ files fstest.MapFS }

func (u unreadable) Open(name string) (fs.File, error) {
	if name == "csv/a.csv" {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return u.files.Open(name)
}

// rejected reports whether err carries decl.Reject's mark. The mark's type
// is decl's own, so it is taken from a mark Reject makes.
func rejected(err error) bool {
	mark := reflect.New(reflect.TypeOf(decl.Reject(errors.New("a rejection"))))
	return errors.As(err, mark.Interface())
}
