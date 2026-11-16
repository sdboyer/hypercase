package hypercase_test

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/jenny"
)

// Step 6's fixtures: a ladder of lifts over the corpus type Script,
//
//	Script → parse → *Parsed → resolve → *Graph
//
// and suites over each rung: calc over Script, format over *Parsed, and
// evaluate over *Graph. Each lift follows the instructions under the input
// view's <lift>/:
//
//	<lift>/error    returned as the error, less its final newline
//	<lift>/panic    panicked with, less its final newline
//	<lift>/stderr   written to stderr
//	<lift>/archive  the archive's name, written to stderr

type (
	// Parsed and Graph are the stages: the case's view, and the trail of
	// the lifts that made them and of the columns that changed them.
	Parsed struct {
		view  fs.FS
		trail []string
	}
	Graph struct {
		view  fs.FS
		trail []string
	}
	// Grid is a second corpus type, of testdata/grid/, with its own lift to
	// *Parsed.
	Grid struct{ fs.FS }
)

var gridType = decl.Type[Grid]{Dir: "grid", Description: "Trees of instructions, in a grid."}

// runLog records what the fixtures' functions ran, in order: lifts, and
// suites' probes and runs. The child prints it after hypercase.Test.
var runLog []string

func logRun(format string, args ...any) { runLog = append(runLog, fmt.Sprintf(format, args...)) }

var liftOutputs = []decl.Output{
	{Pattern: "error.txt", Description: "Why the lift rejects the case."},
	{Pattern: "stderr.txt", Description: "What the lift writes to stderr."},
}

func steerLift(name, archiveName string, in fs.FS, stderr io.Writer) error {
	logRun("%s ran on %s", name, archiveName)
	if data, err := fs.ReadFile(in, name+"/stderr"); err == nil {
		stderr.Write(data)
	}
	if _, err := fs.ReadFile(in, name+"/archive"); err == nil {
		fmt.Fprintf(stderr, "archive: %s\n", archiveName)
	}
	if data, err := fs.ReadFile(in, name+"/panic"); err == nil {
		panic(strings.TrimSuffix(string(data), "\n"))
	}
	if data, err := fs.ReadFile(in, name+"/error"); err == nil {
		return errors.New(strings.TrimSuffix(string(data), "\n"))
	}
	return nil
}

var (
	parse = decl.Lift[Script, *Parsed]{
		Name:        "parse",
		Description: "Parses the script, as its parse/ tree says.",
		Outputs:     liftOutputs,
		Rejects:     []decl.Property{malformed},
		Run: func(archiveName string, in Script, stderr io.Writer) (*Parsed, error) {
			if err := steerLift("parse", archiveName, in, stderr); err != nil {
				return nil, err
			}
			return &Parsed{view: in.FS, trail: []string{"parse"}}, nil
		},
	}
	resolve = decl.Lift[*Parsed, *Graph]{
		Name:        "resolve",
		Description: "Resolves the parsed script, as its resolve/ tree says.",
		Outputs:     liftOutputs,
		Rejects:     []decl.Property{circular, dangling},
		Run: func(archiveName string, p *Parsed, stderr io.Writer) (*Graph, error) {
			if err := steerLift("resolve", archiveName, p.view, stderr); err != nil {
				return nil, err
			}
			return &Graph{view: p.view, trail: append(slices.Clone(p.trail), "resolve")}, nil
		},
	}
	// direct is a second path from Script to *Graph.
	direct = decl.Lift[Script, *Graph]{
		Name:        "direct",
		Description: "Resolves the script without parsing it.",
		Outputs:     liftOutputs,
		Run: func(archiveName string, in Script, stderr io.Writer) (*Graph, error) {
			return &Graph{view: in.FS, trail: []string{"direct"}}, steerLift("direct", archiveName, in, stderr)
		},
	}
	// back closes a cycle: *Graph back to *Parsed.
	back = decl.Lift[*Graph, *Parsed]{
		Name:        "back",
		Description: "Turns a graph back into a parsed script.",
		Outputs:     liftOutputs,
		Run: func(archiveName string, g *Graph, stderr io.Writer) (*Parsed, error) {
			return &Parsed{view: g.view, trail: append(slices.Clone(g.trail), "back")}, nil
		},
	}
	gridParse = decl.Lift[Grid, *Parsed]{
		Name:        "grid-parse",
		Description: "Parses a grid, as its parse/ tree says.",
		Outputs:     liftOutputs,
		Run: func(archiveName string, in Grid, stderr io.Writer) (*Parsed, error) {
			if err := steerLift("grid-parse", archiveName, in, stderr); err != nil {
				return nil, err
			}
			return &Parsed{view: in.FS, trail: []string{"grid-parse"}}, nil
		},
	}
)

