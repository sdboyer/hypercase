package sheetcorpus_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step10/sheetcorpus"
)

// throwaway, set, makes this test binary TestImportFault's child: a run of
// the framework over the throwaway root the parent made.
const throwaway = "SHEETS_STEP10_THROWAWAY"

func init() {
	if os.Getenv(throwaway) == "" {
		return
	}
	// No root-package check: in an update it only says it runs in normal
	// runs, and the child's output is to be import's fault alone.
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("."),
		sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling, sheetcorpus.Unterminated,
		sheetcorpus.SheetType, sheetcorpus.CSVType,
		hypercase.Own(sheetcorpus.Import),
		hypercase.Own(sheetcorpus.Looped, sheetcorpus.Renamed),
	)
}

// TestImportFault: a CSV archive with two CSVs fails import's cell as a
// fault, not a rejection, and an update writes nothing for it. The test
// runs an update over a throwaway root, holding that one archive, in a
// child of this test binary.
func TestImportFault(t *testing.T) {
	if os.Getenv(throwaway) != "" {
		hypercase.Test(t)
		return
	}
	dir := t.TempDir()
	two := "Two CSVs.\n-- in/csv/a.csv --\n4\n-- in/csv/b.csv --\n6\n"
	for name, data := range map[string]string{
		"go.mod":                 "module example.com/throwaway\n\ngo 1.27\n",
		"testdata/csv/two.txtar": two,
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The child starts by the binary's absolute path: a relative argv[0]
	// would resolve in dir.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestImportFault$", "-test.count=1")
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); k != throwaway && k != sheetcorpus.Project.UpdateVar && k != "PWD" {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, throwaway+"=1", sheetcorpus.Project.UpdateVar+"=1")
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	switch {
	case err == nil:
		t.Fatalf("the update passed:\n%s", out)
	case !errors.As(err, &exit):
		t.Fatalf("running the update: %v\n%s", err, out)
	}
	// The child fails import's cell, and the tests above it, and nothing
	// else: any other failure adds lines. Durations and file:line prefixes
	// are dropped, so the output reads the same wherever the code moves, and
	// so is the coverage line a coverage-built binary prints, as the child
	// is whenever this test's binary is.
	got := positions.ReplaceAllString(string(out), "$1")
	got = durations.ReplaceAllString(got, "$1")
	got = coverage.ReplaceAllString(got, "")
	if got != importFault {
		t.Errorf("the update's output:\n%s\nwant:\n%s", got, importFault)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "testdata", "csv", "two.txtar")); err != nil {
		t.Error(err)
	} else if string(got) != two {
		t.Errorf("the update wrote the archive:\n%s", got)
	}
}

var (
	durations = regexp.MustCompile(`(?m)^( *--- \w+: \S+) \(\d+\.\d+s\)$`)
	positions = regexp.MustCompile(`(?m)^( *)[\w.-]+\.go:\d+: `)
	coverage  = regexp.MustCompile(`(?m)^coverage: .*\n`)
)

// importFault is the child's output, whose one failure is import's cell.
const importFault = `--- FAIL: TestImportFault
    --- FAIL: TestImportFault/csv
        --- FAIL: TestImportFault/csv/two
            --- FAIL: TestImportFault/csv/two/import
                csv/two: Two CSVs.
                    import: Turns each CSV field into a cell: row 1, field 2 is B1.
                    import's jenny failed on the case, with an error that is no rejection:
                      import: the input holds ["csv/a.csv" "csv/b.csv"], and a CSV export is one file, csv/<name>.csv, with a name that isn't empty or only white space
                    If import rejects the case, return the error as decl.Reject(err). If not, fix import's jenny. An update writes nothing for this cell.
FAIL
`
