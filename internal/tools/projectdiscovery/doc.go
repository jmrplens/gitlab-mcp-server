// Package projectdiscovery implements MCP tools that resolve Git remote URLs to
// GitLab project metadata.
//
// The package wraps the GitLab Projects API after extracting a project path from
// a complete HTTPS or SSH remote URL:
//
//   - https://docs.gitlab.com/api/projects/
package projectdiscovery
