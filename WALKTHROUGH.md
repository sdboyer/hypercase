# Walkthrough

This walkthrough is hypercase's spec. It starts at a table test, which Go
developers already write, and adds one idea per step. It ends at hypotheses,
claims that relate many runs to each other, whose design is not settled.

A reader can stop at any step and use what it taught. Each step's idea is
useful without the later ones, and hypercase is worth having even if step 12
never pans out.

hypercase is one framework, with everything on. It has no levels to opt into.
A project that upgrades to a later commit carries what later steps add: a
root-package test (step 5), an `outcome` file per cell (step 8), and `Inputs`
or `AllInputs` on every output declaration (step 11). Earlier steps' examples
keep running, and gain what later steps require.

Every step is shown in two toy domains, recipes and spreadsheet formulas,
which step 1 introduces. Steps 1 to 11 show the Go an author writes at that
step, against the API the step adds. A name or format marked "proposed" is not
settled yet.

## 1. A table test

### Where a Go developer already is

A table test is a slice of rows, each with a name, an input and a want, and a
loop that runs each row with `t.Run`. The wants are written in Go. No
framework is involved.

### The toys

The two toys are small Go projects (proposed). Neither domain package knows
anything of hypercase.

- **Recipes.** A recipe card is a text file: a `serves:` line, then one
  ingredient per line, `name: amount unit`, where a count has no unit. The
  package `recipe` parses cards, scales them, tallies them into a shopping
  list in metric units, and extracts a card from a blog post. Its command,
  `shop`, prints a shopping list.
- **Spreadsheet formulas.** A sheet is a directory with one file per cell,
  named for the cell (`A1`, `B1`), holding a number or a formula such as
  `=B1*2`. The package `sheet` parses a sheet, resolves its references,
  evaluates it, prints its formulas back, and imports CSV. Its command,
  `calc`, prints a sheet's values.

Both toys are kept (proposed). Trade-off: two toys double every snippet and
archive. Each meets every need of the twelve steps alone, and two show that
hypercase isn't shaped to one domain.

They live under `walkthrough/` (proposed):

```
walkthrough/
  recipes/              a module: the recipes project
    go.mod
    recipe/             the domain package
    cmd/shop/
    step01/ … step11/   one example package per step
  sheets/               a module: the sheets project, the same shape
    go.mod
    sheet/
    cmd/calc/
    step01/ … step11/
```

- Each toy is a module of its own. Its `go.mod` requires hypercase through
  `replace github.com/sdboyer/hypercase => ../..`, so it builds against the
  checkout, and its dependencies stay out of hypercase's `go.mod`. CI runs
  `go test ./...` at the repository root and at each toy's root.
- Each step's example is a package of its own. From step 3, each step's
  directory also holds that step's corpus (step 5), so no two steps' cases
  meet.
- Trade-off: a module per toy keeps toy dependencies out of hypercase's
  `go.mod`, at the cost of one more `go test` run per toy in CI. Through the
  `replace`, the toys build only against the checkout, never a release.

### What the author writes

```go
// walkthrough/recipes/step01/scale_test.go
package step01

import (
	"testing"

	"github.com/sdboyer/hypercase/walkthrough/recipes/recipe"
)

func TestScale(t *testing.T) {
	tests := []struct {
		name string
		card string
		want string
	}{
		{
			name: "pancakes",
			card: "serves: 2\nflour: 100 g\nmilk: 150 ml\negg: 1\n",
			want: "serves: 4\nflour: 200 g\nmilk: 300 ml\negg: 2\n",
		},
		{
			name: "porridge",
			card: "serves: 1\noats: 50 g\nmilk: 300 ml\n",
			want: "serves: 4\noats: 200 g\nmilk: 1200 ml\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := recipe.Parse([]byte(tt.card))
			if err != nil {
				t.Fatal(err)
			}
			if got := string(r.Scale(4).Format()); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
```

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

Each example is one test file: `walkthrough/recipes/step01/scale_test.go` and
`walkthrough/sheets/step01/evaluate_test.go`.

### What can now be seen

Pass or fail per named row. `go test -run 'TestScale/pancakes' ./step01` runs
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

```go
// walkthrough/recipes/step02/scale_test.go
var update = flag.Bool("update", false, "rewrite the golden files")

func TestScale(t *testing.T) {
	tests := []struct {
		name string
		card string
	}{
		{"pancakes", "serves: 2\nflour: 100 g\nmilk: 150 ml\negg: 1\n"},
		{"porridge", "serves: 1\noats: 50 g\nmilk: 300 ml\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := recipe.Parse([]byte(tt.card))
			if err != nil {
				t.Fatal(err)
			}
			got := r.Scale(4).Format()
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

The sheets test changes the same way. Its rows keep their cells and lose
their wants, and each row's output is `values.Format()`, one line per cell:

```go
// walkthrough/sheets/step02/evaluate_test.go
		t.Run(tt.name, func(t *testing.T) {
			values, err := sheet.EvalCells(tt.cells)
			if err != nil {
				t.Fatal(err)
			}
			got := values.Format()
			golden := filepath.Join("testdata", tt.name+".golden")
			// compared with the golden, or written under -update, as for recipes
		})
```

### How the files change

The wants leave the table, and each row has a golden: `testdata/pancakes.golden`
and `testdata/porridge.golden`; `testdata/twice.golden` and
`testdata/prices.golden`. `go test ./step02 -update` writes them.
`testdata/twice.golden` holds each cell's value, in cell order (row by row,
then column by column):

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
its description, its inputs and its goldens. The **run** is the Go that
answers every archive: it reads a case's inputs and returns the outputs to
compare.

Proposed names: `hypercase.Run`, `hypercase.File`, `hypercase.Register`,
`hypercase.Test`, and `decl.Project` with its `UpdateVar` field. `decl` is
hypercase's package for declarations. It doesn't import `testing`, so a
project's ordinary packages can hold what they declare. Trade-off: a
project's commands can use its declarations, at the cost of a second import
in every test file.

```go
// walkthrough/recipes/step03/recipecorpus/project.go
package recipecorpus

import "github.com/sdboyer/hypercase/decl"

// Project is what the recipes project supplies to hypercase.
var Project = decl.Project{
	UpdateVar: "RECIPES_UPDATE_GOLDEN",
}
```

```go
// walkthrough/recipes/step03/scale_test.go
package step03

import (
	"io/fs"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/walkthrough/recipes/recipe"
	"github.com/sdboyer/hypercase/walkthrough/recipes/step03/recipecorpus"
)

func init() {
	hypercase.Register(recipecorpus.Project, scale)
}

var scale = hypercase.Run{
	Name:        "scale",
	Description: "Scales the recipe card to serve four.",
	Run: func(t testing.TB, in fs.FS) ([]hypercase.File, error) {
		src, err := recipe.ReadCard(in) // the one file under recipe/
		if err != nil {
			t.Fatal(err) // the archive is wrong, not the code under test
		}
		r, err := recipe.Parse(src)
		if err != nil {
			return nil, err
		}
		return []hypercase.File{{Path: "card.txt", Data: r.Scale(4).Format()}}, nil
	},
}

func TestCorpus(t *testing.T) { hypercase.Test(t) }
```

```go
// walkthrough/sheets/step03/sheetcorpus/project.go
var Project = decl.Project{
	UpdateVar: "SHEETS_UPDATE_GOLDEN",
}
```

```go
// walkthrough/sheets/step03/evaluate_test.go
func init() {
	hypercase.Register(sheetcorpus.Project, evaluate)
}

var evaluate = hypercase.Run{
	Name:        "evaluate",
	Description: "Evaluates every cell of the sheet.",
	Run: func(t testing.TB, in fs.FS) ([]hypercase.File, error) {
		values, err := sheet.Eval(in) // the cells under sheet/
		if err != nil {
			return nil, err
		}
		return []hypercase.File{{Path: "values.txt", Data: values.Format()}}, nil
	},
}

