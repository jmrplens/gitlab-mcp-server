# AI Model Evaluation Developer Guide

> **Diátaxis type**: How-to
> **Audience**: 🔧 Maintainers
> **Prerequisites**: Docker, a provider API key for a paid run, and
> [AI Model Evaluation](model-evaluation.md) for what any of it means

This page is how to run the evaluation, add a case, and read what a run left
behind. It assumes the explanation page for why the numbers are shaped the way
they are.

## Source map

| Path                                   | What lives there                                                                       |
| -------------------------------------- | -------------------------------------------------------------------------------------- |
| `test/e2e/modeleval`                   | The runner: configuration, the conversation loop, the session, the slice, the recorder |
| `test/e2e/modeleval/internal/provider` | One adapter per provider, the price table, and the digest of what was sent             |
| `internal/testutil/modelcorpus`        | The corpus: cases, their answer keys, the worlds they need, the prompt boundary rule   |
| `internal/testutil/modelrecord`        | The shard record: the six line types, the writer and the reader                        |
| `internal/testutil/modelscore`         | The scorer: how one attempt becomes a verdict and verdicts become a row                |
| `cmd/gen_model_corpus`                 | The breadth ledger                                                                     |
| `cmd/gen_model_results`                | The fold: shards into the committed record, and the record into the pages              |

The runner carries the `e2e` build tag, so `go build ./...` never compiles it
and `cmd/server` never links any of it.

## Rehearse first: it costs nothing

`fake:perfect` replays each case's own answer key through the entire pipe —
the real binary, a real GitLab, the recording, the scoring and the pages —
without a provider call.

```bash
MODELEVAL_MODELS=fake:perfect make modeleval-ce
```

Do this after any change to the runner, the record or the scorer. What it
proves is that the machinery runs end to end. What it cannot prove is anything
about a model, which is why folding a fake run publishes nothing: every row is
refused by name, saying the fake answers from the corpus key.

## Run a paid evaluation

A real provider needs a key in the environment, consent, and a ceiling.

| Provider spec prefix | Key variable        |
| -------------------- | ------------------- |
| `anthropic:`         | `ANTHROPIC_API_KEY` |
| `openai:`            | `OPENAI_API_KEY`    |
| `google:`            | `GOOGLE_API_KEY`    |
| `qwen:`              | `QWEN_API_KEY`      |

```bash
export ANTHROPIC_API_KEY=...
MODELEVAL_SPEND=yes \
MODELEVAL_BUDGET_USD=25 \
MODELEVAL_MODELS='anthropic:claude-haiku-4-5-20251001' \
make modeleval-ce
```

`make modeleval-ee` is the same against an ephemeral licensed GitLab EE, which
is what the licensed half of the corpus needs.

Before spending on a full run, ask each provider whether it accepts the request
this repository builds. It is two requests per model plus one slice request, it
needs no GitLab, and it is the one paid thing here that is not a run:

```bash
MODELEVAL_PROBE=yes MODELEVAL_MODELS='anthropic:claude-haiku-4-5-20251001' make modeleval-probe
```

## Settings

All of them are read by the harness; `cmd/server` links none of this.

| Setting                       | Default       | Meaning                                                           |
| ----------------------------- | ------------- | ----------------------------------------------------------------- |
| `MODELEVAL_MODELS`            | —             | Comma-separated `provider:model;key=value` specs to ask           |
| `MODELEVAL_SPEND`             | no            | Consent to call a real provider; `yes`, `true` or `1`             |
| `MODELEVAL_BUDGET_USD`        | —             | Ceiling in US dollars the run stops at                            |
| `MODELEVAL_UNPRICED`          | no            | Allow a model the price table has no figure for                   |
| `MODELEVAL_SURFACES`          | dynamic, meta | Surfaces to measure                                               |
| `MODELEVAL_MODE`              | default       | Protective mode: `default`, `read-only` or `safe-mode`            |
| `MODELEVAL_TIER`              | —             | Tier to pin on the server; empty detects it                       |
| `MODELEVAL_META_PARAM_SCHEMA` | opaque        | Meta-tool input-schema mode to serve                              |
| `MODELEVAL_CASES`             | every case    | Comma-separated case IDs to ask                                   |
| `MODELEVAL_REPEAT`            | 1             | Times each attempt is run, so a per-case pass rate is publishable |
| `MODELEVAL_PARALLEL`          | 1             | Attempts of one model in flight at once                           |
| `MODELEVAL_SLICE`             | 128           | Tools a model is shown on the individual surface                  |
| `MODELEVAL_PROBE`             | no            | Consent for the provider contract probe                           |

