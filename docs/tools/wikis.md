# Wikis — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Wikis
> **Individual tools**: 6
> **Meta-tool**: `gitlab_wiki` (when `META_TOOLS=true`, default)
> **GitLab API**: [Project Wikis API](https://docs.gitlab.com/ee/api/wikis.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The wikis domain covers the full lifecycle of GitLab project wiki pages: listing, retrieving, creating, updating, deleting, and uploading file attachments. Supports Markdown, RDoc, AsciiDoc, and Org formats.

When `META_TOOLS=true` (the default), all 6 individual tools below are consolidated into a single `gitlab_wiki` meta-tool that dispatches by `action` parameter.

### Common Questions

> "List wiki pages for project 42"
> "Create a new wiki page"
> "Show the contents of the Home wiki page"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Wiki Pages

### `gitlab_wiki_list`

List all wiki pages in a GitLab project. Optionally include page content by setting with_content=true. Returns title, slug, format, and encoding for each page.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_wiki_get`

Get a single wiki page by slug. Supports retrieving HTML-rendered content and specific page versions. Use gitlab_wiki_list to discover available page slugs.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_wiki_create`

Create a new wiki page in a GitLab project. Supports Markdown (default), RDoc, AsciiDoc, and Org formats.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_wiki_update`

Update an existing wiki page by slug. Can change the title, content, and format. At least one of title, content, or format must be provided.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_wiki_delete`

Delete a wiki page by slug. This action cannot be undone. Use gitlab_wiki_list to find available page slugs.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Deletion cannot be undone.

---

## Attachments

### `gitlab_wiki_upload_attachment`

Upload a file attachment to a project wiki. Provide file content as base64 or a local file path. Returns the file path and Markdown embed snippet.

| Annotation | **Create** |
| ---------- | ---------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_wiki_list` | Wiki Pages | Read |
| 2 | `gitlab_wiki_get` | Wiki Pages | Read |
| 3 | `gitlab_wiki_create` | Wiki Pages | Create |
| 4 | `gitlab_wiki_update` | Wiki Pages | Update |
| 5 | `gitlab_wiki_delete` | Wiki Pages | Delete |
| 6 | `gitlab_wiki_upload_attachment` | Attachments | Create |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_wiki_delete` — deletes a wiki page permanently

---

## Related

- [GitLab Project Wikis API](https://docs.gitlab.com/ee/api/wikis.html)
