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
const Format = 3

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
	// IssueLinks are the project's link patterns, which link text naming an
	// issue to the issue.
	IssueLinks []IssueLink `json:"issueLinks"`
	Properties []Property  `json:"properties"`
	Types      []Type      `json:"types"`
	Lifts      []Lift      `json:"lifts"`
	Suites     []Suite     `json:"suites"`
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
	GoType string `json:"goType"`
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
	// Corpora are the directories of the corpus types that reach the
	// lift's input, under the corpus root's testdata/.
	Corpora []string `json:"corpora"`
	Outputs []Output `json:"outputs"`
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
	Outputs []Output `json:"outputs"`
	// Rejects names the properties the suite rejects.
	Rejects []string `json:"rejects"`
}

// An Output is one of a column's output declarations.
type Output struct {
	Pattern     string `json:"pattern"`
	Description string `json:"description"`
}

// Marshal returns the registry's bytes: indented JSON, ending with a
// newline, with every list sorted, so the bytes depend only on what was
// registered.
func (r Registry) Marshal() []byte {
	r.Format = Format
	r.IssueLinks = sorted(r.IssueLinks, func(l IssueLink) string { return l.Pattern + "\x00" + l.URL })
	r.Properties = sorted(r.Properties, func(p Property) string { return p.Name })
	r.Types = sorted(r.Types, func(t Type) string { return t.Dir })
	lifts := sorted(r.Lifts, func(l Lift) string { return l.Name })
	for i, l := range lifts {
		l.Corpora = sorted(l.Corpora, func(n string) string { return n })
		l.Outputs = sorted(l.Outputs, func(o Output) string { return o.Pattern })
		l.Rejects = sorted(l.Rejects, func(n string) string { return n })
		lifts[i] = l
	}
	r.Lifts = lifts
	suites := sorted(r.Suites, func(s Suite) string { return s.Name })
	for i, s := range suites {
		s.Corpora = sorted(s.Corpora, func(n string) string { return n })
		s.Outputs = sorted(s.Outputs, func(o Output) string { return o.Pattern })
		s.Rejects = sorted(s.Rejects, func(n string) string { return n })
		suites[i] = s
	}
	r.Suites = suites
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

// A FormatError is a registry of a format version this package doesn't
// read.
type FormatError struct{ Format int }

func (e *FormatError) Error() string {
	return fmt.Sprintf("it holds format %d, and this hypercase reads format %d", e.Format, Format)
}

// Parse parses a registry. One of another format version is a
// [*FormatError].
func Parse(data []byte) (*Registry, error) {
	var head struct {
		Format int `json:"format"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("it is not a registry: %w", err)
	}
	if head.Format != Format {
		return nil, &FormatError{Format: head.Format}
	}
	var r Registry
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("it is not a registry: %w", err)
	}
	return &r, nil
}

// ParseAny parses a registry of any format version this package reads,
// from 1 to [Format], as a tool reads registries that other revisions
// committed. An older format is given in today's form, with its own
// Format. A newer one is a [*FormatError].
//
// Formats 1 and 2 had no corpus types and no lifts: a suite named its
// corpus, a directory, and ran over its archives' raw input views. Such a
// suite is given as today's suite over the raw input view, which consumes
// [RawInput] over that one directory. Two lines of development each wrote
// a format 2, one adding the issue links to format 1 and one the corpus
// types and lifts; ParseAny reads both.
func ParseAny(data []byte) (*Registry, error) {
	var head map[string]json.RawMessage
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("it is not a registry: %w", err)
	}
	var format int
	if err := json.Unmarshal(head["format"], &format); err != nil {
		return nil, fmt.Errorf("it is not a registry: its format is no version number")
	}
	switch {
	case format > Format:
		return nil, &FormatError{Format: format}
	case format == Format:
		return Parse(data)
	case format == 1:
		var r v1
		if err := decodeStrict(data, &r); err != nil {
			return nil, err
		}
		return v2Links{Format: r.Format, UpdateVar: r.UpdateVar, CorpusRoot: r.CorpusRoot, RootPackage: r.RootPackage, Properties: r.Properties, Suites: r.Suites}.registry(), nil
	case format == 2 && head["issueLinks"] != nil:
		var r v2Links
		if err := decodeStrict(data, &r); err != nil {
			return nil, err
		}
		return r.registry(), nil
	case format == 2:
		var r v2Types
		if err := decodeStrict(data, &r); err != nil {
			return nil, err
		}
		g := Registry(r)
		return &g, nil
	}
	return nil, fmt.Errorf("it is not a registry: format %d is no version of the registry's format", format)
}

// RawInput is the type a suite over the raw input view consumes, as
// [Suite.Consumes] names it: the archives of the one directory in its
// Corpora, with no corpus type.
const RawInput = "io/fs.FS"

// v1 is format 1: no issue links, no corpus types and no lifts.
type v1 struct {
	Format      int        `json:"format"`
	UpdateVar   string     `json:"updateVar"`
	CorpusRoot  string     `json:"corpusRoot"`
	RootPackage bool       `json:"rootPackage"`
	Properties  []Property `json:"properties"`
	Suites      []v1Suite  `json:"suites"`
}

// v2Links is the format 2 that added the issue links to format 1.
type v2Links struct {
	Format      int         `json:"format"`
	UpdateVar   string      `json:"updateVar"`
	CorpusRoot  string      `json:"corpusRoot"`
	RootPackage bool        `json:"rootPackage"`
	IssueLinks  []IssueLink `json:"issueLinks"`
	Properties  []Property  `json:"properties"`
	Suites      []v1Suite   `json:"suites"`
}

// v2Types is the format 2 that added the corpus types and lifts to format
// 1: format 3 without the issue links.
type v2Types struct {
	Format      int         `json:"format"`
	UpdateVar   string      `json:"updateVar"`
	CorpusRoot  string      `json:"corpusRoot"`
	RootPackage bool        `json:"rootPackage"`
	IssueLinks  []IssueLink `json:"-"`
	Properties  []Property  `json:"properties"`
	Types       []Type      `json:"types"`
	Lifts       []Lift      `json:"lifts"`
	Suites      []Suite     `json:"suites"`
}

// v1Suite is a suite of formats 1 and 2, which named its corpus.
type v1Suite struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Corpus      string   `json:"corpus"`
	Outputs     []Output `json:"outputs"`
	Rejects     []string `json:"rejects"`
}

// registry gives the registry in today's form.
func (r v2Links) registry() *Registry {
	g := &Registry{
		Format:      r.Format,
		UpdateVar:   r.UpdateVar,
		CorpusRoot:  r.CorpusRoot,
		RootPackage: r.RootPackage,
		IssueLinks:  r.IssueLinks,
		Properties:  r.Properties,
	}
	for _, s := range r.Suites {
		g.Suites = append(g.Suites, Suite{
			Name:        s.Name,
			Description: s.Description,
			Consumes:    RawInput,
			Corpora:     []string{s.Corpus},
			Outputs:     s.Outputs,
			Rejects:     s.Rejects,
		})
	}
	return g
}

func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("it is not a registry: %w", err)
	}
	return nil
}

// A Head is what every format of a registry holds, read without its
// format's version.
type Head struct {
	Format      int    `json:"format"`
	CorpusRoot  string `json:"corpusRoot"`
	RootPackage bool   `json:"rootPackage"`
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
	return Head{Format: h.Format, CorpusRoot: *h.CorpusRoot, RootPackage: h.RootPackage}, nil
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
