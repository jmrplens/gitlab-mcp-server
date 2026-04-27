# MCP Resources Reference

This document lists all **29 MCP resources** exposed by gitlab-mcp-server. Resources provide read-only, URI-addressable data that MCP clients can subscribe to or fetch on demand.

> **Diátaxis type**: Reference
> **Audience**: MCP client developers, AI assistant users
> **Prerequisites**: Understanding of MCP resources concept

All resources return `application/json` MIME type.

---

## Static Resources (3)

Static resources have a fixed URI and require no parameters.

| # | Name | URI | Description |
|---|------|-----|-------------|
| 1 | `current_user` | `gitlab://user/current` | Get the currently authenticated GitLab user profile. Returns username, display name, email, state (active/blocked), admin status, and web URL. |
| 2 | `groups` | `gitlab://groups` | List all GitLab groups accessible to the authenticated user. Returns each group's ID, name, full path, description, visibility level, and web URL. |
| 3 | `workspace_roots` | `gitlab://workspace/roots` | List workspace root directories provided by the MCP client. Use these paths to locate .git/config files and extract git remote URLs for project discovery via `gitlab_discover_project`. |

## Resource Templates (21)

Resource templates use URI variables (e.g., `{project_id}`) that the client fills in at request time.

### Project Resources

| # | Name | URI Template | Description |
|---|------|--------------|-------------|
| 4 | `project` | `gitlab://project/{project_id}` | Get basic metadata for a GitLab project by numeric ID or URL-encoded path. Returns name, namespace path, visibility, web URL, description, and default branch. |
| 5 | `project_members` | `gitlab://project/{project_id}/members` | List all members of a GitLab project with their access levels (10=guest, 20=reporter, 30=developer, 40=maintainer, 50=owner). Includes inherited members from parent groups. |
| 6 | `project_labels` | `gitlab://project/{project_id}/labels` | List all labels defined in a GitLab project. Returns each label's name, color, description, and counts of open issues and merge requests using the label. |
| 7 | `project_milestones` | `gitlab://project/{project_id}/milestones` | List all milestones in a GitLab project. Returns each milestone's title, description, state (active/closed), due date, and web URL. |
| 8 | `project_branches` | `gitlab://project/{project_id}/branches` | List all branches in a GitLab project. Returns each branch's name, protection status, merge status, default flag, and web URL. |
| 9 | `project_issues` | `gitlab://project/{project_id}/issues` | List open issues for a GitLab project. Returns each issue's IID, title, state, labels, assignees, author, web URL, and creation date. |
| 10 | `project_releases` | `gitlab://project/{project_id}/releases` | List all releases for a GitLab project. Returns each release's tag name, name, description, author, and creation/release dates. |
| 11 | `project_tags` | `gitlab://project/{project_id}/tags` | List all repository tags for a GitLab project. Returns each tag's name, message, target commit SHA, protection status, and creation date. |
| 12 | `commit` | `gitlab://project/{project_id}/commit/{sha}` | Get details for a single commit by SHA. Returns short_id, title, message, author, committer, authored/committed dates, parent commits, web URL, and stats (additions/deletions/total). |
| 13 | `file_blob` | `gitlab://project/{project_id}/file/{ref}/{+path}` | Get the contents of a repository file at a specific ref (branch, tag, or SHA). Path may include slashes. Files over 1 MiB return metadata only with `truncated=true`. Binary files return metadata with empty content. |
| 14 | `wiki_page` | `gitlab://project/{project_id}/wiki/{slug}` | Get a wiki page by slug. Returns title, slug, format (markdown/rdoc/asciidoc/org), and raw content. Slugs are case-sensitive and use hyphens for spaces. |

### Issue & Merge Request Resources

| # | Name | URI Template | Description |
|---|------|--------------|-------------|
| 15 | `issue` | `gitlab://project/{project_id}/issue/{issue_iid}` | Get details of a specific issue by its IID (project-scoped ID). Returns title, state, labels, assignees, author, web URL, and creation date. |
| 16 | `merge_request` | `gitlab://project/{project_id}/mr/{mr_iid}` | Get details of a specific merge request by its IID (project-scoped ID). Returns title, state, source/target branches, author, merge status, and web URL. |
| 17 | `merge_request_notes` | `gitlab://project/{project_id}/mr/{mr_iid}/notes` | List notes (comments) on a merge request. Returns each note's id, author username, body, system flag, resolvable/resolved flags, and timestamps. |
| 18 | `merge_request_discussions` | `gitlab://project/{project_id}/mr/{mr_iid}/discussions` | List discussion threads on a merge request. Each discussion has an id, individual_note flag, and an array of notes (id, author, body, system, resolved/resolvable, created_at). |

