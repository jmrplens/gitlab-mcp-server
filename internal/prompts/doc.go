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
package prompts
