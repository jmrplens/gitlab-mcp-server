# AI Model Evaluation Results

**This page publishes no results.** Every table it used to carry has been
withdrawn, and the measurement layer that produced them is being rebuilt as a
tagged package under `test/e2e/` on the end-to-end harness. The tables as they
last stood are readable at commit `4587cbfb3`.

The four managed sections (CE dynamic, CE meta-tools, Enterprise/Premium
meta-tools, Enterprise/Premium dynamic) are generated, empty sentence and all,
from `model-results.json` beside this file: `make gen-model-results` draws them,
`make check-model-results` gates them offline, and `make model-results-record`
is what folds a run's shards into the record they are drawn from. Raw reports
and traces are not committed.

A run writes observation and no verdict, so the columns in the record are this
tree's scoring of that observation, computed when the run was folded in. Drawing
a page re-reads them and scores nothing, and a second fold of a run already
published is refused by name rather than replacing it. A scoring rule corrected
later therefore reaches a published row by one path and no other:
`make model-results-refold MODELEVAL_SHARDS=<dir>`, which merges those shards
into the rows they publish again, case by case, naming each case it replaces.
The case is the unit and not the row: a re-run of one corrected case updates its
own entry and every other case keeps the figures it had. Keep a paid run's
shards: a run whose shards are gone cannot be re-scored.

## What the columns mean

<!-- START MODEL EVAL LEGEND -->

**How to read the tables below.** Every figure is a numerator over a denominator rather than a percentage, because a column reading 100% over one attempt and one reading 100% over ninety are not the same claim and a rate cannot tell them apart. The row here is invented; the columns are the real ones, drawn by the same code.

| Model                      |  Clean | Reached | Accepted first time | Argument fidelity | Confirmation | Unaided | Completion |                           Overhead |
| -------------------------- | -----: | ------: | ------------------: | ----------------: | -----------: | ------: | ---------: | ---------------------------------: |
| `example:not-a-real-model` | 5 / 10 | 17 / 19 |             14 / 17 |           24 / 26 |        3 / 4 |  6 / 10 |     7 / 10 | 12 / 17 (9 find, 3 invalid_params) |

- **Clean** — The headline. Attempts that went right end to end **with no help at all**: the task finished, every step was reached, every argument the case gives a truth for matched, every destructive step carried its approval, nothing of ours had to refuse anything, and what it did to GitLab checked out afterwards. It is a conjunction of the columns after it, never an average of them: there is no defensible weighting between them, so a weighted score would be a reading of whoever chose the weights.
- **Reached** — Steps the model got to, or correctly declined, over the steps the case declares. A step is reached when a call named it and the server dispatched it.
- **Accepted first time** — Of the steps reached, how many were reached by the **first** call about them. A second call means the first was refused and repaired.
- **Argument fidelity** — Argument **values** compared against the truth the case declares, not argument names. A model that places `title` correctly and writes the wrong title fails here, which is the whole reason values are compared.
- **Confirmation** — Destructive steps whose reaching call carried the approval, over destructive steps declared. A deletion that ran without one is a finding, not a faster model.
- **Unaided** — Attempts that finished with no refusal of ours anywhere. Weaker than Clean: an attempt can be unaided and still have written a wrong value.
- **Completion** — Attempts that finished the task at all. Weakest of the three: it says the conversation ended correctly and nothing about how.
- **Overhead** — What getting there cost, per step reached, with the two halves apart. A `find` is the dynamic surface's declared cost, since the catalog is not in the tool list and has to be searched. An `invalid_params` is the model learning a parameter name from a rejection, which is what the opaque meta schema leaves it to do. One rate over both would hide each inside the other.

What the columns leave out: an attempt the instance could not offer, one the server's span never described, one the provider would not answer, one this side broke, and one GitLab refused after a correct dispatch. None of the five is the model's, so none is in any denominator.

