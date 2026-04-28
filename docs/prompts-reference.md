# MCP Prompts Reference

This document lists all **38 MCP prompts** exposed by gitlab-mcp-server. Prompts are AI-optimized templates that generate structured summaries, reviews, and assessments from GitLab project data.

> **Diátaxis type**: Reference
> **Audience**: MCP client developers, AI assistant users
> **Prerequisites**: Understanding of MCP prompts concept

---

## Core Prompts (12)

These prompts are registered directly in `internal/prompts/prompts.go`.

### Merge Request Analysis

| # | Name | Arguments | Description |
|---|------|-----------|-------------|
| 1 | `summarize_mr_changes` | `project_id`*, `merge_request_iid`* | Summarize the changed files and key modifications in a merge request. Lists each file with its change type (new/modified/deleted/renamed). |
| 2 | `review_mr` | `project_id`*, `merge_request_iid`* | Generate a structured code review for a merge request. Files are categorized by risk (high-risk, business logic, tests, documentation) with per-file metrics and a review plan. Full diffs included. |
| 3 | `suggest_mr_reviewers` | `project_id`*, `merge_request_iid`* | Suggest suitable merge request reviewers based on the files changed and active project members. Excludes the MR author. |
| 4 | `mr_risk_assessment` | `project_id`*, `merge_request_iid`* | Assess the risk level (LOW/MEDIUM/HIGH/CRITICAL) of a merge request based on size, changed files, sensitive patterns, and conflict status. |

### Project Overview

| # | Name | Arguments | Description |
|---|------|-----------|-------------|
| 5 | `summarize_pipeline_status` | `project_id`* | Summarize the latest CI/CD pipeline status. Groups jobs by outcome (failed/passed/other) and includes failure reasons for debugging. |
| 6 | `summarize_open_mrs` | `project_id`* | Summarize all open merge requests including title, author, branches, age in days, and merge status. Highlights stale MRs (>7 days). |
| 7 | `project_health_check` | `project_id`* | Comprehensive project health assessment combining latest pipeline status, open MRs, and branch hygiene (merged/stale branch counts). |
| 8 | `generate_release_notes` | `project_id`*, `from`*, `to` | Generate release notes from commits and file changes between two Git refs (tags, branches, or SHAs). |
| 9 | `compare_branches` | `project_id`*, `from`*, `to`* | Compare two Git branches or refs showing commit differences and file changes between them. |

### Personal Productivity

| # | Name | Arguments | Description |
|---|------|-----------|-------------|
| 10 | `daily_standup` | `project_id`*, `username` | Generate a daily standup summary based on the user's GitLab activity in the last 24 hours. Includes done/planned/blockers sections. |
| 11 | `team_member_workload` | `project_id`*, `username`*, `days` | Generate a comprehensive workload summary for a specific team member over a configurable time period. |
| 12 | `user_stats` | `project_id`*, `username`, `days` | Generate comprehensive user statistics: contribution events, MR stats, issue stats, daily activity trends, and Mermaid activity chart. |

> `*` = required argument

## Cross-Project Prompts (4)

Personal dashboard prompts that aggregate across all projects. Registered in `internal/prompts/prompt_cross_project.go`.

| # | Name | Arguments | Description |
|---|------|-----------|-------------|
| 13 | `my_open_mrs` | — | Show all open MRs where you are author or assignee across all projects. Grouped by project. |
| 14 | `my_pending_reviews` | — | Show all open MRs where you are assigned as reviewer across all projects. Grouped by project. |
| 15 | `my_issues` | — | Show all issues assigned to you across all projects. Includes overdue detection and project grouping. |
| 16 | `my_activity_summary` | `days` | Generate a personal activity summary for a configurable time period across all projects. Includes daily activity chart. |

## Team Management Prompts (4)

Group-level team management prompts. Registered in `internal/prompts/prompt_team.go`.

| # | Name | Arguments | Description |
|---|------|-----------|-------------|
| 17 | `user_activity_report` | `group_id`*, `username`*, `days` | Generate a detailed activity report for a specific user. Designed for managers to review team member productivity. |
| 18 | `team_overview` | `group_id`* | Generate a team dashboard showing all group members with their open MR counts and recently merged MRs. Includes workload pie chart. |
| 19 | `team_mr_dashboard` | `group_id`*, `state`, `target_branch` | List all merge requests for a GitLab group with optional state and target branch filters. Grouped by project. |
| 20 | `reviewer_workload` | `group_id`* | Analyze review distribution across group members. Shows how many open MRs each member is reviewing and identifies imbalances. |

## Project Report Prompts (5)

Project-level analysis prompts. Registered in `internal/prompts/prompt_project_reports.go`.

| # | Name | Arguments | Description |
|---|------|-----------|-------------|
| 21 | `branch_mr_summary` | `project_id`*, `target_branch`* | List all MRs targeting a specific branch. Shows readiness summary with conflict/draft/approval counts. |
| 22 | `project_activity_report` | `project_id`*, `days` | Generate a project activity report including recent events, merged MRs, and open issues. Shows daily activity chart. |
| 23 | `mr_review_status` | `project_id`* | Analyze discussion health of open MRs. Shows unresolved thread counts per MR to identify items needing attention. |
| 24 | `unassigned_items` | `project_id`* | Find open MRs and issues that have no assignee. Helps identify ownership gaps. |
| 25 | `stale_items_report` | `project_id`*, `stale_days` | Find MRs and issues that haven't been updated for a configurable number of days. Default: 14 days. |

