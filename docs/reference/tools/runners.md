# Runners — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Runners
> **Individual tools**: 34
> **Meta-tool**: `gitlab_runner` (`TOOL_SURFACE=meta` catalog, 34 actions)
> **Dynamic IDs**: `runner.*` (default surface, via `gitlab_execute_action`)
> **GitLab API**: [Runners API](https://docs.gitlab.com/ee/api/runners.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The runners domain covers CI/CD runner management (listing, registration, configuration, removal, token management, manager inspection) and the admin-only runner controller control plane for Ultimate-tier GitLab instances. Runners can be scoped to instances, groups, or projects. The runner controller API is experimental and admin-only.

On the default dynamic surface, these operations are the `runner.*` entries of the canonical action catalog: find them with `gitlab_find_action` and run them with `gitlab_execute_action` by `domain.action` ID. With `TOOL_SURFACE=individual`, each is the tool named in the tables below.

With `TOOL_SURFACE=meta`, the 34 individual tools below are consolidated into a single `gitlab_runner` meta-tool that dispatches by `action` parameter.

### Common Questions

> "List available runners"
> "Show details of runner 5"
> "Which projects use runner 5?"
> "List the managers behind runner 5"
> "Register a new runner controller"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description                                    |
| ---------- | :------: | :---------: | :--------: | ---------------------------------------------- |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation                       |
| **Create** |    —     |     No      |     —      | Creates a new resource                         |
| **Update** |    —     |     No      |    Yes     | Modifies an existing resource                  |
| **Delete** |    —     |     Yes     |    Yes     | Destroys a resource; protected by confirmation |

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

### `gitlab_runner_list_managers`

List the managers (individual runner processes/hosts) of a runner by `runner_id`. Returns each manager's system id, version, revision, platform, architecture, status, and contact IP.

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

## Runner Controllers (Ultimate)

> **Tier**: Ultimate. Admin-only, experimental GitLab API for the agentic runner control plane.

### `gitlab_runner_controller_list`

List every registered runner controller on the instance (admin-only, experimental API). Supports offset and keyset pagination via `order_by`, `page`, `page_token`, `pagination` (offset|keyset), `per_page` (1–100), and `sort` (asc|desc).

| Annotation | **Read** |
| ---------- | -------- |
| Tier       | Ultimate |

### `gitlab_runner_controller_get`

Get one runner controller by its `controller_id`, including its live connection status. Returns id, description, state, connected flag, and created/updated timestamps.

| Annotation | **Read** |
| ---------- | -------- |
| Tier       | Ultimate |

### `gitlab_runner_controller_create`

Register a new runner controller with optional `description` and `state` (enabled, disabled, or dry_run). Returns the created controller with id, description, state, and timestamps.

| Annotation | **Create** |
| ---------- | ---------- |
| Tier       | Ultimate   |

### `gitlab_runner_controller_update`

Update an existing runner controller's `description` or `state` (enabled, disabled, or dry_run) by `controller_id`. Returns the updated controller with id, description, state, and timestamps.

| Annotation | **Update** |
| ---------- | ---------- |
| Tier       | Ultimate   |

### `gitlab_runner_controller_delete`

Permanently remove a runner controller by `controller_id`. Destructive and irreversible; verify the controller_id with `gitlab_runner_controller_list` first.

| Annotation | **Delete** |
| ---------- | ---------- |
| Tier       | Ultimate   |

> **Destructive**: Permanently deletes the runner controller.

---

## Runner Controller Scopes (Ultimate)

> **Tier**: Ultimate. Admin-only. Manage which runners a runner controller may operate: the instance-level scope grants the controller the whole shared instance runner fleet, while runner-level scopes pin the controller to specific instance runners.

### `gitlab_runner_controller_scope_list`

List every scope assigned to a runner controller by `controller_id`. Returns instance-level scopings with timestamps and runner-level scopings with runner IDs and timestamps.

| Annotation | **Read** |
| ---------- | -------- |
| Tier       | Ultimate |

### `gitlab_runner_controller_scope_add_instance`

Grant a runner controller the instance-level scope by `controller_id` so it may operate the entire shared instance runner fleet. Returns the created instance-level scoping with created/updated timestamps.

| Annotation | **Create** |
| ---------- | ---------- |
| Tier       | Ultimate   |

### `gitlab_runner_controller_scope_add_runner`

Scope a runner controller to one specific instance runner by `controller_id` and `runner_id`. Returns the created runner-level scoping with the runner ID and created/updated timestamps.

| Annotation | **Create** |
| ---------- | ---------- |
| Tier       | Ultimate   |

### `gitlab_runner_controller_scope_remove_instance`

Revoke a runner controller's instance-level scope by `controller_id`. Destructive; confirm before calling.

| Annotation | **Delete** |
| ---------- | ---------- |
| Tier       | Ultimate   |

> **Destructive**: Removes the instance-level scope from the runner controller.

### `gitlab_runner_controller_scope_remove_runner`

Remove a specific runner from a runner controller's scope by `controller_id` and `runner_id`. Destructive; confirm before calling.

| Annotation | **Delete** |
| ---------- | ---------- |
| Tier       | Ultimate   |

> **Destructive**: Removes the runner-level scope from the runner controller.

---

## Runner Controller Tokens (Ultimate)

> **Tier**: Ultimate. Admin-only. Manage authentication tokens issued for a runner controller. The secret token value is returned only once at creation or rotation.

### `gitlab_runner_controller_token_list`

List every authentication token issued for a runner controller by `controller_id`. Supports pagination via `order_by`, `page`, `page_token`, `pagination` (offset|keyset), `per_page` (1–100), and `sort` (asc|desc). Returns each token's id, runner controller id, description, last-used time, and timestamps.

| Annotation | **Read** |
| ---------- | -------- |
| Tier       | Ultimate |

### `gitlab_runner_controller_token_get`

Get one runner controller token by `controller_id` and `token_id`. Returns the token's id, runner controller id, description, last-used time, and timestamps.

| Annotation | **Read** |
| ---------- | -------- |
| Tier       | Ultimate |

### `gitlab_runner_controller_token_create`

Create a new authentication token for a runner controller by `controller_id` with optional `description`. Returns the new token including its one-time secret value, id, runner controller id, and description.

| Annotation | **Create** |
| ---------- | ---------- |
| Tier       | Ultimate   |

### `gitlab_runner_controller_token_rotate`

Rotate a runner controller token by `controller_id` and `token_id`, invalidating the old secret and issuing a fresh one. Returns the token with its newly issued one-time secret value and unchanged id.

| Annotation | **Update** |
| ---------- | ---------- |
| Tier       | Ultimate   |

### `gitlab_runner_controller_token_revoke`

Permanently revoke a runner controller token by `controller_id` and `token_id`. Destructive and irreversible; any runner controller using the token will lose access.

| Annotation | **Delete** |
| ---------- | ---------- |
| Tier       | Ultimate   |

> **Destructive**: Permanently deletes the runner controller token.

---

## Tool Summary

| #  | Tool Name                                       | Category                  |  Annotation   | Tier    |
| --: | ----------------------------------------------- | ------------------------- | :-----------: | ------- |
| 1  | `gitlab_runner_list`                            | Runner Management         |     Read      | —       |
| 2  | `gitlab_runner_list_all`                        | Runner Management         |     Read      | —       |
| 3  | `gitlab_runner_get`                             | Runner Management         |     Read      | —       |
| 4  | `gitlab_runner_update`                          | Runner Management         |    Update     | —       |
| 5  | `gitlab_runner_remove`                          | Runner Management         |    Delete     | —       |
| 6  | `gitlab_runner_jobs`                            | Runner Management         |     Read      | —       |
| 7  | `gitlab_runner_list_managers`                   | Runner Management         |     Read      | —       |
| 8  | `gitlab_runner_list_project`                    | Project & Group           |     Read      | —       |
| 9  | `gitlab_runner_enable_project`                  | Project & Group           |    Create     | —       |
| 10 | `gitlab_runner_disable_project`                 | Project & Group           |    Delete     | —       |
| 11 | `gitlab_runner_list_group`                      | Project & Group           |     Read      | —       |
| 12 | `gitlab_runner_register`                        | Registration & Tokens     |    Create     | —       |
| 13 | `gitlab_runner_delete_registered`               | Registration & Tokens     |    Delete     | —       |
| 14 | `gitlab_runner_delete_by_token`                 | Registration & Tokens     |    Delete     | —       |
| 15 | `gitlab_runner_verify`                          | Registration & Tokens     |     Read      | —       |
| 16 | `gitlab_runner_reset_token`                     | Registration & Tokens     |    Update     | —       |
| 17 | `gitlab_runner_reset_instance_reg_token`        | Registration & Tokens     |    Update     | —       |
| 18 | `gitlab_runner_reset_group_reg_token`           | Registration & Tokens     |    Update     | —       |
| 19 | `gitlab_runner_reset_project_reg_token`         | Registration & Tokens     |    Update     | —       |
| 20 | `gitlab_runner_controller_list`                 | Runner Controllers        |     Read      | Ultimate |
| 21 | `gitlab_runner_controller_get`                  | Runner Controllers        |     Read      | Ultimate |
| 22 | `gitlab_runner_controller_create`               | Runner Controllers        |    Create     | Ultimate |
| 23 | `gitlab_runner_controller_update`               | Runner Controllers        |    Update     | Ultimate |
| 24 | `gitlab_runner_controller_delete`               | Runner Controllers        |    Delete     | Ultimate |
| 25 | `gitlab_runner_controller_scope_list`           | Runner Controller Scopes  |     Read      | Ultimate |
| 26 | `gitlab_runner_controller_scope_add_instance`   | Runner Controller Scopes  |    Create     | Ultimate |
| 27 | `gitlab_runner_controller_scope_add_runner`     | Runner Controller Scopes  |    Create     | Ultimate |
| 28 | `gitlab_runner_controller_scope_remove_instance`| Runner Controller Scopes  |    Delete     | Ultimate |
| 29 | `gitlab_runner_controller_scope_remove_runner`  | Runner Controller Scopes  |    Delete     | Ultimate |
| 30 | `gitlab_runner_controller_token_list`           | Runner Controller Tokens  |     Read      | Ultimate |
| 31 | `gitlab_runner_controller_token_get`            | Runner Controller Tokens  |     Read      | Ultimate |
| 32 | `gitlab_runner_controller_token_create`         | Runner Controller Tokens  |    Create     | Ultimate |
| 33 | `gitlab_runner_controller_token_rotate`         | Runner Controller Tokens  |    Update     | Ultimate |
| 34 | `gitlab_runner_controller_token_revoke`         | Runner Controller Tokens  |    Delete     | Ultimate |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_runner_remove` — removes a CI/CD runner permanently
- `gitlab_runner_disable_project` — removes a runner assignment from a project
- `gitlab_runner_delete_registered` — deletes a registered runner by ID
- `gitlab_runner_delete_by_token` — deletes a registered runner by authentication token
- `gitlab_runner_controller_delete` — permanently removes a runner controller
- `gitlab_runner_controller_scope_remove_instance` — revokes a runner controller's instance-level scope
- `gitlab_runner_controller_scope_remove_runner` — removes a specific runner from a runner controller's scope
- `gitlab_runner_controller_token_revoke` — permanently revokes a runner controller token

---

## Related

- [GitLab Runners API](https://docs.gitlab.com/ee/api/runners.html)
