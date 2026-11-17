package hypercase

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
)

// TestOneClosing: report is the one way a cell's notes fail its test, so
// close gives every closing of a message about a cell. The type checker
// finds every use of a method of the testing package's that fails a test,
// writes to its log or skips it, by the method's identity: a call, a
// method value or a method expression, through *testing.T, testing.TB, a
// type that embeds one, or an interface that may hold one. Each is
// report's one t.Errorf, or a site messageSites lists, saying what it
// reports, which is no cell's note. So is each place a testing value
// leaves the package's own functions, for a function value or another
// package's function, which may fail it. And no note's body gives advice,
// which is its closing's: no body holds a run of the words of a closing's
// advice, or of what a closing's claims say.
func TestOneClosing(t *testing.T) {
	s := loadSource(t)
	sites := s.messageSites(t)
	if got := sites["report"]; len(got) != 1 || got[0].what != "Errorf" {
		t.Errorf("report's uses of testing's message methods are %v, and the boundary is its one t.Errorf.", got)
	}
	delete(sites, "report")
	for _, name := range slices.Sorted(maps.Keys(sites)) {
		want, got := messageSites[name], sites[name]
		for i, st := range got {
			switch {
			case i >= len(want):
				t.Errorf("%s: %s uses %s, a site messageSites doesn't list.\nA message about a cell is a list of notes, which report fails its test with, so close gives its closings. If it is about no cell, list it in messageSites, in its place among %s's sites, saying what it reports.", st.at, name, st.what, name)
			case want[i].what != st.what:
				t.Errorf("%s: %s's site %d uses %s, and messageSites lists %s there, for %q.", st.at, name, i+1, st.what, want[i].what, want[i].reports)
			}
		}
		for i := len(got); i < len(want); i++ {
			t.Errorf("messageSites lists %s's site %d, a use of %s, for %q, and %s has %d sites.", name, i+1, want[i].what, want[i].reports, name, len(got))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(messageSites)) {
		if _, ok := sites[name]; !ok {
			t.Errorf("messageSites lists sites of %s, which uses no message method of testing's and passes no testing value on.", name)
		}
		for i, w := range messageSites[name] {
			if strings.TrimSpace(w.reports) == "" {
				t.Errorf("messageSites lists %s's site %d with no reason.", name, i+1)
			}
		}
	}

	// A note's body is what is wrong, never what to do: no sentence of a
	// body holds a sentence of a closing's of three words or more, or is
	// one of two, counting the words that hold a letter. A constant that
	// both reach, as from a helper that names what both refer to, is
	// neither's own.
	advice, bodies := s.closingWords(t)
	both := map[string]bool{}
	for _, a := range advice {
		for _, b := range bodies {
			if a.at == b.at {
				both[a.at] = true
			}
		}
	}
	said := map[string]string{} // a closing's sentence, and the closing's words it is from
	for _, a := range advice {
		if both[a.at] {
			continue
		}
		for _, sn := range sentences(a.text) {
			if words(sn) >= 2 {
				said[sn] = a.text
			}
		}
	}
	for _, b := range bodies {
		if both[b.at] {
			continue
		}
		for _, sn := range sentences(b.text) {
			padded := " " + sn + " "
			for _, c := range slices.Sorted(maps.Keys(said)) {
				if sn == c || words(c) >= 3 && strings.Contains(padded, " "+c+" ") {
					t.Errorf("%s: a note's body says %q, which a closing says, in %q: give it in the note's default closing.", b.at, c, said[c])
				}
			}
		}
	}
}

var (
	sentenceEnd = regexp.MustCompile(`[.;:!?\n]`)
	formatVerb  = regexp.MustCompile(`%[-+# 0-9.]*[a-zA-Z]`)
)

// sentences splits a constant string into its sentences and clauses, each
// lower-cased, with each format verb as %, and its words single-spaced.
func sentences(s string) []string {
	var out []string
	for _, sn := range sentenceEnd.Split(formatVerb.ReplaceAllString(s, "%"), -1) {
		if w := strings.Fields(strings.ToLower(sn)); len(w) > 0 {
			out = append(out, strings.Join(w, " "))
		}
	}
	return out
}

// A site is a use of a message method, or a testing value passed on, and
// where it is.
type site struct{ what, at string }

// messageSites returns, for each function of the package, the uses of
// testing's message methods in it, by the method's name, and the places it
// passes a testing value to a function the package doesn't declare, as
// "pass", in source order. It fails on a method of *testing.T or
// testing.TB that messageMethods and quietMethods don't sort.
func (s *source) messageSites(t *testing.T) map[string][]site {
	t.Helper()
	var testingPkg *types.Package
	for _, p := range s.pkg.Imports() {
		if p.Path() == "testing" {
			testingPkg = p
		}
	}
	if testingPkg == nil {
		t.Fatal("the package doesn't import testing")
	}
	tb := testingPkg.Scope().Lookup("TB").Type().Underlying().(*types.Interface)
	tbMethod := map[string]*types.Func{}
	for m := range tb.Methods() {
		tbMethod[m.Name()] = m
	}
	sorted := map[string]bool{}
	for _, typ := range []types.Type{types.NewPointer(testingPkg.Scope().Lookup("T").Type()), tb} {
		ms := types.NewMethodSet(typ)
		for i := range ms.Len() {
			name := ms.At(i).Obj().Name()
			if !token.IsExported(name) {
				continue
			}
			sorted[name] = true
			if !slices.Contains(messageMethods, name) && !slices.Contains(quietMethods, name) {
				t.Errorf("%s has the method %s, which TestOneClosing doesn't sort: list it in messageMethods if it fails a test, writes to its log or skips it, and in quietMethods if it does none of these.", typ, name)
			}
		}
	}
	for _, name := range slices.Concat(messageMethods, quietMethods) {
		if !sorted[name] {
			t.Errorf("TestOneClosing sorts the method %s, which neither *testing.T nor testing.TB has.", name)
		}
	}
	isMessage := func(fn *types.Func) bool {
		if !slices.Contains(messageMethods, fn.Name()) {
			return false
		}
		if fn.Pkg() != nil && fn.Pkg().Path() == "testing" {
			return true
		}
		// A method of an interface a testing value may be behind.
		sig := fn.Signature()
		m := tbMethod[fn.Name()]
		return sig.Recv() != nil && types.IsInterface(sig.Recv().Type()) && m != nil && types.Identical(sig, m.Signature())
	}
	isTesting := func(typ types.Type) bool { return typ != nil && types.Implements(typ, tb) }
	// declared reports whether a call's function is one the package
	// declares, whose body the inspection reaches.
	declared := func(fun ast.Expr) bool {
		fun = ast.Unparen(fun)
		switch f := fun.(type) {
		case *ast.IndexExpr:
			fun = f.X
		case *ast.IndexListExpr:
			fun = f.X
		}
		var obj types.Object
		switch f := fun.(type) {
		case *ast.Ident:
			obj = s.info.Uses[f]
		case *ast.SelectorExpr:
			if sel := s.info.Selections[f]; sel != nil {
				obj = sel.Obj()
			} else {
				obj = s.info.Uses[f.Sel]
			}
		}
		fn, ok := obj.(*types.Func)
		if !ok || fn.Pkg() != s.pkg {
			return false
		}
		recv := fn.Signature().Recv()
		return recv == nil || !types.IsInterface(recv.Type())
	}
	found := map[string][]site{}
	for _, f := range s.files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			name := s.funcName(fd)
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.SelectorExpr:
					sel := s.info.Selections[n]
					if sel == nil || sel.Kind() == types.FieldVal {
						return true
					}
					if fn := sel.Obj().(*types.Func); isMessage(fn) {
						found[name] = append(found[name], site{fn.Name(), s.fset.Position(n.Sel.Pos()).String()})
					}
				case *ast.CallExpr:
					if declared(n.Fun) {
						return true
					}
					if sel, ok := ast.Unparen(n.Fun).(*ast.SelectorExpr); ok && s.info.Selections[sel] != nil && isTesting(s.info.Selections[sel].Recv()) {
						// A method of the testing value's own.
						return true
					}
					for _, arg := range n.Args {
						if isTesting(s.info.TypeOf(arg)) {
							found[name] = append(found[name], site{"pass", s.fset.Position(arg.Pos()).String()})
						}
					}
				}
				return true
			})
		}
	}
	return found
}

