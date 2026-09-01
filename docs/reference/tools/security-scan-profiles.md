# Security Scan Profiles — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: Security Scan Profiles
> **Individual tools**: 3
> **Meta-tool**: `gitlab_security_scan_profile` (`TOOL_SURFACE=meta` catalog)
> **Dynamic IDs**: `security_scan_profile.*` (default surface, via `gitlab_execute_action`)
> **GitLab API**: [Attach](https://docs.gitlab.com/api/graphql/reference/#mutationsecurityscanprofileattach) · [Detach](https://docs.gitlab.com/api/graphql/reference/#mutationsecurityscanprofiledetach) · [Project scan profile statuses](https://docs.gitlab.com/api/graphql/reference/#project-scanprofilestatuses)
> **Audience**: End users, AI assistant users
> **Requires**: GitLab Ultimate

---

## Overview

Security scan profiles bundle a security scanning configuration — for example dependency scanning — that can be attached to, or detached from, projects and groups via the GitLab GraphQL API. Attaching a profile enables its scanning configuration on the targets; detaching removes it. The per-project status query reports which scan profiles are active, pending, failing, or not configured.

On the default dynamic surface, these operations are the `security_scan_profile.*` entries of the canonical action catalog: find them with `gitlab_find_action` and run them with `gitlab_execute_action` by `domain.action` ID. With `TOOL_SURFACE=individual`, each is the tool named in the tables below.

This domain is distinct from vulnerabilities and security findings: scan profiles configure *how* scanning runs, while findings and vulnerabilities represent scanner output and triage state.

### Prerequisites

- **GitLab Ultimate** (18.7+ for the status query; the built-in `dependency_scanning` profile requires 19.0+).
- The target project or group must belong to a **group namespace**, not a personal namespace, and all targets in one call must share the same root namespace.
- **Attach** takes a built-in **scan type** (`dependency_scanning`, `sast`, `secret_detection`, or `container_scanning`) and creates that namespace's default profile on the fly — no profile has to exist beforehand.
- **Detach** takes the **persisted profile's numeric ID**, which you obtain from `gitlab_list_project_scan_profile_statuses` after attaching (a scan-type name is not accepted by detach).

### Common Questions

> "Attach the dependency-scanning scan profile to project 42"
> "Detach the scan profile from these groups"
> "Which scan profiles are active on group/project?"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description                                        |
| ---------- | :------: | :---------: | :--------: | -------------------------------------------------- |
| **Create** |    —     |     No      |     No     | Attaches a scan profile to targets                 |
| **Delete** |    —     |     Yes     |    Yes     | Detaches a scan profile; protected by confirmation |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only status query                        |

Tools marked **Delete** require confirmation before execution.

---

## Tools

### `gitlab_attach_security_scan_profile`

Attach a security scan profile to one or more projects and/or groups.

| Annotation | **Create** |
| ---------- | ---------- |

| Parameter                  | Type   | Required | Description                                                                                                                                                                                                     |
| -------------------------- | ------ | :------: | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `security_scan_profile_id` | string |   Yes    | A built-in scan type (`dependency_scanning`, `sast`, `secret_detection`, or `container_scanning`) — the default profile is created on the fly. A numeric profile ID or full `gid://` global ID is also accepted |
| `project_ids`              | int[]  |    No    | Numeric IDs of the projects to attach the profile to                                                                                                                                                            |
| `group_ids`                | int[]  |    No    | Numeric IDs of the groups to attach the profile to                                                                                                                                                              |

At least one of `project_ids` or `group_ids` must be provided. Targets must be in a group namespace and share one root namespace. Requires Maintainer or Owner on the targets.

### `gitlab_detach_security_scan_profile`

Detach a security scan profile from one or more projects and/or groups.

| Annotation | **Delete** |
| ---------- | ---------- |

| Parameter                  | Type   | Required | Description                                                                                                                                                    |
| -------------------------- | ------ | :------: | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `security_scan_profile_id` | string |   Yes    | The persisted profile's numeric ID (from `gitlab_list_project_scan_profile_statuses`) or a full `gid://` global ID; a scan-type name is not accepted by detach |
| `project_ids`              | int[]  |    No    | Numeric IDs of the projects to detach the profile from                                                                                                         |
| `group_ids`                | int[]  |    No    | Numeric IDs of the groups to detach the profile from                                                                                                           |

At least one of `project_ids` or `group_ids` must be provided. This is reversible with attach.

> **Destructive**: Protected by confirmation prompt because it disables the scanning configuration on the targets.

### `gitlab_list_project_scan_profile_statuses`

List the security scan profile statuses for a project.

| Annotation | **Read** |
| ---------- | -------- |

| Parameter           | Type   | Required | Description                                                                                        |
| ------------------- | ------ | :------: | -------------------------------------------------------------------------------------------------- |
| `project_full_path` | string |   Yes    | Full project path (`namespace/project`); numeric project IDs are not accepted by the GraphQL query |

Each status is one of `NOT_CONFIGURED`, `PENDING`, `ACTIVE`, `WARNING`, `FAILED`, or `STALE`.

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_attach_security_scan_profile` | Mutation | Create |
| 2 | `gitlab_detach_security_scan_profile` | Mutation | Delete |
| 3 | `gitlab_list_project_scan_profile_statuses` | Query | Read |

---

## Related

- [GitLab securityScanProfileAttach mutation](https://docs.gitlab.com/api/graphql/reference/#mutationsecurityscanprofileattach)
- [Vulnerabilities](vulnerabilities.md) — tracked vulnerability lifecycle management
- [Security Findings](security-findings.md) — pipeline scan findings
- [Security Attributes](security-attributes.md) — namespace-level classification labels
