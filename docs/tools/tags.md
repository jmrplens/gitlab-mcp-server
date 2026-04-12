# Tags — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Tags
> **Individual tools**: 9
> **Meta-tool**: `gitlab_tag` (when `META_TOOLS=true`, default)
> **GitLab API**: [Tags API](https://docs.gitlab.com/ee/api/tags.html), [Protected Tags API](https://docs.gitlab.com/ee/api/protected_tags.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The tags domain covers Git tag management in GitLab projects: creating, retrieving, listing, and deleting tags, verifying tag signatures, and managing protected tag rules with configurable access levels.

When `META_TOOLS=true` (the default), all 9 individual tools below are consolidated into a single `gitlab_tag` meta-tool that dispatches by `action` parameter.

### Common Questions

> "List all tags in project 42"
> "Create a tag v2.0.0 on the main branch"
> "Delete tag v1.0-beta"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Core Tag Operations

### `gitlab_tag_get`

Retrieve detailed information about a single Git tag. Returns tag name, target commit SHA, annotation message, and protection status.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_tag_list`

List Git tags in a GitLab project. Supports search by name pattern, ordering by name/updated/version, and sort direction (asc/desc). Returns paginated results.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_tag_create`

Create a Git tag in a GitLab project pointing to a ref (branch, tag, or SHA). Optionally include an annotation message to create an annotated tag, and release notes in Markdown.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_tag_delete`

Delete a Git tag from a GitLab project. If a release is associated with the tag, the release is also removed.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Associated releases will also be removed.

### `gitlab_tag_get_signature`

Get the X.509 signature of a tag. Returns signature type, verification status, and certificate details.

| Annotation | **Read** |
| ---------- | -------- |

---

## Protected Tags

### `gitlab_tag_list_protected`

List protected tags in a GitLab project with their create access levels. Returns paginated results.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_tag_get_protected`

Get a single protected tag by name, including its create access levels.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_tag_protect`

Protect a repository tag or wildcard pattern. Optionally set create access level or granular permissions (user, group, deploy key).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_tag_unprotect`

Remove protection from a repository tag. The tag itself is not deleted.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Removes the protection rule; the tag remains.

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_tag_get` | Core Tag | Read |
| 2 | `gitlab_tag_list` | Core Tag | Read |
| 3 | `gitlab_tag_create` | Core Tag | Create |
| 4 | `gitlab_tag_delete` | Core Tag | Delete |
| 5 | `gitlab_tag_get_signature` | Core Tag | Read |
| 6 | `gitlab_tag_list_protected` | Protected Tags | Read |
| 7 | `gitlab_tag_get_protected` | Protected Tags | Read |
| 8 | `gitlab_tag_protect` | Protected Tags | Create |
| 9 | `gitlab_tag_unprotect` | Protected Tags | Delete |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_tag_delete` — deletes a tag and any associated release
- `gitlab_tag_unprotect` — removes protection rules from a tag

---

## Related

- [GitLab Tags API](https://docs.gitlab.com/ee/api/tags.html)
- [GitLab Protected Tags API](https://docs.gitlab.com/ee/api/protected_tags.html)
