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
// The latest pipeline of a ref is GitLab's answer for the commit at its head,
// and GitLab answers 403 both when that commit has no pipeline and when the
// caller may not read pipelines. [GetLatest] answers the first from the
// pipeline list of the same ref, saying beside the pipeline that it ran for an
// earlier commit, and lets the list's own refusal report the second.
//
// # GitLab API References
//
// The package wraps the GitLab Pipelines API:
//
//   - https://docs.gitlab.com/api/pipelines/
package pipelines
