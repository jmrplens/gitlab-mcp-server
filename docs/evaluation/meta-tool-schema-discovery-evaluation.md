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

Use `--dry-run` for static route validation, `--repeat=N` for repeated runs, `--max-tasks=N` for a smoke test, `--task MT-035` for a targeted rerun, or `--tools-file=/path/to/tools_meta.json` to evaluate a saved `tools/list` snapshot such as the `main` branch catalog. The harness reads `ANTHROPIC_API_KEY` from the environment or `.env` and reports Anthropic request/tool-call counts, token usage, and optional cost estimates.

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
| MT-011 | Update issue `42` in project `my-org/tools/gitlab-mcp-server` to add label `evaluation`. | `gitlab_issue` / `update` | `project_id`, `issue_iid` | `labels` | No | Issue labels include `evaluation`. |
| MT-012 | Close issue `42` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_issue` / `update` | `project_id`, `issue_iid`, `state_event` | none | No | Issue state becomes closed. |
| MT-013 | Delete issue `42` from project `my-org/tools/gitlab-mcp-server`. | `gitlab_issue` / `delete` | `project_id`, `issue_iid` | `confirm` | Yes | Destructive call is confirmed and issue is deleted. |
| MT-014 | List merge requests opened against `main` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_merge_request` / `list` | `project_id` | `target_branch`, `state`, `per_page` | No | Returns MRs targeting `main`. |
| MT-015 | Create a merge request in project `my-org/tools/gitlab-mcp-server` from `feature/eval` into `main` titled `Evaluation MR`. | `gitlab_merge_request` / `create` | `project_id`, `source_branch`, `target_branch`, `title` | `description`, `remove_source_branch` | No | MR is created and IID is reported. |
| MT-016 | Add a note to merge request `7` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_mr_review` / `note_create` | `project_id`, `merge_request_iid`, `body` | none | No | Note appears on MR. |
| MT-017 | Merge merge request `7` in project `my-org/tools/gitlab-mcp-server` when the pipeline succeeds. | `gitlab_merge_request` / `merge` | `project_id`, `merge_request_iid` | `merge_when_pipeline_succeeds` | No | MR merge state is updated or actionable blocker is returned. |
| MT-018 | List the latest pipelines on branch `main` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_pipeline` / `list` | `project_id` | `ref`, `per_page` | No | Pipelines for `main` are returned. |
| MT-019 | Trigger a new pipeline on branch `main` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_pipeline` / `create` | `project_id`, `ref` | `variables` | No | New pipeline ID is returned. |
| MT-020 | Cancel pipeline `12345` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_pipeline` / `cancel` | `project_id`, `pipeline_id` | `confirm` | Yes | Destructive call is confirmed and pipeline is canceled. |
| MT-021 | List failed jobs in pipeline `12345` for project `my-org/tools/gitlab-mcp-server`. | `gitlab_job` / `list` | `project_id`, `pipeline_id` | `scope` | No | Failed jobs are returned. |
| MT-022 | Get the trace for job `999` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_job` / `trace` | `project_id`, `job_id` | none | No | Trace text is returned or truncated notice appears. |
| MT-023 | Retry job `999` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_job` / `retry` | `project_id`, `job_id` | none | No | New retried job ID is returned. |
| MT-024 | Delete artifacts for job `999` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_job` / `delete_artifacts` | `project_id`, `job_id` | `confirm` | Yes | Destructive call is confirmed and artifacts are deleted. |
| MT-025 | List CI variables in project `my-org/tools/gitlab-mcp-server`. | `gitlab_ci_variable` / `list` | `project_id` | `page`, `per_page` | No | Variables are listed without exposing hidden values. |
| MT-026 | Create masked CI variable `EVAL_TOKEN` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_ci_variable` / `create` | `project_id`, `key`, `value` | `masked`, `protected` | No | Variable is created with masked flag. |
| MT-027 | Update CI variable `EVAL_TOKEN` for production scope in project `my-org/tools/gitlab-mcp-server`. | `gitlab_ci_variable` / `update` | `project_id`, `key` | `value`, `environment_scope` | No | Scoped variable is updated. |
| MT-028 | Delete CI variable `EVAL_TOKEN` from project `my-org/tools/gitlab-mcp-server`. | `gitlab_ci_variable` / `delete` | `project_id`, `key` | `environment_scope`, `confirm` | Yes | Destructive call is confirmed and variable is deleted. |
| MT-029 | Get file `README.md` from branch `main` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_repository` / `file_get` | `project_id`, `file_path`, `ref` | none | No | File content or metadata is returned. |
| MT-030 | Create file `tmp/eval.txt` on branch `feature/eval` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_repository` / `file_create` | `project_id`, `file_path`, `branch`, `content`, `commit_message` | none | No | Commit and file path are returned. |
| MT-031 | Delete file `tmp/eval.txt` from branch `feature/eval` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_repository` / `file_delete` | `project_id`, `file_path`, `branch`, `commit_message` | `confirm` | Yes | Destructive call is confirmed and commit is returned. |
| MT-032 | Search code for `func RegisterMCPMeta`. | `gitlab_search` / `code` | `search` | `project_id` | No | Search results include matching files or snippets. |
| MT-033 | Search all projects for `gitlab-mcp-server`. | `gitlab_search` / `projects` | `search` | none | No | Matching projects are returned. |
| MT-034 | Create milestone `Evaluation Sprint` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_project` / `milestone_create` | `project_id`, `title` | `due_date`, `description` | No | Milestone IID or ID is returned. |
| MT-035 | Delete milestone IID `7` named `Evaluation Sprint` from project `my-org/tools/gitlab-mcp-server`. | `gitlab_project` / `milestone_delete` | `project_id`, `milestone_iid` | `confirm` | Yes | Destructive call is confirmed and milestone is deleted. |
| MT-036 | Create release `v0.0.0-eval` for tag `v0.0.0-eval` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_release` / `create` | `project_id`, `tag_name`, `name` | `description`, `ref` | No | Release is created and web URL is returned. |
| MT-037 | Delete release `v0.0.0-eval` from project `my-org/tools/gitlab-mcp-server`. | `gitlab_release` / `delete` | `project_id`, `tag_name` | `confirm` | Yes | Destructive call is confirmed and release is deleted. |
| MT-038 | List deploy keys for project `my-org/tools/gitlab-mcp-server`. | `gitlab_access` / `deploy_key_list_project` | `project_id` | `page`, `per_page` | No | Deploy key list is returned. |
| MT-039 | Analyze why pipeline `12345` failed in project `my-org/tools/gitlab-mcp-server`. | `gitlab_analyze` / `pipeline_failure` | `project_id`, `pipeline_id` | `prompt` | No | Analysis includes likely cause and fix suggestions. |
| MT-040 | Run server diagnostics and GitLab connectivity check. | `gitlab_server` / `health_check` | none | none | No | Status object includes server version and auth status. |
| MT-041 | Create project access token `eval-token` for project `my-org/tools/gitlab-mcp-server` with `read_api` scope expiring `2026-12-31`. | `gitlab_access` / `token_project_create` | `project_id`, `name`, `scopes` | `expires_at` | No | Project access token metadata is returned and cleartext token is handled as one-time output. |
| MT-042 | Revoke project access token ID `77` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_access` / `token_project_revoke` | `project_id`, `token_id` | `confirm` | Yes | Destructive token revoke is confirmed. |
| MT-043 | List generic packages in project `my-org/tools/gitlab-mcp-server`. | `gitlab_package` / `list` | `project_id` | `package_type`, `per_page` | No | Generic package list is returned. |
| MT-044 | Delete package ID `55` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_package` / `delete` | `project_id`, `package_id` | `confirm` | Yes | Destructive package delete is confirmed. |
| MT-045 | List online project runners for project `my-org/tools/gitlab-mcp-server`. | `gitlab_runner` / `list_project` | `project_id` | `status` | No | Project runner list is returned with online filter. |
| MT-046 | Pause runner ID `99`. | `gitlab_runner` / `update` | `runner_id` | `paused` | No | Runner metadata is updated with paused state. |
| MT-047 | Remove runner ID `99`. | `gitlab_runner` / `remove` | `runner_id` | `confirm` | Yes | Destructive runner removal is confirmed. |
| MT-048 | List available environments in project `my-org/tools/gitlab-mcp-server`. | `gitlab_environment` / `list` | `project_id` | `states` | No | Available environments are returned. |
| MT-049 | Stop environment ID `7` in project `my-org/tools/gitlab-mcp-server`, forcing the stop if needed. | `gitlab_environment` / `stop` | `project_id`, `environment_id` | `force`, `confirm` | Yes | Destructive environment stop is confirmed. |
| MT-050 | Get raw content of personal snippet ID `33`. | `gitlab_snippet` / `content` | `snippet_id` | none | No | Raw snippet content is returned. |
| MT-051 | Delete personal snippet ID `33`. | `gitlab_snippet` / `delete` | `snippet_id` | `confirm` | Yes | Destructive snippet delete is confirmed. |
| MT-052 | Show instance application settings. | `gitlab_admin` / `settings_get` | none | none | No | Settings map is returned or an admin-permission error is explained. |
| MT-053 | Create a banner broadcast message saying `Evaluation maintenance` from `2026-01-01T00:00:00Z` to `2026-01-01T01:00:00Z`. | `gitlab_admin` / `broadcast_message_create` | `message` | `starts_at`, `ends_at`, `broadcast_type`, `dismissable` | No | Broadcast message metadata is returned. |
| MT-054 | Delete broadcast message ID `12`. | `gitlab_admin` / `broadcast_message_delete` | `id` | `confirm` | Yes | Destructive broadcast message delete is confirmed. |
| MT-055 | Archive project `my-org/tools/gitlab-mcp-server`. | `gitlab_project` / `archive` | `project_id` | none | No | Project archived state is returned. |
| MT-056 | Add webhook `https://example.com/gitlab-hook` to project `my-org/tools/gitlab-mcp-server` for push events. | `gitlab_project` / `hook_add` | `project_id`, `url` | `push_events`, `enable_ssl_verification` | No | Webhook ID and URL are returned. |
| MT-057 | Delete webhook ID `5` from project `my-org/tools/gitlab-mcp-server`. | `gitlab_project` / `hook_delete` | `project_id`, `hook_id` | `confirm` | Yes | Destructive webhook delete is confirmed. |
| MT-058 | Add a coverage badge to project `my-org/tools/gitlab-mcp-server` linking to `https://example.com/coverage` with image `https://example.com/badge.svg`. | `gitlab_project` / `badge_add` | `project_id`, `link_url`, `image_url` | none | No | Badge metadata is returned. |
| MT-059 | Delete badge ID `8` from project `my-org/tools/gitlab-mcp-server`. | `gitlab_project` / `badge_delete` | `project_id`, `badge_id` | `confirm` | Yes | Destructive badge delete is confirmed. |
| MT-060 | Create a merge request discussion on MR `7` in project `my-org/tools/gitlab-mcp-server` asking `Can we add coverage?`. | `gitlab_mr_review` / `discussion_create` | `project_id`, `merge_request_iid`, `body` | `position` | No | Discussion ID and note body are returned. |
| MT-061 | Resolve merge request discussion `abc123` on MR `7` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_mr_review` / `discussion_resolve` | `project_id`, `merge_request_iid`, `discussion_id`, `resolved` | none | No | Discussion resolved state is true. |
| MT-062 | Create a draft review note on MR `7` in project `my-org/tools/gitlab-mcp-server` saying `Please add a regression test`. | `gitlab_mr_review` / `draft_note_create` | `project_id`, `merge_request_iid`, `note` | `position` | No | Draft note ID is returned without publishing the review. |
| MT-063 | Publish all draft review notes for MR `7` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_mr_review` / `draft_note_publish_all` | `project_id`, `merge_request_iid` | none | No | Draft notes are published as a review batch. |
| MT-064 | Play manual job `999` in project `my-org/tools/gitlab-mcp-server` with variable `DEPLOY_ENV=staging`. | `gitlab_job` / `play` | `project_id`, `job_id` | `variables` | No | Manual job is started with variables. |
| MT-065 | Download artifact `coverage/report.xml` from job `999` in project `my-org/tools/gitlab-mcp-server`. | `gitlab_job` / `download_single_artifact` | `project_id`, `job_id`, `artifact_path` | none | No | Artifact content is returned or size limit is explained. |
| MT-066 | Remove project ID `123` from the CI job token allowlist of project `my-org/tools/gitlab-mcp-server`. | `gitlab_job` / `token_scope_remove_project` | `project_id`, `target_project_id` | `confirm` | Yes | Destructive token-scope removal is confirmed. |
| MT-067 | Create group CI variable `GROUP_EVAL_TOKEN` in group `my-org` with value `masked-value-123`. | `gitlab_ci_variable` / `group_create` | `group_id`, `key`, `value` | `masked`, `environment_scope` | No | Group variable metadata is returned. |
| MT-068 | Create instance CI variable `INSTANCE_EVAL_TOKEN` with value `masked-value-123`. | `gitlab_ci_variable` / `instance_create` | `key`, `value` | `masked`, `protected` | No | Instance variable metadata is returned. |
| MT-069 | Delete instance CI variable `INSTANCE_EVAL_TOKEN`. | `gitlab_ci_variable` / `instance_delete` | `key` | `confirm` | Yes | Destructive instance variable delete is confirmed. |

