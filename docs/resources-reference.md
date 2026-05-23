# MCP Resources Reference

This document lists the MCP resources exposed by gitlab-mcp-server. With the default `TOOL_SURFACE=dynamic` and `CAPABILITY_SURFACE=full`, the server exposes **46 MCP resources**: GitLab data resources, workflow guides, and a surface-aware tool manifest. Meta and individual modes expose the same public resource shape; the `gitlab://tools` manifest adapts its payload to the active tool surface selected at startup.

> **Diátaxis type**: Reference
> **Audience**: MCP client developers, AI assistant users
> **Prerequisites**: Understanding of MCP resources concept

GitLab data resources and tool manifest resources return `application/json`. Workflow guide resources return `text/markdown`.

MCP separates fixed resources from URI templates. In default dynamic full mode, `resources/list` exposes 9 fixed URIs: the 4 static resources below and the 5 workflow guides. `resources/templates/list` exposes the remaining 37 URI templates. Registries that only inspect `resources/list` may therefore report 9 resources statically even though the runtime MCP resource surface contains 46 entries in total.

---

## Static Resources (4 core)

Static resources have a fixed URI and require no parameters.

| #   | Name              | URI                        | Description                                                                                                                                                                              |
| --- | ----------------- | -------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | `current_user`    | `gitlab://user/current`    | Get the currently authenticated GitLab user profile. Returns username, display name, email, state (active/blocked), admin status, and web URL.                                           |
| 2   | `groups`          | `gitlab://groups`          | List all GitLab groups accessible to the authenticated user. Returns each group's ID, name, full path, description, visibility level, and web URL.                                       |
| 3   | `workspace_roots` | `gitlab://workspace/roots` | List workspace root directories provided by the MCP client. Use these paths to locate .git/config files and extract git remote URLs for project discovery via `gitlab_discover_project`. |
| 4   | `tool_manifest`   | `gitlab://tools`           | Surface-aware manifest of the tools and executable actions available in this server instance. Use `gitlab://tools/{id}` to fetch one entry's accepted call shape and input schema.       |

## Resource Templates (37 core)

Resource templates use URI variables (e.g., `{project_id}`) that the client fills in at request time.

### Project Resources

| #   | Name                 | URI Template                                                 | Description                                                                                                                                                                                                            |
| --- | -------------------- | ------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 5   | `project`            | `gitlab://project/{project_id}`                              | Get basic metadata for a GitLab project by numeric ID or URL-encoded path. Returns name, namespace path, visibility, web URL, description, and default branch.                                                         |
| 6   | `project_members`    | `gitlab://project/{project_id}/members`                      | List all members of a GitLab project with their access levels (10=guest, 20=reporter, 30=developer, 40=maintainer, 50=owner). Includes inherited members from parent groups.                                           |
| 7   | `project_labels`     | `gitlab://project/{project_id}/labels`                       | List all labels defined in a GitLab project. Returns each label's name, color, description, and counts of open issues and merge requests using the label.                                                              |
| 8   | `project_milestones` | `gitlab://project/{project_id}/milestones`                   | List all milestones in a GitLab project. Returns each milestone's title, description, state (active/closed), due date, and web URL.                                                                                    |
| 9   | `project_branches`   | `gitlab://project/{project_id}/branches`                     | List all branches in a GitLab project. Returns each branch's name, protection status, merge status, default flag, and web URL.                                                                                         |
| 10  | `project_issues`     | `gitlab://project/{project_id}/issues`                       | List open issues for a GitLab project. Returns each issue's IID, title, state, labels, assignees, author, web URL, and creation date.                                                                                  |
| 11  | `project_releases`   | `gitlab://project/{project_id}/releases`                     | List all releases for a GitLab project. Returns each release's tag name, name, description, author, and creation/release dates.                                                                                        |
| 12  | `project_tags`       | `gitlab://project/{project_id}/tags`                         | List all repository tags for a GitLab project. Returns each tag's name, message, target commit SHA, protection status, and creation date.                                                                              |
| 13  | `commit`             | `gitlab://project/{project_id}/commit/{sha}`                 | Get details for a single commit by SHA. Returns short_id, title, message, author, committer, authored/committed dates, parent commits, web URL, and stats (additions/deletions/total).                                 |
| 14  | `file_blob`          | `gitlab://project/{project_id}/file/{ref}/{+path}`           | Get the contents of a repository file at a specific ref (branch, tag, or SHA). Path may include slashes. Files over 1 MiB return metadata only with `truncated=true`. Binary files return metadata with empty content. |
| 15  | `wiki_page`          | `gitlab://project/{project_id}/wiki/{slug}`                  | Get a wiki page by slug. Returns title, slug, format (markdown/rdoc/asciidoc/org), and raw content. Slugs are case-sensitive and use hyphens for spaces.                                                               |
| 16  | `branch`             | `gitlab://project/{project_id}/branch/{branch}`              | Get a single branch by name. Returns name, protected/default flags, merge status, last commit, and web URL.                                                                                                            |
| 17  | `tag`                | `gitlab://project/{project_id}/tag/{tag_name}`               | Get a single repository tag by name. Returns name, message, target SHA, protection flag, and creation date.                                                                                                            |
| 18  | `release`            | `gitlab://project/{project_id}/release/{tag_name}`           | Get release details by tag name. Returns name, description, author, dates, and asset summary.                                                                                                                          |
| 19  | `label`              | `gitlab://project/{project_id}/label/{label_id}`             | Get a single project label. Returns name, color, description, and open issue/MR counts.                                                                                                                                |
| 20  | `milestone`          | `gitlab://project/{project_id}/milestone/{milestone_iid}`    | Get a single project milestone by IID. Returns title, description, state, due date, and web URL.                                                                                                                       |
| 21  | `board`              | `gitlab://project/{project_id}/board/{board_id}`             | Get a single issue board by ID. Returns name and lists.                                                                                                                                                                |
| 22  | `deployment`         | `gitlab://project/{project_id}/deployment/{deployment_id}`   | Get a deployment by ID. Returns ref, sha, status, and target environment.                                                                                                                                              |
| 23  | `environment`        | `gitlab://project/{project_id}/environment/{environment_id}` | Get an environment by ID. Returns name, slug, state, and tier.                                                                                                                                                         |
| 24  | `job`                | `gitlab://project/{project_id}/job/{job_id}`                 | Get a single CI/CD job by ID. Returns name, stage, status, ref, duration, and web URL.                                                                                                                                 |
| 25  | `feature_flag`       | `gitlab://project/{project_id}/feature_flag/{name}`          | Get a feature flag by name. Returns description, active flag, and version.                                                                                                                                             |
| 26  | `deploy_key`         | `gitlab://project/{project_id}/deploy_key/{deploy_key_id}`   | Get a project deploy key by ID. Returns title, key, and fingerprint.                                                                                                                                                   |
| 27  | `project_snippet`    | `gitlab://project/{project_id}/snippet/{snippet_id}`         | Get a project-scoped snippet. Returns title, file name, visibility, and web URL.                                                                                                                                       |