| Model                      | Attempts | Turns | Skipped | Unobserved | Provider errors | Harness errors | GitLab refused |
| -------------------------- | -------: | ----: | ------: | ---------: | --------------: | -------------: | -------------: |
| `example:not-a-real-model` |       10 |    23 |       2 |          1 |               1 |              0 |              1 |

- **Attempts** — How many attempts are behind the figures above. The five columns after Turns are **not** in this number.
- **Turns** — Provider requests the row paid for, which is not the same as calls: one turn can carry several tool calls, and a refused request is a turn of its own.
- **Skipped** — The instance did not meet the case's needs, so it never ran. Counting it would rank a model by the license of the instance it was measured on.
- **Unobserved** — It ran and the server's span never described it, so nothing can be said about it either way.
- **Provider errors** — The provider would not answer.
- **Harness errors** — This side broke.
- **GitLab refused** — GitLab refused a call the model dispatched correctly, with the arguments the case declares. That reports the instance or the fixture, not the model.

Tokens, never folded into one figure: a cache read is not an input token, and a table that added them together is how sixty thousand tokens came to be published against five million.

| Model                      |  Input | Output | Cache created | Cache read |
| -------------------------- | -----: | -----: | ------------: | ---------: |
| `example:not-a-real-model` | 120000 |   4200 |         30000 |      88000 |

- **Input** — Uncached input tokens.
- **Output** — Output tokens, reasoning included where the provider bills it there.
- **Cache created** — Input tokens written into the provider's prompt cache.
- **Cache read** — Input tokens served from it, billed at a fraction of the others.

And one thing no column carries: a row is only comparable with another row that agrees with it on surface, mode, tier, schema mode, corpus and tool schemas. The caption above each table says what that table holds fixed, and two tables with different captions are two measurements rather than two readings of one.
<!-- END MODEL EVAL LEGEND -->

## Which tables may be compared with which

Two rules, because there are two questions.

- **Across models**, holding the surface fixed: two rows belong in one table
  only when they agree on surface, protective mode, tier and tier pin, meta
  schema mode, slice size, the corpus and contract fingerprints, the tool
  schemas as that provider received them, and the repeat count. The tool-schema
  fingerprint is in the key because a provider-specific rewrite of the schemas
  is a different surface, whatever the row is called.
- **Across surfaces**, holding the model fixed: everything above except the
  surface and the three things a surface decides for itself, which are the meta
  schema mode, the slice size and the tool-schema fingerprint. Each such row is
  labeled with whichever of the first two it has.

Anything else goes in a table of its own, captioned with what that table holds
fixed. One comparison is never honest whatever the key says: the meta surface's
default `opaque` schema mode withholds parameter names, so a model on it learns
them from the tool description and from `invalid_params` refusals and from
nothing else, and its argument fidelity cannot be read beside a dynamic row,
where `gitlab_find_action` returns the schema, or an individual row, where the
tool carries it. What the mode costs shows up in the overhead column instead.

### What a slice size is

The individual surface publishes one tool per action, and its whole `tools/list`
is 682,878 tokens at Ultimate, 648,852 at Premium and 539,274 at Free/CE
([token footprint](../token-footprint.md)). That is past the context window of
at least one of the providers outright and leaves no room for a conversation on
the others, so nothing there can be measured by sending the whole list.

A case is therefore put to a model on a slice of the catalog, chosen by
`test/e2e/modeleval/slice.go` and seeded by the case identifier: every tool of
the domains the case's own steps touch, every tool registered outside the
catalog, and distractors from other domains filled to a budget, which is 128 by
default and set by `MODELEVAL_SLICE`. The order mixes the two, so position says
nothing about which tool the case needs. The session still serves its whole
catalog and a call to a tool outside the slice is dispatched like any other:
what the slice bounds is what the model was shown, never what it was allowed to
do.