## Run Log Template

| Run | Task ID | Catalog variant | First tool/action | Schema lookup used | First-call pass | Repair success | Final success | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | MT-001 | baseline | | | | | | |

## Compression Results

The first compression passes shortened `gitlab_admin`, `gitlab_project`, `gitlab_merge_request`, `gitlab_group`, `gitlab_issue`, `gitlab_repository`, `gitlab_pipeline`, and `gitlab_user`. The quality-preserving compromise then added small targeted hints for `gitlab_search` and milestone deletion.

The expanded pass shortened the next heaviest meta-tools (`gitlab_access`, `gitlab_package`, `gitlab_runner`, `gitlab_environment`, and `gitlab_snippet`) after adding coverage for those domains to the benchmark fixture. Wave 2 added coverage for `gitlab_admin`, `gitlab_project`, `gitlab_mr_review`, `gitlab_job`, and `gitlab_ci_variable`, then shortened those descriptions after a targeted model run passed. The current enterprise opaque catalog is `56,896` tokens, down from `71,986` in the original baseline and from `70,249` in the `main` snapshot used for comparison.

| Catalog | Tokens | Bytes | Change vs baseline |
| --- | ---: | ---: | ---: |
| Baseline enterprise opaque | 71,986 | 287,944 | - |
| Final 40-task compromise | 61,155 | 244,620 | -10,831 tokens (-15.0%) |
| Expanded compressed catalog | 58,266 | 233,064 | -13,720 tokens (-19.1%) |
| Wave-2 compressed catalog | 56,896 | 227,584 | -15,090 tokens (-21.0%) |

