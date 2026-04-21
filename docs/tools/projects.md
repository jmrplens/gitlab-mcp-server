# Projects — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Projects
> **Individual tools**: 42
> **Meta-tool**: `gitlab_project` (when `META_TOOLS=true`, default)
> **GitLab API**: [Projects API](https://docs.gitlab.com/ee/api/projects.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The projects domain covers the full lifecycle of GitLab projects (repositories): creation, retrieval, listing, updating, deletion, forking, starring, archiving, transferring, webhook management, user/group listings, and push rule configuration.

When `META_TOOLS=true` (the default), all 33 individual tools below are consolidated into a single `gitlab_project` meta-tool that dispatches by `action` parameter.

### Common Questions

> "List all my GitLab projects"
> "Create a new project called my-app"
> "Archive the project my-old-app"
> "Who has access to project 42?"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Core CRUD

### `gitlab_project_create`

Create a new GitLab project (repository). Supports setting namespace, visibility (private/internal/public), description, default branch, optional README initialization, merge method, squash option, topics, and feature flags.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_get`

Retrieve detailed metadata for a GitLab project including name, description, visibility, web URL, default branch, and namespace. Accepts numeric project ID or URL-encoded path (e.g. `group/subgroup/project`). Optionally include statistics, license info, or custom attributes.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_list`

List GitLab projects accessible to the authenticated user. Supports filtering by ownership, search term, visibility, archived status, topic, minimum access level, starred, membership, date ranges, and feature flags. Set `include_pending_delete=true` to include projects scheduled for deletion. Returns paginated results.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_update`

Update GitLab project settings such as name, description, visibility, default branch, merge method, squash option, topics, feature flags, CI/CD config, merge templates, and approval settings. Only specified fields are modified; unset fields remain unchanged.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_delete`

Delete a GitLab project. On instances with delayed deletion, the project is marked/scheduled for deletion. Set `permanently_remove=true` with `full_path` to bypass delayed deletion. Use `gitlab_project_restore` to cancel a scheduled deletion.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Permanent removal cannot be undone.

### `gitlab_project_restore`

Restore a GitLab project that has been marked/scheduled for deletion. Returns the restored project details. Use `gitlab_project_list` with `include_pending_delete=true` to discover projects pending deletion.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Fork & Star

### `gitlab_project_fork`

Fork a GitLab project into a new project. Optionally specify target namespace, name, path, description, visibility, branches to include, and MR default target setting.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_star`

Star a GitLab project for the authenticated user. Returns updated project details with incremented star count.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_unstar`

Remove star from a GitLab project for the authenticated user. Returns updated project details with decremented star count.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_list_forks`

List forks of a GitLab project. Supports filtering by ownership, search, visibility, ordering, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_list_starrers`

List users who have starred a project. Supports filtering by search (name or username) and pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Archive & Transfer

### `gitlab_project_archive`

Archive a GitLab project, making it read-only. Archived projects are hidden from the default project list. Returns updated project details.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_unarchive`

Unarchive a GitLab project, restoring it from read-only state. Returns updated project details.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_transfer`

Transfer a GitLab project to a different namespace. Requires the namespace (ID or path) to transfer to. Returns updated project details with new path.

| Annotation | **Update** |
| ---------- | ---------- |

> **Protected**: Requires confirmation prompt before execution.

---

## Languages

### `gitlab_project_languages`

List programming languages used in a GitLab project with their percentages. Returns a list of languages detected in the repository.

| Annotation | **Read** |
| ---------- | -------- |

---

## Webhooks

### `gitlab_project_hook_list`

List webhooks configured for a GitLab project. Returns paginated list with event trigger status for each hook.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_hook_get`

Get details of a specific project webhook including all event trigger settings.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_hook_add`

Add a webhook to a GitLab project. Configure the URL, secret token, SSL verification, and which events trigger the webhook (push, issues, MRs, tags, notes, jobs, pipelines, wiki, deployments, releases, emoji, etc.).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_hook_edit`

Edit an existing project webhook. Update the URL, events, SSL verification, secret token, or other settings.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_hook_delete`

Delete a webhook from a GitLab project.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_project_hook_test`

Trigger a test event for a project webhook. Sends a sample payload for the specified event type (push_events, issues_events, merge_requests_events, etc.).

| Annotation | **Update** |
| ---------- | ---------- |

---

## User & Group Listings

### `gitlab_project_list_user_projects`

List projects owned by a specific user. Accepts user ID or username. Supports filtering by search, visibility, archived status, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_list_users`

List users who are members of a project. Supports filtering by search (name or username) and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_list_groups`

List ancestor groups of a project. Supports filtering by search, shared groups, minimum access level, skip_groups, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_share_with_group`

Share a project with a group, granting the specified access level. Optionally set an expiration date (YYYY-MM-DD). Access levels: 10=Guest, 20=Reporter, 30=Developer, 40=Maintainer.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_delete_shared_group`

Remove a shared group from a project, revoking the group's access.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_project_list_invited_groups`

List groups that have been invited/shared to a project. Supports filtering by search, minimum access level, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_list_user_contributed`

List projects that a specific user has contributed to. Supports filtering by search, visibility, archived status, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_list_user_starred`

List projects that a specific user has starred. Supports filtering by search, visibility, archived status, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Push Rules

### `gitlab_project_get_push_rules`

Get the push rule configuration for a project (commit message, branch name, file size restrictions, etc.).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_add_push_rule`

Add push rule configuration to a project. Enforce commit message format, branch naming, file size limits, secret detection, and signed commits.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_edit_push_rule`

Modify the push rule configuration of a project. Update commit message, branch name, file restrictions, or signing requirements.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_delete_push_rule`

Delete the push rule configuration from a project. This removes all push restrictions (commit format, branch naming, file size, etc.).

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Uploads

### `gitlab_project_upload`

Upload a file to a GitLab project's Markdown uploads area. Provide either `file_path` (absolute local path) or `content_base64` (base64-encoded content). Returns a Markdown embed string for use in MR descriptions, notes, or discussion bodies.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_upload_list`

List all file uploads (Markdown attachments) for a GitLab project. Returns upload ID, filename, size, and creation date.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_upload_delete`

Delete a file upload (Markdown attachment) from a GitLab project by upload ID. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Import / Export

### `gitlab_schedule_project_export`

Schedule an asynchronous export of a project. Use `gitlab_get_project_export_status` to check progress.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_get_project_export_status`

Get the export status of a project, including download links when the export is finished.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_download_project_export`

Download the finished export archive of a project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_import_project_from_file`

Import a project from an export archive file. Accepts either a local `file_path` or `content` (base64-encoded).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_get_project_import_status`

Get the import status of a project.

| Annotation | **Read** |
| ---------- | -------- |

---

## Iterations

### `gitlab_list_project_iterations`

List iterations for a project. Iterations provide time-boxed planning periods. Supports filtering by state, search, and inclusion of ancestor iterations.

| Annotation | **Read** |
| ---------- | -------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_project_create` | Core CRUD | Create |
| 2 | `gitlab_project_get` | Core CRUD | Read |
| 3 | `gitlab_project_list` | Core CRUD | Read |
| 4 | `gitlab_project_update` | Core CRUD | Update |
| 5 | `gitlab_project_delete` | Core CRUD | Delete |
| 6 | `gitlab_project_restore` | Core CRUD | Update |
| 7 | `gitlab_project_fork` | Fork & Star | Create |
| 8 | `gitlab_project_star` | Fork & Star | Create |
| 9 | `gitlab_project_unstar` | Fork & Star | Update |
| 10 | `gitlab_project_list_forks` | Fork & Star | Read |
| 11 | `gitlab_project_list_starrers` | Fork & Star | Read |
| 12 | `gitlab_project_archive` | Archive & Transfer | Update |
| 13 | `gitlab_project_unarchive` | Archive & Transfer | Update |
| 14 | `gitlab_project_transfer` | Archive & Transfer | Update |
| 15 | `gitlab_project_languages` | Languages | Read |
| 16 | `gitlab_project_hook_list` | Webhooks | Read |
| 17 | `gitlab_project_hook_get` | Webhooks | Read |
| 18 | `gitlab_project_hook_add` | Webhooks | Create |
| 19 | `gitlab_project_hook_edit` | Webhooks | Update |
| 20 | `gitlab_project_hook_delete` | Webhooks | Delete |
| 21 | `gitlab_project_hook_test` | Webhooks | Update |
| 22 | `gitlab_project_list_user_projects` | User & Group | Read |
| 23 | `gitlab_project_list_users` | User & Group | Read |
| 24 | `gitlab_project_list_groups` | User & Group | Read |
| 25 | `gitlab_project_share_with_group` | User & Group | Create |
| 26 | `gitlab_project_delete_shared_group` | User & Group | Delete |
| 27 | `gitlab_project_list_invited_groups` | User & Group | Read |
| 28 | `gitlab_project_list_user_contributed` | User & Group | Read |
| 29 | `gitlab_project_list_user_starred` | User & Group | Read |
| 30 | `gitlab_project_get_push_rules` | Push Rules | Read |
| 31 | `gitlab_project_add_push_rule` | Push Rules | Create |
| 32 | `gitlab_project_edit_push_rule` | Push Rules | Update |
| 33 | `gitlab_project_delete_push_rule` | Push Rules | Delete |
| 34 | `gitlab_project_upload` | Uploads | Create |
| 35 | `gitlab_project_upload_list` | Uploads | Read |
| 36 | `gitlab_project_upload_delete` | Uploads | Delete |
| 37 | `gitlab_schedule_project_export` | Import / Export | Create |
| 38 | `gitlab_get_project_export_status` | Import / Export | Read |
| 39 | `gitlab_download_project_export` | Import / Export | Read |
| 40 | `gitlab_import_project_from_file` | Import / Export | Create |
| 41 | `gitlab_get_project_import_status` | Import / Export | Read |
| 42 | `gitlab_list_project_iterations` | Iterations | Read |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_project_delete` — deletes a project (scheduled or permanent)
- `gitlab_project_transfer` — transfers a project to a different namespace
- `gitlab_project_hook_delete` — removes a webhook
- `gitlab_project_delete_shared_group` — revokes group access
- `gitlab_project_delete_push_rule` — removes all push restrictions
- `gitlab_project_upload_delete` — deletes a file upload from a project

---

## Related

- [GitLab Projects API](https://docs.gitlab.com/ee/api/projects.html)
- [GitLab Project Webhooks API](https://docs.gitlab.com/ee/api/project_hooks.html)
- [GitLab Push Rules API](https://docs.gitlab.com/ee/api/project_push_rules.html)
- [GitLab Uploads API](https://docs.gitlab.com/ee/api/project_uploads.html)
- [GitLab Project Import/Export API](https://docs.gitlab.com/ee/api/project_import_export.html)
- [GitLab Iterations API](https://docs.gitlab.com/ee/api/project_iterations.html)
