// Package issues implements MCP tools for GitLab issue lifecycle operations.
//
// The package covers project issue CRUD, listing, filtering, assignee and
// author queries, confidentiality, labels, milestones, due dates, state
// changes, and issue links to related tool domains such as notes,
// discussions, statistics, and work items. Handlers return typed MCP output
// structs and publish Markdown formatters for individual and meta-tool
// responses.
//
// # GitLab API References
//
// The primary API backing this package is:
//
//   - https://docs.gitlab.com/api/issues/
package issues
