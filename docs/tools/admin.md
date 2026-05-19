# Administration — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Administration
> **Individual tools**: 86
> **Meta-tools**: `gitlab_admin` (consolidated, covers 15 sub-packages), `gitlab_page`, `gitlab_terraform_state`, `gitlab_cluster_agent`, `gitlab_avatar`, `gitlab_dependency_proxy` (`TOOL_SURFACE=meta` catalog)
> **GitLab API**: [Settings](https://docs.gitlab.com/ee/api/settings.html) · [Appearance](https://docs.gitlab.com/ee/api/appearance.html) · [Broadcast Messages](https://docs.gitlab.com/ee/api/broadcast_messages.html) · [Features](https://docs.gitlab.com/ee/api/features.html) · [License](https://docs.gitlab.com/ee/api/license.html) · [System Hooks](https://docs.gitlab.com/ee/api/system_hooks.html) · [Sidekiq](https://docs.gitlab.com/ee/api/sidekiq_metrics.html) · [Plan Limits](https://docs.gitlab.com/ee/api/plan_limits.html) · [Usage Data](https://docs.gitlab.com/ee/api/usage_data.html) · [Pages](https://docs.gitlab.com/ee/api/pages.html) · [Terraform States](https://docs.gitlab.com/ee/api/terraform_state.html) · [Cluster Agents](https://docs.gitlab.com/ee/api/cluster_agents.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The administration domain covers instance-level settings, appearance, broadcast messages, admin feature flags, licensing, system hooks, Sidekiq metrics, plan limits, usage data, database migrations, OAuth2 applications, application statistics, instance metadata, custom attributes, bulk imports, avatars, dependency proxy, GitLab Pages, Terraform states, and cluster agents.

With `TOOL_SURFACE=meta`, the smaller sub-packages (settings through bulk imports) are consolidated into a single `gitlab_admin` meta-tool. The larger sub-packages — pages, terraform states, cluster agents, avatar, and dependency proxy — each have their own meta-tool.

### Common Questions

> "Show the GitLab server settings"
> "List active broadcast messages"
> "Check the server version"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description                                    |
| ---------- | :------: | :---------: | :--------: | ---------------------------------------------- |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation                       |
| **Create** |    —     |     No      |     —      | Creates a new resource                         |
| **Update** |    —     |     No      |    Yes     | Modifies an existing resource                  |
| **Delete** |    —     |     Yes     |    Yes     | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Application Settings

### `gitlab_get_settings`

Get current application settings. Requires admin access. Returns all instance-level settings as key-value pairs.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_update_settings`

Update application settings. Requires admin access. Pass settings as key-value map with snake_case keys matching GitLab API (e.g. signup_enabled, default_project_visibility).

| Annotation | **Update** |
| ---------- | ---------- |

---

## Appearance

### `gitlab_get_appearance`

Get current application appearance settings. Requires admin access.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_update_appearance`

Update application appearance (title, description, messages, PWA settings). Requires admin access.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Broadcast Messages

### `gitlab_list_broadcast_messages`

List all broadcast messages. Requires admin access.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_broadcast_message`

Get a specific broadcast message by ID. Requires admin access.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_broadcast_message`

Create a broadcast message. Requires admin access.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_update_broadcast_message`

Update a broadcast message. Requires admin access.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_broadcast_message`

Delete a broadcast message. Requires admin access.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Admin Feature Flags

### `gitlab_list_features`

List all feature flags (admin). Returns name, state and gates for each flag.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_list_feature_definitions`

List all feature definitions (admin). Returns name, type, group, milestone and default_enabled for each definition.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_set_feature_flag`

Set or create a feature flag (admin). Requires name and value. Supports scoping to user, group, project, namespace, or repository.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_delete_feature_flag`

Delete a feature flag (admin). Requires the flag name.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## License

### `gitlab_get_license`

Get current GitLab license information (admin). Returns plan, expiry, user counts and licensee.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_add_license`

Add a new GitLab license (admin). Requires the Base64-encoded license string.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_delete_license`

Delete a GitLab license by ID (admin).

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## System Hooks

### `gitlab_list_system_hooks`

List all system hooks (admin). Returns ID, URL and event subscriptions.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_system_hook`

Get a system hook by ID (admin).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_add_system_hook`

Add a new system hook (admin). Requires URL. Optionally configure event subscriptions, SSL verification, payload token, and write-only signing token.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_edit_system_hook`

Edit an existing system hook by ID (admin). Supports URL, metadata, event subscriptions, SSL verification, payload token, and write-only signing token updates.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_test_system_hook`

Test a system hook by ID (admin). Triggers a test event and returns the result.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_set_system_hook_url_variable`

Create or update a URL variable for a system hook (admin). Variables can be referenced as placeholders in the hook URL.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_system_hook_url_variable`

Delete a URL variable from a system hook (admin).

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_delete_system_hook`

Delete a system hook by ID (admin).

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Sidekiq Metrics

### `gitlab_get_sidekiq_queue_metrics`

Get Sidekiq queue metrics (admin). Returns backlog and latency for all queues.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_sidekiq_process_metrics`

Get Sidekiq process metrics (admin). Returns information about running Sidekiq processes.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_sidekiq_job_stats`

Get Sidekiq job statistics (admin). Returns processed, failed, and enqueued counts.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_sidekiq_compound_metrics`

Get all Sidekiq metrics in a single compound response (admin). Returns queue metrics, process metrics, and job statistics combined.

| Annotation | **Read** |
| ---------- | -------- |

---

## Plan Limits

### `gitlab_get_plan_limits`

Get current plan limits (admin). Optionally filter by plan name (default, free, bronze, silver, gold, premium, ultimate).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_change_plan_limits`

Change plan limits (admin). Requires plan_name; optionally set individual file size limits.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Usage Data

### `gitlab_get_service_ping`

Get service ping data (admin). Returns recorded_at, license info, and usage counts.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_non_sql_metrics`

Get non-SQL service ping metrics (admin). Returns instance info, license details, and settings.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_usage_queries`

Get service ping SQL queries (admin). Returns the raw SQL queries used for service ping collection.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_metric_definitions`

Get metric definitions as YAML (admin). Returns all metric definitions used in service ping.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_track_event`

Track a single usage event. Params: event (required), send_to_snowplow, namespace_id, project_id.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_track_events`

Track multiple usage events in batch. Params: events (required, array of event objects).

| Annotation | **Create** |
| ---------- | ---------- |

---

## Database Migrations

### `gitlab_mark_migration`

Mark a pending database migration as successfully executed (admin). Params: version (required), database (optional).

| Annotation | **Update** |
| ---------- | ---------- |

---

## OAuth2 Applications

### `gitlab_list_applications`

List all OAuth2 applications (admin). Params: page, per_page.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_application`

Create an OAuth2 application (admin). Params: name (required), redirect_uri (required), scopes (required), confidential.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_delete_application`

Delete an OAuth2 application (admin). Params: id (required).

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Application Statistics

### `gitlab_get_application_statistics`

Get application statistics (admin). Returns counts for users, projects, groups, issues, MRs, etc.

| Annotation | **Read** |
| ---------- | -------- |

---

## Instance Metadata

### `gitlab_get_metadata`

Get GitLab instance metadata (version, revision, KAS info, enterprise flag).

| Annotation | **Read** |
| ---------- | -------- |

---

## Custom Attributes

### `gitlab_list_custom_attributes`

List custom attributes for a user, group, or project (admin). Params: resource_type (required: user|group|project), resource_id (required).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_custom_attribute`

Get a custom attribute by key for a user, group, or project (admin). Params: resource_type (required), resource_id (required), key (required).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_set_custom_attribute`

Set (create/update) a custom attribute for a user, group, or project (admin). Params: resource_type (required), resource_id (required), key (required), value (required).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_delete_custom_attribute`

Delete a custom attribute for a user, group, or project (admin). Params: resource_type (required), resource_id (required), key (required).

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Bulk Imports

### `gitlab_start_bulk_import`

Start a new group or project bulk import migration (admin). Requires source GitLab URL, access token, and entities to migrate.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_list_bulk_imports`

List all group or project bulk import migrations visible to the caller. Optionally filter by status.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_bulk_import`

Get details of a single bulk import migration by ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_cancel_bulk_import`

Cancel an in-progress bulk import migration. Returns the migration with updated status.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_list_bulk_import_entities`

List bulk import migration entities. When `bulk_import_id` is provided, scopes to that import; otherwise returns all entities visible to the caller. Optionally filter by status.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_bulk_import_entity`

Get details of a single bulk import migration entity by `bulk_import_id` and `entity_id`.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_list_bulk_import_entity_failures`

List failed import records for a bulk import migration entity. Useful for diagnosing failed migrations.

| Annotation | **Read** |
| ---------- | -------- |

---

## Avatar

### `gitlab_get_avatar`

Get the avatar URL for an email address.

| Annotation | **Read** |
| ---------- | -------- |

---

## Dependency Proxy

### `gitlab_purge_dependency_proxy`

Purge the dependency proxy cache for a GitLab group.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## GitLab Pages

### `gitlab_pages_get`

Get Pages settings for a project. Returns URL, unique domain status, HTTPS enforcement, deployments, and primary domain.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pages_update`

Update Pages settings for a project. Can configure unique domain, HTTPS enforcement, and primary domain.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_pages_unpublish`

Unpublish Pages for a project. Removes all published Pages content.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_pages_domain_list_all`

List all Pages domains across all projects accessible to the authenticated user.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pages_domain_list`

List Pages domains for a specific project. Supports pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pages_domain_get`

Get a single Pages domain for a project, including certificate details.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pages_domain_create`

Create a new Pages domain for a project. Optionally configure SSL certificate.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_pages_domain_update`

Update an existing Pages domain for a project. Can update SSL settings.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_pages_domain_delete`

Delete a Pages domain from a project.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Terraform States

### `gitlab_list_terraform_states`

List Terraform states for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_terraform_state`

Get details of a Terraform state.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_delete_terraform_state`

Delete a Terraform state.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_delete_terraform_state_version`

Delete a specific version of a Terraform state.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_lock_terraform_state`

Lock a Terraform state.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_unlock_terraform_state`

Unlock a Terraform state.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Cluster Agents

### `gitlab_list_cluster_agents`

List cluster agents for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_cluster_agent`

Get details of a cluster agent.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_register_cluster_agent`

Register a new cluster agent for a GitLab project.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_delete_cluster_agent`

Delete a cluster agent.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_list_cluster_agent_tokens`

List tokens for a cluster agent.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_cluster_agent_token`

Get details of a cluster agent token.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_cluster_agent_token`

Create a token for a cluster agent.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_revoke_cluster_agent_token`

Revoke a cluster agent token.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Audit Events

### `gitlab_list_instance_audit_events`

List instance-level audit events (admin only). Supports filtering by date range.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_instance_audit_event`

Get a single instance-level audit event by ID (admin only).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_list_group_audit_events`

List audit events for a GitLab group. Supports filtering by date range.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_group_audit_event`

Get a single group-level audit event by ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_list_project_audit_events`

List audit events for a GitLab project. Supports filtering by date range.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_project_audit_event`

Get a single project-level audit event by ID.

| Annotation | **Read** |
| ---------- | -------- |

---

## Server Update

### `gitlab_server_check_update`

Check if a newer version of the MCP server is available. Returns current version, latest version, release URL, and release notes.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_server_apply_update`

Download and apply the latest MCP server update. On Linux/macOS the binary is replaced atomically. On Windows the update is downloaded to a staging path with an update script.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_get_settings` | Settings | Read |
| 2 | `gitlab_update_settings` | Settings | Update |
| 3 | `gitlab_get_appearance` | Appearance | Read |
| 4 | `gitlab_update_appearance` | Appearance | Update |
| 5 | `gitlab_list_broadcast_messages` | Broadcast Messages | Read |
| 6 | `gitlab_get_broadcast_message` | Broadcast Messages | Read |
| 7 | `gitlab_create_broadcast_message` | Broadcast Messages | Create |
| 8 | `gitlab_update_broadcast_message` | Broadcast Messages | Update |
| 9 | `gitlab_delete_broadcast_message` | Broadcast Messages | Delete |
| 10 | `gitlab_list_features` | Admin Features | Read |
| 11 | `gitlab_list_feature_definitions` | Admin Features | Read |
| 12 | `gitlab_set_feature_flag` | Admin Features | Create |
| 13 | `gitlab_delete_feature_flag` | Admin Features | Delete |
| 14 | `gitlab_get_license` | License | Read |
| 15 | `gitlab_add_license` | License | Create |
| 16 | `gitlab_delete_license` | License | Delete |
| 17 | `gitlab_list_system_hooks` | System Hooks | Read |
| 18 | `gitlab_get_system_hook` | System Hooks | Read |
| 19 | `gitlab_add_system_hook` | System Hooks | Create |
| 20 | `gitlab_edit_system_hook` | System Hooks | Update |
| 21 | `gitlab_test_system_hook` | System Hooks | Read |
| 22 | `gitlab_set_system_hook_url_variable` | System Hooks | Update |
| 23 | `gitlab_delete_system_hook_url_variable` | System Hooks | Delete |
| 24 | `gitlab_delete_system_hook` | System Hooks | Delete |
| 25 | `gitlab_get_sidekiq_queue_metrics` | Sidekiq | Read |
| 26 | `gitlab_get_sidekiq_process_metrics` | Sidekiq | Read |
| 27 | `gitlab_get_sidekiq_job_stats` | Sidekiq | Read |
| 28 | `gitlab_get_sidekiq_compound_metrics` | Sidekiq | Read |
| 29 | `gitlab_get_plan_limits` | Plan Limits | Read |
| 30 | `gitlab_change_plan_limits` | Plan Limits | Update |
| 31 | `gitlab_get_service_ping` | Usage Data | Read |
| 32 | `gitlab_get_non_sql_metrics` | Usage Data | Read |
| 33 | `gitlab_get_usage_queries` | Usage Data | Read |
| 34 | `gitlab_get_metric_definitions` | Usage Data | Read |
| 35 | `gitlab_track_event` | Usage Data | Create |
| 36 | `gitlab_track_events` | Usage Data | Create |
| 37 | `gitlab_mark_migration` | DB Migrations | Update |
| 38 | `gitlab_list_applications` | Applications | Read |
| 39 | `gitlab_create_application` | Applications | Create |
| 40 | `gitlab_delete_application` | Applications | Delete |
| 41 | `gitlab_get_application_statistics` | Statistics | Read |
| 42 | `gitlab_get_metadata` | Metadata | Read |
| 43 | `gitlab_list_custom_attributes` | Custom Attributes | Read |
| 44 | `gitlab_get_custom_attribute` | Custom Attributes | Read |
| 45 | `gitlab_set_custom_attribute` | Custom Attributes | Create |
| 46 | `gitlab_delete_custom_attribute` | Custom Attributes | Delete |
| 47 | `gitlab_start_bulk_import` | Bulk Imports | Create |
| 48 | `gitlab_list_bulk_imports` | Bulk Imports | Read |
| 49 | `gitlab_get_bulk_import` | Bulk Imports | Read |
| 50 | `gitlab_cancel_bulk_import` | Bulk Imports | Update |
| 51 | `gitlab_list_bulk_import_entities` | Bulk Imports | Read |
| 52 | `gitlab_get_bulk_import_entity` | Bulk Imports | Read |
| 53 | `gitlab_list_bulk_import_entity_failures` | Bulk Imports | Read |
| 54 | `gitlab_get_avatar` | Avatar | Read |
| 55 | `gitlab_purge_dependency_proxy` | Dependency Proxy | Delete |
| 56 | `gitlab_pages_get` | Pages | Read |
| 57 | `gitlab_pages_update` | Pages | Update |
| 58 | `gitlab_pages_unpublish` | Pages | Delete |
| 59 | `gitlab_pages_domain_list_all` | Pages | Read |
| 60 | `gitlab_pages_domain_list` | Pages | Read |
| 61 | `gitlab_pages_domain_get` | Pages | Read |
| 62 | `gitlab_pages_domain_create` | Pages | Create |
| 63 | `gitlab_pages_domain_update` | Pages | Update |
| 64 | `gitlab_pages_domain_delete` | Pages | Delete |
| 65 | `gitlab_list_terraform_states` | Terraform States | Read |
| 66 | `gitlab_get_terraform_state` | Terraform States | Read |
| 67 | `gitlab_delete_terraform_state` | Terraform States | Delete |
| 68 | `gitlab_delete_terraform_state_version` | Terraform States | Delete |
| 69 | `gitlab_lock_terraform_state` | Terraform States | Update |
| 70 | `gitlab_unlock_terraform_state` | Terraform States | Update |
| 71 | `gitlab_list_cluster_agents` | Cluster Agents | Read |
| 72 | `gitlab_get_cluster_agent` | Cluster Agents | Read |
| 73 | `gitlab_register_cluster_agent` | Cluster Agents | Create |
| 74 | `gitlab_delete_cluster_agent` | Cluster Agents | Delete |
| 75 | `gitlab_list_cluster_agent_tokens` | Cluster Agents | Read |
| 76 | `gitlab_get_cluster_agent_token` | Cluster Agents | Read |
| 77 | `gitlab_create_cluster_agent_token` | Cluster Agents | Create |
| 78 | `gitlab_revoke_cluster_agent_token` | Cluster Agents | Delete |
| 79 | `gitlab_list_instance_audit_events` | Audit Events | Read |
| 80 | `gitlab_get_instance_audit_event` | Audit Events | Read |
| 81 | `gitlab_list_group_audit_events` | Audit Events | Read |
| 82 | `gitlab_get_group_audit_event` | Audit Events | Read |
| 83 | `gitlab_list_project_audit_events` | Audit Events | Read |
| 84 | `gitlab_get_project_audit_event` | Audit Events | Read |
| 85 | `gitlab_server_check_update` | Server Update | Read |
| 86 | `gitlab_server_apply_update` | Server Update | Update |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_delete_broadcast_message` — deletes a broadcast message
- `gitlab_delete_feature_flag` — deletes an admin feature flag
- `gitlab_delete_license` — deletes a GitLab license
- `gitlab_delete_system_hook_url_variable` — deletes a system hook URL variable
- `gitlab_delete_system_hook` — deletes a system hook
- `gitlab_delete_application` — deletes an OAuth2 application
- `gitlab_delete_custom_attribute` — deletes a custom attribute
- `gitlab_purge_dependency_proxy` — purges the dependency proxy cache
- `gitlab_pages_unpublish` — unpublishes Pages content
- `gitlab_pages_domain_delete` — deletes a Pages domain
- `gitlab_delete_terraform_state` — deletes a Terraform state
- `gitlab_delete_terraform_state_version` — deletes a Terraform state version
- `gitlab_delete_cluster_agent` — deletes a cluster agent
- `gitlab_revoke_cluster_agent_token` — revokes a cluster agent token

---

## Related

- [GitLab Application Settings API](https://docs.gitlab.com/ee/api/settings.html)
- [GitLab Appearance API](https://docs.gitlab.com/ee/api/appearance.html)
- [GitLab Broadcast Messages API](https://docs.gitlab.com/ee/api/broadcast_messages.html)
- [GitLab Features API](https://docs.gitlab.com/ee/api/features.html)
- [GitLab License API](https://docs.gitlab.com/ee/api/license.html)
- [GitLab System Hooks API](https://docs.gitlab.com/ee/api/system_hooks.html)
- [GitLab Sidekiq Metrics API](https://docs.gitlab.com/ee/api/sidekiq_metrics.html)
- [GitLab Plan Limits API](https://docs.gitlab.com/ee/api/plan_limits.html)
- [GitLab Usage Data API](https://docs.gitlab.com/ee/api/usage_data.html)
- [GitLab Database Migrations API](https://docs.gitlab.com/ee/api/database_migrations.html)
- [GitLab Applications API](https://docs.gitlab.com/ee/api/applications.html)
- [GitLab Statistics API](https://docs.gitlab.com/ee/api/statistics.html)
- [GitLab Metadata API](https://docs.gitlab.com/ee/api/metadata.html)
- [GitLab Custom Attributes API](https://docs.gitlab.com/ee/api/custom_attributes.html)
- [GitLab Bulk Imports API](https://docs.gitlab.com/ee/api/bulk_imports.html)
- [GitLab Avatar API](https://docs.gitlab.com/ee/api/avatar.html)
- [GitLab Dependency Proxy API](https://docs.gitlab.com/ee/api/dependency_proxy.html)
- [GitLab Pages API](https://docs.gitlab.com/ee/api/pages.html)
- [GitLab Terraform States API](https://docs.gitlab.com/ee/api/terraform_state.html)
- [GitLab Cluster Agents API](https://docs.gitlab.com/ee/api/cluster_agents.html)
- [GitLab Audit Events API](https://docs.gitlab.com/ee/api/audit_events.html)
