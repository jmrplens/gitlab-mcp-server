# Groups — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Groups
> **Individual tools**: 61
> **Meta-tools**: `gitlab_group`, `gitlab_group_member`, `gitlab_group_label`, `gitlab_group_milestone`, `gitlab_group_variable`, `gitlab_group_import_export`, `gitlab_group_board`, `gitlab_group_relations_export`, `gitlab_group_markdown_upload` (when `META_TOOLS=true`, default)
> **GitLab API**: [Groups API](https://docs.gitlab.com/ee/api/groups.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The groups domain covers the full lifecycle of GitLab groups: creation, retrieval, listing, updating, deletion, restoration, searching, project transfers, subgroup management, webhooks, members, labels, milestones, CI/CD variables, import/export, issue boards, relations exports, and markdown uploads.

When `META_TOOLS=true` (the default), the 61 individual tools below are consolidated into domain-specific meta-tools that dispatch by `action` parameter.

### Common Questions

> "List my GitLab groups"
> "Who are the members of group my-team?"
> "List projects in the my-org group"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Core Group CRUD

### `gitlab_group_list`

List GitLab groups accessible to the authenticated user. Supports filtering by search term, ownership, and top-level only. Returns paginated results including group name, path, visibility, and web URL.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_get`

Retrieve detailed metadata for a GitLab group including name, path, full path, description, visibility, web URL, and parent group. Accepts numeric group ID or URL-encoded path (e.g. 'group/subgroup').

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_create`

Create a new GitLab group. Requires name; optionally set path, description, visibility, parent_id (for subgroups), request_access_enabled, lfs_enabled, and default_branch.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_update`

Update an existing GitLab group. Supports changing name, path, description, visibility, request_access_enabled, lfs_enabled, and default_branch.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_delete`

Delete a GitLab group. On instances with delayed deletion, the group is marked for deletion. Set permanently_remove=true with full_path to bypass delayed deletion.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Permanent removal cannot be undone.

### `gitlab_group_restore`

Restore a GitLab group that was marked for deletion.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_search`

Search for GitLab groups by name. Returns matching groups with their details.

| Annotation | **Read** |
| ---------- | -------- |

---

## Subgroups & Projects

### `gitlab_subgroups_list`

List descendant subgroups of a GitLab group. Returns each subgroup's name, path, full path, description, visibility, and parent ID. Supports search filter and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_projects`

List projects belonging to a GitLab group. Supports filtering by search, archived status, visibility, and including subgroup projects. Returns project name, path, visibility, and archived status with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_transfer_project`

Transfer a project into a group namespace. Moves the project to become a member of the specified group.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Group Members (legacy in groups package)

### `gitlab_group_members_list`

List all members of a GitLab group including inherited members. Returns user ID, username, name, state, access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner), and web URL. Supports filtering by name/username query.

| Annotation | **Read** |
| ---------- | -------- |

---

## Group Webhooks

### `gitlab_group_hook_list`

List webhooks configured for a GitLab group. Returns hook URL, enabled events, SSL verification status, and creation date with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_hook_get`

Get details of a specific group webhook by hook ID. Returns URL, enabled events, SSL status, and alert status.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_hook_add`

Add a new webhook to a GitLab group. Requires URL; optionally configure event triggers, SSL verification, secret token, and branch filter.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_hook_edit`

Edit an existing group webhook. Supports changing URL, events, SSL verification, secret token, and branch filter.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_hook_delete`

Delete a webhook from a GitLab group.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Group Member Management

### `gitlab_group_member_get`

Get a single member of a GitLab group by user ID. Returns user details including access level, state, and expiration date.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_member_get_inherited`

Get a single inherited member of a GitLab group by user ID. Returns member details including access level inherited from parent groups.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_member_add`

Add a member to a GitLab group. Specify user by user_id or username, and set access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner). Optionally set expiration date.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_member_edit`

Edit a member of a GitLab group. Update access level or expiration date for an existing member.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_member_remove`

Remove a member from a GitLab group. Optionally skip subresource removal and unassign issuables.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_group_share`

Share a GitLab group with another group, granting the shared group a specified access level. Optionally set an expiration date.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_unshare`

Stop sharing a GitLab group with another group, removing the group-level access.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Group Labels

### `gitlab_group_label_list`

List all labels for a GitLab group. Supports filtering by search keyword, including issue/MR counts (with_counts), ancestor/descendant groups, and group-only labels. Returns label name, color, description, open/closed issue counts, and MR counts with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_label_get`

Get details of a single group label by ID or name, including color, description, priority, and issue/MR counts.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_label_create`

Create a new label in a GitLab group with a name, color (hex), optional description, and optional priority.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_label_update`

Update an existing group label. Can change name, color, description, or priority. Only specified fields are modified.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_label_delete`

Delete a group label by ID or name.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_group_label_subscribe`

Subscribe to a group label to receive notifications when the label is applied to issues or merge requests.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_label_unsubscribe`

Unsubscribe from a group label to stop receiving notifications.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Group Milestones

### `gitlab_group_milestone_list`

List all milestones for a GitLab group. Supports filtering by state, title, search, IIDs, date ranges, and ancestor/descendant groups. Returns milestone title, state, dates, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_milestone_get`

Get details of a single group milestone by ID, including title, state, start/due dates, and timestamps.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_milestone_create`

Create a new milestone in a GitLab group with a title, optional description, start date and due date (YYYY-MM-DD).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_milestone_update`

Update an existing group milestone. Can change title, description, dates, or state (activate/close). Only specified fields are modified.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_milestone_delete`

Delete a group milestone by ID.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_group_milestone_issues`

List all issues assigned to a group milestone. Returns issue ID, IID, title, state, and web URL with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_milestone_merge_requests`

List all merge requests assigned to a group milestone. Returns MR ID, IID, title, state, source/target branches with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_milestone_burndown_events`

List all burndown chart events for a group milestone. Returns event timestamps, weights, and actions with pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Group CI/CD Variables

> **Auto-masking**: Variables flagged as `masked` or `hidden` have their values automatically redacted to `[masked]` in all responses.

### `gitlab_group_variable_list`

List CI/CD variables for a GitLab group. Returns paginated results with variable key, type, protection, masking, and environment scope.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_variable_get`

Get a specific CI/CD variable by key from a GitLab group. Optionally filter by environment scope when duplicate keys exist.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_variable_create`

Create a new CI/CD variable in a GitLab group. Requires key and value. Optionally set type (env_var/file), protection, masking, and environment scope.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_variable_update`

Update an existing CI/CD variable in a GitLab group. Specify the key to update and any fields to change: value, type, protection, masking, environment scope.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_variable_delete`

Delete a CI/CD variable from a GitLab group by key. Optionally filter by environment scope. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Group Import/Export

### `gitlab_schedule_group_export`

Schedule an asynchronous export of a group.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_download_group_export`

Download the finished export archive of a group. Returns the archive as base64-encoded content.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_import_group_from_file`

Import a group from an export archive file. Requires a local file path to the .tar.gz archive.

| Annotation | **Create** |
| ---------- | ---------- |

---

## Group Issue Boards

### `gitlab_group_board_list`

List all issue boards for a group.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_board_get`

Get a single group issue board.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_board_create`

Create a new issue board in a group.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_board_update`

Update an existing group issue board.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_board_delete`

Delete a group issue board. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_group_board_list_lists`

List all lists in a group issue board.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_board_list_get`

Get a single list from a group issue board.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_board_list_create`

Create a new list in a group issue board.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_board_list_update`

Update (reorder) a list in a group issue board.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_board_list_delete`

Delete a list from a group issue board. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Group Relations Export

### `gitlab_schedule_group_relations_export`

Schedule a new group relations export.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_list_group_relations_export_status`

List the status of group relations exports.

| Annotation | **Read** |
| ---------- | -------- |

---

## Group Markdown Uploads

### `gitlab_list_group_markdown_uploads`

List markdown uploads for a group.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_delete_group_markdown_upload_by_id`

Delete a group markdown upload by ID.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_delete_group_markdown_upload_by_secret`

Delete a group markdown upload by secret and filename.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_group_list` | Core CRUD | Read |
| 2 | `gitlab_group_get` | Core CRUD | Read |
| 3 | `gitlab_group_create` | Core CRUD | Create |
| 4 | `gitlab_group_update` | Core CRUD | Update |
| 5 | `gitlab_group_delete` | Core CRUD | Delete |
| 6 | `gitlab_group_restore` | Core CRUD | Update |
| 7 | `gitlab_group_search` | Core CRUD | Read |
| 8 | `gitlab_subgroups_list` | Subgroups & Projects | Read |
| 9 | `gitlab_group_projects` | Subgroups & Projects | Read |
| 10 | `gitlab_group_transfer_project` | Subgroups & Projects | Update |
| 11 | `gitlab_group_members_list` | Members (legacy) | Read |
| 12 | `gitlab_group_hook_list` | Webhooks | Read |
| 13 | `gitlab_group_hook_get` | Webhooks | Read |
| 14 | `gitlab_group_hook_add` | Webhooks | Create |
| 15 | `gitlab_group_hook_edit` | Webhooks | Update |
| 16 | `gitlab_group_hook_delete` | Webhooks | Delete |
| 17 | `gitlab_group_member_get` | Member Management | Read |
| 18 | `gitlab_group_member_get_inherited` | Member Management | Read |
| 19 | `gitlab_group_member_add` | Member Management | Create |
| 20 | `gitlab_group_member_edit` | Member Management | Update |
| 21 | `gitlab_group_member_remove` | Member Management | Delete |
| 22 | `gitlab_group_share` | Member Management | Create |
| 23 | `gitlab_group_unshare` | Member Management | Delete |
| 24 | `gitlab_group_label_list` | Labels | Read |
| 25 | `gitlab_group_label_get` | Labels | Read |
| 26 | `gitlab_group_label_create` | Labels | Create |
| 27 | `gitlab_group_label_update` | Labels | Update |
| 28 | `gitlab_group_label_delete` | Labels | Delete |
| 29 | `gitlab_group_label_subscribe` | Labels | Update |
| 30 | `gitlab_group_label_unsubscribe` | Labels | Update |
| 31 | `gitlab_group_milestone_list` | Milestones | Read |
| 32 | `gitlab_group_milestone_get` | Milestones | Read |
| 33 | `gitlab_group_milestone_create` | Milestones | Create |
| 34 | `gitlab_group_milestone_update` | Milestones | Update |
| 35 | `gitlab_group_milestone_delete` | Milestones | Delete |
| 36 | `gitlab_group_milestone_issues` | Milestones | Read |
| 37 | `gitlab_group_milestone_merge_requests` | Milestones | Read |
| 38 | `gitlab_group_milestone_burndown_events` | Milestones | Read |
| 39 | `gitlab_group_variable_list` | CI/CD Variables | Read |
| 40 | `gitlab_group_variable_get` | CI/CD Variables | Read |
| 41 | `gitlab_group_variable_create` | CI/CD Variables | Create |
| 42 | `gitlab_group_variable_update` | CI/CD Variables | Update |
| 43 | `gitlab_group_variable_delete` | CI/CD Variables | Delete |
| 44 | `gitlab_schedule_group_export` | Import/Export | Create |
| 45 | `gitlab_download_group_export` | Import/Export | Read |
| 46 | `gitlab_import_group_from_file` | Import/Export | Create |
| 47 | `gitlab_group_board_list` | Issue Boards | Read |
| 48 | `gitlab_group_board_get` | Issue Boards | Read |
| 49 | `gitlab_group_board_create` | Issue Boards | Create |
| 50 | `gitlab_group_board_update` | Issue Boards | Update |
| 51 | `gitlab_group_board_delete` | Issue Boards | Delete |
| 52 | `gitlab_group_board_list_lists` | Issue Boards | Read |
| 53 | `gitlab_group_board_list_get` | Issue Boards | Read |
| 54 | `gitlab_group_board_list_create` | Issue Boards | Create |
| 55 | `gitlab_group_board_list_update` | Issue Boards | Update |
| 56 | `gitlab_group_board_list_delete` | Issue Boards | Delete |
| 57 | `gitlab_schedule_group_relations_export` | Relations Export | Create |
| 58 | `gitlab_list_group_relations_export_status` | Relations Export | Read |
| 59 | `gitlab_list_group_markdown_uploads` | Markdown Uploads | Read |
| 60 | `gitlab_delete_group_markdown_upload_by_id` | Markdown Uploads | Delete |
| 61 | `gitlab_delete_group_markdown_upload_by_secret` | Markdown Uploads | Delete |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_group_delete` — deletes a group (scheduled or permanent)
- `gitlab_group_hook_delete` — removes a group webhook
- `gitlab_group_member_remove` — removes a member from a group
- `gitlab_group_unshare` — revokes group-to-group sharing
- `gitlab_group_label_delete` — deletes a group label
- `gitlab_group_milestone_delete` — deletes a group milestone
- `gitlab_group_variable_delete` — deletes a group CI/CD variable
- `gitlab_group_board_delete` — deletes a group issue board
- `gitlab_group_board_list_delete` — deletes a list from a group issue board
- `gitlab_delete_group_markdown_upload_by_id` — deletes a markdown upload by ID
- `gitlab_delete_group_markdown_upload_by_secret` — deletes a markdown upload by secret

---

## Related

- [GitLab Groups API](https://docs.gitlab.com/ee/api/groups.html)
- [GitLab Group Members API](https://docs.gitlab.com/ee/api/members.html)
- [GitLab Group Labels API](https://docs.gitlab.com/ee/api/group_labels.html)
- [GitLab Group Milestones API](https://docs.gitlab.com/ee/api/group_milestones.html)
- [GitLab Group Variables API](https://docs.gitlab.com/ee/api/group_level_variables.html)
- [GitLab Group Import/Export API](https://docs.gitlab.com/ee/api/group_import_export.html)
- [GitLab Group Issue Boards API](https://docs.gitlab.com/ee/api/group_boards.html)
- [GitLab Group Webhooks API](https://docs.gitlab.com/ee/api/group_hooks.html)
