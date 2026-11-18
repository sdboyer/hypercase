package hypercase

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/diff"
	"github.com/sdboyer/hypercase/internal/registry"
)

// ownRegistry is what the package registered, as its registry records it.
func (r *runner) ownRegistry() registry.Registry {
	g := registry.Registry{
		UpdateVar:   r.updateVar,
		CorpusRoot:  r.rootRel,
		RootPackage: r.rootPackage,
		Codes:       r.codes != nil,
		Positions:   r.positions != nil,
	}
	for _, l := range r.issueLinks {
		g.IssueLinks = append(g.IssueLinks, registry.IssueLink{Pattern: l.Pattern, URL: l.URL})
	}
	for _, p := range r.properties {
		g.Properties = append(g.Properties, registry.Property{Name: p.Name, Description: p.Description})
	}
	for _, ct := range r.types {
		rt := registry.Type{Dir: ct.dir, Description: ct.description, GoType: goTypeName(ct.goType)}
		for _, tr := range ct.transforms {
			rt.Transforms = append(rt.Transforms, registry.Transform{
				Name:        tr.name,
				Description: tr.description,
				Gives:       propertyNames(tr.gives),
				Keeps:       propertyNames(tr.keeps),
				Probe:       tr.probe != nil,
				Owned:       r.owned[tr],
			})
		}
		g.Types = append(g.Types, rt)
	}
	for _, l := range r.lifts {
		g.Lifts = append(g.Lifts, registry.Lift{
			Name:         l.name,
			Description:  l.description,
			Input:        goTypeName(l.in),
			Product:      goTypeName(l.out),
			Owned:        l.owned,
			Bytes:        l.bytes,
			Corpora:      r.corpora[l.name],
			Paths:        r.paths[l.name],
			Outputs:      registryOutputs(l.outputs),
			ProductFiles: registryOutputs(l.product),
			Rejects:      propertyNames(l.rejects),
		})
	}
	for _, c := range r.columns {
		if c.lift != nil {
			continue
		}
		consumes := goTypeName(c.consumes)
		if c.corpus != "" {
			consumes = registry.RawInput
		}
		s := registry.Suite{
			Name:        c.name,
			Description: c.description,
			Consumes:    consumes,
			Corpora:     r.corpora[c.name],
			Paths:       r.paths[c.name],
			Outputs:     registryOutputs(c.outputs),
			Rejects:     propertyNames(c.rejects),
		}
		if cl := c.classifier; cl != nil {
			s.Classifier = &registry.Classifier{Name: cl.name, Description: cl.description}
		}
		g.Suites = append(g.Suites, s)
	}
	return g
}

func registryOutputs(os []decl.Output) []registry.Output {
	var out []registry.Output
	for _, o := range os {
		out = append(out, registry.Output{Pattern: o.Pattern, Description: o.Description, Inputs: recordedInputs[stated(o.Inputs)]})
	}
	return out
}

// recordedInputs is what a registry records of each statement an Inputs
// makes. Registration refuses statesOther.
var recordedInputs = map[statement]string{
	statesNone:      registry.InputsNone,
	statesFunc:      registry.InputsFunc,
	statesAll:       registry.InputsAll,
	statesPositions: registry.InputsPositions,
}

func propertyNames(ps []decl.Property) []string {
	var names []string
	for _, p := range ps {
		names = append(names, p.Name)
	}
	return names
}

// goTypeName names a Go type by its package's import path and its name, so
// two packages' types of one name differ.
func goTypeName(t reflect.Type) string {
	switch {
	case t.Name() != "" && t.PkgPath() != "":
		return t.PkgPath() + "." + t.Name()
	case t.Kind() == reflect.Pointer:
		return "*" + goTypeName(t.Elem())
	case t.Kind() == reflect.Slice:
		return "[]" + goTypeName(t.Elem())
	}
	return t.String()
}

// checkRegistry compares the package's registry, testdata/hypercase.json,
// with what it registered, like a golden, and an update writes it. A temp
// file beside it, left by a write that was cut short, fails every run.
func (r *runner) checkRegistry(t *testing.T) {
	t.Helper()
	dir := filepath.Join(r.pkgDir, "testdata")
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("reading %s: %v", r.rel(dir), err)
	}
	for _, e := range entries {
		if archive.IsTemp(e.Name()) {
			t.Errorf("%s is a temp file left by a write that was cut short.\nDelete it.", r.rel(filepath.Join(dir, e.Name())))
		}
	}
	p := filepath.Join(dir, registry.Name)
	want := r.ownRegistry().Marshal()
	got, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("reading %s: %v", r.rel(p), err)
		return
	}
	if r.update {
		if err == nil && bytes.Equal(got, want) {
			return
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Errorf("writing %s: %v", r.rel(p), err)
			return
		}
		if err := archive.WriteFile(p, want); err != nil {
			t.Errorf("writing %s: %v", r.rel(p), err)
		}
		return
	}
	switch {
	case err != nil:
		t.Errorf("%s is missing. It records what the package registers, for tools and for the root-package check.\nRun %s to write it.", r.rel(p), r.updateCmd())
	case !bytes.Equal(got, want):
		t.Errorf("%s differs from what the package registers (-committed +registered):\n%s\nRun %s to write it, and read the diff before committing.", r.rel(p), strings.TrimSuffix(diff.Lines(got, want), "\n"), r.updateCmd())
	}
}
