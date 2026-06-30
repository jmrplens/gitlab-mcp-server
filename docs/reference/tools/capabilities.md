# MCP Capabilities — Tool Reference

> **Diátaxis type**: Reference
> **Domain**: MCP Capabilities (Elicitation, Health)
> **Individual tools**: 15
> **Meta-tool**: elicitation tools are always registered individually; health is included in `gitlab_server` meta-tool as `status` action
> **MCP Protocol**: [Elicitation](https://modelcontextprotocol.io/specification/2025-11-25/client/elicitation)
> **Audience**: 👤 End users, AI assistant users

---

## Overview

The MCP capabilities domain covers special tools that leverage Model Context Protocol capabilities rather than standard GitLab REST API endpoints. These tools use **MCP elicitation** (interactive step-by-step user prompts for resource creation), and **health diagnostics** (server connectivity checks).

Elicitation tools require the MCP client to support the elicitation capability. If the client does not support the required capability, the tool returns an informational message instead of failing.

### Common Questions

> "Check the server version"
> "Is the server healthy?"

### Annotation Legend

| Annotation | ReadOnly | Destructive | Idempotent | Description              |
| ---------- | :------: | :---------: | :--------: | ------------------------ |
| **Read**   |   Yes    |     No      |    Yes     | Safe read-only operation |
| **Create** |    —     |     No      |     —      | Creates a new resource   |

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
| 12 | `gitlab_interactive_issue_create` | Elicitation | Create |
| 13 | `gitlab_interactive_mr_create` | Elicitation | Create |
| 14 | `gitlab_interactive_release_create` | Elicitation | Create |
| 15 | `gitlab_interactive_project_create` | Elicitation | Create |
| 16 | `gitlab_server_status` | Health | Read |

### Capability Requirements

| Tool                                | Required MCP Capability | Fallback Behavior                            |
| ----------------------------------- | ----------------------- | -------------------------------------------- |
| `gitlab_interactive_issue_create`   | Elicitation             | Returns informational message if unsupported |
| `gitlab_interactive_mr_create`      | Elicitation             | Returns informational message if unsupported |
| `gitlab_interactive_release_create` | Elicitation             | Returns informational message if unsupported |
| `gitlab_interactive_project_create` | Elicitation             | Returns informational message if unsupported |
| `gitlab_server_status`              | None                    | Always available                             |

---

## Related

- [MCP Elicitation Specification](https://modelcontextprotocol.io/specification/2025-11-25/client/elicitation)
