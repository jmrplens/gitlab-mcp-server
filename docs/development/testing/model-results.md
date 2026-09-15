# AI Model Evaluation Results

**This page publishes no results.** Every table it used to carry has been
withdrawn, and the measurement layer that produced them is being rebuilt as a
tagged package under `test/e2e/` on the end-to-end harness. The tables as they
last stood are readable at commit `4587cbfb3`.

The four managed sections (CE dynamic, CE meta-tools, Enterprise/Premium
meta-tools, Enterprise/Premium dynamic) are kept empty rather than deleted, so
that the first run of the rebuilt harness fills them the way the old publisher
did. Raw reports and traces are not committed.

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

`make eval-surfaces-docker SURFACE=meta` followed by `cmd/eval_mcp_surfaces --publish-docs` publishes a CE meta-tools run between the markers below, newest first.

<!-- START MODEL EVAL META RESULTS -->
<!-- END MODEL EVAL META RESULTS -->

## Enterprise Meta-Tools Results

<!-- START MODEL EVAL ENTERPRISE META RESULTS -->

Withdrawn. The Enterprise meta-tools run published here, dated 20260527, is readable at commit `4587cbfb3`.

<!-- END MODEL EVAL ENTERPRISE META RESULTS -->

## Enterprise Dynamic Results

<!-- START MODEL EVAL ENTERPRISE DYNAMIC RESULTS -->

Withdrawn. The Enterprise dynamic run published here, dated 20260628-015421, is readable at commit `4587cbfb3`.

<!-- END MODEL EVAL ENTERPRISE DYNAMIC RESULTS -->
