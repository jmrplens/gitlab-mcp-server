# Notifications & Events — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Notifications & Events
> **Individual tools**: 44
> **Meta-tools**: `gitlab_notification`, `gitlab_event`, `gitlab_resource_event`, `gitlab_award_emoji` (when `META_TOOLS=true`, default)
> **GitLab API**: [Notification Settings](https://docs.gitlab.com/ee/api/notification_settings.html) · [Events](https://docs.gitlab.com/ee/api/events.html) · [Resource Label/Milestone/State Events](https://docs.gitlab.com/ee/api/resource_label_events.html) · [Award Emoji](https://docs.gitlab.com/ee/api/award_emoji.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The notifications & events domain covers notification settings (global, project, group), project and user events, resource-level change events (label, milestone, state), and award emoji reactions on issues, merge requests, snippets, and their notes.

When `META_TOOLS=true` (the default), the individual tools below are consolidated into four meta-tools that dispatch by `action` parameter.

### Common Questions

> "Show my notification settings"
> "List my pending to-do items"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Notification Settings

### `gitlab_notification_global_get`

Get global notification settings for the authenticated user.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_notification_project_get`

Get notification settings for a specific project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_notification_group_get`

Get notification settings for a specific group.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_notification_global_update`

Update global notification settings for the authenticated user.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_notification_project_update`

Update notification settings for a specific project.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_notification_group_update`

Update notification settings for a specific group.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Events

### `gitlab_project_event_list`

List all visible events for a project. Supports filtering by action type, target type, date range, sort order, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_user_contribution_event_list`

List contribution events for the authenticated user. Supports filtering by action type, target type, date range, sort order, scope, and pagination.

| Annotation | **Read** |
| ---------- | -------- |

---

## Resource Events — Labels

### `gitlab_issue_label_event_list`

List label events for a project issue. Shows when labels were added or removed.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_label_event_get`

Get a single label event for a project issue.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_label_event_list`

List label events for a merge request. Shows when labels were added or removed.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_label_event_get`

Get a single label event for a merge request.

| Annotation | **Read** |
| ---------- | -------- |

---

## Resource Events — Milestones

### `gitlab_issue_milestone_event_list`

List milestone events for a project issue. Shows when milestones were added or removed.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_milestone_event_get`

Get a single milestone event for a project issue.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_milestone_event_list`

List milestone events for a merge request. Shows when milestones were added or removed.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_milestone_event_get`

Get a single milestone event for a merge request.

| Annotation | **Read** |
| ---------- | -------- |

---

## Resource Events — State

### `gitlab_issue_state_event_list`

List state events for a project issue. Shows when the issue was opened, closed, or reopened.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_state_event_get`

Get a single state event for a project issue.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_state_event_list`

List state events for a merge request. Shows when the MR was opened, closed, merged, or reopened.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_state_event_get`

Get a single state event for a merge request.

| Annotation | **Read** |
| ---------- | -------- |

---

## Award Emoji — Issues

### `gitlab_issue_emoji_list`

List all award emoji on a project issue.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_emoji_get`

Get a single award emoji on a project issue.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_emoji_create`

Add an award emoji reaction to a project issue.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_issue_emoji_delete`

Delete an award emoji from a project issue.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Award Emoji — Issue Notes

### `gitlab_issue_note_emoji_list`

List all award emoji on a project issue note.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_note_emoji_get`

Get a single award emoji on a project issue note.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_issue_note_emoji_create`

Add an award emoji reaction to a project issue note.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_issue_note_emoji_delete`

Delete an award emoji from a project issue note.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Award Emoji — Merge Requests

### `gitlab_mr_emoji_list`

List all award emoji on a merge request.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_emoji_get`

Get a single award emoji on a merge request.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_emoji_create`

Add an award emoji reaction to a merge request.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_mr_emoji_delete`

Delete an award emoji from a merge request.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Award Emoji — Merge Request Notes

### `gitlab_mr_note_emoji_list`

List all award emoji on a merge request note.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_note_emoji_get`

Get a single award emoji on a merge request note.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_mr_note_emoji_create`

Add an award emoji reaction to a merge request note.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_mr_note_emoji_delete`

Delete an award emoji from a merge request note.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Award Emoji — Snippets

### `gitlab_snippet_emoji_list`

List all award emoji on a project snippet.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_snippet_emoji_get`

Get a single award emoji on a project snippet.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_snippet_emoji_create`

Add an award emoji reaction to a project snippet.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_snippet_emoji_delete`

Delete an award emoji from a project snippet.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Award Emoji — Snippet Notes

### `gitlab_snippet_note_emoji_list`

List all award emoji on a project snippet note.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_snippet_note_emoji_get`

Get a single award emoji on a project snippet note.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_snippet_note_emoji_create`

Add an award emoji reaction to a project snippet note.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_snippet_note_emoji_delete`

Delete an award emoji from a project snippet note.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_notification_global_get` | Notifications | Read |
| 2 | `gitlab_notification_project_get` | Notifications | Read |
| 3 | `gitlab_notification_group_get` | Notifications | Read |
| 4 | `gitlab_notification_global_update` | Notifications | Update |
| 5 | `gitlab_notification_project_update` | Notifications | Update |
| 6 | `gitlab_notification_group_update` | Notifications | Update |
| 7 | `gitlab_project_event_list` | Events | Read |
| 8 | `gitlab_user_contribution_event_list` | Events | Read |
| 9 | `gitlab_issue_label_event_list` | Resource Events | Read |
| 10 | `gitlab_issue_label_event_get` | Resource Events | Read |
| 11 | `gitlab_mr_label_event_list` | Resource Events | Read |
| 12 | `gitlab_mr_label_event_get` | Resource Events | Read |
| 13 | `gitlab_issue_milestone_event_list` | Resource Events | Read |
| 14 | `gitlab_issue_milestone_event_get` | Resource Events | Read |
| 15 | `gitlab_mr_milestone_event_list` | Resource Events | Read |
| 16 | `gitlab_mr_milestone_event_get` | Resource Events | Read |
| 17 | `gitlab_issue_state_event_list` | Resource Events | Read |
| 18 | `gitlab_issue_state_event_get` | Resource Events | Read |
| 19 | `gitlab_mr_state_event_list` | Resource Events | Read |
| 20 | `gitlab_mr_state_event_get` | Resource Events | Read |
| 21 | `gitlab_issue_emoji_list` | Award Emoji | Read |
| 22 | `gitlab_issue_emoji_get` | Award Emoji | Read |
| 23 | `gitlab_issue_emoji_create` | Award Emoji | Create |
| 24 | `gitlab_issue_emoji_delete` | Award Emoji | Delete |
| 25 | `gitlab_issue_note_emoji_list` | Award Emoji | Read |
| 26 | `gitlab_issue_note_emoji_get` | Award Emoji | Read |
| 27 | `gitlab_issue_note_emoji_create` | Award Emoji | Create |
| 28 | `gitlab_issue_note_emoji_delete` | Award Emoji | Delete |
| 29 | `gitlab_mr_emoji_list` | Award Emoji | Read |
| 30 | `gitlab_mr_emoji_get` | Award Emoji | Read |
| 31 | `gitlab_mr_emoji_create` | Award Emoji | Create |
| 32 | `gitlab_mr_emoji_delete` | Award Emoji | Delete |
| 33 | `gitlab_mr_note_emoji_list` | Award Emoji | Read |
| 34 | `gitlab_mr_note_emoji_get` | Award Emoji | Read |
| 35 | `gitlab_mr_note_emoji_create` | Award Emoji | Create |
| 36 | `gitlab_mr_note_emoji_delete` | Award Emoji | Delete |
| 37 | `gitlab_snippet_emoji_list` | Award Emoji | Read |
| 38 | `gitlab_snippet_emoji_get` | Award Emoji | Read |
| 39 | `gitlab_snippet_emoji_create` | Award Emoji | Create |
| 40 | `gitlab_snippet_emoji_delete` | Award Emoji | Delete |
| 41 | `gitlab_snippet_note_emoji_list` | Award Emoji | Read |
| 42 | `gitlab_snippet_note_emoji_get` | Award Emoji | Read |
| 43 | `gitlab_snippet_note_emoji_create` | Award Emoji | Create |
| 44 | `gitlab_snippet_note_emoji_delete` | Award Emoji | Delete |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_issue_emoji_delete` — removes an award emoji from an issue
- `gitlab_issue_note_emoji_delete` — removes an award emoji from an issue note
- `gitlab_mr_emoji_delete` — removes an award emoji from a merge request
- `gitlab_mr_note_emoji_delete` — removes an award emoji from a merge request note
- `gitlab_snippet_emoji_delete` — removes an award emoji from a snippet
- `gitlab_snippet_note_emoji_delete` — removes an award emoji from a snippet note

---

## Related

- [GitLab Notification Settings API](https://docs.gitlab.com/ee/api/notification_settings.html)
- [GitLab Events API](https://docs.gitlab.com/ee/api/events.html)
- [GitLab Resource Label Events API](https://docs.gitlab.com/ee/api/resource_label_events.html)
- [GitLab Resource Milestone Events API](https://docs.gitlab.com/ee/api/resource_milestone_events.html)
- [GitLab Resource State Events API](https://docs.gitlab.com/ee/api/resource_state_events.html)
- [GitLab Award Emoji API](https://docs.gitlab.com/ee/api/award_emoji.html)
