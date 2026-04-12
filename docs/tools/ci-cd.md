# CI/CD — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: CI/CD (Pipelines, Jobs, Variables, Schedules, Triggers, Lint)
> **Individual tools**: 57
> **Meta-tools**: `gitlab_pipeline`, `gitlab_job`, `gitlab_ci_variable`, `gitlab_pipeline_schedule`, `gitlab_pipeline_trigger`, `gitlab_instance_variable` (when `META_TOOLS=true`, default)
> **GitLab API**: [Pipelines API](https://docs.gitlab.com/ee/api/pipelines.html) · [Jobs API](https://docs.gitlab.com/ee/api/jobs.html) · [CI Variables API](https://docs.gitlab.com/ee/api/project_level_variables.html) · [Pipeline Schedules API](https://docs.gitlab.com/ee/api/pipeline_schedules.html) · [Pipeline Triggers API](https://docs.gitlab.com/ee/api/pipeline_triggers.html) · [Instance Variables API](https://docs.gitlab.com/ee/api/instance_level_ci_variables.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The CI/CD domain covers GitLab's continuous integration and delivery capabilities: pipelines, jobs, CI/CD variables (project and instance level), pipeline schedules, pipeline triggers, and CI configuration linting.

When `META_TOOLS=true` (the default), the 57 individual tools below are consolidated into six meta-tools that dispatch by `action` parameter. CI lint tools are additionally available through the `gitlab_template` meta-tool.

### Common Questions

> "Show pipelines for project 42"
> "What's the status of the latest pipeline?"
> "List the CI/CD variables for my project"
> "Retry the failed pipeline"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## CI Lint

### `gitlab_ci_lint_project`

Validate a project's CI/CD configuration (.gitlab-ci.yml) from the repository. Returns validation status, errors, warnings, merged YAML, and includes.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_ci_lint`

Validate arbitrary CI/CD YAML content within a project's namespace context. Useful for testing CI configuration changes before committing. Returns validation status, errors, warnings, and merged YAML.

| Annotation | **Read** |
| ---------- | -------- |

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

Cancel a running or pending CI/CD job in a GitLab project. Returns the updated job details.

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
| 1 | `gitlab_ci_lint_project` | CI Lint | Read |
| 2 | `gitlab_ci_lint` | CI Lint | Read |
| 3 | `gitlab_pipeline_list` | Pipelines | Read |
| 4 | `gitlab_pipeline_get` | Pipelines | Read |
| 5 | `gitlab_pipeline_create` | Pipelines | Create |
| 6 | `gitlab_pipeline_cancel` | Pipelines | Update |
| 7 | `gitlab_pipeline_retry` | Pipelines | Update |
| 8 | `gitlab_pipeline_update_metadata` | Pipelines | Update |
| 9 | `gitlab_pipeline_delete` | Pipelines | Delete |
| 10 | `gitlab_pipeline_variables` | Pipelines | Read |
| 11 | `gitlab_pipeline_test_report` | Pipelines | Read |
| 12 | `gitlab_pipeline_test_report_summary` | Pipelines | Read |
| 13 | `gitlab_pipeline_latest` | Pipelines | Read |
| 14 | `gitlab_job_list` | Jobs | Read |
| 15 | `gitlab_job_get` | Jobs | Read |
| 16 | `gitlab_job_trace` | Jobs | Read |
| 17 | `gitlab_job_cancel` | Jobs | Update |
| 18 | `gitlab_job_retry` | Jobs | Update |
| 19 | `gitlab_job_play` | Jobs | Update |
| 20 | `gitlab_job_keep_artifacts` | Jobs | Update |
| 21 | `gitlab_job_list_project` | Jobs | Read |
| 22 | `gitlab_job_list_bridges` | Jobs | Read |
| 23 | `gitlab_job_artifacts` | Jobs | Read |
| 24 | `gitlab_job_download_artifacts` | Jobs | Read |
| 25 | `gitlab_job_download_single_artifact` | Jobs | Read |
| 26 | `gitlab_job_download_single_artifact_by_ref` | Jobs | Read |
| 27 | `gitlab_job_erase` | Jobs | Delete |
| 28 | `gitlab_job_delete_artifacts` | Jobs | Delete |
| 29 | `gitlab_job_delete_project_artifacts` | Jobs | Delete |
| 30 | `gitlab_ci_variable_list` | CI Variables (Project) | Read |
| 31 | `gitlab_ci_variable_get` | CI Variables (Project) | Read |
| 32 | `gitlab_ci_variable_create` | CI Variables (Project) | Create |
| 33 | `gitlab_ci_variable_update` | CI Variables (Project) | Update |
| 34 | `gitlab_ci_variable_delete` | CI Variables (Project) | Delete |
| 35 | `gitlab_instance_variable_list` | CI Variables (Instance) | Read |
| 36 | `gitlab_instance_variable_get` | CI Variables (Instance) | Read |
| 37 | `gitlab_instance_variable_create` | CI Variables (Instance) | Create |
| 38 | `gitlab_instance_variable_update` | CI Variables (Instance) | Update |
| 39 | `gitlab_instance_variable_delete` | CI Variables (Instance) | Delete |
| 40 | `gitlab_pipeline_schedule_list` | Pipeline Schedules | Read |
| 41 | `gitlab_pipeline_schedule_get` | Pipeline Schedules | Read |
| 42 | `gitlab_pipeline_schedule_create` | Pipeline Schedules | Create |
| 43 | `gitlab_pipeline_schedule_update` | Pipeline Schedules | Update |
| 44 | `gitlab_pipeline_schedule_delete` | Pipeline Schedules | Delete |
| 45 | `gitlab_pipeline_schedule_run` | Pipeline Schedules | Update |
| 46 | `gitlab_pipeline_schedule_take_ownership` | Pipeline Schedules | Update |
| 47 | `gitlab_pipeline_schedule_create_variable` | Pipeline Schedules | Create |
| 48 | `gitlab_pipeline_schedule_edit_variable` | Pipeline Schedules | Update |
| 49 | `gitlab_pipeline_schedule_delete_variable` | Pipeline Schedules | Delete |
| 50 | `gitlab_pipeline_schedule_list_triggered_pipelines` | Pipeline Schedules | Read |
| 51 | `gitlab_pipeline_trigger_list` | Pipeline Triggers | Read |
| 52 | `gitlab_pipeline_trigger_get` | Pipeline Triggers | Read |
| 53 | `gitlab_pipeline_trigger_create` | Pipeline Triggers | Create |
| 54 | `gitlab_pipeline_trigger_update` | Pipeline Triggers | Update |
| 55 | `gitlab_pipeline_trigger_delete` | Pipeline Triggers | Delete |
| 56 | `gitlab_pipeline_trigger_run` | Pipeline Triggers | Create |
| 57 | `gitlab_job_delete_project_artifacts` | Jobs | Delete |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_pipeline_delete` — permanently deletes a pipeline and all its jobs
- `gitlab_job_erase` — erases job trace log and artifacts
- `gitlab_job_delete_artifacts` — deletes artifacts for a specific job
- `gitlab_job_delete_project_artifacts` — deletes all artifacts across a project
- `gitlab_ci_variable_delete` — deletes a project CI/CD variable
- `gitlab_instance_variable_delete` — deletes an instance CI/CD variable
- `gitlab_pipeline_schedule_delete` — deletes a pipeline schedule
- `gitlab_pipeline_schedule_delete_variable` — deletes a schedule variable
- `gitlab_pipeline_trigger_delete` — deletes a pipeline trigger token

---

## Related

- [GitLab Pipelines API](https://docs.gitlab.com/ee/api/pipelines.html)
- [GitLab Jobs API](https://docs.gitlab.com/ee/api/jobs.html)
- [GitLab Project CI/CD Variables API](https://docs.gitlab.com/ee/api/project_level_variables.html)
- [GitLab Instance CI/CD Variables API](https://docs.gitlab.com/ee/api/instance_level_ci_variables.html)
- [GitLab Pipeline Schedules API](https://docs.gitlab.com/ee/api/pipeline_schedules.html)
- [GitLab Pipeline Triggers API](https://docs.gitlab.com/ee/api/pipeline_triggers.html)
- [GitLab CI Lint API](https://docs.gitlab.com/ee/api/lint.html)
