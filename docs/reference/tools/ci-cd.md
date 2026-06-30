# CI/CD — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: CI/CD (Pipelines, Jobs, Variables, Schedules, Triggers, Resource Groups)
> **Individual tools**: 65
> **Meta-tools**: `gitlab_pipeline`, `gitlab_job`, `gitlab_ci_variable` (`TOOL_SURFACE=meta` catalog). Pipeline schedule actions are accessed via `gitlab_pipeline` with `schedule_*` action prefix; trigger actions via `trigger_*` prefix; resource group actions via `resource_group_*` prefix. The `gitlab_ci_variable` meta-tool also covers group and instance variables (action prefixes `group_variable_*` / `instance_variable_*`). CI lint tools (`gitlab_ci_lint`, `gitlab_ci_lint_project`) live under the `gitlab_template` meta-tool — see [templates.md](templates.md).
> **GitLab API**: [Pipelines API](https://docs.gitlab.com/ee/api/pipelines.html) · [Jobs API](https://docs.gitlab.com/ee/api/jobs.html) · [CI Variables API](https://docs.gitlab.com/ee/api/project_level_variables.html) · [Pipeline Schedules API](https://docs.gitlab.com/ee/api/pipeline_schedules.html) · [Pipeline Triggers API](https://docs.gitlab.com/ee/api/pipeline_triggers.html) · [Instance Variables API](https://docs.gitlab.com/ee/api/instance_level_ci_variables.html) · [Group Variables API](https://docs.gitlab.com/ee/api/group_level_variables.html) · [Resource Groups API](https://docs.gitlab.com/ee/api/resource_groups.html)
> **Audience**: 👤 End users, AI assistant users
>
> 📘 **Want to use CI/CD in a pipeline?** This page lists the *tool schemas*. For an operational walkthrough, see the [CI/CD Usage guide](../../guides/ci-cd.md).

---

## Overview

The CI/CD domain covers GitLab's continuous integration and delivery capabilities: pipelines, jobs, CI/CD variables (project, group, and instance level), pipeline schedules, pipeline triggers, and CI resource groups.

With `TOOL_SURFACE=meta`, the 65 individual tools below are consolidated into three meta-tools that dispatch by `action` parameter. CI lint tools live under the `gitlab_template` meta-tool — see [templates.md](templates.md).

### Common Questions

> "Show pipelines for project 42"
> "What's the status of the latest pipeline?"
> "List the CI/CD variables for my project"
> "Retry the failed pipeline"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description                                    |
| ---------- | :------: | :---------: | :--------: | ---------------------------------------------- |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation                       |
| **Create** |    —     |     No      |     —      | Creates a new resource                         |
| **Update** |    —     |     No      |    Yes     | Modifies an existing resource                  |
| **Delete** |    —     |     Yes     |    Yes     | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Pipelines

### `gitlab_pipeline_list`

List pipelines for a GitLab project. Supports filtering by status (success, failed, running, pending, canceled), scope (running, pending, finished, branches, tags), source (push, web, schedule, merge_request_event), ref (branch/tag), SHA, and username. Returns pipeline ID, status, source, ref, web URL, and timestamps with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pipeline_get`

Retrieve detailed information about a specific pipeline in a GitLab project. Returns pipeline ID, status, source, ref, SHA, duration, coverage, user, timestamps, and YAML errors.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pipeline_create`

Create a new pipeline for a branch or tag. Optionally pass variables (key/value pairs with type env_var or file).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_pipeline_cancel`

Cancel a running pipeline in a GitLab project. Returns the updated pipeline details with canceled status.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_pipeline_retry`

Retry all failed jobs in a pipeline. Returns the updated pipeline details.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_pipeline_update_metadata`

Update the metadata (name) of an existing pipeline.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_pipeline_delete`

Permanently delete a pipeline and all its jobs. This action cannot be undone. Requires at least Maintainer access level.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Permanent deletion cannot be undone.

### `gitlab_pipeline_variables`

Get the variables for a specific pipeline. Returns variable keys, values, and types.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pipeline_test_report`

Get the full test report for a pipeline. Returns total/passed/failed/skipped/error counts and per-suite breakdowns.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pipeline_test_report_summary`

Get a summary of the test report for a pipeline. Returns aggregated counts and per-suite summaries with build IDs.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pipeline_latest`

Get the latest pipeline for a project, optionally filtered by branch/tag ref. Returns full pipeline details.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pipeline_wait`

Wait for a pipeline to reach a terminal state (success, failed, canceled, skipped, manual). Polls the pipeline status at a configurable interval and sends progress notifications. Returns the final pipeline details when done or when the timeout is reached.

| Parameter          | Required | Default | Description                                     |
| ------------------ | -------- | ------- | ----------------------------------------------- |
| `project_id`       | Yes      | —       | Project ID or URL-encoded path                  |
| `pipeline_id`      | Yes      | —       | Pipeline ID to wait for                         |
| `interval_seconds` | No       | 10      | Polling interval in seconds (5–60)              |
| `timeout_seconds`  | No       | 300     | Maximum wait time in seconds (1–3600)           |
| `fail_on_error`    | No       | true    | Return error if pipeline reaches a failed state |

| Annotation | **Read** |
| ---------- | -------- |

---

## Jobs

### `gitlab_job_list`

List jobs for a specific pipeline in a GitLab project. Supports filtering by scope (created, pending, running, failed, success, canceled, skipped, manual). Returns job ID, name, status, stage, runner, duration, and web URL with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_job_get`

Retrieve detailed information about a specific CI/CD job in a GitLab project. Returns job ID, name, status, stage, pipeline, runner, duration, coverage, timestamps, and web URL.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_job_trace`

Retrieve the log (trace) output of a CI/CD job. Returns the raw log text, truncated to 100KB if the log is larger. Useful for debugging failed jobs.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_job_cancel`

Cancel a running or pending CI/CD job in a GitLab project. Supports `force: true` to cancel even when the job is in a non-cancellable state (requires GitLab v17.2+). Returns the updated job details.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_job_retry`

Retry a failed or canceled CI/CD job in a GitLab project. Returns the new job details.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_job_play`

Trigger (play) a manual CI/CD job. Supports passing job variables. Returns updated job details.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_job_keep_artifacts`

Prevent a job's artifacts from being deleted when expiration is set. Returns updated job details.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_job_list_project`

List all jobs across a GitLab project (not limited to a single pipeline). Supports filtering by scope and pagination. Returns job ID, name, status, stage, duration.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_job_list_bridges`

List pipeline bridge (trigger) jobs for a pipeline. Bridge jobs connect upstream and downstream pipelines. Returns bridge ID, name, stage, status, duration, and downstream pipeline ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_job_artifacts`

Download the artifacts archive (zip) for a specific job. Returns base64-encoded content (limited to 1MB). Use for retrieving build outputs.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_job_download_artifacts`

Download the artifacts archive for a specific ref and optional job name. Returns base64-encoded content (limited to 1MB).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_job_download_single_artifact`

Download a single artifact file from a job by its path within the archive. Returns raw file content. Useful for reading specific build outputs like test results or coverage reports.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_job_download_single_artifact_by_ref`

Download a single artifact file by branch/tag name and artifact path. Returns raw file content from the latest successful pipeline for that ref.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_job_erase`

Erase a job's trace log and artifacts. Returns the updated job details with erased_at timestamp.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Erases trace log and artifacts permanently.

### `gitlab_job_delete_artifacts`

Delete the artifacts for a specific job.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Artifact deletion cannot be undone.

### `gitlab_job_delete_project_artifacts`

Delete all artifacts across an entire project. This is a destructive operation.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Deletes all artifacts across the entire project.

### `gitlab_job_wait`

Wait for a CI/CD job to reach a terminal state (success, failed, canceled, skipped, manual). Polls the job status at a configurable interval and sends progress notifications. Returns the final job details when done or when the timeout is reached.

| Parameter          | Required | Default | Description                                |
| ------------------ | -------- | ------- | ------------------------------------------ |
| `project_id`       | Yes      | —       | Project ID or URL-encoded path             |
| `job_id`           | Yes      | —       | Job ID to wait for                         |
| `interval_seconds` | No       | 10      | Polling interval in seconds (5–60)         |
| `timeout_seconds`  | No       | 300     | Maximum wait time in seconds (1–3600)      |
| `fail_on_error`    | No       | true    | Return error if job reaches a failed state |

| Annotation | **Read** |
| ---------- | -------- |

---

## Resource Groups

Resource groups serialize concurrent jobs in a pipeline by sharing a single concurrency lock. Use them to control how queued jobs are dispatched when multiple pipelines target the same environment.

### `gitlab_list_resource_groups`

List the CI resource groups configured for a project. Returns each resource group's ID, key, and process mode that controls how jobs sharing the group are serialized to limit pipeline concurrency.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_resource_group`

Get one CI resource group in a project by key. Returns the resource group ID, key, and process mode (the concurrency mode that controls how jobs sharing the resource group are serialized).

| Parameter    | Required | Description                    |
| ------------ | -------- | ------------------------------ |
| `key`        | Yes      | Resource group key             |
| `project_id` | Yes      | Project ID or URL-encoded path |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_edit_resource_group`

Update the process mode of one CI resource group by key. Returns the updated resource group ID, key, and new process mode that controls how queued jobs sharing the resource group are serialized. Valid `process_mode` values: `unordered`, `oldest_first`, `newest_first`, `newest_ready_first`.

| Parameter      | Required | Description                                                                      |
| -------------- | -------- | -------------------------------------------------------------------------------- |
| `key`          | Yes      | Resource group key                                                               |
| `process_mode` | Yes      | Process mode (`unordered`, `oldest_first`, `newest_first`, `newest_ready_first`) |
| `project_id`   | Yes      | Project ID or URL-encoded path                                                   |

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_list_resource_group_upcoming_jobs`

List the upcoming CI jobs queued for one resource group by key. Returns each pending job's ID, name, status, and stage, ordered as they will run under the resource group's process mode.

| Parameter    | Required | Description                    |
| ------------ | -------- | ------------------------------ |
| `key`        | Yes      | Resource group key             |
| `project_id` | Yes      | Project ID or URL-encoded path |

| Annotation | **Read** |
| ---------- | -------- |

---

## CI/CD Variables (Project)

> **Auto-masking**: Variables flagged as `masked` or `hidden` in GitLab have their values automatically redacted to `[masked]` in all responses. This prevents accidental exposure of secrets through the MCP interface.

### `gitlab_ci_variable_list`

List CI/CD variables for a GitLab project. Returns paginated results with variable key, type, protection, masking, and environment scope.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_ci_variable_get`

Get a specific CI/CD variable by key from a GitLab project. Optionally filter by environment scope when duplicate keys exist.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_ci_variable_create`

Create a new CI/CD variable in a GitLab project. Requires key and value. Optionally set type (env_var/file), protection, masking, and environment scope.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_ci_variable_update`

Update an existing CI/CD variable in a GitLab project. Specify the key to update and any fields to change: value, type, protection, masking, environment scope.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_ci_variable_delete`

Delete a CI/CD variable from a GitLab project by key. Optionally filter by environment scope. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## CI/CD Variables (Instance)

> **Auto-masking**: Variables flagged as `masked` or `hidden` have their values automatically redacted to `[masked]` in all responses.

### `gitlab_instance_variable_list`

List CI/CD variables at the GitLab instance level. Returns paginated results with variable key, type, protection, and masking.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_instance_variable_get`

Get a specific CI/CD variable by key from the GitLab instance level.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_instance_variable_create`

Create a new CI/CD variable at the GitLab instance level. Requires key and value. Optionally set type (env_var/file), protection, and masking.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_instance_variable_update`

Update an existing CI/CD variable at the GitLab instance level. Specify the key to update and any fields to change.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_instance_variable_delete`

Delete a CI/CD variable from the GitLab instance level by key. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## CI/CD Variables (Group)

> **Auto-masking**: Variables flagged as `masked` or `hidden` have their values automatically redacted to `[masked]` in all responses. This prevents accidental exposure of secrets through the MCP interface.

### `gitlab_group_variable_list`

List a group's CI/CD variables with `order_by`, `sort`, and offset or keyset pagination. Returns each variable's key, value, type, protected/masked/hidden/raw flags, environment scope, description, and pagination metadata.

| Parameter    | Required | Default | Description                               |
| ------------ | -------- | ------- | ----------------------------------------- |
| `group_id`   | Yes      | —       | Group ID or URL-encoded path              |
| `order_by`   | No       | —       | Field to sort by                          |
| `sort`       | No       | —       | Sort direction (`asc` / `desc`)           |
| `page`       | No       | 1       | Page number (1+)                          |
| `per_page`   | No       | 20      | Items per page (1–100)                    |
| `pagination` | No       | —       | Pagination strategy (`offset` / `keyset`) |
| `page_token` | No       | —       | Keyset pagination token                   |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_variable_get`

Get a single group CI/CD variable by key, optionally selecting an environment-scoped instance via the filter. Returns the variable's key, value, type, protected/masked/hidden/raw flags, environment scope, and description.

| Parameter           | Required | Description                                                |
| ------------------- | -------- | ---------------------------------------------------------- |
| `group_id`          | Yes      | Group ID or URL-encoded path                               |
| `key`               | Yes      | Variable key                                               |
| `environment_scope` | No       | Environment scope string; use `*` for global               |
| `filter`            | No       | Filter object to disambiguate environment-scoped instances |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_variable_create`

Create a CI/CD variable in a group with type, environment scope, and protected/masked/raw/masked_and_hidden flags. Returns the created variable's key, value, type, flags, environment scope, and description.

| Parameter           | Required | Description                                |
| ------------------- | -------- | ------------------------------------------ |
| `group_id`          | Yes      | Group ID or URL-encoded path               |
| `key`               | Yes      | Variable key                               |
| `value`             | Yes      | Variable value                             |
| `variable_type`     | No       | Variable type (`env_var` / `file`)         |
| `protected`         | No       | Restrict to protected branches/tags        |
| `masked`            | No       | Mask value in job logs                     |
| `masked_and_hidden` | No       | Mask value and hide from API responses     |
| `raw`               | No       | Treat value as raw (no variable expansion) |
| `environment_scope` | No       | Environment scope string                   |
| `description`       | No       | Free-form description                      |

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_variable_update`

Update a group CI/CD variable, selecting an environment-scoped instance via the filter. Returns the updated variable's key, value, type, protected/masked/hidden/raw flags, environment scope, and description.

| Parameter           | Required | Description                                                |
| ------------------- | -------- | ---------------------------------------------------------- |
| `group_id`          | Yes      | Group ID or URL-encoded path                               |
| `key`               | Yes      | Variable key                                               |
| `value`             | No       | New variable value                                         |
| `variable_type`     | No       | New variable type (`env_var` / `file`)                     |
| `protected`         | No       | Restrict to protected branches/tags                        |
| `masked`            | No       | Mask value in job logs                                     |
| `raw`               | No       | Treat value as raw (no variable expansion)                 |
| `environment_scope` | No       | Environment scope string                                   |
| `description`       | No       | Free-form description                                      |
| `filter`            | No       | Filter object to disambiguate environment-scoped instances |

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_variable_delete`

Delete a group CI/CD variable by key, selecting an environment-scoped instance via the filter. This action cannot be undone.

| Parameter           | Required | Description                                                |
| ------------------- | -------- | ---------------------------------------------------------- |
| `group_id`          | Yes      | Group ID or URL-encoded path                               |
| `key`               | Yes      | Variable key                                               |
| `environment_scope` | No       | Environment scope string                                   |
| `filter`            | No       | Filter object to disambiguate environment-scoped instances |

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Variable deletion cannot be undone.

---

## Pipeline Schedules

### `gitlab_pipeline_schedule_list`

List pipeline schedules for a GitLab project. Supports filtering by scope (active, inactive). Returns paginated results with schedule details.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pipeline_schedule_get`

Get details of a specific pipeline schedule in a GitLab project by its ID. Returns description, ref, cron expression, timezone, active state, owner, and timestamps.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pipeline_schedule_create`

Create a new pipeline schedule in a GitLab project. Requires description, ref (branch/tag), and cron expression. Optionally set timezone and active state.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_pipeline_schedule_update`

Update an existing pipeline schedule in a GitLab project. All fields are optional: description, ref, cron, timezone, active state.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_pipeline_schedule_delete`

Permanently delete a pipeline schedule from a GitLab project. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_pipeline_schedule_run`

Trigger an immediate run of a pipeline schedule. Executes the schedule now regardless of its cron timing. Returns the updated schedule details.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_pipeline_schedule_take_ownership`

Take ownership of a pipeline schedule, making the current user the owner. Returns the updated schedule details.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_pipeline_schedule_create_variable`

Create a new variable for a pipeline schedule. Variables are passed to pipelines triggered by the schedule. Supports env_var (default) and file types.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_pipeline_schedule_edit_variable`

Edit an existing pipeline schedule variable by key. Updates the value and optionally the variable type.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_pipeline_schedule_delete_variable`

Delete a pipeline schedule variable by key. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_pipeline_schedule_list_triggered_pipelines`

List all pipelines that were triggered by a specific pipeline schedule. Returns paginated results with pipeline ID, ref, status, and source.

| Annotation | **Read** |
| ---------- | -------- |

---

## Pipeline Triggers

### `gitlab_pipeline_trigger_list`

List pipeline trigger tokens for a project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pipeline_trigger_get`

Get a single pipeline trigger token.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pipeline_trigger_create`

Create a new pipeline trigger token.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_pipeline_trigger_update`

Update a pipeline trigger token description.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_pipeline_trigger_delete`

Delete a pipeline trigger token.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Token deletion cannot be undone.

### `gitlab_pipeline_trigger_run`

Trigger a pipeline using a trigger token.

| Annotation | **Create** |
| ---------- | ---------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_pipeline_list` | Pipelines | Read |
| 2 | `gitlab_pipeline_get` | Pipelines | Read |
| 3 | `gitlab_pipeline_create` | Pipelines | Create |
| 4 | `gitlab_pipeline_cancel` | Pipelines | Update |
| 5 | `gitlab_pipeline_retry` | Pipelines | Update |
| 6 | `gitlab_pipeline_update_metadata` | Pipelines | Update |
| 7 | `gitlab_pipeline_delete` | Pipelines | Delete |
| 8 | `gitlab_pipeline_variables` | Pipelines | Read |
| 9 | `gitlab_pipeline_test_report` | Pipelines | Read |
| 10 | `gitlab_pipeline_test_report_summary` | Pipelines | Read |
| 11 | `gitlab_pipeline_latest` | Pipelines | Read |
| 12 | `gitlab_pipeline_wait` | Pipelines | Read |
| 13 | `gitlab_job_list` | Jobs | Read |
| 14 | `gitlab_job_get` | Jobs | Read |
| 15 | `gitlab_job_trace` | Jobs | Read |
| 16 | `gitlab_job_cancel` | Jobs | Update |
| 17 | `gitlab_job_retry` | Jobs | Update |
| 18 | `gitlab_job_play` | Jobs | Update |
| 19 | `gitlab_job_keep_artifacts` | Jobs | Update |
| 20 | `gitlab_job_list_project` | Jobs | Read |
| 21 | `gitlab_job_list_bridges` | Jobs | Read |
| 22 | `gitlab_job_artifacts` | Jobs | Read |
| 23 | `gitlab_job_download_artifacts` | Jobs | Read |
| 24 | `gitlab_job_download_single_artifact` | Jobs | Read |
| 25 | `gitlab_job_download_single_artifact_by_ref` | Jobs | Read |
| 26 | `gitlab_job_erase` | Jobs | Delete |
| 27 | `gitlab_job_delete_artifacts` | Jobs | Delete |
| 28 | `gitlab_job_delete_project_artifacts` | Jobs | Delete |
| 29 | `gitlab_job_wait` | Jobs | Read |
| 30 | `gitlab_list_resource_groups` | Resource Groups | Read |
| 31 | `gitlab_get_resource_group` | Resource Groups | Read |
| 32 | `gitlab_edit_resource_group` | Resource Groups | Update |
| 33 | `gitlab_list_resource_group_upcoming_jobs` | Resource Groups | Read |
| 34 | `gitlab_ci_variable_list` | CI Variables (Project) | Read |
| 35 | `gitlab_ci_variable_get` | CI Variables (Project) | Read |
| 36 | `gitlab_ci_variable_create` | CI Variables (Project) | Create |
| 37 | `gitlab_ci_variable_update` | CI Variables (Project) | Update |
| 38 | `gitlab_ci_variable_delete` | CI Variables (Project) | Delete |
| 39 | `gitlab_group_variable_list` | CI Variables (Group) | Read |
| 40 | `gitlab_group_variable_get` | CI Variables (Group) | Read |
| 41 | `gitlab_group_variable_create` | CI Variables (Group) | Create |
| 42 | `gitlab_group_variable_update` | CI Variables (Group) | Update |
| 43 | `gitlab_group_variable_delete` | CI Variables (Group) | Delete |
| 44 | `gitlab_instance_variable_list` | CI Variables (Instance) | Read |
| 45 | `gitlab_instance_variable_get` | CI Variables (Instance) | Read |
| 46 | `gitlab_instance_variable_create` | CI Variables (Instance) | Create |
| 47 | `gitlab_instance_variable_update` | CI Variables (Instance) | Update |
| 48 | `gitlab_instance_variable_delete` | CI Variables (Instance) | Delete |
| 49 | `gitlab_pipeline_schedule_list` | Pipeline Schedules | Read |
| 50 | `gitlab_pipeline_schedule_get` | Pipeline Schedules | Read |
| 51 | `gitlab_pipeline_schedule_create` | Pipeline Schedules | Create |
| 52 | `gitlab_pipeline_schedule_update` | Pipeline Schedules | Update |
| 53 | `gitlab_pipeline_schedule_delete` | Pipeline Schedules | Delete |
| 54 | `gitlab_pipeline_schedule_run` | Pipeline Schedules | Update |
| 55 | `gitlab_pipeline_schedule_take_ownership` | Pipeline Schedules | Update |
| 56 | `gitlab_pipeline_schedule_create_variable` | Pipeline Schedules | Create |
| 57 | `gitlab_pipeline_schedule_edit_variable` | Pipeline Schedules | Update |
| 58 | `gitlab_pipeline_schedule_delete_variable` | Pipeline Schedules | Delete |
| 59 | `gitlab_pipeline_schedule_list_triggered_pipelines` | Pipeline Schedules | Read |
| 60 | `gitlab_pipeline_trigger_list` | Pipeline Triggers | Read |
| 61 | `gitlab_pipeline_trigger_get` | Pipeline Triggers | Read |
| 62 | `gitlab_pipeline_trigger_create` | Pipeline Triggers | Create |
| 63 | `gitlab_pipeline_trigger_update` | Pipeline Triggers | Update |
| 64 | `gitlab_pipeline_trigger_delete` | Pipeline Triggers | Delete |
| 65 | `gitlab_pipeline_trigger_run` | Pipeline Triggers | Create |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_pipeline_delete` — permanently deletes a pipeline and all its jobs
- `gitlab_job_erase` — erases job trace log and artifacts
- `gitlab_job_delete_artifacts` — deletes artifacts for a specific job
- `gitlab_job_delete_project_artifacts` — deletes all artifacts across a project
- `gitlab_ci_variable_delete` — deletes a project CI/CD variable
- `gitlab_group_variable_delete` — deletes a group CI/CD variable
- `gitlab_instance_variable_delete` — deletes an instance CI/CD variable
- `gitlab_pipeline_schedule_delete` — deletes a pipeline schedule
- `gitlab_pipeline_schedule_delete_variable` — deletes a schedule variable
- `gitlab_pipeline_trigger_delete` — deletes a pipeline trigger token

---

## Related

- [GitLab Pipelines API](https://docs.gitlab.com/ee/api/pipelines.html)
- [GitLab Jobs API](https://docs.gitlab.com/ee/api/jobs.html)
- [GitLab Project CI/CD Variables API](https://docs.gitlab.com/ee/api/project_level_variables.html)
- [GitLab Group CI/CD Variables API](https://docs.gitlab.com/ee/api/group_level_variables.html)
- [GitLab Instance CI/CD Variables API](https://docs.gitlab.com/ee/api/instance_level_ci_variables.html)
- [GitLab Pipeline Schedules API](https://docs.gitlab.com/ee/api/pipeline_schedules.html)
- [GitLab Pipeline Triggers API](https://docs.gitlab.com/ee/api/pipeline_triggers.html)
- [GitLab Resource Groups API](https://docs.gitlab.com/ee/api/resource_groups.html)
- [CI Lint tools](templates.md) — `gitlab_ci_lint`, `gitlab_ci_lint_project` are documented in the Templates reference
