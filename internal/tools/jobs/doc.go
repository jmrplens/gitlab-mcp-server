// Package jobs implements MCP tools for GitLab CI/CD job operations.
//
// It supports listing jobs, retrieving job details, downloading trace logs,
// canceling and retrying jobs, and waiting for a job to reach a terminal state.
// The wait tool emits MCP progress notifications during polling. The package
// wraps the GitLab Jobs service from client-go v2 and provides Markdown
// rendering for job responses.
package jobs
