# Walkthrough

This walkthrough is hypercase's spec. It starts at a table test, which Go
developers already write, and adds one idea per step. It ends at hypotheses,
claims that relate many runs to each other, whose design is not settled.

A reader can stop at any step and use what it taught. Each step's idea is
useful without the later ones, and hypercase is worth having even if step 12
never pans out.

hypercase is one framework, with everything on. It has no levels to opt into.
A project that upgrades to a later commit carries what later steps add: an
`outcome` file per cell (step 8), and `Inputs` on every output declaration,
with an `inputs` file per cell that runs (step 11). Earlier steps' examples
keep running, and gain what later steps require.

Every step is shown in one toy domain, spreadsheet formulas, which step 1
introduces. Steps 1 to 11 show the Go an author writes at that
step, against the API the step adds. A name or format marked "proposed" is not
settled yet.

## 1. A table test

### Where a Go developer already is

A table test is a slice of rows, each with a name, an input and a want, and a
loop that runs each row with `t.Run`. The wants are written in Go. No
framework is involved.

### The toy

The toy is a small Go project (proposed) whose domain is spreadsheet
formulas. A sheet is a directory with one file per cell, named for the cell
(`A1`, `B1`), holding a number or a formula such as `=B1*2`. The package
`sheet` parses a sheet, resolves its references, evaluates it, prints its
formulas back, and imports CSV. Its command, `calc`, prints a sheet's values.
The domain package knows nothing of hypercase.

It lives under `walkthrough/` (proposed):

```
walkthrough/
  sheets/               a module: the sheets project
    go.mod
    sheet/              the domain package
    cmd/calc/
    step01/ … step11/   one example package per step
```

- The toy is a module of its own. Its `go.mod` requires hypercase through
  `replace github.com/sdboyer/hypercase => ../..`, so it builds against the
  checkout, and its dependencies stay out of hypercase's `go.mod`. CI runs
  `go test ./...` at the repository root and at the toy's root.
- Each step's example is a package of its own. From step 3, each step's
  directory is also that step's corpus root, so no two steps' cases meet.
- Trade-off: a module of its own keeps the toy's dependencies out of
  hypercase's `go.mod`, at the cost of a second `go test` run in CI. Through
  the `replace`, the toy builds only against the checkout, never a release.

### What the author writes

<!-- source: walkthrough/sheets/step01/evaluate_test.go -->
```go
// walkthrough/sheets/step01/evaluate_test.go
package step01

import (
	"maps"
	"testing"

	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
)

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name  string
		cells map[string]string
		want  sheet.Values
	}{
		{
			name:  "twice",
			cells: map[string]string{"A1": "=B1*2", "B1": "3"},
			want:  sheet.Values{"A1": 6, "B1": 3},
		},
		{
			name:  "prices",
			cells: map[string]string{"A1": "4", "B1": "6", "C1": "=A1+B1"},
			want:  sheet.Values{"A1": 4, "B1": 6, "C1": 10},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sheet.EvalCells(tt.cells)
			if err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
```

### How the files change

The example is one test file, `walkthrough/sheets/step01/evaluate_test.go`.

### What can now be seen

Pass or fail per named row. `go test -run 'TestEvaluate/twice' ./step01` runs
one row.

## 2. Goldens

### What step 1 leaves

Wants written in Go become unreadable once outputs grow, and a change of
behavior means editing them row by row.

### What the author writes

Each row's want moves to a file, a **golden**, under the example's
`testdata/`. The test compares each output with its golden byte for byte, and
an update switch rewrites the goldens. Step 2's switch is Go's conventional
`-update` flag (proposed). Trade-off: the flag sets up the problem that
step 3's variable solves, at the cost of teaching a switch the reader then
drops.

The test's rows keep their cells and lose their wants, and each row's output
is `values.Format()`, one line per cell:

<!-- source: walkthrough/sheets/step02/evaluate_test.go -->
```go
// walkthrough/sheets/step02/evaluate_test.go
package step02

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name  string
		cells map[string]string
	}{
		{"twice", map[string]string{"A1": "=B1*2", "B1": "3"}},
		{"prices", map[string]string{"A1": "4", "B1": "6", "C1": "=A1+B1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, err := sheet.EvalCells(tt.cells)
			if err != nil {
				t.Fatal(err)
			}
			got := values.Format()
			golden := filepath.Join("testdata", tt.name+".golden")
			if *update {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s differs:\ngot:\n%s\nwant:\n%s\n"+
					"If the change is intended, run go test ./step02 -update and read the diff.",
					golden, got, want)
			}
		})
	}
}
```

### How the files change

The wants leave the table, and each row has a golden: `testdata/twice.golden`
and `testdata/prices.golden`. `go test ./step02 -update` writes them.
`testdata/twice.golden` holds each cell's value, in cell order (row by row,
then column by column):

<!-- source: walkthrough/sheets/step02/testdata/twice.golden -->
```
A1 6
B1 3
```

### What can now be seen

A change of behavior is a diff of files a reviewer can read, and accepting it
is one command.

## 3. Archives

### What step 2 leaves

Step 2 leaves a case's input in the table and its goldens in files of their
own, tied together only by names. And the flag covers one package. In a
module, `go test ./... -update` fails with
`flag provided but not defined: -update` in every package whose tests don't
define the flag, such as `step01`.

### What the author writes

