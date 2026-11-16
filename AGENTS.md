# Working in hypercase

hypercase is a Go test framework, specified by a walkthrough that is
committed whole and then built one step at a time. Every change follows the
rules below.

## Rules

1. **No claim without a test.** This covers a mechanism ("X fails because
   Y"), a comparison ("better than main"), a regression ("fails on main")
   and a fix ("this fixes Z"). It applies to issues, PR descriptions, commit
   messages and review replies alike. Show the test failing without the
   change and passing with it. A claim that no test can show yet is
   labelled untested, or left out.
2. **Never a flaky test.** New tests pass `go test -count=N` before they are
   committed. Nothing a test compares depends on map iteration order, the
   clock, the environment or test order. A flake that turns up is fixed at
   its cause, never retried or skipped.
3. **Descriptions everywhere.** Everything a project registers with
   hypercase carries a description, and the framework enforces it. That
   covers properties, corpus types, lifts, suites, every output declaration
   (`stdout.txt`, `stderr.txt` and `error.txt` included), transforms,
   classifiers, corpus checks, and hypotheses once they exist. It also
   covers each archive's comment, which describes its base case, each
   hand-written variant's `variant/<name>/about`, and the text of each
   `gaps` line, which says what is right and names the issue it waits on. A
   missing description fails the run, but does not stop an update from
   writing outputs. hypercase's own corpus follows the rule too.
4. **No internals.** The repo is public. Issues, PRs, code, comments,
   examples and test data carry nothing from a private system: no internal
   hostnames, service names, error codes, ticket keys, account ids or
   people. Examples use the walkthrough's toy domains.
5. **Adversarial review before a PR leaves draft:** `/code-review` at high
   effort or above.
6. **Every review finding becomes a corpus case, fixed or not** (or a test,
   where no corpus exists yet). A fixed finding gets a case that fails
   without the fix. An unfixed one gets a case whose cell a `gaps` line
   marks as a known gap: the line says what is right and names the issue it
   waits on, and it freezes the cell, so the cell's committed outputs record
   today's behavior until a person removes the line. Where there is no
   corpus yet, the test records today's behavior and names the issue.
7. **The walkthrough is the spec.** `WALKTHROUGH.md` is committed whole
   before the code. A step's PR adds the runnable example under
   `walkthrough/` that the step's snippets come from, and corrects the doc
   where the implementation differs. From then on, CI fails when one of
   those snippets differs from its example.