Two consequences a reader has to hold. The row publishes the budget **and** the
span its attempts were actually shown, because the slice is chosen per case and
a row aggregates many: the budget is one number, a case whose own domains
outnumber it is shown all of them rather than fewer tools than it needs, and a
caption carrying only the budget would seat an attempt shown 312 tools under
the figure 128. The caption reads `slice of 128 tools (shown 96 to 312, 2 over
budget)` where the attempts differed, and the served-tools figure in the
provenance table beside it says what they were shown out of. And an individual
row measures tool choice **within a slice**,
which is a different question from choice across a whole catalog, so it is a
comparison class of its own and never a column beside a dynamic or meta row that
was shown everything.

## Withdrawn: what the numbers measured, and what they did not

Kept as history, because a reader who followed a link to a figure is owed the
account of why it is gone. These four facts were published beside the tables
before they were withdrawn, and each is a reason no run could have failed for
the reasons it was meant to catch:

- **They were taken from trees that are not in the history.** The CE dynamic
  and Enterprise dynamic tables come from `901ce569286f`, the Enterprise
  meta-tools table from `fe2715491ac7`. Neither commit is an ancestor of
  `main`, so nothing on this page can be reproduced by checking out a released
  version.
- **For part of the corpus the prompt contains the call the scorer checks
  for.** The prompt builder in the evaluator that produced these numbers
  (`cmd/eval_mcp_surfaces`, deleted with them) interpolated the
  expected tool, action and parameters into the task text, and the system
  prompt names the correct action and parameter for about twenty domains. A
  tool-selection or action-selection figure taken from those tasks measures how
  reliably a model copies a value it was handed.
- **Recovery counts repairs made from an answer key.** When a first call fails
  validation, the harness replies with the exact envelope the step expected.
  Each such reply is one repair attempt, and repair attempts are the
  denominator of the Recovery column, so the column says how often a model
  applies a correction it was handed verbatim.
- **The scorer compares parameter names, never their values.** A call is
  accepted when the required parameter names are present and no forbidden or
  unknown name is; the only argument value read anywhere in the pass is
  `confirm`. A project id pointing at the wrong project scores the same as the
  right one.

The gate that holds this is in the publisher: a report may not be published
unless its header declares `Stimulus: uncoached`, which only a run whose
prompts withhold the expected call can write. That gate stays in force and is
one of the things the rebuild carries over.

The four facts above share one cause, which is why repairing them one at a time
did not work. A single struct fed the stimulus the model was given, the
environment it acted in, the scorer that graded it and the report, all at once;
two of those four are answer-keyed by construction, so no channel-by-channel
repair could finish. The rebuild separates them, which the end-to-end harness
already does for the suite that drives the real binary.

## Dynamic Results

<!-- START MODEL EVAL DYNAMIC RESULTS -->

Withdrawn. The CE dynamic run published here, dated 20260627-232303, is readable at commit `4587cbfb3`.
<!-- END MODEL EVAL DYNAMIC RESULTS -->

## Meta-Tools Results

A CE meta-tools run is published between the markers below by `make model-results-record`, which folds the shards of a run of `make modeleval-ce` into the record and redraws every block from it.

<!-- START MODEL EVAL META RESULTS -->

No CE meta-tools run has been published here. The rebuilt harness has not put the corpus to a model.
<!-- END MODEL EVAL META RESULTS -->

## Enterprise Meta-Tools Results

<!-- START MODEL EVAL ENTERPRISE META RESULTS -->

Withdrawn. The Enterprise meta-tools run published here, dated 20260527, is readable at commit `4587cbfb3`.
<!-- END MODEL EVAL ENTERPRISE META RESULTS -->

## Enterprise Dynamic Results

<!-- START MODEL EVAL ENTERPRISE DYNAMIC RESULTS -->

Withdrawn. The Enterprise dynamic run published here, dated 20260628-015421, is readable at commit `4587cbfb3`.
<!-- END MODEL EVAL ENTERPRISE DYNAMIC RESULTS -->
