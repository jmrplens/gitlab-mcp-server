# Merge Requests — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Merge Requests
> **Individual tools**: 56
> **Meta-tool**: `gitlab_merge_request` (when `META_TOOLS=true`, default)
> **GitLab API**: [Merge Requests API](https://docs.gitlab.com/ee/api/merge_requests.html), [Merge Request Approvals API](https://docs.gitlab.com/ee/api/merge_request_approvals.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The merge requests domain covers the full lifecycle of GitLab merge requests: creation, retrieval, listing (project/group/global), updating, merging, rebasing, approval workflows, deletion, subscriptions, time tracking, dependencies, changes/diffs, and context commits.

When `META_TOOLS=true` (the default), all 44 individual tools below are consolidated into a single `gitlab_merge_request` meta-tool that dispatches by `action` parameter.

### Common Questions

> "Show me open merge requests in project 42"
> "Create a merge request from feature-login to main"
> "Merge MR !15 after the pipeline passes"
> "What merge requests are assigned to me?"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Core CRUD

### `gitlab_mr_create`

Create a new merge request in a GitLab project. Requires source and target branch names. Supports title, Markdown description, assignee IDs, reviewer IDs, labels, milestone ID, allow_collaboration, and target_project_id (for cross-project/fork MRs). The squash and remove_source_branch options are omitted by default to preserve repository-level settings; only set them when the user explicitly requests it.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_mr_get`

Retrieve detailed information about a GitLab merge request by its IID (project-scoped ID), including title, description, state, source/target branches, author, assignees, reviewers, labels, and pipeline status.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_list`

List merge requests in a GitLab project. Supports filtering by state (opened/closed/merged/all), author, assignee, reviewer, labels, milestone, and source/target branch. Returns paginated results.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_update`

Update a GitLab merge request's title, description, target branch, assignees, reviewers, labels (replace, add, or remove), milestone, discussion_locked, allow_collaboration, or state event (close/reopen). The squash and remove_source_branch options are omitted by default to preserve repository-level settings; only set them when explicitly requested. Only specified fields are changed.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_mr_delete`

Permanently delete a GitLab merge request. This action cannot be undone. Requires at least Maintainer access level.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Permanent deletion cannot be undone.

---

## Merge & Rebase

### `gitlab_mr_merge`

Merge an accepted GitLab merge request into its target branch. Supports optional squash commits, custom merge commit message, and automatic source branch deletion after merge. The squash and should_remove_source_branch options are omitted by default to preserve repository-level settings; only set them when the user explicitly requests it.

| Annotation | **Update** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Merging is irreversible.

### `gitlab_mr_rebase`

Rebase a merge request's source branch against its target branch. Optionally skip triggering CI pipeline after rebase. Returns whether the rebase is in progress.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_mr_cancel_auto_merge`

Cancel the 'merge when pipeline succeeds' (auto-merge) setting on a GitLab merge request. Returns the updated merge request details. Requires appropriate permissions; returns 405 if already merged/closed or 406 if auto-merge was not enabled.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Approval

### `gitlab_mr_approve`

Approve a GitLab merge request. Adds the authenticated user's approval to the merge request's approval list.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_mr_unapprove`

Remove the authenticated user's approval from a GitLab merge request.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Global & Group Listings

### `gitlab_mr_list_global`

List merge requests across all projects visible to the authenticated user. Supports filtering by state (opened/closed/merged/all), author, reviewer, labels, milestone, draft status, and date ranges. Returns paginated results.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_list_group`

List merge requests in a GitLab group. Supports filtering by state (opened/closed/merged/all), author, reviewer, labels, milestone, draft status, and date ranges. Returns paginated results.

| Annotation | **Read** |
| ---------- | -------- |

---

## Commits & Pipelines

### `gitlab_mr_commits`

List all commits in a GitLab merge request. Returns commit ID, title, author, date, and web URL with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_pipelines`

List all pipelines associated with a GitLab merge request. Returns pipeline ID, status, source, ref, SHA, and web URL.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_create_pipeline`

Create a new pipeline for a GitLab merge request. Triggers a CI/CD pipeline run on the MR's source branch. Returns the created pipeline details.

| Annotation | **Create** |
| ---------- | ---------- |

---

## Participants & Reviewers

### `gitlab_mr_participants`

List all participants (users who have interacted) in a GitLab merge request. Returns user ID, username, name, state, and profile URL.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_reviewers`

List all reviewers assigned to a GitLab merge request. Returns reviewer user details plus review state and assignment date.

| Annotation | **Read** |
| ---------- | -------- |

---

## Issues

### `gitlab_mr_issues_closed`

List all issues that would be closed when a GitLab merge request is merged. Returns issue details including IID, title, state, author, and labels with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_related_issues`

List all issues related to a GitLab merge request (mentioned or linked). Returns issue details including IID, title, state, author, and labels with pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Subscriptions

### `gitlab_mr_subscribe`

Subscribe to a GitLab merge request to receive notifications. Returns the updated MR. Returns 304 if already subscribed.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_mr_unsubscribe`

Unsubscribe from a GitLab merge request to stop receiving notifications. Returns the updated MR. Returns 304 if not subscribed.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Time Tracking

### `gitlab_mr_set_time_estimate`

Set the time estimate for a GitLab merge request using a human-readable duration string (e.g. '3h30m', '1w2d'). Returns updated time stats.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_mr_reset_time_estimate`

Reset the time estimate for a GitLab merge request to zero. Returns updated time stats.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_mr_add_spent_time`

Add spent time to a GitLab merge request. Duration uses human-readable format (e.g. '1h', '30m', '1w2d'). Optional summary describes the work done. Returns updated time stats.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_mr_reset_spent_time`

Reset the total spent time for a GitLab merge request to zero. Returns updated time stats.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_mr_time_stats`

Get time tracking statistics for a GitLab merge request including estimated time and total time spent in both human-readable and seconds format.

| Annotation | **Read** |
| ---------- | -------- |

---

## Todo

### `gitlab_mr_create_todo`

Create a to-do item on a GitLab merge request for the authenticated user. Adds the MR to the user's to-do list for later follow-up.

| Annotation | **Create** |
| ---------- | ---------- |

---

## Dependencies

### `gitlab_mr_dependency_create`

Create a merge request dependency (blocker). The specified blocking MR must be merged before this MR can be merged. Requires Premium or Ultimate license.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_mr_dependency_delete`

Remove a merge request dependency (blocker). The specified blocking MR will no longer prevent this MR from being merged. Requires Premium or Ultimate license.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_mr_dependencies_list`

List all merge request dependencies (blockers) for a GitLab merge request. Returns the list of MRs that must be merged before this MR can be merged. Requires Premium or Ultimate license.

| Annotation | **Read** |
| ---------- | -------- |

---

## Approval Rules

### `gitlab_mr_approval_state`

Get the approval state of a GitLab merge request, including whether approval rules have been overridden and the list of applicable rules with their current status.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_approval_rules`

List the approval rules configured for a GitLab merge request. Returns rule names, types, required approvals, current approvers, and eligible approvers.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_approval_config`

Get the approval configuration for a GitLab merge request including required approvals, current approvers, suggested approvers, and user approval status.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_approval_reset`

Reset all approvals on a GitLab merge request. Requires project_id and merge_request_iid.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_mr_approval_rule_create`

Create an approval rule on a GitLab merge request. Specify the rule name, required approvals, and optionally user/group IDs.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_mr_approval_rule_update`

Update an existing approval rule on a GitLab merge request. Modify the rule name, required approvals, or user/group IDs.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_mr_approval_rule_delete`

Delete an approval rule from a GitLab merge request. Requires project_id, merge_request_iid, and approval_rule_id.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Changes & Diffs

### `gitlab_mr_changes_get`

Get the list of file diffs (changes) for a merge request. Returns old/new paths, diff content, file status (added/deleted/renamed), and file modes.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_diff_versions_list`

List all diff versions of a merge request. Each version represents the state of diffs at a particular point in the MR lifecycle. Returns version IDs, SHAs, state, and creation timestamps.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_diff_version_get`

Get a single merge request diff version with its commits and file diffs. Use the version_id from gitlab_mr_diff_versions_list. Optionally request unified diff format.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_raw_diffs`

Get the raw unified-diff output for a merge request. Returns plain-text diff content suitable for git-apply. Useful for programmatic diff analysis or patch application.

| Annotation | **Read** |
| ---------- | -------- |

---

## Context Commits

### `gitlab_list_mr_context_commits`

List context commits associated with a merge request.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_mr_context_commits`

Add context commits to a merge request.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_delete_mr_context_commits`

Remove context commits from a merge request.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Merge Trains

### `gitlab_list_project_merge_trains`

List all merge trains for a project. Returns merge train entries with ID, merge request, user, pipeline, target branch, status, and duration.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_list_merge_request_in_merge_train`

List merge requests in a merge train for a specific target branch. Supports filtering by scope (`active`, `complete`) and sorting.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_merge_request_on_merge_train`

Get the merge train status for a specific merge request, including ID, status, target branch, and duration.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_add_merge_request_to_merge_train`

Add a merge request to a merge train. Supports auto-merge, SHA verification, and squash options.

| Annotation | **Create** |
| ---------- | ---------- |

---

## External Status Checks

### `gitlab_list_project_status_checks`

List project-level external status checks. Returns paginated list with ID, name, external URL, HMAC, and protected branches.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_list_project_mr_external_status_checks`

List external status checks for a project merge request.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_list_project_external_status_checks`

List external status checks configured for a project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_project_external_status_check`

Create an external status check for a project. Requires name and external URL.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_update_project_external_status_check`

Update an external status check for a project.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_project_external_status_check`

Delete an external status check from a project.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_retry_failed_external_status_check_for_project_mr`

Retry a failed external status check for a project merge request.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_set_project_mr_external_status_check_status`

Set the status of an external status check for a project merge request.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_mr_create` | Core CRUD | Create |
| 2 | `gitlab_mr_get` | Core CRUD | Read |
| 3 | `gitlab_mr_list` | Core CRUD | Read |
| 4 | `gitlab_mr_update` | Core CRUD | Update |
| 5 | `gitlab_mr_delete` | Core CRUD | Delete |
| 6 | `gitlab_mr_merge` | Merge & Rebase | Update |
| 7 | `gitlab_mr_rebase` | Merge & Rebase | Update |
| 8 | `gitlab_mr_cancel_auto_merge` | Merge & Rebase | Update |
| 9 | `gitlab_mr_approve` | Approval | Update |
| 10 | `gitlab_mr_unapprove` | Approval | Update |
| 11 | `gitlab_mr_list_global` | Global & Group | Read |
| 12 | `gitlab_mr_list_group` | Global & Group | Read |
| 13 | `gitlab_mr_commits` | Commits & Pipelines | Read |
| 14 | `gitlab_mr_pipelines` | Commits & Pipelines | Read |
| 15 | `gitlab_mr_create_pipeline` | Commits & Pipelines | Create |
| 16 | `gitlab_mr_participants` | Participants & Reviewers | Read |
| 17 | `gitlab_mr_reviewers` | Participants & Reviewers | Read |
| 18 | `gitlab_mr_issues_closed` | Issues | Read |
| 19 | `gitlab_mr_related_issues` | Issues | Read |
| 20 | `gitlab_mr_subscribe` | Subscriptions | Update |
| 21 | `gitlab_mr_unsubscribe` | Subscriptions | Update |
| 22 | `gitlab_mr_set_time_estimate` | Time Tracking | Update |
| 23 | `gitlab_mr_reset_time_estimate` | Time Tracking | Update |
| 24 | `gitlab_mr_add_spent_time` | Time Tracking | Update |
| 25 | `gitlab_mr_reset_spent_time` | Time Tracking | Update |
| 26 | `gitlab_mr_time_stats` | Time Tracking | Read |
| 27 | `gitlab_mr_create_todo` | Todo | Create |
| 28 | `gitlab_mr_dependency_create` | Dependencies | Create |
| 29 | `gitlab_mr_dependency_delete` | Dependencies | Delete |
| 30 | `gitlab_mr_dependencies_list` | Dependencies | Read |
| 31 | `gitlab_mr_approval_state` | Approval Rules | Read |
| 32 | `gitlab_mr_approval_rules` | Approval Rules | Read |
| 33 | `gitlab_mr_approval_config` | Approval Rules | Read |
| 34 | `gitlab_mr_approval_reset` | Approval Rules | Update |
| 35 | `gitlab_mr_approval_rule_create` | Approval Rules | Create |
| 36 | `gitlab_mr_approval_rule_update` | Approval Rules | Update |
| 37 | `gitlab_mr_approval_rule_delete` | Approval Rules | Delete |
| 38 | `gitlab_mr_changes_get` | Changes & Diffs | Read |
| 39 | `gitlab_mr_diff_versions_list` | Changes & Diffs | Read |
| 40 | `gitlab_mr_diff_version_get` | Changes & Diffs | Read |
| 41 | `gitlab_mr_raw_diffs` | Changes & Diffs | Read |
| 42 | `gitlab_list_mr_context_commits` | Context Commits | Read |
| 43 | `gitlab_create_mr_context_commits` | Context Commits | Create |
| 44 | `gitlab_delete_mr_context_commits` | Context Commits | Delete |
| 45 | `gitlab_list_project_merge_trains` | Merge Trains | Read |
| 46 | `gitlab_list_merge_request_in_merge_train` | Merge Trains | Read |
| 47 | `gitlab_get_merge_request_on_merge_train` | Merge Trains | Read |
| 48 | `gitlab_add_merge_request_to_merge_train` | Merge Trains | Create |
| 49 | `gitlab_list_project_status_checks` | External Status Checks | Read |
| 50 | `gitlab_list_project_mr_external_status_checks` | External Status Checks | Read |
| 51 | `gitlab_list_project_external_status_checks` | External Status Checks | Read |
| 52 | `gitlab_create_project_external_status_check` | External Status Checks | Create |
| 53 | `gitlab_update_project_external_status_check` | External Status Checks | Update |
| 54 | `gitlab_delete_project_external_status_check` | External Status Checks | Delete |
| 55 | `gitlab_retry_failed_external_status_check_for_project_mr` | External Status Checks | Update |
| 56 | `gitlab_set_project_mr_external_status_check_status` | External Status Checks | Update |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` or require user confirmation before execution:

- `gitlab_mr_delete` — permanently deletes a merge request
- `gitlab_mr_merge` — merges an MR into its target branch (irreversible)
- `gitlab_mr_dependency_delete` — removes a merge request dependency
- `gitlab_mr_approval_rule_delete` — deletes an approval rule from a merge request
- `gitlab_delete_mr_context_commits` — removes context commits from a merge request
- `gitlab_delete_project_external_status_check` — deletes an external status check

---

## Related

- [GitLab Merge Requests API](https://docs.gitlab.com/ee/api/merge_requests.html)
- [GitLab Merge Request Approvals API](https://docs.gitlab.com/ee/api/merge_request_approvals.html)
- [GitLab Merge Request Diffs API](https://docs.gitlab.com/ee/api/merge_request_diffs.html)
- [GitLab Merge Request Context Commits API](https://docs.gitlab.com/ee/api/merge_request_context_commits.html)
- [GitLab Merge Trains API](https://docs.gitlab.com/ee/api/merge_trains.html)
- [GitLab External Status Checks API](https://docs.gitlab.com/ee/api/status_checks.html)
