# Environments & Deployments — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Environments, Deployments, Protected Environments, Freeze Periods
> **Individual tools**: 23
> **Meta-tools**: `gitlab_environment` (when `META_TOOLS=true`, default). Protected environment actions use `protected_*` prefix, freeze period actions use `freeze_*` prefix, deployment actions use `deployment_*` prefix.
> **GitLab API**: [Environments API](https://docs.gitlab.com/ee/api/environments.html) · [Deployments API](https://docs.gitlab.com/ee/api/deployments.html) · [Protected Environments API](https://docs.gitlab.com/ee/api/protected_environments.html) · [Freeze Periods API](https://docs.gitlab.com/ee/api/freeze_periods.html)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The environments and deployments domain covers the full lifecycle of GitLab environments, deployments, protected environment configurations, deploy freeze periods, and deployment merge request associations.

When `META_TOOLS=true` (the default), the 23 individual tools below are consolidated into one meta-tool (`gitlab_environment`) that dispatches by `action` parameter.

### Common Questions

> "List environments for project 42"
> "What's deployed to production?"
> "Show the freeze periods for my project"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |
| **Update** | — | No | Yes | Modifies an existing resource |
| **Delete** | — | Yes | Yes | Destroys a resource; protected by confirmation |

Tools marked **Delete** require user confirmation before execution.

---

## Environments

### `gitlab_environment_list`

List environments for a GitLab project. Supports filtering by name, search term, and state. Returns paginated results with environment details including tier, state, and external URL.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_environment_get`

Get details of a specific environment in a GitLab project by its ID. Returns environment name, state, tier, external URL, and timestamps.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_environment_create`

Create a new environment in a GitLab project. Specify name (required), description, external URL, and tier (production, staging, testing, development, other).

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_environment_update`

Update an existing environment in a GitLab project. Can modify name, description, external URL, and tier.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_environment_stop`

Stop a running environment in a GitLab project. Triggers any on_stop actions defined in CI/CD. Use force=true to stop even if the environment has active deployments.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_environment_delete`

Permanently delete an environment from a GitLab project. The environment must be stopped before it can be deleted.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt. Environment must be stopped first.

---

## Deployments

### `gitlab_deployment_list`

List deployments for a GitLab project. Supports filtering by environment name and status (created, running, success, failed, canceled). Returns paginated results with deployment details.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_deployment_get`

Get details of a specific deployment in a GitLab project by its ID. Returns deployment ref, SHA, status, user, environment, and timestamps.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_deployment_create`

Create a new deployment in a GitLab project. Requires environment name, git ref, and SHA. Optionally specify tag flag and initial status.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_deployment_update`

Update the status of an existing deployment in a GitLab project. Use to transition a deployment between states: created, running, success, failed, canceled.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_deployment_approve_or_reject`

Approve or reject a blocked deployment in a GitLab project. Set status to 'approved' or 'rejected'. Optionally include a comment.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_deployment_delete`

Permanently delete a deployment from a GitLab project. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Deployment Merge Requests

### `gitlab_list_deployment_merge_requests`

List merge requests associated with a specific deployment in a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

---

## Protected Environments

### `gitlab_protected_environment_list`

List protected environments in a GitLab project with their deploy access levels and approval rules. Returns paginated results.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_protected_environment_get`

Get a single protected environment by name, including deploy access levels and approval rules.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_protected_environment_protect`

Protect an environment in a GitLab project. Configure deploy access levels, required approvals, and approval rules.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_protected_environment_update`

Update a protected environment's deploy access levels, approval rules, or required approval count.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_protected_environment_unprotect`

Remove protection from an environment. This action cannot be undone.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Freeze Periods

### `gitlab_list_freeze_periods`

List deploy freeze periods for a GitLab project.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_get_freeze_period`

Get a single deploy freeze period by ID.

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_create_freeze_period`

Create a deploy freeze period with cron-based start and end times.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_update_freeze_period`

Update a deploy freeze period's cron schedule or timezone.

| Annotation | **Update** |
| ---------- | ---------- |

### `gitlab_delete_freeze_period`

Delete a deploy freeze period from a project.

| Annotation | **Delete** |
| ---------- | ---------- |

> **Destructive**: Protected by confirmation prompt.

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_environment_list` | Environments | Read |
| 2 | `gitlab_environment_get` | Environments | Read |
| 3 | `gitlab_environment_create` | Environments | Create |
| 4 | `gitlab_environment_update` | Environments | Update |
| 5 | `gitlab_environment_stop` | Environments | Update |
| 6 | `gitlab_environment_delete` | Environments | Delete |
| 7 | `gitlab_deployment_list` | Deployments | Read |
| 8 | `gitlab_deployment_get` | Deployments | Read |
| 9 | `gitlab_deployment_create` | Deployments | Create |
| 10 | `gitlab_deployment_update` | Deployments | Update |
| 11 | `gitlab_deployment_approve_or_reject` | Deployments | Update |
| 12 | `gitlab_deployment_delete` | Deployments | Delete |
| 13 | `gitlab_list_deployment_merge_requests` | Deployment MRs | Read |
| 14 | `gitlab_protected_environment_list` | Protected Environments | Read |
| 15 | `gitlab_protected_environment_get` | Protected Environments | Read |
| 16 | `gitlab_protected_environment_protect` | Protected Environments | Create |
| 17 | `gitlab_protected_environment_update` | Protected Environments | Update |
| 18 | `gitlab_protected_environment_unprotect` | Protected Environments | Delete |
| 19 | `gitlab_list_freeze_periods` | Freeze Periods | Read |
| 20 | `gitlab_get_freeze_period` | Freeze Periods | Read |
| 21 | `gitlab_create_freeze_period` | Freeze Periods | Create |
| 22 | `gitlab_update_freeze_period` | Freeze Periods | Update |
| 23 | `gitlab_delete_freeze_period` | Freeze Periods | Delete |

### Destructive Tools (Require Confirmation)

The following tools are annotated with `DestructiveHint: true` and require user confirmation before execution:

- `gitlab_environment_delete` — deletes an environment (must be stopped first)
- `gitlab_deployment_delete` — permanently deletes a deployment
- `gitlab_protected_environment_unprotect` — removes environment protection
- `gitlab_delete_freeze_period` — deletes a deploy freeze period

---

## Related

- [GitLab Environments API](https://docs.gitlab.com/ee/api/environments.html)
- [GitLab Deployments API](https://docs.gitlab.com/ee/api/deployments.html)
- [GitLab Protected Environments API](https://docs.gitlab.com/ee/api/protected_environments.html)
- [GitLab Freeze Periods API](https://docs.gitlab.com/ee/api/freeze_periods.html)
