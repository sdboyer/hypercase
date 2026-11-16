// Package decl holds the declarations a project makes to hypercase.
//
// It doesn't import testing, so a project's ordinary packages can hold what
// they declare, and its commands can use the declarations too.
package decl

// Project is what a project supplies to hypercase. Every package that
// registers with hypercase registers its project.
type Project struct {
	// UpdateVar names the environment variable that runs an update. Set to
	// 1, it makes go test write the goldens instead of comparing them. One
	// variable covers every corpus in the project, and an update is the
	// whole module's: UpdateVar=1 go test ./..., at the module root.
	UpdateVar string
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
