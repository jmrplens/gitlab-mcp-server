// Package mergerequests implements MCP tools for GitLab merge request
// operations.
//
// The package covers merge request create, get, list, update, merge, rebase,
// close, reopen, reviewer and assignee metadata, branch relationships, squash
// and source-branch cleanup flags, and workflow helpers used by catalog-backed
// meta and dynamic surfaces. Related domains provide approvals, changes,
// context commits, discussions, draft notes, notes, and merge trains.
//
// # GitLab API References
//
// The primary API backing this package is:
//
//   - https://docs.gitlab.com/api/merge_requests/
package mergerequests
