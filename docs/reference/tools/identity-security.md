# Identity & Security — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Identity & Security
> **Individual tools**: 30
> **Meta-tools**: `gitlab_group_scim` and `gitlab_member_role` (with `GITLAB_MCP_TOOL_SURFACE=meta` and the Enterprise/Premium catalog enabled); SSH certificates, security settings, group credentials, LDAP links and SAML links have no meta-tool of their own and are enterprise-only routes inside `gitlab_group` / `gitlab_project`
> **Dynamic IDs**: `group.*`, `group_scim.*`, `member_role.*`, `project.*` (default surface, via `gitlab_execute_action`)
> **GitLab API**: [SCIM API](https://docs.gitlab.com/ee/api/scim.html) · [Group SSH Certificates API](https://docs.gitlab.com/ee/api/group_ssh_certificates.html) · [Security Settings API](https://docs.gitlab.com/ee/api/project_security_settings.html) · [Member Roles API](https://docs.gitlab.com/ee/api/member_roles.html) · [Group Credentials API](https://docs.gitlab.com/user/group/credentials_inventory/) · [LDAP Group Links API](https://docs.gitlab.com/api/merge_request_approval_settings/) · [SAML Group Links API](https://docs.gitlab.com/ee/api/groups.html#saml-group-links)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The identity & security domain covers SCIM identity management for groups, SSH certificate management, project and group security settings (secret push protection), custom member roles at instance and group level, group credential inventory (personal access tokens and SSH keys), LDAP group link management, and SAML group link management.

On the default dynamic surface, these operations are the `group.*`, `group_scim.*`, `member_role.*`, `project.*` entries of the canonical action catalog: find them with `gitlab_find_action` and run them with `gitlab_execute_action` by `domain.action` ID. With `GITLAB_MCP_TOOL_SURFACE=individual`, each is the tool named in the tables below.

With `GITLAB_MCP_TOOL_SURFACE=meta` and the Enterprise/Premium catalog enabled, the 30 individual tools below are reached through meta-tools. Only `gitlab_group_scim` and `gitlab_member_role` are meta-tools of their own; everything else is a route on `gitlab_group` or `gitlab_project` — `ssh_cert_list`, `credential_list_pats` and the group-scoped LDAP and SAML actions on `gitlab_group`, and `security_settings_get` / `security_settings_update` on both, each scoped by the identifier it is given.

### Common Questions

> "List SCIM identities for my group"
> "Create an SSH certificate for a group"
> "Get security settings for project 42"
> "List custom member roles for my group"
> "Show personal access tokens managed by a group"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description                                    |
| ---------- | :------: | :---------: | :--------: | ---------------------------------------------- |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation                       |
| **Create** |    —     |     No      |     —      | Creates a new resource                         |
| **Update** |    —     |     No      |    Yes     | Modifies an existing resource                  |
| **Delete** |    —     |     Yes     |    Yes     | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Group SCIM Identities

### `gitlab_list_group_scim_identities`

List all SCIM identities for a group.

| Parameter  | Type       | Required | Description                  |
| ---------- | ---------- | :------: | ---------------------------- |
| `group_id` | string/int |   Yes    | Group ID or URL-encoded path |

**Annotation**: Read

### `gitlab_get_group_scim_identity`

Get a single SCIM identity by external UID.

| Parameter  | Type       | Required | Description                  |
| ---------- | ---------- | :------: | ---------------------------- |
| `group_id` | string/int |   Yes    | Group ID or URL-encoded path |
| `uid`      | string     |   Yes    | SCIM external UID            |

**Annotation**: Read

### `gitlab_update_group_scim_identity`

Update a SCIM identity's external UID.

| Parameter    | Type       | Required | Description                  |
| ------------ | ---------- | :------: | ---------------------------- |
| `group_id`   | string/int |   Yes    | Group ID or URL-encoded path |
| `uid`        | string     |   Yes    | Current SCIM external UID    |
| `extern_uid` | string     |   Yes    | New external UID value       |

**Annotation**: Update

### `gitlab_delete_group_scim_identity`

Delete a SCIM identity from a group.

| Parameter  | Type       | Required | Description                  |
| ---------- | ---------- | :------: | ---------------------------- |
| `group_id` | string/int |   Yes    | Group ID or URL-encoded path |
| `uid`      | string     |   Yes    | SCIM external UID            |

**Annotation**: Delete

---

## Group SSH Certificates

### `gitlab_list_group_ssh_certificates`

List SSH certificates for a group.

| Parameter  | Type       | Required | Description                  |
| ---------- | ---------- | :------: | ---------------------------- |
| `group_id` | string/int |   Yes    | Group ID or URL-encoded path |

**Annotation**: Read

### `gitlab_create_group_ssh_certificate`

Create an SSH certificate for a group.

| Parameter  | Type       | Required | Description                  |
| ---------- | ---------- | :------: | ---------------------------- |
| `group_id` | string/int |   Yes    | Group ID or URL-encoded path |
| `key`      | string     |   Yes    | SSH public key               |
| `title`    | string     |   Yes    | Certificate title            |

**Annotation**: Create

### `gitlab_delete_group_ssh_certificate`

Delete an SSH certificate from a group.

| Parameter        | Type       | Required | Description                  |
| ---------------- | ---------- | :------: | ---------------------------- |
| `group_id`       | string/int |   Yes    | Group ID or URL-encoded path |
| `certificate_id` | int        |   Yes    | SSH certificate ID           |

**Annotation**: Delete

---

## Security Settings

### `gitlab_get_project_security_settings`

Get all security settings for a project (secret push protection, pre-receive, etc.).

| Parameter    | Type       | Required | Description                    |
| ------------ | ---------- | :------: | ------------------------------ |
| `project_id` | string/int |   Yes    | Project ID or URL-encoded path |

**Annotation**: Read

### `gitlab_update_project_secret_push_protection`

Enable or disable secret push protection for a project.

| Parameter                        | Type       | Required | Description                              |
| -------------------------------- | ---------- | :------: | ---------------------------------------- |
| `project_id`                     | string/int |   Yes    | Project ID or URL-encoded path           |
| `secret_push_protection_enabled` | bool       |   Yes    | Whether to enable secret push protection |

**Annotation**: Update

### `gitlab_update_group_secret_push_protection`

Enable or disable secret push protection for a group.

| Parameter                        | Type       | Required | Description                              |
| -------------------------------- | ---------- | :------: | ---------------------------------------- |
| `group_id`                       | string/int |   Yes    | Group ID or URL-encoded path             |
| `secret_push_protection_enabled` | bool       |   Yes    | Whether to enable secret push protection |
| `projects_to_exclude`            | []int      |    No    | Project IDs to exclude from the setting  |

**Annotation**: Update

---

## Custom Member Roles

### `gitlab_list_instance_member_roles`

List all custom member roles at instance level.

No parameters required.

**Annotation**: Read

### `gitlab_create_instance_member_role`

Create a custom member role at instance level.

| Parameter           | Type   | Required | Description                                                                                                                                                |
| ------------------- | ------ | :------: | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `name`              | string |   Yes    | Role name                                                                                                                                                  |
| `base_access_level` | int    |   Yes    | Base access level (5=Minimal access, 10=Guest, 15=Planner, 20=Reporter, 25=Security Manager, 30=Developer, 40=Maintainer, 50=Owner; 60=Admin is not valid) |
| `description`       | string |    No    | Role description                                                                                                                                           |
| `permissions`       | object |    No    | Permission overrides (20 boolean fields)                                                                                                                   |

**Annotation**: Create

### `gitlab_delete_instance_member_role`

Delete a custom member role at instance level.

| Parameter        | Type | Required | Description    |
| ---------------- | ---- | :------: | -------------- |
| `member_role_id` | int  |   Yes    | Member role ID |

**Annotation**: Delete

### `gitlab_list_group_member_roles`

List custom member roles for a group.

| Parameter  | Type       | Required | Description                  |
| ---------- | ---------- | :------: | ---------------------------- |
| `group_id` | string/int |   Yes    | Group ID or URL-encoded path |

**Annotation**: Read

### `gitlab_create_group_member_role`

Create a custom member role for a group.

| Parameter           | Type       | Required | Description                  |
| ------------------- | ---------- | :------: | ---------------------------- |
| `group_id`          | string/int |   Yes    | Group ID or URL-encoded path |
| `name`              | string     |   Yes    | Role name                    |
| `base_access_level` | int        |   Yes    | Base access level            |
| `description`       | string     |    No    | Role description             |
| `permissions`       | object     |    No    | Permission overrides         |

**Annotation**: Create

### `gitlab_delete_group_member_role`

Delete a custom member role from a group.

| Parameter        | Type       | Required | Description                  |
| ---------------- | ---------- | :------: | ---------------------------- |
| `group_id`       | string/int |   Yes    | Group ID or URL-encoded path |
| `member_role_id` | int        |   Yes    | Member role ID               |

**Annotation**: Delete

---

## Group Credentials

### `gitlab_list_group_personal_access_tokens`

List personal access tokens managed by a group.

| Parameter  | Type       | Required | Description                           |
| ---------- | ---------- | :------: | ------------------------------------- |
| `group_id` | string/int |   Yes    | Group ID or URL-encoded path          |
| `search`   | string     |    No    | Filter tokens by name                 |
| `state`    | string     |    No    | Filter by state (`active`/`inactive`) |
| `revoked`  | bool       |    No    | Filter by revoked status              |
| `page`     | int        |    No    | Page number                           |
| `per_page` | int        |    No    | Items per page                        |

**Annotation**: Read

### `gitlab_list_group_ssh_keys`

List SSH keys managed by a group.

| Parameter  | Type       | Required | Description                  |
| ---------- | ---------- | :------: | ---------------------------- |
| `group_id` | string/int |   Yes    | Group ID or URL-encoded path |
| `page`     | int        |    No    | Page number                  |
| `per_page` | int        |    No    | Items per page               |

**Annotation**: Read

### `gitlab_revoke_group_personal_access_token`

Revoke a personal access token managed by a group.

| Parameter  | Type       | Required | Description                  |
| ---------- | ---------- | :------: | ---------------------------- |
| `group_id` | string/int |   Yes    | Group ID or URL-encoded path |
| `token_id` | int        |   Yes    | Token ID to revoke           |

**Annotation**: Delete

### `gitlab_delete_group_ssh_key`

Delete an SSH key managed by a group.

| Parameter  | Type       | Required | Description                  |
| ---------- | ---------- | :------: | ---------------------------- |
| `group_id` | string/int |   Yes    | Group ID or URL-encoded path |
| `key_id`   | int        |   Yes    | SSH key ID to delete         |

**Annotation**: Delete

---

## Group LDAP Links

### `gitlab_group_ldap_link_list`

List all LDAP group links for a GitLab group.

| Parameter  | Type       | Required | Description                  |
| ---------- | ---------- | :------: | ---------------------------- |
| `group_id` | string/int |   Yes    | Group ID or URL-encoded path |

**Annotation**: Read

### `gitlab_group_ldap_link_add`

Add an LDAP group link to a GitLab group (by CN or filter).

| Parameter        | Type       | Required | Description                                                                 |
| ---------------- | ---------- | :------: | --------------------------------------------------------------------------- |
| `group_id`       | string/int |   Yes    | Group ID or URL-encoded path                                                |
| `cn`             | string     |    No    | LDAP Common Name (CN)                                                       |
| `filter`         | string     |    No    | LDAP filter                                                                 |
| `group_access`   | int        |   Yes    | Access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner) |
| `provider`       | string     |   Yes    | LDAP provider name                                                          |
| `member_role_id` | int        |    No    | Custom member role ID                                                       |

