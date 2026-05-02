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
// dependencies. The dependency direction is: domain sub-packages -> toolutil.
//
// # Common Entry Points
//
// Handlers usually use [WrapErr], [WrapErrWithMessage], or [WrapErrWithHint]
// for errors; [PaginationInput] and [PaginationOutput] for paginated endpoints;
// [StringOrInt] for GitLab IDs that may be numeric or path-based; and
// [RegisterMarkdown] or [RegisterMarkdownResult] to publish type-specific
// Markdown renderers.
//
// Destructive tools use [ConfirmAction] when they need an MCP elicitation step,
// and list/detail outputs commonly embed [HintableOutput] so meta-tools can add
// next-step guidance.
//
// # Dependency Direction
//
// The package dependency shape is intentionally one-way:
//
//	internal/tools/{domain}
//	    |
//	    v
//	toolutil
//	    |
//	    v
//	MCP SDK and GitLab client primitives
package toolutil
