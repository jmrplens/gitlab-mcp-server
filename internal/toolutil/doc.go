// Package toolutil provides shared utilities for MCP tool handler sub-packages.
// It contains error handling, pagination, logging, text processing, Markdown
// formatting helpers, and meta-tool infrastructure used across all domain
// sub-packages under internal/tools/.
//
// The package centralizes cross-cutting behavior for:
//
//   - MCP response construction, annotations, icons, embedded resources, and
//     Markdown formatter registration.
//   - GitLab API support types for pagination, GraphQL cursors, access levels,
//     diff positions, file validation, and flexible string-or-integer IDs.
//   - Operational helpers for structured errors, not-found results, logging,
//     rate limiting, polling, destructive-action confirmation, and identity
//     resolution.
//   - Schema middleware that hardens generated tool schemas and enriches common
//     pagination parameters with numeric bounds.
//
// This package must never import from domain sub-packages to prevent circular
// dependencies. The dependency direction is: domain sub-packages → toolutil.
package toolutil
