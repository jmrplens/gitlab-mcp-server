# Mirrors — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Project Mirrors
> **Individual tools**: 7
> **Meta-tool**: Routes inside `gitlab_project` (enterprise-only, requires the Enterprise/Premium catalog)
> **GitLab API**: [Remote Mirrors API](https://docs.gitlab.com/ee/api/remote_mirrors.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The mirrors domain covers remote mirror management for GitLab projects: listing, retrieving, creating, editing, deleting mirrors, forcing push synchronization, and retrieving SSH public keys for authentication.

When `META_TOOLS=true` (the default) and the Enterprise/Premium catalog is enabled, the 7 individual tools below are available as enterprise-only routes inside the `gitlab_project` meta-tool.

### Common Questions

> "List all remote mirrors for project 42"
> "Add a push mirror to an external repository"
> "Force sync a mirror now"
> "Get the SSH public key for a mirror"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Mirror Operations

### `gitlab_list_project_mirrors`

List all remote mirrors configured for a project. Returns paginated results with mirror URL, direction, status, and synchronization timestamps.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_project_mirror`

Get details of a specific remote mirror by ID. Returns the mirror URL, enabled status, direction (push/pull), authentication method, and last sync timestamps.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_project_mirror_public_key`

Retrieve the SSH public key for a specific remote mirror. This key is used for SSH-based authentication when pushing to the remote repository.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_add_project_mirror`

Add a new remote mirror to a project. Specify the target URL and optionally configure direction, authentication method, and whether to sync only protected branches.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_edit_project_mirror`

Update an existing remote mirror configuration. Modify the URL, enabled status, authentication method, or protected branches setting.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_project_mirror`

Delete a remote mirror from a project. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Deletion cannot be undone.

### `gitlab_force_push_mirror_update`

Trigger an immediate push synchronization for a remote mirror. This forces the mirror to sync now instead of waiting for the next scheduled sync.

| Annotation | **Create** |
| ---------- | ---------- |

> **Note**: Force push uses a POST action (non-idempotent) to trigger synchronization.