A **case** is one input to the code under test. An **archive** is one
[txtar](https://pkg.go.dev/golang.org/x/tools/txtar) file holding one case:
its description, its inputs and its goldens. A **corpus** is a directory of
archives, `testdata/<corpus>/` under the **corpus root**. A **suite** is the
Go that answers every archive of a corpus: its run reads a case's inputs and
returns the outputs to compare.

Proposed names: `hypercase.Suite` with its `Name`, `Description`, `Corpus`,
`Outputs` and `Run` fields; `hypercase.File`; `decl.Output`;
`hypercase.Register`; `hypercase.Root`, the option that passes a corpus root
as a path relative to the module root; `hypercase.RootPackage`, which
registers the root-package check (step 5); `hypercase.Test`; and
`decl.Project` with its `UpdateVar` field. `decl` is hypercase's package for
declarations. It doesn't import `testing`, so a project's ordinary packages
can hold what they declare. Trade-off: a project's commands can use its
declarations, at the cost of a second import in every test file.

A suite's type argument is the type it consumes. Through step 5 it is
`fs.FS`, the case's input view, and the suite names its corpus as `Corpus`:
`sheet`, for the archives under `testdata/sheet/`. Step 6 adds the other
types a suite can consume.

The corpus root is the module root unless a package passes another with
`hypercase.Root`. The walkthrough's examples pass their own directory, so
each step's corpus stands alone. A project with one corpus at its module
root passes nothing. One package per corpus root registers
`hypercase.RootPackage()`.

<!-- source: walkthrough/sheets/step03/sheetcorpus/project.go -->
```go
// walkthrough/sheets/step03/sheetcorpus/project.go
package sheetcorpus

import "github.com/sdboyer/hypercase/decl"

var Project = decl.Project{
	UpdateVar: "SHEETS_UPDATE_GOLDEN",
}
```

<!-- source: walkthrough/sheets/step03/evaluate_test.go -->
```go
// walkthrough/sheets/step03/evaluate_test.go
package step03

import (
	"io"
	"io/fs"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step03/sheetcorpus"
)

func init() {
	hypercase.Register(sheetcorpus.Project, hypercase.Root("step03"), hypercase.RootPackage(), evaluate)
}

var evaluate = hypercase.Suite[fs.FS]{
	Name:        "evaluate",
	Description: "Evaluates every cell of the sheet.",
	Corpus:      "sheet",
	Outputs: []decl.Output{
		{Pattern: "values.txt", Description: "Each cell's value, one per line, in cell order.", Inputs: sheetcorpus.Cells},
	},
	Run: func(t testing.TB, archive string, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
		values, err := sheet.Eval(in) // the cells under sheet/
		if err != nil {
			return nil, err
		}
		return []hypercase.File{{Path: "values.txt", Data: values.Format()}}, nil
	},
}

func TestCorpus(t *testing.T) { hypercase.Test(t) }
```

Each output declaration also states `Inputs`: where in the case's input view
its files come from. Step 11 explains it, and the steps before it show it
without comment.

### How the files change

Each table row and its golden become one archive of the corpus `sheet`.
`testdata/sheet/twice.txtar`:

<!-- source: walkthrough/sheets/step03/testdata/sheet/twice.txtar -->
```
A1 is twice B1.
-- in/sheet/A1 --
=B1*2
-- in/sheet/B1 --
3
-- out/evaluate/inputs --
values.txt
  in/sheet/A1
  in/sheet/B1
-- out/evaluate/outcome --
accepted
-- out/evaluate/values.txt --
A1 6
B1 3
```

`out/evaluate/outcome` holds the cell's outcome, the code's verdict on the
case, which step 8 adds to every cell. `out/evaluate/inputs` holds where
each output file comes from, the sections of the archive its declaration's
`Inputs` maps it to, which step 11 adds to every cell that runs.

The package also writes its registry, `testdata/hypercase.json`, which lists
what it registers (step 5).

The project tells git that archives are not text, so no checkout rewrites
their line endings. Its `.gitattributes` holds:

<!-- source: walkthrough/sheets/.gitattributes -->
```
*.txtar -text
```

Its `.gitignore` names the corpus root's lock, which an update takes
(step 5):

<!-- source: walkthrough/sheets/.gitignore -->
```
.hypercase.lock
```

### Rules

**The archive.**

- A case is one archive. Its file name, less `.txtar`, is the archive's
  name. A corpus's archives are `testdata/<corpus>/*.txtar` under the corpus
  root.
- The archive's **comment**, the text before its first `-- name --` line, is
  the case's description, in prose. It is required. It holds no `key: value`
  lines: nothing in it names the case's type or sets a parameter.
- `in/<tree>/…` holds the case's named input trees, such as `in/sheet/`. The
  run sees one **input view**: everything under `in/`, with the prefix
  removed, as an `fs.FS`. `in/sheet/A1` is `sheet/A1` in the view.
- The top level of an archive is the framework's, a closed set: `in`, `out`,
  `via`, `properties`, `gaps`, `variant` and `transform`. Any other top-level
  name fails the run, so a typo such as `ni/sheet/A1` can't pass
  unread. Later steps give most of these names their meaning. `params` is
  reserved as a tree name under `in/` (step 6), and `checks` as a directory
  name under `testdata/` (step 8).

**The suite.**

- A suite has a name and a description, and its outputs are under
  `out/<suite>/…`. A **column** is something every case runs through, and a
  **cell** is one case run through one column. Cases and columns make a
  **matrix**, and each suite is a column. A package may register several
  suites over one corpus; step 3's example registers one, so its matrix has
  one column. Each column's outputs sit under its own `out/<suite>/`, so the
  archive's layout holds any number of them.
- An archive carries no parameters. What the suite does is fixed in Go and
  said in its description, and so is anything it could vary (step 6 states
  the rule).
- Its run receives a `testing.TB`, the archive's name, the input view, and
  one `io.Writer` each for stdout and stderr, and returns its output files
  and an error. It reports its own failures through the `testing.TB`, such
  as a fixture it can't set up. The error it returns is only what the code
  under test rejected.
- What the run writes to stdout and stderr is compared as the `stdout.txt`
  and `stderr.txt` goldens. Each of those files exists when the run wrote to
  its stream. The process's `os.Stdout` is never replaced.
- At this step an error fails its case; step 4 lets a case expect one. The
  error is recorded as `out/<suite>/error.txt`, its text with a final
  newline, when the suite declares `error.txt` (below), and an update writes
  it there though the cell still fails. Step 3's example declares none,
  since none of its cases errs: an error there also fails as an undeclared
  output, and an update writes nothing for its cell. When the run errs, the
  outputs it did produce are still compared.
- A cell an update writes nothing for fails on its problems first. A normal
  run doesn't ask for the golden a file of it lacks, which no update would
  write, and a file that differs from its golden says the update writes
  nothing until those problems are fixed.
- `error.txt`, `stdout.txt` and `stderr.txt` directly under a column's
  directory are the framework's, and so are the names later steps add there:
  `outcome` (step 8) and `inputs` (step 11). A run that returns an output at
  one of them fails its case.
- Making output deterministic, such as paths, times or addresses, is the
  run's job. The framework normalizes nothing.
- The suite is registered when its package is initialized, before any test
  runs. A test function runs what was registered, and registers nothing.
  Step 5 states the rule for everything a registry lists.

**Declared outputs.**

- Every output is declared with a description, `stdout.txt`, `stderr.txt`
  and `error.txt` included. An undeclared output fails the cell.
- A declaration's pattern matches paths under the column's directory. A file
  that two declarations match fails the cell, naming both.

**Comparison.**

- Each output is compared with the golden section of the same name, byte for
  byte. A missing final newline is a difference, and so is a line ending.
- An output with no golden fails, and an update writes it.

**Updates.**

- Updates use one environment variable, whose name the project supplies as
  `UpdateVar`. One variable covers every corpus in the project.
- An update is the whole module's: `SHEETS_UPDATE_GOLDEN=1 go test ./...`,
  at the module root. Every message and every instruction that tells a person
  to update gives that command.

**Who writes a section.** It can be read from the section's name alone.

| Hand-written, never touched by an update | Update-written |
|---|---|
| the comment, `in/**`, `properties`, `gaps` | `out/**`, `via/**` |

- Step 7 adds the variants' sections.
- No update changes `in/…`. The snapshot primitives (#12), which rewrite a
  case's inputs from a project's own code, refuse to run during an update.

**Section order.** One total rule places every section.

- For the base, the case the archive's `in/` holds: its hand-written
  sections, in the order the person wrote them; then its update-written
  sections, `via/…` then `out/…`, each sorted by name. Step 7 extends the
  rule to variants.
- A case's hand-written sections sit together. A run fails on an archive
  where they don't.
- Where an update-written section goes depends only on names and on the
  hand-written sections, never on where it sat.
- A normal run fails on an archive whose update-written sections are
  anywhere else, and says to run the update. An update puts every
  update-written section in its place. That may move another package's
  section (step 5); it never changes that section's bytes.

**Orphans.**

- An **output** is a file under `out/`. An update never deletes one. An
  output the run no longer produces is an **orphan**: it fails every run,
  updates included, until a person removes it.
- What other cells read as input is not an output: a transform's inputs
  (step 7) and a lift's product under `via/` (step 10). An update replaces
  it whole.

**Regeneration.**

- What an update writes to a cell depends only on the inputs and the code,
  never on committed outputs. Committed outputs decide whether a run fails,
  and, from step 4, that a frozen cell is left alone.
- `hypercase strip`, from `cmd/hypercase` (step 9), removes update-written
  sections, which it tells from hand-written ones by name alone. Every
  hand-written section keeps its bytes, and the sections left keep their
  order. A person runs it, never an update. It strips each archive under
  the corpus root's lock (step 5), and writes it by temp file and rename.
- With no flags it strips every archive under the corpus root, and removes
  the registries that record the root and the findings files in their
  packages' `testdata/checks/` (steps 5 and 8). It keeps a package's
  registry while any findings file of the package is left, so that strip
  run again finds it, and every registry while it leaves any `*.txtar`
  file as it was, for a problem of form, a failed write or a place that is
  no archive's, so that strip run again reads them as this run did.
  `-column <name>` limits it to a column's sections, at every level, and
  `-transform <name>` to those under a transform's variant (step 7).
  Either leaves the registries and findings files.
- `-root <dir>` names the corpus root, relative to the current directory
  or by an absolute path. Without it, strip takes the root that the
  registry of the package in the current directory records, or, at the
  module root with no registry there, the module-root corpus. Anywhere
  else it asks for `-root`, unlike the report (step 9), and so it does for
  a recorded root inside a module nested in the package's. Before it reads
  anything, it fails when the `testdata/` it reads first is a symbolic
  link: `-root`'s, or else the current directory's. Its line that says to
  run strip again then gives `-root` where it was given, naming the same
  directory, and, inside a module, away from its root, with no `-root`,
  says to name the root with `-root` if the current directory then holds
  no registry. Otherwise, a line that says to run strip again gives the
  command, with `-root` unless strip ran at the module root on the
  module-root corpus and the registry there, if there is one, records that
  corpus: a strip may remove the registry that named the root, and one
  that names another root would find that one. A path a shell would read
  as other than itself is quoted.
- It leaves frozen cells (step 4) in place and lists them, and `-frozen`
  strips those too. A frozen cell's line gives the command that strips
  it: `-column` with its column, and `-transform` for a cell of a
  transform's variant. No flag limits strip to one archive or to a
  hand-written variant, so the command strips the column's other frozen
  cells too. It leaves as it was, and lists with the reason, an
  archive with a problem of form, and one that isn't
  `testdata/<corpus>/<archive>.txtar` with both names under the naming
  rule and `<corpus>` other than `checks`, and then exits non-zero. It
  goes by location alone, so it strips an archive in a directory that no
  corpus type declares. It doesn't reach into a nested module or another
  corpus root under `testdata/`, and a symbolic link where it reaches,
  `testdata/` itself included, fails it before it removes anything. It
  reads any other package's `testdata/` through a link in its place, the
  current directory's under `-root` included, and removes a registry
  there that records the root, and the package's findings file, through
  the link (#57). Its last line is the update to run next.
- Stripping and then updating, repeated until an update changes nothing,
  reaches what updating the hand-written sections alone reaches, frozen
  cells excepted: a known state.

**What an archive can't hold** (proposed).

- Read from the source of `golang.org/x/tools/txtar` v0.52.0, and untested:
  `Format` adds a final newline to a section that lacks one, `Parse` gives
  every non-empty section a final newline, and a line of the form
  `-- name --` inside a section starts another when parsed. #4's tests
  settle these. So an output with no final newline, or with a line shaped
  like a section marker, can't be held. Such an output fails its cell,
  naming the file and the reason, and an update writes nothing for that
  cell.
- Every write reads the archive back and compares each section with what it
  meant to write, so no section is ever written in a form that reads back
  differently.
- Trade-off: the alternative is an encoding that the archive package applies
  to such a section and reverses on reading. Failing keeps every archive
  readable as plain text, at the cost of a run normalizing such output
  before it can be compared.

**Names** (proposed).

- A name is lowercase ASCII letters and digits, in words joined by single
  hyphens, starting with a letter: `twice`, `open-quote`. The rule covers
  the names of archives, columns and corpus directories here, and property
  names (step 4), variant and transform names (step 7) and check names
  (step 8). Such a name never collides with another on a case-insensitive
  filesystem, never needs quoting in `-run`, and never holds the space that
  ends a column's name in a `gaps` line (step 4).
- A project can't narrow the rule. A parameter derived from an archive's
  name (step 6) follows a convention that accepts every name the rule
  admits.
- A case is named for its corpus's directory and its archive: the case
  `sheet/twice`. A cell's subtest is named `<case>/<column>`:
  `TestCorpus/sheet/twice/evaluate`. `go test -run 'TestCorpus/sheet/twice/'`
  runs one case's cells. Two archives with one name in different corpora are
  two cases with two names (step 10). Trade-off: no pair of archives has to
  be refused, at the cost of a longer name for every case.
- Trade-off: one strict rule everywhere keeps names safe on every
  filesystem and in every `-run` pattern, at the cost of names that can't
  carry case or punctuation. A column that derives a parameter from an
  archive's name has to accept every name the rule admits.

**Descriptions.**

- The archive's comment, the suite's description and each output
  declaration's description are required. A missing description fails the
  run, but does not stop an update from writing outputs. Each later step
  names what else needs a description, under the same rule.

### What can now be seen

- A case and what it produced, in one file. A change to a case is a diff of
  one file, which a reviewer reads in one place.
- One command updates every corpus in the module.
- A failure names the case and the column, prints both descriptions, and ends
  with what to do. For illustration:

  ```
  --- FAIL: TestCorpus/sheet/twice/evaluate
      sheet/twice: A1 is twice B1.
      evaluate: Evaluates every cell of the sheet.
      out/evaluate/values.txt differs (-golden +got):
      - A1 6
      + A1 6.0
        B1 3
      If the change is intended, run SHEETS_UPDATE_GOLDEN=1 go test ./... and read the diff before committing.
  ```

**When to add an archive** (proposed): when an input makes a point no
existing archive makes. A further behavior of an existing input is another
output of the suite, or another suite, never a copy of the archive. From
step 7, an input that is an existing one with a file or two changed is a
variant of it. Trade-off: the corpus stays small and every column sees every
input, at the cost of a judgment call about what counts as a new point.

## 4. Expected errors and known gaps

### What step 3 leaves

At step 3 every error fails its case, so a case meant to be rejected can't
pass. Committing its `error.txt` doesn't help, because committed outputs
can't be the expectation. They say only that a run matches the last one
accepted, and regenerating them erases that. So expected errors are stated by
hand, with properties, where no update writes.

### What the author writes

A **property** is a named fact about a case, such as `circular`: a sheet in
which some formula refers back to itself. The author declares properties in
Go, states on each case the properties it has, and lists on the suite the
properties it rejects.

Proposed names: `decl.Property`, and the suite's `Rejects` field.

`walkthrough/sheets/step04/sheetcorpus/project.go` declares the property,
beside the project:

<!-- source: walkthrough/sheets/step04/sheetcorpus/project.go#properties -->
```go
var Circular = decl.Property{
	Name:        "circular",
	Description: "Some cell's formula refers back to itself, directly or through other cells.",
}
```

`walkthrough/sheets/step04/evaluate_test.go` registers the property, lists it
in `evaluate`'s `Rejects`, and declares `error.txt`, which a cell that errs
writes:

<!-- source: walkthrough/sheets/step04/evaluate_test.go#suite -->
```go
func init() {
	hypercase.Register(sheetcorpus.Project, hypercase.Root("step04"), hypercase.RootPackage(), sheetcorpus.Circular, evaluate)
}

var evaluate = hypercase.Suite[fs.FS]{
	Name:        "evaluate",
	Description: "Evaluates every cell of the sheet.",
	Corpus:      "sheet",
	Outputs: []decl.Output{
		{Pattern: "values.txt", Description: "Each cell's value, one per line, in cell order.", Inputs: sheetcorpus.Cells},
		{Pattern: "error.txt", Description: "Why the sheet can't be evaluated.", Inputs: decl.FromPositions},
	},
	Rejects: []decl.Property{sheetcorpus.Circular},
	Run: func(t testing.TB, archive string, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
		values, err := sheet.Eval(in) // as in step 3
		if err != nil {
			return nil, err
		}
		return []hypercase.File{{Path: "values.txt", Data: values.Format()}}, nil
	},
}
```

### How the files change

An archive gains a `properties` section, a `gaps` section, or both. Both are
hand-written. `testdata/sheet/loop.txtar` is meant to be rejected:

<!-- source: walkthrough/sheets/step04/testdata/sheet/loop.txtar -->
```
A1 and B1 refer to each other.
-- in/sheet/A1 --
=B1
-- in/sheet/B1 --
=A1
-- properties --
circular
-- out/evaluate/error.txt --
A1 -> B1 -> A1: cycle
-- out/evaluate/inputs --
error.txt
  in/sheet/A1:1-1
  in/sheet/B1:1-1
-- out/evaluate/outcome --
rejected
```

`testdata/sheet/negative.txtar` records a known gap, a cell that errs when
it shouldn't:

<!-- source: walkthrough/sheets/step04/testdata/sheet/negative.txtar -->
```
A1 is a negative number.
-- in/sheet/A1 --
-3
-- gaps --
evaluate A negative number is valid, and evaluates to itself. Waits on sheets#3.
-- out/evaluate/error.txt --
A1: unexpected "-"
-- out/evaluate/inputs --
error.txt
  in/sheet/A1:1-1
-- out/evaluate/outcome --
rejected
```

`hypercase.json` gains the property, and `evaluate`'s `error.txt` and
`Rejects`.

### Rules

**Properties.**

- A property is declared in Go, with a name and a description, in the
  project's ordinary package, through `decl`. It is registered when the
  package is initialized. A property without a description fails the run.
- Property names are scoped to a corpus root (step 5). Two properties with
  one name are refused at registration (step 5).
- A case states the properties it has, by hand, in its `properties` section,
  one property name per line. A case with no section has none. A name that no
  property declares fails the run.
- The `properties` format (proposed): each line is exactly one name. A blank
  line, a name stated twice, and an empty section each fail the run, naming
  the archive and the line. Trade-off: a careless hand edit fails loudly,
  at the cost of no blank lines or comments to group names.
- The suite lists the properties it rejects, in Go, as `Rejects`.

**The expected-error check.** It runs per cell, in every run, updates
included. A cell is expected to err exactly when its case has a property its
column rejects.

- It errs and is expected to: the check passes, and `error.txt` is compared
  byte for byte like any output.
- It errs and is not expected to: the cell fails. The message names the case
  and the column, and ends with two remedies: fix the code, or state the
  property on the case and in the column's `Rejects`. It never offers a
  `gaps` line.
- It does not err and is expected to: the cell fails, naming each property
  the case has that the column rejects.
- No update makes a failing check pass. The check changes nothing about what
  an update writes.
- The check tests that a rejecting column errs, not why. A case that states
  the wrong one of two properties a column rejects still passes, and only
  `error.txt` shows the reason. #19 records this.

**Panics.**

- A panic is never an expected error. A recovered panic fails its cell in
  every run, updates included, whatever the case states, unless a `gaps`
  line marks the cell. Its `error.txt` holds the panic value only, never a
  stack, so it reads the same on every machine. From step 8 its outcome is
  `panicked`.

**Known gaps.**

- A **known gap** is one cell known to be wrong: it errs when it shouldn't,
  doesn't err when it should, or its output is wrong. It is stated by hand,
  per cell, as a line in the case's `gaps` section: the column's name, then
  what is right and the issue it waits on. That text is required. This is
  the one way to record a known gap.
- The `gaps` format (proposed): each line is the column's name, one space,
  then the text, to the end of the line. Text that is empty or starts with a
  space fails the run, as do a blank line and two lines for one cell. The
  format also holds the line that marks a cell of a transform's variant
  (step 7): `transform/<name>`, one space, the column's name, one space,
  then the text. A column's name holds no `/`, so the two lines never read
  alike. Trade-off: one line per cell keeps each gap greppable and forces
  terse text. The transform's line sits in the base's `gaps`, where a person
  writes, at the cost of a second line shape.
- The line **freezes** the cell. While the cell has committed files, no
  update writes to it. If its run produces anything different from them, the
  cell fails, in every run, updates included.
- The one exception is for upgrades. An update adds to a frozen cell a
  framework file the cell lacks (`outcome` at step 8, `inputs` at step 11),
  only when the cell's existing files match today's run and nothing else in
  the run keeps an update from writing the cell. It never changes or removes
  a file the cell has.
- A person resolves a failing frozen cell: remove the line (the gap is
  closed, and the next update writes the cell), or delete the cell's files
  (the wrong behavior changed shape, and the next update writes them).
- Any problem that keeps an update from writing a cell, such as an output
  no declaration matches (step 3) or a failing classifier (step 8), fails a
  frozen cell first, and its message says an update writes nothing for the
  cell. If the cell's files match the run, those messages are all the cell
  reports. If they differ, it also lists the differences, and promises no
  write: the problems keep an update from writing the cell, and so does the
  line while it freezes the cell.
- A marked cell with no committed files is written by an update like any
  other.
- The expected-error check doesn't fail a marked cell. The freeze does that
  work: a cell that stops erring wrongly changes its files.
- A line naming a column that did not run on the case fails the run.
- No update writes or removes a `gaps` section.
- An update may move a frozen cell's sections into their place; it never
  changes their bytes.

**Descriptions.** Properties, and the text of each `gaps` line, are required
under step 3's rule.

### What can now be seen

- Which cases are meant to be rejected, and by which column, stated where no
  update writes. A column that stops rejecting them fails.
- Each known gap on the cell it marks, saying what is right and which issue
  it waits on. The cell's committed files record today's wrong behavior, and
  any change to it fails until a person looks.
- For illustration, after a change that stops `evaluate` finding cycles:

  ```
  --- FAIL: TestCorpus/sheet/loop/evaluate
      sheet/loop: A1 and B1 refer to each other.
      evaluate: Evaluates every cell of the sheet.
      evaluate returned no error, but the case states circular, which evaluate rejects.
      Fix the code, or correct the case's properties or evaluate's Rejects.
  ```

**When to declare a property** (proposed): for a fact about an input that
some column rejects. Name it for the fact (`circular`), never for the column
that rejects it or the code it reports. One fact is one property, however
many columns reject it. A property whose name follows one error code, or that
exists only so one column can reject it, is a sign to record on #19.
Trade-off: one property per fact keeps the names few and shared across
columns, at the cost of not saying which column a case is aimed at; only
`error.txt` says why a cell erred.

## 5. Suites and the matrix

### What step 4 leaves

Every suite runs on every case, all from one package over the corpus root.
A project tests more than one function or command over the same inputs: a
sheet is evaluated, printed back, and given to the `calc` command. Printing
back has nothing to show for a sheet with no formulas. And each of those is
tested in a package of its own, while step 4 says nothing of how packages
share one corpus root.

### What the author writes

Each function or command under test is a suite, in a package of its own,
and the packages share one corpus root. A **probe** decides whether a case
is for the suite.

Packages over one corpus root share its archives, and each writes only its
own columns' sections. The package that registers `hypercase.RootPackage()`
runs the **root-package check**, which reads every package's registry and
every archive, and reports what no column owns.

Proposed names: the suite's `Probe` field.

The example has three packages over one corpus root: the root package owns
`evaluate`, `format/` owns `format`, and `cli/` owns `calc`.

`walkthrough/sheets/step05/sheetcorpus/project.go` gains two properties:

<!-- source: walkthrough/sheets/step05/sheetcorpus/project.go#properties -->
```go
var Malformed = decl.Property{
	Name:        "malformed",
	Description: "Some cell holds neither a number nor a formula.",
}

var Dangling = decl.Property{
	Name:        "dangling",
	Description: "Some formula refers to a cell the sheet doesn't have.",
}
```

`walkthrough/sheets/step05/corpus_test.go`, the root package, registers the
root-package check and owns `evaluate`:

<!-- source: walkthrough/sheets/step05/corpus_test.go#evaluate -->
```go
func init() {
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("step05"),
		hypercase.RootPackage(),
		sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling,
		evaluate,
	)
}

var evaluate = hypercase.Suite[fs.FS]{
	Name:        "evaluate",
	Description: "Evaluates every cell of the sheet.",
	Corpus:      "sheet",
	Outputs: []decl.Output{
		{Pattern: "values.txt", Description: "Each cell's value, one per line, in cell order.", Inputs: sheetcorpus.Cells},
		{Pattern: "error.txt", Description: "Why the sheet can't be evaluated.", Inputs: decl.FromPositions},
	},
	Rejects: []decl.Property{sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling},
	Run: func(t testing.TB, archive string, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
		values, err := sheet.Eval(in)
		if err != nil {
			return nil, err
		}
		return []hypercase.File{{Path: "values.txt", Data: values.Format()}}, nil
	},
}

func TestCorpus(t *testing.T) { hypercase.Test(t) }
```

`walkthrough/sheets/step05/format/corpus_test.go` owns `format`, whose probe
skips a sheet with no formulas:

<!-- source: walkthrough/sheets/step05/format/corpus_test.go#format -->
```go
func init() {
	hypercase.Register(sheetcorpus.Project, hypercase.Root("step05"), sheetcorpus.Malformed, format)
}

var format = hypercase.Suite[fs.FS]{
	Name:        "format",
	Description: "Prints each formula back in canonical form.",
	Corpus:      "sheet",
	Probe: func(in fs.FS) (why string, ok bool) {
		if !sheet.HasFormula(in) {
			return "no formulas", false
		}
		return "", true
	},
	Outputs: []decl.Output{
		{Pattern: "formulas.txt", Description: "Each formula cell and its formula, in cell order.", Inputs: sheetcorpus.Formulas},
		{Pattern: "error.txt", Description: "Why the sheet doesn't parse.", Inputs: decl.FromPositions},
	},
	Rejects: []decl.Property{sheetcorpus.Malformed},
	Run: func(t testing.TB, archive string, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
		p, err := sheet.Parse(in)
		if err != nil {
			return nil, err
		}
		return []hypercase.File{{Path: "formulas.txt", Data: sheet.Format(p)}}, nil
	},
}
```

`walkthrough/sheets/step05/cli/corpus_test.go` owns `calc`, which hands the
command its stdout and stderr:

<!-- source: walkthrough/sheets/step05/cli/corpus_test.go#calc -->
```go
func init() {
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("step05"),
		sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling,
		calc,
	)
}

var calc = hypercase.Suite[fs.FS]{
	Name:        "calc",
	Description: "Runs the calc command on the sheet.",
	Corpus:      "sheet",
	Outputs: []decl.Output{
		{Pattern: "stdout.txt", Description: "The sheet's values as CSV, one row per line.", Inputs: decl.AllInputs},
		{Pattern: "stderr.txt", Description: "A warning for each cell that divides by zero.", Inputs: decl.AllInputs},
		{Pattern: "error.txt", Description: "Why calc failed.", Inputs: decl.FromPositions},
	},
	Rejects: []decl.Property{sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling},
	Run: func(t testing.TB, archive string, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
		return nil, sheet.Calc(in, stdout, stderr)
	},
}
```

### How the files change

- An archive holds every column's sections, each column under its own
  `out/<suite>/`.
- Each package writes its own registry, `testdata/hypercase.json`, in its
  own directory.

`testdata/sheet/twice.txtar`:

<!-- source: walkthrough/sheets/step05/testdata/sheet/twice.txtar -->
```
A1 is twice B1.
-- in/sheet/A1 --
=B1*2
-- in/sheet/B1 --
3
-- out/calc/inputs --
stdout.txt
  all inputs
-- out/calc/outcome --
accepted
-- out/calc/stdout.txt --
6,3
-- out/evaluate/inputs --
values.txt
  in/sheet/A1
  in/sheet/B1
-- out/evaluate/outcome --
accepted
-- out/evaluate/values.txt --
A1 6
B1 3
-- out/format/formulas.txt --
A1 =B1 * 2
-- out/format/inputs --
formulas.txt
  in/sheet/A1
-- out/format/outcome --
accepted
```

`testdata/sheet/stray.txtar`. Every column reads the raw cells, so each one
meets the malformed cell and records the same error:

<!-- source: walkthrough/sheets/step05/testdata/sheet/stray.txtar -->
```
D1 holds a word, which is neither a number nor a formula.
-- in/sheet/A1 --
4
-- in/sheet/B1 --
6
-- in/sheet/C1 --
=A1+B1
-- in/sheet/D1 --
total
-- properties --
malformed
-- out/calc/error.txt --
D1: not a number or a formula: "total"
-- out/calc/inputs --
error.txt
  in/sheet/D1:1-1
-- out/calc/outcome --
rejected
-- out/evaluate/error.txt --
D1: not a number or a formula: "total"
-- out/evaluate/inputs --
error.txt
  in/sheet/D1:1-1
-- out/evaluate/outcome --
rejected
-- out/format/error.txt --
D1: not a number or a formula: "total"
-- out/format/inputs --
error.txt
  in/sheet/D1:1-1
-- out/format/outcome --
rejected
```

`testdata/sheet/negative.txtar`. A known gap marks one cell, and `calc` errs
on `-3` the same wrong way as `evaluate`, so a person adds a `calc` line to
its `gaps`; without it, `calc`'s cell fails as an error nothing expects.
`format` skips the sheet, which has no formulas:

<!-- source: walkthrough/sheets/step05/testdata/sheet/negative.txtar -->
```
A1 is a negative number.
-- in/sheet/A1 --
-3
-- gaps --
evaluate A negative number is valid, and evaluates to itself. Waits on sheets#3.
calc A negative number is valid, and evaluates to itself. Waits on sheets#3.
-- out/calc/error.txt --
A1: unexpected "-"
-- out/calc/inputs --
error.txt
  in/sheet/A1:1-1
-- out/calc/outcome --
rejected
-- out/evaluate/error.txt --
A1: unexpected "-"
-- out/evaluate/inputs --
error.txt
  in/sheet/A1:1-1
-- out/evaluate/outcome --
rejected
-- out/format/outcome --
skipped no formulas
```

The root package's registry, `step05/testdata/hypercase.json`, holds the
update variable, the corpus root, that the package runs the root-package
check, that the project supplies `Positions`, the properties it registers,
and `evaluate`: the type it consumes, `io/fs.FS`, its corpora, its path
from each, its outputs, each with what its `Inputs` states, and what it
rejects. Step 11 explains `positions` and `inputs`. Its `types` and
`lifts` stay empty until step 6, as do the `lifts` of `evaluate`'s path,
and `issueLinks` until step 9. `codes`, whether the project supplies
`Codes`, stays `false`, and `evaluate`'s `classifier` stays `null`, until
step 8:

<!-- source: walkthrough/sheets/step05/testdata/hypercase.json -->
```json
{
  "format": 8,
  "updateVar": "SHEETS_UPDATE_GOLDEN",
  "corpusRoot": "step05",
  "rootPackage": true,
  "codes": false,
  "positions": true,
  "issueLinks": [],
  "properties": [
    {
      "name": "circular",
      "description": "Some cell's formula refers back to itself, directly or through other cells."
    },
    {
      "name": "dangling",
      "description": "Some formula refers to a cell the sheet doesn't have."
    },
    {
      "name": "malformed",
      "description": "Some cell holds neither a number nor a formula."
    }
  ],
  "types": [],
  "lifts": [],
  "suites": [
    {
      "name": "evaluate",
      "description": "Evaluates every cell of the sheet.",
      "consumes": "io/fs.FS",
      "corpora": [
        "sheet"
      ],
      "paths": [
        {
          "corpus": "sheet",
          "lifts": []
        }
      ],
      "outputs": [
        {
          "pattern": "error.txt",
          "description": "Why the sheet can't be evaluated.",
          "inputs": "positions"
        },
        {
          "pattern": "values.txt",
          "description": "Each cell's value, one per line, in cell order.",
          "inputs": "func"
        }
      ],
      "rejects": [
        "circular",
        "dangling",
        "malformed"
      ],
      "classifier": null
    }
  ]
}
```

### Rules

**Suites and cells.**

- Columns are suites. A suite covers a class of outputs, or one command. A
  cell is one case run through one column, and its outputs go under
  `out/<suite>/…`.
- A suite has a name, a description, a corpus, a probe, its declared
  outputs, `Rejects` and a run. Step 3 says what the run receives.
- Column names are unique per corpus root, not per package or per corpus
  type.

**Probes.**

- A probe decides, from the case's input, whether the case is for the
  suite. It receives the input view and nothing else. When it says no, it
  gives the reason. A probe that says no without one fails the cell. A suite
  with no probe is for every case.
- A cell whose probe says no is **skipped**: it runs nothing.
- A suite that applies to no case fails the run, a suite whose corpus has no
  archives included.
- There is no way to skip a cell by name. Probes decide applicability.

**Per cell.**

- The expected-error check, the panic rule and known gaps hold per cell. A
  `gaps` line names one suite and freezes that cell alone.
- A skipped cell is not checked, because it did not run.
- A panic in a probe is recovered and fails its cell like one in a run.
- Orphans are judged per cell, skipped cells included, so a suite that stops
  applying to a case fails until a person removes its outputs.
- An orphaned `stdout.txt`, `stderr.txt` or `error.txt` gets its own message:
  the run wrote nothing to stdout, wrote nothing to stderr, or returned no
  error.

**Registration.**

- Everything a package registers is registered when the package is
  initialized, before any test runs: properties, corpus types, lifts,
  suites, transforms, the root-package check and corpus checks (step 8).
  Its registry lists all but the corpus checks. A test function runs what
  was registered, and registers nothing. So the registry and the orphan
  checks don't depend on `-run`.
- A refusal at registration, such as two properties with one name, panics
  during package initialization with the message the framework would
  otherwise print. So it doesn't depend on `-run` either.

**The registry.** It is each package's `testdata/hypercase.json`.

- It lists everything the package registers but its corpus checks
  (step 8), each with its description, and for each column the type it
  consumes, the corpora it runs over, its path from each (step 6), its
  declared outputs, each with what its `Inputs` states (step 11), and the
  properties it rejects. It records the package's corpus root, whether
  the project supplies `Positions` (step 11), and whether the package runs
  the root-package check.
- It records the name of the project's update variable, for a tool to print
  the update command. No tool reads it yet.
- It carries a format version, which changes on any change to what the
  registry holds.
- It is compared like a golden. An update writes it by temp file and rename.
- The report and other tools read it instead of parsing Go. Each later step
  adds to it.

**Shared corpora.**

- Every package over a corpus root shares its corpora,
  `testdata/<corpus>/*.txtar` (step 3).
- A package owns the columns it registers, and writes and judges only their
  sections. It reads the comment, `in/`, `properties` and `gaps` as input.
- A normal run only reads.
- An update is a read-modify-write per archive under the corpus root's lock,
  `testdata/.hypercase.lock`, which git ignores: take the lock, re-read the
  archive, replace only the package's own sections, write a temp file,
  rename it over the archive, release the lock.
- At the re-read, a package compares the hand-written sections its cells ran
  on with those it re-read. On a difference it writes nothing to that
  archive and fails, saying to run the update again.
- The root-package check (below) runs in normal runs alone, so an update
  walks the corpus root's `testdata/` for symbolic links itself, before it
  takes the lock or writes anything. Its walk skips a nested module, as
  the check's does, but not another corpus root, which a registry in a
  package no test runs may record. On any link it reaches, `testdata/`
  itself included, the update writes nothing and fails, naming the link in
  the check's words. That includes some that no check reports: one in
  another corpus root's directory, outside that root's own `testdata/`. It
  fails too when the walk can't read a directory. A link made while the
  update runs is left to the next normal run's check.
- A temp file left by a killed write, beside an archive, a registry or a
  findings file, fails the run, naming it.
- An update is the whole module's. A per-package update is not supported.

**The root-package check.**

- It runs once per corpus root: for the module-root corpus, in the module's
  root package; for a corpus root passed with `hypercase.Root`, in the
  package that registers `hypercase.RootPackage()` for it. `hypercase.Test`
  runs it on the test itself, before the package's cells.
- It reads every registry under the root and every archive in the
  directories they declare, and fails on:
  - a column two packages register;
  - one property name with two different entries, which the packages
    declare differently, or which a stale registry records, as one edited
    by hand: the update, which rewrites the registries, ends the second;
  - a section no registry claims, such as `out/<suite>/…` after that suite
    was removed, whose failure names `hypercase strip -root <dir> -column
    <suite>`, to run at the module root, then the update. `-root` names
    the corpus root, `.` for the module-root corpus: without it, strip
    takes the root the module root's registry records, which a stale one
    may record otherwise. The failure says the strip reaches the column's
    sections in every archive under the root, and, since strip leaves a
    cell a `gaps` line marks, it says to remove first a line that marks
    one of theirs. Where the column's name breaks the naming rule, as in
    `out/Old/…`, no column can have it, and strip refuses it as
    `-column`, so the failure says to delete the sections by hand, or to
    run `hypercase strip -root <dir>` with no flags, which removes every
    section an update writes, then the update;
  - an archive whose name breaks the naming rule, as `Bad.txtar`: none of
    its cells run, and `hypercase strip` refuses it, so the check reports
    that alone, as strip words it, and names no strip for its sections;
  - a problem of an archive as a whole: a missing description, a top-level
    name outside the closed set, hand-written sections that don't sit
    together, a `properties`
    line naming a property nothing declares, a `gaps` line naming a column
    no registry has;
  - a registry file whose directory holds no `_test.go` file;
  - a registry it can't read, as one of another format version, whose
    failure says to run the update, which writes it, where its directory
    holds a `_test.go` file, and to delete it where the directory holds
    none, since no update writes it there;
  - a directory under `testdata/` that holds archives and that no registry
    declares;
  - a symbolic link under the corpus root's `testdata/`, `testdata/`
    itself included, whatever it names, but in a nested module or another
    corpus root, which the check doesn't reach: a run reads through one,
    where the check, `hypercase strip` and the report (step 9) don't.
- A declared directory that doesn't exist yet is fine.
- A package whose directory holds no `_test.go` file owns and uses
  nothing, whatever its registry records. Any file whose name ends in
  `_test.go` counts, whatever it holds, but one whose name starts with `_`
  or `.`, which Go ignores. The check reads names, not build constraints,
  so a package whose only test file a constraint excludes counts as
  tested, and the result doesn't depend on the environment the check
  runs in. The check counts none of the registry's columns, lifts or
  transforms (steps 6 and 7) as owned, its lifts as used, or the
  transforms its corpus types list as needing an owner, and compares none
  of its lifts' entries with other registries' (step 10). The sections of
  its suites, its owned lifts and its listed transforms stay claimed, so
  the registry's own failure is the one that reports them. A lift needs
  an owner when a package with tests uses it, and a transform when a
  package with tests lists it in a corpus type. One that needs an owner,
  and that only such packages own, fails as owned by no package, and the
  failure names them. The registry's root-package check, corpus
  types and properties still count where the check compares registries,
  which is the stricter choice: a second root-package check, or a corpus
  type or property that another package declares differently, still
  fails.
- In a normal run, every package's run over a corpus root fails when no
  registry records a root-package check for that root.

**Failure messages.** Every failure message ends with what to do: fix the
code, run the update (giving the command), remove the orphan, or add the
description.

**Slow cases** (proposed). hypercase has no tag for slow cases, and `-short`
changes nothing about which cells run. A person selects cells with `-run`,
which leaves the others unjudged. Trade-off: from step 8 every skip writes a
committed outcome, so no `-short` skip can disagree with the files, at the
cost of no built-in fast lane.

### What can now be seen

- The matrix. One archive shows every column's outputs for its case, and a
  change shows, in the diff that causes it, how it moves other columns'
  outputs.
- Which column rejects what: `loop` is accepted by `format` and rejected by
  `evaluate` and `calc`, each stated in Go.
- What each package declares, in one file per package. A change to a
  description or a `Rejects` list shows in the registry's diff.
- What no column owns: a section left behind by a removed suite fails the
  root-package check, by name.

**A suite, or another output of one** (proposed): another output when it
comes from the same call as the suite's other outputs, so it changes with
them; another suite when it calls something else, needs a different probe,
or rejects different properties. One suite per command or function under
test, and one declared output per class of thing it writes. Trade-off:
outputs that change together are judged together, at the cost of one cell
failing when any of a suite's outputs changes.

## 6. Corpus types and lifts

### What step 5 leaves

Every suite reads the raw input and parses it itself. `stray`'s malformed
cell fails `calc`, `evaluate` and `format` alike, three copies of one error,
and each of them lists `malformed` in its `Rejects`. A suite that only needs
a parsed sheet has no way to say so.

### What the author writes

A **corpus type** is a directory of archives, given a Go type. A **lift** is
a registered function from one type to the next, and a **stage** is the type
a lift reaches. Each lift is a column. A suite's type argument can now be a
stage or a corpus type in place of `fs.FS`, and the lifts that reach the
type it consumes run first.

The sheets' ladder is `Sheet → *sheet.Parsed → *sheet.Graph`, through the
lifts `parse` and `resolve`.

Proposed names: `decl.Type`, `decl.Lift`, `hypercase.Own`, `hypercase.Use`,
and `decl.Project`'s `RenderError` field.

`walkthrough/sheets/step06/sheetcorpus/types.go` declares the corpus type and
both lifts:

<!-- source: walkthrough/sheets/step06/sheetcorpus/types.go#types -->
```go
type Sheet struct{ fs.FS } // in/sheet/A1, in/sheet/B1, …

var SheetType = decl.Type[Sheet]{
	Dir:         "sheet", // testdata/sheet/*.txtar
	Description: "A spreadsheet: one file per cell under in/sheet/, holding a number or a formula.",
}

var Parse = decl.Lift[Sheet, *sheet.Parsed]{
	Name:        "parse",
	Description: "Parses every cell.",
	Outputs: []decl.Output{
		{Pattern: "error.txt", Description: "The cell that is neither a number nor a formula.", Inputs: decl.FromPositions},
	},
	Rejects: []decl.Property{Malformed},
	Run: func(archive string, in Sheet, stderr io.Writer) (*sheet.Parsed, error) {
		return sheet.Parse(in)
	},
}

var Resolve = decl.Lift[*sheet.Parsed, *sheet.Graph]{
	Name:        "resolve",
	Description: "Orders the cells so that each comes after the cells it refers to.",
	Outputs: []decl.Output{
		{Pattern: "error.txt", Description: "The cycle, or the reference to a missing cell.", Inputs: decl.FromPositions},
	},
	Rejects: []decl.Property{Circular, Dangling},
	Run: func(archive string, p *sheet.Parsed, stderr io.Writer) (*sheet.Graph, error) {
		return sheet.Resolve(p)
	},
}
```

`walkthrough/sheets/step06/sheetcorpus/project.go` gives the project its
`RenderError`:

<!-- source: walkthrough/sheets/step06/sheetcorpus/project.go#project -->
```go
var Project = decl.Project{
	UpdateVar:   "SHEETS_UPDATE_GOLDEN",
	RenderError: renderError,
	Positions:   positions,
}

// renderError writes the error's text, then quotes each line it points at. A
// position about a file as a whole names only the file.
func renderError(in fs.FS, err error) []byte {
	out := []byte(err.Error() + "\n")
	for _, p := range sheet.Positions(err) {
		if p.Line == 0 {
			out = fmt.Appendf(out, "  %s\n", p.Path)
			continue
		}
		if line := lineAt(in, p.Path, p.Line); line != "" {
			out = fmt.Appendf(out, "  %s:%d: %s\n", p.Path, p.Line, line)
		} else {
			out = fmt.Appendf(out, "  %s:%d\n", p.Path, p.Line)
		}
	}
	return out
}
```

`walkthrough/sheets/step06/corpus_test.go` registers the corpus type, and
owns both lifts and `evaluate`:

<!-- source: walkthrough/sheets/step06/corpus_test.go#evaluate -->
```go
func init() {
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("step06"),
		hypercase.RootPackage(),
		sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling,
		sheetcorpus.SheetType,
		hypercase.Own(sheetcorpus.Parse, sheetcorpus.Resolve, evaluate),
	)
}

var evaluate = hypercase.Suite[*sheet.Graph]{
	Name:        "evaluate",
	Description: "Evaluates every cell of the sheet.",
	Outputs: []decl.Output{
		{Pattern: "values.txt", Description: "Each cell's value, one per line, in cell order.", Inputs: sheetcorpus.Cells},
	},
	Run: func(t testing.TB, archive string, g *sheet.Graph, stdout, stderr io.Writer) ([]hypercase.File, error) {
		return []hypercase.File{{Path: "values.txt", Data: g.Evaluate().Format()}}, nil
	},
}

func TestCorpus(t *testing.T) { hypercase.Test(t) }
```

`walkthrough/sheets/step06/format/corpus_test.go` uses `parse` and owns
`format`:

<!-- source: walkthrough/sheets/step06/format/corpus_test.go#format -->
```go
func init() {
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("step06"),
		sheetcorpus.SheetType,
		hypercase.Use(sheetcorpus.Parse), // computed here; parse's sections are left alone
		hypercase.Own(format),
	)
}

var format = hypercase.Suite[*sheet.Parsed]{
	Name:        "format",
	Description: "Prints each formula back in canonical form.",
	Probe: func(p *sheet.Parsed) (why string, ok bool) {
		if len(p.Formulas()) == 0 {
			return "no formulas", false
		}
		return "", true
	},
	Outputs: []decl.Output{
		{Pattern: "formulas.txt", Description: "Each formula cell and its formula, in cell order.", Inputs: sheetcorpus.Formulas},
	},
	Run: func(t testing.TB, archive string, p *sheet.Parsed, stdout, stderr io.Writer) ([]hypercase.File, error) {
		return []hypercase.File{{Path: "formulas.txt", Data: sheet.Format(p)}}, nil
	},
}
```

`calc`, in `cli/`, consumes the corpus type itself, since it tests a command
on raw input: `hypercase.Suite[sheetcorpus.Sheet]`, with its outputs and
`Rejects` as in step 5.

### How the files change

- The ordinary package gains the corpus type and the lifts, in
  `sheetcorpus/types.go`. Each suite drops `Corpus`, consumes a stage or the
  corpus type in place of `fs.FS`, and lists only what it rejects itself.
  `evaluate` and `format`, which no longer err, drop their `error.txt`
  declarations.
- The step 3 to 5 examples run as written: a `hypercase.Suite[fs.FS]` that
  names its `Corpus` runs over that directory's raw input views, with no
  corpus type. No archive moves.
- A malformed input's error moves to the lift that rejects it. The cells
  past that lift are blocked, and write no outputs, only the `outcome` step
  8 adds, `blocked parse`. So their old outputs are orphans until a person
  removes them: `stray`'s `out/evaluate/error.txt` and
  `out/format/error.txt`.
- A known gap moves with its error. `parse` now errs on `negative`'s `-3`,
  so `evaluate`, past it, is blocked, and a `gaps` line naming a column that
  didn't run fails. A person changes `negative`'s `evaluate` line to name
  `parse`, and removes `out/evaluate/error.txt`, an orphan beside `stray`'s.
  `calc`'s cell stays frozen, and its `error.txt` now differs, since
  `RenderError` quotes the input, so a person deletes that file and the next
  update writes it.
- `error.txt` holds what `RenderError` writes. Two new archives show its
  other form, an error about a file as a whole, which names only the file.
  `notes` holds a file under `in/sheet/` that is not a cell, and its error
  names `sheet/notes`. `blank`'s `A1` is empty, with no line to point at,
  and its error names `sheet/A1`.
- `hypercase.json` lists each corpus type under `types`, with its Go type,
  and each lift under `lifts`, with its input and product types, the
  properties it rejects and whether the package owns it. Each suite's
  `consumes` names the type it consumes. Each column's `paths` gives, for
  each corpus it runs over, the lifts its input passes through, nearest
  first: `resolve`, then `parse`, for `evaluate` over `sheet`.

`testdata/sheet/stray.txtar`. `parse` rejects the sheet once; `calc` still
reads the raw cells:

<!-- source: walkthrough/sheets/step06/testdata/sheet/stray.txtar -->
```
D1 holds a word, which is neither a number nor a formula.
-- in/sheet/A1 --
4
-- in/sheet/B1 --
6
-- in/sheet/C1 --
=A1+B1
-- in/sheet/D1 --
total
-- properties --
malformed
-- out/calc/error.txt --
D1: not a number or a formula: "total"
  sheet/D1:1: total
-- out/calc/inputs --
error.txt
  in/sheet/D1:1-1
-- out/calc/outcome --
rejected
-- out/evaluate/outcome --
blocked parse
-- out/format/outcome --
blocked parse
-- out/parse/error.txt --
D1: not a number or a formula: "total"
  sheet/D1:1: total
-- out/parse/inputs --
error.txt
  in/sheet/D1:1-1
-- out/parse/outcome --
rejected
-- out/resolve/outcome --
blocked parse
```

`testdata/sheet/negative.txtar`:

<!-- source: walkthrough/sheets/step06/testdata/sheet/negative.txtar -->
```
A1 is a negative number.
-- in/sheet/A1 --
-3
-- gaps --
parse A negative number is valid, and evaluates to itself. Waits on sheets#3.
calc A negative number is valid, and evaluates to itself. Waits on sheets#3.
-- out/calc/error.txt --
A1: unexpected "-"
  sheet/A1:1: -3
-- out/calc/inputs --
error.txt
  in/sheet/A1:1-1
-- out/calc/outcome --
rejected
-- out/evaluate/outcome --
blocked parse
-- out/format/outcome --
blocked parse
-- out/parse/error.txt --
A1: unexpected "-"
  sheet/A1:1: -3
-- out/parse/inputs --
error.txt
  in/sheet/A1:1-1
-- out/parse/outcome --
rejected
-- out/resolve/outcome --
blocked parse
```

### Rules

**Corpus types.**

- A corpus type is a directory. In Go it is a named `fs.FS` wrapper type,
  declared with its directory name and a description. Its archives are
  `testdata/<dir>/*.txtar` under the corpus root.
- Nothing in an archive names its type.
- No corpus type declares `checks`, the directory that holds corpus check
  findings (step 8).
- Corpus types and lifts are declared in `decl`, which doesn't import
  `testing`, so a project's commands use them too.

**Lifts.**

- Each lift is a column, whose cell files go under `out/<lift>/`, so a
  malformed input is tested once, at the lift that rejects it.
- A lift lists the properties it rejects (`Rejects`), and the expected-error
  check judges its cell as it does a suite's. `parse` rejects `malformed`, a
  cell that is neither a number nor a formula. `resolve` rejects `circular`,
  and `dangling`, a reference to a missing cell.
- A lift's function receives the archive's name, which is the same for the
  base and every variant (step 7), its typed input and a writer for stderr.
  It takes no `testing.TB`, and the error it returns is its rejection.
- A lift's cell writes `error.txt` when its function returns an error, and
  `stderr.txt` when it writes to stderr. Each is declared like a suite's
  output.
- A test that produces nothing new, such as "every reference names a cell
  that exists", is a suite over the value it tests. A lift always has a
  product.
- Lifts' columns share the suites' name space.
- A lift's column that no archive reaches yet is not a failure. A suite that
  no archive reaches still applies to no case, and fails (step 5).

**Stages.**

- Typed input is staged and lazy: a case holds its raw view plus a memoized
  accessor per stage. A lift runs at most once per case, however many cells
  read its product.
- A case's cells run in column-name order, lifts as their readers need
  them.
- A lift's product is read-only to its readers.

**Suites by type.**

- A suite declares the type it consumes: one stage, or a corpus type for
  the raw trees, as for a command tested on bad input. It runs over every
  archive whose root type reaches that type.
- A `Suite[fs.FS]` names its `Corpus` (step 3), a directory that no corpus
  type declares, and runs over its archives' raw input views. A suite that
  consumes any other type names no `Corpus`: the type says which archives it
  runs over.
- There must be exactly one path to each consumed type, or the run fails
  before any case runs. The registry records that path, for each column
  and each corpus it runs over, as its lifts, nearest the column first. A
  column that consumes a corpus type records a path with no lifts, and so
  does a `Suite[fs.FS]` that names its `Corpus`.
- Probes are distinct from corpus types: the type decides what a suite
  receives, and the probe decides whether a case is for it.
- A suite's probe receives the value of the type the suite consumes, and
  nothing else. Its run receives that value where step 3's receives the
  input view.

**Ownership** (proposed). This step refines step 5's rules, under which a
package owns every column it registers.

- A package owns the suites it registers, directly or with `Own`, and the
  lifts it registers with `Own`, and writes and judges their sections.
- A lift is registered with `Own` or `Use`, never directly.
- `Use` takes lifts, and registers a lift only so its product is computed
  in process. A package that uses a lift neither writes nor judges the
  lift's sections.
- The root-package check fails on a column two packages own, in place of
  step 5's column two packages register, and on a lift that a package
  with tests uses and none owns (step 5's rule for packages with no
  tests). A suite named like the lift owns no lift, and the check fails on
  the two as a collision, since lifts share the suites' column names.
- A package that uses a lift reaches it from each corpus by the path the
  lift's owner records, so the lift's cells the owner writes are the ones
  the user's cells past it are built through. The root-package check fails
  on a lift a package uses from a corpus its owner doesn't run it over,
  and on one a package reaches by other lifts than its owner, as a copy of
  `parse` under another name, owned by the package that uses `resolve`,
  would. It fails once for each package, lift and corpus, and states the
  conflict: the user, the owner, the lift, the corpus, and the user's path
  from the corpus, with the owner's where it has one.
  - Over a corpus the owner doesn't run the lift over, the failure states
    the two ways the conflict ends: the owner runs the lift over the
    corpus by the user's path, or the user doesn't reach the lift from the
    corpus.
  - Over a path that parts from the owner's, it states the two ways: the
    user reaches the lift by the owner's path, or the owner by the
    user's.
  - It names no registrations: which ones make a way true turns on what
    else each package registers, and on the other packages that use the
    lifts. Making a way true may need other registrations to change, such
    as a lift that would then close a cycle or give a type a second path,
    and the failure says so. The cells of a lift a package stops owning,
    which no package writes then, are sections no registry claims, and
    the check names them too.
  - Where a way holds already, a registry is stale, and the failure says
    so. The update, which rewrites the registries, ends it.

**Blocked cells.**

- When a lift on a cell's path fails, the cell is **blocked**: it writes no
  outputs, only its `outcome` from step 8 on, and its old outputs are
  orphans.
- A blocked cell is not checked for an expected error, because it did not
  run. So a case that is invalid in more than one way states each property,
  the first lift to meet one rejects the case, and nothing is expected of
  the cells past it.

**`RenderError`.**

- The project supplies `RenderError func(in fs.FS, err error) []byte`, which
  renders an error for `error.txt`. It receives the cell's input view, so it
  can quote the input. Every error a column's function returns goes through
  it, suites' included. Without it, `error.txt` holds the error's text.
- Its output ends with a newline; one that doesn't fails the cell.
- A panic in it fails the cell, and an update writes nothing for that cell.
- The other hooks land with their first readers: `Codes` at step 8, and
  `Positions` at step 11.

**Parameters.**

- Parameters are fixed in Go, or derived from the archive's name by a fixed
  convention. No lift of this step uses the archive's name, and a bytes
  lift (step 10) can't: its jenny receives its input alone.
- The archive's name is the one thing a column gets from an archive besides
  its input. It is the same for the base and every variant, so no variant
  varies a parameter.
- Fixed in Go is the choice that is easy to expand later: an `in/params/`
  tree can be added, where absent means today's fixed values.
  Archive-carried parameters would be hard to take back.
- Until that expansion, `params` is a reserved tree name: an archive with an
  `in/params/` tree fails, naming it.

**Descriptions.** Corpus types and lifts, and each of a lift's output
declarations, carry a description, under step 3's rule.

### What can now be seen

- Where on its ladder an input stops. `stray` is rejected once on the
  ladder, by `parse`, and nothing past it is expected to do anything. `calc`,
  which reads the raw cells, rejects it too.
- What each column consumes, in the registry: `evaluate` reads a
  `*sheet.Graph`, and `calc` the raw sheet.
- Each error, quoting the input it points at.

## 7. Variants and transforms

### What step 6 leaves

A case that differs from another by one line is a copy of the whole archive,
and nothing says which line matters. And each invalid case is written by
hand, though most are a valid case with one mechanical change, such as a
cycle added.

### What the author writes

An archive holds several cases: the **base**, which is `in/…` and which the
comment describes, and **variants** on it. A variant changes a file or two of
the base. A hand-written variant is written in the archive. A **transform** is
Go code that makes a variant from the base, and the update writes its
variant's sections.

Proposed names: `decl.Transform` with its `Name`, `Description`, `Gives`,
`Keeps`, `Probe` and `Jenny` fields, and `decl.Type`'s `Transforms` field.
The variant prefixes `variant/<name>/` and `transform/<name>/` are proposed.
Trade-off: the prefix says whether a person or an update wrote the variant,
at the cost of reserving two names, as top-level section names and as
column names, and of longer section paths.

A transform's `Jenny` is a
[jenny](https://pkg.go.dev/github.com/sdboyer/jenny): a named generator
function, which here generates the variant's whole input view from the
base. `walkthrough/sheets/step07/sheetcorpus/types.go` declares both
transforms and lists them in the corpus type. Beside them, `view` gives each
file of the sheet that `sheet.Loop` or `sheet.MoveDown` returns as one of the
jenny's files:

<!-- source: walkthrough/sheets/step07/sheetcorpus/types.go#transforms -->
```go
var Looped = decl.Transform[Sheet]{
	Name:        "looped",
	Description: "The first cell in cell order, row by row then column by column, replaced by a reference to itself, so the sheet has a cycle.",
	Gives:       []decl.Property{Circular},
	Probe:       func(base Sheet) (why string, ok bool) { return sheet.Loopable(base) },
	Jenny: jenny.For[Sheet]().New("looped", func(base Sheet) (jenny.Files, error) {
		return view(sheet.Loop(base))
	}),
}

var Renamed = decl.Transform[Sheet]{
	Name:        "renamed",
	Description: "Every cell moved one row down, A1 to A2, with every reference rewritten to the moved cell.",
	Keeps:       []decl.Property{Circular, Dangling},
	Probe:       func(base Sheet) (why string, ok bool) { return sheet.Movable(base) },
	Jenny: jenny.For[Sheet]().New("renamed", func(base Sheet) (jenny.Files, error) {
		return view(sheet.MoveDown(base))
	}),
}

var SheetType = decl.Type[Sheet]{
	Dir:         "sheet",
	Description: "A spreadsheet: one file per cell under in/sheet/, holding a number or a formula.",
	Transforms:  []decl.Transform[Sheet]{Looped, Renamed},
}

// view gives a sheet's every file as a transform's jenny returns them, by its
// path in the sheet's input view.
func view(fsys fs.FS, err error) (jenny.Files, error) {
	if err != nil {
		return nil, err
	}
	var files jenny.Files
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		files = append(files, jenny.NewFile(p, data))
		return nil
	})
	return files, err
}
```

One package owns each transform. The root package,
`walkthrough/sheets/step07/corpus_test.go`, adds both to its registration:

<!-- source: walkthrough/sheets/step07/corpus_test.go#register -->
```go
func init() {
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("step07"),
		hypercase.RootPackage(),
		sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling,
		sheetcorpus.SheetType,
		hypercase.Own(sheetcorpus.Parse, sheetcorpus.Resolve, evaluate),
		hypercase.Own(sheetcorpus.Looped, sheetcorpus.Renamed),
	)
}
```

Every package that registers the corpus type registers the properties its
transforms give and keep. `walkthrough/sheets/step07/format/corpus_test.go`
adds `circular` and `dangling`, which `cli/` registers already for its
`Rejects`:

<!-- source: walkthrough/sheets/step07/format/corpus_test.go#register -->
```go
func init() {
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("step07"),
		sheetcorpus.Circular, sheetcorpus.Dangling,
		sheetcorpus.SheetType,
		hypercase.Use(sheetcorpus.Parse), // computed here; parse's sections are left alone
		hypercase.Own(format),
	)
}
```

### How the files change

Archives gain variants. `testdata/sheet/twice.txtar` has two hand-written
variants and both transforms' variants. `evaluate` is blocked on every
variant but `transform/renamed`:

<!-- source: walkthrough/sheets/step07/testdata/sheet/twice.txtar -->
```
A1 is twice B1.
-- in/sheet/A1 --
=B1*2
-- in/sheet/B1 --
3
-- out/calc/inputs --
stdout.txt
  all inputs
-- out/calc/outcome --
accepted
-- out/calc/stdout.txt --
6,3
-- out/evaluate/inputs --
values.txt
  in/sheet/A1
  in/sheet/B1
-- out/evaluate/outcome --
accepted
-- out/evaluate/values.txt --
A1 6
B1 3
-- out/format/formulas.txt --
A1 =B1 * 2
-- out/format/inputs --
formulas.txt
  in/sheet/A1
-- out/format/outcome --
accepted
-- out/parse/inputs --
-- out/parse/outcome --
accepted
-- out/resolve/inputs --
-- out/resolve/outcome --
accepted
-- variant/cycle/about --
B1 refers back to A1, so the sheet has a cycle.
-- variant/cycle/properties --
circular
-- variant/cycle/in/sheet/B1 --
=A1
-- variant/cycle/out/calc/error.txt --
A1 -> B1 -> A1: cycle
  sheet/A1:1: =B1*2
  sheet/B1:1: =A1
-- variant/cycle/out/calc/inputs --
error.txt
  in/sheet/A1:1-1
  variant/cycle/in/sheet/B1:1-1
-- variant/cycle/out/calc/outcome --
rejected
-- variant/cycle/out/evaluate/outcome --
blocked resolve
-- variant/cycle/out/format/formulas.txt --
A1 =B1 * 2
B1 =A1
-- variant/cycle/out/format/inputs --
formulas.txt
  in/sheet/A1
  variant/cycle/in/sheet/B1
-- variant/cycle/out/format/outcome --
accepted
-- variant/cycle/out/parse/inputs --
-- variant/cycle/out/parse/outcome --
accepted
-- variant/cycle/out/resolve/error.txt --
A1 -> B1 -> A1: cycle
  sheet/A1:1: =B1*2
  sheet/B1:1: =A1
-- variant/cycle/out/resolve/inputs --
error.txt
  in/sheet/A1:1-1
  variant/cycle/in/sheet/B1:1-1
-- variant/cycle/out/resolve/outcome --
rejected
-- variant/missing/about --
B1 is gone, so A1 refers to a cell that doesn't exist.
-- variant/missing/properties --
dangling
-- variant/missing/delete --
sheet/B1
-- variant/missing/out/calc/error.txt --
A1: B1 doesn't exist
  sheet/A1:1: =B1*2
-- variant/missing/out/calc/inputs --
error.txt
  in/sheet/A1:1-1
-- variant/missing/out/calc/outcome --
rejected
-- variant/missing/out/evaluate/outcome --
blocked resolve
-- variant/missing/out/format/formulas.txt --
A1 =B1 * 2
-- variant/missing/out/format/inputs --
formulas.txt
  in/sheet/A1
-- variant/missing/out/format/outcome --
accepted
-- variant/missing/out/parse/inputs --
-- variant/missing/out/parse/outcome --
accepted
-- variant/missing/out/resolve/error.txt --
A1: B1 doesn't exist
  sheet/A1:1: =B1*2
-- variant/missing/out/resolve/inputs --
error.txt
  in/sheet/A1:1-1
-- variant/missing/out/resolve/outcome --
rejected
-- transform/looped/in/sheet/A1 --
=A1
-- transform/looped/out/calc/error.txt --
A1 -> A1: cycle
  sheet/A1:1: =A1
-- transform/looped/out/calc/inputs --
error.txt
  transform/looped/in/sheet/A1:1-1
-- transform/looped/out/calc/outcome --
rejected
-- transform/looped/out/evaluate/outcome --
blocked resolve
-- transform/looped/out/format/formulas.txt --
A1 =A1
-- transform/looped/out/format/inputs --
formulas.txt
  transform/looped/in/sheet/A1
-- transform/looped/out/format/outcome --
accepted
-- transform/looped/out/parse/inputs --
-- transform/looped/out/parse/outcome --
accepted
-- transform/looped/out/resolve/error.txt --
A1 -> A1: cycle
  sheet/A1:1: =A1
-- transform/looped/out/resolve/inputs --
error.txt
  transform/looped/in/sheet/A1:1-1
-- transform/looped/out/resolve/outcome --
rejected
-- transform/renamed/in/sheet/A2 --
=B2*2
-- transform/renamed/in/sheet/B2 --
3
-- transform/renamed/delete --
sheet/A1
sheet/B1
-- transform/renamed/out/calc/inputs --
stdout.txt
  all inputs
-- transform/renamed/out/calc/outcome --
accepted
-- transform/renamed/out/calc/stdout.txt --
,
6,3
-- transform/renamed/out/evaluate/inputs --
values.txt
  transform/renamed/in/sheet/A2
  transform/renamed/in/sheet/B2
-- transform/renamed/out/evaluate/outcome --
accepted
-- transform/renamed/out/evaluate/values.txt --
A2 6
B2 3
-- transform/renamed/out/format/formulas.txt --
A2 =B2 * 2
-- transform/renamed/out/format/inputs --
formulas.txt
  transform/renamed/in/sheet/A2
-- transform/renamed/out/format/outcome --
accepted
-- transform/renamed/out/parse/inputs --
-- transform/renamed/out/parse/outcome --
accepted
-- transform/renamed/out/resolve/inputs --
-- transform/renamed/out/resolve/outcome --
accepted
```

`hypercase.json` gains each corpus type's transforms, with `Gives`, `Keeps`,
whether each has a probe, and whether the package owns it.

### Rules

**Cases.**

- The base is a case like any other.
- Variants are prefixed. A hand-written variant is `variant/<name>/…`, and a
  transform's variant is `transform/<name>/…`. The two kinds have separate
  name spaces.
- Cases are named `<archive>`, `<archive>/variant/<name>` and
  `<archive>/transform/<name>` in subtests and in the report, after the
  corpus directory (step 3): `TestCorpus/sheet/twice/transform/looped/resolve`.
- So `variant` and `transform` are reserved column names. A suite or a lift
  given either is refused at registration.

**Hand-written variants.**

- Hand-written: `about`, which describes the variant; `in/**`, its overlay;
  `delete`; `properties`; `gaps`. Update-written: `variant/<name>/out/**` and
  `variant/<name>/via/**`.
- The overlay is laid over the base path by path: the same path replaces, a
  new path adds, and everything else is inherited.
- `delete` lists the base paths the variant removes, one per line, as paths
  in the input view (proposed). A line naming a path the base lacks fails, naming the
  path. So does a path that the variant both overlays and deletes.
- A variant whose input view equals the base's fails.
- A variant inherits none of the base's properties or gaps: each case states
  its own, in `variant/<name>/properties` and `variant/<name>/gaps`.

**Transforms.**

- A transform is Go code, with a name and a description, that makes a
  variant's inputs from the base. It is part of its corpus type's
  declaration, so every package that runs the type has it.
- Every section of its variant is update-written: `in/**`, `delete`,
  `out/**` and `via/**`. It has no hand-written section.
- It states `Gives` and `Keeps`. Its variant's properties are its `Gives`,
  plus each `Keeps` property the base states.
- A base that states a property the transform neither gives nor keeps is
  declined automatically, with that as the reason.
- Its probe decides from the input only: it receives the base's input view,
  never property names or the archive's name, and gives a reason when it
  declines.
- Its `Jenny` makes the variant's input view from the base's: the files it
  generates are the variant's whole input view, by their paths in it, such
  as `sheet/A1`. It receives the base alone, not the archive's name.
- The registry lists `Gives` and `Keeps`, so every package computes the same
  variants without running anything.
- A package that registers the corpus type registers each property its
  transforms give or keep. Without one, the package's run fails before any
  case runs, naming the property.
- A transform can fail on a base: its jenny errs or panics, or makes a view
  equal to the base's, or one that no archive or file tree can hold, or its
  probe panics, or declines without a reason. The variant then fails in
  every run of the package that owns the transform, naming the failure, and
  no cell runs on it. Every other package skips the variant, saying that
  the owner reports why.
- `looped` makes the first cell refer to itself and gives `circular`, so
  `resolve` is expected to err on every `looped` variant with nothing written
  by hand. `renamed` moves every cell one row down, rewriting each reference,
  and keeps `circular` and `dangling`, so it runs over bases that state
  either; a base that states `malformed` is declined.
- Their probes decline a base only when the transform can't apply to it,
  which is a fact about the sheet: it has no cells; for `looped`, its first
  cell already refers to itself; for `renamed`, a cell or a reference is on
  the last row, with no row below. A sheet a probe can't read, or one
  holding a file that isn't a cell, is malformed, and no reason to decline.
  By design, the probe leaves it to the jenny, which fails on it, and so the
  transform fails. No case in the sheets corpus reaches that: `parse`
  rejects such a sheet, so its base states `malformed`, and both transforms
  decline a base that states a property they neither give nor keep, before
  their probes run.
- It is materialized. An update writes `transform/<name>/in/…` and
  `transform/<name>/delete`, and the owner's normal run checks them like
  goldens.
- Every package's update runs the variant's cells on what the transform
  makes, and every normal run on the committed sections. What an update
  writes depends only on the inputs and the code, so one pass of the
  module's update settles, whichever package runs first. A frozen cell is
  the exception: no update writes it, but for step 4's upgrades. When a
  transform's change moves its output, every update of the package that
  owns its column fails on it, and so does each of that package's normal
  runs once the transform's owner has rewritten the inputs. Before then,
  the cell reads the old inputs, and that package's run of the frozen cell
  passes, while the transform owner's normal run fails on the inputs, which
  differ from what the transform makes. It fails until a person deletes its
  files or removes its `gaps` line, and then the package that owns its
  column runs its update, which writes the cell. Until then, that
  package's normal run still fails on it, since no other package's update
  writes it.
- The update of a package that doesn't own the transform, alone, after the
  transform changed, writes the variant's cells on what the transform
  makes, while its normal run reads the sections the owner has yet to
  rewrite. A cell whose output on those sections differs from what the
  update wrote fails, and the failure shows how the sections differ from
  what the transform makes, and names the module's update.
- When the committed sections give another input view than the one the
  transform makes, the update may do otherwise with a cell than a normal
  run: run it on another value, or skip, block or fail it otherwise. A
  normal run stages the variant on what the transform makes, as the update
  does, to tell, and runs the cell's probe, if it has one, on what that
  reaches. Where the update does otherwise, every message about the cell
  ends with what it does: that it runs the cell, on another value than the
  normal run where the normal run ran it; skips it, and why; blocks it,
  and by which lift; or fails it, and how its probe fails, a panic among
  them. What follows depends on what the message is about:
  - the cell's run, a panic or an error the case doesn't expect among
    them, a file it makes, or a frozen cell's files: to run the update,
    fix what it reports, and read the diff before committing;
  - a probe that says no with no reason, or with one an outcome can't
    hold, and, from step 8 on, the cell's outcome: what the update writes
    for the cell. That is its run's, if anything, where it runs the cell;
    from step 8 on, the outcome it writes where it skips or blocks it; and
    where its probe fails, `panicked`, with the panic in `error.txt`, for
    a probe that panics, and nothing for one that says no with no reason,
    or with one an outcome can't hold. On the outcome, it adds that the
    probe needs fixing;
  - a file the cell commits and doesn't produce: where the update runs the
    cell, to delete nothing by hand before it has run, since its run may
    still write the file; where it skips or blocks it, to fix the probe,
    or the lift or the transform's jenny, or to delete the section by
    hand, which the update leaves in place; where its probe fails, to fix
    the probe;
  - a `gaps` line on a cell that runs nothing: where the update skips or
    blocks it, to remove the line, or fix the probe, or the lift or the
    transform's jenny; otherwise, to run the update, fix what it reports,
    and read the diff before committing.

  Only where the update does with the cell what the normal run does is a
  message's advice its own. Sections that give the same input view, as a
  `delete` line naming a path the base lacks does, run every cell on the
  same value in both runs.
- An update replaces a transform's `in/…` and `delete` whole: a file the
  transform no longer produces is removed, and the removal shows in the diff.
- When a transform no longer applies to a base, no cell runs on its
  variant. Its owner's normal run fails on any `in/…` and `delete` the
  archive still holds for it, whether or not they make an input view, and
  its update removes them. What remains under `transform/<name>/out/` is
  each column's orphan.
- A transform nobody registers any more leaves sections nothing claims. The
  root-package check reports them, and names `hypercase strip -root <dir>
  -transform <name>`, then the update, as it does for a removed suite
  (step 5).
- The root-package check fails on a transform that a package with tests
  lists in a corpus type and no package owns (step 5's rule for packages
  with no tests), and on one that two own.
- The package that owns a transform writes its `in/…` and `delete`, and
  judges them against what the transform makes. When the transform makes
  the variant, every package's normal run fails it if its committed `in/…`
  and `delete` make no input view, such as an empty `delete` or one that
  lists a path twice, and runs none of its cells. Each column's owner
  writes and judges that column's cells on the variant.

**Known gaps in a transform's variant.** A known gap in a cell of a
transform's variant is a line in the base's `gaps` section that names the
transform. Its format (proposed) is `transform/<name>`, one space, the
column's name, one space, then the text. In `negative`, whose `renamed`
variant moves the negative number to A2 and errs the same wrong way as the
base, the last two lines freeze that variant's `parse` and `calc` cells.
Its `looped` variant replaces the number with a cycle, which `resolve` and
`calc` are expected to reject, so no line marks it:

<!-- source: walkthrough/sheets/step07/testdata/sheet/negative.txtar -->
```
A1 is a negative number.
-- in/sheet/A1 --
-3
-- gaps --
parse A negative number is valid, and evaluates to itself. Waits on sheets#3.
calc A negative number is valid, and evaluates to itself. Waits on sheets#3.
transform/renamed parse A negative number is valid, and evaluates to itself. Waits on sheets#3.
transform/renamed calc A negative number is valid, and evaluates to itself. Waits on sheets#3.
-- out/calc/error.txt --
A1: unexpected "-"
  sheet/A1:1: -3
-- out/calc/inputs --
error.txt
  in/sheet/A1:1-1
-- out/calc/outcome --
rejected
-- out/evaluate/outcome --
blocked parse
-- out/format/outcome --
blocked parse
-- out/parse/error.txt --
A1: unexpected "-"
  sheet/A1:1: -3
-- out/parse/inputs --
error.txt
  in/sheet/A1:1-1
-- out/parse/outcome --
rejected
-- out/resolve/outcome --
blocked parse
-- transform/looped/in/sheet/A1 --
=A1
-- transform/looped/out/calc/error.txt --
A1 -> A1: cycle
  sheet/A1:1: =A1
-- transform/looped/out/calc/inputs --
error.txt
  transform/looped/in/sheet/A1:1-1
-- transform/looped/out/calc/outcome --
rejected
-- transform/looped/out/evaluate/outcome --
blocked resolve
-- transform/looped/out/format/formulas.txt --
A1 =A1
-- transform/looped/out/format/inputs --
formulas.txt
  transform/looped/in/sheet/A1
-- transform/looped/out/format/outcome --
accepted
-- transform/looped/out/parse/inputs --
-- transform/looped/out/parse/outcome --
accepted
-- transform/looped/out/resolve/error.txt --
A1 -> A1: cycle
  sheet/A1:1: =A1
-- transform/looped/out/resolve/inputs --
error.txt
  transform/looped/in/sheet/A1:1-1
-- transform/looped/out/resolve/outcome --
rejected
-- transform/renamed/in/sheet/A2 --
-3
-- transform/renamed/delete --
sheet/A1
-- transform/renamed/out/calc/error.txt --
A2: unexpected "-"
  sheet/A2:1: -3
-- transform/renamed/out/calc/inputs --
error.txt
  transform/renamed/in/sheet/A2:1-1
-- transform/renamed/out/calc/outcome --
rejected
-- transform/renamed/out/evaluate/outcome --
blocked parse
-- transform/renamed/out/format/outcome --
blocked parse
-- transform/renamed/out/parse/error.txt --
A2: unexpected "-"
  sheet/A2:1: -3
-- transform/renamed/out/parse/inputs --
error.txt
  transform/renamed/in/sheet/A2:1-1
-- transform/renamed/out/parse/outcome --
rejected
-- transform/renamed/out/resolve/outcome --
blocked parse
```

**Section order, whole.**

1. The base: its hand-written sections, in the order written; then its
   update-written sections, `via/…` then `out/…`, each sorted by name.
2. Each hand-written variant, in the order written: its hand-written
   sections, then its update-written sections, sorted the same way.
3. Each transform's variant, by transform name: its inputs (`in/…`, then
   `delete`), then `via/…`, then `out/…`, each sorted by name.

| Hand-written, never touched by an update | Update-written |
|---|---|
| the comment, `in/**`, `properties`, `gaps` | `out/**`, `via/**` |
| `variant/<name>/`: `about`, `in/**`, `delete`, `properties`, `gaps` | `variant/<name>/`: `out/**`, `via/**` |
| | everything under `transform/<name>/` |

**Descriptions.** Each transform, and each hand-written variant's
`variant/<name>/about`, carries a description, under step 3's rule. A
transform's description is its variant's.

**Beside a property-based tester.** A transform resembles a generator in a
property-based tester, and what such a tester calls a property is what this
doc calls a hypothesis (step 12). The resemblance is in structure, not
volume: a transform is deterministic and makes one committed variant per
base, where such a tester runs thousands of throwaway inputs and shrinks the
ones that fail. For volume, run one beside the matrix and commit what it
finds as a case. A seeded transform that yields several variants is a
possible later expansion.

### What can now be seen

- A variant's point, as its overlay and its `about`: `variant/cycle` is
  `twice` with `B1` referring back to `A1`.
- One mechanical change across a corpus. Every sheet that `looped` doesn't
  decline gains a `looped` row in every column, and each of its `resolve`
  cells is expected to err, with nothing written by hand.
- What a transform claims, checked. Had `renamed` broken a base's cycle,
  `resolve`'s cell on its variant would fail as one that didn't err.

**A variant, a transform, or an archive** (proposed): a variant when the case
is a base with a file or two changed and the change is what the case is
about, which its `about` says; a transform when one mechanical change makes
a useful case from most bases of a type, so its variant earns a row in every
column; a new archive when no base is close. A transform that most bases
decline is better written as variants by hand. Trade-off: a transform gives
breadth with no hand work, at the cost of variants a reviewer sees only in
the committed sections, not where they are written.

## 8. Outcomes

### What step 7 leaves

A cell passes or fails on its files, and its files don't say what the code
did. A rejected cell and an accepted one both pass, and a reader tells them
apart by reading `error.txt`. Nothing records that a cell was skipped, or
which lift blocked it, and nothing asserts anything across cells.

### What the author writes

The framework writes each cell's **outcome**, the code's verdict on its
case. The author may supply `Codes`, which finds an error's codes, and give
a suite a **classifier**, which refines its outcomes.

Proposed names: `decl.Project`'s `Codes` field; `hypercase.Classifier` with
its `Name`, `Description` and `Classify` fields, set as a suite's
`Classifier`; and `hypercase.Result`, what a run produced, which a
classifier reads.

`walkthrough/sheets/step08/sheetcorpus/project.go` gives the project its
`Codes`:

<!-- source: walkthrough/sheets/step08/sheetcorpus/project.go#project -->
```go
var Project = decl.Project{
	UpdateVar:   "SHEETS_UPDATE_GOLDEN",
	RenderError: renderError,
	Positions:   positions,
	Codes: func(err error) []string {
		var e *sheet.Error
		if errors.As(err, &e) {
			return []string{e.Code}
		}
		return nil
	},
}
```

`walkthrough/sheets/step08/corpus_test.go` gives `evaluate` a classifier,
and owns the corpus check `codes` (below):

<!-- source: walkthrough/sheets/step08/corpus_test.go#evaluate -->
```go
func init() {
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("step08"),
		hypercase.RootPackage(),
		sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling,
		sheetcorpus.SheetType,
		hypercase.Own(sheetcorpus.Parse, sheetcorpus.Resolve, evaluate),
		hypercase.Own(sheetcorpus.Looped, sheetcorpus.Renamed),
		hypercase.Own(codes),
	)
}

var evaluate = hypercase.Suite[*sheet.Graph]{
	Name:        "evaluate",
	Description: "Evaluates every cell of the sheet.",
	Outputs: []decl.Output{
		{Pattern: "values.txt", Description: "Each cell's value, one per line, in cell order.", Inputs: sheetcorpus.Cells},
	},
	Run: func(t testing.TB, archive string, g *sheet.Graph, stdout, stderr io.Writer) ([]hypercase.File, error) {
		return []hypercase.File{{Path: "values.txt", Data: g.Evaluate().Format()}}, nil
	},
	Classifier: hypercase.Classifier[*sheet.Graph]{
		Name:        "div-zero",
		Description: "Adds div-zero when some cell divides by zero.",
		Classify: func(g *sheet.Graph, got hypercase.Result) (string, error) {
			if bytes.Contains(got.File("values.txt"), []byte("#DIV/0!")) {
				return "div-zero", nil
			}
			return "", nil
		},
	},
}
```

### How the files change

Every cell has an `outcome` file, skipped and blocked cells included. The
archives shown in steps 3 to 7 have them too: they come from their examples,
which today's updates write. Their projects supply no `Codes`, so their
rejected cells carry none.

`testdata/sheet/divide.txtar`, whose `A1` is `=B1/C1` with `C1` at `0`. The
classifier makes `evaluate`'s outcome `accepted div-zero`, on the base and
on its `renamed` variant, and each rejected cell of its `looped` variant
carries the cycle's code:

<!-- source: walkthrough/sheets/step08/testdata/sheet/divide.txtar -->
```
A1 divides B1 by C1, which is zero.
-- in/sheet/A1 --
=B1/C1
-- in/sheet/B1 --
1
-- in/sheet/C1 --
0
-- out/calc/inputs --
stderr.txt
  all inputs
stdout.txt
  all inputs
-- out/calc/outcome --
accepted
-- out/calc/stderr.txt --
A1: divides by zero
-- out/calc/stdout.txt --
#DIV/0!,1,0
-- out/evaluate/inputs --
values.txt
  in/sheet/A1
  in/sheet/B1
  in/sheet/C1
-- out/evaluate/outcome --
accepted div-zero
-- out/evaluate/values.txt --
A1 #DIV/0!
B1 1
C1 0
-- out/format/formulas.txt --
A1 =B1 / C1
-- out/format/inputs --
formulas.txt
  in/sheet/A1
-- out/format/outcome --
accepted
-- out/parse/inputs --
-- out/parse/outcome --
accepted
-- out/resolve/inputs --
-- out/resolve/outcome --
accepted
-- transform/looped/in/sheet/A1 --
=A1
-- transform/looped/out/calc/error.txt --
A1 -> A1: cycle
  sheet/A1:1: =A1
-- transform/looped/out/calc/inputs --
error.txt
  transform/looped/in/sheet/A1:1-1
-- transform/looped/out/calc/outcome --
rejected S201
-- transform/looped/out/evaluate/outcome --
blocked resolve
-- transform/looped/out/format/formulas.txt --
A1 =A1
-- transform/looped/out/format/inputs --
formulas.txt
  transform/looped/in/sheet/A1
-- transform/looped/out/format/outcome --
accepted
-- transform/looped/out/parse/inputs --
-- transform/looped/out/parse/outcome --
accepted
-- transform/looped/out/resolve/error.txt --
A1 -> A1: cycle
  sheet/A1:1: =A1
-- transform/looped/out/resolve/inputs --
error.txt
  transform/looped/in/sheet/A1:1-1
-- transform/looped/out/resolve/outcome --
rejected S201
-- transform/renamed/in/sheet/A2 --
=B2/C2
-- transform/renamed/in/sheet/B2 --
1
-- transform/renamed/in/sheet/C2 --
0
-- transform/renamed/delete --
sheet/A1
sheet/B1
sheet/C1
-- transform/renamed/out/calc/inputs --
stderr.txt
  all inputs
stdout.txt
  all inputs
-- transform/renamed/out/calc/outcome --
accepted
-- transform/renamed/out/calc/stderr.txt --
A2: divides by zero
-- transform/renamed/out/calc/stdout.txt --
,,
#DIV/0!,1,0
-- transform/renamed/out/evaluate/inputs --
values.txt
  transform/renamed/in/sheet/A2
  transform/renamed/in/sheet/B2
  transform/renamed/in/sheet/C2
-- transform/renamed/out/evaluate/outcome --
accepted div-zero
-- transform/renamed/out/evaluate/values.txt --
A2 #DIV/0!
B2 1
C2 0
-- transform/renamed/out/format/formulas.txt --
A2 =B2 / C2
-- transform/renamed/out/format/inputs --
formulas.txt
  transform/renamed/in/sheet/A2
-- transform/renamed/out/format/outcome --
accepted
-- transform/renamed/out/parse/inputs --
-- transform/renamed/out/parse/outcome --
accepted
-- transform/renamed/out/resolve/inputs --
-- transform/renamed/out/resolve/outcome --
accepted
```

A sheet with no formulas has `format`'s outcome `skipped no formulas`, as
step 5's `negative` shows. From step 6 it needs to parse as well, since
`format` reads `parse`'s product: step 6's `negative` has `blocked parse`
there.

`Codes` changes the outcome of every rejected cell whose error it gives a
code, frozen ones included; a rejected cell whose error it gives none keeps
`rejected`. In archives that already carry outcomes, as step 7's do, the
committed outcome of `negative`'s four frozen cells is `rejected`, and their
run now gives `rejected S101`. So they fail, and no update writes them. A
person deletes those four `outcome` files, and the next update adds them, as
step 4's upgrade does for a frozen cell whose other files match its run.

`hypercase.json` records each suite's classifier, and whether the project
supplies `Codes`: step 5's shows them unset, and the registry of step 8's
root package sets both.

### Rules

**The outcome.**

- Each cell has an `outcome` file in its column's directory:
  `out/<column>/outcome`, and the same under a variant's prefix. The
  framework writes it and compares it byte for byte. A cell that can't
  have one fails, as below.
- It holds the code's verdict, not the test's: `accepted`,
  `rejected <codes>`, `panicked`, `blocked <lift>` or `skipped <why>`. The
  first word is the state, and the rest is detail.
- The project may supply `Codes func(err error) []string`, which gives a
  rejected cell its codes. Without it, a rejected cell carries none.
- An optional suite classifier, with a name and a description, refines the
  outcome: `accepted div-zero`. It can't change the state, and runs only on
  cells that ran. An error or a panic in a classifier fails the cell, and the
  update then writes nothing for that cell: an update is atomic per cell.
- Skipped and blocked cells write an outcome, and their other committed
  files are orphans. A frozen one keeps the files it has, outcome
  included: its `gaps` line fails it already.
- A lift whose function returns neither a product nor an error gives its
  cell no outcome, since the code gave no verdict. The cell fails, an
  update writes nothing for it, and the cells on the lift's product are
  `blocked`.
- A cell with no outcome fails whether or not a `gaps` line marks it: the
  line freezes a cell's files, its outcome among them, and such a cell has
  none. That holds for the lift's cell above, and for a cell whose
  classifier or `Codes` fails, or whose code, reason or detail an outcome
  can't hold (below).
- The expected-error check reads what the run did, never the committed
  outcome. A cell that erred has the outcome `rejected`, so the two agree. A
  `panicked` cell fails unless a `gaps` line marks it.
- A frozen cell's `outcome` is one of its files. A cell frozen before this
  step has none: a normal run fails on the missing file and says to run the
  update, and the update adds it when the cell's other files match the run.
  A cell whose files differ gains nothing and fails as before, and so does
  one whose run fails in a way that keeps an update from writing a cell,
  such as an output no declaration matches. That upgrade, of `outcome` here
  and `inputs` at step 11, is the one write an update makes to a frozen
  cell, and it never changes or removes a file the cell has.
- Between two revisions, a cell moves on two axes: outcome and bytes.

**The `outcome` format** (proposed).

- One line, ending with a newline.
- `accepted`, then the classifier's detail, if any, after a space.
- `rejected`, then the codes, sorted byte by byte, each once, each after a
  space. A rejected cell with no codes is `rejected` alone. A classifier's
  detail follows the codes after a semicolon and a space:
  `rejected S201; <detail>`.
- `blocked`, then the lift that failed: the first lift on the column's path
  to fail, whether or not it is the lift whose product the column consumes.
- `skipped`, then the probe's reason.
- `panicked`, alone.
- A code holds no space, semicolon, carriage return or newline. A reason or
  a classifier's detail is one line, not empty, with no space at either end.
  A cell whose code, reason or detail breaks this fails, naming it, and an
  update writes nothing for that cell.

**Descriptions.** Classifiers carry a description, under step 3's rule.

### Corpus checks

A **corpus check** (proposed) is an assertion about a group of cells, read
from their committed files. Proposed names: `hypercase.Check`, with its
`Name`, `Description` and `Run` fields, and `hypercase.Cell`, a cell as a
check sees it.

Corpus checks are shown here, beside the outcomes they read (proposed).
Trade-off: step 12 would put them beside hypotheses, the other assertions
about groups of cells, at the cost of placing a built feature in a step
whose design isn't settled.

`walkthrough/sheets/step08/corpus_test.go` declares `codes`, which its
`init` gives to `Own`:

<!-- source: walkthrough/sheets/step08/corpus_test.go#codes -->
```go
var codes = hypercase.Check{
	Name:        "codes",
	Description: "Every code the formula evaluator defines is the code of some rejected cell.",
	Run: func(cells []hypercase.Cell) ([]string, error) {
		seen := map[string]bool{}
		for _, c := range cells {
			for _, code := range c.Outcome.Codes { // empty unless rejected
				seen[code] = true
			}
		}
		var found, missing []string
		for _, code := range sheet.Codes {
			if seen[code] {
				found = append(found, code)
			} else {
				missing = append(missing, code)
			}
		}
		if len(missing) > 0 {
			return nil, fmt.Errorf("no rejected cell has %s", strings.Join(missing, ", "))
		}
		return found, nil
	},
}
```

The check's findings, `testdata/checks/codes.txt`:

<!-- source: walkthrough/sheets/step08/testdata/checks/codes.txt -->
```
S101
S201
S202
```

- A corpus check has a name, a description and a `Run` over every cell, and
  is registered when its package is initialized, given to `Register` or to
  `Own`. The package that registers it owns it, and no registry lists it.
  A test function runs it: `hypercase.Test` runs each as the subtest
  `checks/<name>` (proposed).
- A cell, as a check sees it, exposes its committed outcome, its committed
  sections, its column's registry entry, its case's stated properties and
  its known gap. `hypercase.Cell` holds (proposed) `Type`, `Archive`,
  `Case` and `Column`; `Outcome`, with its `State`, `Codes` and `Detail`;
  `Entry`; `Path`, the lifts the column's input passes through from the
  cell's corpus type, nearest the column first, as its `paths` in the
  registry give them; `Properties`; `Files`, the cell's committed files
  under its `out/<column>/`; and `Gap`, the text of the `gaps` line that
  marks the cell, or empty when none does. A transform's variant's `Gap` is
  read from the base's `gaps`, from the line that starts
  `transform/<name>`.
- A cell's `Path` is read from the registry of the package that owns its
  column, as its `Entry` is, so a check sees the path of another package's
  cells too, through the lifts that package uses, and plans none itself.
  Each lift on the path has a cell of the same case, the one the cell is
  built through, where the root-package check passes: a package that uses
  a lift reaches it by the path its owner records (step 6). A rule over
  what a cell is built on reads those cells: a check that holds back a
  cell a `gaps` line marks can also hold back each cell built through a
  lift's cell a line marks, since the cells past it are still built on the
  lift's product.
- A check sees every cell, frozen ones included: the framework exempts
  none, and each check decides. One that states a requirement over cells
  can excuse a cell with a `Gap`, so that a known gap and the requirement
  coexist: it passes while every cell that violates it is frozen, and
  fails once a line is removed. One that counts what the corpus reaches,
  such as `codes`, counts frozen cells too.
- A check reads only committed files, and never runs a suite, a lift or a
  transform. An archive that can't be read fails every check of the
  package, and an update writes no findings. So does a transform's variant
  whose properties can't be known: one no corpus type lists, or one its
  transform declines. So does a registry that lists a suite or a lift
  name twice, or doesn't give each column, a lift the package uses
  included, one path from each corpus it runs over, each corpus once and
  in their order, or whose path names the column itself, a lift the
  registry doesn't list or one lift twice, or names a lift for a suite
  over the raw input view. The root-package check fails on such a
  registry too. Where the package has a `_test.go` file, both say to run
  the update, which writes its registry; where it has none, both say to
  delete the registry, which no update writes.
- A check reads the cells as they are committed when it runs. The module's
  update runs packages at once, so a check can run before another package
  writes its cells: its findings can take a second update to settle, and a
  normal run between the two fails on them.
- Its findings are committed as a golden at `<pkg>/testdata/checks/<name>.txt`
  in the package that owns the check. A file under `testdata/checks/` that
  is no registered check's findings fails a normal run, and an update
  leaves it in place.
- The findings format (proposed): one finding per line, each ending with a
  newline, sorted byte by byte, each once. An empty list is an empty file. A
  finding that is empty or holds a newline fails the check, and an update
  writes no findings.
- An error from `Run` is a violated requirement: it fails every run, updates
  included, and an update writes no findings.
- Corpus checks carry a description, under step 3's rule.
- It is the first assertion about a group of cells.

### What can now be seen

- The code's verdict on every cell, as a file: `divide`'s `looped` variant
  is `blocked resolve` in `evaluate`, and `rejected S201` in `resolve`.
- A change moves a cell on two axes. An outcome that changes is a different
  verdict; bytes that change under one outcome are a different rendering of
  the same verdict.
- A requirement over many cells: if no case reaches `S202` any more, the
  `codes` check fails.

## 9. The report

### What step 8 leaves

Every cell's verdict is a file, spread over the archives. A change that
moves fifty cells is fifty sections in a diff, in path order, with the cells
whose verdict changed mixed among those whose bytes did.

### What the author writes

A person runs `hypercase report`. The project supplies the patterns that
link a known gap's text to the issue it waits on.

The command takes the flags `-root` and `-text`. The patterns are
`decl.Project`'s `IssueLinks` field, a list of `decl.IssueLink`, each a
regular expression and a URL template.

`walkthrough/sheets/step09/sheetcorpus/project.go` gives the project its
`IssueLinks`, for the tracker that `negative`'s `gaps` lines name:

<!-- source: walkthrough/sheets/step09/sheetcorpus/project.go#project -->
```go
var Project = decl.Project{
	UpdateVar:   "SHEETS_UPDATE_GOLDEN",
	RenderError: renderError,
	Positions:   positions,
	Codes: func(err error) []string {
		var e *sheet.Error
		if errors.As(err, &e) {
			return []string{e.Code}
		}
		return nil
	},
	IssueLinks: []decl.IssueLink{
		{Pattern: `sheets#(\d+)`, URL: "https://example.com/sheets/issues/$1"},
	},
}
```

On a branch, in the directory of a package whose registry records the
corpus root:

```sh
hypercase report "$(git merge-base main HEAD)" HEAD > report.html
hypercase report -text "$(git merge-base main HEAD)" HEAD
```

### How the files change

No archive changes. `hypercase.json` gains the link patterns. The report is
for the person to read, and a project needn't commit it.

`walkthrough/sheets/step09/report_test.go` runs both commands as a person
does on a branch. In a scratch repository, it runs the update on the toy at
step 9 and commits it on `main`, makes a change on a branch, runs the update
again, commits again, and runs the report in `step09/`. Its commits hold
what the report reads: `go.mod`, the registries and the archives. The
change makes three edits:

- `calc` gains a classifier, `warned`, which adds `warned` to the outcome
  of each cell where calc writes a warning.
- The archive `blank` is renamed `empty`.
- The sheet package words a file that is not a cell as
  `notes: not the name of a cell`, where it said `notes: not a cell name`.

The example commits what the report writes as its test's goldens, so that
the walkthrough can show it: the plain text, below, and the page,
`walkthrough/sheets/step09/testdata/report.html`, too long to show here,
which a browser opens as it is.

The report reads each archive as each revision commits it.
`testdata/sheet/notes.txtar` holds two of the cells the sample below
reports. At `<a>`:

<!-- source: walkthrough/sheets/step09/testdata/sheet/notes.txtar -->
```
A note kept beside the cells, in a file that is not a cell.
-- in/sheet/A1 --
1
-- in/sheet/notes --
remember the milk
-- properties --
malformed
-- out/calc/error.txt --
notes: not a cell name
  sheet/notes
-- out/calc/inputs --
error.txt
  in/sheet/notes
-- out/calc/outcome --
rejected S101
-- out/evaluate/outcome --
blocked parse
-- out/format/outcome --
blocked parse
-- out/parse/error.txt --
notes: not a cell name
  sheet/notes
-- out/parse/inputs --
error.txt
  in/sheet/notes
-- out/parse/outcome --
rejected S101
-- out/resolve/outcome --
blocked parse
```

and at `<b>`, each `error.txt` starts `notes: not the name of a cell`, under
the same outcome.

### Rules

- `hypercase report <a> <b>`, from `cmd/hypercase`, takes two git revisions,
  reads both from the repository's object store, never the working tree,
  and writes one HTML page on stdout. It exits 0 when it writes the report,
  2 for a usage error, and 1, with the cause on stderr, when it can't build
  one.
- Each argument is one revision, which must resolve to a commit. No range is
  parsed: `A..B` and `A...B` name no commit, and a caller that wants a merge
  base computes it and passes it. Flags come before the revisions: every
  argument after `--`, or after the first revision, is a revision, even one
  that starts with `-`, and none is ever read as a flag of git's.
- It reads registries and archives, never Go source. At each revision it
  merges the registries that record the corpus root, so one module can hold
  several. Only `<a>` may lack them, as a revision from before the project
  used hypercase does; every cell at `<b>` then appeared. A corpus root that
  no registry records at `<b>` fails the report: the root is wrong.
- It reads every registry format from 3 to its own, and refuses any other,
  naming the revision, the registry and its format: for a newer format,
  saying to run the report from that revision's hypercase, and for one
  older than 3, that no hypercase's report reads it. A registry of an older
  format that holds what a later format added is refused too.
- A cell exists at a revision when its `outcome` file does, from registry
  format 5 on. A changed cell takes the first of these ranks that applies:
  outcome changed, at both revisions; appeared, at `<b>` only; disappeared,
  at `<a>` only; bytes changed, under the same outcome, when another of its
  files is added, changed or removed. A change to a case's description,
  inputs, properties or `gaps` lines alone ranks no cell.
- Registry formats 3 and 4 predate `outcome` files, and a cell that wrote no
  file committed none. At a revision whose newest registry is of such a
  format, every case that runs has a cell under every column the registries
  claim over its corpus, and every case, whether it runs or not, has one
  under each column it has files of; the cell's state is unknown. A case
  runs there when the archive holds a section of it: the base always, and a
  transform's variant only when a corpus type lists its transform. A cell of
  unknown state at either revision ranks on its files alone: bytes changed
  when one is added, changed or removed, and never outcome changed.
- Cases go by corpus type, then archive, then the base, the hand-written
  variants by name, and the transforms' variants by transform name; a case's
  cells by column; names byte by byte. Within a rank, changes go in that
  order.
- Each case's stated properties are shown beside it: for a transform's
  variant, its transform's `Gives`, and each of its `Keeps` that the base
  states, marked given or kept. Each `gaps` line shows on the cell it marks,
  linked to its issue through the patterns the project supplies, one per
  issue tracker its `gaps` lines name.
- What the report can't show as it is, it lists as a problem at its
  revision, and doesn't pick: a column two registries claim, whose
  descriptions it shows neither of; a committed column no registry claims; a
  stated property no registry lists; a `gaps` line naming a column, or a
  transform, no registry has; a section it can't place in a case or a cell;
  a cell with files and no `outcome`, whose files it leaves out; an
  `outcome` at a revision whose registries predate `outcome` files, which it
  leaves out unread; a transform's variant whose transform no corpus type
  lists; and a link pattern at `<b>` that is not a regular expression, which
  links nothing. A `properties` or `gaps` section it can't parse, an
  `outcome` section it can't parse at a revision of format 5 or later, and a
  section an archive holds twice, each fail the report.
- hypercase's own CI runs the report over the toy on each pull request, from
  its merge base to its head, once per corpus root, and keeps the pages.
- Step 11 adds input lines and corpus-check findings.

**Flags.**

- `-root <dir>` names the corpus root, relative to the current directory
  and inside the repository; the report refuses an absolute path. Without
  it, the report takes the root that the registry of the package in the
  current directory records, at `<b>`, or at `<a>` where `<b>` has no
  module there, or, with no registry there, the module-root corpus, from
  any directory of the module.
- `-text` writes the problems and the ranked changes as plain text instead
  of the page.

**Link patterns.** Each `decl.IssueLink` is a regular expression and a URL
template, which may use the expression's groups. The report reads them from
the registries at `<b>`. Text that matches a pattern, in a `gaps` line or a
description, links to the URL it makes.

**The page.** One HTML file, which runs no script and loads nothing: its
styles are inline, and every archive's text is escaped. In order:

- the problems, at `<b>`, then at `<a>`;
- the changes, ranked, each file of a changed cell with how many lines it
  adds and removes, and its line diff, folded;
- a matrix per corpus type at `<b>`, a row per case, a column per column that
  runs on the type's archives. Each cell shows its state and detail, or that
  its state is unknown, its rank if it changed, and the `gaps` line that
  marks it; a marked cell that exists is frozen;
- the descriptions of each corpus type, column, output declaration,
  classifier, transform and property, from `<b>`, or from `<a>` for what `<b>`
  lacks, marked as such; and of each archive and hand-written variant only
  at `<a>`, from its `about`.

An unchanged cell carries only its state, so the page grows with the change.
The line diff numbers each distinct line once, so comparing two lines costs
the same whatever their length. It sets aside the lines both files start
and end with, then finds the shortest edit script by Myers' algorithm, whose
cost grows with the length of what is left times the size of its change, up
to a fixed budget for each attempt. Past that, it aligns the two on the
lines each holds exactly once, in an order both share, and diffs each
stretch between them the same way. The whole diff shares one fixed budget,
which alone pays to look for such lines. Each stretch also has a fixed
amount of work for each of its lines, which it spends once the shared
budget is gone. So a short change after a long rewrite is still diffed
exactly when a line both files hold once lies between them, which puts the
change in a stretch of its own. A diff's work is at most the budget plus a
fixed amount for each line, so its cost never grows with the product of
the files' lengths. A
stretch with no such line, or that the budget and its own work no longer
cover, is left unaligned: its diff is one line saying how many lines it
leaves out of each file, and its file says it is partly unaligned in place
of counts that would be wrong. The same revisions give the same bytes.

**The plain-text format.** The problems first, when there are any, as
the page gives them: under the heading `problems`, a line `at <rev>` per
revision that has them, `<b>` first, and under it one line per problem, its
place, where it has one, then what it is. Then one heading per rank that has
changes, in rank order. Under it, one line per cell: its case, its column,
and its outcome, or the change in it. So `outcome` is never listed as a
file. Under each cell, one line per other file of the cell that was added,
changed or removed. For the example's change,
`walkthrough/sheets/step09/testdata/report.txt`:

<!-- source: walkthrough/sheets/step09/testdata/report.txt -->
```
outcome changed
  sheet/divide calc: accepted -> accepted warned
  sheet/divide/transform/renamed calc: accepted -> accepted warned
appeared
  sheet/empty calc: rejected S101
    error.txt added
    inputs added
  sheet/empty evaluate: blocked parse
  sheet/empty format: blocked parse
  sheet/empty parse: rejected S101
    error.txt added
    inputs added
  sheet/empty resolve: blocked parse
disappeared
  sheet/blank calc: rejected S101
    error.txt removed
    inputs removed
  sheet/blank evaluate: blocked parse
  sheet/blank format: blocked parse
  sheet/blank parse: rejected S101
    error.txt removed
    inputs removed
  sheet/blank resolve: blocked parse
bytes changed
  sheet/notes calc: rejected S101
    error.txt changed
  sheet/notes parse: rejected S101
    error.txt changed
```

A renamed archive's cells are new cells: the report knows a case by its
name, so `blank`'s cells disappear and `empty`'s appear, with the same
files. `blank` states `malformed`, which neither transform gives or keeps,
so neither makes a variant of it, and only its base's cells move.

### What can now be seen

- A change ranked across the whole corpus. Changed verdicts come first, then
  what appeared or disappeared, then what was only reworded.
- The matrix for each corpus type at `<b>`, each cell with its state and
  frozen cells marked.
- Each known gap beside its cell, linked to the issue it waits on.
- The descriptions of everything the change touches, what it removed
  included.
- What the registries and archives disagree on, as problems, beside
  everything the report could place.

## 10. Chaining

### What step 9 leaves

A CSV export holds a sheet. To run the sheet's columns on a CSV, a suite
runs the import and everything after it. A change in the import then fails
every column downstream, and the sheet in between is recorded nowhere.

### What the author writes

A **bytes lift** is a lift whose product type is another corpus type. Its
product is the files it makes, which are that type's input, and they are
committed as such.

Proposed names: `decl.Lift`'s `Product` field, which declares the product's
files, and its `Jenny` field, which a bytes lift gives in place of `Run`. The
`Jenny` is a [jenny](https://pkg.go.dev/github.com/sdboyer/jenny), a named
generator function, which here generates the product's files from the lift's
input. It receives the input alone, so `import` finds its CSV by its tree,
not by the archive's name. `view` gives each file of the sheet that
`sheet.ImportCSV` returns as one of the jenny's files. `decl.Reject` marks
the jenny's error as the lift's rejection of the case: the importer's own
error, a `*sheet.Error`, is one. Any other error is no rejection, so it
fails the cell: an input that breaks the corpus type's rule (one file,
`in/csv/<name>.csv`, with a name holding a character that is neither white
space nor a format character); an input the jenny can't walk; and a CSV it
can't read. White space and format characters are Unicode's White_Space and
category Cf, so a name of a zero-width space alone breaks the rule.

`walkthrough/sheets/step10/sheetcorpus/types.go` declares the corpus type,
the three properties `import` rejects, and the lift:

<!-- source: walkthrough/sheets/step10/sheetcorpus/types.go#import -->
```go
type CSV struct{ fs.FS } // in/csv/<name>.csv

var CSVType = decl.Type[CSV]{
	Dir:         "csv",
	Description: "A CSV export: one file, in/csv/<name>.csv, with a name holding a character that is neither white space nor a format character; one row of the sheet per record, where a quoted field may hold a newline.",
}

var Unterminated = decl.Property{
	Name:        "unterminated",
	Description: "Some quoted field is never closed.",
}

var StrayQuote = decl.Property{
	Name:        "stray-quote",
	Description: "Some unquoted field holds a quote, or some quoted field has text after its closing quote.",
}

var OutOfRange = decl.Property{
	Name:        "out-of-range",
	Description: "Some field that is not empty lies past the sheet's last column, XFD, or its last row, 1048576.",
}

var Import = decl.Lift[CSV, Sheet]{
	Name:        "import",
	Description: "Turns each CSV field into a cell: row 1, field 2 is B1.",
	Outputs: []decl.Output{
		{Pattern: "error.txt", Description: "The line the importer can't read, and why.", Inputs: decl.FromPositions},
	},
	Product: []decl.Output{
		{Pattern: "sheet/*", Description: "One file per cell, as a sheet's in/sheet/ holds them.", Inputs: Export},
	},
	Rejects: []decl.Property{Unterminated, StrayQuote, OutOfRange},
	Jenny: jenny.For[CSV]().New("import", func(in CSV) (jenny.Files, error) {
		var paths []string
		err := fs.WalkDir(in, ".", func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				paths = append(paths, p)
			}
			return err
		})
		if err != nil {
			return nil, err
		}
		var name string
		ok := len(paths) == 1 && path.Dir(paths[0]) == "csv"
		if ok {
			name, ok = strings.CutSuffix(path.Base(paths[0]), ".csv")
		}
		ok = ok && strings.ContainsFunc(name, func(r rune) bool {
			return !unicode.In(r, unicode.White_Space, unicode.Cf)
		})
		if !ok {
			return nil, fmt.Errorf("the input holds %q, and a CSV export is one file, csv/<name>.csv, with a name holding a character that is neither white space nor a format character", paths)
		}
		s, err := sheet.ImportCSV(in, paths[0])
		var e *sheet.Error
		if errors.As(err, &e) {
			return nil, decl.Reject(err)
		}
		if err != nil {
			return nil, err
		}
		return view(s, nil)
	}),
}
```

The root package registers the new type and properties, and owns the new
lift. Its `codes` check (step 8) counts the importer's codes,
`sheet.CSVCodes`, beside the evaluator's, `sheet.Codes`.
`walkthrough/sheets/step10/corpus_test.go`:

<!-- source: walkthrough/sheets/step10/corpus_test.go#register -->
```go
func init() {
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("step10"),
		hypercase.RootPackage(),
		sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling,
		sheetcorpus.Unterminated, sheetcorpus.StrayQuote, sheetcorpus.OutOfRange,
		sheetcorpus.SheetType, sheetcorpus.CSVType,
		hypercase.Own(sheetcorpus.Import, sheetcorpus.Parse, sheetcorpus.Resolve, evaluate),
		hypercase.Own(sheetcorpus.Looped, sheetcorpus.Renamed),
		hypercase.Own(codes),
	)
}
```

Each other package whose columns read past it registers the type and adds
the lift to its `Use`. `cli/` uses `sheetcorpus.Import`, for `calc`, and
`format/` uses it beside `sheetcorpus.Parse`:

<!-- source: walkthrough/sheets/step10/format/corpus_test.go#register -->
```go
func init() {
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("step10"),
		sheetcorpus.Circular, sheetcorpus.Dangling,
		sheetcorpus.SheetType, sheetcorpus.CSVType,
		hypercase.Use(sheetcorpus.Import, sheetcorpus.Parse), // computed here; their sections are left alone
		hypercase.Own(format),
	)
}
```

### How the files change

A new corpus directory, `testdata/csv/`. Each archive whose CSV `import`
accepts holds the product under `via/<lift>/`, and every archive holds the
cells of `import` and of every column past it. `testdata/csv/prices.txtar`:

<!-- source: walkthrough/sheets/step10/testdata/csv/prices.txtar -->
```
Two prices and their total, exported as CSV.
-- in/csv/prices.csv --
4,6,=A1+B1
-- via/import/sheet/A1 --
4
-- via/import/sheet/B1 --
6
-- via/import/sheet/C1 --
=A1+B1
-- out/calc/inputs --
stdout.txt
  all inputs
