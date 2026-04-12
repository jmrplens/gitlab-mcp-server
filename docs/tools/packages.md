# Packages — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Packages, Container Registry & Package Protection Rules
> **Individual tools**: 28
> **Meta-tools**: `gitlab_package`, `gitlab_registry`, `gitlab_registry_protection` (when `META_TOOLS=true`, default)
> **GitLab API**: [Packages API](https://docs.gitlab.com/ee/api/packages.html), [Container Registry API](https://docs.gitlab.com/ee/api/container_registry.html), [Package Protection Rules API](https://docs.gitlab.com/ee/api/project_packages_protection_rules.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The packages domain covers the GitLab Generic Package Registry (publish, download, list, delete packages and files) and the Container Registry (repositories, tags, protection rules). It also includes composite operations like publish-and-link (publish a file and create a release asset link in one step) and publish-directory (batch-publish files from a local directory).

When `META_TOOLS=true` (the default), the 24 individual tools below are consolidated into three meta-tools: `gitlab_package` (12 actions including 4 protection rules), `gitlab_registry` (8 actions), and `gitlab_registry_protection` (4 actions).

### Common Questions

> "List packages in project 42"
> "Upload a release binary to the package registry"
> "Show container registry images"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Generic Package Registry

### `gitlab_package_publish`

Publish (upload) a file to the GitLab Generic Package Registry. Provide either file_path (absolute local path) or content_base64 (base64-encoded content), not both. Returns the package file ID, size, SHA256, and download URL.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_package_download`

Download a file from the GitLab Generic Package Registry and save it to a local path. Returns the output path, file size, and SHA256 checksum.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_package_list`

List packages in a GitLab project. Can filter by name, version, type, and supports pagination and sorting.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_package_file_list`

List files within a specific package. Returns file ID, name, size, and SHA256 for each file with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_package_delete`

Delete a package and all its files from the GitLab Package Registry. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Deletes the entire package and all its files.

### `gitlab_package_file_delete`

Delete a single file from a package in the GitLab Package Registry. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

### `gitlab_package_publish_and_link`

Publish a file to the Generic Package Registry and create a release asset link pointing to it in one step. Provide either file_path or content_base64 for the file content. The release identified by tag_name must already exist.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_package_publish_directory`

Publish all matching files from a local directory to the Generic Package Registry. Walks the directory (non-recursive), filters by an optional glob pattern, and publishes each file. Returns the list of published files with checksums and URLs.

| Annotation | **Create** |
| ---------- | ---------- |

---

## Container Registry — Repositories & Tags

### `gitlab_registry_list_project`

List container registry repositories for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_registry_list_group`

List container registry repositories for a GitLab group.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_registry_get_repository`

Get details of a single container registry repository by its ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_registry_delete_repository`

Delete a container registry repository. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Deletion cannot be undone.

### `gitlab_registry_list_tags`

List tags for a container registry repository.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_registry_get_tag`

Get details of a specific container registry repository tag.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_registry_delete_tag`

Delete a single container registry repository tag. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Deletion cannot be undone.

### `gitlab_registry_delete_tags_bulk`

Delete container registry repository tags in bulk using regex patterns. Use name_regex_delete to match tags to delete and name_regex_keep to exclude tags from deletion.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Bulk deletion cannot be undone.

---

## Package Protection Rules

Manage package protection rules that restrict who can push, update, or delete packages matching specific name patterns.

### `gitlab_list_package_protection_rules`

List all package protection rules for a project. Returns rules with their package name patterns, package types, and minimum access levels for push and delete operations.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_package_protection_rule`

Create a new package protection rule for a project. Define a package name pattern (supports `*` wildcard), package type, and minimum access levels required for push and delete.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_update_package_protection_rule`

Update an existing package protection rule. Modify the package name pattern, package type, or minimum access levels for push and delete operations.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_package_protection_rule`

Delete a package protection rule. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Deletion cannot be undone.

---

## Container Registry — Protection Rules

### `gitlab_registry_protection_list`

List container registry protection rules for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_registry_protection_create`

Create a container registry protection rule to restrict push/delete access by minimum access level.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_registry_protection_update`

Update a container registry protection rule.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_registry_protection_delete`

Delete a container registry protection rule. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Deletion cannot be undone.

---

## Project Dependencies

### `gitlab_list_project_dependencies`

List dependencies for a GitLab project. Supports filtering by package manager (bundler, composer, go, gradle, maven, npm, nuget, pip, etc.). Returns name, version, package manager, file path, vulnerabilities, and licenses.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_dependency_list_export`

Create a dependency list export (SBOM) for a pipeline. Returns export ID and status. Use `gitlab_get_dependency_list_export` to check status, then `gitlab_download_dependency_list_export` to download.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_get_dependency_list_export`

Check the status of a dependency list export. Returns export ID, completion status, and download URL when ready.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_download_dependency_list_export`

Download a completed dependency list export (CycloneDX SBOM JSON). Returns raw SBOM content (limited to 1 MB).

| Annotation | **Read** |
| ---------- | -------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_package_publish` | Generic Package Registry | Create |
| 2 | `gitlab_package_download` | Generic Package Registry | Read |
| 3 | `gitlab_package_list` | Generic Package Registry | Read |
| 4 | `gitlab_package_file_list` | Generic Package Registry | Read |
| 5 | `gitlab_package_delete` | Generic Package Registry | Delete |
| 6 | `gitlab_package_file_delete` | Generic Package Registry | Delete |
| 7 | `gitlab_package_publish_and_link` | Generic Package Registry | Create |
| 8 | `gitlab_package_publish_directory` | Generic Package Registry | Create |
| 9 | `gitlab_registry_list_project` | Registry Repositories & Tags | Read |
| 10 | `gitlab_registry_list_group` | Registry Repositories & Tags | Read |
| 11 | `gitlab_registry_get_repository` | Registry Repositories & Tags | Read |
| 12 | `gitlab_registry_delete_repository` | Registry Repositories & Tags | Delete |
| 13 | `gitlab_registry_list_tags` | Registry Repositories & Tags | Read |
| 14 | `gitlab_registry_get_tag` | Registry Repositories & Tags | Read |
| 15 | `gitlab_registry_delete_tag` | Registry Repositories & Tags | Delete |
| 16 | `gitlab_registry_delete_tags_bulk` | Registry Repositories & Tags | Delete |
| 17 | `gitlab_registry_protection_list` | Registry Protection Rules | Read |
| 18 | `gitlab_registry_protection_create` | Registry Protection Rules | Create |
| 19 | `gitlab_registry_protection_update` | Registry Protection Rules | Update |
| 20 | `gitlab_registry_protection_delete` | Registry Protection Rules | Delete |
| 21 | `gitlab_list_package_protection_rules` | Package Protection Rules | Read |
| 22 | `gitlab_create_package_protection_rule` | Package Protection Rules | Create |
| 23 | `gitlab_update_package_protection_rule` | Package Protection Rules | Update |
| 24 | `gitlab_delete_package_protection_rule` | Package Protection Rules | Delete |
| 25 | `gitlab_list_project_dependencies` | Dependencies | Read |
| 26 | `gitlab_create_dependency_list_export` | Dependencies | Create |
| 27 | `gitlab_get_dependency_list_export` | Dependencies | Read |
| 28 | `gitlab_download_dependency_list_export` | Dependencies | Read |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_package_delete` — deletes an entire package and all its files
- `gitlab_package_file_delete` — deletes a single file from a package
- `gitlab_registry_delete_repository` — deletes a container registry repository
- `gitlab_registry_delete_tag` — deletes a single container registry tag
- `gitlab_registry_delete_tags_bulk` — bulk-deletes tags by regex pattern
- `gitlab_registry_protection_delete` — deletes a registry protection rule
- `gitlab_delete_package_protection_rule` — deletes a package protection rule

---

## Related

- [GitLab Packages API](https://docs.gitlab.com/ee/api/packages.html)
- [GitLab Generic Packages API](https://docs.gitlab.com/ee/api/packages/generic.html)
- [GitLab Container Registry API](https://docs.gitlab.com/ee/api/container_registry.html)
- [GitLab Container Registry Protection Rules API](https://docs.gitlab.com/ee/api/container_registry_protection_rules.html)
- [GitLab Dependencies API](https://docs.gitlab.com/ee/api/dependencies.html)
- [GitLab Dependency List Export API](https://docs.gitlab.com/ee/api/dependency_list_export.html)
