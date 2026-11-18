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
	// Codes gives the codes of an error that a column's function returned,
	// for the cell's outcome: rejected, then the codes, sorted, each once.
	// A code is not empty, and holds no space, semicolon, carriage return
	// or newline.
	// Without Codes, a rejected cell carries no codes.
	Codes func(err error) []string
	// Positions finds where in the cell's input view an error that a
	// column's function returned points: the ranges of an error.txt whose
	// declaration states FromPositions. Every package that registers such
	// a declaration, owned or used, supplies it. A panic's error.txt has no
	// error to give it, and so no ranges.
	Positions func(err error) []Range
	// IssueLinks link the text that names an issue, in a gaps line or a
	// description, to the issue: one per issue tracker the project's gaps
	// lines name. The registry records them, and hypercase report reads
	// them from there.
	IssueLinks []IssueLink
}

// A Range is a span of lines in one file of a cell's input view: the view
// RenderError receives, the case's input view with its variant's overlay
// laid over it, or, past a bytes lift, the last one's product.
type Range struct {
	// Path is the file's slash path in the view: sheet/A1.
	Path string
	// Start and End are the span's first and last lines, counted from 1,
	// End included. Start and End both 0 is the whole file.
	Start, End int
}

// Inputs says where an output declaration's files come from, in the
// cell's input view: an InputsFunc, which maps each file to its ranges;
// AllInputs, which says each file depends on the whole view; or, on a
// declaration whose pattern is error.txt, FromPositions. Nothing else
// implements it.
type Inputs interface{ inputs() }

// An InputsFunc maps one output file, by its path under the column's
// directory, or, for a bytes lift's product, by its path in the product,
// to the ranges of the cell's input view it comes from. It receives that
// view, the one RenderError receives, which past a bytes lift is the last
// one's product; for a bytes lift's own declarations, it is the lift's
// input view. It is called for each file of each run of the cell, so it
// is a function of its arguments alone. An error.txt declaration takes no
// InputsFunc.
type InputsFunc func(in fs.FS, path string) ([]Range, error)

func (InputsFunc) inputs() {}

// inputsKind is the kind of AllInputs and FromPositions, which a
// declaration states in place of a function.
type inputsKind int

func (inputsKind) inputs() {}

const (
	// AllInputs states that each of the declaration's files depends on the
	// whole input view. It is the explicit escape from a map, and the
	// cell's inputs file says so of each file.
	AllInputs inputsKind = 1
	// FromPositions states that an error.txt's ranges are where the
	// project's Positions says the error points. Only a declaration whose
	// pattern is exactly error.txt states it.
	FromPositions inputsKind = 2
)

// An IssueLink links text that names an issue to the issue. Each match of
// Pattern, in a gaps line or a description, links to the URL that the
// template URL makes from it.
type IssueLink struct {
	// Pattern is a regular expression, in the syntax of package regexp,
	// that matches text naming an issue: sheets#(\d+).
	Pattern string
	// URL is the template of the issue's URL. It may use the expression's
	// groups, as regexp.Regexp.Expand does:
	// https://example.com/sheets/issues/$1.
	URL string
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
	// Transforms are the type's transforms. Each makes a variant of every
	// archive's base it doesn't decline, so every package that runs the
	// type runs them. One package owns each, by giving it to
	// hypercase.Own, and writes and judges its variants' inputs.
	Transforms []Transform[T]
}

// A Transform makes a variant of a case from its base: one mechanical
// change, such as a cycle added, that makes a useful case from most bases
// of its corpus type. An update writes the variant's inputs, under
// transform/<Name>/ in the base's archive, and each column's cells run on
// it.
type Transform[T ~struct{ fs.FS }] struct {
	// Name names the transform, and its variant's sections,
	// transform/<Name>/. Names are unique per corpus type.
	Name string
	// Description says what the transform changes. It is required, and it
	// is the description of each variant the transform makes.
	Description string
	// Gives lists the properties every variant has. A variant's
	// properties are Gives, and each property of Keeps its base states.
	Gives []Property
	// Keeps lists the properties a variant has when its base states them.
	// A base that states a property the transform neither gives nor keeps
	// is declined, with that as the reason.
	Keeps []Property
	// Probe decides from the base's input view alone whether the transform
	// makes a variant of it. When it says no, it gives the reason. A
	// transform with no probe makes a variant of every base it doesn't
	// decline for its properties.
	Probe func(base T) (why string, ok bool)
	// Jenny makes the variant's input view from the base's: the files it
	// generates are the variant's whole input view, by their paths in it,
	// with the trees as their top-level directories. It is required.
	Jenny jenny.Jenny[T]
}

