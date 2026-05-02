# Meta-Tool Schema Discovery Branch Review Report

## Scope

This report summarizes the `research/metatool-token-schema` branch relative to `main` as of 2026-05-02. It covers the full branch arc: early schema/refactor work, token-reduction changes, schema discovery, validation tooling, Anthropic model evaluations, full catalog workflow coverage, and the documentation cleanup added for review.

## Branch Summary

| Item | Value |
| --- | --- |
| Branch | `research/metatool-token-schema` |
| Baseline | `main` |
| Commits ahead of `main` | 8 committed changes before this documentation cleanup |
| Diff size before documentation cleanup | 34 files, 8,529 insertions, 6,290 deletions |
| Primary goal | Keep `META_TOOLS=true` and `META_PARAM_SCHEMA=opaque` usable while reducing advertised tool-definition tokens. |
| Main acceptance signal | 100% final task success on the latest model-backed fixture, 100% dry-run success on the expanded 134-case fixture, a 100% targeted model-backed capability trace sample, and 48 / 48 meta-tool coverage. |

## Commit Timeline

| Commit | Focus | Review notes |
| --- | --- | --- |
| `480d692e` | Add schema discovery and reduce catalog tokens. | Introduces schema discovery through `gitlab_server`, schema resources, meta-schema utilities, and initial catalog token reduction. |
| `eb1f79ce` | Record meta-tool schema check. | Adds early evaluation documentation and static schema checks. |
| `880edd0e` | Add Anthropic evaluation harness. | Adds `cmd/eval_meta_tools` with model-backed validation, simulated schema lookup, usage reporting, and task fixtures. |
| `3115af98` | Compare baseline catalog quality. | Adds snapshot comparison against `main` to verify token savings do not degrade routing quality. |
| `5c7fb461` | Document Anthropic env options. | Documents evaluation environment variables and model-run configuration. |
| `d8639246` | Compress additional catalog descriptions. | Shortens additional high-cost meta-tool descriptions while retaining schema lookup and safety guidance. |
| `be1231e0` | Harden evaluation and compress covered descriptions. | Adds wave-2 coverage and targeted tests before further compression. |
| `2143f28e` | Add full catalog workflow evaluation. | Expands to 102 cases, 48 / 48 catalog coverage, multi-step scenarios, standalone-tool support, and improved destructive-safety validation. |

## Phase 1: Schema And Meta-Tool Infrastructure

The branch first refactors shared meta-tool behavior so schema discovery can be a first-class capability instead of a large inline schema burden.

Key changes:

| Area | Files | What changed |
| --- | --- | --- |
| Meta schema utilities | `internal/toolutil/meta_schema.go`, `internal/toolutil/meta_schema_test.go` | Adds reusable schema indexing and schema lookup helpers for meta-tool action schemas. |
| Meta-tool runtime helpers | `internal/toolutil/metatool.go`, `internal/toolutil/confirm.go` | Improves envelope validation, unknown-parameter handling, and destructive confirmation metadata. |
| MCP schema resources | `internal/resources/meta_schema.go` | Exposes `gitlab://schema/meta/` and `gitlab://schema/meta/{tool}/{action}` resources. |
| Server registration | `internal/tools/register_mcp_meta.go`, `internal/tools/register_meta.go` | Adds `gitlab_server` schema actions and wires schema lookup into the meta-tool catalog. |
| Server configuration/docs | `.env.example`, `docs/env-reference.md`, `site/src/content/docs/configuration.mdx` | Documents `META_PARAM_SCHEMA` behavior and relevant environment options. |

Review focus:

- Confirm `schema_index` and `schema_get` return enough information for a model to recover from opaque params.
- Confirm schema resources are read-only and do not expose secrets.
- Confirm destructive-action confirmation is represented consistently between runtime behavior, schemas, and documentation.

## Phase 2: Token Reduction

The branch compresses the largest meta-tool descriptions while preserving routing cues and safety instructions.

Current token results:

| Catalog | Definition tokens | Bytes | Change versus original baseline |
| --- | ---: | ---: | ---: |
| Original enterprise opaque baseline | 71,986 | 287,944 | - |
| Final 40-task compromise | 61,155 | 244,620 | -10,831 tokens (-15.0%) |
| Expanded compressed catalog | 58,266 | 233,064 | -13,720 tokens (-19.1%) |
| Wave-2 compressed catalog | 56,896 | 227,584 | -15,090 tokens (-21.0%) |