// staged returns a suite over a stage that the case's <tree>/ steers, as
// steered's does, and that logs its probe and its run.
func staged[T any](name, tree string, view func(T) fs.FS) hypercase.Suite[T] {
	return hypercase.Suite[T]{
		Name:        name,
		Description: "Does what the case's " + tree + "/ tree says, from the stage it consumes.",
		Outputs:     steeredOutputs,
		Probe: func(in T) (string, bool) {
			logRun("%s's probe ran", name)
			return steeredProbe(view(in), tree)
		},
		Run: func(t testing.TB, archiveName string, in T, stdout, stderr io.Writer) ([]hypercase.File, error) {
			logRun("%s ran on %s", name, archiveName)
			return runSteered(t, archiveName, tree, view(in), stdout, stderr)
		},
	}
}

func parsedView(p *Parsed) fs.FS { return p.view }
func graphView(g *Graph) fs.FS   { return g.view }

var (
	evaluate = staged("evaluate", "evaluate", graphView)
	format   = withTrail(staged("format", "format", parsedView))
	calc     = withRejects(logged(steered[Script]("calc", "calc")), malformed)
	outline  = staged("outline", "outline", parsedView)
	// mutate appends to the shared *Parsed, which its readers must not do.
	mutate = hypercase.Suite[*Parsed]{
		Name:        "mutate",
		Description: "Appends itself to the parsed script's trail.",
		Run: func(t testing.TB, archiveName string, p *Parsed, stdout, stderr io.Writer) ([]hypercase.File, error) {
			p.trail = append(p.trail, "mutate")
			return nil, nil
		},
	}
)

// withTrail makes a suite over *Parsed write trail.txt too: the trail of
// the value it receives.
func withTrail(s hypercase.Suite[*Parsed]) hypercase.Suite[*Parsed] {
	run := s.Run
	s.Outputs = append(slices.Clone(s.Outputs), decl.Output{Pattern: "trail.txt", Description: "The trail of the parsed script, as the suite received it."})
	s.Run = func(t testing.TB, archiveName string, p *Parsed, stdout, stderr io.Writer) ([]hypercase.File, error) {
		files, err := run(t, archiveName, p, stdout, stderr)
		return append(files, hypercase.File{Path: "trail.txt", Data: []byte(strings.Join(p.trail, " ") + "\n")}), err
	}
	return s
}

func logged[T any](s hypercase.Suite[T]) hypercase.Suite[T] {
	run := s.Run
	s.Run = func(t testing.TB, archiveName string, in T, stdout, stderr io.Writer) ([]hypercase.File, error) {
		logRun("%s ran on %s", s.Name, archiveName)
		return run(t, archiveName, in, stdout, stderr)
	}
	return s
}

// projectRender renders errors: "rendered: " and the error's text, then
// the case's quote/text, quoted. The view's render/ tree steers it:
// render/panic is panicked with, and render/nonl leaves off the newline.
var projectRender = decl.Project{
	UpdateVar: fixtureUpdate,
	RenderError: func(in fs.FS, err error) []byte {
		if data, e := fs.ReadFile(in, "render/panic"); e == nil {
			panic(strings.TrimSuffix(string(data), "\n"))
		}
		out := "rendered: " + err.Error()
		if _, e := fs.ReadFile(in, "render/nonl"); e == nil {
			return []byte(out)
		}
		out += "\n"
		if data, e := fs.ReadFile(in, "quote/text"); e == nil {
			out += "  quote: " + string(data)
		}
		return []byte(out)
	},
}

var ladderProps = []any{malformed, circular, dangling}

func ladder(p decl.Project, columns ...any) {
	hypercase.Register(append([]any{p, rootPkg, scriptType, own(columns...)}, ladderProps...)...)
}