// The exported methods of *testing.T and testing.TB that fail a test,
// write to its log or skip it, and those that do none of these.
var (
	messageMethods = []string{"Attr", "Error", "Errorf", "Fail", "FailNow", "Fatal", "Fatalf", "Log", "Logf", "Output", "Skip", "SkipNow", "Skipf"}
	quietMethods   = []string{"ArtifactDir", "Chdir", "Cleanup", "Context", "Deadline", "Failed", "Helper", "Name", "Parallel", "Run", "Setenv", "Skipped", "TempDir"}
)

// A messageSite is a use of a message method in a function, or "pass", a
// testing value it passes to a function the package doesn't declare, and
// what it reports, which is no cell's note.
type messageSite struct{ what, reports string }

// messageSites are the sites of each function but report, in source
// order, each with what it reports.
var messageSites = map[string][]messageSite{
	"Suite.column": {
		{"pass", "the suite's Run, which may fail the cell's test through it: runCell follows that with failedTest's note"},
	},
	"Test": {
		{"Errorf", "the module's registries that can't be found, before any case runs"},
		{"Logf", "when the root-package check runs, in an update"},
	},
	"checkRegistered": {
		{"Errorf", "a property a registration names that the package doesn't register"},
		{"Errorf", "a property registered with two descriptions"},
	},
	"newRunner": {
		{"Errorf", "a package that registers no project"},
		{"Errorf", "an update variable set to other than 1"},
		{"Errorf", "the package's directory, which can't be found"},
		{"Errorf", "the module root, which can't be found"},
		{"Errorf", "a root-package check for the module-root corpus registered outside the root package"},
	},
	"runner.appliesToNoCase": {
		{"Errorf", "a suite with no archive whose cells run"},
		{"Errorf", "a suite whose probe says no to every case"},
	},
	"runner.blocked": {
		{"Skipf", "a cell that a lift's missing committed outcome blocks, which it skips: it fails nothing"},
		{"Skipf", "a cell that a lift's committed outcome blocks, which it skips: it fails nothing"},
		{"Skipf", "a cell that a lift with no product and no error blocks, which it skips: it fails nothing"},
		{"Skipf", "a cell that a lift whose committed error matches blocks, which it skips: it fails nothing"},
	},
	"runner.checkArchive": {
		{"Errorf", "an archive's name, in the root-package check"},
		{"Errorf", "an archive that can't be read, in the root-package check"},
		{"Errorf", "an archive's problems of form, in the root-package check"},
		{"Errorf", "an archive with no description, in the root-package check"},
		{"Errorf", "a variant with no description, in the root-package check"},
		{"Errorf", "an archive's update-written sections out of place, in the root-package check"},
		{"Errorf", "a properties line naming no registered property, in the root-package check"},
		{"Errorf", "a gaps line naming no transform, in the root-package check"},
		{"Errorf", "a gaps line naming no column, in the root-package check"},
		{"Errorf", "sections no registry claims, in the root-package check"},
	},
	"runner.checkColumnDescriptions": {
		{"Errorf", "a column with no description"},
		{"Errorf", "a column's output declared with no description"},
	},
	"runner.checkDescriptions": {
		{"Errorf", "a property with no description"},
		{"Errorf", "a corpus type with no description"},
		{"Errorf", "a transform with no description"},
		{"Errorf", "a lift's product file declared with no description"},
		{"Errorf", "a classifier with no description"},
		{"Errorf", "a corpus check with no description"},
	},
	"runner.checkRegistry": {
		{"Errorf", "the package's directory, which can't be read"},
		{"Errorf", "a registry's temp file a write left"},
		{"Errorf", "the registry, which can't be read"},
		{"Errorf", "the registry, which an update can't write"},
		{"Errorf", "the registry, which an update can't write"},
		{"Errorf", "a missing registry"},
		{"Errorf", "a registry that differs from what the package registers"},
	},
	"runner.checkRelations": {
		{"Errorf", "a lift's product shaped as a corpus type the package doesn't register"},
		{"Log", "that no case runs until the registrations agree"},
	},
	"runner.checkTransformRelations": {
		{"Errorf", "an owned transform of a corpus type the package doesn't register"},
		{"Errorf", "an owned transform its corpus type doesn't list"},
		{"Errorf", "an owned transform that differs from the one its corpus type lists"},
	},
	"runner.declined": {
		{"Skipf", "a variant its transform declines, with no note, which it skips: it fails nothing"},
	},
	"runner.orphanFindings": {
		{"Errorf", "the findings directory, which can't be read"},
		{"Errorf", "a findings temp file a write left"},
		{"Errorf", "a findings file of no registered corpus check"},
	},
	"runner.planCorpus": {
		{"Errorf", "a corpus directory that can't be read"},
		{"Logf", "an archive whose name keeps its cells from running, which the root-package check reports"},
		{"Errorf", "an archive that can't be read"},
		{"Logf", "an archive whose problems of form keep its cells from running, which the root-package check reports"},
	},
	"runner.planPaths": {
		{"Errorf", "a type reached by more than one path, before any case runs"},
		{"Errorf", "a column that consumes fs.FS, before any case runs"},
		{"Errorf", "a type no corpus type reaches, before any case runs"},
		{"Log", "that no case runs until each type has one path"},
	},
	"runner.requireRootCheck": {
		{"Errorf", "a corpus root no registry records the root-package check of"},
		{"Errorf", "a root-package check recorded in a directory with no tests"},
	},
	"runner.rootCheck": {
		{"Errorf", "a registry that can't be read, in the root-package check"},
		{"Errorf", "a registry that can't be parsed, in the root-package check"},
		{"Errorf", "a registry with no tests beside it, in the root-package check"},
		{"Errorf", "a root-package check registered twice"},
		{"Error", "a column two registrations hold, in the root-package check"},
		{"Errorf", "a lift packages use and no package owns, in the root-package check"},
		{"Errorf", "a corpus directory with two corpus types, in the root-package check"},
		{"Errorf", "a transform no package owns, in the root-package check"},
		{"Errorf", "a transform two packages own, in the root-package check"},
		{"Errorf", "a property's differing entries, in the root-package check"},
		{"Errorf", "a lift's differing registry entries, in the root-package check"},
		{"Errorf", "a registry temp file a write left, in the root-package check"},
		{"Errorf", "testdata/, which can't be read, in the root-package check"},
		{"Errorf", "archives directly under testdata/, in the root-package check"},
		{"Errorf", "a directory of archives no registry declares, in the root-package check"},
	},
	"runner.runCell": {
		{"pass", "the column's run, which may fail the cell's test through it: failedTest follows that with a note"},
		{"Logf", "the stack of a panic, after report gives the panic's note: a log, it fails nothing"},
	},
	"runner.runCheck": {
		{"Errorf", "a corpus check that panicked"},
		{"Errorf", "a corpus check that failed"},
		{"Errorf", "a corpus check's empty finding"},
		{"Errorf", "a corpus check's finding with a newline"},
		{"Errorf", "a corpus check's findings file, which can't be read"},
		{"Errorf", "a corpus check's findings file, which an update can't write"},
		{"Errorf", "a corpus check's findings file, which an update can't write"},
		{"Errorf", "a corpus check's findings with no golden"},
		{"Errorf", "a corpus check's findings that differ from their golden"},
	},
	"runner.runChecks": {
		{"Errorf", "a corpus check that can't read its cells"},
	},
	"runner.runVariant": {
		{"Skipf", "a variant whose transform failed on the base, which the transform's owner reports: it fails nothing"},
	},
	"runner.skipped": {
		{"Skipf", "a cell whose probe says the case isn't for it, with no note, which it skips: it fails nothing"},
	},
	"runner.write": {
		{"Errorf", "an archive that changed while the update ran, which it doesn't write"},
		{"Errorf", "an archive the update can't write"},
	},
}

