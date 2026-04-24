---
title: "ADR-0005: Meta-tool consolidation from 70 to 27 domain tools"
status: "Accepted"
date: "2026-03-06"
authors: "jmrplens"
tags: ["architecture", "decision", "meta-tools", "consolidation", "llm-optimization"]
superseded_by: ""
---

# ADR-0005: Meta-tool consolidation from 70 to 27 domain tools

## Status

**Accepted** — refines ADR-0004 (modular tools sub-packages).

> **Update (2026-03-08)**: Phase 7 of the tool metadata audit refined the consolidation:
>
> - `gitlab_security` renamed to `gitlab_feature_flags` (only contains feature flag actions; secure-files, error-tracking, alert-management moved to `gitlab_admin`)
> - 28 routes redistributed: MR emoji/events → `gitlab_merge_request`, snippet emoji → `gitlab_snippet`, epic discussions → `gitlab_group`
> - Annotation differentiation: `gitlab_template` and `gitlab_search` now use `ReadOnlyMetaAnnotations`, `gitlab_user` uses `NonDestructiveMetaAnnotations`

## Context

ADR-0004 established modular domain sub-packages (now 117), each optionally exposing its own `RegisterMeta()` function. Over time the meta-tool count grew organically:

| Metric                          | Count |
| ------------------------------- | ----- |
| Inline meta-tools (register_meta.go) | 19    |
| Standalone RegisterMeta calls   | 49    |
| **Total meta-tools exposed**    | **68** |

### Problems with 68 meta-tools

1. **LLM token overhead**: MCP tool lists are sent as system context on every request. 68 meta-tools, each with full JSON Schema descriptions, consume significant token budget.
2. **Tool selection confusion**: LLMs must choose among many similar tools (e.g., `gitlab_group_members`, `gitlab_group_labels`, `gitlab_group_milestones` are separate from `gitlab_group`).
3. **Inconsistent granularity**: Some domains are deeply consolidated (e.g., `gitlab_project` with 55+ actions) while others are one-tool-per-package (e.g., `gitlab_avatar`, `gitlab_pages`, `gitlab_resource_events`).
4. **Client compatibility**: All MCP clients (VS Code Copilot, Cursor, Copilot CLI, OpenCode) render tool lists differently. Fewer, well-organized tools are universally better.

### Research: production MCP patterns

Analysis of production MCP servers reveals common patterns for managing large tool sets:

#### Pattern 1: Domain-scoped mega-tools (adopted)

- One tool per API domain with action dispatch
- Used by: Stripe MCP, GitHub MCP, Slack MCP
- Trade-off: Large description per tool, but minimal tool count
- LLM behavior: Models reliably select the right domain tool and compose action+params

#### Pattern 2: Dynamic tool registration (rejected)

- Server adjusts tool list based on context or user preferences
- Incompatible with standard MCP clients that cache `tools/list` at session start
- Would require `tools/list_changed` notifications, which not all clients support

#### Pattern 3: Tool-of-tools / catalog tool (rejected)

- A meta-meta-tool that returns available actions for a domain
- Adds an extra round-trip (discover actions, then call with action)
- LLMs already handle action dispatch well with good descriptions

#### Pattern 4: Lazy tool loading (rejected)

- Server starts with minimal tools, loads more on demand
- Requires client support for `tools/list_changed`
- VS Code Copilot and Cursor do not reliably re-fetch tool lists

**Conclusion**: Domain-scoped mega-tools (Pattern 1) is the most compatible and token-efficient approach. The consolidation target was **25 meta-tools**, with the final result being **27 meta-tools** (2 additional tools retained for operational separation: `gitlab_health` and `gitlab_summarize_issue`). This keeps the tool list small enough for any LLM context window while covering all 1005 individual tools.

## Decision

**Consolidate 70 meta-tools into 27 domain meta-tools** by absorbing standalone `RegisterMeta` calls into the existing inline meta-tool registration functions.

### Target architecture

```text
25 meta-tools:
├── gitlab_project        # projects + uploads + import/export + statistics + templates + hooks
├── gitlab_repository     # repository + submodules + commits + commit-discussions + files + markdown
├── gitlab_branch         # branches + protected branches
├── gitlab_tag            # tags + protected tags
├── gitlab_merge_request  # MR CRUD + approvals + context-commits + time-tracking
├── gitlab_mr_review      # MR notes + discussions + drafts + changes + diff-versions
├── gitlab_release        # releases + release links
├── gitlab_issue          # issues + notes + discussions + links + statistics + work-items + time-tracking
├── gitlab_pipeline       # pipelines + triggers
├── gitlab_job            # jobs + artifacts + bridges + token-scope + resource-groups
├── gitlab_ci             # CI variables + lint + YAML templates + schedules + instance/group variables
├── gitlab_group          # groups + members + labels + milestones + boards + variables + import/export + badges + relations-export + markdown-uploads
├── gitlab_environment    # environments + deployments + deploy-MRs + protected-envs + freeze-periods
├── gitlab_user           # users + status + SSH-keys + emails + todos + events + keys + namespaces
├── gitlab_search         # global/project/group search + code search
├── gitlab_wiki           # project + group wikis
├── gitlab_package        # packages + container-registry
├── gitlab_snippet        # snippets + snippet-discussions + epic-discussions
├── gitlab_runner         # runners + resource-groups
├── gitlab_access         # access-tokens + deploy-tokens + deploy-keys + access-requests + invites
├── gitlab_notification   # notifications + events + resource-events + award-emoji
├── gitlab_security       # feature-flags + user-lists + secure-files + error-tracking + alert-management
├── gitlab_admin          # settings + appearance + broadcasts + features + license + system-hooks + sidekiq + plan-limits + usage-data + db-migrations + applications + metadata + custom-attrs + bulk-imports + avatar + dependency-proxy + pages + terraform-states + cluster-agents
├── gitlab_board          # project-boards + group-boards
├── gitlab_health         # server health/version check
```

