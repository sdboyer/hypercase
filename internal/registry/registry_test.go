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
