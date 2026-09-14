// Package tools provides the MCP tool orchestration layer for the GitLab MCP
// server.
//
// The package wires the individual, meta, and dynamic GitLab MCP tool surfaces
// to the server. It delegates domain implementations to internal/tools/{domain}
// sub-packages, builds the canonical action catalog for catalog-backed
// surfaces, exposes the gitlab_server meta-tool, applies read-only and safe
// mode behavior, filters tools by personal access token scopes, and delegates
// meta-tool Markdown rendering to the type-based registry in internal/toolutil.
//
// # Catalog-first registration
//
// The package follows the catalog-first registration model documented in
// ADR-0014. The canonical [actioncatalog.Catalog] produced by
// [BuildActionCatalog] is the single source of truth for ordinary GitLab API
// actions and feeds every catalog-backed surface:
//
//   - meta tools registered by [RegisterMetaCatalog]
//   - individual tools projected by [RegisterIndividualCatalogTools]
//   - the dynamic find/execute registry in internal/tools/dynamic
//   - the gitlab://tools schema resource in internal/resources
//   - audits, LLM files, and the model-eval surfaces
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
// [RegisterIndividualCatalogTools], [RegisterMetaCatalog], and dynamic
// mode. There is deliberately no one-call entry point for the meta surface:
// the RegisterAllMeta that used to be one built its catalog without
// [ActionCatalogOptions].IncludeMCP and so registered one tool fewer than the
// binary serves, which is how the published meta counts came to be off by one
// (issue 616). A caller registers the meta surface the way cmd/server does,
// with [RegisterMetaCatalog] and [RegisterMetaStandaloneTools] over a catalog
// it built itself. [SafeModePreview] describes the preview payload returned
// when safe mode intercepts mutating calls.
//
// Domain packages document the official GitLab API pages they wrap. Keeping
// those references in package documentation preserves pkgsite
// discoverability without adding fields to MCP tool schemas or dynamic
// discovery responses.
package tools
