# MCP Tool Icons

Visual identity for every tool, resource, and prompt in gitlab-mcp-server.

> **Diátaxis type**: Reference
> **Audience**: MCP client developers, contributors, integrators

## Overview

gitlab-mcp-server ships **43 unique SVG icons** assigned to all 1004 individual tools, 40/59 meta-tools, 24 resources, and 38 prompts. Icons help MCP clients render recognizable UI elements for each GitLab domain (branches, issues, pipelines, merge requests, etc.).

Icons are defined in [`internal/toolutil/icons.go`](../../internal/toolutil/icons.go) and consumed via the `Icons` field on every `mcp.Tool`, `mcp.Resource`, and `mcp.Prompt` registration.

## MCP Specification

Icons follow the [MCP Icon interface](https://modelcontextprotocol.io/specification/2025-11-25) (protocol version 2025-11-25):

```typescript
interface Icon {
  src: string;          // URI pointing to the icon (HTTP/HTTPS or data: URI)
  mimeType?: string;    // MIME type override
  sizes?: string[];     // Available sizes
  theme?: "light" | "dark"; // Theme hint
}
```

### Client MIME Type Support

| MIME Type | Support Level | Notes |
| --------- | ------------- | ----- |
| `image/png` | **MUST** support | Universal compatibility |
| `image/jpeg` | **MUST** support | Universal compatibility |
| `image/svg+xml` | **SHOULD** support | Scalable, used by this project |
| `image/webp` | **SHOULD** support | Modern efficient format |

gitlab-mcp-server uses `image/svg+xml` exclusively. Clients that only implement the MUST-level MIME types (PNG/JPEG) will not render these icons.

### Client Compatibility

| MCP Client | SVG Icons | Notes |
| ---------- | --------- | ----- |
| VS Code (GitHub Copilot) | Yes | Full SVG rendering support |
| Claude Desktop | No | Does not render tool icons |
| Continue.dev | Partial | Depends on version |

## Implementation Details

### Encoding Format

All icons use inline **data URIs** to avoid external network dependencies:

```text
data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" ...>...</svg>
```

### Design Principles

- **16×16 viewport** — minimal size optimized for tool lists and sidebars
- **Single-path SVGs** — lightweight markup, fast parsing
- **`currentColor` fill** — icons inherit the client's text color, adapting to light and dark themes automatically
- **No external dependencies** — data URIs embedded in the binary, zero network requests
- **One icon per domain** — related tools share the same icon for visual grouping

### Registration Pattern

Each tool sub-package assigns its icon in `register.go`:

```go
func RegisterTools(server *mcp.Server, client *gitlab.Client) {
    mcp.AddTool(server, branches.ListInput{}, branches.ListOutput{},
        &mcp.ToolOptions{
            Name:        "gitlab_list_branches",
            Description: "List branches in a project",
            Icons:       toolutil.IconBranch,  // ← icon assignment
        },
        handler,
    )
}
```

The `icon()` helper in `toolutil` wraps each SVG constant as a `[]mcp.Icon` slice:

```go
func icon(svg string) []mcp.Icon {
    return []mcp.Icon{{Source: "data:" + svgMIME + "," + svg, MIMEType: svgMIME}}
}
```

## Icon Gallery

All 43 icons with their SVG preview, exported variable name, and the tool packages that use each one.

<!-- markdownlint-disable MD033 -->

### Source Control

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='%23555' d='M11.75 2.5a.75.75 0 1 1 0 1.5.75.75 0 0 1 0-1.5m.75 3.17a2.25 2.25 0 1 0-1.5 0v.58A2.25 2.25 0 0 1 8.75 8.5h-2.5A3.73 3.73 0 0 0 4.5 9.3v.45a2.25 2.25 0 1 0 1.5 0V9.3a2.24 2.24 0 0 1 .25-.04h2.5a3.75 3.75 0 0 0 3.75-3.75zM4.25 12a.75.75 0 1 1 0 1.5.75.75 0 0 1 0-1.5'/%3E%3C/svg%3E" width="32" height="32" alt="Branch"> | `IconBranch` | branches, repository, repositorysubmodules |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Ccircle cx='8' cy='8' r='3' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3Cline x1='0' y1='8' x2='5' y2='8' stroke='%23555' stroke-width='1.5'/%3E%3Cline x1='11' y1='8' x2='16' y2='8' stroke='%23555' stroke-width='1.5'/%3E%3C/svg%3E" width="32" height="32" alt="Commit"> | `IconCommit` | commits, mrcontextcommits |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='%23555' d='M2 2h5.5l6.5 6.5-5.5 5.5L2 7.5zm3 1.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3'/%3E%3C/svg%3E" width="32" height="32" alt="Tag"> | `IconTag` | tags |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='%23555' d='M8 1l2 3h4l-3 3 1.5 4L8 8.5 3.5 11 5 7 2 4h4z'/%3E%3C/svg%3E" width="32" height="32" alt="Release"> | `IconRelease` | releases |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='none' stroke='%23555' stroke-width='1.5' d='M3 1.5h7l3 3v10H3z'/%3E%3Cline x1='5' y1='7' x2='11' y2='7' stroke='%23555' stroke-width='1'/%3E%3Cline x1='5' y1='9.5' x2='11' y2='9.5' stroke='%23555' stroke-width='1'/%3E%3C/svg%3E" width="32" height="32" alt="File"> | `IconFile` | files, markdown, pages |

### Issues and Planning

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Ccircle cx='8' cy='8' r='6.5' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3Ccircle cx='8' cy='8' r='2' fill='%23555'/%3E%3C/svg%3E" width="32" height="32" alt="Issue"> | `IconIssue` | issues, workitems |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Crect x='1' y='4' width='14' height='8' rx='4' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3C/svg%3E" width="32" height="32" alt="Label"> | `IconLabel` | awardemoji, badges, grouplabels, labels, topics |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='none' stroke='%23555' stroke-width='1.5' d='M2 14L8 2l6 12z'/%3E%3C/svg%3E" width="32" height="32" alt="Milestone"> | `IconMilestone` | groupmilestones, milestones |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Crect x='1' y='2' width='4' height='12' rx='1' fill='none' stroke='%23555' stroke-width='1'/%3E%3Crect x='6' y='2' width='4' height='8' rx='1' fill='none' stroke='%23555' stroke-width='1'/%3E%3Crect x='11' y='2' width='4' height='10' rx='1' fill='none' stroke='%23555' stroke-width='1'/%3E%3C/svg%3E" width="32" height="32" alt="Board"> | `IconBoard` | boards, groupboards |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='none' stroke='%23555' stroke-width='1.5' d='M6.5 9.5l3-3M4.5 8.5L3 10a2.8 2.8 0 0 0 4 4l1.5-1.5M11.5 7.5L13 6a2.8 2.8 0 0 0-4-4L7.5 3.5'/%3E%3C/svg%3E" width="32" height="32" alt="Link"> | `IconLink` | issuelinks, releaselinks |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Crect x='2' y='2' width='12' height='12' rx='1' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3Cpath d='M5 8l2 2 4-4' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3C/svg%3E" width="32" height="32" alt="Todo"> | `IconTodo` | todos |

### Merge Requests

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='%23555' d='M4.25 2.5a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5M4.5 5.67a2.25 2.25 0 1 1-1.5 0v4.66a2.25 2.25 0 1 1 1.5 0zm0 0'/%3E%3Ccircle cx='11.75' cy='12.25' r='.75' fill='%23555'/%3E%3Cpath fill='%23555' d='M11.75 9.75a2.25 2.25 0 1 0 .75 4.37V9.3a2.25 2.25 0 0 1-.75.45m0-7.25a.75.75 0 1 1 0 1.5.75.75 0 0 1 0-1.5m.75 3.17a2.25 2.25 0 1 0-1.5 0v3.58h1.5z'/%3E%3C/svg%3E" width="32" height="32" alt="MR"> | `IconMR` | deploymentmergerequests, mergerequests, mrapprovals, mrchanges |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='%23555' d='M2 2h12v8H6l-3 3v-3H2z'/%3E%3C/svg%3E" width="32" height="32" alt="Discussion"> | `IconDiscussion` | commitdiscussions, epicdiscussions, issuediscussions, issuenotes, mrdiscussions, mrdraftnotes, mrnotes, snippetdiscussions |

### CI/CD

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Ccircle cx='3' cy='8' r='2' fill='%23555'/%3E%3Ccircle cx='8' cy='8' r='2' fill='%23555'/%3E%3Ccircle cx='13' cy='8' r='2' fill='%23555'/%3E%3Cline x1='5' y1='8' x2='6' y2='8' stroke='%23555' stroke-width='1'/%3E%3Cline x1='10' y1='8' x2='11' y2='8' stroke='%23555' stroke-width='1'/%3E%3C/svg%3E" width="32" height="32" alt="Pipeline"> | `IconPipeline` | cilint, pipelines, pipelinetriggers |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Crect x='2' y='2' width='12' height='12' rx='2' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3Cpath d='M6 5.5l4.5 2.5L6 10.5z' fill='%23555'/%3E%3C/svg%3E" width="32" height="32" alt="Job"> | `IconJob` | jobs, jobtokenscope |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Crect x='3' y='2' width='10' height='12' rx='1' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3Ccircle cx='6' cy='6' r='1' fill='%23555'/%3E%3Ccircle cx='10' cy='6' r='1' fill='%23555'/%3E%3Cline x1='5' y1='10' x2='11' y2='10' stroke='%23555' stroke-width='1'/%3E%3C/svg%3E" width="32" height="32" alt="Runner"> | `IconRunner` | clusteragents, runners, runnercontrollers, runnercontrollerscopes |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Ccircle cx='8' cy='8' r='6' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3Cline x1='8' y1='4' x2='8' y2='8' stroke='%23555' stroke-width='1.5'/%3E%3Cline x1='8' y1='8' x2='11' y2='10' stroke='%23555' stroke-width='1.5'/%3E%3C/svg%3E" width="32" height="32" alt="Schedule"> | `IconSchedule` | freezeperiods, pipelineschedules |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Ctext x='3' y='12' font-size='12' font-family='monospace' fill='%23555'%3E%7Bx%7D%3C/text%3E%3C/svg%3E" width="32" height="32" alt="Variable"> | `IconVariable` | civariables, groupvariables, instancevariables |

### Environments and Deployments

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Ccircle cx='8' cy='8' r='6' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3Cellipse cx='8' cy='8' rx='3' ry='6' fill='none' stroke='%23555' stroke-width='1'/%3E%3Cline x1='2' y1='8' x2='14' y2='8' stroke='%23555' stroke-width='1'/%3E%3C/svg%3E" width="32" height="32" alt="Environment"> | `IconEnvironment` | environments |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='%23555' d='M8 2v8m0 0l-3-3m3 3l3-3'/%3E%3Cline x1='3' y1='13' x2='13' y2='13' stroke='%23555' stroke-width='1.5'/%3E%3C/svg%3E" width="32" height="32" alt="Deploy"> | `IconDeploy` | deployments |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Crect x='5' y='1' width='6' height='4' rx='1' fill='none' stroke='%23555' stroke-width='1'/%3E%3Crect x='1' y='11' width='6' height='4' rx='1' fill='none' stroke='%23555' stroke-width='1'/%3E%3Crect x='9' y='11' width='6' height='4' rx='1' fill='none' stroke='%23555' stroke-width='1'/%3E%3Cline x1='8' y1='5' x2='8' y2='8' stroke='%23555' stroke-width='1'/%3E%3Cline x1='4' y1='8' x2='12' y2='8' stroke='%23555' stroke-width='1'/%3E%3Cline x1='4' y1='8' x2='4' y2='11' stroke='%23555' stroke-width='1'/%3E%3Cline x1='12' y1='8' x2='12' y2='11' stroke='%23555' stroke-width='1'/%3E%3C/svg%3E" width="32" height="32" alt="Infra"> | `IconInfra` | terraformstates |

### Projects and Groups

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Crect x='2' y='3' width='12' height='10' rx='1' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3Cline x1='2' y1='6' x2='14' y2='6' stroke='%23555' stroke-width='1.5'/%3E%3C/svg%3E" width="32" height="32" alt="Project"> | `IconProject` | projectdiscovery, projects |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Ccircle cx='5' cy='5' r='2' fill='%23555'/%3E%3Ccircle cx='11' cy='5' r='2' fill='%23555'/%3E%3Cpath fill='%23555' d='M1 12c0-2 2-3 4-3s4 1 4 3zm6 0c0-2 2-3 4-3s4 1 4 3z'/%3E%3C/svg%3E" width="32" height="32" alt="Group"> | `IconGroup` | groups, namespaces, resourcegroups |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Ccircle cx='8' cy='5' r='3' fill='%23555'/%3E%3Cpath fill='%23555' d='M2 14c0-3 3-5 6-5s6 2 6 5z'/%3E%3C/svg%3E" width="32" height="32" alt="User"> | `IconUser` | accessrequests, avatar, ffuserlists, groupmembers, invites, members, users |

### Packages and Registry

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='none' stroke='%23555' stroke-width='1.5' d='M8 1L14 4.5v7L8 15 2 11.5v-7z'/%3E%3Cline x1='8' y1='8' x2='8' y2='15' stroke='%23555' stroke-width='1'/%3E%3Cline x1='2' y1='4.5' x2='8' y2='8' stroke='%23555' stroke-width='1'/%3E%3Cline x1='14' y1='4.5' x2='8' y2='8' stroke='%23555' stroke-width='1'/%3E%3C/svg%3E" width="32" height="32" alt="Package"> | `IconPackage` | dependencyproxy, packages |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Crect x='2' y='3' width='12' height='10' rx='1' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3Cline x1='5' y1='3' x2='5' y2='13' stroke='%23555' stroke-width='1'/%3E%3Cline x1='8' y1='3' x2='8' y2='13' stroke='%23555' stroke-width='1'/%3E%3Cline x1='11' y1='3' x2='11' y2='13' stroke='%23555' stroke-width='1'/%3E%3Cline x1='2' y1='8' x2='14' y2='8' stroke='%23555' stroke-width='1'/%3E%3C/svg%3E" width="32" height="32" alt="Container"> | `IconContainer` | containerregistry |

### Search and Analytics

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Ccircle cx='6.5' cy='6.5' r='4.5' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3Cline x1='10' y1='10' x2='14.5' y2='14.5' stroke='%23555' stroke-width='1.5'/%3E%3C/svg%3E" width="32" height="32" alt="Search"> | `IconSearch` | search |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpolyline points='1,14 5,6 9,10 15,2' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3C/svg%3E" width="32" height="32" alt="Analytics"> | `IconAnalytics` | appstatistics, issuestatistics, projectstatistics, samplingtools, usagedata |

### Security and Access

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='%23555' d='M8 1L2 4v4c0 4 3 6 6 7 3-1 6-3 6-7V4z'/%3E%3C/svg%3E" width="32" height="32" alt="Security"> | `IconSecurity` | license, protectedenvs, securefiles |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Ccircle cx='6' cy='8' r='4' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3Cline x1='10' y1='8' x2='15' y2='8' stroke='%23555' stroke-width='1.5'/%3E%3Cline x1='13' y1='6' x2='13' y2='10' stroke='%23555' stroke-width='1.5'/%3E%3C/svg%3E" width="32" height="32" alt="Token"> | `IconToken` | accesstokens, deploytokens, jobtokenscope, runnercontrollertokens |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Ccircle cx='5' cy='8' r='3' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3Cline x1='8' y1='8' x2='14' y2='8' stroke='%23555' stroke-width='1.5'/%3E%3Cline x1='12' y1='6' x2='12' y2='8' stroke='%23555' stroke-width='1.5'/%3E%3C/svg%3E" width="32" height="32" alt="Key"> | `IconKey` | deploykeys, keys |

### Documentation and Content

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='%23555' d='M2 2h8l4 4v8H2zm8 0v4h4'/%3E%3Cline x1='4' y1='8' x2='10' y2='8' stroke='%23555' stroke-width='1'/%3E%3Cline x1='4' y1='10' x2='10' y2='10' stroke='%23555' stroke-width='1'/%3E%3C/svg%3E" width="32" height="32" alt="Wiki"> | `IconWiki` | wikis |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='none' stroke='%23555' stroke-width='1.5' d='M5 4L1 8l4 4m6-8l4 4-4 4'/%3E%3C/svg%3E" width="32" height="32" alt="Snippet"> | `IconSnippet` | snippets |

### Configuration and Administration

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Ccircle cx='8' cy='8' r='2.5' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3Cpath fill='%23555' d='M7 1h2v2.1a5 5 0 0 1 1.7.7L12.1 2.4l1.4 1.4-1.4 1.4a5 5 0 0 1 .7 1.7H15v2h-2.1a5 5 0 0 1-.7 1.7l1.4 1.4-1.4 1.4-1.4-1.4a5 5 0 0 1-1.7.7V15H7v-2.1a5 5 0 0 1-1.7-.7L3.9 13.6 2.5 12.2l1.4-1.4a5 5 0 0 1-.7-1.7H1V7h2.1a5 5 0 0 1 .7-1.7L2.5 3.9 3.9 2.5l1.4 1.4A5 5 0 0 1 7 3.1z'/%3E%3C/svg%3E" width="32" height="32" alt="Config"> | `IconConfig` | appearance, applications, customattributes, dbmigrations, elicitationtools, featureflags, features, planlimits, settings, sidekiq |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Crect x='2' y='2' width='12' height='5' rx='1' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3Crect x='2' y='9' width='12' height='5' rx='1' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3Ccircle cx='5' cy='4.5' r='.75' fill='%23555'/%3E%3Ccircle cx='5' cy='11.5' r='.75' fill='%23555'/%3E%3C/svg%3E" width="32" height="32" alt="Server"> | `IconServer` | metadata, serverupdate |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Crect x='2' y='2' width='12' height='12' rx='1' fill='none' stroke='%23555' stroke-width='1.5'/%3E%3Cline x1='2' y1='6' x2='14' y2='6' stroke='%23555' stroke-width='1'/%3E%3Cline x1='6' y1='6' x2='6' y2='14' stroke='%23555' stroke-width='1'/%3E%3C/svg%3E" width="32" height="32" alt="Template"> | `IconTemplate` | ciyamltemplates, dockerfiletemplates, gitignoretemplates, licensetemplates, projecttemplates |

### Notifications and Events

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='%23555' d='M8 1C5 1 4 4 4 6v3l-2 2h12l-2-2V6c0-2-1-5-4-5m-2 13h4c0 1-1 2-2 2s-2-1-2-2'/%3E%3C/svg%3E" width="32" height="32" alt="Notify"> | `IconNotify` | broadcastmessages, notifications |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='%23555' d='M10 1L6 9h3l-2 6 6-8H9l3-6z'/%3E%3C/svg%3E" width="32" height="32" alt="Event"> | `IconEvent` | events, resourceevents |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='%23555' d='M8 1L1 14h14zM8 6v4m0 2v1'/%3E%3C/svg%3E" width="32" height="32" alt="Alert"> | `IconAlert` | alertmanagement, errortracking |

### Integrations and Operations

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='%23555' d='M6 2v4H2v4h4v4h4v-4h4V6h-4V2z'/%3E%3C/svg%3E" width="32" height="32" alt="Integration"> | `IconIntegration` | integrations, systemhooks |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='%23555' d='M8 14s-5.5-3.5-5.5-7A3.5 3.5 0 0 1 8 4.5 3.5 3.5 0 0 1 13.5 7c0 3.5-5.5 7-5.5 7'/%3E%3C/svg%3E" width="32" height="32" alt="Health"> | `IconHealth` | health |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='%23555' d='M8 10V2m0 0L5 5m3-3l3 3'/%3E%3Cline x1='3' y1='13' x2='13' y2='13' stroke='%23555' stroke-width='1.5'/%3E%3C/svg%3E" width="32" height="32" alt="Upload"> | `IconUpload` | groupmarkdownuploads, uploads |
| <img src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' viewBox='0 0 16 16'%3E%3Cpath fill='%23555' d='M8 2v8m0 0l3-3m-3 3L5 7'/%3E%3Crect x='2' y='12' width='12' height='2' rx='1' fill='%23555'/%3E%3C/svg%3E" width="32" height="32" alt="Import"> | `IconImport` | bulkimports, groupimportexport, grouprelationsexport, importservice, projectimportexport |

<!-- markdownlint-enable MD033 -->

## Complete Icon-to-Package Reference

Alphabetical listing of all 43 icons and every sub-package that uses each one.

| Icon | Variable | Packages (110 total) |
| ---- | -------- | -------------------- |
| Alert | `IconAlert` | alertmanagement, errortracking |
| Analytics | `IconAnalytics` | appstatistics, issuestatistics, projectstatistics, samplingtools, usagedata |
| Board | `IconBoard` | boards, groupboards |
| Branch | `IconBranch` | branches, repository, repositorysubmodules |
| Commit | `IconCommit` | commits, mrcontextcommits |
| Config | `IconConfig` | appearance, applications, customattributes, dbmigrations, elicitationtools, featureflags, features, planlimits, settings, sidekiq |
| Container | `IconContainer` | containerregistry |
| Deploy | `IconDeploy` | deployments |
| Discussion | `IconDiscussion` | commitdiscussions, epicdiscussions, issuediscussions, issuenotes, mrdiscussions, mrdraftnotes, mrnotes, snippetdiscussions |
| Environment | `IconEnvironment` | environments |
| Event | `IconEvent` | events, resourceevents |
| File | `IconFile` | files, markdown, pages |
| Group | `IconGroup` | groups, namespaces, resourcegroups |
| Health | `IconHealth` | health |
| Import | `IconImport` | bulkimports, groupimportexport, grouprelationsexport, importservice, projectimportexport |
| Infra | `IconInfra` | terraformstates |
| Integration | `IconIntegration` | integrations, systemhooks |
| Issue | `IconIssue` | issues, workitems |
| Job | `IconJob` | jobs, jobtokenscope |
| Key | `IconKey` | deploykeys, keys |
| Label | `IconLabel` | awardemoji, badges, grouplabels, labels, topics |
| Link | `IconLink` | issuelinks, releaselinks |
| MR | `IconMR` | deploymentmergerequests, mergerequests, mrapprovals, mrchanges |
| Milestone | `IconMilestone` | groupmilestones, milestones |
| Notify | `IconNotify` | broadcastmessages, notifications |
| Package | `IconPackage` | dependencyproxy, packages |
| Pipeline | `IconPipeline` | cilint, pipelines, pipelinetriggers |
| Project | `IconProject` | projectdiscovery, projects |
| Release | `IconRelease` | releases |
| Runner | `IconRunner` | clusteragents, runners, runnercontrollers, runnercontrollerscopes |
| Schedule | `IconSchedule` | freezeperiods, pipelineschedules |
| Search | `IconSearch` | search |
| Security | `IconSecurity` | license, protectedenvs, securefiles |
| Server | `IconServer` | metadata, serverupdate |
| Snippet | `IconSnippet` | snippets |
| Tag | `IconTag` | tags |
| Template | `IconTemplate` | ciyamltemplates, dockerfiletemplates, gitignoretemplates, licensetemplates, projecttemplates |
| Todo | `IconTodo` | todos |
| Token | `IconToken` | accesstokens, deploytokens, jobtokenscope, runnercontrollertokens |
| Upload | `IconUpload` | groupmarkdownuploads, uploads |
| User | `IconUser` | accessrequests, avatar, ffuserlists, groupmembers, invites, members, users |
| Variable | `IconVariable` | civariables, groupvariables, instancevariables |
| Wiki | `IconWiki` | wikis |

## Testing

Icon integrity is validated by 4 unit tests in [`internal/toolutil/icons_test.go`](../../internal/toolutil/icons_test.go):

| Test | Validates |
| ---- | --------- |
| `TestAllIcons_ValidDataURI` | Every icon starts with `data:image/svg+xml,` |
| `TestAllIcons_CorrectMIMEType` | MIME type is `image/svg+xml` |
| `TestAllIcons_NonEmpty` | Source is not empty |
| `TestAllIcons_ContainsSVG` | Source string contains `<svg` markup |

## Security Considerations

Per the MCP specification, clients should treat icon data as untrusted:

- **Validate URIs** — only accept `data:`, `https:`, or `http:` schemes
- **Sanitize SVGs** — SVG content may contain scripts; clients should strip `<script>` tags and event handlers before rendering
- **Restrict resource loading** — icons should not trigger network requests for sub-resources

This project mitigates these risks by using self-contained inline SVGs with no external references, no JavaScript, and no event handlers.

## References

- [MCP Specification — Icons](https://modelcontextprotocol.io/specification/2025-11-25)
- [MCP Go SDK — Icon Type](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#Icon)
- [SVG Data URI Encoding](https://developer.mozilla.org/en-US/docs/Web/SVG/Tutorial/SVG_as_an_Image)
- [Source Code — icons.go](../../internal/toolutil/icons.go)
- [Source Code — icons_test.go](../../internal/toolutil/icons_test.go)
