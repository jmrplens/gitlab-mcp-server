// Package metadata implements MCP tools for the GitLab Metadata API.
//
// The package fetches server version, revision, Enterprise edition status, and
// Kubernetes Agent Server metadata, then renders those values as structured JSON
// and Markdown for MCP tool responses.
//
// The package wraps the GitLab Metadata API:
//
//   - https://docs.gitlab.com/api/metadata/
package metadata
