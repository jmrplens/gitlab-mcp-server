# Projects — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Projects
> **Individual tools**: 91
> **Meta-tool**: `gitlab_project` (`GITLAB_MCP_TOOL_SURFACE=meta` catalog)
> **Dynamic IDs**: `project.*` (default surface, via `gitlab_execute_action`)
> **GitLab API**: [Projects API](https://docs.gitlab.com/ee/api/projects.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The projects domain covers the full lifecycle of GitLab projects (repositories): creation, retrieval, listing, updating, deletion, forking, starring, archiving, transferring, webhook management, user/group listings, project members, project service accounts, push rule configuration, Pages settings and custom domains, integrations, approvals, pull mirroring, target branch rules, fork relations, avatars, housekeeping, repository storage, and uploads (Markdown attachments + avatars).

On the default dynamic surface, these operations are the `project.*` entries of the canonical action catalog: find them with `gitlab_find_action` and run them with `gitlab_execute_action` by `domain.action` ID. With `GITLAB_MCP_TOOL_SURFACE=individual`, each is the tool named in the tables below.

With `GITLAB_MCP_TOOL_SURFACE=meta`, project actions are consolidated into a single `gitlab_project` meta-tool that dispatches by `action` parameter.

### Common Questions

> "List all my GitLab projects"
> "Create a new project called my-app"
> "Archive the project my-old-app"
> "Who has access to project 42?"
> "Create a service account token for project 42"
> "Set up pull mirroring from upstream"
> "Add a custom domain to GitLab Pages"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description                                    |
| ---------- | :------: | :---------: | :--------: | ---------------------------------------------- |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation                       |
| **Create** |    —     |     No      |     —      | Creates a new resource                         |
| **Update** |    —     |     No      |    Yes     | Modifies an existing resource                  |
| **Delete** |    —     |     Yes     |    Yes     | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Core CRUD

### `gitlab_project_create`

Create a new GitLab project (repository). Supports setting namespace, visibility (private/internal/public), description, default branch, optional README initialization, merge method, squash option, protected merge request pipeline settings, topics, and feature flags.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_get`

Retrieve detailed metadata for a GitLab project including name, description, visibility, web URL, default branch, and namespace. Accepts numeric project ID or URL-encoded path (e.g. `group/subgroup/project`). Optionally include statistics, license info, or custom attributes.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_list`

List GitLab projects accessible to the authenticated user. Supports filtering by ownership, search term, visibility, archived status, topic, minimum access level, starred, membership, date ranges, and feature flags. Set `include_pending_delete=true` to include projects scheduled for deletion. Returns paginated results.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_update`

Update GitLab project settings such as name, description, visibility, default branch, merge method, squash option, protected merge request pipeline settings, topics, feature flags, CI/CD config, merge templates, and approval settings. Only specified fields are modified; unset fields remain unchanged.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_delete`

Delete a GitLab project. On instances with delayed deletion, the project is marked/scheduled for deletion. Set `permanently_remove=true` with `full_path` to bypass delayed deletion. Use `gitlab_project_restore` to cancel a scheduled deletion.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Permanent removal cannot be undone.

### `gitlab_project_restore`

Restore a GitLab project that has been marked/scheduled for deletion. Returns the restored project details. Use `gitlab_project_list` with `include_pending_delete=true` to discover projects pending deletion.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Fork & Star

### `gitlab_project_fork`

Fork a GitLab project into a new project. Optionally specify target namespace, name, path, description, visibility, branches to include, and MR default target setting.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_star`

Star a GitLab project for the authenticated user. Returns updated project details with incremented star count.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_unstar`

Remove star from a GitLab project for the authenticated user. Returns updated project details with decremented star count.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_list_forks`

List forks of a GitLab project. Supports filtering by ownership, search, visibility, ordering, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_list_starrers`

List users who have starred a project. Supports filtering by search (name or username) and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_create_fork_relation`

Create a fork relationship to an upstream project. Requires the numeric `forked_from_id` of the source project and the `project_id` of the forked (downstream) project.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_delete_fork_relation`

Remove a project's fork relationship to its upstream. The downstream project keeps its content; only the fork linkage is cleared.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Archive & Transfer

### `gitlab_project_archive`

Archive a GitLab project, making it read-only. Archived projects are hidden from the default project list. Returns updated project details.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_unarchive`

Unarchive a GitLab project, restoring it from read-only state. Returns updated project details.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_transfer`

Transfer a GitLab project to a different namespace. Requires the namespace (ID or path) to transfer to. Returns updated project details with new path.

| Annotation | **Update** |
| ---------- | ---------- |

> **Protected**: Requires confirmation prompt before execution.

---

## Languages

### `gitlab_project_languages`

List programming languages used in a GitLab project with their percentages. Returns a list of languages detected in the repository.

| Annotation | **Read** |
| ---------- | -------- |

---

## Project Administration

### `gitlab_project_create_for_user`

Create a project on behalf of another user (admin). Supports the full project creation surface — visibility, default branch, merge/squash options, feature flags, access levels per feature, mirroring settings, CI/CD config path, container registry policy, and project templates. Requires administrator privileges.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_repository_storage_get`

Get the repository storage shard that hosts a project. Useful when verifying multi-shard GitLab deployments or planning housekeeping. Admin-level on self-managed instances.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_start_housekeeping`

Trigger a housekeeping task on the project (pack loose objects, prune unreachable objects). Returns a success confirmation. Use sparingly — long-running on large repositories.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Pages

### `gitlab_pages_get`

Get the Pages settings for a project. Returns the Pages URL, force-HTTPS and unique-domain flags, primary domain, and recent deployments.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pages_update`

Update the Pages settings for a project. Toggle `pages_https_only`, `pages_unique_domain_enabled`, or change `pages_primary_domain`.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_pages_unpublish`

Unpublish a project's Pages site, removing it from public access. Returns a success confirmation.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Pages Domains

### `gitlab_pages_domain_list`

List the custom Pages domains for a project with pagination. Returns domains with verification status, auto-SSL flag, project ID, and certificate details. Supports keyset and offset pagination (`pagination=keyset|offset`, `per_page` 1–100, `sort=asc|desc`, `order_by`, `page_token`).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pages_domain_list_all`

List ALL GitLab Pages custom domains across the whole instance in one call (admin only). Use instead of `gitlab_pages_domain_list` when the full instance surface is needed.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pages_domain_get`

Get a single Pages custom domain by name. Returns the domain with verification status and code, auto-SSL flag, enabled-until date, and certificate subject/expiration.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_pages_domain_create`

Add a new custom Pages domain to a project. Provide `domain` (e.g. `example.com`), optionally `auto_ssl_enabled`, `certificate` (PEM), and `key` (PEM private key).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_pages_domain_update`

Update an existing Pages custom domain's `auto_ssl_enabled` flag or replace its `certificate` and `key`.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_pages_domain_delete`

Remove a custom Pages domain from a project. Returns a confirmation naming the deleted domain.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Integrations

Generic integration configuration tools. For dedicated, structured integration tools (Jira, Datadog, etc.) see [docs/tools/integrations.md](integrations.md).

### `gitlab_set_integration`

Configure (create or update) any project integration by slug, passing the integration's documented parameters in a free-form `config` object. Use this generic action for integrations without a dedicated tool (e.g. `slack`, `harbor`, `jenkins`, `google-play`, `apple-app-store`, `prometheus`, `microsoft-teams`, `mattermost`, `discord`, `telegram`, `datadog`, `matrix`, `redmine`, `youtrack`, `github`, `pumble`, `pushover`, `teamcity`, `webex-teams`, `zentao`, `custom-issue-tracker`, `external-wiki`, `pipelines-email`, `emails-on-push`). The `config` object is sent verbatim as the request body.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_list_group_integrations`

List all active integrations configured on a group. Requires Owner role (some integrations require GitLab Premium/Ultimate).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_group_integration`

Get details of a specific group integration by `slug` (e.g. `slack`, `jira`, `harbor`). Requires Owner role on the group.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_set_group_integration`

Configure (create or update) any group integration by slug, passing the integration's documented parameters in a free-form `config` object. Requires Owner role on the group (some integrations require GitLab Premium/Ultimate). The `config` object is sent verbatim as the request body.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_delete_group_integration`

Delete (disable) a group integration by `slug`. Requires Owner role on the group. Pass `confirm=true` to skip the MCP elicitation step.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Project Members

Direct and inherited project membership tools. The mutating CRUD tools (`gitlab_project_member_add`/`_edit`/`_delete`) are documented in [docs/tools/access.md](access.md).

### `gitlab_project_members_list`

List a project's members — both direct members and those inherited from ancestor groups. Supports `query`, `user_ids`, `show_seat_info` filters, `order_by` / `sort=asc|desc`, and keyset (`pagination=keyset`, `page_token`) or offset pagination (`per_page` 1–100).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_member_get`

Get a single direct project member by user ID. Returns id, username, name, state, access level, member role, created_by, expiry, and web URL. Does not include inherited members.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_member_get_inherited`

Get a project member including membership inherited from parent groups. Returns id, username, name, state, effective access level, member role, and web URL.

| Annotation | **Read** |
| ---------- | -------- |

---

## Webhooks

### `gitlab_project_hook_list`

List webhooks configured for a GitLab project. Returns paginated list with event trigger status for each hook.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_hook_get`

Get details of a specific project webhook including all event trigger settings.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_hook_add`

Add a webhook to a GitLab project. Configure the URL, secret token, write-only signing token, SSL verification, and which events trigger the webhook (push, issues, MRs, tags, notes, jobs, pipelines, wiki, deployments, releases, milestones, feature flags, vulnerabilities, resource tokens, emoji, etc.).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_hook_edit`

Edit an existing project webhook. Update the URL, events, SSL verification, secret token, write-only signing token, or other settings.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_hook_delete`

Delete a webhook from a GitLab project.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_project_hook_test`

Trigger a test event for a project webhook. Sends a sample payload for the specified event type (push_events, issues_events, merge_requests_events, etc.).

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_hook_set_url_variable`

Set a URL variable on a project webhook. URL variables are template placeholders resolved at request time. Requires `hook_id`, `key`, and `value`.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_hook_delete_url_variable`

Delete a URL variable from a project webhook by `key`. Returns a success confirmation.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_project_hook_set_custom_header`

Set a custom HTTP header on a project webhook. Headers are sent verbatim on every webhook delivery. Requires `hook_id`, `key`, and `value`.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_hook_delete_custom_header`

Delete a custom header from a project webhook by `key`. Returns a success confirmation.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## User & Group Listings

### `gitlab_project_list_user_projects`

List projects owned by a specific user. Accepts user ID or username. Supports filtering by search, visibility, archived status, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_list_users`

List users who are members of a project. Supports filtering by search (name or username) and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_list_groups`

List ancestor groups of a project. Supports filtering by search, shared groups, minimum access level, skip_groups, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_share_with_group`

Share a project with a group, granting the specified access level. Optionally set an expiration date (YYYY-MM-DD). Access levels: 10=Guest, 20=Reporter, 30=Developer, 40=Maintainer.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_delete_shared_group`

Remove a shared group from a project, revoking the group's access.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_project_list_invited_groups`

List groups that have been invited/shared to a project. Supports filtering by search, minimum access level, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_list_user_contributed`

List projects that a specific user has contributed to. Supports filtering by search, visibility, archived status, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_list_user_starred`

List projects that a specific user has starred. Supports filtering by search, visibility, archived status, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Project Service Accounts

Project service account tools require GitLab Premium/Ultimate and sufficient project permissions. They manage service account users scoped to a project and personal access tokens owned by those service accounts.

### `gitlab_project_service_account_list`

List service accounts for a project. Supports ordering by ID or username, sorting direction, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_service_account_create`

Create a project service account. Optionally provide name, username, and email.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_service_account_update`

Update a project service account's name, username, or email.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_service_account_delete`

Delete a project service account. Set `hard_delete=true` only when permanent deletion is intended.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_project_service_account_pat_list`

List personal access tokens for a project service account. Supports pagination and filters such as state, revoked, search, user ID, created/last-used dates, and expiration dates.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_service_account_pat_create`

Create a personal access token for a project service account. Requires token name and scopes; optionally set description and `expires_at` (`YYYY-MM-DD`). The token value is returned only at creation time.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_service_account_pat_rotate`

Rotate a project service account personal access token and return the new token value. Optionally set the new `expires_at` (`YYYY-MM-DD`).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_service_account_pat_revoke`

Revoke a project service account personal access token by token ID.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Push Rules

### `gitlab_project_get_push_rules`

Get the push rule configuration for a project (commit message, branch name, file size restrictions, etc.).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_add_push_rule`

Add push rule configuration to a project. Enforce commit message format, branch naming, file size limits, secret detection, and signed commits.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_edit_push_rule`

Modify the push rule configuration of a project. Update commit message, branch name, file restrictions, or signing requirements.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_delete_push_rule`

Delete the push rule configuration from a project. This removes all push restrictions (commit format, branch naming, file size, etc.).

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Uploads

### `gitlab_project_upload`

Upload a file to a GitLab project's Markdown uploads area. Provide either `file_path` (absolute local path) or `content_base64` (base64-encoded content). Returns a Markdown embed string for use in MR descriptions, notes, or discussion bodies.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_upload_list`

List all file uploads (Markdown attachments) for a GitLab project. Returns upload ID, filename, size, and creation date.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_upload_delete`

Delete a file upload (Markdown attachment) from a GitLab project by upload ID. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_project_upload_delete_by_secret`

Delete a project Markdown upload by its 32-character `secret` and `filename` (the `/uploads/<secret>/<filename>` reference). Useful when the numeric upload ID is not known.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Avatar

### `gitlab_project_upload_avatar`

Upload a project avatar. Provide the `filename` plus either `file_path` (absolute path to a local image file on the MCP server filesystem) or `content_base64` (base64-encoded image content). Returns the project with its new `avatar_url`.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_download_avatar`

Download a project's avatar. Returns the avatar image content.

| Annotation | **Read** |
| ---------- | -------- |

---

## Import / Export

### `gitlab_schedule_project_export`

Schedule an asynchronous export of a project. Use `gitlab_get_project_export_status` to check progress.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_get_project_export_status`

Get the export status of a project, including download links when the export is finished.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_download_project_export`

Download the finished export archive of a project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_import_project_from_file`

Import a project from an export archive file. Accepts either base64-encoded `content_base64` or a local `.tar.gz` `file_path` under the current working directory, OS temp directory, or `GITLAB_MCP_ALLOWED_IMPORT_DIRS` after symlink resolution.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_get_project_import_status`

Get the import status of a project.

| Annotation | **Read** |
| ---------- | -------- |

---

## Approvals (Premium/Ultimate)

Merge request approval configuration and rules. Approval rules and external status checks are only available on GitLab Premium/Ultimate.

### `gitlab_project_approval_config_get`

Get a project's approval configuration. Returns settings such as `reset_approvals_on_push`, `merge_requests_author_approval`, `merge_requests_disable_committers_approval`, `require_reauthentication_to_approve`, and `selective_code_owner_removals`.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_approval_config_change`

Change a project's approval configuration. Accepts `approvals_before_merge`, `disable_overriding_approvers_per_merge_request`, `merge_requests_author_approval`, `merge_requests_disable_committers_approval`, `require_password_to_approve` (deprecated), `require_reauthentication_to_approve`, `reset_approvals_on_push`, and `selective_code_owner_removals`. Only specified fields are modified.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_approval_rule_list`

List a project's approval rules. Supports keyset (`pagination=keyset`, `page_token`, `order_by`, `sort=asc|desc`) and offset pagination (`per_page` 1–100).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_approval_rule_get`

Get a single project approval rule by `rule_id`. Returns name, `approvals_required`, eligible approvers (users and groups), and the protected branch scope.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_approval_rule_create`

Create a project approval rule. Requires `name` and `approvals_required`; optionally assign `user_ids` / `usernames` / `group_ids`, scope by `protected_branch_ids` or `applies_to_all_protected_branches`, or build a report-approver rule with `report_type` (e.g. `code_coverage`, `license_scanning`) and `rule_type` (`regular`, `code_owner`).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_approval_rule_update`

Update an existing project approval rule by `rule_id`. Accept the same fields as `create`; only specified fields are modified.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_approval_rule_delete`

Delete a project approval rule by `rule_id`. Returns a success confirmation naming the rule and project.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Pull Mirroring (Premium/Ultimate)

Pull-mirror (Geo-style remote sync) configuration. Requires GitLab Premium/Ultimate and Maintainer+ role. For push mirrors, see [docs/tools/mirrors.md](mirrors.md).

### `gitlab_project_pull_mirror_get`

Get a project's pull-mirror configuration. Returns the upstream `url`, `enabled` flag, and last sync status.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_pull_mirror_configure`

Configure a project's pull mirror. Provide the upstream `url` (URI format; do not embed credentials — use `auth_user` / `auth_password` separately), `enabled`, and optionally `mirror_branch_regex`, `mirror_overwrites_diverged_branches`, `mirror_trigger_builds`, and `only_mirror_protected_branches`.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_project_start_mirroring`

Trigger an immediate pull-mirror sync for a project so it fetches from its configured upstream now. Returns a success confirmation.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Target Branch Rules (Premium/Ultimate)

Target branch rules map a source-branch name pattern (e.g. `release/*`) to a default target branch for new merge requests. Implemented via the GraphQL API and queried by full project path.

### `gitlab_project_list_target_branch_rules`

List a project's target branch rules. Pass the full project path as `project_id` (e.g. `group/subgroup/project`); the GraphQL query does not accept a numeric ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_project_create_target_branch_rule`

Create a target branch rule mapping a source branch name pattern (`name`, e.g. `release/*`) to a default `target_branch`. Pass the numeric `project_id` (the create mutation requires a numeric ID, not a path).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_project_delete_target_branch_rule`

Delete a target branch rule by its `rule_id` (find via `gitlab_project_list_target_branch_rules`). Returns a success confirmation naming the rule.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_project_create` | Core CRUD | Create |
| 2 | `gitlab_project_get` | Core CRUD | Read |
| 3 | `gitlab_project_list` | Core CRUD | Read |
| 4 | `gitlab_project_update` | Core CRUD | Update |
| 5 | `gitlab_project_delete` | Core CRUD | Delete |
| 6 | `gitlab_project_restore` | Core CRUD | Update |
| 7 | `gitlab_project_create_for_user` | Project Administration | Create |
| 8 | `gitlab_project_repository_storage_get` | Project Administration | Read |
| 9 | `gitlab_project_start_housekeeping` | Project Administration | Update |
| 10 | `gitlab_project_fork` | Fork & Star | Create |
| 11 | `gitlab_project_star` | Fork & Star | Create |
| 12 | `gitlab_project_unstar` | Fork & Star | Update |
| 13 | `gitlab_project_list_forks` | Fork & Star | Read |
| 14 | `gitlab_project_list_starrers` | Fork & Star | Read |
| 15 | `gitlab_project_create_fork_relation` | Fork & Star | Create |
| 16 | `gitlab_project_delete_fork_relation` | Fork & Star | Delete |
| 17 | `gitlab_project_archive` | Archive & Transfer | Update |
| 18 | `gitlab_project_unarchive` | Archive & Transfer | Update |
| 19 | `gitlab_project_transfer` | Archive & Transfer | Update |
| 20 | `gitlab_project_languages` | Languages | Read |
| 21 | `gitlab_pages_get` | Pages | Read |
| 22 | `gitlab_pages_update` | Pages | Update |
| 23 | `gitlab_pages_unpublish` | Pages | Delete |
| 24 | `gitlab_pages_domain_list` | Pages Domains | Read |
| 25 | `gitlab_pages_domain_list_all` | Pages Domains | Read |
| 26 | `gitlab_pages_domain_get` | Pages Domains | Read |
| 27 | `gitlab_pages_domain_create` | Pages Domains | Create |
| 28 | `gitlab_pages_domain_update` | Pages Domains | Update |
| 29 | `gitlab_pages_domain_delete` | Pages Domains | Delete |
| 30 | `gitlab_set_integration` | Integrations | Create |
| 31 | `gitlab_list_group_integrations` | Integrations | Read |
| 32 | `gitlab_get_group_integration` | Integrations | Read |
| 33 | `gitlab_set_group_integration` | Integrations | Create |
| 34 | `gitlab_delete_group_integration` | Integrations | Delete |
| 35 | `gitlab_project_members_list` | Project Members | Read |
| 36 | `gitlab_project_member_get` | Project Members | Read |
| 37 | `gitlab_project_member_get_inherited` | Project Members | Read |
| 38 | `gitlab_project_hook_list` | Webhooks | Read |
| 39 | `gitlab_project_hook_get` | Webhooks | Read |
| 40 | `gitlab_project_hook_add` | Webhooks | Create |
| 41 | `gitlab_project_hook_edit` | Webhooks | Update |
| 42 | `gitlab_project_hook_delete` | Webhooks | Delete |
| 43 | `gitlab_project_hook_test` | Webhooks | Update |
| 44 | `gitlab_project_hook_set_url_variable` | Webhooks | Update |
| 45 | `gitlab_project_hook_delete_url_variable` | Webhooks | Delete |
| 46 | `gitlab_project_hook_set_custom_header` | Webhooks | Update |
| 47 | `gitlab_project_hook_delete_custom_header` | Webhooks | Delete |
| 48 | `gitlab_project_list_user_projects` | User & Group | Read |
| 49 | `gitlab_project_list_users` | User & Group | Read |
| 50 | `gitlab_project_list_groups` | User & Group | Read |
| 51 | `gitlab_project_share_with_group` | User & Group | Create |
| 52 | `gitlab_project_delete_shared_group` | User & Group | Delete |
| 53 | `gitlab_project_list_invited_groups` | User & Group | Read |
| 54 | `gitlab_project_list_user_contributed` | User & Group | Read |
| 55 | `gitlab_project_list_user_starred` | User & Group | Read |
| 56 | `gitlab_project_service_account_list` | Project Service Accounts | Read |
| 57 | `gitlab_project_service_account_create` | Project Service Accounts | Create |
| 58 | `gitlab_project_service_account_update` | Project Service Accounts | Update |
| 59 | `gitlab_project_service_account_delete` | Project Service Accounts | Delete |
| 60 | `gitlab_project_service_account_pat_list` | Project Service Accounts | Read |
| 61 | `gitlab_project_service_account_pat_create` | Project Service Accounts | Create |
| 62 | `gitlab_project_service_account_pat_rotate` | Project Service Accounts | Create |
| 63 | `gitlab_project_service_account_pat_revoke` | Project Service Accounts | Delete |
| 64 | `gitlab_project_get_push_rules` | Push Rules | Read |
| 65 | `gitlab_project_add_push_rule` | Push Rules | Create |
| 66 | `gitlab_project_edit_push_rule` | Push Rules | Update |
| 67 | `gitlab_project_delete_push_rule` | Push Rules | Delete |
| 68 | `gitlab_project_upload` | Uploads | Create |
| 69 | `gitlab_project_upload_list` | Uploads | Read |
| 70 | `gitlab_project_upload_delete` | Uploads | Delete |
| 71 | `gitlab_project_upload_delete_by_secret` | Uploads | Delete |
| 72 | `gitlab_project_upload_avatar` | Avatar | Create |
| 73 | `gitlab_project_download_avatar` | Avatar | Read |
| 74 | `gitlab_schedule_project_export` | Import / Export | Create |
| 75 | `gitlab_get_project_export_status` | Import / Export | Read |
| 76 | `gitlab_download_project_export` | Import / Export | Read |
| 77 | `gitlab_import_project_from_file` | Import / Export | Create |
| 78 | `gitlab_get_project_import_status` | Import / Export | Read |
| 79 | `gitlab_project_approval_config_get` | Approvals (Premium/Ultimate) | Read |
| 80 | `gitlab_project_approval_config_change` | Approvals (Premium/Ultimate) | Update |
| 81 | `gitlab_project_approval_rule_list` | Approvals (Premium/Ultimate) | Read |
| 82 | `gitlab_project_approval_rule_get` | Approvals (Premium/Ultimate) | Read |
| 83 | `gitlab_project_approval_rule_create` | Approvals (Premium/Ultimate) | Create |
| 84 | `gitlab_project_approval_rule_update` | Approvals (Premium/Ultimate) | Update |
| 85 | `gitlab_project_approval_rule_delete` | Approvals (Premium/Ultimate) | Delete |
| 86 | `gitlab_project_pull_mirror_get` | Pull Mirroring (Premium/Ultimate) | Read |
| 87 | `gitlab_project_pull_mirror_configure` | Pull Mirroring (Premium/Ultimate) | Update |
| 88 | `gitlab_project_start_mirroring` | Pull Mirroring (Premium/Ultimate) | Update |
| 89 | `gitlab_project_list_target_branch_rules` | Target Branch Rules (Premium/Ultimate) | Read |
| 90 | `gitlab_project_create_target_branch_rule` | Target Branch Rules (Premium/Ultimate) | Create |
| 91 | `gitlab_project_delete_target_branch_rule` | Target Branch Rules (Premium/Ultimate) | Delete |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_project_delete` — deletes a project (scheduled or permanent)
- `gitlab_project_transfer` — transfers a project to a different namespace
- `gitlab_project_hook_delete` — removes a webhook
- `gitlab_project_hook_delete_url_variable` — removes a URL variable from a webhook
- `gitlab_project_hook_delete_custom_header` — removes a custom header from a webhook
- `gitlab_project_delete_shared_group` — revokes group access
- `gitlab_project_service_account_delete` — deletes a project service account
- `gitlab_project_service_account_pat_revoke` — revokes a project service account PAT
- `gitlab_project_delete_push_rule` — removes all push restrictions
- `gitlab_project_upload_delete` — deletes a file upload from a project
- `gitlab_project_upload_delete_by_secret` — deletes a project upload by its 32-character secret
- `gitlab_pages_unpublish` — unpublishes a Pages site
- `gitlab_pages_domain_delete` — removes a custom Pages domain
- `gitlab_delete_group_integration` — disables a group integration
- `gitlab_project_delete_fork_relation` — clears a project's fork linkage to its upstream
- `gitlab_project_approval_rule_delete` — removes an approval rule
- `gitlab_project_delete_target_branch_rule` — removes a target branch rule

---

## Related

- [GitLab Projects API](https://docs.gitlab.com/ee/api/projects.html)
- [GitLab Project Webhooks API](https://docs.gitlab.com/api/project_webhooks/)
- [GitLab Service Accounts API](https://docs.gitlab.com/api/service_accounts/)
- [GitLab Push Rules API](https://docs.gitlab.com/ee/api/project_push_rules.html)
- [GitLab Uploads API](https://docs.gitlab.com/api/project_markdown_uploads/)
- [GitLab Project Import/Export API](https://docs.gitlab.com/ee/api/project_import_export.html)
- [GitLab Pages API](https://docs.gitlab.com/ee/api/pages.html)
- [GitLab Pages Domains API](https://docs.gitlab.com/ee/api/pages_domains.html)
- [GitLab Integrations API](https://docs.gitlab.com/api/project_integrations/)
- [GitLab Group Integrations API](https://docs.gitlab.com/ee/api/group_integrations.html)
- [GitLab Members API](https://docs.gitlab.com/ee/api/members.html)
- [GitLab Approvals API](https://docs.gitlab.com/ee/api/merge_request_approvals.html)
- [GitLab Remote Mirrors API](https://docs.gitlab.com/ee/api/remote_mirrors.html)
- [GitLab Project Repository Storage Moves API](https://docs.gitlab.com/ee/api/project_repository_storage_moves.html)
