package hypercase_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/root"
)

// The fixtures' declarations. The child registers one set, chosen by name.

var project = decl.Project{UpdateVar: fixtureUpdate}

var rootPkg = hypercase.RootPackage()

var own = hypercase.Own

// The fixtures' corpus types.
type (
	// Script holds the archives of testdata/script/, trees of instructions
	// that the fixtures' columns follow.
	Script struct{ fs.FS }
	// Other and Later are corpus types of other directories.
	Other struct{ fs.FS }
	Later struct{ fs.FS }
)

var (
	scriptType = decl.Type[Script]{Dir: "script", Description: "Trees of instructions, under in/<tree>/, that the fixtures' columns follow."}
	otherType  = decl.Type[Other]{Dir: "other", Description: "Trees of instructions, in another directory."}
	laterType  = decl.Type[Later]{Dir: "later", Description: "Trees of instructions, in a directory with no archive yet."}
)

// steered returns a suite its cases steer, over a corpus type. Each file
// under the input view's <tree>/ is an instruction:
//
//	<tree>/out/<path>      returned as the output <path>
//	<tree>/nonl/<path>     returned as <path>, less its final newline
//	<tree>/crlf/<path>     returned as <path>, with CRLF line endings
//	<tree>/extranl/<path>  returned as <path>, with one more newline
//	<tree>/marker/<path>   returned as <path>, with a line shaped like a section marker
//	<tree>/twice/<path>    returned as <path>, twice
//	<tree>/up/<path>       returned as ../<path>, outside the cell's directory
//	<tree>/stdout          written to stdout
//	<tree>/stderr          written to stderr
//	<tree>/error           returned as the error, less its final newline
//	<tree>/panic           panicked with, less its final newline
//	<tree>/fatal           given to t.Fatal
//	<tree>/skip            given to t.Skip
//	<tree>/tempdir         returned as dir.txt: the path of a t.TempDir
//	<tree>/archive         returned as archive.txt: the archive's name
//	<tree>/edit            names a section of the archive, or "comment", then
//	                       optionally a line: the run adds the line, or
//	                       "edited", to it on disk, as a person's edit
//	                       during the run would
//	<tree>/probe           read by withProbe's probe, and ignored by the run
//	<tree>/probe-panic     read by withProbe's probe, and ignored by the run
func steered[T ~struct{ fs.FS }](name, tree string) hypercase.Suite[T] {
	return hypercase.Suite[T]{
		Name:        name,
		Description: "Does what the case's " + tree + "/ tree says.",
		Outputs:     steeredOutputs,
		Run: func(t testing.TB, archiveName string, in T, stdout, stderr io.Writer) ([]hypercase.File, error) {
			return runSteered(t, archiveName, tree, struct{ fs.FS }(in).FS, stdout, stderr)
		},
	}
}

var steeredOutputs = []decl.Output{
	{Pattern: "values.txt", Description: "The values the case's instructions give."},
	{Pattern: "?.txt", Description: "A file the case's instructions name with one letter."},
	{Pattern: "dir.txt", Description: "The path of a temp dir the run made."},
	{Pattern: "archive.txt", Description: "The archive's name, as the run received it."},
	{Pattern: "error.txt", Description: "The error the case's instructions give."},
	{Pattern: "stdout.txt", Description: "What the case's instructions write to stdout."},
	{Pattern: "stderr.txt", Description: "What the case's instructions write to stderr."},
}

var (
	echo  = steered[Script]("echo", "do")
	shout = steered[Script]("shout", "shout")
)