// liftRegistrations are step 6's declaration sets, by name.
var liftRegistrations = map[string]func(){
	"ladder":          func() { ladder(project, parse, resolve, evaluate, format, calc) },
	"ladder-shuffled": func() { ladder(project, format, calc, resolve, evaluate, parse) },
	"ladder-mutate":   func() { ladder(project, parse, resolve, evaluate, format, calc, mutate) },
	"ladder-render":   func() { ladder(projectRender, parse, resolve, evaluate, format, calc) },

	// Packages over one shared ladder: a owns the lifts, b uses parse.
	"ladder-a": func() { ladder(project, parse, resolve, evaluate) },
	"ladder-a-useonly": func() {
		hypercase.Register(project, rootPkg, malformed, circular, dangling, scriptType, hypercase.Use(parse), own(resolve, evaluate))
	},
	"ladder-b":      func() { hypercase.Register(project, scriptType, hypercase.Use(parse), own(outline)) },
	"ladder-b-owns": func() { hypercase.Register(project, malformed, scriptType, own(parse, outline)) },
	"ladder-b-grid": func() {
		hypercase.Register(project, decl.Type[Grid]{Dir: "script", Description: "A grid, in the script directory."}, own(steered[Grid]("outline", "outline")))
	},
	"ladder-b-grid-other": func() {
		hypercase.Register(project, hypercase.Root("other"), rootPkg, decl.Type[Grid]{Dir: "script", Description: "A grid, in the script directory."}, own(steered[Grid]("outline", "outline")))
	},

	// Paths.
	"two-paths": func() { ladder(project, parse, resolve, direct, evaluate) },
	"split": func() {
		ladder(project, parse, resolve, evaluate)
		hypercase.Register(own(direct))
	},
	"split-second": func() { ladder(project, direct, evaluate) },
	"cycle":        func() { ladder(project, parse, resolve, back, evaluate) },
	"unreached":    func() { ladder(project, evaluate) },
	"fs-lift": func() {
		hypercase.Register(project, rootPkg, scriptType, own(decl.Lift[fs.FS, *Parsed]{
			Name: "parse-raw", Description: "Parses the raw view.",
			Run: func(_ string, in fs.FS, _ io.Writer) (*Parsed, error) { return &Parsed{view: in}, nil },
		}))
	},
	"two-types": func() {
		hypercase.Register(project, rootPkg, malformed, scriptType, gridType, own(parse, gridParse, format))
	},

	// Refusals.
	"fs-product": func() {
		hypercase.Register(project, scriptType, own(decl.Lift[Script, Other]{
			Name: "copy", Description: "Copies the script.",
			Run: func(_ string, in Script, _ io.Writer) (Other, error) { return Other{in.FS}, nil },
		}))
	},
	"jenny-lift": func() {
		hypercase.Register(project, scriptType, own(decl.Lift[Script, *Parsed]{
			Name: "parse", Description: "Parses the script.",
			Jenny: jenny.For[Script]().New("parse", func(Script) (jenny.File, error) { return nil, nil }),
		}))
	},
	"lift-no-run": func() {
		hypercase.Register(project, scriptType, own(decl.Lift[Script, *Parsed]{Name: "parse", Description: "Parses nothing."}))
	},
	"lift-bad-output": func() {
		l := parse
		l.Outputs = []decl.Output{{Pattern: "values.txt", Description: "The values."}}
		hypercase.Register(project, scriptType, own(l))
	},
	"lift-bad-name": func() {
		l := parse
		l.Name = "Parse"
		hypercase.Register(project, scriptType, own(l))
	},
	"checks-dir": func() { hypercase.Register(project, decl.Type[Script]{Dir: "checks", Description: "A script."}) },
	"type-twice": func() { hypercase.Register(project, scriptType, scriptType) },
	"type-two-dirs": func() {
		hypercase.Register(project, scriptType, decl.Type[Script]{Dir: "grid", Description: "A script, in a grid."})
	},
	"dir-twice": func() {
		hypercase.Register(project, scriptType, decl.Type[Grid]{Dir: "script", Description: "A grid."})
	},
	"dir-twice-calls": func() {
		hypercase.Register(project, scriptType)
		hypercase.Register(decl.Type[Grid]{Dir: "script", Description: "A grid."})
	},
	"lift-suite-name": func() { hypercase.Register(project, scriptType, own(parse, withName(calc, "parse"))) },
	"lift-suite-name-calls": func() {
		hypercase.Register(project, scriptType, own(withName(calc, "parse")))
		hypercase.Register(own(parse))
	},
	"two-lifts-name": func() {
		g := gridParse
		g.Name = "parse"
		hypercase.Register(project, scriptType, gridType, own(parse, g))
	},
	"own-and-use": func() { hypercase.Register(project, scriptType, own(parse), hypercase.Use(parse)) },
	"own-and-use-calls": func() {
		hypercase.Register(project, scriptType, hypercase.Use(parse))
		hypercase.Register(own(parse))
	},
	"bare-lift": func() { hypercase.Register(project, scriptType, parse) },
	"use-suite": func() { hypercase.Register(project, scriptType, hypercase.Use(calc)) },
	"own-other": func() { hypercase.Register(project, scriptType, own(malformed)) },

	// Relations and descriptions.
	"lift-rejects-unregistered": func() { hypercase.Register(project, rootPkg, scriptType, own(parse)) },
	"lift-rejects-later": func() {
		hypercase.Register(project, rootPkg, scriptType, own(parse))
		hypercase.Register(malformed)
	},
	"lift-only": func() { hypercase.Register(project, rootPkg, malformed, scriptType, own(parse)) },
	"calc-only": func() { hypercase.Register(project, rootPkg, malformed, scriptType, own(calc)) },
	"lift-nodesc": func() {
		l := parse
		l.Description = ""
		l.Outputs = []decl.Output{{Pattern: "error.txt", Description: " "}, {Pattern: "stderr.txt", Description: "What it writes."}}
		hypercase.Register(project, rootPkg, malformed, decl.Type[Script]{Dir: "script"}, own(l, calc))
	},
}