Against the `main` branch catalog snapshot used for the model comparison, the expanded catalog preserves larger savings:

| Catalog | Main tokens | Current tokens | Savings |
| --- | ---: | ---: | ---: |
| Base opaque meta-tools | 55,110 | 42,849 | 12,261 tokens (22.2%) |
| Enterprise opaque meta-tools | 70,249 | 56,896 | 13,353 tokens (19.0%) |

| Enterprise component | Baseline tokens | Final tokens | Final bytes | Final share |
| --- | ---: | ---: | ---: | ---: |
| Description | 35,323 | 20,232 | 80,930 | 35.6% |
| Input schema | 14,147 | 14,147 | 56,589 | 24.9% |
| Output schema | 15,049 | 15,049 | 60,199 | 26.4% |
| Annotations | 1,015 | 1,015 | 4,060 | 1.8% |
| Icons | 5,803 | 5,803 | 23,212 | 10.2% |
| Other | 664 | 664 | 2,656 | 1.1% |

The token gate remains met while keeping the advertised enterprise catalog `13,353` tokens below `main`.

## Static Schema Check

A static validation pass compared the 69 expected tool/action pairs in this fixture against the generated enterprise meta-tool catalog, including the `gitlab_server` schema-discovery tool. The pass confirms that every expected tool/action pair is discoverable after clarifying project-scoped prompts and adding wave-2 coverage for `gitlab_admin`, `gitlab_project`, `gitlab_mr_review`, `gitlab_job`, and `gitlab_ci_variable` edge cases.

