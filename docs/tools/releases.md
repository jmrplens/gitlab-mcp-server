# Releases — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Releases & Release Links
> **Individual tools**: 12
> **Meta-tool**: `gitlab_release` (when `META_TOOLS=true`, default)
> **GitLab API**: [Releases API](https://docs.gitlab.com/ee/api/releases/) · [Release Links API](https://docs.gitlab.com/ee/api/releases/links.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The releases domain covers the full lifecycle of GitLab releases and their associated asset links: creating releases for existing tags, retrieving release details, listing releases, updating metadata, deleting releases, and managing asset links attached to releases.

When `META_TOOLS=true` (the default), all 11 individual tools below are consolidated into a single `gitlab_release` meta-tool that dispatches by `action` parameter.

### Common Questions

> "List all releases for project 42"
> "Create a release for tag v2.0.0"
> "What changed between v1.0 and v2.0?"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Releases

### `gitlab_release_create`

Create a GitLab release associated with an existing Git tag. Includes release title, Markdown description, and optional milestones. The tag must exist before creating the release.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_release_get`

Retrieve detailed information about a specific GitLab release by its tag name, including title, description, author, creation date, and associated assets.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_release_list`

List all releases for a GitLab project ordered by release date. Returns paginated results including each release's metadata, tag, and asset links.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_release_latest`

Get the latest release for a GitLab project. Returns the most recently created release without needing to know the tag name.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_release_update`

Update an existing GitLab release's title, description, milestones, or released date. Identified by project and tag name. Only specified fields are changed.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_release_delete`

Delete a GitLab release. The underlying Git tag is preserved and not deleted.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. The Git tag is preserved.

---

## Release Links

### `gitlab_release_link_create`

Add an asset link to a GitLab release. Supports link types: runbook, package, image, or other. Links appear in the release's assets section.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_release_link_get`

Get details of a specific release asset link by its ID, including name, URL, type, and whether it is external.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_release_link_list`

List all asset links attached to a specific GitLab release identified by tag name. Returns link names, URLs, types, and IDs.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_release_link_update`

Update an existing release asset link. Can change name, URL, filepath, direct asset path, or link type. Only specified fields are changed.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_release_link_delete`

Remove an asset link from a GitLab release by its link ID.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Group Releases

### `gitlab_group_release_list`

List releases across all projects in a GitLab group. Returns paginated list of releases with tag, name, dates, and author.

| Annotation | **Read** |
| ---------- | -------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_release_create` | Releases | Create |
| 2 | `gitlab_release_get` | Releases | Read |
| 3 | `gitlab_release_list` | Releases | Read |
| 4 | `gitlab_release_latest` | Releases | Read |
| 5 | `gitlab_release_update` | Releases | Update |
| 6 | `gitlab_release_delete` | Releases | Delete |
| 7 | `gitlab_release_link_create` | Release Links | Create |
| 8 | `gitlab_release_link_get` | Release Links | Read |
| 9 | `gitlab_release_link_list` | Release Links | Read |
| 10 | `gitlab_release_link_update` | Release Links | Update |
| 11 | `gitlab_release_link_delete` | Release Links | Delete |
| 12 | `gitlab_group_release_list` | Group Releases | Read |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_release_delete` — deletes a release (Git tag is preserved)
- `gitlab_release_link_delete` — removes an asset link from a release

---

## Related

- [GitLab Releases API](https://docs.gitlab.com/ee/api/releases/)
- [GitLab Release Links API](https://docs.gitlab.com/ee/api/releases/links.html)
- [GitLab Group Releases API](https://docs.gitlab.com/ee/api/group_releases.html)
