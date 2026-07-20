# Groups — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Groups
> **Individual tools**: 104
> **Meta-tools**: `gitlab_group`, `gitlab_group_member`, `gitlab_group_label`, `gitlab_group_milestone`, `gitlab_group_board`, `gitlab_group_relations_export`, `gitlab_group_markdown_upload`, `gitlab_group_push_rule`, `gitlab_group_protected_branch`, `gitlab_group_protected_environment`, `gitlab_group_release`, `gitlab_group_service_account`, `gitlab_group_wiki` (`TOOL_SURFACE=meta` catalog)
> **GitLab API**: [Groups API](https://docs.gitlab.com/ee/api/groups.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The groups domain covers the full lifecycle of GitLab groups: creation, retrieval, listing, updating, deletion, restoration, archiving, searching, transfers, subgroup management, webhooks (including custom headers, URL variables, and test/resend operations), push rules, protected branches, protected environments, releases, service accounts and their personal access tokens, group wikis, sharing with other groups, label events on group epics, and markdown uploads.

With `TOOL_SURFACE=meta`, the 104 individual tools below are consolidated into domain-specific meta-tools that dispatch by `action` parameter.

### Common Questions

> "List my GitLab groups"
> "Who are the members of group my-team?"
> "List projects in the my-org group"
> "Archive the legacy-team group"
> "Share group platform with the developers group"
> "Protect the main branch in group my-org"
> "List billable members of group acme"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description                                    |
| ---------- | :------: | :---------: | :--------: | ---------------------------------------------- |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation                       |
| **Create** |    —     |     No      |     —      | Creates a new resource                         |
| **Update** |    —     |     No      |    Yes     | Modifies an existing resource                  |
| **Delete** |    —     |     Yes     |    Yes     | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Core Group CRUD

### `gitlab_group_list`

List GitLab groups accessible to the authenticated user. Supports filtering by search term, ownership, and top-level only. Returns paginated results including group name, path, visibility, and web URL.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_get`

Retrieve detailed metadata for a GitLab group including name, path, full path, description, visibility, web URL, and parent group. Accepts numeric group ID or URL-encoded path (e.g. 'group/subgroup').

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_create`

Create a new GitLab group. Requires name; optionally set path, description, visibility, parent_id (for subgroups), organization_id (GitLab.com multi-organization), request_access_enabled, lfs_enabled, crm_enabled, default_branch, math/Duo and web-based-commit-signing toggles, and the unique-project-download-limit controls (limit, interval, allowlist, alertlist, auto-ban — Ultimate).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_update`

Update an existing GitLab group. Supports changing name, path, description, visibility, request_access_enabled, lfs_enabled, crm_enabled, and default_branch.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_delete`

Delete a GitLab group. On instances with delayed deletion, the group is marked for deletion. Set permanently_remove=true with full_path to bypass delayed deletion.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Permanent removal cannot be undone.

### `gitlab_group_restore`

Restore a GitLab group that was marked for deletion.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_search`

Search for GitLab groups by name. Returns matching groups with their details.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_archive`

Archive a GitLab group, making it and its projects read-only. Idempotent: archiving an already-archived group is a no-op.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_unarchive`

Unarchive a previously archived GitLab group, restoring write access. Idempotent: unarchiving a non-archived group is a no-op.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_transfer`

Transfer a GitLab group under a new parent (or to top level by omitting `parent_id`). Requires Owner role on both the group being moved and the destination. Use `gitlab_group_transfer_locations` first to find valid destinations.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_upload_avatar`

Upload or replace a GitLab group's avatar image. Requires `group_id`, `filename`, and exactly one of `file_path` (absolute path the MCP server reads) or `content_base64` (inline base64-encoded image). Image must be JPG/PNG/GIF under 200 KB. Requires Owner role.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Subgroups & Projects

### `gitlab_subgroups_list`

List descendant subgroups of a GitLab group. Returns each subgroup's name, path, full path, description, visibility, and parent ID. Supports search filter and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_projects`

List projects belonging to a GitLab group. Supports filtering by search, archived status, visibility, and including subgroup projects. Returns project name, path, visibility, and archived status with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_transfer_project`

Transfer a project into a group namespace. Moves the project to become a member of the specified group.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_shared_projects_list`

List projects shared *into* a GitLab group from other namespaces (not the group's own projects). Supports filtering by search, archived status, visibility, minimum access level, and starred status. Use this when the user asks which external projects a group can access via sharing.

| Annotation | **Read** |
| ---------- | -------- |

---

## Group Relations

### `gitlab_group_shared_with_list`

List the groups that have been shared with a group (group-to-group shares). Supports filtering by search, minimum access level, and visibility with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_transfer_locations`

List the candidate parent groups available for transferring a group (groups you can move this group into). Supports filtering by search with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_share_with_group`

Share a GitLab group with another group via the Groups API (distinct from `gitlab_group_share`, which uses the Group Members API). Requires `group_id`, `shared_group_id`, and `group_access` (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner). Optionally set `expires_at` (YYYY-MM-DD) and `member_role_id` (Ultimate custom role). Requires Owner role.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_unshare_from_group`

Revoke a group-to-group share created via the Groups API. Destructive. Requires `group_id` and `shared_group_id`. Requires Owner role.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Removes the shared group's access link.

---

## Group Members (legacy in groups package)

### `gitlab_group_members_list`

List all members of a GitLab group including inherited members. Returns user ID, username, name, state, access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner), and web URL. Supports filtering by name/username query.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_list_billable_group_members`

List the billable members of a group with seat, membership type, `removable` flag, `is_last_owner`, last activity, and last login (Premium/Ultimate). Supports search, ordering, and offset or keyset pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_list_billable_member_memberships`

List the memberships (source group/project, access level, dates) of a billable group member (Premium/Ultimate). Requires `group_id` and `user_id`.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_remove_billable_group_member`

Remove a billable member from a group, freeing a seat (Premium/Ultimate). Destructive: requires confirmation. Check the `removable` flag from `gitlab_list_billable_group_members` first.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Frees a paid seat.

### `gitlab_issue_list_group`

List issues across all projects in a group. Supports filtering by state, assignee, author, labels, milestone, confidential, iteration, search, issue type (`issue`, `incident`, `test_case`, `task`), and date ranges. Returns matching issues with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_list_provisioned_users`

List users provisioned for a GitLab group via SAML/SCIM (Premium/Ultimate). Supports filtering by `username`, `search`, `active`, `blocked`, `created_after`, and `created_before`. Distinct from `gitlab_group_members_list`, which returns regular group membership.

| Annotation | **Read** |
| ---------- | -------- |

---

## Group Webhooks

> Group webhooks (Premium/Ultimate) deliver group-wide events to external endpoints. GitLab Free/CE responds 404 to the whole `/groups/:id/hooks` namespace, and the MCP catalog gates these tools to Premium and Ultimate tiers.

### `gitlab_group_hook_list`

List webhooks configured for a GitLab group. Returns hook URL, enabled events, SSL verification status, and creation date with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_hook_get`

Get details of a specific group webhook by hook ID. Returns URL, enabled events, SSL status, and alert status.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_hook_add`

Add a new webhook to a GitLab group. Requires URL; optionally configure event triggers, SSL verification, secret token, write-only signing token, branch filter, and `branch_filter_strategy` (`wildcard`, `regex`, or `all_branches`). The full set of supported event flags includes push, tag push, push_events_branch_filter, branch_filter_strategy, issues, confidential issues, merge requests, note, confidential note, job, pipeline, wiki page, subgroup, member, release, deployment, feature flag, milestone, vulnerability, emoji, resource access token, and project events.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_hook_edit`

Edit an existing group webhook. Supports changing URL, events, SSL verification, secret token, write-only signing token, branch filter, and `branch_filter_strategy` (`wildcard`, `regex`, or `all_branches`). Event flags include push, tag push, push_events_branch_filter, branch_filter_strategy, issues, confidential issues, merge requests, note, confidential note, job, pipeline, wiki page, subgroup, member, release, deployment, feature flag, milestone, vulnerability, emoji, resource access token, and project events.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_hook_delete`

Delete a webhook from a GitLab group.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_group_hook_test`

Trigger a test event for a GitLab group webhook. Requires `group_id`, `hook_id`, and `trigger` (event type: `push_events`, `tag_push_events`, `issues_events`, `confidential_issues_events`, `note_events`, `merge_requests_events`, `job_events`, `pipeline_events`, `wiki_page_events`, and others).

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_hook_resend_event`

Resend a previously delivered GitLab group hook event to retry a failed delivery. Requires `group_id`, `hook_id`, and `hook_event_id`.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_hook_set_custom_header`

Set (create or update) a custom HTTP header on a GitLab group webhook by `key`. The `value` is write-only and masked on read. Requires `group_id`, `hook_id`, `key`, and `value`.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_hook_delete_custom_header`

Delete a custom HTTP header from a GitLab group webhook by `key`. Destructive. Requires `group_id`, `hook_id`, and `key`.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_group_hook_set_url_variable`

Set (create or update) a templated URL variable on a GitLab group webhook by `key`. The `value` is write-only and masked on read. Requires `group_id`, `hook_id`, `key`, and `value`.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_hook_delete_url_variable`

Delete a templated URL variable from a GitLab group webhook by `key`. Destructive. Requires `group_id`, `hook_id`, and `key`.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Group Member Management

### `gitlab_group_member_get`

Get a single member of a GitLab group by user ID. Returns user details including access level, state, and expiration date.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_member_get_inherited`

Get a single inherited member of a GitLab group by user ID. Returns member details including access level inherited from parent groups.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_member_remove`

Remove a member from a GitLab group. Optionally skip subresource removal and unassign issuables.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_group_share`

Share a GitLab group with another group, granting the shared group a specified access level. Optionally set an expiration date.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_unshare`

Stop sharing a GitLab group with another group, removing the group-level access.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Group Push Rules

> Group push rules (Premium/Ultimate) configure commit, branch, file-name, and tag policies that apply to every project in the group. Rules form a singleton per group — there is no `push_rule_id`.

### `gitlab_group_get_push_rules`

Get the singleton push-rule configuration for a GitLab group (Premium/Ultimate). Returns `commit_message_regex`, `commit_message_negative_regex`, `branch_name_regex`, `author_email_regex`, `file_name_regex`, `max_file_size`, `member_check`, `prevent_secrets`, `reject_unsigned_commits`, `reject_non_dco_commits`, `commit_committer_check`, `commit_committer_name_check`, `deny_delete_tag`, and `allow_push_to_third_party` settings. Requires Owner role.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_add_push_rule`

Create a push-rule configuration for a GitLab group (Premium/Ultimate). Include at least one rule-setting parameter (for example `commit_message_regex`, `reject_unsigned_commits`, `prevent_secrets`, `branch_name_regex`, or `deny_delete_tag`); do not call `add` with only `group_id`. Requires Owner role.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_edit_push_rule`

Edit an existing push-rule configuration for a GitLab group (Premium/Ultimate). Send `group_id` plus only the fields to change. Use `reject_unsigned_commits` (not `deny_unsigned_commits`). Requires Owner role.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_delete_push_rule`

Delete the push-rule configuration for a GitLab group (Premium/Ultimate). Destructive. Requires `group_id` and Owner role.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Group Protected Branches

> Group-level protected branches cascade to all subgroup projects, applying the same push, merge, and unprotect rules.

### `gitlab_group_protected_branch_list`

List group-level protected branches with search and offset or keyset pagination. Returns protected branches with push, merge, and unprotect access levels, force-push and code-owner-approval flags, and pagination metadata.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_protected_branch_get`

Get a single group-level protected branch by `branch` (name or wildcard). Returns push, merge, and unprotect access levels (with user/group entries), force-push, and code-owner-approval flags.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_protected_branch_protect`

Protect a group-level branch or wildcard. Requires `group_id` and `name`. Configure `push_access_level` (0=No access, 30=Developer, 40=Maintainer, 60=Admin), `merge_access_level`, `unprotect_access_level`, and optional `allow_force_push`, `code_owner_approval_required`, and per-user/per-group `allowed_to_push`, `allowed_to_merge`, `allowed_to_unprotect` entries.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_protected_branch_update`

Update a group-level protected branch by `branch` (name or wildcard). Add or remove (`_destroy`) `allowed_to_push`, `allowed_to_merge`, and `allowed_to_unprotect` entries, and toggle `allow_force_push` and `code_owner_approval_required`.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_protected_branch_unprotect`

Unprotect a group-level branch or wildcard, cascading the removal to all subgroup projects. Destructive. Requires `group_id` and `branch`.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Cascades to all subgroup projects.

---

## Group Protected Environments

> Group-level protected environments apply deploy-access and approval rules to every subgroup project. Environment tier is one of `production`, `staging`, `testing`, `development`, or `other`.

### `gitlab_group_protected_environment_list`

List group-level protected environments with order/sort and offset or keyset pagination. Returns protected environments with their deploy access levels, required approval count, approval rules, and pagination metadata.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_protected_environment_get`

Get a single group-level protected environment by `environment` (tier). Returns the environment with its deploy access levels (id, access level, user/group, group inheritance) and approval rules.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_protected_environment_protect`

Protect a group-level environment tier with deploy access levels and approval rules; protection cascades to all subgroup projects. Requires `group_id` and `name` (tier). Configure `deploy_access_levels`, `approval_rules`, and `required_approval_count`.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_protected_environment_update`

Update a group-level protected environment's deploy access levels and approval rules. Use `_destroy` on an entry to remove it. Requires `group_id` and `environment`.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_protected_environment_unprotect`

Unprotect a group-level environment tier, removing its deployment gates from the group and its subgroup projects. Destructive. Requires `group_id` and `environment`.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Cascades to all subgroup projects.

---

## Group Releases

### `gitlab_group_release_list`

List releases across all projects in a group with pagination, ordering (`released_at`, `created_at`), and keyset pagination support. Returns tag names, release names, dates, author, commit, assets, milestones, evidences, and `_links` per release. Set `simple=true` to return only limited fields.

| Annotation | **Read** |
| ---------- | -------- |

---

## Group Service Accounts

> Group service accounts are bot users owned by a top-level group, used to issue personal access tokens scoped to the group. Available on all tiers; requires Owner role.

### `gitlab_group_service_account_list`

List all service accounts for a GitLab group. Returns ID, name, username, and email with pagination and keyset support.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_service_account_create`

Create a service account in a top-level GitLab group. Send `group_id`, plus `name` and `username`, and optionally `email`. Requires Owner role.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_service_account_update`

Update a service account in a top-level GitLab group by `service_account_id`. Send `group_id` plus the fields to change (`name`, `username`, `email`).

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_service_account_delete`

Delete a service account from a top-level GitLab group. Destructive. Set `hard_delete=true` to hard-delete. Requires `group_id` and `service_account_id`.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_group_service_account_pat_list`

List personal access tokens (PATs) for a group service account with filtering by `revoked`, `state` (`active` or `inactive`), `search`, `user_id`, and date ranges (`created_after`, `created_before`, `expires_after`, `expires_before`, `last_used_after`, `last_used_before`).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_service_account_pat_create`

Create a personal access token for a group service account. Requires `group_id`, `service_account_id`, `name`, and `scopes` (e.g. `api`, `read_api`, `read_user`). Optionally set `expires_at` (YYYY-MM-DD) and `description`. Returns the token value once; capture it immediately.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_service_account_pat_revoke`

Revoke a personal access token for a group service account. Destructive. Requires `group_id`, `service_account_id`, and `token_id`.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_group_service_account_pat_rotate`

Rotate a group service account's personal access token: revokes `token_id` and issues a replacement in one step. Requires `group_id`, `service_account_id`, and `token_id`. Optionally set `expires_at` (YYYY-MM-DD). Returns the new token value once.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Group Wikis

> Group wikis are a GitLab Premium feature. A group's wiki pages are separate from project wikis and require the group itself to be on Premium or higher.

### `gitlab_group_wiki_list`

List a group's wiki pages. Returns each page's title, slug, and format. Set `with_content=true` to include full page content (use sparingly for large wikis).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_wiki_get`

Get a single group wiki page by `slug`. Returns title, slug, format, content, and encoding. Set `render_html=true` to get HTML-rendered content, or `version` to fetch a specific historical revision by SHA.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_wiki_create`

Create a new group wiki page. Requires `group_id`, `title`, and `content`. Optionally set `format` (`markdown`, `rdoc`, `asciidoc`, or `org`; default `markdown`).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_wiki_edit`

Update an existing group wiki page by `slug`. Send `group_id` and `slug` plus the fields to change: `title`, `content`, and optionally `format`.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_wiki_delete`

Delete a group wiki page permanently by `slug`. Destructive.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Permanent removal cannot be undone.

---

## Group Epic Label Events

> Label events for group epics (Premium/Ultimate) record when labels were added or removed from an epic, including the acting user.

### `gitlab_list_group_epic_label_events`

List label events for a group epic (Premium/Ultimate). Returns each event's action, the label object, the acting user, and pagination metadata. Requires `group_id` and `epic_iid`.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_group_epic_label_event`

Get a single label event for a group epic (Premium/Ultimate) by `label_event_id`. Returns the event with action, the label object, and the acting user. Requires `group_id`, `epic_iid`, and `label_event_id`.

| Annotation | **Read** |
| ---------- | -------- |

---

## Group Labels

### `gitlab_group_label_list`

List all labels for a GitLab group. Supports filtering by search keyword, including issue/MR counts (with_counts), ancestor/descendant groups, and group-only labels. Returns label name, color, description, open/closed issue counts, and MR counts with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_label_get`

Get details of a single group label by ID or name, including color, description, priority, and issue/MR counts.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_label_create`

Create a new label in a GitLab group with a name, color (hex), optional description, optional priority, and optional `archived` flag (Premium/Ultimate). Archived labels are hidden from the default label picker but retained for filtering and reporting.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_label_update`

Update an existing group label. Can change name, color, description, priority, or the `archived` flag (Premium/Ultimate). Only specified fields are modified.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_label_delete`

Delete a group label by ID or name.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_group_label_subscribe`

Subscribe to a group label to receive notifications when the label is applied to issues or merge requests.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_label_unsubscribe`

Unsubscribe from a group label to stop receiving notifications.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Group Milestones

### `gitlab_group_milestone_list`

List all milestones for a GitLab group. Supports filtering by state, title, search, IIDs, date ranges, and ancestor/descendant groups. Returns milestone title, state, dates, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_milestone_get`

Get details of a single group milestone by ID, including title, state, start/due dates, and timestamps.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_milestone_create`

Create a new milestone in a GitLab group with a title, optional description, start date and due date (YYYY-MM-DD).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_milestone_update`

Update an existing group milestone. Can change title, description, dates, or state (activate/close). Only specified fields are modified.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_milestone_delete`

Delete a group milestone by ID.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_group_milestone_issues`

List all issues assigned to a group milestone. Returns issue ID, IID, title, state, and web URL with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_milestone_merge_requests`

List all merge requests assigned to a group milestone. Returns MR ID, IID, title, state, source/target branches with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_milestone_burndown_events`

List all burndown chart events for a group milestone. Returns event timestamps, weights, and actions with pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Group Import/Export

### `gitlab_schedule_group_export`

Schedule an asynchronous export of a group.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_download_group_export`

Download the finished export archive of a group. Returns the archive as base64-encoded content.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_import_group_from_file`

Import a group from an export archive file. Requires a local `.tar.gz` archive under the current working directory, OS temp directory, or `GITLAB_MCP_ALLOWED_IMPORT_DIRS` after symlink resolution.

| Annotation | **Create** |
| ---------- | ---------- |

---

## Group Issue Boards

### `gitlab_group_board_list`

List all issue boards for a group.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_board_get`

Get a single group issue board.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_board_create`

Create a new issue board in a group.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_board_update`

Update an existing group issue board.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_board_delete`

Delete a group issue board. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_group_board_list_lists`

List all lists in a group issue board.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_board_list_get`

Get a single list from a group issue board.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_group_board_list_create`

Create a new list in a group issue board.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_group_board_list_update`

Update (reorder) a list in a group issue board.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_group_board_list_delete`

Delete a list from a group issue board. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Group Relations Export

### `gitlab_schedule_group_relations_export`

Schedule a new group relations export.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_list_group_relations_export_status`

List the status of group relations exports.

| Annotation | **Read** |
| ---------- | -------- |

---

## Group Markdown Uploads

### `gitlab_list_group_markdown_uploads`

List markdown uploads for a group.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_delete_group_markdown_upload_by_id`

Delete a group markdown upload by ID.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_delete_group_markdown_upload_by_secret`

Delete a group markdown upload by secret and filename.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_group_list` | Core CRUD | Read |
| 2 | `gitlab_group_get` | Core CRUD | Read |
| 3 | `gitlab_group_create` | Core CRUD | Create |
| 4 | `gitlab_group_update` | Core CRUD | Update |
| 5 | `gitlab_group_delete` | Core CRUD | Delete |
| 6 | `gitlab_group_restore` | Core CRUD | Update |
| 7 | `gitlab_group_search` | Core CRUD | Read |
| 8 | `gitlab_group_archive` | Core CRUD | Update |
| 9 | `gitlab_group_unarchive` | Core CRUD | Update |
| 10 | `gitlab_group_transfer` | Core CRUD | Update |
| 11 | `gitlab_group_upload_avatar` | Core CRUD | Update |
| 12 | `gitlab_subgroups_list` | Subgroups & Projects | Read |
| 13 | `gitlab_group_projects` | Subgroups & Projects | Read |
| 14 | `gitlab_group_transfer_project` | Subgroups & Projects | Update |
| 15 | `gitlab_group_shared_projects_list` | Subgroups & Projects | Read |
| 16 | `gitlab_group_shared_with_list` | Group Relations | Read |
| 17 | `gitlab_group_transfer_locations` | Group Relations | Read |
| 18 | `gitlab_group_share_with_group` | Group Relations | Create |
| 19 | `gitlab_group_unshare_from_group` | Group Relations | Delete |
| 20 | `gitlab_group_members_list` | Members (legacy) | Read |
| 21 | `gitlab_list_billable_group_members` | Members (legacy) | Read |
| 22 | `gitlab_list_billable_member_memberships` | Members (legacy) | Read |
| 23 | `gitlab_remove_billable_group_member` | Members (legacy) | Delete |
| 24 | `gitlab_issue_list_group` | Members (legacy) | Read |
| 25 | `gitlab_group_list_provisioned_users` | Members (legacy) | Read |
| 26 | `gitlab_group_hook_list` | Webhooks (Premium/Ultimate) | Read |
| 27 | `gitlab_group_hook_get` | Webhooks (Premium/Ultimate) | Read |
| 28 | `gitlab_group_hook_add` | Webhooks (Premium/Ultimate) | Create |
| 29 | `gitlab_group_hook_edit` | Webhooks (Premium/Ultimate) | Update |
| 30 | `gitlab_group_hook_delete` | Webhooks (Premium/Ultimate) | Delete |
| 31 | `gitlab_group_hook_test` | Webhooks (Premium/Ultimate) | Update |
| 32 | `gitlab_group_hook_resend_event` | Webhooks (Premium/Ultimate) | Update |
| 33 | `gitlab_group_hook_set_custom_header` | Webhooks (Premium/Ultimate) | Update |
| 34 | `gitlab_group_hook_delete_custom_header` | Webhooks (Premium/Ultimate) | Delete |
| 35 | `gitlab_group_hook_set_url_variable` | Webhooks (Premium/Ultimate) | Update |
| 36 | `gitlab_group_hook_delete_url_variable` | Webhooks (Premium/Ultimate) | Delete |
| 37 | `gitlab_group_member_get` | Member Management | Read |
| 38 | `gitlab_group_member_get_inherited` | Member Management | Read |
| 39 | `gitlab_group_member_remove` | Member Management | Delete |
| 40 | `gitlab_group_share` | Member Management | Create |
| 41 | `gitlab_group_unshare` | Member Management | Delete |
| 42 | `gitlab_group_get_push_rules` | Push Rules (Premium/Ultimate) | Read |
| 43 | `gitlab_group_add_push_rule` | Push Rules (Premium/Ultimate) | Create |
| 44 | `gitlab_group_edit_push_rule` | Push Rules (Premium/Ultimate) | Update |
| 45 | `gitlab_group_delete_push_rule` | Push Rules (Premium/Ultimate) | Delete |
| 46 | `gitlab_group_protected_branch_list` | Protected Branches | Read |
| 47 | `gitlab_group_protected_branch_get` | Protected Branches | Read |
| 48 | `gitlab_group_protected_branch_protect` | Protected Branches | Create |
| 49 | `gitlab_group_protected_branch_update` | Protected Branches | Update |
| 50 | `gitlab_group_protected_branch_unprotect` | Protected Branches | Delete |
| 51 | `gitlab_group_protected_environment_list` | Protected Environments | Read |
| 52 | `gitlab_group_protected_environment_get` | Protected Environments | Read |
| 53 | `gitlab_group_protected_environment_protect` | Protected Environments | Create |
| 54 | `gitlab_group_protected_environment_update` | Protected Environments | Update |
| 55 | `gitlab_group_protected_environment_unprotect` | Protected Environments | Delete |
| 56 | `gitlab_group_release_list` | Releases | Read |
| 57 | `gitlab_group_service_account_list` | Service Accounts | Read |
| 58 | `gitlab_group_service_account_create` | Service Accounts | Create |
| 59 | `gitlab_group_service_account_update` | Service Accounts | Update |
| 60 | `gitlab_group_service_account_delete` | Service Accounts | Delete |
| 61 | `gitlab_group_service_account_pat_list` | Service Accounts | Read |
| 62 | `gitlab_group_service_account_pat_create` | Service Accounts | Create |
| 63 | `gitlab_group_service_account_pat_revoke` | Service Accounts | Delete |
| 64 | `gitlab_group_service_account_pat_rotate` | Service Accounts | Update |
| 65 | `gitlab_group_wiki_list` | Wikis (Premium) | Read |
| 66 | `gitlab_group_wiki_get` | Wikis (Premium) | Read |
| 67 | `gitlab_group_wiki_create` | Wikis (Premium) | Create |
| 68 | `gitlab_group_wiki_edit` | Wikis (Premium) | Update |
| 69 | `gitlab_group_wiki_delete` | Wikis (Premium) | Delete |
| 70 | `gitlab_list_group_epic_label_events` | Epic Label Events (Premium/Ultimate) | Read |
| 71 | `gitlab_get_group_epic_label_event` | Epic Label Events (Premium/Ultimate) | Read |
| 72 | `gitlab_group_label_list` | Labels | Read |
| 73 | `gitlab_group_label_get` | Labels | Read |
| 74 | `gitlab_group_label_create` | Labels | Create |
| 75 | `gitlab_group_label_update` | Labels | Update |
| 76 | `gitlab_group_label_delete` | Labels | Delete |
| 77 | `gitlab_group_label_subscribe` | Labels | Update |
| 78 | `gitlab_group_label_unsubscribe` | Labels | Update |
| 79 | `gitlab_group_milestone_list` | Milestones | Read |
| 80 | `gitlab_group_milestone_get` | Milestones | Read |
| 81 | `gitlab_group_milestone_create` | Milestones | Create |
| 82 | `gitlab_group_milestone_update` | Milestones | Update |
| 83 | `gitlab_group_milestone_delete` | Milestones | Delete |
| 84 | `gitlab_group_milestone_issues` | Milestones | Read |
| 85 | `gitlab_group_milestone_merge_requests` | Milestones | Read |
| 86 | `gitlab_group_milestone_burndown_events` | Milestones | Read |
| 87 | `gitlab_schedule_group_export` | Import/Export | Create |
| 88 | `gitlab_download_group_export` | Import/Export | Read |
| 89 | `gitlab_import_group_from_file` | Import/Export | Create |
| 90 | `gitlab_group_board_list` | Issue Boards | Read |
| 91 | `gitlab_group_board_get` | Issue Boards | Read |
| 92 | `gitlab_group_board_create` | Issue Boards | Create |
| 93 | `gitlab_group_board_update` | Issue Boards | Update |
| 94 | `gitlab_group_board_delete` | Issue Boards | Delete |
| 95 | `gitlab_group_board_list_lists` | Issue Boards | Read |
| 96 | `gitlab_group_board_list_get` | Issue Boards | Read |
| 97 | `gitlab_group_board_list_create` | Issue Boards | Create |
| 98 | `gitlab_group_board_list_update` | Issue Boards | Update |
| 99 | `gitlab_group_board_list_delete` | Issue Boards | Delete |
| 100 | `gitlab_schedule_group_relations_export` | Relations Export | Create |
| 101 | `gitlab_list_group_relations_export_status` | Relations Export | Read |
| 102 | `gitlab_list_group_markdown_uploads` | Markdown Uploads | Read |
| 103 | `gitlab_delete_group_markdown_upload_by_id` | Markdown Uploads | Delete |
| 104 | `gitlab_delete_group_markdown_upload_by_secret` | Markdown Uploads | Delete |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_group_delete` — deletes a group (scheduled or permanent)
- `gitlab_group_hook_delete` — removes a group webhook
- `gitlab_group_hook_delete_custom_header` — removes a custom header from a group webhook
- `gitlab_group_hook_delete_url_variable` — removes a URL variable from a group webhook
- `gitlab_group_member_remove` — removes a member from a group
- `gitlab_group_unshare` — revokes group-to-group sharing
- `gitlab_group_unshare_from_group` — revokes a group-to-group share via the Groups API
- `gitlab_remove_billable_group_member` — removes a billable group member, freeing a seat
- `gitlab_group_label_delete` — deletes a group label
- `gitlab_group_milestone_delete` — deletes a group milestone
- `gitlab_group_delete_push_rule` — removes the group's push rules
- `gitlab_group_protected_branch_unprotect` — unprotects a group-level branch, cascading to subgroups
- `gitlab_group_protected_environment_unprotect` — unprotects a group-level environment tier, cascading to subgroups
- `gitlab_group_service_account_delete` — deletes a group service account
- `gitlab_group_service_account_pat_revoke` — revokes a service account personal access token
- `gitlab_group_wiki_delete` — permanently deletes a group wiki page
- `gitlab_group_board_delete` — deletes a group issue board
- `gitlab_group_board_list_delete` — deletes a list from a group issue board
- `gitlab_delete_group_markdown_upload_by_id` — deletes a markdown upload by ID
- `gitlab_delete_group_markdown_upload_by_secret` — deletes a markdown upload by secret

---

## Related

- [GitLab Groups API](https://docs.gitlab.com/ee/api/groups.html)
- [GitLab Group Members API](https://docs.gitlab.com/ee/api/members.html)
- [GitLab Group Labels API](https://docs.gitlab.com/ee/api/group_labels.html)
- [GitLab Group Milestones API](https://docs.gitlab.com/ee/api/group_milestones.html)
- [GitLab Group Import/Export API](https://docs.gitlab.com/ee/api/group_import_export.html)
- [GitLab Group Issue Boards API](https://docs.gitlab.com/ee/api/group_boards.html)
- [GitLab Group Webhooks API](https://docs.gitlab.com/ee/api/group_hooks.html)
- [GitLab Protected Branches API](https://docs.gitlab.com/ee/api/protected_branches.html)
- [GitLab Push Rules API](https://docs.gitlab.com/ee/api/protected_branches.html#push-rules)
- [GitLab Group Wikis API](https://docs.gitlab.com/ee/api/wikis.html)
