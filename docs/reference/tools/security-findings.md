# Security Findings — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Security Findings
> **Individual tools**: 1
> **Meta-tool**: `gitlab_security_finding` (`GITLAB_MCP_TOOL_SURFACE=meta` catalog; the sibling groups are `gitlab_security_attribute`, `gitlab_security_category` and `gitlab_security_scan_profile`)
> **Dynamic IDs**: `security_finding.*` (default surface, via `gitlab_execute_action`)
> **GitLab API**: [Pipeline Security Report Findings GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#pipelinesecurityreportfindings)
> **Audience**: 👤 End users, AI assistant users
> **Requires**: GitLab Ultimate or Premium

---

## Overview

The security findings domain provides access to per-pipeline security scan results via the GitLab GraphQL API. This replaces the deprecated REST `vulnerability_findings` endpoint with the GraphQL `Pipeline.securityReportFindings` query.

On the default dynamic surface, these operations are the `security_finding.*` entries of the canonical action catalog: find them with `gitlab_find_action` and run them with `gitlab_execute_action` by `domain.action` ID. With `GITLAB_MCP_TOOL_SURFACE=individual`, each is the tool named in the tables below.

Security findings differ from vulnerabilities: findings are raw scan results from a specific pipeline run, while vulnerabilities are deduplicated, tracked entities across pipeline runs. Use security findings to inspect what a specific pipeline scan detected; use vulnerabilities for ongoing triage and remediation.

### Common Questions

> "What did the SAST scan find in pipeline 456?"
> "List all critical findings from the latest pipeline"
> "Show me secret detection findings in pipeline 123"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description              |
| ---------- | :------: | :---------: | :--------: | ------------------------ |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation |

---

## Tools

### `gitlab_list_security_findings`

List security report findings for a specific pipeline run. Supports filtering by severity, scanner, report type and state, and sorting by severity. Returns paginated results with finding details including title, severity, scanner info, code location, identifiers (CVE/CWE), and linked vulnerability state.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter      | Type     | Required | Description                                                                                                                                                                                               |
| -------------- | -------- | :------: | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `project_path` | string   |   Yes    | Full path of the project (e.g. `my-group/my-project`)                                                                                                                                                     |
| `pipeline_iid` | string   |   Yes    | Pipeline IID (internal ID within the project)                                                                                                                                                             |
| `severity`     | string[] |    No    | Filter by severity: `CRITICAL`, `HIGH`, `MEDIUM`, `LOW`, `INFO`, `UNKNOWN`                                                                                                                                |
| `scanner`      | string[] |    No    | Filter by scanner external IDs                                                                                                                                                                            |
| `report_type`  | string[] |    No    | Filter by report type: `SAST`, `DAST`, `DEPENDENCY_SCANNING`, `CONTAINER_SCANNING`, `SECRET_DETECTION`, `COVERAGE_FUZZING`, `API_FUZZING`, `CLUSTER_IMAGE_SCANNING`, `SARIF` (`SARIF` needs GitLab 18.11) |
| `state`        | string[] |    No    | Filter by state: `DETECTED`, `CONFIRMED`, `DISMISSED`, `RESOLVED`                                                                                                                                         |
| `sort`         | string   |    No    | Sort order: `severity_desc` (default) or `severity_asc`                                                                                                                                                   |
| `first`        | int      |    No    | Number of items per page (default: 20)                                                                                                                                                                    |
| `after`        | string   |    No    | Cursor for forward pagination                                                                                                                                                                             |
| `last`         | int      |    No    | Number of items per page when paging backward. Cannot be combined with `first`                                                                                                                            |
| `before`       | string   |    No    | Cursor for backward pagination, from a previous response's `start_cursor`                                                                                                                                 |

### Output fields

Each finding includes:

| Field                 | Type   | Description                                                                                                                                                                    |
| --------------------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `uuid`                | string | Unique identifier for the finding, the same `uuid` its vulnerability carries                                                                                                   |
| `title`               | string | Human-readable title                                                                                                                                                           |
| `severity`            | string | Severity level                                                                                                                                                                 |
| `original_severity`   | string | Severity before any override                                                                                                                                                   |
| `report_type`         | string | Scanner report type                                                                                                                                                            |
| `scanner`             | object | Scanner name, vendor, and external ID                                                                                                                                          |
| `description`         | string | Detailed description                                                                                                                                                           |
| `solution`            | string | Recommended remediation                                                                                                                                                        |
| `identifiers`         | array  | CVE, CWE, OWASP identifiers with URLs                                                                                                                                          |
| `location`            | object | Where it was found, in the terms of its scan type, the same shape a vulnerability's location has                                                                               |
| `state`               | string | Finding state                                                                                                                                                                  |
| `state_comment`       | string | The comment written with the latest dismissal                                                                                                                                  |
| `dismissed_at`        | string | When the vulnerability the finding became was dismissed                                                                                                                        |
| `dismissed_by`        | object | Who dismissed it: `username`, `name`, `web_url`                                                                                                                                |
| `dismissal_reason`    | string | Why it was dismissed                                                                                                                                                           |
| `false_positive`      | bool   | GitLab's false-positive verdict, only where the project is licensed for the detection                                                                                          |
| `evidence`            | object | Supporting evidence: `summary`, `source`, `source_id`, `source_url`, and the HTTP `request` and `response` a DAST or API fuzzing scan recorded, with any `supporting_messages` |
| `remediations`        | array  | Fixes the scanner proposed: `summary`, and as `diff` the patch in the form `git apply` takes, decoded from the base64 the report format carries it in                          |
| `links`               | array  | References the security report attached: `name` and `url`                                                                                                                      |
| `assets`              | array  | Artifacts the scan attached: `name`, `type`, `url`                                                                                                                             |
| `token_status`        | object | For a leaked secret, whether it still works: `status`, `last_verified_at`, `created_at` and `updated_at`                                                                       |
| `vulnerability_id`    | string | Linked vulnerability GID (if tracked)                                                                                                                                          |
| `vulnerability_state` | string | Current state of the linked vulnerability                                                                                                                                      |

The issue links and the merge request of a finding are those of its vulnerability, since GitLab resolves both through it: read them with `vulnerability.get` on the `vulnerability_id`.

`severity` and `report_type` are plain strings in GitLab's schema, which GitLab looks up in enums of its own, so a value outside the lists above is refused before anything is sent, naming the values it takes. Sent on, an unknown severity is answered with a 500, and an unknown report type is dropped without a word and filters out every finding, which reads as a pipeline that found nothing. The report types are the vulnerability report types less the two a pipeline scan never records, `CONTAINER_SCANNING_FOR_REGISTRY` and `GENERIC`, which the vulnerability list does take. Both lists are held by the package's tests to the pinned GitLab schema. `SARIF` is the one value younger than the action: GitLab's pipeline scans gained it in 18.11, so on 18.5 to 18.10 it passes the check here and GitLab drops it the way it drops any value it has no scan type for, answering with no findings. Both GitLab behaviours are recorded in the [upstream register](../../development/upstream-bugs.md), with this check as the workaround each needs until GitLab validates the argument itself.

The action needs GitLab 18.5 or later, the release that added the newest fields it reads (`original_severity` and the token status's `last_verified_at`). A finding's `unverified` flag, added in 18.11, is not read, since GitLab refuses a whole document that names a field it does not have.

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_list_security_findings` | Query | Read |

---

## Notes

- Finding locations vary by scan type, and every member of GitLab's location union is read: `file` is the file, the DAST request path or the container image, and the fields beside it are the ones the [vulnerabilities reference](vulnerabilities.md#notes) lists for each scan type
- Findings may or may not be linked to a tracked vulnerability — check `vulnerability_id` to determine if the finding has been promoted to a vulnerability
- This tool replaces the deprecated REST `GET /projects/:id/vulnerability_findings` endpoint

## Related

- [GitLab Security Report Findings GraphQL API](https://docs.gitlab.com/ee/api/graphql/reference/#pipelinesecurityreportfindings)
- [GitLab Security Scanning](https://docs.gitlab.com/ee/user/application_security/)
- [Vulnerabilities](vulnerabilities.md) — tracked vulnerability lifecycle management
