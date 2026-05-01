// Package tools provides the MCP tool orchestration layer for the GitLab MCP
// server.
//
// The package wires individual GitLab MCP tools and domain-scoped meta-tools to
// the server, delegates domain implementations to internal/tools/{domain}
// sub-packages, exposes the gitlab_server meta-tool, applies read-only and safe
// mode behavior, filters tools by personal access token scopes, and delegates
// meta-tool Markdown rendering to the type-based registry in internal/toolutil.
package tools
