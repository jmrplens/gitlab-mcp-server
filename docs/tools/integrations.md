# Integrations — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Integrations, Badges, Topics, Epic Discussions, Import
> **Individual tools**: 32
> **Meta-tools**: `gitlab_project` (integrations + badges), `gitlab_admin` (topics), `gitlab_epic_discussion`, `gitlab_import` (when `META_TOOLS=true`, default)
> **GitLab API**: [Integrations API](https://docs.gitlab.com/ee/api/integrations.html), [Badges API](https://docs.gitlab.com/ee/api/project_badges.html), [Topics API](https://docs.gitlab.com/ee/api/topics.html), [Epic Discussions API](https://docs.gitlab.com/ee/api/epic_discussions.html), [Import API](https://docs.gitlab.com/ee/api/import.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The integrations domain covers miscellaneous GitLab tools that don't belong to other major domains: project/group integrations (services), project/group badges, instance-level topics, epic discussion threads, and repository import from external services (GitHub, Bitbucket).

When `META_TOOLS=true` (the default), integration and badge tools are consolidated into `gitlab_project`, topic tools into `gitlab_admin`, epic discussion tools into `gitlab_epic_discussion`, and import tools into `gitlab_import`.

### Common Questions

> "List integrations for project 42"
> "Show the webhook settings"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

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

Configure the Jira integration for a project. Sets up the connection to a Jira instance with URL, credentials, and event triggers.

| Annotation | **Create** |
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

## Epic Discussions

### `gitlab_list_epic_discussions`

List discussion threads on a group epic.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_epic_discussion`

Get a single discussion thread on a group epic.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_epic_discussion`

Create a new discussion thread on a group epic.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_add_epic_discussion_note`

Add a reply note to an existing epic discussion thread.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_update_epic_discussion_note`

Update an existing note in an epic discussion thread.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_epic_discussion_note`

Delete a note from an epic discussion thread.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

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

Import a repository from Bitbucket Cloud into GitLab.

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
| 22 | `gitlab_list_epic_discussions` | Epic Discussions | Read |
| 23 | `gitlab_get_epic_discussion` | Epic Discussions | Read |
| 24 | `gitlab_create_epic_discussion` | Epic Discussions | Create |
| 25 | `gitlab_add_epic_discussion_note` | Epic Discussions | Create |
| 26 | `gitlab_update_epic_discussion_note` | Epic Discussions | Update |
| 27 | `gitlab_delete_epic_discussion_note` | Epic Discussions | Delete |
| 28 | `gitlab_import_from_github` | Import Service | Create |
| 29 | `gitlab_cancel_github_import` | Import Service | Update |
| 30 | `gitlab_import_github_gists` | Import Service | Create |
| 31 | `gitlab_import_from_bitbucket_cloud` | Import Service | Create |
| 32 | `gitlab_import_from_bitbucket_server` | Import Service | Create |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_delete_integration` — disables a project integration
- `gitlab_delete_project_badge` — removes a badge from a project
- `gitlab_delete_group_badge` — removes a badge from a group
- `gitlab_delete_topic` — deletes a project topic (admin)
- `gitlab_delete_epic_discussion_note` — deletes a note from an epic discussion

---

## Related

- [GitLab Integrations API](https://docs.gitlab.com/ee/api/integrations.html)
- [GitLab Project Badges API](https://docs.gitlab.com/ee/api/project_badges.html)
- [GitLab Group Badges API](https://docs.gitlab.com/ee/api/group_badges.html)
- [GitLab Topics API](https://docs.gitlab.com/ee/api/topics.html)
- [GitLab Epic Discussions API](https://docs.gitlab.com/ee/api/epic_discussions.html)
- [GitLab Import API](https://docs.gitlab.com/ee/api/import.html)
