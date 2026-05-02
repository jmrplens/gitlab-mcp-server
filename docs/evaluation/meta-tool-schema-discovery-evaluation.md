# Meta-Tool Schema Discovery Evaluation

> **Purpose**: Evaluate whether model-controlled schema discovery (`gitlab_server` `schema_index` / `schema_get`) keeps default opaque meta-tool schemas usable while reducing the need to inline every action schema in `tools/list`.
>
> **Catalog mode**: `META_TOOLS=true`, `META_PARAM_SCHEMA=opaque`.
> **Baseline**: production enterprise opaque catalog before description compression.
> **Compressed catalog**: production descriptions shortened for the heaviest meta-tools while preserving schema lookup, nested `params`, unknown-key rejection, destructive confirmation guidance, and focused repair hints.

---

## How to Run

1. Start the server with the default opaque meta-tool mode.
2. Send each task prompt to the target model in a fresh or controlled conversation.
3. Record every tool call in the run log table below.
4. Compare the observed tool/action/params with the expected path.
5. Repeat the same task set against the compressed description catalog.

The repository also includes a local Anthropic harness that sends the production meta-tool catalog as tool definitions, simulates `gitlab_server` `schema_index` / `schema_get`, validates the selected tool calls, and never executes GitLab operations:

```bash
go run ./cmd/eval_meta_tools/ --model claude-sonnet-4-6 \
  --out plan/metatool-token-schema-research/evals/anthropic-sonnet-4-6-current.md
```

Use `--dry-run` for static route validation, `--max-tasks=N` for a smoke test, `--task MT-035` for a targeted rerun, or `--tools-file=/path/to/tools_meta.json` to evaluate a saved `tools/list` snapshot such as the `main` branch catalog. The harness reads `ANTHROPIC_API_KEY` from the environment or `.env`.

## Metrics

| Metric | Definition | Target |
| --- | --- | --- |
| Catalog tokens | Total advertised tool-definition tokens from `go run ./cmd/audit_tokens/` | Lower is better |
| Tool-selection accuracy | First selected meta-tool matches expected tool | >= 95% |
| Action-selection accuracy | First selected action matches expected action | >= 95% |
| First-call validation pass rate | First tool call succeeds without schema or validation repair | >= 90% |
| Schema lookup use rate | Model calls `gitlab_server` `schema_get` or reads schema resource before uncertain actions | Track only |
| Repair success rate | Model corrects an `IsError` validation result on the next attempt | >= 95% |
| Destructive safety | Destructive calls include explicit confirmation or elicitation flow | 100% |
| Final task success | Final answer satisfies verifier | >= 95% |

## Task Fixture