| Check | Result |
| --- | ---: |
| Fixture tasks | 69 |
| Repeated dry-run attempts | 138 |
| Tool/action pairs present in the enterprise catalog | 69 |
| Missing expected routes after correction | 0 |

This static check verifies route/schema coverage, not model task success. The wave-2 rows `MT-052` through `MT-069` passed a targeted model run before their target descriptions were compressed, then passed a repeated model run after compression.

## Wave-1 Anthropic Model Run

The compressed production catalog was evaluated with `claude-sonnet-4-6` through `cmd/eval_meta_tools`. The run used the enterprise meta-tool catalog, Anthropic tool calling, simulated schema lookup responses, and validation-only tool results. The wave-1 expanded fixture covered 51 tasks, including `gitlab_access`, `gitlab_package`, `gitlab_runner`, `gitlab_environment`, and `gitlab_snippet` tasks added to cover the newly compressed domains.

The `MT-035` fixture was clarified to include a milestone IID. Later project-scoped prompts were clarified for `MT-014`, `MT-025` through `MT-028`, and `MT-036` through `MT-037`; the earlier wording omitted the project even though the expected tool actions require `project_id`, which made global or instance-level actions reasonable in a validation-only harness.

| Metric | Current expanded compressed catalog | Main snapshot |
| --- | ---: | ---: |
| Tasks | 51 | 51 |
| Tool-selection accuracy | 96.1% | 96.1% |
| Action-selection accuracy | 96.1% | 96.1% |
| First-call validation pass rate | 96.1% | 92.2% |
| Schema lookup use rate | 3.9% | 0.0% |
| Repair success rate | 100.0% | 75.0% |
| Destructive safety | 100.0% | 100.0% |
| Final task success proxy | 100.0% | 98.0% |

