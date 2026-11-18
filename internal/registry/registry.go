// Package registry reads and writes a package's registry,
// testdata/hypercase.json in the package's directory: everything the
// package registers with hypercase, each with its description. Tools read a
// registry instead of parsing Go.
//
// The package doesn't import testing, so a command reads registries the
// same way a test does.
package registry

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Format is the version of what a registry holds. It changes on any change
// to that.
const Format = 9

// Name is a registry's file name, in its package's testdata/ directory.
const Name = "hypercase.json"

// A Registry is what one package registers.
type Registry struct {
	Format int `json:"format"`
	// UpdateVar names the project's update variable, so a tool can print
	// the update command.
	UpdateVar string `json:"updateVar"`
	// CorpusRoot is the package's corpus root, as a slash path relative to
	// the module root: "." for the module root itself.
	CorpusRoot string `json:"corpusRoot"`
	// RootPackage records that the package runs the root-package check for
	// its corpus root.
	RootPackage bool `json:"rootPackage"`
	// Codes records that the project supplies Codes, which gives a
	// rejected cell's outcome its codes.
	Codes bool `json:"codes"`
	// Positions records that the project supplies Positions, which finds
	// where an error points: the ranges of each error.txt whose
	// declaration states FromPositions.
	Positions bool `json:"positions"`
	// IssueLinks are the project's link patterns, which link text naming an
	// issue to the issue.
	IssueLinks []IssueLink `json:"issueLinks"`
	Properties []Property  `json:"properties"`
	Types      []Type      `json:"types"`
	Lifts      []Lift      `json:"lifts"`
	Suites     []Suite     `json:"suites"`
	// Checks are the corpus checks the package registers, sorted by name.
	// It is nil in a registry of a format before ChecksFormat, which
	// records none, and only there: from ChecksFormat on, a package that
	// registers none lists none.
	Checks []Check `json:"checks"`
}

// An IssueLink is one of the project's link patterns: a regular expression,
// and the template of the URL that each match links to.
type IssueLink struct {
	Pattern string `json:"pattern"`
	URL     string `json:"url"`
}

// A Property is a registered property.
type Property struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// A Type is a registered corpus type.
type Type struct {
	// Dir is the directory of the type's archives, under the corpus root's
	// testdata/.
	Dir         string `json:"dir"`
	Description string `json:"description"`
	// GoType is the Go type, by its package's import path and its name.
	GoType     string      `json:"goType"`
	Transforms []Transform `json:"transforms"`
}

// A Transform is one of a corpus type's transforms.
type Transform struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Gives and Keeps name the properties the transform gives and keeps, so
	// a tool computes its variants' properties without running anything.
	Gives []string `json:"gives"`
	Keeps []string `json:"keeps"`
	// Probe records that the transform has a probe.
	Probe bool `json:"probe"`
	// Owned records that the package owns the transform, and writes and
	// judges its variants' inputs.
	Owned bool `json:"owned"`
}

// VariantProperties returns what a transform's variant of a base that
// states base states: what the transform gives, then each property it
// keeps that the base states, in the base's order. A transform declines a
// base that states a property it neither gives nor keeps: neither lists
// those, and the variant, which isn't made, states nothing.
func VariantProperties(gives, keeps, base []string) (props, neither []string) {
	props = slices.Clone(gives)
	for _, p := range base {
		switch {
		case slices.Contains(keeps, p):
			if !slices.Contains(props, p) {
				props = append(props, p)
			}
		case !slices.Contains(gives, p):
			neither = append(neither, p)
		}
	}
	if len(neither) > 0 {
		return nil, neither
	}
	return props, nil
}

// A Lift is a registered lift, which the package owns or only uses.
type Lift struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Input and Product are the Go types the lift goes from and to.
	Input   string `json:"input"`
	Product string `json:"product"`
	// Owned records that the package owns the lift's column.
	Owned bool `json:"owned"`
	// Bytes records a bytes lift: its product is a corpus type's input
	// view, which each case commits under via/<Name>/, beside the lift's
	// cell files under out/<Name>/.
	Bytes bool `json:"bytes"`
	// Corpora are the directories of the corpus types that reach the
	// lift's input, under the corpus root's testdata/.
	Corpora []string `json:"corpora"`
	// Paths are the paths of lifts the lift's input passes through, one
	// from each of Corpora.
	Paths   []Path   `json:"paths"`
	Outputs []Output `json:"outputs"`
	// ProductFiles declares a bytes lift's product's files, by their paths
	// under via/<Name>/. Any other lift's is empty.
	ProductFiles []Output `json:"productFiles"`
	// Rejects names the properties the lift rejects.
	Rejects []string `json:"rejects"`
}

