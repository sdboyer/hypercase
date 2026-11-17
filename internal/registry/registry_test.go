package registry

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// olderData is a registry of an older format, which only Peek reads.
const olderData = `{"format": 1, "updateVar": "X", "corpusRoot": "step05", "rootPackage": true,
	"properties": [{"name": "circular", "description": "A loop."}],
	"suites": [{"name": "calc", "description": "Runs calc.", "corpus": "sheet",
		"outputs": [{"pattern": "stdout.txt", "description": "What it prints."}], "rejects": ["circular"]}]}`

// TestParse: a registry of this hypercase's format reads back as what was
// marshaled; one of another format, older or newer, is a FormatError that
// names it; and what is no registry is refused as none.
func TestParse(t *testing.T) {
	now := Registry{
		UpdateVar: "X", CorpusRoot: ".",
		IssueLinks: []IssueLink{{Pattern: `sheets#(\d+)`, URL: "https://example.com/$1"}},
		Types: []Type{{Dir: "sheet", Description: "A sheet.", GoType: "example.com/m.Sheet", Transforms: []Transform{
			{Name: "renamed", Description: "Moves.", Gives: []string{}, Keeps: []string{"dangling", "circular"}, Probe: true},
			{Name: "looped", Description: "Loops.", Gives: []string{"circular"}, Owned: true},
		}}},
		Suites: []Suite{{
			Name: "calc", Description: "Runs calc.", Consumes: RawInput, Corpora: []string{"sheet"},
			Outputs: []Output{{Pattern: "stdout.txt", Description: "What it prints."}}, Rejects: []string{"circular"},
		}},
	}
	got, err := Parse(now.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Marshal(), now.Marshal()) {
		t.Errorf("format %d read back as\n%s\nwant\n%s", Format, got.Marshal(), now.Marshal())
	}
	// Marshal sorts the transforms by name, and their properties.
	if trs := got.Types[0].Transforms; trs[0].Name != "looped" || !slices.Equal(trs[1].Keeps, []string{"circular", "dangling"}) || trs[0].Keeps == nil {
		t.Errorf("the transforms are written as %+v", trs)
	}

	for _, data := range []string{olderData, fmt.Sprintf(`{"format": %d}`, Format+1)} {
		_, err = Parse([]byte(data))
		var fe *FormatError
		want := 1
		if data != olderData {
			want = Format + 1
		}
		if !errors.As(err, &fe) || fe.Format != want {
			t.Errorf("format %d: got %v, want a FormatError", want, err)
		}
	}

	for _, bad := range []string{
		`{"format": 0}`,
		`{"updateVar": "X"}`,
		`{"format": "3"}`,
		fmt.Sprintf(`{"format": %d, "a field this hypercase doesn't know": []}`, Format),
		`[]`,
	} {
		if _, err := Parse([]byte(bad)); err == nil || !strings.HasPrefix(err.Error(), "it is not a registry") {
			t.Errorf("%s: got %v, want it refused as no registry", bad, err)
		}
	}
}

