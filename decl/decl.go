// Package decl holds the declarations a project makes to hypercase.
//
// It doesn't import testing, so a project's ordinary packages can hold what
// they declare, and its commands can use the declarations too.
package decl

import (
	"io"
	"io/fs"

	"github.com/sdboyer/jenny"
)

// Project is what a project supplies to hypercase. Every package that
// registers with hypercase registers its project.
type Project struct {
	// UpdateVar names the environment variable that runs an update. Set to
	// 1, it makes go test write the goldens instead of comparing them. One
	// variable covers every corpus in the project, and an update is the
	// whole module's: UpdateVar=1 go test ./..., at the module root.
	UpdateVar string
	// RenderError renders an error that a column's function returned, for
	// the cell's error.txt. It receives the cell's input view, so it can
	// quote the input the error points at, and what it returns ends with a
	// newline. Without it, error.txt holds the error's text and a newline.
	// A panic's error.txt is never rendered: it holds the panic value.
	RenderError func(in fs.FS, err error) []byte
}

// A Type is a corpus type: a directory of archives, given a Go type. T
// wraps a case's input view, everything under its archive's in/, with the
// trees as its top-level directories. A column that consumes T receives
// the raw trees.
type Type[T ~struct{ fs.FS }] struct {
	// Dir names the directory of the type's archives, testdata/<Dir>/
	// under the corpus root. Nothing in an archive names its type.
	Dir string
	// Description says what the type's inputs are. It is required.
	Description string
}

// A Lift is a registered function from one type to the next: from a corpus
// type or a stage to a stage, the type the lift reaches. Each lift is a
// column, so a case the lift rejects is tested once, at the lift. Its cell
// files go under out/<Name>/: error.txt when its function returns an error,
// and stderr.txt when it writes to stderr.
type Lift[In, Out any] struct {
	// Name is the column's name, and names its directory, out/<Name>/.
	// Lifts and suites share one name space.
	Name string
	// Description says what the lift does. It is required.
	Description string
	// Outputs declares error.txt and stderr.txt, as a suite declares its
	// outputs, each with a description.
	Outputs []Output
	// Rejects lists the properties the lift rejects. Its cell is expected
	// to err exactly when its case states one of them.
	Rejects []Property
	// Run makes the product from the input. It receives the archive's
	// name, the typed input and a writer for stderr, and the error it
	// returns is its rejection. It takes no testing.TB, so a command can
	// call it too. Its product is read-only to the columns that read it.
	Run func(archive string, in In, stderr io.Writer) (Out, error)
	// Jenny makes a product that is a corpus type, a named fs.FS wrapper:
	// the files it generates from the input are the product's input view.
	// Such a lift gives Jenny in place of Run. This hypercase refuses a
	// lift whose product is a corpus type, so no lift gives Jenny yet.
	Jenny jenny.Jenny[In]
}

// A Property is a named fact about a case, such as circular: a sheet in
// which some formula refers back to itself. A case states the properties it
// has in its archive's properties section, and a column lists those it
// rejects. A case is expected to err in a column exactly when it states a
// property the column rejects.
type Property struct {
	// Name is the property's name, as a case's properties section states it.
	Name string
	// Description says what the fact is. It is required.
	Description string
}

// An Output declares a class of files a column writes under its directory,
// out/<column>/. Every file a cell produces matches exactly one of its
// column's declarations: each file the run returns, and stdout.txt,
// stderr.txt and error.txt, which the framework writes for it.
type Output struct {
	// Pattern matches the files' slash paths under the column's directory,
	// in the syntax of path.Match: values.txt, or tables/*.csv. A * doesn't
	// match a slash.
	Pattern string
	// Description says what the files hold. It is required.
	Description string
}
