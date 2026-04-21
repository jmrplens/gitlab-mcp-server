# Tool Documentation

Per-domain tool reference for gitlab-mcp-server. Each document covers one logical domain, listing all individual MCP tools with their descriptions, parameter tables, and annotation types.

## Domains

| Domain | Tools | Meta-tool | Document |
| --- | ---: | --- | --- |
| Projects | 42 | `gitlab_project` | [projects.md](projects.md) |
| Repository & Files | 41 | `gitlab_repository` | [repository.md](repository.md) |
| Branches | 10 | `gitlab_branch` | [branches.md](branches.md) |
| Tags | 9 | `gitlab_tag` | [tags.md](tags.md) |
| Merge Requests | 54 | `gitlab_merge_request` | [merge-requests.md](merge-requests.md) |
| MR Review | 23 | `gitlab_mr_review` | [mr-review.md](mr-review.md) |
| Issues | 44 | `gitlab_issue` | [issues.md](issues.md) |
| CI/CD | 58 | `gitlab_pipeline`, `gitlab_job`, etc. | [ci-cd.md](ci-cd.md) |
| Releases | 12 | `gitlab_release` | [releases.md](releases.md) |
| Environments & Deployments | 24 | `gitlab_environment`, `gitlab_deployment` | [environments.md](environments.md) |
| Groups | 69 | `gitlab_group` | [groups.md](groups.md) |
| Users & Todos | 27 | `gitlab_user` | [users.md](users.md) |
| Access & Tokens | 68 | various | [access.md](access.md) |
| Boards, Labels & Milestones | 26 | `gitlab_project`, `gitlab_group` | [boards.md](boards.md) |
| Search | 11 | `gitlab_search` | [search.md](search.md) |
| Wikis | 6 | `gitlab_wiki` | [wikis.md](wikis.md) |
| Snippets | 24 | `gitlab_snippet` | [snippets.md](snippets.md) |
| Packages & Registry | 28 | `gitlab_package` | [packages.md](packages.md) |
| Mirrors | 7 | `gitlab_project` (enterprise routes) | [mirrors.md](mirrors.md) |
| Runners & Resource Groups | 24 | `gitlab_runner` | [runners.md](runners.md) |
| Security & Feature Flags | 28 | various | [security.md](security.md) |
| Notifications & Events | 48 | various | [notifications.md](notifications.md) |
| Admin & Instance | 77 | `gitlab_admin` | [admin.md](admin.md) |
| Templates | 10 | `gitlab_template` | [templates.md](templates.md) |
| Integrations & Misc | 34 | various | [integrations.md](integrations.md) |
| MCP Capabilities | 16 | — | [capabilities.md](capabilities.md) |
| Project Discovery | 1 | — | [project-discovery.md](project-discovery.md) |
| Identity & Security | 28 | `gitlab_group_scim`, `gitlab_member_role`, etc. | [identity-security.md](identity-security.md) |
| Enterprise Users & Attestations | 6 | `gitlab_enterprise_user`, `gitlab_attestation` | [enterprise-attestations.md](enterprise-attestations.md) |
| Analytics & Compliance | 12 | `gitlab_group` (enterprise routes), `gitlab_compliance_policy`, `gitlab_project_alias` | [analytics-compliance.md](analytics-compliance.md) |
| Geo & Model Registry | 9 | `gitlab_geo`, `gitlab_model_registry` | [geo-model-registry.md](geo-model-registry.md) |
| Repository Storage Moves | 18 | `gitlab_storage_move` | [storage-moves.md](storage-moves.md) |
| Epics | 17 | `gitlab_epic` | [epics.md](epics.md) |
| Vulnerabilities | 8 | `gitlab_vulnerability` | [vulnerabilities.md](vulnerabilities.md) |
| Security Findings | 1 | `gitlab_security` | [security-findings.md](security-findings.md) |
| CI/CD Catalog | 2 | `gitlab_ci_catalog` | [ci-catalog.md](ci-catalog.md) |
| Branch Rules | 1 | `gitlab_branch` (routed) | [branch-rules.md](branch-rules.md) |
| Custom Emoji | 3 | `gitlab_custom_emoji` | [custom-emoji.md](custom-emoji.md) |

> **Note**: The `events` sub-package (3 tools) is referenced by both Users & Todos and Notifications & Events domains. Four sub-packages (`projectimportexport`, `projectstatistics`, `uploads`, `deploymentmergerequests` — 12 tools) are covered by their parent domain docs (Projects, Environments). Five GraphQL-only domains (Vulnerabilities, Security Findings, CI/CD Catalog, Branch Rules, Custom Emoji) use the GitLab GraphQL API instead of REST — see [GraphQL Integration](../graphql.md) for details.

## Response Format

All tool responses include **dual output**:

- **Markdown content** with tables, clickable `[text](url)` links, formatted dates, and `💡 Next steps` hints — targeted at the LLM via `audience: ["assistant"]` annotations
- **Structured JSON** (`structuredContent`) with typed fields — for meta-tools, this also includes a `next_steps` array with actionable hints for JSON-only clients like VS Code

14 domains include **clickable links** in list results: merge requests, issues, pipelines, projects, branches, commits, releases, tags, todos, milestones, members, environments, groups, and packages. Clicking a link opens the entity directly in GitLab.

See [Output Format](../output-format.md) for details on annotations, priorities, and response anatomy.
