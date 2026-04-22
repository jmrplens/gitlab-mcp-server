# MCP Capabilities — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: MCP Capabilities (Sampling, Elicitation, Health)
> **Individual tools**: 16
> **Meta-tool**: `gitlab_analyze` (11 sampling actions), elicitation tools are always registered individually; health is included in `gitlab_server` meta-tool as `status` action
> **MCP Protocol**: [Sampling](https://modelcontextprotocol.io/specification/2025-11-25/client/sampling), [Elicitation](https://modelcontextprotocol.io/specification/2025-11-25/client/elicitation)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The MCP capabilities domain covers special tools that leverage Model Context Protocol capabilities rather than standard GitLab REST API endpoints. These tools use **MCP sampling** (LLM-assisted analysis with human-in-the-loop approval), **MCP elicitation** (interactive step-by-step user prompts for resource creation), and **health diagnostics** (server connectivity checks).

Sampling tools require the MCP client to support the sampling capability. Elicitation tools require the MCP client to support the elicitation capability. If the client does not support the required capability, the tool returns an informational message instead of failing.

### Common Questions

> "Check the server version"
> "Is the server healthy?"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description |
| ---------- | :------: | :---------: | :--------: | ----------- |
| **Read**   | Yes | No | Yes | Safe read-only operation |
| **Create** | — | No | — | Creates a new resource |

---

## Sampling Tools

### `gitlab_analyze_mr_changes`

Analyze a GitLab merge request using LLM-assisted code review via MCP sampling. Fetches MR details and diffs, then requests LLM analysis for code quality, bugs, and improvements. Requires the MCP client to support the sampling capability (human-in-the-loop approval).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_summarize_issue`

Summarize a GitLab issue discussion using LLM-assisted analysis via MCP sampling. Fetches issue details and all notes, then requests LLM summary of key decisions and action items. Requires the MCP client to support the sampling capability (human-in-the-loop approval).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_generate_release_notes`

Generate polished release notes using LLM-assisted analysis via MCP sampling. Compares two Git refs, fetches commits and merged MRs with labels, then requests LLM to produce categorized release notes (Features, Bug Fixes, Improvements, Breaking Changes). Requires the MCP client to support the sampling capability (human-in-the-loop approval).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_analyze_pipeline_failure`

Analyze a GitLab pipeline failure using LLM-assisted root cause analysis via MCP sampling. Fetches pipeline details, failed jobs and their traces, then requests LLM analysis for root cause, fix suggestions, and impact assessment. Requires the MCP client to support the sampling capability (human-in-the-loop approval).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_summarize_mr_review`

Summarize a GitLab merge request review using LLM-assisted analysis via MCP sampling. Fetches MR details, discussions, and approval state, then requests LLM summary of reviewer feedback, unresolved threads, and action items. Requires the MCP client to support the sampling capability (human-in-the-loop approval).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_generate_milestone_report`

Generate a comprehensive milestone progress report using LLM-assisted analysis via MCP sampling. Fetches milestone details, linked issues and merge requests, then requests LLM to produce a data-driven progress report with metrics, risks, and recommendations. Requires the MCP client to support the sampling capability (human-in-the-loop approval).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_analyze_ci_configuration`

Analyze a GitLab project's CI/CD configuration using LLM-assisted analysis via MCP sampling. Lints the CI config, fetches merged YAML and includes, then requests LLM analysis for best practices, performance, security, and maintainability. Requires the MCP client to support the sampling capability (human-in-the-loop approval).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_analyze_issue_scope`

Analyze a GitLab issue's scope and effort using LLM-assisted analysis via MCP sampling. Fetches issue details, time stats, participants, related MRs, and discussion notes, then requests LLM to assess scope, complexity, risks, and whether the issue should be broken down. Requires the MCP client to support the sampling capability (human-in-the-loop approval).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_review_mr_security`

Perform a security-focused review of a GitLab merge request using LLM-assisted analysis via MCP sampling. Fetches MR details and code diffs, then requests LLM to identify injection vulnerabilities, auth issues, exposed secrets, and OWASP Top 10 findings. Requires the MCP client to support the sampling capability (human-in-the-loop approval).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_find_technical_debt`

Find and analyze technical debt in a GitLab project using LLM-assisted analysis via MCP sampling. Searches for TODO, FIXME, HACK, XXX, and DEPRECATED markers in source code, then requests LLM to categorize, prioritize, and recommend a remediation strategy. Requires the MCP client to support the sampling capability (human-in-the-loop approval).

| Annotation | **Read** |
| ---------- | -------- |

### `gitlab_analyze_deployment_history`

Analyze deployment history and patterns for a GitLab project using LLM-assisted analysis via MCP sampling. Fetches recent deployments, then requests LLM to assess deployment frequency, success rate, rollback patterns, and suggest improvements. Requires the MCP client to support the sampling capability (human-in-the-loop approval).

| Annotation | **Read** |
| ---------- | -------- |

---

## Elicitation Tools

### `gitlab_interactive_issue_create`

Interactively create a GitLab issue with step-by-step user prompts via MCP elicitation. Guides the user through entering title, description, labels, and confidentiality settings with confirmation before creation. Requires the MCP client to support the elicitation capability.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_interactive_mr_create`

Interactively create a GitLab merge request with step-by-step user prompts via MCP elicitation. Guides the user through entering branches, title, description, labels, squash/remove-source options with confirmation. Requires the MCP client to support the elicitation capability.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_interactive_release_create`

Interactively create a GitLab release with step-by-step user prompts via MCP elicitation. Guides the user through entering tag name, release name, description with confirmation before creation. Requires the MCP client to support the elicitation capability.

| Annotation | **Create** |
| ---------- | ---------- |

### `gitlab_interactive_project_create`

Interactively create a GitLab project with step-by-step user prompts via MCP elicitation. Guides the user through entering name, description, visibility, README initialization, and default branch with confirmation. Requires the MCP client to support the elicitation capability.

| Annotation | **Create** |
| ---------- | ---------- |

---

## Health

### `gitlab_server_status`

Check MCP server health and GitLab connectivity. Returns server version, author, department, repository, GitLab version, authentication status, current user, and response time. Use this to diagnose connection issues.

| Annotation | **Read** |
| ---------- | -------- |

---

## Tool Summary

| # | Tool Name | Category | Annotation |
| --: | --------- | -------- | :--------: |
| 1 | `gitlab_analyze_mr_changes` | Sampling | Read |
| 2 | `gitlab_summarize_issue` | Sampling | Read |
| 3 | `gitlab_generate_release_notes` | Sampling | Read |
| 4 | `gitlab_analyze_pipeline_failure` | Sampling | Read |
| 5 | `gitlab_summarize_mr_review` | Sampling | Read |
| 6 | `gitlab_generate_milestone_report` | Sampling | Read |
| 7 | `gitlab_analyze_ci_configuration` | Sampling | Read |
| 8 | `gitlab_analyze_issue_scope` | Sampling | Read |
| 9 | `gitlab_review_mr_security` | Sampling | Read |
| 10 | `gitlab_find_technical_debt` | Sampling | Read |
| 11 | `gitlab_analyze_deployment_history` | Sampling | Read |
| 12 | `gitlab_interactive_issue_create` | Elicitation | Create |
| 13 | `gitlab_interactive_mr_create` | Elicitation | Create |
| 14 | `gitlab_interactive_release_create` | Elicitation | Create |
| 15 | `gitlab_interactive_project_create` | Elicitation | Create |
| 16 | `gitlab_server_status` | Health | Read |

### Capability Requirements

| Tool | Required MCP Capability | Fallback Behavior |
| ---- | ----------------------- | ----------------- |
| `gitlab_analyze_mr_changes` | Sampling | Returns informational message if unsupported |
| `gitlab_summarize_issue` | Sampling | Returns informational message if unsupported |
| `gitlab_generate_release_notes` | Sampling | Returns informational message if unsupported |
| `gitlab_analyze_pipeline_failure` | Sampling | Returns informational message if unsupported |
| `gitlab_summarize_mr_review` | Sampling | Returns informational message if unsupported |
| `gitlab_generate_milestone_report` | Sampling | Returns informational message if unsupported |
| `gitlab_analyze_ci_configuration` | Sampling | Returns informational message if unsupported |
| `gitlab_analyze_issue_scope` | Sampling | Returns informational message if unsupported |
| `gitlab_review_mr_security` | Sampling | Returns informational message if unsupported |
| `gitlab_find_technical_debt` | Sampling | Returns informational message if unsupported |
| `gitlab_analyze_deployment_history` | Sampling | Returns informational message if unsupported |
| `gitlab_interactive_issue_create` | Elicitation | Returns informational message if unsupported |
| `gitlab_interactive_mr_create` | Elicitation | Returns informational message if unsupported |
| `gitlab_interactive_release_create` | Elicitation | Returns informational message if unsupported |
| `gitlab_interactive_project_create` | Elicitation | Returns informational message if unsupported |
| `gitlab_server_status` | None | Always available |

---

## Related

- [MCP Sampling Specification](https://modelcontextprotocol.io/specification/2025-11-25/client/sampling)
- [MCP Elicitation Specification](https://modelcontextprotocol.io/specification/2025-11-25/client/elicitation)
