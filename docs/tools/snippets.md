# Snippets — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Snippets
> **Individual tools**: 26
> **Meta-tools**: `gitlab_snippet`, `gitlab_project_snippet`, `gitlab_snippet_discussion` (when `META_TOOLS=true`, default)
> **GitLab API**: [Snippets API](https://docs.gitlab.com/ee/api/snippets.html), [Project Snippets API](https://docs.gitlab.com/ee/api/project_snippets.html), [Snippet Discussions API](https://docs.gitlab.com/ee/api/discussions.html#snippets), [Notes API — Snippets](https://docs.gitlab.com/ee/api/notes.html#snippets)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The snippets domain covers personal snippets, project snippets, snippet discussion threads, and snippet notes. Personal snippets belong to the authenticated user, while project snippets are scoped to a specific project. Discussion threads enable threaded conversations on project snippets. Snippet notes are individual comments (non-threaded) on project snippets.

When `META_TOOLS=true` (the default), the 26 individual tools below are consolidated into three meta-tools: `gitlab_snippet` (9 actions), `gitlab_project_snippet` (11 actions including notes), and `gitlab_snippet_discussion` (6 actions).

### Common Questions

> "List my snippets"
> "Create a snippet with this code"
> "Show discussions on snippet 10"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Personal Snippets

### `gitlab_snippet_list`

List all snippets for the current authenticated user.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_snippet_list_all`

List all snippets across the GitLab instance (admin endpoint).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_snippet_get`

Get a single personal snippet by ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_snippet_content`

Get the raw content of a personal snippet.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_snippet_file_content`

Get the raw content of a specific file in a snippet by ref and filename.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_snippet_create`

Create a new personal snippet. Use 'files' for multi-file snippets or 'file_name'+'content' for single-file.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_snippet_update`

Update an existing personal snippet.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_snippet_delete`

Delete a personal snippet. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Deletion cannot be undone.

### `gitlab_snippet_explore`

List all public snippets on the GitLab instance.

| Annotation | **Read** |
| ---------- | -------- |

---

## Project Snippets

### `gitlab_project_snippet_list`

List snippets for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_snippet_get`

Get a single project snippet by ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_snippet_content`

Get the raw content of a project snippet.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_snippet_create`

Create a new snippet in a GitLab project. Use 'files' for multi-file snippets or 'file_name'+'content' for single-file.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_snippet_update`

Update an existing project snippet.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_snippet_delete`

Delete a project snippet. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Deletion cannot be undone.

---

## Snippet Discussions

### `gitlab_list_snippet_discussions`

List discussion threads on a project snippet.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_snippet_discussion`

Get a single discussion thread on a project snippet.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_snippet_discussion`

Create a new discussion thread on a project snippet.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_add_snippet_discussion_note`

Add a reply note to an existing snippet discussion thread.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_update_snippet_discussion_note`

Update an existing note in a snippet discussion thread.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_snippet_discussion_note`

Delete a note from a snippet discussion thread.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Deletion cannot be undone.

---

## Snippet Notes

### `gitlab_snippet_note_list`

List all comments (notes) on a GitLab project snippet. Supports ordering and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_snippet_note_get`

Get a single comment (note) from a GitLab project snippet by its note ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_snippet_note_create`

Add a comment (note) to a GitLab project snippet. Supports Markdown formatting.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_snippet_note_update`

Edit the body text of an existing comment on a GitLab project snippet.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_snippet_note_delete`

Permanently delete a comment from a GitLab project snippet.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Deletion cannot be undone.

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_snippet_list` | Personal Snippets | Read |
| 2 | `gitlab_snippet_list_all` | Personal Snippets | Read |
| 3 | `gitlab_snippet_get` | Personal Snippets | Read |
| 4 | `gitlab_snippet_content` | Personal Snippets | Read |
| 5 | `gitlab_snippet_file_content` | Personal Snippets | Read |
| 6 | `gitlab_snippet_create` | Personal Snippets | Create |
| 7 | `gitlab_snippet_update` | Personal Snippets | Update |
| 8 | `gitlab_snippet_delete` | Personal Snippets | Delete |
| 9 | `gitlab_snippet_explore` | Personal Snippets | Read |
| 10 | `gitlab_project_snippet_list` | Project Snippets | Read |
| 11 | `gitlab_project_snippet_get` | Project Snippets | Read |
| 12 | `gitlab_project_snippet_content` | Project Snippets | Read |
| 13 | `gitlab_project_snippet_create` | Project Snippets | Create |
| 14 | `gitlab_project_snippet_update` | Project Snippets | Update |
| 15 | `gitlab_project_snippet_delete` | Project Snippets | Delete |
| 16 | `gitlab_list_snippet_discussions` | Snippet Discussions | Read |
| 17 | `gitlab_get_snippet_discussion` | Snippet Discussions | Read |
| 18 | `gitlab_create_snippet_discussion` | Snippet Discussions | Create |
| 19 | `gitlab_add_snippet_discussion_note` | Snippet Discussions | Create |
| 20 | `gitlab_update_snippet_discussion_note` | Snippet Discussions | Update |
| 21 | `gitlab_delete_snippet_discussion_note` | Snippet Discussions | Delete |
| 22 | `gitlab_snippet_note_list` | Snippet Notes | Read |
| 23 | `gitlab_snippet_note_get` | Snippet Notes | Read |
| 24 | `gitlab_snippet_note_create` | Snippet Notes | Create |
| 25 | `gitlab_snippet_note_update` | Snippet Notes | Update |
| 26 | `gitlab_snippet_note_delete` | Snippet Notes | Delete |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_snippet_delete` — deletes a personal snippet permanently
- `gitlab_project_snippet_delete` — deletes a project snippet permanently
- `gitlab_delete_snippet_discussion_note` — deletes a discussion note permanently
- `gitlab_snippet_note_delete` — deletes a snippet note permanently

---

## Related

- [GitLab Snippets API](https://docs.gitlab.com/ee/api/snippets.html)
- [GitLab Project Snippets API](https://docs.gitlab.com/ee/api/project_snippets.html)
- [GitLab Discussions API — Snippets](https://docs.gitlab.com/ee/api/discussions.html#snippets)
- [GitLab Notes API — Snippets](https://docs.gitlab.com/ee/api/notes.html#snippets)
