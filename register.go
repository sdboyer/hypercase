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
)

// Register registers what a package declares to hypercase: its project,
// its properties, its suites and its options. It is called when the
// package is initialized, from an init function, before any test runs. A
// test function runs what was registered with [Test], and registers
// nothing.
//
// A refusal, such as two suites with one name, panics with the message the
// framework would otherwise print, so no test in the package runs. A check
// that relates two registrations, such as a Rejects entry naming a
// property, waits until initialization is over, and fails the run before
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
		case columnDecl:
			reg.addColumn(v.column())
		case Option:
			v.apply(&reg)
		default:
			refuse(fmt.Sprintf("Register was given a %T, which is nothing hypercase registers. Pass a decl.Project, a decl.Property, a hypercase.Suite or an option, such as hypercase.Root.", item))
		}
	}
}

// An Option is a registration that declares nothing: where the package's
// corpus root is, or that the package runs the root-package check.
// [Register] takes it.
type Option struct {
	apply func(r *registrations)
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
	columns     []*column
	root        string // the corpus root, relative to the module root
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
	for _, l := range p.IssueLinks {
		if _, err := regexp.Compile(l.Pattern); err != nil {
			refuse(fmt.Sprintf("the project's issue link pattern %q is not a regular expression: %v. Fix the pattern.", l.Pattern, err))
		}
		if strings.TrimSpace(l.URL) == "" {
			refuse(fmt.Sprintf("the project's issue link pattern %q has no URL. Give the template of the issue's URL, such as https://example.com/sheets/issues/$1.", l.Pattern))
		}
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
	if c.consumes != fsType {
		refuse(fmt.Sprintf("the suite %s consumes %s. A suite consumes the input view, fs.FS.", c.name, c.consumes))
	}
	if c.corpus == "" {
		refuse(fmt.Sprintf("the suite %s names no Corpus. Name the directory of the archives it runs over, testdata/<corpus>/ under the corpus root.", c.name))
	}
	if !validName(c.corpus) {
		refuse(fmt.Sprintf("the suite %s names the corpus %q, which breaks the naming rule: %s. Rename the corpus's directory.", c.name, c.corpus, nameRule))
	}
	for i, p := range c.rejects {
		for _, q := range c.rejects[:i] {
			if p.Name == q.Name {
				refuse(fmt.Sprintf("the suite %s rejects %s twice. List each property once.", c.name, p.Name))
			}
		}
	}
	for _, o := range c.outputs {
		if _, err := path.Match(o.Pattern, ""); err != nil || !fs.ValidPath(o.Pattern) || o.Pattern == "." {
			refuse(fmt.Sprintf("the suite %s declares an output with the pattern %q, which is not a pattern of slash paths under out/%s/ in the syntax of path.Match. Fix the pattern.", c.name, o.Pattern, c.name))
		}
	}
	for _, o := range r.columns {
		if o.name == c.name {
			refuse(fmt.Sprintf("two suites are named %s. Column names are unique; rename one.", c.name))
		}
	}
	r.columns = append(r.columns, c)
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
