// Package samplingtools provides MCP tools that use the MCP sampling capability
// for LLM-assisted analysis of GitLab issues, merge requests, pipelines,
// releases, milestones, CI configuration, security posture, technical debt, and
// deployment history.
//
// The package builds GitLab context from REST and GraphQL APIs, formats that
// context as Markdown for model analysis, invokes client-approved sampling, and
// returns typed outputs with Markdown formatters for individual tools and the
// gitlab_analyze meta-tool. GraphQL context builders aggregate related merge
// request, issue, pipeline, milestone, and deployment data into a single request
// when possible; callers fall back to REST when GraphQL is unavailable or the
// project identifier cannot be used as a full path.
package samplingtools
