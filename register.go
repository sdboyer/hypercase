package hypercase

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/erase"
)

// Register registers what a package declares to hypercase: its project,
// its properties, its corpus types, and its options, among them the
// columns it owns and the lifts it uses. It is called when the package is
// initialized, from an init function, before any test runs. A test
// function runs what was registered with [Test], and registers nothing.
//
// A refusal, such as two columns with one name, panics with the message
// the framework would otherwise print, so no test in the package runs. A
// check that needs the whole set, such as a Rejects entry naming a
// property, or the paths from the corpus types to the types the columns
// consume, waits until initialization is over, and fails the run before
// any case runs.
func Register(items ...any) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if reg.started || inTest() {
		refuse("Register was called after a test started. Register what the package declares from an init function, before any test runs.")
	}
	for _, item := range items {
		switch v := item.(type) {
		case decl.Project:
			reg.addProject(v)
		case decl.Property:
			reg.addProperty(v)
		case Option:
			v.apply(&reg)
		case columnDecl:
			refuse(fmt.Sprintf("Register was given the suite %s. A package owns the suites it runs: pass it to hypercase.Own.", v.column().name))
		default:
			e, _ := erase.Of(item)
			switch e := e.(type) {
			case *erase.Type:
				reg.addType(e)
			case *erase.Lift:
				refuse(fmt.Sprintf("Register was given the lift %s. Pass it to hypercase.Own, in the package that owns its column, or to hypercase.Use, in a package that only reads its product.", e.Name))
			default:
				refuse(fmt.Sprintf("Register was given a %T, which is nothing hypercase registers. Pass a decl.Project, a decl.Property, a decl.Type or an option, such as hypercase.Own or hypercase.Root.", item))
			}
		}
	}
}

// An Option is a registration that isn't a declaration: the columns the
// package owns, the lifts it uses, where its corpus root is, or that it
// runs the root-package check. [Register] takes it.
type Option struct {
	apply func(r *registrations)
}

// Own is the option that registers the columns the package owns: suites,
// and lifts, [decl.Lift] values. The package runs their cells, and writes
// and judges their sections. One package owns each column.
func Own(columns ...any) Option {
	return Option{apply: func(r *registrations) {
		for _, item := range columns {
			if c, ok := item.(columnDecl); ok {
				r.addColumn(c.column())
				continue
			}
			if e, _ := erase.Of(item); e != nil {
				if l, ok := e.(*erase.Lift); ok {
					r.addLift(newLift(l, true))
					continue
				}
			}
			refuse(fmt.Sprintf("hypercase.Own was given a %T. It takes the columns the package owns: hypercase.Suite and decl.Lift values.", item))
		}
	}}
}

// Use is the option that registers lifts, [decl.Lift] values, whose
// products the package's columns read, and whose columns another package
// owns. The package computes their products, and neither writes nor judges
// their sections. A failing lift still blocks the package's cells past it,
// and what it writes to stderr is dropped.
func Use(lifts ...any) Option {
	return Option{apply: func(r *registrations) {
		for _, item := range lifts {
			if e, _ := erase.Of(item); e != nil {
				if l, ok := e.(*erase.Lift); ok {
					r.addLift(newLift(l, false))
					continue
				}
			}
			if c, ok := item.(columnDecl); ok {
				refuse(fmt.Sprintf("hypercase.Use was given the suite %s. It takes lifts only: a package runs only its own suites. Own the suite in the package that runs it.", c.column().name))
			}
			refuse(fmt.Sprintf("hypercase.Use was given a %T. It takes lifts, decl.Lift values.", item))
		}
	}}
}

// Root is the option that passes the package's corpus root, as a slash
// path relative to the module root. Without it, the corpus root is the
// module root. Corpora live under the corpus root, at testdata/<corpus>/,
// and every package over one corpus root shares its archives.
func Root(dir string) Option {
	return Option{apply: func(r *registrations) {
		if r.rootSet {
			refuse(fmt.Sprintf("Register was given hypercase.Root a second time, with %q. A package has one corpus root; pass it once.", dir))
		}
		if !fs.ValidPath(dir) {
			refuse(fmt.Sprintf("hypercase.Root was given %q, which is not a slash path relative to the module root, such as walkthrough/step05. Pass the corpus root's path.", dir))
		}
		r.root, r.rootSet = dir, true
	}}
}

