# Analytics & Compliance

Tools for group activity analytics, DORA metrics, project statistics, admin compliance policy settings, and project aliases.

## Group Activity Analytics (`groupanalytics`)

Retrieve counts of recently created issues, merge requests, and new members for a group
(last 90 days). These tools use the GitLab Group Activity Analytics API.

### Tools

| Tool | Description | Annotations |
| --- | --- | --- |
| `gitlab_get_recently_created_issues_count` | Get count of recently created issues (last 90 days) | Read-only |
| `gitlab_get_recently_created_mr_count` | Get count of recently created merge requests (last 90 days) | Read-only |
| `gitlab_get_recently_added_members_count` | Get count of recently added members (last 90 days) | Read-only |

### Meta-tool

Group analytics actions are now enterprise-only routes inside **`gitlab_group`** (requires the Enterprise/Premium catalog).

Actions: `issues_count`, `mr_count`, `members_count`

### Examples

Get recently created issues count:

```json
{
  "action": "issues_count",
  "group_path": "my-group"
}
```

Get recently created MR count:

```json
{
  "action": "mr_count",
  "group_path": "my-group"
}
```

Get recently added members count:

```json
{
  "action": "members_count",
  "group_path": "parent/child-group"
}
```

### Parameters

All three tools share the same parameter:

| Parameter | Type | Required | Description |
| --- | --- | --- | --- |
| `group_path` | string | Yes | Full path of the group (e.g. `my-group` or `parent/child`) |

---

## Compliance Policy Settings (`compliancepolicy`)

Manage admin-level compliance policy settings for the GitLab instance. These are
instance-wide settings that control the compliance security policy project namespace.

### Tools

| Tool | Description | Annotations |
| --- | --- | --- |
| `gitlab_get_compliance_policy_settings` | Get compliance policy settings | Read-only |
| `gitlab_update_compliance_policy_settings` | Update compliance policy settings | Update |

### Meta-tool

**`gitlab_compliance_policy`** — Manage admin compliance policy settings.

Actions: `get`, `update`

### Examples

Get current settings:

```json
{
  "action": "get"
}
```

Update compliance security policy namespace:

```json
{
  "action": "update",
  "csp_namespace_id": 42
}
```

### Parameters

#### Get

No parameters required.

#### Update

| Parameter | Type | Required | Description |
| --- | --- | --- | --- |
| `csp_namespace_id` | int | No | Namespace ID for the compliance security policy project |

---

## Project Aliases (`projectaliases`)

Manage project aliases (admin-only). Project aliases allow accessing projects via
alternative names, providing a convenient shortcut. All operations require admin privileges.

### Tools

| Tool | Description | Annotations |
| --- | --- | --- |
| `gitlab_list_project_aliases` | List all project aliases | Read-only |
| `gitlab_get_project_alias` | Get a specific project alias by name | Read-only |
| `gitlab_create_project_alias` | Create a new project alias | Create |
| `gitlab_delete_project_alias` | Delete a project alias by name | Destructive |

### Meta-tool

**`gitlab_project_alias`** — Manage project aliases (admin-only).

Actions: `list`, `get`, `create`, `delete`

### Examples

List all aliases:

```json
{
  "action": "list"
}
```

Get an alias:

```json
{
  "action": "get",
  "name": "my-alias"
}
```

Create an alias:

```json
{
  "action": "create",
  "name": "my-alias",
  "project_id": 123
}
```

Delete an alias:

```json
{
  "action": "delete",
  "name": "my-alias"
}
```

### Parameters

#### List

No parameters required.

#### Get / Delete

| Parameter | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | Yes | The alias name |

#### Create

| Parameter | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | Yes | The alias name to create |
| `project_id` | int | Yes | The numeric project ID to alias |

---

## DORA Metrics (`dorametrics`)

Retrieve [DORA metrics](https://docs.gitlab.com/ee/api/dora/metrics.html) for projects
and groups. DORA measures deployment frequency, lead time for changes, time to
restore service, and change failure rate. Requires GitLab Premium / Ultimate.

### Tools

| Tool | Description | Annotations |
| --- | --- | --- |
| `gitlab_get_project_dora_metrics` | Get DORA metrics for a project | Read-only |
| `gitlab_get_group_dora_metrics` | Get DORA metrics for a group | Read-only |

### Meta-tool

**`gitlab_dora_metrics`** — retrieve DORA metrics (requires the Enterprise/Premium catalog).

Actions: `project`, `group`

### Parameters

| Parameter | Type | Required | Description |
| --- | --- | --- | --- |
| `project_id` / `group_id` | string | Yes | Project or group ID / URL-encoded path |
| `metric` | string | Yes | `deployment_frequency`, `lead_time_for_changes`, `time_to_restore_service`, `change_failure_rate` |
| `start_date` | string | No | Start date (`YYYY-MM-DD`) |
| `end_date` | string | No | End date (`YYYY-MM-DD`) |
| `interval` | string | No | Aggregation: `daily`, `monthly`, `all` (default: `daily`) |
| `environment_tiers` | []string | No | Filter by tiers (e.g. `production`, `staging`) |

---

## Project Statistics (`projectstatistics`)

### Tools

| Tool | Description | Annotations |
| --- | --- | --- |
| `gitlab_get_project_statistics` | Get project fetch statistics for the last 30 days | Read-only |

### Parameters

| Parameter | Type | Required | Description |
| --- | --- | --- | --- |
| `project_id` | string | Yes | Project ID or URL-encoded path |
