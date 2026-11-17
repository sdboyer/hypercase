package hypercase

import (
	"go/ast"
	"go/constant"
	"go/printer"
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
// reads every function of the package, its func literals and its
// package-level variables' initializers among them, and finds every use of
// a testing value: a value of *testing.T, testing.TB, a type that embeds
// one, or any type that implements testing.TB. Each use is a site but as
// the receiver of a method quietMethods lists, which neither writes a
// message nor ends a test, or as an argument to a function the package
// declares, whose parameter, as the function declares it, has the value's
// own type, and whose body the check reads. A site is a message method,
// called or taken as a method value or a method expression, also through
// an interface that may hold a testing value; or the value passed to
// another function, assigned, converted, returned or made part of a
// composite literal, and any other use. Each is report's one t.Errorf, or
// a site messageSites lists for its function by what it is and its
// message, saying what it reports, which is no cell's note: each listed
// site is one use. A message method's call gives its message's words in
// the constant it formats, which names it. And no note's body gives
// advice, which is its closing's: no body holds a run of the words of a
// closing's advice, or of what a closing's claims say. A closing's advice
// says nothing of what an update does: no constant of it names an update.
// Nor does a note's body mention the update at all, which is its
// closing's to name: no constant of a body says "update", in any case, or
// holds the words updateCmd gives the update command, followed into every
// package of the module that gives one, as the archive package's problems.
// What another module or the standard library gives a body is data here,
// as the corpus's is. TestClosings checks every body its grids give for
// the update's command, variable and name.
func TestOneClosing(t *testing.T) {
	s := loadSource(t)
	sites := s.messageSites(t)
	if got := sites["report"]; len(got) != 1 || got[0].what != "Errorf" {
		t.Errorf("report's uses of testing values are %v, and the boundary is its one t.Errorf.", got)
	}
	delete(sites, "report")
	// messageSites lists a site by its message, so a message method's call
	// gives its message's words in the constant it formats, which name it:
	// a message made elsewhere and passed whole, as "%s\n%s" or a variable,
	// would be listed by what names no message, and match any.
	for _, name := range slices.Sorted(maps.Keys(sites)) {
		for _, st := range sites[name] {
			if st.msg != "" && !st.text && slices.Contains(messageMethods, st.what) {
				t.Errorf("%s: %s uses %s, with the message %q, which holds no words of its own.\nmessageSites lists a site by its message, so the message's words are the constant it formats, which names it: give them there, and the values as its arguments.", st.at, name, st.what, st.msg)
			}
		}
	}
	type key struct{ what, msg string }
	names := slices.Sorted(maps.Keys(sites))
	for name := range messageSites {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		listed := map[key][]messageSite{}
		for _, w := range messageSites[name] {
			listed[key{w.what, w.msg}] = append(listed[key{w.what, w.msg}], w)
			if strings.TrimSpace(w.reports) == "" {
				t.Errorf("messageSites lists %s's use of %s, with the message %q, with no reason.", name, w.what, w.msg)
			}
		}
		found := map[key][]site{}
		for _, st := range sites[name] {
			found[key{st.what, st.msg}] = append(found[key{st.what, st.msg}], st)
		}
		for _, k := range slices.SortedFunc(maps.Keys(found), func(a, b key) int { return strings.Compare(a.what+" "+a.msg, b.what+" "+b.msg) }) {
			for _, st := range found[k][min(len(listed[k]), len(found[k])):] {
				t.Errorf("%s: %s uses %s, with the message %q, a site messageSites doesn't list.\nA message about a cell is a list of notes, which report fails its test with, so close gives its closings. If it is about no cell, list it in messageSites, under %s, by what it is and its message, saying what it reports.", st.at, name, st.what, st.msg, name)
			}
		}
		for _, k := range slices.SortedFunc(maps.Keys(listed), func(a, b key) int { return strings.Compare(a.what+" "+a.msg, b.what+" "+b.msg) }) {
			for _, w := range listed[k][min(len(found[k]), len(listed[k])):] {
				t.Errorf("messageSites lists, under %s, a use of %s with the message %q, for %q, which no site is left to match: %s has %d such sites, and messageSites lists %d.", name, w.what, w.msg, w.reports, name, len(found[k]), len(listed[k]))
			}
		}
	}

	// A note's body is what is wrong, never what to do: no sentence of a
	// body holds a sentence of a closing's of three words or more, or is
	// one of two, counting the words that hold a letter. A constant that
	// both reach, as from a helper that names what both refer to, is
	// neither's own.
	advice, claimed, bodies := s.closingWords(t)
	// A closing's advice says what to do, and what an update does is its
	// claims' to say, which TestClosings checks: no constant of its advice
	// names an update. It runs the update only as the command, which it
	// formats in.
	for _, a := range advice {
		for _, w := range strings.FieldsFunc(strings.ToLower(a.text), func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' }) {
			if strings.TrimSuffix(w, "'s") == "update" || w == "updates" {
				t.Errorf("%s: a closing's advice says %q, which names an update.\nWhat an update does is the closing's claims' to say, which TestClosings checks against what it does: say it in a field of claims, or say nothing of it.", a.at, a.text)
				break
			}
		}
	}
	advice = append(advice, claimed...)
	// The constants updateCmd formats the update command with.
	var cmd []string
	ast.Inspect(s.funcDecl(t, "runner.updateCmd").Body, func(n ast.Node) bool {
		if e, ok := n.(ast.Expr); ok {
			if tv, ok := s.info.Types[e]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
				cmd = append(cmd, constant.StringVal(tv.Value))
				return false
			}
		}
		return true
	})
	if len(cmd) == 0 {
		t.Fatalf("updateCmd formats the update command with no constant")
	}
	for _, b := range bodies {
		if strings.Contains(strings.ToLower(b.text), "update") {
			t.Errorf("%s: a note's body says %q, which mentions the update.\nA body says what is wrong; what the update does, and that it is the fix, is the closing's to say: say it in a field of claims, or say nothing of it.", b.at, b.text)
		}
		for _, c := range cmd {
			if strings.Contains(b.text, c) {
				t.Errorf("%s: a note's body says %q, which holds the update command's words, %q.\nA body says what is wrong; what the update does, and that it is the fix, is the closing's to say: say it in a field of claims, or say nothing of it.", b.at, b.text, c)
			}
		}
	}
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
	// formatVerb matches a format verb, as fmt's grammar has it: its
	// flags, a width and a precision, each a number or a *, an explicit
	// argument index before either's * and before the verb, and the verb,
	// %% among them.
	formatVerb = regexp.MustCompile(`%[-+# 0]*(?:(?:\[\d+\])?\*|\d+)?(?:\.(?:(?:\[\d+\])?\*|\d+)?)?(?:\[\d+\])?[a-zA-Z%]`)
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

// A site is a use of a testing value, and where it is: what is the name
// of the message method it is the receiver of, or how else it is used
// (pass, assign, convert, return, compose, use, or a method neither
// messageMethods nor quietMethods lists), and msg is the use's message: a
// message method's, as message gives it, or the source of what it is
// passed to, or of the statement or expression that uses it otherwise.
// text records, of a message method's call, that its message is a
// constant that holds words past its format verbs, which name it.
type site struct {
	what, msg, at string
	text          bool
}

// messageSites returns, for each function of the package, by funcName,
// and each package-level variable, as var <name>, the sites in its body or
// initializer, func literals included. It fails on a method of *testing.T
// or testing.TB that messageMethods and quietMethods don't sort.
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
	isQuiet := func(fn *types.Func) bool {
		return fn.Pkg() != nil && fn.Pkg().Path() == "testing" && slices.Contains(quietMethods, fn.Name())
	}
	isTesting := func(typ types.Type) bool { return typ != nil && types.Implements(typ, tb) }
	// followed reports whether the argument i of a call is one the check
	// follows into its callee: the callee is a function the package
	// declares, whose body the check reads, and its parameter, as the
	// callee declares it, has the argument's own type. A parameter whose
	// declared type is a type parameter isn't followed, whatever a call
	// instantiates it with: the body takes the value as the type
	// parameter.
	followed := func(call *ast.CallExpr, i int, arg ast.Expr) bool {
		fun := ast.Unparen(call.Fun)
		switch f := fun.(type) {
		case *ast.IndexExpr:
			fun = f.X
		case *ast.IndexListExpr:
			fun = f.X
		}
		var obj types.Object
		methodExpr := false
		switch f := fun.(type) {
		case *ast.Ident:
			obj = s.info.Uses[f]
		case *ast.SelectorExpr:
			if sel := s.info.Selections[f]; sel != nil {
				obj, methodExpr = sel.Obj(), sel.Kind() == types.MethodExpr
			} else {
				obj = s.info.Uses[f.Sel]
			}
		}
		fn, ok := obj.(*types.Func)
		if !ok || fn.Pkg() != s.pkg {
			return false
		}
		sig := fn.Origin().Signature()
		if recv := sig.Recv(); recv != nil && types.IsInterface(recv.Type()) {
			return false
		}
		ps := sig.Params()
		var pt types.Type
		switch {
		case methodExpr && i == 0:
			// A method expression's first argument is its receiver.
			pt = sig.Recv().Type()
		case methodExpr:
			i--
			fallthrough
		default:
			switch {
			case sig.Variadic() && i >= ps.Len()-1:
				pt = ps.At(ps.Len() - 1).Type()
				if !call.Ellipsis.IsValid() {
					pt = pt.(*types.Slice).Elem()
				}
			case i < ps.Len():
				pt = ps.At(i).Type()
			}
		}
		return pt != nil && types.Identical(pt, s.info.TypeOf(arg))
	}
	found := map[string][]site{}
	inspect := func(name string, root ast.Node) {
		add := func(what, msg string, at token.Pos) {
			found[name] = append(found[name], site{what: what, msg: msg, at: s.fset.Position(at).String()})
		}
		var stack []ast.Node
		ast.Inspect(root, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			var parent, grand ast.Node
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			if len(stack) > 1 {
				grand = stack[len(stack)-2]
			}
			stack = append(stack, n)
			// A message method, however it is reached.
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if ms := s.info.Selections[sel]; ms != nil && ms.Kind() != types.FieldVal {
					if fn := ms.Obj().(*types.Func); isMessage(fn) {
						st := site{what: fn.Name(), at: s.fset.Position(sel.Sel.Pos()).String()}
						if call, ok := parent.(*ast.CallExpr); ok && ast.Unparen(call.Fun) == sel {
							st.msg, st.text = s.message(call)
						}
						found[name] = append(found[name], st)
					}
				}
			}
			e, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			tv, ok := s.info.Types[e]
			if !ok || !tv.IsValue() || !isTesting(tv.Type) {
				return true
			}
			switch p := parent.(type) {
			case *ast.ParenExpr:
				// The parenthesized expression is the use.
			case *ast.SelectorExpr:
				if p.Sel == e {
					// A field's name: the selector is the use.
					return true
				}
				ms := s.info.Selections[p]
				if ms == nil || ms.Kind() == types.FieldVal {
					return true
				}
				if fn := ms.Obj().(*types.Func); !isMessage(fn) && !isQuiet(fn) {
					add("method "+fn.Name(), s.src(p), p.Sel.Pos())
				}
			case *ast.CallExpr:
				i := slices.Index(p.Args, e)
				switch {
				case i < 0:
					add("use", s.src(p), e.Pos())
				case s.info.Types[p.Fun].IsType():
					add("convert", s.src(p), e.Pos())
				case !followed(p, i, e):
					add("pass", s.src(p.Fun), e.Pos())
				}
			case *ast.AssignStmt:
				if !slices.Contains(p.Lhs, e) {
					add("assign", s.src(p), e.Pos())
				}
			case *ast.ValueSpec:
				add("assign", s.src(p), e.Pos())
			case *ast.ReturnStmt:
				add("return", s.src(p), e.Pos())
			case *ast.CompositeLit:
				add("compose", s.src(p), e.Pos())
			case *ast.KeyValueExpr:
				if _, ok := grand.(*ast.CompositeLit); ok && p.Value == e {
					add("compose", s.src(grand), e.Pos())
				} else {
					add("use", s.src(p), e.Pos())
				}
			default:
				add("use", s.src(parent), e.Pos())
			}
			return true
		})
	}
	for _, f := range s.files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Body != nil {
					inspect(s.funcName(d), d.Body)
				}
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					continue
				}
				for _, sp := range d.Specs {
					vs := sp.(*ast.ValueSpec)
					var names []string
					for _, id := range vs.Names {
						names = append(names, id.Name)
					}
					inspect("var "+strings.Join(names, ", "), vs)
				}
			}
		}
	}
	return found
}