// TestParseAny: a registry of each format from Oldest to Format reads in
// today's form, holding what its format holds; one that holds a key a
// later format added is no registry; and a format older than Oldest or
// newer than Format is a FormatError that names it. Parse still reads
// Format alone.
func TestParseAny(t *testing.T) {
	// A format 3 registry, as hypercase's main branch wrote it: corpus
	// types with no transforms, no codes, and suites with no classifier.
	const v3 = `{"format": 3, "updateVar": "X", "corpusRoot": ".", "rootPackage": true,
	"issueLinks": [{"pattern": "sheets#(\\d+)", "url": "https://example.com/$1"}],
	"properties": [{"name": "circular", "description": "A loop."}],
	"types": [{"dir": "sheet", "description": "A sheet.", "goType": "example.com/m.Sheet"}],
	"lifts": [],
	"suites": [{"name": "calc", "description": "Runs calc.", "consumes": "io/fs.FS", "corpora": ["sheet"],
		"outputs": [{"pattern": "stdout.txt", "description": "What it prints."}], "rejects": ["circular"]}]}`
	v4 := strings.Replace(v3, `"format": 3`, `"format": 4`, 1)
	v4 = strings.Replace(v4, `"goType": "example.com/m.Sheet"}`, `"goType": "example.com/m.Sheet",
		"transforms": [{"name": "looped", "description": "Loops.", "gives": ["circular"], "keeps": [], "probe": false, "owned": true}]}`, 1)
	// A format 5 registry, which says whether the project supplies Codes
	// and gives each suite's classifier, with a lift that says nothing of
	// bytes or a product's files.
	v5 := strings.Replace(v4, `"format": 4`, `"format": 5`, 1)
	v5 = strings.Replace(v5, `"rootPackage": true,`, `"rootPackage": true, "codes": false,`, 1)
	v5 = strings.Replace(v5, `"rejects": ["circular"]}`, `"rejects": ["circular"], "classifier": null}`, 1)
	v5 = strings.Replace(v5, `"lifts": []`, `"lifts": [{"name": "parse", "description": "Parses.", "input": "example.com/m.Sheet",
		"product": "example.com/m.Parsed", "owned": true, "corpora": ["sheet"], "outputs": [], "rejects": []}]`, 1)
	for _, tt := range []struct {
		data              string
		format            int
		transforms, lifts int
	}{
		{v3, 3, 0, 0},
		{v4, 4, 1, 0},
		{v5, 5, 1, 1},
	} {
		g, err := ParseAny([]byte(tt.data))
		if err != nil {
			t.Errorf("format %d: %v", tt.format, err)
			continue
		}
		if g.Format != tt.format || g.CorpusRoot != "." || !g.RootPackage || len(g.IssueLinks) != 1 ||
			len(g.Properties) != 1 || len(g.Suites) != 1 || g.Suites[0].Outputs[0].Pattern != "stdout.txt" ||
			len(g.Types) != 1 || len(g.Types[0].Transforms) != tt.transforms || g.Codes || g.Suites[0].Classifier != nil ||
			len(g.Lifts) != tt.lifts || slices.ContainsFunc(g.Lifts, func(l Lift) bool { return l.Bytes || l.ProductFiles != nil }) {
			t.Errorf("format %d reads as %+v", tt.format, g)
		}
		if _, err := Parse([]byte(tt.data)); !errors.As(err, new(*FormatError)) {
			t.Errorf("Parse of format %d: got %v, want a FormatError", tt.format, err)
		}
	}
	now := Registry{CorpusRoot: ".", Codes: true, Suites: []Suite{{Name: "evaluate", Classifier: &Classifier{Name: "div-zero", Description: "Divides by zero."}}},
		Lifts: []Lift{{Name: "import", Bytes: true, ProductFiles: []Output{{Pattern: "sheet/*", Description: "A cell."}}}}}
	if g, err := ParseAny(now.Marshal()); err != nil || !bytes.Equal(g.Marshal(), now.Marshal()) {
		t.Errorf("format %d: got %+v, %v", Format, g, err)
	}

	for _, tt := range []struct{ data, want string }{
		{strings.Replace(v3, `"goType": "example.com/m.Sheet"}`, `"goType": "example.com/m.Sheet", "transforms": []}`, 1), "it is not a registry: it holds format 3, and types[].transforms, which format 4 added"},
		{strings.Replace(v3, `"rootPackage": true,`, `"rootPackage": true, "codes": false,`, 1), "it is not a registry: it holds format 3, and codes, which format 5 added"},
		{strings.Replace(v4, `"rootPackage": true,`, `"rootPackage": true, "codes": null,`, 1), "it is not a registry: it holds format 4, and codes, which format 5 added"},
		{strings.Replace(v4, `"rejects": ["circular"]}`, `"rejects": ["circular"], "classifier": null}`, 1), "it is not a registry: it holds format 4, and suites[].classifier, which format 5 added"},
		{strings.Replace(v5, `"owned": true, "corpora"`, `"owned": true, "bytes": false, "corpora"`, 1), "it is not a registry: it holds format 5, and lifts[].bytes, which format 6 added"},
		{strings.Replace(v5, `"rejects": []}]`, `"rejects": [], "productFiles": null}]`, 1), "it is not a registry: it holds format 5, and lifts[].productFiles, which format 6 added"},
		{strings.Replace(v4, `"rejects": ["circular"]}`, `"rejects": ["circular"], "later": 1}`, 1), `it is not a registry: json: unknown field "later"`},
		{`{"format": 0}`, "it is not a registry: it holds no format version"},
		{`[]`, "it is not a registry"},
	} {
		if _, err := ParseAny([]byte(tt.data)); err == nil || !strings.HasPrefix(err.Error(), tt.want) {
			t.Errorf("%.60s…: got %v, want %q", tt.data, err, tt.want)
		}
	}
	for _, format := range []int{1, 2, Format + 1} {
		_, err := ParseAny(fmt.Appendf(nil, `{"format": %d}`, format))
		var fe *FormatError
		if !errors.As(err, &fe) || fe.Format != format {
			t.Errorf("format %d: got %v, want a FormatError", format, err)
		}
	}
}

// TestPeek: a registry's corpus root is read whatever its format, an older
// or a newer one's included, so a reader can leave another root's registry
// unread.
func TestPeek(t *testing.T) {
	newer := fmt.Sprintf(`{"format": %d, "corpusRoot": "step09", "rootPackage": true, "a field of a later hypercase": {}}`, Format+1)
	for _, tt := range []struct{ data, root string }{
		{olderData, "step05"},
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

// TestVariantProperties: a transform's variant states what the transform
// gives, then what it keeps that the base states, in the base's order; a
// base that states a property the transform neither gives nor keeps is
// declined, and its variant states nothing.
func TestVariantProperties(t *testing.T) {
	for _, c := range []struct {
		name                   string
		gives, keeps, base     []string
		wantProps, wantNeither []string
	}{
		{name: "gives", gives: []string{"circular"}, wantProps: []string{"circular"}},
		{name: "keeps in the base's order", gives: []string{"x"}, keeps: []string{"b", "a", "c"}, base: []string{"a", "b"}, wantProps: []string{"x", "a", "b"}},
		{name: "a given property the base states", gives: []string{"a"}, keeps: []string{"b"}, base: []string{"b", "a"}, wantProps: []string{"a", "b"}},
		{name: "declines", gives: []string{"circular"}, keeps: []string{"a"}, base: []string{"a", "dangling", "z"}, wantNeither: []string{"dangling", "z"}},
	} {
		props, neither := VariantProperties(c.gives, c.keeps, c.base)
		if !slices.Equal(props, c.wantProps) || !slices.Equal(neither, c.wantNeither) {
			t.Errorf("%s: VariantProperties = %q, %q, want %q, %q", c.name, props, neither, c.wantProps, c.wantNeither)
		}
	}
}