// RootPackage is the option that registers the root-package check for the
// package's corpus root. One package per corpus root registers it: for the
// module-root corpus, the module's root package. [Test] runs the check on
// the test itself, before the package's cells. It reads every package's
// registry and every archive under the root, and reports what no column
// owns.
func RootPackage() Option {
	return Option{apply: func(r *registrations) {
		if r.rootPackage {
			refuse("Register was given hypercase.RootPackage a second time. Register the root-package check once.")
		}
		r.rootPackage = true
	}}
}

// reg holds what the package registered. A test binary is one package, so
// the registry is the package's.
var reg = registrations{root: "."}

type registrations struct {
	mu          sync.Mutex
	started     bool
	project     *decl.Project
	properties  []decl.Property
	types       []*corpusType
	lifts       []*lift   // owned and used
	columns     []*column // the owned suites and lifts
	root        string    // the corpus root, relative to the module root
	rootSet     bool
	rootPackage bool
}

func (r *registrations) addProject(p decl.Project) {
	if r.project != nil {
		refuse("Register was given a decl.Project a second time. Register the project once per package.")
	}
	if !updateVarRE.MatchString(p.UpdateVar) {
		refuse(fmt.Sprintf("the project's UpdateVar, %q, is not an environment variable's name: uppercase ASCII letters, digits and underscores, starting with a letter. Name the variable that runs an update, such as RECIPES_UPDATE_GOLDEN.", p.UpdateVar))
	}
	r.project = &p
}

func (r *registrations) addProperty(p decl.Property) {
	if !validName(p.Name) {
		refuse(fmt.Sprintf("the property name %q breaks the naming rule: %s. Rename the property.", p.Name, nameRule))
	}
	for _, o := range r.properties {
		if o.Name == p.Name {
			refuse(fmt.Sprintf("two properties are named %s. Register each property once.", p.Name))
		}
	}
	r.properties = append(r.properties, p)
}

func (r *registrations) addColumn(c *column) {
	if !validName(c.name) {
		refuse(fmt.Sprintf("the suite name %q breaks the naming rule: %s. Rename the suite.", c.name, nameRule))
	}
	if c.run == nil {
		refuse(fmt.Sprintf("the suite %s has no Run. Give it the function that runs a case.", c.name))
	}
	if c.corpus != "" {
		refuse(fmt.Sprintf("the suite %s names the Corpus %q. A suite runs over the archives whose corpus type reaches the type it consumes. Declare a decl.Type with Dir %q, make the suite consume it or a stage it reaches, and remove Corpus.", c.name, c.corpus, c.corpus))
	}
	checkRejects("suite", c.name, c.rejects)
	checkPatterns("suite", c.name, c.outputs)
	r.checkColumnName(c.name, c.describe())
	r.columns = append(r.columns, c)
}

func (r *registrations) addLift(l *lift) {
	if !validName(l.name) {
		refuse(fmt.Sprintf("the lift name %q breaks the naming rule: %s. Rename the lift.", l.name, nameRule))
	}
	switch {
	case isCorpusShape(l.out):
		refuse(fmt.Sprintf("the lift %s's product, %s, is a corpus type, and this hypercase doesn't run lifts whose product is a corpus type. Make the product a stage, a type that isn't a named fs.FS wrapper.", l.name, l.out))
	case l.jenny:
		refuse(fmt.Sprintf("the lift %s gives a Jenny, which makes a product that is a corpus type, and its product is %s. Give it Run, which makes the product.", l.name, l.out))
	case l.run == nil:
		refuse(fmt.Sprintf("the lift %s has no Run. Give it the function that makes its product.", l.name))
	}
	checkRejects("lift", l.name, l.rejects)
	checkPatterns("lift", l.name, l.outputs)
	for _, o := range l.outputs {
		if a, _ := path.Match(o.Pattern, errorFile); !a {
			if b, _ := path.Match(o.Pattern, stderrFile); !b {
				refuse(fmt.Sprintf("the lift %s declares an output with the pattern %q, which matches neither %s nor %s, the only files a lift's cell writes. Remove the declaration.", l.name, o.Pattern, errorFile, stderrFile))
			}
		}
	}
	for _, o := range r.lifts {
		if o.name != l.name || o.in != l.in || o.out != l.out {
			continue
		}
		switch {
		case o.owned && l.owned:
			refuse(fmt.Sprintf("the lift %s is given to hypercase.Own twice. Own it once.", l.name))
		case !o.owned && !l.owned:
			refuse(fmt.Sprintf("the lift %s is given to hypercase.Use twice. Use it once.", l.name))
		default:
			refuse(fmt.Sprintf("the lift %s is given to both hypercase.Own and hypercase.Use. A package owns a lift, or only uses it: pass it to one.", l.name))
		}
	}
	r.checkColumnName(l.name, l.describe())
	r.lifts = append(r.lifts, l)
	if l.owned {
		r.columns = append(r.columns, l.column())
	}
}

