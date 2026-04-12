# Access & Authentication — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Access & Authentication
> **Individual tools**: 62
> **Meta-tools**: `gitlab_access_token`, `gitlab_deploy_token`, `gitlab_deploy_key`, `gitlab_access_request`, `gitlab_invite`, `gitlab_job_token_scope` (when `META_TOOLS=true`, default)
> **GitLab API**: [Access Tokens API](https://docs.gitlab.com/ee/api/project_access_tokens.html), [Deploy Tokens API](https://docs.gitlab.com/ee/api/deploy_tokens.html), [Deploy Keys API](https://docs.gitlab.com/ee/api/deploy_keys.html), [Members API](https://docs.gitlab.com/ee/api/members.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The access & authentication domain covers project/group/personal access tokens, deploy tokens, deploy keys, access requests, invitations, CI/CD job token scope management, and project member management.

When `META_TOOLS=true` (the default), the 62 individual tools below are consolidated into domain-specific meta-tools that dispatch by `action` parameter.

### Common Questions

> "List access tokens for project 42"
> "Create a deploy token for my project"
> "Show deploy keys for project 42"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Project Access Tokens

### `gitlab_project_access_token_list`

List all access tokens for a GitLab project. Filter by state (active, inactive).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_access_token_get`

Get a specific project access token by its ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_access_token_create`

Create a new project access token with specified name, scopes, access level, and optional expiry date.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_access_token_rotate`

Rotate a project access token, generating a new token value. Optionally set a new expiry date.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_access_token_revoke`

Revoke a project access token. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_project_access_token_rotate_self`

Rotate the project access token used for the current request. Returns the new token value.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Group Access Tokens

### `gitlab_group_access_token_list`

List all access tokens for a GitLab group. Filter by state (active, inactive).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_access_token_get`

Get a specific group access token by its ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_access_token_create`

Create a new group access token with specified name, scopes, access level, and optional expiry date.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_access_token_rotate`

Rotate a group access token, generating a new token value. Optionally set a new expiry date.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_access_token_revoke`

Revoke a group access token. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_group_access_token_rotate_self`

Rotate the group access token used for the current request. Returns the new token value.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Personal Access Tokens

### `gitlab_personal_access_token_list`

List personal access tokens. Filter by state, search by name, or filter by user ID (admin only).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_personal_access_token_get`

Get a personal access token by ID. Use token_id=0 to retrieve the current token used for authentication.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_personal_access_token_rotate`

Rotate a personal access token, generating a new token value. Optionally set a new expiry date.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_personal_access_token_revoke`

Revoke a personal access token by ID. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_personal_access_token_rotate_self`

Rotate the personal access token used for the current request. Returns the new token value.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_personal_access_token_revoke_self`

Revoke the personal access token used for the current request. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Deploy Tokens

### `gitlab_deploy_token_list_all`

List all instance-level deploy tokens. Requires admin access.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_deploy_token_list_project`

List all deploy tokens for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_deploy_token_list_group`

List all deploy tokens for a GitLab group.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_deploy_token_get_project`

Get a specific deploy token for a project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_deploy_token_get_group`

Get a specific deploy token for a group.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_deploy_token_create_project`

Create a deploy token for a project with name, scopes, optional username and expiry date.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_deploy_token_create_group`

Create a deploy token for a group with name, scopes, optional username and expiry date.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_deploy_token_delete_project`

Delete a deploy token from a project. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_deploy_token_delete_group`

Delete a deploy token from a group. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Deploy Keys

### `gitlab_deploy_key_list_project`

List all deploy keys for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_deploy_key_get`

Get a specific deploy key for a project by its ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_deploy_key_add`

Add a deploy key to a GitLab project with title, public SSH key, and optional push access and expiry date.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_deploy_key_update`

Update an existing deploy key's title or push access permission.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_deploy_key_delete`

Remove a deploy key from a GitLab project.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_deploy_key_enable`

Enable an existing deploy key for a project (e.g., a key shared from another project).

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_deploy_key_list_all`

List all instance-level deploy keys. Requires admin access. Filter by public keys.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_deploy_key_add_instance`

Create an instance-level deploy key with title, public SSH key, and optional expiry date. Requires admin access.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_deploy_key_list_user_project`

List all deploy keys across projects for a specific user.

| Annotation | **Read** |
| ---------- | -------- |

---

## Access Requests

### `gitlab_access_request_list_project`

List access requests for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_access_request_list_group`

List access requests for a GitLab group.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_access_request_request_project`

Request access to a GitLab project for the authenticated user.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_access_request_request_group`

Request access to a GitLab group for the authenticated user.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_access_request_approve_project`

Approve a project access request. Optionally set the access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer).

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_access_request_approve_group`

Approve a group access request. Optionally set the access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer).

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_access_request_deny_project`

