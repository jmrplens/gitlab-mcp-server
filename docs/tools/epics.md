# Epics — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Epics, Epic Issues, Epic Notes & Epic Boards
> **Individual tools**: 17
> **Meta-tool**: `gitlab_group` (epic routes, when `META_TOOLS=true`, default)
> **GitLab API**: [Epics API](https://docs.gitlab.com/ee/api/epics.html) · [Epic Issues API](https://docs.gitlab.com/ee/api/epic_issues.html) · [Epic Notes API](https://docs.gitlab.com/ee/api/notes.html) · [Epic Boards API](https://docs.gitlab.com/ee/api/group_boards.html)
> **Audience**: 👤 End users, AI assistant users
> **Tier**: GitLab Premium / Ultimate

---

## Overview

The epics domain covers managing GitLab group epics — high-level planning items that can span multiple projects and group issues together. This includes CRUD operations on epics, managing epic-issue assignments, commenting on epics via notes, and listing epic boards.

Epics require GitLab Premium or Ultimate and are always scoped to a group.

### Common Questions

> "List all open epics in group 5"
> "Create an epic titled 'Q3 Planning' in the engineering group"
> "What issues are assigned to epic #12?"
> "Add a comment to epic #7"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

---

## Epics

### `gitlab_epic_list`

List epics for a GitLab group. Supports filtering by state, labels, author, search text, and pagination. Can include epics from ancestor or descendant groups.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_epic_get`

Get a single group epic by its IID, including title, description, state, labels, dates, and author.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_epic_get_links`

Get all child epics of a parent epic. Returns the list of sub-epics linked to the specified epic.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_epic_create`

Create a new epic in a GitLab group. Supports title, description, labels, confidentiality, parent epic, and color.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_epic_update`

Update an existing group epic. Can modify title, description, labels (replace, add, or remove), state (close/reopen), confidentiality, parent, and color.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_epic_delete`

Permanently delete an epic from a GitLab group.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Epic Issues

### `gitlab_epic_issue_list`

List all issues assigned to a GitLab group epic. Supports pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_epic_issue_assign`

Assign an existing issue to a GitLab group epic. The issue is identified by its global ID.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_epic_issue_remove`

Remove an issue from a GitLab group epic using the epic-issue association ID.

| Annotation | **Delete** |
| ---------- | ---------- |

### `gitlab_epic_issue_update`

Reorder an issue within a GitLab group epic by moving it before or after another epic-issue.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Epic Notes

### `gitlab_epic_note_list`

List all comments (notes) on a GitLab group epic. Supports ordering by `created_at` or `updated_at`, sort direction, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_epic_note_get`

Get a single comment (note) from a GitLab group epic by its note ID, including author, timestamps, body, and system flag.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_epic_note_create`

Add a comment (note) to a GitLab group epic. Supports Markdown formatting.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_epic_note_update`

Edit the body text of an existing comment on a GitLab group epic. Only the note author or a group maintainer can update a note.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_epic_note_delete`

Permanently delete a comment from a GitLab group epic. Only the note author or a group maintainer can delete a note.

| Annotation | **Delete** |
| ---------- | ---------- |

---

## Epic Boards

### `gitlab_group_epic_board_list`

List all epic boards in a GitLab group. Supports pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_epic_board_get`

Get a single epic board in a GitLab group by its ID, including board lists (columns) and labels.

| Annotation | **Read** |
| ---------- | -------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_epic_list` | Epics | Read |
| 2 | `gitlab_epic_get` | Epics | Read |
| 3 | `gitlab_epic_get_links` | Epics | Read |
| 4 | `gitlab_epic_create` | Epics | Create |
| 5 | `gitlab_epic_update` | Epics | Update |
| 6 | `gitlab_epic_delete` | Epics | Delete |
| 7 | `gitlab_epic_issue_list` | Epic Issues | Read |
| 8 | `gitlab_epic_issue_assign` | Epic Issues | Create |
| 9 | `gitlab_epic_issue_remove` | Epic Issues | Delete |
| 10 | `gitlab_epic_issue_update` | Epic Issues | Update |
| 11 | `gitlab_epic_note_list` | Epic Notes | Read |
| 12 | `gitlab_epic_note_get` | Epic Notes | Read |
| 13 | `gitlab_epic_note_create` | Epic Notes | Create |
| 14 | `gitlab_epic_note_update` | Epic Notes | Update |
| 15 | `gitlab_epic_note_delete` | Epic Notes | Delete |
| 16 | `gitlab_group_epic_board_list` | Epic Boards | Read |
| 17 | `gitlab_group_epic_board_get` | Epic Boards | Read |

### Destructive Tools (Require Confirmation)

- `gitlab_epic_delete` — permanently deletes an epic
- `gitlab_epic_issue_remove` — removes an issue from an epic
- `gitlab_epic_note_delete` — permanently deletes a note from an epic

---

## Related

- [GitLab Epics API](https://docs.gitlab.com/ee/api/epics.html)
- [GitLab Epic Issues API](https://docs.gitlab.com/ee/api/epic_issues.html)
- [GitLab Notes API (Epic Notes)](https://docs.gitlab.com/ee/api/notes.html#epics)
- [GitLab Group Epic Boards API](https://docs.gitlab.com/ee/api/group_boards.html)
- [Groups — Tool Reference](groups.md)
