# Issues — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Issues
> **Individual tools**: 42
> **Meta-tools**: `gitlab_issue` (when `META_TOOLS=true`, default), `gitlab_issue_discussion`, `gitlab_issue_statistics`
> **GitLab API**: [Issues API](https://docs.gitlab.com/ee/api/issues.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The issues domain covers the full lifecycle of GitLab issues: creation, retrieval, listing, updating, deletion, reordering, moving between projects, subscriptions, to-do creation, time tracking, participants, related merge requests, notes (comments), issue links, discussion threads, issue statistics, and work items.

When `META_TOOLS=true` (the default), core issue tools (including notes, links, and work items) are consolidated into a single `gitlab_issue` meta-tool that dispatches by `action` parameter. Discussions and statistics have their own meta-tools: `gitlab_issue_discussion` and `gitlab_issue_statistics`.

### Common Questions

> "List open issues in project 42"
> "Create an issue about the login bug"
> "Close issue #10 in my-project"
> "What issues are assigned to me?"

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

### `gitlab_issue_create`

Create a new issue in a GitLab project. Supports title, description (Markdown), assignees, labels, milestone, due date, confidential flag, issue_type (issue/incident/test_case/task), weight, and epic_id. Returns the created issue with ID, IID, state, and web URL.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_issue_get`

Retrieve a single GitLab issue by its project-scoped IID. Returns title, description, state, labels, assignees, milestone, author, timestamps, and web URL.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_list`

List issues for a GitLab project with filters for state, labels, milestone, assignee, author, and search. Returns paginated results with issue details.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_update`

Update a GitLab issue. Supports changing title, description, state (close/reopen), assignees, labels (replace, add, or remove), milestone, due date, confidential flag, issue_type, weight, and discussion_locked. Only specified fields are modified.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_issue_delete`

Permanently delete a GitLab issue. This action cannot be undone. Requires at least Maintainer access level.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Permanent deletion cannot be undone.

---

## Query & Navigation

### `gitlab_issue_list_group`

List issues across all projects in a GitLab group. Supports filtering by state, labels, milestone, scope, time range, assignee, author, and search. Returns paginated issue details including project reference.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_list_all`

List issues visible to the authenticated user across all projects (global scope). Supports filtering by state, labels, milestone, scope, search, assignee, author, time range, confidential flag, and ordering. Returns paginated results.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_get_by_id`

Retrieve a single GitLab issue by its global numeric ID (not the project-scoped IID). Useful when you have the global issue ID from another API response.

| Annotation | **Read** |
| ---------- | -------- |

---

## Actions

### `gitlab_issue_reorder`

Reorder an issue by specifying the issue to position it before or after. Use move_after_id and/or move_before_id to set the relative position.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_issue_move`

Move an issue from one project to another. Requires at least Reporter access on both the source and target projects.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_issue_subscribe`

Subscribe the authenticated user to an issue to receive notifications on updates.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_issue_unsubscribe`

Unsubscribe the authenticated user from an issue to stop receiving notifications.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_issue_create_todo`

Create a to-do item for the authenticated user on the specified issue. The to-do will appear in the user's GitLab to-do list.

| Annotation | **Create** |
| ---------- | ---------- |

---

## Time Tracking

### `gitlab_issue_time_estimate_set`

Set the time estimate for an issue using a human-readable duration (e.g. 3h30m, 1w2d).

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_issue_time_estimate_reset`

Reset the time estimate for an issue back to zero.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_issue_spent_time_add`

Add spent time to an issue using a human-readable duration (e.g. 1h, 30m) with an optional summary.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_issue_spent_time_reset`

Reset the total spent time for an issue to zero.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_issue_time_stats_get`

Get time tracking statistics for an issue (estimate and spent time).

| Annotation | **Read** |
| ---------- | -------- |

---

## Relationships

### `gitlab_issue_participants`

List all participants (users who engaged) in an issue. Returns usernames, names, and profile URLs.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_mrs_closing`

List merge requests that will close this issue when merged. Returns MR details including source/target branches.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_mrs_related`

List merge requests related to this issue. Returns MR details including source/target branches.

| Annotation | **Read** |
| ---------- | -------- |

---

## Notes (Comments)

### `gitlab_issue_note_create`

Add a comment (note) to a GitLab issue. Supports Markdown formatting and optional internal visibility flag (visible only to project members).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_issue_note_list`

List all comments (notes) on a GitLab issue. Supports ordering by created_at or updated_at, sort direction, and pagination. Returns note body, author, timestamps, and system/internal flags.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_note_get`

Get a single comment (note) from a GitLab issue by its note ID, including author, timestamps, body, and internal/system flags.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_note_update`

Edit the body text of an existing comment on a GitLab issue. Only the note author or a project maintainer can update a note.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_issue_note_delete`

Permanently delete a comment from a GitLab issue. Only the note author or a project maintainer can delete a note.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Issue Links

### `gitlab_issue_link_list`

List issue relations (linked issues) for a given issue in a GitLab project. Returns related issues with link type (relates_to, blocks, is_blocked_by).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_link_get`

Get a specific issue link by ID, returning source and target issue details with link type.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_link_create`

Create a link between two issues. Specify source project/issue and target project/issue. Link types: relates_to (default), blocks, is_blocked_by.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_issue_link_delete`

Delete an issue link, removing the two-way relationship between the linked issues. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Discussions

### `gitlab_list_issue_discussions`

List discussion threads on a project issue.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_issue_discussion`

Get a single discussion thread on a project issue.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_issue_discussion`

Create a new discussion thread on a project issue.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_add_issue_discussion_note`

Add a reply note to an existing issue discussion thread.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_update_issue_discussion_note`

Update an existing note in an issue discussion thread.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_issue_discussion_note`

Delete a note from an issue discussion thread.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Statistics

### `gitlab_get_issue_statistics`

Get global issue statistics (counts of all/opened/closed issues).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_group_issue_statistics`

Get issue statistics for a group.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_project_issue_statistics`

Get issue statistics for a project.

| Annotation | **Read** |
| ---------- | -------- |

---

## Work Items (Experimental)

### `gitlab_get_work_item`

Get a single work item by IID. Experimental: the Work Items API may introduce breaking changes between minor versions.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_list_work_items`

List work items for a project or group. Supports filtering by state, type, labels, author, search. Experimental: the Work Items API may introduce breaking changes between minor versions.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_work_item`

Create a new work item. Requires full_path, work_item_type_id, and title. Experimental: the Work Items API may introduce breaking changes between minor versions.

| Annotation | **Create** |
| ---------- | ---------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_issue_create` | Core CRUD | Create |
| 2 | `gitlab_issue_get` | Core CRUD | Read |
| 3 | `gitlab_issue_list` | Core CRUD | Read |
| 4 | `gitlab_issue_update` | Core CRUD | Update |
| 5 | `gitlab_issue_delete` | Core CRUD | Delete |
| 6 | `gitlab_issue_list_group` | Query & Navigation | Read |
| 7 | `gitlab_issue_list_all` | Query & Navigation | Read |
| 8 | `gitlab_issue_get_by_id` | Query & Navigation | Read |
| 9 | `gitlab_issue_reorder` | Actions | Update |
| 10 | `gitlab_issue_move` | Actions | Update |
| 11 | `gitlab_issue_subscribe` | Actions | Update |
| 12 | `gitlab_issue_unsubscribe` | Actions | Update |
| 13 | `gitlab_issue_create_todo` | Actions | Create |
| 14 | `gitlab_issue_time_estimate_set` | Time Tracking | Update |
| 15 | `gitlab_issue_time_estimate_reset` | Time Tracking | Update |
| 16 | `gitlab_issue_spent_time_add` | Time Tracking | Update |
| 17 | `gitlab_issue_spent_time_reset` | Time Tracking | Update |
| 18 | `gitlab_issue_time_stats_get` | Time Tracking | Read |
| 19 | `gitlab_issue_participants` | Relationships | Read |
| 20 | `gitlab_issue_mrs_closing` | Relationships | Read |
| 21 | `gitlab_issue_mrs_related` | Relationships | Read |
| 22 | `gitlab_issue_note_create` | Notes | Create |
| 23 | `gitlab_issue_note_list` | Notes | Read |
| 24 | `gitlab_issue_note_get` | Notes | Read |
| 25 | `gitlab_issue_note_update` | Notes | Update |
| 26 | `gitlab_issue_note_delete` | Notes | Delete |
| 27 | `gitlab_issue_link_list` | Issue Links | Read |
| 28 | `gitlab_issue_link_get` | Issue Links | Read |
| 29 | `gitlab_issue_link_create` | Issue Links | Create |
| 30 | `gitlab_issue_link_delete` | Issue Links | Delete |
| 31 | `gitlab_list_issue_discussions` | Discussions | Read |
| 32 | `gitlab_get_issue_discussion` | Discussions | Read |
| 33 | `gitlab_create_issue_discussion` | Discussions | Create |
| 34 | `gitlab_add_issue_discussion_note` | Discussions | Create |
| 35 | `gitlab_update_issue_discussion_note` | Discussions | Update |
| 36 | `gitlab_delete_issue_discussion_note` | Discussions | Delete |
| 37 | `gitlab_get_issue_statistics` | Statistics | Read |
| 38 | `gitlab_get_group_issue_statistics` | Statistics | Read |
| 39 | `gitlab_get_project_issue_statistics` | Statistics | Read |
| 40 | `gitlab_get_work_item` | Work Items | Read |
| 41 | `gitlab_list_work_items` | Work Items | Read |
| 42 | `gitlab_create_work_item` | Work Items | Create |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_issue_delete` — permanently deletes an issue
- `gitlab_issue_note_delete` — permanently deletes an issue comment
- `gitlab_issue_link_delete` — removes the link between two issues
- `gitlab_delete_issue_discussion_note` — deletes a note from a discussion thread

---

## Related

- [GitLab Issues API](https://docs.gitlab.com/ee/api/issues.html)
- [GitLab Issue Notes API](https://docs.gitlab.com/ee/api/notes.html#issues)
- [GitLab Issue Links API](https://docs.gitlab.com/ee/api/issue_links.html)
- [GitLab Discussions API](https://docs.gitlab.com/ee/api/discussions.html#issues)
- [GitLab Issue Statistics API](https://docs.gitlab.com/ee/api/issues_statistics.html)
- [GitLab Work Items API](https://docs.gitlab.com/ee/api/work_items.html)
