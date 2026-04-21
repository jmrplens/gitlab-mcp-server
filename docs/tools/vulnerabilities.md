# Vulnerabilities — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Vulnerabilities
> **Individual tools**: 8
> **Meta-tool**: `gitlab_vulnerability` (when `META_TOOLS=true`, default)
> **GitLab API**: [Vulnerabilities GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#queryvulnerabilities)
> **Audience**: 👤 End users, AI assistant users
> **Requires**: GitLab Ultimate or Premium

---

## Overview

The vulnerabilities domain provides full lifecycle management for security vulnerabilities detected by GitLab security scanners. All operations use the GitLab GraphQL API (no REST equivalent for these queries/mutations). This domain covers listing, inspecting, and triaging vulnerabilities, as well as retrieving severity counts and per-pipeline security report summaries.

When `META_TOOLS=true` (the default), all 8 individual tools below are consolidated into a single `gitlab_vulnerability` meta-tool that dispatches by `action` parameter.

### Common Questions

> "List all critical vulnerabilities in my project"
> "Show me the details of vulnerability gid://gitlab/Vulnerability/42"
> "Dismiss vulnerability 42 as a false positive"
> "How many vulnerabilities does my project have by severity?"
> "What security scanners ran in pipeline 123?"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Update** | — | No | Yes | Modifies an existing resource |

---

## Vulnerability Operations

### `gitlab_list_vulnerabilities`

List project vulnerabilities with extensive filtering support. Returns a paginated list with severity, state, scanner, report type, primary identifier, and detected date.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `project_path` | string | Yes | Full path of the project (e.g. `my-group/my-project`) |
| `severity` | string[] | No | Filter by severity: `CRITICAL`, `HIGH`, `MEDIUM`, `LOW`, `INFO`, `UNKNOWN` |
| `state` | string[] | No | Filter by state: `DETECTED`, `CONFIRMED`, `DISMISSED`, `RESOLVED` |
| `scanner` | string[] | No | Filter by scanner external IDs |
| `report_type` | string[] | No | Filter by report type: `SAST`, `DAST`, `DEPENDENCY_SCANNING`, `CONTAINER_SCANNING`, `SECRET_DETECTION`, `COVERAGE_FUZZING`, `API_FUZZING`, `CLUSTER_IMAGE_SCANNING` |
| `has_issues` | bool | No | Filter by whether a linked issue exists |
| `has_resolution` | bool | No | Filter by whether a resolution exists |
| `sort` | string | No | Sort order: `severity_desc`, `severity_asc`, `detected_desc`, `detected_asc` |
| `first` | int | No | Number of items per page (default: 20) |
| `after` | string | No | Cursor for forward pagination |

### `gitlab_get_vulnerability`

Get full details of a single vulnerability by its GID. Returns complete vulnerability information including all identifiers, scanner details, code location, solution, linked issues, and merge requests.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `id` | string | Yes | Vulnerability GID (e.g. `gid://gitlab/Vulnerability/42`) |

---

## Vulnerability State Mutations

State transitions follow the GitLab vulnerability lifecycle:

```text
DETECTED → CONFIRMED → RESOLVED
    ↓                     ↓
 DISMISSED ←──────── (revert) ← any state
```

### `gitlab_dismiss_vulnerability`

Dismiss a vulnerability with an optional comment and reason. Valid dismissal reasons: `ACCEPTABLE_RISK`, `FALSE_POSITIVE`, `MITIGATING_CONTROL`, `USED_IN_TESTS`, `NOT_APPLICABLE`.

| Annotation | **Update** |
| ---------- | ---------- |

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `id` | string | Yes | Vulnerability GID (e.g. `gid://gitlab/Vulnerability/42`) |
| `comment` | string | No | Reason for dismissal |
| `dismissal_reason` | string | No | Structured reason: `ACCEPTABLE_RISK`, `FALSE_POSITIVE`, `MITIGATING_CONTROL`, `USED_IN_TESTS`, `NOT_APPLICABLE` |

### `gitlab_confirm_vulnerability`

Confirm a detected vulnerability. Changes state from `DETECTED` to `CONFIRMED`.

| Annotation | **Update** |
| ---------- | ---------- |

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `id` | string | Yes | Vulnerability GID (e.g. `gid://gitlab/Vulnerability/42`) |

### `gitlab_resolve_vulnerability`

Resolve a vulnerability. Changes state to `RESOLVED`.

| Annotation | **Update** |
| ---------- | ---------- |

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `id` | string | Yes | Vulnerability GID (e.g. `gid://gitlab/Vulnerability/42`) |

### `gitlab_revert_vulnerability`

Revert a vulnerability back to `DETECTED` state from any other state.

| Annotation | **Update** |
| ---------- | ---------- |

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `id` | string | Yes | Vulnerability GID (e.g. `gid://gitlab/Vulnerability/42`) |

---

## Summary & Reporting

### `gitlab_vulnerability_severity_count`

Get vulnerability severity counts for a project. Returns counts per severity level (critical, high, medium, low, info, unknown) and the total.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `project_path` | string | Yes | Full path of the project (e.g. `my-group/my-project`) |

### `gitlab_pipeline_security_summary`

Get the security report summary for a specific pipeline. Returns scanner-level breakdown with vulnerability counts and scanned resource counts for each scanner type: SAST, DAST, Dependency Scanning, Container Scanning, Secret Detection, Coverage Fuzzing, API Fuzzing, and Cluster Image Scanning.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `project_path` | string | Yes | Full path of the project (e.g. `my-group/my-project`) |
| `pipeline_iid` | string | Yes | Pipeline IID (internal ID within the project) |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_list_vulnerabilities` | Query | Read |
| 2 | `gitlab_get_vulnerability` | Query | Read |
| 3 | `gitlab_dismiss_vulnerability` | Mutation | Update |
| 4 | `gitlab_confirm_vulnerability` | Mutation | Update |
| 5 | `gitlab_resolve_vulnerability` | Mutation | Update |
| 6 | `gitlab_revert_vulnerability` | Mutation | Update |
| 7 | `gitlab_vulnerability_severity_count` | Summary | Read |
| 8 | `gitlab_pipeline_security_summary` | Summary | Read |

---

## Notes

- All identifiers use GitLab Global IDs (GIDs) in the format `gid://gitlab/Vulnerability/{numeric_id}`
- Vulnerability location depends on the scanner type — SAST/Secret Detection return file/line, DAST returns URL path, Container Scanning returns image name
- Severity badges are rendered with emoji indicators: 🔴 CRITICAL, 🟠 HIGH, 🟡 MEDIUM, 🔵 LOW, ℹ️ INFO

## Related

- [GitLab Vulnerability GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#queryvulnerabilities)
- [GitLab Vulnerability Management](https://docs.gitlab.com/ee/user/application_security/vulnerability_report/)
- [Security Findings](security-findings.md) — per-pipeline scan findings
