# MCP Tool Icons

Visual identity for every tool, resource, and prompt in gitlab-mcp-server.

> **Diátaxis type**: Reference
> **Audience**: MCP client developers, contributors, integrators

## Overview

gitlab-mcp-server ships **44 unique SVG icons** assigned to all 1004 individual tools, 40/59 meta-tools, 24 resources, and 38 prompts. Icons help MCP clients render recognizable UI elements for each GitLab domain (branches, issues, pipelines, merge requests, etc.).

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

All 44 icons with their SVG preview, exported variable name, and the tool packages that use each one.

<!-- markdownlint-disable MD033 -->

### Source Control

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="icons/branch.svg" width="32" height="32" alt="Branch"> | `IconBranch` | branches, repository, repositorysubmodules |
| <img src="icons/commit.svg" width="32" height="32" alt="Commit"> | `IconCommit` | commits, mrcontextcommits |
| <img src="icons/tag.svg" width="32" height="32" alt="Tag"> | `IconTag` | tags |
| <img src="icons/release.svg" width="32" height="32" alt="Release"> | `IconRelease` | releases |
| <img src="icons/file.svg" width="32" height="32" alt="File"> | `IconFile` | files, markdown, pages |

### Issues and Planning

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="icons/issue.svg" width="32" height="32" alt="Issue"> | `IconIssue` | issues, workitems |
| <img src="icons/label.svg" width="32" height="32" alt="Label"> | `IconLabel` | awardemoji, badges, grouplabels, labels, topics |
| <img src="icons/milestone.svg" width="32" height="32" alt="Milestone"> | `IconMilestone` | groupmilestones, milestones |
| <img src="icons/board.svg" width="32" height="32" alt="Board"> | `IconBoard` | boards, groupboards |
| <img src="icons/link.svg" width="32" height="32" alt="Link"> | `IconLink` | issuelinks, releaselinks |
| <img src="icons/epic.svg" width="32" height="32" alt="Epic"> | `IconEpic` | epicissues, epicnotes, epics |
| <img src="icons/todo.svg" width="32" height="32" alt="Todo"> | `IconTodo` | todos |

### Merge Requests

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="icons/mr.svg" width="32" height="32" alt="MR"> | `IconMR` | deploymentmergerequests, mergerequests, mrapprovals, mrchanges |
| <img src="icons/discussion.svg" width="32" height="32" alt="Discussion"> | `IconDiscussion` | commitdiscussions, epicdiscussions, issuediscussions, issuenotes, mrdiscussions, mrdraftnotes, mrnotes, snippetdiscussions |

### CI/CD

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="icons/pipeline.svg" width="32" height="32" alt="Pipeline"> | `IconPipeline` | cilint, pipelines, pipelinetriggers |
| <img src="icons/job.svg" width="32" height="32" alt="Job"> | `IconJob` | jobs, jobtokenscope |
| <img src="icons/runner.svg" width="32" height="32" alt="Runner"> | `IconRunner` | clusteragents, runners, runnercontrollers, runnercontrollerscopes |
| <img src="icons/schedule.svg" width="32" height="32" alt="Schedule"> | `IconSchedule` | freezeperiods, pipelineschedules |
| <img src="icons/variable.svg" width="32" height="32" alt="Variable"> | `IconVariable` | civariables, groupvariables, instancevariables |

### Environments and Deployments

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="icons/environment.svg" width="32" height="32" alt="Environment"> | `IconEnvironment` | environments |
| <img src="icons/deploy.svg" width="32" height="32" alt="Deploy"> | `IconDeploy` | deployments |
| <img src="icons/infra.svg" width="32" height="32" alt="Infra"> | `IconInfra` | terraformstates |

### Projects and Groups

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="icons/project.svg" width="32" height="32" alt="Project"> | `IconProject` | projectdiscovery, projects |
| <img src="icons/group.svg" width="32" height="32" alt="Group"> | `IconGroup` | groups, namespaces, resourcegroups |
| <img src="icons/user.svg" width="32" height="32" alt="User"> | `IconUser` | accessrequests, avatar, ffuserlists, groupmembers, invites, members, users |

### Packages and Registry

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="icons/package.svg" width="32" height="32" alt="Package"> | `IconPackage` | dependencyproxy, packages |
| <img src="icons/container.svg" width="32" height="32" alt="Container"> | `IconContainer` | containerregistry |

### Search and Analytics

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="icons/search.svg" width="32" height="32" alt="Search"> | `IconSearch` | search |
| <img src="icons/analytics.svg" width="32" height="32" alt="Analytics"> | `IconAnalytics` | appstatistics, issuestatistics, projectstatistics, samplingtools, usagedata |

### Security and Access

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="icons/security.svg" width="32" height="32" alt="Security"> | `IconSecurity` | license, protectedenvs, securefiles |
| <img src="icons/token.svg" width="32" height="32" alt="Token"> | `IconToken` | accesstokens, deploytokens, jobtokenscope, runnercontrollertokens |
| <img src="icons/key.svg" width="32" height="32" alt="Key"> | `IconKey` | deploykeys, keys |

### Documentation and Content

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="icons/wiki.svg" width="32" height="32" alt="Wiki"> | `IconWiki` | wikis |
| <img src="icons/snippet.svg" width="32" height="32" alt="Snippet"> | `IconSnippet` | snippets |

### Configuration and Administration

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="icons/config.svg" width="32" height="32" alt="Config"> | `IconConfig` | appearance, applications, customattributes, dbmigrations, elicitationtools, featureflags, features, planlimits, settings, sidekiq |
| <img src="icons/server.svg" width="32" height="32" alt="Server"> | `IconServer` | metadata, serverupdate |
| <img src="icons/template.svg" width="32" height="32" alt="Template"> | `IconTemplate` | ciyamltemplates, dockerfiletemplates, gitignoretemplates, licensetemplates, projecttemplates |

### Notifications and Events

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="icons/notify.svg" width="32" height="32" alt="Notify"> | `IconNotify` | broadcastmessages, notifications |
| <img src="icons/event.svg" width="32" height="32" alt="Event"> | `IconEvent` | events, resourceevents |
| <img src="icons/alert.svg" width="32" height="32" alt="Alert"> | `IconAlert` | alertmanagement, errortracking |

### Integrations and Operations

| Preview | Name | Packages |
| ------- | ---- | -------- |
| <img src="icons/integration.svg" width="32" height="32" alt="Integration"> | `IconIntegration` | integrations, systemhooks |
| <img src="icons/health.svg" width="32" height="32" alt="Health"> | `IconHealth` | health |
| <img src="icons/upload.svg" width="32" height="32" alt="Upload"> | `IconUpload` | groupmarkdownuploads, uploads |
| <img src="icons/import.svg" width="32" height="32" alt="Import"> | `IconImport` | bulkimports, groupimportexport, grouprelationsexport, importservice, projectimportexport |

<!-- markdownlint-enable MD033 -->

## Complete Icon-to-Package Reference

Alphabetical listing of all 44 icons and every sub-package that uses each one.

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
| Epic | `IconEpic` | epicissues, epicnotes, epics |
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
