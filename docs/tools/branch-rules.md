# Branch Rules — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Branch Rules
> **Individual tools**: 1
> **Meta-tool**: `gitlab_branch` (when `META_TOOLS=true`, routed as a branch action)
> **GitLab API**: [Branch Rules GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#projectbranchrules)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The branch rules domain provides an aggregated read-only view of branch protections, approval rules, and external status checks via the GitLab GraphQL API. Branch rules consolidate information that would otherwise require multiple REST API calls across protected branches, approval rules, and external status checks into a single query.

This tool complements the existing REST-based branch protection tools (`gitlab_branch_protect`, `gitlab_protected_branches_list`, etc.) by providing a unified read-only overview.

### Common Questions

> "What branch rules are configured for my project?"
> "Which branches require code owner approval?"
> "How many approval rules are on the main branch?"
> "Are there any external status checks configured?"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |

---

## Tools

### `gitlab_list_branch_rules`

List branch rules for a project. Returns a paginated list of all branch rules with their protection settings, approval rules, and external status checks.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `project_path` | string | Yes | Full path of the project (e.g. `my-group/my-project`) |
| `first` | int | No | Number of items per page (default: 20) |
| `after` | string | No | Cursor for forward pagination |

### Output fields

Each branch rule includes:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `name` | string | Branch name or pattern (e.g. `main`, `release/*`) |
| `is_default` | bool | Whether this is the default branch |
| `is_protected` | bool | Whether the branch is protected |
| `matching_branches_count` | int | Number of branches matching this rule |
| `created_at` | string | Rule creation timestamp |
| `updated_at` | string | Rule last update timestamp |
| `branch_protection` | object | Protection settings (see below) |
| `approval_rules` | array | Associated approval rules (see below) |
| `external_status_checks` | array | External status checks (see below) |

### Branch protection settings

| Field | Type | Description |
| ----- | ---- | ----------- |
| `allow_force_push` | bool | Whether force push is allowed |
| `code_owner_approval_required` | bool | Whether code owner approval is required |

### Approval rules

| Field | Type | Description |
| ----- | ---- | ----------- |
| `name` | string | Approval rule name |
| `approvals_required` | int | Number of required approvals |
| `type` | string | Rule type (e.g. `REGULAR`, `CODE_OWNER`) |

### External status checks

| Field | Type | Description |
| ----- | ---- | ----------- |
| `name` | string | Check name |
| `external_url` | string | URL of the external service |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_list_branch_rules` | Query | Read |

---

## Notes

- Branch rules are read-only via GraphQL — to modify branch protections, use the REST-based `gitlab_branch_protect` and `gitlab_protected_branch_update` tools
- The `matching_branches_count` field shows how many actual branches match wildcard patterns (e.g. `release/*`)
- Approval rules and external status checks are only available on GitLab Premium/Ultimate

## Related

- [GitLab Branch Rules GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#projectbranchrules)
- [GitLab Branch Rules](https://docs.gitlab.com/ee/user/project/repository/branches/branch_rules.html)
- [Branches](branches.md) — REST-based branch management and protection tools