Deny a project access request. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_access_request_deny_group`

Deny a group access request. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Invitations

### `gitlab_project_invite_list_pending`

List all pending invitations for a project. Supports filtering by query and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_invite_list_pending`

List all pending invitations for a group. Supports filtering by query and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_invite`

Invite a user to a project by email or user ID. Requires access_level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_invite`

Invite a user to a group by email or user ID. Requires access_level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner).

| Annotation | **Create** |
| ---------- | ---------- |

---

## Job Token Scope

### `gitlab_get_job_token_access_settings`

Get the CI/CD job token access settings for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_patch_job_token_access_settings`

Update the CI/CD job token access settings for a GitLab project.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_list_job_token_inbound_allowlist`

List projects on the CI/CD job token inbound allowlist for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_add_project_job_token_allowlist`

Add a project to the CI/CD job token inbound allowlist.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_remove_project_job_token_allowlist`

Remove a project from the CI/CD job token inbound allowlist.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_list_job_token_group_allowlist`

List groups on the CI/CD job token allowlist for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_add_group_job_token_allowlist`

Add a group to the CI/CD job token allowlist.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_remove_group_job_token_allowlist`

Remove a group from the CI/CD job token allowlist.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Project Members

### `gitlab_project_members_list`

List all members of a GitLab project including inherited members from parent groups. Returns user ID, username, name, state, access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner), and web URL. Supports filtering by name/username query.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_member_get`

Get details of a specific project member by user ID. Returns access level, state, username, and membership info.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_member_get_inherited`

Get a project member including inherited membership from parent groups. Returns access level, state, and membership origin.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_member_add`

Add a user as a member of a project. Requires user_id (or username) and access_level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner). Optionally set expires_at (YYYY-MM-DD) and member_role_id.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_member_edit`

Edit a project member's access level or expiration. Requires access_level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner).

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_member_delete`

