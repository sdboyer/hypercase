package hypercase_test

import (
	"errors"
	"io/fs"
	"strings"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/decl"
)

// Step 8's declarations: outcomes, Codes, classifiers and corpus checks.

// steeredCodes is a project's Codes that its cases steer: an error that
// reads "codes <a>,<b>" has the codes a and b, as written; one that reads
// codes-panic panics; any other has none.
func steeredCodes(err error) []string {
	msg := err.Error()
	if msg == "codes-panic" {
		panic("codes boom")
	}
	if rest, ok := strings.CutPrefix(msg, "codes "); ok {
		return strings.Split(rest, ",")
	}
	return nil
}

var projectCodes = decl.Project{UpdateVar: fixtureUpdate, Codes: steeredCodes}

// steeredClassifier is a classifier its cases steer, through the input
// view's cl/ tree:
//
//	cl/detail  its bytes, less a final newline, are the detail
//	cl/file    names a cell's file, whose bytes, trimmed, are the detail
//	cl/err     the detail says whether the run returned an error
//	cl/error   the classifier fails with what it holds
//	cl/panic   the classifier panics with what it holds
var steeredClassifier = hypercase.Classifier[Script]{
	Name:        "steered",
	Description: "Gives the detail the case's cl/ tree says.",
	Classify: func(in Script, got hypercase.Result) (string, error) {
		read := func(p string) (string, bool) {
			data, err := fs.ReadFile(in, "cl/"+p)
			return strings.TrimSuffix(string(data), "\n"), err == nil
		}
		if s, ok := read("panic"); ok {
			panic(s)
		}
		if s, ok := read("error"); ok {
			return "", errors.New(s)
		}
		if s, ok := read("detail"); ok {
			return s, nil
		}
		if s, ok := read("file"); ok {
			return strings.TrimSpace(string(got.File(s))), nil
		}
		if _, ok := read("err"); ok {
			if got.Err != nil {
				return "saw-error", nil
			}
			return "saw-none", nil
		}
		return "", nil
	},
}

func withClassifier(s hypercase.Suite[Script], c hypercase.Classifier[Script]) hypercase.Suite[Script] {
	s.Classifier = c
	return s
}

// classified is echo with a probe and the steered classifier, rejecting
// circular.
var classified = withClassifier(withRejects(withProbe(echo), circular), steeredClassifier)

// outcomeRegistrations are step 8's declaration sets, by name.
var outcomeRegistrations = map[string]func(){
	"outcomes": func() { hypercase.Register(projectCodes, rootPkg, circular, scriptType, own(classified)) },

	// Classifiers that break a rule.
	"cl-bad-name": func() {
		c := steeredClassifier
		c.Name = "Steered"
		hypercase.Register(projectCodes, rootPkg, circular, scriptType, own(withClassifier(echo, c)))
	},
	"cl-no-classify": func() {
		c := steeredClassifier
		c.Classify = nil
		hypercase.Register(projectCodes, rootPkg, circular, scriptType, own(withClassifier(echo, c)))
	},
	"cl-no-description": func() {
		c := steeredClassifier
		c.Description = " "
		hypercase.Register(projectCodes, rootPkg, circular, scriptType, own(withClassifier(classified, c)))
	},
}
