# hypercase beside training corpora, and in categorical terms

Two ways of looking at the structure hypercase builds. Neither is needed to
use it. They are for reasoning about where it could go, and much of what
follows is speculative. Where it is, it says so.

The structure, in one paragraph:
- Corpus types are input types, and lifts are functions between them that
  can fail; the type a lift produces is the stage it reaches. Columns are
  the lifts and suites; a suite observes what a lift produces.
- A case is one input, and a cell is one case run through one column.
- A property P classifies cases: a case's polarity (P, anti-P or neither) is
  a fact about its input. A hypothesis classifies columns: a column's
  polarity (P, anti-P or neutral) is a claim.
- A transform makes a variant of a case. A pair is a case and a variant that
  differ in P.

## 1. Training corpora and hyperparameters

| hypercase | ML data and evaluation practice |
|---|---|
| a corpus of cases | a dataset or benchmark |
| a corpus type | a schema or modality |
| a column | the function under evaluation: a model, or a pipeline stage |
| the matrix of outcomes | an evaluation matrix: models × items |
| a property, a case's polarity | a data slice, or an attribute |
| a predicate that classifies cases | a labeling function |
| a transform, a variant | a perturbation or augmentation |
| a neutral column over pairs | an invariance test |
| a P or anti-P column | a directional expectation |
| a reverse map | data attribution |
| parameters fixed in Go | hyperparameters |

**Slices and labels.** A case's polarity is slice membership. Its sources
come in a fixed order:
1. **Transforms.** The label is known by construction, as it is for
   synthetic data.