Against the `main` snapshot used for the model comparison:

| Catalog area | Main tokens | Current tokens | Savings |
| --- | ---: | ---: | ---: |
| Base opaque meta-tools | 55,110 | 42,849 | 12,261 tokens (22.2%) |
| Enterprise opaque meta-tools | 70,249 | 56,896 | 13,353 tokens (19.0%) |

Review focus:

- Compare compressed descriptions in `internal/tools/register_meta.go` with the corresponding action maps.
- Check that neighboring-tool hints still route ambiguous tasks correctly.
- Check that destructive and credential-related actions still mention confirmation or one-time-secret constraints.

## Phase 3: Evaluation Harness

The branch adds a dedicated Go command under `cmd/eval_meta_tools`.

Capabilities:

| Capability | Details |
| --- | --- |
| Static dry-run | Validates expected routes and standalone tools without calling a model. |
| Anthropic tool-calling run | Sends the generated catalog to Anthropic and validates emitted tool calls. |
| Schema lookup simulation | Simulates `gitlab_server` `schema_index` and `schema_get` responses without executing GitLab operations. |
| Snapshot comparison | Runs against a saved `tools/list` snapshot through `--tools-file`. |
| Targeted reruns | Supports `--task`, `--max-tasks`, `--repeat`, `--pause`, and retry flags. |
| Usage reporting | Records requests, tool calls, input/output tokens, cache write/read tokens, and estimated cost. |
| Multi-step scenarios | Validates ordered workflows with multiple tool calls and simulated continuation. |
| Standalone tools | Handles interactive tools and project discovery tools that do not use the `{action, params}` envelope. |
| Trace artifacts | Writes per-case JSON traces, a `traces.jsonl` corpus, and an `index.md` for model-backed runs. |

Review focus:

- `parseTasksMarkdown` and `parseTaskRow` for fixture parsing.
- `validateStepCall`, `validateActionToolCall`, and `validateStandaloneToolCall` for route validation.
- `evaluateTask` for multi-step continuation and repair behavior.
- `calculateMetrics` for destructive-safety and repair-success semantics.

## Phase 4: Model Evaluation Results

Latest model-backed full fixture result:

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

Usage for the full run:

| Metric | Result |
| --- | ---: |
| Anthropic requests | 127 |
| Tool calls emitted | 136 |
| Input tokens | 8,637 |
| Output tokens | 10,768 |
| Cache creation input tokens | 64,562 |
| Cache read input tokens | 4,870,388 |
| Estimated cost | $1.8907 |

Current versus `main` on the 51-task comparison fixture:

| Metric | Current catalog | Main snapshot |
| --- | ---: | ---: |
| Tool-selection accuracy | 96.1% | 96.1% |
| Action-selection accuracy | 96.1% | 96.1% |
| First-call validation pass rate | 96.1% | 92.2% |
| Schema lookup use rate | 3.9% | 0.0% |
| Repair success rate | 100.0% | 75.0% |
| Destructive safety | 100.0% | 100.0% |
| Final task success proxy | 100.0% | 98.0% |

The `main` snapshot failed the server-diagnostics task because `gitlab_server` did not exist in that catalog.

The current working tree expands beyond this full run with sampling, elicitation, destructive-action, and trace-artifact coverage. The expanded fixture is validated by dry-run and unit tests, and a targeted Anthropic sample for representative new rows is recorded under `plan/metatool-token-schema-research/evals/2026-05-02-anthropic-sonnet-4-6-capability-trace-sample.md` with per-case traces in the sibling `.traces/` directory.

## Phase 5: Full Catalog Coverage

The final committed evaluation expansion plus the current follow-up adds:

| Coverage item | Count |
| --- | ---: |
| Single-operation cases | 116 |
| Multi-step scenarios | 13 |
| Failure simulation scenarios | 5 |
| Total automated cases | 134 |
| Expected tool operations across all cases | 174 |
| Catalog tools covered | 48 / 48 |
| Unique action routes covered by expected steps | 134 / 1007 |

Important additions:

- `MT-070` through `MT-092` close the remaining catalog coverage gaps.
- `MT-080` through `MT-083` verify standalone interactive tools.
- `MT-091` verifies that vulnerability routes use `project_path`, not `project_id`.
- `MS-001` through `MS-013` exercise cross-domain workflows with ordered steps.
- `MT-093` through `MT-098` broaden sampling coverage across `gitlab_analyze` actions.
- `MT-099` through `MT-116` add additional destructive coverage across branches, tags, pipelines, users, feature flags, custom emoji, wikis, merge requests, issues, access credentials, repository discussions, admin state, and project mirroring.
- `MS-011` through `MS-013` add elicitation, sampling, and feature-flag cleanup workflows.
- Destructive metrics now require confirmation only when the model attempts the expected destructive route, avoiding false safety failures for harmless read-only repair attempts.
- `MF-001` through `MF-005` add transient GitLab failure, 404 fallback, poisoned-output continuation, unsupported sampling fallback, and unsupported elicitation fallback scenarios.
- Fixture validation now compares destructive flags and listed params against live route metadata and generated action schemas.

## Phase 6: Documentation Cleanup

The documentation cleanup in this working tree reorganizes `docs/evaluation` into:

| Document | Purpose |
| --- | --- |
| `docs/evaluation/README.md` | Evaluation index and harness overview. |
| `docs/evaluation/automated-meta-tool-cases.md` | Human-readable explanation of all 134 automated cases. |
| `docs/evaluation/current-results.md` | Current static, model, token, and quality results. |
| `docs/evaluation/user-prompt-playbook.md` | Copy-ready prompts for manual user/model testing. |

The cleanup removes the earlier mixed documents that combined prompts, results, historical notes, and manual Spanish observations.

## Review Order Recommendation

1. Review `docs/evaluation/README.md`, `automated-meta-tool-cases.md`, and `current-results.md` first to understand the intended behavior and acceptance gates.
2. Review `cmd/eval_meta_tools/main.go` and `main_test.go` to validate that the harness measures what the docs claim.
3. Review `internal/toolutil/meta_schema.go`, `internal/resources/meta_schema.go`, and `internal/tools/register_mcp_meta.go` for the schema discovery contract.
4. Review compressed descriptions in `internal/tools/register_meta.go`, especially high-impact tools such as `gitlab_admin`, `gitlab_project`, `gitlab_group`, `gitlab_repository`, `gitlab_job`, `gitlab_access`, and `gitlab_ci_variable`.
5. Review generated catalog snapshot changes in `internal/tools/testdata/tools_meta.json` after understanding the source description changes.
6. Review user-facing docs under `docs/` and `site/src/content/docs/` last for consistency with implementation.

## Risks And Limitations

| Risk | Mitigation |
| --- | --- |
| Model-backed results can drift across provider versions. | Reports record model, date, token usage, and cost; use repeated runs before release. |
| The harness does not execute GitLab mutations. | It validates trajectory, parameters, and safety; live E2E remains separate. |
| Compressed descriptions may remove rare routing cues. | The 134-case fixture covers all advertised meta-tools and includes multi-step plus failure-injection workflows. |
| Generated `tools_meta.json` is large. | Review source descriptions first, then confirm generated changes are expected. |
| External links to old evaluation docs may break after cleanup. | Review docs index and known references; add redirect notes if external consumers depend on old filenames. |

## Validation Commands

Use focused validation before merging:

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
go run ./cmd/eval_meta_tools/ --task=MT-093,MT-099,MT-101,MF-004,MF-005 --repeat=1 --out plan/metatool-token-schema-research/evals/2026-05-02-anthropic-sonnet-4-6-capability-trace-sample.md --trace-dir plan/metatool-token-schema-research/evals/2026-05-02-anthropic-sonnet-4-6-capability-trace-sample.traces
```

For a full model-backed run:

```bash
go run ./cmd/eval_meta_tools/ \
  --model claude-sonnet-4-6 \
  --repeat=1 \
  --pause=250ms \
  --retries=8 \
  --retry-wait=65s \
  --out /tmp/eval-fullfixture.md
```

## Reviewer Checklist

- [ ] Schema discovery returns exact action schemas for opaque meta-tools.
- [ ] `META_PARAM_SCHEMA=opaque` remains the default documented path.
- [ ] Compressed descriptions still mention schema lookup, nested params, unknown-parameter rejection, and destructive confirmation where applicable.
- [ ] Destructive actions require explicit confirmation in schemas, validation, and evaluation cases.
- [ ] The 134-case fixture covers every advertised meta-tool and includes deterministic failure-injection and capability-fallback scenarios.
- [ ] Model-backed results meet the quality gates in `docs/evaluation/current-results.md`.
- [ ] Generated docs and test metrics are synchronized with `cmd/gen_testing_docs`.
- [ ] User-facing docs and developer docs describe the same configuration behavior.