-- out/calc/outcome --
accepted
-- out/calc/stdout.txt --
4,6,10
-- out/evaluate/inputs --
values.txt
  via/import/sheet/A1
  via/import/sheet/B1
  via/import/sheet/C1
-- out/evaluate/outcome --
accepted
-- out/evaluate/values.txt --
A1 4
B1 6
C1 10
-- out/format/formulas.txt --
C1 =A1 + B1
-- out/format/inputs --
formulas.txt
  via/import/sheet/C1
-- out/format/outcome --
accepted
-- out/import/inputs --
via/import/sheet/A1
  in/csv/prices.csv
via/import/sheet/B1
  in/csv/prices.csv
via/import/sheet/C1
  in/csv/prices.csv
-- out/import/outcome --
accepted
-- out/parse/inputs --
-- out/parse/outcome --
accepted
-- out/resolve/inputs --
-- out/resolve/outcome --
accepted
```

`testdata/csv/open-quote.txtar`. Every column past `import` is blocked by
it:

<!-- source: walkthrough/sheets/step10/testdata/csv/open-quote.txtar -->
```
A quoted field that never closes.
-- in/csv/open-quote.csv --
4,"six,=A1+B1
-- properties --
unterminated
-- out/calc/outcome --
blocked import
-- out/evaluate/outcome --
blocked import
-- out/format/outcome --
blocked import
-- out/import/error.txt --
import: line 1: quote never closes
  csv/open-quote.csv:1: 4,"six,=A1+B1
