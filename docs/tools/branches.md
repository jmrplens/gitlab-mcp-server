# Branches — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Branches
> **Individual tools**: 10
> **Meta-tool**: `gitlab_branch` (when `META_TOOLS=true`, default)
> **GitLab API**: [Branches API](https://docs.gitlab.com/ee/api/branches.html), [Protected Branches API](https://docs.gitlab.com/ee/api/protected_branches.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The branches domain covers Git branch management in GitLab projects: retrieving, creating, listing, and deleting branches, as well as protecting and unprotecting branches with configurable access levels. Protected branch tools allow inspecting and updating push/merge access restrictions.

When `META_TOOLS=true` (the default), all 10 individual tools below are consolidated into a single `gitlab_branch` meta-tool that dispatches by `action` parameter.

### Common Questions

> "List all branches in project 42"
> "Create a branch called feature-login from main"
> "Delete the old-feature branch"
> "Which branches are protected?"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Core Branch Operations

### `gitlab_branch_get`

Retrieve detailed information about a single branch in a GitLab project. Returns branch name, merged/protected/default status, web URL, and latest commit ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_branch_list`

List Git branches in a GitLab project. Supports optional name search filter. Returns paginated results including each branch's protection status and latest commit info.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_branch_create`

Create a new Git branch in a GitLab project from a ref (branch name, tag name, or commit SHA).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_branch_delete`

Delete a branch from a GitLab repository. Cannot delete the default branch or protected branches.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Cannot delete the default branch or protected branches.

### `gitlab_branch_delete_merged`

Delete all branches that have been merged into the default branch. The default branch and protected branches are never deleted.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Bulk operation — deletes all merged branches except the default and protected branches.

---

## Branch Protection

### `gitlab_branch_protect`

Protect a GitLab repository branch by setting push and merge access levels (0=no access, 30=developer, 40=maintainer, 60=admin). Protected branches cannot be force-pushed or deleted.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_branch_unprotect`

Remove all protection rules from a GitLab branch, allowing unrestricted push, merge, and force-push access.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Removes all access restrictions from the branch.

### `gitlab_protected_branches_list`

List all protected branches in a GitLab project with their configured push and merge access level restrictions. Returns paginated results.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_protected_branch_get`

Get details of a single protected branch by name, including push/merge access levels, force push, and code owner approval settings.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_protected_branch_update`

Update settings on an existing protected branch (allow_force_push, code_owner_approval_required). Use gitlab_branch_protect to initially protect a branch.

| Annotation | **Update** |
| ---------- | ---------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_branch_get` | Core Branch | Read |
| 2 | `gitlab_branch_list` | Core Branch | Read |
| 3 | `gitlab_branch_create` | Core Branch | Create |
| 4 | `gitlab_branch_delete` | Core Branch | Delete |
| 5 | `gitlab_branch_delete_merged` | Core Branch | Delete |
| 6 | `gitlab_branch_protect` | Branch Protection | Update |
| 7 | `gitlab_branch_unprotect` | Branch Protection | Delete |
| 8 | `gitlab_protected_branches_list` | Branch Protection | Read |
| 9 | `gitlab_protected_branch_get` | Branch Protection | Read |
| 10 | `gitlab_protected_branch_update` | Branch Protection | Update |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_branch_delete` — deletes a single branch
- `gitlab_branch_delete_merged` — bulk deletes all merged branches
- `gitlab_branch_unprotect` — removes protection rules from a branch

---

## Related

- [GitLab Branches API](https://docs.gitlab.com/ee/api/branches.html)
- [GitLab Protected Branches API](https://docs.gitlab.com/ee/api/protected_branches.html)
