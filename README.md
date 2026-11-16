# hypercase

Without a structure to work in, agents writing tests keep inventing new
micro-patterns. People can't keep up, and neither can the agents that come
later. hypercase is a way of organizing tests that aims to close that gap,
by moving the discussion up a level: from single tests to what groups of
them say together.

## The name

`hyper` is for higher dimensions, and for hyperproperties: claims that
relate several runs, where an ordinary test's claim is about one. `case` is
the unit present at every step, from a row of a table test to one half of a
pair.

## The principle

Take conventional testing techniques, then group and slice them so they
yield higher-order information people can think in, while they can still
drop down to the lower-order detail.

Both ends matter. hypercase starts from tests familiar in conventional
practice and ends at hyperproperties. Each step's idea is useful without the
later ones, and the framework is worth having even if the last step never
pans out.

A matrix of cases × columns shows, in the change that causes it, how that
change moves other parts' outputs. txtar is only how a case is stored, not
the point.

## Status

In design. The spec is a walkthrough, `WALKTHROUGH.md`, committed whole
before the code (#2), with target snippets for the steps not built yet. Each
step's PR adds the runnable example its snippets come from, which CI runs,
and corrects the doc where the implementation differs.

## License

Apache-2.0. See [LICENSE](LICENSE).