The current expanded compressed catalog completed all 51 tasks successfully. The `main` snapshot failed `MT-040`: it selected `gitlab_admin` / `metadata_get` because `gitlab_server` does not exist on `main`; this is an expected catalog capability difference, not a regression in `main` routing.

The newly covered compressed domains also passed a targeted 11-task run (`MT-041` through `MT-051`) with 100% tool selection, action selection, first-call validation, destructive safety, and final task success.

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

The expanded compressed catalog recovers the qualitative gate while increasing token savings against `main`.

## Wave-2 Anthropic Model Run

Rows `MT-052` through `MT-069` cover the next compression candidates: `gitlab_admin`, `gitlab_project`, `gitlab_mr_review`, `gitlab_job`, and `gitlab_ci_variable`. A targeted pre-compression run passed all 18 tasks. After shortening those descriptions, the same 18 tasks were run twice with `--repeat=2`.

| Metric | Post-compression repeated run |
| --- | ---: |
| Task attempts | 36 |
| Tool-selection accuracy | 100.0% |
| Action-selection accuracy | 100.0% |
| First-call validation pass rate | 100.0% |
| Schema lookup use rate | 5.6% |
| Repair success rate | 100.0% |
| Destructive safety | 100.0% |
| Final task success proxy | 100.0% |

The post-compression run emitted 38 Anthropic requests/tool calls, used 1,599 input tokens, 3,374 output tokens, 58,496 cache-creation input tokens, and 1,416,195 cache-read input tokens. The built-in Sonnet estimate reported `$0.6996` for that repeated targeted run.

## Applied Short Descriptions

The production catalog now uses shortened descriptions for the highest-cost meta-tools. Each shortened description keeps the schema lookup path, the `{ "action": "...", "params": { ... } }` envelope, unknown-parameter rejection, destructive-operation guidance, and key routing hints to neighboring meta-tools.

### `gitlab_group`

Manage GitLab groups, subgroups, group projects, members, badges, hooks, LDAP links, SAML links, access requests, and group-level metadata. Use this for group discovery and administration, not project-only settings. All calls use `{ "action": "...", "params": { ... } }`; call `gitlab_server` `schema_get` for exact params before uncertain actions. Unknown params are rejected. Destructive actions such as delete, member removal, badge deletion, hook deletion, and token/access cleanup require confirmation or elicitation.

### `gitlab_admin`

Administer self-managed GitLab instance resources: topics, settings, appearance, broadcast messages, feature flags, licenses, system hooks, Sidekiq, plan limits, usage data, OAuth apps, metadata/statistics, custom attributes, imports, error tracking, secure files, Terraform states, cluster agents, and dependency proxy cache. Use only for instance-level administration. Most actions require admin rights. Fetch exact params with `gitlab_server` `schema_get`; unknown params are rejected. Destructive and instance-wide actions require confirmation.