-- out/import/inputs --
error.txt
  in/csv/open-quote.csv:1-1
-- out/import/outcome --
rejected C101
-- out/parse/outcome --
blocked import
-- out/resolve/outcome --
blocked import
```

A jenny's error starts with the jenny's name, so `import`'s `error.txt` starts
`import: `. `Codes` reaches the importer's error through the jenny and
`decl.Reject`, so the outcome carries its code.

`testdata/csv/stray-quote.txtar` states `stray-quote`, for a quote inside an
unquoted field, and `testdata/csv/past-xfd.txtar` states `out-of-range`, for
a field in column 16385, one past XFD. `import` rejects them as `C102` and
`C103`, and every column past it is `blocked import`, as for `open-quote`.
So some CSV reaches each of the importer's codes, and the `codes` check's
findings, `testdata/checks/codes.txt`, list them beside the evaluator's:

<!-- source: walkthrough/sheets/step10/testdata/checks/codes.txt -->
```
C101
C102
C103
S101
S201
S202
```

`testdata/csv/loop.txtar` states `circular`, which `resolve` rejects past
the lift. Its `error.txt` quotes `sheet/A1` and `sheet/B1`, which only the
product holds: past `import`, `RenderError` gets the product as the input
view.

<!-- source: walkthrough/sheets/step10/testdata/csv/loop.txtar -->
```
A1 and B1 refer to each other, exported as CSV.
-- in/csv/loop.csv --
=B1,=A1
-- properties --
circular
-- via/import/sheet/A1 --
=B1
-- via/import/sheet/B1 --
=A1
-- out/calc/error.txt --
A1 -> B1 -> A1: cycle
  sheet/A1:1: =B1
  sheet/B1:1: =A1
