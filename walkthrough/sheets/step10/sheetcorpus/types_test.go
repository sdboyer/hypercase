package sheetcorpus_test

import (
	"errors"
	"slices"
	"testing"
	"testing/fstest"

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
