# Geo & Model Registry — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Geo & Model Registry
> **Individual tools**: 9
> **Meta-tools**: `gitlab_geo`, `gitlab_model_registry` (`GITLAB_MCP_TOOL_SURFACE=meta` catalog)
> **Dynamic IDs**: `geo.*`, `model_registry.*` (default surface, via `gitlab_execute_action`)
> **GitLab API**: [Geo Sites](https://docs.gitlab.com/ee/api/geo_sites.html) · [Model Registry](https://docs.gitlab.com/ee/api/model_registry.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The Geo & Model Registry domain covers GitLab Geo replication site management (create, list, get, edit, delete, repair, status) and ML model registry file downloads.

On the default dynamic surface, these operations are the `geo.*` and `model_registry.*` entries of the canonical action catalog: find them with `gitlab_find_action` and run them with `gitlab_execute_action` by `domain.action` ID. With `GITLAB_MCP_TOOL_SURFACE=individual`, each is the tool named in the tables below.

With `GITLAB_MCP_TOOL_SURFACE=meta`, the individual tools below are consolidated into two meta-tools that dispatch by `action` parameter.

### Common Questions

> "List all Geo sites"
> "Get the replication status of Geo site 1"
> "Download a model package file"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description                                    |
| ---------- | :------: | :---------: | :--------: | ---------------------------------------------- |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation                       |
| **Create** |    —     |     No      |     —      | Creates a new resource                         |
| **Update** |    —     |     No      |    Yes     | Modifies an existing resource                  |
| **Delete** |    —     |     Yes     |    Yes     | Destroys a resource; protected by confirmation |

---

## Geo Site Tools

### gitlab_create_geo_site

| Property      | Value                             |
| ------------- | --------------------------------- |
| **Action**    | Create                            |
| **Meta-tool** | `gitlab_geo` → `action: "create"` |

Create a new Geo replication site.

**Parameters:**

| Name                                  | Type     | Required | Description                                    |
| ------------------------------------- | -------- | :------: | ---------------------------------------------- |
| `name`                                | string   |    —     | Unique name of the Geo site                    |
| `url`                                 | string   |    —     | External URL of the Geo site                   |
| `primary`                             | bool     |    —     | Whether this is a primary site                 |
| `enabled`                             | bool     |    —     | Whether the site is enabled                    |
| `internal_url`                        | string   |    —     | Internal URL of the Geo site                   |
| `files_max_capacity`                  | int64    |    —     | Max LFS/attachment backfill downloads          |
| `repos_max_capacity`                  | int64    |    —     | Max concurrent repository backfill syncs       |
| `verification_max_capacity`           | int64    |    —     | Max concurrent verification jobs               |
| `container_repositories_max_capacity` | int64    |    —     | Max concurrent container repository syncs      |
| `sync_object_storage`                 | bool     |    —     | Whether to sync object-stored data             |
| `selective_sync_type`                 | string   |    —     | Selective sync type: `namespaces` or `shards`  |
| `selective_sync_shards`               | []string |    —     | Storage shards to sync                         |
| `selective_sync_namespace_ids`        | []int64  |    —     | Namespace IDs to sync                          |
| `minimum_reverification_interval`     | int64    |    —     | Minimum interval (days) before re-verification |

---

### gitlab_list_geo_sites

| Property      | Value                           |
| ------------- | ------------------------------- |
| **Action**    | Read                            |
| **Meta-tool** | `gitlab_geo` → `action: "list"` |

List all Geo replication sites with pagination.

**Parameters:** Standard pagination (`page`, `per_page`).

---

### gitlab_get_geo_site

| Property      | Value                          |
| ------------- | ------------------------------ |
| **Action**    | Read                           |
| **Meta-tool** | `gitlab_geo` → `action: "get"` |

Get configuration of a specific Geo site by ID.

**Parameters:**

| Name | Type  | Required | Description                |
| ---- | ----- | :------: | -------------------------- |
| `id` | int64 |    ✅     | Numeric ID of the Geo site |

---

### gitlab_edit_geo_site

| Property      | Value                           |
| ------------- | ------------------------------- |
| **Action**    | Update                          |
| **Meta-tool** | `gitlab_geo` → `action: "edit"` |

Update configuration of an existing Geo site.

**Parameters:**

| Name                                                                          | Type   | Required | Description                 |
| ----------------------------------------------------------------------------- | ------ | :------: | --------------------------- |
| `id`                                                                          | int64  |    ✅     | Numeric ID of the Geo site  |
| `name`                                                                        | string |    —     | Unique name of the Geo site |
| `url`                                                                         | string |    —     | External URL                |
| `enabled`                                                                     | bool   |    —     | Whether the site is enabled |
| _(plus other fields from create, except `primary` and `sync_object_storage`)_ |        |          |                             |

---

### gitlab_delete_geo_site

| Property      | Value                             |
| ------------- | --------------------------------- |
| **Action**    | Delete                            |
| **Meta-tool** | `gitlab_geo` → `action: "delete"` |

Delete a Geo replication site. Protected by confirmation prompt.

**Parameters:**

| Name | Type  | Required | Description                |
| ---- | ----- | :------: | -------------------------- |
| `id` | int64 |    ✅     | Numeric ID of the Geo site |

---

### gitlab_repair_geo_site

| Property      | Value                             |
| ------------- | --------------------------------- |
| **Action**    | Update                            |
| **Meta-tool** | `gitlab_geo` → `action: "repair"` |

Repair the OAuth authentication of a Geo site.

**Parameters:**

| Name | Type  | Required | Description                |
| ---- | ----- | :------: | -------------------------- |
| `id` | int64 |    ✅     | Numeric ID of the Geo site |

---

### gitlab_list_status_all_geo_sites

| Property      | Value                                  |
| ------------- | -------------------------------------- |
| **Action**    | Read                                   |
| **Meta-tool** | `gitlab_geo` → `action: "list_status"` |

Retrieve replication status of all Geo sites.

**Parameters:** Standard pagination (`page`, `per_page`).

---

### gitlab_get_status_geo_site

| Property      | Value                                 |
| ------------- | ------------------------------------- |
| **Action**    | Read                                  |
| **Meta-tool** | `gitlab_geo` → `action: "get_status"` |

Retrieve replication status of a specific Geo site.

**Parameters:**

| Name | Type  | Required | Description                |
| ---- | ----- | :------: | -------------------------- |
| `id` | int64 |    ✅     | Numeric ID of the Geo site |

---

## Model Registry Tools

### gitlab_download_ml_model_package

| Property      | Value                                          |
| ------------- | ---------------------------------------------- |
| **Action**    | Read                                           |
| **Meta-tool** | `gitlab_model_registry` → `action: "download"` |

Download a machine learning model package file. Returns the file content as base64-encoded data.

**Parameters:**

| Name               | Type       | Required | Description                                             |
| ------------------ | ---------- | :------: | ------------------------------------------------------- |
| `project_id`       | string/int |    ✅     | Project ID or URL-encoded path                          |
| `model_version_id` | string/int |    ✅     | Model version ID (numeric or string like `candidate:5`) |
| `path`             | string     |    ✅     | Path within the model package                           |
| `filename`         | string     |    ✅     | Name of the file to download                            |