## Analytics Prompts (4)

Velocity and release analytics prompts. Registered in `internal/prompts/prompt_analytics.go`.

| # | Name | Arguments | Description |
|---|------|-----------|-------------|
| 26 | `merge_velocity` | `project_id`*, `days` | Analyze MR throughput metrics. Shows merge rate, average time-to-merge, and daily merged count chart. |
| 27 | `release_readiness` | `project_id`*, `branch` | Check readiness of a release branch by analyzing open MRs targeting it, draft/conflict counts, and unresolved discussion threads. |
| 28 | `release_cadence` | `project_id`* | Analyze release frequency. Shows time between releases, average cadence, and release history chart. |
| 29 | `weekly_team_recap` | `group_id`*, `days` | Generate a comprehensive weekly recap for a team. Combines merged MRs, open MRs, issues activity, and events into a single summary. |

## Milestone & Label Prompts (4)

Milestone tracking, label analysis, and contributor ranking. Registered in `internal/prompts/prompt_milestone_label.go`.

| # | Name | Arguments | Description |
|---|------|-----------|-------------|
| 30 | `milestone_progress` | `project_id`*, `milestone` | Track milestone progress for a project. Shows issue/MR completion, progress bar, and due date risk. Omit milestone for all active milestones. |
| 31 | `label_distribution` | `project_id`* | Analyze label usage distribution. Shows open/closed issue counts and open MR counts per label. |
| 32 | `group_milestone_progress` | `group_id`* | Track milestone progress across all projects in a group. Shows issue/MR completion per milestone with progress bars. |
| 33 | `project_contributors` | `project_id`* | Rank project contributors by commits, additions, and deletions using the repository contributors API. |

## Project Audit Prompts (5)

Project configuration audit prompts. Registered in `internal/prompts/prompt_audit.go`.

| # | Name | Arguments | Description |
|---|------|-----------|-------------|
| 34 | `audit_project_settings` | `project_id`* | Audit core project settings: visibility, merge strategy, CI/CD, push rules, feature toggles, and storage statistics. Identifies misconfigurations. |
| 35 | `audit_branch_protection` | `project_id`* | Audit branch protection rules: protected branches, push/merge access levels, code owner approvals. Checks if the default branch is protected. |
| 36 | `audit_project_access` | `project_id`* | Audit user access: members by access level, blocked/inactive accounts, elevated privileges, shared groups. Follows least-privilege principle. |
| 37 | `audit_project_workflow` | `project_id`* | Audit workflow configuration: labels (with description gaps), milestones (active/closed, due dates), issue and MR templates. |
| 38 | `audit_project_full` | `project_id`* | Comprehensive project audit combining settings, branch protection, access, labels, milestones, templates, webhooks, and push rules with a quick scorecard. |

## Common Arguments

| Argument | Type | Description |
|----------|------|-------------|
| `project_id` | string (required) | Project ID (numeric) or URL-encoded path (e.g. `group/project`) |
| `merge_request_iid` | string (required) | Merge request IID (project-scoped numeric ID, visible as `!N` in GitLab) |
| `group_id` | string (required) | Group ID (numeric) or URL-encoded path |
| `username` | string | GitLab username (defaults to authenticated user when omitted) |
| `days` | string | Number of days to look back (default varies by prompt: 7 or 30) |
| `from` / `to` | string | Git refs: tag name, branch name, or commit SHA |
| `branch` | string | Target branch name (default: `main`) |
| `target_branch` | string | Target branch for MR filtering |
| `state` | string | MR/issue state filter (default: `opened`) |
| `milestone` | string | Specific milestone title |
| `stale_days` | string | Days without update to consider stale (default: 14) |

## Autocomplete Support

All prompt arguments support intelligent autocomplete via the completions handler (`internal/completions/`). When a client sends a `completion/complete` request for a prompt argument, the server queries GitLab to suggest matching values.

## Source Files

| File | Prompts |
|------|---------|
| [`prompts.go`](../internal/prompts/prompts.go) | 12 core prompts |
| [`prompt_cross_project.go`](../internal/prompts/prompt_cross_project.go) | 4 cross-project prompts |
| [`prompt_team.go`](../internal/prompts/prompt_team.go) | 4 team management prompts |
| [`prompt_project_reports.go`](../internal/prompts/prompt_project_reports.go) | 5 project report prompts |
| [`prompt_analytics.go`](../internal/prompts/prompt_analytics.go) | 4 analytics prompts (incl. `weekly_team_recap`) |
| [`prompt_milestone_label.go`](../internal/prompts/prompt_milestone_label.go) | 4 milestone & label prompts |
| [`prompt_audit.go`](../internal/prompts/prompt_audit.go) | 5 project audit prompts |
