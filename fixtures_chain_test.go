package hypercase_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/jenny"
)

// Step 10's declarations: bytes lifts, whose product is a corpus type's
// input view, committed under via/<lift>/. Export is the corpus type of
// testdata/export/, and the bytes lift import makes a Script of each
// export, which step 6's ladder runs on:
//
//	Export → import → Script → parse → *Parsed → resolve → *Graph
//
// The suites list, over Script, and evaluate, over *Graph, run on the
// exports through import. import follows the export's import/ tree:
//
//	import/out/<path>     made as the product's <path>
//	import/nonl/<path>    made as <path>, less its final newline
//	import/marker/<path>  made as <path>, with a line shaped like a
//	                      section marker
//	import/nest/<path>    made as <path>, and as <path>/below, a file
//	                      below it
//	import/space/<path>   made as <path>, with a space after its name
//	import/newline/<path> made as <path>, with a newline and x after its
//	                      name
//	import/dup/<path>     made as <path>, twice
//	import/case/<path>    made as <path>, and as <path> in upper case
//	import/render/<path>  made as <path>, a tree that fails to render
//	import/error          returned as the error, less its final newline,
//	                      which decl.Reject marks as the rejection
//	import/fail           returned as the error, less its final newline,
//	                      unmarked
//	import/coded          a *codedError, with the code it holds, less its
//	                      final newline, which decl.Reject marks
//	import/panic          panicked with, less its final newline
//
// An import/ tree with more than one of error, fail and coded returns
// their errors joined, with errors.Join, in that order. An export whose
// import/ tree has none of these is imported as an empty product.
type (
	Export   struct{ fs.FS }
	Unpacked struct{ view fs.FS }
)

var unterminated = decl.Property{Name: "unterminated", Description: "The export holds a quoted field it never closes."}

// A codedError is an error with a code, which projectCodesAs finds by
// errors.As, through decl.Reject and the jenny's wrapping.
type codedError struct{ code string }

func (e *codedError) Error() string { return "an error with the code " + e.code }

var projectCodesAs = decl.Project{
	UpdateVar: fixtureUpdate,
	Codes: func(err error) []string {
		var c *codedError
		if errors.As(err, &c) {
			return []string{c.code}
		}
		return nil
	},
}

// steerImport is a bytes lift's jenny, which the export's <name>/ tree
// steers.
func steerImport(name string, in fs.FS) (jenny.Files, error) {
	logRun("%s ran", name)
	if data, err := fs.ReadFile(in, name+"/panic"); err == nil {
		panic(strings.TrimSuffix(string(data), "\n"))
	}
	var errs []error
	if data, err := fs.ReadFile(in, name+"/error"); err == nil {
		errs = append(errs, decl.Reject(errors.New(strings.TrimSuffix(string(data), "\n"))))
	}
	if data, err := fs.ReadFile(in, name+"/fail"); err == nil {
		errs = append(errs, errors.New(strings.TrimSuffix(string(data), "\n")))
	}
	if data, err := fs.ReadFile(in, name+"/coded"); err == nil {
		errs = append(errs, decl.Reject(&codedError{code: strings.TrimSuffix(string(data), "\n")}))
	}
	switch len(errs) {
	case 0:
	case 1:
		return nil, errs[0]
	default:
		return nil, errors.Join(errs...)
	}
	paths, err := archive.Paths(in)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	// faulty are files jenny refuses: two at one path, or a tree that
	// fails to render.
	var faulty jenny.Files
	for _, p := range paths {
		data, err := fs.ReadFile(in, p)
		if err != nil {
			return nil, err
		}
		if rest, ok := strings.CutPrefix(p, name+"/dup/"); ok {
			faulty = append(faulty, jenny.NewFile(rest, data), jenny.NewFile(rest, data))
		}
		if rest, ok := strings.CutPrefix(p, name+"/case/"); ok {
			faulty = append(faulty, jenny.NewFile(rest, data), jenny.NewFile(strings.ToUpper(rest), data))
		}
		if rest, ok := strings.CutPrefix(p, name+"/render/"); ok {
			faulty = append(faulty, jenny.NewTree(rest, data, func(d []byte) ([]byte, error) {
				return nil, errors.New("the tree won't render")
			}))
		}
		if rest, ok := strings.CutPrefix(p, name+"/out/"); ok {
			files[rest] = data
		}
		if rest, ok := strings.CutPrefix(p, name+"/nest/"); ok {
			files[rest] = data
			files[rest+"/below"] = data
		}
		if rest, ok := strings.CutPrefix(p, name+"/nonl/"); ok {
			files[rest] = bytes.TrimSuffix(data, []byte("\n"))
		}
		if rest, ok := strings.CutPrefix(p, name+"/marker/"); ok {
			files[rest] = append(data, "-- x --\n"...)
		}
		if rest, ok := strings.CutPrefix(p, name+"/space/"); ok {
			files[rest+" "] = data
		}
		if rest, ok := strings.CutPrefix(p, name+"/newline/"); ok {
			files[rest+"\nx"] = data
		}
	}
	var out jenny.Files
	for _, p := range slices.Sorted(maps.Keys(files)) {
		out = append(out, jenny.NewFile(p, files[p]))
	}
	return append(out, faulty...), nil
}

