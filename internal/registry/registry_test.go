package registry

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// TestParseAny: a tool reads every format up to its own, older ones in
// today's form, and refuses a newer one as a FormatError.
func TestParseAny(t *testing.T) {
	v1 := `{"format": 1, "updateVar": "X", "corpusRoot": "step05", "rootPackage": true, "properties": [{"name": "circular", "description": "A loop."}], "suites": []}`
	r, err := ParseAny([]byte(v1))
	if err != nil {
		t.Fatalf("format 1: %v", err)
	}
	if r.Format != 1 || r.CorpusRoot != "step05" || len(r.Properties) != 1 || r.IssueLinks != nil {
		t.Errorf("format 1 read as %+v", r)
	}

	now := Registry{UpdateVar: "X", CorpusRoot: ".", IssueLinks: []IssueLink{{Pattern: `sheets#(\d+)`, URL: "https://example.com/$1"}}}
	r, err = ParseAny(now.Marshal())
	if err != nil {
		t.Fatalf("format %d: %v", Format, err)
	}
	if r.Format != Format || len(r.IssueLinks) != 1 || r.IssueLinks[0].URL != "https://example.com/$1" {
		t.Errorf("format %d read as %+v", Format, r)
	}

	_, err = ParseAny([]byte(`{"format": 3}`))
	var fe *FormatError
	if !errors.As(err, &fe) || fe.Format != 3 {
		t.Errorf("format 3: got %v, want a FormatError", err)
	}

	for _, bad := range []string{
		`{"format": 0}`,
		`{"updateVar": "X"}`,
		`{"format": 1, "issueLinks": []}`, // a field format 1 never had
		`[]`,
	} {
		if _, err := ParseAny([]byte(bad)); err == nil || !strings.HasPrefix(err.Error(), "it is not a registry") {
			t.Errorf("%s: got %v, want it refused as no registry", bad, err)
		}
	}
}

// TestFindFS: registries are found where go list ./... looks, and nowhere
// else.
func TestFindFS(t *testing.T) {
	reg := &fstest.MapFile{Data: []byte("{}\n")}
	fsys := fstest.MapFS{
		"go.mod":                             {Data: []byte("module x\n")},
		"testdata/hypercase.json":            reg,
		"a/testdata/hypercase.json":          reg,
		"a/b/testdata/hypercase.json":        reg,
		"testdata/c/testdata/hypercase.json": reg,
		"vendor/v/testdata/hypercase.json":   reg,
		".hidden/testdata/hypercase.json":    reg,
		"_skip/testdata/hypercase.json":      reg,
		"nested/go.mod":                      {Data: []byte("module y\n")},
		"nested/testdata/hypercase.json":     reg,
		"dir/testdata/hypercase.json/x":      reg, // a directory, not a registry
	}
	got, err := FindFS(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{".", "a", "a/b"}; !slices.Equal(got, want) {
		t.Errorf("FindFS = %q, want %q", got, want)
	}
}
