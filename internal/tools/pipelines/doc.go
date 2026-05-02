// Package pipelines implements MCP tools for GitLab pipeline operations.
//
// It supports listing, retrieving, creating, canceling, retrying, deleting, and
// waiting for pipelines. The wait tool polls server-side and emits MCP progress
// notifications while a pipeline moves toward a terminal state. The package
// wraps the GitLab Pipelines service from client-go v2 and provides Markdown
// rendering for pipeline responses.
package pipelines
