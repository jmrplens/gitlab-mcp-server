# Repository — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Repository (tree, compare, files, commits, submodules, discussions, markdown)
> **Individual tools**: 38
> **Meta-tool**: `gitlab_repository` (when `META_TOOLS=true`, default)
> **GitLab API**: [Repositories API](https://docs.gitlab.com/ee/api/repositories.html), [Repository Files API](https://docs.gitlab.com/ee/api/repository_files.html), [Commits API](https://docs.gitlab.com/ee/api/commits.html), [Commit Discussions API](https://docs.gitlab.com/ee/api/discussions.html#commits), [Repository Submodules API](https://docs.gitlab.com/ee/api/repository_submodules.html), [Markdown API](https://docs.gitlab.com/ee/api/markdown.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The repository domain covers operations on GitLab repository content: browsing the file tree, comparing refs, downloading archives, changelogs, inspecting blobs, managing repository files (CRUD, blame, metadata), commits (list, create, diff, cherry-pick, revert, signatures, statuses, comments), commit discussions, submodule updates, and rendering Markdown.

This domain spans six sub-packages: `repository`, `files`, `commits`, `commitdiscussions`, `repositorysubmodules`, and `markdown`.

When `META_TOOLS=true` (the default), all 38 individual tools below are consolidated into a single `gitlab_repository` meta-tool that dispatches by `action` parameter.

### Common Questions

> "Show me the file tree of project 42"
> "Compare the main and develop branches"
> "Get the contents of README.md in project 42"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Repository Tree & Compare

### `gitlab_repository_tree`

List the files and directories (tree) of a GitLab repository at a given path and ref. Returns file name, type (blob/tree), mode, and path with pagination. Use recursive flag to list all files in subdirectories.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_repository_compare`

Compare two branches, tags, or commits in a GitLab repository. Returns the list of commits between them and the diffs (changed files) with old/new paths and diff text.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_repository_contributors`

List repository contributors with commit, addition, and deletion counts. Supports ordering by name, email, or commits and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_repository_merge_base`

Find the common ancestor (merge base) commit of two or more branches, tags, or commits.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_repository_blob`

Get a git blob by SHA from a repository. Returns the blob content as base64-encoded string. Use tree listing to find blob SHAs.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_repository_raw_blob`

Get the raw text content of a git blob by SHA. Returns the content as plain text. Use for human-readable file content.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_repository_archive`

Get the download URL for a repository archive. Supports tar.gz, tar.bz2, zip formats and optional SHA/branch/tag/path filters. Returns the URL (does not download binary content).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_repository_changelog_add`

Add changelog data to a changelog file by creating a commit. Requires version string. Optionally specify branch, from/to range, config file, and commit message.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_repository_changelog_generate`

Generate changelog data (notes) without committing. Returns the changelog notes as Markdown text. Requires version string.

| Annotation | **Read** |
| ---------- | -------- |

---

## Repository Files

### `gitlab_file_get`

Retrieve the decoded content of a single file from a GitLab repository at a specific ref (branch name, tag name, or commit SHA). Returns file content, size, encoding, and last commit ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_file_create`

Create a new file in a GitLab repository. Requires branch and commit message. Optionally specify encoding (text/base64), start_branch, and author info.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_file_update`

Update an existing file in a GitLab repository. Requires branch and commit message. Supports last_commit_id for optimistic locking.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_file_delete`

Delete a file from a GitLab repository. Requires branch and commit message. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. File deletion cannot be undone.

### `gitlab_file_blame`

Get blame information for a file in a GitLab repository. Shows which commit and author last modified each line range. Supports optional line range filtering.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_file_metadata`

Get file metadata (size, encoding, blob ID, commit IDs, SHA-256) without retrieving content. Useful for checking file existence or properties.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_file_raw`

Get the raw content of a file from a GitLab repository as plain text. Unlike gitlab_file_get, returns unprocessed content without metadata.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_file_raw_metadata`

Get file metadata via HEAD request to the raw file endpoint. Returns size, encoding, blob ID, commit IDs, and SHA-256 without retrieving content. Lighter than gitlab_file_metadata — uses a HEAD request instead of GET.

| Annotation | **Read** |
| ---------- | -------- |

---

## Commits

### `gitlab_commit_list`

List commits in a GitLab repository. Supports filtering by branch/tag (ref_name), date range (since/until in ISO 8601), file path, and author. Optionally includes commit stats (additions/deletions). Returns commit ID, title, author, date, and web URL with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_commit_create`

Create a Git commit with one or more file actions (create, update, delete, move, chmod) in a GitLab repository. Supports multi-file atomic commits on any branch with optional author override.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_commit_get`

Retrieve a single commit by SHA from a GitLab project. Returns commit ID, title, full message, author/committer info, parent IDs, stats (additions/deletions/total), and web URL.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_commit_diff`

List the diffs (changed files) for a specific commit in a GitLab project. Returns old/new paths, diff text, and flags for new/renamed/deleted files with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_commit_refs`

Get branches and tags a commit is pushed to. Returns ref type (branch/tag) and name. Supports filtering by type and pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_commit_comments`

List comments on a specific commit. Returns comment text, file path, line number, and author with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_commit_comment_create`

Post a comment on a commit. Supports file-level inline comments with path and line number.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_commit_statuses`

List pipeline statuses of a commit. Returns status state, name, ref, description, and coverage. Supports filtering by ref, stage, name, and pipeline_id with pagination.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_commit_status_set`

Set the pipeline status of a commit. State can be: pending, running, success, failed, or canceled. Supports optional ref, name, target_url, description, coverage, and pipeline_id.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_commit_merge_requests`

List merge requests associated with a commit. Returns MR IID, title, state, source/target branches, author, and web URL.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_commit_cherry_pick`

Cherry-pick a commit to a target branch. Supports dry_run to check for conflicts without creating the commit, and custom commit message.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_commit_revert`

Revert a commit on a target branch. Creates a new commit that undoes the changes of the specified commit.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_commit_signature`

Get the GPG signature of a commit if it was signed. Returns verification status, key ID, user name, and email.

| Annotation | **Read** |
| ---------- | -------- |

---

## Commit Discussions

### `gitlab_list_commit_discussions`

List discussion threads on a project commit.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_commit_discussion`

Get a single discussion thread on a project commit.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_commit_discussion`

Create a new discussion thread on a project commit. Supports inline diff comments via position.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_add_commit_discussion_note`

Add a reply note to an existing commit discussion thread.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_update_commit_discussion_note`

Update an existing note in a commit discussion thread.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_commit_discussion_note`

Delete a note from a commit discussion thread.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Submodules

### `gitlab_update_repository_submodule`

Update an existing submodule reference in a GitLab repository to point to a new commit SHA.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Markdown Rendering

### `gitlab_render_markdown`

Render arbitrary markdown text to HTML using the GitLab API. Supports GitLab Flavoured Markdown (GFM) and project-scoped references.

| Annotation | **Read** |
| ---------- | -------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_repository_tree` | Tree & Compare | Read |
| 2 | `gitlab_repository_compare` | Tree & Compare | Read |
| 3 | `gitlab_repository_contributors` | Tree & Compare | Read |
| 4 | `gitlab_repository_merge_base` | Tree & Compare | Read |
| 5 | `gitlab_repository_blob` | Tree & Compare | Read |
| 6 | `gitlab_repository_raw_blob` | Tree & Compare | Read |
| 7 | `gitlab_repository_archive` | Tree & Compare | Read |
| 8 | `gitlab_repository_changelog_add` | Tree & Compare | Create |
| 9 | `gitlab_repository_changelog_generate` | Tree & Compare | Read |
| 10 | `gitlab_file_get` | Files | Read |
| 11 | `gitlab_file_create` | Files | Create |
| 12 | `gitlab_file_update` | Files | Update |
| 13 | `gitlab_file_delete` | Files | Delete |
| 14 | `gitlab_file_blame` | Files | Read |
| 15 | `gitlab_file_metadata` | Files | Read |
| 16 | `gitlab_file_raw` | Files | Read |
| 17 | `gitlab_file_raw_metadata` | Files | Read |
| 18 | `gitlab_commit_list` | Commits | Read |
| 19 | `gitlab_commit_create` | Commits | Create |
| 20 | `gitlab_commit_get` | Commits | Read |
| 21 | `gitlab_commit_diff` | Commits | Read |
| 22 | `gitlab_commit_refs` | Commits | Read |
| 23 | `gitlab_commit_comments` | Commits | Read |
| 24 | `gitlab_commit_comment_create` | Commits | Create |
| 25 | `gitlab_commit_statuses` | Commits | Read |
| 26 | `gitlab_commit_status_set` | Commits | Create |
| 27 | `gitlab_commit_merge_requests` | Commits | Read |
| 28 | `gitlab_commit_cherry_pick` | Commits | Create |
| 29 | `gitlab_commit_revert` | Commits | Create |
| 30 | `gitlab_commit_signature` | Commits | Read |
| 31 | `gitlab_list_commit_discussions` | Commit Discussions | Read |
| 32 | `gitlab_get_commit_discussion` | Commit Discussions | Read |
| 33 | `gitlab_create_commit_discussion` | Commit Discussions | Create |
| 34 | `gitlab_add_commit_discussion_note` | Commit Discussions | Create |
| 35 | `gitlab_update_commit_discussion_note` | Commit Discussions | Update |
| 36 | `gitlab_delete_commit_discussion_note` | Commit Discussions | Delete |
| 37 | `gitlab_update_repository_submodule` | Submodules | Update |
| 38 | `gitlab_render_markdown` | Markdown | Read |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_file_delete` — deletes a file from the repository
- `gitlab_delete_commit_discussion_note` — deletes a note from a commit discussion

---

## Related

- [GitLab Repositories API](https://docs.gitlab.com/ee/api/repositories.html)
- [GitLab Repository Files API](https://docs.gitlab.com/ee/api/repository_files.html)
- [GitLab Commits API](https://docs.gitlab.com/ee/api/commits.html)
- [GitLab Commit Discussions API](https://docs.gitlab.com/ee/api/discussions.html#commits)
- [GitLab Repository Submodules API](https://docs.gitlab.com/ee/api/repository_submodules.html)
- [GitLab Markdown API](https://docs.gitlab.com/ee/api/markdown.html)
