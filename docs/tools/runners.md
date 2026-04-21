# Runners — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Runners & Resource Groups
> **Individual tools**: 22
> **Meta-tools**: `gitlab_runner`, `gitlab_resource_group` (when `META_TOOLS=true`, default)
> **GitLab API**: [Runners API](https://docs.gitlab.com/ee/api/runners.html), [Resource Groups API](https://docs.gitlab.com/ee/api/resource_groups.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The runners domain covers CI/CD runner management (listing, registration, configuration, removal, token management) and resource groups (listing, editing process modes, viewing upcoming jobs). Runners can be scoped to instances, groups, or projects.

When `META_TOOLS=true` (the default), the 22 individual tools below are consolidated into two meta-tools: `gitlab_runner` (18 actions) and `gitlab_resource_group` (4 actions).

### Common Questions

> "List available runners"
> "Show details of runner 5"
> "Which projects use runner 5?"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Runner Management

### `gitlab_runner_list`

List owned CI/CD runners. Filter by type (instance_type, group_type, project_type), status (online, offline, stale, never_contacted), paused state, and tags.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_runner_list_all`

List all CI/CD runners in the GitLab instance (admin). Filter by type, status, paused state, and tags.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_runner_get`

Get detailed information about a specific CI/CD runner by its ID. Returns description, status, tags, access level, projects, and groups.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_runner_update`

Update a CI/CD runner's configuration. Modify description, paused state, tags, access level, maximum timeout, and maintenance note.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_runner_remove`

Remove a CI/CD runner by its ID. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Removal cannot be undone.

### `gitlab_runner_jobs`

List jobs processed by a specific CI/CD runner. Filter by status (running, success, failed, canceled). Supports sorting and pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Project & Group Runners

### `gitlab_runner_list_project`

List CI/CD runners available in a specific project. Filter by type, status, and tags.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_runner_enable_project`

Assign an existing CI/CD runner to a project. Requires project_id and runner_id.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_runner_disable_project`

Remove a CI/CD runner from a project. The runner itself is not deleted.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Removes the runner assignment from the project.

### `gitlab_runner_list_group`

List CI/CD runners available in a specific group. Filter by type, status, and tags.

| Annotation | **Read** |
| ---------- | -------- |

---

## Runner Registration & Tokens

### `gitlab_runner_register`

Register a new CI/CD runner with a registration token. Optionally set description, tags, access level, and timeout.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_runner_delete_registered`

Delete a registered CI/CD runner by its ID. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Deletion cannot be undone.

### `gitlab_runner_delete_by_token`

Delete a registered CI/CD runner using its authentication token. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Deletion cannot be undone.

### `gitlab_runner_verify`

Verify a CI/CD runner authentication token. Returns success if the token is valid.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_runner_reset_token`

Reset the authentication token for a CI/CD runner. Returns the new token and expiry.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_runner_reset_instance_reg_token`

Reset the instance-level runner registration token. Deprecated: scheduled for removal in GitLab 20.0.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_runner_reset_group_reg_token`

Reset a group's runner registration token. Deprecated: scheduled for removal in GitLab 20.0.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_runner_reset_project_reg_token`

Reset a project's runner registration token. Deprecated: scheduled for removal in GitLab 20.0.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Resource Groups

### `gitlab_list_resource_groups`

List resource groups for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_resource_group`

Get details of a resource group.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_edit_resource_group`

Edit a resource group process mode.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_list_resource_group_upcoming_jobs`

List upcoming jobs for a resource group.

| Annotation | **Read** |
| ---------- | -------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_runner_list` | Runner Management | Read |
| 2 | `gitlab_runner_list_all` | Runner Management | Read |
| 3 | `gitlab_runner_get` | Runner Management | Read |
| 4 | `gitlab_runner_update` | Runner Management | Update |
| 5 | `gitlab_runner_remove` | Runner Management | Delete |
| 6 | `gitlab_runner_jobs` | Runner Management | Read |
| 7 | `gitlab_runner_list_project` | Project & Group | Read |
| 8 | `gitlab_runner_enable_project` | Project & Group | Create |
| 9 | `gitlab_runner_disable_project` | Project & Group | Delete |
| 10 | `gitlab_runner_list_group` | Project & Group | Read |
| 11 | `gitlab_runner_register` | Registration & Tokens | Create |
| 12 | `gitlab_runner_delete_registered` | Registration & Tokens | Delete |
| 13 | `gitlab_runner_delete_by_token` | Registration & Tokens | Delete |
| 14 | `gitlab_runner_verify` | Registration & Tokens | Read |
| 15 | `gitlab_runner_reset_token` | Registration & Tokens | Update |
| 16 | `gitlab_runner_reset_instance_reg_token` | Registration & Tokens | Update |
| 17 | `gitlab_runner_reset_group_reg_token` | Registration & Tokens | Update |
| 18 | `gitlab_runner_reset_project_reg_token` | Registration & Tokens | Update |
| 19 | `gitlab_list_resource_groups` | Resource Groups | Read |
| 20 | `gitlab_get_resource_group` | Resource Groups | Read |
| 21 | `gitlab_edit_resource_group` | Resource Groups | Update |
| 22 | `gitlab_list_resource_group_upcoming_jobs` | Resource Groups | Read |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_runner_remove` — removes a CI/CD runner permanently
- `gitlab_runner_disable_project` — removes a runner assignment from a project
- `gitlab_runner_delete_registered` — deletes a registered runner by ID
- `gitlab_runner_delete_by_token` — deletes a registered runner by authentication token

---

## Related

- [GitLab Runners API](https://docs.gitlab.com/ee/api/runners.html)
- [GitLab Resource Groups API](https://docs.gitlab.com/ee/api/resource_groups.html)
