package hypercase_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"slices"
	"sync"

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

// A viewRecord is what recordingViews records of one call of Positions:
// the error's text, and digests of the views that RenderError and
// Positions received with it, "" where RenderError received none.
type viewRecord struct {
	Error       string `json:"error"`
	RenderError string `json:"renderError"`
	Positions   string `json:"positions"`
}

// recordingViews is p, which supplies Positions, with a RenderError and a
// Positions that record the view each receives: a child run with hooksEnv
// set to a file's path gives it the fixtures' project. RenderError keeps a
// digest of its view by the error it renders, and renders as p's does, or,
// where p has none, as the framework does. Positions appends a viewRecord
// to the file at path, with the digest RenderError kept for the same
// error, and finds what p's finds.
func recordingViews(p decl.Project, path string) decl.Project {
	var mu sync.Mutex
	rendered := map[error]string{}
	render, positions := p.RenderError, p.Positions
	p.RenderError = func(in fs.FS, err error) []byte {
		d := digestOf(in)
		mu.Lock()
		rendered[err] = d
		mu.Unlock()
		if render == nil {
			return []byte(err.Error() + "\n")
		}
		return render(in, err)
	}
	p.Positions = func(in fs.FS, err error) []decl.Range {
		rec := viewRecord{Error: err.Error(), Positions: digestOf(in)}
		mu.Lock()
		rec.RenderError = rendered[err]
		data, e := json.Marshal(rec)
		if e == nil {
			e = appendLine(path, data)
		}
		mu.Unlock()
		if e != nil {
			panic(e)
		}
		return positions(in, err)
	}
	return p
}

// appendLine appends data and a newline to the file at path, in one write.
func appendLine(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// digestOf is the viewDigest of a view's files.
func digestOf(view fs.FS) string {
	files, err := viewFiles(view)
	if err != nil {
		panic(err)
	}
	return viewDigest(files)
}

// viewDigest is a digest of a view's files, by path, as an Observed
// records them.
func viewDigest(files map[string]string) string {
	h := sha256.New()
	for _, p := range slices.Sorted(maps.Keys(files)) {
		fmt.Fprintf(h, "%d %s %d\n%s", len(p), p, len(files[p]), files[p])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// sharedStderr is one declaration of stderr.txt, which the lift
// parseShared and the suite chatty each declare: the file comes from
// parse/stderr, whole, in the view each one's cell runs on.
var sharedStderr = decl.Output{
	Pattern:     "stderr.txt",
	Description: "What parse/stderr says to write to stderr.",
	Inputs: decl.InputsFunc(func(fs.FS, string) ([]decl.Range, error) {
		return []decl.Range{{Path: "parse/stderr"}}, nil
	}),
}

var (
	// parseShared is parse, declaring sharedStderr.
	parseShared = func() decl.Lift[Script, *Parsed] {
		l := parse
		l.Outputs = []decl.Output{liftOutputs[0], sharedStderr}
		return l
	}()
	// chatty follows the case's parse/ tree, as parse does, and declares
	// sharedStderr alone.
	chatty = withOutputs(steered[Script]("chatty", "parse"), sharedStderr)
)

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
	// One declaration, sharedStderr, of a lift and of a suite.
	"inputs-shared": func() { ladder(project, parseShared, chatty) },

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
