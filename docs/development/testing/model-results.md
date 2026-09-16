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
`make model-results-refold MODELEVAL_SHARDS=<dir>`, which drops the rows those
shards publish, names each one it dropped, and folds them again. Keep a paid
run's shards: a run whose shards are gone cannot be re-scored.

## What the columns mean

Every column is a numerator over a denominator, both printed, with a dash when
there was nothing to divide. That is the first thing the withdrawn tables did
not do: a rate over one attempt and a rate over ninety looked the same, and a
column with nothing behind it read as a perfect score.

| Column              | Numerator                                                                                | Denominator                   |
| ------------------- | ---------------------------------------------------------------------------------------- | ----------------------------- |
| Reached             | steps reached in order, or correctly declined where the mode withholds them              | non-optional steps declared   |
| Accepted first time | steps whose first call about them was the one that reached them                          | steps reached                 |
| Argument fidelity   | argument values that matched their truth                                                 | arguments declared comparable |
| Confirmation        | destructive steps whose reaching call carried the confirmation                           | destructive steps declared    |
| Unaided             | attempts completed with no `invalid_params` and no `needs_confirmation` refusal anywhere | attempts run                  |
| Completion          | attempts completed                                                                       | attempts run                  |
| Overhead            | catalog searches plus `invalid_params` retries, with the two halves also printed apart   | steps reached                 |

An argument the case declares authored, such as the text of an issue title, is
in neither half of the fidelity column: nothing compares prose a model was free
to write. Five kinds of attempt are counted and then left out of every column,
because none of them is the model's: one the instance could not offer, one whose
server span never arrived, one the provider would not answer, one this side
broke, and one GitLab refused after a correct dispatch with the right arguments.
Each is published beside the columns rather than folded into them.

Tokens are four numbers and never one. A cache read is not an input token, and
adding them together is how one withdrawn table came to publish 62,638 tokens
against five million.

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
  for.** The prompt builder in `cmd/eval_mcp_surfaces` interpolates the
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