Each counted setting has a ceiling, and the ceiling is there to turn a typo
into a refusal rather than into a bill: a repeat of 1000 multiplies a sweep's
cost by a thousand, and it is a mistyped figure every time.

## Re-run one case

A case that failed for a reason you have since fixed does not need the other
257 re-run. Name it, and fold the result in on its own:

```bash
MODELEVAL_SPEND=yes MODELEVAL_CASES=MS-037 \
MODELEVAL_MODELS='anthropic:claude-haiku-4-5-20251001' \
make modeleval-ce

make model-results-record MODELEVAL_SHARDS=dist/modeleval/ce
```

## Publish what a run observed

A run writes observation and scores nothing. The numbers are computed when you
fold it in:

```bash
make model-results-record MODELEVAL_SHARDS=dist/modeleval/ce   # fold in and redraw
make model-results-refold MODELEVAL_SHARDS=dist/modeleval/ce   # re-score under today's rules
make gen-model-results                                          # redraw from the record alone
make check-model-results                                        # the offline gate CI runs
```

**Keep the shards.** They are under `dist/modeleval/`, which Git ignores, and
they are the only thing a corrected scoring rule can be applied to: the
committed record holds scored columns, a redraw scores nothing, and a run whose
shards were discarded can never be re-scored. A second fold of a run already
published is refused by name rather than replacing it, which is what `-refold`
is for.

## Add or change a case

1. Put the case in the file its kind belongs to under
   `internal/testutil/modelcorpus`: `read.go`, `mutating.go`, `destructive.go`,
   or the `licensed_*` sibling when it needs Premium or Ultimate.
2. Declare its key: the steps in order, the action of each, the parameters that
   must match, whether the step is destructive, and the world the case needs.
3. Write the prompt without naming the tool, the action or any parameter of its
   own key. If the literal genuinely is the request, add an entry to
   `declarations.go` with the reason rather than leaving the finding to be
   waved through.
4. Run `go test ./internal/testutil/modelcorpus/ -count=1`. The boundary rule,
   the world coverage and the key accessors all gate there.
5. Run `make gen-model-corpus` and commit the ledger it rewrites.
6. Rehearse with `MODELEVAL_MODELS=fake:perfect make modeleval-ce`: the fake
   walks the key, so a case whose world or key is wrong fails without a
   provider call.

## Triage a failure

The question to answer first is **whose failure it is**, because four of the
five things that can go wrong are not the model's and the record says which:

| Where it reads           | Value                   | It means                                                               | Do this                                  |
| ------------------------ | ----------------------- | ---------------------------------------------------------------------- | ---------------------------------------- |
| the attempt's `ended_by` | `skipped`               | The instance did not meet the case's needs; it never ran               | Nothing, or run the EE target            |
| the attempt's `ended_by` | `harness_error`         | This side broke                                                        | Fix the harness; the `reason` says where |
| the attempt's `ended_by` | `provider_error`        | The provider would not answer                                          | Re-run; check the probe                  |
| a step's answer          | `gitlab_refused`        | GitLab refused a correctly dispatched call with the declared arguments | Fix the fixture or the case's world      |
| the attempt's `ended_by` | `completed`, column red | The model did the wrong thing                                          | This is the finding                      |

Only the last is a result about a model. The first four are counted, published
and kept out of every column precisely so they cannot be read as one.

**The triage rule that matters most:** when a case fails because the model
could not tell which action to use, the fix is the tool's description, its
aliases or its error message — not the prompt. Editing a stimulus until a case
passes is the defect the boundary test exists to refuse, and it converts a
measurement into a restatement of the answer.

## Keep the documentation current

- Cases, worlds or the corpus shape change: `make gen-model-corpus`, then
  update [AI Model Evaluation](model-evaluation.md) if what is measured moved.
- A run is folded in: `make model-results-record`, which redraws
  [AI Model Evaluation Results](model-results.md) and the README blocks.
- Tests added or moved: `go run ./cmd/gen_testing_docs/`.
- Never hand-write a figure into a page. What a page says comes from the
  record, and the provenance beside it is what makes it a measurement.