### CI/CD Resources

| # | Name | URI Template | Description |
|---|------|--------------|-------------|
| 19 | `latest_pipeline` | `gitlab://project/{project_id}/pipelines/latest` | Get the most recent CI/CD pipeline for a GitLab project. Returns pipeline ID, status (running/pending/success/failed/canceled), ref, SHA, source, and web URL. |
| 20 | `pipeline` | `gitlab://project/{project_id}/pipeline/{pipeline_id}` | Get details of a specific CI/CD pipeline by its numeric ID. Returns pipeline status, ref, SHA, source, and web URL. |
| 21 | `pipeline_jobs` | `gitlab://project/{project_id}/pipeline/{pipeline_id}/jobs` | List all jobs for a specific CI/CD pipeline including each job's name, stage, status, duration, failure reason (if failed), and web URL. |

### Group Resources

| # | Name | URI Template | Description |
|---|------|--------------|-------------|
| 22 | `group` | `gitlab://group/{group_id}` | Get details for a specific GitLab group by numeric ID or URL-encoded path. Returns name, full path, description, visibility, and web URL. |
| 23 | `group_members` | `gitlab://group/{group_id}/members` | List all members of a GitLab group with their access levels (10=guest, 20=reporter, 30=developer, 40=maintainer, 50=owner). Includes inherited members. |
| 24 | `group_projects` | `gitlab://group/{group_id}/projects` | List all projects within a GitLab group. Returns each project's ID, name, namespace path, visibility, web URL, description, and default branch. |

## Workflow Guide Resources (5)

Static best-practice guides that provide AI assistants with GitLab workflow knowledge without requiring API calls.

| # | Name | URI | Description |
|---|------|-----|-------------|
| 25 | `guide_git_workflow` | `gitlab://guides/git-workflow` | Git branching strategy, commit hygiene, and merge best practices for GitLab projects. |
| 26 | `guide_merge_request_hygiene` | `gitlab://guides/merge-request-hygiene` | MR best practices: sizing, descriptions, review workflow, and merge strategies. |
| 27 | `guide_conventional_commits` | `gitlab://guides/conventional-commits` | Conventional Commits specification with GitLab-specific examples and automation tips. |
| 28 | `guide_code_review` | `gitlab://guides/code-review` | Structured code review checklist covering quality, security, testing, and architecture. |
| 29 | `guide_pipeline_troubleshooting` | `gitlab://guides/pipeline-troubleshooting` | CI/CD pipeline debugging guide: common failures, job logs, retry strategies, and caching issues. |

## URI Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `project_id` | string | Numeric project ID or URL-encoded path (e.g., `group%2Fproject`) |
| `group_id` | string | Numeric group ID or URL-encoded path |
| `pipeline_id` | integer | Numeric pipeline ID |
| `mr_iid` | integer | Merge request IID (project-scoped numeric ID, visible as `!N` in GitLab) |
| `issue_iid` | integer | Issue IID (project-scoped numeric ID, visible as `#N` in GitLab) |
| `sha` | string | Commit SHA (full or short) |
| `ref` | string | Branch name, tag name, or commit SHA |
| `path` | string | Repository file path (may contain slashes; uses RFC 6570 reserved expansion `{+path}`) |
| `slug` | string | Wiki page slug (case-sensitive; spaces are replaced with hyphens) |

## Autocomplete Support

All URI template parameters support intelligent autocomplete via the completions handler (`internal/completions/`). When a client sends a `completion/complete` request for a resource parameter, the server queries GitLab to suggest matching values (e.g., project IDs, group IDs).

## Source

Resources are implemented in [`internal/resources/resources.go`](../internal/resources/resources.go) (18 core resources), [`internal/resources/workspace_roots.go`](../internal/resources/workspace_roots.go) (workspace roots resource), and [`internal/resources/workflow_guides.go`](../internal/resources/workflow_guides.go) (5 workflow guide resources).
