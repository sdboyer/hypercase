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
	"path/filepath"
	"slices"
	"strings"
)

// Format is the version of what a registry holds. It changes on any change
// to that.
const Format = 2

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

// A Suite is a registered suite.
type Suite struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Corpus is the directory, under the corpus root's testdata/, of the
	// archives the suite runs over.
	Corpus  string   `json:"corpus"`
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
	suites := sorted(r.Suites, func(s Suite) string { return s.Name })
	for i, s := range suites {
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
	var found []Found
	err := filepath.WalkDir(moduleRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if p != moduleRoot {
			name := d.Name()
			if name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
				return filepath.SkipDir
			}
		}
		reg := filepath.Join(p, "testdata", Name)
		if fi, err := os.Stat(reg); err == nil && fi.Mode().IsRegular() {
			found = append(found, Found{Path: reg, Pkg: p})
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	})
	return found, err
}