// checkColumnName refuses a second column, or lift, of one name: lifts and
// suites share the column names, and a lift the package uses is a column
// of the corpus root.
func (r *registrations) checkColumnName(name, what string) {
	for _, c := range r.columns {
		if c.name == name {
			refuse(fmt.Sprintf("two columns are named %s: %s, and %s. Column names are unique; rename one.", name, c.describe(), what))
		}
	}
	for _, l := range r.lifts {
		if l.name == name {
			refuse(fmt.Sprintf("two columns are named %s: %s, and %s. Column names are unique; rename one.", name, l.describe(), what))
		}
	}
}

func checkRejects(kind, name string, rejects []decl.Property) {
	for i, p := range rejects {
		for _, q := range rejects[:i] {
			if p.Name == q.Name {
				refuse(fmt.Sprintf("the %s %s rejects %s twice. List each property once.", kind, name, p.Name))
			}
		}
	}
}

func checkPatterns(kind, name string, outputs []decl.Output) {
	for _, o := range outputs {
		if _, err := path.Match(o.Pattern, ""); err != nil || !fs.ValidPath(o.Pattern) || o.Pattern == "." {
			refuse(fmt.Sprintf("the %s %s declares an output with the pattern %q, which is not a pattern of slash paths under out/%s/ in the syntax of path.Match. Fix the pattern.", kind, name, o.Pattern, name))
		}
	}
}

// checksDir is the directory name that holds corpus check findings, which
// no corpus type declares.
const checksDir = "checks"

func (r *registrations) addType(e *erase.Type) {
	t := &corpusType{dir: e.Dir, description: e.Description, goType: e.GoType, new: e.New}
	switch {
	case !validName(t.dir):
		refuse(fmt.Sprintf("the corpus type %s declares the directory %q, which breaks the naming rule: %s. Rename the directory.", t.goType, t.dir, nameRule))
	case t.dir == checksDir:
		refuse(fmt.Sprintf("the corpus type %s declares the directory %s, a name reserved for the findings of corpus checks. Name another directory.", t.goType, t.dir))
	}
	for _, o := range r.types {
		switch {
		case o.goType == t.goType && o.dir == t.dir:
			refuse(fmt.Sprintf("the corpus type %s is registered twice. Register it once.", t.goType))
		case o.dir == t.dir:
			refuse(fmt.Sprintf("two corpus types declare the directory %s: %s and %s. An archive's directory gives its type; declare another directory for one of them.", t.dir, o.goType, t.goType))
		case o.goType == t.goType:
			refuse(fmt.Sprintf("the corpus type %s is declared twice, with the directories %s and %s. A corpus type has one directory; register one declaration.", t.goType, o.dir, t.dir))
		}
	}
	r.types = append(r.types, t)
}

// property returns the registered property of a name.
func (r *registrations) property(name string) (decl.Property, bool) {
	for _, p := range r.properties {
		if p.Name == name {
			return p, true
		}
	}
	return decl.Property{}, false
}

// refuse panics with a refusal at registration.
func refuse(msg string) {
	panic("hypercase: " + msg)
}

// inTest reports whether the caller runs inside a test function.
func inTest() bool {
	pcs := make([]uintptr, 64)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if strings.HasPrefix(f.Function, "testing.tRunner") {
			return true
		}
		if !more {
			return false
		}
	}
}

var updateVarRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

const nameRule = archive.NameRule

func validName(s string) bool { return archive.ValidName(s) }
