package registry

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/sdboyer/hypercase/internal/diff"
	"github.com/sdboyer/hypercase/internal/registrytest"
)

// olderData is a registry of an older format, which only Peek reads.
const olderData = `{"format": 1, "updateVar": "X", "corpusRoot": "step05", "rootPackage": true,
	"properties": [{"name": "circular", "description": "A loop."}],
	"suites": [{"name": "calc", "description": "Runs calc.", "corpus": "sheet",
		"outputs": [{"pattern": "stdout.txt", "description": "What it prints."}], "rejects": ["circular"]}]}`

// TestParse: a registry of this hypercase's format reads back as what was
// marshaled; one of another format, older or newer, is a FormatError that
// names it; and what is no registry is refused as none. Marshal sorts a
// column's paths by corpus, and keeps each path's lifts in their order,
// the nearest to the column first, writing none as []. A registry the
// update couldn't have written is refused: one whose paths it couldn't
// have planned, or that records an Inputs registration refuses.
func TestParse(t *testing.T) {
	now := Registry{
		UpdateVar: "X", CorpusRoot: ".", Positions: true,
		IssueLinks: []IssueLink{{Pattern: `sheets#(\d+)`, URL: "https://example.com/$1"}},
		Types: []Type{{Dir: "sheet", Description: "A sheet.", GoType: "example.com/m.Sheet", Transforms: []Transform{
			{Name: "renamed", Description: "Moves.", Gives: []string{}, Keeps: []string{"dangling", "circular"}, Probe: true},
			{Name: "looped", Description: "Loops.", Gives: []string{"circular"}, Owned: true},
		}}},
		Suites: []Suite{{
			Name: "calc", Description: "Runs calc.", Consumes: RawInput, Corpora: []string{"sheet"}, Paths: []Path{{Corpus: "sheet"}},
			Outputs: []Output{
				{Pattern: "stdout.txt", Description: "What it prints.", Inputs: InputsFunc},
				{Pattern: "error.txt", Description: "Why it fails.", Inputs: InputsPositions},
			},
			Rejects: []string{"circular"},
		}, {
			Name: "evaluate", Description: "Evaluates.", Consumes: "*example.com/m.Graph", Corpora: []string{"sheet", "csv"},
			Paths: []Path{{Corpus: "sheet", Lifts: []string{"resolve", "parse"}}, {Corpus: "csv", Lifts: []string{"resolve", "parse", "import"}}},
			Outputs: []Output{
				{Pattern: "values.txt", Description: "The values.", Inputs: InputsNone},
				{Pattern: "e*", Description: "Its e files.", Inputs: InputsAll},
			},
		}},
		Lifts: []Lift{
			{Name: "parse", Corpora: []string{"sheet"}, Paths: []Path{{Corpus: "sheet"}},
				Outputs: []Output{{Pattern: "error.txt", Description: "Why it fails.", Inputs: InputsAll}}},
			{Name: "resolve", Corpora: []string{"sheet"}, Paths: []Path{{Corpus: "sheet", Lifts: []string{"parse"}}}},
			// A product file's pattern may match error.txt: no product file
			// is a cell's error.txt.
			{Name: "import", Corpora: []string{"csv"}, Paths: []Path{{Corpus: "csv"}}, Bytes: true,
				ProductFiles: []Output{{Pattern: "*", Description: "A file.", Inputs: InputsFunc}}},
		},
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
	wantPaths := []Path{{Corpus: "csv", Lifts: []string{"resolve", "parse", "import"}}, {Corpus: "sheet", Lifts: []string{"resolve", "parse"}}}
	if ps := got.Suites[1].Paths; !reflect.DeepEqual(ps, wantPaths) {
		t.Errorf("evaluate's paths are written as %+v, want %+v", ps, wantPaths)
	}
	if ps := got.Lifts[1].Paths; len(ps) != 1 || ps[0].Lifts == nil || !bytes.Contains(now.Marshal(), []byte(`"corpus": "sheet",
          "lifts": []`)) {
		t.Errorf("parse's path is written as %+v, and the registry as\n%s", ps, now.Marshal())
	}

	// A registry the update couldn't have written is refused, as written
	// by hand, which Marshal, which sorts, doesn't do.
	for _, tt := range []struct {
		edit func(r *Registry)
		want string
	}{
		{func(r *Registry) { r.Suites[1].Paths = r.Suites[1].Paths[1:] }, "the suite evaluate runs over testdata/csv/ and testdata/sheet/, and has paths from testdata/sheet/, where a registry gives each column one path from each corpus it runs over, in the same order"},
		{func(r *Registry) { slices.Reverse(r.Suites[1].Paths) }, "the suite evaluate runs over testdata/csv/ and testdata/sheet/, and has paths from testdata/sheet/ and testdata/csv/,"},
		{func(r *Registry) { r.Suites[1].Paths = append(r.Suites[1].Paths, r.Suites[1].Paths[1]) }, "has paths from testdata/csv/, testdata/sheet/ and testdata/sheet/,"},
		{func(r *Registry) { r.Suites[0].Paths = nil }, "the suite calc runs over testdata/sheet/, and has paths from none,"},
		{func(r *Registry) { r.Lifts[2].Paths = nil }, "the lift resolve runs over testdata/sheet/, and has paths from none,"},
		{func(r *Registry) { r.Suites[1].Paths[0].Lifts[2] = "nosuch" }, "the suite evaluate's path from testdata/csv/ names nosuch, and the registry lists no lift of that name, where every lift on a column's path is one its package registers"},
		{func(r *Registry) { r.Lifts[2].Paths[0].Lifts = []string{"resolve"} }, "the lift resolve's path from testdata/sheet/ names the lift itself, where a column's path names the lifts before it"},
		{func(r *Registry) {
			r.Lifts[2].Corpora = []string{"sheet", "sheet"}
			r.Lifts[2].Paths = append(r.Lifts[2].Paths, r.Lifts[2].Paths[0])
		}, "the lift resolve runs over testdata/sheet/ twice, where a registry lists each corpus a column runs over once"},
		{func(r *Registry) { r.Suites[0].Paths[0].Lifts = []string{"parse"} }, "the suite calc consumes the raw input view, and its path from testdata/sheet/ names parse, where a suite over the raw input view passes through no lift"},
		{func(r *Registry) { r.Suites[1].Paths[1].Lifts = []string{"resolve", "parse", "parse"} }, "the suite evaluate's path from testdata/sheet/ names parse twice, where a column's path names each lift before it once"},
		{func(r *Registry) { r.Lifts = append(r.Lifts, r.Lifts[2]) }, "the registry lists the lift resolve twice, where each suite and lift a package registers, owned or used, has a name of its own"},
		{func(r *Registry) { r.Suites = append(r.Suites, r.Suites[0]) }, "the registry lists the suite calc twice, where each suite and lift a package registers, owned or used, has a name of its own"},
		{func(r *Registry) { r.Suites[0].Name = "parse" }, "the registry lists a suite and a lift named parse, where each suite and lift a package registers, owned or used, has a name of its own"},
		{func(r *Registry) { r.Suites[0].Outputs[1].Inputs = "" }, `the suite calc declares the output stdout.txt with inputs "", where a registry records "func", "positions", "all" or "none"`},
		{func(r *Registry) { r.Lifts[0].ProductFiles[0].Inputs = "lines" }, `the lift import declares the product file * with inputs "lines", where a registry records "func", "positions", "all" or "none"`},
		{func(r *Registry) { r.Suites[1].Outputs[1].Inputs = InputsPositions }, `the suite evaluate declares the output values.txt with inputs "positions", where only an output whose pattern is error.txt takes its ranges from Positions`},
		{func(r *Registry) { r.Lifts[0].ProductFiles[0] = Output{Pattern: "error.txt", Inputs: InputsPositions} }, `the lift import declares the product file error.txt with inputs "positions", where only an output whose pattern is error.txt takes its ranges from Positions`},
		{func(r *Registry) { r.Suites[1].Outputs[0].Inputs = InputsFunc }, `the suite evaluate declares the output e* with inputs "func", and the pattern matches error.txt, where error.txt's ranges come from Positions or the whole input view, never from a function`},
		{func(r *Registry) { r.Lifts[1].Outputs[0].Inputs = InputsFunc }, `the lift parse declares the output error.txt with inputs "func", and the pattern matches error.txt,`},
	} {
		r, err := Parse(now.Marshal())
		if err != nil {
			t.Fatal(err)
		}
		tt.edit(r)
		data, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Parse(data); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("a registry edited by hand: got %v, want %q", err, tt.want)
		}
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
	// A format 6 registry, whose lift says whether it is a bytes lift and
	// declares its product's files, and whose columns give no paths.
	v6 := strings.Replace(v5, `"format": 5`, `"format": 6`, 1)
	v6 = strings.Replace(v6, `"owned": true, "corpora"`, `"owned": true, "bytes": false, "corpora"`, 1)
	v6 = strings.Replace(v6, `"rejects": []}]`, `"rejects": [], "productFiles": []}]`, 1)
	// A format 7 registry, whose columns give their paths, and whose output
	// declarations say nothing of Inputs, nor the registry of Positions.
	v7 := strings.Replace(v6, `"format": 6`, `"format": 7`, 1)
	v7 = strings.Replace(v7, `"corpora": ["sheet"], "outputs": []`, `"corpora": ["sheet"], "paths": [{"corpus": "sheet", "lifts": []}], "outputs": [{"pattern": "error.txt", "description": "Why it fails."}]`, 1)
	v7 = strings.Replace(v7, `"productFiles": []`, `"productFiles": [{"pattern": "sheet/*", "description": "A cell."}]`, 1)
	v7 = strings.Replace(v7, `"consumes": "io/fs.FS", "corpora": ["sheet"],`, `"consumes": "io/fs.FS", "corpora": ["sheet"], "paths": [{"corpus": "sheet", "lifts": []}],`, 1)
	for _, tt := range []struct {
		data              string
		format            int
		transforms, lifts int
	}{
		{v3, 3, 0, 0},
		{v4, 4, 1, 0},
		{v5, 5, 1, 1},
		{v6, 6, 1, 1},
		{v7, 7, 1, 1},
	} {
		g, err := ParseAny([]byte(tt.data))
		if err != nil {
			t.Errorf("format %d: %v", tt.format, err)
			continue
		}
		var outputs []Output
		for _, l := range g.Lifts {
			outputs = slices.Concat(outputs, l.Outputs, l.ProductFiles)
		}
		if g.Format != tt.format || g.CorpusRoot != "." || !g.RootPackage || len(g.IssueLinks) != 1 ||
			len(g.Properties) != 1 || len(g.Suites) != 1 || g.Suites[0].Outputs[0].Pattern != "stdout.txt" ||
			len(g.Types) != 1 || len(g.Types[0].Transforms) != tt.transforms || g.Codes || g.Suites[0].Classifier != nil ||
			len(g.Lifts) != tt.lifts || slices.ContainsFunc(g.Lifts, func(l Lift) bool { return l.Bytes }) ||
			tt.format < 6 && slices.ContainsFunc(g.Lifts, func(l Lift) bool { return l.ProductFiles != nil }) ||
			tt.format < 7 && (g.Suites[0].Paths != nil || slices.ContainsFunc(g.Lifts, func(l Lift) bool { return l.Paths != nil })) ||
			tt.format == 7 && (len(g.Suites[0].Paths) != 1 || len(outputs) != 2) ||
			g.Positions || slices.ContainsFunc(append(outputs, g.Suites[0].Outputs...), func(o Output) bool { return o.Inputs != "" }) {
			t.Errorf("format %d reads as %+v", tt.format, g)
		}
		if _, err := Parse([]byte(tt.data)); !errors.As(err, new(*FormatError)) {
			t.Errorf("Parse of format %d: got %v, want a FormatError", tt.format, err)
		}
	}
	now := Registry{CorpusRoot: ".", Codes: true, Positions: true, Suites: []Suite{{Name: "evaluate", Classifier: &Classifier{Name: "div-zero", Description: "Divides by zero."},
		Corpora: []string{"sheet"}, Paths: []Path{{Corpus: "sheet", Lifts: []string{"resolve", "parse"}}},
		Outputs: []Output{{Pattern: "error.txt", Description: "Why it fails.", Inputs: InputsPositions}}}},
		Lifts: []Lift{{Name: "import", Bytes: true, ProductFiles: []Output{{Pattern: "sheet/*", Description: "A cell.", Inputs: InputsFunc}}, Corpora: []string{"csv"}, Paths: []Path{{Corpus: "csv"}}}}}
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
		{strings.Replace(v6, `"corpora": ["sheet"], "outputs": []`, `"corpora": ["sheet"], "paths": [], "outputs": []`, 1), "it is not a registry: it holds format 6, and lifts[].paths, which format 7 added"},
		{strings.Replace(v6, `"consumes": "io/fs.FS", "corpora": ["sheet"],`, `"consumes": "io/fs.FS", "corpora": ["sheet"], "paths": [{"corpus": "sheet", "lifts": []}],`, 1), "it is not a registry: it holds format 6, and suites[].paths, which format 7 added"},
		{strings.Replace(v7, `"description": "Why it fails."}`, `"description": "Why it fails.", "inputs": null}`, 1), "it is not a registry: it holds format 7, and lifts[].outputs[].inputs, which format 8 added"},
		{strings.Replace(v7, `"description": "A cell."}`, `"description": "A cell.", "inputs": "func"}`, 1), "it is not a registry: it holds format 7, and lifts[].productFiles[].inputs, which format 8 added"},
		{strings.Replace(v7, `"rootPackage": true,`, `"rootPackage": true, "positions": false,`, 1), "it is not a registry: it holds format 7, and positions, which format 8 added"},
		{strings.Replace(v7, `"description": "What it prints."}`, `"description": "What it prints.", "inputs": "all"}`, 1), "it is not a registry: it holds format 7, and suites[].outputs[].inputs, which format 8 added"},
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

// keysVar writes today's format's keys, as it writes the module's other
// goldens.
const keysVar = "HYPERCASE_UPDATE_GOLDEN"

// frozenKeys holds the SHA-256 sum of testdata/keys-<N>.txt for each format
// N from Oldest to today's. A format that is no longer today's is history:
// some commit of main holds registries of it, and its keys never change.
// Today's keys change only with its sum, by hand: once a commit of main
// holds today's format, a change to its keys is a new format, which only
// the person changing them knows, so no update rewrites the sum.
var frozenKeys = map[int]string{
	3: "5de45696f0737fd8e97d96e72ad6d26e45ab1daac642f49933226e152e590760",
	4: "504994ef22efd44b15d7ac406d4f8d1be76d1c6c4ebc07a8451d89fd9f1c1e95",
	5: "ee647b477b5596ed4f4377538de3840c72a76c139300f735db39d12b15b2f3ed",
	6: "8296bfb2da3b7ab45e4e249079e81a8f51cc23de8fb2a204e4632029c3e662e4",
	7: "10ed6d7e887c1f6afc455f8de8f6cbdc4c1184ee7e34ba900faced74b4fbcd3f",
	8: "814db4c29a8ee83244465ac6dcda61e31eb7609e36400a01c96c9b71e740790e",
}

// TestFormatKeys: testdata/keys-<N>.txt holds the key paths of a registry
// of format N, as encoding/json writes them, each written while its format
// was today's, and frozen by its sum in frozenKeys, today's too. ParseAny's
// added table is what they say each format added. No format drops a key
// an older one holds. A key a format adds, whose parent the format before
// it holds, is in added, at that format. A registry of each format, holding
// every key its format holds, reads, and with any key a later format
// added, null or not, is refused as no registry, by that key and the
// format that added it. HYPERCASE_UPDATE_GOLDEN=1 writes today's format's
// keys.
func TestFormatKeys(t *testing.T) {
	now := []byte(strings.Join(registrytest.KeyPaths(t, reflect.TypeFor[Registry]()), "\n") + "\n")
	golden := func(f int) string { return filepath.Join("testdata", fmt.Sprintf("keys-%d.txt", f)) }
	if os.Getenv(keysVar) == "1" {
		if err := os.WriteFile(golden(Format), now, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	keys := map[int][]string{}
	for f := Oldest; f <= Format; f++ {
		data, err := os.ReadFile(golden(f))
		if err != nil {
			t.Fatalf("%v\nRun %s=1 go test ./... to write format %d's keys.", err, keysVar, Format)
		}
		if f == Format && !bytes.Equal(data, now) {
			t.Errorf("a registry's keys are not those of format %d (-%s +today):\n%s\nA change to them is a new format, unless no commit of main holds format %d: bump Format if one does, give added what the change adds, and run %s=1 go test ./... to write the keys.", Format, golden(f), diff.Lines(data, now), Format, keysVar)
		}
		switch sum := fmt.Sprintf("%x", sha256.Sum256(data)); {
		case frozenKeys[f] == "" && f < Format:
			t.Errorf("format %d is below today's, %d, and frozenKeys has no sum of its keys, %s.\nGive frozenKeys %d: %q, the sum of the keys format %d held while it was today's.", f, Format, golden(f), f, sum, f)
		case frozenKeys[f] == "":
			t.Errorf("frozenKeys has no sum of today's format's keys, %s.\nGive frozenKeys %d: %q, once the keys are those format %d holds.", golden(f), f, sum, f)
		case sum != frozenKeys[f] && f < Format:
			t.Errorf("%s changed: format %d is below today's, %d, so its keys are history, and frozenKeys has the sum %s, where the file's is %s.\nRestore the keys whose sum frozenKeys holds. A change to a registry's keys is a new format.", golden(f), f, Format, frozenKeys[f], sum)
		case sum != frozenKeys[f]:
			t.Errorf("%s changed: frozenKeys has the sum %s of today's format's keys, where the file's is %s.\nIf a commit of main holds format %d, the change is a new format: restore the keys, bump Format, give added what the change adds, and run %s=1 go test ./... to write the new format's keys. If none does, give frozenKeys %d: %q.", golden(f), frozenKeys[f], sum, f, keysVar, f, sum)
		}
		keys[f] = strings.Fields(string(data))
	}
	for _, f := range slices.Sorted(maps.Keys(frozenKeys)) {
		if f > Format || f < Oldest {
			t.Errorf("frozenKeys has a sum for format %d, and freezes only the formats from Oldest, %d, to today's, %d.", f, Oldest, Format)
		}
	}

	type newKey struct {
		format int
		key    string
	}
	var adds []newKey
	for f := Oldest + 1; f <= Format; f++ {
		for _, k := range keys[f-1] {
			if !slices.Contains(keys[f], k) {
				t.Errorf("format %d drops %s, which format %d holds: ParseAny reads format %d in today's form, which refuses it.", f, k, f-1, f-1)
			}
		}
		for _, k := range keys[f] {
			if p := parentKey(k); !slices.Contains(keys[f-1], k) && (p == "" || slices.Contains(keys[f-1], p)) {
				adds = append(adds, newKey{f, k})
			}
		}
	}
	var table []newKey
	for _, a := range added {
		table = append(table, newKey{a.format, a.key})
	}
	if !slices.Equal(table, adds) {
		t.Errorf("added holds %v, and the formats' keys say each format added %v, in format order.\nGive added each key a format adds, whose parent the format before it holds, at that format. A key added at today's format, %d, is a new format if a commit of main holds format %d: then bump Format, give added the key at the new format, and run %s=1 go test ./... to write its keys.", table, adds, Format, Format, keysVar)
	}

	full := registrytest.Full(t, reflect.TypeFor[Registry]())
	for f := Oldest; f <= Format; f++ {
		holds := func(more string) func(string) bool {
			return func(k string) bool {
				return slices.Contains(keys[f], k) || more != "" && (k == more || strings.HasPrefix(k, more+".") || strings.HasPrefix(k, more+"[]."))
			}
		}
		reg := func(more, null string) []byte {
			m := shape(full, "", holds(more), null).(map[string]any)
			m["format"] = f
			data, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			return data
		}
		if g, err := ParseAny(reg("", "")); err != nil || g.Format != f {
			t.Errorf("a format %d registry that holds every key of its format: got %+v, %v", f, g, err)
		}
		for _, a := range adds {
			if p := parentKey(a.key); a.format <= f || p != "" && !slices.Contains(keys[f], p) {
				continue
			}
			want := fmt.Sprintf("it is not a registry: it holds format %d, and %s, which format %d added", f, a.key, a.format)
			for _, null := range []string{"", a.key} {
				if _, err := ParseAny(reg(a.key, null)); err == nil || err.Error() != want {
					t.Errorf("a format %d registry that holds %s (null: %v): got %v, want %q", f, a.key, null != "", err, want)
				}
			}
		}
	}
}

// parentKey returns the key path of the key that holds k, or "" for a key
// of the registry itself.
func parentKey(k string) string {
	i := strings.LastIndex(k, ".")
	if i < 0 {
		return ""
	}
	return strings.TrimSuffix(k[:i], "[]")
}

// shape returns v holding only the keys that holds holds, and null for the
// key null names.
func shape(v any, prefix string, holds func(string) bool, null string) any {
	switch v := v.(type) {
	case map[string]any:
		m := map[string]any{}
		for k, c := range v {
			switch p := prefix + k; {
			case !holds(p):
			case p == null:
				m[k] = nil
			case reflect.TypeOf(c).Kind() == reflect.Slice:
				m[k] = shape(c, p+"[].", holds, null)
			default:
				m[k] = shape(c, p+".", holds, null)
			}
		}
		return m
	case []any:
		var l []any
		for _, c := range v {
			l = append(l, shape(c, prefix, holds, null))
		}
		return l
	}
	return v
}

// TestPeek: a registry's corpus root is read whatever its format, an older
// or a newer one's included, so a reader can leave another root's registry
// unread. Its update variable is read too, and is "" when it holds none.
func TestPeek(t *testing.T) {
	newer := fmt.Sprintf(`{"format": %d, "corpusRoot": "step09", "rootPackage": true, "a field of a later hypercase": {}}`, Format+1)
	for _, tt := range []struct{ data, root, updateVar string }{
		{olderData, "step05", "X"},
		{string(Registry{CorpusRoot: ".", UpdateVar: "Y"}.Marshal()), ".", "Y"},
		{newer, "step09", ""},
	} {
		h, err := Peek([]byte(tt.data))
		if err != nil || h.CorpusRoot != tt.root || h.UpdateVar != tt.updateVar {
			t.Errorf("Peek(%.40s…) = %+v, %v; want the corpus root %s and the update variable %q", tt.data, h, err, tt.root, tt.updateVar)
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
