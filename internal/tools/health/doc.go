// Package health implements MCP tools for GitLab server and MCP server health
// checks.
//
// The package wraps the GitLab metadata read and the current-user read used
// to verify connectivity and authentication. It asks GET /metadata rather
// than GET /version, which serves the same answer and GitLab has deprecated
// since 15.5:
//
//   - https://docs.gitlab.com/api/metadata/
//   - https://docs.gitlab.com/api/users/
package health
