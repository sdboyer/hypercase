package hypercase_test

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/archive"
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

// cellsCheck lists every cell, with what a check reads of it.
var cellsCheck = hypercase.Check{
	Name:        "cells",
	Description: "Lists every cell, with its outcome, its column's entry, its case's properties and its files.",
	Run: func(cells []hypercase.Cell) ([]string, error) {
		var out []string
		for _, c := range cells {
			name := c.Type + "/" + c.Archive
			if c.Case != "" {
				name += "/" + c.Case
			}
			outcome := strings.TrimSuffix(string(c.Outcome.Format()), "\n")
			if c.Outcome.State == "" {
				outcome = "no outcome"
			}
			out = append(out, fmt.Sprintf("%s %s: %s; the %s %s of %s, rejecting [%s]; states [%s]; files [%s]",
				name, c.Column, outcome, c.Entry.Kind, c.Entry.Name, c.Entry.Package, strings.Join(c.Entry.Rejects, " "),
				strings.Join(c.Properties, " "), strings.Join(slices.Sorted(maps.Keys(c.Files)), " ")))
		}
		return out, nil
	},
}

// steeredCheck is a check whose Run returns what it is given.
func steeredCheck(name string, findings []string, err error) hypercase.Check {
	return hypercase.Check{
		Name:        name,
		Description: "Gives the findings it was declared with.",
		Run:         func([]hypercase.Cell) ([]string, error) { return findings, err },
	}
}

var (
	// unsortedCheck gives its findings out of order, and one twice.
	unsortedCheck = steeredCheck("unsorted", []string{"b", "a", "b"}, nil)
	// mutatingCheck changes the cells it reads, which no other check sees.
	// Its name sorts first, so it runs before the others.
	mutatingCheck = hypercase.Check{
		Name:        "alters",
		Description: "Changes the cells it reads, and finds nothing.",
		Run: func(cells []hypercase.Cell) ([]string, error) {
			for i := range cells {
				cells[i].Column = "changed"
				for p := range cells[i].Files {
					cells[i].Files[p] = nil
				}
			}
			return nil, nil
		},
	}
)

// located requires every rejected cell's error.txt to start with the
// cell it is about, as "A1: ". A cell a gaps line marks is excused. Each
// such cell is a finding, with its gap, whatever its outcome, so the
// findings record every cell's Gap.
var locatedCheck = hypercase.Check{
	Name:        "located",
	Description: "Every rejected cell's error starts with the cell it is about, but a known gap's.",
	Run: func(cells []hypercase.Cell) ([]string, error) {
		var findings, unlocated []string
		for _, c := range cells {
			name := c.Type + "/" + c.Archive
			if c.Case != "" {
				name += "/" + c.Case
			}
			name += " " + c.Column
			switch {
			case c.Gap != "":
				findings = append(findings, name+" is a known gap: "+c.Gap)
			case c.Outcome.State == archive.Rejected && !cellLead.Match(c.Files["error.txt"]):
				unlocated = append(unlocated, name)
			}
		}
		if len(unlocated) > 0 {
			return nil, fmt.Errorf("the error.txt of %s starts with no cell", strings.Join(unlocated, ", "))
		}
		return findings, nil
	},
}

var cellLead = regexp.MustCompile(`^[A-Z]+[0-9]+: `)

func checks(items ...any) {
	hypercase.Register(append([]any{project, rootPkg, scriptType}, items...)...)
}

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

	// Corpus checks.
	"tr-checks": func() {
		transforms(transformType, own(echoRejects, looped, upper), cellsCheck, unsortedCheck, mutatingCheck)
	},
	"check-gaps": func() { transforms(transformType, own(echoRejects, looped, upper), locatedCheck) },
	"tr-b-checks": func() {
		hypercase.Register(append([]any{project, transformType, own(withName(echoRejects, "copy"), cellsCheck)}, sheetsProps...)...)
	},
	"check-error": func() { checks(own(echo), steeredCheck("fails", nil, errors.New("no cell is rejected"))) },
	"check-panic": func() {
		checks(own(echo, hypercase.Check{Name: "boom", Description: "Panics.", Run: func([]hypercase.Cell) ([]string, error) { panic("check boom") }}))
	},
	"check-newline":  func() { checks(own(echo), steeredCheck("lines", []string{"one\ntwo"}, nil)) },
	"check-empty":    func() { checks(own(echo), steeredCheck("empty", []string{"a", ""}, nil)) },
	"check-none":     func() { checks(own(echo), steeredCheck("none", nil, nil)) },
	"check-bad-name": func() { checks(own(echo), steeredCheck("Cells", nil, nil)) },
	"check-no-run":   func() { checks(own(echo), hypercase.Check{Name: "idle", Description: "Runs nothing."}) },
	"check-twice":    func() { checks(own(echo), steeredCheck("none", nil, nil), steeredCheck("none", nil, nil)) },
	"check-no-description": func() {
		c := steeredCheck("none", nil, nil)
		c.Description = "\n"
		checks(own(echo), c)
	},
}
