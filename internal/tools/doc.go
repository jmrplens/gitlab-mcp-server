// Package tools provides the MCP tool orchestration layer for the GitLab MCP
// server.
//
// The package wires individual GitLab MCP tools and domain-scoped meta-tools to
// the server, delegates domain implementations to internal/tools/{domain}
// sub-packages, exposes the gitlab_server meta-tool, applies read-only and safe
// mode behavior, filters tools by personal access token scopes, and delegates
// meta-tool Markdown rendering to the type-based registry in internal/toolutil.
//
// # Architecture
//
// The high-level registration flow is:
//
//	cmd/server
//	    |
//	    v
//	RegisterAll and RegisterAllMeta
//	    |
//	    v
//	internal/tools/{domain}
//	    |
//	    v
//	GitLab REST and GraphQL APIs
//
// [RegisterAll] registers the individual tools. [RegisterAllMeta] registers the
// domain-scoped meta-tools that dispatch to action maps. [SafeModePreview]
// describes the preview payload returned when safe mode intercepts mutating
// calls.
package tools
