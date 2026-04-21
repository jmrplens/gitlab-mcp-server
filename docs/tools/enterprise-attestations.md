# Enterprise Users & Attestations

Tools for managing enterprise users at the group level and build attestations at the project level.

## Enterprise Users (`enterpriseusers`)

Enterprise users are managed at the top-level group and represent users provisioned through SSO/SCIM
or direct group management.

### Tools

| Tool | Description | Annotations |
| --- | --- | --- |
| `gitlab_list_enterprise_users` | List all enterprise users for a group | Read-only |
| `gitlab_get_enterprise_user` | Get details of a specific enterprise user | Read-only |
| `gitlab_disable_2fa_enterprise_user` | Disable two-factor authentication for a user | Update |
| `gitlab_delete_enterprise_user` | Delete an enterprise user (soft or hard delete) | Destructive |

### Meta-tool

**`gitlab_enterprise_user`** — Manage enterprise users for a GitLab group.

Actions: `list`, `get`, `disable_2fa`, `delete`

### Examples

List enterprise users:

```json
{
  "action": "list",
  "group_id": "my-group",
  "search": "alice",
  "active": true
}
```

Get a specific user:

```json
{
  "action": "get",
  "group_id": "my-group",
  "user_id": 42
}
```

Disable 2FA for a user:

```json
{
  "action": "disable_2fa",
  "group_id": "my-group",
  "user_id": 42
}
```

Hard delete a user:

```json
{
  "action": "delete",
  "group_id": "my-group",
  "user_id": 42,
  "hard_delete": true
}
```

### Parameters

#### List

| Parameter | Type | Required | Description |
| --- | --- | --- | --- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `username` | string | No | Filter by exact username |
| `search` | string | No | Search by name, username, or email |
| `active` | bool | No | Filter for active users |
| `blocked` | bool | No | Filter for blocked users |
| `created_after` | string | No | ISO 8601 date filter |
| `created_before` | string | No | ISO 8601 date filter |
| `two_factor` | string | No | Filter by 2FA: `enabled` or `disabled` |
| `page` | int | No | Page number |
| `per_page` | int | No | Items per page (max 100) |

#### Get / Disable 2FA / Delete

| Parameter | Type | Required | Description |
| --- | --- | --- | --- |
| `group_id` | string/int | Yes | Group ID or URL-encoded path |
| `user_id` | int | Yes | User ID |
| `hard_delete` | bool | No | Permanently delete (delete only) |

---

## Attestations (`attestations`)

Build attestations provide SLSA (Supply-chain Levels for Software Artifacts) provenance
information for CI/CD builds. They are scoped to a project and identified by subject digest.

### Tools

| Tool | Description | Annotations |
| --- | --- | --- |
| `gitlab_list_attestations` | List attestations matching a subject digest | Read-only |
| `gitlab_download_attestation` | Download attestation content (base64-encoded) | Read-only |

### Meta-tool

**`gitlab_attestation`** — Manage build attestations for a GitLab project.

Actions: `list`, `download`

### Examples

List attestations by digest:

```json
{
  "action": "list",
  "project_id": "my-project",
  "subject_digest": "sha256:abc123def456"
}
```

Download an attestation:

```json
{
  "action": "download",
  "project_id": "my-project",
  "attestation_iid": 1
}
```

### Parameters

#### List

| Parameter | Type | Required | Description |
| --- | --- | --- | --- |
| `project_id` | string/int | Yes | Project ID or URL-encoded path |
| `subject_digest` | string | Yes | Subject digest hash to filter attestations |

#### Download

| Parameter | Type | Required | Description |
| --- | --- | --- | --- |
| `project_id` | string/int | Yes | Project ID or URL-encoded path |
| `attestation_iid` | int | Yes | Attestation IID (project-scoped) |

### Response

The download tool returns:

| Field | Description |
| --- | --- |
| `attestation_iid` | The IID of the downloaded attestation |
| `size` | Size in bytes |
| `content_base64` | Base64-encoded binary content |
