// Package jobs implements MCP tools for GitLab CI/CD job operations.
//
// It supports listing jobs, retrieving job details, downloading trace logs,
// canceling and retrying jobs, and waiting for a job to reach a terminal state.
// The wait tool emits MCP progress notifications during polling. The package
// wraps the GitLab Jobs service from client-go v2 and provides Markdown
// rendering for job responses.
//
// # Runtime Behavior
//
// Trace and artifact-related actions can return large payloads, so handlers use
// focused output structs and Markdown summaries instead of expanding every raw
// field. Wait operations honor context cancellation and report progress through
// the MCP progress capability.
//
// # GitLab API References
//
// The package wraps the GitLab Jobs API:
//
//   - https://docs.gitlab.com/api/jobs/
package jobs
