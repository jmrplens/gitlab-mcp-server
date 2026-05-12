// Package tools provides the MCP tool orchestration layer for the GitLab MCP
// server.
//
// The package wires the individual, meta, and dynamic GitLab MCP tool surfaces
// to the server. It delegates domain implementations to internal/tools/{domain}
// sub-packages, builds the canonical action catalog for catalog-backed
// surfaces, exposes the gitlab_server meta-tool, applies read-only and safe mode
// behavior, filters tools by personal access token scopes, and delegates
// meta-tool Markdown rendering to the type-based registry in internal/toolutil.
//
// # Architecture
//
// The high-level registration flow is:
//
//	cmd/server
//	    |
//	    +--> RegisterAll --> internal/tools/{domain}.RegisterTools
//	    |
//	    +--> BuildActionCatalog --> RegisterMetaCatalog
//	    |
//	    +--> BuildActionCatalog --> dynamic.RegisterCatalogTools
//
// [RegisterAll] registers the individual tools directly. [BuildActionCatalog]
// builds the canonical action catalog used by [RegisterMetaCatalog] and dynamic
// mode. [RegisterAllMeta] preserves the legacy meta registration entry point by
// building and registering that catalog. [SafeModePreview] describes the preview
// payload returned when safe mode intercepts mutating calls.
package tools
