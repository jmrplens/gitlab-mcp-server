# Users — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Users
> **Individual tools**: 27
> **Meta-tools**: `gitlab_user`, `gitlab_event`, `gitlab_key`, `gitlab_namespace` (`TOOL_SURFACE=meta` catalog)
> **GitLab API**: [Users API](https://docs.gitlab.com/ee/api/users.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The users domain covers user profile retrieval, status management, SSH keys, emails, contribution events, association counts, to-do management, project/user events, SSH key lookups, and namespace operations.

With `TOOL_SURFACE=meta`, the individual tools below are consolidated into domain-specific meta-tools that dispatch by `action` parameter.

### Common Questions

> "Who am I logged in as?"
> "Show me user john's recent activity"
> "List my SSH keys"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description                                    |
| ---------- | :------: | :---------: | :--------: | ---------------------------------------------- |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation                       |
| **Create** |    —     |     No      |     —      | Creates a new resource                         |
| **Update** |    —     |     No      |    Yes     | Modifies an existing resource                  |
| **Delete** |    —     |     Yes     |    Yes     | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## User Profile

### `gitlab_user_current`

Retrieve information about the currently authenticated GitLab user. Returns user ID, username, name, email, state, avatar URL, web URL, and admin status. Useful for confirming identity and permissions.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_list_users`

List GitLab users with optional filters. Supports search by name/username/email, filtering by active/blocked/external status, ordering, and pagination. Useful for finding users or auditing accounts.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_user`

Retrieve detailed information about a specific GitLab user by their ID. Returns profile details including username, email, state, bio, and admin status.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_user_status`

Retrieve the status of a specific GitLab user. Returns emoji, message, availability, and clear-at time.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_set_user_status`

Set the status of the currently authenticated GitLab user. Supports setting emoji, message, availability (not_set/busy), and auto-clear duration.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_list_ssh_keys`

List SSH keys for the currently authenticated GitLab user. Returns key ID, title, key content, usage type, and creation/expiration dates.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_list_emails`

List email addresses for the currently authenticated GitLab user. Returns email ID, address, and confirmation status.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_list_user_contribution_events`

List contribution events for a specific GitLab user. Returns events with action type, target information, and timestamps. Supports filtering by action, target type, date range, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_user_associations_count`

Get the count of a user's associations including groups, projects, issues, and merge requests. Useful for understanding user activity scope before account management operations.

| Annotation | **Read** |
| ---------- | -------- |

---

## To-Dos

### `gitlab_todo_list`

List pending to-do items for the authenticated user. Returns paginated results with action, target, type, and state. Use page and per_page for pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_todo_mark_done`

Mark a single pending to-do item as done by its ID. Use gitlab_todo_list to find to-do item IDs first.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_todo_mark_all_done`

Mark ALL pending to-do items as done for the authenticated user. This affects all pending to-dos, not just those on a specific project.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Events

### `gitlab_project_event_list`

List all visible events for a project. Supports filtering by action type, target type, date range, sort order, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_user_contribution_event_list`

List contribution events for the authenticated user. Supports filtering by action type, target type, date range, sort order, scope, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## SSH Keys

### `gitlab_get_key_with_user`

Get an SSH key and its associated user by key ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_key_by_fingerprint`

Get an SSH key and its user by SSH key fingerprint (SHA256: or MD5:).

| Annotation | **Read** |
| ---------- | -------- |

---

## Namespaces

### `gitlab_namespace_list`

List all namespaces visible to the authenticated user. Supports filtering by search, owned-only, top-level-only, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_namespace_get`

Get details of a single namespace by ID or path.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_namespace_exists`

Check whether a namespace path exists (is taken). Returns availability and suggested alternatives if the path is taken.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_namespace_search`

Search namespaces by query string. Returns matching namespaces with pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Instance Service Accounts (Enterprise/Premium)

### `gitlab_create_service_account`

Create a new instance-level service account. Optionally set `name`, `username`, and `email`. Requires admin token. Returns the created user object.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_list_service_accounts`

List all instance-level service accounts. Supports ordering by `id`, `username`, or `name` with `sort` direction and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_update_instance_service_account`

Update an instance-level service account. Can change `name`, `username`, or `email`. Requires admin token. Returns the updated service account including `email` and `unconfirmed_email` fields.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Group Service Accounts

### `gitlab_group_service_account_list`

List all service accounts for a GitLab group. Returns ID, name, username, and email.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_service_account_create`

Create a service account in a GitLab group (top-level only). Requires name and username.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_service_account_update`

Update a service account in a GitLab group (top-level only). Can change name or username.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_service_account_delete`

Delete a service account from a GitLab group (top-level only).

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_group_service_account_pat_list`

List personal access tokens for a group service account.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_service_account_pat_create`

Create a personal access token for a group service account. Returns the token value (shown only once).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_service_account_pat_revoke`

Revoke a personal access token for a group service account.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_user_current` | User Profile | Read |
| 2 | `gitlab_list_users` | User Profile | Read |
| 3 | `gitlab_get_user` | User Profile | Read |
| 4 | `gitlab_get_user_status` | User Profile | Read |
| 5 | `gitlab_set_user_status` | User Profile | Update |
| 6 | `gitlab_list_ssh_keys` | User Profile | Read |
| 7 | `gitlab_list_emails` | User Profile | Read |
| 8 | `gitlab_list_user_contribution_events` | User Profile | Read |
| 9 | `gitlab_get_user_associations_count` | User Profile | Read |
| 10 | `gitlab_todo_list` | To-Dos | Read |
| 11 | `gitlab_todo_mark_done` | To-Dos | Update |
| 12 | `gitlab_todo_mark_all_done` | To-Dos | Update |
| 13 | `gitlab_project_event_list` | Events | Read |
| 14 | `gitlab_user_contribution_event_list` | Events | Read |
| 15 | `gitlab_get_key_with_user` | SSH Keys | Read |
| 16 | `gitlab_get_key_by_fingerprint` | SSH Keys | Read |
| 17 | `gitlab_namespace_list` | Namespaces | Read |
| 18 | `gitlab_namespace_get` | Namespaces | Read |
| 19 | `gitlab_namespace_exists` | Namespaces | Read |
| 20 | `gitlab_namespace_search` | Namespaces | Read |
| 21 | `gitlab_create_service_account` | Instance Service Accounts | Create |
| 22 | `gitlab_list_service_accounts` | Instance Service Accounts | Read |
| 23 | `gitlab_update_instance_service_account` | Instance Service Accounts | Update |
| 24 | `gitlab_group_service_account_list` | Group Service Accounts | Read |
| 25 | `gitlab_group_service_account_create` | Group Service Accounts | Create |
| 26 | `gitlab_group_service_account_update` | Group Service Accounts | Update |
| 27 | `gitlab_group_service_account_delete` | Group Service Accounts | Delete |
| 28 | `gitlab_group_service_account_pat_list` | Group Service Accounts | Read |
| 29 | `gitlab_group_service_account_pat_create` | Group Service Accounts | Create |
| 30 | `gitlab_group_service_account_pat_revoke` | Group Service Accounts | Delete |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation:

- `gitlab_group_service_account_delete` — deletes a group service account
- `gitlab_group_service_account_pat_revoke` — revokes a service account PAT

---

## Related

- [GitLab Users API](https://docs.gitlab.com/ee/api/users.html)
- [GitLab To-Dos API](https://docs.gitlab.com/ee/api/todos.html)
- [GitLab Events API](https://docs.gitlab.com/ee/api/events.html)
- [GitLab Keys API](https://docs.gitlab.com/ee/api/keys.html)
- [GitLab Namespaces API](https://docs.gitlab.com/ee/api/namespaces.html)
- [GitLab Group Service Accounts API](https://docs.gitlab.com/ee/api/group_service_accounts.html)
