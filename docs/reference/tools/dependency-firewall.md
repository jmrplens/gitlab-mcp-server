# Dependency Firewall — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Dependency Firewall
> **Individual tools**: 1
> **Meta-tool**: Route inside `gitlab_project` (enterprise-only, requires the Enterprise/Premium catalog)
> **Dynamic IDs**: `project.dependency_firewall_evaluate` (default surface, via `gitlab_execute_action`)
> **GitLab API**: [Dependency Firewall API](https://docs.gitlab.com/api/dependency_firewall/)
> **Audience**: 👤 End users, AI assistant users
> **Tier**: Premium, Ultimate
> **Status**: Experiment, behind the `dependency_firewall_phase1` feature flag

---

## Overview

The Dependency Firewall evaluates one package coordinate against a project's policies and answers whether the package is allowed, warned or blocked. It is a question, not a change: nothing is installed, downloaded or recorded, so the action is read-only and stays available under `--read-only` and to a `read_api` token.

On the default dynamic surface this is `project.dependency_firewall_evaluate`: find it with `gitlab_find_action` and run it with `gitlab_execute_action`. With `GITLAB_MCP_TOOL_SURFACE=individual` it is `gitlab_project_dependency_firewall_evaluate`. With `GITLAB_MCP_TOOL_SURFACE=meta` and the Enterprise/Premium catalog enabled it is the `dependency_firewall_evaluate` action on the `gitlab_project` meta-tool.

### Availability

The API is documented as `Tier: Premium, Ultimate` and `Offering: GitLab.com, GitLab Self-Managed, GitLab Dedicated`, so the action is gated at Premium and is offered on self-managed instances as well as GitLab.com. It was introduced in GitLab 19.4 behind the `dependency_firewall_phase1` feature flag, which is **disabled by default**, and it is an experiment: GitLab may change its shape between releases, and so may the client library this server calls.

While that flag is off, every project on the instance answers `404`, which is the same status a project the token cannot read returns. The tool therefore answers a `404` with guidance that names the flag first, so a caller is not left retrying with different project references against an instance where the endpoint does not exist at all.

### Common Questions

> "Is lodash 4.17.15 blocked by the dependency firewall on group/project?"
> "Check com.example:trivial-lib 1.2.3 against our maven policies"
> "Which policy blocks this package?"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description              |
| ---------- | :------: | :---------: | :--------: | ------------------------ |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation |

---

## Tools

### `gitlab_project_dependency_firewall_evaluate`

Evaluate one package coordinate against a project's Dependency Firewall policies. **Premium**. Returns the outcome and, when a policy matched, the policy that produced it.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter    | Type          | Required | Description                                                                                                                                                                                                                               |
| ------------ | ------------- | :------: | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `project_id` | string or int |   Yes    | Project ID or URL-encoded path                                                                                                                                                                                                            |
| `ecosystem`  | string        |   Yes    | One of `cargo`, `composer`, `conan`, `gem`, `golang`, `maven`, `npm`, `nuget`, `pub`, `pypi`, `swift`                                                                                                                                     |
| `name`       | string        |   Yes    | Package name, maximum 255 characters. For `maven` use the `groupId:artifactId` form, such as `com.example:trivial-lib`. For `pypi`, names are normalized per PEP 503 before evaluation, so `Flask_Login` and `flask-login` are equivalent |
| `version`    | string        |   Yes    | Package version, maximum 255 characters                                                                                                                                                                                                   |

### Output fields

| Field     | Type          | Description                                                                     |
| --------- | ------------- | ------------------------------------------------------------------------------- |
| `outcome` | string        | `allowed`, `warned` or `blocked`                                                |
| `reason`  | string / null | The policy that produced a `warned` or `blocked` outcome. `null` when `allowed` |

The three outcomes carry the meanings GitLab documents:

| Outcome   | Meaning                                                           |
| --------- | ----------------------------------------------------------------- |
| `allowed` | No policy rule matched the package                                |
| `warned`  | A policy rule matched, and the matching policy is in warn mode    |
| `blocked` | A policy rule matched, and the matching policy is in enforce mode |

An `allowed` outcome means no rule matched. It is not an assertion that GitLab holds vulnerability or license data for the package: one absent from the package metadata database is allowed too.

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_project_dependency_firewall_evaluate` | Query | Read |

---

## Notes

- The documented optional `operation` attribute (`download` or `upload`, defaulting to `download`) is not exposed, because the client library's options struct has no field for it. The gap is recorded in [upstream bugs and gaps](../../development/upstream-bugs.md).
- The API also documents a `GET /projects/:id/dependency_firewall/enablement` endpoint. The client library does not wrap it either, and it is recorded in the same place.
- Evaluate a specific version. The endpoint takes an exact version string and does not resolve ranges.
