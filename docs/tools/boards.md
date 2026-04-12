# Boards — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Boards, Labels & Milestones
> **Individual tools**: 25
> **Meta-tool**: `gitlab_board` (when `META_TOOLS=true`, default)
> **GitLab API**: [Issue Boards API](https://docs.gitlab.com/ee/api/boards.html), [Labels API](https://docs.gitlab.com/ee/api/labels.html), [Milestones API](https://docs.gitlab.com/ee/api/milestones.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The boards domain covers project issue boards (board CRUD and board list management), project labels (CRUD, subscription, promotion), and project milestones (CRUD, associated issues, and merge requests). Boards organize issues into columns; labels and milestones are key attributes used to filter and categorize board lists.

When `META_TOOLS=true` (the default), the 10 board tools are consolidated into a single `gitlab_board` meta-tool that dispatches by `action` parameter. Labels (8 tools) and milestones (7 tools) remain as individual tools.

### Common Questions

> "List issue boards for project 42"
> "Show the lists on board 1"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Board CRUD

### `gitlab_board_list`

List all issue boards for a project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_board_get`

Get a single issue board.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_board_create`

Create a new issue board in a project.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_board_update`

Update an existing issue board.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_board_delete`

Delete an issue board from a project. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Deletion cannot be undone.

---

## Board Lists

### `gitlab_board_list_lists`

List all lists in an issue board.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_board_list_get`

Get a single list from an issue board.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_board_list_create`

Create a new list in an issue board.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_board_list_update`

Update (reorder) a list in an issue board.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_board_list_delete`

Delete a list from an issue board. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Deletion cannot be undone.

---

## Labels

### `gitlab_label_list`

List all labels for a GitLab project. Supports filtering by search keyword, including issue/MR counts (with_counts), and including labels from ancestor groups. Returns label name, color, description, open/closed issue counts, and merge request counts with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_label_get`

Get details of a single project label by ID or name, including color, description, priority, and issue/MR counts.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_label_create`

Create a new label in a GitLab project with a name, color (hex), optional description, and optional priority.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_label_update`

Update an existing project label. Can change name, color, description, or priority. Only specified fields are modified.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_label_delete`

Delete a project label by ID or name.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_label_subscribe`

Subscribe to a project label to receive notifications when the label is applied to issues or merge requests.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_label_unsubscribe`

Unsubscribe from a project label to stop receiving notifications.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_label_promote`

Promote a project label to a group label, making it available to all projects in the group.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Milestones

### `gitlab_milestone_list`

List milestones for a GitLab project. Supports filtering by state (active, closed), exact title, search keyword, and including milestones from ancestor groups. Returns milestone title, description, state, start/due dates, web URL, and expiration status with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_milestone_get`

Get details of a single project milestone by ID. Returns milestone title, description, state, start/due dates, web URL, and expiration status.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_milestone_create`

Create a new milestone in a GitLab project. Requires title; optionally set description, start_date (YYYY-MM-DD), and due_date (YYYY-MM-DD).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_milestone_update`

Update an existing project milestone. Supports changing title, description, start_date, due_date, and state_event (activate/close).

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_milestone_delete`

Delete a project milestone. This action is irreversible.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Deletion is irreversible.

### `gitlab_milestone_issues`

List all issues assigned to a project milestone. Returns issue IID, title, state, web URL, and creation date with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_milestone_merge_requests`

List all merge requests assigned to a project milestone. Returns MR IID, title, state, source/target branches, web URL, and creation date with pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_board_list` | Board CRUD | Read |
| 2 | `gitlab_board_get` | Board CRUD | Read |
| 3 | `gitlab_board_create` | Board CRUD | Create |
| 4 | `gitlab_board_update` | Board CRUD | Update |
| 5 | `gitlab_board_delete` | Board CRUD | Delete |
| 6 | `gitlab_board_list_lists` | Board Lists | Read |
| 7 | `gitlab_board_list_get` | Board Lists | Read |
| 8 | `gitlab_board_list_create` | Board Lists | Create |
| 9 | `gitlab_board_list_update` | Board Lists | Update |
| 10 | `gitlab_board_list_delete` | Board Lists | Delete |
| 11 | `gitlab_label_list` | Labels | Read |
| 12 | `gitlab_label_get` | Labels | Read |
| 13 | `gitlab_label_create` | Labels | Create |
| 14 | `gitlab_label_update` | Labels | Update |
| 15 | `gitlab_label_delete` | Labels | Delete |
| 16 | `gitlab_label_subscribe` | Labels | Update |
| 17 | `gitlab_label_unsubscribe` | Labels | Update |
| 18 | `gitlab_label_promote` | Labels | Update |
| 19 | `gitlab_milestone_list` | Milestones | Read |
| 20 | `gitlab_milestone_get` | Milestones | Read |
| 21 | `gitlab_milestone_create` | Milestones | Create |
| 22 | `gitlab_milestone_update` | Milestones | Update |
| 23 | `gitlab_milestone_delete` | Milestones | Delete |
| 24 | `gitlab_milestone_issues` | Milestones | Read |
| 25 | `gitlab_milestone_merge_requests` | Milestones | Read |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_board_delete` — deletes an issue board
- `gitlab_board_list_delete` — deletes a list from an issue board
- `gitlab_label_delete` — deletes a project label
- `gitlab_milestone_delete` — deletes a project milestone

---

## Related

- [GitLab Issue Boards API](https://docs.gitlab.com/ee/api/boards.html)
- [GitLab Labels API](https://docs.gitlab.com/ee/api/labels.html)
- [GitLab Milestones API](https://docs.gitlab.com/ee/api/milestones.html)
