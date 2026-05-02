# Current Evaluation Results

This document summarizes the latest evaluation state for the `research/metatool-token-schema` branch.

## Snapshot

| Item | Value |
| --- | --- |
| Date | 2026-05-02 |
| Branch under review | `research/metatool-token-schema` |
| Baseline branch | `main` |
| Catalog mode | `META_TOOLS=true`, `META_PARAM_SCHEMA=opaque` |
| Model used for full run | `claude-sonnet-4-6` |
| Automated fixture size | 102 cases |
| Catalog coverage | 48 / 48 meta-tools |

## Latest Full Fixture Run

The full run used the current enterprise opaque meta-tool catalog, Anthropic tool calling, simulated tool results, and local validation. The harness did not execute GitLab mutations.

| Metric | Result |
| --- | ---: |
| Task and scenario attempts | 102 |
| Catalog tools covered | 48 / 48 |
| Tool-selection accuracy | 97.1% |
| Action-selection accuracy | 96.1% |
| First-call validation pass rate | 96.1% |
| Schema lookup use rate | 0.0% |
| Repair success rate | 100.0% |
| Destructive safety | 100.0% |
| Final task success proxy | 100.0% |

### Usage And Cost

| Metric | Result |
| --- | ---: |
| Anthropic requests | 127 |
| Tool calls emitted | 136 |
| Input tokens | 8,637 |
| Output tokens | 10,768 |
| Cache creation input tokens | 64,562 |
| Cache read input tokens | 4,870,388 |
| Estimated cost | $1.8907 |
| Pricing source | Default Claude Sonnet estimate in the harness |

### Interpretation

The model completed every case in the expanded fixture. Four percent of cases needed repair or multi-step continuation before the final validated call, but all repair attempts succeeded and destructive safety stayed at 100%.

## Static Route Check

The dry-run mode validates the fixture against the generated local catalog without calling Anthropic.

| Check | Result |
| --- | ---: |
| Fixture cases | 102 |
| Repeated dry-run attempts | 102 |
| Expected tool/action or standalone paths present | 102 |
| Catalog tools covered by expected steps | 48 / 48 |
| Missing expected routes | 0 |
| Final task success proxy | 100.0% |

Command:

```bash
go run ./cmd/eval_meta_tools/ --dry-run --repeat=1 --out /tmp/eval-final-dry-run.md
```

## Multi-Step Scenario Run

Rows `MS-001` through `MS-010` exercise ordered workflows with 3 to 5 expected tool operations each.

| Metric | Result |
| --- | ---: |
| Scenario attempts | 10 |
| Expected steps | 40 |
| Tool-selection accuracy | 100.0% |
| Action-selection accuracy | 100.0% |
| First-call validation pass rate | 100.0% |
| Schema lookup use rate | 0.0% |
| Repair success rate | 100.0% |
| Destructive safety | 100.0% |
| Final task success proxy | 100.0% |
| Estimated cost | $0.4296 |

The multi-step scenario run is useful as a faster smoke test for route sequencing, standalone tool calls, destructive confirmation, and cross-domain context retention.

## Current Versus Main Snapshot

A 51-task comparison was run against the current catalog and a `main` branch `tools/list` snapshot.

| Metric | Current catalog | Main snapshot |
| --- | ---: | ---: |
| Tasks | 51 | 51 |
| Tool-selection accuracy | 96.1% | 96.1% |
| Action-selection accuracy | 96.1% | 96.1% |
| First-call validation pass rate | 96.1% | 92.2% |
| Schema lookup use rate | 3.9% | 0.0% |
| Repair success rate | 100.0% | 75.0% |
| Destructive safety | 100.0% | 100.0% |
| Final task success proxy | 100.0% | 98.0% |

The `main` snapshot failed the server-diagnostics case because `gitlab_server` schema discovery did not exist in that catalog. This is a capability difference introduced by the branch, not a routing regression in `main`.

## Token Results

| Catalog | Definition tokens | Bytes | Change versus original baseline |
| --- | ---: | ---: | ---: |
| Original enterprise opaque baseline | 71,986 | 287,944 | - |
| Final 40-task compromise | 61,155 | 244,620 | -10,831 tokens (-15.0%) |
| Expanded compressed catalog | 58,266 | 233,064 | -13,720 tokens (-19.1%) |
| Wave-2 compressed catalog | 56,896 | 227,584 | -15,090 tokens (-21.0%) |

Against the `main` snapshot used for model comparison:

| Catalog area | Main tokens | Current tokens | Savings |
| --- | ---: | ---: | ---: |
| Base opaque meta-tools | 55,110 | 42,849 | 12,261 tokens (22.2%) |
| Enterprise opaque meta-tools | 70,249 | 56,896 | 13,353 tokens (19.0%) |

## Quality Gates

| Gate | Target | Current status |
| --- | ---: | --- |
| Tool-selection accuracy | >= 95% | Pass, 97.1% full fixture. |
| Action-selection accuracy | >= 95% | Pass, 96.1% full fixture. |
| First-call validation pass rate | >= 90% | Pass, 96.1% full fixture. |
| Repair success rate | >= 95% | Pass, 100.0% full fixture. |
| Destructive safety | 100% | Pass, 100.0% full fixture. |
| Final task success proxy | >= 95% | Pass, 100.0% full fixture. |
| Catalog coverage | 100% advertised meta-tools | Pass, 48 / 48. |

## Validation Commands

The latest documentation and harness changes were validated with:

```bash
go run ./cmd/gen_testing_docs/
go run ./cmd/gen_testing_docs/ --check
go test ./cmd/eval_meta_tools -count=1
go vet ./cmd/eval_meta_tools
golangci-lint run ./cmd/eval_meta_tools
npx markdownlint-cli2 docs/evaluation/*.md docs/development/testing.md
go run ./cmd/eval_meta_tools/ --dry-run --repeat=1 --out /tmp/eval-final-dry-run.md
```

## Known Limitations

- The harness validates tool trajectories and required parameters; it does not call GitLab in evaluation mode.
- The final task success proxy is based on validated calls, not live GitLab state changes.
- Model-backed runs are sensitive to model version, catalog text, and provider-side behavior. Reports should always record model, date, request count, token usage, and cost.
- Full Anthropic runs cost more than static validation. Use `--task` and `--repeat` for targeted iteration before running the full 102-case suite.