// A Lift is a registered function from one type to the next: from a corpus
// type or a stage to the type the lift reaches, a stage or another corpus
// type. Each lift is a column, so a case the lift rejects is tested once,
// at the lift. Its cell files go under out/<Name>/: error.txt when its
// function returns an error, and stderr.txt when it writes to stderr.
//
// A lift whose product is a corpus type is a bytes lift. Its product is
// the files it makes, that type's input view, and they are committed in
// the case's archive under via/<Name>/, laid out as the type's in/ would
// be: via/<Name>/sheet/A1 is sheet/A1 in the view its readers get. A
// normal run gives the columns past it the committed product, and an
// update the product the lift makes.
type Lift[In, Out any] struct {
	// Name is the column's name, and names its directory, out/<Name>/, and
	// a bytes lift's product's, via/<Name>/. Lifts and suites share one
	// name space.
	Name string
	// Description says what the lift does. It is required.
	Description string
	// Outputs declares error.txt and stderr.txt, as a suite declares its
	// outputs, each with a description. A bytes lift's cell writes no
	// stderr.txt, since its jenny has no stderr, so it declares error.txt
	// alone.
	Outputs []Output
	// Product declares a bytes lift's product's files, by their paths in
	// its input view, as Outputs declares the cell's files: sheet/*. Each
	// file is in a tree, a top-level directory of the view, and the tree
	// params is reserved, as in every input view. A file no declaration
	// matches fails the lift's cell, and so does one that two match. A lift
	// whose product isn't a corpus type declares none.
	Product []Output
	// Rejects lists the properties the lift rejects. Its cell is expected
	// to err exactly when its case states one of them.
	Rejects []Property
	// Run makes the product from the input, for a lift whose product is a
	// stage. It receives the archive's name, the typed input and a writer
	// for stderr, and the error it returns is its rejection. It takes no
	// testing.TB, so a command can call it too. Its product is read-only
	// to the columns that read it.
	Run func(archive string, in In, stderr io.Writer) (Out, error)
	// Jenny makes a bytes lift's product, in place of Run: the files it
	// generates from the input are the product's input view, by their
	// paths in it. An error it returns is the lift's rejection when
	// [Reject] marks all of it: each error the returned one wraps or joins,
	// down to those that wrap none, is one Reject made or below one, as
	// Unwrap finds them. Any other error is a fault of the jenny, jenny's
	// own included, such as two files at one path, and so is a rejection
	// joined with a fault, as jenny's Pipeline joins its jennies' errors:
	// it fails the lift's cell, and no update writes the cell.
	Jenny jenny.Jenny[In]
}

// Reject marks err as a bytes lift's rejection of its input, for the
// lift's jenny to return. A jenny's error has two sources, the jenny's
// generator and jenny itself, and only an error Reject marks all of is a
// rejection, so jenny's own fault, alone or joined with a rejection, is
// no rejection. But Reject marks all of err, whatever it holds: a
// generator that runs a jenny of its own and returns that jenny's error
// marked has jenny's faults in it, such as two files at one path,
// recorded as its rejection. Mark only what the generator rejects.
//
// The framework finds the marks through Unwrap() error and
// Unwrap() []error alone, never through an error's own As or Is method.
// So an error that reaches the errors it holds through As, and not
// through Unwrap, is a fault, however many of them Reject marks.
// go-multierror's is such an error when it holds two or more: its Unwrap
// gives a chain, and each chain's Unwrap gives the chain of the rest,
// never an error the chain holds, down to the last, which wraps none.
// Holding one error, its Unwrap gives that error, so one error Reject
// marks is a rejection there. When the generator rejects the input with
// an error that holds others, mark that error whole, with Reject. An
// error reached through more than 100 Unwrap calls, as in an error whose
// Unwrap leads back to itself, makes the whole error a fault.
//
// The rejection's text is err's, and it wraps err, so errors.As reaches
// err through it. Reject returns nil for a nil err.
func Reject(err error) error {
	if err == nil {
		return nil
	}
	return &rejection{err}
}

// A rejection is an error Reject marks.
type rejection struct{ err error }

func (r *rejection) Error() string { return r.err.Error() }
func (r *rejection) Unwrap() error { return r.err }

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
	// Inputs says where in the cell's input view the files come from: a
	// map, an InputsFunc; AllInputs; or, for error.txt, FromPositions or
	// AllInputs. It is required: a declaration that states none fails the
	// run. The cell's inputs file, out/<column>/inputs, records what it
	// gives for each file.
	Inputs Inputs
}
