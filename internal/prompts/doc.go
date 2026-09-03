// Package prompts registers MCP prompt templates that generate AI-optimized
// summaries, reviews, reports, and assessments from GitLab project, group, and
// cross-project data.
//
// The package includes project prompts for merge requests, pipelines, branches,
// release notes, and health checks; cross-project prompts that aggregate global
// Merge Requests and Issues API data; team, analytics, audit, milestone, label,
// and project-report prompts; and shared helper functions used by those prompt
// handlers.
//
// # Prompt Families
//
// The prompt catalog is organized around common GitLab workflows:
//
//   - Code review, merge request risk, and reviewer suggestions.
//   - Release notes, project health, pipeline status, and branch cleanup.
//   - Cross-project issue and merge request triage.
//   - Team activity, analytics, audit, milestone, and label reports.
//
// [Register] adds every prompt template to an MCP server. Prompt handlers use
// shared helpers in this package to keep GitLab API access, pagination, and
// Markdown assembly consistent across prompt families.
//
// # Narrowing
//
// A prompt is a third request path to data a tool also returns, running handler
// code with the same credential, so the operator's --exclude-tools must reach
// it: [RegisterOptions] carries the excluded catalog actions and
// promptBackingActions is the table relating the two surfaces. This mirrors the
// resource surface exactly, and for the same reason: exclusion is the
// recommended mitigation for a tool, and a mitigation that covers one of three
// paths is not one.
//
// One control still does not reach this surface, because it is applied where
// tools are registered rather than here: the tools/call rate limiter does not
// meter prompts/get, so a prompt remains an unmetered proxy to GitLab. It is
// noted so the silence does not read as coverage.
package prompts
