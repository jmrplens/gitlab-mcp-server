// Package pipelines implements MCP tools for GitLab pipeline operations.
//
// It supports listing, retrieving, creating, canceling, retrying, deleting, and
// waiting for pipelines. The wait tool polls server-side and emits MCP progress
// notifications while a pipeline moves toward a terminal state. The package
// wraps the GitLab Pipelines service from client-go v2 and provides Markdown
// rendering for pipeline responses.
//
// # Runtime Behavior
//
// Wait operations respect context cancellation and keep long-running polling
// visible through MCP progress notifications. Mutating operations are annotated
// through ActionSpecs so read-only, safe mode, and destructive confirmation
// behavior stay consistent across individual, meta, and dynamic surfaces.
//
// # GitLab API References
//
// The package wraps the GitLab Pipelines API:
//
//   - https://docs.gitlab.com/api/pipelines/
package pipelines