func TestCorpus(t *testing.T) { hypercase.Test(t) }
```

### How the files change

Each table row and its golden become one archive. `testdata/pancakes.txtar`:

```
Pancakes for two.
-- in/recipe/pancakes.txt --
serves: 2
flour: 100 g
milk: 150 ml
egg: 1
-- out/scale/card.txt --
serves: 4
flour: 200 g
milk: 300 ml
egg: 2
```

`testdata/twice.txtar`:

```
A1 is twice B1.
-- in/sheet/A1 --
=B1*2
-- in/sheet/B1 --
3
-- out/evaluate/values.txt --
A1 6
B1 3
```

The project tells git that archives are not text, so no checkout rewrites
their line endings. Its `.gitattributes` holds:

```
*.txtar -text
```

### Rules

**The archive.**

- A case is one archive. Its file name, less `.txtar`, is the archive's name,
  which names the case. At this step a package's archives are its own
  `testdata/*.txtar`; step 5 moves them under a corpus root.
- The archive's **comment**, the text before its first `-- name --` line, is
  the case's description, in prose. It is required. It holds no `key: value`
  lines: nothing in it names the case's type or sets a parameter.
- `in/<tree>/…` holds the case's named input trees, such as `in/recipe/` and
  `in/sheet/`. The run sees one **input view**: everything under `in/`, with
  the prefix removed, as an `fs.FS`. `in/recipe/pancakes.txt` is
  `recipe/pancakes.txt` in the view.
- The top level of an archive is the framework's, a closed set: `in`, `out`,
  `via`, `properties`, `gaps`, `variant` and `transform`. Any other top-level
  name fails the run, so a typo such as `ni/recipe/pancakes.txt` can't pass
  unread. Later steps give most of these names their meaning. `params` is
  reserved as a tree name under `in/` (step 6), and `checks` as a directory
  name under `testdata/` (step 8).

**The run.**

- A run has a name and a description, and its outputs are under
  `out/<run>/…`. A **column** is something every case runs through, and a
  **cell** is one case run through one column. Cases and columns make a
  **matrix**. The run is a matrix with one column: step 5 adds columns, and
  the archive's own layout doesn't change.
- What the run does is fixed in Go and said in its description: `scale`
  scales every card to serve four, and no archive says so. An archive carries
  no parameters.
- The run receives a `testing.TB` and the input view, and returns its output
  files and an error. It reports its own failures through the `testing.TB`,
  such as a fixture it can't set up. The error it returns is only what the
  code under test rejected.
- At this step an error fails its case; step 4 lets a case expect one. The
  error is recorded as `out/<run>/error.txt`: its text, with a final newline.
  When the run errs, the outputs it did produce are still compared.
- `error.txt` directly under a column's directory is the framework's, and so
  are the names later steps add there: `stdout.txt` and `stderr.txt`
  (step 5), `outcome` (step 8) and `inputs` (step 11). A run that returns an
  output at one of them fails its case.
- Making output deterministic, such as paths, times or addresses, is the
  run's job. The framework normalizes nothing.
- The run is registered when its package is initialized, before any test
  runs. A test function runs what was registered, and registers nothing.
  Step 5 states the rule for everything a registry lists.

**Comparison.**

- Each output is compared with the golden section of the same name, byte for
  byte. A missing final newline is a difference, and so is a line ending.
- An output with no golden fails, and an update writes it.

**Updates.**

- Updates use one environment variable, whose name the project supplies as
  `UpdateVar`. One variable covers every corpus in the project.
- An update is the whole module's: `RECIPES_UPDATE_GOLDEN=1 go test ./...`,
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
- `hypercase strip` (#16) removes update-written sections, which it
  recognizes by name, under the corpus root's lock (step 5). A person runs
  it, never an update. With no flags it strips every archive under the corpus
  root, and the registry and findings files (steps 5 and 8). `-column <name>`
  and `-transform <name>` limit it. It leaves frozen cells (step 4) in place
  and lists them, and `-frozen` strips those too.
- Stripping and then updating, repeated until an update changes nothing,
  reaches a known state.

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
  hyphens, starting with a letter: `pancakes`, `shopping-list`. The rule
  covers archive and column names here, and property names (step 4), corpus
  directories (step 6), variant and transform names (step 7) and check names
  (step 8). Such a name never collides with another on a case-insensitive
  filesystem, never needs quoting in `-run`, and never holds the space that
  ends a column's name in a `gaps` line (step 4).
- A project can't narrow the rule. A parameter derived from an archive's
  name (step 6) follows a convention that accepts every name the rule
  admits.
- A cell's subtest is named `<case>/<column>`: `TestCorpus/pancakes/scale`.
  `go test -run 'TestCorpus/pancakes/'` runs one case's cells.
- Trade-off: one strict rule everywhere keeps names safe on every
  filesystem and in every `-run` pattern, at the cost of names that can't
  carry case or punctuation. A column that derives a parameter from an
  archive's name has to accept every name the rule admits.

**Descriptions.**

- The archive's comment and the run's description are required. A missing
  description fails the run, but does not stop an update from writing
  outputs. Each later step names what else needs a description, under the
  same rule.

### What can now be seen

- A case and what it produced, in one file. A change to a case is a diff of
  one file, which a reviewer reads in one place.
- One command updates every corpus in the module.
- A failure names the case and the column, prints both descriptions, and ends
  with what to do. For illustration:

  ```
  --- FAIL: TestCorpus/pancakes/scale
      pancakes: Pancakes for two.
      scale: Scales the recipe card to serve four.
      out/scale/card.txt differs (-golden +got):
        serves: 4
      - flour: 200 g
      + flour: 200.0 g
        milk: 300 ml
        egg: 2
      If the change is intended, run RECIPES_UPDATE_GOLDEN=1 go test ./... and read the diff before committing.
  ```

**When to add an archive** (proposed): when an input makes a point no
existing archive makes. A further behavior of an existing input is another
output of the run (from step 5, perhaps another column), never a copy of the
archive. From step 7, an input that is an existing one with a file or two
changed is a variant of it. Trade-off: the corpus stays small and every
column sees every input, at the cost of a judgment call about what counts as
a new point.

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
Go, states on each case the properties it has, and lists on the run the
properties it rejects.

Proposed names: `decl.Property`, and the run's `Rejects` field.

```go
// walkthrough/recipes/step04/recipecorpus/properties.go
var Garbled = decl.Property{
	Name:        "garbled",
	Description: "Some line after serves: is not an ingredient, name: amount unit.",
}
```

```go
// walkthrough/recipes/step04/scale_test.go
func init() {
	hypercase.Register(recipecorpus.Project, recipecorpus.Garbled, scale)
}

var scale = hypercase.Run{
	Name:        "scale",
	Description: "Scales the recipe card to serve four.",
	Rejects:     []decl.Property{recipecorpus.Garbled},
	Run: func(t testing.TB, in fs.FS) ([]hypercase.File, error) {
		// as in step 3
	},
}
```

```go
// walkthrough/sheets/step04/sheetcorpus/properties.go
var Circular = decl.Property{
	Name:        "circular",
	Description: "Some cell's formula refers back to itself, directly or through other cells.",
}
```

```go
// walkthrough/sheets/step04/evaluate_test.go
func init() {
	hypercase.Register(sheetcorpus.Project, sheetcorpus.Circular, evaluate)
}

var evaluate = hypercase.Run{
	Name:        "evaluate",
	Description: "Evaluates every cell of the sheet.",
	Rejects:     []decl.Property{sheetcorpus.Circular},
	Run: func(t testing.TB, in fs.FS) ([]hypercase.File, error) {
		// as in step 3
	},
}
```

### How the files change

An archive gains a `properties` section, a `gaps` section, or both. Both are
hand-written. `testdata/crepes.txtar` is meant to be rejected:

```
A line of method among the ingredients.
-- in/recipe/crepes.txt --
serves: 2
flour: 100 g
whisk until smooth
milk: 250 ml
-- properties --
garbled
-- out/scale/error.txt --
line 3: not an ingredient: "whisk until smooth"
```

`testdata/loop.txtar`:

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
```

`testdata/omelette.txtar` records a known gap whose output is wrong:

```
An omelette for three, with one egg.
-- in/recipe/omelette.txt --
serves: 3
egg: 1
butter: 15 g
-- gaps --
scale A count scales to a whole number, so egg: 1 for four. Waits on recipes#2.
-- out/scale/card.txt --
serves: 4
egg: 1.3333333333333333
butter: 20 g
```

`testdata/negative.txtar` records one that errs when it shouldn't:

```
A1 is a negative number.
-- in/sheet/A1 --
-3
-- gaps --
evaluate A negative number is valid, and evaluates to itself. Waits on sheets#3.
-- out/evaluate/error.txt --
A1: unexpected "-"
```

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
- The run lists the properties it rejects, in Go, as `Rejects`.

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
  only when the cell's existing files match today's run. It never changes or
  removes a file the cell has.
- A person resolves a failing frozen cell: remove the line (the gap is
  closed, and the next update writes the cell), or delete the cell's files
  (the wrong behavior changed shape, and the next update writes them).
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
  --- FAIL: TestCorpus/loop/evaluate
      loop: A1 and B1 refer to each other.
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

One run answers every archive, through one function. A project tests more
than one function or command over the same inputs: a recipe card is scaled,
tallied into a shopping list, and given to the `shop` command; a sheet is
evaluated, printed back, and given to `calc`. With one run per package, each
of those keeps its own copy of every input. And what a command prints to
stdout and stderr has nowhere to go.

### What the author writes

Columns are **suites**. A suite covers a class of outputs, or one command. It
has a name, a description, a probe, its declared outputs, `Rejects` and a
run. A **probe** decides whether a case is for the suite.

A suite runs over a **corpus**, a directory of archives under the **corpus
root**. Packages over one corpus root share its archives, and each writes
its own columns' sections. One package per corpus root registers the
**root-package check**, which reads every package's registry and every
archive, and reports what no column owns.

Proposed names: `hypercase.Suite` with its `Corpus`, `Probe`, `Outputs`,
`Rejects` and `Run` fields; `decl.Output`; `hypercase.Root`, the option that
passes a corpus root as a path relative to the module root; and
`hypercase.RootPackage`, which registers the root-package check.

The walkthrough's examples pass their own directory as the corpus root, so
each step's corpus stands alone. A project with one corpus at its module
root passes nothing.

```go
// walkthrough/recipes/step05/recipecorpus/properties.go gains:
var Vague = decl.Property{
	Name:        "vague",
	Description: "Some ingredient is measured in a unit with no fixed size, such as a handful.",
}
```

```go
// walkthrough/recipes/step05/corpus_test.go: the corpus root's package, with scale and shopping-list
func init() {
	hypercase.Register(
		recipecorpus.Project,
		hypercase.Root("step05"),
		hypercase.RootPackage(),
		recipecorpus.Garbled, recipecorpus.Vague,
		scale, shoppingList,
	)
}

var scale = hypercase.Suite{
	Name:        "scale",
	Description: "Scales the recipe card to serve four.",
	Corpus:      "recipe", // testdata/recipe/*.txtar
	Probe: func(in fs.FS) (why string, ok bool) {
		if recipe.Serves(in) == 4 {
			return "already serves 4", false
		}
		return "", true
	},
	Outputs: []decl.Output{
		{Pattern: "card.txt", Description: "The card, scaled to serve four."},
		{Pattern: "error.txt", Description: "Why the card doesn't parse."},
	},
	Rejects: []decl.Property{recipecorpus.Garbled},
	Run: func(t testing.TB, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
		// as in step 3
	},
}

var shoppingList = hypercase.Suite{
	Name:        "shopping-list",
	Description: "Tallies the card's ingredients into a shopping list in metric units.",
	Corpus:      "recipe",
	Outputs: []decl.Output{
		{Pattern: "list.txt", Description: "What to buy, one ingredient per line, in metric units."},
		{Pattern: "error.txt", Description: "Why the card can't be tallied."},
	},
	Rejects: []decl.Property{recipecorpus.Garbled, recipecorpus.Vague},
	Run: func(t testing.TB, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
		src, err := recipe.ReadCard(in)
		if err != nil {
			t.Fatal(err)
		}
		r, err := recipe.Parse(src)
		if err != nil {
			return nil, err
		}
		list, err := recipe.Tally(r)
		if err != nil {
			return nil, err
		}
		return []hypercase.File{{Path: "list.txt", Data: list.Format()}}, nil
	},
}

func TestCorpus(t *testing.T) { hypercase.Test(t) }
```

```go
// walkthrough/recipes/step05/shop/corpus_test.go: the shop command
func init() {
	hypercase.Register(
		recipecorpus.Project,
		hypercase.Root("step05"),
		recipecorpus.Garbled,
		shop,
	)
}

var shop = hypercase.Suite{
	Name:        "shop",
	Description: "Runs the shop command on the card.",
	Corpus:      "recipe",
	Outputs: []decl.Output{
		{Pattern: "stdout.txt", Description: "The shopping list the command prints."},
		{Pattern: "stderr.txt", Description: "A warning for each ingredient the command leaves off the list."},
		{Pattern: "error.txt", Description: "Why the command failed."},
	},
	Rejects: []decl.Property{recipecorpus.Garbled},
	Run: func(t testing.TB, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
		return nil, recipe.Shop(in, stdout, stderr)
	},
}

func TestCorpus(t *testing.T) { hypercase.Test(t) }
```

The sheets example has three packages over one corpus root: the root package
owns `evaluate`, `format/` owns `format`, and `cli/` owns `calc`.

```go
// walkthrough/sheets/step05/sheetcorpus/properties.go gains:
var Malformed = decl.Property{
	Name:        "malformed",
	Description: "Some cell holds neither a number nor a formula.",
}

var Dangling = decl.Property{
	Name:        "dangling",
	Description: "Some formula refers to a cell the sheet doesn't have.",
}
```

```go
// walkthrough/sheets/step05/corpus_test.go: the corpus root's package, with evaluate
func init() {
	hypercase.Register(
		sheetcorpus.Project,
		hypercase.Root("step05"),
		hypercase.RootPackage(),
		sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling,
		evaluate,
	)
}

var evaluate = hypercase.Suite{
	Name:        "evaluate",
	Description: "Evaluates every cell of the sheet.",
	Corpus:      "sheet",
	Outputs: []decl.Output{
		{Pattern: "values.txt", Description: "Each cell's value, one per line, in cell order."},
		{Pattern: "error.txt", Description: "Why the sheet can't be evaluated."},
	},
	Rejects: []decl.Property{sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling},
	Run: func(t testing.TB, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
		// as in step 3
	},
}

func TestCorpus(t *testing.T) { hypercase.Test(t) }
```

```go
// walkthrough/sheets/step05/format/corpus_test.go registers sheetcorpus.Project,
// hypercase.Root("step05"), sheetcorpus.Malformed and format.
var format = hypercase.Suite{
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
		{Pattern: "formulas.txt", Description: "Each formula cell and its formula, in cell order."},
		{Pattern: "error.txt", Description: "Why the sheet doesn't parse."},
	},
	Rejects: []decl.Property{sheetcorpus.Malformed},
	Run: func(t testing.TB, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
		p, err := sheet.Parse(in)
		if err != nil {
			return nil, err
		}
		return []hypercase.File{{Path: "formulas.txt", Data: sheet.Format(p)}}, nil
	},
}
```

```go
// walkthrough/sheets/step05/cli/corpus_test.go registers sheetcorpus.Project,
// hypercase.Root("step05"), the three properties and calc.
var calc = hypercase.Suite{
	Name:        "calc",
	Description: "Runs the calc command on the sheet.",
	Corpus:      "sheet",
	Outputs: []decl.Output{
		{Pattern: "stdout.txt", Description: "The sheet's values as CSV, one row per line."},
		{Pattern: "stderr.txt", Description: "A warning for each cell that divides by zero."},
		{Pattern: "error.txt", Description: "Why calc failed."},
	},
	Rejects: []decl.Property{sheetcorpus.Malformed, sheetcorpus.Circular, sheetcorpus.Dangling},
	Run: func(t testing.TB, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
		return nil, sheet.Calc(in, stdout, stderr)
	},
}
```

### How the files change

- Archives move from a package's `testdata/` to the corpus root's
  `testdata/<corpus>/`: `step05/testdata/recipe/broth.txtar`,
  `step05/testdata/sheet/twice.txtar`. The step 3 and step 4 examples'
  archives move the same way, and each of their runs becomes a suite: it
  gains `Corpus`, output declarations and the two writers.
- An archive holds every column's sections, each column under its own
  `out/<suite>/`.
- Each package writes its registry, `testdata/hypercase.json`, in its own
  directory.
- The project's `.gitignore` gains `.hypercase.lock`.

`testdata/recipe/broth.txtar`. `shop` warns about the handful and goes on;
`shopping-list` rejects it:

```
Broth finished with a handful of parsley.
-- in/recipe/broth.txt --
serves: 2
stock: 500 ml
onion: 1
parsley: 1 handful
-- properties --
vague
-- out/scale/card.txt --
serves: 4
stock: 1000 ml
onion: 2
parsley: 2 handful
-- out/shop/stderr.txt --
parsley: left off: a handful has no fixed size
-- out/shop/stdout.txt --
onion: 1
stock: 500 ml
-- out/shopping-list/error.txt --
line 4: parsley: a handful has no fixed size
```

`testdata/sheet/twice.txtar`:

```
A1 is twice B1.
-- in/sheet/A1 --
=B1*2
-- in/sheet/B1 --
3
-- out/calc/stdout.txt --
6,3
-- out/evaluate/values.txt --
A1 6
B1 3
-- out/format/formulas.txt --
A1 =B1 * 2
```

`testdata/sheet/stray.txtar`. Every column reads the raw cells, so each one
meets the malformed cell and records the same error:

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
-- out/evaluate/error.txt --
D1: not a number or a formula: "total"
-- out/format/error.txt --
D1: not a number or a formula: "total"
```

The root package's registry, `step05/testdata/hypercase.json`, sketched
(proposed):

```json
{
  "format": 1,
  "updateVar": "SHEETS_UPDATE_GOLDEN",
  "corpusRoot": "step05",
  "rootPackage": true,
  "properties": [
    {"name": "circular", "description": "Some cell's formula refers back to itself, directly or through other cells."},
    {"name": "dangling", "description": "Some formula refers to a cell the sheet doesn't have."},
    {"name": "malformed", "description": "Some cell holds neither a number nor a formula."}
  ],
  "suites": [
    {
      "name": "evaluate",
      "description": "Evaluates every cell of the sheet.",
      "corpus": "sheet",
      "outputs": [
        {"pattern": "error.txt", "description": "Why the sheet can't be evaluated."},
        {"pattern": "values.txt", "description": "Each cell's value, one per line, in cell order."}
      ],
      "rejects": ["circular", "dangling", "malformed"]
    }
  ]
}
```

### Rules

**Suites and cells.**

- Columns are suites. Step 3's run is a suite with one column from here on.
  A suite covers a class of outputs, or one command. A cell is one case run
  through one column, and its outputs go under `out/<suite>/…`.
- A suite has a name, a description, a probe, its declared outputs,
  `Rejects` and a run. The run receives a `testing.TB`, the input view, and
  one `io.Writer` each for stdout and stderr, compared as the `stdout.txt`
  and `stderr.txt` goldens. Each of those files exists when the run wrote to
  its stream. The process's `os.Stdout` is never replaced.
- Column names are unique per corpus root, not per package or per corpus
  type.
- A case's name gains its corpus's directory (proposed): the case
  `recipe/broth`, and the cell subtest `TestCorpus/recipe/broth/scale`. Two
  archives with one name in different corpora are two cases with two names
  (step 10). Trade-off: no pair of archives has to be refused, at the cost
  of a longer name for every case.

**Probes.**

- A probe decides, from the case's input, whether the case is for the
  suite. It receives the input view and nothing else. When it says no, it
  gives the reason. A probe that says no without one fails the cell. A suite
  with no probe is for every case.
- A cell whose probe says no is **skipped**: it runs nothing.
- A suite that applies to no case fails the run, a suite whose corpus has no
  archives included.
- There is no way to skip a cell by name. Probes decide applicability.

**Declared outputs.**

- Every output is declared with a description, `stdout.txt`, `stderr.txt`
  and `error.txt` included. An undeclared output fails the cell.
- A declaration's pattern matches paths under the column's directory. A file
  that two declarations match fails the cell, naming both.

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

- Everything a registry lists is registered when its package is
  initialized, before any test runs: properties, corpus types, lifts,
  suites, transforms, corpus checks and the root-package check. A test
  function runs what was registered, and registers nothing. So the registry
  and the orphan checks don't depend on `-run`.
- A refusal at registration, such as two properties with one name, panics
  during package initialization with the message the framework would
  otherwise print. So it doesn't depend on `-run` either.

**The registry.** It is each package's `testdata/hypercase.json`.

- It lists everything the package registers, each with its description, the
  properties each column rejects, and each column's corpus root and corpus.
- It records the name of the project's update variable, so `hypercase strip`
  and `hypercase report` can print the update command.
- It carries a format version, which changes on any change to what the
  registry holds.
- It is compared like a golden. An update writes it by temp file and rename.
- The report and other tools read it instead of parsing Go. Each later step
  adds to it.

**Shared corpora.**

- Corpora live under the corpus root, the module root unless the project
  passes another, at `testdata/<corpus>/*.txtar`. At this step a suite names
  its corpus by a directory under `testdata/`; step 6 makes that the corpus
  type's directory.
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
  - one property name with two different entries;
  - a section no registry claims, such as `out/nutrition/…` after that suite
    was removed;
  - a problem of an archive as a whole: a missing description, a top-level
    name outside the closed set, hand-written sections that don't sit
    together, a `properties`
    line naming a property nothing declares, a `gaps` line naming a column
    no registry has;
  - a registry file whose directory holds no `_test.go` file;
  - a directory under `testdata/` that holds archives and that no registry
    declares.
- A declared directory that doesn't exist yet is fine.
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

**Descriptions.** Each suite and every output declaration, `stdout.txt`,
`stderr.txt` and `error.txt` included, carries a description, under step 3's
rule.

### What can now be seen

- The matrix. One archive shows every column's outputs for its case, and a
  change shows, in the diff that causes it, how it moves other columns'
  outputs.
- Which column rejects what: `broth` is accepted by `scale` and `shop` and
  rejected by `shopping-list`, each stated in Go.
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
a lift reaches. Each lift is a column. A suite declares the type it
consumes, a stage or a corpus type, and the lifts that reach it run first.

The recipes ladder is `RecipeCard → *recipe.Recipe → *recipe.ShoppingList`,
through the lifts `parse` and `tally`. The sheets ladder is
`Sheet → *sheet.Parsed → *sheet.Graph`, through `parse` and `resolve`.

Proposed names: `decl.Type`, `decl.Lift`, `hypercase.Suite[T]`, whose type
argument is the type the suite consumes, `hypercase.Own`, `hypercase.Use`,
and `decl.Project`'s `RenderError` field. A suite's run receives the
archive's name from this step. Its probe receives the input only.

```go
// walkthrough/sheets/step06/sheetcorpus/types.go
type Sheet struct{ fs.FS } // in/sheet/A1, in/sheet/B1, …

var SheetType = decl.Type[Sheet]{
	Dir:         "sheet", // testdata/sheet/*.txtar
	Description: "A spreadsheet: one file per cell under in/sheet/, holding a number or a formula.",
}

var Parse = decl.Lift[Sheet, *sheet.Parsed]{
	Name:        "parse",
	Description: "Parses every cell.",
	Outputs: []decl.Output{
		{Pattern: "error.txt", Description: "The cell that is neither a number nor a formula."},
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
		{Pattern: "error.txt", Description: "The cycle, or the reference to a missing cell."},
	},
	Rejects: []decl.Property{Circular, Dangling},
	Run: func(archive string, p *sheet.Parsed, stderr io.Writer) (*sheet.Graph, error) {
		return sheet.Resolve(p)
	},
}
```

```go
// walkthrough/sheets/step06/sheetcorpus/project.go
var Project = decl.Project{
	UpdateVar:   "SHEETS_UPDATE_GOLDEN",
	RenderError: renderError,
}

// renderError writes the error's text, then quotes each line it points at.
func renderError(in fs.FS, err error) []byte {
	out := []byte(err.Error() + "\n")
	for _, p := range sheet.Positions(err) {
		out = fmt.Appendf(out, "  %s:%d: %s\n", p.Path, p.Line, lineAt(in, p.Path, p.Line))
	}
	return out
}
```

```go
// walkthrough/sheets/step06/corpus_test.go: owns both lifts and evaluate
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
		{Pattern: "values.txt", Description: "Each cell's value, one per line, in cell order."},
	},
	Run: func(t testing.TB, archive string, g *sheet.Graph, stdout, stderr io.Writer) ([]hypercase.File, error) {
		return []hypercase.File{{Path: "values.txt", Data: g.Evaluate().Format()}}, nil
	},
}

func TestCorpus(t *testing.T) { hypercase.Test(t) }
```

```go
// walkthrough/sheets/step06/format/corpus_test.go: uses parse, owns format
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
		{Pattern: "formulas.txt", Description: "Each formula cell and its formula, in cell order."},
	},
	Run: func(t testing.TB, archive string, p *sheet.Parsed, stdout, stderr io.Writer) ([]hypercase.File, error) {
		return []hypercase.File{{Path: "formulas.txt", Data: sheet.Format(p)}}, nil
	},
}
```

`calc`, in `cli/`, consumes the corpus type itself, since it tests a command
on raw input: `hypercase.Suite[sheetcorpus.Sheet]`, with its outputs and
`Rejects` as in step 5.

```go
// walkthrough/recipes/step06/recipecorpus/types.go
type RecipeCard struct{ fs.FS } // in/recipe/<archive>.txt

var CardType = decl.Type[RecipeCard]{
	Dir:         "recipe",
	Description: "A recipe card: in/recipe/<archive>.txt, a serves line, then one ingredient per line.",
}

var Parse = decl.Lift[RecipeCard, *recipe.Recipe]{
	Name:        "parse",
	Description: "Parses the card named after the archive.",
	Outputs: []decl.Output{
		{Pattern: "error.txt", Description: "The line that isn't an ingredient."},
	},
	Rejects: []decl.Property{Garbled},
	Run: func(archive string, in RecipeCard, stderr io.Writer) (*recipe.Recipe, error) {
		return recipe.ParseFile(in, "recipe/"+archive+".txt")
	},
}

var Tally = decl.Lift[*recipe.Recipe, *recipe.ShoppingList]{
	Name:        "tally",
	Description: "Tallies the ingredients into a shopping list in metric units.",
	Outputs: []decl.Output{
		{Pattern: "error.txt", Description: "The ingredient whose unit has no fixed size."},
	},
	Rejects: []decl.Property{Vague},
	Run: func(archive string, r *recipe.Recipe, stderr io.Writer) (*recipe.ShoppingList, error) {
		return recipe.Tally(r)
	},
}
```

```go
// walkthrough/recipes/step06/corpus_test.go: owns both lifts, scale and shopping-list
func init() {
	hypercase.Register(
		recipecorpus.Project, // with a renderError of the same shape as the sheets'
		hypercase.Root("step06"),
		hypercase.RootPackage(),
		recipecorpus.Garbled, recipecorpus.Vague,
		recipecorpus.CardType,
		hypercase.Own(recipecorpus.Parse, recipecorpus.Tally, scale, shoppingList),
	)
}

var scale = hypercase.Suite[*recipe.Recipe]{
	Name:        "scale",
	Description: "Scales the recipe to serve four.",
	Probe: func(r *recipe.Recipe) (why string, ok bool) {
		if r.Serves == 4 {
			return "already serves 4", false
		}
		return "", true
	},
	Outputs: []decl.Output{
		{Pattern: "card.txt", Description: "The card, scaled to serve four."},
	},
	Run: func(t testing.TB, archive string, r *recipe.Recipe, stdout, stderr io.Writer) ([]hypercase.File, error) {
		return []hypercase.File{{Path: "card.txt", Data: r.Scale(4).Format()}}, nil
	},
}

var shoppingList = hypercase.Suite[*recipe.ShoppingList]{
	Name:        "shopping-list",
	Description: "Prints the shopping list.",
	Outputs: []decl.Output{
		{Pattern: "list.txt", Description: "What to buy, one ingredient per line, in metric units."},
	},
	Run: func(t testing.TB, archive string, l *recipe.ShoppingList, stdout, stderr io.Writer) ([]hypercase.File, error) {
		return []hypercase.File{{Path: "list.txt", Data: l.Format()}}, nil
	},
}

func TestCorpus(t *testing.T) { hypercase.Test(t) }
```

`shop`, in `shop/`, is a `hypercase.Suite[recipecorpus.RecipeCard]`, and
still rejects `garbled`: the command parses the card itself.

### How the files change

- The ordinary package gains the corpus type and the lifts. Each suite loses
  `Corpus`, gains its consumed type, and lists only what it rejects itself.
- The step 3 to 5 examples gain a corpus type each, and no archive moves.
- A malformed input's error moves to the lift that rejects it. The cells
  past that lift are blocked and write nothing, so their old sections are
  orphans until a person removes them: `stray`'s `out/evaluate/error.txt`
  and `out/format/error.txt`, and `broth`'s `out/shopping-list/error.txt`.
- `error.txt` holds what `RenderError` writes.
- `hypercase.json` gains each corpus type, each lift with the properties it
  rejects and whether the package owns it, and each suite's consumed type.

`testdata/sheet/stray.txtar`. `parse` rejects the sheet once; `calc` still
reads the raw cells:

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
-- out/parse/error.txt --
D1: not a number or a formula: "total"
  sheet/D1:1: total
```

`testdata/recipe/broth.txtar`. `tally` rejects the handful, and
`shopping-list`, past it, is blocked:

```
Broth finished with a handful of parsley.
-- in/recipe/broth.txt --
serves: 2
stock: 500 ml
onion: 1
parsley: 1 handful
-- properties --
vague
-- out/scale/card.txt --
serves: 4
stock: 1000 ml
onion: 2
parsley: 2 handful
-- out/shop/stderr.txt --
parsley: left off: a handful has no fixed size
-- out/shop/stdout.txt --
onion: 1
stock: 500 ml
-- out/tally/error.txt --
line 4: parsley: a handful has no fixed size
  recipe/broth.txt:4: parsley: 1 handful
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
  and `dangling`, a reference to a missing cell. In the recipes, `parse`
  rejects `garbled` and `tally` rejects `vague`.
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
- There must be exactly one path to each consumed type, or the run fails
  before any case runs.
- Probes are distinct from corpus types: the type decides what a suite
  receives, and the probe decides whether a case is for it.
- A suite's probe receives the value of the type the suite consumes, and
  nothing else. Its run receives the archive's name too, and the stdout and
  stderr writers.

**Ownership** (proposed). This step refines step 5's rules, under which
every registration owns.

- From here, a package owns the columns it registers with `Own`, and writes
  and judges their sections.
- `Use` takes lifts, and registers a lift only so its product is computed
  in process. A package that uses a lift neither writes nor judges the
  lift's sections.
- The root-package check fails on a column two packages own, in place of
  step 5's column two packages register, and on a lift that packages use
  and none owns.

**Blocked cells.**

- When a lift on a cell's path fails, the cell is **blocked**: it writes no
  outputs, and its old outputs are orphans.
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
  `Positions` and `Range` at step 11.

**Parameters.**

- Parameters are fixed in Go, or derived from the archive's name by a fixed
  convention: the recipes' `parse` reads `recipe/<archive>.txt`.
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

- Where on its ladder an input stops. `stray` is rejected once, by `parse`,
  and `broth` once, by `tally`; nothing past them is expected to do
  anything.
- What each column consumes, in the registry: `evaluate` reads a
  `*sheet.Graph`, and `calc` the raw sheet.
- Each error, quoting the input it points at.

## 7. Variants and transforms

### What step 6 leaves

A case that differs from another by one line is a copy of the whole archive,
and nothing says which line matters. And each invalid case is written by
hand, though most are a valid case with one mechanical change: a cycle added,
or an ingredient with no fixed size.

### What the author writes

An archive holds several cases: the **base**, which is `in/…` and which the
comment describes, and **variants** on it. A variant changes a file or two of
the base. A hand-written variant is written in the archive. A **transform** is
Go code that makes a variant from the base, and the update writes its
variant's sections.

Proposed names: `decl.Transform` with its `Name`, `Description`, `Gives`,
`Keeps`, `Probe` and `Run` fields, and `decl.Type`'s `Transforms` field. The
variant prefixes `variant/<name>/` and `transform/<name>/` are proposed.
Trade-off: the prefix says whether a person or an update wrote the variant,
at the cost of two reserved top-level names and longer section paths.

```go
// walkthrough/sheets/step07/sheetcorpus/types.go
var Looped = decl.Transform[Sheet]{
	Name:        "looped",
	Description: "The first cell, by name, replaced by a reference to itself, so the sheet has a cycle.",
	Gives:       []decl.Property{Circular},
	Probe: func(base Sheet) (why string, ok bool) {
		// declines a sheet whose first cell already refers to itself
	},
	Run: func(archive string, base Sheet) (fs.FS, error) {
		// the base, with its first cell replaced
	},
}

var Renamed = decl.Transform[Sheet]{
	Name:        "renamed",
	Description: "Every cell moved one row down, A1 to A2, with every reference rewritten to the moved cell.",
	Keeps:       []decl.Property{Circular, Dangling},
	Probe: func(base Sheet) (why string, ok bool) {
		// declines an empty sheet
	},
	Run: func(archive string, base Sheet) (fs.FS, error) {
		return sheet.MoveDown(base)
	},
}

var SheetType = decl.Type[Sheet]{
	Dir:         "sheet",
	Description: "A spreadsheet: one file per cell under in/sheet/, holding a number or a formula.",
	Transforms:  []decl.Transform[Sheet]{Looped, Renamed},
}
```

```go
// walkthrough/recipes/step07/recipecorpus/types.go
var Garnished = decl.Transform[RecipeCard]{
	Name:        "garnished",
	Description: "Adds parsley: 1 handful, a unit with no fixed size, as the card's last line.",
	Gives:       []decl.Property{Vague},
	Probe: func(base RecipeCard) (why string, ok bool) {
		// reads the one file under recipe/, and declines a card that already lists parsley
	},
	Run: func(archive string, base RecipeCard) (fs.FS, error) {
		// the base, with one more line in recipe/<archive>.txt
	},
}

var Doubled = decl.Transform[RecipeCard]{
	Name:        "doubled",
	Description: "Doubles serves and every amount, so the card makes twice as much.",
	Keeps:       []decl.Property{Vague},
	Run: func(archive string, base RecipeCard) (fs.FS, error) {
		// the base, with every number in recipe/<archive>.txt doubled
	},
}

var CardType = decl.Type[RecipeCard]{
	Dir:         "recipe",
	Description: "A recipe card: in/recipe/<archive>.txt, a serves line, then one ingredient per line.",
	Transforms:  []decl.Transform[RecipeCard]{Garnished, Doubled},
}
```

One package owns each transform. The root packages add
`hypercase.Own(sheetcorpus.Looped, sheetcorpus.Renamed)` and
`hypercase.Own(recipecorpus.Garnished, recipecorpus.Doubled)` to their
registrations.

### How the files change

Archives gain variants. `testdata/recipe/pancakes.txtar`, with a hand-written
variant and both transforms' variants. `scale` skips `transform/doubled`,
which already serves four, and `shopping-list` is blocked on
`transform/garnished` and `variant/method`:

```
Pancakes for two.
-- in/recipe/pancakes.txt --
serves: 2
flour: 100 g
milk: 150 ml
egg: 1
-- out/scale/card.txt --
serves: 4
flour: 200 g
milk: 300 ml
egg: 2
-- out/shop/stdout.txt --
egg: 1
flour: 100 g
milk: 150 ml
-- out/shopping-list/list.txt --
egg: 1
flour: 100 g
milk: 150 ml
-- variant/method/about --
A line of method among the ingredients, which no ingredient parser reads.
-- variant/method/properties --
garbled
-- variant/method/in/recipe/pancakes.txt --
serves: 2
flour: 100 g
whisk until smooth
milk: 150 ml
egg: 1
-- variant/method/out/parse/error.txt --
line 3: not an ingredient: "whisk until smooth"
  recipe/pancakes.txt:3: whisk until smooth
-- variant/method/out/shop/error.txt --
line 3: not an ingredient: "whisk until smooth"
  recipe/pancakes.txt:3: whisk until smooth
-- transform/doubled/in/recipe/pancakes.txt --
serves: 4
flour: 200 g
milk: 300 ml
egg: 2
-- transform/doubled/out/shop/stdout.txt --
egg: 2
flour: 200 g
milk: 300 ml
-- transform/doubled/out/shopping-list/list.txt --
egg: 2
flour: 200 g
milk: 300 ml
-- transform/garnished/in/recipe/pancakes.txt --
serves: 2
flour: 100 g
milk: 150 ml
egg: 1
parsley: 1 handful
-- transform/garnished/out/scale/card.txt --
serves: 4
flour: 200 g
milk: 300 ml
egg: 2
parsley: 2 handful
-- transform/garnished/out/shop/stderr.txt --
parsley: left off: a handful has no fixed size
-- transform/garnished/out/shop/stdout.txt --
egg: 1
flour: 100 g
milk: 150 ml
-- transform/garnished/out/tally/error.txt --
line 5: parsley: a handful has no fixed size
  recipe/pancakes.txt:5: parsley: 1 handful
```

`testdata/sheet/twice.txtar`, with its `calc` and `format` sections left out.
`evaluate` is blocked on every variant but `transform/renamed`:

```
A1 is twice B1.
-- in/sheet/A1 --
=B1*2
-- in/sheet/B1 --
3
-- out/evaluate/values.txt --
A1 6
B1 3
-- variant/cycle/about --
B1 refers back to A1, so the sheet has a cycle.
-- variant/cycle/properties --
circular
-- variant/cycle/in/sheet/B1 --
=A1
-- variant/cycle/out/resolve/error.txt --
A1 -> B1 -> A1: cycle
  sheet/A1:1: =B1*2
  sheet/B1:1: =A1
-- variant/missing/about --
B1 is gone, so A1 refers to a cell that doesn't exist.
-- variant/missing/properties --
dangling
-- variant/missing/delete --
sheet/B1
-- variant/missing/out/resolve/error.txt --
A1: B1 doesn't exist
  sheet/A1:1: =B1*2
-- transform/looped/in/sheet/A1 --
=A1
-- transform/looped/out/resolve/error.txt --
A1 -> A1: cycle
  sheet/A1:1: =A1
-- transform/renamed/in/sheet/A2 --
=B2*2
-- transform/renamed/in/sheet/B2 --
3
-- transform/renamed/delete --
sheet/A1
sheet/B1
-- transform/renamed/out/evaluate/values.txt --
A2 6
B2 3
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
  corpus directory (step 5): `TestCorpus/recipe/pancakes/transform/garnished/tally`.

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
- Its `Run` receives the archive's name (proposed) and the base's input
  view, and returns the variant's input view.
- The registry lists `Gives` and `Keeps`, so every package computes the same
  variants without running anything.
- `looped` makes the first cell refer to itself and gives `circular`, so
  `resolve` is expected to err on every `looped` variant with nothing written
  by hand. `renamed` moves every cell one row down, rewriting each reference,
  and keeps `circular` and `dangling`, so it runs over bases that state
  either; a base that states `malformed` is declined. In the recipes,
  `garnished` gives `vague`, so `tally` is expected to err on every
  `garnished` variant; `doubled` keeps `vague`; a base that states `garbled`
  is declined by both.
- It is materialized. An update writes `transform/<name>/in/…` and
  `transform/<name>/delete`, and a normal run checks them like goldens. In a
  normal run the variant's cells read the committed sections.
- An update replaces a transform's `in/…` and `delete` whole: a file the
  transform no longer produces is removed, and the removal shows in the diff.
- When a transform no longer applies to a base, its owner's update removes
  its `in/…` and `delete`, and what remains under `transform/<name>/out/` is
  each column's orphan.
- A transform nobody registers any more leaves sections nothing claims. The
  root-package check reports them and names the strip command.
- The package that owns a transform writes and judges its `in/…` and
  `delete`. Each column's owner writes and judges that column's cells on the
  variant.

**Known gaps in a transform's variant.** A known gap in a cell of a
transform's variant is a line in the base's `gaps` section that names the
transform. Its format (proposed) is `transform/<name>`, one space, the
column's name, one space, then the text. In `omelette`, whose doubled card
scales its eggs the same wrong way as the base:

```
-- gaps --
scale A count scales to a whole number, so egg: 1 for four. Waits on recipes#2.
transform/doubled scale A count scales to a whole number, so egg: 1 for four. Waits on recipes#2.
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

- A variant's point, as its overlay and its `about`: `variant/method` is
  `pancakes` with one line of method added.
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

```go
// walkthrough/recipes/step08/recipecorpus/project.go
var Project = decl.Project{
	UpdateVar:   "RECIPES_UPDATE_GOLDEN",
	RenderError: renderError,
	Codes: func(err error) []string {
		var e *recipe.Error
		if errors.As(err, &e) {
			return []string{e.Code}
		}
		return nil
	},
}
```

```go
// walkthrough/recipes/step08/corpus_test.go
var scale = hypercase.Suite[*recipe.Recipe]{
	Name: "scale",
	Description: "Scales the recipe to serve four, rounding metric amounts to the unit " +
		"and others to the quarter.",
	Probe: servesFour, // step 6's probe
	Outputs: []decl.Output{
		{Pattern: "card.txt", Description: "The card, scaled to serve four."},
		{Pattern: "stderr.txt", Description: "Each amount scale rounded, and to what."},
	},
	Classifier: hypercase.Classifier[*recipe.Recipe]{
		Name:        "rounded",
		Description: "Adds rounded when scale rounded some amount.",
		Classify: func(r *recipe.Recipe, got hypercase.Result) (string, error) {
			if len(got.Stderr) > 0 {
				return "rounded", nil
			}
			return "", nil
		},
	},
	Run: func(t testing.TB, archive string, r *recipe.Recipe, stdout, stderr io.Writer) ([]hypercase.File, error) {
		scaled := r.Scale(4)
		for _, note := range scaled.Rounded {
			fmt.Fprintln(stderr, note)
		}
		return []hypercase.File{{Path: "card.txt", Data: scaled.Format()}}, nil
	},
}
```

The sheets project supplies the same `Codes`, over `*sheet.Error`, and gives
`evaluate` a classifier:

```go
// walkthrough/sheets/step08/corpus_test.go: evaluate gains
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
```

### How the files change

Every cell gains an `outcome` file, skipped and blocked cells included.

`testdata/recipe/scones.txtar`, with its transforms' variants left out:

```
Scones for three, in imperial measures.
-- in/recipe/scones.txt --
serves: 3
flour: 8 oz
butter: 2 oz
milk: 5 floz
-- out/parse/outcome --
accepted
-- out/scale/card.txt --
serves: 4
flour: 10 3/4 oz
butter: 2 3/4 oz
milk: 6 3/4 floz
-- out/scale/outcome --
accepted rounded
-- out/scale/stderr.txt --
flour: 10 2/3 oz rounded to 10 3/4 oz
butter: 2 2/3 oz rounded to 2 3/4 oz
milk: 6 2/3 floz rounded to 6 3/4 floz
-- out/shop/outcome --
accepted
-- out/shop/stdout.txt --
butter: 57 g
flour: 227 g
milk: 142 ml
-- out/shopping-list/list.txt --
butter: 57 g
flour: 227 g
milk: 142 ml
-- out/shopping-list/outcome --
accepted
-- out/tally/outcome --
accepted
```

`testdata/sheet/twice.txtar`, with its `missing` variant and its transforms'
variants left out:

```
A1 is twice B1.
-- in/sheet/A1 --
=B1*2
-- in/sheet/B1 --
3
-- out/calc/outcome --
accepted
-- out/calc/stdout.txt --
6,3
-- out/evaluate/outcome --
accepted
-- out/evaluate/values.txt --
A1 6
B1 3
-- out/format/formulas.txt --
A1 =B1 * 2
-- out/format/outcome --
accepted
-- out/parse/outcome --
accepted
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
-- variant/cycle/out/calc/outcome --
rejected S201
-- variant/cycle/out/evaluate/outcome --
blocked resolve
-- variant/cycle/out/format/formulas.txt --
A1 =B1 * 2
B1 =A1
-- variant/cycle/out/format/outcome --
accepted
-- variant/cycle/out/parse/outcome --
accepted
-- variant/cycle/out/resolve/error.txt --
A1 -> B1 -> A1: cycle
  sheet/A1:1: =B1*2
  sheet/B1:1: =A1
-- variant/cycle/out/resolve/outcome --
rejected S201
```

Elsewhere in the corpora, `pancakes/transform/doubled`'s `scale` cell is
`skipped already serves 4`, and a sheet whose `C1` is `=A1/B1`, with `B1` at
`0`, has `evaluate`'s outcome `accepted div-zero`.

The archives shown in steps 3 to 7 are as their own step leaves them. Their
examples carry an `outcome` per cell from this step on, and the next update
writes them (proposed). `hypercase.json` gains each suite's classifier, and
whether the project supplies `Codes`.

### Rules

**The outcome.**

- Each cell has an `outcome` file in its column's directory:
  `out/<column>/outcome`, and the same under a variant's prefix. The
  framework writes it and compares it byte for byte.
- It holds the code's verdict, not the test's: `accepted`,
  `rejected <codes>`, `panicked`, `blocked <lift>` or `skipped <why>`. The
  first word is the state, and the rest is detail.
- The project may supply `Codes func(err error) []string`, which gives a
  rejected cell its codes. Without it, a rejected cell carries none.
- An optional suite classifier, with a name and a description, refines the
  outcome: `accepted rounded`. It can't change the state, and runs only on
  cells that ran. An error or a panic in a classifier fails the cell, and the
  update then writes nothing for that cell: an update is atomic per cell.
- Skipped and blocked cells write an outcome, and their other committed
  files are orphans.
- The expected-error check reads what the run did, never the committed
  outcome. A cell that erred has the outcome `rejected`, so the two agree. A
  `panicked` cell fails unless a `gaps` line marks it.
- A frozen cell's `outcome` is one of its files. A cell frozen before this
  step has none: a normal run fails on the missing file and says to run the
  update, and the update adds it when the cell's other files match the run.
  A cell whose files differ gains nothing and fails as before. That upgrade,
  of `outcome` here and `inputs` at step 11, is the one write an update makes
  to a frozen cell, and it never changes or removes a file the cell has.
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
- A code holds no space, semicolon or newline. A reason or a classifier's
  detail is one line, not empty, with no space at either end. A cell whose
  code, reason or detail breaks this fails, naming it, and an update writes
  nothing for that cell.

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

```go
// walkthrough/sheets/step08/corpus_test.go, registered with the rest
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

The recipes project registers the same check over `recipe.Codes`, the codes
of the card parser and of `tally`. The sheets' findings,
`testdata/checks/codes.txt`:

```
S101
S201
S202
```

- A corpus check has a name, a description and a `Run` over every cell, and
  is registered when its package is initialized. A test function runs it:
  `hypercase.Test` runs each as the subtest `checks/<name>` (proposed).
- A cell, as a check sees it, exposes its committed outcome, its committed
  sections, its column's registry entry and its case's stated properties.
  `hypercase.Cell` holds (proposed) `Type`, `Archive`, `Case` and `Column`;
  `Outcome`, with its `State`, `Codes` and `Detail`; `Entry`; `Properties`;
  and `Files`, the cell's committed files under its `out/<column>/`.
- A check reads only committed files, and never runs a suite, a lift or a
  transform.
- Its findings are committed as a golden at `<pkg>/testdata/checks/<name>.txt`
  in the package that owns the check.
- The findings format (proposed): one finding per line, each ending with a
  newline, sorted byte by byte, each once. An empty list is an empty file. A
  finding that holds a newline fails the check.
- An error from `Run` is a violated requirement: it fails every run, updates
  included, and an update writes no findings.
- Corpus checks carry a description, under step 3's rule.
- It is the first assertion about a group of cells.

### What can now be seen

- The code's verdict on every cell, as a file: `broth`'s `shopping-list` is
  `blocked tally`, and `scones`' `scale` is `accepted rounded`.
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

Proposed: the flags `-root` and `-text`, and `decl.Project`'s `IssueLinks`
field, a list of `decl.IssueLink`, each a regular expression and a URL
template.

```go
// walkthrough/sheets/step09/sheetcorpus/project.go: Project gains
	IssueLinks: []decl.IssueLink{
		{Pattern: `sheets#(\d+)`, URL: "https://example.com/sheets/issues/$1"},
	},
```

The recipes project supplies `recipes#(\d+)` the same way.

```sh
hypercase report main HEAD > report.html
hypercase report -text main HEAD
```

### How the files change

No archive changes. `hypercase.json` gains the link patterns. The report is
a file the person reads; nothing commits it.

The report reads each archive as each revision commits it. Two of the cells
the samples below report, with every other column left out. In the recipes,
`testdata/recipe/scones.txtar` at `<a>`:

```
Scones for three, in imperial measures.
-- in/recipe/scones.txt --
serves: 3
flour: 8 oz
butter: 2 oz
milk: 5 floz
-- out/scale/card.txt --
serves: 4
flour: 10 3/4 oz
butter: 2 3/4 oz
milk: 6 3/4 floz
-- out/scale/outcome --
accepted rounded
-- out/scale/stderr.txt --
flour: 10 2/3 oz rounded to 10 3/4 oz
butter: 2 2/3 oz rounded to 2 3/4 oz
milk: 6 2/3 floz rounded to 6 3/4 floz
```

and at `<b>`, where `scale` writes thirds:

```
Scones for three, in imperial measures.
-- in/recipe/scones.txt --
serves: 3
flour: 8 oz
butter: 2 oz
milk: 5 floz
-- out/scale/card.txt --
serves: 4
flour: 10 2/3 oz
butter: 2 2/3 oz
milk: 6 2/3 floz
-- out/scale/outcome --
accepted
```

In the sheets, `testdata/sheet/loop.txtar` at `<a>`:

```
A1 and B1 refer to each other.
-- in/sheet/A1 --
=B1
-- in/sheet/B1 --
=A1
-- properties --
circular
-- out/calc/error.txt --
A1 -> B1 -> A1: cycle
  sheet/A1:1: =B1
  sheet/B1:1: =A1
-- out/calc/outcome --
rejected S201
-- out/resolve/error.txt --
A1 -> B1 -> A1: cycle
  sheet/A1:1: =B1
  sheet/B1:1: =A1
-- out/resolve/outcome --
rejected S201
```

and at `<b>`, where a cycle is worded `cycle: A1 -> B1 -> A1`:

```
A1 and B1 refer to each other.
-- in/sheet/A1 --
=B1
-- in/sheet/B1 --
=A1
-- properties --
circular
-- out/calc/error.txt --
cycle: A1 -> B1 -> A1
  sheet/A1:1: =B1
  sheet/B1:1: =A1
-- out/calc/outcome --
rejected S201
-- out/resolve/error.txt --
cycle: A1 -> B1 -> A1
  sheet/A1:1: =B1
  sheet/B1:1: =A1
-- out/resolve/outcome --
rejected S201
```

### Rules

- `hypercase report <a> <b>`, from `cmd/hypercase`, takes two git revisions,
  reads both from the repository's object store, and produces a static HTML
  file, which it writes on stdout (proposed).
- It reads registries and archives, never Go source. It reads every
  registry format version up to its own, and refuses a newer one, saying to
  run the report from the newer revision's hypercase.
- It works from outcomes alone. It shows the matrix, and ranks changes:
  changed outcomes first, then cells appearing or disappearing, then bytes
  changed under the same outcome.
- Each case's stated properties are shown beside it, and each `gaps` line on
  the cell it marks, linked to its issue through the patterns the project
  supplies, one per issue tracker its `gaps` lines name.
- hypercase's own CI runs the report over the toys.
- Step 11 adds input lines and corpus-check findings.

**Flags** (proposed).

- `-root <dir>` names the corpus root, relative to the current directory.
  Without it, the report finds the root as a test does.
- `-text` writes the ranked changes as plain text instead of the page.

**Link patterns** (proposed). Each `decl.IssueLink` is a regular expression
and a URL template, which may use the expression's groups. The report reads
them from the registries at `<b>`. Text that matches a pattern, in a `gaps`
line or a description, links to the URL it makes.

**The plain-text format** (proposed). One heading per rank that has
changes, in rank order. Under it, one line per cell: its case, its column,
and its outcome, or the change in it. So `outcome` is never listed as a
file. Under each cell, one line per other file of the cell that was added,
changed or removed. For a change to the recipes that writes amounts in
thirds as well as quarters:

```
outcome changed
  recipe/scones scale: accepted rounded -> accepted
    card.txt changed
    stderr.txt removed
  recipe/scones/transform/doubled scale: accepted rounded -> accepted
    card.txt changed
    stderr.txt removed
  recipe/scones/transform/garnished scale: accepted rounded -> accepted
    card.txt changed
    stderr.txt removed
```

For a change to the sheets that adds the archive `ratio`, and words a cycle
as `cycle: A1 -> B1 -> A1`, with the transforms' variants left out:

```
appeared
  sheet/ratio calc: accepted
    stdout.txt added
  sheet/ratio evaluate: accepted
    values.txt added
  sheet/ratio format: accepted
    formulas.txt added
  sheet/ratio parse: accepted
  sheet/ratio resolve: accepted
bytes changed
  sheet/loop calc: rejected S201
    error.txt changed
  sheet/loop resolve: rejected S201
    error.txt changed
  sheet/twice/variant/cycle calc: rejected S201
    error.txt changed
  sheet/twice/variant/cycle resolve: rejected S201
    error.txt changed
```

### What can now be seen

- A change ranked across the whole corpus. Changed verdicts come first, then
  what appeared or disappeared, then what was only reworded.
- The matrix for each corpus type at `<b>`, each cell with its state and
  frozen cells marked.
- Each known gap beside its cell, linked to the issue it waits on.

## 10. Chaining

### What step 9 leaves

A blog post holds a recipe, and a CSV export holds a sheet. To run the card's
columns on a post, a suite runs extraction and everything after it. A change
in extraction then fails every column downstream, and the card in between is
recorded nowhere.

### What the author writes

A **bytes lift** is a lift whose product type is another corpus type. Its
product is the files it makes, which are that type's input, and they are
committed as such.

Proposed names: `decl.Lift`'s `Product` field, which declares the product's
files.

```go
// walkthrough/recipes/step10/recipecorpus/types.go gains
type BlogPost struct{ fs.FS } // in/post/<archive>.txt

var PostType = decl.Type[BlogPost]{
	Dir:         "post",
	Description: "A blog post: in/post/<archive>.txt, whose recipe follows a ## Recipe heading.",
}

var Recipeless = decl.Property{
	Name:        "recipeless",
	Description: "The post has no ## Recipe heading.",
}

var Extract = decl.Lift[BlogPost, RecipeCard]{
	Name:        "extract",
	Description: "Writes the post's recipe as a card named after the archive.",
	Outputs: []decl.Output{
		{Pattern: "error.txt", Description: "Why the post yields no card."},
	},
	Product: []decl.Output{
		{Pattern: "recipe/*.txt", Description: "The card, laid out as a recipe card's in/recipe/."},
	},
	Rejects: []decl.Property{Recipeless},
	Run: func(archive string, in BlogPost, stderr io.Writer) (RecipeCard, error) {
		card, err := recipe.Extract(in, "post/"+archive+".txt", "recipe/"+archive+".txt")
		return RecipeCard{card}, err
	},
}
```

```go
// walkthrough/sheets/step10/sheetcorpus/types.go gains
type CSV struct{ fs.FS } // in/csv/<archive>.csv

var CSVType = decl.Type[CSV]{
	Dir:         "csv",
	Description: "A CSV export: in/csv/<archive>.csv, one row of the sheet per line.",
}

var Unterminated = decl.Property{
	Name:        "unterminated",
	Description: "Some quoted field is never closed.",
}

var Import = decl.Lift[CSV, Sheet]{
	Name:        "import",
	Description: "Turns each CSV field into a cell: row 1, field 2 is B1.",
	Outputs: []decl.Output{
		{Pattern: "error.txt", Description: "The line the importer can't read, and why."},
	},
	Product: []decl.Output{
		{Pattern: "sheet/*", Description: "One file per cell, as a sheet's in/sheet/ holds them."},
	},
	Rejects: []decl.Property{Unterminated},
	Run: func(archive string, in CSV, stderr io.Writer) (Sheet, error) {
		cells, err := sheet.ImportCSV(in, "csv/"+archive+".csv")
		return Sheet{cells}, err
	},
}
```

The root packages register the new type and property, and own the new lift.
Each other package whose columns read past it registers the type and adds
the lift to its `Use`: `format/` uses `sheetcorpus.Import` and
`sheetcorpus.Parse`.

### How the files change

A new corpus directory per toy, `testdata/post/` and `testdata/csv/`. Each of
their archives holds the product under `via/<lift>/`, and the cells of every
column the product reaches. `testdata/post/pancakes.txtar`:

```
A blog post about Sunday pancakes, with the recipe at the end.
-- in/post/pancakes.txt --
# Sunday pancakes

We make these every week, and the last one is always argued over.

## Recipe

- serves 2
- 100 g flour
- 150 ml milk
- 1 egg
-- via/extract/recipe/pancakes.txt --
serves: 2
flour: 100 g
milk: 150 ml
egg: 1
-- out/extract/outcome --
accepted
-- out/parse/outcome --
accepted
-- out/scale/card.txt --
serves: 4
flour: 200 g
milk: 300 ml
egg: 2
-- out/scale/outcome --
accepted
-- out/shop/outcome --
accepted
-- out/shop/stdout.txt --
egg: 1
flour: 100 g
milk: 150 ml
-- out/shopping-list/list.txt --
egg: 1
flour: 100 g
milk: 150 ml
-- out/shopping-list/outcome --
accepted
-- out/tally/outcome --
accepted
```

`testdata/csv/prices.txtar`:

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
-- out/calc/outcome --
accepted
-- out/calc/stdout.txt --
4,6,10
-- out/evaluate/outcome --
accepted
-- out/evaluate/values.txt --
A1 4
B1 6
C1 10
-- out/format/formulas.txt --
C1 =A1 + B1
-- out/format/outcome --
accepted
-- out/import/outcome --
accepted
-- out/parse/outcome --
accepted
-- out/resolve/outcome --
accepted
```

`testdata/csv/open-quote.txtar`. Every column past `import` is blocked by
it:

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
line 1: quote never closes
  csv/open-quote.csv:1: 4,"six,=A1+B1
-- out/import/outcome --
rejected C101
-- out/parse/outcome --
blocked import
-- out/resolve/outcome --
blocked import
```

`hypercase.json` marks each bytes lift, and lists its product declarations.

### Rules

- An upstream product is read as downstream input, within the same archive.
- A bytes lift's product is another corpus type's input. It is committed
  under `via/<lift>/`, which holds the product only, under whatever file
  names the product has: `via/import/sheet/A1` is the downstream case's
  `sheet/A1`.
- The lift's own cell files sit in `out/<lift>/`, like any column's. A
  lift's name covers both directories.
- A variant's product goes under `variant/<name>/via/<lift>/` or
  `transform/<name>/via/<lift>/`, and holds the variant's whole product, not
  an overlay on the base's.
- `out/` holds only terminal outputs: nothing downstream reads from it.
- An update replaces `via/<lift>/…` whole, at every level: a file the lift
  no longer produces is removed, and the removal shows in the diff. An
  update writes `via/` first, then runs whatever reads it.
- In a normal run, downstream columns read the committed `via/` bytes, so
  churn upstream fails only the upstream column. A downstream column takes
  `blocked` from the lift's committed `outcome`. A lift that is `accepted`
  with no product files gives its readers an empty view.
- A bytes lift's frozen cell includes its `via/` product.
- The properties a case states hold for every column the case reaches,
  through the lift too. A CSV with a quote that never closes states
  `unterminated`, which `import` rejects, and a CSV whose imported sheet has
  a cycle states `circular`, which `resolve` rejects. A post with no recipe
  states `recipeless`, which `extract` rejects, and a post whose recipe calls
  for a handful of something states `vague`, which `tally` rejects.
- One type's lift can feed another type's ladder: a `BlogPost`'s `extract`
  makes a recipe card, and a `CSV`'s `import` makes a sheet. Paths through
  bytes lifts count toward the one path to each consumed type.
- A suite reaches across types by the type it consumes: `shop`, which
  consumes `RecipeCard`, runs on every post that `extract` accepts.
- Past a bytes lift, a cell's input view, which `RenderError` receives, is
  that lift's product.
- `testdata/sheet/prices.txtar` and `testdata/csv/prices.txtar` both reach
  `evaluate`, the second through `import`. Their cases are `sheet/prices` and
  `csv/prices` (step 5), and their cells' subtests
  `TestCorpus/sheet/prices/evaluate` and `TestCorpus/csv/prices/evaluate`.

### What can now be seen

- The card a post yields, as committed bytes beside the post. A change to
  extraction shows as a change to `via/extract/`, and fails `extract`'s
  column alone until the update; the columns downstream read the committed
  card.
- Where a chained input stops: `open-quote` at `import`, with every column
  past it `blocked import`.
- A CSV and a hand-written sheet with the same cells side by side in the
  matrix, `csv/prices` beside `sheet/prices`.

## 11. Reverse maps

### What step 10 leaves

The report shows which outputs changed, not which input lines they came
from. A reviewer reading a changed `list.txt` searches the card by hand,
and, through a bytes lift, the post.

### What the author writes

Every output declaration says which input lines its files come from. That
map from an output back to its inputs is a **reverse map**. A **range** is a
span of lines in one file of the cell's input view.

Proposed names: `decl.Output`'s `Inputs` field; `decl.InputsFunc`, a
function that maps one output file; `decl.AllInputs`, the escape;
`decl.FromPositions`, which an `error.txt` declaration states to take its
ranges from `Positions`; `decl.Range`; and `decl.Project`'s `Positions`
field.

```go
// walkthrough/recipes/step11/recipecorpus/project.go: Project gains
	Positions: func(err error) []decl.Range {
		var rs []decl.Range
		for _, p := range recipe.Positions(err) {
			rs = append(rs, decl.Range{Path: p.Path, Start: p.Line, End: p.Line})
		}
		return rs
	},
```

```go
// walkthrough/recipes/step11/corpus_test.go: shopping-list's declaration
	Outputs: []decl.Output{
		{
			Pattern:     "list.txt",
			Description: "What to buy, one ingredient per line, in metric units.",
			Inputs: decl.InputsFunc(func(in fs.FS, path string) ([]decl.Range, error) {
				card, lines, err := recipe.CardLines(in) // the card's path, and how many lines it has
				if err != nil {
					return nil, err
				}
				return []decl.Range{{Path: card, Start: 2, End: lines}}, nil // every line after serves:
			}),
		},
	},
```

`scale`'s `card.txt` maps to the whole card, `Range{Path: card}`. `parse`'s
`error.txt` states `Inputs: decl.FromPositions`. `shop`'s `stdout.txt` and
`stderr.txt` state `Inputs: decl.AllInputs`, and its `error.txt` states
`decl.FromPositions`.

```go
// walkthrough/sheets/step11/corpus_test.go: evaluate's declaration
	Outputs: []decl.Output{
		{
			Pattern:     "values.txt",
			Description: "Each cell's value, one per line, in cell order.",
			Inputs: decl.InputsFunc(func(in fs.FS, path string) ([]decl.Range, error) {
				cells, err := fs.Glob(in, "sheet/*")
				if err != nil {
					return nil, err
				}
				rs := make([]decl.Range, len(cells))
				for i, c := range cells {
					rs[i] = decl.Range{Path: c} // 0, 0: the whole file
				}
				return rs, nil
			}),
		},
	},
```

`format`'s `formulas.txt` maps to its formula cells only. The sheets project
supplies `Positions` over `sheet.Positions`, and `parse`'s, `resolve`'s and
`calc`'s `error.txt` state `decl.FromPositions`. `calc`'s `stdout.txt` and
`stderr.txt` state `decl.AllInputs`.

### How the files change

Every cell that ran gains an `inputs` file. The archives shown in steps 3 to
10 are as their own step leaves them. Their examples carry `inputs` from this
step on, and the next update writes them (proposed). `hypercase.json`
records, for each output declaration, whether it maps its files with
`Inputs`, takes its ranges from `Positions`, or states `AllInputs`.

`testdata/recipe/pancakes.txtar`, with its variants left out:

```
Pancakes for two.
-- in/recipe/pancakes.txt --
serves: 2
flour: 100 g
milk: 150 ml
egg: 1
-- out/parse/inputs --
-- out/parse/outcome --
accepted
-- out/scale/card.txt --
serves: 4
flour: 200 g
milk: 300 ml
egg: 2
-- out/scale/inputs --
card.txt
  in/recipe/pancakes.txt
-- out/scale/outcome --
accepted
-- out/shop/inputs --
stdout.txt
  all inputs
-- out/shop/outcome --
accepted
-- out/shop/stdout.txt --
egg: 1
flour: 100 g
milk: 150 ml
-- out/shopping-list/inputs --
list.txt
  in/recipe/pancakes.txt:2-4
-- out/shopping-list/list.txt --
egg: 1
flour: 100 g
milk: 150 ml
-- out/shopping-list/outcome --
accepted
-- out/tally/inputs --
-- out/tally/outcome --
accepted
```

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

Past a bytes lift, ranges point at the product. In
`testdata/post/pancakes.txtar`, `scale`'s `inputs` is:

```
card.txt
  via/extract/recipe/pancakes.txt
```

and `extract`'s own `inputs` maps its product file, named by its section, to
the post's recipe:

```
via/extract/recipe/pancakes.txt
  in/post/pancakes.txt:7-10
```

**The `inputs` format** (proposed).

- For each output file of the cell, in path order: a line holding the
  file's path, then one line per range, indented two spaces. A bytes lift's
  product files follow its own files, each named by its section.
- A range is its archive section and its lines, `in/recipe/pancakes.txt:2-4`.
  A whole file is its section alone, `in/sheet/A1`. `AllInputs` is
  `all inputs`.
- Ranges are sorted by section, then by first line, then by last. A repeated
  range appears once, and overlapping or adjacent ranges in one section are
  merged.
- An output file with no ranges is its path line alone. So is an `error.txt`
  whose error has no position.
- A cell that ran and wrote no output files has an empty `inputs`.
- Every line ends with a newline.

### Rules

- Each output declaration has `Inputs`. A map is a
  `func(in fs.FS, path string) ([]Range, error)`, given as
  `decl.InputsFunc`, which receives the cell's input view and works per
  output file. It is required, with `AllInputs` as the explicit escape,
  which the report shows as such.
- `Range{Path, Start, End}` uses 1-based inclusive lines in the cell's input
  view, and `0, 0` means the whole file. The project may supply
  `Positions func(err error) []Range`, which finds an error's positions.
- A cell has one input view: its case's inputs, with a variant's overlay and
  deletions applied; past a bytes lift, that lift's product, and past
  several, the last one's. A range outside it is bad, and fails the cell
  (proposed).
  The framework maps each range through the overlay to archive sections.
- Each `error.txt` declaration, a suite's or a lift's, states where its
  ranges come from: `Positions`, through `decl.FromPositions`, or
  `AllInputs`. One that states neither fails the run. There is no default.
- An error or a panic in `Inputs` fails the cell, and the update writes
  nothing for that cell.
- Ranges are committed per cell as `out/<column>/inputs`, and the same under
  a variant's prefix, compared byte for byte. A frozen cell without `inputs`
  gains it on update, as with `outcome` at step 8; an update never changes
  or removes an `inputs` the cell has.
- So the report stays a pure function of repository files, and range changes
  show in pull-request diffs. The accepted cost is line-shift churn in edited
  sections.
- A range through a `via/` product stops at the product's lines (proposed):
  it names a `via/<lift>/…` section, and is not followed back through the
  lift's own `inputs`.
- The report shows each changed output beside its input lines, ranks ranges
  that changed alone fourth, and includes corpus-check findings (#18).
- This is the last code step, because requiring `Inputs` is source-breaking
  for every project.

### What can now be seen

- Each changed output beside the input lines it came from. A changed
  `list.txt` shows lines 2 to 4 of the card beside it.
- Which overlay a variant's output depends on: `cycle`'s `resolve` error
  points at `variant/cycle/in/sheet/B1`, the line the variant changed.
- Where a declaration gives no map, and says so: `shop`'s and `calc`'s
  stdout read `all inputs`.

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

In the recipes, for illustration, `imperial` is P for a recipe whose every
measured amount is imperial (`scones`), anti-P when every one is metric
(`pancakes`), and neither when they're mixed.

- `scale` could be claimed P: on an imperial card it rounds,
  `accepted rounded`, and on a metric one it gives `accepted`.
- A suite `to-imperial`, which rewrites a card in imperial units, could be
  claimed anti-P: it rounds on a metric card, and has nothing to round on an
  imperial one.
- `shopping-list` always converts to metric, so it is neutral: an imperial
  variant's list equals its metric base's. A transform that rewrites every
  metric amount in imperial units, and gives `imperial`, makes such pairs.

In the sheets, `circular` is P for a sheet with a cycle, anti-P for a sheet
that parses and has none, and neither for one that doesn't parse.

- `resolve` is P: a matching cell is `rejected`, and a mismatching one
  `accepted`. Its `Rejects` entry already states that claim.
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

Two or three hypotheses, written by hand as corpus checks over the toys and
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