// A Suite is a registered suite.
type Suite struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Consumes is the Go type the suite consumes: RawInput for a suite over
	// the raw input view, whose one corpus is the directory its Corpus names.
	Consumes string `json:"consumes"`
	// Corpora are the directories of the corpus types that reach the type
	// the suite consumes, under the corpus root's testdata/.
	Corpora []string `json:"corpora"`
	// Paths are the paths of lifts the suite's input passes through, one
	// from each of Corpora.
	Paths   []Path   `json:"paths"`
	Outputs []Output `json:"outputs"`
	// Rejects names the properties the suite rejects.
	Rejects []string `json:"rejects"`
	// Classifier is the suite's classifier, which refines its cells'
	// outcomes, or nil.
	Classifier *Classifier `json:"classifier"`
}

// A Path is the path of lifts a column's input passes through from one
// corpus type, the one path the package plans from it, or from a suite's
// Corpus.
type Path struct {
	// Corpus is the corpus type's directory, or the suite's Corpus, under
	// the corpus root's testdata/.
	Corpus string `json:"corpus"`
	// Lifts names the path's lifts, the nearest to the column first: from
	// a sheet to a *sheet.Graph, resolve, then parse. It is empty for a
	// column that consumes the corpus type, and for a suite over its
	// Corpus's raw input views.
	Lifts []string `json:"lifts"`
}

// A Check is a corpus check the package registers.
type Check struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// A Classifier is a suite's classifier.
type Classifier struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// An Output is one of a column's output declarations.
type Output struct {
	Pattern     string `json:"pattern"`
	Description string `json:"description"`
	// Inputs is what the declaration states its files' ranges come from:
	// InputsFunc, InputsPositions, InputsAll or InputsNone. It is "" in a
	// registry of a format before InputsFormat, which records none.
	Inputs string `json:"inputs"`
}

// What an output declaration's Inputs states, as [Output.Inputs] records
// it.
const (
	// InputsFunc maps each file with a function.
	InputsFunc = "func"
	// InputsPositions takes an error.txt's ranges from where the error
	// points, through the project's Positions.
	InputsPositions = "positions"
	// InputsAll maps each file to the whole input view.
	InputsAll = "all"
	// InputsNone states nothing, which fails the run.
	InputsNone = "none"
)

// BytesLifts returns what the registries say of the columns over each
// corpus, by corpus and then column: whether the column is a bytes lift,
// whose via/<column>/… is part of its cell. A column is one when any of
// their entries for a lift of its name over the corpus, owned or used,
// records a bytes lift. Every other column they record over the corpus, a
// suite or a lift, is none. So registries that disagree on a lift make it
// a bytes lift, and a frozen cell keeps its via/. The root-package check
// fails on each registry that makes them disagree: on the lift's entries
// where the registries of packages with tests differ, and on a registry
// in a package with none by its own failure, since it compares no such
// registry's entries. hypercase strip and the root-package check both
// read the registries through it, so that the check's advice on a frozen
// cell's via/ is what strip does, and while they disagree on a lift, the
// check's advice on a frozen cell's via/ of the lift waits on them.
func BytesLifts(regs []*Registry) map[string]map[string]bool {
	lifts := map[string]map[string]bool{}
	column := func(name string, bytes bool, corpora []string) {
		for _, c := range corpora {
			if lifts[c] == nil {
				lifts[c] = map[string]bool{}
			}
			lifts[c][name] = lifts[c][name] || bytes
		}
	}
	for _, r := range regs {
		for _, s := range r.Suites {
			column(s.Name, false, s.Corpora)
		}
		for _, l := range r.Lifts {
			column(l.Name, l.Bytes, l.Corpora)
		}
	}
	return lifts
}