Remove a member from a project.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_project_access_token_list` | Project Access Tokens | Read |
| 2 | `gitlab_project_access_token_get` | Project Access Tokens | Read |
| 3 | `gitlab_project_access_token_create` | Project Access Tokens | Create |
| 4 | `gitlab_project_access_token_rotate` | Project Access Tokens | Update |
| 5 | `gitlab_project_access_token_revoke` | Project Access Tokens | Delete |
| 6 | `gitlab_project_access_token_rotate_self` | Project Access Tokens | Update |
| 7 | `gitlab_group_access_token_list` | Group Access Tokens | Read |
| 8 | `gitlab_group_access_token_get` | Group Access Tokens | Read |
| 9 | `gitlab_group_access_token_create` | Group Access Tokens | Create |
| 10 | `gitlab_group_access_token_rotate` | Group Access Tokens | Update |
| 11 | `gitlab_group_access_token_revoke` | Group Access Tokens | Delete |
| 12 | `gitlab_group_access_token_rotate_self` | Group Access Tokens | Update |
| 13 | `gitlab_personal_access_token_list` | Personal Access Tokens | Read |
| 14 | `gitlab_personal_access_token_get` | Personal Access Tokens | Read |
| 15 | `gitlab_personal_access_token_rotate` | Personal Access Tokens | Update |
| 16 | `gitlab_personal_access_token_revoke` | Personal Access Tokens | Delete |
| 17 | `gitlab_personal_access_token_rotate_self` | Personal Access Tokens | Update |
| 18 | `gitlab_personal_access_token_revoke_self` | Personal Access Tokens | Delete |
| 19 | `gitlab_deploy_token_list_all` | Deploy Tokens | Read |
| 20 | `gitlab_deploy_token_list_project` | Deploy Tokens | Read |
| 21 | `gitlab_deploy_token_list_group` | Deploy Tokens | Read |
| 22 | `gitlab_deploy_token_get_project` | Deploy Tokens | Read |
| 23 | `gitlab_deploy_token_get_group` | Deploy Tokens | Read |
| 24 | `gitlab_deploy_token_create_project` | Deploy Tokens | Create |
| 25 | `gitlab_deploy_token_create_group` | Deploy Tokens | Create |
| 26 | `gitlab_deploy_token_delete_project` | Deploy Tokens | Delete |
| 27 | `gitlab_deploy_token_delete_group` | Deploy Tokens | Delete |
| 28 | `gitlab_deploy_key_list_project` | Deploy Keys | Read |
| 29 | `gitlab_deploy_key_get` | Deploy Keys | Read |
| 30 | `gitlab_deploy_key_add` | Deploy Keys | Create |
| 31 | `gitlab_deploy_key_update` | Deploy Keys | Update |
| 32 | `gitlab_deploy_key_delete` | Deploy Keys | Delete |
| 33 | `gitlab_deploy_key_enable` | Deploy Keys | Update |
| 34 | `gitlab_deploy_key_list_all` | Deploy Keys | Read |
| 35 | `gitlab_deploy_key_add_instance` | Deploy Keys | Create |
| 36 | `gitlab_deploy_key_list_user_project` | Deploy Keys | Read |
| 37 | `gitlab_access_request_list_project` | Access Requests | Read |
| 38 | `gitlab_access_request_list_group` | Access Requests | Read |
| 39 | `gitlab_access_request_request_project` | Access Requests | Create |
| 40 | `gitlab_access_request_request_group` | Access Requests | Create |
| 41 | `gitlab_access_request_approve_project` | Access Requests | Update |
| 42 | `gitlab_access_request_approve_group` | Access Requests | Update |
| 43 | `gitlab_access_request_deny_project` | Access Requests | Delete |
| 44 | `gitlab_access_request_deny_group` | Access Requests | Delete |
| 45 | `gitlab_project_invite_list_pending` | Invitations | Read |
| 46 | `gitlab_group_invite_list_pending` | Invitations | Read |
| 47 | `gitlab_project_invite` | Invitations | Create |
| 48 | `gitlab_group_invite` | Invitations | Create |
| 49 | `gitlab_get_job_token_access_settings` | Job Token Scope | Read |
| 50 | `gitlab_patch_job_token_access_settings` | Job Token Scope | Update |
| 51 | `gitlab_list_job_token_inbound_allowlist` | Job Token Scope | Read |
| 52 | `gitlab_add_project_job_token_allowlist` | Job Token Scope | Create |
| 53 | `gitlab_remove_project_job_token_allowlist` | Job Token Scope | Delete |
| 54 | `gitlab_list_job_token_group_allowlist` | Job Token Scope | Read |
| 55 | `gitlab_add_group_job_token_allowlist` | Job Token Scope | Create |
| 56 | `gitlab_remove_group_job_token_allowlist` | Job Token Scope | Delete |
| 57 | `gitlab_project_members_list` | Project Members | Read |
| 58 | `gitlab_project_member_get` | Project Members | Read |
| 59 | `gitlab_project_member_get_inherited` | Project Members | Read |
| 60 | `gitlab_project_member_add` | Project Members | Create |
| 61 | `gitlab_project_member_edit` | Project Members | Update |
| 62 | `gitlab_project_member_delete` | Project Members | Delete |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_project_access_token_revoke` — revokes a project access token
- `gitlab_group_access_token_revoke` — revokes a group access token
- `gitlab_personal_access_token_revoke` — revokes a personal access token
- `gitlab_personal_access_token_revoke_self` — revokes the current personal access token
- `gitlab_deploy_token_delete_project` — deletes a project deploy token
- `gitlab_deploy_token_delete_group` — deletes a group deploy token
- `gitlab_deploy_key_delete` — removes a deploy key from a project
- `gitlab_access_request_deny_project` — denies a project access request
- `gitlab_access_request_deny_group` — denies a group access request
- `gitlab_remove_project_job_token_allowlist` — removes a project from job token allowlist
- `gitlab_remove_group_job_token_allowlist` — removes a group from job token allowlist
- `gitlab_project_member_delete` — removes a member from a project

---

## Related

- [GitLab Project Access Tokens API](https://docs.gitlab.com/ee/api/project_access_tokens.html)
- [GitLab Group Access Tokens API](https://docs.gitlab.com/ee/api/group_access_tokens.html)
- [GitLab Personal Access Tokens API](https://docs.gitlab.com/ee/api/personal_access_tokens.html)
- [GitLab Deploy Tokens API](https://docs.gitlab.com/ee/api/deploy_tokens.html)
- [GitLab Deploy Keys API](https://docs.gitlab.com/ee/api/deploy_keys.html)
- [GitLab Access Requests API](https://docs.gitlab.com/ee/api/access_requests.html)
- [GitLab Invitations API](https://docs.gitlab.com/ee/api/invitations.html)
- [GitLab Job Token Scope API](https://docs.gitlab.com/ee/api/project_job_token_scopes.html)
- [GitLab Members API](https://docs.gitlab.com/ee/api/members.html)