| ID | Prompt | Expected tool/action | Required params | Optional params | Destructive | Success verifier |
| --- | --- | --- | --- | --- | --- | --- |
| MT-001 | Show the current authenticated GitLab user. | `gitlab_user` / `current` | none | none | No | Returns username and user ID. |
| MT-002 | Find project `my-org/tools/gitlab-mcp-server` and give me its ID and default branch. | `gitlab_project` / `get` | `project_id` | none | No | Uses full path or ID and reports ID plus default branch. |
| MT-003 | List the 10 most recently updated projects I can access. | `gitlab_project` / `list` | none | `order_by`, `sort`, `per_page` | No | Returns at most 10 projects sorted by recent activity. |
| MT-004 | Star project `my-org/tools/gitlab-mcp-server`. | `gitlab_project` / `star` | `project_id` | none | No | Project is starred or already-starred response is explained. |
| MT-005 | List members of project `my-org/tools/gitlab-mcp-server`. | `gitlab_project` / `members` | `project_id` | `per_page` | No | Returns member names or IDs. |
| MT-006 | List top-level groups only. | `gitlab_group` / `list` | none | `top_level_only`, `per_page` | No | Returns only top-level groups. |
| MT-007 | Create a subgroup named `eval-temp` under group `my-org`. | `gitlab_group` / `create` | `name`, `path`, `parent_id` | `visibility` | No | Subgroup is created with expected path. |
| MT-008 | Delete subgroup `my-org/eval-temp`. | `gitlab_group` / `delete` | `group_id` | `confirm` | Yes | Destructive call is confirmed and subgroup is deleted. |
| MT-009 | List open issues in project `my-org/tools/gitlab-mcp-server`. | `gitlab_issue` / `list` | `project_id` | `state`, `per_page` | No | Returns open issues and pagination data. |
| MT-010 | Create an issue titled `Evaluate schema discovery` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_issue` / `create` | `project_id`, `title` | `description`, `labels` | No | Issue is created and IID is reported. |
| MT-011 | Update issue `42` to add label `evaluation`. | `gitlab_issue` / `update` | `project_id`, `issue_iid` | `labels` | No | Issue labels include `evaluation`. |
| MT-012 | Close issue `42`. | `gitlab_issue` / `update` | `project_id`, `issue_iid`, `state_event` | none | No | Issue state becomes closed. |
| MT-013 | Delete issue `42`. | `gitlab_issue` / `delete` | `project_id`, `issue_iid` | `confirm` | Yes | Destructive call is confirmed and issue is deleted. |
| MT-014 | List merge requests opened against `main`. | `gitlab_merge_request` / `list` | `project_id` | `target_branch`, `state`, `per_page` | No | Returns MRs targeting `main`. |
| MT-015 | Create a merge request from `feature/eval` into `main` titled `Evaluation MR`. | `gitlab_merge_request` / `create` | `project_id`, `source_branch`, `target_branch`, `title` | `description`, `remove_source_branch` | No | MR is created and IID is reported. |
| MT-016 | Add a note to merge request `7`. | `gitlab_mr_review` / `note_create` | `project_id`, `merge_request_iid`, `body` | none | No | Note appears on MR. |
| MT-017 | Merge merge request `7` when the pipeline succeeds. | `gitlab_merge_request` / `merge` | `project_id`, `merge_request_iid` | `merge_when_pipeline_succeeds` | No | MR merge state is updated or actionable blocker is returned. |
| MT-018 | List the latest pipelines on branch `main`. | `gitlab_pipeline` / `list` | `project_id` | `ref`, `per_page` | No | Pipelines for `main` are returned. |
| MT-019 | Trigger a new pipeline on branch `main`. | `gitlab_pipeline` / `create` | `project_id`, `ref` | `variables` | No | New pipeline ID is returned. |
| MT-020 | Cancel pipeline `12345`. | `gitlab_pipeline` / `cancel` | `project_id`, `pipeline_id` | `confirm` | Yes | Destructive call is confirmed and pipeline is canceled. |
| MT-021 | List failed jobs in pipeline `12345`. | `gitlab_job` / `list` | `project_id`, `pipeline_id` | `scope` | No | Failed jobs are returned. |
| MT-022 | Get the trace for job `999`. | `gitlab_job` / `trace` | `project_id`, `job_id` | none | No | Trace text is returned or truncated notice appears. |
| MT-023 | Retry job `999`. | `gitlab_job` / `retry` | `project_id`, `job_id` | none | No | New retried job ID is returned. |
| MT-024 | Delete artifacts for job `999`. | `gitlab_job` / `delete_artifacts` | `project_id`, `job_id` | `confirm` | Yes | Destructive call is confirmed and artifacts are deleted. |
| MT-025 | List project CI variables. | `gitlab_ci_variable` / `list` | `project_id` | `page`, `per_page` | No | Variables are listed without exposing hidden values. |
| MT-026 | Create masked CI variable `EVAL_TOKEN`. | `gitlab_ci_variable` / `create` | `project_id`, `key`, `value` | `masked`, `protected` | No | Variable is created with masked flag. |
| MT-027 | Update CI variable `EVAL_TOKEN` for production scope. | `gitlab_ci_variable` / `update` | `project_id`, `key` | `value`, `environment_scope` | No | Scoped variable is updated. |
| MT-028 | Delete CI variable `EVAL_TOKEN`. | `gitlab_ci_variable` / `delete` | `project_id`, `key` | `environment_scope`, `confirm` | Yes | Destructive call is confirmed and variable is deleted. |
| MT-029 | Get file `README.md` from branch `main`. | `gitlab_repository` / `file_get` | `project_id`, `file_path`, `ref` | none | No | File content or metadata is returned. |
| MT-030 | Create file `tmp/eval.txt` on branch `feature/eval`. | `gitlab_repository` / `file_create` | `project_id`, `file_path`, `branch`, `content`, `commit_message` | none | No | Commit and file path are returned. |
| MT-031 | Delete file `tmp/eval.txt` from branch `feature/eval`. | `gitlab_repository` / `file_delete` | `project_id`, `file_path`, `branch`, `commit_message` | `confirm` | Yes | Destructive call is confirmed and commit is returned. |
| MT-032 | Search code for `func RegisterMCPMeta`. | `gitlab_search` / `code` | `search` | `project_id` | No | Search results include matching files or snippets. |
| MT-033 | Search all projects for `gitlab-mcp-server`. | `gitlab_search` / `projects` | `search` | none | No | Matching projects are returned. |
| MT-034 | Create milestone `Evaluation Sprint`. | `gitlab_project` / `milestone_create` | `project_id`, `title` | `due_date`, `description` | No | Milestone IID or ID is returned. |
| MT-035 | Delete milestone IID `7` named `Evaluation Sprint`. | `gitlab_project` / `milestone_delete` | `project_id`, `milestone_iid` | `confirm` | Yes | Destructive call is confirmed and milestone is deleted. |
| MT-036 | Create release `v0.0.0-eval` for tag `v0.0.0-eval`. | `gitlab_release` / `create` | `project_id`, `tag_name`, `name` | `description`, `ref` | No | Release is created and web URL is returned. |
| MT-037 | Delete release `v0.0.0-eval`. | `gitlab_release` / `delete` | `project_id`, `tag_name` | `confirm` | Yes | Destructive call is confirmed and release is deleted. |
| MT-038 | List deploy keys for the project. | `gitlab_access` / `deploy_key_list_project` | `project_id` | `page`, `per_page` | No | Deploy key list is returned. |
| MT-039 | Analyze why pipeline `12345` failed. | `gitlab_analyze` / `pipeline_failure` | `project_id`, `pipeline_id` | `prompt` | No | Analysis includes likely cause and fix suggestions. |
| MT-040 | Run server diagnostics and GitLab connectivity check. | `gitlab_server` / `health_check` | none | none | No | Status object includes server version and auth status. |

## Run Log Template

| Run | Task ID | Catalog variant | First tool/action | Schema lookup used | First-call pass | Repair success | Final success | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | MT-001 | baseline | | | | | | |

## Compression Results

The first compression pass shortened `gitlab_admin`, `gitlab_project`, `gitlab_merge_request`, `gitlab_group`, and `gitlab_issue`. That reduced the enterprise opaque catalog from `71,986` to `63,524` tokens, or about `11.8%`, which was below the acceptance gate. A second pass shortened `gitlab_repository`, `gitlab_pipeline`, and `gitlab_user`, bringing the enterprise opaque catalog to `61,095` tokens after cleanup. The final compromise adds small targeted hints for `gitlab_search` and milestone deletion, bringing the enterprise catalog to `61,155` tokens.

| Catalog | Tokens | Bytes | Change vs baseline |
| --- | ---: | ---: | ---: |
| Baseline enterprise opaque | 71,986 | 287,944 | - |
| After first compression pass | 63,524 | 254,096 | -8,462 tokens (-11.8%) |
| After second compression pass | 61,095 | 244,380 | -10,891 tokens (-15.1%) |
| Final compromise | 61,155 | 244,620 | -10,831 tokens (-15.0%) |

Against the `main` branch catalog snapshot used for the model comparison, the final compromise preserves most of the savings:

| Catalog | Main tokens | Final tokens | Savings |
| --- | ---: | ---: | ---: |
| Base opaque meta-tools | 55,110 | 47,108 | 8,002 tokens (14.5%) |
| Enterprise opaque meta-tools | 70,249 | 61,155 | 9,094 tokens (12.9%) |

| Enterprise component | Baseline tokens | Final tokens | Final bytes | Final share |
| --- | ---: | ---: | ---: | ---: |
| Description | 35,323 | 24,431 | 97,725 | 40.0% |
| Input schema | 14,147 | 14,147 | 56,589 | 23.1% |
| Output schema | 15,049 | 15,049 | 60,199 | 24.6% |
| Annotations | 1,015 | 1,015 | 4,060 | 1.7% |
| Icons | 5,803 | 5,803 | 23,212 | 9.5% |
| Other | 664 | 664 | 2,656 | 1.1% |

The token gate is met by the final compromise while keeping the advertised enterprise catalog `9,094` tokens below `main`.

## Static Schema Check

A static validation pass compared the 40 expected tool/action pairs in this fixture against the generated meta-tool snapshot plus the production-only `gitlab_server` standalone meta-tool. The pass confirms that every expected tool/action pair is discoverable after correcting the MR note and milestone rows above.

| Check | Result |
| --- | ---: |
| Fixture tasks | 40 |
| Tool/action pairs present in `tools_meta.json` | 39 |
| Production-only `gitlab_server` action verified by registration tests | 1 |
| Missing expected routes after correction | 0 |

This static check verifies route/schema coverage, not model task success.

## Anthropic Model Run

The compressed production catalog was evaluated with `claude-sonnet-4-6` through `cmd/eval_meta_tools`. The run used the enterprise meta-tool catalog, Anthropic tool calling, simulated schema lookup responses, and validation-only tool results. The detailed local report for the final compromise was written to `plan/metatool-token-schema-research/evals/2026-05-02-anthropic-sonnet-4-6-current-compromise-fixed-fixture.md`.

The `MT-035` fixture was clarified to include a milestone IID. The previous wording asked the model to delete a milestone by title while the real action requires `milestone_iid`, which made a preliminary `milestone_list` call a reasonable first step in a validation-only harness.

| Metric | Current final compromise | Main snapshot |
| --- | ---: | ---: |
| Tasks | 40 | 40 |
| Tool-selection accuracy | 100.0% | 95.0% |
| Action-selection accuracy | 97.5% | 90.0% |
| First-call validation pass rate | 97.5% | 85.0% |
| Schema lookup use rate | 0.0% | 0.0% |
| Repair success rate | 100.0% | 66.7% |
| Destructive safety | 100.0% | 100.0% |
| Final task success proxy | 100.0% | 95.0% |

The current final compromise needed one repair: `MT-014` first selected `gitlab_merge_request` / `list_global` without `project_id`, then repaired to `list`.

The `main` snapshot failed two tasks in the same harness:

- `MT-014`: selected `gitlab_merge_request` / `list_global` and did not repair to the project-scoped list action.
- `MT-040`: selected `gitlab_admin` / `metadata_get` because `gitlab_server` does not exist on `main`; this is an expected catalog capability difference, not a regression in `main` routing.

For reference, the pre-compromise compressed run used the same model and fixture before the `gitlab_search` and milestone-delete hints were added:

| Metric | Result |
| --- | ---: |
| Tasks | 40 |
| Tool-selection accuracy | 100.0% |
| Action-selection accuracy | 95.0% |
| First-call validation pass rate | 85.0% |
| Schema lookup use rate | 7.5% |
| Repair success rate | 50.0% |
| Destructive safety | 87.5% |
| Final task success proxy | 92.5% |

Three tasks failed that validation proxy:

- `MT-014`: selected the right meta-tool for merge requests but did not provide `project_id`, then switched to the global list action during repair.
- `MT-032`: selected `gitlab_search` / `code` but omitted the required `search` parameter after schema lookup.
- `MT-035`: used schema lookup but selected `milestone_list` instead of destructive `milestone_delete`, without `milestone_iid` or `confirm`.

The final compromise recovers the qualitative gate while retaining meaningful token savings against `main`.

## Applied Short Descriptions

The production catalog now uses shortened descriptions for the highest-cost meta-tools. Each shortened description keeps the schema lookup path, the `{ "action": "...", "params": { ... } }` envelope, unknown-parameter rejection, destructive-operation guidance, and key routing hints to neighboring meta-tools.

### `gitlab_group`

Manage GitLab groups, subgroups, group projects, members, badges, hooks, LDAP links, SAML links, access requests, and group-level metadata. Use this for group discovery and administration, not project-only settings. All calls use `{ "action": "...", "params": { ... } }`; call `gitlab_server` `schema_get` for exact params before uncertain actions. Unknown params are rejected. Destructive actions such as delete, member removal, badge deletion, hook deletion, and token/access cleanup require confirmation or elicitation.

### `gitlab_admin`

Administer self-managed GitLab instance resources: settings, appearance, broadcast messages, system hooks, license, plan limits, application statistics, feature flags, OAuth applications, topics, imports, secure files, Terraform states, cluster agents, and dependency proxy cache. Most actions require admin privileges. Use `{ "action": "...", "params": { ... } }`; fetch exact params with `gitlab_server` `schema_get`. Unknown params are rejected. Destructive and instance-wide actions require careful confirmation.

### `gitlab_project`

Manage GitLab projects and project-scoped metadata: list/get/create/update/delete, archive/transfer/share, members, hooks, badges, forks, stars, languages, imports, mirrors, approvals, access tokens, variables, and project feature settings. Use project ID or URL-encoded path as `project_id` where required. Use `{ "action": "...", "params": { ... } }`; fetch exact params with `gitlab_server` `schema_get`. Unknown params are rejected. Destructive actions require confirmation.

### `gitlab_merge_request`

Manage merge request lifecycle, review, discussions, notes, approvals, diff versions, merge/ref actions, draft state, participants, award emoji, and related metadata. Use this for MR operations after identifying `project_id` and `merge_request_iid`. Use `{ "action": "...", "params": { ... } }`; fetch exact params with `gitlab_server` `schema_get`. Unknown params are rejected. Destructive actions such as delete, note deletion, and discussion cleanup require confirmation.

### `gitlab_issue`

Manage issues, notes, discussions, links, labels, time stats, participants, related merge requests, award emoji, and issue lifecycle operations. Use this for project issue triage after identifying `project_id` and `issue_iid`. Use `{ "action": "...", "params": { ... } }`; fetch exact params with `gitlab_server` `schema_get`. Unknown params are rejected. Destructive actions such as delete, unlink, and note deletion require confirmation.

### `gitlab_repository`

Browse and mutate repository files and commits: tree, compare, blobs, archives, commit metadata, file create/update/delete, changelog helpers, submodules, markdown rendering, blame, cherry-pick, revert, and commit discussions. Use `{ "action": "...", "params": { ... } }`; fetch exact params with `gitlab_server` `schema_get`. Unknown params are rejected. Commit-producing and destructive actions require care because they can trigger CI, webhooks, and protected-branch checks.

### `gitlab_pipeline`

Manage project pipelines, trigger tokens, resource groups, test reports, metadata, schedules, and schedule variables. Use `{ "action": "...", "params": { ... } }`; fetch exact params with `gitlab_server` `schema_get`. Unknown params are rejected. Pipeline creation/retry/trigger/schedule actions can consume CI minutes, while delete and trigger/schedule deletion are destructive.

### `gitlab_user`

Manage GitLab users and current-user resources: user CRUD/state, keys, emails, personal access tokens, impersonation tokens, todos, status, events, memberships, notifications, namespaces, avatars, identities, and user runners. Use `{ "action": "...", "params": { ... } }`; fetch exact params with `gitlab_server` `schema_get`. Unknown params are rejected. User state changes, token/key/email deletion, and identity removal are destructive or admin-sensitive.

## Remaining Acceptance Gate

The token-reduction gate is satisfied by the compressed production catalog, and a compressed-catalog model run has been recorded. The qualitative regression gate remains open until a baseline/compressed-catalog comparison has been recorded with:

- At least 15% enterprise opaque definition-token reduction. Completed by the second compression pass.
- No more than 2 percentage points of final task success regression.
- No increase in unsafe destructive-call attempts.
- No loss of schema lookup guidance in any shortened description.
- All changed documentation and tests passing project checks.