-- out/calc/inputs --
error.txt
  via/import/sheet/A1:1-1
  via/import/sheet/B1:1-1
-- out/calc/outcome --
rejected S201
-- out/evaluate/outcome --
blocked resolve
-- out/format/formulas.txt --
A1 =B1
B1 =A1
-- out/format/inputs --
formulas.txt
  via/import/sheet/A1
  via/import/sheet/B1
-- out/format/outcome --
accepted
-- out/import/inputs --
via/import/sheet/A1
  in/csv/loop.csv
via/import/sheet/B1
  in/csv/loop.csv
-- out/import/outcome --
accepted
-- out/parse/inputs --
-- out/parse/outcome --
accepted
-- out/resolve/error.txt --
A1 -> B1 -> A1: cycle
  sheet/A1:1: =B1
  sheet/B1:1: =A1
-- out/resolve/inputs --
error.txt
  via/import/sheet/A1:1-1
  via/import/sheet/B1:1-1
-- out/resolve/outcome --
rejected S201
```

`hypercase.json` gives every lift `"bytes"`, true for a bytes lift, and
`"productFiles"`, its product declarations, which are empty for every other
lift.

### Rules

- An upstream product is read as downstream input, within the same archive.
- A bytes lift's product is another corpus type's input. It is committed
  under `via/<lift>/`, which holds the product only, under whatever file
  names the product has: `via/import/sheet/A1` is the downstream case's
  `sheet/A1`.
- A lift whose product type is a corpus type gives `Jenny`, not `Run`, and
  declares its product's files in `Product`, each with a pattern and a
  description, as a suite declares its outputs. A lift whose product is a
  stage gives no `Product`. The jenny receives the input alone, with no
  archive name and no stderr, so the lift's cell writes `outcome` and
  `error.txt` only, and `error.txt` is the one output it may declare.
- Each product file sits in a tree, a top-level directory of the product's
  input view, as `sheet/A1` does; `params/` is reserved there as in every
  input view. A pattern of `Product` that can match no such file fails the
  run.
- A product file outside a tree or under `params/`, one that no declaration
  or two declarations match, and one an archive can't hold fail the lift's
  cell, and an update writes nothing for that cell. An archive can't hold a
  file with no final newline, or one whose name doesn't read back as
  written, such as a name with a newline in it or a space at its end.
- A bytes lift rejects a case only with an error that `decl.Reject` marks
  all of. The framework walks the error's tree, through `Unwrap() error`
  and `Unwrap() []error`, and stops at each error `decl.Reject` made; an
  error that wraps none, reached without passing one, makes the whole error
  a fault of the jenny. So a join of rejections is a rejection, and so is a
  join that holds one rejection alone, as jenny joins the error of a jenny
  with one input. A rejection joined with any other error is a fault, as
  jenny's `Pipeline` joins its jennies' errors. Jenny's own errors are
  faults too: two files at one path, two whose paths differ only in case, a
  file that fails to render. A fault fails the lift's cell, and an update
  writes nothing for that cell.
- `decl.Reject` marks all of what it is given. A generator that runs a
  jenny of its own, and marks that jenny's error, has jenny's faults in it
  recorded as its rejection, so it marks only what it rejects.
- The walk never calls an error's own `As` or `Is`. An error that reaches
  the errors it holds through `As`, and not through `Unwrap`, is a fault
  however many of them `decl.Reject` marks. go-multierror's is such an
  error when it holds two or more: its `Unwrap` gives a chain, and each
  chain's `Unwrap` gives the chain of the rest, never an error the chain
  holds, down to the last, which wraps none. Holding one error, its
  `Unwrap` gives that error, so one marked error is a rejection there.
  When the importer rejects the CSV with an error that holds others, it
  marks that error whole. An error reached through more than 100 `Unwrap`
  calls, as in an error whose `Unwrap` leads back to itself, makes the
  whole error a fault.
- In an update, when anything but a `gaps` line keeps a bytes lift's cell
  from being written, such as a fault of the jenny, a product an archive
  can't hold or a `RenderError` that panics, every cell past the lift
  fails, runs nothing, and is not written either: a normal run reads what
  the lift's cell commits. That is the owner's verdict, under its Project.
  A package that only uses the lift doesn't judge the lift's cell with its
  own Project, which may differ from the owner's: it judges the cell under
  a bare Project, whose `RenderError`, `Codes` and `Positions` (step 11)
  fail no cell, and by its own declaration of the lift. When it registers
  the owner's declaration, it holds the cells past the lift back on what
  keeps the cell from being written under any Project, such as a fault of
  the jenny or a product an archive can't hold. The root-package check
  fails when the declarations differ in what the registries record, below;
  it can't compare two jennies. When the owner's Project alone keeps its
  update from writing the lift's cell, as a `RenderError` or a `Positions`
  that panics on the lift's error does, the using package's update writes
  the cells past the lift, and its normal runs, which read the lift's
  committed outcome, fail them until the owner's update writes that cell.
- The lift's own cell files sit in `out/<lift>/`, like any column's. A
  lift's name covers both directories.
- A variant's product goes under `variant/<name>/via/<lift>/` or
  `transform/<name>/via/<lift>/`, and holds the variant's whole product, not
  an overlay on the base's.
- `out/` holds only terminal outputs: nothing downstream reads from it.
- The update of the package that owns the lift replaces `via/<lift>/…`
  whole, at every level: a file the lift no longer produces is removed, and
  the removal shows in the diff. A lift that fails on the case or is
  blocked, or whose variant's transform declines the base, has no product,
  and the update removes its `via/<lift>/`. That package's normal run fails
  on what remains there, and says to run the update, unless the update
  runs the lift on another value, past another bytes lift, as below.
- In an update, the cells past the lift read the product it makes in that
  run, in every package. In a normal run, every package's cells past it
  read the committed `via/` bytes, so churn upstream fails only the
  upstream column, and only the package that owns the lift runs it, in the
  lift's own cell. A downstream column takes `blocked` from the lift's
  committed `outcome`: any outcome but `accepted`, or none, blocks it. A
  lift that is `accepted` with no product files gives its readers an empty
  view.
- A cell past a bytes lift may run in the update on another value than in
  a normal run, since the update reruns every lift on its path, or be
  skipped, blocked or failed there otherwise. When the update runs the
  cell, the cell's committed files, a bytes lift's product among them, are
  no orphans, and what the update writes for the cell, if anything, is its
  run's. A normal run of a package that owns every bytes lift on the
  cell's path stages the case as its update does, on what a transform
  makes for its variant, and runs the cell's probe, if it has one, on what
  that reaches. Where the update does otherwise with the cell, every
  message about the cell ends with what it does, as step 7 gives for a
  transform's variant, a panic of the cell's run or its probe and a frozen
  cell's files among them. So does each message about the lift's own cell,
  a file of its committed product included. Where a lift before every
  bytes lift on the cell's path fails on the case in the update, which
  then reaches none of them, the message names it, and the last bytes
  lift, whose outcome and product a normal run reads from the archive.
  On a transform's variant that the update runs on what the transform
  makes now, where a normal run reads the inputs the archive holds, it
  names the transform for that instead, and no bytes lift.
  When the update writes nothing for the cell of a lift on the cell's
  path, the message says so, names the lift, and says to fix what that
  cell reports, and, on a file the cell commits, to delete nothing by hand
  before the update has run. Another
  package's normal run doesn't run the bytes lifts it doesn't own, so it
  can't tell what its update does with a cell past one: every message
  about such a cell says so, names the lift, says that a normal run reads
  the lift's outcome and product from the archive, and says to run the
  update, fix what it reports, and read the diff before committing, or, on
  a file the cell commits, to delete nothing by hand before the update has
  run. The update may not reach the lift at all, where a lift before it
  fails on the case.
- A cell past a bytes lift that runs on a committed product the lift no
  longer makes, and no longer produces a file it commits, runs on what the
  lift makes in the update, whose run may still write the file. A normal
  run of a package that owns every bytes lift on the cell's path tells
  whether the update runs the cell on another product, and if it does,
  says to delete nothing by hand before the update has run.
- A bytes lift past another bytes lift is such a cell too: in a normal run
  of the package that owns both, it runs on the earlier lift's committed
  product, and its update reruns it on what the earlier lift makes. A file
  of its committed product that its normal run doesn't make, because the
  run made another product or failed on the case, is one the update may
  still make. The normal run says what the update does with the lift's
  cell: if it runs the lift, to delete nothing by hand before it has run;
  if it blocks the lift, that it removes the file; if it writes nothing for
  the earlier lift's cell, to fix what that cell reports, and delete
  nothing by hand before the update has run.
- A bytes lift's frozen cell includes its `via/` product: no update writes
  it, and a product that differs from what the lift makes fails the cell,
  in every run. An update writes the cells past it from what the lift makes
  now. So when the lift's outcome now accepts the case where the committed
  one doesn't, or the other way round, or both accept and the products
  differ, the cells past it fail normal runs, which read the committed
  outcome and product, until a person resolves the frozen cell. Deleting
  the cell's files includes deleting its product. `hypercase strip` leaves
  both in place, and `-frozen` strips both. It reads from the registries
  which columns are bytes lifts: a `via/<column>/…` of a column they say is
  none is part of no cell, so strip removes it even when a `gaps` line
  marks the column's cell. A column they don't name keeps its `via/` with
  its frozen cell, and so does one that any of them says is a bytes lift,
  where they disagree. The root-package check fails on each registry that
  makes them disagree: on the lift, where registries of packages with
  tests record it differently, and on a registry in a package with no
  `_test.go` file, by that registry's own failure.
- The properties a case states hold for every column the case reaches,
  through the lift too. A CSV with a quote that never closes states
  `unterminated`, which `import` rejects, and a CSV whose imported sheet has
  a cycle states `circular`, which `resolve` rejects.
- One type's lift can feed another type's ladder: a `CSV`'s `import` makes a
  sheet. Paths through bytes lifts count toward the one path to each
  consumed type, and a column's path from `csv` in the registry, and so its
  cells' `Path` (step 8), runs through `import`: `resolve`, `parse`, then
  `import`, for `evaluate`. The `format` package, which uses `import` and
  `parse`, reaches `parse` from `csv` through `import`, as the root
  package, which owns both, does: step 6's rule for a lift a package uses.
- A suite reaches across types by the type it consumes: `calc`, which
  consumes `Sheet`, runs on every CSV that `import` accepts.
- Past a bytes lift, a cell's input view, which `RenderError` receives, is
  that lift's product.
- The root-package check claims `via/<lift>/…` in a corpus for each bytes
  lift a package owns over it; any other `via/` section is unclaimed, and
  its failure names `hypercase strip -root <dir> -column <lift>`, then the
  update, as it does for a removed suite (step 5). Where the lift's name
  breaks the naming rule, as in `via/Old/…`, the failure says to delete
  the sections by hand, or to strip with no flags, as for a column of
  such a name. When a `gaps` line marks a cell of those sections and the
  registries disagree on whether the lift is a bytes lift, whether strip
  leaves them turns on that, so the failure names no strip: it says to
  settle the registries first, then run the check again.
- The root-package check fails when the registries of packages with tests
  that own or use a lift record it differently, any lift: its
  description, input, product, `bytes`, outputs, `productFiles` or
  rejects. It fails once per lift, naming the packages and every field
  that differs. A registry in a package with no `_test.go` file isn't
  compared: no test runs what it records, and its own failure says to
  delete it. When the packages register one declaration already, a
  registry is stale, and the update, which rewrites the registries, ends
  the failure. The comparisons of properties and corpus types (steps 5 and
  6) say so too.
- A corpus check (step 8) receives a bytes lift's committed product as its
  cell's `Product`, by each file's path under `via/<lift>/`.
- The report (step 9) shows a bytes lift's product as files of the lift's
  cell, after the cell's own files, each as `via/<lift>/` and its path in
  the product: `via/import/sheet/A1`, under the same name in a variant. So
  a change to the product alone ranks the lift's cell bytes changed, and
  each cell past the lift ranks on its own files. The lift's product
  declarations are described with its outputs, each as `via/<lift>/` and
  its pattern. A `via/<lift>/` section whose lift no registry declares as a
  bytes lift over its corpus is a problem, which the report leaves out, so
  it compares no product of the section's cell between the revisions. A
  product with no `outcome` in its cell is a problem too, as step 9's
  files with no `outcome` are, named by the cell's `via/<lift>/`.
- `testdata/sheet/prices.txtar` and `testdata/csv/prices.txtar` both reach
  `evaluate`, the second through `import`. Their cases are `sheet/prices` and
  `csv/prices` (step 3), and their cells' subtests
  `TestCorpus/sheet/prices/evaluate` and `TestCorpus/csv/prices/evaluate`.

### What can now be seen

- The sheet a CSV yields, as committed bytes beside the CSV. A change to the
  import shows as a change to `via/import/`, which the report ranks on
  `import`'s cell, and fails `import`'s column alone until the update; the
  columns downstream read the committed sheet.
- Where a chained input stops: `open-quote`, `stray-quote` and `past-xfd`
  at `import`, with every column past it `blocked import`.
- Every code the importer defines, reached by some CSV: if no CSV reaches
  `C102` any more, the `codes` check fails.
- A CSV and a hand-written sheet with the same cells side by side in the
  matrix, `csv/prices` beside `sheet/prices`.

## 11. Reverse maps

### What step 10 leaves

The report shows which outputs changed, not which input lines they came
from. A reviewer reading a changed `values.txt` searches the sheet's cells by
hand, and, through a bytes lift, the CSV.

### What the author writes

Every output declaration says which input lines its files come from, in its
`Inputs` field. That map from an output back to its inputs is a **reverse
map**. A **range** is a span of lines in one file of the cell's input view.
The steps before this one show their declarations' `Inputs` without
comment.

Adopting this step means editing every output declaration: each suite's,
`stdout.txt`, `stderr.txt` and `error.txt` included, each lift's `Outputs`,
and each bytes lift's `Product`. Each states one of these (proposed names):

- a `decl.InputsFunc`, a function that maps one output file to its ranges;
- `decl.AllInputs`, which says each file depends on the whole input view;
- `decl.FromPositions`, on a declaration whose pattern is exactly
  `error.txt`, which takes the error's ranges from the project's
  `Positions`.

`walkthrough/sheets/step11/sheetcorpus/inputs.go` holds the toy's maps, and
the `Positions` its `Project` sets, `Positions: positions`:

<!-- source: walkthrough/sheets/step11/sheetcorpus/inputs.go -->
```go
// walkthrough/sheets/step11/sheetcorpus/inputs.go
package sheetcorpus

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
)

