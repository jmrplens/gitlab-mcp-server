# Issues — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Issues
> **Individual tools**: 56
> **Meta-tool**: `gitlab_issue` (`GITLAB_MCP_TOOL_SURFACE=meta` catalog)
> **Dynamic IDs**: `issue.*` (default surface, via `gitlab_execute_action`)
> **GitLab API**: [Issues API](https://docs.gitlab.com/ee/api/issues.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The issues domain covers the full lifecycle of GitLab issues: creation, retrieval, listing, updating, deletion, reordering, moving between projects, subscriptions, to-do creation, time tracking, participants, related merge requests, notes (comments), issue links, discussion threads, issue statistics, work items, and work item saved views.

On the default dynamic surface, these operations are the `issue.*` entries of the canonical action catalog: find them with `gitlab_find_action` and run them with `gitlab_execute_action` by `domain.action` ID. With `GITLAB_MCP_TOOL_SURFACE=individual`, each is the tool named in the tables below.

With `GITLAB_MCP_TOOL_SURFACE=meta`, the whole domain is one `gitlab_issue` meta-tool that dispatches by `action` parameter — notes, links, work items, work item saved views, award emoji, resource events, discussions and statistics included. There is no separate discussion or statistics meta-tool; those are actions on `gitlab_issue` (`discussion_list`, `statistics_get`).

### Common Questions

> "List open issues in project 42"
> "Create an issue about the login bug"
> "Close issue #10 in my-project"
> "What issues are assigned to me?"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description                                    |
| ---------- | :------: | :---------: | :--------: | ---------------------------------------------- |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation                       |
| **Create** |    —     |     No      |     —      | Creates a new resource                         |
| **Update** |    —     |     No      |    Yes     | Modifies an existing resource                  |
| **Delete** |    —     |     Yes     |    Yes     | Destroys a resource; protected by confirmation |

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

## Iterations (Premium)

> Iterations are time-boxed sprints (Premium). Iteration events record when an issue's iteration was assigned or removed and when its weight was set.

### `gitlab_list_group_iterations`

List iterations (sprints) for a group (Premium). Supports filtering by `state` (`opened`, `upcoming`, `current`, `closed`, `all`), `include_ancestors`, and `search` (title). Returns iterations with sequence, title, description, state, start and due dates, web URL, and pagination metadata.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_list_project_iterations`

List iterations (sprints) for a project (Premium). Supports filtering by `state`, `include_ancestors` (pulls in ancestor-group iterations), and `search` (title). Returns iterations with id, iid, sequence, group_id, title, description, state, start/due dates, timestamps, web URL, and pagination metadata.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_iteration_event_list`

List iteration events for an issue (Premium) — every assignment or removal of an iteration against the issue, with action, iteration, acting user, and pagination metadata. Supports keyset or offset pagination (`page`, `per_page` 1–100, `pagination`, `page_token`, `order_by`, `sort` `asc|desc`).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_iteration_event_get`

Get a single iteration event for an issue (Premium) by `iteration_event_id`. Returns the event with action, the iteration object, and the acting user.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_weight_event_list`

List weight events for an issue (Premium) — every weight value set, with the weight, the acting user, and pagination metadata. Supports keyset or offset pagination (`page`, `per_page` 1–100, `pagination`, `page_token`, `order_by`, `sort` `asc|desc`).

| Annotation | **Read** |
| ---------- | -------- |

---

## Work Items (Experimental)

### `gitlab_get_work_item`

Get a single work item by IID. Returns hierarchy child work items (namespace path and IID) alongside linked items. Experimental: the Work Items API may introduce breaking changes between minor versions.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_list_work_items`

List work items for a project or group. Supports filtering by state, type, labels, author, search. Each item includes its assignees, labels, linked items and hierarchy child work items (namespace path and IID), with cursor pagination (`first`/`after` forward, `last`/`before` backward). The cursor picks the direction: `before` on its own pages backward at the default size, and naming both `first` and `last` is refused, because GitLab refuses it too. `sort` is a GraphQL `WorkItemSort` value such as `CREATED_DESC` (the default), `TITLE_ASC` or `PRIORITY_DESC`, not the `asc`/`desc` pair the REST endpoints take. Experimental: the Work Items API may introduce breaking changes between minor versions.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_work_item`

Create a new work item. Requires full_path, work_item_type_id, and title. Supports status (TODO/IN_PROGRESS/DONE/WONT_DO/DUPLICATE) and linked_items to link other work items on creation. Experimental: the Work Items API may introduce breaking changes between minor versions.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_update_work_item`

Update an existing work item by IID. Supports changing title, state (CLOSE/REOPEN), description, assignees, milestone, labels (add/remove), dates, weight, health status, iteration, color, and status (TODO/IN_PROGRESS/DONE/WONT_DO/DUPLICATE). `assignee_ids` and `crm_contact_ids` replace the whole list: an empty array removes every entry, and omitting the field leaves it untouched; removing entries that exist requires `confirm=true` or an approved confirmation prompt. Experimental: the Work Items API may introduce breaking changes between minor versions.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_work_item`

Permanently delete a work item by IID. This action cannot be undone. Experimental: the Work Items API may introduce breaking changes between minor versions.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_list_work_item_types`

List available work item types (system-defined and custom) for a project or group namespace. Returns type ID, name, and enabled flag. Supports filtering by name and `only_available`, with cursor pagination (`first`/`after` forward, `last`/`before` backward). The cursor picks the direction: `before` on its own pages backward at the default size, and naming both `first` and `last` is refused, because GitLab refuses it too. Experimental: the Work Items API may introduce breaking changes between minor versions.

| Annotation | **Read** |
| ---------- | -------- |

---

## Work Item Saved Views (Experimental)

A saved view stores a named, reusable work item filter under a group or project namespace: the filter itself, the sort order, and the display settings the consuming UI renders it with. Available on Free, Premium and Ultimate. The GraphQL surface is marked experimental by GitLab and may introduce breaking changes between minor versions.

`sort` is a `WorkItemSort` enum value (`CREATED_ASC`, `CREATED_DESC`, `TITLE_ASC`, `TITLE_DESC`, `UPDATED_ASC`, `UPDATED_DESC`, `PRIORITY_ASC`, `WEIGHT_DESC` and the rest of the enum). `display_settings` is an opaque JSON object GitLab validates against its own schema, so its keys are camelCase: `viewMode` (`list`, `board` or `table`), `hiddenMetadataKeys`, `collapsedGroups`, `visibleGroups`, `groupOrder`.

`filters` mirrors GitLab's `WorkItemSavedViewFilterInput` one for one, including the nested `not`, `or`, `hierarchy_filters`, `status` and `custom_field` sub-objects. The eight time filters (`created_after`, `created_before`, `closed_after`, `closed_before`, `due_after`, `due_before`, `updated_after`, `updated_before`) take ISO 8601 timestamps.

### `gitlab_work_item_saved_view_get`

Get a single saved view by namespace path and numeric ID. This is the only action that returns the view's `filters`: GitLab resolves that field at most once per GraphQL request, so the list query does not ask for it. Experimental: the Work Item Saved Views API may introduce breaking changes between minor versions.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_work_item_saved_view_list`

List the saved views under a group or project namespace, with cursor pagination (`first`/`after` forward, `last`/`before` backward). The cursor picks the direction: `before` on its own pages backward at the default size, and naming both `first` and `last` is refused, because GitLab refuses it too. `filters` is omitted from every entry. Experimental: the Work Item Saved Views API may introduce breaking changes between minor versions.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_work_item_saved_view_create`

Create a saved view under a namespace. Requires `namespace_path`, `name` and `sort`. Optional `description`, `is_private` (defaults to true), `filters` and `display_settings`; an omitted `display_settings` is stored as an empty object. Experimental: the Work Item Saved Views API may introduce breaking changes between minor versions.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_work_item_saved_view_update`

Update a saved view by numeric ID. Every field is optional and an omitted one is left unchanged. Supplying `filters` or `display_settings` replaces the stored value wholesale, so read the current one with `gitlab_work_item_saved_view_get` first when the intent is to add a condition rather than replace the query. Experimental: the Work Item Saved Views API may introduce breaking changes between minor versions.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_work_item_saved_view_delete`

Permanently delete a saved view by numeric ID. This action cannot be undone and removes the view for everyone it was shared with. Experimental: the Work Item Saved Views API may introduce breaking changes between minor versions.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_work_item_saved_view_subscribe`

Subscribe the authenticated user to a saved view, so it appears among their followed views. Experimental: the Work Item Saved Views API may introduce breaking changes between minor versions.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_work_item_saved_view_unsubscribe`

Unsubscribe the authenticated user from a saved view. The view itself is untouched. Experimental: the Work Item Saved Views API may introduce breaking changes between minor versions.

| Annotation | **Update** |
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
| 6 | `gitlab_issue_list_all` | Query & Navigation | Read |
| 7 | `gitlab_issue_get_by_id` | Query & Navigation | Read |
| 8 | `gitlab_issue_reorder` | Actions | Update |
| 9 | `gitlab_issue_move` | Actions | Update |
| 10 | `gitlab_issue_subscribe` | Actions | Update |
| 11 | `gitlab_issue_unsubscribe` | Actions | Update |
| 12 | `gitlab_issue_create_todo` | Actions | Create |
| 13 | `gitlab_issue_time_estimate_set` | Time Tracking | Update |
| 14 | `gitlab_issue_time_estimate_reset` | Time Tracking | Update |
| 15 | `gitlab_issue_spent_time_add` | Time Tracking | Update |
| 16 | `gitlab_issue_spent_time_reset` | Time Tracking | Update |
| 17 | `gitlab_issue_time_stats_get` | Time Tracking | Read |
| 18 | `gitlab_issue_participants` | Relationships | Read |
| 19 | `gitlab_issue_mrs_closing` | Relationships | Read |
| 20 | `gitlab_issue_mrs_related` | Relationships | Read |
| 21 | `gitlab_issue_note_create` | Notes | Create |
| 22 | `gitlab_issue_note_list` | Notes | Read |
| 23 | `gitlab_issue_note_get` | Notes | Read |
| 24 | `gitlab_issue_note_update` | Notes | Update |
| 25 | `gitlab_issue_note_delete` | Notes | Delete |
| 26 | `gitlab_issue_link_list` | Issue Links | Read |
| 27 | `gitlab_issue_link_get` | Issue Links | Read |
| 28 | `gitlab_issue_link_create` | Issue Links | Create |
| 29 | `gitlab_issue_link_delete` | Issue Links | Delete |
| 30 | `gitlab_list_issue_discussions` | Discussions | Read |
| 31 | `gitlab_get_issue_discussion` | Discussions | Read |
| 32 | `gitlab_create_issue_discussion` | Discussions | Create |
| 33 | `gitlab_add_issue_discussion_note` | Discussions | Create |
| 34 | `gitlab_update_issue_discussion_note` | Discussions | Update |
| 35 | `gitlab_delete_issue_discussion_note` | Discussions | Delete |
| 36 | `gitlab_get_issue_statistics` | Statistics | Read |
| 37 | `gitlab_get_group_issue_statistics` | Statistics | Read |
| 38 | `gitlab_get_project_issue_statistics` | Statistics | Read |
| 39 | `gitlab_list_group_iterations` | Iterations (Premium) | Read |
| 40 | `gitlab_list_project_iterations` | Iterations (Premium) | Read |
| 41 | `gitlab_issue_iteration_event_list` | Iterations (Premium) | Read |
| 42 | `gitlab_issue_iteration_event_get` | Iterations (Premium) | Read |
| 43 | `gitlab_issue_weight_event_list` | Iterations (Premium) | Read |
| 44 | `gitlab_get_work_item` | Work Items | Read |
| 45 | `gitlab_list_work_items` | Work Items | Read |
| 46 | `gitlab_create_work_item` | Work Items | Create |
| 47 | `gitlab_update_work_item` | Work Items | Update |
| 48 | `gitlab_delete_work_item` | Work Items | Delete |
| 49 | `gitlab_list_work_item_types` | Work Items | Read |
| 50 | `gitlab_work_item_saved_view_get` | Work Item Saved Views | Read |
| 51 | `gitlab_work_item_saved_view_list` | Work Item Saved Views | Read |
| 52 | `gitlab_work_item_saved_view_create` | Work Item Saved Views | Create |
| 53 | `gitlab_work_item_saved_view_update` | Work Item Saved Views | Update |
| 54 | `gitlab_work_item_saved_view_delete` | Work Item Saved Views | Delete |
| 55 | `gitlab_work_item_saved_view_subscribe` | Work Item Saved Views | Update |
| 56 | `gitlab_work_item_saved_view_unsubscribe` | Work Item Saved Views | Update |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_issue_delete` — permanently deletes an issue
- `gitlab_issue_note_delete` — permanently deletes an issue comment
- `gitlab_issue_link_delete` — removes the link between two issues
- `gitlab_delete_issue_discussion_note` — deletes a note from a discussion thread
- `gitlab_delete_work_item` — permanently deletes a work item
- `gitlab_work_item_saved_view_delete` deletes a work item saved view permanently

---

## Related

- [GitLab Issues API](https://docs.gitlab.com/ee/api/issues.html)
- [GitLab Issue Notes API](https://docs.gitlab.com/ee/api/notes.html#issues)
- [GitLab Issue Links API](https://docs.gitlab.com/ee/api/issue_links.html)
- [GitLab Discussions API](https://docs.gitlab.com/ee/api/discussions.html#issues)
- [GitLab Issue Statistics API](https://docs.gitlab.com/ee/api/issues_statistics.html)
- [GitLab Work Items API](https://docs.gitlab.com/api/graphql/reference/#workitem)
- [GitLab Work Item Saved Views](https://docs.gitlab.com/user/work_items/saved_views/)
