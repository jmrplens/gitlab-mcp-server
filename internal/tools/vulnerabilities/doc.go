// Package vulnerabilities implements MCP tools for GitLab vulnerability
// management through the GraphQL API.
//
// The package lists project vulnerabilities, retrieves individual vulnerability
// details, computes severity and pipeline security summaries, mutates
// vulnerability state through dismiss, confirm, resolve, and revert operations,
// and renders vulnerability outputs as Markdown.
//
// The package wraps the GitLab Vulnerabilities API and GraphQL vulnerability
// fields:
//
//   - https://docs.gitlab.com/api/vulnerabilities/
//   - https://docs.gitlab.com/api/graphql/reference/
package vulnerabilities
