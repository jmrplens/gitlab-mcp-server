# Epics — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Epics, Epic Issues, Epic Notes, Epic Discussions & Epic Boards
> **Individual tools**: 17
> **Meta-tool**: `gitlab_group` (epic routes in the `GITLAB_MCP_TOOL_SURFACE=meta` catalog)
> **Dynamic IDs**: `group.*` (default surface, via `gitlab_execute_action`)
> **GitLab API**: [Work Items API (GraphQL)](https://docs.gitlab.com/ee/api/graphql/reference/#workitem) · [Epic Links API (REST)](https://docs.gitlab.com/ee/api/epic_links.html) · [Epic Boards API (REST)](https://docs.gitlab.com/ee/api/group_boards.html)
> **Audience**: 👤 End users, AI assistant users
> **Tier**: GitLab Premium / Ultimate

---

## Overview

The epics domain covers managing GitLab group epics — high-level planning items that can span multiple projects and group issues together. This includes CRUD operations on epics, managing epic-issue assignments, commenting on epics via notes, and listing epic boards.

On the default dynamic surface, these operations are the `group.*` entries of the canonical action catalog: find them with `gitlab_find_action` and run them with `gitlab_execute_action` by `domain.action` ID. With `GITLAB_MCP_TOOL_SURFACE=individual`, each is the tool named in the tables below.

Epics require GitLab Premium or Ultimate and are always scoped to a group.

> **Migrated to Work Items GraphQL API**: The epic tools (`list`, `get`,
> `create`, `update`, `delete`), epic issues, epic notes, and epic discussions
> now use the Work Items GraphQL API via client-go's `WorkItems` service.
> The deprecated Epics REST API (deprecated GitLab 17.0, removal planned 19.0)
> is no longer used for these operations. `get_links` remains on REST because
> client-go v2 does not yet expose a GraphQL query for work item children.
> Epic Boards still use REST.
> See [ADR-0009](../../development/adr/adr-0009-progressive-graphql-migration.md) for the
> migration strategy.

### Common Questions

> "List all open epics in group 5"
> "Create an epic titled 'Q3 Planning' in the engineering group"
> "What issues are assigned to epic #12?"
> "Add a comment to epic #7"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description                                    |
| ---------- | :------: | :---------: | :--------: | ---------------------------------------------- |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation                       |
| **Create** |    —     |     No      |     —      | Creates a new resource                         |
| **Update** |    —     |     No      |    Yes     | Modifies an existing resource                  |
| **Delete** |    —     |     Yes     |    Yes     | Destroys a resource; protected by confirmation |

---

## Epics

> **What the two REST-backed epic actions return.** `gitlab_epic_list` on its REST path and `gitlab_epic_get_links` decode GitLab's epic response whole rather than the subset the SDK struct declares, so they also carry `parent_iid`, `work_item_id`, `color`, `text_color`, `web_edit_url`, `references`, `imported`, `imported_from`, `_links`, `end_date`, `start_date_from_inherited_source` and `due_date_from_inherited_source`. On the list path `with_labels_details` fills `label_details` beside the `labels` names instead of failing the call, and the `author` object carries `locked` and `public_email` beside the six keys the SDK type declares. `user_notes_count` and `url` are gone: GitLab sends neither on any epic endpoint, and both were always zero.
>
> **Three keys the generated OpenAPI record lists and these actions do not publish.** `subscribed` is rendered only when the route asks the entity for it, which GitLab does on the single-epic REST GET alone, a route neither action calls; `reference` only under `with_reference`, which nothing sets and which GitLab deprecated in favour of `references`; and `label_details` only where `with_labels_details` exists, which is the list endpoint and not the child-epics one, so `gitlab_epic_get_links` never carries it. All were always empty. The record says what an epic entity can render, not what a given endpoint does.

### `gitlab_epic_list`

List epics for a GitLab group. Filters by state, search text (with `in` choosing title, description or both), author, assignees, labels, milestone, weight, health status, subscription, explicit IIDs or global IDs, parent epic, and the created, updated, closed and due date ranges. Pages in both directions: `first` and `after` forward, `last` and `before` backward.

| Annotation | **Read** |
| ---------- | -------- |

> **Two APIs answer this action, and the pagination block says which.** A request naming only what the REST epics endpoint accepts is served by it and answers with `offset_pagination`: `page`, `per_page`, `total_items`, `total_pages`, `next_page` and `has_more`, paged with `page`/`per_page` or the keyset pair. Any filter only the Work Items GraphQL query can express routes the whole request through that query instead, and it answers with `pagination`: `has_next_page`, `has_previous_page`, `end_cursor` and `start_cursor`, paged with `first`/`after` forward and `last`/`before` backward. Exactly one of the two blocks is ever present, and it is omitted when empty, so the one that came back is what tells a caller which page request to send next. Publishing one shape for both would mean answering a full REST page with `has_next_page` false, which is how a caller stops one page short of the rest of the list. `order_by` and `sort` apply on either path: the Work Items query takes the pair as one value and the server assembles it. `author_id` and `with_labels_details` are REST-only and are dropped when another filter takes the Work Items path; use `author_username` there.
>
> **The page request follows the same split, and a request that crosses it is refused.** `page`, `per_page`, `pagination` and `page_token` are the REST endpoint's; `first`, `after`, `last` and `before` are the Work Items query's. Naming one of the first four beside a filter that routes to the Work Items API is an error that names both halves, because that API pages by cursor and a cursor cannot be computed from a page number without walking the pages before it. Until it was refused, `page: 2` with such a filter answered with cursor page one and no error, which reads as a list that ends where it does not. Either drop the filter, or ask for the next page with `after` from the previous response's `end_cursor`.

### `gitlab_epic_get`

Get a single group epic by its IID via the Work Items GraphQL API. Returns title, description, state, labels (names and full label details), dates, author, assignees, children, linked items, and health status.

| Annotation | **Read** |
| ---------- | -------- |

> **No `status` and no `iteration_id`.** An Epic work item carries neither the STATUS nor the ITERATION widget, so both keys were null on every response and they are no longer published. Issues and tasks do carry them: reach those through the `work_item.get` action (`gitlab_get_work_item` with `GITLAB_MCP_TOOL_SURFACE=individual`).

### `gitlab_epic_get_links`

Get all child epics of a parent epic (via REST API). Returns the list of sub-epics linked to the specified epic.

> **Note**: This tool uses the REST API because the Work Items GraphQL API does not yet support listing children.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_epic_create`

Create a new epic in a GitLab group via the Work Items GraphQL API. Supports title, description, labels, assignees, confidentiality, color, weight, health status, milestone, a parent epic (`parent_id`, which creates a sub-epic in one call), links to other epics (`linked_items`), a backdated `created_at` for group owners and administrators, and a `create_source` tracking label.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_epic_update`

Update an existing group epic via the Work Items GraphQL API. Can modify title, description, labels (add or remove), assignees, dates, weight, health status, milestone, parent epic, and state (close/reopen).

| Annotation | **Update** |
| ---------- | ---------- |

> **No `status`, `iteration_id` or `crm_contact_ids`.** The work item mutation accepts all three for the types that carry the STATUS, ITERATION and CRM_CONTACTS widgets, and an Epic carries none of them, so GitLab refuses each on an epic. They stay available on the `work_item.*` actions, where the caller chooses the type.

### `gitlab_epic_delete`

Permanently delete an epic from a GitLab group via the Work Items GraphQL API.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Epic Issues

### `gitlab_epic_issue_list`

List all issues assigned to a GitLab group epic via the Work Items GraphQL API. Pages in both directions: `first` and `after` forward, `last` and `before` backward, one page size at a time.

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

List all comments (notes) on a GitLab group epic via the Work Items GraphQL API. Supports ordering and cursor-based pagination. The work item notes widget pages forward only: it rejects `last` and `before`, so the response carries `has_next_page` and `end_cursor` alone.

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

List all discussion threads on a GitLab group epic via the Work Items GraphQL API. Supports cursor-based pagination. The work item notes widget pages forward only: it rejects `last` and `before`, so the response carries `has_next_page` and `end_cursor` alone.

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
- [GraphQL Integration](../../concepts/graphql.md)
- [ADR-0009: Progressive GraphQL Migration](../../development/adr/adr-0009-progressive-graphql-migration.md)
- [Groups — Tool Reference](groups.md)