// Marshal returns the registry's bytes: indented JSON, ending with a
// newline, with every list sorted, so the bytes depend only on what was
// registered.
func (r Registry) Marshal() []byte {
	r.Format = Format
	r.IssueLinks = sorted(r.IssueLinks, func(l IssueLink) string { return l.Pattern + "\x00" + l.URL })
	r.Properties = sorted(r.Properties, func(p Property) string { return p.Name })
	types := sorted(r.Types, func(t Type) string { return t.Dir })
	for i, t := range types {
		trs := sorted(t.Transforms, func(tr Transform) string { return tr.Name })
		for j, tr := range trs {
			tr.Gives = sorted(tr.Gives, func(n string) string { return n })
			tr.Keeps = sorted(tr.Keeps, func(n string) string { return n })
			trs[j] = tr
		}
		t.Transforms = trs
		types[i] = t
	}
	r.Types = types
	lifts := sorted(r.Lifts, func(l Lift) string { return l.Name })
	for i, l := range lifts {
		l.Corpora = sorted(l.Corpora, func(n string) string { return n })
		l.Paths = sortedPaths(l.Paths)
		l.Outputs = sorted(l.Outputs, func(o Output) string { return o.Pattern })
		l.ProductFiles = sorted(l.ProductFiles, func(o Output) string { return o.Pattern })
		l.Rejects = sorted(l.Rejects, func(n string) string { return n })
		lifts[i] = l
	}
	r.Lifts = lifts
	suites := sorted(r.Suites, func(s Suite) string { return s.Name })
	for i, s := range suites {
		s.Corpora = sorted(s.Corpora, func(n string) string { return n })
		s.Paths = sortedPaths(s.Paths)
		s.Outputs = sorted(s.Outputs, func(o Output) string { return o.Pattern })
		s.Rejects = sorted(s.Rejects, func(n string) string { return n })
		suites[i] = s
	}
	r.Suites = suites
	r.Checks = sorted(r.Checks, func(c Check) string { return c.Name })
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		panic(err) // a Registry always encodes
	}
	return buf.Bytes()
}

// sorted returns a sorted copy of s, never nil, so an empty list is
// written as [].
func sorted[T any](s []T, key func(T) string) []T {
	out := append(make([]T, 0, len(s)), s...)
	slices.SortStableFunc(out, func(a, b T) int { return cmp.Compare(key(a), key(b)) })
	return out
}

// sortedPaths returns a copy of ps sorted by corpus, never nil, with each
// path's lifts in their order, never nil either.
func sortedPaths(ps []Path) []Path {
	out := sorted(ps, func(p Path) string { return p.Corpus })
	for i, p := range out {
		out[i].Lifts = append(make([]string, 0, len(p.Lifts)), p.Lifts...)
	}
	return out
}

// A FormatError is a registry of a format version this package doesn't
// read.
type FormatError struct{ Format int }

func (e *FormatError) Error() string {
	return fmt.Sprintf("it holds format %d, and this hypercase reads format %d", e.Format, Format)
}