### Issue & Merge Request Resources

| #   | Name                        | URI Template                                                       | Description                                                                                                                                                                     |
| --- | --------------------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 28  | `issue`                     | `gitlab://project/{project_id}/issue/{issue_iid}`                  | Get details of a specific issue by its IID (project-scoped ID). Returns title, state, labels, assignees, author, web URL, and creation date.                                    |
| 29  | `merge_request`             | `gitlab://project/{project_id}/mr/{merge_request_iid}`             | Get details of a specific merge request by its IID (project-scoped ID). Returns title, state, source/target branches, author, merge status, and web URL.                        |
| 30  | `merge_request_notes`       | `gitlab://project/{project_id}/mr/{merge_request_iid}/notes`       | List notes (comments) on a merge request. Returns each note's id, author username, body, system flag, resolvable/resolved flags, and timestamps.                                |
| 31  | `merge_request_discussions` | `gitlab://project/{project_id}/mr/{merge_request_iid}/discussions` | List discussion threads on a merge request. Each discussion has an id, individual_note flag, and an array of notes (id, author, body, system, resolved/resolvable, created_at). |

### CI/CD Resources

| #   | Name              | URI Template                                                | Description                                                                                                                                                    |
| --- | ----------------- | ----------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 32  | `latest_pipeline` | `gitlab://project/{project_id}/pipelines/latest`            | Get the most recent CI/CD pipeline for a GitLab project. Returns pipeline ID, status (running/pending/success/failed/canceled), ref, SHA, source, and web URL. |
| 33  | `pipeline`        | `gitlab://project/{project_id}/pipeline/{pipeline_id}`      | Get details of a specific CI/CD pipeline by its numeric ID. Returns pipeline status, ref, SHA, source, and web URL.                                            |
| 34  | `pipeline_jobs`   | `gitlab://project/{project_id}/pipeline/{pipeline_id}/jobs` | List all jobs for a specific CI/CD pipeline including each job's name, stage, status, duration, failure reason (if failed), and web URL.                       |

### Group Resources

| #   | Name              | URI Template                                          | Description                                                                                                                                             |
| --- | ----------------- | ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 35  | `group`           | `gitlab://group/{group_id}`                           | Get details for a specific GitLab group by numeric ID or URL-encoded path. Returns name, full path, description, visibility, and web URL.               |
| 36  | `group_members`   | `gitlab://group/{group_id}/members`                   | List all members of a GitLab group with their access levels (10=guest, 20=reporter, 30=developer, 40=maintainer, 50=owner). Includes inherited members. |
| 37  | `group_projects`  | `gitlab://group/{group_id}/projects`                  | List all projects within a GitLab group. Returns each project's ID, name, namespace path, visibility, web URL, description, and default branch.         |
| 38  | `group_label`     | `gitlab://group/{group_id}/label/{label_id}`          | Get a single group label. Returns name, color, description, and open issue/MR counts.                                                                   |
| 39  | `group_milestone` | `gitlab://group/{group_id}/milestone/{milestone_iid}` | Get a single group milestone by IID. Returns title, description, state, due date, and web URL.                                                          |

