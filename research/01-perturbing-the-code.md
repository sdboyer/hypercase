# Perturbing the code: mutants beside transforms

A transform perturbs a case's input: it makes a variant from a base, and
the matrix shows which cells move. A mutant does the same to the code: it
breaks one check in a column's implementation, and the matrix shows which
cells move. This note sets the two side by side, shows a mutant run against
the sheets, and lists what hypercase would need in order to host mutants,
in Go and in other languages. Nothing here is built. Claims are either
shown by a run described well enough to repeat (sections 2 and 4), cited,
or labelled untested.

Vocabulary used below:
- **Guard**: one check that refuses an input, such as `Resolve`'s cycle
  check, which gives S201.
- **Mutant**: the program with one guard broken. It is run against tests
  that should fail without the guard.
- **Killed**, **survived**: some test failed against the mutant, or none
  did. These are mutation testing's terms, from DeMillo, Lipton and Sayward
  (1978).
- **Site**: where in the source a mutant applies.
- **Footprint**: the set of cells a mutant moves.

## 1. Two perturbations, one matrix

| | transform | mutant |
|---|---|---|
| perturbs | a case's input | a column's code |
| held fixed | the code | the corpus |
| made by | a jenny, from a base | an edit, at a site |
| states | the properties it gives or keeps | the property whose guard it breaks |
| a cell moves when | the column isn't invariant under the transform | the column depends on the guard |
| older name | metamorphic testing (Chen et al., 1998) | mutation testing (DeMillo et al., 1978) |

In note 00's terms, a transform is an endomorphism `t: R → R` on a root
type, and a pair is `(x, t ∘ x)`. A mutant replaces one arrow of the
registered diagram, a lift `f` by `f'` or a suite `s` by `s'`, and leaves
every other arrow and every case alone. So it is a delta, in note 00's
sense: two interpretations of one diagram. It is a delta with a stated aim:
it breaks P's guard. That makes the prediction known before the run: the
cells that move are the P cases' cells, in the columns that reject P and
the columns past them; every other cell stays. Note 00's delta prediction
says only that the neutral columns agree. A mutant's says which cells
move, too.

A cell is then a function of three things, not two: the case, the column,
and the code. Transforms vary the first, mutants the third. This is a third
axis, as note 00's parameter sweep is. It differs from a sweep in that
each point on it is a defect, chosen to be one, not a setting.

## 2. A mutant against the sheets

The mutant breaks `Resolve`'s cycle guard
(`walkthrough/sheets/sheet/resolve.go`). A cell reached while it is still
being visited is skipped, not reported:

```diff
 		case visiting:
-			return cycleError(stack[slices.Index(stack, c):])
+			if false {
+				return cycleError(stack[slices.Index(stack, c):])
+			}
+			return nil
```

The guard stays in the source under `if false`, so the mutant still
compiles. Without that, `slices` would go unused, the build would fail, and
a build failure would count as a kill.

The mutated file is written outside the checkout, and Go's `-overlay` flag
swaps it in for the build (section 4):

```json
{"Replace": {"<checkout>/walkthrough/sheets/sheet/resolve.go": "<scratch>/resolve.go"}}
```

```sh
go -C walkthrough/sheets test -count=1 -overlay <scratch>/overlay.json ./step11/...
```

Step 11 passes without the overlay. With it, `step11` and `step11/cli`
fail, and `step11/format` and `step11/sheetcorpus` pass. The checkout is
untouched: `git status` is clean afterwards. 30 cells move:

| case or variant | cells that move |
|---|---|
| `csv/loop` | resolve, evaluate, calc |
| `sheet/last-row` | resolve, evaluate, calc |
| `sheet/loop` | resolve, evaluate, calc |
| `sheet/loop/transform/renamed` | resolve, evaluate, calc |
| `sheet/twice/variant/cycle` | resolve, evaluate, calc |
| `sheet/{divide,loop,negative,prices,twice}/transform/looped` | resolve, evaluate, calc |

