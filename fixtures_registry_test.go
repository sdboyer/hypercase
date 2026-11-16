package hypercase_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/sdboyer/hypercase/internal/diff"
)

// TestFixtureRegistries checks each fixture package's committed registry
// against what its registration writes, so a test fails only on what it
// sets out to change. HYPERCASE_UPDATE_GOLDEN=1 writes them.
func TestFixtureRegistries(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(filepath.Join("testdata", "fixtures"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		pkgs, ok := fixturePackages[e.Name()]
		if !ok {
			t.Errorf("testdata/fixtures/%s is not in fixturePackages. Add it, with its packages and their registrations.", e.Name())
			continue
		}
		for _, p := range pkgs {
			t.Run(e.Name()+"/"+p.dir, func(t *testing.T) {
				t.Parallel()
				s := newSandbox(t, e.Name())
				reg := filepath.Join(p.dir, "testdata", "hypercase.json")
				if err := os.Remove(s.path(reg)); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				s.run(p.dir, p.registration, true)
				got := s.read(reg)
				committed := filepath.Join("testdata", "fixtures", e.Name(), reg)
				if os.Getenv(transcriptsVar) == "1" {
					if err := os.WriteFile(committed, got, 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				want, err := os.ReadFile(committed)
				if err != nil {
					t.Fatalf("%v\nRun %s=1 go test ./... to write it.", err, transcriptsVar)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("%s differs from what %s registers (-committed +registered):\n%s\nRun %s=1 go test ./... to write it.", committed, p.registration, diff.Lines(want, got), transcriptsVar)
				}
			})
		}
	}
}