**Annotation**: Create

### `gitlab_group_ldap_link_delete`

Delete a group LDAP link by CN or filter.

| Parameter  | Type       | Required | Description                  |
| ---------- | ---------- | :------: | ---------------------------- |
| `group_id` | string/int |   Yes    | Group ID or URL-encoded path |
| `cn`       | string     |    No    | LDAP Common Name to delete   |
| `filter`   | string     |    No    | LDAP filter to delete        |
| `provider` | string     |    No    | LDAP provider name           |

**Annotation**: Delete

### `gitlab_group_ldap_link_delete_for_provider`

Delete a group LDAP link for a specific provider.

| Parameter  | Type       | Required | Description                  |
| ---------- | ---------- | :------: | ---------------------------- |
| `group_id` | string/int |   Yes    | Group ID or URL-encoded path |
| `provider` | string     |   Yes    | LDAP provider name           |
| `cn`       | string     |   Yes    | LDAP Common Name             |

**Annotation**: Delete

### `gitlab_group_ldap_sync`

Trigger a synchronization of the group's LDAP group links. The GitLab API accepts the request and runs the sync asynchronously in the background.

| Parameter  | Type       | Required | Description                  |
| ---------- | ---------- | :------: | ---------------------------- |
| `group_id` | string/int |   Yes    | Group ID or URL-encoded path |

