package hypercase

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
)

// Register registers what a package declares to hypercase: its project, its
// properties and its suites. It is called when the package is initialized,
// from an init function, before any test runs. A test function runs what
// was registered with [Test], and registers nothing.
//
// A refusal, such as two suites with one name, panics with the message the
// framework would otherwise print, so no test in the package runs. A check
// that relates two registrations, such as a Rejects entry naming a property,
// waits until initialization is over, and fails the run before any case
// runs.
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
		default:
			refuse(fmt.Sprintf("Register was given a %T, which is nothing hypercase registers. Pass a decl.Project, a decl.Property or a hypercase.Suite.", item))
		}
	}
}

// reg holds what the package registered. A test binary is one package, so
// the registry is the package's.
var reg registry

type registry struct {
	mu         sync.Mutex
	started    bool
	project    *decl.Project
	properties []decl.Property
	columns    []*column
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

func (r *registry) addProperty(p decl.Property) {
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
	for i, p := range c.rejects {
		for _, q := range c.rejects[:i] {
			if p.Name == q.Name {
				refuse(fmt.Sprintf("the suite %s rejects %s twice. List each property once.", c.name, p.Name))
			}
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
func (r *registry) property(name string) (decl.Property, bool) {
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
