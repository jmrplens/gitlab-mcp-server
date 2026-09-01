# Users — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Users & Todos
> **Individual tools**: 64
> **Meta-tools**: `gitlab_user` (`TOOL_SURFACE=meta` catalog)
> **Dynamic IDs**: `user.*` (default surface, via `gitlab_execute_action`)
> **GitLab API**: [Users API](https://docs.gitlab.com/ee/api/users.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The users domain covers user profile retrieval, status management, SSH keys, GPG keys, emails, avatars, account lifecycle (create/modify/delete/block/ban/activate), association counts, contribution events, membership reports, personal access tokens, user-scoped runners, to-do management, project/user events, SSH key lookups, namespace operations, and instance-level service accounts.

On the default dynamic surface, these operations are the `user.*` entries of the canonical action catalog: find them with `gitlab_find_action` and run them with `gitlab_execute_action` by `domain.action` ID. With `TOOL_SURFACE=individual`, each is the tool named in the tables below.

With `TOOL_SURFACE=meta`, the individual tools below are consolidated into domain-specific meta-tools that dispatch by `action` parameter.

### Common Questions

> "Who am I logged in as?"
> "Show me user john's recent activity"
> "List my SSH keys"
> "Add a new SSH key to my account"
> "Block a user from signing in"
> "What are my pending to-do items?"

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

List GitLab users with optional filters. Supports search by name/username/email, filtering by active/blocked/external status, ordering by `id`, `name`, `username`, `created_at`, or `updated_at`, and pagination. Useful for finding users or auditing accounts.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_user`

Retrieve detailed information about a specific GitLab user by their ID. Returns profile details including username, email, state, bio, and admin status.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_user_status`

Retrieve the status of a specific GitLab user by their ID. Returns emoji, message, availability, and clear-at time.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_set_user_status`

Set the status of the currently authenticated GitLab user. Supports setting emoji, message, availability (`not_set` or `busy`), and auto-clear duration (`30_minutes`, `3_hours`, `8_hours`, `1_day`, `3_days`, `7_days`, or `30_days`).

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_current_user_status`

Retrieve the status of the currently authenticated GitLab user. Returns emoji, message, availability, and clear-at time.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_avatar`

Resolve the avatar URL for a known email address. Optional `size` parameter selects the pixel size. Returns the resolved avatar URL.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_upload_user_avatar`

Set or replace the currently authenticated user's avatar image. Provide a `filename` plus exactly one of `file_path` (a local image on the MCP server) or `content_base64` (base64-encoded JPG/PNG/GIF under 200 KB). Targets the token's own user; there is no `user_id` parameter.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_list_user_contribution_events`

List contribution events for a specific GitLab user. Returns events with action type, target information, and timestamps. Supports filtering by action, target type, date range, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_user_associations_count`

Get the count of a user's associations including groups, projects, issues, and merge requests. Useful for understanding user activity scope before account management operations.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_user_activities`

List recent user activities (last-activity date per username, admin only). Supports filtering by a `from` date. Useful for auditing who has been active recently.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_user_memberships`

List a user's project and group memberships by `user_id`, optionally filtered by `type` (`Project` or `Namespace`). Returns source ID, name, type, and access level per membership.

| Annotation | **Read** |
| ---------- | -------- |

---

## Account Lifecycle (Admin)

### `gitlab_create_user`

Create a new GitLab user account (admin only). Required: `email`, `name`, `username`. Optional flags include `admin`, `auditor`, `external`, `private_profile`, `projects_limit`, `skip_confirmation`, `reset_password`, and social/profile fields. Premium/Ultimate fields include `auditor`, `extra_shared_runners_minutes_limit`, and `shared_runners_minutes_limit`.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_modify_user`

Update an existing user's profile fields (email, name, flags, social links) by `user_id` (admin only). Useful for changing a user's account details.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_user`

Delete a user account by `user_id` (admin only). Supports `hard_delete` to remove all contributions. Returns confirmation with the deleted user ID.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_activate_user`

Activate a deactivated user account by `user_id` (admin only). Reverses the effect of `gitlab_deactivate_user`.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_deactivate_user`

Deactivate an active user account by `user_id` (admin only). Reversible via `gitlab_activate_user`.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_approve_user`

Approve a pending user sign-up by `user_id` (admin only). Useful when the GitLab instance requires admin approval for new registrations.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_reject_user`

Reject a pending user sign-up by `user_id` (admin only). Permanently deletes the pending user.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_block_user`

Block a user from signing in by `user_id` (admin only). Reversible via `gitlab_unblock_user`.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_unblock_user`

Unblock a previously blocked user by `user_id` (admin only). Reverses the effect of `gitlab_block_user`.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_ban_user`

Ban a user by `user_id` (admin only). Reversible via `gitlab_unban_user`. Bans hide the user's content.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_unban_user`

Unban a previously banned user by `user_id` (admin only). Reverses the effect of `gitlab_ban_user`.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_disable_two_factor`

Disable two-factor authentication for a user by `user_id` (admin only). Useful for clearing a locked-out user's 2FA.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_user_identity`

Delete a user's external authentication identity by `user_id` and `provider` (admin only). Use to unlink an SSO/LDAP identity from a user.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## SSH Keys

### `gitlab_list_ssh_keys`

List SSH keys for the currently authenticated GitLab user. Returns key ID, title, key content, usage type, and creation/expiration dates.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_ssh_key`

Fetch one of the currently authenticated user's SSH keys by `key_id`. Returns key ID, title, public key, usage type, and expiry.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_add_ssh_key`

Add an SSH public key to the currently authenticated user's account. Requires `title` and `key`. Optional `expires_at` (ISO 8601 date `YYYY-MM-DD`) and `usage_type` (`auth`, `signing`, or `auth_and_signing`, default `auth_and_signing`).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_delete_ssh_key`

Delete one of the currently authenticated user's SSH keys by `key_id`.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_list_ssh_keys_for_user`

List SSH keys for a specific user by `user_id`. Returns key summaries with ID, title, fingerprint, usage type, and expiry. Supports keyset pagination via `pagination`, `order_by`, `sort`, and `page_token`.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_ssh_key_for_user`

Fetch a specific SSH key for a specific user by `user_id` and `key_id`. Returns key ID, title, public key, usage type, and expiry.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_add_ssh_key_for_user`

Add an SSH public key to a specific user's account (admin only) by `user_id`. Requires `title` and `key`. Optional `expires_at` and `usage_type`.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_delete_ssh_key_for_user`

Delete a specific SSH key from a specific user's account (admin only) by `user_id` and `key_id`.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_get_key_with_user`

Get an SSH key and its associated user by `key_id`.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_key_by_fingerprint`

Get an SSH key and its user by SSH key fingerprint (`SHA256:` or `MD5:`).

| Annotation | **Read** |
| ---------- | -------- |

---

## GPG Keys

### `gitlab_list_gpg_keys`

List the currently authenticated user's GPG keys. Returns each key's ID, armored public key, and creation timestamp.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_gpg_key`

Fetch a single GPG key belonging to the currently authenticated user by `key_id`. Returns the key's ID, armored public key, and creation timestamp.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_add_gpg_key`

Add a GPG key to the currently authenticated user's account. `key` must be an ASCII-armored OpenPGP public key block whose fingerprint is unique across GitLab.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_delete_gpg_key`

Delete a GPG key from the currently authenticated user's account by `key_id`.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_list_gpg_keys_for_user`

List a specific user's GPG keys by `user_id`. Returns each key's ID, armored public key, and creation timestamp.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_gpg_key_for_user`

Fetch a single GPG key for a specific user by `user_id` and `key_id`. Returns the key's ID, armored public key, and creation timestamp.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_add_gpg_key_for_user`

Add a GPG key to a specific user's account (admin only) by `user_id`. `key` must be an ASCII-armored OpenPGP public key block whose fingerprint is unique across GitLab.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_delete_gpg_key_for_user`

Delete a GPG key from a specific user's account (admin only) by `user_id` and `key_id`.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Emails

### `gitlab_list_emails`

List email addresses for the currently authenticated GitLab user. Returns email ID, address, and confirmation status.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_email`

Fetch a single email address belonging to the currently authenticated user by `email_id`. Returns the email's ID, address, and confirmation timestamp.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_add_email`

Add an email address to the currently authenticated user's account. `skip_confirmation` requires an admin token. The email must be a valid RFC 5322 address that is not already taken.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_delete_email`

Delete an email address from the currently authenticated user's account by `email_id`. The primary email cannot be deleted.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_list_emails_for_user`

List all email addresses registered to a specific user's account by `user_id`. Supports offset and keyset pagination plus `order_by` and `sort`.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_add_email_for_user`

Add an email address to a specific user's account (admin only) by `user_id`. `skip_confirmation` requires an admin token.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_delete_email_for_user`

Delete an email address from a specific user's account (admin only) by `user_id` and `email_id`. The primary email cannot be deleted.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Personal Access Tokens & Runners

### `gitlab_create_current_user_pat`

Create a personal access token for the currently authenticated user. Requires `name` and `scopes`. Optional `expires_at`. Returns the token ID, the secret token value (capture it immediately — it is shown only once), scopes, and expiry.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_create_user_runner`

Register a CI runner scoped to the currently authenticated user. Requires `runner_type` (`instance_type`, `group_type`, or `project_type`); group runners require `group_id`, project runners require `project_id`. Returns runner ID, authentication token, and token expiry.

| Annotation | **Create** |
| ---------- | ---------- |

---

## To-Dos

### `gitlab_todo_list`

List pending to-do items for the authenticated user. Returns paginated results with action, target, type, and state. Use `page` and `per_page` for pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_todo_mark_done`

Mark a single pending to-do item as done by its ID. Use `gitlab_todo_list` to find to-do item IDs first.

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

## Instance Service Accounts

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

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_user_current` | User Profile | Read |
| 2 | `gitlab_list_users` | User Profile | Read |
| 3 | `gitlab_get_user` | User Profile | Read |
| 4 | `gitlab_get_user_status` | User Profile | Read |
| 5 | `gitlab_set_user_status` | User Profile | Update |
| 6 | `gitlab_current_user_status` | User Profile | Read |
| 7 | `gitlab_get_avatar` | User Profile | Read |
| 8 | `gitlab_upload_user_avatar` | User Profile | Update |
| 9 | `gitlab_list_user_contribution_events` | User Profile | Read |
| 10 | `gitlab_get_user_associations_count` | User Profile | Read |
| 11 | `gitlab_get_user_activities` | User Profile | Read |
| 12 | `gitlab_get_user_memberships` | User Profile | Read |
| 13 | `gitlab_create_user` | Account Lifecycle | Create |
| 14 | `gitlab_modify_user` | Account Lifecycle | Update |
| 15 | `gitlab_delete_user` | Account Lifecycle | Delete |
| 16 | `gitlab_activate_user` | Account Lifecycle | Update |
| 17 | `gitlab_deactivate_user` | Account Lifecycle | Update |
| 18 | `gitlab_approve_user` | Account Lifecycle | Update |
| 19 | `gitlab_reject_user` | Account Lifecycle | Delete |
| 20 | `gitlab_block_user` | Account Lifecycle | Update |
| 21 | `gitlab_unblock_user` | Account Lifecycle | Update |
| 22 | `gitlab_ban_user` | Account Lifecycle | Update |
| 23 | `gitlab_unban_user` | Account Lifecycle | Update |
| 24 | `gitlab_disable_two_factor` | Account Lifecycle | Update |
| 25 | `gitlab_delete_user_identity` | Account Lifecycle | Delete |
| 26 | `gitlab_list_ssh_keys` | SSH Keys | Read |
| 27 | `gitlab_get_ssh_key` | SSH Keys | Read |
| 28 | `gitlab_add_ssh_key` | SSH Keys | Create |
| 29 | `gitlab_delete_ssh_key` | SSH Keys | Delete |
| 30 | `gitlab_list_ssh_keys_for_user` | SSH Keys | Read |
| 31 | `gitlab_get_ssh_key_for_user` | SSH Keys | Read |
| 32 | `gitlab_add_ssh_key_for_user` | SSH Keys | Create |
| 33 | `gitlab_delete_ssh_key_for_user` | SSH Keys | Delete |
| 34 | `gitlab_get_key_with_user` | SSH Keys | Read |
| 35 | `gitlab_get_key_by_fingerprint` | SSH Keys | Read |
| 36 | `gitlab_list_gpg_keys` | GPG Keys | Read |
| 37 | `gitlab_get_gpg_key` | GPG Keys | Read |
| 38 | `gitlab_add_gpg_key` | GPG Keys | Create |
| 39 | `gitlab_delete_gpg_key` | GPG Keys | Delete |
| 40 | `gitlab_list_gpg_keys_for_user` | GPG Keys | Read |
| 41 | `gitlab_get_gpg_key_for_user` | GPG Keys | Read |
| 42 | `gitlab_add_gpg_key_for_user` | GPG Keys | Create |
| 43 | `gitlab_delete_gpg_key_for_user` | GPG Keys | Delete |
| 44 | `gitlab_list_emails` | Emails | Read |
| 45 | `gitlab_get_email` | Emails | Read |
| 46 | `gitlab_add_email` | Emails | Create |
| 47 | `gitlab_delete_email` | Emails | Delete |
| 48 | `gitlab_list_emails_for_user` | Emails | Read |
| 49 | `gitlab_add_email_for_user` | Emails | Create |
| 50 | `gitlab_delete_email_for_user` | Emails | Delete |
| 51 | `gitlab_create_current_user_pat` | Personal Access Tokens & Runners | Create |
| 52 | `gitlab_create_user_runner` | Personal Access Tokens & Runners | Create |
| 53 | `gitlab_todo_list` | To-Dos | Read |
| 54 | `gitlab_todo_mark_done` | To-Dos | Update |
| 55 | `gitlab_todo_mark_all_done` | To-Dos | Update |
| 56 | `gitlab_project_event_list` | Events | Read |
| 57 | `gitlab_user_contribution_event_list` | Events | Read |
| 58 | `gitlab_namespace_list` | Namespaces | Read |
| 59 | `gitlab_namespace_get` | Namespaces | Read |
| 60 | `gitlab_namespace_exists` | Namespaces | Read |
| 61 | `gitlab_namespace_search` | Namespaces | Read |
| 62 | `gitlab_create_service_account` | Instance Service Accounts | Create |
| 63 | `gitlab_list_service_accounts` | Instance Service Accounts | Read |
| 64 | `gitlab_update_instance_service_account` | Instance Service Accounts | Update |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_delete_user` — deletes a user account
- `gitlab_reject_user` — rejects (and deletes) a pending user sign-up
- `gitlab_delete_user_identity` — deletes a user's external identity
- `gitlab_delete_ssh_key` — deletes the current user's SSH key
- `gitlab_delete_ssh_key_for_user` — deletes a specific user's SSH key
- `gitlab_delete_gpg_key` — deletes the current user's GPG key
- `gitlab_delete_gpg_key_for_user` — deletes a specific user's GPG key
- `gitlab_delete_email` — deletes the current user's email address
- `gitlab_delete_email_for_user` — deletes a specific user's email address

---

## Related

- [GitLab Users API](https://docs.gitlab.com/ee/api/users.html)
- [GitLab User Keys (SSH) API](https://docs.gitlab.com/ee/api/user_keys.html)
- [GitLab User GPG Keys API](https://docs.gitlab.com/api/user_keys/)
- [GitLab Emails API](https://docs.gitlab.com/api/user_email_addresses/)
- [GitLab Avatars API](https://docs.gitlab.com/ee/api/avatar.html)
- [GitLab To-Dos API](https://docs.gitlab.com/ee/api/todos.html)
- [GitLab Events API](https://docs.gitlab.com/ee/api/events.html)
- [GitLab Keys API](https://docs.gitlab.com/ee/api/keys.html)
- [GitLab Namespaces API](https://docs.gitlab.com/ee/api/namespaces.html)
- [GitLab Service Accounts API](https://docs.gitlab.com/ee/api/service_accounts.html)
