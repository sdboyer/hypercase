package hypercase_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
)

// The fixtures' declarations. The child registers one set, chosen by name.

var project = decl.Project{UpdateVar: fixtureUpdate}

// echo is a suite its cases steer. Each file under the input view's do/
// tree is an instruction:
//
//	do/out/<path>      returned as the output <path>
//	do/nonl/<path>     returned as <path>, less its final newline
//	do/crlf/<path>     returned as <path>, with CRLF line endings
//	do/extranl/<path>  returned as <path>, with one more newline
//	do/marker/<path>   returned as <path>, with a line shaped like a section marker
//	do/stdout          written to stdout
//	do/stderr          written to stderr
//	do/error           returned as the error, less its final newline
//	do/panic           panicked with, less its final newline
//	do/fatal           given to t.Fatal
//	do/skip            given to t.Skip
//	do/tempdir         returned as dir.txt: the path of a t.TempDir
var echo = hypercase.Suite[fs.FS]{
	Name:        "echo",
	Description: "Does what the case's do/ tree says.",
	Run:         runEcho,
}

func runEcho(t testing.TB, archiveName string, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
	var files []hypercase.File
	paths, err := archive.Paths(in)
	if err != nil {
		t.Fatal(err)
	}
	var retErr error
	for _, p := range paths {
		rest, ok := strings.CutPrefix(p, "do/")
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
		default:
			t.Fatalf("echo: no instruction %s", p)
		}
	}
	return files, retErr
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

var (
	circular = decl.Property{Name: "circular", Description: "Some cell's formula refers back to itself, directly or through other cells."}
	dangling = decl.Property{Name: "dangling", Description: "Some formula refers to a cell the sheet doesn't have."}
	// malformed is registered, and no column of the fixtures rejects it.
	malformed = decl.Property{Name: "malformed", Description: "Some cell holds neither a number nor a formula."}
)

// registrations are the fixtures' declaration sets, by name.
var registrations = map[string]func(){
	"echo":        func() { hypercase.Register(project, echo) },
	"echo-nodesc": func() { hypercase.Register(project, withDescription(echo, " \n")) },
	"bad-name":    func() { hypercase.Register(project, withName(echo, "Echo")) },
	"two-names":   func() { hypercase.Register(project, echo, echo) },
	"no-project":  func() { hypercase.Register(echo) },
	"no-run": func() {
		hypercase.Register(project, hypercase.Suite[fs.FS]{Name: "echo", Description: "Runs nothing."})
	},
	"two-projects": func() {
		hypercase.Register(project, echo)
		hypercase.Register(project)
	},
	"late":    func() { hypercase.Register(project, echo) },
	"unknown": func() { hypercase.Register(project, 42) },

	"rejects": func() {
		hypercase.Register(project, circular, dangling, malformed, withRejects(echo, circular, dangling))
	},
	"rejects-unregistered": func() { hypercase.Register(project, withRejects(echo, circular)) },
	"rejects-registered-later": func() {
		hypercase.Register(project, withRejects(echo, circular))
		hypercase.Register(circular)
	},
	"rejects-other-description": func() {
		hypercase.Register(project, decl.Property{Name: "circular", Description: "A loop."}, withRejects(echo, circular))
	},
	"property-nodesc": func() {
		p := decl.Property{Name: "circular"}
		hypercase.Register(project, p, withRejects(echo, p))
	},
	"two-properties":    func() { hypercase.Register(project, circular, circular) },
	"bad-property-name": func() { hypercase.Register(project, decl.Property{Name: "Circular", Description: "A loop."}) },
	"rejects-twice": func() {
		hypercase.Register(project, circular, withRejects(echo, circular, circular))
	},
}

// duringTest are what some fixtures do in the child's test, before
// hypercase.Test runs.
var duringTest = map[string]func(t *testing.T){
	"late": func(t *testing.T) { hypercase.Register(withName(echo, "late")) },
}

func runTest(t *testing.T) { hypercase.Test(t) }
