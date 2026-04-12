# Security Findings — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Security Findings
> **Individual tools**: 1
> **Meta-tool**: `gitlab_security` (when `META_TOOLS=true`, default)
> **GitLab API**: [Pipeline Security Report Findings GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#pipelinesecurityreportfindings)
> **Audience**: 👤 End users, AI assistant users
> **Requires**: GitLab Ultimate or Premium

---

## Overview

The security findings domain provides access to per-pipeline security scan results via the GitLab GraphQL API. This replaces the deprecated REST `vulnerability_findings` endpoint with the GraphQL `Pipeline.securityReportFindings` query.

Security findings differ from vulnerabilities: findings are raw scan results from a specific pipeline run, while vulnerabilities are deduplicated, tracked entities across pipeline runs. Use security findings to inspect what a specific pipeline scan detected; use vulnerabilities for ongoing triage and remediation.

### Common Questions

> "What did the SAST scan find in pipeline 456?"
> "List all critical findings from the latest pipeline"
> "Show me secret detection findings in pipeline 123"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |

---

## Tools

### `gitlab_list_security_findings`

List security report findings for a specific pipeline run. Supports filtering by severity, confidence level, scanner, and report type. Returns paginated results with finding details including name, severity, confidence, scanner info, code location, identifiers (CVE/CWE), and linked vulnerability state.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter | Type | Required | Description |
| --------- | ---- | :------: | ----------- |
| `project_path` | string | Yes | Full path of the project (e.g. `my-group/my-project`) |
| `pipeline_iid` | string | Yes | Pipeline IID (internal ID within the project) |
| `severity` | string[] | No | Filter by severity: `CRITICAL`, `HIGH`, `MEDIUM`, `LOW`, `INFO`, `UNKNOWN` |
| `confidence` | string[] | No | Filter by confidence: `CONFIRMED`, `HIGH`, `MEDIUM`, `LOW`, `UNKNOWN`, `EXPERIMENTAL`, `IGNORE` |
| `scanner` | string[] | No | Filter by scanner external IDs |
| `report_type` | string[] | No | Filter by report type: `SAST`, `DAST`, `DEPENDENCY_SCANNING`, `CONTAINER_SCANNING`, `SECRET_DETECTION`, `COVERAGE_FUZZING`, `API_FUZZING`, `CLUSTER_IMAGE_SCANNING` |
| `first` | int | No | Number of items per page (default: 20) |
| `after` | string | No | Cursor for forward pagination |

### Output fields

Each finding includes:

| Field | Type | Description |
| ----- | ---- | ----------- |
| `uuid` | string | Unique identifier for the finding |
| `name` | string | Finding name |
| `title` | string | Human-readable title |
| `severity` | string | Severity level |
| `confidence` | string | Confidence level |
| `report_type` | string | Scanner report type |
| `scanner` | object | Scanner name, vendor, and external ID |
| `description` | string | Detailed description |
| `solution` | string | Recommended remediation |
| `identifiers` | array | CVE, CWE, OWASP identifiers with URLs |
| `location` | object | File path, line numbers (scanner-specific) |
| `state` | string | Finding state |
| `evidence` | object | Supporting evidence data |
| `vulnerability_id` | string | Linked vulnerability GID (if tracked) |
| `vulnerability_state` | string | Current state of the linked vulnerability |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_list_security_findings` | Query | Read |

---

## Notes

- Finding locations vary by scanner type: SAST and Secret Detection return file/line, DAST returns URL path, Container Scanning returns image name, Dependency Scanning returns file path
- Findings may or may not be linked to a tracked vulnerability — check `vulnerability_id` to determine if the finding has been promoted to a vulnerability
- This tool replaces the deprecated REST `GET /projects/:id/vulnerability_findings` endpoint

## Related

- [GitLab Security Report Findings GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#pipelinesecurityreportfindings)
- [GitLab Security Scanning](https://docs.gitlab.com/ee/user/application_security/)
- [Vulnerabilities](vulnerabilities.md) — tracked vulnerability lifecycle management