// TestMessageSitesFollowed: messageSites follows a testing value into a
// function the package declares only where the parameter, as the
// function declares it, has the value's type: a function's, or a method's
// called through a method expression, whose first argument is its
// receiver. Passed to a parameter whose declared type is a type
// parameter, the value is a site, whatever the call instantiates it with.
func TestMessageSitesFollowed(t *testing.T) {
	t.Parallel()
	dir := tempPackage(t, map[string]string{"p.go": `package p

import "testing"

var held any

func keep[T any](x T) { held = x }

func take(t *testing.T) { t.Log("taken") }

type r struct{}

func (r) take(t *testing.T) { t.Log("taken by r") }

func generic(t *testing.T) { keep(t) }

func plain(t *testing.T) { take(t) }

func method(t *testing.T) { r.take(r{}, t) }
`})
	s, err := typeCheck(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for name, sites := range s.messageSites(t) {
		for _, st := range sites {
			got[name] = append(got[name], st.what+" "+st.msg)
		}
	}
	want := map[string][]string{
		"generic": {"pass keep"},
		"take":    {"Log taken"},
		"r.take":  {"Log taken by r"},
	}
	if !maps.EqualFunc(got, want, slices.Equal) {
		t.Errorf("the sites, by function, are %v, want %v", got, want)
	}
}

// TestFormatVerbs: formatVerb takes each format verb whole, as fmt's
// grammar has it, explicit argument indexes and * widths and precisions
// among them, so a message method's message that is format verbs alone
// holds no words of its own, and TestOneClosing refuses it.
func TestFormatVerbs(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"%s", "%v", "%+v", "%#v", "%q", "%-8s", "% x", "%08b", "%6.2f", "%.3s", "%*d", "%-*d", "%.*s", "%[1]s", "%[2]q", "%[2]*d", "%[2]*[1]d", "%[3]*.[2]*[1]f", "%.[2]*d", "%6.2[1]f", "%%"} {
		if got := formatVerb.ReplaceAllString("a "+v+" b", "%"); got != "a % b" {
			t.Errorf("formatVerb makes %q of %q, want %q", got, "a "+v+" b", "a % b")
		}
	}
	dir := tempPackage(t, map[string]string{"p.go": `package p

import "testing"

func indexed(t *testing.T, msg, fix string) { t.Errorf("%[1]s\n%[2]s", msg, fix) }

func starred(t *testing.T, n int, msg string) { t.Errorf("%*d: %s", n, n, msg) }

func worded(t *testing.T, msg string) { t.Errorf("the cell %[1]s fails", msg) }
`})
	s, err := typeCheck(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for name, sites := range s.messageSites(t) {
		for _, st := range sites {
			got[name] = st.text
		}
	}
	if want := map[string]bool{"indexed": false, "starred": false, "worded": true}; !maps.Equal(got, want) {
		t.Errorf("whether each function's message holds words of its own: %v, want %v", got, want)
	}
}

// message is a message method's message, as messageSites names it: its
// first argument's constant value, with its words single-spaced, or, where
// that has none, the argument's source, or "" for a call with none. text
// reports that the message is a constant that holds words past its
// format verbs.
func (s *source) message(call *ast.CallExpr) (msg string, text bool) {
	if len(call.Args) == 0 {
		return "", false
	}
	if tv := s.info.Types[call.Args[0]]; tv.Value != nil && tv.Value.Kind() == constant.String {
		v := constant.StringVal(tv.Value)
		return strings.Join(strings.Fields(v), " "), words(formatVerb.ReplaceAllString(v, " ")) > 0
	}
	return s.src(call.Args[0]), false
}

// src is a node's source, with its words single-spaced.
func (s *source) src(n ast.Node) string {
	var b strings.Builder
	if err := printer.Fprint(&b, s.fset, n); err != nil {
		panic(err)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// The exported methods of *testing.T and testing.TB that fail a test,
// write to its log or skip it, and those that do none of these.
var (
	messageMethods = []string{"Attr", "Error", "Errorf", "Fail", "FailNow", "Fatal", "Fatalf", "Log", "Logf", "Output", "Skip", "SkipNow", "Skipf"}
	quietMethods   = []string{"ArtifactDir", "Chdir", "Cleanup", "Context", "Deadline", "Failed", "Helper", "Name", "Parallel", "Run", "Setenv", "Skipped", "TempDir"}
)

// A messageSite is a site a function lists: what it is, as a site names
// it, its message, and what it reports, which is no cell's note.
type messageSite struct{ what, msg, reports string }

// messageSites are the sites of each function but report, each by what it
// is and its message, with what it reports. Their order is free.
var messageSites = map[string][]messageSite{
	"Suite.column": {
		{"pass", "run", "the suite's Run, which may fail the cell's test through it: runCell follows that with failedTest's note"},
	},
	"Test": {
		{"Errorf", "finding the module's registries: %v", "the module's registries that can't be found, before any case runs"},
		{"Logf", "The root-package check for %s runs in normal runs, so it runs after the update: go test ./...", "when the root-package check runs, in an update"},
	},
	"checkRegistered": {
		{"Errorf", "%s %s %s, and the package registers no property of that name. Register the property, or remove it from %s.", "a property a registration names that the package doesn't register"},
		{"Errorf", "%s %s a property named %s, which the package registers with another description. Keep one declaration of %s, with one description: register it, and list it in %s.", "a property registered with two descriptions"},
	},
	"newRunner": {
		{"Errorf", "the package registered no decl.Project. Register the project, with its UpdateVar, from an init function.", "a package that registers no project"},
		{"Errorf", "%s is set to %q. Set it to 1 to run an update, or unset it.", "an update variable set to other than 1"},
		{"Errorf", "finding the package's directory: %v", "the package's directory, which can't be found"},
		{"Errorf", "finding the module root: %v", "the module root, which can't be found"},
		{"Errorf", "the package registers the root-package check for the module-root corpus, and is not the module's root package, %s. Register hypercase.RootPackage() in the module's root package, or pass this package's corpus root with hypercase.Root.", "a root-package check for the module-root corpus registered outside the root package"},
	},
	"runner.appliesToNoCase": {
		{"Errorf", "the suite %s applies to no case: %s no archive whose cells run. A suite that applies to no case checks nothing. Add a case it applies to.", "a suite with no archive whose cells run"},
		{"Errorf", "the suite %s applies to no case: its probe says no to every case of %s. A suite that applies to no case checks nothing. Add a case it applies to, or fix the probe.", "a suite whose probe says no to every case"},
	},
	"runner.blocked": {
		{"Skipf", "%s: %s is blocked: %s holds no %s, the outcome of the lift %s, which the cells past it read in a normal run.", "a cell that a lift's missing committed outcome blocks, which it skips: it fails nothing"},
		{"Skipf", "%s: %s is blocked: %s, %s, records the lift %s's outcome as %q.", "a cell that a lift's committed outcome blocks, which it skips: it fails nothing"},
		{"Skipf", "%s: %s is blocked: the lift %s failed on the case: it returned no product and no error.", "a cell that a lift with no product and no error blocks, which it skips: it fails nothing"},
		{"Skipf", "%s: %s is blocked: the lift %s failed on the case, and %s, %s, records its error.", "a cell that a lift whose committed error matches blocks, which it skips: it fails nothing"},
	},
	"runner.checkArchive": {
		{"Errorf", "%s is no archive a run reads: %s. %s.", "an archive hypercase strip refuses by its path, such as by its name, in the root-package check"},
		{"Errorf", "reading %s: %v", "an archive that can't be read, in the root-package check"},
		{"Errorf", "%s has problems of form, so none of its cells run, and no update writes it: %s Fix the archive by hand.", "an archive's problems of form, in the root-package check"},
		{"Errorf", "%s: %s Write the case's description, in prose, as the archive's comment, the text before its first section.", "an archive with no description, in the root-package check"},
		{"Errorf", "%s: %s Write each variant's description, in prose, in its about section.", "a variant with no description, in the root-package check"},
		{"Errorf", "%s has update-written sections out of their place. Run %s to put them in place.", "an archive's update-written sections out of place, in the root-package check"},
		{"Errorf", "%s: %s states %s, and no registry under %s lists a property of that name. Declare the property, or correct the line.", "a properties line naming no registered property, in the root-package check"},
		{"Errorf", "%s: %s, line %d, names transform/%s, and no corpus type of testdata/%s/ lists a transform of that name. List the transform, or remove the line.", "a gaps line naming no transform, in the root-package check"},
		{"Errorf", "%s: %s, line %d, names %s, and no registry has a column of that name over the corpus %s. Register the column, or remove the line.", "a gaps line naming no column, in the root-package check"},
		{"Errorf", "%s: no registry claims %s: %s. %s leaves them in place. Delete them by hand, or run %s at the module root: with no flags, it removes every section an update writes, from every archive under %s but a frozen cell's, and the registries and findings files of the root, and lists what it leaves. Then run %s until it changes nothing, and read what it reports.", "sections no registry claims, of a column whose name hypercase strip refuses, in the root-package check"},
		{"Errorf", "%s: no registry claims %s: %s. If not, first settle whether %s is a bytes lift, on which the registries differ, as the check's failures on them say, then run the check again: until they agree, it can't tell whether %s strip from removing them.", "sections no registry claims, of a lift the registries differ on, in the root-package check"},
		{"Errorf", "%s: no registry claims %s: %s. If not, %srun %s at the module root. It removes %s from every archive under %s, but a frozen cell's, and lists what it leaves. Then run %s until it changes nothing, and read what it reports.", "sections no registry claims, in the root-package check"},
	},
	"runner.checkColumnDescriptions": {
		{"Errorf", "the %s %s has no description. Add one: it says what the %s does, in prose.", "a column with no description"},
		{"Errorf", "the %s %s declares the output %s with no description. Add one: it says what the files hold, in prose.", "a column's output declared with no description"},
	},
	"runner.checkDescriptions": {
		{"Errorf", "the property %s has no description. Add one: it says what the fact about a case is, in prose.", "a property with no description"},
		{"Errorf", "the corpus type %s, of testdata/%s/, has no description. Add one: it says what the type's inputs are, in prose.", "a corpus type with no description"},
		{"Errorf", "the transform %s, of the corpus type %s, has no description. Add one: it says what the transform changes, in prose, and it describes each variant the transform makes.", "a transform with no description"},
		{"Errorf", "the lift %s declares the product file %s with no description. Add one: it says what the files hold, in prose.", "a lift's product file declared with no description"},
		{"Errorf", "the suite %s's classifier %s has no description. Add one: it says what the classifier's details mean, in prose.", "a classifier with no description"},
		{"Errorf", "the corpus check %s has no description. Add one: it says what the check asserts, in prose.", "a corpus check with no description"},
	},
	"runner.checkRegistry": {
		{"Errorf", "reading %s: %v", "the package's directory, which can't be read"},
		{"Errorf", "%s is a temp file left by a write that was cut short. Delete it.", "a registry's temp file a write left"},
		{"Errorf", "reading %s: %v", "the registry, which can't be read"},
		{"Errorf", "writing %s: %v", "the registry, which an update can't write"},
		{"Errorf", "writing %s: %v", "the registry, which an update can't write"},
		{"Errorf", "%s is missing. It records what the package registers, for tools and for the root-package check. Run %s to write it.", "a missing registry"},
		{"Errorf", "%s differs from what the package registers (-committed +registered): %s Run %s to write it, and read the diff before committing.", "a registry that differs from what the package registers"},
	},
	"runner.checkRelations": {
		{"Errorf", "the lift %s's product, %s, is shaped as a corpus type, and the package registers no such corpus type. Register the corpus type, whose input view the lift's product is, or make the product a stage, a type that isn't a named fs.FS wrapper.", "a lift's product shaped as a corpus type the package doesn't register"},
		{"Log", "No case runs until the registrations agree.", "that no case runs until the registrations agree"},
	},
	"runner.checkTransformRelations": {
		{"Errorf", "the package owns the transform %s, of the corpus type %s, and registers no such corpus type. Register the corpus type, which lists the transform in its Transforms.", "an owned transform of a corpus type the package doesn't register"},
		{"Errorf", "the package owns the transform %s, and the corpus type %s lists no transform of that name. List the transform in the type's Transforms, or remove it from hypercase.Own.", "an owned transform its corpus type doesn't list"},
		{"Errorf", "the package owns a transform named %s that differs from the one the corpus type %s lists. Own the declaration the type lists: declare the transform once, and use that declaration in both places.", "an owned transform that differs from the one its corpus type lists"},
	},
	"runner.declined": {
		{"Skipf", "%s: %s declines the base: %s", "a variant its transform declines, with no note, which it skips: it fails nothing"},
	},
	"runner.orphanFindings": {
		{"Errorf", "reading %s: %v", "the findings directory, which can't be read"},
		{"Errorf", "%s is a temp file left by a write that was cut short. Delete it.", "a findings temp file a write left"},
		{"Errorf", "%s is the findings of no corpus check the package registers. If a check should write it, register the check. If not, delete it by hand; %s leaves it in place.", "a findings file of no registered corpus check"},
	},
	"runner.planCorpus": {
		{"Errorf", "reading %s: %v", "a corpus directory that can't be read"},
		{"Logf", "%s: the archive's name breaks the naming rule, so none of its cells run. The root-package check reports it.", "an archive whose name keeps its cells from running, which the root-package check reports"},
		{"Errorf", "reading %s: %v", "an archive that can't be read"},
		{"Logf", "%s has problems of form, so none of its cells run, and no update writes it. The root-package check reports them.", "an archive whose problems of form keep its cells from running, which the root-package check reports"},
	},
	"runner.planPaths": {
		{"Errorf", "from the corpus type %s, of testdata/%s/, %s, which %s, is reached %s: %s There must be exactly one path to each type a column consumes. Remove a lift, or give the paths distinct product types.", "a type reached by more than one path, before any case runs"},
		{"Errorf", "%s fs.FS, which no corpus type is. A lift consumes a corpus type, for the raw trees, or a stage another lift reaches; only a suite runs over the raw input view, by naming a Corpus. Declare a decl.Type for the archives' directory, and consume it.", "a column that consumes fs.FS, before any case runs"},
		{"Errorf", "%s %s, which no registered corpus type reaches. Register the lift that produces it, or the corpus type.", "a type no corpus type reaches, before any case runs"},
		{"Log", "No case runs until each type a column consumes is reached by one path.", "that no case runs until each type has one path"},
	},
	"runner.requireRootCheck": {
		{"Errorf", "no registry records a root-package check for %s, so nothing reports what no column there owns. Register hypercase.RootPackage() in one package over it, then run %s to record it in the package's registry.", "a corpus root no registry records the root-package check of"},
		{"Errorf", "%s records the root-package check for %s, and %s holds no _test.go file, so no test runs the check, and nothing reports what no column there owns. Restore the package's tests, or register hypercase.RootPackage() in another package over it, then run %s to record it in that package's registry.", "a root-package check recorded in a directory with no tests"},
	},
	"runner.rootCheck": {
		{"Errorf", "reading %s: %v", "a registry that can't be read, in the root-package check"},
		{"Errorf", "%s can't be read: %v. Run %s to write it.", "a registry that can't be parsed, in the root-package check"},
		{"Errorf", "%s is a registry, and %s holds no _test.go file, so no test compares it, and it still claims sections. Delete it, or restore the package's tests.", "a registry with no tests beside it, in the root-package check"},
		{"Errorf", "%s register the root-package check for %s. It runs once per corpus root. Register it in one package.", "a root-package check registered twice"},
		{"Errorf", "the column %s is %s. %s %s", "a column two registrations hold, in the root-package check"},
		{"Errorf", "the lift %s is used by %s, and no package owns it, so its error is recorded nowhere.%s %s", "a lift packages use and no package owns, in the root-package check"},
		{"Errorf", "the corpus directory %s has two different corpus types: %s An archive's directory gives its type. %s", "a corpus directory with two corpus types, in the root-package check"},
		{"Errorf", "the transform %s, of testdata/%s/, is owned by no package, so nothing writes its variants' inputs, or judges them against what it makes.%s %s", "a transform no package owns, in the root-package check"},
		{"Errorf", "the transform %s, of testdata/%s/, is owned by %s. One package owns each transform. Own it in one of them.", "a transform two packages own, in the root-package check"},
		{"Errorf", "the property %s has two different entries: %s Declare the property once, in a package the others import, and register that declaration. %s", "a property's differing entries, in the root-package check"},
		{"Errorf", "the registries that own or use the lift %s differ in %s: %s Every package that owns or uses a lift registers one declaration of it: %s. Declare the lift once, in a package the others import, and register that declaration. %s", "a lift's differing registry entries, in the root-package check"},
		{"Errorf", "%s is a temp file left by a write that was cut short. Delete it.", "a registry temp file a write left, in the root-package check"},
		{"Errorf", "reading %s: %v", "testdata/, which can't be read, in the root-package check"},
		{"Errorf", "%s is a symbolic link, and a corpus root's testdata/ is a directory that holds none. Remove it, or replace it with a copy of what it names, if it names anything.", "a symbolic link in place of testdata/, in the root-package check"},
		{"Errorf", "testdata/ holds no symbolic links: %s is one. Remove it, or replace it with a copy of what it names, if it names anything.", "a symbolic link under testdata/, in the root-package check"},
		{"Errorf", "%s holds archives directly, and an archive belongs in a corpus's directory, testdata/<corpus>/. Move the archives into one, or delete them.", "archives directly under testdata/, in the root-package check"},
		{"Errorf", "%s holds archives, and no registry declares the directory: no corpus type has it as its Dir, and no suite names it as its Corpus. Declare a decl.Type with that Dir, or move or delete the archives.", "a directory of archives no registry declares, in the root-package check"},
	},
	"runner.runCell": {
		{"pass", "col.run", "the column's run, which may fail the cell's test through it: failedTest follows that with a note"},
		{"Logf", "stack of the panic: %s", "the stack of a panic, after report gives the panic's note: a log, it fails nothing"},
	},
	"runner.runCheck": {
		{"Errorf", "the corpus check %s panicked: %v Fix the check. An update writes no findings.", "a corpus check that panicked"},
		{"Errorf", "the corpus check %s fails: %v The cells violate what the check requires. Fix the code or the corpus, or the check. An update writes no findings.", "a corpus check that failed"},
		{"Errorf", "the corpus check %s gave an empty finding. Fix the check: a finding says something, on one line. An update writes no findings.", "a corpus check's empty finding"},
		{"Errorf", "the corpus check %s gave a finding that holds a newline: %q. Fix the check: a finding is one line. An update writes no findings.", "a corpus check's finding with a newline"},
		{"Errorf", "reading %s: %v", "a corpus check's findings file, which can't be read"},
		{"Errorf", "writing %s: %v", "a corpus check's findings file, which an update can't write"},
		{"Errorf", "writing %s: %v", "a corpus check's findings file, which an update can't write"},
		{"Errorf", "%s, the corpus check %s's findings, has no golden. Run %s to write it, and read it before committing.", "a corpus check's findings with no golden"},
		{"Errorf", "%s, the corpus check %s's findings, differs (-golden +got): %s If the change is intended, run %s and read the diff before committing.", "a corpus check's findings that differ from their golden"},
	},
	"runner.runChecks": {
		{"Errorf", "the corpus check %s can't read its cells: %v Fix what the message names. No check runs, and an update writes no findings.", "a corpus check that can't read its cells"},
	},
	"runner.runVariant": {
		{"Skipf", "%s: the transform %s failed on the base, and the package that owns it reports why.", "a variant whose transform failed on the base, which the transform's owner reports: it fails nothing"},
	},
	"runner.skipped": {
		{"Skipf", "%s: %s's probe says the case isn't for it: %s", "a cell whose probe says the case isn't for it, with no note, which it skips: it fails nothing"},
	},
	"runner.write": {
		{"Errorf", "%s is a symbolic link, and a corpus root's testdata/ is a directory that holds none, so the update wrote nothing to %s. Remove it, or replace it with a copy of what it names, if it names anything, then run %s again.", "a symbolic link in place of testdata/, which an update writes no archive through"},
		{"Errorf", "testdata/ holds no symbolic links: %s is one, so the update wrote nothing to %s. Remove it, or replace it with a copy of what it names, if it names anything, then run %s again.", "a symbolic link under testdata/ on an archive's path, which an update writes no archive through"},
		{"Errorf", "%s changed while the update ran: its comment or hand-written sections differ from those its cells ran on, so the update wrote nothing to it. Run %s again.", "an archive that changed while the update ran, which it doesn't write"},
		{"Errorf", "writing %s: %v", "an archive the update can't write"},
	},
}

// A text is a constant string, and where it is.
type text struct{ text, at string }

// closingWords returns the constant strings that make the advice of the
// package's closings, those of what their claims say, and those that make
// its notes' bodies, however an expression gives them: in a note's or a
// closing's literal or an assignment to its field, or in what is assigned
// to a variable, a field or a parameter they take, or what a function
// they call returns, followed to the constants, through every package
// whose source loadSource reads: this one, and each of the module that it
// imports. What a function of another module or the standard library
// returns is data to it, as what the corpus holds is.
func (s *source) closingWords(t *testing.T) (advice, claimed, bodies []text) {
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
	var objOf func(e ast.Expr) types.Object
	objOf = func(e ast.Expr) types.Object {
		var o types.Object
		switch e := ast.Unparen(e).(type) {
		case *ast.Ident:
			if o = s.info.Uses[e]; o == nil {
				o = s.info.Defs[e]
			}
		case *ast.SelectorExpr:
			if sel := s.info.Selections[e]; sel != nil {
				o = sel.Obj()
			} else {
				o = s.info.Uses[e.Sel]
			}
		case *ast.IndexExpr:
			// A generic function, instantiated.
			return objOf(e.X)
		case *ast.IndexListExpr:
			return objOf(e.X)
		}
		switch x := o.(type) {
		case *types.Func:
			return x.Origin()
		case *types.Var:
			return x.Origin()
		}
		return o
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
	for _, f := range slices.Concat(s.files, s.deps) {
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
				if fn, ok := objOf(n.Fun).(*types.Func); ok && s.ours(fn.Pkg()) {
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
				if v, ok := objOf(ex).(*types.Var); ok && s.ours(v.Pkg()) {
					out = append(out, flows(v)...)
				}
			case *ast.CallExpr:
				if fn, ok := objOf(ex.Fun).(*types.Func); ok && s.ours(fn.Pkg()) {
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
		claimed = append(claimed, in(s.funcDecl(t, name).Body)...)
	}
	clear(seen)
	bodies = flows(field("note", "body"))
	// A constant reaches by more than one way, as a field's value and as
	// what a function returns: each is given once.
	once := func(ts []text) []text {
		had := map[text]bool{}
		return slices.DeleteFunc(ts, func(x text) bool {
			defer func() { had[x] = true }()
			return had[x]
		})
	}
	return once(advice), once(claimed), once(bodies)
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
