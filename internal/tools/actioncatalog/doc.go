// Package actioncatalog provides the canonical GitLab action catalog shared by
// catalog-backed MCP tool surfaces.
//
// The catalog is the intermediate action core between typed GitLab handlers and
// the public meta and dynamic tool surfaces. It stores executable actions as
// deterministic groups, preserving route metadata such as input schemas, output
// schemas, destructive flags, action aliases, tags, icons, descriptions, and
// formatter hooks.
//
// Action IDs use the stable domain.action form derived from the backing
// meta-tool name and action name. For example, gitlab_project/create becomes
// project.create, and gitlab_merge_request/list becomes merge_request.list.
// Dynamic mode uses these IDs directly, while meta-tools expose the same routes
// through action dispatch inside domain tools.
//
// This package is not the registry for individual MCP tools. Individual tools
// are still registered directly by internal/tools.RegisterAll for compatibility.
// Meta-tools and dynamic tools consume this catalog through adapters such as
// internal/tools.RegisterMetaCatalog and internal/tools/dynamic.NewRegistryFromCatalog.
//
// # Invariants
//
// Catalog construction must be deterministic. Groups preserve explicit action
// order for user-facing descriptions, cloning avoids mutable alias/schema
// sharing between surfaces, and validation rejects duplicate action IDs or
// ambiguous aliases before tools are registered.
package actioncatalog