2. **Predicates.** These are programmatic labels, as in weak supervision
   (Snorkel's labeling functions). They are cheap, and can disagree with
   each other.
3. **Declarations.** These are manual annotation, the most expensive, so
   they come last.

ML data work converges on the same order, for the same reason: a label is
cheapest to get exactly where the data was generated.

**Behavioral testing.** CheckList (Ribeiro et al., 2020) tests NLP models
with three kinds of test, and each has a hypercase counterpart:

| CheckList | What it tests | hypercase |
|---|---|---|
| minimum functionality tests | fixed expected outputs per case | an ordinary golden |
| invariance tests | a perturbation should not change the output | a neutral column over pairs |
| directional expectation tests | a perturbation should change the output in a known way | a P or anti-P column over pairs |

Metamorphic testing (Chen et al., 1998) is the older software-testing form of
the same idea. What hypercase adds is where the claim sits. It is on the
column, as a declared polarity, apart from the cases. So one property is
checked against every column at once, and on the changes between two
revisions as well as at one.

**Hypotheses first.** Stating a hypothesis before reading results is
preregistration. Inference that only explains violations of stated
hypotheses avoids the forking-paths problem: slice an evaluation enough ways
and something always looks significant, and with thousands of cells and many
properties it will.

**Attribution.** ML has to estimate which training examples shaped an
output: influence functions, datamodels. A reverse map knows exactly,
because each output declaration states its inputs. The two agree in shape
and differ in kind: one is declared, the other estimated.

**Hyperparameters.** A column's parameters are fixed in Go, as a model's
hyperparameters are fixed for one evaluation. There are two ways to expand
that, and they are not the same:
- **Varying a column's parameters adds a third axis,** cases × columns ×
  settings, which is a sweep. It multiplies the columns.
- **Parameters carried by the case, an `in/params/` tree,** are per-example
  conditioning, closer to a prompt than to a hyperparameter. They refine the
  cases.

**Where the analogy breaks.**
- **No noise.** Everything here is deterministic and compared byte for byte.
  ML evaluation is statistical; a cell has no noise to average away.
- **No held-out set.** Goldens are committed beside the code, to be read by
  whoever changes it. They are a specification, so "contamination" doesn't
  apply. The risk in its place is an update that accepts whatever the code
  now does. That is why accepting a change in a cell's state should take an
  explicit step.

**Speculative: the corpus as training data.** A corpus is a structured,
versioned record. For each case it holds:
- the input, and each column's output;
- the code's verdict;
- where each output line came from;
- the case's property labels, and the columns' declared polarities.

Across revisions it adds each change's delta, the predictions stated for
it, and whether they held. That is the shape of supervision for agents that
change code. Untested.

## 2. In categorical terms

This part is all speculative. It offers a precise vocabulary, not a claim
that the implementation needs one.

**Types and lifts.**
- Corpus types and stages are objects.
- A lift can fail, so it is a Kleisli arrow `A → E + B` for the error monad
  `E + –`.
- Lifts compose by Kleisli composition.
- A blocked cell is what that composition does with a failure: the error
  propagates, and nothing downstream runs.

**Exactly one path.** From each root type there is exactly one path to each
type a suite consumes. In the free category on the lift graph, every
diagram out of a root then commutes trivially: there are never two composites
to compare. Allowing two paths would turn commutativity into a claim to
test: that both routes give the same observation. That would be a test kind
of its own.

**Cases and cells.**
- A case is a global element `x: 1 → R` of its root type `R`.
- Take a column `s` over a type `T`, reached by the path `p: R → T`. Its cell
  for `x` is the composite `s ∘ p ∘ x: 1 → E + Out`.
- The matrix evaluates every column at every case. It is an
  Outcome-valued relation between the set of cases and the set of columns.

**Chaining.** A bytes lift's committed product, under `via/`, factors the
composite through an intermediate object and records that object's value. A
normal run reads the recorded value, which cuts the composite at that point.
So a change upstream shows at the lift, and not in every column downstream.

**Variants.** A transform is an endomorphism `t: R → R` on a root type. It is
partial, because it may decline a base. A pair is `(x, t ∘ x)`.

**Neutral is invariance.**
- A neutral column that claims "equals base" says `s ∘ p ∘ t = s ∘ p`. The
  column is invariant under `t`, so it factors through the coequalizer of
  `id` and `t`.
- Neutral on outcome only is the same equation after projecting
  `Out → Outcome`.
- A directional claim is, more generally, equivariance: `s ∘ p ∘ t = g ∘ s ∘
  p` for some `g` on outputs. Metamorphic relations are such squares.

**Polarity is a claim about factoring.**
- Case polarity is a map `π_C: Cases → {P, anti-P, neither}`.
- Column polarity is a partial map `π_S: Columns → {P, anti-P, neutral}`,
  undefined on the columns a hypothesis doesn't list.
- A hypothesis claims that, where `π_S` is defined, the outcome relation
  factors through `π_C × π_S` and a small prediction table.
- Its violations are the cells where it doesn't factor.

**Properties across types.**
- A property may have a predicate `P_A` on `A` and `P_B` on `B`. It is
  coherent along a lift `f: A → B` when `P_B ∘ f = P_A` wherever `f`
  succeeds: `f` preserves the property.
- Then `f` carries a pair in `A` to a pair in `B`, and a neutral claim about
  a column over `B` can be checked with pairs made in `A`.
- Where `f` doesn't preserve the property, a conclusion drawn within one type
  doesn't carry across to the other. This is the precise sense in which a
  property can manifest differently in different types.
- Cases move forward along lifts, covariantly: `A` to `B`. Predicates move
  backward, contravariantly: a predicate on `B` gives one on `A`, `P_B ∘ f`.
  So one predicate, defined at a type that many ladders reach, classifies
  the cases of every root type upstream of it.
- Across those root types there are no pairs, so the conclusions are
  correlational: across unpaired cases, does a column's outcome covary with
  the cases' polarity? Within an archive, pairs give the controlled form of
  the same question.

**Deltas.** Two revisions interpret the same diagram twice. The lift and
suite names stay; the functions behind them change. A delta prediction says
the two interpretations agree on the neutral columns for a change aimed at
P. Loosely, it is a natural transformation between two functors from the
registered diagram into the Kleisli category, the identity on the neutral
part.

**Reverse maps.** Each output declaration pairs a forward function with a
backward map, from output lines to input ranges. Forward-and-backward pairs
are what lenses and the other optics formalize. thema's lenses are the same
family, between schema versions instead of between inputs and outputs.
Whether reverse maps compose as optics do, through `via/` products, is
still open: so far a range through a product points at the product's own
lines.
