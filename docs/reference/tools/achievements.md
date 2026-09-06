# Achievements — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Achievements
> **Individual tools**: 12
> **Meta-tool**: `gitlab_achievement` (`GITLAB_MCP_TOOL_SURFACE=meta` catalog)
> **Dynamic IDs**: `achievement.*` (default surface, via `gitlab_execute_action`)
> **GitLab API**: [Achievement GraphQL API](https://docs.gitlab.com/api/graphql/reference/#achievement)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The achievements domain manages the badges a namespace defines and the awards handed out from them, through the GitLab GraphQL API. A group or project defines an **achievement**, and awarding it to a person creates a **user achievement**: a separate record, with its own numeric ID, that can be revoked, hidden from a profile, reordered, or erased independently of the badge behind it.

Achievements are available on every tier (Free, Premium and Ultimate) on GitLab.com and GitLab Self-Managed. They became generally available in GitLab 19.3, when the `achievements` feature flag was removed. On an instance older than 19.2 the mutations exist but the feature is off by default, and an administrator has to enable that flag before any of these tools return data.

On the default dynamic surface, these operations are the `achievement.*` entries of the canonical action catalog: find them with `gitlab_find_action` and run them with `gitlab_execute_action` by `domain.action` ID. With `GITLAB_MCP_TOOL_SURFACE=individual`, each is the tool named in the tables below.

With `GITLAB_MCP_TOOL_SURFACE=meta`, all 12 individual tools below are consolidated into a single `gitlab_achievement` meta-tool that dispatches by `action` parameter.

### The two identifiers

Almost every mistake in this domain is a confusion between two numbers:

| Identifier            | Names                                 | Produced by                                                            | Consumed by                                                                                                                                |
| --------------------- | ------------------------------------- | ---------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `achievement_id`      | A badge the namespace defines         | `achievement.list`, `achievement.create`                               | `achievement.update`, `achievement.delete`, `achievement.award`, `achievement.recipients`, `achievement.unique_users`                      |
| `user_achievement_id` | One award of that badge to one person | `achievement.award`, `achievement.recipients`, `achievement.user_list` | `achievement.revoke`, `achievement.user_achievement_update`, `achievement.user_achievement_delete`, `achievement.user_achievement_reorder` |

Both are plain numbers. The GraphQL global-ID form (`gid://gitlab/Achievements::Achievement/15`) is built by the server and is never an input.

### Removing things: three different operations

| Intent                                         | Action                                | Effect                                                           |
| ---------------------------------------------- | ------------------------------------- | ---------------------------------------------------------------- |
| Take the badge back from one person, on record | `achievement.revoke`                  | Keeps the award and stamps `revoked_at` and `revoked_by_user_id` |
| Erase one award, leaving no trace              | `achievement.user_achievement_delete` | Removes the award record outright                                |
| Retire the badge for everyone                  | `achievement.delete`                  | Removes the achievement and every award made from it             |

### Common Questions

> "What achievements does the platform group define?"
> "Give the First Contribution badge to user 42"
> "Who holds the First Contribution achievement?"
> "Hide that award from my profile"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description                                    |
| ---------- | :------: | :---------: | :--------: | ---------------------------------------------- |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation                       |
| **Create** |    —     |     No      |     —      | Creates a new resource                         |
| **Update** |    —     |     No      |    Yes     | Changes an existing resource                   |
| **Delete** |    —     |     Yes     |    Yes     | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

### Pagination

The four list tools paginate by GraphQL cursor, not by page number. They accept `first`/`after` for forward paging and `last`/`before` for backward paging, and reject `page` and `per_page`. `first` defaults to 20 and is clamped to 100. Take the next cursor from `pagination.end_cursor` in the previous response.

### Sending an avatar

`gitlab_achievement_create` and `gitlab_achievement_update` accept an avatar image by either of two routes, and exactly one of them at a time:

- `avatar_file_path` — an absolute path the MCP server reads from its own disk. It is refused when the server is reached over HTTP, because the path would name the server's filesystem rather than the caller's.
- `avatar_content_base64` — the image bytes inline. This is the route that works in every deployment.

`avatar_filename` is required with either route, because GitLab does not treat an upload part without a file name as a file. `avatar_content_type` is optional and defaults to `application/octet-stream`.

---

## Tools

### `gitlab_achievement_list`

List the achievements a group or project namespace defines, with cursor pagination. This is the tool that turns a namespace path into the numeric `achievement_id` every other tool needs.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter   | Type   | Required | Description                                                       |
| ----------- | ------ | :------: | ----------------------------------------------------------------- |
| `full_path` | string |   Yes    | Full path of the group or project (e.g. `my-group/my-project`)    |
| `ids`       | int[]  |    No    | Numeric achievement IDs to restrict the result to                 |
| `first`     | int    |    No    | Items to return from the start of the page (default 20, max 100)  |
| `after`     | string |    No    | Cursor for forward pagination, from `pagination.end_cursor`       |
| `last`      | int    |    No    | Items to return from the end of the page, for backward pagination |
| `before`    | string |    No    | Cursor for backward pagination, from `pagination.start_cursor`    |

### `gitlab_achievement_create`

Define a new achievement in a namespace, addressed by its numeric `namespace_id`. Creating an achievement gives it to nobody.

| Annotation | **Create** |
| ---------- | ---------- |

| Parameter               | Type   | Required | Description                                                         |
| ----------------------- | ------ | :------: | ------------------------------------------------------------------- |
| `namespace_id`          | int    |   Yes    | Numeric ID of the owning group or project namespace                 |
| `name`                  | string |   Yes    | Display name of the achievement (e.g. `First Contribution`)         |
| `description`           | string |    No    | What the achievement is awarded for                                 |
| `avatar_filename`       | string |    No    | File name for the avatar image; required whenever an avatar is sent |
| `avatar_content_type`   | string |    No    | MIME type of the avatar (default `application/octet-stream`)        |
| `avatar_file_path`      | string |    No    | Absolute local path to the image; unavailable over HTTP             |
| `avatar_content_base64` | string |    No    | Base64-encoded image bytes; alternative to `avatar_file_path`       |

### `gitlab_achievement_update`

Change an existing achievement's name, description, or avatar. Every field is optional and an omitted one keeps its current value, so this cannot clear a description.

| Annotation | **Update** |
| ---------- | ---------- |

| Parameter               | Type   | Required | Description                                             |
| ----------------------- | ------ | :------: | ------------------------------------------------------- |
| `achievement_id`        | int    |   Yes    | Numeric ID of the achievement to change                 |
| `name`                  | string |    No    | New display name                                        |
| `description`           | string |    No    | New description                                         |
| `avatar_filename`       | string |    No    | File name for the avatar image                          |
| `avatar_content_type`   | string |    No    | MIME type of the avatar                                 |
| `avatar_file_path`      | string |    No    | Absolute local path to the image; unavailable over HTTP |
| `avatar_content_base64` | string |    No    | Base64-encoded image bytes                              |

### `gitlab_achievement_delete`

Delete an achievement definition. Every award ever made from it is removed with it.

| Annotation | **Delete** |
| ---------- | ---------- |

| Parameter        | Type | Required | Description                             |
| ---------------- | ---- | :------: | --------------------------------------- |
| `achievement_id` | int  |   Yes    | Numeric ID of the achievement to delete |

> **Destructive**: Requires user confirmation before execution. To take a badge back from one person instead, use `gitlab_achievement_revoke`.

### `gitlab_achievement_award`

Award an achievement to one user, creating an award record with its own ID. That new ID, not the `achievement_id` passed in, is what the revoke and user-achievement tools take.

| Annotation | **Create** |
| ---------- | ---------- |

| Parameter        | Type   | Required | Description                                     |
| ---------------- | ------ | :------: | ----------------------------------------------- |
| `achievement_id` | int    |   Yes    | Numeric ID of the achievement to hand out       |
| `user_id`        | int    |   Yes    | Numeric ID of the recipient                     |
| `award_message`  | string |    No    | Note shown with the award, up to 200 characters |

### `gitlab_achievement_revoke`

Revoke one award, keeping the record and stamping who revoked it and when.

| Annotation | **Delete** |
| ---------- | ---------- |

| Parameter             | Type | Required | Description                       |
| --------------------- | ---- | :------: | --------------------------------- |
| `user_achievement_id` | int  |   Yes    | Numeric ID of the award to revoke |

> **Destructive**: Requires user confirmation before execution. The award record survives, marked revoked.

### `gitlab_achievement_user_achievement_update`

Change one award, which today means whether the recipient's profile displays it.

| Annotation | **Update** |
| ---------- | ---------- |

| Parameter             | Type | Required | Description                                                     |
| --------------------- | ---- | :------: | --------------------------------------------------------------- |
| `user_achievement_id` | int  |   Yes    | Numeric ID of the award to change                               |
| `show_on_profile`     | bool |    No    | Whether the profile displays the award; omit to leave unchanged |

### `gitlab_achievement_user_achievement_delete`

Erase one award record outright, leaving no trace that the badge was ever held.

| Annotation | **Delete** |
| ---------- | ---------- |

| Parameter             | Type | Required | Description                       |
| --------------------- | ---- | :------: | --------------------------------- |
| `user_achievement_id` | int  |   Yes    | Numeric ID of the award to delete |

> **Destructive**: Requires user confirmation before execution. Use `gitlab_achievement_revoke` instead when the history should be kept.

### `gitlab_achievement_user_achievement_reorder`

Set the order one user's awards appear in on their profile, highest priority first. The whole sequence is replaced, so pass every award ID of that user in the wanted order.

| Annotation | **Update** |
| ---------- | ---------- |

| Parameter              | Type  | Required | Description                                                   |
| ---------------------- | ----- | :------: | ------------------------------------------------------------- |
| `user_achievement_ids` | int[] |   Yes    | Award IDs in the wanted order, all belonging to the same user |

### `gitlab_achievement_user_list`

List the awards one user holds, found by username. This is the person-centred view; `gitlab_achievement_recipients` answers the opposite question.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter        | Type   | Required | Description                                                      |
| ---------------- | ------ | :------: | ---------------------------------------------------------------- |
| `username`       | string |   Yes    | Account name, without a leading `@`                              |
| `include_hidden` | bool   |    No    | Include awards hidden from the profile (restricted visibility)   |
| `first`          | int    |    No    | Items to return from the start of the page (default 20, max 100) |
| `after`          | string |    No    | Cursor for forward pagination                                    |
| `last`           | int    |    No    | Items to return from the end of the page                         |
| `before`         | string |    No    | Cursor for backward pagination                                   |

> `include_hidden` only changes the result for the user themself and for namespace or instance maintainers and owners. Any other caller sees the same list either way.

### `gitlab_achievement_recipients`

List every award of one achievement, including repeat awards to the same person and awards already revoked. Each entry carries the `user_achievement_id` that revoke and delete take.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter        | Type   | Required | Description                                                      |
| ---------------- | ------ | :------: | ---------------------------------------------------------------- |
| `full_path`      | string |   Yes    | Full path of the namespace that owns the achievement             |
| `achievement_id` | int    |   Yes    | Numeric ID of the achievement                                    |
| `first`          | int    |    No    | Items to return from the start of the page (default 20, max 100) |
| `after`          | string |    No    | Cursor for forward pagination                                    |
| `last`           | int    |    No    | Items to return from the end of the page                         |
| `before`         | string |    No    | Cursor for backward pagination                                   |

### `gitlab_achievement_unique_users`

List the distinct users who hold one achievement, counting a person once however many times they were awarded it. Returns user profiles, so it carries no `user_achievement_id`.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter        | Type   | Required | Description                                                      |
| ---------------- | ------ | :------: | ---------------------------------------------------------------- |
| `full_path`      | string |   Yes    | Full path of the namespace that owns the achievement             |
| `achievement_id` | int    |   Yes    | Numeric ID of the achievement                                    |
| `first`          | int    |    No    | Items to return from the start of the page (default 20, max 100) |
| `after`          | string |    No    | Cursor for forward pagination                                    |
| `last`           | int    |    No    | Items to return from the end of the page                         |
| `before`         | string |    No    | Cursor for backward pagination                                   |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_achievement_list` | Query | Read |
| 2 | `gitlab_achievement_user_list` | Query | Read |
| 3 | `gitlab_achievement_recipients` | Query | Read |
| 4 | `gitlab_achievement_unique_users` | Query | Read |
| 5 | `gitlab_achievement_create` | Mutation | Create |
| 6 | `gitlab_achievement_award` | Mutation | Create |
| 7 | `gitlab_achievement_update` | Mutation | Update |
| 8 | `gitlab_achievement_user_achievement_update` | Mutation | Update |
| 9 | `gitlab_achievement_user_achievement_reorder` | Mutation | Update |
| 10 | `gitlab_achievement_delete` | Mutation | Delete |
| 11 | `gitlab_achievement_revoke` | Mutation | Delete |
| 12 | `gitlab_achievement_user_achievement_delete` | Mutation | Delete |

### Destructive Tools (Require Confirmation)

- `gitlab_achievement_delete` — removes the achievement and every award made from it
- `gitlab_achievement_revoke` — takes a badge back from one holder, keeping the record
- `gitlab_achievement_user_achievement_delete` — erases one award record outright

---

## Notes

- Achievements are namespace-scoped: a group or project defines them, and they are awarded to individual users.
- `achievement.create` addresses the namespace by numeric `namespace_id`, while the list and recipient tools address it by `full_path`. This asymmetry comes from the GraphQL schema.
- `gitlab_achievement_recipients` returns revoked awards alongside live ones. Read `revoked_at` to tell them apart.
- Reordering replaces the whole priority sequence rather than moving one entry, so read the current order with `gitlab_achievement_user_list` first.
- Every endpoint is GraphQL. A missing achievement, a missing namespace, and a feature that is switched off all arrive as the same not-found error, which is why the error hints name the availability caveat.

## Related

- [GitLab Achievements](https://docs.gitlab.com/user/profile/achievements/)
- [Achievement GraphQL type](https://docs.gitlab.com/api/graphql/reference/#achievement)
- [UserAchievement GraphQL type](https://docs.gitlab.com/api/graphql/reference/#userachievement)
- [GraphQL Integration](../../concepts/graphql.md)