### Consolidation mapping (49 standalone → absorbed)

| Standalone package        | Target meta-tool      | Action prefix |
| ------------------------- | --------------------- | ------------- |
| accessrequests            | gitlab_access         | access_request_* |
| accesstokens              | gitlab_access         | token_* |
| alertmanagement           | gitlab_admin → gitlab_security | alert_* |
| avatar                    | gitlab_admin          | avatar_* |
| awardemoji                | gitlab_notification   | emoji_* |
| boards                    | gitlab_board          | board_* |
| clusteragents             | gitlab_admin          | cluster_agent_* |
| commitdiscussions         | gitlab_repository     | commit_discussion_* |
| containerregistry         | gitlab_package        | registry_* |
| dependencyproxy           | gitlab_admin          | dependency_proxy_* |
| deploykeys                | gitlab_access         | deploy_key_* |
| deploytokens              | gitlab_access         | deploy_token_* |
| epicdiscussions           | gitlab_snippet        | epic_discussion_* |
| errortracking             | gitlab_security       | error_tracking_* |
| events                    | gitlab_notification   | event_* |
| featureflags              | gitlab_security       | feature_flag_* |
| ffuserlists               | gitlab_security       | user_list_* |
| freezeperiods             | gitlab_environment    | freeze_period_* |
| groupboards               | gitlab_board          | group_board_* |
| groupimportexport         | gitlab_group          | import_*, export_* |
| grouplabels               | gitlab_group          | label_* |
| groupmarkdownuploads      | gitlab_group          | upload_* |
| groupmembers              | gitlab_group          | member_* |
| groupmilestones           | gitlab_group          | milestone_* |
| grouprelationsexport      | gitlab_group          | relations_export_* |
| groupvariables            | gitlab_ci             | group_variable_* |
| importservice             | gitlab_admin          | import_* |
| instancevariables         | gitlab_ci             | instance_variable_* |
| invites                   | gitlab_access         | invite_* |
| issuediscussions          | gitlab_issue          | discussion_* |
| issuestatistics           | gitlab_issue          | statistics_* |
| jobtokenscope             | gitlab_job            | token_scope_* |
| keys                      | gitlab_user           | key_* |
| namespaces                | gitlab_user           | namespace_* |
| notifications             | gitlab_notification   | notification_* |
| packages                  | gitlab_package        | package_* |
| pages                     | gitlab_admin          | pages_* |
| pipelinetriggers          | gitlab_pipeline       | trigger_* |
| projectimportexport       | gitlab_project        | import_*, export_* |
| projectstatistics         | gitlab_project        | statistics_* |
| protectedenvs             | gitlab_environment    | protected_env_* |
| resourceevents            | gitlab_notification   | resource_event_* |
| resourcegroups            | gitlab_job            | resource_group_* |
| runners                   | gitlab_runner         | runner_* |
| search                    | gitlab_search         | (already standalone) |
| securefiles               | gitlab_security       | secure_file_* |
| snippetdiscussions        | gitlab_snippet        | snippet_discussion_* |
| snippets                  | gitlab_snippet        | snippet_* |
| terraformstates           | gitlab_admin          | terraform_* |

### Implementation approach

1. **Phase-by-phase migration**: Each domain phase absorbs its standalone packages into the target inline meta-tool
2. **Backward compatibility**: Individual tools remain unchanged (RegisterAll). Only meta-mode changes.
3. **Action naming**: Absorbed actions use `{subdomain}_{verb}` prefix to avoid collisions (e.g., `member_list`, `label_create`)
4. **Description enhancement**: Each consolidated meta-tool gets comprehensive action documentation in its tool description
5. **No code deletion**: Sub-package `RegisterMeta()` functions remain but stop being called from `RegisterAllMeta()`

## Consequences

### Positive

- **POS-001**: Token reduction — from 70 to 27 meta-tools reduces `tools/list` response by ~61%
- **POS-002**: Simpler tool selection — LLMs only choose among 27 domain tools instead of 70
- **POS-003**: Better discoverability — comprehensive action lists in tool descriptions
- **POS-004**: Consistent granularity — every domain has exactly one meta-tool
- **POS-005**: Universal client compatibility — fewer tools work better across all MCP clients

### Negative

- **NEG-001**: Larger tool descriptions — mega-tools have long action lists in their descriptions
- **NEG-002**: Action name collisions — must be careful with prefix naming in consolidated tools
- **NEG-003**: Migration effort — requires updating `register_meta.go` and testing all routes

### Risks mitigated

- **No individual tool changes**: `RegisterAll()` path is unaffected
- **Phased rollout**: One domain at a time, verified by compilation and tests
- **Action naming convention**: `{subdomain}_{verb}` prevents collisions
- **E2E validation**: Meta-tool workflow test covers all routes

## Validation

After consolidation:

- `go build ./...` — clean
- `go test ./internal/... -count=1` — all packages pass
- `META_TOOLS=true` exposes exactly 27 meta-tools
- `META_TOOLS=false` exposes 1005 individual tools (unchanged)
- E2E meta-tool workflow covers all consolidated routes
