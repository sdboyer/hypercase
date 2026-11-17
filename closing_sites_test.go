package hypercase

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestOneClosing: report is the one way a cell's notes fail its test, so
// close gives every closing of a message about a cell. The functions
// that fail a test through its *testing.T are report and those listed
// here, none of which reports a cell's notes. And no note's body gives
// advice, which is its closing's.
func TestOneClosing(t *testing.T) {
	// Each function, but report, that fails a test, and what it reports.
	others := map[string]string{
		"Test":                           "what keeps the run from starting",
		"newRunner":                      "what keeps the runner from being made",
		"checkRegistered":                "a property a registration names, and no registration registers",
		"runner.checkRelations":          "what relates two registrations",
		"runner.checkTransformRelations": "what relates a transform to the registrations",
		"runner.checkDescriptions":       "a registration's missing description",
		"runner.checkColumnDescriptions": "a column's missing descriptions",
		"runner.requireRootCheck":        "a corpus root no package's registry records the root-package check of",
		"runner.planPaths":               "the paths from the corpus types, before any case runs",
		"runner.planCorpus":              "a corpus's directory or archive that can't be read",
		"runner.runVariant":              "a transform's variant that no cell runs on",
		"runner.judgeInputs":             "a transform's variant's committed inputs, in its owner",
		"runner.write":                   "what keeps an update from writing an archive",
		"runner.appliesToNoCase":         "a suite that applies to no case",
		"runner.checkRegistry":           "the package's registry, against what it registers",
		"runner.rootCheck":               "the root-package check",
		"runner.checkArchive":            "an archive as a whole, in the root-package check",
		"runner.runChecks":               "the corpus checks",
		"runner.runCheck":                "a corpus check's findings",
		"runner.orphanFindings":          "a file under testdata/checks/ that is no check's findings",
	}
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	failing := map[string][]string{} // function → the places it fails a test
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			name := fd.Name.Name
			if fd.Recv != nil {
				typ := fd.Recv.List[0].Type
				if s, ok := typ.(*ast.StarExpr); ok {
					typ = s.X
				}
				if ix, ok := typ.(*ast.IndexExpr); ok {
					typ = ix.X
				}
				name = typ.(*ast.Ident).Name + "." + name
			}
			// The names of the *testing.T values in reach: the function's
			// parameters, and those of the closures in it.
			ts := map[string]bool{}
			ast.Inspect(fd, func(n ast.Node) bool {
				var params *ast.FieldList
				switch n := n.(type) {
				case *ast.FuncDecl:
					params = n.Type.Params
				case *ast.FuncLit:
					params = n.Type.Params
				}
				if params != nil {
					for _, p := range params.List {
						if isTestingT(p.Type) {
							for _, id := range p.Names {
								ts[id.Name] = true
							}
						}
					}
				}
				return true
			})
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if id, ok := sel.X.(*ast.Ident); ok && ts[id.Name] && slices.Contains([]string{"Error", "Errorf", "Fatal", "Fatalf", "Fail", "FailNow"}, sel.Sel.Name) {
					failing[name] = append(failing[name], fset.Position(call.Pos()).String())
				}
				// A note's body is what is wrong, never what to do.
				if sel.Sel.Name == "add" && len(call.Args) == 3 {
					ast.Inspect(call.Args[1], func(n ast.Node) bool {
						lit, ok := n.(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							return true
						}
						s, _ := strconv.Unquote(lit.Value)
						for _, advice := range []string{"If the change is intended", "Fix ", "Run ", "run %s", "Remove the line", "delete the section", "Delete nothing", "An update writes nothing for this cell", "until the problems above are fixed", "read the diff"} {
							if strings.Contains(s, advice) {
								t.Errorf("%s: a note's body says %q, which is advice: give it in the note's default closing.", fset.Position(lit.Pos()), advice)
							}
						}
						return true
					})
				}
				return true
			})
		}
	}
	if _, ok := failing["report"]; !ok {
		t.Errorf("report fails no test")
	}
	for _, name := range slices.Sorted(maps.Keys(failing)) {
		if _, ok := others[name]; !ok && name != "report" {
			t.Errorf("%s fails a test, at %s.\nA message about a cell is a list of notes, which report fails its test with, so close gives its closings. If it is about no cell, list it in others, saying what it reports.", name, strings.Join(failing[name], ", "))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(others)) {
		if _, ok := failing[name]; !ok {
			t.Errorf("others lists %s, which fails no test", name)
		}
	}
}

// isTestingT reports whether a parameter's type is *testing.T.
func isTestingT(e ast.Expr) bool {
	s, ok := e.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := s.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "testing" && sel.Sel.Name == "T"
}
