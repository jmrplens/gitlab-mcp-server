# AI Model Evaluation

> **Diátaxis type**: Explanation
> **Audience**: Users, evaluators, maintainers
> **Prerequisites**: Basic understanding of MCP tools and GitLab operations

Model evaluation asks whether a real model can use `gitlab-mcp-server` to do a
real thing to a real GitLab. It is not a benchmark of prose. It is a benchmark
of tool use: choosing the action, shaping the arguments, carrying a
confirmation where one is required, and finishing without being corrected.

It exists because an MCP server is an interface for models. A tool can be right
for a person and hard for a model: a description that does not say which of two
actions to use, a schema too large to read, a parameter name a model has to
learn from a rejection, an error that says what went wrong and not what to do.
None of that shows up in a unit test.

## What it measures, and against what

The whole of it runs in `test/e2e/modeleval`. A run boots a GitLab, starts the
**real `cmd/server` binary** and drives it over stdio, so a model call crosses a
process boundary and then the network, exactly as a client's would.

That is the load-bearing choice, and it is a correction of the evaluator this
replaced. That one registered a server inside its own process. Everything the
binary does on the way to serving a tool — its startup, its middleware chain,
the post-registration visibility pass — was absent, so a read-only evaluation
scored a surface on which the interactive create flows still created. What is
measured now is the surface a client is served, because it is the same process
serving it.

## What is asked

The corpus is `internal/testutil/modelcorpus`: **258 cases** declaring **464
steps** and naming **313 of 1084 catalog actions** across 42 of 46 domains.
Three kinds, by prefix:

| Prefix | Kind                | What it asks                                                       |
| ------ | ------------------- | ------------------------------------------------------------------ |
| `MT-`  | Single operation    | One clear task that should take one action.                        |
| `MS-`  | Multi-step workflow | Several actions in the order the task requires.                    |
| `MF-`  | Failure handling    | A step the server refuses, or one that needs an explicit approval. |

[Model evaluation corpus breadth](model-corpus.md) is the generated ledger of
what that covers, per domain and per tier. Read it as breadth and never as
coverage: it says which actions a model is asked to reach, and nothing about
how well any model reaches them.

**A case declares its answer, and the prompt may not contain it.** Each case
carries a key: the steps, the action of each, the parameters that must be
right, whether the step is destructive, and the world the case needs. The
prompt is written separately and is held to a rule — a stimulus may not name
its own case's tool, action or parameter — because a prompt that hands the
model the call turns tool selection into copying. That rule is
`TestContract_NoStimulusNamesItsOwnAnswer`, an ordinary unit test that runs on
every `go test ./internal/...`, and `TestContract_FindsAPlantedAnswer` beside
it proves the rule can fail. Where the literal really is the request (a case
about creating a branch called `release`), the finding is written down in
`declarations.go` with its reason rather than waved through.

## Observation and verdict are separate

A run writes down **what happened** and computes **nothing**. Every attempt,
turn, call and post-run check lands in a shard as a record of observation: what
the model was sent, what it emitted, which tool it called with which arguments,
what the server dispatched, what GitLab held afterwards, and what the request
was billed for.

The verdict is computed afterwards, by `cmd/gen_model_results`, from those
shards and from the corpus at HEAD.

The split is what makes a correction cheap and a published number durable. A
scoring rule fixed today can re-score a run from last month without spending a
token, through `make model-results-refold`. The price is that **the shards are
the only thing a correction can be applied to**: the committed record holds
scored columns, redrawing scores nothing, and a run whose shards were thrown
away can never be re-scored.

## The seven columns

Each is a ratio, and both halves are published. A column reading 100% over one
attempt and one reading 100% over ninety are not the same claim.

