# Tool Documentation

Per-domain tool reference for gitlab-mcp-server. Each document covers one logical domain, listing all individual MCP tools with their descriptions, parameter tables, and annotation types.

## Server scope rationale

Every operation is defined once in the canonical action catalog and projected onto three surfaces: the default dynamic pair (`gitlab_find_action` and `gitlab_execute_action`, which call actions by `domain.action` ID), a curated set of domain meta-tools (`GITLAB_MCP_TOOL_SURFACE=meta`), each consolidating a non-overlapping GitLab REST/GraphQL surface area, and one tool per action (`GITLAB_MCP_TOOL_SURFACE=individual`). The domain boundary described below is the meta-tool boundary, and it is also the `domain` half of every canonical ID, so it holds on every surface. The self-managed Enterprise/Premium catalog exposes 1073 individual tools; GitLab.com Enterprise/Premium adds 6 experimental Orbit Knowledge Graph tools for a maximum of 1079. The meta-tool boundary maps 1:1 to GitLab's resource taxonomy (project, group, merge_request, issue, pipeline, etc.), so domains never duplicate functionality: lifecycle CRUD lives in the resource's own meta-tool (`gitlab_merge_request` for MR open/merge/close), peripheral concerns ship as siblings (`gitlab_mr_review` for diffs, comments, draft notes; `gitlab_pipeline` for CI runs, never MR pipelines). Project discovery in `gitlab_discover_project`, Orbit graph queries in `gitlab_orbit`, and infrastructure self-management in `gitlab_server`. Enterprise/Premium-only domains (epics, audit events, DORA metrics, vulnerabilities, compliance, Orbit, etc.) are gated by `GITLAB_MCP_TIER` (Premium or Ultimate) and either ship as dedicated meta-tools or attach as additional actions on the relevant base meta-tool, never as duplicate alternatives. This keeps every meta-tool independently invokable, free of overlap with its siblings, and minimal in surface area for LLM tool selection.

The [dynamic toolset](../../concepts/dynamic-tools.md) reuses the same canonical action catalog as meta-tools. In dynamic mode, clients see only `gitlab_find_action` and `gitlab_execute_action`, then discover and execute the same canonical domain actions progressively.

This directory is a domain-oriented reference, not a one-heading-per-runtime-tool dump. The table below groups related actions into stable user-facing domains so humans can scan the API surface without reading more than a thousand individual entries. For the exact runtime catalog exposed by the current `GITLAB_MCP_TOOL_SURFACE`, including every action ID, input schema, output schema, annotations, compatibility names, and deprecation state, read `gitlab://tools` and then `gitlab://tools/{id}` from the MCP server.

## Domains

