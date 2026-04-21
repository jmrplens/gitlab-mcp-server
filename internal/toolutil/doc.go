// Package toolutil provides shared utilities for MCP tool handler sub-packages.
// It contains error handling, pagination, logging, text processing, markdown
// formatting helpers, and meta-tool infrastructure used across all domain
// sub-packages under internal/tools/.
//
// This package must never import from domain sub-packages to prevent circular
// dependencies. The dependency direction is: domain sub-packages → toolutil.
package toolutil
