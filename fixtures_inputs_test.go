package hypercase_test

import (
	"io/fs"
	"slices"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/decl"
)

// Step 11's declaration sets, by name.

// mapsNothing is an InputsFunc that maps each file to no range.
var mapsNothing = decl.InputsFunc(func(fs.FS, string) ([]decl.Range, error) { return nil, nil })

// stating returns the declarations with each one's Inputs set to in.
func stating(in decl.Inputs, outputs []decl.Output) []decl.Output {
	out := slices.Clone(outputs)
	for i := range out {
		out[i].Inputs = in
	}
	return out
}

// The declarations that state nothing: parseNone, parse's copy whose
// error.txt and stderr.txt state nothing, and importNone, import's copy
// whose product files state nothing.
var (
	parseNone = func() decl.Lift[Script, *Parsed] {
		l := parse
		l.Outputs = stating(nil, l.Outputs)
		return l
	}()
	importNone = func() decl.Lift[Export, Script] {
		l := importLift
		l.Product = stating(nil, l.Product)
		return l
	}()
)

// projectNoPositions is project with no Positions.
var projectNoPositions = decl.Project{UpdateVar: fixtureUpdate}

// inputsRegistrations are step 11's declaration sets, by name.
var inputsRegistrations = map[string]func(){
	// Declarations that state nothing, or FromPositions in a project with
	// no Positions. echo states nothing, and declares e*, which matches
	// error.txt.
	"inputs-none-suite": func() {
		hypercase.Register(project, rootPkg, scriptType, own(withOutputs(echo, append(stating(nil, echo.Outputs),
			decl.Output{Pattern: "e*", Description: "A file whose name starts with e."})...)))
	},
	// . owns parseNone, and ./b uses it.
	"inputs-none-lift-a": func() { ladder(project, parseNone, resolve, evaluate) },
	"inputs-none-lift-b": func() { hypercase.Register(project, scriptType, hypercase.Use(parseNone), own(outline)) },
	// . owns importNone, and ./b uses it.
	"inputs-none-product-a": func() { chain(project, exportType, own(importNone, parse, resolve)) },
	"inputs-none-product-b": func() {
		hypercase.Register(append([]any{project, scriptType, exportType, hypercase.Use(importNone, parse, resolve), own(evaluate, list, louder)}, chainProps...)...)
	},
	"inputs-no-positions": func() { ladder(projectNoPositions, parse, resolve, evaluate, format, calc) },

	// Refusals at registration: what an output declaration's Inputs
	// states that no declaration may.
	"inputs-positions-values": func() {
		hypercase.Register(project, scriptType, own(withOutputs(echo, decl.Output{Pattern: "values.txt", Description: "The values.", Inputs: decl.FromPositions})))
	},
	"inputs-func-error": func() {
		hypercase.Register(project, scriptType, own(withOutputs(echo, decl.Output{Pattern: "error.txt", Description: "The error.", Inputs: mapsNothing})))
	},
	"inputs-func-star": func() {
		hypercase.Register(project, scriptType, own(withOutputs(echo, decl.Output{Pattern: "*.txt", Description: "Every file.", Inputs: mapsNothing})))
	},
	"inputs-other": func() {
		hypercase.Register(project, scriptType, own(withOutputs(echo, decl.Output{Pattern: "values.txt", Description: "The values.", Inputs: decl.AllInputs + 2})))
	},
	"inputs-lift-positions": func() {
		l := parse
		l.Outputs = []decl.Output{{Pattern: "stderr.txt", Description: "What it writes.", Inputs: decl.FromPositions}}
		hypercase.Register(project, scriptType, own(l))
	},
	"inputs-lift-func": func() {
		l := parse
		l.Outputs = []decl.Output{{Pattern: "*.txt", Description: "Its files.", Inputs: mapsNothing}}
		hypercase.Register(project, scriptType, own(l))
	},
	"inputs-product-positions": func() {
		l := importLift
		l.Product = []decl.Output{{Pattern: "parse/*", Description: "What parse does.", Inputs: decl.FromPositions}}
		hypercase.Register(project, scriptType, plainExportType, unterminated, own(l))
	},
	"inputs-product-other": func() {
		l := importLift
		l.Product = []decl.Output{{Pattern: "parse/*", Description: "What parse does.", Inputs: decl.FromPositions + 1}}
		hypercase.Register(project, scriptType, plainExportType, unterminated, hypercase.Use(l))
	},
}