What the run shows:
- **The footprint is the circular slice, and nothing else.** Every case
  that states `circular` moves, as does every variant that gives it
  (`looped`) or keeps it from a circular base (`loop`'s `renamed`). No
  other case moves. That includes the other `renamed` variants,
  `twice/variant/missing`, and every case `resolve` rejects as `dangling`.
- **The columns are those from `resolve` on.** `resolve` and `calc` go from
  `rejected S201` to `accepted`, and `evaluate` from `blocked resolve` to
  `accepted`. `format` reaches only `parse`, so it doesn't move.
- **Each kill says why.** Each of the 20 `resolve` and `calc` cells fails
  with "resolve returned no error, but the case states circular, which
  resolve rejects" (the variants say "the variant states"). The run fails
  on an expectation the corpus already declares, through `Rejects` and the
  cases' properties, not only because an output's bytes changed.

So the footprint matched the delta prediction of section 1 exactly. Nobody
wrote that prediction down. It follows from declarations the corpus
already holds: the property, the column's `Rejects`, and the lift graph.

## 3. What the matrix gives a mutant

A plain mutation runner has three weaknesses. These are shown in a
consuming project's runner, which placed about sixty mutants by hand, one
at each guard it had made a claim about:

1. **The site is a string.** The runner replaces an exact string that must
   occur once in its file. A refactor or a reformat breaks it. The break is
   loud, "site found 0 times", not silent, but every site is upkeep.
2. **Any failure is a kill.** A non-zero exit counts, so a mutant that
   doesn't compile kills every test while showing nothing. The runner
   relies on the author writing mutants that compile, as above. Tools
   differ on this (section 6).
3. **A test can pass or fail for the wrong reason.** One mutant survived
   because its test asserted only an error code, and the mutant still
   produced that code, by another path. A test that fails for a reason
   unrelated to the guard kills a mutant just as well. And a hand-picked
   test filter that is too narrow makes a survivor of a mutant that an
   unlisted test would kill.

The matrix answers the third, and helps with the second:
- **Killed becomes a footprint.** "Some test failed" becomes "these cells
  moved, with these reasons". A wrong-reason kill shows as cells outside
  the predicted slice, or as a reason other than the guard's property.
- **A build failure is not a footprint.** A package that doesn't build
  moves no cells; it fails to build. A runner that reads the matrix can
  tell the two apart. One that reads the exit code can't.
- **A survivor names a gap in the corpus.** If no cell moves, no case in
  P's slice reaches the guard. The fix is a case, not a one-off test.
- **A footprint wider than predicted is a finding, about either side.**
  Either a case lacks a property it has, a column's `Rejects` is wrong, or
  the guard refuses more than one property. Each of these is something
  the corpus states and the mutant checks. Untested: no run here has
  produced a wider footprint.

The site, the first weakness, is a separate problem. Section 5 covers it.

## 4. Never writing the tree

The consuming project's runner writes each mutant into the working tree,
runs the tests, and restores the file in a `finally`. That has three
costs:
- **Runs are serial.** Two mutants can't share one tree.
- **The tree is wrong while a mutant runs.** An editor, a language server
  or a hook reads the mutant.
- **An interrupted run can leave the mutant in the tree.** `finally`
  doesn't run on SIGKILL.

A mutant should be a layer over the tree, never a write into it.

**In Go, the toolchain has the layer.** `go build -overlay=file.json`, and
`go test`, take `{"Replace": {"<disk path>": "<backing path>"}}` and build
as if each disk file held its backing file's contents. A throwaway module,
not committed, shows what it covers. It has one embedded file, one guard,
and a test that reads the embedded file, reads the same file from disk,
and calls the guard:

| | without the overlay | with it |
|---|---|---|
| a `//go:embed` file | the disk's bytes | the backing file's bytes |
| a Go source file | the guard | the mutant (its test fails) |
| a file the test reads at run time | the disk's bytes | the disk's bytes |

The last row is documented in `go help build`: "overlays will not appear
when binaries and tests are run through go run and go test". `go help
cache` says "The cache is safe for concurrent invocations of the go
command", so mutants can run at once, each with its own overlay. Packages a
mutant doesn't touch come from the cache. Untested: how much running
mutants concurrently gains over running them one by one, given that each
`go test` already spreads packages across cores.

A mutant that changes a file the tests read at run time is outside what an
overlay covers. hypercase's archives are read at run time, but a mutant
changes code, never an archive. Changing an archive is a transform's job.

**Beyond Go, the layer has to come from somewhere else.** A hypercase that
runs other languages' columns needs a layer that doesn't depend on one
toolchain:

| layer | covers run-time reads | cost |
|---|---|---|
| the toolchain's own overlay (Go's `-overlay`) | no | per language; not every toolchain has one |
| a union filesystem: overlayfs in a Linux mount namespace, a FUSE union | yes | per OS; needs a mount namespace or FUSE; macOS has no overlayfs |
| a copy-on-write clone of the tree: APFS `clonefile`, a reflink copy, a btrfs snapshot | yes | per filesystem; a build cache may key on the path and miss (untested) |
| a build system whose inputs are content digests (Bazel, Buck2, Nix) | yes, inside its sandbox | the project has to build with it |

The last is the cleanest fit. A mutant there is just another input
digest, and the build graph reruns exactly the actions that depend on it.
But it asks the consuming project to adopt the build system. The union
filesystem asks nothing of the project, and costs per-OS setup.

## 5. Where a mutant applies

A mutant needs a stable address for its guard. The options:

| site | survives a reformat | survives a refactor | cost |
|---|---|---|---|
| an exact string | no | no | none up front, upkeep after |
| a syntax-tree path (`go/ast`, tree-sitter) | yes | no | per language; tree-sitter spreads it across languages |
| a marker comment at the guard | yes | yes, if the comment moves with the code | puts test apparatus in production source |
| every node an operator matches, as generic mutation tools do | yes | yes | no hand placement, but many mutants, some equivalent to the original |

Hand-placed mutants and operator-generated ones mirror hand-written
variants and transforms. A hand-placed mutant states its aim, the property
whose guard it breaks, so its footprint has a prediction. An
operator-generated one states none, so it can only be killed or survive.
That argues for hand placement wherever a prediction is wanted, and for
operators only as a sweep for guards nobody named.

Industry practice splits the same way (section 6):
- Generators, such as Google's, make every mutant again from the syntax
  tree on each run, and store none.
- The durable hand-placed sites in use are named markers (failpoint's) or
  comments that exclude a site (SQLite's).

## 6. Industry correspondences

This covers practice since about 2005. Sources marked "(summary only)"
were read through an abstract or a search summary, not in full.

**Mutation testing at scale.**
- **Google** ([Petrović et al., *Practical Mutation Testing at
  Scale*](https://arxiv.org/abs/2102.11378)).
  - Mutants are generated from the syntax tree for each changelist: "one
    for each affected covered line", and never more than one in a line.
  - More than a hundred "arid node" rules keep mutants out of code where
    they are unproductive, such as logging.
  - Surviving mutants are shown to the author and reviewers in code
    review, each with "Please Fix" and "Not useful" links. The share of
    productive mutants rose from 15% to 89%.
  - In Go, deleting code leaves unused variables and imports, which don't
    compile. The mutator collects "all used symbols" into a slice instead.
    This is section 2's `if false`, done by the mutator instead of the
    author.
  - Nothing is stored. Mutants are generated again on every run, so no
    site can go stale.
- **Build failures.** The tools disagree about weakness 2:
  - [PIT](https://github.com/hcoles/pitest/blob/master/pitest/src/main/java/org/pitest/mutationtest/DetectionStatus.java)
    counts a mutant that doesn't build as detected.
  - [Stryker](https://stryker-mutator.io/docs/mutation-testing-elements/mutant-states-and-metrics/)
    leaves compile errors out of the score.
  - [cargo-mutants](https://mutants.rs/using-results.html) calls such a
    mutant unviable, which says nothing about the tests.
- **Reasons.** Two tools record more than the verdict:
  - Stryker's report schema records which tests killed each mutant, and
    the failure messages they gave.
  - PIT's `fullMutationMatrix` records every test that kills each mutant:
    a mutants × tests matrix. Its docs call it partially supported.
  - That matrix is the nearest thing to a footprint, at the granularity of
    tests, not cells.
- **[SQLite](https://www.sqlite.org/testing.html).**
  - SQLite flips each branch instruction in the compiled assembly, to an
    unconditional jump or a no-op.
  - Its criterion is that "every branch instruction makes a difference in
    the output". That is the footprint criterion in one line.
  - Branches that only make the code faster give false positives. They are
    marked in the source with `/*OPTIMIZATION-IF-TRUE*/` comments: a marker
    site, used to exclude a mutant rather than to place one.
- **Durable hand-placed sites.** These exist as named markers, for fault
  injection rather than mutation. [failpoint](https://github.com/pingcap/failpoint)'s
  `failpoint.Inject("name", …)` is rewritten from the syntax tree and
  enabled by name.
- **Meta's ACH** ([2025](https://arxiv.org/abs/2501.12862)).
  - A language model writes mutants. Only those that build and pass the
    existing tests are kept, 9,095 of 31,677.
  - A model also judges which are equivalent to the original.
  - The mutants are thrown away once tests that kill them are written.

**A code perturbation that predicts no move.**
[CockroachDB's metamorphic constants](https://pkg.go.dev/github.com/cockroachdb/cockroachdb-parser/pkg/util/metamorphic):
- Test builds randomize "magic constants", such as batch sizes, whose
  value "cannot affect the output of any SQL DMLs".
- The main corpus has to pass with whichever value is chosen.
- This is a code perturbation whose prediction is that no cell moves:
  section 1's neutral claim, on the code axis. A mutant predicts a directed
  footprint; a metamorphic constant predicts an empty one.
- [Pebble's metamorphic test](https://www.cockroachlabs.com/blog/metamorphic-testing-the-database/)
  does the same with configurations. One sequence of operations runs under
  many configurations, the default one is the base case, and any
  difference fails.

**Transforms that carry facts.**
- spirv-fuzz, from GraphicsFuzz, which Google acquired, has 88 kinds of
  transform. Each is serialized, along with the facts it establishes, such
  as `BlockIsDead` and `DataSynonym`
  ([protobufs](https://github.com/KhronosGroup/SPIRV-Tools/blob/main/source/fuzz/protobufs/spvtoolsfuzz.proto)).
  That is the closest match to `Gives` and `Keeps`.
- Because transforms are recorded, a failing sequence of them can itself
  be reduced, by delta debugging.
- Bugs found are reduced and become tests in the Vulkan conformance suite
  ([ECOOP 2020](https://2020.ecoop.org/details/ecoop-2020-papers/22/Putting-Randomized-Compiler-Testing-into-Production),
  summary only).

**Update modes, and the case against running one under a mutant.**
- [Jest](https://jestjs.io/docs/snapshot-testing) doesn't write snapshots
  in CI, and asks its users to "fight against the habit of regenerating
  snapshots".
- [insta](https://insta.rs/docs/cli/) can reject unreferenced snapshots in
  CI.
- LLVM [lost a check that mattered](https://discourse.llvm.org/t/debuginfo-optimization-tests-and-automatic-check-generation/67246)
  when a test was converted to generated checks. It was found "TOTALLY BY
  CHANCE".

**Known gaps.**
- rustc's [`known-bug` tests](https://rustc-dev-guide.rust-lang.org/tests/ui.html)
  are a "sentinel that will fail if the bug is incidentally fixed", as a
  frozen gap's cell is.
- [pytest](https://docs.pytest.org/en/stable/how-to/skipping.html)'s
  strict `xfail` fails an unexpected pass. Its `raises=` limits which
  exception counts as the expected failure, the only right-reason check on
  a known failure the research found.
- Chromium's [TestExpectations](https://chromium.googlesource.com/chromium/src/+/HEAD/docs/testing/web_test_expectations.md)
  require a bug on every line. Stale lines are removed by tools that mine
  CI history (summary only).

**Deltas over a matrix.** Several systems report a change as what it moved
across a matrix. That is a footprint's shape, though for real changes, not
synthetic ones:
- The wpt.fyi PR check compares head with base and lists the regressed
  tests (summary only).
- The [TypeScript bot](https://github.com/Microsoft/TypeScript/wiki/Triggering-Typescript-Bot)
  runs the top repositories against a PR and against main, and lists the
  new errors.
- [Crater](https://rustc-dev-guide.rust-lang.org/tests/crater.html)
  builds and tests every published crate under two compilers.
- rustc's compare modes and Chromium's
  [virtual suites](https://chromium.googlesource.com/chromium/src/+/HEAD/docs/testing/web_tests.md)
  fall back to the base test's golden. A mode's own file exists only where
  its output moved: a footprint, stored.

**The combination: not found.** The research found no system in industry
that judges a corpus by which outputs a mutant moves, or that puts code
perturbation and input perturbation in one model. The nearest:
- SQLite's criterion, above.
- Görz et al. ([USENIX Security 2023](https://www.usenix.org/conference/usenixsecurity23/presentation/gorz),
  summary only) grade fuzzers by the mutants their inputs kill. The input
  perturbers are judged by code perturbations.
- Hughes's [*How to Specify It!*](https://research.chalmers.se/publication/517894/file/517894_Fulltext.pdf)
  (summary only) runs eight planted bugs against each kind of property.
  The metamorphic properties missed none.

The closest whole systems are rustc's compiletest (revisions and compare
modes as columns, `--bless`, `known-bug`), Chromium's web tests with WPT
(tests × platforms × virtual suites, expectations with bugs, PR deltas),
and GraphicsFuzz with spirv-fuzz (recorded transforms that carry facts,
bugs fed back into the corpus). None of them has mutants.

## 7. What hosting mutants would take

A sketch, not a design. All untested.

- **A declaration.** A mutant beside each transform: a name, a description
  (rule 3), a site, and the property whose guard it breaks. One package
  owns it, as one owns each lift, suite and transform.
- **A prediction, derived.** The footprint the declarations imply: P's
  slice, in the columns that reject P and the columns past them. A
  hypothesis (#20) can state the prediction where the declarations can't,
  such as for a guard no `Rejects` names.
- **Neutral perturbations too.** A perturbation predicted to move no cell,
  as a metamorphic constant is (section 6), is the code axis's
  invariance claim. A mutant is its directional counterpart. Both are
  delta predictions, made before the run.
- **A run that never writes.** A check-mode run of the packages that
  register the corpus, under the mutant's layer. It never runs an update,
  since an update would write the mutant's outputs into the archives as
  goldens.
- **A report.** For each mutant, its footprint against its prediction:
  cells outside the prediction, cells inside it that didn't move, and the
  reasons the cells give.
- **Language independence.** The layer in section 4, chosen per platform,
  behind one interface: apply a mutant, run the matrix, discard.

## 8. Open questions

- **Can the prediction always be derived?** On the sheets it could,
  because the guard's property is one that a column rejects. A guard that
  shapes an output, without refusing an input, has no property to derive
  from.
- **Masking.** `Resolve` checks for dangling references before cycles, so a
  case that is both circular and dangling would not move under this mutant.
  The corpus has no such case. Should a footprint that a check's order
  masks be predicted, or flagged?
- **Frozen cells and gaps.** A frozen cell whose bytes a mutant moves fails
  as any moved cell does. Should a mutant's report show a known gap's
  cells apart from the rest?
- **Seeded defects (#54).** #54 seeds defects by hand, hides their
  manifests, and measures who finds them. A mutant is a visible seeded
  defect, with its footprint as its manifest. Whether mutants could supply
  #54's defect classes, or whether being visible rules them out, is open.
