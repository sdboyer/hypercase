package hypercase

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/diff"
	"github.com/sdboyer/hypercase/internal/registry"
)

// ownRegistry is what the package registered, as its registry records it.
func (r *runner) ownRegistry() registry.Registry {
	g := registry.Registry{
		UpdateVar:   r.updateVar,
		CorpusRoot:  r.rootRel,
		RootPackage: r.rootPackage,
	}
	for _, l := range r.issueLinks {
		g.IssueLinks = append(g.IssueLinks, registry.IssueLink{Pattern: l.Pattern, URL: l.URL})
	}
	for _, p := range r.properties {
		g.Properties = append(g.Properties, registry.Property{Name: p.Name, Description: p.Description})
	}
	for _, c := range r.columns {
		s := registry.Suite{Name: c.name, Description: c.description, Corpus: c.corpus}
		for _, o := range c.outputs {
			s.Outputs = append(s.Outputs, registry.Output{Pattern: o.Pattern, Description: o.Description})
		}
		for _, p := range c.rejects {
			s.Rejects = append(s.Rejects, p.Name)
		}
		g.Suites = append(g.Suites, s)
	}
	return g
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
