# MR Review — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: MR Review
> **Individual tools**: 23
> **Meta-tool**: `gitlab_mr_review` (when `META_TOOLS=true`, default)
> **GitLab API**: [MR Notes API](https://docs.gitlab.com/ee/api/notes.html#merge-requests), [MR Discussions API](https://docs.gitlab.com/ee/api/discussions.html#merge-requests), [MR Draft Notes API](https://docs.gitlab.com/ee/api/draft_notes.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The MR review domain covers all comment and review operations on GitLab merge requests: top-level notes (comments), threaded discussions (including inline diff comments), and draft notes (pending review comments that remain private until published).

When `META_TOOLS=true` (the default), all 19 individual tools below are consolidated into a single `gitlab_mr_review` meta-tool that dispatches by `action` parameter.

### Common Questions

> "Show me the comments on MR !15"
> "Add a comment to merge request !15"
> "What are the code changes in MR !15?"
> "Show draft notes on MR !20"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Notes

### `gitlab_mr_note_create`

Add a comment (note) to a GitLab merge request. The comment appears in the merge request's activity timeline as a top-level note.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_mr_notes_list`

List all comments (notes) on a GitLab merge request ordered by creation date. Includes both user comments and system-generated notes (status changes, label updates). Returns paginated results.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_note_get`

Get a single comment (note) from a GitLab merge request by its note ID, including author, timestamps, resolution status, and body.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_note_update`

Edit the body text of an existing comment on a GitLab merge request. Only the note author or a project maintainer can update a note.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_mr_note_delete`

Permanently delete a comment from a GitLab merge request. Only the note author or a project maintainer can delete a note.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Discussions

### `gitlab_mr_discussion_create`

Start a new threaded discussion on a GitLab merge request. Can be a general discussion or an inline diff comment positioned on a specific file, line, and diff ref (SHA).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_mr_discussion_list`

List all discussion threads on a GitLab merge request including inline diff comments and general discussions. Each thread contains its notes and resolution status. Returns paginated results.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_discussion_get`

Get a single discussion thread from a GitLab merge request by its discussion ID, including all notes in the thread.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_discussion_resolve`

Resolve or unresolve a discussion thread on a GitLab merge request. Resolved discussions are collapsed in the UI to reduce review noise.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_mr_discussion_reply`

Add a reply to an existing discussion thread on a GitLab merge request. The reply appears nested under the original discussion note.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_mr_discussion_note_update`

Update the body or resolved status of a note within a merge request discussion thread. You can modify the text, change resolution status, or both.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_mr_discussion_note_delete`

Delete a note from a merge request discussion thread. Only the note author or project maintainers can delete notes.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Draft Notes

### `gitlab_mr_draft_note_list`

List all draft notes (pending review comments) on a GitLab merge request. Draft notes are only visible to the author until published. Supports pagination and sorting.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_draft_note_get`

Get a single draft note from a GitLab merge request by its ID. Returns the note body, author, and associated commit/discussion details.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_draft_note_create`

Create a new draft note (pending review comment) on a GitLab merge request. Draft notes stay private until published. Can be attached to a specific commit or reply to a discussion.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_mr_draft_note_update`

Update the body text of an existing draft note on a GitLab merge request. Only the draft author can update it.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_mr_draft_note_delete`

Permanently delete a draft note from a GitLab merge request. Only the draft author can delete it.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_mr_draft_note_publish`

Publish a single draft note on a GitLab merge request, making it visible to all participants. This action cannot be undone.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_mr_draft_note_publish_all`

Publish all pending draft notes on a GitLab merge request at once, making them visible to all participants. This action cannot be undone.

| Annotation | **Update** |
| ---------- | ---------- |

---

## MR Approval Settings

### `gitlab_get_group_mr_approval_settings`

Get group-level merge request approval settings: author/committer approval, approver list overrides, approval retention on push, and reauthentication. Settings may be locked or inherited. Requires GitLab Premium.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_update_group_mr_approval_settings`

Update group-level merge request approval settings. Only include settings you want to change. Requires GitLab Premium.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_get_project_mr_approval_settings`

Get project-level merge request approval settings: author/committer approval, approver list overrides, approval retention on push, selective code owner removals, and reauthentication. Requires GitLab Premium.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_update_project_mr_approval_settings`

Update project-level merge request approval settings. Only include settings you want to change. Requires GitLab Premium.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_mr_note_create` | Notes | Create |
| 2 | `gitlab_mr_notes_list` | Notes | Read |
| 3 | `gitlab_mr_note_get` | Notes | Read |
| 4 | `gitlab_mr_note_update` | Notes | Update |
| 5 | `gitlab_mr_note_delete` | Notes | Delete |
| 6 | `gitlab_mr_discussion_create` | Discussions | Create |
| 7 | `gitlab_mr_discussion_list` | Discussions | Read |
| 8 | `gitlab_mr_discussion_get` | Discussions | Read |
| 9 | `gitlab_mr_discussion_resolve` | Discussions | Update |
| 10 | `gitlab_mr_discussion_reply` | Discussions | Create |
| 11 | `gitlab_mr_discussion_note_update` | Discussions | Update |
| 12 | `gitlab_mr_discussion_note_delete` | Discussions | Delete |
| 13 | `gitlab_mr_draft_note_list` | Draft Notes | Read |
| 14 | `gitlab_mr_draft_note_get` | Draft Notes | Read |
| 15 | `gitlab_mr_draft_note_create` | Draft Notes | Create |
| 16 | `gitlab_mr_draft_note_update` | Draft Notes | Update |
| 17 | `gitlab_mr_draft_note_delete` | Draft Notes | Delete |
| 18 | `gitlab_mr_draft_note_publish` | Draft Notes | Update |
| 19 | `gitlab_mr_draft_note_publish_all` | Draft Notes | Update |
| 20 | `gitlab_get_group_mr_approval_settings` | Approval Settings | Read |
| 21 | `gitlab_update_group_mr_approval_settings` | Approval Settings | Update |
| 22 | `gitlab_get_project_mr_approval_settings` | Approval Settings | Read |
| 23 | `gitlab_update_project_mr_approval_settings` | Approval Settings | Update |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_mr_note_delete` — permanently deletes a comment from a merge request
- `gitlab_mr_discussion_note_delete` — deletes a note from a discussion thread
- `gitlab_mr_draft_note_delete` — permanently deletes a draft note

---

## Related

- [GitLab Merge Request Notes API](https://docs.gitlab.com/ee/api/notes.html#merge-requests)
- [GitLab Merge Request Discussions API](https://docs.gitlab.com/ee/api/discussions.html#merge-requests)
- [GitLab Draft Notes API](https://docs.gitlab.com/ee/api/draft_notes.html)
- [GitLab MR Approval Settings API](https://docs.gitlab.com/ee/api/group_level_mr_approvals.html)
