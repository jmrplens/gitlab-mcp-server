// Package securityscanprofiles implements MCP tools for GitLab security scan
// profiles through the GraphQL API.
//
// Security scan profiles bundle a security scanning configuration (for example
// dependency-scanning post-processing) that can be attached to, or detached
// from, projects and groups. The package attaches and detaches profiles,
// lists the per-project scan profile statuses, and renders the results as
// Markdown.
//
// The package wraps the GitLab GraphQL security scan profile mutations and
// queries:
//
//   - https://docs.gitlab.com/api/graphql/reference/#mutationsecurityscanprofileattach
//   - https://docs.gitlab.com/api/graphql/reference/#mutationsecurityscanprofiledetach
//   - https://docs.gitlab.com/api/graphql/reference/#project-scanprofilestatuses
package securityscanprofiles