var (
	importOutputs = []decl.Output{{Pattern: "error.txt", Description: "Why the importer rejects the export."}}
	importProduct = []decl.Output{
		{Pattern: "parse/*", Description: "What parse does, as a script's parse/ tree says."},
		{Pattern: "resolve/*", Description: "What resolve does, as a script's resolve/ tree says."},
		{Pattern: "evaluate/*", Description: "What evaluate does, as a script's evaluate/ tree says."},
		{Pattern: "evaluate/out/*", Description: "An output evaluate returns."},
		{Pattern: "quote/*", Description: "What a renderer of errors quotes."},
		{Pattern: "names/*", Description: "Files named as a cell's own files are."},
		{Pattern: "inputs/*", Description: "A tree named as a cell's inputs file is."},
	}

	importLift = decl.Lift[Export, Script]{
		Name:        "import",
		Description: "Imports the export as a script, as its import/ tree says.",
		Outputs:     importOutputs,
		Product:     importProduct,
		Rejects:     []decl.Property{unterminated},
		Jenny: jenny.For[Export]().New("import", func(in Export) (jenny.Files, error) {
			return steerImport("import", in)
		}),
	}
	// unpack and repack are a ladder of two lifts to Script, the second a
	// bytes lift: unpack follows the export's unpack/ tree as step 6's
	// lifts do, and repack its repack/ tree as import does its import/.
	unpack = decl.Lift[Export, *Unpacked]{
		Name:        "unpack",
		Description: "Unpacks the export, as its unpack/ tree says.",
		Outputs:     liftOutputs,
		Rejects:     []decl.Property{unterminated},
		Run: func(archiveName string, in Export, stderr io.Writer) (*Unpacked, error) {
			if err := steerLift("unpack", archiveName, in, stderr); err != nil {
				return nil, err
			}
			return &Unpacked{view: in.FS}, nil
		},
	}
	repack = decl.Lift[*Unpacked, Script]{
		Name:        "repack",
		Description: "Repacks the unpacked export as a script, as its repack/ tree says.",
		Outputs:     importOutputs,
		Product:     importProduct,
		Jenny: jenny.For[*Unpacked]().New("repack", func(in *Unpacked) (jenny.Files, error) {
			return steerImport("repack", in.view)
		}),
	}
	// importParsed is a second path from Export to *Parsed, beside import
	// then parse.
	importParsed = decl.Lift[Export, *Parsed]{
		Name:        "import-parse",
		Description: "Parses the export without importing it.",
		Outputs:     liftOutputs,
		Run: func(_ string, in Export, _ io.Writer) (*Parsed, error) {
			return &Parsed{view: in.FS, trail: []string{"import-parse"}}, nil
		},
	}
	// exportGrid is a bytes lift to Export, which import lifts on, so a
	// grid reaches Script through two bytes lifts: export follows the
	// grid's export/ tree as import does its import/.
	exportGrid = decl.Lift[Grid, Export]{
		Name:        "export",
		Description: "Exports the grid, as its export/ tree says.",
		Outputs:     []decl.Output{{Pattern: "error.txt", Description: "Why the exporter rejects the grid."}},
		Product:     []decl.Output{{Pattern: "import/out/quote/*", Description: "What import makes as its product's quote/ tree."}},
		Rejects:     []decl.Property{unterminated},
		Jenny: jenny.For[Grid]().New("export", func(in Grid) (jenny.Files, error) {
			return steerImport("export", in)
		}),
	}

	// list lists its input view, which, past import, is import's product.
	list = hypercase.Suite[Script]{
		Name:        "list",
		Description: "Lists the files of the script's input view, with their bytes.",
		Outputs:     []decl.Output{{Pattern: "files.txt", Description: "How many files the view holds, then each one's path and bytes, quoted."}},
		Run: func(t testing.TB, _ string, in Script, _, _ io.Writer) ([]hypercase.File, error) {
			paths, err := archive.Paths(in)
			if err != nil {
				return nil, err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "%d files\n", len(paths))
			for _, p := range paths {
				data, err := fs.ReadFile(in, p)
				if err != nil {
					return nil, err
				}
				fmt.Fprintf(&b, "%s %q\n", p, data)
			}
			return []hypercase.File{{Path: "files.txt", Data: []byte(b.String())}}, nil
		},
	}

	// louder uppercases what evaluate returns, and keeps unterminated.
	louder = decl.Transform[Export]{
		Name:        "louder",
		Description: "Uppercases every file under import/out/evaluate/out/.",
		Keeps:       []decl.Property{unterminated},
		Probe: func(in Export) (string, bool) {
			if m, _ := fs.Glob(in, "import/out/evaluate/out/*"); len(m) == 0 {
				return "the export makes no output of evaluate", false
			}
			return "", true
		},
		Jenny: jenny.For[Export]().New("louder", func(in Export) (jenny.Files, error) {
			paths, err := archive.Paths(in)
			if err != nil {
				return nil, err
			}
			var out jenny.Files
			for _, p := range paths {
				data, err := fs.ReadFile(in, p)
				if err != nil {
					return nil, err
				}
				if strings.HasPrefix(p, "import/out/evaluate/out/") {
					data = bytes.ToUpper(data)
				}
				out = append(out, jenny.NewFile(p, data))
			}
			return out, nil
		}),
	}

	exportType      = decl.Type[Export]{Dir: "export", Description: "Exports, whose import/ tree says what the importer makes of them.", Transforms: []decl.Transform[Export]{louder}}
	plainExportType = decl.Type[Export]{Dir: "export", Description: exportType.Description}

	chainProps = []any{malformed, circular, dangling, unterminated}

	// exportCheck finds each committed file of import's product, with its
	// bytes, as a corpus check reads it.
	exportCheck = hypercase.Check{
		Name:        "products",
		Description: "Lists each file of import's committed product.",
		Run: func(cells []hypercase.Cell) ([]string, error) {
			var findings []string
			for _, c := range cells {
				if c.Product == nil {
					continue
				}
				name := c.Type + "/" + c.Archive
				if c.Case != "" {
					name += "/" + c.Case
				}
				for _, p := range slices.Sorted(maps.Keys(c.Product)) {
					findings = append(findings, fmt.Sprintf("%s %s %s %q", name, c.Column, p, c.Product[p]))
				}
			}
			return findings, nil
		},
	}
)

