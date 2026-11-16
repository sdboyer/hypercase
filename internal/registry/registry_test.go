package registry

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// The registries of each format, and what ParseAny gives for them.
const (
	v1Data = `{"format": 1, "updateVar": "X", "corpusRoot": "step05", "rootPackage": true,
		"properties": [{"name": "circular", "description": "A loop."}],
		"suites": [{"name": "calc", "description": "Runs calc.", "corpus": "sheet",
			"outputs": [{"pattern": "stdout.txt", "description": "What it prints."}], "rejects": ["circular"]}]}`
	v2LinksData = `{"format": 2, "updateVar": "X", "corpusRoot": "step05", "rootPackage": true,
		"issueLinks": [{"pattern": "sheets#(\\d+)", "url": "https://example.com/$1"}],
		"properties": [{"name": "circular", "description": "A loop."}],
		"suites": [{"name": "calc", "description": "Runs calc.", "corpus": "sheet",
			"outputs": [{"pattern": "stdout.txt", "description": "What it prints."}], "rejects": ["circular"]}]}`
	v2TypesData = `{"format": 2, "updateVar": "X", "corpusRoot": "step06", "rootPackage": false,
		"properties": [],
		"types": [{"dir": "sheet", "description": "A sheet.", "goType": "example.com/m.Sheet"}],
		"lifts": [{"name": "parse", "description": "Parses.", "input": "example.com/m.Sheet", "product": "*example.com/m.Parsed",
			"owned": true, "corpora": ["sheet"], "outputs": [], "rejects": []}],
		"suites": [{"name": "format", "description": "Formats.", "consumes": "*example.com/m.Parsed", "corpora": ["sheet"],
			"outputs": [], "rejects": []}]}`
)

func rawSuite() Suite {
	return Suite{
		Name: "calc", Description: "Runs calc.", Consumes: RawInput, Corpora: []string{"sheet"},
		Outputs: []Output{{Pattern: "stdout.txt", Description: "What it prints."}}, Rejects: []string{"circular"},
	}
}

var (
	props       = []Property{{Name: "circular", Description: "A loop."}}
	v1Want      = &Registry{Format: 1, UpdateVar: "X", CorpusRoot: "step05", RootPackage: true, Properties: props, Suites: []Suite{rawSuite()}}
	v2LinksWant = &Registry{
		Format: 2, UpdateVar: "X", CorpusRoot: "step05", RootPackage: true,
		IssueLinks: []IssueLink{{Pattern: `sheets#(\d+)`, URL: "https://example.com/$1"}},
		Properties: props, Suites: []Suite{rawSuite()},
	}
	v2TypesWant = &Registry{
		Format: 2, UpdateVar: "X", CorpusRoot: "step06", Properties: []Property{},
		Types: []Type{{Dir: "sheet", Description: "A sheet.", GoType: "example.com/m.Sheet"}},
		Lifts: []Lift{{
			Name: "parse", Description: "Parses.", Input: "example.com/m.Sheet", Product: "*example.com/m.Parsed",
			Owned: true, Corpora: []string{"sheet"}, Outputs: []Output{}, Rejects: []string{},
		}},
		Suites: []Suite{{Name: "format", Description: "Formats.", Consumes: "*example.com/m.Parsed", Corpora: []string{"sheet"}, Outputs: []Output{}, Rejects: []string{}}},
	}
)

// TestParseAny: a tool reads every format up to its own, older ones in
// today's form, both format 2s among them, and refuses a newer one as a
// FormatError.
func TestParseAny(t *testing.T) {
	now := Registry{
		UpdateVar: "X", CorpusRoot: ".",
		IssueLinks: []IssueLink{{Pattern: `sheets#(\d+)`, URL: "https://example.com/$1"}},
		Types:      []Type{{Dir: "sheet", Description: "A sheet.", GoType: "example.com/m.Sheet"}},
		Suites:     []Suite{rawSuite()},
	}
	nowWant, err := Parse(now.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		data string
		want *Registry
	}{
		{"format 1", v1Data, v1Want},
		{"format 2 with issue links", v2LinksData, v2LinksWant},
		{"format 2 with corpus types and lifts", v2TypesData, v2TypesWant},
		{fmt.Sprintf("format %d", Format), string(now.Marshal()), nowWant},
	} {
		got, err := ParseAny([]byte(tt.data))
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s read as\n%+v\nwant\n%+v", tt.name, got, tt.want)
		}
	}

	_, err = ParseAny(fmt.Appendf(nil, `{"format": %d}`, Format+1))
	var fe *FormatError
	if !errors.As(err, &fe) || fe.Format != Format+1 {
		t.Errorf("format %d: got %v, want a FormatError", Format+1, err)
	}

	for _, bad := range []string{
		`{"format": 0}`,
		`{"updateVar": "X"}`,
		`{"format": "2"}`,
		`{"format": 1, "issueLinks": []}`,                         // a field format 1 never had
		`{"format": 2, "issueLinks": [], "types": []}`,            // fields of both format 2s
		`{"format": 2, "suites": [{"name": "x", "corpus": "a"}]}`, // a suite of the other format 2
		`[]`,
	} {
		if _, err := ParseAny([]byte(bad)); err == nil || !strings.HasPrefix(err.Error(), "it is not a registry") {
			t.Errorf("%s: got %v, want it refused as no registry", bad, err)
		}
	}
}

// TestPeek: a registry's corpus root is read whatever its format, a newer
// one's included, so a reader can leave another root's registry unread.
func TestPeek(t *testing.T) {
	newer := fmt.Sprintf(`{"format": %d, "corpusRoot": "step09", "rootPackage": true, "a field of a later hypercase": {}}`, Format+1)
	for _, tt := range []struct{ data, root string }{
		{v1Data, "step05"},
		{v2LinksData, "step05"},
		{v2TypesData, "step06"},
		{string(Registry{CorpusRoot: "."}.Marshal()), "."},
		{newer, "step09"},
	} {
		h, err := Peek([]byte(tt.data))
		if err != nil || h.CorpusRoot != tt.root {
			t.Errorf("Peek(%.40s…) = %+v, %v; want the corpus root %s", tt.data, h, err, tt.root)
		}
	}
	if h, _ := Peek([]byte(newer)); h.Format != Format+1 || !h.RootPackage {
		t.Errorf("Peek of a newer format = %+v", h)
	}

	_, err := Peek(fmt.Appendf(nil, `{"format": %d}`, Format+1))
	var fe *FormatError
	if !errors.As(err, &fe) || fe.Format != Format+1 {
		t.Errorf("a newer format that names no corpus root: got %v, want a FormatError", err)
	}
	for _, bad := range []string{`{"format": 1}`, `{"format": 3, "corpusRoot": 7}`, `not json`} {
		if _, err := Peek([]byte(bad)); err == nil || !strings.HasPrefix(err.Error(), "it is not a registry") {
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