// Cells maps an output file to every cell of the sheet, whole: a cell's
// value depends on the cells its formula refers to, and theirs on others.
var Cells = decl.InputsFunc(func(in fs.FS, _ string) ([]decl.Range, error) {
	cells, err := fs.Glob(in, "sheet/*")
	if err != nil {
		return nil, err
	}
	rs := make([]decl.Range, len(cells))
	for i, c := range cells {
		rs[i] = decl.Range{Path: c} // 0, 0: the whole file
	}
	return rs, nil
})

// Formulas maps an output file to the cells that hold a formula, whole:
// the cells format prints.
var Formulas = decl.InputsFunc(func(in fs.FS, _ string) ([]decl.Range, error) {
	p, err := sheet.Parse(in)
	if err != nil {
		return nil, err
	}
	var rs []decl.Range
	for _, name := range p.Formulas() {
		rs = append(rs, decl.Range{Path: "sheet/" + name})
	}
	return rs, nil
})

// Field maps a product file of import, one cell of the sheet, to the lines
// of the CSV export its field spans.
var Field = decl.InputsFunc(func(in fs.FS, path string) ([]decl.Range, error) {
	csvs, err := fs.Glob(in, "csv/*.csv")
	if err != nil {
		return nil, err
	}
	if len(csvs) != 1 {
		return nil, fmt.Errorf("the input holds %q, and a CSV export is one file", csvs)
	}
	first, last, err := sheet.FieldLines(in, csvs[0], strings.TrimPrefix(path, "sheet/"))
	if err != nil {
		return nil, err
	}
	return []decl.Range{{Path: csvs[0], Start: first, End: last}}, nil
})

