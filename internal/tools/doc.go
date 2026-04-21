// Package tools provides the MCP tool orchestration layer for the GitLab MCP
// server. It contains markdown rendering, meta-tool infrastructure, and tool
// registration, delegating domain-specific implementations to 162 sub-packages
// under internal/tools/{domain}/. Shared utilities (errors, pagination, logging)
// live in internal/toolutil/.
package tools
