# Repository Storage Moves — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Repository Storage Moves
> **Individual tools**: 18
> **Meta-tool**: `gitlab_storage_move` (when `META_TOOLS=true`, default)
> **GitLab API**: [Project Repository Storage Moves](https://docs.gitlab.com/ee/api/project_repository_storage_moves.html) · [Group Repository Storage Moves](https://docs.gitlab.com/ee/api/group_repository_storage_moves.html) · [Snippet Repository Storage Moves](https://docs.gitlab.com/ee/api/snippet_repository_storage_moves.html)
> **Audience**: 👤 GitLab administrators

---

## Overview

Repository storage moves allow GitLab administrators to migrate repositories between storage shards. Three entity types support storage moves: projects, groups, and snippets. Each entity type provides six operations: list all moves, list moves for a specific entity, get a single move, get a move for a specific entity, schedule a move, and schedule moves for all entities.

All operations require **admin access**.

When `META_TOOLS=true` (the default), all 18 tools are consolidated into a single `gitlab_storage_move` meta-tool with an `action` parameter.

### Common Questions

> "List all in-progress project storage moves"
> "Schedule a storage move for project 42 to the new shard"
> "Check the status of a group storage move"

### Annotation Legend

| Annotation   | ReadOnly | Destructive | Idempotent | Description                     |
| ------------ | :------: | :---------: | :--------: | ------------------------------- |
| **Read**     |   Yes    |     No      |    Yes     | Safe read-only operation        |
| **Create**   |    —     |     No      |     —      | Schedules a new storage move    |

---

## Project Storage Moves

### `gitlab_retrieve_all_project_storage_moves`

Retrieve all project repository storage moves (admin only). Returns a paginated list of all project storage moves across the instance.

| Parameter  | Type | Required | Description          |
| ---------- | ---- | :------: | -------------------- |
| `page`     | int  |    —     | Page number          |
| `per_page` | int  |    —     | Results per page     |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_retrieve_project_storage_moves`

Retrieve all repository storage moves for a specific project (admin only).

| Parameter    | Type | Required | Description      |
| ------------ | ---- | :------: | ---------------- |
| `project_id` | int  |   Yes    | Project ID       |
| `page`       | int  |    —     | Page number      |
| `per_page`   | int  |    —     | Results per page |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_project_storage_move`

Get a single project repository storage move by ID (admin only).

| Parameter | Type | Required | Description       |
| --------- | ---- | :------: | ----------------- |
| `id`      | int  |   Yes    | Storage move ID   |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_project_storage_move_for_project`

Get a single repository storage move for a specific project (admin only).

| Parameter    | Type | Required | Description      |
| ------------ | ---- | :------: | ---------------- |
| `project_id` | int  |   Yes    | Project ID       |
| `id`         | int  |   Yes    | Storage move ID  |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_schedule_project_storage_move`

Schedule a repository storage move for a project (admin only). Optionally specify a destination storage name.

| Parameter                    | Type   | Required | Description                    |
| ---------------------------- | ------ | :------: | ------------------------------ |
| `project_id`                 | int    |   Yes    | Project ID                     |
| `destination_storage_name`   | string |    —     | Target storage shard name      |

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_schedule_all_project_storage_moves`

Schedule repository storage moves for all projects (admin only). Migrates all projects from one storage shard to another.

| Parameter                    | Type   | Required | Description                    |
| ---------------------------- | ------ | :------: | ------------------------------ |
| `source_storage_name`        | string |    —     | Source storage shard name      |
| `destination_storage_name`   | string |    —     | Target storage shard name      |

| Annotation | **Create** |
| ---------- | ---------- |

---

## Group Storage Moves

### `gitlab_retrieve_all_group_storage_moves`

Retrieve all group repository storage moves (admin only). Returns a paginated list of all group storage moves across the instance.

| Parameter  | Type | Required | Description          |
| ---------- | ---- | :------: | -------------------- |
| `page`     | int  |    —     | Page number          |
| `per_page` | int  |    —     | Results per page     |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_retrieve_group_storage_moves`

Retrieve all repository storage moves for a specific group (admin only).

| Parameter  | Type | Required | Description      |
| ---------- | ---- | :------: | ---------------- |
| `group_id` | int  |   Yes    | Group ID         |
| `page`     | int  |    —     | Page number      |
| `per_page` | int  |    —     | Results per page |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_group_storage_move`

Get a single group repository storage move by ID (admin only).

| Parameter | Type | Required | Description       |
| --------- | ---- | :------: | ----------------- |
| `id`      | int  |   Yes    | Storage move ID   |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_group_storage_move_for_group`

Get a single repository storage move for a specific group (admin only).

| Parameter  | Type | Required | Description      |
| ---------- | ---- | :------: | ---------------- |
| `group_id` | int  |   Yes    | Group ID         |
| `id`       | int  |   Yes    | Storage move ID  |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_schedule_group_storage_move`

Schedule a repository storage move for a group (admin only). Optionally specify a destination storage name.

| Parameter                    | Type   | Required | Description                    |
| ---------------------------- | ------ | :------: | ------------------------------ |
| `group_id`                   | int    |   Yes    | Group ID                       |
| `destination_storage_name`   | string |    —     | Target storage shard name      |

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_schedule_all_group_storage_moves`

Schedule repository storage moves for all groups (admin only). Migrates all groups from one storage shard to another.

| Parameter                    | Type   | Required | Description                    |
| ---------------------------- | ------ | :------: | ------------------------------ |
| `source_storage_name`        | string |    —     | Source storage shard name      |
| `destination_storage_name`   | string |    —     | Target storage shard name      |

| Annotation | **Create** |
| ---------- | ---------- |

---

## Snippet Storage Moves

### `gitlab_retrieve_all_snippet_storage_moves`

Retrieve all snippet repository storage moves (admin only). Returns a paginated list of all snippet storage moves across the instance.

| Parameter  | Type | Required | Description          |
| ---------- | ---- | :------: | -------------------- |
| `page`     | int  |    —     | Page number          |
| `per_page` | int  |    —     | Results per page     |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_retrieve_snippet_storage_moves`

Retrieve all repository storage moves for a specific snippet (admin only).

| Parameter    | Type | Required | Description      |
| ------------ | ---- | :------: | ---------------- |
| `snippet_id` | int  |   Yes    | Snippet ID       |
| `page`       | int  |    —     | Page number      |
| `per_page`   | int  |    —     | Results per page |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_snippet_storage_move`

Get a single snippet repository storage move by ID (admin only).

| Parameter | Type | Required | Description       |
| --------- | ---- | :------: | ----------------- |
| `id`      | int  |   Yes    | Storage move ID   |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_snippet_storage_move_for_snippet`

Get a single repository storage move for a specific snippet (admin only).

| Parameter    | Type | Required | Description      |
| ------------ | ---- | :------: | ---------------- |
| `snippet_id` | int  |   Yes    | Snippet ID       |
| `id`         | int  |   Yes    | Storage move ID  |

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_schedule_snippet_storage_move`

Schedule a repository storage move for a snippet (admin only). Optionally specify a destination storage name.

| Parameter                    | Type   | Required | Description                    |
| ---------------------------- | ------ | :------: | ------------------------------ |
| `snippet_id`                 | int    |   Yes    | Snippet ID                     |
| `destination_storage_name`   | string |    —     | Target storage shard name      |

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_schedule_all_snippet_storage_moves`

Schedule repository storage moves for all snippets (admin only). Migrates all snippets from one storage shard to another.

| Parameter                    | Type   | Required | Description                    |
| ---------------------------- | ------ | :------: | ------------------------------ |
| `source_storage_name`        | string |    —     | Source storage shard name      |
| `destination_storage_name`   | string |    —     | Target storage shard name      |

| Annotation | **Create** |
| ---------- | ---------- |

---

## Meta-tool: `gitlab_storage_move`

When `META_TOOLS=true`, all 18 tools are available through a single `gitlab_storage_move` meta-tool. Use the `action` parameter to select the operation.

### Action Mapping

| Action                     | Equivalent Tool                                    |
| -------------------------- | -------------------------------------------------- |
| `retrieve_all_project`     | `gitlab_retrieve_all_project_storage_moves`        |
| `retrieve_project`         | `gitlab_retrieve_project_storage_moves`            |
| `get_project`              | `gitlab_get_project_storage_move`                  |
| `get_project_for_project`  | `gitlab_get_project_storage_move_for_project`      |
| `schedule_project`         | `gitlab_schedule_project_storage_move`             |
| `schedule_all_project`     | `gitlab_schedule_all_project_storage_moves`        |
| `retrieve_all_group`       | `gitlab_retrieve_all_group_storage_moves`          |
| `retrieve_group`           | `gitlab_retrieve_group_storage_moves`              |
| `get_group`                | `gitlab_get_group_storage_move`                    |
| `get_group_for_group`      | `gitlab_get_group_storage_move_for_group`          |
| `schedule_group`           | `gitlab_schedule_group_storage_move`               |
| `schedule_all_group`       | `gitlab_schedule_all_group_storage_moves`          |
| `retrieve_all_snippet`     | `gitlab_retrieve_all_snippet_storage_moves`        |
| `retrieve_snippet`         | `gitlab_retrieve_snippet_storage_moves`            |
| `get_snippet`              | `gitlab_get_snippet_storage_move`                  |
| `get_snippet_for_snippet`  | `gitlab_get_snippet_storage_move_for_snippet`      |
| `schedule_snippet`         | `gitlab_schedule_snippet_storage_move`             |
| `schedule_all_snippet`     | `gitlab_schedule_all_snippet_storage_moves`        |
