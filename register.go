package hypercase

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/sdboyer/hypercase/decl"
)

// Register registers what a package declares to hypercase: its project and
// its suites. It is called when the package is initialized, from an init
// function, before any test runs. A test function runs what was registered
// with [Test], and registers nothing.
//
// A refusal, such as two suites with one name, panics with the message the
// framework would otherwise print, so no test in the package runs.
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
		case columnDecl:
			reg.addColumn(v.column())
		default:
			refuse(fmt.Sprintf("Register was given a %T, which is nothing hypercase registers. Pass a decl.Project or a hypercase.Suite.", item))
		}
	}
}

// reg holds what the package registered. A test binary is one package, so
// the registry is the package's.
var reg registry

type registry struct {
	mu      sync.Mutex
	started bool
	project *decl.Project
	columns []*column
}

func (r *registry) addProject(p decl.Project) {
	if r.project != nil {
		refuse("Register was given a decl.Project a second time. Register the project once per package.")
	}
	if !updateVarRE.MatchString(p.UpdateVar) {
		refuse(fmt.Sprintf("the project's UpdateVar, %q, is not an environment variable's name: uppercase ASCII letters, digits and underscores, starting with a letter. Name the variable that runs an update, such as RECIPES_UPDATE_GOLDEN.", p.UpdateVar))
	}
	r.project = &p
}

func (r *registry) addColumn(c *column) {
	if !validName(c.name) {
		refuse(fmt.Sprintf("the suite name %q breaks the naming rule: %s. Rename the suite.", c.name, nameRule))
	}
	if c.run == nil {
		refuse(fmt.Sprintf("the suite %s has no Run. Give it the function that runs a case.", c.name))
	}
	if c.consumes != fsType {
		refuse(fmt.Sprintf("the suite %s consumes %s. A suite consumes the input view, fs.FS.", c.name, c.consumes))
	}
	for _, o := range r.columns {
		if o.name == c.name {
			refuse(fmt.Sprintf("two suites are named %s. Column names are unique; rename one.", c.name))
		}
	}
	r.columns = append(r.columns, c)
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

var (
	nameRE      = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	updateVarRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
)

const nameRule = "lowercase ASCII letters and digits, in words joined by single hyphens, starting with a letter"

func validName(s string) bool { return nameRE.MatchString(s) }