// Parse parses a registry. One of another format version is a
// [*FormatError]. So that every reader of today's registries reads the
// columns and paths the update writes, Parse refuses one the update
// couldn't have written: it lists each suite and lift under a name of its
// own, a column runs over each of its corpora once, and its paths are one
// from each, in their order, and name only lifts the registry lists, each
// once, never the column itself, and none for a suite over the raw input
// view; each output declaration records one of the statements of Inputs,
// in a place registration admits it; and it lists its corpus checks, an
// empty list for none, each under a name of its own.
func Parse(data []byte) (*Registry, error) {
	var head struct {
		Format int `json:"format"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("it is not a registry: %w", err)
	}
	switch {
	case head.Format < 1:
		return nil, errors.New("it is not a registry: it holds no format version")
	case head.Format != Format:
		return nil, &FormatError{Format: head.Format}
	}
	r, err := decode(data)
	if err != nil {
		return nil, err
	}
	if err := r.check(); err != nil {
		return nil, err
	}
	return r, nil
}

// parseChecks are Parse's checks of what a registry holds, each with the
// oldest format whose registries it holds of: from that format on, the
// update couldn't have written a registry the check refuses. Registration
// has refused a second column, or lift, of one name since format 3, and
// planned a column over each corpus once: no test holds that, but main's
// history does, read commit by commit from format 3 on. Format 7 added the
// paths, format 8 the statements of Inputs, and format 9 the corpus
// checks.
var parseChecks = []struct {
	format int
	check  func(*Registry) error
}{
	{3, (*Registry).checkNames},
	{3, (*Registry).checkCorpora},
	{7, (*Registry).checkPaths},
	{8, (*Registry).checkInputs},
	{9, (*Registry).checkChecksListed},
	{9, (*Registry).checkCheckNames},
}

// check reports the first of parseChecks that holds of the registry's
// format and refuses it.
func (r *Registry) check() error {
	for _, c := range parseChecks {
		if r.Format < c.format {
			continue
		}
		if err := c.check(r); err != nil {
			return err
		}
	}
	return nil
}

// errorFile is the output file that holds a column's error.
const errorFile = "error.txt"

// checkInputs reports the first output declaration, a suite's, a lift's or
// a product file's, whose Inputs the update couldn't have written: one
// that records none of the four statements, InputsPositions on a pattern
// that isn't error.txt or on a product file, or InputsFunc on an output
// pattern that matches error.txt. Registration refuses each of these.
func (r *Registry) checkInputs() error {
	check := func(kind, name, what string, outs []Output) error {
		for _, o := range outs {
			matches, _ := path.Match(o.Pattern, errorFile)
			switch o.Inputs {
			case InputsAll, InputsNone:
			case InputsFunc:
				if what == "output" && matches {
					return fmt.Errorf("the %s %s declares the %s %s with inputs %q, and the pattern matches %s, where %s's ranges come from Positions or the whole input view, never from a function", kind, name, what, o.Pattern, o.Inputs, errorFile, errorFile)
				}
			case InputsPositions:
				if what != "output" || o.Pattern != errorFile {
					return fmt.Errorf("the %s %s declares the %s %s with inputs %q, where only an output whose pattern is %s takes its ranges from Positions", kind, name, what, o.Pattern, o.Inputs, errorFile)
				}
			default:
				return fmt.Errorf("the %s %s declares the %s %s with inputs %q, where a registry records %q, %q, %q or %q", kind, name, what, o.Pattern, o.Inputs, InputsFunc, InputsPositions, InputsAll, InputsNone)
			}
		}
		return nil
	}
	for _, s := range r.Suites {
		if err := check("suite", s.Name, "output", s.Outputs); err != nil {
			return err
		}
	}
	for _, l := range r.Lifts {
		if err := check("lift", l.Name, "output", l.Outputs); err != nil {
			return err
		}
		if err := check("lift", l.Name, "product file", l.ProductFiles); err != nil {
			return err
		}
	}
	return nil
}

// checkNames reports the first name the registry lists twice among its
// suites and lifts, owned or used: registration refuses a second column,
// or lift, of one name.
func (r *Registry) checkNames() error {
	kinds := map[string]string{} // name → "suite" or "lift"
	check := func(kind, name string) error {
		switch was, ok := kinds[name]; {
		case !ok:
			kinds[name] = kind
			return nil
		case was == kind:
			return fmt.Errorf("the registry lists the %s %s twice, where each suite and lift a package registers, owned or used, has a name of its own", kind, name)
		default:
			return fmt.Errorf("the registry lists a suite and a lift named %s, where each suite and lift a package registers, owned or used, has a name of its own", name)
		}
	}
	for _, s := range r.Suites {
		if err := check("suite", s.Name); err != nil {
			return err
		}
	}
	for _, l := range r.Lifts {
		if err := check("lift", l.Name); err != nil {
			return err
		}
	}
	return nil
}

// checkCorpora reports the first column, suite or lift, owned or used,
// that the registry lists as running over one corpus twice.
func (r *Registry) checkCorpora() error {
	check := func(kind, name string, corpora []string) error {
		for i, c := range corpora {
			if slices.Contains(corpora[:i], c) {
				return fmt.Errorf("the %s %s runs over testdata/%s/ twice, where a registry lists each corpus a column runs over once", kind, name, c)
			}
		}
		return nil
	}
	for _, s := range r.Suites {
		if err := check("suite", s.Name, s.Corpora); err != nil {
			return err
		}
	}
	for _, l := range r.Lifts {
		if err := check("lift", l.Name, l.Corpora); err != nil {
			return err
		}
	}
	return nil
}

// checkCheckNames reports the first corpus check the registry lists
// twice: registration refuses a second check of one name in a package.
// checkChecksListed reports a registry that gives no list of corpus checks,
// which the update writes, empty where the package registers none.
func (r *Registry) checkChecksListed() error {
	if r.Checks == nil {
		return errors.New("the registry gives no list of corpus checks, where a registry lists each corpus check its package registers, and an empty list for none")
	}
	return nil
}

func (r *Registry) checkCheckNames() error {
	for i, c := range r.Checks {
		if slices.ContainsFunc(r.Checks[:i], func(o Check) bool { return o.Name == c.Name }) {
			return fmt.Errorf("the registry lists the corpus check %s twice, where each corpus check a package registers has a name of its own", c.Name)
		}
	}
	return nil
}

// checkPaths reports the first column, suite or lift, owned or used,
// whose paths the update couldn't have written.
func (r *Registry) checkPaths() error {
	lifts := map[string]bool{}
	for _, l := range r.Lifts {
		lifts[l.Name] = true
	}
	check := func(kind, name string, raw bool, corpora []string, paths []Path) error {
		from := []string{}
		for _, p := range paths {
			from = append(from, p.Corpus)
		}
		if !slices.Equal(from, corpora) {
			return fmt.Errorf("the %s %s runs over %s, and has paths from %s, where a registry gives each column one path from each corpus it runs over, in the same order", kind, name, dirList(corpora), dirList(from))
		}
		for _, p := range paths {
			if raw && len(p.Lifts) > 0 {
				return fmt.Errorf("the %s %s consumes the raw input view, and its path from testdata/%s/ names %s, where a suite over the raw input view passes through no lift", kind, name, p.Corpus, strings.Join(p.Lifts, ", "))
			}
			for i, l := range p.Lifts {
				switch {
				case l == name:
					return fmt.Errorf("the %s %s's path from testdata/%s/ names the %s itself, where a column's path names the lifts before it", kind, name, p.Corpus, kind)
				case !lifts[l]:
					return fmt.Errorf("the %s %s's path from testdata/%s/ names %s, and the registry lists no lift of that name, where every lift on a column's path is one its package registers", kind, name, p.Corpus, l)
				case slices.Contains(p.Lifts[:i], l):
					return fmt.Errorf("the %s %s's path from testdata/%s/ names %s twice, where a column's path names each lift before it once", kind, name, p.Corpus, l)
				}
			}
		}
		return nil
	}
	for _, s := range r.Suites {
		if err := check("suite", s.Name, s.Consumes == RawInput, s.Corpora, s.Paths); err != nil {
			return err
		}
	}
	for _, l := range r.Lifts {
		if err := check("lift", l.Name, false, l.Corpora, l.Paths); err != nil {
			return err
		}
	}
	return nil
}

// dirList names corpus directories, as testdata/<dir>/ each, in prose,
// or none.
func dirList(dirs []string) string {
	if len(dirs) == 0 {
		return "none"
	}
	var names []string
	for _, d := range dirs {
		names = append(names, "testdata/"+d+"/")
	}
	if len(names) == 1 {
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// decode decodes a registry, refusing a key its Registry doesn't hold.
func decode(data []byte) (*Registry, error) {
	var r Registry
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("it is not a registry: %w", err)
	}
	return &r, nil
}

// Oldest is the oldest format version [ParseAny] reads: the one main held
// when it first held a registry. Formats 1 and 2 are only at commits of
// the branch that brought the registry to main, whose merge read neither.
const Oldest = 3

// OutcomeFormat is the first format version whose revisions commit an
// outcome file for every cell. Before it, a cell commits only the files
// its column writes, and none at all when it writes none.
const OutcomeFormat = 5

// InputsFormat is the first format version whose registries record what
// each output declaration's Inputs states, and whether the project
// supplies Positions.
const InputsFormat = 8

// ChecksFormat is the first format version whose registries list the
// corpus checks their packages register. Before it, a reader finds a
// check's findings only by their files' names.
const ChecksFormat = 9

// added is what each format version added to the one before it, by the
// key that holds it, which a registry of an older format doesn't hold.
// Format 4 added each corpus type's transforms, format 5 whether the
// project supplies Codes and each suite's classifier, format 6 whether
// each lift is a bytes lift and its product's declarations, format 7 each
// lift's and suite's path from each of its corpora, format 8 whether the
// project supplies Positions and what each output declaration's Inputs
// states, and format 9 the corpus checks the package registers. None
// changed what an earlier key holds. Each format's keys are in
// testdata/keys-<N>.txt, which TestFormatKeys checks the table against.
var added = []struct {
	format int
	key    string
	holds  func(presence) bool
}{
	{4, "types[].transforms", func(p presence) bool {
		return slices.ContainsFunc(p.Types, func(t typePresence) bool { return t.Transforms != nil })
	}},
	{5, "codes", func(p presence) bool { return p.Codes != nil }},
	{5, "suites[].classifier", func(p presence) bool {
		return slices.ContainsFunc(p.Suites, func(s suitePresence) bool { return s.Classifier != nil })
	}},
	{6, "lifts[].bytes", func(p presence) bool {
		return slices.ContainsFunc(p.Lifts, func(l liftPresence) bool { return l.Bytes != nil })
	}},
	{6, "lifts[].productFiles", func(p presence) bool {
		return slices.ContainsFunc(p.Lifts, func(l liftPresence) bool { return l.ProductFiles != nil })
	}},
	{7, "lifts[].paths", func(p presence) bool {
		return slices.ContainsFunc(p.Lifts, func(l liftPresence) bool { return l.Paths != nil })
	}},
	{7, "suites[].paths", func(p presence) bool {
		return slices.ContainsFunc(p.Suites, func(s suitePresence) bool { return s.Paths != nil })
	}},
	{8, "lifts[].outputs[].inputs", func(p presence) bool {
		return slices.ContainsFunc(p.Lifts, func(l liftPresence) bool { return holdsInputs(l.Outputs) })
	}},
	{8, "lifts[].productFiles[].inputs", func(p presence) bool {
		return slices.ContainsFunc(p.Lifts, func(l liftPresence) bool {
			// productFiles that aren't a list hold no inputs, and decode
			// refuses them.
			var files []outputPresence
			_ = json.Unmarshal(l.ProductFiles, &files)
			return holdsInputs(files)
		})
	}},
	{8, "positions", func(p presence) bool { return p.Positions != nil }},
	{8, "suites[].outputs[].inputs", func(p presence) bool {
		return slices.ContainsFunc(p.Suites, func(s suitePresence) bool { return holdsInputs(s.Outputs) })
	}},
	{9, "checks", func(p presence) bool { return p.Checks != nil }},
}

// presence records which keys that a later format added a registry
// holds. A key given as null is held.
type presence struct {
	Checks    json.RawMessage `json:"checks"`
	Codes     json.RawMessage `json:"codes"`
	Positions json.RawMessage `json:"positions"`
	Types     []typePresence  `json:"types"`
	Lifts     []liftPresence  `json:"lifts"`
	Suites    []suitePresence `json:"suites"`
}

type liftPresence struct {
	Bytes        json.RawMessage  `json:"bytes"`
	Outputs      []outputPresence `json:"outputs"`
	ProductFiles json.RawMessage  `json:"productFiles"`
	Paths        json.RawMessage  `json:"paths"`
}

type typePresence struct {
	Transforms json.RawMessage `json:"transforms"`
}

type suitePresence struct {
	Classifier json.RawMessage  `json:"classifier"`
	Outputs    []outputPresence `json:"outputs"`
	Paths      json.RawMessage  `json:"paths"`
}

type outputPresence struct {
	Inputs json.RawMessage `json:"inputs"`
}

// holdsInputs reports whether any of the output declarations holds inputs.
func holdsInputs(outs []outputPresence) bool {
	return slices.ContainsFunc(outs, func(o outputPresence) bool { return o.Inputs != nil })
}

// ParseAny parses a registry of any format version from [Oldest] to
// [Format], as a tool reads the registries that other revisions committed.
// One of an older format is given in today's form, with its own Format,
// and is refused as no registry if it holds a key a later format added.
// It refuses what Parse would refuse of a registry of its format: each of
// Parse's checks that holds of the format applies, and Parse's checks of
// what a later format added don't. A format older than Oldest or newer
// than Format is a [*FormatError].
func ParseAny(data []byte) (*Registry, error) {
	var head struct {
		Format int `json:"format"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("it is not a registry: %w", err)
	}
	switch {
	case head.Format < 1:
		return nil, errors.New("it is not a registry: it holds no format version")
	case head.Format < Oldest || head.Format > Format:
		return nil, &FormatError{Format: head.Format}
	}
	var p presence
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("it is not a registry: %w", err)
	}
	for _, a := range added {
		if head.Format < a.format && a.holds(p) {
			return nil, fmt.Errorf("it is not a registry: it holds format %d, and %s, which format %d added", head.Format, a.key, a.format)
		}
	}
	r, err := decode(data)
	if err != nil {
		return nil, err
	}
	if err := r.check(); err != nil {
		return nil, err
	}
	return r, nil
}

