# Custom Emoji — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Custom Emoji
> **Individual tools**: 3
> **Meta-tool**: `gitlab_custom_emoji` (when `META_TOOLS=true`, default)
> **GitLab API**: [Custom Emoji GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#groupcustomemoji)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The custom emoji domain provides management of group-level custom emoji via the GitLab GraphQL API. Custom emoji are distinct from award emoji (reactions on issues/MRs) — they are custom images uploaded to a group that can be used as reactions or in Markdown text across the group's projects.

When `META_TOOLS=true` (the default), all 3 individual tools below are consolidated into a single `gitlab_custom_emoji` meta-tool that dispatches by `action` parameter.

### Common Questions

> "List all custom emoji in my group"
> "Add a party_parrot emoji to the team group"
> "Delete the outdated custom emoji"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Tools

### `gitlab_list_custom_emoji`

List all custom emoji for a GitLab group. Returns a paginated list with ID, name, image URL, and creation date.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_path` | string | Yes | Group full path (e.g. `my-group`) |
| `first` | int | No | Number of items per page (default: 20) |
| `after` | string | No | Cursor for forward pagination |

### `gitlab_create_custom_emoji`

Create a custom emoji in a GitLab group. Requires the group path, emoji name (without colons), and a URL to the emoji image.

| Annotation | **Create** |
| ---------- | ---------- |

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `group_path` | string | Yes | Group full path (e.g. `my-group`) |
| `name` | string | Yes | Emoji name without colons (e.g. `party_parrot`) |
| `url` | string | Yes | URL to the emoji image (PNG or GIF recommended) |

### `gitlab_delete_custom_emoji`

Delete a custom emoji from a GitLab group. Requires the emoji GID.

| Annotation | **Delete** |
| ---------- | ---------- |

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `id` | string | Yes | Custom emoji GID (e.g. `gid://gitlab/CustomEmoji/1`) |

> **Destructive**: Requires user confirmation before execution. The emoji will be removed from all projects in the group.

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_list_custom_emoji` | Query | Read |
| 2 | `gitlab_create_custom_emoji` | Mutation | Create |
| 3 | `gitlab_delete_custom_emoji` | Mutation | Delete |

### Destructive Tools (Require Confirmation)

- `gitlab_delete_custom_emoji` — permanently removes a custom emoji from the group

---

## Notes

- Custom emoji are group-scoped — they are available across all projects within the group
- Emoji names must be unique within a group and should not conflict with built-in GitLab emoji
- Recommended image formats: PNG or GIF (animated GIFs are supported)
- The `external` field indicates whether the emoji image is hosted externally

## Related

- [GitLab Custom Emoji GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#groupcustomemoji)
- [GitLab Custom Emoji](https://docs.gitlab.com/ee/user/emoji_reactions.html#custom-emoji)
