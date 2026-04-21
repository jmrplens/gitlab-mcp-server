# Epics — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Epics, Epic Issues, Epic Notes, Epic Discussions & Epic Boards
> **Individual tools**: 24
> **Meta-tool**: `gitlab_group` (epic routes, when `META_TOOLS=true`, default)
> **GitLab API**: [Work Items API (GraphQL)](https://docs.gitlab.com/ee/api/graphql/reference/#workitem) · [Epic Links API (REST)](https://docs.gitlab.com/ee/api/epic_links.html) · [Epic Boards API (REST)](https://docs.gitlab.com/ee/api/group_boards.html)
> **Audience**: 👤 End users, AI assistant users
> **Tier**: GitLab Premium / Ultimate

---

## Overview

The epics domain covers managing GitLab group epics — high-level planning items that can span multiple projects and group issues together. This includes CRUD operations on epics, managing epic-issue assignments, commenting on epics via notes, and listing epic boards.

Epics require GitLab Premium or Ultimate and are always scoped to a group.

> **Migrated to Work Items GraphQL API**: The epic tools (`list`, `get`,
> `create`, `update`, `delete`), epic issues, epic notes, and epic discussions
> now use the Work Items GraphQL API via client-go's `WorkItems` service.
> The deprecated Epics REST API (deprecated GitLab 17.0, removal planned 19.0)
> is no longer used for these operations. `get_links` remains on REST because
> client-go v2 does not yet expose a GraphQL query for work item children.
> Epic Boards still use REST.
> See [ADR-0009](../adr/adr-0009-progressive-graphql-migration.md) for the
> migration strategy.

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

List epics for a GitLab group via the Work Items GraphQL API (type=Epic). Supports filtering by state, labels, author, search text, and cursor-based pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_epic_get`

Get a single group epic by its IID via the Work Items GraphQL API. Returns title, description, state, labels, dates, author, assignees, linked items, and health status.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_epic_get_links`

Get all child epics of a parent epic (via REST API). Returns the list of sub-epics linked to the specified epic.

> **Note**: This tool uses the REST API because the Work Items GraphQL API does not yet support listing children.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_epic_create`

Create a new epic in a GitLab group via the Work Items GraphQL API. Supports title, description, labels, confidentiality, and color.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_epic_update`

Update an existing group epic via the Work Items GraphQL API. Can modify title, description, labels (replace, add, or remove), and state (close/reopen).

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_epic_delete`

Permanently delete an epic from a GitLab group via the Work Items GraphQL API.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Epic Issues

### `gitlab_epic_issue_list`

List all issues assigned to a GitLab group epic via the Work Items GraphQL API. Supports pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_epic_issue_assign`

Assign an existing issue to a GitLab group epic via the Work Items GraphQL API.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_epic_issue_remove`

Remove an issue from a GitLab group epic via the Work Items GraphQL API.

| Annotation | **Delete** |
| ---------- | ---------- |

### `gitlab_epic_issue_update`

Update the relationship of an issue within a GitLab group epic via the Work Items GraphQL API.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Epic Notes

### `gitlab_epic_note_list`

List all comments (notes) on a GitLab group epic via the Work Items GraphQL API. Supports ordering and cursor-based pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_epic_note_get`

Get a single comment (note) from a GitLab group epic via the Work Items GraphQL API, including author, timestamps, body, and system flag.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_epic_note_create`

Add a comment (note) to a GitLab group epic via the Work Items GraphQL API. Supports Markdown formatting.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_epic_note_update`

Edit the body text of an existing comment on a GitLab group epic via the Work Items GraphQL API.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_epic_note_delete`

Permanently delete a comment from a GitLab group epic via the Work Items GraphQL API.

| Annotation | **Delete** |
| ---------- | ---------- |

---

## Epic Discussions

### `gitlab_list_epic_discussions`

List all discussion threads on a GitLab group epic via the Work Items GraphQL API. Supports cursor-based pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_epic_discussion`

Get a single discussion thread from a GitLab group epic via the Work Items GraphQL API, including all notes in the thread.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_epic_discussion`

Create a new discussion thread on a GitLab group epic via the Work Items GraphQL API. Supports Markdown formatting.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_add_epic_discussion_note`

Reply to an existing discussion thread on a GitLab group epic via the Work Items GraphQL API.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_update_epic_discussion_note`

Update an existing note in an epic discussion thread via the Work Items GraphQL API.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_epic_discussion_note`

Permanently delete a note from an epic discussion thread via the Work Items GraphQL API.

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
| 16 | `gitlab_list_epic_discussions` | Epic Discussions | Read |
| 17 | `gitlab_get_epic_discussion` | Epic Discussions | Read |
| 18 | `gitlab_create_epic_discussion` | Epic Discussions | Create |
| 19 | `gitlab_add_epic_discussion_note` | Epic Discussions | Create |
| 20 | `gitlab_update_epic_discussion_note` | Epic Discussions | Update |
| 21 | `gitlab_delete_epic_discussion_note` | Epic Discussions | Delete |
| 22 | `gitlab_group_epic_board_list` | Epic Boards | Read |
| 23 | `gitlab_group_epic_board_get` | Epic Boards | Read |

### Destructive Tools (Require Confirmation)

- `gitlab_epic_delete` — permanently deletes an epic
- `gitlab_epic_issue_remove` — removes an issue from an epic
- `gitlab_epic_note_delete` — permanently deletes a note from an epic
- `gitlab_delete_epic_discussion_note` — permanently deletes a discussion note from an epic

---

## Related

- [GitLab Work Items GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#workitem)
- [GitLab Epics REST API (deprecated)](https://docs.gitlab.com/ee/api/epics.html)
- [GitLab Group Epic Boards API](https://docs.gitlab.com/ee/api/group_boards.html)
- [GraphQL Integration](../graphql.md)
- [ADR-0009: Progressive GraphQL Migration](../adr/adr-0009-progressive-graphql-migration.md)
- [Groups — Tool Reference](groups.md)