// RawInput is the type a suite over the raw input view consumes, as
// [Suite.Consumes] names it: the archives of the one directory in its
// Corpora, with no corpus type.
const RawInput = "io/fs.FS"

// A Head is what every format of a registry holds, read without its
// format's version.
type Head struct {
	Format      int    `json:"format"`
	CorpusRoot  string `json:"corpusRoot"`
	RootPackage bool   `json:"rootPackage"`
	// UpdateVar names the project's update variable. It is "" for a
	// registry that holds none, as one of a later format might.
	UpdateVar string `json:"updateVar"`
}

// Peek reads a registry's head, whatever its format, so a reader can tell
// which corpus root a registry is of before it can read the rest. A
// registry of a newer format that names no corpus root is a
// [*FormatError].
func Peek(data []byte) (Head, error) {
	var h struct {
		Format      int     `json:"format"`
		CorpusRoot  *string `json:"corpusRoot"`
		RootPackage bool    `json:"rootPackage"`
		UpdateVar   string  `json:"updateVar"`
	}
	if err := json.Unmarshal(data, &h); err != nil {
		return Head{}, fmt.Errorf("it is not a registry: %w", err)
	}
	switch {
	case h.CorpusRoot == nil && h.Format > Format:
		return Head{}, &FormatError{Format: h.Format}
	case h.CorpusRoot == nil:
		return Head{}, errors.New("it is not a registry: it names no corpus root")
	}
	return Head{Format: h.Format, CorpusRoot: *h.CorpusRoot, RootPackage: h.RootPackage, UpdateVar: h.UpdateVar}, nil
}