func chain(p decl.Project, items ...any) {
	hypercase.Register(append([]any{p, rootPkg, scriptType}, append(chainProps, items...)...)...)
}

// chainB registers ./b of the chain-shared fixture, with the Project p.
func chainB(p decl.Project) {
	hypercase.Register(append([]any{p, scriptType, exportType, hypercase.Use(importLift, parse, resolve), own(evaluate, list, louder)}, chainProps...)...)
}

// chainRegistrations are step 10's declaration sets, by name.
var chainRegistrations = map[string]func(){
	"chain": func() { chain(project, exportType, own(importLift, parse, resolve, evaluate, list, louder)) },
	"chain-norejects": func() {
		l := importLift
		l.Rejects = nil
		chain(project, exportType, own(l, parse, resolve, evaluate, list, louder))
	},
	"chain-check": func() {
		chain(project, exportType, own(importLift, parse, resolve, evaluate, list, louder, exportCheck))
	},
	"chain-paths": func() {
		chain(project, exportType, own(importLift, parse, resolve, evaluate, list, louder, pathsCheck))
	},
	"chain-twice-paths": func() { chain(project, gridType, plainExportType, own(exportGrid, importLift, list, pathsCheck)) },
	"chain-codes":       func() { chain(projectCodesAs, exportType, own(importLift, parse, resolve, evaluate, list, louder)) },
	"chain-render":      func() { chain(projectRender, plainExportType, own(importLift, parse, resolve, evaluate)) },
	"chain-past":        func() { chain(project, plainExportType, own(unpack, repack, parse, resolve, evaluate, list)) },
	"chain-two-paths":   func() { chain(project, plainExportType, own(importLift, parse, importParsed, format)) },
	"chain-twice":       func() { chain(project, gridType, plainExportType, own(exportGrid, importLift, list)) },

	// Packages over one export corpus: . owns import, parse and resolve,
	// and ./b uses them, and owns evaluate, list and the transform louder.
	// In the -render pairs, one package's Project is projectRender.
	"chain-a":        func() { chain(project, exportType, own(importLift, parse, resolve)) },
	"chain-a-paths":  func() { chain(project, exportType, own(importLift, parse, resolve, pathsCheck)) },
	"chain-b":        func() { chainB(project) },
	"chain-a-render": func() { chain(projectRender, exportType, own(importLift, parse, resolve)) },
	// used-fewer-a and -b reach parse and resolve otherwise: . owns them
	// over script alone, and ./b, which uses them, reaches them from
	// export too, through import, which it owns.
	"used-fewer-a": func() { chain(project, own(parse, resolve, pathsCheck)) },
	// used-fewer-a-fixed reaches parse and resolve from export as ./b
	// does, by using import.
	"used-fewer-a-fixed": func() {
		chain(project, plainExportType, own(parse, resolve, pathsCheck), hypercase.Use(importLift))
	},
	"used-fewer-b": func() {
		hypercase.Register(append([]any{project, scriptType, plainExportType, own(importLift, evaluate), hypercase.Use(parse, resolve)}, chainProps...)...)
	},
	// used-unowned-b is used-fewer-b using import, which no package owns,
	// and used-unowned-a-fixed reaches parse and resolve from export as
	// ./b does, by owning import.
	"used-unowned-b": func() {
		hypercase.Register(append([]any{project, scriptType, plainExportType, own(evaluate), hypercase.Use(importLift, parse, resolve)}, chainProps...)...)
	},
	"used-unowned-a-fixed": func() { chain(project, plainExportType, own(importLift, parse, resolve, pathsCheck)) },
	"chain-b-render":       func() { chainB(projectRender) },
	// chain-b-nooutputs is chain-b with a copy of import that declares no
	// outputs.
	"chain-b-nooutputs": func() {
		l := importLift
		l.Outputs = nil
		hypercase.Register(append([]any{project, scriptType, exportType, hypercase.Use(l, parse, resolve), own(evaluate, list, louder)}, chainProps...)...)
	},

	// Packages over one grid corpus: . owns export, and ./b uses it, and
	// owns import and list.
	"chain-twice-a": func() { chain(project, gridType, plainExportType, own(exportGrid)) },
	"chain-twice-b": func() {
		hypercase.Register(append([]any{project, scriptType, gridType, plainExportType, hypercase.Use(exportGrid), own(importLift, list)}, chainProps...)...)
	},

	// Refusals.
	"bytes-run": func() {
		hypercase.Register(project, scriptType, plainExportType, own(decl.Lift[Export, Script]{
			Name: "import", Description: "Imports the export.", Product: importProduct,
			Run: func(_ string, in Export, _ io.Writer) (Script, error) { return Script{in.FS}, nil },
		}))
	},
	"bytes-both": func() {
		l := importLift
		l.Run = func(_ string, in Export, _ io.Writer) (Script, error) { return Script{in.FS}, nil }
		hypercase.Register(project, scriptType, plainExportType, unterminated, own(l))
	},
	"bytes-no-jenny": func() {
		l := importLift
		l.Jenny = jenny.Jenny[Export]{}
		hypercase.Register(project, scriptType, plainExportType, unterminated, own(l))
	},
	"bytes-no-product": func() {
		l := importLift
		l.Product = nil
		hypercase.Register(project, scriptType, plainExportType, unterminated, own(l))
	},
	"bytes-stderr": func() {
		l := importLift
		l.Outputs = liftOutputs
		hypercase.Register(project, scriptType, plainExportType, unterminated, own(l))
	},
	"bytes-product-flat": func() {
		l := importLift
		l.Product = []decl.Output{{Pattern: "*", Description: "A file."}}
		hypercase.Register(project, scriptType, plainExportType, unterminated, own(l))
	},
	"bytes-product-params": func() {
		l := importLift
		l.Product = []decl.Output{{Pattern: "params/*", Description: "A parameter."}}
		hypercase.Register(project, scriptType, plainExportType, unterminated, own(l))
	},
	"bytes-product-pattern": func() {
		l := importLift
		l.Product = []decl.Output{{Pattern: "parse/[", Description: "A broken pattern."}}
		hypercase.Register(project, scriptType, plainExportType, unterminated, own(l))
	},
	"stage-product": func() {
		l := parse
		l.Product = importProduct
		hypercase.Register(project, scriptType, malformed, own(l))
	},
	// bytes-unregistered registers no corpus type of import's product.
	"bytes-unregistered": func() {
		hypercase.Register(project, rootPkg, plainExportType, unterminated, own(importLift))
	},
	"bytes-product-nodesc": func() {
		l := importLift
		l.Product = []decl.Output{{Pattern: "parse/*", Description: " "}}
		hypercase.Register(project, rootPkg, scriptType, plainExportType, unterminated, own(l))
	},
}
