# Identity & Security — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Identity & Security
> **Individual tools**: 28
> **Meta-tools**: `gitlab_group_scim`, `gitlab_member_role` (when `META_TOOLS=true` and the Enterprise/Premium catalog is enabled); `gitlab_group_ssh_certificate`, `gitlab_security_settings`, `gitlab_group_credential` are now enterprise-only routes inside `gitlab_group`/`gitlab_project`
> **GitLab API**: [SCIM API](https://docs.gitlab.com/ee/api/scim.html) · [Group SSH Certificates API](https://docs.gitlab.com/ee/api/group_ssh_certificates.html) · [Security Settings API](https://docs.gitlab.com/ee/api/project_security_settings.html) · [Member Roles API](https://docs.gitlab.com/ee/api/member_roles.html) · [Group Credentials API](https://docs.gitlab.com/ee/api/group_credentials.html) · [LDAP Group Links API](https://docs.gitlab.com/ee/api/group_level_mr_approvals.html) · [SAML Group Links API](https://docs.gitlab.com/ee/api/groups.html#saml-group-links)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The identity & security domain covers SCIM identity management for groups, SSH certificate management, project and group security settings (secret push protection), custom member roles at instance and group level, group credential inventory (personal access tokens and SSH keys), LDAP group link management, and SAML group link management.

When `META_TOOLS=true` (the default) and the Enterprise/Premium catalog is enabled, the 20 individual tools below are consolidated into meta-tools. `gitlab_group_scim` and `gitlab_member_role` are standalone enterprise meta-tools, while SSH certificates, security settings, and group credentials are routes inside `gitlab_group`/`gitlab_project`.

### Common Questions

> "List SCIM identities for my group"
> "Create an SSH certificate for a group"
> "Get security settings for project 42"
> "List custom member roles for my group"
> "Show personal access tokens managed by a group"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Group SCIM Identities

### `gitlab_list_group_scim_identities`

List all SCIM identities for a group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |

**Annotation**: Read

### `gitlab_get_group_scim_identity`

Get a single SCIM identity by external UID.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `uid` | string | Yes | SCIM external UID |

**Annotation**: Read

### `gitlab_update_group_scim_identity`

Update a SCIM identity's external UID.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `uid` | string | Yes | Current SCIM external UID |
| `extern_uid` | string | Yes | New external UID value |

**Annotation**: Update

### `gitlab_delete_group_scim_identity`

Delete a SCIM identity from a group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `uid` | string | Yes | SCIM external UID |

**Annotation**: Delete

---

## Group SSH Certificates

### `gitlab_list_group_ssh_certificates`

List SSH certificates for a group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |

**Annotation**: Read

### `gitlab_create_group_ssh_certificate`

Create an SSH certificate for a group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `key` | string | Yes | SSH public key |
| `title` | string | Yes | Certificate title |

**Annotation**: Create

### `gitlab_delete_group_ssh_certificate`

Delete an SSH certificate from a group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `certificate_id` | int | Yes | SSH certificate ID |

**Annotation**: Delete

---

## Security Settings

### `gitlab_get_project_security_settings`

Get all security settings for a project (secret push protection, pre-receive, etc.).

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `project_id` | string/int | Yes | Project ID or URL-encoded path |

**Annotation**: Read

### `gitlab_update_project_secret_push_protection`

Enable or disable secret push protection for a project.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `project_id` | string/int | Yes | Project ID or URL-encoded path |
| `secret_push_protection_enabled` | bool | Yes | Whether to enable secret push protection |

**Annotation**: Update

### `gitlab_update_group_secret_push_protection`

Enable or disable secret push protection for a group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `secret_push_protection_enabled` | bool | Yes | Whether to enable secret push protection |
| `projects_to_exclude` | []int | No | Project IDs to exclude from the setting |

**Annotation**: Update

---

## Custom Member Roles

### `gitlab_list_instance_member_roles`

List all custom member roles at instance level.

No parameters required.

**Annotation**: Read

### `gitlab_create_instance_member_role`

Create a custom member role at instance level.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `name` | string | Yes | Role name |
| `base_access_level` | int | Yes | Base access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner) |
| `description` | string | No | Role description |
| `permissions` | object | No | Permission overrides (20 boolean fields) |

**Annotation**: Create

### `gitlab_delete_instance_member_role`

Delete a custom member role at instance level.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `member_role_id` | int | Yes | Member role ID |

**Annotation**: Delete

### `gitlab_list_group_member_roles`

List custom member roles for a group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |

**Annotation**: Read

