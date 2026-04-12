# Search — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Search
> **Individual tools**: 10
> **Meta-tool**: `gitlab_search` (when `META_TOOLS=true`, default)
> **GitLab API**: [Search API](https://docs.gitlab.com/ee/api/search.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The search domain provides keyword-based search across GitLab resources: code (blobs), merge requests, issues, commits, milestones, notes (comments), projects, snippets, users, and wiki pages. Each search tool supports scoping by project, group, or global, with paginated results.

When `META_TOOLS=true` (the default), all 10 individual tools below are consolidated into a single `gitlab_search` meta-tool that dispatches by `action` parameter.

### Common Questions

> "Search for 'login' across all projects"
> "Find merge requests mentioning 'bug fix'"
> "Search for issues with the label 'critical'"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |

All search tools are read-only.

---

## Code

### `gitlab_search_code`

Search for code (blobs) in GitLab. Searches within a project (project_id), a group (group_id), or globally. Returns matching file name, path, ref, and a content snippet with pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Merge Requests

### `gitlab_search_merge_requests`

Search for merge requests by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching merge requests with title, state, author, labels, and web URL with pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Issues

### `gitlab_search_issues`

Search for issues by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching issues with title, state, labels, assignees, and web URL with pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Commits

### `gitlab_search_commits`

Search for commits by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching commits with ID, title, author, date, and web URL with pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Milestones

### `gitlab_search_milestones`

Search for milestones by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching milestones with title, state, dates, and web URL with pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Notes

### `gitlab_search_notes`

Search for notes (comments) within a GitLab project by keyword. Returns matching notes with body, author, noteable type/ID, and timestamps with pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Projects

### `gitlab_search_projects`

Search for projects by keyword. Searches within a group (group_id) or globally. Returns matching projects with name, path, visibility, and web URL with pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Snippets

### `gitlab_search_snippets`

Search for snippet titles globally in GitLab. Returns matching snippets with title, file name, description, author, and web URL with pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Users

### `gitlab_search_users`

Search for users by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching users with username, name, state, and web URL with pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Wiki

### `gitlab_search_wiki`

Search for wiki blobs by keyword. Searches within a project (project_id), a group (group_id), or globally. Returns matching wiki pages with title, slug, content, and format with pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_search_code` | Code | Read |
| 2 | `gitlab_search_merge_requests` | Merge Requests | Read |
| 3 | `gitlab_search_issues` | Issues | Read |
| 4 | `gitlab_search_commits` | Commits | Read |
| 5 | `gitlab_search_milestones` | Milestones | Read |
| 6 | `gitlab_search_notes` | Notes | Read |
| 7 | `gitlab_search_projects` | Projects | Read |
| 8 | `gitlab_search_snippets` | Snippets | Read |
| 9 | `gitlab_search_users` | Users | Read |
| 10 | `gitlab_search_wiki` | Wiki | Read |

### Destructive Tools (Require Confirmation)

None — all search tools are read-only.

---

## Related

- [GitLab Search API](https://docs.gitlab.com/ee/api/search.html)
- [GitLab Advanced Search](https://docs.gitlab.com/ee/user/search/advanced_search.html)
