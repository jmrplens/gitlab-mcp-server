// Package dynamic implements low-token GitLab MCP tool surfaces over the
// canonical action catalog.
//
// Dynamic mode exposes a small discovery and execution interface instead of
// advertising every GitLab operation as an MCP tool. It registers
// gitlab_find_action and gitlab_execute_action.
//
// The package builds a deterministic search index from actioncatalog.Catalog,
// resolves canonical domain.action IDs and aliases, returns exact schemas on
// demand, and dispatches execution through the same ActionRoute metadata used
// by meta-tools. It does not wrap or call the visible individual MCP tools.
//
// # Token Budget
//
// Dynamic mode is intentionally sparse: find returns ranked action summaries
// with exact schemas, and execute routes one canonical action. Descriptive
// package and API-reference documentation belongs in Go package comments and
// project docs, not in dynamic response fields.
//
// # Safety
//
// Execution reuses catalog metadata for destructive flags, read-only filtering,
// safe-mode previews, compatibility aliases, parameter alias normalization, and
// schema lookup. This keeps dynamic behavior aligned with meta-tools without a
// second action registry.
package dynamic
