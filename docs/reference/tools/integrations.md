# Integrations — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Integrations, Badges, Topics, Import
> **Individual tools**: 29
> **Meta-tools**: `gitlab_project` (integrations + badges), `gitlab_admin` (topics and imports) (`GITLAB_MCP_TOOL_SURFACE=meta` catalog)
> **Dynamic IDs**: `admin.*`, `group.*`, `project.*` (default surface, via `gitlab_execute_action`)
> **GitLab API**: [Integrations API](https://docs.gitlab.com/api/project_integrations/), [Badges API](https://docs.gitlab.com/ee/api/project_badges.html), [Topics API](https://docs.gitlab.com/ee/api/topics.html), [Import API](https://docs.gitlab.com/ee/api/import.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The integrations domain covers miscellaneous GitLab tools that don't belong to other major domains: project/group integrations (services), project/group badges, instance-level topics, and repository import from external services (GitHub, Bitbucket).

On the default dynamic surface, these operations are the `admin.*`, `group.*`, `project.*` entries of the canonical action catalog: find them with `gitlab_find_action` and run them with `gitlab_execute_action` by `domain.action` ID. With `GITLAB_MCP_TOOL_SURFACE=individual`, each is the tool named in the tables below.

With `GITLAB_MCP_TOOL_SURFACE=meta`, integration and badge tools are consolidated into `gitlab_project`, and topic and import tools into `gitlab_admin`.

### Common Questions

> "List integrations for project 42"
> "Show the webhook settings"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description                                    |
| ---------- | :------: | :---------: | :--------: | ---------------------------------------------- |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation                       |
| **Create** |    —     |     No      |     —      | Creates a new resource                         |
| **Update** |    —     |     No      |    Yes     | Modifies an existing resource                  |
| **Delete** |    —     |     Yes     |    Yes     | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Project Integrations

### `gitlab_list_integrations`

List all integrations (services) configured for a project, including their active status.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_integration`

Get details of a specific project integration by slug (e.g. jira, slack, discord, mattermost, microsoft-teams, telegram, datadog, jenkins, emails-on-push, pipelines-email, external-wiki, custom-issue-tracker, drone-ci, github, harbor, matrix, redmine, youtrack, slack-slash-commands, mattermost-slash-commands).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_delete_integration`

Delete (disable) a project integration by slug. Supports the same slugs as get, plus 'slack-application' for disabling the GitLab for Slack app.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_set_jira_integration`

Configure the Jira integration for a project. Sets up the connection to a Jira instance with URL, credentials, and event triggers, plus issue-key checks (require/exist/assignee/status with allowed statuses) and vulnerability ticket creation (project key, issue type, customization).

| Annotation | **Create** |
| ---------- | ---------- |

---

## Group Integrations (Datadog)

The group-level Datadog integration is configured at the group scope and inherits down to descendant subgroups when `use_inherited_settings=true`. Requires Owner role and GitLab Premium/Ultimate (self-managed EE or GitLab.com). The `api_key` field is write-only — the read endpoint never returns it.

The read and set outputs are dual-shape, mirroring client-go's own struct: the canonical Datadog configuration lives in the nested `properties` object (`api_url`, `datadog_env`, `datadog_service`, `datadog_site`, `datadog_tags`, `datadog_ci_visibility`, `archive_trace_events`), while the flat top-level copies of those fields are **deprecated** conveniences that will be removed together with client-go's deprecated flat fields at the v3 dependency bump — prefer `properties.*`. On older GitLab servers that omit the nested object, `properties` is absent and the flat fields carry the data.

### `gitlab_get_group_datadog_integration`

Read the Datadog integration configured on a group.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_set_group_datadog_integration`

Create or update the Datadog integration on a group. At least one of `api_key`, `api_url`, `datadog_env`, `datadog_service`, `datadog_site`, `datadog_tags`, `datadog_ci_visibility`, `archive_trace_events`, or `use_inherited_settings=true` must be supplied.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_delete_group_datadog_integration`

Remove the Datadog integration from a group. The stored `api_key` is cleared; deletion is irreversible.

| Annotation | **Delete** |
| ---------- | ---------- |

---

## Project Badges

### `gitlab_list_project_badges`

List all badges of a project, including inherited group badges.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_project_badge`

Get a specific project badge by ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_add_project_badge`

Add a new badge to a project. Badge URLs support placeholders like %{project_path}, %{default_branch}, %{commit_sha}.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_edit_project_badge`

Edit an existing project badge.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_project_badge`

Remove a badge from a project.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_preview_project_badge`

Preview how a project badge renders after placeholder interpolation, without creating it.

| Annotation | **Read** |
| ---------- | -------- |

---

## Group Badges

### `gitlab_list_group_badges`

List all badges of a group.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_group_badge`

Get a specific group badge by ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_add_group_badge`

Add a new badge to a group. Badge URLs support placeholders like %{project_path}, %{default_branch}, %{commit_sha}.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_edit_group_badge`

Edit an existing group badge.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_group_badge`

Remove a badge from a group.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_preview_group_badge`

Preview how a group badge renders after placeholder interpolation, without creating it.

| Annotation | **Read** |
| ---------- | -------- |

---

## Topics

### `gitlab_list_topics`

List project topics. Can be filtered by search query.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_topic`

Get a specific project topic by ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_topic`

Create a new project topic. Requires admin access.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_update_topic`

Update a project topic. Requires admin access.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_topic`

Delete a project topic. Requires admin access.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Requires admin access.

---

## Import Service

### `gitlab_import_from_github`

Import a repository from GitHub into GitLab.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_cancel_github_import`

Cancel an ongoing GitHub project import.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_import_github_gists`

Import GitHub gists into GitLab snippets.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_import_from_bitbucket_cloud`

Import a repository from Bitbucket Cloud into GitLab. Authenticate with either the legacy `bitbucket_app_password` or the newer `bitbucket_api_token` + `bitbucket_email` pair (Atlassian API tokens replace app passwords).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_import_from_bitbucket_server`

Import a repository from Bitbucket Server into GitLab.

| Annotation | **Create** |
| ---------- | ---------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_list_integrations` | Project Integrations | Read |
| 2 | `gitlab_get_integration` | Project Integrations | Read |
| 3 | `gitlab_delete_integration` | Project Integrations | Delete |
| 4 | `gitlab_set_jira_integration` | Project Integrations | Create |
| 5 | `gitlab_list_project_badges` | Project Badges | Read |
| 6 | `gitlab_get_project_badge` | Project Badges | Read |
| 7 | `gitlab_add_project_badge` | Project Badges | Create |
| 8 | `gitlab_edit_project_badge` | Project Badges | Update |
| 9 | `gitlab_delete_project_badge` | Project Badges | Delete |
| 10 | `gitlab_preview_project_badge` | Project Badges | Read |
| 11 | `gitlab_list_group_badges` | Group Badges | Read |
| 12 | `gitlab_get_group_badge` | Group Badges | Read |
| 13 | `gitlab_add_group_badge` | Group Badges | Create |
| 14 | `gitlab_edit_group_badge` | Group Badges | Update |
| 15 | `gitlab_delete_group_badge` | Group Badges | Delete |
| 16 | `gitlab_preview_group_badge` | Group Badges | Read |
| 17 | `gitlab_list_topics` | Topics | Read |
| 18 | `gitlab_get_topic` | Topics | Read |
| 19 | `gitlab_create_topic` | Topics | Create |
| 20 | `gitlab_update_topic` | Topics | Update |
| 21 | `gitlab_delete_topic` | Topics | Delete |
| 22 | `gitlab_import_from_github` | Import Service | Create |
| 23 | `gitlab_cancel_github_import` | Import Service | Update |
| 24 | `gitlab_import_github_gists` | Import Service | Create |
| 25 | `gitlab_import_from_bitbucket_cloud` | Import Service | Create |
| 26 | `gitlab_import_from_bitbucket_server` | Import Service | Create |
| 27 | `gitlab_get_group_datadog_integration` | Group Integrations (Datadog) | Read |
| 28 | `gitlab_set_group_datadog_integration` | Group Integrations (Datadog) | Create |
| 29 | `gitlab_delete_group_datadog_integration` | Group Integrations (Datadog) | Delete |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_delete_integration` — disables a project integration
- `gitlab_delete_project_badge` — removes a badge from a project
- `gitlab_delete_group_badge` — removes a badge from a group
- `gitlab_delete_topic` — deletes a project topic (admin)
- `gitlab_delete_group_datadog_integration` — removes the Datadog integration from a group

---

## Related

- [GitLab Integrations API](https://docs.gitlab.com/api/project_integrations/)
- [GitLab Project Badges API](https://docs.gitlab.com/ee/api/project_badges.html)
- [GitLab Group Badges API](https://docs.gitlab.com/ee/api/group_badges.html)
- [GitLab Topics API](https://docs.gitlab.com/ee/api/topics.html)
- [GitLab Import API](https://docs.gitlab.com/ee/api/import.html)