### `gitlab_create_group_member_role`

Create a custom member role for a group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `name` | string | Yes | Role name |
| `base_access_level` | int | Yes | Base access level |
| `description` | string | No | Role description |
| `permissions` | object | No | Permission overrides |

**Annotation**: Create

### `gitlab_delete_group_member_role`

Delete a custom member role from a group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `member_role_id` | int | Yes | Member role ID |

**Annotation**: Delete

---

## Group Credentials

### `gitlab_list_group_personal_access_tokens`

List personal access tokens managed by a group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `search` | string | No | Filter tokens by name |
| `state` | string | No | Filter by state (`active`/`inactive`) |
| `revoked` | bool | No | Filter by revoked status |
| `page` | int | No | Page number |
| `per_page` | int | No | Items per page |

**Annotation**: Read

### `gitlab_list_group_ssh_keys`

List SSH keys managed by a group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `page` | int | No | Page number |
| `per_page` | int | No | Items per page |

**Annotation**: Read

### `gitlab_revoke_group_personal_access_token`

Revoke a personal access token managed by a group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `token_id` | int | Yes | Token ID to revoke |

**Annotation**: Delete

### `gitlab_delete_group_ssh_key`

Delete an SSH key managed by a group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `key_id` | int | Yes | SSH key ID to delete |

**Annotation**: Delete

---

## Group LDAP Links

### `gitlab_group_ldap_link_list`

List all LDAP group links for a GitLab group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |

**Annotation**: Read

### `gitlab_group_ldap_link_add`

Add an LDAP group link to a GitLab group (by CN or filter).

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `cn` | string | No | LDAP Common Name (CN) |
| `filter` | string | No | LDAP filter |
| `group_access` | int | Yes | Access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner) |
| `provider` | string | Yes | LDAP provider name |
| `member_role_id` | int | No | Custom member role ID |

**Annotation**: Create

### `gitlab_group_ldap_link_delete`

Delete a group LDAP link by CN or filter.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `cn` | string | No | LDAP Common Name to delete |
| `filter` | string | No | LDAP filter to delete |
| `provider` | string | No | LDAP provider name |

**Annotation**: Delete

### `gitlab_group_ldap_link_delete_for_provider`

Delete a group LDAP link for a specific provider.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `provider` | string | Yes | LDAP provider name |
| `cn` | string | Yes | LDAP Common Name |

**Annotation**: Delete

---

## Group SAML Links

### `gitlab_group_saml_link_list`

List all SAML group links for a GitLab group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |

**Annotation**: Read

### `gitlab_group_saml_link_get`

Get a single SAML group link by name.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `saml_group_name` | string | Yes | Name of the SAML group |

**Annotation**: Read

### `gitlab_group_saml_link_add`

Add a SAML group link to a GitLab group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `saml_group_name` | string | Yes | Name of the SAML group |
| `access_level` | int | Yes | Access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner) |
| `member_role_id` | int | No | Custom member role ID |
| `provider` | string | No | SAML provider name |

**Annotation**: Create

### `gitlab_group_saml_link_delete`

Delete a SAML group link from a GitLab group.

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `saml_group_name` | string | Yes | Name of the SAML group to delete |

**Annotation**: Delete

---

## Meta-Tool Reference

When `META_TOOLS=true`, the following meta-tools replace the individual tools above:

### `gitlab_group_scim`

Manage SCIM identities for a group (enterprise-only, requires the Enterprise/Premium catalog). Actions: `list`, `get`, `update`, `delete`.

### `gitlab_group_ssh_certificate`

SSH certificate management is now available as enterprise-only routes inside **`gitlab_group`** (requires the Enterprise/Premium catalog). Actions: `ssh_certificate_list`, `ssh_certificate_create`, `ssh_certificate_delete`.

### `gitlab_security_settings`

Security settings are now available as enterprise-only routes split between **`gitlab_project`** and **`gitlab_group`** (requires the Enterprise/Premium catalog). Project actions: `security_settings_get`, `security_settings_update`. Group actions: `security_settings_update`.

### `gitlab_member_role`

Manage custom member roles at instance and group level (enterprise-only, requires the Enterprise/Premium catalog). Actions: `list_instance`, `create_instance`, `delete_instance`, `list_group`, `create_group`, `delete_group`.

### `gitlab_group_credential`

Group credential management is now available as enterprise-only routes inside **`gitlab_group`** (requires the Enterprise/Premium catalog). Actions: `credential_list_pats`, `credential_list_ssh_keys`, `credential_revoke_pat`, `credential_delete_ssh_key`.