### Personal Snippet

| #   | Name      | URI Template                    | Description                                                                              |
| --- | --------- | ------------------------------- | ---------------------------------------------------------------------------------------- |
| 40  | `snippet` | `gitlab://snippet/{snippet_id}` | Get a personal (user-scoped) snippet. Returns title, file name, visibility, and web URL. |

### Tool Manifest Detail

| #   | Name          | URI Template          | Description                                                                                                                                                                                                                     |
| --- | ------------- | --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 41  | `tool_detail` | `gitlab://tools/{id}` | Accepted call shape and input schema for one entry from `gitlab://tools`. Replace `{id}` with an entry ID such as `project.get` in dynamic mode, `gitlab_project.get` in meta mode, or `gitlab_get_project` in individual mode. |

The tool manifest is the public discovery layer for every tool surface. The normal discovery flow is:

1. Read `gitlab://tools` to see the active surface, the tools advertised through `tools/list`, and the executable entries accepted by that surface.
2. Read `gitlab://tools/{id}` for the chosen entry. The response includes `call`, `input_schema`, destructive metadata, read-only metadata, and required params when they are known.
3. Call the indicated MCP tool using the returned call shape.

For example, `gitlab://tools/project.get` describes a dynamic action that calls `gitlab_execute_action` with `action="project.get"` and params under `params`. `gitlab://tools/gitlab_project.get` describes a meta-tool action that calls `gitlab_project` with `action="get"` and params under `params`. `gitlab://tools/gitlab_get_project` describes an individual tool call where arguments are passed directly to the tool.

## Workflow Guide Resources (5)

Static best-practice guides that provide AI assistants with GitLab workflow knowledge without requiring API calls.

| #   | Name                             | URI                                        | Description                                                                                      |
| --- | -------------------------------- | ------------------------------------------ | ------------------------------------------------------------------------------------------------ |
| 42  | `guide_git_workflow`             | `gitlab://guides/git-workflow`             | Git branching strategy, commit hygiene, and merge best practices for GitLab projects.            |
| 43  | `guide_merge_request_hygiene`    | `gitlab://guides/merge-request-hygiene`    | MR best practices: sizing, descriptions, review workflow, and merge strategies.                  |
| 44  | `guide_conventional_commits`     | `gitlab://guides/conventional-commits`     | Conventional Commits specification with GitLab-specific examples and automation tips.            |
| 45  | `guide_code_review`              | `gitlab://guides/code-review`              | Structured code review checklist covering quality, security, testing, and architecture.          |
| 46  | `guide_pipeline_troubleshooting` | `gitlab://guides/pipeline-troubleshooting` | CI/CD pipeline debugging guide: common failures, job logs, retry strategies, and caching issues. |

## URI Parameters

| Parameter           | Type    | Description                                                                            |
| ------------------- | ------- | -------------------------------------------------------------------------------------- |
| `project_id`        | string  | Numeric project ID or URL-encoded path (e.g., `group%2Fproject`)                       |
| `group_id`          | string  | Numeric group ID or URL-encoded path                                                   |
| `pipeline_id`       | integer | Numeric pipeline ID                                                                    |
| `merge_request_iid` | integer | Merge request IID (project-scoped numeric ID, visible as `!N` in GitLab)               |
| `issue_iid`         | integer | Issue IID (project-scoped numeric ID, visible as `#N` in GitLab)                       |
| `sha`               | string  | Commit SHA (full or short)                                                             |
| `ref`               | string  | Branch name, tag name, or commit SHA                                                   |
| `path`              | string  | Repository file path (may contain slashes; uses RFC 6570 reserved expansion `{+path}`) |
| `slug`              | string  | Wiki page slug (case-sensitive; spaces are replaced with hyphens)                      |

## Autocomplete Support

All URI template parameters support intelligent autocomplete via the completions handler (`internal/completions/`). When a client sends a `completion/complete` request for a resource parameter, the server queries GitLab to suggest matching values (e.g., project IDs, group IDs).

## Source

Resources are implemented in [`internal/resources/resources.go`](../internal/resources/resources.go) (GitLab data resources and templates), [`internal/resources/tool_manifest.go`](../internal/resources/tool_manifest.go) (surface-aware tool manifest), [`internal/resources/workspace_roots.go`](../internal/resources/workspace_roots.go) (workspace roots resource), and [`internal/resources/workflow_guides.go`](../internal/resources/workflow_guides.go) (5 workflow guide resources).