// A text is a constant string, and where it is.
type text struct{ text, at string }

// closingWords returns the constant strings that make the advice of the
// package's closings, and what their claims say, and those that make its
// notes' bodies, however an expression gives them: in a note's or a
// closing's literal or an assignment to its field, or in what is assigned
// to a variable, a field or a parameter they take, or what a function of
// the package they call returns, followed to the constants.
func (s *source) closingWords(t *testing.T) (advice, bodies []text) {
	t.Helper()
	field := func(typeName, name string) *types.Var {
		st := s.pkg.Scope().Lookup(typeName).Type().Underlying().(*types.Struct)
		for f := range st.Fields() {
			if f.Name() == name {
				return f
			}
		}
		t.Fatalf("%s has no field %s", typeName, name)
		return nil
	}
	objOf := func(e ast.Expr) types.Object {
		switch e := ast.Unparen(e).(type) {
		case *ast.Ident:
			if o := s.info.Uses[e]; o != nil {
				return o
			}
			return s.info.Defs[e]
		case *ast.SelectorExpr:
			if sel := s.info.Selections[e]; sel != nil {
				return sel.Obj()
			}
			return s.info.Uses[e.Sel]
		}
		return nil
	}
	// What is assigned to each variable, field and parameter, the calls
	// of each function, and what it returns.
	assigned := map[types.Object][]ast.Expr{}
	calls := map[*types.Func][]*ast.CallExpr{}
	returns := map[*types.Func][]ast.Expr{}
	type param struct {
		fn *types.Func
		i  int
	}
	params := map[*types.Var]param{}
	for _, f := range s.files {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok {
				fn := s.info.Defs[fd.Name].(*types.Func)
				for i := range fn.Signature().Params().Len() {
					params[fn.Signature().Params().At(i)] = param{fn, i}
				}
				if fd.Body != nil {
					ast.Inspect(fd.Body, func(n ast.Node) bool {
						if _, ok := n.(*ast.FuncLit); ok {
							return false
						}
						if r, ok := n.(*ast.ReturnStmt); ok {
							returns[fn] = append(returns[fn], r.Results...)
						}
						return true
					})
				}
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				if len(n.Lhs) == len(n.Rhs) {
					for i, l := range n.Lhs {
						if o := objOf(l); o != nil {
							assigned[o] = append(assigned[o], n.Rhs[i])
						}
					}
				}
			case *ast.ValueSpec:
				if len(n.Names) == len(n.Values) {
					for i, id := range n.Names {
						assigned[s.info.Defs[id]] = append(assigned[s.info.Defs[id]], n.Values[i])
					}
				}
			case *ast.CompositeLit:
				st, ok := s.info.TypeOf(n).Underlying().(*types.Struct)
				if !ok {
					return true
				}
				for i, el := range n.Elts {
					if kv, ok := el.(*ast.KeyValueExpr); ok {
						if o := s.info.Uses[kv.Key.(*ast.Ident)]; o != nil {
							assigned[o] = append(assigned[o], kv.Value)
						}
						continue
					}
					assigned[st.Field(i)] = append(assigned[st.Field(i)], el)
				}
			case *ast.CallExpr:
				if fn, ok := objOf(n.Fun).(*types.Func); ok && fn.Pkg() == s.pkg {
					calls[fn] = append(calls[fn], n)
				}
			}
			return true
		})
	}
	seen := map[types.Object]bool{}
	var flows func(o types.Object) []text
	var in func(e ast.Node) []text
	in = func(e ast.Node) []text {
		var out []text
		ast.Inspect(e, func(n ast.Node) bool {
			ex, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			if tv, ok := s.info.Types[ex]; ok && tv.Value != nil {
				if tv.Value.Kind() == constant.String {
					out = append(out, text{constant.StringVal(tv.Value), s.fset.Position(ex.Pos()).String()})
				}
				return false
			}
			switch ex := ex.(type) {
			case *ast.Ident, *ast.SelectorExpr:
				if v, ok := objOf(ex).(*types.Var); ok && v.Pkg() == s.pkg {
					out = append(out, flows(v)...)
				}
			case *ast.CallExpr:
				if fn, ok := objOf(ex.Fun).(*types.Func); ok && fn.Pkg() == s.pkg {
					out = append(out, flows(fn)...)
				}
			}
			return true
		})
		return out
	}
	flows = func(o types.Object) []text {
		if seen[o] {
			return nil
		}
		seen[o] = true
		var out []text
		for _, e := range assigned[o] {
			out = append(out, in(e)...)
		}
		if fn, ok := o.(*types.Func); ok {
			for _, e := range returns[fn] {
				out = append(out, in(e)...)
			}
		}
		if v, ok := o.(*types.Var); ok {
			if p, ok := params[v]; ok {
				for _, call := range calls[p.fn] {
					if p.i < len(call.Args) {
						out = append(out, in(call.Args[p.i])...)
					}
				}
			}
		}
		return out
	}
	advice = flows(field("closing", "advice"))
	// What claims say is the words of the functions that render them.
	for _, name := range []string{"staging.text", "staging.does", "staged"} {
		advice = append(advice, in(s.funcDecl(t, name).Body)...)
	}
	clear(seen)
	bodies = flows(field("note", "body"))
	return advice, bodies
}

// words counts a sentence's words that hold a letter.
func words(sn string) int {
	n := 0
	for _, w := range strings.Fields(sn) {
		if strings.IndexFunc(w, unicode.IsLetter) >= 0 {
			n++
		}
	}
	return n
}