| Column              | Numerator                                                     | Denominator                   |
| ------------------- | ------------------------------------------------------------- | ----------------------------- |
| Reached             | Steps reached, or correctly declined                          | Non-optional steps declared   |
| Accepted first time | Steps whose first call about them was the one that reached it | Steps reached                 |
| Argument fidelity   | Arguments whose value matched the truth                       | Arguments declared comparable |
| Confirmation        | Destructive steps whose reaching call carried the approval    | Destructive steps declared    |
| Unaided             | Attempts that completed with no refusal of ours anywhere      | Attempts run                  |
| Completion          | Attempts that completed                                       | Attempts run                  |
| Overhead            | Catalog searches plus refusals for arguments                  | Steps reached                 |

Two of them need their shape explained.

**Argument fidelity compares values, not names.** An argument the case declares
a truth for is compared against that truth; an argument whose value the case
leaves to the model is in neither half. A figure that compared only parameter
names would say a model placed `title` correctly while it wrote the wrong title.

**Overhead keeps its two halves apart** and publishes them separately beside the
combined rate. A discovery call is the dynamic surface's declared cost — the
catalog is not in the tool list, so a model must search it, and that is the
design. An `invalid_params` refusal is a model learning a parameter name from a
rejection, which is what the default opaque meta schema leaves it to do, and it
is the number that says what that mode costs. One rate over both would hide
each inside the other.

## What is counted and then left out

Five outcomes are counted, published, and kept out of every column. None of
them is the model's:

| Outcome        | Why it is not the model's                                               |
| -------------- | ----------------------------------------------------------------------- |
| Skipped        | The instance did not meet the case's needs; it never ran.               |
| Unobserved     | It ran and the server's span never arrived, so nothing can be said.     |
| Provider error | The provider would not answer.                                          |
| Harness error  | This side broke.                                                        |
| GitLab refused | GitLab refused a call the model dispatched with the declared arguments. |

Counting a skip would rank a model by the licence of the instance it was
measured on. Counting the other four would publish a provider's outage, a bug
of ours or a fixture's state as a model that did not finish the task.

**Declines are published apart**, in two kinds, because they are not one thing.
A model that explains in text that a read-only deployment will not do this is
what such a deployment wants. A model that sends the call and is refused is the
conversation ending correctly and nothing more. Folding them together would
credit the second with the first.

## Reading a row

A row is a measurement, and its identity is everything that changes **what** was
measured rather than how well it went: the model, the surface, the protective
mode, the tier and whether it was pinned, the meta schema mode, the slice size,
the corpus and contract digests, the tool-schema digest as that provider
received it, and the repeat count. Two rows differing anywhere there are two
measurements and never two readings of one.

Beside that, the provenance: the commit, the date, the instance's edition and
version, the credential's scopes, how many tools the session served, and what
the row cost as four token figures. Never one figure: folding a cache read into
an input token is how a table came to claim sixty thousand tokens against five
million.

A row with a hole in its provenance is refused rather than published with the
hole, because the reader who would be misled is the one comparing it with
another row.

**Surfaces are comparison classes.** Dynamic and meta are measured by default.
The individual surface is not, and that is a decision rather than an oversight:
its whole tool list is 682,878 tokens at Ultimate, over at least one provider's
context window outright, so a model can only be shown a deterministic slice of
it — and a measurement within a slice is a different question from choice
across a whole catalog. Where an individual row exists it carries both the
budget and the span its attempts were actually shown, since a case whose own
domains outnumber the budget is shown all of them.

## What a run costs

It asks a paid provider, so **nothing schedules it**. There is no CI job, no
nightly, no hook. A run happens when somebody decides to spend the money and
says so: `MODELEVAL_SPEND=yes` is required before a real provider is called,
`MODELEVAL_BUDGET_USD` is the ceiling it stops at, and a model the price table
has no figure for refuses to start unless `MODELEVAL_UNPRICED` says otherwise.

A rehearsal costs nothing. `MODELEVAL_MODELS=fake:perfect` replays the corpus
key through the whole pipe — the binary, GitLab, the recording, the scoring and
the pages — without a provider call. What it proves is that the machinery runs;
what it cannot prove is anything about a model, which is why folding a fake run
refuses every row it would publish, by name.

See [the developer guide](model-evaluation-developer.md) for how to run one,
add a case, and read a shard, and [the results page](model-results.md) for what
is published today.
