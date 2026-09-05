// Package config loads, normalizes, and validates runtime configuration for the
// GitLab MCP server.
//
// Configuration comes from environment variables, .env files, and CLI flags in
// cmd/server. The package centralizes defaults and bounds for stdio mode, HTTP
// mode, OAuth token verification, upload limits, safe
// mode, read-only mode, rate limiting, tool surfaces, capability surfaces, and
// meta-tool schema detail.
//
// # Validation Model
//
// Loaders keep user-facing configuration forgiving while preserving hard bounds
// that protect runtime behavior: URL values are parsed and normalized, duration
// and size fields are clamped to documented limits, and invalid enum values are
// reported before server registration begins.
package config
