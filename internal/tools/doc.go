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
//	    +--> RegisterAll --> BuildActionCatalog --> RegisterIndividualCatalogTools
//	    |
//	    +--> BuildActionCatalog --> RegisterMetaCatalog
//	    |
//	    +--> BuildActionCatalog --> dynamic.RegisterCatalogFindExecuteTools
//
// [RegisterAll] registers the individual tools by projecting the canonical
// action catalog. [BuildActionCatalog] builds the catalog used by
// [RegisterIndividualCatalogTools], [RegisterMetaCatalog], and dynamic mode.
// [RegisterAllMeta] preserves the meta registration entry point by building and
// registering that catalog. [SafeModePreview] describes the preview payload
// returned when safe mode intercepts mutating calls.
//
// Domain packages document the official GitLab API pages they wrap. Keeping
// those references in package documentation preserves pkgsite discoverability
// without adding fields to MCP tool schemas or dynamic discovery responses.
package tools