| Domain                                | Tools | Meta-tool                                                                              | Document                                                 |
| ------------------------------------- | ----: | -------------------------------------------------------------------------------------- | -------------------------------------------------------- |
| Projects                              |    91 | `gitlab_project`                                                                       | [projects.md](projects.md)                               |
| Repository & Files                    |    40 | `gitlab_repository`                                                                    | [repository.md](repository.md)                           |
| Branches                              |    10 | `gitlab_branch`                                                                        | [branches.md](branches.md)                               |
| Tags                                  |     9 | `gitlab_tag`                                                                           | [tags.md](tags.md)                                       |
| Merge Requests                        |    56 | `gitlab_merge_request`                                                                 | [merge-requests.md](merge-requests.md)                   |
| MR Review                             |    23 | `gitlab_mr_review`                                                                     | [mr-review.md](mr-review.md)                             |
| Issues                                |    56 | `gitlab_issue`                                                                         | [issues.md](issues.md)                                   |
| CI/CD                                 |    65 | `gitlab_pipeline`, `gitlab_job`, etc.                                                  | [ci-cd.md](ci-cd.md)                                     |
| Releases                              |    12 | `gitlab_release`                                                                       | [releases.md](releases.md)                               |
| Environments & Deployments            |    23 | `gitlab_environment`                                                                   | [environments.md](environments.md)                       |
| Groups                                |   104 | `gitlab_group`                                                                         | [groups.md](groups.md)                                   |
| Users & Todos                         |    64 | `gitlab_user`                                                                          | [users.md](users.md)                                     |
| Access & Tokens                       |    62 | various                                                                                | [access.md](access.md)                                   |
| Boards, Labels & Milestones           |    25 | `gitlab_project`, `gitlab_group`                                                       | [boards.md](boards.md)                                   |
| Search                                |    10 | `gitlab_search`                                                                        | [search.md](search.md)                                   |
| Orbit (GitLab.com Enterprise/Premium) |     6 | `gitlab_orbit`                                                                         | [orbit.md](orbit.md)                                     |
| Wikis                                 |     6 | `gitlab_wiki`                                                                          | [wikis.md](wikis.md)                                     |
| Snippets                              |    26 | `gitlab_snippet`                                                                       | [snippets.md](snippets.md)                               |
| Packages & Registry                   |    33 | `gitlab_package`                                                                       | [packages.md](packages.md)                               |
| Mirrors                               |     7 | `gitlab_project` (enterprise routes)                                                   | [mirrors.md](mirrors.md)                                 |
| Dependency Firewall                   |     1 | `gitlab_project` (enterprise routes)                                                   | [dependency-firewall.md](dependency-firewall.md)         |
| Runners & Resource Groups             |    34 | `gitlab_runner`                                                                        | [runners.md](runners.md)                                 |
| Security & Feature Flags              |    28 | various                                                                                | [security.md](security.md)                               |
| Notifications & Events                |    42 | various                                                                                | [notifications.md](notifications.md)                     |
| Admin & Instance                      |    75 | `gitlab_admin`                                                                         | [admin.md](admin.md)                                     |
| Templates                             |    12 | `gitlab_template`                                                                      | [templates.md](templates.md)                             |
| Integrations & Misc                   |    29 | various                                                                                | [integrations.md](integrations.md)                       |
| MCP Capabilities                      |     5 | `gitlab_server` (plus individually-registered elicitation tools)                       | [capabilities.md](capabilities.md)                       |
| Project Discovery                     |     1 | `gitlab_discover_project` (individual tool)                                            | [project-discovery.md](project-discovery.md)             |
| Identity & Security                   |    30 | `gitlab_group_scim`, `gitlab_member_role`, etc.                                        | [identity-security.md](identity-security.md)             |
| Enterprise Users & Attestations       |     6 | `gitlab_enterprise_user`, `gitlab_attestation`                                         | [enterprise-attestations.md](enterprise-attestations.md) |
| Analytics & Compliance                |    12 | `gitlab_group` (enterprise routes), `gitlab_compliance_policy`, `gitlab_project_alias` | [analytics-compliance.md](analytics-compliance.md)       |
| Geo & Model Registry                  |     9 | `gitlab_geo`, `gitlab_model_registry`                                                  | [geo-model-registry.md](geo-model-registry.md)           |
| Repository Storage Moves              |    18 | `gitlab_storage_move`                                                                  | [storage-moves.md](storage-moves.md)                     |
| Epics                                 |    23 | `gitlab_group` (enterprise routes)                                                     | [epics.md](epics.md)                                     |
| Vulnerabilities                       |     8 | `gitlab_vulnerability`                                                                 | [vulnerabilities.md](vulnerabilities.md)                 |
| Security Attributes                   |     5 | `gitlab_security_attribute`                                                            | [security-attributes.md](security-attributes.md)         |
| Security Categories                   |     3 | `gitlab_security_category`                                                             | [security-categories.md](security-categories.md)         |
| Security Findings                     |     1 | `gitlab_security_finding`                                                              | [security-findings.md](security-findings.md)             |
| Security Scan Profiles                |     3 | `gitlab_security_scan_profile`                                                         | [security-scan-profiles.md](security-scan-profiles.md)   |
| CI/CD Catalog                         |     2 | `gitlab_ci_catalog`                                                                    | [ci-catalog.md](ci-catalog.md)                           |
| Branch Rules                          |     1 | `gitlab_branch` (routed)                                                               | [branch-rules.md](branch-rules.md)                       |
| Custom Emoji                          |     3 | `gitlab_custom_emoji`                                                                  | [custom-emoji.md](custom-emoji.md)                       |
| Achievements                          |    12 | `gitlab_achievement`                                                                   | [achievements.md](achievements.md)                       |

> **Note**: The `events` sub-package (2 tools: `gitlab_project_event_list`, `gitlab_user_contribution_event_list`) is registered into the `gitlab_user` catalog group and documented in `users.md` (DOC-002 first-claimer-wins rule). Five sub-packages (`projectimportexport`, `projectstatistics`, `uploads`, `projectserviceaccounts`, `deploymentmergerequests` — 20 tools) are covered by their parent domain docs (Projects, Environments). Orbit is registered only for `https://gitlab.com` connections with the Enterprise/Premium catalog enabled. Nine GraphQL-only domains (Vulnerabilities, Security Attributes, Security Categories, Security Findings, Security Scan Profiles, CI/CD Catalog, Branch Rules, Custom Emoji, Achievements) use the GitLab GraphQL API instead of REST — see [GraphQL Integration](../../concepts/graphql.md) for details.

## Response Format

All tool responses include **dual output**:

- **Markdown content** with tables, clickable `[text](url)` links, formatted dates, and `💡 Next steps` hints — targeted at the LLM via `audience: ["assistant"]` annotations
- **Structured JSON** (`structuredContent`) with typed fields — for meta-tools, this also includes a `next_steps` array with actionable hints for JSON-only clients like VS Code

14 domains include **clickable links** in list results: merge requests, issues, pipelines, projects, branches, commits, releases, tags, todos, milestones, members, environments, groups, and packages. Clicking a link opens the entity directly in GitLab.

See [Output Format](../output-format.md) for details on annotations, priorities, and response anatomy.