**Annotation**: Create (asynchronous action)

---

## Group SAML Links

### `gitlab_group_saml_link_list`

List all SAML group links for a GitLab group.

| Parameter  | Type       | Required | Description                  |
| ---------- | ---------- | :------: | ---------------------------- |
| `group_id` | string/int |   Yes    | Group ID or URL-encoded path |

**Annotation**: Read

### `gitlab_group_saml_link_get`

Get a single SAML group link by name.

| Parameter         | Type       | Required | Description                  |
| ----------------- | ---------- | :------: | ---------------------------- |
| `group_id`        | string/int |   Yes    | Group ID or URL-encoded path |
| `saml_group_name` | string     |   Yes    | Name of the SAML group       |

**Annotation**: Read

### `gitlab_group_saml_link_add`

Add a SAML group link to a GitLab group.

| Parameter         | Type       | Required | Description                                                                 |
| ----------------- | ---------- | :------: | --------------------------------------------------------------------------- |
| `group_id`        | string/int |   Yes    | Group ID or URL-encoded path                                                |
| `saml_group_name` | string     |   Yes    | Name of the SAML group                                                      |
| `access_level`    | int        |   Yes    | Access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner) |
| `member_role_id`  | int        |    No    | Custom member role ID                                                       |
| `provider`        | string     |    No    | SAML provider name                                                          |

**Annotation**: Create

### `gitlab_group_saml_link_delete`

Delete a SAML group link from a GitLab group.

| Parameter         | Type       | Required | Description                      |
| ----------------- | ---------- | :------: | -------------------------------- |
| `group_id`        | string/int |   Yes    | Group ID or URL-encoded path     |
| `saml_group_name` | string     |   Yes    | Name of the SAML group to delete |

**Annotation**: Delete

### `gitlab_group_saml_users_list`

List the users provisioned via SAML SSO for a top-level group. Supports filtering by search, exact username, active/blocked state, and creation-date window with pagination.

| Parameter        | Type       | Required | Description                                        |
| ---------------- | ---------- | :------: | -------------------------------------------------- |
| `group_id`       | string/int |   Yes    | Top-level group ID or URL-encoded path             |
| `search`         | string     |    No    | Filter by name, username, or public email          |
| `username`       | string     |    No    | Filter by an exact username                        |
| `active`         | bool       |    No    | Limit to active users only                         |
| `blocked`        | bool       |    No    | Limit to blocked users only                        |
| `created_after`  | string     |    No    | Return users created after this RFC3339 timestamp  |
| `created_before` | string     |    No    | Return users created before this RFC3339 timestamp |

**Annotation**: Read
