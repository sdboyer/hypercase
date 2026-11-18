package hypercase_test

import (
	"io/fs"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/decl"
)

// Step 11's declaration sets, by name.

// mapsNothing is an InputsFunc that maps each file to no range.
var mapsNothing = decl.InputsFunc(func(fs.FS, string) ([]decl.Range, error) { return nil, nil })

// inputsRegistrations are step 11's refusals at registration: what an
// output declaration's Inputs states that no declaration may.
var inputsRegistrations = map[string]func(){
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
