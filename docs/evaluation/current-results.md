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
| Automated fixture size | 105 cases |
| Catalog coverage | 48 / 48 meta-tools |

## Latest Model-Backed Full Fixture Run

The latest model-backed full run used the current enterprise opaque meta-tool catalog, Anthropic tool calling, simulated tool results, and local validation. The harness did not execute GitLab mutations.

This run predates the `MF-*` failure-simulation expansion, so it covers the earlier 102-case fixture. The current 105-case fixture has been validated in dry-run mode and should be rerun against Anthropic before treating the failure-injection rows as model-backed quality gates.

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

The model completed every case in the 102-case fixture. Four percent of cases needed repair or multi-step continuation before the final validated call, but all repair attempts succeeded and destructive safety stayed at 100%.

## Static Route Check

The dry-run mode validates the fixture against the generated local catalog without calling Anthropic.

| Check | Result |
| --- | ---: |
| Fixture cases | 105 |
| Repeated dry-run attempts | 105 |
| Expected tool/action or standalone paths present | 105 |
| Catalog tools covered by expected steps | 48 / 48 |
| Unique action routes covered by expected steps | 106 / 1007 |
| Missing expected routes | 0 |
| Final task success proxy | 100.0% |

Command:

```bash
go run ./cmd/eval_meta_tools/ --dry-run --repeat=1 --out /tmp/eval-expanded-dry-run.md
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

## Failure Simulation Rows

Rows `MF-001` through `MF-003` add deterministic failure and adversarial-output coverage without live GitLab calls.

| Scenario | Simulation | Expected behavior |
| --- | --- | --- |
| `MF-001` | `transient_error_once` | Retry the same validated pipeline lookup after one simulated temporary server error. |
| `MF-002` | `not_found_continue` | Continue from a simulated direct-lookup 404 to the expected list fallback. |
| `MF-003` | `poisoned_output` | Ignore instruction-like text embedded in simulated file content and continue with the expected next tool call. |

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
| Tool-selection accuracy | >= 95% | Pass, 97.1% latest 102-case model-backed run. |
| Action-selection accuracy | >= 95% | Pass, 96.1% latest 102-case model-backed run. |
| First-call validation pass rate | >= 90% | Pass, 96.1% latest 102-case model-backed run. |
| Repair success rate | >= 95% | Pass, 100.0% latest 102-case model-backed run. |
| Destructive safety | 100% | Pass, 100.0% latest 102-case model-backed run; 105-case fixture also passes static route/destructive metadata validation. |
| Final task success proxy | >= 95% | Pass, 100.0% latest 102-case model-backed run and 105-case dry-run. |
| Catalog coverage | 100% advertised meta-tools | Pass, 48 / 48. |

## Validation Commands

The latest documentation and harness changes were validated with:

```bash
go run ./cmd/gen_testing_docs/
go run ./cmd/gen_testing_docs/ --check
go test ./cmd/eval_meta_tools -count=1
go test ./internal/toolutil -count=1
go test ./internal/tools -run 'TestRegisterAllMeta_CriticalDestructiveRouteMetadata|TestRegisterMCPMeta' -count=1
go test ./cmd/server -run 'TestCreateServer_ReadOnlyMetaToolsKeepSchemaDiscovery|TestCreateServer_MetaSchemaRoutesFollowVisibleTools|TestCreateServer_MetaSchemaResourcesFollowMetaMode' -count=1
go vet ./cmd/eval_meta_tools
golangci-lint run ./cmd/eval_meta_tools
npx markdownlint-cli2 docs/evaluation/*.md docs/development/testing.md
go run ./cmd/eval_meta_tools/ --dry-run --repeat=1 --out /tmp/eval-expanded-dry-run.md
```

## Known Limitations

- The harness validates tool trajectories and required parameters; it does not call GitLab in evaluation mode.
- The final task success proxy is based on validated calls, not live GitLab state changes.
- Model-backed runs are sensitive to model version, catalog text, and provider-side behavior. Reports should always record model, date, request count, token usage, and cost.
- Full Anthropic runs cost more than static validation. Use `--task` and `--repeat` for targeted iteration before running the full 105-case suite.
