# Packages — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Packages, Container Registry & Package Protection Rules
> **Individual tools**: 34
> **Meta-tool**: `gitlab_package` (`GITLAB_MCP_TOOL_SURFACE=meta` catalog)
> **Dynamic IDs**: `dependency.*`, `package.*` (default surface, via `gitlab_execute_action`)
> **GitLab API**: [Packages API](https://docs.gitlab.com/ee/api/packages.html), [Container Registry API](https://docs.gitlab.com/ee/api/container_registry.html), [Package Protection Rules API](https://docs.gitlab.com/ee/api/project_packages_protection_rules.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The packages domain covers the GitLab Generic Package Registry (publish, download, list, delete packages and files) and the Container Registry (repositories, tags, protection rules). It also includes composite operations like publish-and-link (publish a file and create a release asset link in one step) and publish-directory (batch-publish files from a local directory).

On the default dynamic surface, these operations are the `dependency.*`, `package.*` entries of the canonical action catalog: find them with `gitlab_find_action` and run them with `gitlab_execute_action` by `domain.action` ID. With `GITLAB_MCP_TOOL_SURFACE=individual`, each is the tool named in the tables below.

With `GITLAB_MCP_TOOL_SURFACE=meta`, the package-domain tools below are consolidated into the `gitlab_package` meta-tool. It includes generic package actions (`publish`, `download`, `list`, `group_list`, `get`, `file_list`, delete actions), container registry actions with `registry_*` prefixes, container registry protection actions with `registry_rule_*` prefixes, and package protection actions with `protection_rule_*` prefixes. Enterprise/Premium dependency tools remain gated by `GITLAB_MCP_TIER` (Premium or Ultimate).

### Common Questions

> "List packages in project 42"
> "Which other versions of package 17 exist, and which pipeline built each?"
> "Upload a release binary to the package registry"
> "Show container registry images"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description                                    |
| ---------- | :------: | :---------: | :--------: | ---------------------------------------------- |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation                       |
| **Create** |    —     |     No      |     —      | Creates a new resource                         |
| **Update** |    —     |     No      |    Yes     | Modifies an existing resource                  |
| **Delete** |    —     |     Yes     |    Yes     | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Generic Package Registry

> **Local paths are confined to allow-listed roots.** Every `file_path` and `directory_path` below must resolve, after symlinks, under the working directory, the OS temporary directory, or a directory named in `GITLAB_MCP_ALLOWED_UPLOAD_DIRS` (path-list separated: `:` on Unix, `;` on Windows). Destinations written by a download follow the same rule under `GITLAB_MCP_ALLOWED_DOWNLOAD_DIRS`. A server reached over HTTP refuses every local path outright, whatever those variables say: the caller has no files on the machine the server runs on, so `content_base64` is the remote form.

### `gitlab_package_publish`

Publish (upload) a file to the GitLab Generic Package Registry. Provide either file_path (absolute local path) or content_base64 (base64-encoded content), not both. The `file_name` may include `/` to describe a directory structure inside the package; segments between separators must not be empty, `.`, or `..`. Returns the package file ID, size, SHA256, and download URL.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_package_download`

Download a file from the GitLab Generic Package Registry and save it to a local path. Returns the output path, file size, and SHA256 checksum.

The read is of the registry, but the write is to this machine's disk, so the action is classified as mutating rather than read-only: `--read-only` removes it, `--safe-mode` previews it, a `read_api` token is not served it, and a client that auto-approves `readOnlyHint: true` no longer auto-approves a file write. `output_path` must resolve under the working directory, the OS temporary directory, or a directory named in `GITLAB_MCP_ALLOWED_DOWNLOAD_DIRS`; a server reached over HTTP refuses every local path, since the caller has no files on this machine.

`output_path` is written only once the whole file has arrived. The body goes to a temporary file in the directory of the file `output_path` resolves to, its own directory unless it is a symlink (named `.gitlab-mcp-server-download-*.partial`), which is synced and then renamed over that file. On Unix the directory is synced after the rename as well, so a crash soon after the tool reports success cannot undo the rename and leave the previous file, or no file, in its place; a filesystem that refuses to sync a directory does not fail the download, which is already in place, and the server logs it at debug level. A download that fails, whether GitLab answers an error, the connection drops partway or the call is cancelled or reaches the action timeout, removes that temporary file and leaves `output_path` exactly as it was: still absent if it was absent, holding its previous content if it held a file. When the temporary file itself cannot be removed, the error says so beside the reason the download failed. Directories created on the way to `output_path` stay, since they are empty and a retry needs them, and a server killed outright can leave a `.partial` file behind but never a partial `output_path`.

A file already at `output_path` is replaced rather than written through: the result is a new file, and a hard link to the old file keeps the old content. On Unix the new file is readable and writable by its owner alone (mode 0600), whatever the old file allowed; on Windows it takes the access the directory grants to new files, not the old file's own ACL. An `output_path` that is a symlink to a regular file inside the allowed directories is resolved to that file, which is what gets replaced while the link stays; a dangling link, one naming something other than a regular file, or one leading outside the allowed directories is refused. On Unix the replacement is a single atomic rename. The temporary file is created in the directory of the file being replaced, so the server needs write permission on that directory (write permission on the file alone no longer suffices), and the old file stays in place until the new one is complete, so the volume needs room for both. On Windows it fails while another process holds `output_path` open without allowing it to be deleted, as a running program does, or when the file is marked read-only, and `output_path` then keeps its previous content.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_package_list`

List packages in a GitLab project. Can filter by name, version, type, and supports pagination and sorting. When GitLab includes the pipeline that last built a package, the response publishes it under `pipeline` with every key GitLab's pipeline entity sends: the pipeline's `iid`, `project_id` and `source`, and its user's `public_email` and `locked`, which client-go does not decode, are read from the captured response. GitLab's `pipelines` key is not published: it has been deprecated since GitLab 16.1 and GitLab renders it as an empty list whatever the package. A package's other versions are not published either, since GitLab sends them only to `gitlab_package_get`. Every timestamp, the package's, its tags' and its pipeline's, is RFC 3339 to the second, and a tag has the same shape here as on a package version.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_package_get`

Get one package of a project by the `package_id` a listing returns (canonical ID `package.get`, `GET /projects/:id/packages/:package_id`). The answer carries every field `gitlab_package_list` does, and the package's other versions under `versions`, each with its tags and the pipeline that built it, which GitLab sends only to this read and never to a listing. A version's tags and times are published in the shape and to the precision the package's own are, RFC 3339 to the second. The owning project's `project_id` and `project_path` are not among the fields: GitLab sends them only to the group listing. Each version of a package is a package of its own with its own `package_id`, so the package a listing names at version `1.0.0` lists `2.0.0` among its other versions, and the reverse.

GitLab reads only a package whose status is `default` or `deprecated` here, and answers 404 for any other. A listing shows a package in `error` status by default, and one in `hidden`, `processing` or `pending_destruction` when asked for by status, so a `package_id` taken from a listing can answer 404 while the package still exists; the status column of that listing says which it is, and listing again only hands back the same `package_id`. A deleted version answers 404 as well. Either way the tool returns a not-found result naming the package and the project, and both reasons, rather than an error.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_package_file_list`

List files within a specific package. Returns file ID, name, size, and SHA256 for each file with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_list_group_packages`

List packages across a GitLab group and its descendant projects with optional filters and ordering. Supports filtering by package name and type (composer, conan, generic, golang, helm, maven, npm, nuget, pypi, terraform_module) and status (default, hidden, processing, error, pending_destruction, deprecated). Set `exclude_subgroups` to limit results to the group's direct projects, and `include_versionless` to include packages without a version. Order by `created_at`, `name`, `version`, `type`, or `project_path`.

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

## Container Registry — Tag Protection Rules

Tag protection rules restrict who can push or delete container image **tags** that match a pattern, independent of the repository-path protection rules above. Omitting both minimum access levels makes matching tags immutable.

### `gitlab_registry_tag_protection_list`

List container registry tag protection rules for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_registry_tag_protection_create`

Create a container registry tag protection rule. The `tag_name_pattern` is an RE2 regular expression; omit both access levels to make matching tags immutable.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_registry_tag_protection_update`

Update a container registry tag protection rule.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_registry_tag_protection_delete`

Delete a container registry tag protection rule. This action cannot be undone.

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
| 2 | `gitlab_package_download` | Generic Package Registry | Update |
| 3 | `gitlab_package_list` | Generic Package Registry | Read |
| 4 | `gitlab_package_get` | Generic Package Registry | Read |
| 5 | `gitlab_package_file_list` | Generic Package Registry | Read |
| 6 | `gitlab_package_delete` | Generic Package Registry | Delete |
| 7 | `gitlab_package_file_delete` | Generic Package Registry | Delete |
| 8 | `gitlab_package_publish_and_link` | Generic Package Registry | Create |
| 9 | `gitlab_package_publish_directory` | Generic Package Registry | Create |
| 10 | `gitlab_registry_list_project` | Registry Repositories & Tags | Read |
| 11 | `gitlab_registry_list_group` | Registry Repositories & Tags | Read |
| 12 | `gitlab_registry_get_repository` | Registry Repositories & Tags | Read |
| 13 | `gitlab_registry_delete_repository` | Registry Repositories & Tags | Delete |
| 14 | `gitlab_registry_list_tags` | Registry Repositories & Tags | Read |
| 15 | `gitlab_registry_get_tag` | Registry Repositories & Tags | Read |
| 16 | `gitlab_registry_delete_tag` | Registry Repositories & Tags | Delete |
| 17 | `gitlab_registry_delete_tags_bulk` | Registry Repositories & Tags | Delete |
| 18 | `gitlab_registry_protection_list` | Registry Protection Rules | Read |
| 19 | `gitlab_registry_protection_create` | Registry Protection Rules | Create |
| 20 | `gitlab_registry_protection_update` | Registry Protection Rules | Update |
| 21 | `gitlab_registry_protection_delete` | Registry Protection Rules | Delete |
| 22 | `gitlab_registry_tag_protection_list` | Registry Tag Protection Rules | Read |
| 23 | `gitlab_registry_tag_protection_create` | Registry Tag Protection Rules | Create |
| 24 | `gitlab_registry_tag_protection_update` | Registry Tag Protection Rules | Update |
| 25 | `gitlab_registry_tag_protection_delete` | Registry Tag Protection Rules | Delete |
| 26 | `gitlab_list_package_protection_rules` | Package Protection Rules | Read |
| 27 | `gitlab_create_package_protection_rule` | Package Protection Rules | Create |
| 28 | `gitlab_update_package_protection_rule` | Package Protection Rules | Update |
| 29 | `gitlab_delete_package_protection_rule` | Package Protection Rules | Delete |
| 30 | `gitlab_list_project_dependencies` | Dependencies | Read |
| 31 | `gitlab_create_dependency_list_export` | Dependencies | Create |
| 32 | `gitlab_get_dependency_list_export` | Dependencies | Read |
| 33 | `gitlab_download_dependency_list_export` | Dependencies | Read |
| 34 | `gitlab_list_group_packages` | Generic Package Registry | Read |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_package_delete` — deletes an entire package and all its files
- `gitlab_package_file_delete` — deletes a single file from a package
- `gitlab_registry_delete_repository` — deletes a container registry repository
- `gitlab_registry_delete_tag` — deletes a single container registry tag
- `gitlab_registry_delete_tags_bulk` — bulk-deletes tags by regex pattern
- `gitlab_registry_protection_delete` — deletes a registry protection rule
- `gitlab_registry_tag_protection_delete` — deletes a registry tag protection rule
- `gitlab_delete_package_protection_rule` — deletes a package protection rule

---

## Related

- [GitLab Packages API](https://docs.gitlab.com/ee/api/packages.html)
- [GitLab Generic Packages API](https://docs.gitlab.com/user/packages/generic_packages/)
- [GitLab Container Registry API](https://docs.gitlab.com/ee/api/container_registry.html)
- [GitLab Container Registry Protection Rules API](https://docs.gitlab.com/api/container_repository_protection_rules/)
- [GitLab Dependencies API](https://docs.gitlab.com/ee/api/dependencies.html)
- [GitLab Dependency List Export API](https://docs.gitlab.com/ee/api/dependency_list_export.html)
