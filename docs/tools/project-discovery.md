# Project Discovery — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Project Discovery
> **Individual tools**: 1
> **Meta-tool**: None (standalone utility tool, always registered individually)
> **Source**: [`internal/tools/projectdiscovery/`](../../internal/tools/projectdiscovery/)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The project discovery domain provides a utility tool that resolves git remote URLs to GitLab projects. This enables LLMs to automatically discover the `project_id` needed for all other GitLab operations by reading the workspace `.git/config` file.

### Common Questions

> "What GitLab project is this repository?"
> "Find the project ID from the git remote URL"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |

---

## Tools

### `gitlab_discover_project`

Resolve a git remote URL to a GitLab project. Extract the remote URL from the workspace `.git/config` file (look for `[remote "origin"] url = ...`) and pass it here to discover the `project_id` needed for all other GitLab operations.

Supports both HTTPS and SSH remote URL formats:

- **HTTPS**: `https://gitlab.example.com/group/subgroup/project.git`
- **SSH shorthand**: `git@gitlab.example.com:group/subgroup/project.git`
- **SSH protocol**: `ssh://git@gitlab.example.com/group/project.git`

| Annotation | **Read** |
| ---------- | -------- |

**Parameters:**

| Name | Type | Required | Description |
| --- | --- | :---: | --- |
| `remote_url` | string | Yes | Git remote URL from `.git/config` (e.g., `https://gitlab.example.com/group/project.git` or `git@gitlab.example.com:group/project.git`) |

**Returns**: Project ID, name, path, `path_with_namespace`, web URL, default branch, description, visibility, clone URLs, and the extracted path.

**Example workflow:**

```text
1. LLM reads .git/config → finds url = git@gitlab.example.com:team/my-app.git
2. LLM calls: gitlab_discover_project(remote_url="git@gitlab.example.com:team/my-app.git")
3. Returns: { id: 42, path_with_namespace: "team/my-app", default_branch: "main", ... }
4. LLM uses project_id=42 for subsequent operations (create MR, list issues, etc.)
```

---

## Related Resources

| Resource | URI | Description |
| --- | --- | --- |
| `workspace_roots` | `gitlab://workspace/roots` | List workspace root directories from the MCP client to find `.git/config` locations |
| `current_user` | `gitlab://user/current` | Confirm authentication before project discovery |

## Project Discovery Workflow

When an LLM needs to work with a GitLab project in the current workspace:

1. **Read workspace roots** — Fetch `gitlab://workspace/roots` to discover workspace directory paths
2. **Find git config** — Read the `.git/config` file from a workspace root to find the `[remote "origin"]` URL
3. **Resolve project** — Call `gitlab_discover_project` with the remote URL
4. **Use project_id** — All subsequent GitLab operations use the returned `project_id`

Alternative approaches (if `.git/config` is not accessible):

- **Search by name**: `gitlab_search_projects(query="my-project")` or `gitlab_project_list(owned=true, search="my-project")`
- **Direct path**: `gitlab_project_get(project_id="group/subgroup/project")` if the path is known
