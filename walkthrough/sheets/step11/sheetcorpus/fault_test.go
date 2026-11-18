package sheetcorpus_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step11/sheetcorpus"
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
		sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling,
		sheetcorpus.Unterminated, sheetcorpus.StrayQuote, sheetcorpus.OutOfRange,
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
	// The child's environment is these two variables alone. It needs no
	// other, and any other could change the output compared below, as
	// GODEBUG=inittrace=1 does.
	cmd.Env = []string{throwaway + "=1", sheetcorpus.Project.UpdateVar + "=1"}
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
	if got := strip(string(out)); got != importFault {
		t.Errorf("the update's output:\n%s\nwant:\n%s", got, importFault)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "testdata", "csv", "two.txtar")); err != nil {
		t.Error(err)
	} else if string(got) != two {
		t.Errorf("the update wrote the archive:\n%s", got)
	}
}

// TestStrip: strip drops a duration, a file:line prefix and a coverage line
// only where the testing package writes them, so a message that holds one
// reads as written.
func TestStrip(t *testing.T) {
	out := `--- FAIL: TestA (0.01s)
    --- FAIL: TestA/b (0.00s)
        fault_test.go:42: a message that quotes x.go:1: and ends --- FAIL: TestC (0.00s)
        --- FAIL: TestD (0.00s) is a message too
        a message that ends coverage: 1.0% of statements
FAIL
coverage: 50.0% of statements
`
	want := `--- FAIL: TestA
    --- FAIL: TestA/b
        a message that quotes x.go:1: and ends --- FAIL: TestC (0.00s)
        --- FAIL: TestD (0.00s) is a message too
        a message that ends coverage: 1.0% of statements
FAIL
`
	if got := strip(out); got != want {
		t.Errorf("strip gave:\n%s\nwant:\n%s", got, want)
	}
}

// strip drops from a test binary's output each duration of a test's result
// line, each file:line prefix of a message, and the coverage line.
func strip(out string) string {
	out = positions.ReplaceAllString(out, "$1")
	out = durations.ReplaceAllString(out, "$1")
	return coverage.ReplaceAllString(out, "")
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
                      import: the input holds ["csv/a.csv" "csv/b.csv"], and a CSV export is one file, csv/<name>.csv, with a name holding a character that is neither white space nor a format character
                    If import rejects the case, return the error as decl.Reject(err). If not, fix import's jenny. An update writes nothing for this cell.
FAIL
`