// positions finds the lines an error points at, sheet.Positions's: each
// line it names, or the whole file for a position about the file as a
// whole, whose Line is 0. Its paths are in the input view already, so it
// doesn't read the view.
func positions(_ fs.FS, err error) []decl.Range {
	var rs []decl.Range
	for _, p := range sheet.Positions(err) {
		rs = append(rs, decl.Range{Path: p.Path, Start: p.Line, End: p.Line})
	}
	return rs
}
```

`evaluate`'s `values.txt` states `Cells`, and `format`'s `formulas.txt`
states `Formulas`. Every `error.txt` states `decl.FromPositions`, and
`calc`'s `stdout.txt` and `stderr.txt` state `decl.AllInputs`.

A bytes lift's product declarations are output declarations too. `import`'s
`Product`, `sheet/*`, states `Field`, which receives each product file's
path as the pattern matches it, such as `sheet/B1`, and gives the lines of
the CSV that cell's field spans, through the toy's `sheet.FieldLines`. A
field spans more than one line when it is quoted and holds a newline. Steps
3 to 10 map at the grain the toy knows by then: step 10's `Export` maps each
product file to the whole CSV.

### How the files change

Every cell that ran has an `inputs` file, `out/<column>/inputs`, and the
same under a variant's prefix, as the archives of steps 3 to 10 show. It
holds, for each of the cell's output files, the archive sections and lines
the file's declaration maps it to. The update writes it with the cell, and
a normal run compares it byte for byte, as any golden. `hypercase.json`
records what each output declaration states in `Inputs` (step 5).

`testdata/sheet/twice.txtar`, with its `cycle` variant's `calc`, `evaluate`
and `parse` cells, and its other variants, left out. A range into a path the
variant replaces maps to the variant's section:

```
A1 is twice B1.
-- in/sheet/A1 --
=B1*2
-- in/sheet/B1 --
3
-- out/calc/inputs --
stdout.txt
  all inputs
-- out/calc/outcome --
accepted
-- out/calc/stdout.txt --
6,3
-- out/evaluate/inputs --
values.txt
  in/sheet/A1
  in/sheet/B1
-- out/evaluate/outcome --
accepted
-- out/evaluate/values.txt --
A1 6
B1 3
-- out/format/formulas.txt --
A1 =B1 * 2
-- out/format/inputs --
formulas.txt
  in/sheet/A1
-- out/format/outcome --
accepted
-- out/parse/inputs --
-- out/parse/outcome --
accepted
-- out/resolve/inputs --
-- out/resolve/outcome --
accepted
-- variant/cycle/about --
B1 refers back to A1, so the sheet has a cycle.
-- variant/cycle/properties --
circular
-- variant/cycle/in/sheet/B1 --
=A1
-- variant/cycle/out/format/formulas.txt --
A1 =B1 * 2
B1 =A1
-- variant/cycle/out/format/inputs --
formulas.txt
  in/sheet/A1
  variant/cycle/in/sheet/B1
-- variant/cycle/out/format/outcome --
accepted
-- variant/cycle/out/resolve/error.txt --
A1 -> B1 -> A1: cycle
  sheet/A1:1: =B1*2
  sheet/B1:1: =A1
-- variant/cycle/out/resolve/inputs --
error.txt
  in/sheet/A1:1-1
  variant/cycle/in/sheet/B1:1-1
-- variant/cycle/out/resolve/outcome --
rejected S201
```

Past a bytes lift, ranges point at the product.
`testdata/csv/wrapped.txtar` is a CSV whose second field the export wrapped
onto a second line:

<!-- source: walkthrough/sheets/step11/testdata/csv/wrapped.txtar -->
```
A price and a formula the export wrapped onto a second line, in a quoted field.
-- in/csv/wrapped.csv --
4,"=A1*
2"
-- properties --
malformed
-- via/import/sheet/A1 --
4
-- via/import/sheet/B1 --
=A1*
2
-- out/calc/error.txt --
B1: more than one line
  sheet/B1:2: 2
-- out/calc/inputs --
error.txt
  via/import/sheet/B1:2-2
-- out/calc/outcome --
rejected S101
-- out/evaluate/outcome --
blocked parse
-- out/format/outcome --
blocked parse
-- out/import/inputs --
via/import/sheet/A1
  in/csv/wrapped.csv:1-1
via/import/sheet/B1
  in/csv/wrapped.csv:1-2
-- out/import/outcome --
accepted
-- out/parse/error.txt --
B1: more than one line
  sheet/B1:2: 2
-- out/parse/inputs --
error.txt
  via/import/sheet/B1:2-2
-- out/parse/outcome --
rejected S101
-- out/resolve/outcome --
blocked parse
```

`parse`'s and `calc`'s `error.txt` point at line 2 of the product's
`sheet/B1`, `via/import/sheet/B1:2-2`. `import`'s own `out/import/inputs`,
with no files of its own to list, maps each product file, named by its
section, to the CSV lines its field spans: `B1`'s spans lines 1 and 2. The
cells blocked at `parse` have no `inputs`.

**The `inputs` format** (proposed).

- For each output file of the cell, in path order, bytewise: a line holding
  the file's path, then one line per range, indented two spaces. A bytes
  lift's product file is named by its section below the case's prefix,
  `via/<lift>/<path>`.
- A range is its archive section and its lines, `in/sheet/A1:1-1`.
  A whole file is its section alone, `in/sheet/A1`. `AllInputs` is
  `all inputs`.
- Ranges are sorted by section, then by first line, then by last. A
  repeated range appears once, overlapping or adjacent line ranges in one
  section are merged, and a whole section absorbs its line ranges.
- An output file with no ranges is its path line alone. So is an
  `error.txt` whose error has no position, and a panic's `error.txt`, which
  has no error for `Positions` to read.
- A path or a section that isn't plain is written as a Go string literal,
  `"like this"`. A name is plain when it is printable ASCII, holds no colon,
  double quote or backslash, neither starts nor ends with a space, doesn't
  start with a hyphen, and isn't `all inputs`.
- A cell that ran and wrote no output files has an empty `inputs`.
- Every line ends with a newline.

### Rules

- Each output declaration states `Inputs`. A map is a
  `func(in fs.FS, path string) ([]Range, error)`, given as
  `decl.InputsFunc`. It works per output file, and receives the cell's input
  view, the one `RenderError` receives; for a bytes lift's own
  declarations, its `Product` included, that is the lift's input view. It
  is called for each file of each run of the cell, so it is a function of
  its arguments alone. `AllInputs` is the explicit escape, which the cell's
  `inputs` and the registry show as such. There is no default.
- Only a declaration whose pattern is exactly `error.txt` states
  `FromPositions`, and never one in a `Product`. One whose pattern matches
  `error.txt` states no `InputsFunc`, so an `error.txt` states
  `FromPositions` or `AllInputs`. Registration refuses the others.
- A declaration that states nothing fails the run, naming it and what it
  can state, as a missing description does (step 3). So does
  `FromPositions` in a project with no `Positions`: every package that
  registers such a declaration, owned or used, supplies
  `Positions func(in fs.FS, err error) []Range`, which finds where an error
  points. It receives the cell's input view, the one `RenderError`
  receives, so it can read what turns a position into a path in the view,
  such as a file that names the root a loader's absolute positions are
  under. It is called for each such `error.txt` on each run of its cell,
  so it is a function of its arguments alone. The cells still run and the
  update writes their outputs. It
  writes no `inputs` for a cell with a file of such a declaration, and
  leaves one the cell has as it is; every other cell gets its own.
- `Range{Path, Start, End}` uses lines counted from 1, `End` included, in
  the cell's input view, and `0, 0` means the whole file.
- A cell has one input view: its case's inputs, with a variant's overlay and
  deletions applied; past a bytes lift, that lift's product, and past
  several, the last one's. A range that names no lines of it is bad: a path
  that isn't a clean slash path, that the view lacks, or that is a
  directory; a negative line; one of `Start` and `End` 0 and not the other;
  an `End` before its `Start`, or past the file's last line.
- The framework maps each range to the section a normal run reads the file
  from: `in/<path>`; a variant's own file, even one equal to the base's, at
  `variant/<name>/in/<path>`; a transform's own file at
  `transform/<name>/in/<path>`, and one it inherits at `in/<path>`; past a
  bytes lift, `via/<lift>/<path>` below the case's prefix.
- A range through a `via/` product stops at the product's lines (proposed):
  it names a `via/<lift>/…` section, and is not followed back through the
  lift's own `inputs`.
- A bad range, an error or a panic in an `InputsFunc`, and a panic in
  `Positions` fail the cell. The failure names the file, what gave its
  ranges and the range, and says what to fix, and the update writes nothing
  for that cell. When the cell is a bytes lift's, the fault is the owner's
  verdict on the lift's cell, as is anything else that keeps that cell from
  being written (step 10). In the owner's update, every cell past the lift
  fails and is not written either. A normal run reads what the lift's cell
  commits: where the archive holds it, the cells past the lift run on the
  committed product, and where it doesn't, as for a case no update has
  written, they fail on the files they lack. A package that only uses the
  lift judges its cell under a bare Project, whose `Positions` finds
  nothing, so a `Positions` that panics holds nothing back there, and that
  package's update writes the cells past the lift.
- A cell with any file that can't be judged, such as an output no
  declaration matches, has its `inputs` neither compared nor written.
- A frozen cell without `inputs` gains it on update, as with `outcome` at
  step 8, when its other files match today's run; an update never changes
  or removes an `inputs` the cell has. So an edit that shifts lines a
  frozen cell maps fails that cell until a person deletes its files.
  Whole-file ranges never shift.
- An `inputs` beside a skipped or blocked cell is an orphan, as any output
  the run no longer produces is (step 3), until a person removes it.
- `hypercase strip` removes `inputs` with its column, keeps it with a
  frozen cell, and removes it under `-frozen`. Stripping a bytes lift's
  column removes its `via/` sections, which other cells' committed `inputs`
  still name, until the update rewrites both.
- The registry records, for each output and product declaration,
  `"inputs"`: `"func"`, `"positions"`, `"all"`, or `"none"` for one that
  states nothing; and `"positions"`, whether the project supplies
  `Positions`. The root-package check compares what each of a lift's
  declarations states across the packages that own or use it, among its
  outputs and `productFiles` (step 10).
- So the report stays a pure function of repository files, and range
  changes show in pull-request diffs. The accepted cost is line-shift churn
  in edited sections.
- The report lists `inputs` among a cell's files, and diffs it as any other
  (step 9). Showing each changed output beside its input lines, and ranking
  a change to ranges alone, is #18.
- Requiring `Inputs` breaks every project's declarations, which is why it
  comes last of the steps that change them.

### What can now be seen

- Which input lines each output came from, in the archive beside it, and in
  a pull request's diff when they move. A changed `values.txt` has the cells
  it depends on beside it.
- Which overlay a variant's output depends on: `cycle`'s `resolve` error
  points at `variant/cycle/in/sheet/B1`, the line the variant changed.
- Through a bytes lift, which CSV lines each cell came from: `wrapped`'s
  `B1` spans lines 1 and 2 of the CSV.
- Where a declaration gives no map, and says so: `calc`'s stdout reads
  `all inputs`.

## 12. Hypotheses

> **Design not settled.** This step is target text. #20 settles its design,
> and #19 decides whether expected errors and hypotheses share one set of
> property names.

### What step 11 leaves

Outcomes, the report and corpus checks say what each cell did. They don't
say why groups of cells move together.

### What the author writes

A **hypothesis** is a claim that relates several runs. Formal methods calls
such a claim a **hyperproperty**, and the `hyper` in the name refers to
that.

These terms are settled:

- A case's **polarity** for a property P is P, anti-P or neither, and it is
  a fact about its input.
- A column's polarity is P, anti-P or neutral, and it is a hypothesis's
  claim.
- P is a property of the case, never of the system under test.
- Neither means no prediction can be made from the case. Neutral is a claim
  of indifference, which a case can't make.

The target text proposes:

- A hypothesis has a name, a description, its property, a polarity for each
  column it lists, and predictions on the outcome axis.
- It is evaluated for one revision as a corpus check, so its violations are
  committed findings that the report shows. Predictions about deltas between
  revisions come later.
- A column's `Rejects` entry reads as a P column whose prediction is that
  matching cases are rejected. Every run already checks it.
- A hypothesis carries a description, under step 3's rule.

For illustration, `circular` is P for a sheet that parses and has a cycle,
and anti-P for one that parses, has no cycle and refers to no missing cell.
Every other sheet is neither: one that doesn't parse never reaches
`resolve`, and `resolve` rejects a `dangling` sheet with no cycle for another
reason, so no prediction about cycles can be made from either.

- `resolve` is P: a matching cell is `rejected`, and a mismatching one
  `accepted`. Its `Rejects` entry already states the first half.
- `evaluate` is P: a matching cell is `blocked resolve`.
- `format` is neutral: both cases of a pair, a sheet with formulas and no
  cycle and its `looped` variant, have the same outcome.

### How the files change

A hypothesis evaluated as a corpus check commits its findings at
`<pkg>/testdata/checks/<name>.txt`, as any check does (step 8). Each finding
names the case, the column, the prediction and the committed outcome
(proposed). A change that breaks a claim shows as that file's diff, and in
the report.

### Evidence first

Two or three hypotheses, written by hand as corpus checks over the toy and
over a consuming project's corpus, show what a hypothesis has to say, what a
violation looks like, and where it shows up. The design follows from them.

### What #20 settles

- How hypotheses are evaluated.
- How a predicate over a stage, whose answer depends on the lifts under
  test, is kept from following a regression in them.
- The order of polarity sources, and what happens when two disagree, so a
  statement and a predicate never disagree silently: whether a source that
  disagrees with the one that decided fails the run.
- How a case states anti-P, and whether a case no source applies to is
  neither or a separate state.
- Whether column polarity earns its keep on the outcome axis.
- What a delta prediction is about.
- Whether a prediction that doesn't fit its column's polarity fails
  registration.

### What can now be seen

One claim over many cells. A change that moves many cells either fits the
stated hypotheses, or shows up as their violations, each named by case and
column.