// A Found is a registry file under a module root.
type Found struct {
	Path string // the registry file
	Pkg  string // the directory of the package it belongs to
}

// Find returns the registries under a module root, sorted by path. It
// looks in the directories go list ./... looks in, whether or not they hold
// Go files: every directory under the root except those named testdata or
// vendor, those whose names start with . or _, and nested modules.
func Find(moduleRoot string) ([]Found, error) {
	pkgs, err := FindFS(os.DirFS(moduleRoot))
	var found []Found
	for _, p := range pkgs {
		dir := filepath.Join(moduleRoot, filepath.FromSlash(p))
		found = append(found, Found{Path: filepath.Join(dir, "testdata", Name), Pkg: dir})
	}
	return found, err
}

// FindFS is [Find] over a file system whose root is the module root, such
// as a revision's tree. It returns the directory of each registry's
// package, as a slash path: "." for the module root.
func FindFS(fsys fs.FS) ([]string, error) {
	var pkgs []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if p != "." {
			name := d.Name()
			if name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return fs.SkipDir
			}
			if _, err := fs.Stat(fsys, path.Join(p, "go.mod")); err == nil {
				return fs.SkipDir
			}
		}
		reg := path.Join(p, "testdata", Name)
		if fi, err := fs.Stat(fsys, reg); err == nil && fi.Mode().IsRegular() {
			pkgs = append(pkgs, p)
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	})
	return pkgs, err
}
