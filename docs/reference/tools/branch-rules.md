# Branch Rules — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Branch Rules
> **Individual tools**: 1
> **Meta-tool**: `gitlab_branch` (with `GITLAB_MCP_TOOL_SURFACE=meta`, routed as a branch action)
> **Dynamic IDs**: `branch.*` (default surface, via `gitlab_execute_action`)
> **GitLab API**: [Branch Rules GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#projectbranchrules)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The branch rules domain provides an aggregated read-only view of branch protections, approval rules, and external status checks via the GitLab GraphQL API. Branch rules consolidate information that would otherwise require multiple REST API calls across protected branches, approval rules, and external status checks into a single query.

On the default dynamic surface, these operations are the `branch.*` entries of the canonical action catalog: find them with `gitlab_find_action` and run them with `gitlab_execute_action` by `domain.action` ID. With `GITLAB_MCP_TOOL_SURFACE=individual`, each is the tool named in the tables below.

This tool complements the existing REST-based branch protection tools (`gitlab_branch_protect`, `gitlab_protected_branches_list`, etc.) by providing a unified read-only overview.

### Common Questions

> "What branch rules are configured for my project?"
> "Who can push to or merge into `main`?"
> "Which branches require code owner approval?"
> "How many approval rules are on the main branch?"
> "Are there any external status checks configured?"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description              |
| ---------- | :------: | :---------: | :--------: | ------------------------ |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation |

---

## Tools

### `gitlab_list_branch_rules`

List branch rules for a project. Returns a paginated list of all branch rules with their protection settings, approval rules, and external status checks.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter      | Type   | Required | Description                                           |
| -------------- | ------ | :------: | ----------------------------------------------------- |
| `project_path` | string |   Yes    | Full path of the project (e.g. `my-group/my-project`) |
| `first`        | int    |    No    | Number of items per page (default: 20)                |
| `after`        | string |    No    | Cursor for forward pagination                         |

`Project.branchRules` pages forward only: it rejects `last` and `before`, and reports no previous page, so the response carries `has_next_page` and `end_cursor` alone.

### Output fields

Each branch rule includes the fields below. The server sends one of two GraphQL documents: a Community Edition instance, or one without a Premium or Ultimate license, is asked only for the fields every edition defines, and every field marked Premium or Ultimate is then absent rather than `false`, since the instance was never asked. The tier is the licensed feature the field reports on; GitLab's GraphQL schema declares none.

| Field                     | Type   | Tier     | Description                                                                       |
| ------------------------- | ------ | -------- | --------------------------------------------------------------------------------- |
| `id`                      | string | Free     | Global ID of the rule; absent for the rules GitLab derives (such as all branches) |
| `name`                    | string | Free     | Branch name or pattern (e.g. `main`, `release/*`)                                 |
| `is_default`              | bool   | Free     | Whether this is the default branch                                                |
| `is_protected`            | bool   | Free     | Whether the branch is protected                                                   |
| `matching_branches_count` | int    | Free     | Number of branches matching this rule                                             |
| `created_at`              | string | Free     | Rule creation timestamp                                                           |
| `updated_at`              | string | Free     | Rule last update timestamp                                                        |
| `branch_protection`       | object | Free     | Protection settings (see below)                                                   |
| `approval_rules`          | array  | Premium  | Associated approval rules (see below)                                             |
| `external_status_checks`  | array  | Ultimate | External status checks (see below)                                                |

### Branch protection settings

| Field                                         | Type  | Tier     | Description                                                             |
| --------------------------------------------- | ----- | -------- | ----------------------------------------------------------------------- |
| `allow_force_push`                            | bool  | Free     | Whether force push is allowed                                           |
| `push_access_levels`                          | array | Free     | Who may push (see the grants below); a push grant may name a deploy key |
| `merge_access_levels`                         | array | Free     | Who may merge                                                           |
| `unprotect_access_levels`                     | array | Premium  | Who may unprotect the branch                                            |
| `code_owner_approval_required`                | bool  | Premium  | Whether code owner approval is required                                 |
| `modification_blocked_by_policy`              | bool  | Ultimate | Whether a security policy prevents changing the protection              |
| `protected_from_push_by_security_policy`      | bool  | Ultimate | Whether a security policy prevents push and force push                  |
| `warn_modification_blocked_by_policy`         | bool  | Ultimate | Whether a warn-mode security policy would prevent changing it           |
| `warn_protected_from_push_by_security_policy` | bool  | Ultimate | Whether a warn-mode security policy would prevent push                  |

### Grants

Each entry of `push_access_levels`, `merge_access_levels` and `unprotect_access_levels` is one grant:

| Field                      | Type   | Tier    | Description                                                                                              |
| -------------------------- | ------ | ------- | -------------------------------------------------------------------------------------------------------- |
| `access_level`             | int    | Free    | GitLab access level of the grant (`0` no one, `30` developer, `40` maintainer, `60` admin)               |
| `access_level_description` | string | Free    | GitLab's own reading of the grant: the role's name, or the user's or group's                             |
| `user`                     | object | Premium | The user granted (`id`, `username`, `name`, `public_email`, `avatar_url`, `web_url`, `web_path`)         |
| `group`                    | object | Premium | The group granted (`id`, `name`, `web_url`, `avatar_url`); its web URL spells its whole path             |
| `deploy_key`               | object | Free    | Push grants only: the deploy key granted (`id`, `title`, `expires_at`, and the `user` it is assigned to) |

### Approval rules

| Field                | Type   | Description                                                  |
| -------------------- | ------ | ------------------------------------------------------------ |
| `id`                 | string | Global ID of the approval rule                               |
| `name`               | string | Approval rule name                                           |
| `approvals_required` | int    | Number of required approvals                                 |
| `type`               | string | Rule type (e.g. `REGULAR`, `CODE_OWNER`)                     |
| `eligible_approvers` | array  | Users eligible to approve, in the same user shape as a grant |

### External status checks

| Field          | Type   | Description                               |
| -------------- | ------ | ----------------------------------------- |
| `id`           | string | Global ID of the status check             |
| `name`         | string | Check name                                |
| `external_url` | string | URL of the external service               |
| `hmac`         | bool   | Whether an HMAC secret signs the requests |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_list_branch_rules` | Query | Read |

---

## Notes

- Branch rules are read-only via GraphQL: to modify branch protections, use the REST-based `gitlab_branch_protect` and `gitlab_protected_branch_update` tools
- The `matching_branches_count` field shows how many actual branches match wildcard patterns (e.g. `release/*`)
- Approval rules, unprotect grants and user and group grants are only available on GitLab Premium/Ultimate; external status checks and the security-policy flags on Ultimate
- The grant lists, an approval rule's eligible approvers and the lists under a rule are read from their first page, which GitLab sizes at up to 100 entries
- GitLab refuses a whole GraphQL document that names a field it does not have, so the newest field a document selects is the oldest release it works on: on a Premium or Ultimate instance the listing needs GitLab 18.8 (the two warn-mode policy flags), and on a Community or Free one nothing newer than 16.11
- Left out on purpose: the fields GitLab still marks as experiments (a rule's and a protection's group-level flag, a rule's squash option, the custom role a grant names), an approval rule's coverage threshold, which GitLab added in 19.2, and a granted group's parent, which took the Premium and Ultimate document past the complexity GitLab accepts at a page of 100 (it scores 226 of 250 without it, measured on GitLab.com)

## Related

- [GitLab Branch Rules GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#projectbranchrules)
- [GitLab Branch Rules](https://docs.gitlab.com/ee/user/project/repository/branches/branch_rules.html)
- [Branches](branches.md) — REST-based branch management and protection tools