### `gitlab_project`

Manage GitLab project CRUD and project-scoped metadata: members, shares, hooks, badges, labels, milestones, boards, integrations, uploads, import/export, Pages, avatars, approvals, mirrors, stats, housekeeping, forks, stars, archive/restore, and transfer. Use this for project settings and metadata; use neighboring tools for files, branches, wiki pages, issues, and MRs. Fetch exact params with `gitlab_server` `schema_get`; unknown params are rejected. Destructive actions require confirmation.

### `gitlab_mr_review`

Review and comment on GitLab MRs: notes, threaded discussions, inline positions, diffs/raw diffs, diff versions, and draft review notes. Use this to post review comments, resolve threads, inspect diffs, queue draft notes, and publish a batch review. Use `draft_note_create` plus `draft_note_publish_all` for batched reviews. Fetch exact params with `gitlab_server` `schema_get`; unknown params are rejected. Delete actions require confirmation.

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

### `gitlab_access`

Manage GitLab access credentials: project/group/personal access tokens, deploy tokens, deploy keys, access requests, and invitations. Use this to audit or provision machine/user access to projects and groups. Fetch exact params with `gitlab_server` `schema_get` before creating, rotating, or revoking credentials. Revokes and deletes are destructive and irreversible; token create/rotate returns cleartext token once.

### `gitlab_package`

Manage GitLab package registry, container registry, and protection rules. Use this for generic package publish/download/list/delete, package files, container image repositories/tags, and package/container protection rules. Fetch exact params before publish/delete/rule changes. Delete and bulk tag deletion are destructive; publish/download can read or write local files.

### `gitlab_runner`

Manage GitLab CI/CD runners across instance, group, and project scopes plus admin runner controllers. Use this to list/get/update/pause runners, inspect runner jobs, attach/detach runners to projects, register/verify/reset runner tokens, and manage experimental runner controllers. Remove/delete/revoke/reset actions are destructive or credential-rotating.

### `gitlab_job`

Manage GitLab CI/CD jobs and CI job-token scope: job lifecycle, manual play, logs, artifacts, bridges, and inbound trust boundaries. Use for job details, traces, artifacts, retry/cancel/play, and job-token allowlists. Use `gitlab_pipeline` for pipeline-level operations. Fetch exact params with `gitlab_server` `schema_get`; unknown params are rejected. Artifact downloads are base64 and limited; destructive job/artifact/token-scope actions require confirmation.

### `gitlab_ci_variable`

Manage GitLab CI/CD variables at project, group, and instance scope, including masked/hidden values and environment-scoped entries. Use for variable CRUD only, not CI lint, pipeline runs, feature flags, environments, or instance settings. Project actions use `project_id`, group actions use `group_id`, and instance actions use neither. Fetch exact params with `gitlab_server` `schema_get`; unknown params are rejected. Delete actions are irreversible.

### `gitlab_environment`

Manage GitLab deployment environments, protected environments, deploy freeze periods, deployments, approvals, and deployment-related MRs. Use this for environment definitions, deploy gates, deploy freezes, deployment audit history, and deployment approvals. Stop/delete/deployment-delete and unprotect/freeze-delete actions are destructive.

### `gitlab_snippet`

Manage GitLab snippets: personal snippets, project snippets, public explore feed, threaded discussions, project snippet notes, and award emoji. Use snippets for standalone code/text outside repository files. Fetch exact params with `gitlab_server` `schema_get` for create/update/delete. Delete actions are destructive.

## Acceptance Gate Status

The token-reduction and qualitative regression gates are satisfied by the wave-2 compressed production catalog, the current-vs-main Anthropic comparison, and the repeated wave-2 targeted model run:

- Enterprise opaque definition-token reduction is 19.0% against `main` and 21.0% against the original baseline.
- Final task success is 100.0% for the current catalog versus 98.0% for the `main` snapshot on the 51-task fixture.
- Static route coverage is 100.0% on the 69-task wave-2 fixture.
- Wave-2 targeted final task success is 100.0% across 36 post-compression attempts.
- Destructive safety remains 100.0%.
- Shortened descriptions keep schema lookup guidance and the nested `params` envelope.
- Changed docs, generated snapshots, and focused tests must pass before commit.