func runSteered(t testing.TB, archiveName, tree string, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
	var files []hypercase.File
	paths, err := archive.Paths(in)
	if err != nil {
		t.Fatal(err)
	}
	var retErr error
	for _, p := range paths {
		rest, ok := strings.CutPrefix(p, tree+"/")
		if !ok {
			continue
		}
		data, err := fs.ReadFile(in, p)
		if err != nil {
			t.Fatal(err)
		}
		text := strings.TrimSuffix(string(data), "\n")
		kind, sub, _ := strings.Cut(rest, "/")
		switch kind {
		case "out":
			files = append(files, hypercase.File{Path: sub, Data: data})
		case "nonl":
			files = append(files, hypercase.File{Path: sub, Data: bytes.TrimSuffix(data, []byte("\n"))})
		case "crlf":
			files = append(files, hypercase.File{Path: sub, Data: bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n"))})
		case "extranl":
			files = append(files, hypercase.File{Path: sub, Data: append(bytes.Clone(data), '\n')})
		case "marker":
			files = append(files, hypercase.File{Path: sub, Data: append(bytes.Clone(data), "-- x --\n"...)})
		case "twice":
			files = append(files, hypercase.File{Path: sub, Data: data}, hypercase.File{Path: sub, Data: data})
		case "up":
			files = append(files, hypercase.File{Path: "../" + sub, Data: data})
		case "stdout":
			fmt.Fprint(stdout, string(data))
		case "stderr":
			fmt.Fprint(stderr, string(data))
		case "error":
			retErr = errors.New(text)
		case "panic":
			panic(text)
		case "fatal":
			t.Fatal(text)
		case "skip":
			t.Skip(text)
		case "tempdir":
			files = append(files, hypercase.File{Path: "dir.txt", Data: []byte(t.TempDir() + "\n")})
		case "archive":
			files = append(files, hypercase.File{Path: "archive.txt", Data: []byte(archiveName + "\n")})
		case "edit":
			editArchive(t, archiveName, text)
		case "probe", "probe-panic":
		default:
			t.Fatalf("%s: no instruction %s", tree, p)
		}
	}
	return files, retErr
}

// editArchive adds a line to a section of the archive, or to its comment,
// unless it already ends with that line. The instruction is the section's
// name, then optionally the line, which is "edited" when it is left out.
func editArchive(t testing.TB, archiveName, instruction string) {
	section, line, _ := strings.Cut(instruction, "\n")
	if line == "" {
		line = "edited"
	}
	line += "\n"
	mod, err := root.Module(".")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(mod, "testdata", "script", archiveName+".txtar")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	a := archive.Parse(data)
	edit := func(b []byte) []byte {
		if bytes.HasSuffix(b, []byte(line)) {
			return b
		}
		return append(b, line...)
	}
	if section == "comment" {
		a.Comment = edit(a.Comment)
	}
	for i := range a.Sections {
		if a.Sections[i].Name == section {
			a.Sections[i].Data = edit(a.Sections[i].Data)
		}
	}
	if !bytes.Equal(a.Format(), data) {
		if err := archive.Write(p, a); err != nil {
			t.Fatal(err)
		}
	}
}

// withProbe gives a suite the probe that reads the do/ tree: do/probe holds
// the reason it says no, and an empty one says no with no reason;
// do/probe-panic holds what it panics with.
func withProbe(s hypercase.Suite[Script]) hypercase.Suite[Script] {
	s.Probe = func(in Script) (string, bool) { return steeredProbe(in, "do") }
	return s
}

// steeredProbe is the probe that reads a tree: <tree>/probe holds the
// reason it says no, and an empty one says no with no reason;
// <tree>/probe-panic holds what it panics with.
func steeredProbe(in fs.FS, tree string) (string, bool) {
	if data, err := fs.ReadFile(in, tree+"/probe-panic"); err == nil {
		panic(strings.TrimSuffix(string(data), "\n"))
	}
	data, err := fs.ReadFile(in, tree+"/probe")
	if err != nil {
		return "", true
	}
	return strings.TrimSpace(string(data)), false
}

func withName[T any](s hypercase.Suite[T], name string) hypercase.Suite[T] {
	s.Name = name
	return s
}

func withDescription[T any](s hypercase.Suite[T], d string) hypercase.Suite[T] {
	s.Description = d
	return s
}

func withRejects[T any](s hypercase.Suite[T], ps ...decl.Property) hypercase.Suite[T] {
	s.Rejects = ps
	return s
}

func withCorpus[T any](s hypercase.Suite[T], corpus string) hypercase.Suite[T] {
	s.Corpus = corpus
	return s
}

func withOutputs[T any](s hypercase.Suite[T], os ...decl.Output) hypercase.Suite[T] {
	s.Outputs = os
	return s
}

var (
	circular = decl.Property{Name: "circular", Description: "Some cell's formula refers back to itself, directly or through other cells."}
	dangling = decl.Property{Name: "dangling", Description: "Some formula refers to a cell the sheet doesn't have."}
	// malformed is registered, and no column of the step 3 to 5 fixtures
	// rejects it.
	malformed = decl.Property{Name: "malformed", Description: "Some cell holds neither a number nor a formula."}
	// loop is circular, declared again with another description.
	loop = decl.Property{Name: "circular", Description: "A loop."}
)

// registrations are the fixtures' declaration sets, by name.
var registrations = map[string]func(){
	"echo":        func() { hypercase.Register(project, rootPkg, scriptType, own(echo)) },
	"echo-nodesc": func() { hypercase.Register(project, rootPkg, scriptType, own(withDescription(echo, " \n"))) },
	"bad-name":    func() { hypercase.Register(project, scriptType, own(withName(echo, "Echo"))) },
	"two-names":   func() { hypercase.Register(project, scriptType, own(echo, echo)) },
	"no-project":  func() { hypercase.Register(scriptType, own(echo)) },
	"no-run": func() {
		hypercase.Register(project, scriptType, own(hypercase.Suite[Script]{Name: "echo", Description: "Runs nothing."}))
	},
	"two-projects": func() {
		hypercase.Register(project, scriptType, own(echo))
		hypercase.Register(project)
	},
	"late":    func() { hypercase.Register(project, rootPkg, scriptType, own(echo)) },
	"unknown": func() { hypercase.Register(project, 42) },

	"rejects": func() {
		hypercase.Register(project, rootPkg, circular, dangling, malformed, scriptType, own(withRejects(echo, circular, dangling)))
	},
	"rejects-unregistered": func() { hypercase.Register(project, rootPkg, scriptType, own(withRejects(echo, circular))) },
	"rejects-registered-later": func() {
		hypercase.Register(project, rootPkg, scriptType, own(withRejects(echo, circular)))
		hypercase.Register(circular)
	},
	"rejects-other-description": func() {
		hypercase.Register(project, rootPkg, loop, scriptType, own(withRejects(echo, circular)))
	},
	"property-nodesc": func() {
		p := decl.Property{Name: "circular"}
		hypercase.Register(project, rootPkg, p, scriptType, own(withRejects(echo, p)))
	},
	"two-properties":    func() { hypercase.Register(project, circular, circular) },
	"bad-property-name": func() { hypercase.Register(project, decl.Property{Name: "Circular", Description: "A loop."}) },
	"rejects-twice": func() {
		hypercase.Register(project, circular, scriptType, own(withRejects(echo, circular, circular)))
	},

	"echo-shout": func() { hypercase.Register(project, rootPkg, scriptType, own(echo, shout)) },
	"two-corpora": func() {
		hypercase.Register(project, rootPkg, scriptType, otherType, own(echo, steered[Other]("shout", "shout")))
	},
	"probe": func() {
		hypercase.Register(project, rootPkg, circular, scriptType, own(withRejects(withProbe(echo), circular)))
	},
	"probe-none": func() {
		hypercase.Register(project, rootPkg, circular, scriptType, own(withRejects(echo, circular)))
	},
	"nested": func() {
		hypercase.Register(project, rootPkg, scriptType, own(withOutputs(echo, append(slices.Clone(steeredOutputs),
			decl.Output{Pattern: "*/*", Description: "A file one directory down."})...)))
	},
	// unwritable can make each of outputs()'s problems: a.txt matches two
	// declarations, and its RenderError is steered.
	"unwritable": func() {
		hypercase.Register(projectRender, rootPkg, scriptType, own(withOutputs(echo, append(slices.Clone(steeredOutputs),
			decl.Output{Pattern: "a.txt", Description: "The file a, which ?.txt matches too."},
			decl.Output{Pattern: "*/*", Description: "A file one directory down."})...)))
	},
	"narrow": func() {
		hypercase.Register(project, rootPkg, scriptType, own(withOutputs(echo,
			decl.Output{Pattern: "values.txt", Description: "The values."},
			decl.Output{Pattern: "?.txt", Description: "A file named with one letter."},
			decl.Output{Pattern: "a.txt", Description: "The file a."},
		)))
	},
	"output-nodesc": func() {
		hypercase.Register(project, rootPkg, scriptType, own(withOutputs(echo, decl.Output{Pattern: "values.txt", Description: "\n"})))
	},
	"bad-pattern": func() {
		hypercase.Register(project, scriptType, own(withOutputs(echo, decl.Output{Pattern: "values[.txt", Description: "The values."})))
	},
	"bad-pattern-path": func() {
		hypercase.Register(project, scriptType, own(withOutputs(echo, decl.Output{Pattern: "../values.txt", Description: "The values."})))
	},
	"corpus-set":        func() { hypercase.Register(project, scriptType, own(withCorpus(echo, "script"))) },
	"bad-dir":           func() { hypercase.Register(project, decl.Type[Script]{Dir: "Script", Description: "A script."}) },
	"two-roots":         func() { hypercase.Register(project, hypercase.Root("a"), hypercase.Root("b")) },
	"bad-root":          func() { hypercase.Register(project, hypercase.Root("/abs")) },
	"two-root-packages": func() { hypercase.Register(project, rootPkg, rootPkg) },
	"no-root-check":     func() { hypercase.Register(project, scriptType, own(echo)) },

	// Packages over one shared corpus: a at the module root, b below it.
	"pkg-a":          func() { hypercase.Register(project, rootPkg, scriptType, own(echo)) },
	"pkg-a-noroot":   func() { hypercase.Register(project, scriptType, own(echo)) },
	"pkg-a-circular": func() { hypercase.Register(project, rootPkg, circular, scriptType, own(echo)) },
	"pkg-b":          func() { hypercase.Register(project, scriptType, own(shout)) },
	"pkg-b-gone":     func() { hypercase.Register(project) },
	"root-only":      func() { hypercase.Register(project, rootPkg) },
	"pkg-b-echo":     func() { hypercase.Register(project, scriptType, own(withName(shout, "echo"))) },
	"pkg-b-circular": func() { hypercase.Register(project, circular, scriptType, own(withRejects(shout, circular))) },
	"pkg-b-loop":     func() { hypercase.Register(project, loop, scriptType, own(withRejects(shout, loop))) },
	"pkg-b-root":     func() { hypercase.Register(project, rootPkg, scriptType, own(shout)) },
	"pkg-b-other":    func() { hypercase.Register(project, hypercase.Root("other"), scriptType, own(shout)) },
	"pkg-b-later":    func() { hypercase.Register(project, laterType, own(steered[Later]("shout", "shout"))) },
	"corp-a":         func() { hypercase.Register(project, hypercase.Root("corp"), rootPkg, scriptType, own(echo)) },
	"corp-b":         func() { hypercase.Register(project, hypercase.Root("corp"), rootPkg, scriptType, own(shout)) },

	// Issue links, given out of order.
	"echo-links": func() {
		p := project
		p.IssueLinks = []decl.IssueLink{
			{Pattern: `sheets#(\d+)`, URL: "https://example.com/sheets/issues/$1"},
			{Pattern: `recipes#(\d+)`, URL: "https://example.com/recipes/issues/$1"},
		}
		hypercase.Register(p, rootPkg, scriptType, own(echo))
	},
	"bad-link-pattern": func() {
		p := project
		p.IssueLinks = []decl.IssueLink{{Pattern: `sheets#(\d+`, URL: "https://example.com/sheets/issues/$1"}}
		hypercase.Register(p, rootPkg, scriptType, own(echo))
	},
	"link-no-url": func() {
		p := project
		p.IssueLinks = []decl.IssueLink{{Pattern: `sheets#(\d+)`, URL: " "}}
		hypercase.Register(p, rootPkg, scriptType, own(echo))
	},
}

// duringTest are what some fixtures do in the child's test, before
// hypercase.Test runs.
var duringTest = map[string]func(t *testing.T){
	"late": func(t *testing.T) { hypercase.Register(own(withName(echo, "late"))) },
}

func runTest(t *testing.T) { hypercase.Test(t) }

// fixturePackages names, for each fixture, its packages' directories and
// the registration each runs, so TestFixtureRegistries can check their
// committed registries.
var fixturePackages = map[string][]fixturePkg{
	"bad-archive-name": {{".", "echo"}},
	"basic":            {{".", "echo"}},
	"cant-hold":        {{".", "echo"}},
	"changed":          {{".", "echo"}},
	"descriptions":     {{".", "echo"}},
	"error":            {{".", "echo"}},
	"expected":         {{".", "rejects"}},
	"fatal":            {{".", "echo"}},
	"form":             {{".", "echo"}},
	"framework-names":  {{".", "echo"}},
	"gaps":             {{".", "rejects"}},
	"gaps-form":        {{".", "rejects"}},
	"missing-error":    {{".", "rejects"}},
	"missing-golden":   {{".", "echo"}},
	"newline":          {{".", "echo"}},
	"order":            {{".", "echo"}},
	"orphans":          {{".", "echo"}},
	"panic":            {{".", "echo"}},
	"panic-states":     {{".", "rejects"}},
	"properties-form":  {{".", "rejects"}},
	"regen":            {{".", "echo"}},
	"skip":             {{".", "echo"}},
	"temp-left":        {{".", "echo"}},
	"tempdir":          {{".", "echo"}},
	"unexpected":       {{".", "rejects"}},
	"unknown-property": {{".", "rejects"}},
	"unrejected":       {{".", "rejects"}},
	"weakness":         {{".", "rejects"}},

	"two-columns":  {{".", "echo-shout"}},
	"two-corpora":  {{".", "two-corpora"}},
	"probes":       {{".", "probe"}},
	"no-case":      {{".", "probe"}},
	"empty-corpus": {{".", "echo"}},
	"outputs":      {{".", "narrow"}},
	"file-dir":     {{".", "nested"}},
	"shared":       {{".", "pkg-a"}, {"b", "pkg-b"}},

	"ladder":        {{".", "ladder"}},
	"lift-errors":   {{".", "ladder"}},
	"render":        {{".", "ladder-render"}},
	"params":        {{".", "calc-only"}},
	"two-types":     {{".", "two-types"}},
	"shared-ladder": {{".", "ladder-a"}, {"b", "ladder-b"}},
	"no-archive":    {{".", "lift-only"}},
	"no-product":    {{".", "picked"}},

	"variants":           {{".", "variants"}},
	"transforms":         {{".", "transforms"}},
	"transform-failures": {{".", "poke"}},

	"outcomes":         {{".", "outcomes"}},
	"outcome-failures": {{".", "outcomes"}},
	"check-gaps":       {{".", "check-gaps"}},

	"chain":        {{".", "chain"}},
	"chain-render": {{".", "chain-render"}},
	"chain-past":   {{".", "chain-past"}},
	"chain-shared": {{".", "chain-a"}, {"b", "chain-b"}},
}

type fixturePkg struct{ dir, registration string }
